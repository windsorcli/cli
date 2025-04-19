package env

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/aws"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

// AwsEnvMocks holds all mock objects used in AWS environment tests
type AwsEnvMocks struct {
	Injector      di.Injector
	ConfigHandler *config.MockConfigHandler
	Shell         *shell.MockShell
}

// setupSafeAwsEnvMocks creates and configures mock objects for AWS environment tests.
// It accepts an optional injector parameter and returns initialized AwsEnvMocks.
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
				AWSProfile:     stringPtr("default"),
				AWSEndpointURL: stringPtr("https://aws.endpoint"),
				S3Hostname:     stringPtr("s3.amazonaws.com"),
				MWAAEndpoint:   stringPtr("https://mwaa.endpoint"),
			},
		}
	}
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return filepath.FromSlash("/mock/config/root"), nil
	}
	mockConfigHandler.GetContextFunc = func() string {
		return "test-context"
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

// =============================================================================
// Test Public Methods
// =============================================================================

// TestAwsEnv_GetEnvVars tests the GetEnvVars method of the AwsEnvPrinter
func TestAwsEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new AwsEnvPrinter with mock AWS config file
		mocks := setupSafeAwsEnvMocks()

		// And AWS config file exists
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/.aws/config") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		awsEnvPrinter := NewAwsEnvPrinter(mocks.Injector)
		awsEnvPrinter.Initialize()

		// When calling GetEnvVars
		envVars, err := awsEnvPrinter.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And AWS_CONFIG_FILE should be set correctly
		expectedConfigFile := filepath.FromSlash("/mock/config/root/.aws/config")
		if envVars["AWS_CONFIG_FILE"] != expectedConfigFile {
			t.Errorf("AWS_CONFIG_FILE = %v, want %v", envVars["AWS_CONFIG_FILE"], expectedConfigFile)
		}
	})

	t.Run("MissingConfiguration", func(t *testing.T) {
		// Given a new AwsEnvPrinter with no AWS configuration
		mocks := setupSafeAwsEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{AWS: nil}
		}

		awsEnvPrinter := NewAwsEnvPrinter(mocks.Injector)
		awsEnvPrinter.Initialize()

		// When calling GetEnvVars
		output := captureStdout(t, func() {
			_, err := awsEnvPrinter.GetEnvVars()
			if err != nil {
				fmt.Println(err)
			}
		})

		// Then appropriate error message should be output
		expectedOutput := "context configuration or AWS configuration is missing\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("NoAwsConfigFile", func(t *testing.T) {
		// Given a new AwsEnvPrinter with valid AWS config but no config file
		mocks := setupSafeAwsEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				AWS: &aws.AWSConfig{
					AWSProfile:     stringPtr("default"),
					AWSEndpointURL: stringPtr("https://example.com"),
					S3Hostname:     stringPtr("s3.example.com"),
					MWAAEndpoint:   stringPtr("mwaa.example.com"),
				},
			}
		}
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/non/existent/path", nil
		}

		awsEnvPrinter := NewAwsEnvPrinter(mocks.Injector)
		awsEnvPrinter.Initialize()

		// When calling GetEnvVars
		output := captureStdout(t, func() {
			_, err := awsEnvPrinter.GetEnvVars()
			if err != nil {
				fmt.Println(err)
			}
		})

		// Then no output should be produced
		if output != "" {
			t.Errorf("output = %v, want empty output", output)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		// Given a new AwsEnvPrinter with failing GetConfigRoot
		mocks := setupSafeAwsEnvMocks()
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}

		awsEnvPrinter := NewAwsEnvPrinter(mocks.Injector)
		awsEnvPrinter.Initialize()

		// When calling GetEnvVars
		output := captureStdout(t, func() {
			_, err := awsEnvPrinter.GetEnvVars()
			if err != nil {
				fmt.Println(err)
			}
		})

		// Then appropriate error message should be output
		expectedOutput := "error retrieving configuration root directory: mock context error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})
}

// TestAwsEnv_Print tests the Print method of the AwsEnvPrinter
func TestAwsEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new AwsEnvPrinter with mock AWS config file
		mocks := setupSafeAwsEnvMocks()
		awsEnvPrinter := NewAwsEnvPrinter(mocks.Injector)
		awsEnvPrinter.Initialize()

		// And AWS config file exists
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/.aws/config") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// And PrintEnvVarsFunc is mocked
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) {
			capturedEnvVars = envVars
		}

		// When calling Print
		err := awsEnvPrinter.Print()

		// Then no error should be returned
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// And environment variables should be set correctly
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
		// Given a new AwsEnvPrinter with no AWS configuration
		mocks := setupSafeAwsEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				AWS: nil,
			}
		}

		awsEnvPrinter := NewAwsEnvPrinter(mocks.Injector)
		awsEnvPrinter.Initialize()

		// When calling Print
		err := awsEnvPrinter.Print()

		// Then appropriate error should be returned
		if err == nil {
			t.Error("expected error, got nil")
		}
		expectedError := "error getting environment variables: context configuration or AWS configuration is missing"
		if err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err.Error(), expectedError)
		}
	})
}
