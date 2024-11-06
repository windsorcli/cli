package helpers

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

type AwsHelperMocks struct {
	Container        di.ContainerInterface
	CLIConfigHandler *config.MockConfigHandler
	Shell            *shell.MockShell
	Context          *context.MockContext
}

func createAwsHelperMocks(mockContainer ...di.ContainerInterface) *AwsHelperMocks {
	var container di.ContainerInterface
	if len(mockContainer) > 0 {
		container = mockContainer[0]
	} else {
		container = di.NewContainer()
	}

	// Create mock instances
	mockCLIConfigHandler := config.NewMockConfigHandler()
	mockCLIConfigHandler.LoadConfigFunc = func(path string) error { return nil }
	mockCLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) { return "mock-value", nil }
	mockCLIConfigHandler.GetIntFunc = func(key string, defaultValue ...int) (int, error) { return 0, nil }
	mockCLIConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) (bool, error) { return false, nil }
	mockCLIConfigHandler.SetFunc = func(key string, value interface{}) error { return nil }
	mockCLIConfigHandler.SaveConfigFunc = func(path string) error { return nil }
	mockCLIConfigHandler.GetFunc = func(key string) (interface{}, error) { return nil, nil }
	mockCLIConfigHandler.SetDefaultFunc = func(context config.Context) error { return nil }
	mockCLIConfigHandler.GetConfigFunc = func() (*config.Context, error) { return nil, nil }

	mockShell := shell.NewMockShell()
	mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
		return "mock-exec-output", nil
	}
	mockShell.GetProjectRootFunc = func() (string, error) { return "/mock/project/root", nil }

	mockContext := context.NewMockContext()
	mockContext.GetContextFunc = func() (string, error) { return "mock-context", nil }
	mockContext.SetContextFunc = func(context string) error { return nil }
	mockContext.GetConfigRootFunc = func() (string, error) { return "/mock/config/root", nil }

	// Register mocks in the DI container
	container.Register("cliConfigHandler", mockCLIConfigHandler)
	container.Register("contextHandler", mockContext)
	container.Register("shell", mockShell)

	return &AwsHelperMocks{
		Container:        container,
		CLIConfigHandler: mockCLIConfigHandler,
		Shell:            mockShell,
		Context:          mockContext,
	}
}

func TestAwsHelper_NewAwsHelper(t *testing.T) {
	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Create mock DI container and set resolve error for cliConfigHandler
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveError("cliConfigHandler", fmt.Errorf("error resolving cliConfigHandler"))

		// Attempt to create AwsHelper
		_, err := NewAwsHelper(mockContainer.DIContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving cliConfigHandler") {
			t.Fatalf("expected error resolving cliConfigHandler, got %v", err)
		}
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Create mock DI container and set resolve error for context
		mockContainer := di.NewMockContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContainer.Register("cliConfigHandler", mockConfigHandler)
		mockContainer.SetResolveError("context", fmt.Errorf("error resolving context"))

		// Attempt to create AwsHelper
		_, err := NewAwsHelper(mockContainer.DIContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving context") {
			t.Fatalf("expected error resolving context, got %v", err)
		}
	})
}

func TestAwsHelper_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create mock DI container with necessary mocks
		mocks := createAwsHelperMocks()

		// Create an instance of AwsHelper
		awsHelper, err := NewAwsHelper(mocks.Container.(*di.DIContainer))
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

func TestAwsHelper_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create mock DI container with necessary mocks
		mocks := createAwsHelperMocks()

		// Mock GetConfig to return a valid AWS configuration
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				AWS: &config.AWSConfig{
					Localstack: &config.LocalstackConfig{
						Create:   ptrBool(true),
						Services: []string{"s3", "dynamodb"},
					},
				},
			}, nil
		}

		// Create an instance of AwsHelper
		awsHelper, err := NewAwsHelper(mocks.Container.(*di.DIContainer))
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
		// Create mock DI container with necessary mocks
		mocks := createAwsHelperMocks()

		// Mock GetConfig to return a valid AWS configuration
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				AWS: &config.AWSConfig{
					Localstack: &config.LocalstackConfig{
						Create:   ptrBool(true),
						Services: []string{"s3", "dynamodb"},
					},
				},
			}, nil
		}

		// Create an instance of AwsHelper
		awsHelper, err := NewAwsHelper(mocks.Container.(*di.DIContainer))
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

		// Create mock DI container with necessary mocks
		mocks := createAwsHelperMocks()

		// Mock GetConfig to return a valid AWS configuration
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				AWS: &config.AWSConfig{
					Localstack: &config.LocalstackConfig{
						Create:   ptrBool(true),
						Services: []string{"s3", "dynamodb"},
					},
				},
			}, nil
		}

		// Create an instance of AwsHelper
		awsHelper, err := NewAwsHelper(mocks.Container.(*di.DIContainer))
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
		// Create mock DI container with necessary mocks
		mocks := createAwsHelperMocks()

		// Mock GetConfig to return an error
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, fmt.Errorf("error retrieving context config")
		}

		// Create an instance of AwsHelper
		awsHelper, err := NewAwsHelper(mocks.Container.(*di.DIContainer))
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
		// Create mock DI container with necessary mocks
		mocks := createAwsHelperMocks()

		// Mock GetConfig to return a context with nil AWS configuration
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				AWS: nil,
			}, nil
		}

		// Create an instance of AwsHelper
		awsHelper, err := NewAwsHelper(mocks.Container.(*di.DIContainer))
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
		// Create mock DI container with necessary mocks
		mocks := createAwsHelperMocks()

		// Mock GetConfig to return a context with nil Localstack configuration
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				AWS: &config.AWSConfig{
					Localstack: nil,
				},
			}, nil
		}

		// Create an instance of AwsHelper
		awsHelper, err := NewAwsHelper(mocks.Container.(*di.DIContainer))
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
		// Create mock DI container with necessary mocks
		mocks := createAwsHelperMocks()

		// Create an instance of AwsHelper
		awsHelper, err := NewAwsHelper(mocks.Container.(*di.DIContainer))
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

func TestAwsHelper_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := createAwsHelperMocks()

		// Create an instance of AwsHelper
		awsHelper, err := NewAwsHelper(mocks.Container.(*di.DIContainer))
		if err != nil {
			t.Fatalf("NewAwsHelper() error = %v", err)
		}

		// When: Up is called
		err = awsHelper.Up()
		if err != nil {
			t.Fatalf("Up() error = %v", err)
		}
	})
}

func TestAwsHelper_Info(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create mock DI container with necessary mocks
		mocks := createAwsHelperMocks()

		// Create an instance of AwsHelper
		awsHelper, err := NewAwsHelper(mocks.Container.(*di.DIContainer))
		if err != nil {
			t.Fatalf("NewAwsHelper() error = %v", err)
		}

		// When: Info is called
		info, err := awsHelper.Info()
		if err != nil {
			t.Fatalf("Info() error = %v", err)
		}

		// Then: no error should be returned and info should be nil
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if info != nil {
			t.Errorf("Expected info to be nil, got %v", info)
		}
	})
}
