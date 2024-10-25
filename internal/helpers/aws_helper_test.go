package helpers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

func TestAwsHelper_NewAwsHelper(t *testing.T) {
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
}

func TestAwsHelper_Initialize(t *testing.T) {
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
}

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
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return contextPath, nil
		}

		// Mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				AWS: &config.AWSConfig{
					AWSProfile:     ptrString("mock_profile"),
					AWSEndpointURL: ptrString("mock_aws_endpoint_url"),
					S3Hostname:     ptrString("mock_s3_hostname"),
					MWAAEndpoint:   ptrString("mock_mwaa_endpoint"),
				},
			}, nil
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

	t.Run("AWSConfigNil", func(t *testing.T) {
		// Given: a mock config handler that returns a context with a nil AWS config
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				AWS: nil, // Simulate a nil AWS configuration
			}, nil
		}

		// Mock context
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
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

		// When: GetEnvVars is called
		envVars, err := awsHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: an empty map should be returned
		expectedEnvVars := map[string]string{}
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

		// Mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				AWS: &config.AWSConfig{
					AWSProfile:     ptrString("default"),
					AWSEndpointURL: ptrString("http://aws.test:4566"),
					S3Hostname:     ptrString("http://s3.local.aws.test:4566"),
					MWAAEndpoint:   ptrString("http://mwaa.local.aws.test:4566"),
				},
			}, nil
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
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				AWS: &config.AWSConfig{},
			}, nil
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

		// When calling GetEnvVars
		expectedError := "error retrieving config root"

		_, err = awsHelper.GetEnvVars()
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
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
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				AWS: &config.AWSConfig{
					AWSProfile:     ptrString("default"),
					AWSEndpointURL: ptrString("http://aws.test:4566"),
					S3Hostname:     ptrString("http://s3.local.aws.test:4566"),
					MWAAEndpoint:   ptrString("http://mwaa.local.aws.test:4566"),
				},
			}, nil
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
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				AWS: &config.AWSConfig{
					AWSProfile:     ptrString("default"),
					AWSEndpointURL: nil,
					S3Hostname:     nil,
					MWAAEndpoint:   nil,
				},
			}, nil
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

	t.Run("ErrorRetrievingContextConfig", func(t *testing.T) {
		// Given: a mock config handler that returns an error
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, errors.New("mock error retrieving context config")
		}

		// Mock context
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
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

		// When: GetEnvVars is called
		_, err = awsHelper.GetEnvVars()

		// Then: an error should be returned
		expectedError := "error retrieving context config"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %q, got %v", expectedError, err)
		}
	})
}

func TestAwsHelper_PostEnvExec(t *testing.T) {
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
}

func TestAwsHelper_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler and context
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				AWS: &config.AWSConfig{
					Localstack: &config.LocalstackConfig{
						Create:   ptrBool(true),
						Services: []string{"s3", "dynamodb"},
					},
				},
			}, nil
		}
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

		// Then: the compose configuration should include the Localstack service
		if composeConfig == nil || len(composeConfig.Services) == 0 {
			t.Fatalf("expected non-nil composeConfig with services, got %v", composeConfig)
		}

		service := composeConfig.Services[0]
		if service.Name != "aws.test" {
			t.Errorf("expected service name 'aws.test', got %v", service.Name)
		}
		if service.Environment["SERVICES"] == nil || *service.Environment["SERVICES"] != "s3,dynamodb" {
			t.Errorf("expected SERVICES environment variable to be 's3,dynamodb', got %v", service.Environment["SERVICES"])
		}
	})

	t.Run("LocalstackConfigured", func(t *testing.T) {
		// Given: a mock config handler with Localstack configuration
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				AWS: &config.AWSConfig{
					Localstack: &config.LocalstackConfig{
						Create:   ptrBool(true),
						Services: []string{"s3", "dynamodb"},
					},
				},
			}, nil
		}

		// Mock context
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

		// Then: the compose configuration should include the Localstack service
		if composeConfig == nil || len(composeConfig.Services) == 0 {
			t.Fatalf("expected non-nil composeConfig with services, got %v", composeConfig)
		}

		service := composeConfig.Services[0]
		if service.Name != "aws.test" {
			t.Errorf("expected service name 'aws.test', got %v", service.Name)
		}
		if service.Environment["SERVICES"] == nil || *service.Environment["SERVICES"] != "s3,dynamodb" {
			t.Errorf("expected SERVICES environment variable to be 's3,dynamodb', got %v", service.Environment["SERVICES"])
		}
	})

	t.Run("LocalstackWithAuthToken", func(t *testing.T) {
		// Set the LOCALSTACK_AUTH_TOKEN environment variable
		os.Setenv("LOCALSTACK_AUTH_TOKEN", "mock_token")
		defer os.Unsetenv("LOCALSTACK_AUTH_TOKEN")

		// Given: a mock config handler with Localstack configuration
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				AWS: &config.AWSConfig{
					Localstack: &config.LocalstackConfig{
						Create:   ptrBool(true),
						Services: []string{"s3"},
					},
				},
			}, nil
		}

		// Mock context
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

		// Then: the compose configuration should include the Localstack service with auth token
		if composeConfig == nil || len(composeConfig.Services) == 0 {
			t.Fatalf("expected non-nil composeConfig with services, got %v", composeConfig)
		}

		service := composeConfig.Services[0]
		if len(service.Secrets) == 0 || service.Secrets[0].Source != "LOCALSTACK_AUTH_TOKEN" {
			t.Errorf("expected service to have LOCALSTACK_AUTH_TOKEN secret, got %v", service.Secrets)
		}
	})

	t.Run("ErrorRetrievingContextConfig", func(t *testing.T) {
		// Given: a mock config handler that returns an error
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, fmt.Errorf("mock error retrieving context config")
		}

		// Mock context
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
		_, err = awsHelper.GetComposeConfig()

		// Then: an error should be returned
		expectedError := "error retrieving context config"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %q, got %v", expectedError, err)
		}
	})

	t.Run("AWSConfigNil", func(t *testing.T) {
		// Given: a mock config handler that returns a context with a nil AWS config
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				AWS: nil, // Simulate a nil AWS configuration
			}, nil
		}

		// Mock context
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

		// Then: nil should be returned
		if composeConfig != nil {
			t.Fatalf("expected nil composeConfig, got %v", composeConfig)
		}
	})

	t.Run("LocalstackConfigNil", func(t *testing.T) {
		// Given: a mock config handler that returns a context with a nil Localstack config
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				AWS: &config.AWSConfig{
					Localstack: nil, // Simulate a nil Localstack configuration
				},
			}, nil
		}

		// Mock context
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

		// Then: nil should be returned
		if composeConfig != nil {
			t.Fatalf("expected nil composeConfig, got %v", composeConfig)
		}
	})
}

func TestAwsHelper_WriteConfig(t *testing.T) {
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
}
