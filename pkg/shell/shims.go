package shell

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

// Shims for system functions to facilitate testing by allowing overrides.

// Current working directory retrieval
var getwd = os.Getwd

// Command execution
var execCommand = osExecCommand

// Process state exit code retrieval
var processStateExitCode = func(ps *os.ProcessState) int {
	return ps.ExitCode()
}

// Process state creation
var newProcessState = func() *os.ProcessState {
	return &os.ProcessState{}
}

// osExecCommand wraps exec.Command for testing purposes.
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

// osUserHomeDir is a variable that points to os.UserHomeDir, allowing it to be overridden in tests
var osUserHomeDir = os.UserHomeDir

// osStat is a variable that points to os.Stat, allowing it to be overridden in tests
var osStat = os.Stat

// osOpenFile is a variable that points to os.OpenFile, allowing it to be overridden in tests
var osOpenFile = os.OpenFile

// osReadFile is a variable that points to os.ReadFile, allowing it to be overridden in tests
var osReadFile = os.ReadFile

// osWriteFile is a variable that points to os.WriteFile, allowing it to be overridden in tests
var osWriteFile = os.WriteFile

// osMkdirAll is a variable that points to os.MkdirAll, allowing it to be overridden in tests
var osMkdirAll = os.MkdirAll

// cmdOutput is a shim for cmd.Output, allowing it to be overridden in tests
var cmdOutput = func(cmd *exec.Cmd) (string, error) {
	output, err := cmd.Output()
	return string(output), err
}

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

// osExecutable is a variable that points to os.Executable, allowing it to be overridden in tests
var osExecutable = os.Executable

// hookTemplateNew is a variable that points to template.New, allowing it to be overridden in tests
var hookTemplateNew = func(name string) *template.Template {
	return template.New(name)
}

// hookTemplateParse is a variable that points to template.Template.Parse, allowing it to be overridden in tests
var hookTemplateParse = func(tmpl *template.Template, text string) (*template.Template, error) {
	return tmpl.Parse(text)
}

// hookTemplateExecute is a variable that points to template.Template.Execute, allowing it to be overridden in tests
var hookTemplateExecute = func(tmpl *template.Template, wr io.Writer, data interface{}) error {
	return tmpl.Execute(wr, data)
}

// filepathRel is a variable that points to filepath.Rel, allowing it to be overridden in tests
var filepathRel = filepath.Rel
