package shell

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"time"

	"github.com/briandowns/spinner"
	"github.com/windsorcli/cli/pkg/di"
)

// maxFolderSearchDepth is the maximum depth to search for the project root
const maxFolderSearchDepth = 10

// HookContext are the variables available during hook template evaluation
type HookContext struct {
	// SelfPath is the unescaped absolute path to direnv
	SelfPath string
}

// Shell interface defines methods for shell operations
type Shell interface {
	// Initialize initializes the shell environment
	Initialize() error
	// SetVerbosity sets the verbosity flag
	SetVerbosity(verbose bool)
	// PrintEnvVars prints the provided environment variables
	PrintEnvVars(envVars map[string]string) error
	// PrintAlias retrieves the shell alias
	PrintAlias(envVars map[string]string) error
	// GetProjectRoot retrieves the project root directory
	GetProjectRoot() (string, error)
	// Exec executes a command with optional privilege elevation
	Exec(command string, args ...string) (string, error)
	// ExecSilent executes a command and returns its output as a string without printing to stdout or stderr
	ExecSilent(command string, args ...string) (string, error)
	// ExecSudo executes a command with sudo if not already present and returns its output as a string while suppressing it from being printed
	ExecSudo(message string, command string, args ...string) (string, error)
	// ExecProgress executes a command and returns its output as a string while displaying progress status
	ExecProgress(message string, command string, args ...string) (string, error)
	// InstallHook installs a shell hook for the specified shell name
	InstallHook(shellName string) error
}

// DefaultShell is the default implementation of the Shell interface
type DefaultShell struct {
	projectRoot string
	injector    di.Injector
	verbose     bool
}

// NewDefaultShell creates a new instance of DefaultShell
func NewDefaultShell(injector di.Injector) *DefaultShell {
	return &DefaultShell{
		injector: injector,
	}
}

// Initialize initializes the shell
func (s *DefaultShell) Initialize() error {
	return nil
}

// SetVerbosity sets the verbosity flag
func (s *DefaultShell) SetVerbosity(verbose bool) {
	s.verbose = verbose
}

// GetProjectRoot finds the project root. It checks for a cached root first.
// If not found, it looks for "windsor.yaml" or "windsor.yml" in the current
// directory and its parents. Returns the root path or an error if not found.
func (s *DefaultShell) GetProjectRoot() (string, error) {
	if s.projectRoot != "" {
		return s.projectRoot, nil
	}

	currentDir, err := getwd()
	if err != nil {
		return "", err
	}

	for {
		windsorYaml := filepath.Join(currentDir, "windsor.yaml")
		windsorYml := filepath.Join(currentDir, "windsor.yml")

		if _, err := osStat(windsorYaml); err == nil {
			s.projectRoot = currentDir
			return s.projectRoot, nil
		}
		if _, err := osStat(windsorYml); err == nil {
			s.projectRoot = currentDir
			return s.projectRoot, nil
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			return "", nil
		}
		currentDir = parentDir
	}
}

