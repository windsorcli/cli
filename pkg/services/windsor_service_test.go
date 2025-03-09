package services

import (
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// setupSafeWindsorServiceMocks sets up mock components for WindsorService
func setupSafeWindsorServiceMocks(optionalInjector ...di.Injector) *MockComponents {
	var injector di.Injector
	if len(optionalInjector) > 0 {
		injector = optionalInjector[0]
	} else {
		injector = di.NewMockInjector()
	}

	mockConfigHandler := config.NewMockConfigHandler()
	mockShell := shell.NewMockShell(injector)

	// Mock some environment variables
	mockEnvVars := map[string]string{
		"ENV_VAR_1": "value1",
		"ENV_VAR_2": "value2",
	}
	mockConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
		if key == "environment" {
			return mockEnvVars
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return nil
	}

	// Mock the DNS enabled configuration
	mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		if key == "dns.enabled" {
			return true
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return false
	}

	// Use a real DNS service instead of a mock
	dnsService := NewDNSService(injector)
	injector.Register("dnsService", dnsService)

	// Register mock instances in the injector
	injector.Register("configHandler", mockConfigHandler)
	injector.Register("shell", mockShell)

	return &MockComponents{
		Injector:          injector,
		MockConfigHandler: mockConfigHandler,
		MockShell:         mockShell,
	}
}

func TestWindsorService_NewWindsorService(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a set of mock components
		mocks := setupSafeWindsorServiceMocks()

		// When: a new WindsorService is created
		windsorService := NewWindsorService(mocks.Injector)
		if windsorService == nil {
			t.Fatalf("expected WindsorService, got nil")
		}

		// Then: the WindsorService should have the correct injector
		if windsorService.injector != mocks.Injector {
			t.Errorf("expected injector %v, got %v", mocks.Injector, windsorService.injector)
		}
	})
}

func TestWindsorService_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a WindsorService instance
		mocks := setupSafeWindsorServiceMocks()
		windsorService := NewWindsorService(mocks.Injector)

		// Initialize the WindsorService
		err := windsorService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := windsorService.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then: verify the configuration contains the expected service
		expectedName := "windsor"
		expectedImage := constants.DEFAULT_WINDSOR_IMAGE
		serviceFound := false

		for _, service := range composeConfig.Services {
			if service.Name == expectedName && service.Image == expectedImage {
				serviceFound = true
				break
			}
		}

		if !serviceFound {
			t.Errorf("expected service with name %q and image %q to be in the list of configurations:\n%+v", expectedName, expectedImage, composeConfig.Services)
		}
	})

	t.Run("ErrorResolvingServices", func(t *testing.T) {
		mockInjector := di.NewMockInjector()

		// Given: a WindsorService instance with a mocked injector that simulates an error
		mocks := setupSafeWindsorServiceMocks(mockInjector)
		mockInjector.SetResolveAllError((*Service)(nil), fmt.Errorf("mocked resolution error"))
		windsorService := NewWindsorService(mocks.Injector)

		// Initialize the WindsorService
		err := windsorService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		_, err = windsorService.GetComposeConfig()

		// Then: an error should be returned due to DNS service resolution failure
		if err == nil || err.Error() != "error retrieving DNS service: mocked resolution error" {
			t.Errorf("expected error 'error retrieving DNS service: mocked resolution error', got %v", err)
		}
	})

	t.Run("NilDNSService", func(t *testing.T) {
		// Given: a WindsorService instance with a nil DNS service
		mocks := setupSafeWindsorServiceMocks()
		mocks.Injector.Register("dnsService", nil)
		windsorService := NewWindsorService(mocks.Injector)

		// Initialize the WindsorService
		err := windsorService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		_, err = windsorService.GetComposeConfig()

		// Then: an error should be returned due to DNS service being nil
		if err == nil || err.Error() != "DNS service not found" {
			t.Errorf("expected error 'DNS service not found', got %v", err)
		}
	})
}
