package shell

import (
	"os"
	"os/exec"
)

// getwd is a variable that points to os.Getwd, allowing it to be overridden in tests
var getwd = os.Getwd

// execCommand is a variable that points to exec.Command, allowing it to be overridden in tests
var execCommand = osExecCommand

// osExecCommand is a wrapper around exec.Command to allow it to be overridden in tests
func osExecCommand(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...)
}

// osReadFile is a variable that points to os.ReadFile, allowing it to be overridden in tests
var osReadFile = os.ReadFile

// cmdStart is a variable that points to cmd.Start, allowing it to be overridden in tests
var cmdStart = func(cmd *exec.Cmd) error {
	return cmd.Start()
}

// cmdWait is a variable that points to cmd.Wait, allowing it to be overridden in tests
var cmdWait = func(cmd *exec.Cmd) error {
	return cmd.Wait()
}
