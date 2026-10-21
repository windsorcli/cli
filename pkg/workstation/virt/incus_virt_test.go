package virt

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Test Setup
// =============================================================================

type IncusTestMocks struct {
	*VirtTestMocks
	VM *MockVirt
}

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

	configStr := `
contexts:
  mock-context:
    platform: incus
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
	}

	for _, opt := range opts {
		opt(incusMocks)
	}

	return incusMocks
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewIncusVirt(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
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
	})

	t.Run("NilServiceList", func(t *testing.T) {
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		if incusVirt == nil {
			t.Fatal("Expected IncusVirt, got nil")
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
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given an IncusVirt with valid mocks
		incusVirt, _ := setup(t)

		// When calling Up
		err := incusVirt.Up(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("NotIncusRuntime", func(t *testing.T) {
		// Given an IncusVirt with docker platform (not incus)
		incusVirt, mocks := setup(t)
		if err := mocks.ConfigHandler.Set("platform", "docker"); err != nil {
			t.Fatalf("Failed to set platform: %v", err)
		}
		if err := mocks.ConfigHandler.Set("provider", "docker"); err != nil {
			t.Fatalf("Failed to set provider: %v", err)
		}

		// When calling Up
		err := incusVirt.Up(context.Background())

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
		err := incusVirt.Up(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to ensure") {
			t.Errorf("Expected error about ensuring remote, got %v", err)
		}
	})

}

func TestIncusVirt_Down(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
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
		err := incusVirt.Down(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("NotIncusRuntime", func(t *testing.T) {
		// Given an IncusVirt with docker platform (not incus)
		incusVirt, mocks := setup(t)
		if err := mocks.ConfigHandler.Set("platform", "docker"); err != nil {
			t.Fatalf("Failed to set platform: %v", err)
		}
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
		err := incusVirt.Down(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("StopsColimaDaemonWithCorrectProfile", func(t *testing.T) {
		incusVirt, mocks := setup(t)
		if err := mocks.ConfigHandler.Set("workstation.runtime", "colima"); err != nil {
			t.Fatalf("Failed to set workstation.runtime: %v", err)
		}
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
		originalExecSilent := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && len(args) >= 3 && args[0] == "daemon" && args[1] == "stop" {
				daemonStopCalled = true
				daemonStopProfile = args[2]
				return "", nil
			}
			if command == "colima" && len(args) >= 1 && args[0] == "stop" {
				return "", nil
			}
			if originalExecSilent != nil {
				return originalExecSilent(command, args...)
			}
			return "", nil
		}
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "incus" && len(args) >= 2 && args[0] == "delete" {
				return "", nil
			}
			if command == "colima" && len(args) >= 1 && args[0] == "delete" {
				return "", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		err := incusVirt.Down(context.Background())

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
		incusVirt := NewIncusVirt(mocks.Runtime)
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

// =============================================================================
// Test Private Methods
// =============================================================================

func TestIncusVirt_validateVMForIncus(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		if err := mocks.ConfigHandler.Set("platform", "incus"); err != nil {
			t.Fatalf("Failed to set platform: %v", err)
		}
		incusVirt := NewIncusVirt(mocks.Runtime)
		return incusVirt, mocks
	}

	t.Run("ReturnsInfoWhenAddressIsSet", func(t *testing.T) {
		// Given an IncusVirt and a VM with an address
		incusVirt, _ := setup(t)

		// When validating the VM
		info, err := incusVirt.validateVMForIncus(VMInfo{Address: "192.168.1.100"})

		// Then the VM info should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if info.Address != "192.168.1.100" {
			t.Errorf("Expected address 192.168.1.100, got %q", info.Address)
		}
	})

	t.Run("ErrorWithRemediationWhenAddressIsEmpty", func(t *testing.T) {
		// Given an IncusVirt and a VM without an address
		incusVirt, _ := setup(t)

		// When validating the VM
		_, err := incusVirt.validateVMForIncus(VMInfo{})

		// Then an error with a remediation hint should be returned
		if err == nil || !strings.Contains(err.Error(), "has no IP address") {
			t.Fatalf("Expected no IP address error, got %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "windsor down --clean") {
			t.Errorf("Expected remediation to mention windsor down --clean, got %v", we)
		}
	})
}

func TestIncusVirt_ensureRemote(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("RemoteExists", func(t *testing.T) {
		// Given an IncusVirt with existing remote
		incusVirt, _ := setup(t)

		// When ensuring remote
		err := incusVirt.ensureRemote("docker", "https://docker.io")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("RemoteDoesNotExist", func(t *testing.T) {
		// Given an IncusVirt with non-existing remote
		incusVirt, mocks := setup(t)
		remoteListCallCount := 0
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "incus" && len(args) >= 2 && args[0] == "remote" && args[1] == "list" {
				remoteListCallCount++
				if remoteListCallCount == 1 {
					return `{"existing":{"name":"existing","url":"https://existing.io","protocol":"oci","public":true}}`, nil
				}
				return `{"docker":{"name":"docker","url":"https://docker.io","protocol":"oci","public":true},"existing":{"name":"existing","url":"https://existing.io","protocol":"oci","public":true}}`, nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "incus" && len(args) >= 2 && args[0] == "remote" && args[1] == "add" {
				return "", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When ensuring remote
		err := incusVirt.ensureRemote("docker", "https://docker.io")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorCheckingRemote", func(t *testing.T) {
		// Given an IncusVirt with error checking remote
		incusVirt, mocks := setup(t)
		originalFunc := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus remote list --format json") {
					return "", fmt.Errorf("remote list error")
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When ensuring remote
		err := incusVirt.ensureRemote("docker", "https://docker.io")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
	})

	t.Run("ErrorAddingRemoteButExists", func(t *testing.T) {
		// Given an IncusVirt where add fails but remote exists
		incusVirt, mocks := setup(t)
		remoteListCallCount := 0
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "incus" && len(args) >= 2 && args[0] == "remote" && args[1] == "list" {
				remoteListCallCount++
				if remoteListCallCount == 1 {
					return `{"existing":{"name":"existing","url":"https://existing.io","protocol":"oci","public":true}}`, nil
				}
				return `{"docker":{"name":"docker","url":"https://docker.io","protocol":"oci","public":true}}`, nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "incus" && len(args) >= 2 && args[0] == "remote" && args[1] == "add" {
				return "", fmt.Errorf("add error")
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When ensuring remote
		err := incusVirt.ensureRemote("docker", "https://docker.io")

		// Then no error should be returned (remote exists after failed add)
		if err != nil {
			t.Errorf("Expected no error when remote exists after failed add, got %v", err)
		}
	})
}

func TestIncusVirt_remoteExists(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("RemoteExists", func(t *testing.T) {
		// Given an IncusVirt with existing remote
		incusVirt, _ := setup(t)

		// When checking if remote exists
		exists, err := incusVirt.remoteExists("docker")

		// Then it should return true
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !exists {
			t.Error("Expected true, got false")
		}
	})

	t.Run("RemoteDoesNotExist", func(t *testing.T) {
		// Given an IncusVirt with non-existing remote
		incusVirt, mocks := setup(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "incus" && len(args) >= 2 && args[0] == "remote" && args[1] == "list" {
				return `{"existing":{"name":"existing","url":"https://existing.io","protocol":"oci","public":true}}`, nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When checking if remote exists
		exists, err := incusVirt.remoteExists("non-existent")

		// Then it should return false
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if exists {
			t.Error("Expected false, got true")
		}
	})

	t.Run("ErrorGettingRemotes", func(t *testing.T) {
		// Given an IncusVirt with error getting remotes
		incusVirt, mocks := setup(t)
		originalFunc := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus remote list --format json") {
					return "", fmt.Errorf("remote list error")
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When checking if remote exists
		_, err := incusVirt.remoteExists("docker")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
	})
}

func TestIncusVirt_getRemotes(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("ErrorExecutingCommand", func(t *testing.T) {
		// Given an IncusVirt with error executing command
		incusVirt, mocks := setup(t)
		originalFunc := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus remote list --format json") {
					return "", fmt.Errorf("exec error")
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When getting remotes
		_, err := incusVirt.getRemotes()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
	})

	t.Run("ErrorParsingJSON", func(t *testing.T) {
		// Given an IncusVirt with invalid JSON
		incusVirt, mocks := setup(t)
		originalFunc := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus remote list --format json") {
					return `invalid json`, nil
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When getting remotes
		_, err := incusVirt.getRemotes()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse remotes JSON") {
			t.Errorf("Expected JSON parse error, got %v", err)
		}
	})
}

func TestIncusVirt_ensureFileExists(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("FileExists", func(t *testing.T) {
		// Given an IncusVirt with existing file
		incusVirt, mocks := setup(t)
		originalFunc := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "test -e") {
					return "", nil
				}
				if strings.Contains(actualCmd, "mkdir -p") {
					return "", nil
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args...)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When ensuring file exists
		err := incusVirt.ensureFileExists("/path/to/file")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("FileDoesNotExist", func(t *testing.T) {
		// Given an IncusVirt with non-existing file
		incusVirt, mocks := setup(t)
		testCallCount := 0
		originalFunc := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "test -e") {
					testCallCount++
					if testCallCount == 1 {
						return "", fmt.Errorf("file does not exist")
					}
					return "", nil
				}
				if strings.Contains(actualCmd, "mkdir -p") {
					return "", nil
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args...)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When ensuring file exists
		err := incusVirt.ensureFileExists("/path/to/file")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorCreatingParentDir", func(t *testing.T) {
		// Given an IncusVirt with error creating parent directory
		incusVirt, mocks := setup(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "test" && len(args) >= 1 && args[0] == "-e" {
				return "", fmt.Errorf("file does not exist")
			}
			if command == "mkdir" && len(args) >= 1 && args[0] == "-p" {
				return "", fmt.Errorf("mkdir error")
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When ensuring file exists
		err := incusVirt.ensureFileExists("/path/to/file")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
	})
}

func TestIncusVirt_waitForInstanceReady(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("InstanceBecomesReady", func(t *testing.T) {
		// Given an IncusVirt with instance that becomes ready quickly
		incusVirt, mocks := setup(t)
		callCount := 0
		originalFunc := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus operation list --format json") {
					callCount++
					if callCount == 1 {
						return `[{"id":"op1","status":"running","resources":{"instances":["test-service"]}}]`, nil
					}
					return `[]`, nil
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When waiting for instance to be ready (with short timeout to avoid hanging)
		err := incusVirt.waitForInstanceReady("test-service", 50*time.Millisecond)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("TimeoutWaitingForReady", func(t *testing.T) {
		// Given an IncusVirt with instance that never becomes ready
		incusVirt, mocks := setup(t)
		originalFunc := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus operation list --format json") {
					return `[{"id":"op1","status":"running","resources":{"instances":["test-service"]}}]`, nil
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When waiting for instance to be ready with very short timeout
		err := incusVirt.waitForInstanceReady("test-service", 10*time.Millisecond)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout") {
			t.Errorf("Expected timeout error, got %v", err)
		}
	})
}

func TestIncusVirt_pollUntilNotBusy(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("InstanceNotBusy", func(t *testing.T) {
		// Given an IncusVirt with non-busy instance
		incusVirt, _ := setup(t)

		// When polling until not busy
		err := incusVirt.pollUntilNotBusy("test-service", 0, time.Time{})

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("InstanceBecomesNotBusy", func(t *testing.T) {
		// Given an IncusVirt with instance that becomes not busy quickly
		incusVirt, mocks := setup(t)
		callCount := 0
		originalFunc := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus operation list --format json") {
					callCount++
					if callCount == 1 {
						return `[{"id":"op1","status":"running","resources":{"instances":["test-service"]}}]`, nil
					}
					return `[]`, nil
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When polling until not busy (will exit on second call)
		err := incusVirt.pollUntilNotBusy("test-service", 0, time.Time{})

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if callCount < 2 {
			t.Errorf("Expected at least 2 calls to check busy status, got %d", callCount)
		}
	})

	t.Run("TimeoutByIterations", func(t *testing.T) {
		// Given an IncusVirt with instance that stays busy
		incusVirt, mocks := setup(t)
		callCount := 0
		originalFunc := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus operation list --format json") {
					callCount++
					return `[{"id":"op1","status":"running","resources":{"instances":["test-service"]}}]`, nil
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When polling until not busy with max iterations
		err := incusVirt.pollUntilNotBusy("test-service", 5, time.Time{})

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "still busy after") {
			t.Errorf("Expected busy error, got %v", err)
		}
		if callCount != 5 {
			t.Errorf("Expected 5 calls before timeout, got %d", callCount)
		}
	})

	t.Run("TimeoutByDeadline", func(t *testing.T) {
		// Given an IncusVirt with instance that stays busy
		incusVirt, mocks := setup(t)
		callCount := 0
		originalFunc := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus operation list --format json") {
					callCount++
					return `[{"id":"op1","status":"running","resources":{"instances":["test-service"]}}]`, nil
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When polling until not busy with deadline
		deadline := time.Now().Add(20 * time.Millisecond)
		err := incusVirt.pollUntilNotBusy("test-service", 0, deadline)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "still busy after deadline") {
			t.Errorf("Expected deadline error, got %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "incus operation list") {
			t.Errorf("Expected remediation to mention incus operation list, got %v", we)
		}
		if callCount < 1 {
			t.Errorf("Expected at least 1 call before timeout, got %d", callCount)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestIncusVirt_sanitizeInstanceName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "ValidName",
			input:    "test-service",
			expected: "test-service",
		},
		{
			name:     "WithUnderscore",
			input:    "test_service",
			expected: "test-service",
		},
		{
			name:     "WithSpecialChars",
			input:    "test.service@123",
			expected: "test-service-123",
		},
		{
			name:     "WithSpaces",
			input:    "test service",
			expected: "test-service",
		},
		{
			name:     "MixedCase",
			input:    "TestService",
			expected: "TestService",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When sanitizing instance name
			result := sanitizeInstanceName(tt.input)

			// Then it should match expected
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}
