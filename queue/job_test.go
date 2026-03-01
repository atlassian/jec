package queue

import (
	"encoding/json"
	"github.com/atlassian/jec/runbook"
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
	mockMessageHandler.HandleFunc = func(message Message) (payload *runbook.ActionResultPayload, e error) {
		return mockActionResultPayload, nil
	}

	message := Message{
		MessageId: mockMessageId,
		Body:      "mockBody",
		ChannelId: mockChannelId,
	}

	return &job{
		messageHandler: mockMessageHandler,
		message:        message,
		executeMutex:   &sync.Mutex{},
		apiKey:         mockApiKey,
		baseUrl:        mockBaseUrl,
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

	jecJob := newJobTest()
	jecJob.baseUrl = testServer.URL

	wg.Add(1)
	err := jecJob.Execute()

	wg.Wait()
	assert.Nil(t, err)

	expectedState := int32(jobFinished)
	actualState := jecJob.state

	assert.Equal(t, expectedState, actualState)
}

func TestMultipleExecute(t *testing.T) {
	wg := &sync.WaitGroup{}

	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(http.StatusAccepted)
		wg.Done()
	}))
	defer testServer.Close()

	jecJob := newJobTest()
	jecJob.baseUrl = testServer.URL

	errorResults := make(chan error, 25)

	wg.Add(26) // 25 execute try + 1 successful execute send result to testServer
	for i := 0; i < 25; i++ {
		go func() {
			defer wg.Done()
			err := jecJob.Execute()
			if err != nil {
				errorResults <- jecJob.Execute()
			}
		}()
	}

	wg.Wait()
	expectedState := int32(jobFinished)
	actualState := jecJob.state

	assert.Equal(t, expectedState, actualState) // only one execute finished
	assert.Equal(t, 24, len(errorResults))      // other executes will fail
}

func TestExecuteInNotInitialState(t *testing.T) {

	jecJob := newJobTest()
	jecJob.state = jobExecuting

	err := jecJob.Execute()
	assert.NotNil(t, err)

	expectedErr := errors.Errorf("Job[%s] is already executing or finished.", jecJob.Id())
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

	jecJob := newJobTest()
	jecJob.baseUrl = testServer.URL

	jecJob.messageHandler.(*MockMessageHandler).HandleFunc = func(message Message) (payload *runbook.ActionResultPayload, e error) {
		return errPayload, errors.New("Process Error")
	}

	wg.Add(1)
	err := jecJob.Execute()

	wg.Wait()
	assert.NotNil(t, err)

	expectedErr := errors.Errorf("Message[%s] could not be processed: %s", jecJob.Id(), "Process Error")
	assert.EqualError(t, err, expectedErr.Error())

	expectedState := int32(jobError)
	actualState := jecJob.state

	assert.Equal(t, expectedState, actualState)
}
