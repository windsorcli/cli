package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

type AwsHelperMocks struct {
	Injector         di.Injector
	CLIConfigHandler *config.MockConfigHandler
	Shell            *shell.MockShell
	Context          *context.MockContext
}

func createAwsHelperMocks(mockInjector ...di.Injector) *AwsHelperMocks {
	var injector di.Injector
	if len(mockInjector) > 0 {
		injector = mockInjector[0]
	} else {
		injector = di.NewMockInjector()
	}

	// Create mock instances
	mockCLIConfigHandler := config.NewMockConfigHandler()
	mockCLIConfigHandler.LoadConfigFunc = func(path string) error { return nil }
	mockCLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string { return "mock-value" }
	mockCLIConfigHandler.GetIntFunc = func(key string, defaultValue ...int) int { return 0 }
	mockCLIConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool { return false }
	mockCLIConfigHandler.SetFunc = func(key string, value interface{}) error { return nil }
	mockCLIConfigHandler.SaveConfigFunc = func(path string) error { return nil }
	mockCLIConfigHandler.GetFunc = func(key string) (interface{}, error) { return nil, nil }
	mockCLIConfigHandler.SetDefaultFunc = func(context config.Context) error { return nil }
	mockCLIConfigHandler.GetConfigFunc = func() *config.Context { return nil }

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
	injector.Register("cliConfigHandler", mockCLIConfigHandler)
	injector.Register("contextHandler", mockContext)
	injector.Register("shell", mockShell)

	return &AwsHelperMocks{
		Injector:         injector,
		CLIConfigHandler: mockCLIConfigHandler,
		Shell:            mockShell,
		Context:          mockContext,
	}
}

func TestAwsHelper_NewAwsHelper(t *testing.T) {
	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Create mock injector and set resolve error for cliConfigHandler
		mockInjector := di.NewMockInjector()
		mockInjector.SetResolveError("cliConfigHandler", fmt.Errorf("error resolving cliConfigHandler"))

		// Attempt to create AwsHelper
		_, err := NewAwsHelper(mockInjector)
		if err == nil || !strings.Contains(err.Error(), "error resolving cliConfigHandler") {
			t.Fatalf("expected error resolving cliConfigHandler, got %v", err)
		}
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Create mock injector and set resolve error for context
		mockInjector := di.NewMockInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockInjector.Register("cliConfigHandler", mockConfigHandler)
		mockInjector.SetResolveError("context", fmt.Errorf("error resolving context"))

		// Attempt to create AwsHelper
		_, err := NewAwsHelper(mockInjector)
		if err == nil || !strings.Contains(err.Error(), "error resolving context") {
			t.Fatalf("expected error resolving context, got %v", err)
		}
	})
}

func TestAwsHelper_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create mock injector with necessary mocks
		mocks := createAwsHelperMocks()

		// Mock GetConfig to return a valid AWS configuration
		mocks.CLIConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				AWS: &config.AWSConfig{
					Localstack: &config.LocalstackConfig{
						Create:   ptrBool(true),
						Services: []string{"s3", "dynamodb"},
					},
				},
			}
		}

		// Create an instance of AwsHelper
		awsHelper, err := NewAwsHelper(mocks.Injector)
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
		// Create mock injector with necessary mocks
		mocks := createAwsHelperMocks()

		// Mock GetConfig to return a valid AWS configuration
		mocks.CLIConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				AWS: &config.AWSConfig{
					Localstack: &config.LocalstackConfig{
						Create:   ptrBool(true),
						Services: []string{"s3", "dynamodb"},
					},
				},
			}
		}

		// Create an instance of AwsHelper
		awsHelper, err := NewAwsHelper(mocks.Injector)
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

		// Create mock injector with necessary mocks
		mocks := createAwsHelperMocks()

		// Mock GetConfig to return a valid AWS configuration
		mocks.CLIConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				AWS: &config.AWSConfig{
					Localstack: &config.LocalstackConfig{
						Create:   ptrBool(true),
						Services: []string{"s3", "dynamodb"},
					},
				},
			}
		}

		// Create an instance of AwsHelper
		awsHelper, err := NewAwsHelper(mocks.Injector)
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

	t.Run("AWSConfigNil", func(t *testing.T) {
		// Create mock injector with necessary mocks
		mocks := createAwsHelperMocks()

		// Mock GetConfig to return a context with nil AWS configuration
		mocks.CLIConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				AWS: nil,
			}
		}

		// Create an instance of AwsHelper
		awsHelper, err := NewAwsHelper(mocks.Injector)
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
		// Create mock injector with necessary mocks
		mocks := createAwsHelperMocks()

		// Mock GetConfig to return a context with nil Localstack configuration
		mocks.CLIConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				AWS: &config.AWSConfig{
					Localstack: nil,
				},
			}
		}

		// Create an instance of AwsHelper
		awsHelper, err := NewAwsHelper(mocks.Injector)
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
