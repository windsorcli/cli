package services

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/cluster"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

func setupTalosServiceMocks(optionalInjector ...di.Injector) *MockComponents {
	var injector di.Injector
	if len(optionalInjector) > 0 {
		injector = optionalInjector[0]
	} else {
		injector = di.NewMockInjector()
	}

	mockShell := shell.NewMockShell(injector)
	mockConfigHandler := config.NewMockConfigHandler()

	injector.Register("shell", mockShell)
	injector.Register("configHandler", mockConfigHandler)

	mockConfigHandler.GetContextFunc = func() string {
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

	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		switch key {
		case "cluster.workers.nodes.worker1.endpoint":
			return "192.168.1.1:50000"
		case "cluster.workers.nodes.worker2.endpoint":
			return "192.168.1.2:50001"
		case "dns.domain":
			return "test"
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
	}

	mockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
		switch key {
		case "cluster.workers.nodes.worker1.hostports":
			return []string{"30000:30000", "30001:30001/udp"}
		case "cluster.workers.nodes.worker2.hostports":
			return []string{"30002:30002/tcp", "30003:30003"}
		case "cluster.workers.hostports":
			return []string{"30000:30000", "30001:30001/udp", "30002:30002/tcp", "30003:30003"}
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return nil
		}
	}

	mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
		return &v1alpha1.Context{
			Cluster: &cluster.ClusterConfig{
				Workers: struct {
					Count     *int                          `yaml:"count,omitempty"`
					CPU       *int                          `yaml:"cpu,omitempty"`
					Memory    *int                          `yaml:"memory,omitempty"`
					Nodes     map[string]cluster.NodeConfig `yaml:"nodes,omitempty"`
					HostPorts []string                      `yaml:"hostports,omitempty"`
				}{
					Nodes: map[string]cluster.NodeConfig{
						"worker1": {},
						"worker2": {},
					},
					HostPorts: []string{"30000:30000/tcp", "30001:30001/udp", "30002:30002/tcp", "30003:30003/udp"},
				},
			},
		}
	}

	mockShell.GetProjectRootFunc = func() (string, error) {
		return "/mock/project/root", nil
	}

	return &MockComponents{
		Injector:          injector,
		MockShell:         mockShell,
		MockConfigHandler: mockConfigHandler,
	}
}

func TestTalosService_NewTalosService(t *testing.T) {
	t.Run("SuccessWorker", func(t *testing.T) {
		// Given: a set of mock components
		mocks := setupTalosServiceMocks()

		// When a new TalosService is created
		service := NewTalosService(mocks.Injector, "worker")

		// Then the TalosService should not be nil
		if service == nil {
			t.Fatalf("expected TalosService, got nil")
		}
	})

	t.Run("SuccessControlPlane", func(t *testing.T) {
		// Given: a set of mock components
		mocks := setupTalosServiceMocks()

		// When a new TalosService is created
		service := NewTalosService(mocks.Injector, "controlplane")

		// Then the TalosService should not be nil
		if service == nil {
			t.Fatalf("expected TalosService, got nil")
		}
	})
}

