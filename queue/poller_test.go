package queue

import (
	"bytes"
	"encoding/json"
	"github.com/atlassian/jec/conf"
	"github.com/atlassian/jec/retryer"
	"github.com/atlassian/jec/worker_pool"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/http"
	"strconv"
	"sync"
	"testing"
)

var mockPollerConf = &conf.PollerConf{
	PollingWaitIntervalInMillis: pollingWaitIntervalInMillis,
	VisibilityTimeoutInSeconds:  visibilityTimeoutInSec,
	MaxNumberOfMessages:         maxNumberOfMessages,
}

func newPollerTest() *poller {
	return &poller{
		quit:        make(chan struct{}),
		wakeUp:      make(chan struct{}),
		isRunning:   false,
		isRunningWg: &sync.WaitGroup{},
		startStopMu: &sync.Mutex{},
		conf: &conf.Configuration{
			ApiKey:               mockApiKey,
			BaseUrl:              mockBaseUrl,
			PollerConf:           *mockPollerConf,
			ActionSpecifications: mockActionSpecs,
		},
		channelId:          mockChannelId,
		workerPool:         NewMockWorkerPool(),
		messageHandler:     NewMockMessageHandler(),
		retryer:            &retryer.Retryer{},
		queueMessageLogrus: &logrus.Logger{},
	}
}

func mockFetchMessages(messages []*JECMessage, err error) func(r *retryer.Retryer, request *retryer.Request) (*http.Response, error) {
	return func(r *retryer.Retryer, request *retryer.Request) (*http.Response, error) {
		if err != nil {
			return nil, err
		}
		body, _ := json.Marshal(messages)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       ioutil.NopCloser(bytes.NewReader(body)),
		}, nil
	}
}

func TestStartAndStopPolling(t *testing.T) {

	poller := newPollerTest()

	err := poller.Start()
	assert.Nil(t, err)
	assert.Equal(t, true, poller.isRunning)

	err = poller.Start()
	assert.NotNil(t, err)
	assert.Equal(t, "Poller is already running.", err.Error())
	assert.Equal(t, true, poller.isRunning)

	err = poller.Stop()
	assert.Nil(t, err)
	assert.Equal(t, false, poller.isRunning)
}

func TestStopPollingNonPollingState(t *testing.T) {

	poller := newPollerTest()

	err := poller.Stop()
	assert.NotNil(t, err)
	assert.Equal(t, "Poller is not running.", err.Error())
}

func TestPollWithNoAvailableWorker(t *testing.T) {

	poller := newPollerTest()

	poller.workerPool.(*MockWorkerPool).NumberOfAvailableWorkerFunc = func() int32 {
		return 0
	}

	shouldWait := poller.poll()
	assert.True(t, shouldWait)
}

func TestPollWithReceiveError(t *testing.T) {

	poller := newPollerTest()

	poller.workerPool.(*MockWorkerPool).NumberOfAvailableWorkerFunc = func() int32 {
		return 1
	}
	poller.retryer.DoFunc = mockFetchMessages(nil, errors.New(""))

	shouldWait := poller.poll()
	assert.True(t, shouldWait)
}

func TestPollZeroMessage(t *testing.T) {

	poller := newPollerTest()

	poller.workerPool.(*MockWorkerPool).NumberOfAvailableWorkerFunc = func() int32 {
		return 1
	}
	poller.retryer.DoFunc = mockFetchMessages([]*JECMessage{}, nil)

	logrus.SetLevel(logrus.DebugLevel)
	shouldWait := poller.poll()
	assert.True(t, shouldWait)
}

func TestPollMaxMessage(t *testing.T) {

	poller := newPollerTest()

	expected := int64(4)

	poller.workerPool.(*MockWorkerPool).NumberOfAvailableWorkerFunc = func() int32 {
		return int32(expected)
	}

	messages := make([]*JECMessage, expected)
	for i := int64(0); i < expected; i++ {
		messages[i] = &JECMessage{
			MessageId: strconv.FormatInt(i, 10),
			Body:      "body",
			ChannelId: mockChannelId,
		}
	}
	poller.retryer.DoFunc = mockFetchMessages(messages, nil)

	submitCount := 0
	poller.workerPool.(*MockWorkerPool).SubmitFunc = func(job worker_pool.Job) (bool, error) {
		submitCount++
		return true, nil
	}

	shouldWait := poller.poll()
	assert.False(t, shouldWait)
	assert.Equal(t, int(expected), submitCount)
}

