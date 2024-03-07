//go:build !windows
// +build !windows

package runbook

import (
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"syscall"
)

type CallbackContextHandlerUnix struct {
	pipePath              string
	callbackContextBuffer []byte
	pipeOpenedByScript    bool
}

func NewCallbackContextHandler(executionId string) *CallbackContextHandlerUnix {
	return &CallbackContextHandlerUnix{
		pipePath:              `jecCallbackPipe-` + executionId,
		callbackContextBuffer: make([]byte, 4096),
		pipeOpenedByScript:    false,
	}
}

func (callbackContextHandler *CallbackContextHandlerUnix) CreatePipe() {
	err := syscall.Mkfifo(callbackContextHandler.pipePath, 0666)
	if err != nil {
		logrus.Debugf("Could not create named pipe with name %s. Error: %s", callbackContextHandler.pipePath, err.Error())
	}
}

func (callbackContextHandler *CallbackContextHandlerUnix) Read() {
	file, err := os.OpenFile(callbackContextHandler.pipePath, os.O_RDONLY, os.ModeNamedPipe)
	if err != nil {
		logrus.Debugf("Could not open named pipe. Error: %s", err.Error())
	}
	defer file.Close()
	callbackContextHandler.pipeOpenedByScript = true

	data, err := ioutil.ReadAll(file)
	_ = copy(callbackContextHandler.callbackContextBuffer, data)
	if err != nil {
		logrus.Debug("Error reading from the named pipe:", err)
	}

	err = os.Remove(callbackContextHandler.pipePath)
	if err != nil {
		logrus.Debugf("Could not delete named pipe %s", callbackContextHandler.pipePath)
	}
}

func (callbackContextHandler *CallbackContextHandlerUnix) ClosePipe() {
	if !callbackContextHandler.pipeOpenedByScript {
		// If Read() go routine has not read callback context from script execution then write empty string, so that can terminate
		file, err := os.OpenFile(callbackContextHandler.pipePath, os.O_WRONLY, os.ModeNamedPipe)
		if err != nil {
			logrus.Debug()
		}
		defer file.Close()

		data := []byte("Execution Finished!")
		_, err = file.Write(data)
		if err != nil {
			logrus.Debug("Could not write to named pipe")
		}
	}
}
