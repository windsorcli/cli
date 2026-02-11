// The incus_virt_test package is a test suite for the IncusVirt implementation
// It provides test coverage for Incus container management functionality
// It serves as a verification framework for Incus virtualization operations
// It enables testing of Incus-specific features and error handling

package virt

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/workstation/services"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupIncusMocks(t *testing.T, opts ...func(*IncusTestMocks)) *IncusTestMocks {
	t.Helper()

	mocks := setupVirtMocks(t)

	instanceRunning := false
	deviceExists := false

	// Use MockShell directly, just like ColimaVirt tests do
	// Mock ExecSilentWithTimeout for getInstances() which now uses timeout
	mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
		// Handle colima ls for getVMInfo - colima returns array but code expects single object
		// Return just the JSON object (first element of array)
		if command == "colima" && len(args) >= 3 && args[0] == "ls" && args[1] == "--profile" {
			return `{"name":"windsor-mock-context","status":"Running","address":"192.168.1.100","arch":"x86_64","cpus":2,"disk":60,"memory":2147483648,"runtime":"incus"}`, nil
		}
		// Handle colima ssh commands that wrap incus commands
		if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
			actualCmd := args[6]
			if strings.Contains(actualCmd, "incus list --format json") {
				status := "Stopped"
				statusCode := 102
				if instanceRunning {
					status = "Running"
					statusCode = 103
				}
				devices := "{}"
				if deviceExists {
					devices = `{"eth0":{}}`
				}
				return fmt.Sprintf(`[{"name":"test-service","status":"%s","status_code":%d,"type":"container","expanded_devices":%s}]`, status, statusCode, devices), nil
			}
			if strings.Contains(actualCmd, "incus operation list --format json") {
				return `[]`, nil
			}
			if strings.Contains(actualCmd, "incus remote list --format json") {
				return `{"docker":{"name":"docker","url":"https://docker.io","protocol":"oci","public":true},"ghcr":{"name":"ghcr","url":"https://ghcr.io","protocol":"oci","public":true}}`, nil
			}
			if strings.Contains(actualCmd, "incus config device get") {
				return "10.0.0.10", nil
			}
			if strings.Contains(actualCmd, "incus config device add") {
				deviceExists = true
				return "", nil
			}
			if strings.Contains(actualCmd, "test -e") {
				return "", nil
			}
			if strings.Contains(actualCmd, "mkdir -p") {
				return "", nil
			}
		}
		// Direct incus commands (for backward compatibility)
		if command == "incus" && len(args) >= 3 && args[0] == "list" && args[1] == "--format" && args[2] == "json" {
			status := "Stopped"
			statusCode := 102
			if instanceRunning {
				status = "Running"
				statusCode = 103
			}
			devices := "{}"
			if deviceExists {
				devices = `{"eth0":{}}`
			}
			return fmt.Sprintf(`[{"name":"test-service","status":"%s","status_code":%d,"type":"container","expanded_devices":%s}]`, status, statusCode, devices), nil
		}
		// Fall back to ExecSilent for other commands
		if mocks.Shell.ExecSilentFunc != nil {
			return mocks.Shell.ExecSilentFunc(command, args...)
		}
		return "", fmt.Errorf("unexpected command: %s %v", command, args)
	}

	mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
		if command == "colima" && len(args) >= 2 && args[0] == "delete" {
			return "", nil
		}
		if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
			actualCmd := args[6]
			if strings.Contains(actualCmd, "incus") {
				return "", nil
			}
		}
		if command == "incus" {
			switch args[0] {
			case "operation":
				if len(args) >= 2 && args[1] == "list" {
					return `[]`, nil
				}
			case "remote":
				if len(args) >= 2 && args[1] == "list" {
					return `{"docker":{"name":"docker","url":"https://docker.io","protocol":"oci","public":true},"ghcr":{"name":"ghcr","url":"https://ghcr.io","protocol":"oci","public":true}}`, nil
				}
			case "config":
				if len(args) >= 4 && args[1] == "device" {
					if args[2] == "get" {
						return "10.0.0.10", nil
					}
					if args[2] == "add" && strings.Contains(strings.Join(args, " "), "eth0") {
						deviceExists = true
					}
					return "", nil
				}
			case "stop":
				instanceRunning = false
				return "", nil
			case "start":
				instanceRunning = true
				return "", nil
			case "delete":
				if len(args) >= 2 && args[1] == "--force" {
					return "", nil
				}
				return "", nil
			case "test":
				if len(args) >= 1 && args[0] == "-e" {
					return "", nil
				}
			case "mkdir":
				if len(args) >= 1 && args[0] == "-p" {
					return "", nil
				}
			}
		}
		return "", fmt.Errorf("unexpected command: %s %v", command, args)
	}

	mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
		if command == "colima" && len(args) >= 2 && args[0] == "start" {
			return "", nil
		}
		if command == "incus" {
			switch args[0] {
			case "remote":
				if len(args) >= 2 && args[1] == "add" {
					return "", nil
				}
			case "launch":
				return "", nil
			}
		}
		return "", fmt.Errorf("unexpected command: %s %v", command, args)
	}

	mockVM := NewMockVirt()

	mockService := services.NewMockService()
	mockService.GetNameFunc = func() string {
		return "test-service"
	}
	mockService.GetHostnameFunc = func() string {
		return "test-service.mock.domain.com"
	}
	mockService.GetAddressFunc = func() string {
		return "10.0.0.10"
	}
	mockService.GetIncusConfigFunc = func() (*services.IncusConfig, error) {
		return &services.IncusConfig{
			Type:     "container",
			Image:    "docker:alpine:latest",
			Config:   map[string]string{"user.hostname": "test-service.mock.domain.com"},
			Devices:  map[string]map[string]string{},
			Profiles: []string{"default"},
		}, nil
	}

	configStr := `
contexts:
  mock-context:
    provider: incus
    vm:
      runtime: incus
    network:
      name: incusbr0
      cidr_block: 10.0.0.0/24
    dns:
      domain: mock.domain.com
      enabled: true
      address: 10.0.0.53`

	if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
		t.Fatalf("Failed to load config string: %v", err)
	}

	incusMocks := &IncusTestMocks{
		VirtTestMocks: mocks,
		VM:            mockVM,
		Service:       mockService,
	}

	for _, opt := range opts {
		opt(incusMocks)
	}

	return incusMocks
}

