package env

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

type AwsEnvMocks struct {
	Injector       di.Injector
	ConfigHandler  *config.MockConfigHandler
	ContextHandler *context.MockContext
	Shell          *shell.MockShell
}

func setupSafeAwsEnvMocks(injector ...di.Injector) *AwsEnvMocks {
	// Use the provided injector or create a new one if not provided
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewMockInjector()
	}

	// Create a mock ConfigHandler using its constructor
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetConfigFunc = func() *config.Context {
		return &config.Context{
			AWS: &config.AWSConfig{
				AWSProfile:     stringPtr("default"),
				AWSEndpointURL: stringPtr("https://aws.endpoint"),
				S3Hostname:     stringPtr("s3.amazonaws.com"),
				MWAAEndpoint:   stringPtr("https://mwaa.endpoint"),
			},
		}
	}
	mockContext := context.NewMockContext()
	mockContext.GetConfigRootFunc = func() (string, error) {
		return filepath.FromSlash("/mock/config/root"), nil
	}

	// Create a mock Shell using its constructor
	mockShell := shell.NewMockShell()

	// Register the mocks in the DI injector
	mockInjector.Register("configHandler", mockConfigHandler)
	mockInjector.Register("contextHandler", mockContext)
	mockInjector.Register("shell", mockShell)

	return &AwsEnvMocks{
		Injector:       mockInjector,
		ConfigHandler:  mockConfigHandler,
		ContextHandler: mockContext,
		Shell:          mockShell,
	}
}

func TestAwsEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSafeAwsEnvMocks to create mocks
		mocks := setupSafeAwsEnvMocks()

		// Mock the stat function to simulate the existence of the AWS config file
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/.aws/config") {
				return nil, nil // Simulate that the file exists
			}
			return nil, os.ErrNotExist
		}

		awsEnvPrinter := NewAwsEnvPrinter(mocks.Injector)
		awsEnvPrinter.Initialize()

		// When calling GetEnvVars
		envVars, err := awsEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then the environment variables should be set correctly
		expectedConfigFile := filepath.FromSlash("/mock/config/root/.aws/config")
		if envVars["AWS_CONFIG_FILE"] != expectedConfigFile {
			t.Errorf("AWS_CONFIG_FILE = %v, want %v", envVars["AWS_CONFIG_FILE"], expectedConfigFile)
		}
	})

	t.Run("MissingConfiguration", func(t *testing.T) {
		// Use setupSafeAwsEnvMocks to create mocks
		mocks := setupSafeAwsEnvMocks()

		// Override the GetConfigFunc to return nil for AWS configuration
		mocks.ConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{AWS: nil}
		}

		mockInjector := mocks.Injector

		awsEnvPrinter := NewAwsEnvPrinter(mockInjector)
		awsEnvPrinter.Initialize()

		// Capture stdout
		output := captureStdout(t, func() {
			// When calling GetEnvVars
			_, err := awsEnvPrinter.GetEnvVars()
			if err != nil {
				fmt.Println(err)
			}
		})

		// Then the output should indicate the missing configuration
		expectedOutput := "context configuration or AWS configuration is missing\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("NoAwsConfigFile", func(t *testing.T) {
		// Use setupSafeAwsEnvMocks to create mocks
		mocks := setupSafeAwsEnvMocks()

		// Override the GetConfigFunc to return a valid AWS configuration
		mocks.ConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				AWS: &config.AWSConfig{
					AWSProfile:     stringPtr("default"),
					AWSEndpointURL: stringPtr("https://example.com"),
					S3Hostname:     stringPtr("s3.example.com"),
					MWAAEndpoint:   stringPtr("mwaa.example.com"),
				},
			}
		}

		// Override the GetConfigRootFunc to return a valid path
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "/non/existent/path", nil
		}

		mockInjector := mocks.Injector

		awsEnvPrinter := NewAwsEnvPrinter(mockInjector)
		awsEnvPrinter.Initialize()

		// Capture stdout
		output := captureStdout(t, func() {
			// When calling GetEnvVars
			_, err := awsEnvPrinter.GetEnvVars()
			if err != nil {
				fmt.Println(err)
			}
		})

		// Then the output should not include AWS_CONFIG_FILE and should not indicate an error
		if output != "" {
			t.Errorf("output = %v, want empty output", output)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		// Use setupSafeAwsEnvMocks to create mocks
		mocks := setupSafeAwsEnvMocks()

		// Override the GetConfigRootFunc to simulate an error
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}

		mockInjector := mocks.Injector

		awsEnvPrinter := NewAwsEnvPrinter(mockInjector)
		awsEnvPrinter.Initialize()

		// Capture stdout
		output := captureStdout(t, func() {
			// When calling GetEnvVars
			_, err := awsEnvPrinter.GetEnvVars()
			if err != nil {
				fmt.Println(err)
			}
		})

		// Then the output should indicate the error
		expectedOutput := "error retrieving configuration root directory: mock context error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})
}

func TestAwsEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSafeAwsEnvMocks to create mocks
		mocks := setupSafeAwsEnvMocks()
		mockInjector := mocks.Injector
		awsEnvPrinter := NewAwsEnvPrinter(mockInjector)
		awsEnvPrinter.Initialize()

		// Mock the stat function to simulate the existence of the AWS config file
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/.aws/config") {
				return nil, nil // Simulate that the file exists
			}
			return nil, os.ErrNotExist
		}

		// Mock the PrintEnvVarsFunc to verify it is called with the correct envVars
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
		}

		// Call Print and check for errors
		err := awsEnvPrinter.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that PrintEnvVarsFunc was called with the correct envVars
		expectedEnvVars := map[string]string{
			"AWS_CONFIG_FILE":  filepath.FromSlash("/mock/config/root/.aws/config"),
			"AWS_PROFILE":      "default",
			"AWS_ENDPOINT_URL": "https://aws.endpoint",
			"S3_HOSTNAME":      "s3.amazonaws.com",
			"MWAA_ENDPOINT":    "https://mwaa.endpoint",
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Use setupSafeAwsEnvMocks to create mocks
		mocks := setupSafeAwsEnvMocks()

		// Set AWS configuration to nil to simulate the error condition
		mocks.ConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				AWS: nil,
			}
		}

		mockInjector := mocks.Injector
		awsEnvPrinter := NewAwsEnvPrinter(mockInjector)
		awsEnvPrinter.Initialize()

		// Call Print and expect an error
		err := awsEnvPrinter.Print()
		if err == nil {
			t.Error("expected error, got nil")
		}

		// Verify the error message
		expectedError := "error getting environment variables: context configuration or AWS configuration is missing"
		if err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err.Error(), expectedError)
		}
	})
}
