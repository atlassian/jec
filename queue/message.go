package queue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/atlassian/jec/conf"
	"github.com/atlassian/jec/git"
	"github.com/atlassian/jec/runbook"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io"
	"time"
)

type MessageHandler interface {
	Handle(message sqs.Message) (*runbook.ActionResultPayload, error)
}

type messageHandler struct {
	repositories  git.Repositories
	actionSpecs   conf.ActionSpecifications
	actionLoggers map[string]io.Writer
}

func NewMessageHandler(repositories git.Repositories, actionSpecs conf.ActionSpecifications, actionLoggers map[string]io.Writer) MessageHandler {
	return &messageHandler{
		repositories:  repositories,
		actionSpecs:   actionSpecs,
		actionLoggers: actionLoggers,
	}
}

func (mh *messageHandler) Handle(message sqs.Message) (*runbook.ActionResultPayload, error) {
	queuePayload := payload{}
	err := json.Unmarshal([]byte(*message.Body), &queuePayload)
	if err != nil {
		return nil, err
	}

	actionType := queuePayload.ActionType
	action := queuePayload.MappedAction.Name
	if action == "" {
		action = queuePayload.Action
	}
	if action == "" {
		return nil, errors.Errorf("SQS message does not contain action property.")
	}

	result := &runbook.ActionResultPayload{
		EntityId:   queuePayload.Entity.Id,
		EntityType: queuePayload.Entity.Type,
		Action:     action,
		ActionType: actionType,
		RequestId:  queuePayload.RequestId,
	}

	mappedAction, err := mh.resolveMappedAction(action, actionType)
	if err != nil {
		result.IsSuccessful = false
		result.FailureMessage = err.Error()
		return result, err
	}

	start := time.Now()
	executionResult, callbackContext, err := mh.execute(mappedAction, &message)
	took := time.Since(start)

	result.CallbackContext = callbackContext

	switch err := err.(type) {
	case *runbook.ExecError:
		result.IsSuccessful = false
		result.FailureMessage = fmt.Sprintf("Err: %s, Stderr: %s", err.Error(), err.Stderr)
		logrus.Debugf("Action[%s] execution of message[%s] failed: %s Stderr: %s", action, *message.MessageId, err.Error(), err.Stderr)
	case nil:
		result.IsSuccessful = true
		if !queuePayload.DiscardScriptResponse && queuePayload.ActionType == HttpActionType {
			httpResult := &runbook.HttpResponse{}
			err := json.Unmarshal([]byte(executionResult), httpResult)
			if err != nil {
				result.IsSuccessful = false
				logrus.Debugf("Http Action[%s] execution of message[%s] failed, could not parse http response fields: %s, error: %s",
					action, *message.MessageId, executionResult, err.Error())
				result.FailureMessage = "Could not parse http response fields: " + executionResult
			} else {
				result.HttpResponse = httpResult
			}
		}
		logrus.Debugf("Action[%s] execution of message[%s] has been completed and it took %f seconds.", action, *message.MessageId, took.Seconds())

	default:
		return nil, err
	}

	return result, nil
}

func (mh *messageHandler) resolveMappedAction(action string, actionType string) (*conf.MappedAction, error) {
	mappedAction, ok := mh.actionSpecs.ActionMappings[conf.ActionName(action)]

	if !ok {
		failureMessage := fmt.Sprintf("No mapped action is configured for requested action[%s]. "+
			"The request will be ignored.", action)
		return nil, errors.Errorf(failureMessage)
	}

	if mappedAction.Type != actionType {
		failureMessage := fmt.Sprintf("The type[%s] of the mapped action[%s] is not compatible with requested type[%s]. "+
			"The request will be ignored.", mappedAction.Type, action, actionType)
		return nil, errors.Errorf(failureMessage)
	}

	return &mappedAction, nil
}

func (mh *messageHandler) execute(mappedAction *conf.MappedAction, message *sqs.Message) (string, string, error) {

	sourceType := mappedAction.SourceType
	switch sourceType {
	case conf.GitSourceType:
		if mh.repositories == nil {
			return "", "", errors.New("Repositories should be provided.")
		}

		repository, err := mh.repositories.Get(mappedAction.GitOptions.Url)
		if err != nil {
			return "", "", err
		}

		repository.RLock()
		defer repository.RUnlock()
		fallthrough

	case conf.LocalSourceType:
		args := append(mh.actionSpecs.GlobalFlags.Args(), mappedAction.Flags.Args()...)
		args = append(args, []string{"-payload", *message.Body}...)
		args = append(args, mh.actionSpecs.GlobalArgs...)
		args = append(args, mappedAction.Args...)
		env := append(mh.actionSpecs.GlobalEnv, mappedAction.Env...)

		stdout := mh.actionLoggers[mappedAction.Stdout]
		stdoutBuff := &bytes.Buffer{}
		if mappedAction.Type == HttpActionType {
			if stdout != nil {
				stdout = io.MultiWriter(stdoutBuff, mh.actionLoggers[mappedAction.Stdout])
			} else {
				stdout = stdoutBuff
			}
		}
		stderr := mh.actionLoggers[mappedAction.Stderr]

		callbackContext, err := runbook.ExecuteFunc(*message.MessageId, mappedAction.Filepath, args, env, stdout, stderr)
		return stdoutBuff.String(), callbackContext, err
	default:
		return "", "", errors.Errorf("Unknown action sourceType[%s].", sourceType)
	}
}
