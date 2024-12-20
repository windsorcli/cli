package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/windsorcli/cli/internal/config"
	"github.com/windsorcli/cli/internal/context"
	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/shell"
)

type LocalstackServiceMocks struct {
	Injector      di.Injector
	ConfigHandler *config.MockConfigHandler
	Shell         *shell.MockShell
	Context       *context.MockContext
}

func createLocalstackServiceMocks(mockInjector ...di.Injector) *LocalstackServiceMocks {
	var injector di.Injector
	if len(mockInjector) > 0 {
		injector = mockInjector[0]
	} else {
		injector = di.NewMockInjector()
	}

	// Create mock instances
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.LoadConfigFunc = func(path string) error { return nil }
	mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) int { return 0 }
	mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool { return false }
	mockConfigHandler.SaveConfigFunc = func(path string) error { return nil }

	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		if key == "dns.name" {
			return "test"
		}
		return "mock-value"
	}

	mockShell := shell.NewMockShell()
	mockShell.ExecFunc = func(command string, args ...string) (string, error) {
		return "mock-exec-output", nil
	}
	mockShell.GetProjectRootFunc = func() (string, error) { return filepath.FromSlash("/mock/project/root"), nil }

	mockContext := context.NewMockContext()
	mockContext.GetContextFunc = func() string { return "mock-context" }
	mockContext.SetContextFunc = func(context string) error { return nil }
	mockContext.GetConfigRootFunc = func() (string, error) { return filepath.FromSlash("/mock/config/root"), nil }

	// Register mocks in the injector
	injector.Register("configHandler", mockConfigHandler)
	injector.Register("contextHandler", mockContext)
	injector.Register("shell", mockShell)

	return &LocalstackServiceMocks{
		Injector:      injector,
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
		Context:       mockContext,
	}
}

func TestLocalstackService_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create mock injector with necessary mocks
		mocks := createLocalstackServiceMocks()

		// Mock GetConfig to return a valid Localstack configuration
		mocks.ConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				AWS: &config.AWSConfig{
					Localstack: &config.LocalstackConfig{
						Enabled:  ptrBool(true),
						Services: []string{"s3", "dynamodb"},
					},
				},
			}
		}

		// Create an instance of LocalstackService
		localstackService := NewLocalstackService(mocks.Injector)

		// Initialize the service
		if err := localstackService.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := localstackService.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then: the compose configuration should include the Localstack service
		if composeConfig == nil || len(composeConfig.Services) == 0 {
			t.Fatalf("expected non-nil composeConfig with services, got %v", composeConfig)
		}

		service := composeConfig.Services[0]
		if service.Name != "aws.test" {
			t.Errorf("expected service name 'localstack', got %v", service.Name)
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
		mocks := createLocalstackServiceMocks()

		// Mock GetConfig to return a valid Localstack configuration
		mocks.ConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				AWS: &config.AWSConfig{
					Localstack: &config.LocalstackConfig{
						Enabled:  ptrBool(true),
						Services: []string{"s3", "dynamodb"},
					},
				},
			}
		}

		// Create an instance of LocalstackService
		localstackService := NewLocalstackService(mocks.Injector)

		// Initialize the service
		if err := localstackService.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := localstackService.GetComposeConfig()
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
}
