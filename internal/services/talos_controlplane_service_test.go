package services

import (
	"fmt"
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/constants"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

func setupSafeTalosControlPlaneServiceMocks(optionalInjector ...di.Injector) *MockComponents {
	var injector di.Injector
	if len(optionalInjector) > 0 {
		injector = optionalInjector[0]
	} else {
		injector = di.NewMockInjector()
	}

	mockContext := context.NewMockContext()
	mockShell := shell.NewMockShell(injector)
	mockConfigHandler := config.NewMockConfigHandler()

	// Register mock instances in the injector
	injector.Register("contextHandler", mockContext)
	injector.Register("shell", mockShell)
	injector.Register("configHandler", mockConfigHandler)

	// Implement GetContextFunc on mock context
	mockContext.GetContextFunc = func() (string, error) {
		return "mock-context", nil
	}

	// Mock the functions that are actually called in talos_controlplane_service.go
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		if key == "cluster.driver" {
			return "talos"
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}

	mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) int {
		switch key {
		case "cluster.controlplanes.cpu":
			return constants.DEFAULT_TALOS_CONTROL_PLANE_CPU
		case "cluster.controlplanes.memory":
			return constants.DEFAULT_TALOS_CONTROL_PLANE_RAM
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return 0
		}
	}

	mockShell.GetProjectRootFunc = func() (string, error) {
		return "/mock/project/root", nil
	}

	return &MockComponents{
		Injector:          injector,
		MockContext:       mockContext,
		MockShell:         mockShell,
		MockConfigHandler: mockConfigHandler,
	}
}

func TestTalosControlPlaneService_NewTalosControlPlaneService(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a set of mock components
		mocks := setupSafeTalosControlPlaneServiceMocks()

		// When: a new TalosControlPlaneService is created
		service := NewTalosControlPlaneService(mocks.Injector)

		// Then: the TalosControlPlaneService should not be nil
		if service == nil {
			t.Fatalf("expected TalosControlPlaneService, got nil")
		}
	})
}

func TestTalosControlPlaneService_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a set of mock components
		mocks := setupSafeTalosControlPlaneServiceMocks()

		// When: the TalosControlPlaneService is initialized
		service := NewTalosControlPlaneService(mocks.Injector)
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Given: a set of mock components with an error in resolving configHandler
		mockInjector := di.NewMockInjector()
		setupSafeTalosControlPlaneServiceMocks(mockInjector)
		mockInjector.SetResolveError("configHandler", fmt.Errorf("mock error resolving configHandler"))

		// When: a new TalosControlPlaneService is created
		service := NewTalosControlPlaneService(mockInjector)

		// Then: the initialization should return an error
		err := service.Initialize()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}
		if err.Error() != "error resolving configHandler: mock error resolving configHandler" {
			t.Fatalf("expected error message 'error resolving configHandler: mock error resolving configHandler', got %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Create injector and register configHandler and context
		mockInjector := di.NewMockInjector()
		setupSafeTalosControlPlaneServiceMocks(mockInjector)
		mockInjector.SetResolveError("shell", fmt.Errorf("mock error resolving shell"))

		// Attempt to create TalosControlPlaneService
		service := NewTalosControlPlaneService(mockInjector)
		if service == nil {
			t.Fatalf("expected TalosControlPlaneService, got nil")
		}

		// Initialize the service
		err := service.Initialize()
		if err == nil {
			t.Fatalf("Expected an error during initialization, got nil")
		}
	})
}

func TestTalosControlPlaneService_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupSafeTalosControlPlaneServiceMocks()
		service := NewTalosControlPlaneService(mocks.Injector)
		service.SetName("controlplane-1.test")

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Mock the GetComposeConfig method to return a valid config
		expectedConfig := &types.Config{
			Services: []types.ServiceConfig{
				{
					Name:  "controlplane-1.test",
					Image: constants.DEFAULT_TALOS_IMAGE,
				},
			},
		}

		// When: the GetComposeConfig method is called
		config, err := service.GetComposeConfig()

		// Then: no error should be returned and the config should match the expected config
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if config == nil {
			t.Fatalf("expected config, got nil")
		}
		if len(config.Services) != 1 {
			t.Fatalf("expected 1 services, got %d", len(config.Services))
		}
		if config.Services[0].Name != expectedConfig.Services[0].Name {
			t.Fatalf("expected service name %s, got %s", expectedConfig.Services[0].Name, config.Services[0].Name)
		}
		if config.Services[0].Image != expectedConfig.Services[0].Image {
			t.Fatalf("expected service image %s, got %s", expectedConfig.Services[0].Image, config.Services[0].Image)
		}
	})

	t.Run("ClusterDriverNotTalos", func(t *testing.T) {
		// Given: a set of mock components
		mocks := setupSafeTalosControlPlaneServiceMocks()
		service := NewTalosControlPlaneService(mocks.Injector)

		// Mock the configHandler to return a non-Talos cluster driver
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "non-talos"
			}
			return ""
		}

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// When: the GetComposeConfig method is called
		config, err := service.GetComposeConfig()

		// Then: no error should be returned and the config should be nil
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if config != nil {
			t.Fatalf("expected nil config, got %v", config)
		}
	})
}
