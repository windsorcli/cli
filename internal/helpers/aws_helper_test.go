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
	"github.com/windsor-hotel/cli/internal/di"
)

func TestAwsHelper(t *testing.T) {
	t.Run("Initialize", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given: a mock config handler and context
			mockConfigHandler := config.NewMockConfigHandler()
			mockContext := context.NewMockContext()

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Create an instance of AwsHelper
			awsHelper, err := NewAwsHelper(diContainer)
			if err != nil {
				t.Fatalf("NewAwsHelper() error = %v", err)
			}

			// When: Initialize is called
			err = awsHelper.Initialize()
			if err != nil {
				t.Fatalf("Initialize() error = %v", err)
			}

			// Then: no error should be returned
			if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
		})
	})

	t.Run("GetEnvVars", func(t *testing.T) {
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
			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return contextPath, nil
			}
			mockContext.GetContextFunc = func() (string, error) {
				return "local", nil
			}

			// Mock config handler
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
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
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Create AwsHelper
			awsHelper, err := NewAwsHelper(diContainer)
			if err != nil {
				t.Fatalf("NewAwsHelper() error = %v", err)
			}

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
			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return contextPath, nil
			}
			mockContext.GetContextFunc = func() (string, error) {
				return "local", nil
			}

			// Mock config handler
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
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
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Create AwsHelper
			awsHelper, err := NewAwsHelper(diContainer)
			if err != nil {
				t.Fatalf("NewAwsHelper() error = %v", err)
			}

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
			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return "", errors.New("error retrieving config root")
			}

			// Mock config handler
			mockConfigHandler := config.NewMockConfigHandler()

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Create AwsHelper
			awsHelper, err := NewAwsHelper(diContainer)
			if err != nil {
				t.Fatalf("NewAwsHelper() error = %v", err)
			}

			// When calling GetEnvVars
			expectedError := "error retrieving config root"

			_, err = awsHelper.GetEnvVars()
			if err == nil || !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
			}
		})

		t.Run("ErrorRetrievingCurrentContext", func(t *testing.T) {
			// Given: a mock context that returns an error for GetContext
			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return "/mock/config/root", nil
			}
			mockContext.GetContextFunc = func() (string, error) {
				return "", errors.New("error retrieving current context")
			}

			// Mock config handler
			mockConfigHandler := config.NewMockConfigHandler()

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Create AwsHelper
			awsHelper, err := NewAwsHelper(diContainer)
			if err != nil {
				t.Fatalf("NewAwsHelper() error = %v", err)
			}

			// When: GetEnvVars is called
			_, err = awsHelper.GetEnvVars()

			// Then: an error should be returned
			expectedError := "error retrieving current context"
			if err == nil || !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %q, got %v", expectedError, err)
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
			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return contextPath, nil
			}
			mockContext.GetContextFunc = func() (string, error) {
				return "local", nil
			}

			// Mock config handler
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
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
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Create AwsHelper
			awsHelper, err := NewAwsHelper(diContainer)
			if err != nil {
				t.Fatalf("NewAwsHelper() error = %v", err)
			}

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
			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return contextPath, nil
			}
			mockContext.GetContextFunc = func() (string, error) {
				return "local", nil
			}

			// Mock config handler
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
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
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Create AwsHelper
			awsHelper, err := NewAwsHelper(diContainer)
			if err != nil {
				t.Fatalf("NewAwsHelper() error = %v", err)
			}

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
			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return contextPath, nil
			}
			mockContext.GetContextFunc = func() (string, error) {
				return "local", nil
			}

			// Mock config handler
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
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
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Create AwsHelper
			awsHelper, err := NewAwsHelper(diContainer)
			if err != nil {
				t.Fatalf("NewAwsHelper() error = %v", err)
			}

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
			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return contextPath, nil
			}
			mockContext.GetContextFunc = func() (string, error) {
				return "remote", nil
			}

			// Mock config handler
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
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
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Create AwsHelper
			awsHelper, err := NewAwsHelper(diContainer)
			if err != nil {
				t.Fatalf("NewAwsHelper() error = %v", err)
			}

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
	})

	t.Run("PostEnvExec", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a AwsHelper instance
			mockConfigHandler := config.NewMockConfigHandler()
			mockContext := context.NewMockContext()

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			awsHelper, err := NewAwsHelper(diContainer)
			if err != nil {
				t.Fatalf("NewAwsHelper() error = %v", err)
			}

			// When calling PostEnvExec
			err = awsHelper.PostEnvExec()

			// Then no error should be returned
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	})

	t.Run("NewAwsHelper", func(t *testing.T) {
		t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
			// Create DI container without registering cliConfigHandler
			diContainer := di.NewContainer()

			// Attempt to create AwsHelper
			_, err := NewAwsHelper(diContainer)
			if err == nil || !strings.Contains(err.Error(), "error resolving cliConfigHandler") {
				t.Fatalf("expected error resolving cliConfigHandler, got %v", err)
			}
		})

		t.Run("ErrorResolvingContext", func(t *testing.T) {
			// Create DI container and register only cliConfigHandler
			diContainer := di.NewContainer()
			mockConfigHandler := config.NewMockConfigHandler()
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// Attempt to create AwsHelper
			_, err := NewAwsHelper(diContainer)
			if err == nil || !strings.Contains(err.Error(), "error resolving context") {
				t.Fatalf("expected error resolving context, got %v", err)
			}
		})
	})

	t.Run("GetComposeConfig", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given: a mock config handler and context
			mockConfigHandler := config.NewMockConfigHandler()
			mockContext := context.NewMockContext()

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Create an instance of AwsHelper
			awsHelper, err := NewAwsHelper(diContainer)
			if err != nil {
				t.Fatalf("NewAwsHelper() error = %v", err)
			}

			// When: GetComposeConfig is called
			composeConfig, err := awsHelper.GetComposeConfig()
			if err != nil {
				t.Fatalf("GetComposeConfig() error = %v", err)
			}

			// Then: the result should be nil as per the stub implementation
			if composeConfig != nil {
				t.Errorf("expected nil, got %v", composeConfig)
			}
		})
	})

	t.Run("WriteConfig", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given: a mock config handler and context
			mockConfigHandler := config.NewMockConfigHandler()
			mockContext := context.NewMockContext()
			mockContext.GetContextFunc = func() (string, error) {
				return "test-context", nil
			}
			mockContext.GetConfigRootFunc = func() (string, error) {
				return "/path/to/config", nil
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Create an instance of AwsHelper
			awsHelper, err := NewAwsHelper(diContainer)
			if err != nil {
				t.Fatalf("NewAwsHelper() error = %v", err)
			}

			// When: WriteConfig is called
			err = awsHelper.WriteConfig()
			if err != nil {
				t.Fatalf("WriteConfig() error = %v", err)
			}

			// Then: no error should be returned
			if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
		})
	})
}
