package services

import (
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/internal/config"
	"github.com/windsorcli/cli/internal/constants"
	"github.com/windsorcli/cli/internal/context"
	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/shell"
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
	mockContext.GetContextFunc = func() string {
		return "mock-context"
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

func TestTalosControlPlaneService_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		testCases := []struct {
			name     string
			setName  bool
			expected string
		}{
			{"WithoutSetName", false, "controlplane"},
			{"WithSetName", true, "custom.controlplane"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Setup mocks for this test
				mocks := setupSafeTalosControlPlaneServiceMocks()
				service := NewTalosControlPlaneService(mocks.Injector)

				// Optionally set the name
				if tc.setName {
					service.SetName(tc.expected)
				}

				// Initialize the service
				err := service.Initialize()
				if err != nil {
					t.Fatalf("expected no error during initialization, got %v", err)
				}

				// Mock the GetComposeConfig method to return a valid config
				expectedConfig := &types.Config{
					Services: []types.ServiceConfig{
						{
							Name:  tc.expected,
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
		}
	})
}