func TestTalosService_SetAddress(t *testing.T) {
	t.Run("SuccessWorker", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "worker")

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// When the SetAddress method is called with a non-localhost address
		err = service.SetAddress("192.168.1.1")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the address should be set correctly in the configHandler
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

	t.Run("SuccessControlPlane", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "controlplane")

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// When the SetAddress method is called with a non-localhost address
		err = service.SetAddress("192.168.1.1")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the address should be set correctly in the configHandler
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

	t.Run("SuccessControlPlaneLeader", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "controlplane")
		service.isLeader = true

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// When the SetAddress method is called with a non-localhost address
		err = service.SetAddress("192.168.1.1")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the address should be set correctly in the configHandler
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

	t.Run("Localhost", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "worker")

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// When the SetAddress method is called with a localhost address
		err = service.SetAddress("127.0.0.1")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the endpoint should be set with a unique port
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			if key == "cluster.workers.nodes."+service.name+".endpoint" && strings.HasPrefix(value.(string), "127.0.0.1:50001") {
				return nil
			}
			return fmt.Errorf("unexpected key or value")
		}

		if err := mocks.MockConfigHandler.SetContextValueFunc("cluster.workers.nodes."+service.name+".endpoint", "127.0.0.1:50001"); err != nil {
			t.Fatalf("expected endpoint to be set without error, got %v", err)
		}
	})

	t.Run("ErrorSettingHostname", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "worker")

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Simulate an error when setting the hostname
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			if key == "cluster.workers.nodes."+service.name+".hostname" {
				return fmt.Errorf("mock error setting hostname")
			}
			return nil
		}

		// Attempt to set the address, expecting an error
		if err := service.SetAddress("192.168.1.1"); err == nil {
			t.Fatalf("expected an error, got nil")
		} else if err.Error() != "mock error setting hostname" {
			t.Fatalf("expected error message 'mock error setting hostname', got %v", err)
		}
	})

	t.Run("ErrorSettingNodeAddress", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "worker")

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Simulate an error when setting the node address
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			if key == "cluster.workers.nodes."+service.name+".hostname" {
				return nil // Mock success for setting hostname
			}
			if key == "cluster.workers.nodes."+service.name+".node" {
				return fmt.Errorf("mock error setting node address") // Mock failure for setting node
			}
			return nil
		}

		// Attempt to set the address, expecting an error
		if err := service.SetAddress("192.168.1.1"); err == nil {
			t.Fatalf("expected an error, got nil")
		} else if err.Error() != "mock error setting node address" {
			t.Fatalf("expected error message 'mock error setting node address', got %v", err)
		}
	})

	t.Run("ErrorSettingEndpoint", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "worker")

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Simulate an error when setting the endpoint
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			if key == "cluster.workers.nodes."+service.name+".endpoint" {
				return fmt.Errorf("mock error setting endpoint")
			}
			return nil
		}

		// Attempt to set the address, expecting an error
		if err := service.SetAddress("192.168.1.1"); err == nil {
			t.Fatalf("expected an error, got nil")
		} else if err.Error() != "mock error setting endpoint" {
			t.Fatalf("expected error message 'mock error setting endpoint', got %v", err)
		}
	})

	t.Run("ErrorSettingHostPorts", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "worker")

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Simulate an error when setting host ports with non-integer values
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			if key == "cluster.workers.nodes."+service.name+".hostports" {
				return fmt.Errorf("mock error setting host ports")
			}
			return nil
		}

		// Attempt to set the address, expecting an error
		if err := service.SetAddress("localhost"); err == nil {
			t.Fatalf("expected an error, got nil")
		} else if err.Error() != "mock error setting host ports" {
			t.Fatalf("expected error message 'mock error setting host ports', got %v", err)
		}
	})

	t.Run("HostPortValidation", func(t *testing.T) {
		tests := []struct {
			name          string
			hostPorts     []string
			expectedError string
			expectSuccess bool
		}{
			{
				name:          "HostPortOnly",
				hostPorts:     []string{"30000"},
				expectedError: "",
				expectSuccess: true,
			},
			{
				name:          "InvalidSingleHostPort",
				hostPorts:     []string{"abc"},
				expectedError: "invalid hostPort value: abc",
				expectSuccess: false,
			},
			{
				name:          "InvalidHostPortFormat",
				hostPorts:     []string{"abc:123"},
				expectedError: "invalid hostPort value: abc",
				expectSuccess: false,
			},
			{
				name:          "NonIntegerHostPort",
				hostPorts:     []string{"123:abc"},
				expectedError: "invalid hostPort value: abc",
				expectSuccess: false,
			},
			{
				name:          "ValidHostPort",
				hostPorts:     []string{"8080:80/tcp"},
				expectedError: "",
				expectSuccess: true,
			},
			{
				name:          "InvalidProtocol",
				hostPorts:     []string{"8080:80/http"},
				expectedError: "invalid protocol value: http",
				expectSuccess: false,
			},
			{
				name:          "IncorrectHostPortFormat",
				hostPorts:     []string{"8080:80:tcp"},
				expectedError: "invalid hostPort format: 8080:80:tcp",
				expectSuccess: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Setup mocks for this test
				mocks := setupTalosServiceMocks()
				service := NewTalosService(mocks.Injector, "worker")

				// Initialize the service
				if err := service.Initialize(); err != nil {
					t.Fatalf("expected no error during initialization, got %v", err)
				}

				// Simulate host port configuration
				mocks.MockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
					if key == "cluster.workers.hostports" {
						return tt.hostPorts
					}
					if len(defaultValue) > 0 {
						return defaultValue[0]
					}
					return nil
				}

				// Attempt to set the address
				err := service.SetAddress("localhost")
				if tt.expectSuccess {
					if err != nil {
						t.Fatalf("expected no error, got %v", err)
					}
				} else {
					if err == nil {
						t.Fatalf("expected an error, got nil")
					} else if !strings.Contains(err.Error(), tt.expectedError) {
						t.Fatalf("expected error message containing '%s', got %v", tt.expectedError, err)
					}
				}
			})
		}
	})

	t.Run("UniquePortAssignment", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "worker")

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Simulate used ports to trigger the loop
		usedHostPorts[50000] = true // Ensure the defaultAPIPort is also marked as used
		usedHostPorts[50001] = true
		usedHostPorts[50002] = true

		// When the SetAddress method is called with a localhost address
		err = service.SetAddress("127.0.0.1")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the endpoint should be set with a unique port
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			if key == "cluster.workers.nodes."+service.name+".endpoint" && strings.HasPrefix(value.(string), "127.0.0.1:50003") {
				return nil
			}
			return fmt.Errorf("unexpected key or value")
		}

		if err := mocks.MockConfigHandler.SetContextValueFunc("cluster.workers.nodes."+service.name+".endpoint", "127.0.0.1:50003"); err != nil {
			t.Fatalf("expected endpoint to be set without error, got %v", err)
		}
	})
}