// Exec runs a command with args, capturing stdout and stderr. It prints output and returns stdout as a string.
// If the command is "sudo", it connects stdin to the terminal for password input.
func (s *DefaultShell) Exec(command string, args ...string) (string, error) {
	cmd := execCommand(command, args...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	if command == "sudo" {
		cmd.Stdin = os.Stdin
	}
	if err := cmdStart(cmd); err != nil {
		return stdoutBuf.String(), fmt.Errorf("command start failed: %w", err)
	}
	if err := cmdWait(cmd); err != nil {
		return stdoutBuf.String(), fmt.Errorf("command execution failed: %w", err)
	}
	return stdoutBuf.String(), nil
}

// ExecSudo runs a command with 'sudo', ensuring elevated privileges. It handles password prompts by
// connecting to the terminal and captures the command's output. If verbose mode is enabled, it prints
// a message to stderr. The function returns the command's stdout or an error if execution fails.
func (s *DefaultShell) ExecSudo(message string, command string, args ...string) (string, error) {
	if s.verbose {
		fmt.Fprintln(os.Stderr, message)
		return s.Exec("sudo", append([]string{command}, args...)...)
	}

	if command != "sudo" {
		args = append([]string{command}, args...)
		command = "sudo"
	}

	cmd := execCommand(command, args...)
	tty, err := osOpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("failed to open /dev/tty: %w", err)
	}
	defer tty.Close()

	cmd.Stdin = tty
	cmd.Stderr = tty

	var stdoutBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf

	if err := cmdStart(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
		return stdoutBuf.String(), err
	}

	err = cmdWait(cmd)

	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
		return stdoutBuf.String(), fmt.Errorf("command execution failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)

	return stdoutBuf.String(), nil
}

// ExecSilent is a method that runs a command quietly, capturing its output.
// It returns the command's stdout as a string and any error encountered.
func (s *DefaultShell) ExecSilent(command string, args ...string) (string, error) {
	if s.verbose {
		return s.Exec(command, args...)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := execCommand(command, args...)

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmdRun(cmd); err != nil {
		return stdoutBuf.String(), fmt.Errorf("command execution failed: %w\n%s", err, stderrBuf.String())
	}

	return stdoutBuf.String(), nil
}

// ExecProgress is a method of the DefaultShell struct that executes a command with a progress indicator.
// It takes a message, a command, and arguments, using the Exec method if verbose mode is enabled.
// Otherwise, it captures stdout and stderr with pipes and uses a spinner to show progress.
// The method returns the command's stdout as a string and any error encountered.
func (s *DefaultShell) ExecProgress(message string, command string, args ...string) (string, error) {
	if s.verbose {
		fmt.Fprintln(os.Stderr, message)
		return s.Exec(command, args...)
	}

	cmd := execCommand(command, args...)

	stdoutPipe, err := cmdStdoutPipe(cmd)
	if err != nil {
		return "", err
	}

	stderrPipe, err := cmdStderrPipe(cmd)
	if err != nil {
		return "", err
	}

	if err := cmdStart(cmd); err != nil {
		return "", err
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	errChan := make(chan error, 2)

	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " " + message
	spin.Start()

	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for bufioScannerScan(scanner) {
			line := scanner.Text()
			stdoutBuf.WriteString(line + "\n")
		}
		if err := bufioScannerErr(scanner); err != nil {
			errChan <- fmt.Errorf("error reading stdout: %w", err)
			return
		}
		errChan <- nil
	}()

	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for bufioScannerScan(scanner) {
			line := scanner.Text()
			stderrBuf.WriteString(line + "\n")
		}
		if err := bufioScannerErr(scanner); err != nil {
			errChan <- fmt.Errorf("error reading stderr: %w", err)
			return
		}
		errChan <- nil
	}()

	if err := cmdWait(cmd); err != nil {
		spin.Stop()
		fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n%s", message, stderrBuf.String())
		return stdoutBuf.String(), fmt.Errorf("command execution failed: %w\n%s", err, stderrBuf.String())
	}

	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n%s", message, stderrBuf.String())
			return stdoutBuf.String(), err
		}
	}

	spin.Stop()
	fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)

	return stdoutBuf.String(), nil
}

// InstallHook installs a shell hook if it exists for the given shell name.
// It executes the hook command silently and returns an error if the shell is unsupported.
func (s *DefaultShell) InstallHook(shellName string) error {
	hookCommand, exists := shellHooks[shellName]
	if !exists {
		return fmt.Errorf("Unsupported shell: %s", shellName)
	}

	selfPath, err := osExecutable()
	if err != nil {
		return err
	}

	selfPath = strings.Replace(selfPath, "\\", "/", -1)
	ctx := HookContext{selfPath}

	hookTemplate := hookTemplateNew("hook")
	if hookTemplate == nil {
		return fmt.Errorf("failed to create new template")
	}
	hookTemplate, err = hookTemplateParse(hookTemplate, string(hookCommand))
	if err != nil {
		return err
	}

	err = hookTemplateExecute(hookTemplate, os.Stdout, ctx)
	if err != nil {
		return err
	}

	return nil
}
