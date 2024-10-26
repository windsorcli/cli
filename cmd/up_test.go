package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/shell"
)

func TestUpCmd(t *testing.T) {
	// Save and restore the original exit function
	originalExitFunc := exitFunc
	exitFunc = func(code int) {
		mockExit(code, "")
	}
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a valid context, config handler, and shell
		mockContextInstance := context.NewMockContext()
		mockCliConfigHandler := config.NewMockConfigHandler()
		mockShell, _ := shell.NewMockShell("unix")

		// Mock functions
		mockContextInstance.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		driver := "colima"
		mockCliConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: &driver,
				},
			}, nil
		}
		mockShell.ExecFunc = func(command string, args ...string) (string, error) {
			expectedCommand := fmt.Sprintf("colima start windsor-%s", "test-context")
			if command == expectedCommand {
				return "colima started", nil
			}
			return "", fmt.Errorf("unexpected command: %s", command)
		}

		// Setup container with mock dependencies
		deps := MockDependencies{
			CLIConfigHandler: mockCliConfigHandler,
			Shell:            mockShell,
			ContextInstance:  mockContextInstance,
		}
		setupContainer(deps)

		// Capture stdout
		output := captureStdout(func() {
			// Execute the 'windsor up' command
			rootCmd.SetArgs([]string{"up"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Verify the output
		expectedOutput := "colima started\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorGettingContext", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a context that returns an error
		mockContextInstance := context.NewMockContext()
		mockContextInstance.GetContextFunc = func() (string, error) {
			return "", errors.New("context error")
		}

		// Setup container with mock context
		deps := MockDependencies{
			ContextInstance: mockContextInstance,
		}
		container := setupContainer(deps)
		Initialize(container)

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// Execute the 'windsor up' command
		rootCmd.SetArgs([]string{"up"})
		err := rootCmd.Execute()

		// Verify error
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		expectedError := "Error getting context: context error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorGettingConfigVerbose", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Set verbose mode to true
		verbose = true
		defer func() { verbose = false }() // Reset after test

		// Given a context and config handler that returns an error
		mockContextInstance := context.NewMockContext()
		mockCliConfigHandler := config.NewMockConfigHandler()

		// Mock functions with errors
		mockContextInstance.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockCliConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, errors.New("config error")
		}

		// Setup container with mock dependencies
		deps := MockDependencies{
			CLIConfigHandler: mockCliConfigHandler,
			ContextInstance:  mockContextInstance,
		}
		container := setupContainer(deps)
		Initialize(container)

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// Execute the 'windsor up' command
		rootCmd.SetArgs([]string{"up"})
		err := rootCmd.Execute()

		// Verify error
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		expectedError := "Error getting context configuration: config error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorGettingConfigNonVerbose", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Ensure verbose mode is false
		verbose = false

		// Given a context and config handler that returns an error
		mockContextInstance := context.NewMockContext()
		mockCliConfigHandler := config.NewMockConfigHandler()

		// Mock functions with errors
		mockContextInstance.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockCliConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, errors.New("config error")
		}

		// Setup container with mock dependencies
		deps := MockDependencies{
			CLIConfigHandler: mockCliConfigHandler,
			ContextInstance:  mockContextInstance,
		}
		container := setupContainer(deps)
		Initialize(container)

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// Execute the 'windsor up' command
		rootCmd.SetArgs([]string{"up"})
		err := rootCmd.Execute()

		// Verify no error as verbose is false
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("DriverNotColima", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a context and config handler with a different driver
		mockContextInstance := context.NewMockContext()
		mockCliConfigHandler := config.NewMockConfigHandler()
		mockShell, _ := shell.NewMockShell("unix")
		execCalled := false

		// Mock functions
		mockContextInstance.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		driver := "otherdriver"
		mockCliConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: &driver,
				},
			}, nil
		}
		mockShell.ExecFunc = func(command string, args ...string) (string, error) {
			execCalled = true
			return "", errors.New("should not be called")
		}

		// Setup container
		deps := MockDependencies{
			CLIConfigHandler: mockCliConfigHandler,
			Shell:            mockShell,
			ContextInstance:  mockContextInstance,
		}
		container := setupContainer(deps)
		Initialize(container)

		// Execute the 'windsor up' command
		rootCmd.SetArgs([]string{"up"})
		err := rootCmd.Execute()

		// Verify no error and that shell Exec was not called
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if execCalled {
			t.Error("Expected shell Exec not to be called")
		}
	})

	t.Run("ErrorExecutingShellCommandVerbose", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Set verbose mode to true
		verbose = true
		defer func() { verbose = false }() // Reset after test

		// Given a context, config handler, and shell that returns an error
		mockContextInstance := context.NewMockContext()
		mockCliConfigHandler := config.NewMockConfigHandler()
		mockShell, _ := shell.NewMockShell("unix")

		// Mock functions with errors
		mockContextInstance.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		driver := "colima"
		mockCliConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: &driver,
				},
			}, nil
		}
		mockShell.ExecFunc = func(command string, args ...string) (string, error) {
			return "", errors.New("shell command error")
		}

		// Setup container
		deps := MockDependencies{
			CLIConfigHandler: mockCliConfigHandler,
			Shell:            mockShell,
			ContextInstance:  mockContextInstance,
		}
		container := setupContainer(deps)
		Initialize(container)

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// Execute the 'windsor up' command
		rootCmd.SetArgs([]string{"up"})
		err := rootCmd.Execute()

		// Verify error
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		expectedError := "shell command error"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorExecutingShellCommandNonVerbose", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Ensure verbose mode is false
		verbose = false

		// Given a context, config handler, and shell that returns an error
		mockContextInstance := context.NewMockContext()
		mockCliConfigHandler := config.NewMockConfigHandler()
		mockShell, _ := shell.NewMockShell("unix")

		// Mock functions with errors
		mockContextInstance.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		driver := "colima"
		mockCliConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: &driver,
				},
			}, nil
		}
		mockShell.ExecFunc = func(command string, args ...string) (string, error) {
			return "", errors.New("shell command error")
		}

		// Setup container
		deps := MockDependencies{
			CLIConfigHandler: mockCliConfigHandler,
			Shell:            mockShell,
			ContextInstance:  mockContextInstance,
		}
		container := setupContainer(deps)
		Initialize(container)

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// Execute the 'windsor up' command
		rootCmd.SetArgs([]string{"up"})
		err := rootCmd.Execute()

		// Verify no error as verbose is false
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	// Additional test cases can be added here
}
