package services

import (
	"fmt"
	"math"
	"os"
	"strconv"
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
			return "192.168.1.1:" + strconv.Itoa(constants.DEFAULT_TALOS_API_PORT)
		case "cluster.workers.nodes.worker2.endpoint":
			return "192.168.1.2:50001"
		case "dns.domain":
			return "test"
		case "cluster.workers.local_volume_path":
			return "/var/local"
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
		case "cluster.workers.nodes.worker1.volumes":
			return []string{"/data/worker1:/mnt/data", "/logs/worker1:/mnt/logs"}
		case "cluster.workers.nodes.worker2.volumes":
			return []string{"/data/worker2:/mnt/data", "/logs/worker2:/mnt/logs"}
		case "cluster.workers.volumes":
			return []string{"/data/common:/mnt/data", "/logs/common:/mnt/logs"}
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
				Workers: cluster.NodeGroupConfig{
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
		// Reset package-level variables
		nextAPIPort = constants.DEFAULT_TALOS_API_PORT + 1

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
		usedHostPorts[constants.DEFAULT_TALOS_API_PORT] = true // Ensure the defaultAPIPort is also marked as used
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

	t.Run("PortIncrement", func(t *testing.T) {
		// Reset package-level variables
		nextAPIPort = constants.DEFAULT_TALOS_API_PORT + 1

		// Setup mocks for this test
		mocks := setupTalosServiceMocks()

		// Mock vm.driver to enable localhost mode
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			return ""
		}

		// Create and initialize first service (non-leader)
		service1 := NewTalosService(mocks.Injector, "worker1")
		service1.isLeader = false
		err := service1.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Set address for first service
		err = service1.SetAddress("127.0.0.1")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Create and initialize second service (non-leader)
		service2 := NewTalosService(mocks.Injector, "worker2")
		service2.isLeader = false
		err = service2.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Set address for second service
		err = service2.SetAddress("127.0.0.1")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify that the ports were incremented correctly
		expectedPort1 := constants.DEFAULT_TALOS_API_PORT + 1
		expectedPort2 := constants.DEFAULT_TALOS_API_PORT + 2

		// Check if the ports were set correctly in the config handler
		var setContextValueCalls = make(map[string]interface{})
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			setContextValueCalls[key] = value
			return nil
		}

		// Set endpoints for both services
		err = mocks.MockConfigHandler.SetContextValue("cluster.workers.nodes.worker1.endpoint", fmt.Sprintf("127.0.0.1:%d", expectedPort1))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		err = mocks.MockConfigHandler.SetContextValue("cluster.workers.nodes.worker2.endpoint", fmt.Sprintf("127.0.0.1:%d", expectedPort2))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify the endpoints were set with correct ports
		endpoint1 := setContextValueCalls["cluster.workers.nodes.worker1.endpoint"]
		endpoint2 := setContextValueCalls["cluster.workers.nodes.worker2.endpoint"]

		if endpoint1 != fmt.Sprintf("127.0.0.1:%d", expectedPort1) {
			t.Errorf("Expected endpoint1 to be 127.0.0.1:%d, got %v", expectedPort1, endpoint1)
		}
		if endpoint2 != fmt.Sprintf("127.0.0.1:%d", expectedPort2) {
			t.Errorf("Expected endpoint2 to be 127.0.0.1:%d, got %v", expectedPort2, endpoint2)
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

	t.Run("InvalidVolumeFormat", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "worker")

		// Mock the GetStringSlice method to return an invalid volume format
		mocks.MockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			if key == "cluster.workers.volumes" {
				return []string{"invalidVolumeFormat"}
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
			t.Fatalf("expected an error due to invalid volume format, got nil")
		}
		if err.Error() != "invalid volume format: invalidVolumeFormat" {
			t.Fatalf("expected error message 'invalid volume format: invalidVolumeFormat', got %v", err)
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

	t.Run("ErrorMkdirAll", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "worker")

		// Mock the mkdirAll function to return an error
		originalMkdirAll := mkdirAll
		defer func() { mkdirAll = originalMkdirAll }()
		mkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mocked mkdirAll error")
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
			t.Fatalf("expected an error due to mkdirAll failure, got nil")
		}
		if !strings.Contains(err.Error(), "mocked mkdirAll error") {
			t.Fatalf("expected error message containing 'mocked mkdirAll error', got %v", err)
		}
	})

	t.Run("InvalidHostPortFormat", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "worker")

		// Mock the GetStringSlice method to return an invalid host port format
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

	t.Run("InvalidHostPortValue", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "worker")

		// Mock the GetStringSlice method to return an invalid host port value
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

	t.Run("LocalhostModeControlPlaneLeader", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "controlplane")

		// Mock vm.driver to enable localhost mode
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			return ""
		}

		// Set isLeader to true
		service.isLeader = true

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// When the GetComposeConfig method is called
		config, err := service.GetComposeConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the config should contain both API and Kubernetes ports
		if len(config.Services) != 1 {
			t.Fatalf("expected 1 service, got %d", len(config.Services))
		}

		serviceConfig := config.Services[0]
		if len(serviceConfig.Ports) != 2 {
			t.Fatalf("expected 2 ports, got %d", len(serviceConfig.Ports))
		}

		// Verify API port
		foundAPIPort := false
		foundKubePort := false
		for _, port := range serviceConfig.Ports {
			if port.Target == uint32(constants.DEFAULT_TALOS_API_PORT) && port.Protocol == "tcp" {
				foundAPIPort = true
			}
			if port.Target == 6443 && port.Published == "6443" && port.Protocol == "tcp" {
				foundKubePort = true
			}
		}

		if !foundAPIPort {
			t.Error("expected to find API port configuration")
		}
		if !foundKubePort {
			t.Error("expected to find Kubernetes API port configuration")
		}
	})

	t.Run("LocalhostModeControlPlaneNonLeader", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()
		service := NewTalosService(mocks.Injector, "controlplane")

		// Mock vm.driver to enable localhost mode
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			return ""
		}

		// Set isLeader to false
		service.isLeader = false

		// Initialize the service
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// When the GetComposeConfig method is called
		config, err := service.GetComposeConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the config should contain only the API port
		if len(config.Services) != 1 {
			t.Fatalf("expected 1 service, got %d", len(config.Services))
		}

		serviceConfig := config.Services[0]
		if len(serviceConfig.Ports) != 1 {
			t.Fatalf("expected 1 port, got %d", len(serviceConfig.Ports))
		}

		// Verify only API port is present
		port := serviceConfig.Ports[0]
		if port.Target != uint32(constants.DEFAULT_TALOS_API_PORT) || port.Protocol != "tcp" {
			t.Errorf("expected API port configuration, got target=%d protocol=%s", port.Target, port.Protocol)
		}
	})

	t.Run("PortIncrementInGetComposeConfig", func(t *testing.T) {
		// Reset package-level variables
		nextAPIPort = constants.DEFAULT_TALOS_API_PORT + 1

		// Setup mocks for this test
		mocks := setupTalosServiceMocks()

		// Track SetContextValue calls
		setContextValueCalls := make(map[string]string)
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			if strValue, ok := value.(string); ok {
				setContextValueCalls[key] = strValue
			}
			return nil
		}

		// Mock GetStringSlice to return empty hostports
		mocks.MockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			return []string{}
		}

		// Mock GetString to return the stored endpoint values
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			if strings.HasSuffix(key, ".endpoint") {
				if value, exists := setContextValueCalls[key]; exists {
					return value
				}
			}
			return ""
		}

		// Create and initialize first service (non-leader)
		service1 := NewTalosService(mocks.Injector, "worker1")
		service1.isLeader = false
		service1.SetName("worker1")
		err := service1.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Set address for first service
		err = service1.SetAddress("127.0.0.1")
		if err != nil {
			t.Fatalf("expected no error setting address, got %v", err)
		}

		// Get compose config for first service
		config1, err := service1.GetComposeConfig()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Create and initialize second service (non-leader)
		service2 := NewTalosService(mocks.Injector, "worker2")
		service2.isLeader = false
		service2.SetName("worker2")
		err = service2.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Set address for second service
		err = service2.SetAddress("127.0.0.1")
		if err != nil {
			t.Fatalf("expected no error setting address, got %v", err)
		}

		// Get compose config for second service
		config2, err := service2.GetComposeConfig()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify port configurations
		if len(config1.Services) != 1 {
			t.Fatalf("expected 1 service in config1, got %d", len(config1.Services))
		}
		if len(config2.Services) != 1 {
			t.Fatalf("expected 1 service in config2, got %d", len(config2.Services))
		}

		// Check ports for first service
		ports1 := config1.Services[0].Ports
		if len(ports1) != 1 {
			t.Fatalf("expected 1 port in service1, got %d", len(ports1))
		}
		if ports1[0].Target != uint32(constants.DEFAULT_TALOS_API_PORT) || ports1[0].Published != "50001" {
			t.Errorf("expected port %d:50001 in service1, got %d:%s", constants.DEFAULT_TALOS_API_PORT, ports1[0].Target, ports1[0].Published)
		}

		// Check ports for second service
		ports2 := config2.Services[0].Ports
		if len(ports2) != 1 {
			t.Fatalf("expected 1 port in service2, got %d", len(ports2))
		}
		if ports2[0].Target != uint32(constants.DEFAULT_TALOS_API_PORT) || ports2[0].Published != "50002" {
			t.Errorf("expected port %d:50002 in service2, got %d:%s", constants.DEFAULT_TALOS_API_PORT, ports2[0].Target, ports2[0].Published)
		}
	})

	t.Run("DNSConfiguration", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()

		// Mock GetString to return DNS address
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.address" {
				return "10.0.0.53"
			}
			return ""
		}

		// Create and initialize service
		service := NewTalosService(mocks.Injector, "worker")
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Get compose config
		config, err := service.GetComposeConfig()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify DNS configuration
		if len(config.Services) != 1 {
			t.Fatalf("expected 1 service, got %d", len(config.Services))
		}

		serviceConfig := config.Services[0]
		if serviceConfig.DNS == nil {
			t.Fatal("expected DNS to be initialized")
		}
		if len(serviceConfig.DNS) != 1 {
			t.Fatalf("expected 1 DNS entry, got %d", len(serviceConfig.DNS))
		}
		if serviceConfig.DNS[0] != "10.0.0.53" {
			t.Errorf("expected DNS address 10.0.0.53, got %s", serviceConfig.DNS[0])
		}
	})

	t.Run("DNSConfigurationDuplicate", func(t *testing.T) {
		// Setup mocks for this test
		mocks := setupTalosServiceMocks()

		// Mock GetString to return DNS address
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.address" {
				return "10.0.0.53"
			}
			return ""
		}

		// Create and initialize service
		service := NewTalosService(mocks.Injector, "worker")
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Get compose config twice to test duplicate prevention
		config1, err := service.GetComposeConfig()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		config2, err := service.GetComposeConfig()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify DNS configuration in both configs
		if len(config1.Services) != 1 || len(config2.Services) != 1 {
			t.Fatalf("expected 1 service in each config, got %d and %d", len(config1.Services), len(config2.Services))
		}

		serviceConfig1 := config1.Services[0]
		serviceConfig2 := config2.Services[0]

		if serviceConfig1.DNS == nil || serviceConfig2.DNS == nil {
			t.Fatal("expected DNS to be initialized in both configs")
		}
		if len(serviceConfig1.DNS) != 1 || len(serviceConfig2.DNS) != 1 {
			t.Fatalf("expected 1 DNS entry in each config, got %d and %d", len(serviceConfig1.DNS), len(serviceConfig2.DNS))
		}
		if serviceConfig1.DNS[0] != "10.0.0.53" || serviceConfig2.DNS[0] != "10.0.0.53" {
			t.Errorf("expected DNS address 10.0.0.53 in both configs, got %s and %s", serviceConfig1.DNS[0], serviceConfig2.DNS[0])
		}
	})
}