func TestPollMaxMessageUpperBound(t *testing.T) {

	poller := newPollerTest()

	availableWorkerCount := int64(12)

	poller.workerPool.(*MockWorkerPool).NumberOfAvailableWorkerFunc = func() int32 {
		return int32(availableWorkerCount)
	}

	// API returns more messages than maxNumberOfMessages, poller should cap
	messages := make([]*JECMessage, 20)
	for i := 0; i < 20; i++ {
		messages[i] = &JECMessage{
			MessageId: strconv.FormatInt(int64(i), 10),
			Body:      "body",
			ChannelId: mockChannelId,
		}
	}
	poller.retryer.DoFunc = mockFetchMessages(messages, nil)

	submitCount := 0
	poller.workerPool.(*MockWorkerPool).SubmitFunc = func(job worker_pool.Job) (bool, error) {
		submitCount++
		return true, nil
	}

	shouldWait := poller.poll()
	assert.False(t, shouldWait)
	assert.Equal(t, int(poller.conf.PollerConf.MaxNumberOfMessages), submitCount)
}

func TestPollMessageSubmitFail(t *testing.T) {

	poller := newPollerTest()

	expected := int64(4)

	poller.workerPool.(*MockWorkerPool).NumberOfAvailableWorkerFunc = func() int32 {
		return int32(expected)
	}

	messages := make([]*JECMessage, expected)
	for i := int64(0); i < expected; i++ {
		messages[i] = &JECMessage{
			MessageId: strconv.FormatInt(i, 10),
			Body:      "body",
			ChannelId: mockChannelId,
		}
	}
	poller.retryer.DoFunc = mockFetchMessages(messages, nil)

	submitCount := 0
	poller.workerPool.(*MockWorkerPool).SubmitFunc = func(job worker_pool.Job) (bool, error) {
		submitCount++
		return false, nil
	}

	shouldWait := poller.poll()

	assert.False(t, shouldWait)
	assert.Equal(t, int(expected), submitCount)
}

func TestPollMessageSubmitError(t *testing.T) {

	poller := newPollerTest()

	expected := int64(5)

	poller.workerPool.(*MockWorkerPool).NumberOfAvailableWorkerFunc = func() int32 {
		return int32(expected)
	}

	messages := make([]*JECMessage, expected)
	for i := int64(0); i < expected; i++ {
		messages[i] = &JECMessage{
			MessageId: strconv.FormatInt(i, 10),
			Body:      "body",
			ChannelId: mockChannelId,
		}
	}
	poller.retryer.DoFunc = mockFetchMessages(messages, nil)

	submitCount := 0
	poller.workerPool.(*MockWorkerPool).SubmitFunc = func(job worker_pool.Job) (bool, error) {
		submitCount++
		return false, errors.New("Submit Error")
	}

	shouldWait := poller.poll()

	assert.True(t, shouldWait)
	assert.Equal(t, 1, submitCount)
}

func TestPollMessageSubmitSuccess(t *testing.T) {

	poller := newPollerTest()

	poller.workerPool.(*MockWorkerPool).NumberOfAvailableWorkerFunc = func() int32 {
		return 5
	}

	messages := make([]*JECMessage, 5)
	for i := 0; i < 5; i++ {
		messages[i] = &JECMessage{
			MessageId: strconv.FormatInt(int64(i), 10),
			Body:      "body",
			ChannelId: mockChannelId,
		}
	}
	poller.retryer.DoFunc = mockFetchMessages(messages, nil)

	poller.workerPool.(*MockWorkerPool).SubmitFunc = func(job worker_pool.Job) (bool, error) {
		return true, nil
	}

	shouldWait := poller.poll()

	assert.False(t, shouldWait)
}

// Mock Poller
type MockPoller struct {
	StartPollingFunc func() error
	StopPollingFunc  func() error
	ChannelIdFunc    func() string
}

func NewMockPoller() Poller {
	return &MockPoller{}
}

func NewMockPollerForQueueProcessor(workerPool worker_pool.WorkerPool,
	messageHandler MessageHandler, conf *conf.Configuration, channelId string) Poller {
	return NewMockPoller()
}

func (p *MockPoller) Start() error {
	if p.StartPollingFunc != nil {
		return p.StartPollingFunc()
	}
	return nil
}

func (p *MockPoller) Stop() error {
	if p.StopPollingFunc != nil {
		return p.StopPollingFunc()
	}
	return nil
}

func (p *MockPoller) ChannelId() string {
	if p.ChannelIdFunc != nil {
		return p.ChannelIdFunc()
	}
	return mockChannelId
}
