package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
)

// Helper function to create a new container and register a mock config handler
func setupContainer(mockHandler config.ConfigHandler) di.ContainerInterface {
	container := di.NewContainer()
	container.Register("configHandler", mockHandler)
	Initialize(container)

	// Ensure configHandler is set correctly
	instance, err := container.Resolve("configHandler")
	if err != nil {
		panic("Error resolving configHandler: " + err.Error())
	}
	configHandler, _ = instance.(config.ConfigHandler)
	return container
}

// Helper function to capture stdout output
func captureStdout(f func()) string {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stdout = oldStdout
	return buf.String()
}

// Helper function to capture stderr output
func captureStderr(f func()) string {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	f()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stderr = oldStderr
	return buf.String()
}

func TestPreRunLoadConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a valid config handler
		mockHandler := config.NewMockConfigHandler(
			func(path string) error { return nil },
			func(key string) (string, error) { return "value", nil },
			nil, nil, nil, nil,
		)
		setupContainer(mockHandler)

		// When preRunLoadConfig is executed
		err := preRunLoadConfig(nil, nil)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("preRunLoadConfig() error = %v, expected no error", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a config handler that returns an error
		mockHandler := config.NewMockConfigHandler(
			func(path string) error { return errors.New("config load error") },
			func(key string) (string, error) { return "", errors.New("config load error") },
			nil, nil, nil, nil,
		)
		setupContainer(mockHandler)

		// When preRunLoadConfig is executed
		err := preRunLoadConfig(nil, nil)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("preRunLoadConfig() expected error, got nil")
		}
		expectedError := "Error loading config file: config load error"
		if err.Error() != expectedError {
			t.Fatalf("preRunLoadConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("NoConfigHandler", func(t *testing.T) {
		// Given no config handler is registered
		configHandler = nil

		// When preRunLoadConfig is executed
		err := preRunLoadConfig(nil, nil)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("preRunLoadConfig() expected error, got nil")
		}
		expectedError := "configHandler is not initialized"
		if err.Error() != expectedError {
			t.Fatalf("preRunLoadConfig() error = %v, expected '%s'", err, expectedError)
		}
	})
}

func TestExecute(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a valid config handler
		mockHandler := config.NewMockConfigHandler(
			func(path string) error { return nil },
			func(key string) (string, error) { return "value", nil },
			nil, nil, nil, nil,
		)
		setupContainer(mockHandler)

		// Mock exitFunc to capture the exit code
		var exitCode int
		exitFunc = func(code int) {
			exitCode = code
		}

		// Add a dummy subcommand to trigger PersistentPreRunE
		dummyCmd := &cobra.Command{
			Use: "dummy",
			Run: func(cmd *cobra.Command, args []string) {},
		}
		rootCmd.AddCommand(dummyCmd)
		rootCmd.SetArgs([]string{"dummy"})

		// When the command is executed
		err := rootCmd.Execute()

		// Then no error should be returned and exitFunc should not be called
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if exitCode != 0 {
			t.Errorf("exitFunc was called with code %d, expected 0", exitCode)
		}

		// Cleanup
		rootCmd.RemoveCommand(dummyCmd)
	})

	t.Run("LoadConfigError", func(t *testing.T) {
		// Given a config handler that returns an error
		mockHandler := config.NewMockConfigHandler(
			func(path string) error { return errors.New("config load error") },
			func(key string) (string, error) { return "", errors.New("config load error") },
			nil, nil, nil, nil,
		)
		setupContainer(mockHandler)

		// Mock exitFunc to capture the exit code
		var exitCode int
		exitFunc = func(code int) {
			exitCode = code
		}

		// Add a dummy subcommand to trigger PersistentPreRunE
		dummyCmd := &cobra.Command{
			Use: "dummy",
			Run: func(cmd *cobra.Command, args []string) {},
		}
		rootCmd.AddCommand(dummyCmd)
		rootCmd.SetArgs([]string{"dummy"})

		// When the command is executed and stderr is captured
		actualErrorMsg := captureStderr(func() {
			Execute()
		})

		// Then exitFunc should be called with code 1 and the error message should be printed to stderr
		if exitCode != 1 {
			t.Errorf("exitFunc was not called with code 1, got %d", exitCode)
		}
		expectedErrorMsg := "Error loading config file: config load error\n"
		if !strings.Contains(actualErrorMsg, expectedErrorMsg) {
			t.Errorf("Expected error message to contain '%s', got '%s'", expectedErrorMsg, actualErrorMsg)
		}

		// Cleanup
		rootCmd.RemoveCommand(dummyCmd)
	})
}

func TestInitialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a valid config handler
		mockHandler := config.NewMockConfigHandler(
			func(path string) error { return nil },
			func(key string) (string, error) { return "value", nil },
			nil, nil, nil, nil,
		)
		setupContainer(mockHandler)

		// Mock exitFunc to capture the exit code
		var exitCode int
		exitFunc = func(code int) {
			exitCode = code
		}

		// When the cmd package is initialized and stderr is captured
		actualErrorMsg := captureStderr(func() {
			Initialize(container)
		})

		// Then exitFunc should not be called and no error message should be printed to stderr
		if exitCode != 0 {
			t.Errorf("exitFunc was called with code %d, expected 0", exitCode)
		}
		if actualErrorMsg != "" {
			t.Errorf("Expected no error message, got '%s'", actualErrorMsg)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given no config handler is registered
		container := di.NewContainer()
		Initialize(container)

		// Mock exitFunc to capture the exit code
		var exitCode int
		exitFunc = func(code int) {
			exitCode = code
		}

		// When the cmd package is initialized and stderr is captured
		actualErrorMsg := captureStderr(func() {
			Initialize(container)
		})

		// Then exitFunc should be called with code 1 and the error message should be printed to stderr
		if exitCode != 1 {
			t.Errorf("exitFunc was not called with code 1, got %d", exitCode)
		}
		expectedErrorMsg := "Error resolving configHandler: no instance registered with name configHandler\n"
		if !strings.Contains(actualErrorMsg, expectedErrorMsg) {
			t.Errorf("Expected error message to contain '%s', got '%s'", expectedErrorMsg, actualErrorMsg)
		}
	})
}
