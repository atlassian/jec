package runbook

import (
	"bytes"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func Execute(executionId string, executablePath string, args, environmentVars []string, stdout, stderr io.Writer) (string, error) {

	callbackContextHandler := NewCallbackContextHandler(executionId)
	callbackContextHandler.CreatePipe()

	go callbackContextHandler.Read()

	if args == nil {
		args = []string{}
	} else if environmentVars == nil {
		environmentVars = []string{}
	}

	args = append(args, []string{"--jecNamedPipe", callbackContextHandler.pipePath}...)

	var cmd *exec.Cmd
	fileExt := filepath.Ext(strings.ToLower(executablePath))
	command, exist := executables[fileExt]

	if exist {
		args = append(append(command[1:], executablePath), args...)
		cmd = exec.Command(command[0], args...)
	} else {
		cmd = exec.Command(executablePath, args...)
	}

	cmd.Env = append(os.Environ(), environmentVars...)

	stderrBuff := &bytes.Buffer{}
	cmd.Stderr = stderrBuff
	if stderr != nil {
		cmd.Stderr = io.MultiWriter(stderr, cmd.Stderr)
	}
	if stdout != nil {
		cmd.Stdout = stdout
	}

	err := cmd.Run()

	callbackContextHandler.ClosePipe()

	if err != nil {
		return "", &ExecError{stderrBuff.String(), err}
	}

	callbackContext := bytes.NewBuffer(bytes.Trim(callbackContextHandler.callbackContextBuffer, "\x00")).String()
	logrus.Debug("Callback context: " + callbackContext)
	return callbackContext, nil
}
