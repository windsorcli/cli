package services

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Test Setup
// =============================================================================

// setupTalosServiceMocks creates and returns mock components for TalosService tests
func setupTalosServiceMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()

	// Create base mocks using setupMocks
	mocks := setupMocks(t, opts...)

	// Load config - use provided config if available, otherwise use default
	var configToLoad string
	if len(opts) > 0 && opts[0].ConfigStr != "" {
		configToLoad = opts[0].ConfigStr
	} else {
		configToLoad = fmt.Sprintf(`
version: v1alpha1
contexts:
  mock-context:
    dns:
      domain: test
    vm:
      driver: docker-desktop
    cluster:
      controlplanes:
        nodes:
          controlplane1:
            endpoint: "192.168.1.10:50000"
      workers:
        nodes:
          worker1:
            endpoint: "192.168.1.1:%d"
            hostports:
              - "30000:30000"
              - "30001:30001/udp"
            volumes:
              - "/data/worker1:/mnt/data"
              - "/logs/worker1:/mnt/logs"
          worker2:
            endpoint: "192.168.1.2:50001"
            hostports:
              - "30002:30002/tcp"
              - "30003:30003"
            volumes:
              - "/data/worker2:/mnt/data"
              - "/logs/worker2:/mnt/logs"
        hostports:
          - "30000:30000/tcp"
          - "30001:30001/udp"
          - "30002:30002/tcp"
          - "30003:30003/udp"
        volumes:
          - "/data/common:/mnt/data"
          - "/logs/common:/mnt/logs"
        local_volume_path: "/var/local"
        cpu: %d
        memory: %d
`, constants.DefaultTalosAPIPort,
			constants.DefaultTalosWorkerCPU,
			constants.DefaultTalosWorkerRAM)
	}

	if err := mocks.ConfigHandler.LoadConfigString(configToLoad); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	mocks.Shell.GetProjectRootFunc = func() (string, error) {
		return "/mock/project/root", nil
	}

	return mocks
}

// =============================================================================
// Test Constructor
// =============================================================================