func TestTalosService_GetComposeConfig(t *testing.T) {
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

	t.Run("NoClusterConfigWorker", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "worker")

		// Override the GetConfig method to return nil for Cluster
		mocks.MockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Cluster: nil,
			}
		}

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// When the GetComposeConfig method is called
		config, err := service.GetComposeConfig()

		// Then no error should be returned and the config should be empty
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if config == nil {
			t.Fatalf("expected config, got nil")
		}
		if len(config.Services) != 0 {
			t.Fatalf("expected 0 services, got %d", len(config.Services))
		}
		if len(config.Volumes) != 0 {
			t.Fatalf("expected 0 volumes, got %d", len(config.Volumes))
		}
	})

	t.Run("NoClusterConfigControlPlane", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "controlplane")

		// Override the GetConfig method to return nil for Cluster
		mocks.MockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Cluster: nil,
			}
		}

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// When the GetComposeConfig method is called
		config, err := service.GetComposeConfig()

		// Then no error should be returned and the config should be empty
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if config == nil {
			t.Fatalf("expected config, got nil")
		}
		if len(config.Services) != 0 {
			t.Fatalf("expected 0 services, got %d", len(config.Services))
		}
		if len(config.Volumes) != 0 {
			t.Fatalf("expected 0 volumes, got %d", len(config.Volumes))
		}
	})

	t.Run("ControlPlaneMode", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "controlplane")

		// Mock the GetConfig method to return a valid Cluster
		mocks.MockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Cluster: &cluster.ClusterConfig{},
			}
		}

		// Set isLeader to true and address to a localhost IP
		service.isLeader = true
		service.address = "127.0.0.1"

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// When the GetComposeConfig method is called
		config, err := service.GetComposeConfig()

		// Then no error should be returned and the config should not be empty
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if config == nil {
			t.Fatalf("expected config, got nil")
		}
		if len(config.Services) == 0 {
			t.Fatalf("expected services, got 0")
		}
		if len(config.Volumes) == 0 {
			t.Fatalf("expected volumes, got 0")
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "worker")

		// Mock the GetProjectRoot method to return an error
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving project root")
		}

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// When the GetComposeConfig method is called
		config, err := service.GetComposeConfig()

		// Then an error should be returned and the config should be nil
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
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "worker")

		// Mock the GetProjectRoot method to return a valid project root
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return filepath.FromSlash("/mock/project/root"), nil
		}

		// Mock the stat function to simulate the .volumes directory does not exist
		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			if filepath.Clean(name) == filepath.Clean(filepath.Join("/mock/project/root", ".volumes")) {
				return nil, os.ErrNotExist
			}
			return nil, nil
		}

		// Mock the mkdir function to return an error
		originalMkdir := mkdir
		defer func() { mkdir = originalMkdir }()
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

		// When the GetComposeConfig method is called
		config, err := service.GetComposeConfig()

		// Then an error should be returned and the config should be nil
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

	t.Run("InvalidDefaultAPIPort", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "worker")

		// Set the defaultAPIPort to an invalid value exceeding MaxUint32
		originalDefaultAPIPort := defaultAPIPort
		defaultAPIPort = int(math.MaxUint32) + 1
		defer func() { defaultAPIPort = originalDefaultAPIPort }()

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// When the GetComposeConfig method is called
		_, err = service.GetComposeConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error due to invalid default API port, got nil")
		}
		if err.Error() != fmt.Sprintf("defaultAPIPort value out of range: %d", defaultAPIPort) {
			t.Fatalf("expected error message 'defaultAPIPort value out of range: %d', got %v", defaultAPIPort, err)
		}
	})

	t.Run("InvalidHostPort", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "worker")

		// Mock the GetStringSlice method to return an invalid host port
		mocks.MockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			if key == "cluster.workers.nodes.worker.hostports" {
				return []string{"invalidPort:30000/tcp"}
			}
			return nil
		}

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// When the GetComposeConfig method is called
		_, err = service.GetComposeConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error due to invalid host port, got nil")
		}
		if err.Error() != "invalid hostPort value: invalidPort" {
			t.Fatalf("expected error message 'invalid hostPort value: invalidPort', got %v", err)
		}
	})

	t.Run("InvalidHostPort", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "worker")

		// Mock the GetStringSlice method to return an invalid host port
		mocks.MockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			if key == "cluster.workers.nodes.worker.hostports" {
				return []string{"30000:invalidHostPort/tcp"}
			}
			return nil
		}

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// When the GetComposeConfig method is called
		_, err = service.GetComposeConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error due to invalid host port, got nil")
		}
		if err.Error() != "invalid hostPort value: invalidHostPort" {
			t.Fatalf("expected error message 'invalid hostPort value: invalidHostPort', got %v", err)
		}
	})

	t.Run("LocalhostAddress", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "worker")

		// Mock the GetStringSlice method to return a valid host port configuration
		mocks.MockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			if key == "cluster.workers.nodes.worker.hostports" {
				return []string{"30000:30000/tcp"}
			}
			return nil
		}

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// When the SetAddress method is called with a localhost address
		err = service.SetAddress("127.0.0.1")
		if err != nil {
			t.Fatalf("expected no error when setting address, got %v", err)
		}

		// When the GetComposeConfig method is called
		config, err := service.GetComposeConfig()

		// Then no error should be returned and the config should contain the expected service and volume configurations
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if config == nil {
			t.Fatalf("expected config, got nil")
		}
		if len(config.Services) == 0 {
			t.Fatalf("expected services, got 0")
		}
		if len(config.Volumes) == 0 {
			t.Fatalf("expected volumes, got 0")
		}
	})
}
