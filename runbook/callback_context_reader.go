//go:build !windows
// +build !windows

package runbook

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"syscall"
)

var CallbackContextReaderFunc = CallbackContextReader
var CreatePipeFunc = CreatePipe

func CreatePipe(pipePath string) interface{} {
	err := syscall.Mkfifo(pipePath, 0666)
	if err != nil {
		logrus.Debugf("Could not create named pipe with name %s. Error: %s", pipePath, err.Error())
	}
	return pipePath
}

func CallbackContextReader(contextBuffer []byte, pipeListener interface{}) {
	pipePath, ok := pipeListener.(string)
	if !ok {
		logrus.Debug("Could not convert pipe for pipe listener.")
		return
	}
	file, err := os.OpenFile(pipePath, os.O_RDONLY, os.ModeNamedPipe)
	if err != nil {
		logrus.Debugf("Could not open named pipe. Error: %s", err.Error())
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	_ = copy(contextBuffer, data)
	if err != nil {
		fmt.Println("Error reading from the named pipe:", err)
	}
}
