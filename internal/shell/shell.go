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
	// PrintEnvVars prints the provided environment variables
	PrintEnvVars(envVars map[string]string)
	// GetProjectRoot retrieves the project root directory
	GetProjectRoot() (string, error)
	// Exec executes a command with optional privilege elevation
	Exec(verbose bool, message string, command string, args ...string) (string, error)
}

// DefaultShell is the default implementation of the Shell interface
type DefaultShell struct {
	projectRoot string
	mu          sync.Mutex
}

// NewDefaultShell creates a new instance of DefaultShell
func NewDefaultShell() *DefaultShell {
	return &DefaultShell{}
}

// GetProjectRoot retrieves the project root directory
func (d *DefaultShell) GetProjectRoot() (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Return cached project root if available
	if d.projectRoot != "" {
		return d.projectRoot, nil
	}

	// Try to get the git root first
	cmd := execCommand("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err == nil {
		d.projectRoot = strings.TrimSpace(string(output))
		return d.projectRoot, nil
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
			d.projectRoot = currentDir
			return d.projectRoot, nil
		}
		if _, err := os.Stat(windsorYml); err == nil {
			d.projectRoot = currentDir
			return d.projectRoot, nil
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
func (d *DefaultShell) Exec(verbose bool, message string, command string, args ...string) (string, error) {
	cmd := execCommand(command, args...)

	// Buffers to capture output
	var stdoutBuf, stderrBuf bytes.Buffer

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmdStart(cmd); err != nil {
		return "", fmt.Errorf("failed to start command: %w", err)
	}

	// Initialize spinner
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " " + message
	s.Start()

	err := cmd.Wait()
	s.Stop()

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
