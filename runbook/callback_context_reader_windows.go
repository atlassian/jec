//go:build windows
// +build windows

package runbook

import (
	winio "github.com/Microsoft/go-winio"
	"github.com/sirupsen/logrus"
	"net"
)

var CallbackContextReaderFunc = CallbackContextReaderForWindows
var CreatePipeFunc = CreatePipeForWindows

func CreatePipeForWindows(pipePath string) interface{} {
	listener, err := winio.ListenPipe(pipePath, nil)
	if err != nil {
		logrus.Debugf("Could not create named pipe: %s Error: %s", pipePath, err.Error())
	}
	return listener
}

func CallbackContextReaderForWindows(contextBuffer []byte, pipeListener interface{}) {
	listener, ok := pipeListener.(net.Listener)
	if !ok {
		logrus.Debug("Could not convert pipe for windows pipe listener.")
		return
	}
	defer listener.Close()

	pipe, err := listener.Accept()
	if err != nil {
		logrus.Debugf("Could not accept connection from pipe. Error: %s", err.Error())
	}
	defer pipe.Close()

	_, err = pipe.Read(contextBuffer)
	if err != nil {
		logrus.Debugf("Could not read data from pipe. Error: %s", err.Error())
	}
}
