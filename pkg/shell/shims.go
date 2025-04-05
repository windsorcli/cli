package shell

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"text/template"
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

// osUserHomeDir is a variable that points to os.UserHomeDir, allowing it to be overridden in tests
var osUserHomeDir = os.UserHomeDir

// osSetenv is a variable that points to os.Setenv, allowing it to be overridden in tests
var osSetenv = os.Setenv

// osUnsetenv is a variable that points to os.Unsetenv, allowing it to be overridden in tests
var osUnsetenv = os.Unsetenv

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
