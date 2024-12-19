package shell

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/windsorcli/cli/internal/config"
	"github.com/windsorcli/cli/internal/di"
)

// maxFolderSearchDepth is the maximum depth to search for the project root
const maxFolderSearchDepth = 10

// Shell interface defines methods for shell operations
type Shell interface {
	Initialize() error
	// PrintEnvVars prints the provided environment variables
	PrintEnvVars(envVars map[string]string) error
	// PrintAlias retrieves the shell alias
	PrintAlias(envVars map[string]string) error
	// GetProjectRoot retrieves the project root directory
	GetProjectRoot() (string, error)
	// Exec executes a command with optional privilege elevation
	Exec(message string, command string, args ...string) (string, error)
	// ExecSilent executes a command and returns its output as a string without printing to stdout or stderr
	ExecSilent(command string, args ...string) (string, error)
	// ExecProgress executes a command and returns its output as a string while displaying progress status
	ExecProgress(message string, command string, args ...string) (string, error)
}

// DefaultShell is the default implementation of the Shell interface
type DefaultShell struct {
	projectRoot   string
	injector      di.Injector
	configHandler config.ConfigHandler
}

// NewDefaultShell creates a new instance of DefaultShell
func NewDefaultShell(injector di.Injector) *DefaultShell {
	return &DefaultShell{
		injector: injector,
	}
}

func (s *DefaultShell) Initialize() error {
	configHandler, ok := s.injector.Resolve("configHandler").(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("error resolving configHandler")
	}
	s.configHandler = configHandler

	return nil
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

// Exec executes a command and returns its output as a string
func (s *DefaultShell) Exec(message string, command string, args ...string) (string, error) {
	cmd := execCommand(command, args...)

	// Create pipes for stdout and stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Print the message if it is not empty
	if message != "" {
		fmt.Println(message)
	}

	// Start the command
	if err := cmdStart(cmd); err != nil {
		return "", fmt.Errorf("failed to start command: %w", err)
	}

	// Buffers to capture stdout and stderr
	var stdoutBuf, stderrBuf bytes.Buffer

	// Channel to capture errors from goroutines
	errChan := make(chan error, 2)

	// Signal handling
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-signalChan
		fmt.Println("\nInterrupt received, stopping command...")
		if err := cmd.Process.Kill(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to kill process: %v\n", err)
		}
	}()

	// Goroutine to read stdout
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Println(line) // Directly print each line
			stdoutBuf.WriteString(line + "\n")
		}
		if err := scanner.Err(); err != nil {
			errChan <- fmt.Errorf("error reading stdout: %w", err)
		} else {
			errChan <- nil
		}
	}()

	// Goroutine to read stderr
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintln(os.Stderr, line) // Print each line of stderr
			stderrBuf.WriteString(line + "\n")
		}
		if err := scanner.Err(); err != nil {
			errChan <- fmt.Errorf("error reading stderr: %w", err)
		} else {
			errChan <- nil
		}
	}()

	// Handle sudo commands
	if command == "sudo" {
		cmd.Stdin = os.Stdin // Allow password input for sudo
	}

	// Wait for the command to finish
	if err := cmdWait(cmd); err != nil {
		return "", fmt.Errorf("command execution failed: %w\n%s", err, stderrBuf.String())
	}

	// Check for errors from the goroutines
	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			return "", err
		}
	}

	return stdoutBuf.String(), nil
}

// ExecSilent executes a command and returns its output as a string without printing to stdout or stderr
func ExecSilent(command string, args ...string) (string, error) {
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := execCommand(command, args...)
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// Handle sudo commands
	if command == "sudo" {
		cmd.Stdin = os.Stdin // Allow password input for sudo
	}

	// Wait for the command to finish
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("command execution failed: %w\n%s", err, stderrBuf.String())
	}

	return stdoutBuf.String(), nil
}

// ExecProgress executes a command and returns its output as a string while displaying progress status
func ExecProgress(message string, command string, args ...string) (string, error) {
	cmd := execCommand(command, args...)

	// Set up pipes to capture stdout and stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	// Start the command execution
	if err := cmd.Start(); err != nil {
		return "", err
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	errChan := make(chan error, 2) // Channel to capture errors from goroutines

	// Goroutine to read and process stdout
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		progress := "" // Initialize progress string
		for scanner.Scan() {
			line := scanner.Text()
			stdoutBuf.WriteString(line + "\n")   // Append line to stdout buffer
			progress += "."                      // Append a dot to the progress string
			fmt.Print("\r" + message + progress) // Print progress message with accumulated dots
		}
		if err := scanner.Err(); err != nil {
			errChan <- fmt.Errorf("error reading stdout: %w", err)
		} else {
			errChan <- nil
		}
	}()

	// Goroutine to read and process stderr
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			stderrBuf.WriteString(line + "\n") // Append line to stderr buffer
		}
		if err := scanner.Err(); err != nil {
			errChan <- fmt.Errorf("error reading stderr: %w", err)
		} else {
			errChan <- nil
		}
	}()

	// Wait for the command to complete
	if err := cmdWait(cmd); err != nil {
		return "", fmt.Errorf("command execution failed: %w\n%s", err, stderrBuf.String())
	}

	// Check for errors from the stdout and stderr goroutines
	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			return "", err
		}
	}

	return stdoutBuf.String(), nil // Return the captured stdout as a string
}
