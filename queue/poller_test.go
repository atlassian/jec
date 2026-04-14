package queue

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"sync"
	"testing"

	"github.com/atlassian/jec/conf"
	"github.com/atlassian/jec/retryer"
	"github.com/atlassian/jec/worker_pool"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var mockPollerConf = &conf.PollerConf{
	PollingWaitIntervalInMillis: pollingWaitIntervalInMillis,
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
		tracker:            newMessageTracker(),
	}
}

func mockFetchMessages(messages []*Message, err error) func(r *retryer.Retryer, request *retryer.Request) (*http.Response, error) {
	return func(r *retryer.Retryer, request *retryer.Request) (*http.Response, error) {
		if err != nil {
			return nil, err
		}
		body, _ := json.Marshal(MessageResponse{ChannelId: mockChannelId, Messages: messages})
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

// TestPollWithNoAvailableWorker removed - no longer checks available workers before fetching

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
	poller.retryer.DoFunc = mockFetchMessages([]*Message{}, nil)

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

	messages := make([]*Message, expected)
	for i := int64(0); i < expected; i++ {
		messages[i] = &Message{
			Id:            strconv.FormatInt(i, 10),
			ReceiptHandle: "handle-" + strconv.FormatInt(i, 10),
			Body:          "body",
		}
	}

	deleteCallCount := 0
	poller.retryer.DoFunc = func(r *retryer.Retryer, request *retryer.Request) (*http.Response, error) {
		if request.Method == http.MethodDelete {
			deleteCallCount++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(bytes.NewReader([]byte(""))),
			}, nil
		}
		// Fetch messages
		body, _ := json.Marshal(MessageResponse{ChannelId: mockChannelId, Messages: messages})
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       ioutil.NopCloser(bytes.NewReader(body)),
		}, nil
	}

	submitCount := 0
	poller.workerPool.(*MockWorkerPool).SubmitFunc = func(job worker_pool.Job) (bool, error) {
		submitCount++
		return true, nil
	}

	shouldWait := poller.poll()
	assert.False(t, shouldWait)
	assert.Equal(t, int(expected), submitCount)
	assert.Equal(t, 1, deleteCallCount) // All messages submitted, so delete should be called
}

func TestPollFetchAllMessages(t *testing.T) {

	poller := newPollerTest()

	availableWorkerCount := int64(12)

	poller.workerPool.(*MockWorkerPool).NumberOfAvailableWorkerFunc = func() int32 {
		return int32(availableWorkerCount)
	}

	// API returns more messages - poller should fetch all (no capping)
	messageCount := 20
	messages := make([]*Message, messageCount)
	for i := 0; i < messageCount; i++ {
		messages[i] = &Message{
			Id:            strconv.FormatInt(int64(i), 10),
			ReceiptHandle: "handle-" + strconv.FormatInt(int64(i), 10),
			Body:          "body",
		}
	}

	deleteCallCount := 0
	poller.retryer.DoFunc = func(r *retryer.Retryer, request *retryer.Request) (*http.Response, error) {
		if request.Method == http.MethodDelete {
			deleteCallCount++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(bytes.NewReader([]byte(""))),
			}, nil
		}
		// Fetch messages
		body, _ := json.Marshal(MessageResponse{ChannelId: mockChannelId, Messages: messages})
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       ioutil.NopCloser(bytes.NewReader(body)),
		}, nil
	}

	submitCount := 0
	poller.workerPool.(*MockWorkerPool).SubmitFunc = func(job worker_pool.Job) (bool, error) {
		submitCount++
		return true, nil
	}

	shouldWait := poller.poll()
	assert.False(t, shouldWait)
	assert.Equal(t, messageCount, submitCount) // All messages should be submitted
	assert.Equal(t, 1, deleteCallCount)        // Delete should be called
}

