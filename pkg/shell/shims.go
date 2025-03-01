package shell

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"text/template"
)

// Shims for system functions to facilitate testing by allowing overrides.
var (
	// Current working directory retrieval
	getwd = os.Getwd

	// Command execution
	execCommand = osExecCommand

	// Command run execution
	cmdRun = func(cmd *exec.Cmd) error {
		return cmd.Run()
	}

	// Command start execution
	cmdStart = func(cmd *exec.Cmd) error {
		return cmd.Start()
	}

	// User home directory retrieval
	osUserHomeDir = os.UserHomeDir

	// File status retrieval
	osStat = os.Stat

	// File opening
	osOpenFile = os.OpenFile

	// File reading
	osReadFile = os.ReadFile

	// File writing
	osWriteFile = os.WriteFile

	// Directory creation
	osMkdirAll = os.MkdirAll

	// Command wait execution
	cmdWait = func(cmd *exec.Cmd) error {
		return cmd.Wait()
	}

	// Command stdout pipe
	cmdStdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
		return cmd.StdoutPipe()
	}

	// Command stderr pipe
	cmdStderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
		return cmd.StderrPipe()
	}

	// Scanner scan operation
	bufioScannerScan = func(scanner *bufio.Scanner) bool {
		return scanner.Scan()
	}

	// Scanner error retrieval
	bufioScannerErr = func(scanner *bufio.Scanner) error {
		return scanner.Err()
	}

	// Executable path retrieval
	osExecutable = os.Executable

	// Template creation
	hookTemplateNew = func(name string) *template.Template {
		return template.New(name)
	}

	// Template parsing
	hookTemplateParse = func(tmpl *template.Template, text string) (*template.Template, error) {
		return tmpl.Parse(text)
	}

	// Template execution
	hookTemplateExecute = func(tmpl *template.Template, wr io.Writer, data interface{}) error {
		return tmpl.Execute(wr, data)
	}

	// Process state exit code retrieval
	processStateExitCode = func(ps *os.ProcessState) int {
		return ps.ExitCode()
	}

	// Process state creation
	newProcessState = func() *os.ProcessState {
		return &os.ProcessState{}
	}
)

// osExecCommand wraps exec.Command for testing purposes.
func osExecCommand(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...)
}
