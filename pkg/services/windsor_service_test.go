package services

import (
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
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

	// Register mock instances in the injector
	injector.Register("configHandler", mockConfigHandler)

	return &MockComponents{
		Injector:          injector,
		MockConfigHandler: mockConfigHandler,
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

		windsorService.Initialize()

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
}