func TestPollMessageSubmitFail(t *testing.T) {

	poller := newPollerTest()

	expected := int64(4)

	poller.workerPool.(*MockWorkerPool).NumberOfAvailableWorkerFunc = func() int32 {
		return int32(expected)
	}

	messages := make([]*Message, expected)
	for i := int64(0); i < expected; i++ {
		messages[i] = &Message{
			Id:            strconv.FormatInt(i, 10),
			ReceiptHandle: "handle-" + strconv.FormatInt(i, 10),
			Body:          "body",
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

	messages := make([]*Message, expected)
	for i := int64(0); i < expected; i++ {
		messages[i] = &Message{
			Id:            strconv.FormatInt(i, 10),
			ReceiptHandle: "handle-" + strconv.FormatInt(i, 10),
			Body:          "body",
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

	messages := make([]*Message, 5)
	for i := 0; i < 5; i++ {
		messages[i] = &Message{
			Id:            strconv.FormatInt(int64(i), 10),
			ReceiptHandle: "handle-" + strconv.FormatInt(int64(i), 10),
			Body:          "body",
		}
	}

	deleteCallCount := 0
	poller.retryer.DoFunc = func(r *retryer.Retryer, request *retryer.Request) (*http.Response, error) {
		if request.Method == http.MethodDelete {
			deleteCallCount++
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       ioutil.NopCloser(bytes.NewReader([]byte(""))),
			}, nil
		}
		// Fetch messages
		body, _ := json.Marshal(MessageResponse{ChannelId: mockChannelId, Messages: messages})
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       ioutil.NopCloser(bytes.NewReader(body)),
		}, nil
	}

	poller.workerPool.(*MockWorkerPool).SubmitFunc = func(job worker_pool.Job) (bool, error) {
		return true, nil
	}

	shouldWait := poller.poll()

	assert.False(t, shouldWait)
	assert.Equal(t, 1, deleteCallCount) // All submitted, delete should be called
}

func TestPollWithDedup(t *testing.T) {

	poller := newPollerTest()

	// Pre-populate tracker with one already-processed message
	poller.tracker.putIfAbsent("0", "handle-0")
	poller.tracker.MarkProcessed("0")

	// Fetch 3 messages, where message 0 is already processed
	messages := make([]*Message, 3)
	for i := 0; i < 3; i++ {
		messages[i] = &Message{
			Id:            strconv.FormatInt(int64(i), 10),
			ReceiptHandle: "handle-" + strconv.FormatInt(int64(i), 10),
			Body:          "body",
		}
	}

	deleteCallCount := 0
	poller.retryer.DoFunc = func(r *retryer.Retryer, request *retryer.Request) (*http.Response, error) {
		if request.Method == http.MethodDelete {
			deleteCallCount++
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       ioutil.NopCloser(bytes.NewReader([]byte(""))),
			}, nil
		}
		// Fetch messages
		body, _ := json.Marshal(MessageResponse{ChannelId: mockChannelId, Messages: messages})
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       ioutil.NopCloser(bytes.NewReader(body)),
		}, nil
	}

	poller.workerPool.(*MockWorkerPool).NumberOfAvailableWorkerFunc = func() int32 {
		return 1
	}

	submitCount := 0
	poller.workerPool.(*MockWorkerPool).SubmitFunc = func(job worker_pool.Job) (bool, error) {
		submitCount++
		return true, nil
	}

	shouldWait := poller.poll()

	assert.False(t, shouldWait)
	assert.Equal(t, 2, submitCount) // Only 2 messages submitted (0 was skipped)
	assert.Equal(t, 1, deleteCallCount)
}

