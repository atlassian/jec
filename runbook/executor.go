package runbook

import (
	"bytes"
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var ExecuteFunc = Execute

var executables = map[string][]string{
	".bat":    {"cmd", "/C"},
	".cmd":    {"cmd", "/C"},
	".ps1":    {"powershell", "-File"},
	".sh":     {"sh"},
	".py":     {"python"},
	".groovy": {"groovy"},
	".go":     {"go", "run"},
}

type ExecError struct {
	Stderr string
	error
}

func Execute(executablePath string, args, environmentVars []string, stdout, stderr io.Writer) (string, error) {

	callbackContextBuffer := make([]byte, 4096)

	var waitGroup sync.WaitGroup

	var pipePath string
	if runtime.GOOS == "windows" {
		pipePath = `\\.\pipe\jecNamedPipe`
	} else {
		pipePath = "jecNamedPipe"
	}
	pipe := CreatePipeFunc(pipePath)

	waitGroup.Add(1)
	go CallbackContextReaderFunc(callbackContextBuffer, pipe)

	if args == nil {
		args = []string{}
	} else if environmentVars == nil {
		environmentVars = []string{}
	}

	var cmd *exec.Cmd
	fileExt := filepath.Ext(strings.ToLower(executablePath))
	command, exist := executables[fileExt]

	if exist {
		args = append(append(command[1:], executablePath), args...)
		cmd = exec.Command(command[0], args...)
	} else {
		cmd = exec.Command(executablePath, args...)
	}

	cmd.Env = append(append(os.Environ(), fmt.Sprintf("JEC_PIPE_PATH=%s", pipePath)), environmentVars...)

	stderrBuff := &bytes.Buffer{}
	cmd.Stderr = stderrBuff
	if stderr != nil {
		cmd.Stderr = io.MultiWriter(stderr, cmd.Stderr)
	}
	if stdout != nil {
		cmd.Stdout = stdout
	}

	err := cmd.Run()
	if err != nil {
		return "", &ExecError{stderrBuff.String(), err}
	}

	waitGroup.Done()

	err = os.Remove(pipePath)
	if err != nil {
		logrus.Debugf("Could not delete named pipe %s", pipePath)
	}

	callbackContext := bytes.NewBuffer(bytes.Trim(callbackContextBuffer, "\x00")).String()

	logrus.Debugf("Recived data from named pipe: %s\n", callbackContext) // TODO: Remove this line

	return callbackContext, nil
}
