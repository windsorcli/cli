package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
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
	Exec(verbose bool, message string, command string, args ...string) (string, error)
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
func (s *DefaultShell) Exec(verbose bool, message string, command string, args ...string) (string, error) {
	cmd := execCommand(command, args...)

	var outputBuffer, errorBuffer strings.Builder
	cmd.Stdout = &outputBuffer
	cmd.Stderr = &errorBuffer

	// Always print the message if it is not empty
	if message != "" {
		fmt.Println(message)
	}

	// Start the command and handle errors
	errChan := make(chan error, 1)
	go func() {
		if err := cmdStart(cmd); err != nil {
			errChan <- fmt.Errorf("failed to start command: %w", err)
			return
		}

		if err := cmdWait(cmd); err != nil {
			errChan <- fmt.Errorf("command execution failed: %w\n%s", err, errorBuffer.String())
			return
		}

		errChan <- nil
	}()

	// Wait for the command to finish or an error to occur
	select {
	case err := <-errChan:
		if err != nil {
			return "", err
		}
	}

	return outputBuffer.String(), nil
}
