package shell

import (
	"bufio"
	"io"
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

// cmdRun is a variable that points to cmd.Run, allowing it to be overridden in tests
var cmdRun = func(cmd *exec.Cmd) error {
	return cmd.Run()
}

// cmdStart is a variable that points to cmd.Start, allowing it to be overridden in tests
var cmdStart = func(cmd *exec.Cmd) error {
	return cmd.Start()
}

// osOpenFile is a variable that points to os.OpenFile, allowing it to be overridden in tests
var osOpenFile = os.OpenFile

// osReadFile is a variable that points to os.ReadFile, allowing it to be overridden in tests
var osReadFile = os.ReadFile

// cmdWait is a variable that points to cmd.Wait, allowing it to be overridden in tests
var cmdWait = func(cmd *exec.Cmd) error {
	return cmd.Wait()
}

// cmdStdoutPipe is a variable that points to cmd.StdoutPipe, allowing it to be overridden in tests
var cmdStdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
	return cmd.StdoutPipe()
}

// cmdStderrPipe is a variable that points to cmd.StderrPipe, allowing it to be overridden in tests
var cmdStderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
	return cmd.StderrPipe()
}

// bufioScannerScan is a variable that points to bufio.Scanner.Scan, allowing it to be overridden in tests
var bufioScannerScan = func(scanner *bufio.Scanner) bool {
	return scanner.Scan()
}

// bufioScannerErr is a variable that points to bufio.Scanner.Err, allowing it to be overridden in tests
var bufioScannerErr = func(scanner *bufio.Scanner) error {
	return scanner.Err()
}