// TestTalosService_NewTalosService tests the constructor for TalosService
func TestTalosService_NewTalosService(t *testing.T) {
	t.Run("SuccessWorker", func(t *testing.T) {
		// Given a set of mock components
		mocks := setupTalosServiceMocks(t)

		// When a new TalosService is created
		service := NewTalosService(mocks.Injector, "worker")

		// Then the TalosService should not be nil
		if service == nil {
			t.Fatalf("expected TalosService, got nil")
		}
	})

	t.Run("SuccessControlPlane", func(t *testing.T) {
		// Given: a set of mock components
		mocks := setupTalosServiceMocks(t)

		// When a new TalosService is created
		service := NewTalosService(mocks.Injector, "controlplane")

		// Then the TalosService should not be nil
		if service == nil {
			t.Fatalf("expected TalosService, got nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestTalosService_SetAddress tests the SetAddress method of TalosService
func TestTalosService_SetAddress(t *testing.T) {
	setup := func(t *testing.T) (*TalosService, *Mocks) {
		t.Helper()

		// Reset package-level variables
		controlPlaneLeader = nil

		mocks := setupTalosServiceMocks(t)
		service := NewTalosService(mocks.Injector, "controlplane")
		service.SetName("controlplane1")
		if err := service.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service: %v", err)
		}
		return service, mocks
	}

	t.Run("SuccessLeaderControlPlane", func(t *testing.T) {
		// Given a TalosService with mock components
		service, mocks := setup(t)

		// When SetAddress is called
		err := service.SetAddress("192.168.1.10", nil)

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the endpoint should be set correctly
		expectedEndpoint := fmt.Sprintf("127.0.0.1:%d", constants.DefaultTalosAPIPort)
		actualEndpoint := mocks.ConfigHandler.GetString("cluster.controlplanes.nodes.controlplane1.endpoint", "")
		if actualEndpoint != expectedEndpoint {
			t.Errorf("expected endpoint %s, got %s", expectedEndpoint, actualEndpoint)
		}

		// And the hostname should be set correctly
		expectedHostname := "controlplane1"
		actualHostname := mocks.ConfigHandler.GetString("cluster.controlplanes.nodes.controlplane1.hostname", "")
		if actualHostname != expectedHostname {
			t.Errorf("expected hostname %s, got %s", expectedHostname, actualHostname)
		}
	})

	t.Run("SuccessNonLeaderControlPlane", func(t *testing.T) {
		// Given a TalosService with mock components
		mocks := setupTalosServiceMocks(t)

		// Reset package-level variables
		controlPlaneLeader = nil

		// Create a leader first
		leader := NewTalosService(mocks.Injector, "controlplane")
		leader.SetName("controlplane1")
		if err := leader.Initialize(); err != nil {
			t.Fatalf("Failed to initialize leader service: %v", err)
		}
		portAllocator := NewPortAllocator()
		if err := leader.SetAddress("192.168.1.10", portAllocator); err != nil {
			t.Fatalf("Failed to set leader address: %v", err)
		}

		// Create a non-leader control plane
		service := NewTalosService(mocks.Injector, "controlplane")
		service.SetName("controlplane2")
		if err := service.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service: %v", err)
		}

		// Enable localhost mode
		if err := mocks.ConfigHandler.Set("vm.driver", "docker-desktop"); err != nil {
			t.Fatalf("Failed to set VM driver: %v", err)
		}

		// When SetAddress is called
		err := service.SetAddress("192.168.1.11", portAllocator)

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the endpoint should be set correctly with an incremented port
		expectedEndpoint := fmt.Sprintf("127.0.0.1:%d", constants.DefaultTalosAPIPort+1)
		actualEndpoint := mocks.ConfigHandler.GetString("cluster.controlplanes.nodes.controlplane2.endpoint", "")
		if actualEndpoint != expectedEndpoint {
			t.Errorf("expected endpoint %s, got %s", expectedEndpoint, actualEndpoint)
		}

		// And the hostname should be set correctly
		expectedHostname := "controlplane2"
		actualHostname := mocks.ConfigHandler.GetString("cluster.controlplanes.nodes.controlplane2.hostname", "")
		if actualHostname != expectedHostname {
			t.Errorf("expected hostname %s, got %s", expectedHostname, actualHostname)
		}
	})

	t.Run("SuccessWorker", func(t *testing.T) {
		// Given a TalosService with mock components
		mocks := setupTalosServiceMocks(t)

		// Reset package-level variables
		controlPlaneLeader = nil

		// Create a worker node
		service := NewTalosService(mocks.Injector, "worker")
		service.SetName("worker1")
		if err := service.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service: %v", err)
		}

		// Enable localhost mode
		if err := mocks.ConfigHandler.Set("vm.driver", "docker-desktop"); err != nil {
			t.Fatalf("Failed to set VM driver: %v", err)
		}

		// When SetAddress is called
		portAllocator := NewPortAllocator()
		err := service.SetAddress("192.168.1.20", portAllocator)

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the endpoint should be set correctly
		expectedEndpoint := fmt.Sprintf("127.0.0.1:%d", constants.DefaultTalosAPIPort+1)
		actualEndpoint := mocks.ConfigHandler.GetString("cluster.workers.nodes.worker1.endpoint", "")
		if actualEndpoint != expectedEndpoint {
			t.Errorf("expected endpoint %s, got %s", expectedEndpoint, actualEndpoint)
		}

		// And the hostname should be set correctly
		expectedHostname := "worker1"
		actualHostname := mocks.ConfigHandler.GetString("cluster.workers.nodes.worker1.hostname", "")
		if actualHostname != expectedHostname {
			t.Errorf("expected hostname %s, got %s", expectedHostname, actualHostname)
		}
	})

	t.Run("SuccessWithHostPorts", func(t *testing.T) {
		// Given a TalosService with mock components
		mocks := setupTalosServiceMocks(t)

		// Reset package-level variables
		controlPlaneLeader = nil

		// Create a worker node with host ports
		service := NewTalosService(mocks.Injector, "worker")
		service.SetName("worker1")
		if err := service.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service: %v", err)
		}

		// Configure host ports
		hostPorts := []string{
			"30000:30000",
			"30001:30001/udp",
			"30002:30002/tcp",
		}
		if err := mocks.ConfigHandler.Set("cluster.workers.hostports", hostPorts); err != nil {
			t.Fatalf("Failed to set host ports: %v", err)
		}

		// When SetAddress is called
		err := service.SetAddress("192.168.1.20", nil)

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the host ports should be set correctly with incremented ports if needed
		actualHostPorts := mocks.ConfigHandler.GetStringSlice("cluster.workers.nodes.worker1.hostports", []string{})
		expectedHostPorts := []string{
			"30000:30000/tcp",
			"30001:30001/udp",
			"30002:30002/tcp",
		}

		if len(actualHostPorts) != len(expectedHostPorts) {
			t.Errorf("expected %d host ports, got %d", len(expectedHostPorts), len(actualHostPorts))
		}

		for i, expectedPort := range expectedHostPorts {
			if i >= len(actualHostPorts) {
				t.Errorf("missing expected host port %s", expectedPort)
				continue
			}
			if actualHostPorts[i] != expectedPort {
				t.Errorf("expected host port %s, got %s", expectedPort, actualHostPorts[i])
			}
		}
	})

	t.Run("InvalidHostPortFormat", func(t *testing.T) {
		// Given a TalosService with mock components
		mocks := setupTalosServiceMocks(t)

		// Reset package-level variables
		controlPlaneLeader = nil

		// Create a worker node
		service := NewTalosService(mocks.Injector, "worker")
		service.SetName("worker1")
		if err := service.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service: %v", err)
		}

		// And invalid host port format in config
		if err := mocks.ConfigHandler.Set("cluster.workers.hostports", []string{"invalid:format:extra"}); err != nil {
			t.Fatalf("Failed to set invalid host port format: %v", err)
		}

		// When SetAddress is called
		err := service.SetAddress("192.168.1.20", nil)

		// Then there should be an error
		if err == nil {
			t.Error("expected error for invalid host port format, got nil")
		}
		if !strings.Contains(err.Error(), "invalid hostPort format") {
			t.Errorf("expected error about invalid host port format, got %v", err)
		}
	})

	t.Run("InvalidProtocol", func(t *testing.T) {
		// Given a TalosService with mock components
		mocks := setupTalosServiceMocks(t)

		// Reset package-level variables
		controlPlaneLeader = nil

		// Create a worker node
		service := NewTalosService(mocks.Injector, "worker")
		service.SetName("worker1")
		if err := service.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service: %v", err)
		}

		// And invalid protocol in config
		if err := mocks.ConfigHandler.Set("cluster.workers.hostports", []string{"30000:30000/invalid"}); err != nil {
			t.Fatalf("Failed to set invalid protocol: %v", err)
		}

		// When SetAddress is called
		err := service.SetAddress("192.168.1.20", nil)

		// Then there should be an error
		if err == nil {
			t.Error("expected error for invalid protocol, got nil")
		}
		if !strings.Contains(err.Error(), "invalid protocol value") {
			t.Errorf("expected error about invalid protocol, got %v", err)
		}
	})

	t.Run("PortConflictResolution", func(t *testing.T) {
		// Given a TalosService with mock components
		mocks := setupTalosServiceMocks(t)

		// Reset package-level variables
		controlPlaneLeader = nil

		// Create first worker node
		service1 := NewTalosService(mocks.Injector, "worker")
		service1.SetName("worker1")
		if err := service1.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service1: %v", err)
		}

		// Set host ports for first worker
		if err := mocks.ConfigHandler.Set("cluster.workers.hostports", []string{"30000:30000"}); err != nil {
			t.Fatalf("Failed to set host ports: %v", err)
		}

		// When SetAddress is called for first worker
		portAllocator := NewPortAllocator()
		if err := service1.SetAddress("192.168.1.20", portAllocator); err != nil {
			t.Fatalf("Failed to set address for service1: %v", err)
		}

		// Create second worker node
		service2 := NewTalosService(mocks.Injector, "worker")
		service2.SetName("worker2")
		if err := service2.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service2: %v", err)
		}

		// Set same host ports for second worker
		if err := mocks.ConfigHandler.Set("cluster.workers.hostports", []string{"30000:30000"}); err != nil {
			t.Fatalf("Failed to set host ports: %v", err)
		}

		// When SetAddress is called for second worker
		if err := service2.SetAddress("192.168.1.21", portAllocator); err != nil {
			t.Fatalf("Failed to set address for service2: %v", err)
		}

		// Then the second worker should have the same host port (no conflict resolution)
		actualHostPorts := mocks.ConfigHandler.GetStringSlice("cluster.workers.nodes.worker2.hostports", []string{})
		if len(actualHostPorts) != 1 {
			t.Fatalf("expected 1 host port, got %d", len(actualHostPorts))
		}
		expectedHostPort := "30000:30000/tcp"
		if actualHostPorts[0] != expectedHostPort {
			t.Errorf("expected host port %s, got %s", expectedHostPort, actualHostPorts[0])
		}
	})

	t.Run("SuccessWithCustomTLD", func(t *testing.T) {
		// Given a TalosService with mock components
		mocks := setupTalosServiceMocks(t)

		// Reset package-level variables
		controlPlaneLeader = nil

		// And a custom TLD
		if err := mocks.ConfigHandler.Set("dns.domain", "custom.local"); err != nil {
			t.Fatalf("Failed to set custom TLD: %v", err)
		}

		// Create a worker node
		service := NewTalosService(mocks.Injector, "worker")
		service.SetName("worker1")
		if err := service.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service: %v", err)
		}

		// Enable localhost mode
		if err := mocks.ConfigHandler.Set("vm.driver", "docker-desktop"); err != nil {
			t.Fatalf("Failed to set VM driver: %v", err)
		}

		// When SetAddress is called
		portAllocator := NewPortAllocator()
		err := service.SetAddress("192.168.1.20", portAllocator)

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the endpoint should use the custom TLD
		expectedEndpoint := fmt.Sprintf("127.0.0.1:%d", constants.DefaultTalosAPIPort+1)
		actualEndpoint := mocks.ConfigHandler.GetString("cluster.workers.nodes.worker1.endpoint", "")
		if actualEndpoint != expectedEndpoint {
			t.Errorf("expected endpoint %s, got %s", expectedEndpoint, actualEndpoint)
		}

		// And the hostname should be set correctly
		expectedHostname := "worker1"
		actualHostname := mocks.ConfigHandler.GetString("cluster.workers.nodes.worker1.hostname", "")
		if actualHostname != expectedHostname {
			t.Errorf("expected hostname %s, got %s", expectedHostname, actualHostname)
		}
	})

	t.Run("InvalidHostPortNumber", func(t *testing.T) {
		// Given a TalosService with mock components
		mocks := setupTalosServiceMocks(t)

		// Reset package-level variables
		controlPlaneLeader = nil

		// Create a worker node
		service := NewTalosService(mocks.Injector, "worker")
		service.SetName("worker1")
		if err := service.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service: %v", err)
		}

		// And invalid host port format in config
		hostPorts := []string{
			"abc:30000", // Non-numeric host port
		}
		if err := mocks.ConfigHandler.Set("cluster.workers.hostports", hostPorts); err != nil {
			t.Fatalf("Failed to set host ports: %v", err)
		}

		// When SetAddress is called
		err := service.SetAddress("192.168.1.20", nil)

		// Then there should be an error
		if err == nil {
			t.Error("expected error for invalid host port number, got nil")
		}
		if !strings.Contains(err.Error(), "invalid hostPort value") {
			t.Errorf("expected error about invalid host port value, got %v", err)
		}

		// And with invalid node port format
		hostPorts = []string{
			"30000:xyz", // Non-numeric node port
		}
		if err := mocks.ConfigHandler.Set("cluster.workers.hostports", hostPorts); err != nil {
			t.Fatalf("Failed to set host ports: %v", err)
		}

		// When SetAddress is called
		err = service.SetAddress("192.168.1.20", nil)

		// Then there should be an error
		if err == nil {
			t.Error("expected error for invalid node port number, got nil")
		}
		if !strings.Contains(err.Error(), "invalid hostPort value") {
			t.Errorf("expected error about invalid host port value, got %v", err)
		}

		// And with single non-numeric port
		hostPorts = []string{
			"xyz", // Non-numeric single port
		}
		if err := mocks.ConfigHandler.Set("cluster.workers.hostports", hostPorts); err != nil {
			t.Fatalf("Failed to set host ports: %v", err)
		}

		// When SetAddress is called
		err = service.SetAddress("192.168.1.20", nil)

		// Then there should be an error
		if err == nil {
			t.Error("expected error for invalid single port number, got nil")
		}
		if !strings.Contains(err.Error(), "invalid hostPort value") {
			t.Errorf("expected error about invalid host port value, got %v", err)
		}
	})

	t.Run("MultiplePortConflictResolution", func(t *testing.T) {
		// Given a TalosService with mock components
		mocks := setupTalosServiceMocks(t)

		// Reset package-level variables
		controlPlaneLeader = nil

		// Create first worker node
		service1 := NewTalosService(mocks.Injector, "worker")
		service1.SetName("worker1")
		if err := service1.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service1: %v", err)
		}

		// Set multiple host ports for first worker
		hostPorts1 := []string{
			"30000:30000",
			"30001:30001",
			"30002:30002",
		}
		if err := mocks.ConfigHandler.Set("cluster.workers.hostports", hostPorts1); err != nil {
			t.Fatalf("Failed to set host ports: %v", err)
		}

		// When SetAddress is called for first worker
		portAllocator := NewPortAllocator()
		if err := service1.SetAddress("192.168.1.20", portAllocator); err != nil {
			t.Fatalf("Failed to set address for service1: %v", err)
		}

		// Create second worker node
		service2 := NewTalosService(mocks.Injector, "worker")
		service2.SetName("worker2")
		if err := service2.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service2: %v", err)
		}

		// Set overlapping host ports for second worker
		hostPorts2 := []string{
			"30001:30001", // Overlaps with first worker
			"30002:30002", // Overlaps with first worker
			"30003:30003", // New port
		}
		if err := mocks.ConfigHandler.Set("cluster.workers.hostports", hostPorts2); err != nil {
			t.Fatalf("Failed to set host ports: %v", err)
		}

		// When SetAddress is called for second worker
		if err := service2.SetAddress("192.168.1.21", portAllocator); err != nil {
			t.Fatalf("Failed to set address for service2: %v", err)
		}

		// Then the second worker should have the configured host ports (no conflict resolution)
		actualHostPorts := mocks.ConfigHandler.GetStringSlice("cluster.workers.nodes.worker2.hostports", []string{})
		expectedHostPorts := []string{
			"30001:30001/tcp",
			"30002:30002/tcp",
			"30003:30003/tcp",
		}

		if len(actualHostPorts) != len(expectedHostPorts) {
			t.Fatalf("expected %d host ports, got %d", len(expectedHostPorts), len(actualHostPorts))
		}

		for i, expectedPort := range expectedHostPorts {
			if actualHostPorts[i] != expectedPort {
				t.Errorf("expected host port %s, got %s", expectedPort, actualHostPorts[i])
			}
		}

		// Create third worker node
		service3 := NewTalosService(mocks.Injector, "worker")
		service3.SetName("worker3")
		if err := service3.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service3: %v", err)
		}

		// Set overlapping host ports for third worker
		hostPorts3 := []string{
			"30000:30000", // Overlaps with first worker
			"30003:30004", // Overlaps with second worker's incremented port
		}
		if err := mocks.ConfigHandler.Set("cluster.workers.hostports", hostPorts3); err != nil {
			t.Fatalf("Failed to set host ports: %v", err)
		}

		// When SetAddress is called for third worker
		if err := service3.SetAddress("192.168.1.22", nil); err != nil {
			t.Fatalf("Failed to set address for service3: %v", err)
		}

		// Then the third worker should have the configured host ports (no conflict resolution)
		actualHostPorts = mocks.ConfigHandler.GetStringSlice("cluster.workers.nodes.worker3.hostports", []string{})
		expectedHostPorts = []string{
			"30000:30000/tcp",
			"30003:30004/tcp",
		}

		if len(actualHostPorts) != len(expectedHostPorts) {
			t.Fatalf("expected %d host ports, got %d", len(expectedHostPorts), len(actualHostPorts))
		}

		for i, expectedPort := range expectedHostPorts {
			if actualHostPorts[i] != expectedPort {
				t.Errorf("expected host port %s, got %s", expectedPort, actualHostPorts[i])
			}
		}
	})

	t.Run("BaseServiceError", func(t *testing.T) {
		// Given a TalosService with mock components
		service, _ := setup(t)

		// When SetAddress is called with an invalid address
		err := service.SetAddress("", nil)

		// Then there should be an error
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("HostPortConflictResolution", func(t *testing.T) {
		// Given a TalosService with mock components
		mocks := setupTalosServiceMocks(t)

		// Reset package-level variables
		controlPlaneLeader = nil

		// Create a worker node with conflicting host ports
		service := NewTalosService(mocks.Injector, "worker")
		service.SetName("worker1")
		if err := service.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service: %v", err)
		}

		// Configure host ports with a conflict
		hostPorts := []string{
			"30000:30000",
			"30000:30001", // Intentional conflict with first port
		}
		if err := mocks.ConfigHandler.Set("cluster.workers.hostports", hostPorts); err != nil {
			t.Fatalf("Failed to set host ports: %v", err)
		}

		// When SetAddress is called
		portAllocator := NewPortAllocator()
		err := service.SetAddress("192.168.1.20", portAllocator)

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the host ports should be set as configured (no conflict resolution)
		actualHostPorts := mocks.ConfigHandler.GetStringSlice("cluster.workers.nodes.worker1.hostports", []string{})
		expectedHostPorts := []string{
			"30000:30000/tcp",
			"30000:30001/tcp",
		}

		if len(actualHostPorts) != len(expectedHostPorts) {
			t.Errorf("expected %d host ports, got %d", len(expectedHostPorts), len(actualHostPorts))
		}

		for i, expectedPort := range expectedHostPorts {
			if i >= len(actualHostPorts) {
				t.Errorf("missing expected host port %s", expectedPort)
				continue
			}
			if actualHostPorts[i] != expectedPort {
				t.Errorf("expected host port %s, got %s", expectedPort, actualHostPorts[i])
			}
		}
	})

	t.Run("InvalidProtocol", func(t *testing.T) {
		// Given a TalosService with mock components
		mocks := setupTalosServiceMocks(t)

		// Reset package-level variables
		controlPlaneLeader = nil

		// Create a worker node with invalid protocol
		service := NewTalosService(mocks.Injector, "worker")
		service.SetName("worker1")
		if err := service.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service: %v", err)
		}

		// Configure host ports with invalid protocol
		hostPorts := []string{
			"30000:30000/invalid",
		}
		if err := mocks.ConfigHandler.Set("cluster.workers.hostports", hostPorts); err != nil {
			t.Fatalf("Failed to set host ports: %v", err)
		}

		// When SetAddress is called
		err := service.SetAddress("192.168.1.20", nil)

		// Then there should be an error
		if err == nil {
			t.Error("expected error for invalid protocol, got nil")
		}
		if !strings.Contains(err.Error(), "invalid protocol value") {
			t.Errorf("expected error about invalid protocol, got: %v", err)
		}
	})

	t.Run("InvalidHostPortFormat", func(t *testing.T) {
		// Given a TalosService with mock components
		mocks := setupTalosServiceMocks(t)

		// Reset package-level variables
		controlPlaneLeader = nil

		// Create a worker node with invalid host port format
		service := NewTalosService(mocks.Injector, "worker")
		service.SetName("worker1")
		if err := service.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service: %v", err)
		}

		// Configure host ports with invalid format
		hostPorts := []string{
			"30000:30000:30000", // Too many colons
		}
		if err := mocks.ConfigHandler.Set("cluster.workers.hostports", hostPorts); err != nil {
			t.Fatalf("Failed to set host ports: %v", err)
		}

		// When SetAddress is called
		err := service.SetAddress("192.168.1.20", nil)

		// Then there should be an error
		if err == nil {
			t.Error("expected error for invalid host port format, got nil")
		}
		if !strings.Contains(err.Error(), "invalid hostPort format") {
			t.Errorf("expected error about invalid format, got: %v", err)
		}
	})

	t.Run("InvalidPortNumber", func(t *testing.T) {
		// Given a TalosService with mock components
		mocks := setupTalosServiceMocks(t)

		// Reset package-level variables
		controlPlaneLeader = nil

		// Create a worker node with invalid port number
		service := NewTalosService(mocks.Injector, "worker")
		service.SetName("worker1")
		if err := service.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service: %v", err)
		}

		// Configure host ports with invalid port number
		hostPorts := []string{
			"invalid:30000",
		}
		if err := mocks.ConfigHandler.Set("cluster.workers.hostports", hostPorts); err != nil {
			t.Fatalf("Failed to set host ports: %v", err)
		}

		// When SetAddress is called
		err := service.SetAddress("192.168.1.20", nil)

		// Then there should be an error
		if err == nil {
			t.Error("expected error for invalid port number, got nil")
		}
		if !strings.Contains(err.Error(), "invalid hostPort value") {
			t.Errorf("expected error about invalid port value, got: %v", err)
		}
	})

	t.Run("SinglePort", func(t *testing.T) {
		// Given a single port string
		hostPortStr := "30000"

		// When validateHostPort is called
		hostPort, nodePort, protocol, err := validateHostPort(hostPortStr)

		// Then there should be no error
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the ports should be set correctly
		if hostPort != 30000 {
			t.Errorf("expected hostPort 30000, got %d", hostPort)
		}
		if nodePort != 30000 {
			t.Errorf("expected nodePort 30000, got %d", nodePort)
		}
		if protocol != "tcp" {
			t.Errorf("expected protocol tcp, got %s", protocol)
		}
	})

	t.Run("PortExceedsUint32", func(t *testing.T) {
		// Given a port string exceeding uint32 max
		hostPortStr := fmt.Sprintf("%d", math.MaxUint32+1)

		// When validateHostPort is called
		hostPort, nodePort, protocol, err := validateHostPort(hostPortStr)

		// Then there should be an error
		if err == nil {
			t.Error("expected error for port exceeding uint32 max, got nil")
		}
		if !strings.Contains(err.Error(), "invalid hostPort value") {
			t.Errorf("expected error about invalid hostPort value, got %v", err)
		}

		// And the return values should be zero
		if hostPort != 0 {
			t.Errorf("expected hostPort 0, got %d", hostPort)
		}
		if nodePort != 0 {
			t.Errorf("expected nodePort 0, got %d", nodePort)
		}
		if protocol != "" {
			t.Errorf("expected empty protocol, got %s", protocol)
		}
	})
}

