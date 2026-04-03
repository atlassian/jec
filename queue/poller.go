package queue

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/atlassian/jec/conf"
	"github.com/atlassian/jec/retryer"
	"github.com/atlassian/jec/util"
	"github.com/atlassian/jec/worker_pool"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

const messagesPath = "/jsm/ops/jec/v2/messages/channel/"

type Poller interface {
	Processor
	ChannelId() string
}

type poller struct {
	workerPool     worker_pool.WorkerPool
	messageHandler MessageHandler
	retryer        *retryer.Retryer

	channelId          string
	conf               *conf.Configuration
	queueMessageLogrus *logrus.Logger
	registrator        *messageRegistrator

	isRunning   bool
	isRunningWg *sync.WaitGroup
	startStopMu *sync.Mutex
	quit        chan struct{}
	wakeUp      chan struct{}
}

func NewPoller(workerPool worker_pool.WorkerPool,
	messageHandler MessageHandler,
	conf *conf.Configuration,
	channelId string) Poller {

	return &poller{
		workerPool:         workerPool,
		messageHandler:     messageHandler,
		retryer:            &retryer.Retryer{},
		channelId:          channelId,
		conf:               conf,
		queueMessageLogrus: newQueueMessageLogrus(channelId),
		registrator:        newMessageRegistrator(),
		isRunning:          false,
		isRunningWg:        &sync.WaitGroup{},
		startStopMu:        &sync.Mutex{},
		quit:               make(chan struct{}),
		wakeUp:             make(chan struct{}),
	}
}

func (p *poller) ChannelId() string {
	return p.channelId
}

func (p *poller) Start() error {
	defer p.startStopMu.Unlock()
	p.startStopMu.Lock()

	if p.isRunning {
		return errors.New("Poller is already running.")
	}

	p.isRunningWg.Add(1)
	go p.run()

	p.isRunning = true

	return nil
}

func (p *poller) Stop() error {
	defer p.startStopMu.Unlock()
	p.startStopMu.Lock()

	if !p.isRunning {
		return errors.New("Poller is not running.")
	}

	close(p.quit)
	close(p.wakeUp)

	p.isRunningWg.Wait()
	p.isRunning = false

	return nil
}

func (p *poller) fetchMessages() ([]*Message, error) {

	url := fmt.Sprintf("%s%s%s", p.conf.BaseUrl, messagesPath, p.channelId)

	request, err := retryer.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	request.Header.Add("Authorization", "GenieKey "+p.conf.ApiKey)
	request.Header.Add("X-JEC-Client-Info", UserAgentHeader)

	response, err := p.retryer.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(response.Body)
		return nil, errors.Errorf("Failed to fetch messages from channel[%s], status: %s, message: %s", p.channelId, response.Status, body)
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	var messages []*Message
	err = json.Unmarshal(body, &messages)
	if err != nil {
		return nil, err
	}

	return messages, nil
}

func (p *poller) poll() (shouldWait bool) {

	availableWorkerCount := p.workerPool.NumberOfAvailableWorker()
	if !(availableWorkerCount > 0) {
		return true
	}

	messages, err := p.fetchMessages()
	if err != nil {
		logrus.Errorf("Poller[%s] could not fetch messages: %s", p.channelId, err.Error())
		return true
	}

	messageLength := len(messages)
	if messageLength == 0 {
		logrus.Tracef("There is no new message in channel[%s].", p.channelId)
		return true
	}

	logrus.Debugf("Received %d messages from channel[%s].", messageLength, p.channelId)

	// Add all fetched messages to registrator
	for i := 0; i < messageLength; i++ {
		p.registrator.Add(messages[i].MessageId, messages[i].MessageHandle)
	}

	// Submit messages (with dedup and tracking)
	for i := 0; i < messageLength; i++ {
		// Skip if already processed (dedup)
		if p.registrator.IsProcessed(messages[i].MessageId) {
			logrus.Debugf("Message[%s] already processed, skipping.", messages[i].MessageId)
			continue
		}

		p.queueMessageLogrus.
			WithField("messageId", messages[i].MessageId).
			Info("Message body: ", messages[i].Body)

		job := newJob(
			p.messageHandler,
			*messages[i],
			p.conf.ApiKey,
			p.conf.BaseUrl,
		)

		isSubmitted, err := p.workerPool.Submit(job)
		if err != nil {
			logrus.Debugf("Error occurred while submitting: %s.", err.Error())
			return true
		} else if !isSubmitted {
			logrus.Debugf("Job[%s] could not be submitted.", messages[i].MessageId)
		} else {
			// Mark as processed only if successfully submitted
			p.registrator.MarkProcessed(messages[i].MessageId)
		}
	}

	// Check if all messages are processed
	if p.registrator.AllProcessed() {
		err := deleteMessages(p.conf.BaseUrl, p.conf.ApiKey, p.channelId, p.retryer, p.registrator.ProcessedEntries())
		if err != nil {
			logrus.Errorf("Failed to delete messages from channel[%s]: %s", p.channelId, err.Error())
		} else {
			p.registrator.Reset()
		}
	}

	return false
}

func (p *poller) wait(pollingWaitInterval time.Duration) {

	logrus.Tracef("Poller[%s] will wait %s before next polling", p.channelId, pollingWaitInterval.String())

	ticker := time.NewTicker(pollingWaitInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.wakeUp:
			logrus.Debugf("Poller[%s] has been interrupted while waiting for next polling.", p.channelId)
			return
		case <-ticker.C:
			return
		}
	}
}

func (p *poller) run() {

	logrus.Infof("Poller[%s] has started to run.", p.channelId)

	pollingWaitInterval := p.conf.PollerConf.PollingWaitIntervalInMillis * time.Millisecond

	for {
		select {
		case <-p.quit:
			logrus.Infof("Poller[%s] has stopped to poll.", p.channelId)
			p.isRunningWg.Done()
			return
		default:
			if shouldWait := p.poll(); shouldWait {
				p.wait(pollingWaitInterval)
			}
		}
	}
}

func newQueueMessageLogrus(channelId string) *logrus.Logger {
	logFilePath := filepath.Join("/var", "log", "jec", "jecQueueMessages-"+channelId+"-"+strconv.Itoa(os.Getpid())+".log")
	queueMessageLogger := &lumberjack.Logger{
		Filename:  logFilePath,
		MaxSize:   3,  // MB
		MaxAge:    10, // Days
		LocalTime: true,
	}

	queueMessageLogrus := logrus.New()
	queueMessageLogrus.SetFormatter(conf.PrepareLogFormat())

	err := queueMessageLogger.Rotate()
	if err != nil {
		logrus.Info("Cannot create log file for queueMessages. Reason: ", err)
	}

	queueMessageLogrus.SetOutput(queueMessageLogger)

	go util.CheckLogFile(queueMessageLogger, time.Second*10)

	return queueMessageLogrus
}
