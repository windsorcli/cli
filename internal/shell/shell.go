package shell

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/ssh"
)

// maxDepth is the maximum depth to search for the project root
const maxDepth = 10

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
	Exec(verbose bool, message string, command string, args ...string) (string, error)
}

// DefaultShell is the default implementation of the Shell interface
type DefaultShell struct {
	projectRoot string
	mu          sync.Mutex
	injector    di.Injector
	sshClient   ssh.Client
}

// NewDefaultShell creates a new instance of DefaultShell
func NewDefaultShell(injector di.Injector) *DefaultShell {
	return &DefaultShell{
		injector: injector,
	}
}

func (s *DefaultShell) Initialize() error {
	// Get the SSH client
	sshClient, err := s.injector.Resolve("sshClient")
	if err != nil {
		return fmt.Errorf("failed to resolve SSH client: %w", err)
	}
	s.sshClient = sshClient.(ssh.Client)

	return nil
}

// GetProjectRoot retrieves the project root directory
func (s *DefaultShell) GetProjectRoot() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

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
		if depth > maxDepth {
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
func (s *DefaultShell) Exec(verbose bool, message string, command string, args ...string) (string, error) {
	cmd := execCommand(command, args...)

	// Buffers to capture output
	var stdoutBuf, stderrBuf bytes.Buffer

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmdStart(cmd); err != nil {
		return "", fmt.Errorf("failed to start command: %w", err)
	}

	var spinnerInstance *spinner.Spinner
	if message != "" {
		// Initialize spinner only if a message is provided
		spinnerInstance = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		spinnerInstance.Suffix = " " + message
		spinnerInstance.Start()
	}

	err := cmd.Wait()
	if spinnerInstance != nil {
		spinnerInstance.Stop()
	}

	if err != nil {
		// Print stderr on error
		fmt.Print(stderrBuf.String())
		return "", fmt.Errorf("command execution failed: %w", err)
	}

	if verbose {
		// Print stdout if verbose
		fmt.Print(stdoutBuf.String())
	}

	return stdoutBuf.String(), nil
}