// TestTalosService_GetComposeConfig tests the GetComposeConfig method of TalosService
func TestTalosService_GetComposeConfig(t *testing.T) {
	setup := func(t *testing.T) (*TalosService, *Mocks) {
		t.Helper()

		// Reset package-level variables
		controlPlaneLeader = nil

		mocks := setupTalosServiceMocks(t)
		service := NewTalosService(mocks.Injector, "controlplane")
		service.shims = mocks.Shims
		service.SetName("controlplane1")
		if err := service.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service: %v", err)
		}

		// Mock MkdirAll to always succeed
		service.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		return service, mocks
	}

	setupWorker := func(t *testing.T) (*TalosService, *Mocks) {
		t.Helper()

		// Reset package-level variables
		controlPlaneLeader = nil

		mocks := setupTalosServiceMocks(t)
		service := NewTalosService(mocks.Injector, "worker")
		service.shims = mocks.Shims
		service.SetName("worker1")
		if err := service.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service: %v", err)
		}

		// Mock MkdirAll to always succeed
		service.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		return service, mocks
	}

	t.Run("SuccessControlPlane", func(t *testing.T) {
		// Given a TalosService with mock components
		service, _ := setup(t)

		// When GetComposeConfig is called
		config, err := service.GetComposeConfig()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the config should be correctly populated
		if len(config.Services) != 1 {
			t.Fatalf("expected 1 service, got %d", len(config.Services))
		}

		// And the service should have the correct configuration
		serviceConfig, exists := config.Services["controlplane1"]
		if !exists {
			t.Fatalf("expected service 'controlplane1' to exist in compose config")
		}
		if serviceConfig.Name != "controlplane1" {
			t.Errorf("expected service name controlplane1, got %s", serviceConfig.Name)
		}
		if serviceConfig.Image != constants.DefaultTalosImage {
			t.Errorf("expected image %s, got %s", constants.DefaultTalosImage, serviceConfig.Image)
		}
		if !serviceConfig.Privileged {
			t.Error("expected service to be privileged")
		}
		if !serviceConfig.ReadOnly {
			t.Error("expected service to be read-only")
		}
		if len(serviceConfig.SecurityOpt) != 1 || serviceConfig.SecurityOpt[0] != "seccomp=unconfined" {
			t.Errorf("expected security opt seccomp=unconfined, got %v", serviceConfig.SecurityOpt)
		}
		if len(serviceConfig.Tmpfs) != 3 {
			t.Errorf("expected 3 tmpfs mounts, got %d", len(serviceConfig.Tmpfs))
		}

		// And the service should have the correct environment variables
		if serviceConfig.Environment["PLATFORM"] == nil || *serviceConfig.Environment["PLATFORM"] != "container" {
			t.Error("expected PLATFORM=container environment variable")
		}
		if serviceConfig.Environment["TALOSSKU"] == nil {
			t.Error("expected TALOSSKU environment variable")
		}

		// And the service should have the correct volumes
		expectedVolumes := []string{
			"controlplane1_system_state:/system/state",
			"controlplane1_var:/var",
			"controlplane1_etc_cni:/etc/cni",
			"controlplane1_etc_kubernetes:/etc/kubernetes",
			"controlplane1_usr_libexec_kubernetes:/usr/libexec/kubernetes",
			"controlplane1_opt:/opt",
		}
		for _, expectedVolume := range expectedVolumes {
			found := false
			for _, volume := range serviceConfig.Volumes {
				if fmt.Sprintf("%s:%s", volume.Source, volume.Target) == expectedVolume {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected volume %s not found", expectedVolume)
			}
		}
	})

	t.Run("SuccessWorker", func(t *testing.T) {
		// Given a TalosService with mock components
		service, _ := setupWorker(t)

		// When GetComposeConfig is called
		config, err := service.GetComposeConfig()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the config should be correctly populated
		if len(config.Services) != 1 {
			t.Fatalf("expected 1 service, got %d", len(config.Services))
		}

		// And the service should have the correct configuration
		serviceConfig, exists := config.Services["worker1"]
		if !exists {
			t.Fatalf("expected service 'worker1' to exist in compose config")
		}
		if serviceConfig.Name != "worker1" {
			t.Errorf("expected service name worker1, got %s", serviceConfig.Name)
		}
		if serviceConfig.Image != constants.DefaultTalosImage {
			t.Errorf("expected image %s, got %s", constants.DefaultTalosImage, serviceConfig.Image)
		}

		// And the service should have worker-specific CPU and RAM settings
		if serviceConfig.Environment["TALOSSKU"] == nil {
			t.Error("expected TALOSSKU environment variable")
		} else {
			expectedSKU := fmt.Sprintf("%dCPU-%dRAM", constants.DefaultTalosWorkerCPU, constants.DefaultTalosWorkerRAM*1024)
			if *serviceConfig.Environment["TALOSSKU"] != expectedSKU {
				t.Errorf("expected TALOSSKU=%s, got %s", expectedSKU, *serviceConfig.Environment["TALOSSKU"])
			}
		}
	})

	t.Run("SuccessWithCustomImage", func(t *testing.T) {
		// Given a TalosService with mock components
		service, mocks := setup(t)

		// And a custom image is configured
		customImage := "custom/talos:latest"
		if err := mocks.ConfigHandler.Set("cluster.controlplanes.nodes.controlplane1.image", customImage); err != nil {
			t.Fatalf("Failed to set custom image: %v", err)
		}

		// When GetComposeConfig is called
		config, err := service.GetComposeConfig()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the config should use the custom image
		serviceConfig, exists := config.Services["controlplane1"]
		if !exists {
			t.Fatalf("expected service 'controlplane1' to exist in compose config")
		}
		if serviceConfig.Image != customImage {
			t.Errorf("expected image %s, got %s", customImage, serviceConfig.Image)
		}
	})

	t.Run("SuccessWithVolumes", func(t *testing.T) {
		// Given a TalosService with mock components
		service, mocks := setup(t)

		// And custom volumes are configured
		volumes := []string{
			"/data/controlplane1:/mnt/data",
			"/logs/controlplane1:/mnt/logs",
		}
		if err := mocks.ConfigHandler.Set("cluster.controlplanes.volumes", volumes); err != nil {
			t.Fatalf("Failed to set volumes: %v", err)
		}

		// When GetComposeConfig is called
		config, err := service.GetComposeConfig()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the config should have the custom volumes
		serviceConfig, exists := config.Services["controlplane1"]
		if !exists {
			t.Fatalf("expected service 'controlplane1' to exist in compose config")
		}
		for _, expectedVolume := range volumes {
			found := false
			for _, volume := range serviceConfig.Volumes {
				if volume.Type == "bind" && fmt.Sprintf("%s:%s", volume.Source, volume.Target) == expectedVolume {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected volume %s not found", expectedVolume)
			}
		}
	})

	t.Run("SuccessEmptyConfig", func(t *testing.T) {
		// Given a TalosService with mock components and empty cluster config
		emptyConfig := &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  mock-context:
    dns:
      domain: test
    vm:
      driver: docker-desktop
`,
		}
		mocks := setupTalosServiceMocks(t, emptyConfig)
		service := NewTalosService(mocks.Injector, "controlplane")
		service.shims = mocks.Shims
		service.SetName("controlplane1")
		if err := service.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service: %v", err)
		}

		// When GetComposeConfig is called
		config, err := service.GetComposeConfig()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the config should be empty
		if len(config.Services) != 0 {
			t.Errorf("expected 0 services, got %d", len(config.Services))
		}
		if len(config.Volumes) != 0 {
			t.Errorf("expected 0 volumes, got %d", len(config.Volumes))
		}
	})

	t.Run("EmptyConfig", func(t *testing.T) {
		// Given a TalosService with mock components and empty cluster config
		emptyConfig := &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  mock-context:
    dns:
      domain: test
    vm:
      driver: docker-desktop
`,
		}
		mocks := setupTalosServiceMocks(t, emptyConfig)
		service := NewTalosService(mocks.Injector, "controlplane")
		service.shims = mocks.Shims
		service.SetName("controlplane1")
		if err := service.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service: %v", err)
		}

		// When GetComposeConfig is called
		config, err := service.GetComposeConfig()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the config should be empty
		if len(config.Services) != 0 {
			t.Errorf("expected 0 services, got %d", len(config.Services))
		}
		if len(config.Volumes) != 0 {
			t.Errorf("expected 0 volumes, got %d", len(config.Volumes))
		}
	})

	t.Run("CustomImagePriority", func(t *testing.T) {
		// Given a TalosService with mock components
		service, mocks := setup(t)

		// And custom images at different levels
		if err := mocks.ConfigHandler.Set("cluster.image", "cluster-wide:latest"); err != nil {
			t.Fatalf("Failed to set cluster-wide image: %v", err)
		}
		if err := mocks.ConfigHandler.Set("cluster.controlplanes.image", "group-specific:latest"); err != nil {
			t.Fatalf("Failed to set group-specific image: %v", err)
		}
		if err := mocks.ConfigHandler.Set("cluster.controlplanes.nodes.controlplane1.image", "node-specific:latest"); err != nil {
			t.Fatalf("Failed to set node-specific image: %v", err)
		}

		// When GetComposeConfig is called
		config, err := service.GetComposeConfig()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the node-specific image should be used
		serviceConfig, exists := config.Services["controlplane1"]
		if !exists {
			t.Fatalf("expected service 'controlplane1' to exist in compose config")
		}
		if serviceConfig.Image != "node-specific:latest" {
			t.Errorf("expected node-specific image, got %s", serviceConfig.Image)
		}
	})

	t.Run("CustomVolumes", func(t *testing.T) {
		// Given a TalosService with mock components
		service, mocks := setup(t)

		// And custom volumes
		customVolumes := []string{
			"/data/controlplane1:/mnt/data",
			"/logs/controlplane1:/mnt/logs",
		}
		if err := mocks.ConfigHandler.Set("cluster.controlplanes.volumes", customVolumes); err != nil {
			t.Fatalf("Failed to set custom volumes: %v", err)
		}

		// When GetComposeConfig is called
		config, err := service.GetComposeConfig()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the custom volumes should be included
		serviceConfig, exists := config.Services["controlplane1"]
		if !exists {
			t.Fatalf("expected service 'controlplane1' to exist in compose config")
		}
		for _, expectedVolume := range customVolumes {
			found := false
			for _, volume := range serviceConfig.Volumes {
				if volume.Type == "bind" && fmt.Sprintf("%s:%s", volume.Source, volume.Target) == expectedVolume {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected volume %s not found", expectedVolume)
			}
		}
	})

	t.Run("InvalidVolumeFormat", func(t *testing.T) {
		// Given a TalosService with mock components
		service, mocks := setup(t)

		// And invalid volume format in config
		if err := mocks.ConfigHandler.Set("cluster.controlplanes.volumes", []string{"invalid:format:extra"}); err != nil {
			t.Fatalf("Failed to set invalid volume format: %v", err)
		}

		// When GetComposeConfig is called
		_, err := service.GetComposeConfig()

		// Then there should be an error
		if err == nil {
			t.Error("expected error for invalid volume format, got nil")
		}
		if !strings.Contains(err.Error(), "invalid volume format") {
			t.Errorf("expected error about invalid volume format, got %v", err)
		}
	})

	t.Run("SuccessWithDNS", func(t *testing.T) {
		// Given a TalosService with mock components
		mocks := setupTalosServiceMocks(t)

		// And DNS configuration is set
		mocks.ConfigHandler.Set("dns.domain", "test")
		mocks.ConfigHandler.Set("dns.address", "192.168.1.1")

		// Create a worker node
		service := NewTalosService(mocks.Injector, "worker")
		service.SetName("worker1")
		if err := service.Initialize(); err != nil {
			t.Fatalf("Failed to initialize service: %v", err)
		}

		// Mock MkdirAll to always succeed
		service.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		// When GetComposeConfig is called
		cfg, err := service.GetComposeConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the config should be valid
		if cfg == nil {
			t.Fatalf("expected non-nil config, got nil")
		}

		// And the service should be configured correctly
		if len(cfg.Services) != 1 {
			t.Fatalf("expected 1 service, got %d", len(cfg.Services))
		}

		// And the service should have the correct name
		serviceConfig, exists := cfg.Services["worker1"]
		if !exists {
			t.Fatalf("expected service 'worker1' to exist in compose config")
		}
		if serviceConfig.Name != "worker1" {
			t.Errorf("expected service name 'worker1', got '%s'", serviceConfig.Name)
		}
	})

	t.Run("SuccessWithCustomPorts", func(t *testing.T) {
		// Given a TalosService with mock components
		service, mocks := setup(t)

		// And localhost mode is enabled
		if err := mocks.ConfigHandler.Set("vm.driver", "docker-desktop"); err != nil {
			t.Fatalf("Failed to set VM driver: %v", err)
		}

		// And custom host ports
		hostPorts := []string{
			"30000:30000/tcp",
			"30001:30001/udp",
		}
		if err := mocks.ConfigHandler.Set("cluster.controlplanes.nodes.controlplane1.hostports", hostPorts); err != nil {
			t.Fatalf("Failed to set host ports: %v", err)
		}

		// When GetComposeConfig is called
		config, err := service.GetComposeConfig()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the ports should be configured correctly
		serviceConfig, exists := config.Services["controlplane1"]
		if !exists {
			t.Fatalf("expected service 'controlplane1' to exist in compose config")
		}
		if len(serviceConfig.Ports) != 4 { // 2 custom ports + default API port + kubernetes port
			t.Errorf("expected 4 ports, got %d", len(serviceConfig.Ports))
		}

		// Check default API port
		if serviceConfig.Ports[0].Target != uint32(constants.DefaultTalosAPIPort) {
			t.Errorf("expected target port %d, got %d", constants.DefaultTalosAPIPort, serviceConfig.Ports[0].Target)
		}
		if serviceConfig.Ports[0].Published != fmt.Sprintf("%d", constants.DefaultTalosAPIPort) {
			t.Errorf("expected published port %d, got %s", constants.DefaultTalosAPIPort, serviceConfig.Ports[0].Published)
		}
		if serviceConfig.Ports[0].Protocol != "tcp" {
			t.Errorf("expected protocol tcp, got %s", serviceConfig.Ports[0].Protocol)
		}

		// Check kubernetes port
		if serviceConfig.Ports[1].Target != 6443 {
			t.Errorf("expected target port 6443, got %d", serviceConfig.Ports[1].Target)
		}
		if serviceConfig.Ports[1].Published != "6443" {
			t.Errorf("expected published port 6443, got %s", serviceConfig.Ports[1].Published)
		}
		if serviceConfig.Ports[1].Protocol != "tcp" {
			t.Errorf("expected protocol tcp, got %s", serviceConfig.Ports[1].Protocol)
		}

		// Check first custom port
		if serviceConfig.Ports[2].Target != 30000 {
			t.Errorf("expected target port 30000, got %d", serviceConfig.Ports[2].Target)
		}
		if serviceConfig.Ports[2].Published != "30000" {
			t.Errorf("expected published port 30000, got %s", serviceConfig.Ports[2].Published)
		}
		if serviceConfig.Ports[2].Protocol != "tcp" {
			t.Errorf("expected protocol tcp, got %s", serviceConfig.Ports[2].Protocol)
		}

		// Check second custom port
		if serviceConfig.Ports[3].Target != 30001 {
			t.Errorf("expected target port 30001, got %d", serviceConfig.Ports[3].Target)
		}
		if serviceConfig.Ports[3].Published != "30001" {
			t.Errorf("expected published port 30001, got %s", serviceConfig.Ports[3].Published)
		}
		if serviceConfig.Ports[3].Protocol != "udp" {
			t.Errorf("expected protocol udp, got %s", serviceConfig.Ports[3].Protocol)
		}
	})

	t.Run("InvalidPortRange", func(t *testing.T) {
		// Given a TalosService with mock components
		service, mocks := setup(t)

		// And localhost mode is enabled
		if err := mocks.ConfigHandler.Set("vm.driver", "docker-desktop"); err != nil {
			t.Fatalf("Failed to set VM driver: %v", err)
		}

		// And an invalid port range
		hostPorts := []string{
			fmt.Sprintf("%d:30000/tcp", math.MaxUint32+1), // Port too large
		}
		if err := mocks.ConfigHandler.Set("cluster.controlplanes.nodes.controlplane1.hostports", hostPorts); err != nil {
			t.Fatalf("Failed to set host ports: %v", err)
		}

		// When GetComposeConfig is called
		_, err := service.GetComposeConfig()

		// Then there should be an error
		if err == nil {
			t.Error("expected error for invalid port range, got nil")
		}
		if !strings.Contains(err.Error(), "invalid hostPort value") {
			t.Errorf("expected error about invalid hostPort value, got %v", err)
		}
	})

	t.Run("InvalidDefaultAPIPort", func(t *testing.T) {
		// Given a TalosService with mock components
		service, mocks := setup(t)

		// And localhost mode is enabled
		if err := mocks.ConfigHandler.Set("vm.driver", "docker-desktop"); err != nil {
			t.Fatalf("Failed to set VM driver: %v", err)
		}

		// When GetComposeConfig is called
		_, err := service.GetComposeConfig()

		// Then there should be no error (defaultAPIPort is now a const, so this test is no longer applicable)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("SuccessWithEnvVarVolumes", func(t *testing.T) {
		// Given a TalosService with mock components
		service, mocks := setup(t)

		// And environment variables are set
		os.Setenv("TEST_DATA_DIR", "/test/data")
		defer os.Unsetenv("TEST_DATA_DIR")

		// And volumes with environment variables
		volumes := []string{
			"${TEST_DATA_DIR}/controlplane1:/mnt/data",
			"/logs/controlplane1:/mnt/logs",
		}
		if err := mocks.ConfigHandler.Set("cluster.controlplanes.volumes", volumes); err != nil {
			t.Fatalf("Failed to set volumes: %v", err)
		}

		// Mock MkdirAll to verify expanded paths
		var expandedPaths []string
		service.shims.MkdirAll = func(path string, perm os.FileMode) error {
			expandedPaths = append(expandedPaths, path)
			return nil
		}

		// When GetComposeConfig is called
		config, err := service.GetComposeConfig()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the directories should be created with expanded paths
		expectedPaths := []string{
			"/test/data/controlplane1",
			"/logs/controlplane1",
		}
		if len(expandedPaths) != len(expectedPaths) {
			t.Errorf("expected %d paths, got %d", len(expectedPaths), len(expandedPaths))
		}
		for i, expectedPath := range expectedPaths {
			if i >= len(expandedPaths) {
				t.Errorf("missing expected path %s", expectedPath)
				continue
			}
			if expandedPaths[i] != expectedPath {
				t.Errorf("expected expanded path %s, got %s", expectedPath, expandedPaths[i])
			}
		}

		// And the volume config should use the original paths with variables
		serviceConfig, exists := config.Services["controlplane1"]
		if !exists {
			t.Fatalf("expected service 'controlplane1' to exist in compose config")
		}
		for _, expectedVolume := range volumes {
			found := false
			for _, volume := range serviceConfig.Volumes {
				if volume.Type == "bind" {
					parts := strings.Split(expectedVolume, ":")
					if volume.Source == parts[0] && volume.Target == parts[1] {
						found = true
						break
					}
				}
			}
			if !found {
				t.Errorf("volume %s not found in config", expectedVolume)
			}
		}
	})

	t.Run("SuccessWithCustomResources", func(t *testing.T) {
		// Given a TalosService with mock components for control plane
		service, mocks := setup(t)

		// And custom CPU and RAM settings
		customCPU := 4
		customRAM := 8
		if err := mocks.ConfigHandler.Set("cluster.controlplanes.cpu", customCPU); err != nil {
			t.Fatalf("Failed to set control plane CPU: %v", err)
		}
		if err := mocks.ConfigHandler.Set("cluster.controlplanes.memory", customRAM); err != nil {
			t.Fatalf("Failed to set control plane RAM: %v", err)
		}

		// When GetComposeConfig is called
		config, err := service.GetComposeConfig()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the control plane should have the custom CPU and RAM settings
		serviceConfig, exists := config.Services["controlplane1"]
		if !exists {
			t.Fatalf("expected service 'controlplane1' to exist in compose config")
		}
		expectedSKU := fmt.Sprintf("%dCPU-%dRAM", customCPU, customRAM*1024)
		if serviceConfig.Environment["TALOSSKU"] == nil {
			t.Error("expected TALOSSKU environment variable")
		} else if *serviceConfig.Environment["TALOSSKU"] != expectedSKU {
			t.Errorf("expected TALOSSKU=%s, got %s", expectedSKU, *serviceConfig.Environment["TALOSSKU"])
		}

		// Given a TalosService with mock components for worker
		service, mocks = setupWorker(t)

		// And custom CPU and RAM settings
		customWorkerCPU := 2
		customWorkerRAM := 4
		if err := mocks.ConfigHandler.Set("cluster.workers.cpu", customWorkerCPU); err != nil {
			t.Fatalf("Failed to set worker CPU: %v", err)
		}
		if err := mocks.ConfigHandler.Set("cluster.workers.memory", customWorkerRAM); err != nil {
			t.Fatalf("Failed to set worker RAM: %v", err)
		}

		// When GetComposeConfig is called
		config, err = service.GetComposeConfig()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the worker should have the custom CPU and RAM settings
		serviceConfig, exists = config.Services["worker1"]
		if !exists {
			t.Fatalf("expected service 'worker1' to exist in compose config")
		}
		expectedSKU = fmt.Sprintf("%dCPU-%dRAM", customWorkerCPU, customWorkerRAM*1024)
		if serviceConfig.Environment["TALOSSKU"] == nil {
			t.Error("expected TALOSSKU environment variable")
		} else if *serviceConfig.Environment["TALOSSKU"] != expectedSKU {
			t.Errorf("expected TALOSSKU=%s, got %s", expectedSKU, *serviceConfig.Environment["TALOSSKU"])
		}
	})

	t.Run("FailedDirectoryCreation", func(t *testing.T) {
		// Given a TalosService with mock components
		service, mocks := setup(t)

		// And custom volumes are configured
		volumes := []string{
			"/data/controlplane1:/mnt/data",
		}
		if err := mocks.ConfigHandler.Set("cluster.controlplanes.volumes", volumes); err != nil {
			t.Fatalf("Failed to set volumes: %v", err)
		}

		// And MkdirAll is mocked to fail
		service.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("failed to create directory")
		}

		// When GetComposeConfig is called
		_, err := service.GetComposeConfig()

		// Then there should be an error
		if err == nil {
			t.Error("expected error for failed directory creation, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create directory") {
			t.Errorf("expected error about failed directory creation, got %v", err)
		}
	})

	t.Run("EmptyServiceName", func(t *testing.T) {
		// Given a TalosService with mock components
		service, _ := setup(t)

		// And the service name is not set
		service.SetName("")

		// When GetComposeConfig is called
		config, err := service.GetComposeConfig()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the service name should fall back to "controlplane"
		serviceConfig, exists := config.Services["controlplane"]
		if !exists {
			t.Fatalf("expected service 'controlplane' to exist in compose config")
		}
		if serviceConfig.Name != "controlplane" {
			t.Errorf("expected service name 'controlplane', got %s", serviceConfig.Name)
		}

		// And the container name should use the fallback name with context prefix
		expectedContainerName := "controlplane.test"
		if serviceConfig.ContainerName != expectedContainerName {
			t.Errorf("expected container name %s, got %s", expectedContainerName, serviceConfig.ContainerName)
		}

		// And the volumes should use the fallback name
		expectedVolumeName := "controlplane_system_state"
		found := false
		for _, volume := range serviceConfig.Volumes {
			if volume.Source == expectedVolumeName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected volume with source %s not found", expectedVolumeName)
		}
	})

	t.Run("DuplicateDNSAddress", func(t *testing.T) {
		// Given a TalosService with mock components
		service, mocks := setup(t)

		// And DNS configuration
		if err := mocks.ConfigHandler.Set("dns.address", "8.8.8.8"); err != nil {
			t.Fatalf("Failed to set DNS address: %v", err)
		}

		// And the DNS address is already in the list
		service.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		config, err := service.GetComposeConfig()
		if err != nil {
			t.Fatalf("Failed first GetComposeConfig: %v", err)
		}

		// When GetComposeConfig is called again
		config, err = service.GetComposeConfig()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the service should be configured correctly
		if len(config.Services) != 1 {
			t.Fatalf("expected 1 service, got %d", len(config.Services))
		}
	})

	t.Run("InvalidPortValue", func(t *testing.T) {
		// Given a TalosService with mock components
		service, mocks := setup(t)

		// And an invalid port value in config
		if err := mocks.ConfigHandler.Set("cluster.controlplanes.nodes.controlplane1.endpoint", "controlplane1.test:invalid"); err != nil {
			t.Fatalf("Failed to set invalid port value: %v", err)
		}

		// When GetComposeConfig is called
		_, err := service.GetComposeConfig()

		// Then there should be an error
		if err == nil {
			t.Error("expected error for invalid port value, got nil")
		}
		if !strings.Contains(err.Error(), "invalid port value") {
			t.Errorf("expected error about invalid port value, got %v", err)
		}
	})
}
