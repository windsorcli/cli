package services

import (
	"fmt"
	"testing"

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
		if key == "dns.name" {
			return "test"
		}
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

func TestTalosControlPlaneService_SetAddress(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a set of mock components
		mocks := setupSafeTalosControlPlaneServiceMocks()
		service := NewTalosControlPlaneService(mocks.Injector)

		// Create a map to track the calls to SetFunc
		setCalls := make(map[string]interface{})

		mocks.MockConfigHandler.SetFunc = func(key string, value interface{}) error {
			setCalls[key] = value
			return nil
		}

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// When: the SetAddress method is called
		service.SetName("controlplane-1")
		err = service.SetAddress("192.168.1.1")

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And: configHandler.Set should be called with the expected node and endpoint
		expectedHostnameKey := "cluster.controlplanes.nodes.controlplane-1.hostname"
		expectedNodeKey := "cluster.controlplanes.nodes.controlplane-1.node"
		expectedEndpointKey := "cluster.controlplanes.nodes.controlplane-1.endpoint"
		expectedHostnameValue := "controlplane-1.test"
		expectedNodeValue := "192.168.1.1:50000"
		expectedEndpointValue := "192.168.1.1"

		if setCalls[expectedHostnameKey] != expectedHostnameValue {
			t.Errorf("expected %s to be set to %s, got %v", expectedHostnameKey, expectedHostnameValue, setCalls[expectedHostnameKey])
		}

		if setCalls[expectedNodeKey] != expectedNodeValue {
			t.Errorf("expected %s to be set to %s, got %v", expectedNodeKey, expectedNodeValue, setCalls[expectedNodeKey])
		}

		if setCalls[expectedEndpointKey] != expectedEndpointValue {
			t.Errorf("expected %s to be set to %s, got %v", expectedEndpointKey, expectedEndpointValue, setCalls[expectedEndpointKey])
		}
	})

	t.Run("SetFailures", func(t *testing.T) {
		// Given: a set of mock components
		mocks := setupSafeTalosControlPlaneServiceMocks()
		service := NewTalosControlPlaneService(mocks.Injector)

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}
		service.SetName("controlplane-1")

		// Define the error scenarios
		errorScenarios := []struct {
			description string
			setup       func()
		}{
			{
				description: "configHandler.Set hostname fails",
				setup: func() {
					mocks.MockConfigHandler.SetFunc = func(key string, value interface{}) error {
						if key == "cluster.controlplanes.nodes.controlplane-1.hostname" {
							return fmt.Errorf("configHandler.Set hostname error")
						}
						return nil
					}
				},
			},
			{
				description: "configHandler.Set node fails",
				setup: func() {
					mocks.MockConfigHandler.SetFunc = func(key string, value interface{}) error {
						if key == "cluster.controlplanes.nodes.controlplane-1.node" {
							return fmt.Errorf("configHandler.Set node error")
						}
						return nil
					}
				},
			},
			{
				description: "configHandler.Set endpoint fails",
				setup: func() {
					mocks.MockConfigHandler.SetFunc = func(key string, value interface{}) error {
						if key == "cluster.controlplanes.nodes.controlplane-1.endpoint" {
							return fmt.Errorf("configHandler.Set endpoint error")
						}
						return nil
					}
				},
			},
		}

		for _, scenario := range errorScenarios {
			t.Run(scenario.description, func(t *testing.T) {
				// Setup the specific error scenario
				scenario.setup()

				// When: the SetAddress method is called
				err := service.SetAddress("192.168.1.1")

				// Then: an error should be returned
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
			})
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
			{"WithoutSetName", false, "controlplane.test"},
			{"WithSetName", true, "custom.test"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Setup mocks for this test
				mocks := setupSafeTalosControlPlaneServiceMocks()
				service := NewTalosControlPlaneService(mocks.Injector)

				// Optionally set the name
				if tc.setName {
					service.SetName("custom")
				}

				// Initialize the service
				err := service.Initialize()
				if err != nil {
					t.Fatalf("expected no error during initialization, got %v", err)
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
					t.Fatalf("expected 1 service, got %d", len(config.Services))
				}
				if config.Services[0].Name != tc.expected {
					t.Fatalf("expected service name %s, got %s", tc.expected, config.Services[0].Name)
				}
				if config.Services[0].Image != constants.DEFAULT_TALOS_IMAGE {
					t.Fatalf("expected service image %s, got %s", constants.DEFAULT_TALOS_IMAGE, config.Services[0].Image)
				}
			})
		}
	})
}
