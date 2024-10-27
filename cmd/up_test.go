package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/helpers"
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

	// Save and restore the original container
	originalContainer := container
	t.Cleanup(func() {
		container = originalContainer
	})

	t.Run("Success", func(t *testing.T) {
		// Mock the necessary dependencies
		mockShell, _ := shell.NewMockShell("cmd")
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			switch command {
			case "colima":
				if args[0] == "start" {
					return "colima start successful", nil
				}
				if args[0] == "ls" && args[1] == "--profile" {
					colimaInfo := `{
						"address": "192.168.5.2",
						"arch": "x86_64",
						"cpus": 4,
						"disk": 10737418240,
						"memory": 2147483648,
						"name": "windsor-test-context",
						"runtime": "docker",
						"status": "Running"
					}`
					return colimaInfo, nil
				}
			case "docker-compose":
				if args[0] == "up" {
					return "docker-compose up successful", nil
				}
			case "docker":
				if args[0] == "info" {
					return "docker info successful", nil
				}
				if args[0] == "ps" && args[1] == "--filter" && args[2] == "label=managed_by=windsor" && args[3] == "--format" && args[4] == "{{.ID}}" {
					dockerPsInfo := `1234567890ab
abcdef123456`
					return dockerPsInfo, nil
				}
				if args[0] == "inspect" {
					dockerInspectInfo := `{
						"role": "example-role",
						"com.docker.compose.service": "example-service"
					}`
					return dockerInspectInfo, nil
				}
			}
			return "", nil
		}

		mockCliConfigHandler := config.NewMockConfigHandler()
		mockCliConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			driver := "colima" // Define the driver variable
			return &config.Context{
				VM: &config.VMConfig{
					Driver: &driver,
				},
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
			}, nil
		}

		mockContextInstance := context.NewMockContext()
		mockContextInstance.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		// Setup container with mock dependencies
		deps := MockDependencies{
			CLIConfigHandler: mockCliConfigHandler,
			Shell:            mockShell,
			ContextInstance:  mockContextInstance,
		}
		container := setupContainer(deps)
		t.Cleanup(func() {
			container = originalContainer
		})
		Initialize(container)

		// Capture output
		output := captureStdout(func() {
			// Execute the 'windsor up' command
			rootCmd.SetArgs([]string{"up"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Verify output contains success message
		expectedSuccessMessage := "Welcome to the Windsor Environment!"
		if !strings.Contains(output, expectedSuccessMessage) {
			t.Errorf("Expected output to contain success message '%s', got %q", expectedSuccessMessage, output)
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
		t.Cleanup(func() {
			container = originalContainer
		})
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

	t.Run("ErrorSettingEnvironmentVariable", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a context and config handler that returns valid data
		mockContextInstance := context.NewMockContext()
		mockCliConfigHandler := config.NewMockConfigHandler()
		mockHelper := helpers.NewMockHelper()

		// Mock functions with valid data
		mockContextInstance.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockCliConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: new(string),
				},
			}, nil
		}
		mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{"TEST_VAR": "test_value"}, nil
		}

		// Mock osSetenv to return an error
		originalOsSetenv := osSetenv
		osSetenv = func(key, value string) error {
			return fmt.Errorf("setenv error")
		}
		t.Cleanup(func() {
			osSetenv = originalOsSetenv
		})

		// Setup container with mock dependencies
		deps := MockDependencies{
			CLIConfigHandler: mockCliConfigHandler,
			ContextInstance:  mockContextInstance,
			ColimaHelper:     mockHelper,
		}
		container := setupContainer(deps)
		t.Cleanup(func() {
			container = originalContainer
		})
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
		expectedError := "Error setting environment variable TEST_VAR: setenv error"
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
		t.Cleanup(func() {
			container = originalContainer
		})
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
		t.Cleanup(func() {
			container = originalContainer
		})
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

	t.Run("ErrorExecutingColimaStart", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a context, config handler, and shell with colima start error
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
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "colima" && len(args) == 2 && args[0] == "start" && args[1] == "windsor-test-context" {
				return "", fmt.Errorf("colima start error")
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// Setup container with mock dependencies
		deps := MockDependencies{
			CLIConfigHandler: mockCliConfigHandler,
			Shell:            mockShell,
			ContextInstance:  mockContextInstance,
		}
		container := setupContainer(deps)
		t.Cleanup(func() {
			container = originalContainer
		})
		Initialize(container)

		// Execute the 'windsor up' command
		rootCmd.SetArgs([]string{"up"})
		err := rootCmd.Execute()

		// Verify the error
		expectedError := "Error executing command colima [start windsor-test-context]: colima start error\n"
		if err == nil || err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %v", expectedError, err)
		}
	})

	t.Run("DockerDaemonNotRunning", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a context, config handler, and shell with Docker daemon not running
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
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
			}, nil
		}
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "docker" && len(args) == 1 && args[0] == "info" {
				return "", fmt.Errorf("Docker daemon is not running")
			}
			if command == "colima" && len(args) == 2 && args[0] == "start" && args[1] == "windsor-test-context" {
				return "colima started", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// Setup container with mock dependencies
		deps := MockDependencies{
			CLIConfigHandler: mockCliConfigHandler,
			Shell:            mockShell,
			ContextInstance:  mockContextInstance,
		}
		container := setupContainer(deps)
		t.Cleanup(func() {
			container = originalContainer
		})
		Initialize(container)

		// Execute the 'windsor up' command
		rootCmd.SetArgs([]string{"up"})
		err := rootCmd.Execute()

		// Verify the error
		expectedError := "Docker daemon is not running: Docker daemon is not running"
		if err == nil || err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %v", expectedError, err)
		}
	})

	t.Run("VMNotSet", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a context and config handler with VM not set
		mockContextInstance := context.NewMockContext()
		mockCliConfigHandler := config.NewMockConfigHandler()

		// Mock functions
		mockContextInstance.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockCliConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: nil,
			}, nil
		}

		// Setup container
		deps := MockDependencies{
			CLIConfigHandler: mockCliConfigHandler,
			ContextInstance:  mockContextInstance,
		}
		container := setupContainer(deps)
		t.Cleanup(func() {
			container = originalContainer
		})
		Initialize(container)

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
		expectedOutput := ""
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("VMNotSetVerbose", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Ensure verbose mode is true
		verbose = true

		// Given a context and config handler with VM not set
		mockContextInstance := context.NewMockContext()
		mockCliConfigHandler := config.NewMockConfigHandler()

		// Mock functions
		mockContextInstance.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockCliConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: nil,
			}, nil
		}

		// Setup container
		deps := MockDependencies{
			CLIConfigHandler: mockCliConfigHandler,
			ContextInstance:  mockContextInstance,
		}
		container := setupContainer(deps)
		t.Cleanup(func() {
			container = originalContainer
		})
		Initialize(container)

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
		expectedOutput := "VM configuration is not set, skipping VM start\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("CollectEnvVarsError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a context and config handler with VM set
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
		// Mock the helper to produce an error
		mockHelper := helpers.NewMockHelper()
		mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("mock error collecting env vars")
		}

		// Setup container
		deps := MockDependencies{
			CLIConfigHandler: mockCliConfigHandler,
			Shell:            mockShell,
			ContextInstance:  mockContextInstance,
			ColimaHelper:     mockHelper,
		}
		container := setupContainer(deps)
		t.Cleanup(func() {
			container = originalContainer
		})
		Initialize(container)

		// Execute the 'windsor up' command with --verbose flag
		rootCmd.SetArgs([]string{"up", "--verbose"})
		err := rootCmd.Execute()

		// Verify the error
		if err == nil || !strings.Contains(err.Error(), "mock error collecting env vars") {
			t.Fatalf("Expected error containing 'mock error collecting env vars', got %v", err)
		}
	})

	// Test case for error when writing Colima config
	t.Run("VMDriverIsColima_ErrorWritingColimaConfig", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a context, config handler, and colima helper that returns an error when writing config
		mockContextInstance := context.NewMockContext()
		mockCliConfigHandler := config.NewMockConfigHandler()
		mockShell, _ := shell.NewMockShell("unix")

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

		// Mock colima helper
		mockColimaHelper := helpers.NewMockHelper()
		mockColimaHelper.WriteConfigFunc = func() error {
			return errors.New("write config error")
		}

		// Mock the helper to return environment variables
		mockHelper := helpers.NewMockHelper()
		mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{"TEST_VAR": "test_value"}, nil
		}

		// Setup container
		deps := MockDependencies{
			CLIConfigHandler: mockCliConfigHandler,
			Shell:            mockShell,
			ContextInstance:  mockContextInstance,
			ColimaHelper:     mockColimaHelper,
		}
		container := setupContainer(deps)
		t.Cleanup(func() {
			container = originalContainer
		})
		Initialize(container)

		// Execute the 'windsor up' command
		rootCmd.SetArgs([]string{"up"})
		err := rootCmd.Execute()

		// Verify error
		expectedError := "Error writing colima config: write config error"
		if err == nil || err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %v", expectedError, err)
		}
	})

	// Test case for Docker enabled and error starting docker-compose
	t.Run("DockerEnabled_ErrorStartingDockerCompose", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a context, config handler with Docker enabled, and shell returning error on docker-compose up
		mockContextInstance := context.NewMockContext()
		mockCliConfigHandler := config.NewMockConfigHandler()
		mockShell, _ := shell.NewMockShell("unix")

		mockContextInstance.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		driver := "colima"
		dockerEnabled := true
		mockCliConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: &driver,
				},
				Docker: &config.DockerConfig{
					Enabled: &dockerEnabled,
				},
			}, nil
		}

		// Mock environment variables
		mockHelper := helpers.NewMockHelper()
		mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{"TEST_VAR": "test_value"}, nil
		}

		// Mock shell commands
		callCount := 0
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "colima" && args[0] == "start" {
				return "colima started", nil
			} else if command == "docker" && args[0] == "info" {
				return "Docker daemon info", nil
			} else if command == "docker-compose" && args[0] == "up" {
				callCount++
				return "", errors.New("docker-compose up error")
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// Setup container
		deps := MockDependencies{
			CLIConfigHandler: mockCliConfigHandler,
			Shell:            mockShell,
			ContextInstance:  mockContextInstance,
			ColimaHelper:     mockHelper,
		}
		container := setupContainer(deps)
		t.Cleanup(func() {
			container = originalContainer
		})
		Initialize(container)

		// Capture output
		output := captureStdout(func() {
			// Execute the 'windsor up' command
			rootCmd.SetArgs([]string{"up"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatal("Expected error, got nil")
			}
			if !strings.Contains(err.Error(), "docker-compose up error") {
				t.Errorf("Expected error containing 'docker-compose up error', got %q", err.Error())
			}
		})

		// Verify that it retried the expected number of times
		if callCount != 3 {
			t.Errorf("Expected docker-compose up to be called 3 times, was called %d times", callCount)
		}

		// Verify output contains retry messages
		expectedRetryMessage := "Retrying docker-compose up..."
		retryCount := strings.Count(output, expectedRetryMessage)
		if retryCount != 2 { // it retries 2 times after the initial failure
			t.Errorf("Expected 2 retries, got %d", retryCount)
		}
	})

	t.Run("DockerComposeRetryLogic", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a context, config handler, and shell with docker-compose up error
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
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
			}, nil
		}
		callCount := 0
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			switch command {
			case "colima":
				if args[0] == "start" {
					return "colima started", nil
				}
				if args[0] == "ls" && args[1] == "--profile" {
					return `{"address":"192.168.5.2","arch":"x86_64","cpus":4,"disk":10737418240,"memory":2147483648,"name":"windsor-test-context","runtime":"docker","status":"Running"}`, nil
				}
			case "docker":
				if args[0] == "info" {
					return "Docker daemon info", nil
				}
				if args[0] == "ps" && args[1] == "--filter" {
					return "container_id_1\ncontainer_id_2\ncontainer_id_3", nil
				}
				if args[0] == "inspect" {
					return `{"role":"example-role","com.docker.compose.service":"example-service"}`, nil
				}
			case "docker-compose":
				if args[0] == "up" {
					callCount++
					if callCount < 3 {
						return "", fmt.Errorf("docker-compose up error")
					}
					return "docker-compose up successful", nil
				}
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// Setup container with mock dependencies
		deps := MockDependencies{
			CLIConfigHandler: mockCliConfigHandler,
			Shell:            mockShell,
			ContextInstance:  mockContextInstance,
		}
		container := setupContainer(deps)
		t.Cleanup(func() {
			container = originalContainer
		})
		Initialize(container)

		// Capture output
		output := captureStdout(func() {
			// Execute the 'windsor up' command
			rootCmd.SetArgs([]string{"up"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Verify that it retried the expected number of times
		if callCount != 3 {
			t.Errorf("Expected docker-compose up to be called 3 times, was called %d times", callCount)
		}

		// Verify output contains retry messages
		expectedRetryMessage := "Retrying docker-compose up..."
		retryCount := strings.Count(output, expectedRetryMessage)
		if retryCount != 2 { // it retries 2 times after the initial failure
			t.Errorf("Expected 2 retries, got %d", retryCount)
		}

		// Verify output contains partial success message
		expectedSuccessMessage := "Welcome to the Windsor Environment!"
		if !strings.Contains(output, expectedSuccessMessage) {
			t.Errorf("Expected output to contain partial success message '%s', got %q", expectedSuccessMessage, output)
		}
	})

	t.Run("InvalidColimaInfoJSON", func(t *testing.T) {
		// Mock the necessary dependencies
		mockShell, _ := shell.NewMockShell("cmd")
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			switch command {
			case "colima":
				if args[0] == "start" {
					return "colima start successful", nil
				}
				if args[0] == "ls" && args[1] == "--profile" {
					// Return invalid JSON
					return "{invalid-json}", nil
				}
			case "docker-compose":
				if args[0] == "up" {
					return "docker-compose up successful", nil
				}
			case "docker":
				if args[0] == "info" {
					return "docker info successful", nil
				}
				if args[0] == "ps" && args[1] == "--filter" && args[2] == "label=managed_by=windsor" && args[3] == "--format" && args[4] == "{{.ID}}" {
					dockerPsInfo := `1234567890ab
	abcdef123456`
					return dockerPsInfo, nil
				}
				if args[0] == "inspect" {
					dockerInspectInfo := `{
							"role": "example-role",
							"com.docker.compose.service": "example-service"
						}`
					return dockerInspectInfo, nil
				}
			}
			return "", nil
		}

		mockCliConfigHandler := config.NewMockConfigHandler()
		mockCliConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			driver := "colima" // Define the driver variable
			return &config.Context{
				VM: &config.VMConfig{
					Driver: &driver,
				},
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
			}, nil
		}

		mockContextInstance := context.NewMockContext()
		mockContextInstance.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		// Setup container with mock dependencies
		deps := MockDependencies{
			CLIConfigHandler: mockCliConfigHandler,
			Shell:            mockShell,
			ContextInstance:  mockContextInstance,
		}
		container := setupContainer(deps)
		t.Cleanup(func() {
			container = originalContainer
		})
		Initialize(container)

		// Capture output
		output := captureStdout(func() {
			// Execute the 'windsor up' command
			rootCmd.SetArgs([]string{"up"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error due to invalid JSON, but got nil")
			}
		})

		// Verify output contains error message for invalid JSON
		expectedErrorMessage := "Error fetching Colima info"
		if !strings.Contains(output, expectedErrorMessage) {
			t.Errorf("Expected output to contain error message '%s', got %q", expectedErrorMessage, output)
		}
	})
}

func TestGetDockerServicesInfo(t *testing.T) {
	// Mock the necessary dependencies
	mockShell, _ := shell.NewMockShell("cmd")
	mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
		switch command {
		case "docker":
			if args[0] == "ps" && args[1] == "--filter" && args[2] == "label=managed_by=windsor" && args[3] == "--format" && args[4] == "{{.ID}}" {
				return "1234567890ab\nabcdef123456", nil
			}
			if args[0] == "inspect" {
				if args[1] == "1234567890ab" {
					return `{"role": "unknown-role", "com.docker.compose.service": "example-service"}`, nil
				}
				if args[1] == "abcdef123456" {
					return `{"com.docker.compose.service": "example-service"}`, nil
				}
			}
		}
		return "", nil
	}

	// Test case: container without 'role' label
	t.Run("Container without role label", func(t *testing.T) {
		shellInstance = mockShell
		output, err := getDockerServicesInfo()
		if err != nil {
			t.Fatalf("getDockerServicesInfo() error = %v", err)
		}
		expectedOutput := ""
		if output != expectedOutput {
			t.Errorf("Expected output to be '%s', got '%s'", expectedOutput, output)
		}
	})

	// Test case: container with unknown role
	t.Run("Container with unknown role", func(t *testing.T) {
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			switch command {
			case "docker":
				if args[0] == "ps" && args[1] == "--filter" && args[2] == "label=managed_by=windsor" && args[3] == "--format" && args[4] == "{{.ID}}" {
					return "1234567890ab", nil
				}
				if args[0] == "inspect" {
					return `{"role": "unknown-role", "com.docker.compose.service": "example-service"}`, nil
				}
			}
			return "", nil
		}
		shellInstance = mockShell
		output, err := getDockerServicesInfo()
		if err != nil {
			t.Fatalf("getDockerServicesInfo() error = %v", err)
		}
		expectedOutput := ""
		if output != expectedOutput {
			t.Errorf("Expected output to be '%s', got '%s'", expectedOutput, output)
		}
	})

	// Test case: container without 'com.docker.compose.service' label
	t.Run("Container without com.docker.compose.service label", func(t *testing.T) {
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			switch command {
			case "docker":
				if args[0] == "ps" && args[1] == "--filter" && args[2] == "label=managed_by=windsor" && args[3] == "--format" && args[4] == "{{.ID}}" {
					return "abcdef123456", nil
				}
				if args[0] == "inspect" {
					return `{"role": "worker"}`, nil
				}
			}
			return "", nil
		}
		shellInstance = mockShell
		output, err := getDockerServicesInfo()
		if err != nil {
			t.Fatalf("getDockerServicesInfo() error = %v", err)
		}
		expectedOutput := ""
		if output != expectedOutput {
			t.Errorf("Expected output to be '%s', got '%s'", expectedOutput, output)
		}
	})
}