func TestPollAllSubmittedDeleteSuccess(t *testing.T) {

	poller := newPollerTest()

	messages := make([]*Message, 3)
	for i := 0; i < 3; i++ {
		messages[i] = &Message{
			Id:            strconv.FormatInt(int64(i), 10),
			ReceiptHandle: "handle-" + strconv.FormatInt(int64(i), 10),
			Body:          "body",
		}
	}

	deleteCallCount := 0
	poller.retryer.DoFunc = func(r *retryer.Retryer, request *retryer.Request) (*http.Response, error) {
		if request.Method == http.MethodDelete {
			deleteCallCount++
			// Verify the tracker has messages before delete
			assert.Equal(t, 3, len(poller.tracker.messages))
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       ioutil.NopCloser(bytes.NewReader([]byte(""))),
			}, nil
		}
		// Fetch messages
		body, _ := json.Marshal(MessageResponse{ChannelId: mockChannelId, Messages: messages})
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       ioutil.NopCloser(bytes.NewReader(body)),
		}, nil
	}

	poller.workerPool.(*MockWorkerPool).NumberOfAvailableWorkerFunc = func() int32 {
		return 1
	}

	submitCount := 0
	poller.workerPool.(*MockWorkerPool).SubmitFunc = func(job worker_pool.Job) (bool, error) {
		submitCount++
		return true, nil
	}

	shouldWait := poller.poll()

	assert.False(t, shouldWait)
	assert.Equal(t, 3, submitCount)
	assert.Equal(t, 1, deleteCallCount)
	assert.Equal(t, 0, len(poller.tracker.messages)) // Registrator should be reset
}

func TestPollAllSubmittedDeleteFailure(t *testing.T) {

	poller := newPollerTest()

	messages := make([]*Message, 2)
	for i := 0; i < 2; i++ {
		messages[i] = &Message{
			Id:            strconv.FormatInt(int64(i), 10),
			ReceiptHandle: "handle-" + strconv.FormatInt(int64(i), 10),
			Body:          "body",
		}
	}

	deleteCallCount := 0
	poller.retryer.DoFunc = func(r *retryer.Retryer, request *retryer.Request) (*http.Response, error) {
		if request.Method == http.MethodDelete {
			deleteCallCount++
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       ioutil.NopCloser(bytes.NewReader([]byte("Delete failed"))),
			}, nil
		}
		// Fetch messages
		body, _ := json.Marshal(MessageResponse{ChannelId: mockChannelId, Messages: messages})
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       ioutil.NopCloser(bytes.NewReader(body)),
		}, nil
	}

	poller.workerPool.(*MockWorkerPool).NumberOfAvailableWorkerFunc = func() int32 {
		return 1
	}

	poller.workerPool.(*MockWorkerPool).SubmitFunc = func(job worker_pool.Job) (bool, error) {
		return true, nil
	}

	shouldWait := poller.poll()

	assert.False(t, shouldWait)
	assert.Equal(t, 1, deleteCallCount)
	assert.Equal(t, 2, len(poller.tracker.messages)) // Registrator should NOT be reset on delete failure
}

func TestPollPartialSubmitNoDelete(t *testing.T) {

	poller := newPollerTest()

	messages := make([]*Message, 3)
	for i := 0; i < 3; i++ {
		messages[i] = &Message{
			Id:            strconv.FormatInt(int64(i), 10),
			ReceiptHandle: "handle-" + strconv.FormatInt(int64(i), 10),
			Body:          "body",
		}
	}
	poller.retryer.DoFunc = mockFetchMessages(messages, nil)

	poller.workerPool.(*MockWorkerPool).NumberOfAvailableWorkerFunc = func() int32 {
		return 1
	}

	submitCount := 0
	poller.workerPool.(*MockWorkerPool).SubmitFunc = func(job worker_pool.Job) (bool, error) {
		submitCount++
		if submitCount < 3 {
			return true, nil
		}
		// Last one rejected
		return false, nil
	}

	shouldWait := poller.poll()

	assert.False(t, shouldWait)
	assert.Equal(t, 3, submitCount)
	// Not all submitted, delete should not be called (tracker should still have messages)
	assert.Equal(t, 3, len(poller.tracker.messages)) // All 3 are tracked, but only 2 are processed
	// Verify that only 2 are marked as processed
	processedCount := 0
	for _, msg := range poller.tracker.messages {
		if msg.processed {
			processedCount++
		}
	}
	assert.Equal(t, 2, processedCount)
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
