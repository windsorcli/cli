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
)

func TestAwsHelper_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a valid context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		awsConfigPath := filepath.Join(contextPath, ".aws", "config")

		// Ensure the AWS config file exists
		err := os.MkdirAll(filepath.Dir(awsConfigPath), 0755)
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
				case "aws_profile":
					return "mock_profile", nil
				case "aws_endpoint_url":
					return "", nil
				case "s3_hostname":
					return "mock_s3_hostname", nil
				case "mwaa_endpoint":
					return "mock_mwaa_endpoint", nil
				default:
					return "", nil
				}
			},
		}

		// Set environment variable
		os.Setenv("AWS_ENDPOINT_URL", "mock_aws_endpoint_url")
		defer os.Unsetenv("AWS_ENDPOINT_URL") // Clean up

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
				if key == "aws_profile" {
					return "default", nil
				}
				return "", nil
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
			"AWS_ENDPOINT_URL": "",
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
		err := os.MkdirAll(filepath.Dir(awsConfigPath), 0755)
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
				if key == "aws_profile" {
					return "", errors.New("error retrieving aws_profile")
				}
				return "", nil
			},
		}

		// Create AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, nil, mockContext)

		// When: GetEnvVars is called
		envVars, err := awsHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly with default AWS_PROFILE
		expectedEnvVars := map[string]string{
			"AWS_CONFIG_FILE":  awsConfigPath,
			"AWS_PROFILE":      "default",
			"AWS_ENDPOINT_URL": "",
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
		err := os.MkdirAll(filepath.Dir(awsConfigPath), 0755)
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
				return "remote", nil
			},
		}

		// Mock config handler
		mockConfigHandler := &config.MockConfigHandler{
			GetConfigValueFunc: func(key string) (string, error) {
				if key == "aws_profile" {
					return "", errors.New("error retrieving aws_profile")
				}
				if key == "aws_endpoint_url" {
					return "", errors.New("error retrieving aws_endpoint_url")
				}
				return "", nil
			},
		}

		// Create AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, nil, mockContext)

		// When: GetEnvVars is called
		envVars, err := awsHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly with empty AWS_ENDPOINT_URL
		expectedEnvVars := map[string]string{
			"AWS_CONFIG_FILE":  awsConfigPath,
			"AWS_PROFILE":      "default",
			"AWS_ENDPOINT_URL": "",
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
		setConfigValueCallCount := 0
		saveConfigCallCount := 0

		// Mock config handler
		mockConfigHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				setConfigValueCallCount++
				return nil
			},
			SaveConfigFunc: func(path string) error {
				saveConfigCallCount++
				return nil
			},
		}

		// Create AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, nil, &context.MockContext{})

		// When: SetConfig is called with non-empty values
		err := awsHelper.SetConfig("new_endpoint_url", "new_profile")
		if err != nil {
			t.Fatalf("SetConfig() error = %v", err)
		}

		// Then: the config values should be set correctly
		if setConfigValueCallCount != 2 {
			t.Errorf("expected 2 calls to SetConfigValue, got %d", setConfigValueCallCount)
		}
		if saveConfigCallCount != 1 {
			t.Errorf("expected 1 call to SaveConfig, got %d", saveConfigCallCount)
		}
	})

	t.Run("EmptyValues", func(t *testing.T) {
		setConfigValueCallCount := 0
		saveConfigCallCount := 0

		// Mock config handler
		mockConfigHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				setConfigValueCallCount++
				return nil
			},
			SaveConfigFunc: func(path string) error {
				saveConfigCallCount++
				return nil
			},
		}

		// Create AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, nil, &context.MockContext{})

		// When: SetConfig is called with empty values
		err := awsHelper.SetConfig("", "")
		if err != nil {
			t.Fatalf("SetConfig() error = %v", err)
		}

		// Then: the config values should not be set
		if setConfigValueCallCount != 0 {
			t.Errorf("expected 0 calls to SetConfigValue, got %d", setConfigValueCallCount)
		}
		if saveConfigCallCount != 1 {
			t.Errorf("expected 1 call to SaveConfig, got %d", saveConfigCallCount)
		}
	})

	t.Run("OnlyAwsProfile", func(t *testing.T) {
		setConfigValueCallCount := 0
		saveConfigCallCount := 0

		// Mock config handler
		mockConfigHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				setConfigValueCallCount++
				return nil
			},
			SaveConfigFunc: func(path string) error {
				saveConfigCallCount++
				return nil
			},
		}

		// Create AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, nil, &context.MockContext{})

		// When: SetConfig is called with only awsProfile non-empty
		err := awsHelper.SetConfig("", "new_profile")
		if err != nil {
			t.Fatalf("SetConfig() error = %v", err)
		}

		// Then: only awsProfile should be set
		if setConfigValueCallCount != 1 {
			t.Errorf("expected 1 call to SetConfigValue, got %d", setConfigValueCallCount)
		}
		if saveConfigCallCount != 1 {
			t.Errorf("expected 1 call to SaveConfig, got %d", saveConfigCallCount)
		}
	})

	t.Run("ErrorSettingAwsProfile", func(t *testing.T) {
		// Mock config handler
		mockConfigHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				if key == "aws_profile" {
					return errors.New("error setting aws_profile")
				}
				return nil
			},
			SaveConfigFunc: func(path string) error {
				return nil
			},
		}

		// Create AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, nil, &context.MockContext{})

		// When: SetConfig is called with non-empty awsProfile
		err := awsHelper.SetConfig("", "new_profile")
		if err == nil || err.Error() != "error setting aws_profile: error setting aws_profile" {
			t.Fatalf("expected error setting aws_profile, got %v", err)
		}
	})

	t.Run("ErrorSettingConfigValue", func(t *testing.T) {
		// Mock config handler
		mockConfigHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				return errors.New("error setting config value")
			},
			SaveConfigFunc: func(path string) error {
				return nil
			},
		}

		// Create AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, nil, &context.MockContext{})

		// When: SetConfig is called with non-empty values
		err := awsHelper.SetConfig("new_endpoint_url", "new_profile")
		if err == nil || err.Error() != "error setting aws_endpoint_url: error setting config value" {
			t.Fatalf("expected error setting aws_endpoint_url, got %v", err)
		}
	})

	t.Run("ErrorSavingConfig", func(t *testing.T) {
		// Mock config handler
		mockConfigHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				return nil
			},
			SaveConfigFunc: func(path string) error {
				return errors.New("error saving config")
			},
		}

		// Create AwsHelper
		awsHelper := NewAwsHelper(mockConfigHandler, nil, &context.MockContext{})

		// When: SetConfig is called with non-empty values
		err := awsHelper.SetConfig("new_endpoint_url", "new_profile")
		if err == nil || err.Error() != "error saving config: error saving config" {
			t.Fatalf("expected error saving config, got %v", err)
		}
	})
}
