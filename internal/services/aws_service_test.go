package services

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

type AwsServiceMocks struct {
	Injector      di.Injector
	ConfigHandler *config.MockConfigHandler
	Shell         *shell.MockShell
	Context       *context.MockContext
}

func createAwsServiceMocks(mockInjector ...di.Injector) *AwsServiceMocks {
	var injector di.Injector
	if len(mockInjector) > 0 {
		injector = mockInjector[0]
	} else {
		injector = di.NewMockInjector()
	}

	// Create mock instances
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.LoadConfigFunc = func(path string) error { return nil }
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string { return "mock-value" }
	mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) int { return 0 }
	mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool { return false }
	mockConfigHandler.SetFunc = func(key string, value interface{}) error { return nil }
	mockConfigHandler.SaveConfigFunc = func(path string) error { return nil }
	mockConfigHandler.GetFunc = func(key string) (interface{}, error) { return nil, nil }
	mockConfigHandler.SetDefaultFunc = func(context config.Context) error { return nil }
	mockConfigHandler.GetConfigFunc = func() *config.Context { return nil }

	mockShell := shell.NewMockShell()
	mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
		return "mock-exec-output", nil
	}
	mockShell.GetProjectRootFunc = func() (string, error) { return filepath.FromSlash("/mock/project/root"), nil }

	mockContext := context.NewMockContext()
	mockContext.GetContextFunc = func() (string, error) { return "mock-context", nil }
	mockContext.SetContextFunc = func(context string) error { return nil }
	mockContext.GetConfigRootFunc = func() (string, error) { return filepath.FromSlash("/mock/config/root"), nil }

	// Register mocks in the injector
	injector.Register("configHandler", mockConfigHandler)
	injector.Register("contextHandler", mockContext)
	injector.Register("shell", mockShell)

	return &AwsServiceMocks{
		Injector:      injector,
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
		Context:       mockContext,
	}
}

func TestAwsService_NewAwsService(t *testing.T) {
	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Create mock injector and set resolve error for configHandler
		mockInjector := di.NewMockInjector()
		mockInjector.SetResolveError("configHandler", fmt.Errorf("error resolving configHandler"))

		// Attempt to create AwsService
		awsService := NewAwsService(mockInjector)
		if awsService == nil {
			t.Fatalf("expected error resolving configHandler")
		}
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Create mock injector and set resolve error for context
		mockInjector := di.NewMockInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockInjector.Register("configHandler", mockConfigHandler)
		mockInjector.SetResolveError("context", fmt.Errorf("error resolving context"))

		// Attempt to create AwsService
		awsService := NewAwsService(mockInjector)
		if awsService == nil {
			t.Fatalf("expected error resolving context")
		}
	})
}

func TestAwsService_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create mock injector with necessary mocks
		mocks := createAwsServiceMocks()

		// Mock GetConfig to return a valid AWS configuration
		mocks.ConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				AWS: &config.AWSConfig{
					Localstack: &config.LocalstackConfig{
						Create:   ptrBool(true),
						Services: []string{"s3", "dynamodb"},
					},
				},
			}
		}

		// Create an instance of AwsService
		awsService := NewAwsService(mocks.Injector)

		// Initialize the service
		if err := awsService.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := awsService.GetComposeConfig()
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
		// Create mock injector with necessary mocks
		mocks := createAwsServiceMocks()

		// Mock GetConfig to return a valid AWS configuration
		mocks.ConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				AWS: &config.AWSConfig{
					Localstack: &config.LocalstackConfig{
						Create:   ptrBool(true),
						Services: []string{"s3", "dynamodb"},
					},
				},
			}
		}

		// Create an instance of AwsService
		awsService := NewAwsService(mocks.Injector)

		// Initialize the service
		if err := awsService.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := awsService.GetComposeConfig()
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

		// Create mock injector with necessary mocks
		mocks := createAwsServiceMocks()

		// Mock GetConfig to return a valid AWS configuration
		mocks.ConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				AWS: &config.AWSConfig{
					Localstack: &config.LocalstackConfig{
						Create:   ptrBool(true),
						Services: []string{"s3", "dynamodb"},
					},
				},
			}
		}

		// Create an instance of AwsService
		awsService := NewAwsService(mocks.Injector)

		// Initialize the service
		if err := awsService.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := awsService.GetComposeConfig()
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

	t.Run("AWSConfigNil", func(t *testing.T) {
		// Create mock injector with necessary mocks
		mocks := createAwsServiceMocks()

		// Mock GetConfig to return a context with nil AWS configuration
		mocks.ConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				AWS: nil,
			}
		}

		// Create an instance of AwsService
		awsService := NewAwsService(mocks.Injector)

		// Initialize the service
		if err := awsService.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := awsService.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then: nil should be returned
		if composeConfig != nil {
			t.Fatalf("expected nil composeConfig, got %v", composeConfig)
		}
	})

	t.Run("LocalstackConfigNil", func(t *testing.T) {
		// Create mock injector with necessary mocks
		mocks := createAwsServiceMocks()

		// Mock GetConfig to return a context with nil Localstack configuration
		mocks.ConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				AWS: &config.AWSConfig{
					Localstack: nil,
				},
			}
		}

		// Create an instance of AwsService
		awsService := NewAwsService(mocks.Injector)

		// Initialize the service
		if err := awsService.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := awsService.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then: nil should be returned
		if composeConfig != nil {
			t.Fatalf("expected nil composeConfig, got %v", composeConfig)
		}
	})
}
