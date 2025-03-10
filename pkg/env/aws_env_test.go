package env

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/aws"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

type AwsEnvMocks struct {
	Injector      di.Injector
	ConfigHandler *config.MockConfigHandler
	Shell         *shell.MockShell
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
	mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
		return &v1alpha1.Context{
			AWS: &aws.AWSConfig{
				Profile:      stringPtr("default"),
				EndpointURL:  stringPtr("https://aws.endpoint"),
				S3Hostname:   stringPtr("s3.amazonaws.com"),
				MWAAEndpoint: stringPtr("https://mwaa.endpoint"),
			},
		}
	}
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return filepath.FromSlash("/mock/config/root"), nil
	}
	mockConfigHandler.GetContextFunc = func() string {
		return "test-context"
	}

	// Mock GetString method to return specific values for testing
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		switch key {
		case "aws.profile":
			return "default"
		case "aws.endpoint_url":
			return "https://aws.endpoint"
		case "aws.s3_hostname":
			return "s3.amazonaws.com"
		case "aws.mwaa_endpoint":
			return "https://mwaa.endpoint"
		case "aws.region":
			return "us-east-1"
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
	}

	// Create a mock Shell using its constructor
	mockShell := shell.NewMockShell()

	// Register the mocks in the DI injector
	mockInjector.Register("configHandler", mockConfigHandler)
	mockInjector.Register("shell", mockShell)

	return &AwsEnvMocks{
		Injector:      mockInjector,
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
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

	// t.Run("MissingConfiguration", func(t *testing.T) {
	// 	// Use setupSafeAwsEnvMocks to create mocks
	// 	mocks := setupSafeAwsEnvMocks()

	// 	// Override the GetConfigFunc to return nil for AWS configuration
	// 	mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
	// 		return &v1alpha1.Context{AWS: nil}
	// 	}

	// 	mockInjector := mocks.Injector

	// 	awsEnvPrinter := NewAwsEnvPrinter(mockInjector)
	// 	awsEnvPrinter.Initialize()

	// 	// Capture stdout
	// 	output := captureStdout(t, func() {
	// 		// When calling GetEnvVars
	// 		_, err := awsEnvPrinter.GetEnvVars()
	// 		if err != nil {
	// 			fmt.Println(err)
	// 		}
	// 	})

	// 	// Then the output should indicate the missing configuration
	// 	expectedOutput := "context configuration or AWS configuration is missing\n"
	// 	if output != expectedOutput {
	// 		t.Errorf("output = %v, want %v", output, expectedOutput)
	// 	}
	// })

	t.Run("NoAwsConfigFile", func(t *testing.T) {
		// Use setupSafeAwsEnvMocks to create mocks
		mocks := setupSafeAwsEnvMocks()

		// Override the GetConfigFunc to return a valid AWS configuration
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				AWS: &aws.AWSConfig{
					Profile:      stringPtr("default"),
					EndpointURL:  stringPtr("https://example.com"),
					S3Hostname:   stringPtr("s3.example.com"),
					MWAAEndpoint: stringPtr("mwaa.example.com"),
				},
			}
		}

		// Override the GetConfigRootFunc to return a valid path
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
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
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
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
			"AWS_CONFIG_FILE":       filepath.FromSlash("/mock/config/root/.aws/config"),
			"AWS_PROFILE":           "default",
			"AWS_ENDPOINT_URL":      "https://aws.endpoint",
			"AWS_ENDPOINT_URL_S3":   "s3.amazonaws.com",
			"AWS_ENDPOINT_URL_MWAA": "https://mwaa.endpoint",
			"AWS_REGION":            "us-east-1",
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, got %v", expectedEnvVars, capturedEnvVars)
		}
	})

	t.Run("ErrorRetrievingConfigRoot", func(t *testing.T) {
		// Use setupSafeAwsEnvMocks to create mocks
		mocks := setupSafeAwsEnvMocks()
		mockInjector := mocks.Injector

		// Override the GetConfigRoot function to simulate an error
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock config root error")
		}

		awsEnvPrinter := NewAwsEnvPrinter(mockInjector)
		awsEnvPrinter.Initialize()

		// Capture stdout
		output := captureStdout(t, func() {
			// When calling Print
			err := awsEnvPrinter.Print()
			if err != nil {
				fmt.Println(err)
			}
		})

		// Then the output should indicate the error
		if !strings.Contains(output, "mock config root error") {
			t.Errorf("output = %v, want it to contain %v", output, "mock config root error")
		}
	})
}