type IncusTestMocks struct {
	*VirtTestMocks
	VM      *MockVirt
	Service *services.MockService
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewIncusVirt(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{mocks.Service})
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given valid runtime, services, VM, and SSH client
		incusVirt, _ := setup(t)

		// Then IncusVirt should be created
		if incusVirt == nil {
			t.Fatal("Expected IncusVirt, got nil")
		}
		if incusVirt.ColimaVirt == nil {
			t.Error("Expected ColimaVirt to be set")
		}
		if len(incusVirt.services) != 1 {
			t.Errorf("Expected 1 service, got %d", len(incusVirt.services))
		}
	})

	t.Run("NilServiceList", func(t *testing.T) {
		// Given nil service list
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, nil)

		// Then services should be empty
		if len(incusVirt.services) != 0 {
			t.Errorf("Expected 0 services, got %d", len(incusVirt.services))
		}
	})

	t.Run("NilServiceInList", func(t *testing.T) {
		// Given service list with nil service
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{mocks.Service, nil})

		// Then nil service should be filtered out
		if len(incusVirt.services) != 1 {
			t.Errorf("Expected 1 service, got %d", len(incusVirt.services))
		}
	})

	t.Run("ServicesSorted", func(t *testing.T) {
		// Given multiple services
		mocks := setupIncusMocks(t)
		serviceA := services.NewMockService()
		serviceA.SetName("service-a")
		serviceB := services.NewMockService()
		serviceB.SetName("service-b")
		serviceC := services.NewMockService()
		serviceC.SetName("service-c")

		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{serviceC, serviceA, serviceB})

		// Then services should be sorted by name
		if len(incusVirt.services) != 3 {
			t.Fatalf("Expected 3 services, got %d", len(incusVirt.services))
		}
		if incusVirt.services[0].GetName() != "service-a" {
			t.Errorf("Expected first service to be service-a, got %s", incusVirt.services[0].GetName())
		}
		if incusVirt.services[1].GetName() != "service-b" {
			t.Errorf("Expected second service to be service-b, got %s", incusVirt.services[1].GetName())
		}
		if incusVirt.services[2].GetName() != "service-c" {
			t.Errorf("Expected third service to be service-c, got %s", incusVirt.services[2].GetName())
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestIncusVirt_Up(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{mocks.Service})
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given an IncusVirt with valid mocks
		incusVirt, _ := setup(t)

		// When calling Up
		err := incusVirt.Up()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("NotIncusRuntime", func(t *testing.T) {
		// Given an IncusVirt with docker provider (not incus)
		incusVirt, mocks := setup(t)
		if err := mocks.ConfigHandler.Set("provider", "docker"); err != nil {
			t.Fatalf("Failed to set provider: %v", err)
		}

		// When calling Up
		err := incusVirt.Up()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorEnsureRemote", func(t *testing.T) {
		// Given an IncusVirt with remote check error
		incusVirt, mocks := setup(t)
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus remote list --format json") {
					return "", fmt.Errorf("remote list error")
				}
			}
			if originalExecSilentWithTimeout != nil {
				return originalExecSilentWithTimeout(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When calling Up
		err := incusVirt.Up()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to ensure") {
			t.Errorf("Expected error about ensuring remote, got %v", err)
		}
	})

	t.Run("ErrorGetIncusConfig", func(t *testing.T) {
		// Given an IncusVirt with service that errors on GetIncusConfig
		incusVirt, mocks := setup(t)
		mocks.Service.GetIncusConfigFunc = func() (*services.IncusConfig, error) {
			return nil, fmt.Errorf("get incus config error")
		}

		// When calling Up
		err := incusVirt.Up()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get Incus config") {
			t.Errorf("Expected error about getting Incus config, got %v", err)
		}
	})

	t.Run("NilIncusConfig", func(t *testing.T) {
		// Given an IncusVirt with service that returns nil IncusConfig
		incusVirt, mocks := setup(t)
		mocks.Service.GetIncusConfigFunc = func() (*services.IncusConfig, error) {
			return nil, nil
		}

		// When calling Up
		err := incusVirt.Up()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorCreateInstance", func(t *testing.T) {
		// Given an IncusVirt with instance creation error
		incusVirt, mocks := setup(t)
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					return `[]`, nil
				}
				if strings.Contains(actualCmd, "incus operation list --format json") {
					return `[]`, nil
				}
				if strings.Contains(actualCmd, "incus remote list --format json") {
					return `{"docker":{"name":"docker","url":"https://docker.io","protocol":"oci","public":true},"ghcr":{"name":"ghcr","url":"https://ghcr.io","protocol":"oci","public":true}}`, nil
				}
			}
			if originalExecSilentWithTimeout != nil {
				return originalExecSilentWithTimeout(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}
		launchCallCount := 0
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "colima" && len(args) >= 2 && args[0] == "start" {
				return "", nil
			}
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus launch") {
					launchCallCount++
					return "", fmt.Errorf("launch error")
				}
				if strings.Contains(actualCmd, "incus remote add") {
					return "", nil
				}
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}
		originalExecSilent := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus launch") {
					return "", fmt.Errorf("launch error")
				}
			}
			if originalExecSilent != nil {
				return originalExecSilent(command, args...)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When calling Up
		err := incusVirt.Up()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create Incus instance") {
			t.Errorf("Expected error about creating instance, got %v", err)
		}
	})

	t.Run("ErrorValidateVMForIncusNoAddress", func(t *testing.T) {
		// Given an IncusVirt with provider incus and VM running but no IP address
		incusVirt, mocks := setup(t)
		if err := mocks.ConfigHandler.Set("provider", "incus"); err != nil {
			t.Fatalf("Failed to set provider: %v", err)
		}

		// And service returns a valid IncusConfig so instance creation is attempted
		// This ensures startIncusContainers runs validation
		mocks.Service.GetIncusConfigFunc = func() (*services.IncusConfig, error) {
			return &services.IncusConfig{
				Type:  "container",
				Image: "docker:alpine:latest",
			}, nil
		}

		// And getVMInfo returns VM info with address initially (so VM is considered running),
		// but then returns empty address when called from startIncusContainers
		callCount := 0
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			// Handle colima ls for getVMInfo
			if command == "colima" && len(args) >= 3 && args[0] == "ls" && args[1] == "--profile" {
				callCount++
				if callCount == 1 {
					// First call from Up() - return VM with address so it's considered running
					return `{"name":"windsor-mock-context","status":"Running","address":"192.168.1.2","arch":"x86_64","cpus":2,"disk":60,"memory":2147483648,"runtime":"incus"}`, nil
				}
				// Second call from startIncusContainers - return VM without address to trigger validation error
				return `{"name":"windsor-mock-context","status":"Running","address":"","arch":"x86_64","cpus":2,"disk":60,"memory":2147483648,"runtime":"incus"}`, nil
			}
			// Handle remote list
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus remote list --format json") {
					return `{"docker":{"name":"docker","url":"https://docker.io","protocol":"oci","public":true},"ghcr":{"name":"ghcr","url":"https://ghcr.io","protocol":"oci","public":true}}`, nil
				}
			}
			if originalExecSilentWithTimeout != nil {
				return originalExecSilentWithTimeout(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When calling Up
		err := incusVirt.Up()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "VM is running but has no IP address") {
			t.Errorf("Expected error about missing IP address, got %v", err)
		}
		if !strings.Contains(err.Error(), "windsor down --clean") {
			t.Errorf("Expected error to contain troubleshooting message, got %v", err)
		}
	})
}

