package queue

import (
	"github.com/atlassian/jec/runbook"
	"github.com/aws/aws-sdk-go/service/sqs"
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
	queueProvider  SQSProvider
	messageHandler MessageHandler

	message sqs.Message
	ownerId string
	apiKey  string
	baseUrl string

	state        int32
	executeMutex *sync.Mutex
}

func newJob(queueProvider SQSProvider, messageHandler MessageHandler, message sqs.Message, apiKey, baseUrl, ownerId string) *job {
	return &job{
		queueProvider:  queueProvider,
		messageHandler: messageHandler,
		message:        message,
		ownerId:        ownerId,
		apiKey:         apiKey,
		baseUrl:        baseUrl,
		state:          jobInitial,
		executeMutex:   &sync.Mutex{},
	}
}

func (j *job) Id() string {
	return *j.message.MessageId
}

func (j *job) sqsMessage() sqs.Message {
	return j.message
}

func (j *job) Execute() error {

	defer j.executeMutex.Unlock()
	j.executeMutex.Lock()

	if j.state != jobInitial {
		return errors.Errorf("Job[%s] is already executing or finished.", j.Id())
	}
	j.state = jobExecuting

	region := j.queueProvider.Properties().Region()
	messageId := j.Id()

	err := j.queueProvider.DeleteMessage(&j.message)
	if err != nil {
		j.state = jobError
		return errors.Errorf("Message[%s] could not be deleted from the queue[%s]: %s", messageId, region, err)
	}

	logrus.Debugf("Message[%s] is deleted from the queue[%s].", messageId, region)

	messageAttr := j.sqsMessage().MessageAttributes

	if messageAttr == nil ||
		*messageAttr[ownerId].StringValue != j.ownerId &&
			*messageAttr[channelId].StringValue != j.ownerId {
		j.state = jobError
		return errors.Errorf("Message[%s] is invalid, will not be processed.", messageId)
	}

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
