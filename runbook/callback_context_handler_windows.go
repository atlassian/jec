//go:build windows
// +build windows

package runbook

import (
	winio "github.com/Microsoft/go-winio"
	"github.com/sirupsen/logrus"
	"net"
)

type CallbackContextHandlerWindows struct {
	pipePath              string
	pipeListener          net.Listener
	callbackContextBuffer []byte
	pipeOpenedByScript    bool
}

func NewCallbackContextHandler(executionId string) *CallbackContextHandlerWindows {
	return &CallbackContextHandlerWindows{
		pipePath:              `\\.\pipe\jecCallbackPipe-` + executionId,
		callbackContextBuffer: make([]byte, 4096),
		pipeOpenedByScript:    false,
	}
}

func (callbackContextHandler *CallbackContextHandlerWindows) CreatePipe() {
	listener, err := winio.ListenPipe(callbackContextHandler.pipePath, nil)
	callbackContextHandler.pipeListener = listener
	if err != nil {
		logrus.Debugf("Could not create named pipe: %s Error: %s", callbackContextHandler.pipePath, err.Error())
	}
}

func (callbackContextHandler *CallbackContextHandlerWindows) Read() {
	defer callbackContextHandler.pipeListener.Close()
	pipe, err := callbackContextHandler.pipeListener.Accept()
	if err != nil {
		logrus.Debugf("Could not accept connection from pipe. Error: %s", err.Error())
		return
	}
	defer pipe.Close()
	callbackContextHandler.pipeOpenedByScript = true

	_, err = pipe.Read(callbackContextHandler.callbackContextBuffer)
	if err != nil {
		logrus.Debugf("Could not read data from pipe. Error: %s", err.Error())
	}
}

func (callbackContextHandler *CallbackContextHandlerWindows) ClosePipe() {
	if !callbackContextHandler.pipeOpenedByScript {
		// If Read() go routine has not read callback context from script execution then write empty string, so that can terminate
		pipe, err := winio.DialPipe(callbackContextHandler.pipePath, nil)
		if err != nil {
			logrus.Debug("Could not connect to named pipe")
		}
		defer pipe.Close()

		data := []byte("Execution Finished!")
		_, err = pipe.Write(data)
		if err != nil {
			logrus.Debug("Could not write to named pipe")
		}
	}
}
