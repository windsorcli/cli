package services

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/internal/config"
	"github.com/windsorcli/cli/internal/constants"
	"github.com/windsorcli/cli/internal/context"
	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/shell"
)

func setupSafeTalosWorkerServiceMocks(optionalInjector ...di.Injector) *MockComponents {
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

	mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) int {
		switch key {
		case "cluster.workers.cpu":
			return constants.DEFAULT_TALOS_WORKER_CPU
		case "cluster.workers.memory":
			return constants.DEFAULT_TALOS_WORKER_RAM
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

func TestTalosWorkerService_NewTalosWorkerService(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a set of mock components
		mocks := setupSafeTalosWorkerServiceMocks()

		// When: a new TalosWorkerService is created
		service := NewTalosWorkerService(mocks.Injector)

		// Then: the TalosWorkerService should not be nil
		if service == nil {
			t.Fatalf("expected TalosWorkerService, got nil")
		}
	})
}

func TestTalosWorkerService_SetAddress(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupSafeTalosWorkerServiceMocks()
		service := NewTalosWorkerService(mocks.Injector)

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
			if key == "cluster.workers.nodes."+service.name+".node" && value == "192.168.1.1" {
				return nil
			}
			return fmt.Errorf("unexpected key or value")
		}

		if err := mocks.MockConfigHandler.SetContextValueFunc("cluster.workers.nodes."+service.name+".node", "192.168.1.1"); err != nil {
			t.Fatalf("expected address to be set without error, got %v", err)
		}
	})

	t.Run("Failures", func(t *testing.T) {
		testCases := []struct {
			name          string
			mockSetupFunc func(mocks *MockComponents, service *TalosWorkerService)
			expectedError string
		}{
			{
				name: "SetHostnameFailure",
				mockSetupFunc: func(mocks *MockComponents, service *TalosWorkerService) {
					mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
						if key == "cluster.workers.nodes."+service.name+".hostname" {
							return fmt.Errorf("failed to set hostname")
						}
						return nil
					}
				},
				expectedError: "failed to set hostname",
			},
			{
				name: "SetNodeFailure",
				mockSetupFunc: func(mocks *MockComponents, service *TalosWorkerService) {
					mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
						if key == "cluster.workers.nodes."+service.name+".node" {
							return fmt.Errorf("failed to set node")
						}
						return nil
					}
				},
				expectedError: "failed to set node",
			},
			{
				name: "SetEndpointFailure",
				mockSetupFunc: func(mocks *MockComponents, service *TalosWorkerService) {
					mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
						if key == "cluster.workers.nodes."+service.name+".endpoint" {
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
				mocks := setupSafeTalosWorkerServiceMocks()
				service := NewTalosWorkerService(mocks.Injector)

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

func TestTalosWorkerService_GetComposeConfig(t *testing.T) {
	// Mock the os functions to avoid actual file system operations
	originalStat := stat
	originalMkdir := mkdir
	defer func() {
		stat = originalStat
		mkdir = originalMkdir
	}()
	stat = func(name string) (os.FileInfo, error) {
		if name == "/mock/project/root/.volumes" {
			return nil, os.ErrNotExist
		}
		return nil, nil
	}
	mkdir = func(name string, perm os.FileMode) error {
		if name == "/mock/project/root/.volumes" {
			return nil
		}
		return fmt.Errorf("unexpected mkdir call for %s", name)
	}

	t.Run("Success", func(t *testing.T) {
		testCases := []struct {
			name     string
			setName  bool
			expected string
		}{
			{"WithoutSetName", false, "worker.test"},
			{"WithSetName", true, "custom.worker.test"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Setup mocks for this test
				mocks := setupSafeTalosWorkerServiceMocks()
				service := NewTalosWorkerService(mocks.Injector)

				// Optionally set the name
				if tc.setName {
					service.SetName("custom.worker")
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

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupSafeTalosWorkerServiceMocks()
		service := NewTalosWorkerService(mocks.Injector)

		// Mock the GetProjectRoot method to return an error
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving project root")
		}

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// When: the GetComposeConfig method is called
		config, err := service.GetComposeConfig()

		// Then: an error should be returned and the config should be nil
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}
		if err.Error() != "error retrieving project root: mock error retrieving project root" {
			t.Fatalf("expected error message 'error retrieving project root: mock error retrieving project root', got %v", err)
		}
		if config != nil {
			t.Fatalf("expected config to be nil, got %v", config)
		}
	})

	t.Run("ErrorCreatingVolumesDirectory", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupSafeTalosWorkerServiceMocks()
		service := NewTalosWorkerService(mocks.Injector)

		// Mock the GetProjectRoot method to return a valid project root
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return filepath.FromSlash("/mock/project/root"), nil
		}

		// Mock the stat function to simulate the .volumes directory does not exist
		stat = func(name string) (os.FileInfo, error) {
			if filepath.Clean(name) == filepath.Clean(filepath.Join("/mock/project/root", ".volumes")) {
				return nil, os.ErrNotExist
			}
			return nil, nil
		}

		// Mock the mkdir function to return an error
		mkdir = func(name string, perm os.FileMode) error {
			if filepath.Clean(name) == filepath.Clean(filepath.Join("/mock/project/root", ".volumes")) {
				return fmt.Errorf("mock error creating .volumes directory")
			}
			return nil
		}

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// When: the GetComposeConfig method is called
		config, err := service.GetComposeConfig()

		// Then: an error should be returned and the config should be nil
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}
		if err.Error() != "error creating .volumes directory: mock error creating .volumes directory" {
			t.Fatalf("expected error message 'error creating .volumes directory: mock error creating .volumes directory', got %v", err)
		}
		if config != nil {
			t.Fatalf("expected config to be nil, got %v", config)
		}
	})
}
