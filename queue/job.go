package queue

import (
	"github.com/atlassian/jec/runbook"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

const (
	jobInitial = iota
	jobExecuting
	jobFinished
	jobError
)

type job struct {
	messageHandler MessageHandler

	message Message
	apiKey  string
	baseUrl string

	state        int32
	executeMutex *sync.Mutex
}

func newJob(messageHandler MessageHandler, message Message, apiKey, baseUrl string) *job {
	return &job{
		messageHandler: messageHandler,
		message:        message,
		apiKey:         apiKey,
		baseUrl:        baseUrl,
		state:          jobInitial,
		executeMutex:   &sync.Mutex{},
	}
}

func (j *job) Id() string {
	return j.message.MessageId
}

func (j *job) Execute() error {

	defer j.executeMutex.Unlock()
	j.executeMutex.Lock()

	if j.state != jobInitial {
		return errors.Errorf("Job[%s] is already executing or finished.", j.Id())
	}
	j.state = jobExecuting

	messageId := j.Id()

	result, err := j.messageHandler.Handle(j.message)

	if result != nil {
		go func() {
			start := time.Now()

			err = runbook.SendResultToJsmFunc(result, j.apiKey, j.baseUrl)
			if err != nil {
				logrus.Warnf("Could not send action result[%+v] of message[%s] to Jira Service Management: %s", result, messageId, err)
			} else {
				took := time.Since(start)
				logrus.Debugf("Successfully sent result of message[%s] to Jira Service Management and it took %f seconds.", messageId, took.Seconds())
			}
		}()
	}

	if err != nil {
		j.state = jobError
		return errors.Errorf("Message[%s] could not be processed: %s", messageId, err)
	}

	j.state = jobFinished
	return nil
}
