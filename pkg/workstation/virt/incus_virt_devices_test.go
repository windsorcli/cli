package virt

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestIncusVirt_deviceExists(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("DeviceDoesNotExist", func(t *testing.T) {
		// Given an IncusVirt with instance that doesn't have the device
		incusVirt, mocks := setup(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "incus" && len(args) >= 1 && args[0] == "list" {
				return `[{"name":"test-service","status":"Running","status_code":103,"type":"container","expanded_devices":{"eth0":{}}}]`, nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When checking if device exists
		exists, err := incusVirt.deviceExists("test-service", "eth1")

		// Then it should return false
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if exists {
			t.Error("Expected false, got true")
		}
	})

	t.Run("InstanceNotFound", func(t *testing.T) {
		// Given an IncusVirt with instance that doesn't exist
		incusVirt, mocks := setup(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "incus" && len(args) >= 1 && args[0] == "list" {
				return `[]`, nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When checking if device exists
		exists, err := incusVirt.deviceExists("non-existent", "eth0")

		// Then it should return false
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if exists {
			t.Error("Expected false, got true")
		}
	})

	t.Run("ExpandedDevicesNil", func(t *testing.T) {
		// Given an IncusVirt with instance that has nil ExpandedDevices
		incusVirt, mocks := setup(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "incus" && len(args) >= 1 && args[0] == "list" {
				return `[{"name":"test-service","status":"Running","status_code":103,"type":"container"}]`, nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When checking if device exists
		exists, err := incusVirt.deviceExists("test-service", "eth0")

		// Then it should return false
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if exists {
			t.Error("Expected false, got true")
		}
	})

	t.Run("ErrorGettingInstances", func(t *testing.T) {
		// Given an IncusVirt with error getting instances
		incusVirt, mocks := setup(t)
		originalFunc := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					return "", fmt.Errorf("list error")
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When checking if device exists
		_, err := incusVirt.deviceExists("test-service", "eth0")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
	})
}

func TestIncusVirt_configureNetworkDevice(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("DeviceDoesNotExist", func(t *testing.T) {
		// Given an IncusVirt with non-existing device
		incusVirt, mocks := setup(t)
		opCallCount := 0
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		originalExecSilent := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					return `[{"name":"test-service","status":"Stopped","status_code":102,"type":"container","expanded_devices":{}}]`, nil
				}
				if strings.Contains(actualCmd, "incus operation list --format json") {
					opCallCount++
					if opCallCount == 1 {
						return `[{"id":"op1","status":"running","resources":{"instances":["test-service"]}}]`, nil
					}
					return `[]`, nil
				}
			}
			if originalExecSilentWithTimeout != nil {
				return originalExecSilentWithTimeout(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus config device add") {
					return "", nil
				}
				if strings.Contains(actualCmd, "incus stop") {
					return "", nil
				}
				if strings.Contains(actualCmd, "incus start") {
					return "", nil
				}
			}
			if originalExecSilent != nil {
				return originalExecSilent(command, args...)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When configuring network device
		err := incusVirt.configureNetworkDevice(&IncusInstanceConfig{
			Name:    "test-service",
			Network: "incusbr0",
			IPv4:    "10.0.0.10",
		})

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("DeviceExists", func(t *testing.T) {
		// Given an IncusVirt with existing device
		incusVirt, mocks := setup(t)
		opCallCount := 0
		instanceRunning := true
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		originalExecSilent := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					status := "Stopped"
					statusCode := 102
					if instanceRunning {
						status = "Running"
						statusCode = 103
					}
					return fmt.Sprintf(`[{"name":"test-service","status":"%s","status_code":%d,"type":"container","expanded_devices":{"eth0":{}}}]`, status, statusCode), nil
				}
				if strings.Contains(actualCmd, "incus operation list --format json") {
					opCallCount++
					if opCallCount == 1 {
						return `[{"id":"op1","status":"running","resources":{"instances":["test-service"]}}]`, nil
					}
					return `[]`, nil
				}
			}
			if originalExecSilentWithTimeout != nil {
				return originalExecSilentWithTimeout(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus config device get") && strings.Contains(actualCmd, "ipv4.address") {
					return "10.0.0.11", nil
				}
				if strings.Contains(actualCmd, "incus stop") {
					instanceRunning = false
					return "", nil
				}
				if strings.Contains(actualCmd, "incus config device set") {
					return "", fmt.Errorf("set error")
				}
				if strings.Contains(actualCmd, "incus config device override") {
					return "", nil
				}
				if strings.Contains(actualCmd, "incus start") {
					instanceRunning = true
					return "", nil
				}
			}
			if originalExecSilent != nil {
				return originalExecSilent(command, args...)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When configuring network device
		err := incusVirt.configureNetworkDevice(&IncusInstanceConfig{
			Name:    "test-service",
			Network: "incusbr0",
			IPv4:    "10.0.0.10",
		})

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestIncusVirt_addNetworkDevice(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("AddNetworkDevice", func(t *testing.T) {
		// Given an IncusVirt adding network device
		incusVirt, mocks := setup(t)
		opCallCount := 0
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		originalExecSilent := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					return `[{"name":"test-service","status":"Stopped","status_code":102,"type":"container","expanded_devices":{}}]`, nil
				}
				if strings.Contains(actualCmd, "incus operation list --format json") {
					opCallCount++
					if opCallCount == 1 {
						return `[{"id":"op1","status":"running","resources":{"instances":["test-service"]}}]`, nil
					}
					return `[]`, nil
				}
			}
			if originalExecSilentWithTimeout != nil {
				return originalExecSilentWithTimeout(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus stop") {
					return "", nil
				}
				if strings.Contains(actualCmd, "incus config device add") {
					return "", nil
				}
				if strings.Contains(actualCmd, "incus start") {
					return "", nil
				}
			}
			if originalExecSilent != nil {
				return originalExecSilent(command, args...)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When adding network device
		err := incusVirt.addNetworkDevice(&IncusInstanceConfig{
			Name:    "test-service",
			Network: "incusbr0",
			IPv4:    "10.0.0.10",
		})

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorAddingDeviceButExists", func(t *testing.T) {
		incusVirt, mocks := setup(t)
		instanceRunning := true
		callCount := 0
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		originalExecSilent := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					callCount++
					if callCount == 1 {
						status := "Stopped"
						statusCode := 102
						if instanceRunning {
							status = "Running"
							statusCode = 103
						}
						return fmt.Sprintf(`[{"name":"test-service","status":"%s","status_code":%d,"type":"container","expanded_devices":{}}]`, status, statusCode), nil
					}
					return `[{"name":"test-service","status":"Stopped","status_code":102,"type":"container","expanded_devices":{"eth0":{}}}]`, nil
				}
				if strings.Contains(actualCmd, "incus operation list --format json") {
					return `[]`, nil
				}
			}
			if originalExecSilentWithTimeout != nil {
				return originalExecSilentWithTimeout(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus stop") {
					instanceRunning = false
					return "", nil
				}
				if strings.Contains(actualCmd, "incus config device add") {
					return "", fmt.Errorf("device add error")
				}
				if strings.Contains(actualCmd, "incus start") {
					instanceRunning = true
					return "", nil
				}
			}
			if originalExecSilent != nil {
				return originalExecSilent(command, args...)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		err := incusVirt.addNetworkDevice(&IncusInstanceConfig{
			Name:    "test-service",
			Network: "incusbr0",
			IPv4:    "10.0.0.10",
		})

		if err != nil {
			t.Errorf("Expected no error (device exists), got %v", err)
		}
	})

	t.Run("ErrorCheckingDeviceExists", func(t *testing.T) {
		incusVirt, mocks := setup(t)
		instanceRunning := true
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "incus" && len(args) >= 1 && args[0] == "list" {
				status := "Stopped"
				statusCode := 102
				if instanceRunning {
					status = "Running"
					statusCode = 103
				}
				if len(args) >= 2 && args[1] == "--format" {
					return fmt.Sprintf(`[{"name":"test-service","status":"%s","status_code":%d,"type":"container","expanded_devices":{}}]`, status, statusCode), nil
				}
				return "", fmt.Errorf("list error")
			}
			if command == "incus" && len(args) >= 2 && args[0] == "operation" && args[1] == "list" {
				return `[]`, nil
			}
			if command == "incus" && len(args) >= 1 && args[0] == "stop" {
				instanceRunning = false
				return "", nil
			}
			if command == "incus" && len(args) >= 7 && args[0] == "config" && args[1] == "device" && args[2] == "add" {
				return "", fmt.Errorf("device add error")
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		err := incusVirt.addNetworkDevice(&IncusInstanceConfig{
			Name:    "test-service",
			Network: "incusbr0",
			IPv4:    "10.0.0.10",
		})

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to add network device") {
			t.Errorf("Expected error about adding device, got %v", err)
		}
	})
}

func TestIncusVirt_updateNetworkDevice(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("IPAlreadySet", func(t *testing.T) {
		// Given an IncusVirt with IP already set correctly
		incusVirt, mocks := setup(t)
		originalExecSilent := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus config device get") && strings.Contains(actualCmd, "ipv4.address") {
					return "10.0.0.10", nil
				}
			}
			if originalExecSilent != nil {
				return originalExecSilent(command, args...)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When updating network device
		err := incusVirt.updateNetworkDevice(&IncusInstanceConfig{
			Name:    "test-service",
			IPv4:    "10.0.0.10",
			Network: "incusbr0",
		})

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("UpdateIP", func(t *testing.T) {
		// Given an IncusVirt updating IP
		incusVirt, mocks := setup(t)
		opCallCount := 0
		instanceRunning := true
		originalExecSilent := mocks.Shell.ExecSilentFunc
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus config device get") && strings.Contains(actualCmd, "ipv4.address") {
					return "10.0.0.11", nil
				}
				if strings.Contains(actualCmd, "incus stop") {
					instanceRunning = false
					return "", nil
				}
				if strings.Contains(actualCmd, "incus config device set") {
					return "", nil
				}
				if strings.Contains(actualCmd, "incus start") {
					instanceRunning = true
					return "", nil
				}
			}
			if originalExecSilent != nil {
				return originalExecSilent(command, args...)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					status := "Stopped"
					statusCode := 102
					if instanceRunning {
						status = "Running"
						statusCode = 103
					}
					return fmt.Sprintf(`[{"name":"test-service","status":"%s","status_code":%d,"type":"container","expanded_devices":{"eth0":{}}}]`, status, statusCode), nil
				}
				if strings.Contains(actualCmd, "incus operation list --format json") {
					opCallCount++
					if opCallCount == 1 {
						return `[{"id":"op1","status":"running","resources":{"instances":["test-service"]}}]`, nil
					}
					return `[]`, nil
				}
			}
			if originalExecSilentWithTimeout != nil {
				return originalExecSilentWithTimeout(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When updating network device
		err := incusVirt.updateNetworkDevice(&IncusInstanceConfig{
			Name:    "test-service",
			IPv4:    "10.0.0.10",
			Network: "incusbr0",
		})

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorOnSetUseOverride", func(t *testing.T) {
		// Given an IncusVirt where set fails, uses override
		incusVirt, mocks := setup(t)
		opCallCount := 0
		setCallCount := 0
		instanceRunning := false
		originalExecSilent := mocks.Shell.ExecSilentFunc
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus config device get") && strings.Contains(actualCmd, "ipv4.address") {
					return "10.0.0.11", nil
				}
				if strings.Contains(actualCmd, "incus stop") {
					instanceRunning = false
					return "", nil
				}
				if strings.Contains(actualCmd, "incus config device set") {
					setCallCount++
					if setCallCount == 1 {
						return "", fmt.Errorf("device is busy")
					}
					return "", nil
				}
				if strings.Contains(actualCmd, "incus config device set") && strings.Contains(actualCmd, "use-override") {
					return "", nil
				}
			}
			if originalExecSilent != nil {
				return originalExecSilent(command, args...)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					status := "Stopped"
					statusCode := 102
					if instanceRunning {
						status = "Running"
						statusCode = 103
					}
					return fmt.Sprintf(`[{"name":"test-service","status":"%s","status_code":%d,"type":"container","expanded_devices":{"eth0":{}}}]`, status, statusCode), nil
				}
				if strings.Contains(actualCmd, "incus operation list --format json") {
					opCallCount++
					if opCallCount == 1 {
						return `[{"id":"op1","status":"running","resources":{"instances":["test-service"]}}]`, nil
					}
					return `[]`, nil
				}
			}
			if originalExecSilentWithTimeout != nil {
				return originalExecSilentWithTimeout(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When updating network device
		err := incusVirt.updateNetworkDevice(&IncusInstanceConfig{
			Name:    "test-service",
			IPv4:    "10.0.0.10",
			Network: "incusbr0",
		})

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestIncusVirt_addNonNetworkDevices(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("AddNonNetworkDevices", func(t *testing.T) {
		// Given an IncusVirt with non-network devices
		incusVirt, mocks := setup(t)
		originalExecSilent := mocks.Shell.ExecSilentFunc
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
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "test -e") {
					return "", nil
				}
				if strings.Contains(actualCmd, "incus config device add") {
					return "", nil
				}
				if strings.Contains(actualCmd, "incus config device get") {
					return "", fmt.Errorf("device not found")
				}
			}
			if originalExecSilent != nil {
				return originalExecSilent(command, args...)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When adding non-network devices
		err := incusVirt.addNonNetworkDevices(&IncusInstanceConfig{
			Name: "test-service",
			Devices: map[string]map[string]string{
				"disk1": {
					"type":   "disk",
					"source": "/path/to/disk",
					"path":   "/mnt/disk",
				},
				"eth0": {
					"type": "nic",
				},
			},
		})

		// Then no error should be returned (eth0 is skipped)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SkipDeviceWithoutType", func(t *testing.T) {
		// Given an IncusVirt with device without type
		incusVirt, _ := setup(t)

		// When adding devices without type
		err := incusVirt.addNonNetworkDevices(&IncusInstanceConfig{
			Name: "test-service",
			Devices: map[string]map[string]string{
				"disk1": {},
			},
		})

		// Then no error should be returned (skipped)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestIncusVirt_addDevice(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("DeviceExists", func(t *testing.T) {
		// Given an IncusVirt with existing device
		incusVirt, mocks := setup(t)
		originalFunc := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					return `[{"name":"test-service","status":"Running","status_code":103,"type":"container","expanded_devices":{"disk1":{}}}]`, nil
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When adding device
		err := incusVirt.addDevice("test-service", "disk1", map[string]string{"type": "disk"})

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("AddDiskDevice", func(t *testing.T) {
		// Given an IncusVirt adding disk device
		incusVirt, mocks := setup(t)
		originalExecSilent := mocks.Shell.ExecSilentFunc
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
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "test -e") {
					return "", nil
				}
				if strings.Contains(actualCmd, "incus config device add") {
					return "", nil
				}
				if strings.Contains(actualCmd, "incus config device get") {
					return "", fmt.Errorf("device not found")
				}
			}
			if originalExecSilent != nil {
				return originalExecSilent(command, args...)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When adding disk device
		err := incusVirt.addDevice("test-service", "disk1", map[string]string{
			"type":   "disk",
			"source": "/path/to/disk",
			"path":   "/mnt/disk",
		})

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorAddingDeviceButExists", func(t *testing.T) {
		// Given an IncusVirt where add fails but device exists
		incusVirt, mocks := setup(t)
		deviceCheckCount := 0
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		originalExecProgress := mocks.Shell.ExecProgressFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					deviceCheckCount++
					if deviceCheckCount == 1 {
						return `[{"name":"test-service","status":"Running","status_code":103,"type":"container","expanded_devices":{}}]`, nil
					}
					return `[{"name":"test-service","status":"Running","status_code":103,"type":"container","expanded_devices":{"disk1":{}}}]`, nil
				}
			}
			if originalExecSilentWithTimeout != nil {
				return originalExecSilentWithTimeout(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus config device add") {
					return "", fmt.Errorf("add error")
				}
			}
			if originalExecProgress != nil {
				return originalExecProgress(message, command, args...)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When adding device
		err := incusVirt.addDevice("test-service", "disk1", map[string]string{"type": "disk"})

		// Then no error should be returned (device exists)
		if err != nil {
			t.Errorf("Expected no error when device exists after failed add, got %v", err)
		}
	})
}
