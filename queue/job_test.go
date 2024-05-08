package queue

import (
	"encoding/json"
	"github.com/atlassian/jec/runbook"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

var mockActionResultPayload = &runbook.ActionResultPayload{
	Action:    "MockAction",
	RequestId: "RequestId",
}

func newJobTest() *job {
	mockMessageHandler := &MockMessageHandler{}
	mockMessageHandler.HandleFunc = func(message sqs.Message) (payload *runbook.ActionResultPayload, e error) {
		return mockActionResultPayload, nil
	}

	body := "mockBody"
	messageAttr := map[string]*sqs.MessageAttributeValue{ownerId: {StringValue: &mockOwnerId}}

	message := sqs.Message{
		MessageId:         &mockMessageId,
		Body:              &body,
		MessageAttributes: messageAttr,
	}

	return &job{
		queueProvider:  NewMockQueueProvider(),
		messageHandler: mockMessageHandler,
		message:        message,
		executeMutex:   &sync.Mutex{},
		apiKey:         mockApiKey,
		baseUrl:        mockBaseUrl,
		ownerId:        mockOwnerId,
		state:          jobInitial,
	}
}

func TestExecute(t *testing.T) {
	wg := &sync.WaitGroup{}

	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(http.StatusAccepted)

		actionResult := &runbook.ActionResultPayload{}
		body, _ := ioutil.ReadAll(req.Body)
		json.Unmarshal(body, actionResult)

		assert.Equal(t, mockActionResultPayload, actionResult)
		assert.Equal(t, "GenieKey "+mockApiKey, req.Header.Get("Authorization"))
		wg.Done()
	}))
	defer testServer.Close()

	sqsJob := newJobTest()
	sqsJob.baseUrl = testServer.URL

	wg.Add(1)
	err := sqsJob.Execute()

	wg.Wait()
	assert.Nil(t, err)

	expectedState := int32(jobFinished)
	actualState := sqsJob.state

	assert.Equal(t, expectedState, actualState)
}

func TestMultipleExecute(t *testing.T) {
	wg := &sync.WaitGroup{}

	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(http.StatusAccepted)
		wg.Done()
	}))
	defer testServer.Close()

	sqsJob := newJobTest()
	sqsJob.baseUrl = testServer.URL

	errorResults := make(chan error, 25)

	wg.Add(26) // 25 execute try + 1 successful execute send result to testServer
	for i := 0; i < 25; i++ {
		go func() {
			defer wg.Done()
			err := sqsJob.Execute()
			if err != nil {
				errorResults <- sqsJob.Execute()
			}
		}()
	}

	wg.Wait()
	expectedState := int32(jobFinished)
	actualState := sqsJob.state

	assert.Equal(t, expectedState, actualState) // only one execute finished
	assert.Equal(t, 24, len(errorResults))      // other executes will fail
}

func TestExecuteInNotInitialState(t *testing.T) {

	sqsJob := newJobTest()
	sqsJob.state = jobExecuting

	err := sqsJob.Execute()
	assert.NotNil(t, err)

	expectedErr := errors.Errorf("Job[%s] is already executing or finished.", sqsJob.Id())
	assert.EqualError(t, err, expectedErr.Error())
}

func TestExecuteWithProcessError(t *testing.T) {
	wg := &sync.WaitGroup{}

	errPayload := mockActionResultPayload
	errPayload.IsSuccessful = true
	errPayload.FailureMessage = "Process Error"

	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(http.StatusAccepted)

		actionResult := &runbook.ActionResultPayload{}
		body, _ := ioutil.ReadAll(req.Body)
		json.Unmarshal(body, actionResult)

		assert.Equal(t, errPayload, actionResult)
		assert.Equal(t, "GenieKey "+mockApiKey, req.Header.Get("Authorization"))
		wg.Done()
	}))
	defer testServer.Close()

	sqsJob := newJobTest()
	sqsJob.baseUrl = testServer.URL

	sqsJob.messageHandler.(*MockMessageHandler).HandleFunc = func(message sqs.Message) (payload *runbook.ActionResultPayload, e error) {
		return errPayload, errors.New("Process Error")
	}

	wg.Add(1)
	err := sqsJob.Execute()

	wg.Wait()
	assert.NotNil(t, err)

	expectedErr := errors.Errorf("Message[%s] could not be processed: %s", sqsJob.Id(), "Process Error")
	assert.EqualError(t, err, expectedErr.Error())

	expectedState := int32(jobError)
	actualState := sqsJob.state

	assert.Equal(t, expectedState, actualState)
}

func TestExecuteWithDeleteError(t *testing.T) {

	sqsJob := newJobTest()

	sqsJob.queueProvider.(*MockSQSProvider).DeleteMessageFunc = func(message *sqs.Message) error {
		return errors.New("Delete Error")
	}

	err := sqsJob.Execute()
	assert.NotNil(t, err)

	expectedErr := errors.Errorf("Message[%s] could not be deleted from the queue[%s]: %s", sqsJob.Id(), sqsJob.queueProvider.Properties().Region(), "Delete Error")
	assert.EqualError(t, err, expectedErr.Error())

	expectedState := int32(jobError)
	actualState := sqsJob.state

	assert.Equal(t, expectedState, actualState)
}

func TestExecuteWithInvalidQueueMessage(t *testing.T) {

	sqsJob := newJobTest()

	falseIntegrationId := "falseIntegrationId"
	messageAttr := map[string]*sqs.MessageAttributeValue{ownerId: {StringValue: &falseIntegrationId}, channelId: {StringValue: &falseIntegrationId}}
	sqsJob.message = sqs.Message{MessageAttributes: messageAttr, MessageId: &mockMessageId}

	err := sqsJob.Execute()
	assert.NotNil(t, err)

	expectedErr := errors.Errorf("Message[%s] is invalid, will not be processed.", sqsJob.Id())
	assert.EqualError(t, err, expectedErr.Error())

	expectedState := int32(jobError)
	actualState := sqsJob.state

	assert.Equal(t, expectedState, actualState)
}
