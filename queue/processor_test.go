package queue

import (
	"bytes"
	"encoding/json"
	"github.com/atlassian/jec/conf"
	"github.com/atlassian/jec/git"
	"github.com/atlassian/jec/retryer"
	"github.com/atlassian/jec/worker_pool"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/http"
	"sync"
	"testing"
	"time"
)

var mockConf = &conf.Configuration{
	ApiKey:     "ApiKey",
	PollerConf: *mockPollerConf,
	PoolConf:   *mockPoolConf,
}

var mockPoolConf = &conf.PoolConf{
	MaxNumberOfWorker: 16,
	MinNumberOfWorker: 2,
}

func newQueueProcessorTest() *processor {

	return &processor{
		workerPool:    NewMockWorkerPool(),
		configuration: mockConf,
		repositories:  git.NewRepositories(),
		quit:          make(chan struct{}),
		isRunning:     false,
		isRunningWg:   &sync.WaitGroup{},
		startStopMu:   &sync.Mutex{},
		retryer:       &retryer.Retryer{},
	}
}

func mockAuthenticateSuccess(r *retryer.Retryer, request *retryer.Request) (*http.Response, error) {
	authResp := authenticateResponse{
		ChannelId: mockChannelId,
		OwnerId:   "mockOwnerId",
	}
	body, _ := json.Marshal(authResp)
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       ioutil.NopCloser(bytes.NewReader(body)),
	}, nil
}

func mockAuthenticateError(r *retryer.Retryer, request *retryer.Request) (*http.Response, error) {
	return nil, errors.New("Test http error has occurred while authenticating.")
}

func TestValidateNewQueueProcessor(t *testing.T) {
	configuration := &conf.Configuration{}
	processor := NewProcessor(configuration).(*processor)

	assert.Equal(t, int64(maxNumberOfMessages), processor.configuration.PollerConf.MaxNumberOfMessages)
	assert.Equal(t, int64(visibilityTimeoutInSec), processor.configuration.PollerConf.VisibilityTimeoutInSeconds)
	assert.Equal(t, time.Duration(pollingWaitIntervalInMillis), processor.configuration.PollerConf.PollingWaitIntervalInMillis)
}

func TestStartAndStopQueueProcessor(t *testing.T) {

	defer func() {
		newPollerFunc = NewPoller
	}()

	processor := newQueueProcessorTest()

	processor.retryer.DoFunc = mockAuthenticateSuccess
	newPollerFunc = NewMockPollerForQueueProcessor

	err := processor.Start()
	assert.Nil(t, err)
	assert.True(t, processor.isRunning)
	assert.NotNil(t, processor.poller)

	err = processor.Stop()
	assert.Nil(t, err)
	assert.False(t, processor.isRunning)
}

func TestStartQueueProcessorAuthenticationError(t *testing.T) {

	defer func() {
		newPollerFunc = NewPoller
	}()

	processor := newQueueProcessorTest()

	processor.retryer.DoFunc = mockAuthenticateError
	newPollerFunc = NewMockPollerForQueueProcessor

	err := processor.Start()

	assert.NotNil(t, err)
	assert.Equal(t, "Test http error has occurred while authenticating.", err.Error())
}

func TestStopQueueProcessorWhileNotRunning(t *testing.T) {

	processor := newQueueProcessorTest()

	err := processor.Stop()

	assert.NotNil(t, err)
	assert.Equal(t, "Queue processor is not running.", err.Error())
}

func TestAuthenticate(t *testing.T) {

	processor := newQueueProcessorTest()

	var actualRequest *http.Request

	processor.retryer.DoFunc = func(r *retryer.Retryer, request *retryer.Request) (*http.Response, error) {
		actualRequest = request.Request
		return mockAuthenticateSuccess(r, request)
	}

	authResp, err := processor.authenticate()

	assert.Nil(t, err)
	assert.Equal(t, mockChannelId, authResp.ChannelId)
	assert.Contains(t, actualRequest.URL.Path, authenticatePath)
}

func TestAuthenticateError(t *testing.T) {

	processor := newQueueProcessorTest()
	processor.retryer.DoFunc = mockAuthenticateError

	_, err := processor.authenticate()

	assert.NotNil(t, err)
	assert.Equal(t, "Test http error has occurred while authenticating.", err.Error())
}

func TestAuthenticateRequestError(t *testing.T) {

	processor := newQueueProcessorTest()
	processor.configuration = &conf.Configuration{
		BaseUrl: "invalid",
	}

	_, err := processor.authenticate()

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "unsupported protocol scheme")
}

// Mock QueueProcessor
type MockQueueProcessor struct {
	StartProcessingFunc func() error
	StopProcessingFunc  func() error
	IsRunningFunc       func() bool
	WaitFunc            func()
}

func NewMockQueueProcessor() *MockQueueProcessor {
	return &MockQueueProcessor{}
}

func (m *MockQueueProcessor) StartProcessing() error {
	if m.StartProcessingFunc != nil {
		return m.StartProcessingFunc()
	}
	return nil
}

func (m *MockQueueProcessor) StopProcessing() error {
	if m.StopProcessingFunc != nil {
		return m.StopProcessingFunc()
	}
	return nil
}

func (m *MockQueueProcessor) IsRunning() bool {
	if m.IsRunningFunc != nil {
		return m.IsRunningFunc()
	}
	return false
}

func (m *MockQueueProcessor) Wait() {
	if m.WaitFunc != nil {
		m.WaitFunc()
	}
}

// Mock Worker Pool
type MockWorkerPool struct {
	NumberOfAvailableWorkerFunc func() int32
	StartFunc                   func() error
	StopFunc                    func() error
	SubmitFunc                  func(worker_pool.Job) (bool, error)
}

func NewMockWorkerPool() *MockWorkerPool {
	return &MockWorkerPool{}
}

func (m *MockWorkerPool) NumberOfAvailableWorker() int32 {
	if m.NumberOfAvailableWorkerFunc != nil {
		return m.NumberOfAvailableWorkerFunc()
	}
	return 0
}

func (m *MockWorkerPool) Start() error {
	if m.StartFunc != nil {
		return m.StartFunc()
	}
	return nil
}

func (m *MockWorkerPool) Stop() error {
	if m.StopFunc != nil {
		return m.StopFunc()
	}
	return nil
}

func (m *MockWorkerPool) Submit(job worker_pool.Job) (bool, error) {
	if m.SubmitFunc != nil {
		return m.SubmitFunc(job)
	}
	return false, nil
}
