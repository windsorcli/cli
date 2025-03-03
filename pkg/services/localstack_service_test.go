package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/aws"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

type LocalstackServiceMocks struct {
	Injector      di.Injector
	ConfigHandler *config.MockConfigHandler
	Shell         *shell.MockShell
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
		if key == "dns.domain" {
			return "test"
		}
		return "mock-value"
	}

	mockShell := shell.NewMockShell()
	mockShell.ExecFunc = func(command string, args ...string) (string, int, error) {
		return "mock-exec-output", 0, nil
	}
	mockShell.GetProjectRootFunc = func() (string, error) { return filepath.FromSlash("/mock/project/root"), nil }

	mockConfigHandler.GetContextFunc = func() string { return "mock-context" }
	mockConfigHandler.SetContextFunc = func(context string) error { return nil }
	mockConfigHandler.GetConfigRootFunc = func() (string, error) { return filepath.FromSlash("/mock/config/root"), nil }

	// Mock GetConfig to return a valid Localstack configuration with SERVICES set
	mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
		return &v1alpha1.Context{
			AWS: &aws.AWSConfig{
				Localstack: &aws.LocalstackConfig{
					Enabled:  ptrBool(true),
					Services: []string{"s3", "dynamodb"},
				},
			},
		}
	}

	// Mock GetStringSlice to return a list of services for Localstack
	mockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
		if key == "aws.localstack.services" {
			return []string{"s3", "dynamodb"}
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return nil
	}

	// Register mocks in the injector
	injector.Register("configHandler", mockConfigHandler)
	injector.Register("shell", mockShell)

	return &LocalstackServiceMocks{
		Injector:      injector,
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
	}
}

func TestLocalstackService_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create mock injector with necessary mocks
		mocks := createLocalstackServiceMocks()

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
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				AWS: &aws.AWSConfig{
					Localstack: &aws.LocalstackConfig{
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
		if service.Environment["LOCALSTACK_AUTH_TOKEN"] == nil || *service.Environment["LOCALSTACK_AUTH_TOKEN"] != "${LOCALSTACK_AUTH_TOKEN}" {
			t.Errorf("expected service to have LOCALSTACK_AUTH_TOKEN environment variable, got %v", service.Environment["LOCALSTACK_AUTH_TOKEN"])
		}
	})

	t.Run("InvalidServicesDetected", func(t *testing.T) {
		// Create mock injector with necessary mocks
		mocks := createLocalstackServiceMocks()

		// Mock GetStringSlice to return an invalid Localstack configuration
		mocks.ConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			if key == "aws.localstack.services" {
				return []string{"invalidService"}
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return nil
		}

		// Create an instance of LocalstackService
		localstackService := NewLocalstackService(mocks.Injector)

		// Initialize the service
		if err := localstackService.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		_, err := localstackService.GetComposeConfig()

		// Then: an error should be returned indicating invalid services
		if err == nil {
			t.Fatalf("expected error due to invalid services, got nil")
		}

		expectedError := "invalid services found: invalidService"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("expected error to contain %q, got %v", expectedError, err)
		}
	})
}

func TestLocalstackService_SupportsWildcard(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create mock injector with necessary mocks
		mocks := createLocalstackServiceMocks()

		// Create an instance of LocalstackService
		localstackService := NewLocalstackService(mocks.Injector)

		// Initialize the service
		if err := localstackService.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: SupportsWildcard is called
		supportsWildcard := localstackService.SupportsWildcard()

		// Then: the result should match the expected outcome
		expectedSupportsWildcard := true
		if supportsWildcard != expectedSupportsWildcard {
			t.Fatalf("expected SupportsWildcard to be %v, got %v", expectedSupportsWildcard, supportsWildcard)
		}
	})
}
