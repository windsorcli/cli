package shell

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"time"

	"github.com/briandowns/spinner"
	"github.com/windsorcli/cli/pkg/config"
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
	projectRoot   string
	injector      di.Injector
	configHandler config.ConfigHandler
	verbose       bool
}

// NewDefaultShell creates a new instance of DefaultShell
func NewDefaultShell(injector di.Injector) *DefaultShell {
	return &DefaultShell{
		injector: injector,
	}
}

// Initialize initializes the shell
func (s *DefaultShell) Initialize() error {
	configHandler, ok := s.injector.Resolve("configHandler").(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("error resolving configHandler")
	}
	s.configHandler = configHandler

	return nil
}

// SetVerbosity sets the verbosity flag
func (s *DefaultShell) SetVerbosity(verbose bool) {
	s.verbose = verbose
}

// GetProjectRoot retrieves the project root directory
func (s *DefaultShell) GetProjectRoot() (string, error) {
	// Return cached project root if available
	if s.projectRoot != "" {
		return s.projectRoot, nil
	}

	// Try to get the git root first
	cmd := execCommand("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err == nil {
		s.projectRoot = strings.TrimSpace(string(output))
		return s.projectRoot, nil
	}

	// If git command fails, search for windsor.yaml or windsor.yml
	currentDir, err := getwd()
	if err != nil {
		return "", err
	}

	depth := 0
	for {
		if depth > maxFolderSearchDepth {
			return "", nil
		}

		// Check for windsor.yaml file
		windsorYaml := filepath.Join(currentDir, "windsor.yaml")
		// Check for windsor.yml file
		windsorYml := filepath.Join(currentDir, "windsor.yml")

		if _, err := os.Stat(windsorYaml); err == nil {
			s.projectRoot = currentDir
			return s.projectRoot, nil
		}
		if _, err := os.Stat(windsorYml); err == nil {
			s.projectRoot = currentDir
			return s.projectRoot, nil
		}

		// Move to the parent directory
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			// We've reached the root of the file system
			return "", nil
		}
		currentDir = parentDir
		depth++
	}
}

// Exec executes a command, prints its output, and returns it as a string
func (s *DefaultShell) Exec(command string, args ...string) (string, error) {
	cmd := execCommand(command, args...)

	// Capture stdout and stderr in buffers
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// Handle sudo commands
	if command == "sudo" {
		cmd.Stdin = os.Stdin // Allow password input for sudo
	}

	// Run the command
	if err := cmdRun(cmd); err != nil {
		return "", fmt.Errorf("command execution failed: %w", err)
	}

	// Print the captured stdout to the user
	fmt.Print(stdoutBuf.String())

	return stdoutBuf.String(), nil
}

// ExecSudo executes a command with sudo if not already present and returns its output while suppressing it from being printed
func (s *DefaultShell) ExecSudo(message string, command string, args ...string) (string, error) {
	if s.verbose {
		fmt.Println(message)
		return s.Exec("sudo", append([]string{command}, args...)...)
	}

	// If the command is not sudo, add sudo to the command
	if command != "sudo" {
		args = append([]string{command}, args...)
		command = "sudo"
	}

	cmd := execCommand(command, args...)

	// Open the controlling terminal
	tty, err := osOpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("failed to open /dev/tty: %w", err)
	}
	defer tty.Close()

	// Set the command's stdin and stderr to tty for password input and prompt
	cmd.Stdin = tty
	cmd.Stderr = tty

	// Capture stdout in a buffer
	var stdoutBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf

	// Start the command
	if err := cmdStart(cmd); err != nil {
		fmt.Printf("\033[31m✗ %s - Failed\033[0m\n", message)
		return "", err
	}

	// Wait for the command to complete
	err = cmdWait(cmd)

	if err != nil {
		fmt.Printf("\033[31m✗ %s - Failed\033[0m\n", message)
		return "", fmt.Errorf("command execution failed: %w", err)
	}

	// Print success message with a green checkbox and "Done"
	fmt.Printf("\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)

	// Return the captured stdout as a string
	return stdoutBuf.String(), nil
}

// ExecSilent executes a command and returns its output as a string without printing to stdout or stderr
func (s *DefaultShell) ExecSilent(command string, args ...string) (string, error) {
	if s.verbose {
		return s.Exec(command, args...)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := execCommand(command, args...)

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// Run the command
	if err := cmdRun(cmd); err != nil {
		return "", fmt.Errorf("command execution failed: %w\n%s", err, stderrBuf.String())
	}

	return stdoutBuf.String(), nil
}

// ExecProgress executes a command and returns its output as a string while displaying progress status
func (s *DefaultShell) ExecProgress(message string, command string, args ...string) (string, error) {
	if s.verbose {
		fmt.Println(message)
		return s.Exec(command, args...)
	}

	cmd := execCommand(command, args...)

	// Set up pipes to capture stdout and stderr
	stdoutPipe, err := cmdStdoutPipe(cmd)
	if err != nil {
		return "", err
	}

	stderrPipe, err := cmdStderrPipe(cmd)
	if err != nil {
		return "", err
	}

	// Start the command execution
	if err := cmdStart(cmd); err != nil {
		return "", err
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	errChan := make(chan error, 2) // Channel to capture errors from goroutines

	// Initialize the spinner with color
	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " " + message
	spin.Start()

	// Goroutine to read and process stdout
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for bufioScannerScan(scanner) {
			line := scanner.Text()
			stdoutBuf.WriteString(line + "\n") // Append line to stdout buffer
		}
		if err := bufioScannerErr(scanner); err != nil {
			errChan <- fmt.Errorf("error reading stdout: %w", err)
			return
		}
		errChan <- nil
	}()

	// Goroutine to read and process stderr
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for bufioScannerScan(scanner) {
			line := scanner.Text()
			stderrBuf.WriteString(line + "\n") // Append line to stderr buffer
		}
		if err := bufioScannerErr(scanner); err != nil {
			errChan <- fmt.Errorf("error reading stderr: %w", err)
			return
		}
		errChan <- nil
	}()

	// Wait for the command to complete
	if err := cmdWait(cmd); err != nil {
		spin.Stop()                                                                 // Stop the spinner
		fmt.Printf("\033[31m✗ %s - Failed\033[0m\n%s", message, stderrBuf.String()) // Print failure message in red
		return "", fmt.Errorf("command execution failed: %w\n%s", err, stderrBuf.String())
	}

	// Check for errors from the stdout and stderr goroutines
	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			spin.Stop()                                                                 // Stop the spinner
			fmt.Printf("\033[31m✗ %s - Failed\033[0m\n%s", message, stderrBuf.String()) // Print failure message in red
			return "", err
		}
	}

	spin.Stop() // Stop the spinner

	// Print success message with a green checkbox and "Done"
	fmt.Printf("\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)

	return stdoutBuf.String(), nil // Return the captured stdout as a string
}

// InstallHook installs a shell hook if it exists for the given shell name.
// It executes the hook command silently and returns an error if the shell is unsupported.
func (s *DefaultShell) InstallHook(shellName string) error {

	// Retrieve the hook command for the specified shell
	hookCommand, exists := shellHooks[shellName]
	if !exists {
		return fmt.Errorf("Unsupported shell: %s", shellName)
	}

	selfPath, err := osExecutable()
	if err != nil {
		return err
	}

	// Convert Windows path if needed
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
