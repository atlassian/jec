package queue

import (
	"encoding/json"
	"github.com/atlassian/jec/conf"
	"github.com/atlassian/jec/git"
	"github.com/atlassian/jec/retryer"
	"github.com/atlassian/jec/worker_pool"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
)

var UserAgentHeader string

const (
	pollingWaitIntervalInMillis = 1000
	maxNumberOfMessages         = 10

	repositoryRefreshPeriod = time.Minute
)

const authenticatePath = "/jsm/ops/jec/v1/authenticate"

type authenticateResponse struct {
	ChannelId string `json:"channelId"`
	OwnerId   string `json:"ownerId"`
	OwnerType string `json:"ownerType"`
}

var newPollerFunc = NewPoller

type Processor interface {
	Start() error
	Stop() error
}

type processor struct {
	workerPool worker_pool.WorkerPool
	poller     Poller

	retryer *retryer.Retryer

	configuration *conf.Configuration
	repositories  git.Repositories
	actionLoggers map[string]io.Writer

	isRunning   bool
	isRunningWg *sync.WaitGroup
	startStopMu *sync.Mutex
	quit        chan struct{}
}

func NewProcessor(conf *conf.Configuration) Processor {

	if conf.PollerConf.MaxNumberOfMessages <= 0 {
		logrus.Infof("Max number of messages should be greater than 0, default value[%d] is set.", maxNumberOfMessages)
		conf.PollerConf.MaxNumberOfMessages = maxNumberOfMessages
	}

	if conf.PollerConf.PollingWaitIntervalInMillis <= 1000 {
		logrus.Infof("Polling wait interval should be equal or greater than 1000ms, default value[%d ms.] is set.", pollingWaitIntervalInMillis)
		conf.PollerConf.PollingWaitIntervalInMillis = pollingWaitIntervalInMillis
	}

	return &processor{
		workerPool:    worker_pool.New(&conf.PoolConf),
		configuration: conf,
		repositories:  git.NewRepositories(),
		actionLoggers: newActionLoggers(conf.ActionMappings),
		quit:          make(chan struct{}),
		isRunning:     false,
		isRunningWg:   &sync.WaitGroup{},
		startStopMu:   &sync.Mutex{},
		retryer:       &retryer.Retryer{},
	}
}

func (qp *processor) Start() error {
	defer qp.startStopMu.Unlock()
	qp.startStopMu.Lock()

	if qp.isRunning {
		return errors.New("Queue processor is already running.")
	}

	logrus.Infof("Queue processor is starting.")

	authResp, err := qp.authenticate()
	if err != nil {
		logrus.Errorf("Queue processor could not authenticate and will terminate.")
		return err
	}

	err = qp.repositories.DownloadAll(qp.configuration.ActionMappings.GitActions())
	if err != nil {
		logrus.Errorf("Queue processor could not clone a git repository and will terminate.")
		return err
	}

	if qp.repositories.NotEmpty() {
		qp.isRunningWg.Add(1)
		go qp.startPullingRepositories(repositoryRefreshPeriod)

		conf.AddRepositoryPathToGitActionFilepaths(qp.configuration.ActionMappings, qp.repositories)
	}

	qp.workerPool.Start()

	messageHandler := &messageHandler{
		repositories:  qp.repositories,
		actionSpecs:   qp.configuration.ActionSpecifications,
		actionLoggers: qp.actionLoggers,
	}

	qp.poller = newPollerFunc(
		qp.workerPool,
		messageHandler,
		qp.configuration,
		authResp.ChannelId,
	)
	qp.poller.Start()

	qp.isRunning = true
	logrus.Infof("Queue processor has started.")
	return nil
}

func (qp *processor) Stop() error {
	defer qp.startStopMu.Unlock()
	qp.startStopMu.Lock()

	if !qp.isRunning {
		return errors.New("Queue processor is not running.")
	}

	logrus.Infof("Queue processor is stopping.")

	close(qp.quit)
	qp.isRunningWg.Wait()

	if qp.poller != nil {
		qp.poller.Stop()
	}

	qp.workerPool.Stop()
	qp.repositories.RemoveAll()

	qp.isRunning = false
	logrus.Infof("Queue processor has stopped.")
	return nil
}

func (qp *processor) authenticate() (*authenticateResponse, error) {

	url := qp.configuration.BaseUrl + authenticatePath

	request, err := retryer.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	request.Header.Add("Authorization", "GenieKey "+qp.configuration.ApiKey)
	request.Header.Add("X-JEC-Client-Info", UserAgentHeader)

	response, err := qp.retryer.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(response.Body)
		return nil, errors.Errorf("Token could not be received from Jira Service Management, status: %s, message: %s", response.Status, body)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	authResp := &authenticateResponse{}
	err = json.Unmarshal(body, authResp)
	if err != nil {
		return nil, err
	}

	logrus.Infof("Successfully authenticated. ChannelId: %s, OwnerId: %s, OwnerType: %s", authResp.ChannelId, authResp.OwnerId, authResp.OwnerType)
	return authResp, nil
}

func (qp *processor) startPullingRepositories(pullPeriod time.Duration) {

	logrus.Infof("Repositories will be updated in every %s.", pullPeriod.String())

	ticker := time.NewTicker(pullPeriod)

	for {
		select {
		case <-qp.quit:
			ticker.Stop()
			logrus.Info("All git repositories will be removed.")
			qp.isRunningWg.Done()
			return
		case <-ticker.C:
			ticker.Stop()
			qp.repositories.PullAll()
			ticker = time.NewTicker(pullPeriod)
		}
	}
}

func newActionLoggers(mappings conf.ActionMappings) map[string]io.Writer {
	actionLoggers := make(map[string]io.Writer)
	for _, action := range mappings {
		if action.Stdout != "" {
			if _, ok := actionLoggers[action.Stdout]; !ok {
				actionLoggers[action.Stdout] = newLogger(action.Stdout)
			}
		}
		if action.Stderr != "" {
			if _, ok := actionLoggers[action.Stderr]; !ok {
				actionLoggers[action.Stderr] = newLogger(action.Stderr)
			}
		}
	}
	return actionLoggers
}

func newLogger(filename string) *lumberjack.Logger {
	return &lumberjack.Logger{
		Filename:  filename,
		MaxSize:   3, // MB
		MaxAge:    1, // Days
		LocalTime: true,
	}
}
