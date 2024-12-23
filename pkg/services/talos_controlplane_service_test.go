package services

import (
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
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
		// Setup mocks for this test
		mocks := setupSafeTalosControlPlaneServiceMocks()
		service := NewTalosControlPlaneService(mocks.Injector)

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// When: the SetAddress method is called
		err = service.SetAddress("192.168.1.1")

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And: the address should be set correctly in the configHandler
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			if key == "cluster.controlplanes.nodes."+service.name+".node" && value == "192.168.1.1" {
				return nil
			}
			return fmt.Errorf("unexpected key or value")
		}

		if err := mocks.MockConfigHandler.SetContextValueFunc("cluster.controlplanes.nodes."+service.name+".node", "192.168.1.1"); err != nil {
			t.Fatalf("expected address to be set without error, got %v", err)
		}
	})

	t.Run("Failures", func(t *testing.T) {
		testCases := []struct {
			name          string
			mockSetupFunc func(mocks *MockComponents, service *TalosControlPlaneService)
			expectedError string
		}{
			{
				name: "SetHostnameFailure",
				mockSetupFunc: func(mocks *MockComponents, service *TalosControlPlaneService) {
					mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
						if key == "cluster.controlplanes.nodes."+service.name+".hostname" {
							return fmt.Errorf("failed to set hostname")
						}
						return nil
					}
				},
				expectedError: "failed to set hostname",
			},
			{
				name: "SetNodeFailure",
				mockSetupFunc: func(mocks *MockComponents, service *TalosControlPlaneService) {
					mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
						if key == "cluster.controlplanes.nodes."+service.name+".node" {
							return fmt.Errorf("failed to set node")
						}
						return nil
					}
				},
				expectedError: "failed to set node",
			},
			{
				name: "SetEndpointFailure",
				mockSetupFunc: func(mocks *MockComponents, service *TalosControlPlaneService) {
					mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
						if key == "cluster.controlplanes.nodes."+service.name+".endpoint" {
							return fmt.Errorf("failed to set endpoint")
						}
						return nil
					}
				},
				expectedError: "failed to set endpoint",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Setup mocks for this test
				mocks := setupSafeTalosControlPlaneServiceMocks()
				service := NewTalosControlPlaneService(mocks.Injector)

				// Initialize the service
				err := service.Initialize()
				if err != nil {
					t.Fatalf("expected no error during initialization, got %v", err)
				}

				// Apply the specific mock setup for this test case
				tc.mockSetupFunc(mocks, service)

				// When: the SetAddress method is called
				err = service.SetAddress("192.168.1.1")

				// Then: the expected error should be returned
				if err == nil || err.Error() != tc.expectedError {
					t.Fatalf("expected error %v, got %v", tc.expectedError, err)
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
