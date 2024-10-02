package helpers

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/shell"
)

func TestAwsHelper_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a valid context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		awsConfigPath := filepath.Join(contextPath, ".aws", "config")

		// Ensure the AWS config file exists
		err := mkdirAll(filepath.Dir(awsConfigPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create AWS config directory: %v", err)
		}
		_, err = os.Create(awsConfigPath)
		if err != nil {
			t.Fatalf("Failed to create AWS config file: %v", err)
		}
		defer os.RemoveAll(filepath.Dir(awsConfigPath)) // Clean up

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
			GetContextFunc: func() (string, error) {
				return "local", nil
			},
		}

		// Mock config handler
		mockConfigHandler := &config.MockConfigHandler{
			GetConfigValueFunc: func(key string) (string, error) {
				switch key {
				case "contexts.local.aws.aws_profile":
					return "mock_profile", nil
				case "contexts.local.aws.aws_endpoint_url":
					return "mock_aws_endpoint_url", nil
				case "contexts.local.aws.s3_hostname":
					return "mock_s3_hostname", nil
				case "contexts.local.aws.mwaa_endpoint":
					return "mock_mwaa_endpoint", nil
				default:
					return "", nil
				}
			},
		}

		// Create AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, nil, mockContext)

		// When: GetEnvVars is called
		envVars, err := awsHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly
		expectedEnvVars := map[string]string{
			"AWS_CONFIG_FILE":  awsConfigPath,
			"AWS_PROFILE":      "mock_profile",
			"AWS_ENDPOINT_URL": "mock_aws_endpoint_url",
			"S3_HOSTNAME":      "mock_s3_hostname",
			"MWAA_ENDPOINT":    "mock_mwaa_endpoint",
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("FileNotExist", func(t *testing.T) {
		// Given: a non-existent context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "non-existent-context")
		awsConfigPath := ""

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
			GetContextFunc: func() (string, error) {
				return "local", nil
			},
		}

		// Mock config handler
		mockConfigHandler := &config.MockConfigHandler{
			GetConfigValueFunc: func(key string) (string, error) {
				switch key {
				case "contexts.local.aws.aws_profile":
					return "default", nil
				case "contexts.local.aws.aws_endpoint_url":
					return "", nil
				case "contexts.local.aws.s3_hostname":
					return "", nil
				case "contexts.local.aws.mwaa_endpoint":
					return "", nil
				default:
					return "", nil
				}
			},
		}

		// Create AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, nil, mockContext)

		// When: GetEnvVars is called
		envVars, err := awsHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly with an empty AWS_CONFIG_FILE
		expectedEnvVars := map[string]string{
			"AWS_CONFIG_FILE":  awsConfigPath,
			"AWS_PROFILE":      "default",
			"AWS_ENDPOINT_URL": "http://aws.test:4566",
			"S3_HOSTNAME":      "http://s3.local.aws.test:4566",
			"MWAA_ENDPOINT":    "http://mwaa.local.aws.test:4566",
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("ErrorRetrievingConfigRoot", func(t *testing.T) {
		// Given a mock context that returns an error for config root
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return "", errors.New("error retrieving config root")
			},
		}

		// Mock config handler
		mockConfigHandler := &config.MockConfigHandler{}

		// Create AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, nil, mockContext)

		// When calling GetEnvVars
		expectedError := "error retrieving config root"

		_, err := awsHelper.GetEnvVars()
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})

	t.Run("ErrorRetrievingCurrentContext", func(t *testing.T) {
		// Given a mock context that returns an error for current context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return "/mock/config/root", nil
			},
			GetContextFunc: func() (string, error) {
				return "", errors.New("error retrieving current context")
			},
		}

		// Mock config handler
		mockConfigHandler := &config.MockConfigHandler{}

		// Create AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, nil, mockContext)

		// When calling GetEnvVars
		expectedError := "error retrieving current context"

		_, err := awsHelper.GetEnvVars()
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})

	t.Run("ErrorRetrievingAwsProfile", func(t *testing.T) {
		// Given: a valid context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		awsConfigPath := filepath.Join(contextPath, ".aws", "config")

		// Ensure the AWS config file exists
		err := mkdirAll(filepath.Dir(awsConfigPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create AWS config directory: %v", err)
		}
		_, err = os.Create(awsConfigPath)
		if err != nil {
			t.Fatalf("Failed to create AWS config file: %v", err)
		}
		defer os.RemoveAll(filepath.Dir(awsConfigPath)) // Clean up

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
			GetContextFunc: func() (string, error) {
				return "local", nil
			},
		}

		// Mock config handler
		mockConfigHandler := &config.MockConfigHandler{
			GetConfigValueFunc: func(key string) (string, error) {
				switch key {
				case "contexts.local.aws.aws_profile":
					return "", errors.New("mock error retrieving aws profile")
				case "contexts.local.aws.aws_endpoint_url":
					return "http://aws.test:4566", nil
				case "contexts.local.aws.s3_hostname":
					return "http://s3.local.aws.test:4566", nil
				case "contexts.local.aws.mwaa_endpoint":
					return "http://mwaa.local.aws.test:4566", nil
				default:
					return "", nil
				}
			},
		}

		// Create AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, nil, mockContext)

		// When: GetEnvVars is called
		envVars, err := awsHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly with default local values
		expectedEnvVars := map[string]string{
			"AWS_CONFIG_FILE":  awsConfigPath,
			"AWS_PROFILE":      "default",
			"AWS_ENDPOINT_URL": "http://aws.test:4566",
			"S3_HOSTNAME":      "http://s3.local.aws.test:4566",
			"MWAA_ENDPOINT":    "http://mwaa.local.aws.test:4566",
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("ErrorRetrievingAwsEndpointURL", func(t *testing.T) {
		// Given: a valid context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		awsConfigPath := filepath.Join(contextPath, ".aws", "config")

		// Ensure the AWS config file exists
		err := mkdirAll(filepath.Dir(awsConfigPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create AWS config directory: %v", err)
		}
		_, err = os.Create(awsConfigPath)
		if err != nil {
			t.Fatalf("Failed to create AWS config file: %v", err)
		}
		defer os.RemoveAll(filepath.Dir(awsConfigPath)) // Clean up

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
			GetContextFunc: func() (string, error) {
				return "local", nil
			},
		}

		// Mock config handler
		mockConfigHandler := &config.MockConfigHandler{
			GetConfigValueFunc: func(key string) (string, error) {
				switch key {
				case "contexts.local.aws.aws_profile":
					return "default", nil
				case "contexts.local.aws.aws_endpoint_url":
					return "", errors.New("mock error retrieving aws endpoint url")
				case "contexts.local.aws.s3_hostname":
					return "http://s3.local.aws.test:4566", nil
				case "contexts.local.aws.mwaa_endpoint":
					return "http://mwaa.local.aws.test:4566", nil
				default:
					return "", nil
				}
			},
		}

		// Create AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, nil, mockContext)

		// When: GetEnvVars is called
		envVars, err := awsHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly with default local values
		expectedEnvVars := map[string]string{
			"AWS_CONFIG_FILE":  awsConfigPath,
			"AWS_PROFILE":      "default",
			"AWS_ENDPOINT_URL": "http://aws.test:4566",
			"S3_HOSTNAME":      "http://s3.local.aws.test:4566",
			"MWAA_ENDPOINT":    "http://mwaa.local.aws.test:4566",
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("LocalContextWithDefaults", func(t *testing.T) {
		// Given: a local context with no specific endpoint URLs set
		contextPath := "/mock/config/root"
		awsConfigPath := ""

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
			GetContextFunc: func() (string, error) {
				return "local", nil
			},
		}

		// Mock config handler
		mockConfigHandler := &config.MockConfigHandler{
			GetConfigValueFunc: func(key string) (string, error) {
				switch key {
				case "contexts.local.aws.aws_profile":
					return "default", nil
				case "contexts.local.aws.aws_endpoint_url":
					return "", nil
				case "contexts.local.aws.s3_hostname":
					return "", nil
				case "contexts.local.aws.mwaa_endpoint":
					return "", nil
				default:
					return "", nil
				}
			},
		}

		// Create AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, nil, mockContext)

		// When: GetEnvVars is called
		envVars, err := awsHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly with default local values
		expectedEnvVars := map[string]string{
			"AWS_CONFIG_FILE":  awsConfigPath,
			"AWS_PROFILE":      "default",
			"AWS_ENDPOINT_URL": "http://aws.test:4566",
			"S3_HOSTNAME":      "http://s3.local.aws.test:4566",
			"MWAA_ENDPOINT":    "http://mwaa.local.aws.test:4566",
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("NonLocalContext", func(t *testing.T) {
		// Given: a non-local context with no specific endpoint URLs set
		contextPath := "/mock/config/root"
		awsConfigPath := ""

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
			GetContextFunc: func() (string, error) {
				return "remote", nil
			},
		}

		// Mock config handler
		mockConfigHandler := &config.MockConfigHandler{
			GetConfigValueFunc: func(key string) (string, error) {
				switch key {
				case "contexts.remote.aws.aws_profile":
					return "default", nil
				case "contexts.remote.aws.aws_endpoint_url":
					return "", nil
				case "contexts.remote.aws.s3_hostname":
					return "", nil
				case "contexts.remote.aws.mwaa_endpoint":
					return "", nil
				default:
					return "", nil
				}
			},
		}

		// Create AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, nil, mockContext)

		// When: GetEnvVars is called
		envVars, err := awsHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly with empty values for S3_HOSTNAME and MWAA_ENDPOINT
		expectedEnvVars := map[string]string{
			"AWS_CONFIG_FILE":  awsConfigPath,
			"AWS_PROFILE":      "default",
			"AWS_ENDPOINT_URL": "",
			"S3_HOSTNAME":      "",
			"MWAA_ENDPOINT":    "",
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})
}

func TestAwsHelper_PostEnvExec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a AwsHelper instance
		mockConfigHandler := &config.MockConfigHandler{}
		mockContext := &context.MockContext{}

		awsHelper := NewAwsHelper(mockConfigHandler, nil, mockContext)

		// When calling PostEnvExec
		err := awsHelper.PostEnvExec()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestAwsHelper_SetConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, shell, and context
		mockConfigHandler := config.NewMockConfigHandler(
			func(path string) error { return nil },
			func(key string) (string, error) { return "value", nil },
			func(key, value string) error { return nil },
			func(path string) error { return nil },
			func(key string) (map[string]interface{}, error) { return nil, nil },
			func(key string) ([]string, error) { return nil, nil },
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
			GetConfigRootFunc: func() (string, error) {
				return "/path/to/config", nil
			},
		}

		// Create an instance of AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, mockShell, mockContext)

		// When: SetConfig is called with valid values
		err = awsHelper.SetConfig("http://example.com", "test-profile")
		if err != nil {
			t.Fatalf("SetConfig() error = %v", err)
		}

		// Then: no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("EmptyValues", func(t *testing.T) {
		// Given: a mock config handler, shell, and context
		mockConfigHandler := config.NewMockConfigHandler(
			func(path string) error { return nil },
			func(key string) (string, error) { return "value", nil },
			func(key, value string) error { return nil },
			func(path string) error { return nil },
			func(key string) (map[string]interface{}, error) { return nil, nil },
			func(key string) ([]string, error) { return nil, nil },
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
			GetConfigRootFunc: func() (string, error) {
				return "/path/to/config", nil
			},
		}

		// Create an instance of AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, mockShell, mockContext)

		// When: SetConfig is called with empty values
		err = awsHelper.SetConfig("", "")
		if err != nil {
			t.Fatalf("SetConfig() error = %v", err)
		}

		// Then: no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("OnlyAwsProfile", func(t *testing.T) {
		// Given: a mock config handler, shell, and context
		mockConfigHandler := config.NewMockConfigHandler(
			func(path string) error { return nil },
			func(key string) (string, error) { return "value", nil },
			func(key, value string) error { return nil },
			func(path string) error { return nil },
			func(key string) (map[string]interface{}, error) { return nil, nil },
			func(key string) ([]string, error) { return nil, nil },
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
			GetConfigRootFunc: func() (string, error) {
				return "/path/to/config", nil
			},
		}

		// Create an instance of AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, mockShell, mockContext)

		// When: SetConfig is called with only aws_profile
		err = awsHelper.SetConfig("", "test-profile")
		if err != nil {
			t.Fatalf("SetConfig() error = %v", err)
		}

		// Then: no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorSettingAwsProfile", func(t *testing.T) {
		// Given: a mock config handler, shell, and context
		mockConfigHandler := config.NewMockConfigHandler(
			func(path string) error { return nil },
			func(key string) (string, error) { return "value", nil },
			func(key, value string) error {
				if key == "contexts.test-context.aws.aws_profile" {
					return errors.New("error setting aws_profile")
				}
				return nil
			},
			func(path string) error { return nil },
			func(key string) (map[string]interface{}, error) { return nil, nil },
			func(key string) ([]string, error) { return nil, nil },
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
			GetConfigRootFunc: func() (string, error) {
				return "/path/to/config", nil
			},
		}

		// Create an instance of AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, mockShell, mockContext)

		// When: SetConfig is called and setting aws_profile fails
		err = awsHelper.SetConfig("aws_profile", "test-profile")
		if err == nil || !strings.Contains(err.Error(), "error setting aws_profile") {
			t.Fatalf("expected error setting aws_profile, got %v", err)
		}
	})

	t.Run("ErrorSettingConfigValue", func(t *testing.T) {
		// Given: a mock config handler, shell, and context
		mockConfigHandler := config.NewMockConfigHandler(
			func(path string) error { return nil },
			func(key string) (string, error) { return "value", nil },
			func(key, value string) error {
				if key == "contexts.test-context.aws.aws_endpoint_url" {
					return errors.New("error setting aws_endpoint_url")
				}
				return nil
			},
			func(path string) error { return nil },
			func(key string) (map[string]interface{}, error) { return nil, nil },
			func(key string) ([]string, error) { return nil, nil },
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
			GetConfigRootFunc: func() (string, error) {
				return "/path/to/config", nil
			},
		}

		// Create an instance of AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, mockShell, mockContext)

		// When: SetConfig is called and setting aws_endpoint_url fails
		err = awsHelper.SetConfig("aws_endpoint_url", "http://example.com")
		if err == nil || !strings.Contains(err.Error(), "error setting aws_endpoint_url") {
			t.Fatalf("expected error setting aws_endpoint_url, got %v", err)
		}
	})

	t.Run("ErrorSavingConfig", func(t *testing.T) {
		// Given: a mock config handler, shell, and context
		mockConfigHandler := config.NewMockConfigHandler(
			func(path string) error { return nil },
			func(key string) (string, error) { return "value", nil },
			func(key, value string) error { return nil },
			func(path string) error { return errors.New("error saving config") },
			func(key string) (map[string]interface{}, error) { return nil, nil },
			func(key string) ([]string, error) { return nil, nil },
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
			GetConfigRootFunc: func() (string, error) {
				return "/path/to/config", nil
			},
		}

		// Create an instance of AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, mockShell, mockContext)

		// When: SetConfig is called and saving config fails
		err = awsHelper.SetConfig("http://example.com", "test-profile")
		if err == nil || !strings.Contains(err.Error(), "error saving config") {
			t.Fatalf("expected error saving config, got %v", err)
		}
	})

	t.Run("ErrorRetrievingCurrentContext", func(t *testing.T) {
		// Given: a mock config handler, shell, and context
		mockConfigHandler := config.NewMockConfigHandler(
			func(path string) error { return nil },
			func(key string) (string, error) { return "value", nil },
			func(key, value string) error { return nil },
			func(path string) error { return nil },
			func(key string) (map[string]interface{}, error) { return nil, nil },
			func(key string) ([]string, error) { return nil, nil },
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "", errors.New("error retrieving current context")
			},
			GetConfigRootFunc: func() (string, error) {
				return "/path/to/config", nil
			},
		}

		// Create an instance of AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, mockShell, mockContext)

		// When: SetConfig is called
		err = awsHelper.SetConfig("http://example.com", "test-profile")
		if err == nil || !strings.Contains(err.Error(), "error retrieving current context") {
			t.Fatalf("expected error retrieving current context, got %v", err)
		}
	})

}