func TestIncusVirt_Down(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{mocks.Service})
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given an IncusVirt with valid mocks
		incusVirt, mocks := setup(t)
		// Mock ExecSilentWithTimeout for instanceExists check
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					// Instance exists
					return `[{"name":"test-service","status":"Running","status_code":103,"type":"container","expanded_devices":{}}]`, nil
				}
			}
			if originalExecSilentWithTimeout != nil {
				return originalExecSilentWithTimeout(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "incus" && len(args) >= 2 && args[0] == "delete" && args[1] == "test-service" {
				return "", nil
			}
			if command == "colima" && len(args) >= 1 && args[0] == "stop" {
				return "", nil
			}
			if command == "colima" && len(args) >= 1 && args[0] == "delete" {
				return "", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When calling Down
		err := incusVirt.Down()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("NotIncusRuntime", func(t *testing.T) {
		// Given an IncusVirt with docker provider (not incus)
		incusVirt, mocks := setup(t)
		if err := mocks.ConfigHandler.Set("provider", "docker"); err != nil {
			t.Fatalf("Failed to set provider: %v", err)
		}
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "colima" && len(args) >= 1 && args[0] == "stop" {
				return "", nil
			}
			if command == "colima" && len(args) >= 1 && args[0] == "delete" {
				return "", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When calling Down
		err := incusVirt.Down()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorDeleteInstance", func(t *testing.T) {
		// Given an IncusVirt with delete error
		incusVirt, mocks := setup(t)
		// Mock ExecSilentWithTimeout for instanceExists check
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "incus" && len(args) >= 3 && args[0] == "list" && args[1] == "--format" && args[2] == "json" {
				// Instance exists
				return `[{"name":"test-service","status":"Running","status_code":103,"type":"container","expanded_devices":{}}]`, nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "incus" && len(args) > 0 && args[0] == "delete" {
				return "", fmt.Errorf("delete error")
			}
			if command == "colima" && len(args) >= 1 && args[0] == "stop" {
				return "", nil
			}
			if command == "colima" && len(args) >= 1 && args[0] == "delete" {
				return "", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When calling Down
		err := incusVirt.Down()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to delete Incus instance") {
			t.Errorf("Expected error about deleting instance, got %v", err)
		}
	})

	t.Run("StopsColimaDaemonWithCorrectProfile", func(t *testing.T) {
		incusVirt, mocks := setup(t)
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					return `[{"name":"test-service","status":"Running","status_code":103,"type":"container","expanded_devices":{}}]`, nil
				}
			}
			if originalExecSilentWithTimeout != nil {
				return originalExecSilentWithTimeout(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		daemonStopCalled := false
		daemonStopProfile := ""
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "colima" && len(args) >= 3 && args[0] == "daemon" && args[1] == "stop" {
				daemonStopCalled = true
				daemonStopProfile = args[2]
				return "", nil
			}
			if command == "incus" && len(args) >= 2 && args[0] == "delete" {
				return "", nil
			}
			if command == "colima" && len(args) >= 1 && args[0] == "stop" {
				return "", nil
			}
			if command == "colima" && len(args) >= 1 && args[0] == "delete" {
				return "", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		err := incusVirt.Down()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !daemonStopCalled {
			t.Error("Expected colima daemon stop to be called")
		}
		expectedProfile := "windsor-mock-context"
		if daemonStopProfile != expectedProfile {
			t.Errorf("Expected daemon stop profile %q, got %q", expectedProfile, daemonStopProfile)
		}
	})
}

func TestIncusVirt_WriteConfig(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{mocks.Service})
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given an IncusVirt
		incusVirt, _ := setup(t)

		// When calling WriteConfig
		err := incusVirt.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}
