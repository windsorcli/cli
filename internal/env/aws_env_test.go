package env

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

type AwsEnvMocks struct {
	Container      di.ContainerInterface
	ConfigHandler  *config.MockConfigHandler
	ContextHandler *context.MockContext
	Shell          *shell.MockShell
}

func setupSafeAwsEnvMocks(container ...di.ContainerInterface) *AwsEnvMocks {
	// Use the provided DI container or create a new one if not provided
	var mockContainer di.ContainerInterface
	if len(container) > 0 {
		mockContainer = container[0]
	} else {
		mockContainer = di.NewContainer()
	}

	// Create a mock ConfigHandler using its constructor
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
		return &config.Context{
			AWS: &config.AWSConfig{
				AWSProfile:     stringPtr("default"),
				AWSEndpointURL: stringPtr("https://aws.endpoint"),
				S3Hostname:     stringPtr("s3.amazonaws.com"),
				MWAAEndpoint:   stringPtr("https://mwaa.endpoint"),
			},
		}, nil
	}

	// Create a mock Context using its constructor
	mockContext := context.NewMockContext()
	mockContext.GetConfigRootFunc = func() (string, error) {
		return "/mock/config/root", nil
	}

	// Create a mock Shell using its constructor
	mockShell := shell.NewMockShell()

	// Register the mocks in the DI container
	mockContainer.Register("cliConfigHandler", mockConfigHandler)
	mockContainer.Register("contextHandler", mockContext)
	mockContainer.Register("shell", mockShell)

	return &AwsEnvMocks{
		Container:      mockContainer,
		ConfigHandler:  mockConfigHandler,
		ContextHandler: mockContext,
		Shell:          mockShell,
	}
}

func TestAwsEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSafeAwsEnvMocks to create mocks
		mocks := setupSafeAwsEnvMocks()

		// Override the PrintEnvVarsFunc to ensure success
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) {
			t.Log("PrintEnvVarsFunc called successfully with envVars:", envVars)
		}

		// Mock the stat function to simulate the existence of the AWS config file
		stat = func(name string) (os.FileInfo, error) {
			if name == "/mock/config/root/.aws/config" {
				return nil, nil // Simulate that the file exists
			}
			return nil, os.ErrNotExist
		}

		awsEnv := NewAwsEnv(mocks.Container)

		// When calling Print
		envVars := make(map[string]string)
		awsEnv.Print(envVars)

		// Then the environment variables should be set correctly
		if envVars["AWS_CONFIG_FILE"] != "/mock/config/root/.aws/config" {
			t.Errorf("AWS_CONFIG_FILE = %v, want %v", envVars["AWS_CONFIG_FILE"], "/mock/config/root/.aws/config")
		}
	})

	t.Run("ResolveConfigHandlerError", func(t *testing.T) {
		// Create a mock DI container using NewMockContainer
		mockContainer := di.NewMockContainer()

		// Use setupSafeAwsEnvMocks to create mocks with the mock container
		setupSafeAwsEnvMocks(mockContainer)

		// Set the resolve error for cliConfigHandler in the mock container
		mockContainer.SetResolveError("cliConfigHandler", fmt.Errorf("mock resolve error"))

		awsEnv := NewAwsEnv(mockContainer)

		// Capture stdout
		output := captureStdout(t, func() {
			// When calling Print
			envVars := make(map[string]string)
			awsEnv.Print(envVars)
		})

		// Then the output should indicate the resolve error
		expectedOutput := "Error resolving cliConfigHandler: mock resolve error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("AssertConfigHandlerError", func(t *testing.T) {
		// Create a normal DI container
		container := di.NewContainer()

		// Register an invalid config handler with the container
		container.Register("cliConfigHandler", "invalidType")

		awsEnv := NewAwsEnv(container)

		// Capture stdout
		output := captureStdout(t, func() {
			// When calling Print
			envVars := make(map[string]string)
			awsEnv.Print(envVars)
		})

		// Then the output should indicate the type assertion error
		expectedOutput := "Failed to cast cliConfigHandler to config.ConfigHandler\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("ResolveContextHandlerError", func(t *testing.T) {
		// Create a mock DI container using NewMockContainer
		mockContainer := di.NewMockContainer()

		// Use setupSafeAwsEnvMocks to create mocks with the mock container
		setupSafeAwsEnvMocks(mockContainer)

		// Set the resolve error for contextHandler in the mock container
		mockContainer.SetResolveError("contextHandler", fmt.Errorf("mock resolve error"))

		awsEnv := NewAwsEnv(mockContainer)

		// Capture stdout
		output := captureStdout(t, func() {
			// When calling Print
			envVars := make(map[string]string)
			awsEnv.Print(envVars)
		})

		// Then the output should indicate the resolve error
		expectedOutput := "Error resolving contextHandler: mock resolve error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("AssertContextHandlerError", func(t *testing.T) {
		// Create a mock DI container using NewContainer
		mockContainer := di.NewContainer()

		// Use setupSafeAwsEnvMocks to create mocks with the mock container
		setupSafeAwsEnvMocks(mockContainer)

		// Register an invalid context handler with the container
		mockContainer.Register("contextHandler", "invalidType")

		awsEnv := NewAwsEnv(mockContainer)

		// Capture stdout
		output := captureStdout(t, func() {
			// When calling Print
			envVars := make(map[string]string)
			awsEnv.Print(envVars)
		})

		// Then the output should indicate the type assertion error
		expectedOutput := "Failed to cast contextHandler to context.ContextInterface\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("ResolveShellError", func(t *testing.T) {
		// Create a mock DI container using NewMockContainer
		mockContainer := di.NewMockContainer()

		// Use setupSafeAwsEnvMocks to create mocks with the mock container
		setupSafeAwsEnvMocks(mockContainer)

		// Set the resolve error for shell in the mock container
		mockContainer.SetResolveError("shell", fmt.Errorf("mock resolve error"))

		awsEnv := NewAwsEnv(mockContainer)

		// Capture stdout
		output := captureStdout(t, func() {
			// When calling Print
			envVars := make(map[string]string)
			awsEnv.Print(envVars)
		})

		// Then the output should indicate the resolve error
		expectedOutput := "Error resolving shell: mock resolve error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("AssertShellError", func(t *testing.T) {
		// Create a mock DI container using NewMockContainer
		mockContainer := di.NewMockContainer()

		// Use setupSafeAwsEnvMocks to create mocks with the mock container
		setupSafeAwsEnvMocks(mockContainer)

		// Register an invalid shell with the container
		mockContainer.Register("shell", "invalidType")

		awsEnv := NewAwsEnv(mockContainer)

		// Capture stdout
		output := captureStdout(t, func() {
			// When calling Print
			envVars := make(map[string]string)
			awsEnv.Print(envVars)
		})

		// Then the output should indicate the type assertion error
		expectedOutput := "Failed to cast shell to shell.Shell\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("ConfigHandlerError", func(t *testing.T) {
		// Use setupSafeAwsEnvMocks to create mocks
		mocks := setupSafeAwsEnvMocks()

		// Override the GetConfigFunc to simulate an error
		mocks.ConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, errors.New("mock config error")
		}

		mockContainer := mocks.Container

		awsEnv := NewAwsEnv(mockContainer)

		// Capture stdout
		output := captureStdout(t, func() {
			// When calling Print
			envVars := make(map[string]string)
			awsEnv.Print(envVars)
		})

		// Then the output should indicate the error
		expectedOutput := "Error retrieving context configuration: mock config error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("MissingConfiguration", func(t *testing.T) {
		// Use setupSafeAwsEnvMocks to create mocks
		mocks := setupSafeAwsEnvMocks()

		// Override the GetConfigFunc to return nil for AWS configuration
		mocks.ConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{AWS: nil}, nil
		}

		mockContainer := mocks.Container

		awsEnv := NewAwsEnv(mockContainer)

		// Capture stdout
		output := captureStdout(t, func() {
			// When calling Print
			envVars := make(map[string]string)
			awsEnv.Print(envVars)
		})

		// Then the output should indicate the missing configuration
		expectedOutput := "Context configuration or AWS configuration is missing\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("NoAwsConfigFile", func(t *testing.T) {
		// Use setupSafeAwsEnvMocks to create mocks
		mocks := setupSafeAwsEnvMocks()

		// Override the GetConfigFunc to return a valid AWS configuration
		mocks.ConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				AWS: &config.AWSConfig{
					AWSProfile:     stringPtr("default"),
					AWSEndpointURL: stringPtr("https://example.com"),
					S3Hostname:     stringPtr("s3.example.com"),
					MWAAEndpoint:   stringPtr("mwaa.example.com"),
				},
			}, nil
		}

		// Override the GetConfigRootFunc to return a valid path
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "/non/existent/path", nil
		}

		mockContainer := mocks.Container

		awsEnv := NewAwsEnv(mockContainer)

		// Capture stdout
		envVars := make(map[string]string)
		captureStdout(t, func() {
			// When calling Print
			awsEnv.Print(envVars)
		})

		// Then the output should not contain AWS_CONFIG_FILE
		if _, exists := envVars["AWS_CONFIG_FILE"]; exists {
			t.Errorf("AWS_CONFIG_FILE should not be set when no config file is present")
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		// Use setupSafeAwsEnvMocks to create mocks
		mocks := setupSafeAwsEnvMocks()

		// Override the GetConfigRootFunc to simulate an error
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}

		mockContainer := mocks.Container

		awsEnv := NewAwsEnv(mockContainer)

		// Capture stdout
		output := captureStdout(t, func() {
			// When calling Print
			envVars := make(map[string]string)
			awsEnv.Print(envVars)
		})

		// Then the output should indicate the error
		expectedOutput := "Error retrieving configuration root directory: mock context error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})
}

func stringPtr(s string) *string {
	return &s
}