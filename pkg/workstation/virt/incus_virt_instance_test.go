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

func TestIncusVirt_instanceExists(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("InstanceExists", func(t *testing.T) {
		// Given an IncusVirt with existing instance
		incusVirt, _ := setup(t)

		// When checking if instance exists
		exists, err := incusVirt.instanceExists("test-service")

		// Then it should return true
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !exists {
			t.Error("Expected true, got false")
		}
	})

	t.Run("InstanceDoesNotExist", func(t *testing.T) {
		// Given an IncusVirt with non-existing instance
		incusVirt, mocks := setup(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "incus" && len(args) >= 1 && args[0] == "list" {
				return `[]`, nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When checking if instance exists
		exists, err := incusVirt.instanceExists("non-existent")

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

		// When checking if instance exists
		_, err := incusVirt.instanceExists("test-service")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
	})
}

func TestIncusVirt_instanceIsRunning(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("InstanceNotFound", func(t *testing.T) {
		// Given an IncusVirt with instance that doesn't exist
		incusVirt, mocks := setup(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "incus" && len(args) >= 1 && args[0] == "list" {
				return `[]`, nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When checking if instance is running
		running, err := incusVirt.instanceIsRunning("non-existent")

		// Then it should return false with no error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if running {
			t.Error("Expected false, got true")
		}
	})

	t.Run("InstanceRunningByStatusCode", func(t *testing.T) {
		// Given an IncusVirt with instance that has status code 103
		incusVirt, mocks := setup(t)
		originalFunc := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					return `[{"name":"test-service","status":"Stopped","status_code":103,"type":"container","expanded_devices":{}}]`, nil
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When checking if instance is running
		running, err := incusVirt.instanceIsRunning("test-service")

		// Then it should return true
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !running {
			t.Error("Expected true, got false")
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

		// When checking if instance is running
		_, err := incusVirt.instanceIsRunning("test-service")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
	})
}

func TestIncusVirt_instanceIsBusy(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("InstanceIsBusy", func(t *testing.T) {
		// Given an IncusVirt with busy instance
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

		// When checking if instance is busy
		busy, err := incusVirt.instanceIsBusy("test-service")

		// Then it should return true
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !busy {
			t.Error("Expected instance to be busy")
		}
	})

	t.Run("InstanceIsNotBusy", func(t *testing.T) {
		// Given an IncusVirt with non-busy instance
		incusVirt, _ := setup(t)

		// When checking if instance is busy
		busy, err := incusVirt.instanceIsBusy("test-service")

		// Then it should return false
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if busy {
			t.Error("Expected instance to not be busy")
		}
	})

	t.Run("ErrorGettingOperations", func(t *testing.T) {
		// Given an IncusVirt with error getting operations
		incusVirt, mocks := setup(t)
		originalFunc := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus operation list --format json") {
					return "", fmt.Errorf("operation list error")
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When checking if instance is busy
		_, err := incusVirt.instanceIsBusy("test-service")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
	})
}

func TestIncusVirt_getInstances(t *testing.T) {
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
				if strings.Contains(actualCmd, "incus list --format json") {
					return "", fmt.Errorf("exec error")
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When getting instances
		_, err := incusVirt.getInstances()

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
				if strings.Contains(actualCmd, "incus list --format json") {
					return `invalid json`, nil
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When getting instances
		_, err := incusVirt.getInstances()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse instances JSON") {
			t.Errorf("Expected JSON parse error, got %v", err)
		}
	})
}

func TestIncusVirt_getOperations(t *testing.T) {
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
				if strings.Contains(actualCmd, "incus operation list --format json") {
					return "", fmt.Errorf("exec error")
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When getting operations
		_, err := incusVirt.getOperations()

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
				if strings.Contains(actualCmd, "incus operation list --format json") {
					return `invalid json`, nil
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When getting operations
		_, err := incusVirt.getOperations()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse operations JSON") {
			t.Errorf("Expected JSON parse error, got %v", err)
		}
	})
}

func TestIncusVirt_launchInstance(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("InstanceDoesNotExist", func(t *testing.T) {
		// Given an IncusVirt with non-existing instance
		incusVirt, mocks := setup(t)
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		originalExecProgress := mocks.Shell.ExecProgressFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					return `[]`, nil
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
				if strings.Contains(actualCmd, "incus launch") {
					return "", nil
				}
			}
			if originalExecProgress != nil {
				return originalExecProgress(message, command, args...)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When launching instance
		err := incusVirt.launchInstance(&IncusInstanceConfig{
			Name:  "new-service",
			Type:  "container",
			Image: "docker:alpine:latest",
		})

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorCheckingInstanceExists", func(t *testing.T) {
		// Given an IncusVirt with error checking instance
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

		// When launching instance
		err := incusVirt.launchInstance(&IncusInstanceConfig{
			Name:  "test-service",
			Type:  "container",
			Image: "docker:alpine:latest",
		})

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
	})
}

func TestIncusVirt_buildLaunchArgs(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("Container", func(t *testing.T) {
		// Given an IncusVirt
		incusVirt, _ := setup(t)

		// When building launch args for container
		args := incusVirt.buildLaunchArgs(&IncusInstanceConfig{
			Name:  "test-service",
			Type:  "container",
			Image: "docker:alpine:latest",
		})

		// Then args should not include --vm
		hasVM := false
		for _, arg := range args {
			if arg == "--vm" {
				hasVM = true
				break
			}
		}
		if hasVM {
			t.Error("Expected args to not include --vm for container")
		}
	})

	t.Run("VM", func(t *testing.T) {
		// Given an IncusVirt
		incusVirt, _ := setup(t)

		// When building launch args for VM
		args := incusVirt.buildLaunchArgs(&IncusInstanceConfig{
			Name:  "test-service",
			Type:  "vm",
			Image: "docker:alpine:latest",
		})

		// Then args should include --vm
		hasVM := false
		for _, arg := range args {
			if arg == "--vm" {
				hasVM = true
				break
			}
		}
		if !hasVM {
			t.Error("Expected args to include --vm for VM")
		}
	})

	t.Run("WithNetwork", func(t *testing.T) {
		// Given an IncusVirt
		incusVirt, _ := setup(t)

		// When building launch args with network but no IPv4
		args := incusVirt.buildLaunchArgs(&IncusInstanceConfig{
			Name:    "test-service",
			Type:    "container",
			Image:   "docker:alpine:latest",
			Network: "incusbr0",
		})

		// Then args should include --network
		hasNetwork := false
		for i, arg := range args {
			if arg == "--network" && i+1 < len(args) && args[i+1] == "incusbr0" {
				hasNetwork = true
				break
			}
		}
		if !hasNetwork {
			t.Error("Expected args to include --network")
		}
	})

	t.Run("WithConfig", func(t *testing.T) {
		// Given an IncusVirt
		incusVirt, _ := setup(t)

		// When building launch args with config
		args := incusVirt.buildLaunchArgs(&IncusInstanceConfig{
			Name:  "test-service",
			Type:  "container",
			Image: "docker:alpine:latest",
			Config: map[string]string{
				"key1": "value1",
				"key2": "value with spaces",
			},
		})

		// Then args should include --config entries
		hasConfig := false
		for i, arg := range args {
			if arg == "--config" && i+1 < len(args) {
				hasConfig = true
				configValue := args[i+1]
				if !strings.Contains(configValue, "key1=value1") && !strings.Contains(configValue, "key2=\"value with spaces\"") {
					t.Errorf("Expected config value to be properly formatted, got %s", configValue)
				}
				break
			}
		}
		if !hasConfig {
			t.Error("Expected args to include --config")
		}
	})

	t.Run("WithResources", func(t *testing.T) {
		// Given an IncusVirt
		incusVirt, _ := setup(t)

		// When building launch args with resources
		args := incusVirt.buildLaunchArgs(&IncusInstanceConfig{
			Name:  "test-service",
			Type:  "container",
			Image: "docker:alpine:latest",
			Resources: map[string]string{
				"limits.cpu": "2",
			},
		})

		// Then args should include --config entries for resources
		hasResource := false
		for i, arg := range args {
			if arg == "--config" && i+1 < len(args) && strings.Contains(args[i+1], "limits.cpu=2") {
				hasResource = true
				break
			}
		}
		if !hasResource {
			t.Error("Expected args to include resource config")
		}
	})

	t.Run("WithProfiles", func(t *testing.T) {
		// Given an IncusVirt
		incusVirt, _ := setup(t)

		// When building launch args with profiles
		args := incusVirt.buildLaunchArgs(&IncusInstanceConfig{
			Name:     "test-service",
			Type:     "container",
			Image:    "docker:alpine:latest",
			Profiles: []string{"profile1", "profile2"},
		})

		// Then args should include --profile entries
		profileCount := 0
		for i, arg := range args {
			if arg == "--profile" && i+1 < len(args) {
				profileCount++
			}
		}
		if profileCount != 2 {
			t.Errorf("Expected 2 profile entries, got %d", profileCount)
		}
	})
}

func TestIncusVirt_handleInstanceLaunchError(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("InstanceCreatedDespiteError", func(t *testing.T) {
		// Given an IncusVirt where instance was created despite launch error
		incusVirt, _ := setup(t)

		// When handling launch error
		err := incusVirt.handleInstanceLaunchError("test-service", "message", []string{"launch", "image", "name"}, fmt.Errorf("launch error"))

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("InstanceBusy", func(t *testing.T) {
		// Given an IncusVirt where instance is busy
		incusVirt, mocks := setup(t)
		callCount := 0
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "incus" && len(args) >= 1 && args[0] == "list" {
				callCount++
				if callCount == 1 {
					return `[]`, nil
				}
				return `[{"name":"test-service","status":"Running","status_code":103,"type":"container","expanded_devices":{}}]`, nil
			}
			if command == "incus" && len(args) >= 2 && args[0] == "operation" && args[1] == "list" {
				callCount++
				if callCount == 2 {
					return `[{"id":"op1","status":"running","resources":{"instances":["test-service"]}}]`, nil
				}
				return `[]`, nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "incus" && len(args) > 0 && args[0] == "launch" {
				return "", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When handling launch error
		err := incusVirt.handleInstanceLaunchError("test-service", "message", []string{"launch", "image", "name"}, fmt.Errorf("launch error"))

		// Then no error should be returned (retry succeeds)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorAfterWait", func(t *testing.T) {
		// Given an IncusVirt where retry after wait fails
		incusVirt, mocks := setup(t)
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					return `[]`, nil
				}
				if strings.Contains(actualCmd, "incus operation list --format json") {
					return `[{"id":"op1","status":"running","resources":{"instances":["test-service"]}}]`, nil
				}
			}
			if originalExecSilentWithTimeout != nil {
				return originalExecSilentWithTimeout(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "incus" && len(args) > 0 && args[0] == "launch" {
				return "", fmt.Errorf("launch error")
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When handling launch error
		err := incusVirt.handleInstanceLaunchError("test-service", "message", []string{"launch", "image", "name"}, fmt.Errorf("launch error"))

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
	})

	t.Run("ErrorCheckingInstanceExists", func(t *testing.T) {
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

		err := incusVirt.handleInstanceLaunchError("test-service", "message", []string{"launch", "image", "name"}, fmt.Errorf("launch error"))

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to launch Incus instance") {
			t.Errorf("Expected error about launching instance, got %v", err)
		}
	})

	t.Run("ErrorCheckingBusy", func(t *testing.T) {
		incusVirt, mocks := setup(t)
		callCount := 0
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					callCount++
					if callCount == 1 {
						return `[]`, nil
					}
					return "", fmt.Errorf("list error")
				}
				if strings.Contains(actualCmd, "incus operation list --format json") {
					return "", fmt.Errorf("operation list error")
				}
			}
			if originalExecSilentWithTimeout != nil {
				return originalExecSilentWithTimeout(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		err := incusVirt.handleInstanceLaunchError("test-service", "message", []string{"launch", "image", "name"}, fmt.Errorf("launch error"))

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to launch Incus instance") {
			t.Errorf("Expected error about launching instance, got %v", err)
		}
	})

	t.Run("ErrorWaitingForReady", func(t *testing.T) {
		incusVirt, mocks := setup(t)
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					return `[]`, nil
				}
				if strings.Contains(actualCmd, "incus operation list --format json") {
					return `[{"id":"op1","status":"running","resources":{"instances":["test-service"]}}]`, nil
				}
			}
			if originalExecSilentWithTimeout != nil {
				return originalExecSilentWithTimeout(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		err := incusVirt.handleInstanceLaunchError("test-service", "message", []string{"launch", "image", "name"}, fmt.Errorf("launch error"))

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "instance is busy") {
			t.Errorf("Expected error about instance busy, got %v", err)
		}
	})

	t.Run("InstanceExistsAndBusy", func(t *testing.T) {
		// Given an IncusVirt where instance exists and stays busy (will timeout)
		incusVirt, mocks := setup(t)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		callCount := 0
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					callCount++
					if callCount == 1 {
						return `[{"name":"test-service","status":"Running","status_code":103,"type":"container","expanded_devices":{}}]`, nil
					}
					return `[{"name":"test-service","status":"Running","status_code":103,"type":"container","expanded_devices":{}}]`, nil
				}
				if strings.Contains(actualCmd, "incus operation list --format json") {
					return `[{"id":"op1","status":"running","resources":{"instances":["test-service"]}}]`, nil
				}
			}
			if originalExecSilentWithTimeout != nil {
				return originalExecSilentWithTimeout(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When handling launch error (will timeout waiting for instance to not be busy)
		err := incusVirt.handleInstanceLaunchError("test-service", "launch message", []string{"launch", "image"}, fmt.Errorf("launch error"))

		// Then an error should be returned (timeout waiting for instance to not be busy)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "instance exists but is busy") {
			t.Errorf("Expected error about instance busy, got %v", err)
		}
	})

	t.Run("BusyWaitThenRetrySucceeds", func(t *testing.T) {
		// Given an IncusVirt where instance is busy, wait succeeds, and retry succeeds
		incusVirt, mocks := setup(t)
		opCallCount := 0
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "incus" && len(args) >= 1 && args[0] == "list" {
				return `[]`, nil
			}
			if command == "incus" && len(args) >= 2 && args[0] == "operation" && args[1] == "list" {
				opCallCount++
				if opCallCount == 1 {
					return `[{"id":"op1","status":"running","resources":{"instances":["test-service"]}}]`, nil
				}
				return `[]`, nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "incus" && len(args) > 0 && args[0] == "launch" {
				return "", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When handling launch error
		err := incusVirt.handleInstanceLaunchError("test-service", "launch message", []string{"launch", "image"}, fmt.Errorf("launch error"))

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestIncusVirt_ensureInstanceNotBusy(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("ErrorCheckingBusy", func(t *testing.T) {
		// Given an IncusVirt where checking busy fails
		incusVirt, mocks := setup(t)
		originalFunc := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus operation list --format json") {
					return "", fmt.Errorf("operation list error")
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When ensuring instance is not busy
		err := incusVirt.ensureInstanceNotBusy("test-service", "error prefix")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to check if instance is busy") {
			t.Errorf("Expected error about checking busy, got %v", err)
		}
	})
}

func TestIncusVirt_stopInstanceIfRunning(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("InstanceNotRunning", func(t *testing.T) {
		// Given an IncusVirt with non-running instance
		incusVirt, mocks := setup(t)
		originalFunc := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					return `[{"name":"test-service","status":"Stopped","status_code":102,"type":"container","expanded_devices":{}}]`, nil
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When stopping instance if running
		wasStopped, err := incusVirt.stopInstanceIfRunning("test-service", "error prefix")

		// Then it should return false (not stopped) and no error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if wasStopped {
			t.Error("Expected wasStopped to be false, got true")
		}
	})

	t.Run("InstanceRunningStopsSuccessfully", func(t *testing.T) {
		// Given an IncusVirt with running instance
		incusVirt, mocks := setup(t)
		instanceListCallCount := 0
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		originalExecSilent := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					instanceListCallCount++
					if instanceListCallCount == 1 {
						return `[{"name":"test-service","status":"Running","status_code":103,"type":"container","expanded_devices":{}}]`, nil
					}
					return `[{"name":"test-service","status":"Stopped","status_code":102,"type":"container","expanded_devices":{}}]`, nil
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
			}
			if originalExecSilent != nil {
				return originalExecSilent(command, args...)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When stopping instance if running
		wasStopped, err := incusVirt.stopInstanceIfRunning("test-service", "error prefix")

		// Then it should return true (was stopped) and no error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !wasStopped {
			t.Error("Expected wasStopped to be true, got false")
		}
	})

	t.Run("ErrorCheckingRunning", func(t *testing.T) {
		// Given an IncusVirt with error checking instance status
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

		// When stopping instance if running
		_, err := incusVirt.stopInstanceIfRunning("test-service", "error prefix")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to check if instance is running") {
			t.Errorf("Expected error about checking running, got %v", err)
		}
	})

	t.Run("ErrorStoppingButStopped", func(t *testing.T) {
		// Given an IncusVirt where stop fails but instance is stopped
		incusVirt, mocks := setup(t)
		instanceListCallCount := 0
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		originalExecSilent := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					instanceListCallCount++
					if instanceListCallCount == 1 {
						return `[{"name":"test-service","status":"Running","status_code":103,"type":"container","expanded_devices":{}}]`, nil
					}
					return `[{"name":"test-service","status":"Stopped","status_code":102,"type":"container","expanded_devices":{}}]`, nil
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
					return "", fmt.Errorf("stop error")
				}
			}
			if originalExecSilent != nil {
				return originalExecSilent(command, args...)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When stopping instance if running
		wasStopped, err := incusVirt.stopInstanceIfRunning("test-service", "error prefix")

		// Then it should return true (instance is stopped) and no error (even though stop command failed)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !wasStopped {
			t.Error("Expected wasStopped to be true (instance is stopped), got false")
		}
	})

	t.Run("InstanceStillRunningAfterStop", func(t *testing.T) {
		// Given an IncusVirt where instance is still running after stop
		incusVirt, mocks := setup(t)
		instanceListCallCount := 0
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		originalExecSilent := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					instanceListCallCount++
					if instanceListCallCount == 1 {
						return `[{"name":"test-service","status":"Running","status_code":103,"type":"container","expanded_devices":{}}]`, nil
					}
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
				if strings.Contains(actualCmd, "incus stop") {
					return "", nil
				}
			}
			if originalExecSilent != nil {
				return originalExecSilent(command, args...)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When stopping instance if running
		_, err := incusVirt.stopInstanceIfRunning("test-service", "error prefix")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "still running after stop") {
			t.Errorf("Expected error about still running, got %v", err)
		}
	})
}

func TestIncusVirt_startInstance(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("ErrorStarting", func(t *testing.T) {
		// Given an IncusVirt with error starting instance
		incusVirt, mocks := setup(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "incus" && len(args) >= 1 && args[0] == "start" {
				return "", fmt.Errorf("start error")
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When starting instance
		err := incusVirt.startInstance("test-service", "error prefix")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error prefix") {
			t.Errorf("Expected error with prefix, got %v", err)
		}
	})
}

func TestIncusVirt_ensureInstanceRunning(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("InstanceAlreadyRunning", func(t *testing.T) {
		// Given an IncusVirt with running instance
		incusVirt, _ := setup(t)

		// When ensuring instance is running
		err := incusVirt.ensureInstanceRunning("test-service")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("StartStoppedInstance", func(t *testing.T) {
		// Given an IncusVirt with stopped instance
		incusVirt, mocks := setup(t)
		callCount := 0
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		originalExecSilent := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					callCount++
					if callCount == 1 {
						return `[{"name":"test-service","status":"Stopped","status_code":102,"type":"container","expanded_devices":{}}]`, nil
					}
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
				if strings.Contains(actualCmd, "incus start") {
					return "", nil
				}
			}
			if originalExecSilent != nil {
				return originalExecSilent(command, args...)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When ensuring instance is running
		err := incusVirt.ensureInstanceRunning("test-service")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorStartingButRunning", func(t *testing.T) {
		// Given an IncusVirt where start fails but instance is running
		incusVirt, mocks := setup(t)
		callCount := 0
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		originalExecSilent := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					callCount++
					if callCount == 1 {
						return `[{"name":"test-service","status":"Stopped","status_code":102,"type":"container","expanded_devices":{}}]`, nil
					}
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
				if strings.Contains(actualCmd, "incus start") {
					return "", fmt.Errorf("start error")
				}
			}
			if originalExecSilent != nil {
				return originalExecSilent(command, args...)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When ensuring instance is running
		err := incusVirt.ensureInstanceRunning("test-service")

		// Then no error should be returned (instance is running)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestIncusVirt_deleteInstance(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime)
		incusVirt.pollInterval = 1 * time.Millisecond
		incusVirt.maxWaitTimeout = 50 * time.Millisecond
		return incusVirt, mocks
	}

	t.Run("InstanceDoesNotExist", func(t *testing.T) {
		// Given an IncusVirt with non-existing instance
		incusVirt, mocks := setup(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "incus" && len(args) >= 1 && args[0] == "list" {
				return `[]`, nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When deleting instance
		err := incusVirt.deleteInstance("non-existent")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorDeletingButGone", func(t *testing.T) {
		// Given an IncusVirt where delete fails but instance is gone
		incusVirt, mocks := setup(t)
		callCount := 0
		originalExecSilentWithTimeout := mocks.Shell.ExecSilentWithTimeoutFunc
		originalExecProgress := mocks.Shell.ExecProgressFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if strings.Contains(actualCmd, "incus list --format json") {
					callCount++
					if callCount == 1 {
						return `[{"name":"test-service","status":"Running","status_code":103,"type":"container","expanded_devices":{}}]`, nil
					}
					return `[]`, nil
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
				if strings.Contains(actualCmd, "incus delete") {
					return "", fmt.Errorf("delete error")
				}
			}
			if originalExecProgress != nil {
				return originalExecProgress(message, command, args...)
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When deleting instance
		err := incusVirt.deleteInstance("test-service")

		// Then no error should be returned (instance is gone)
		if err != nil {
			t.Errorf("Expected no error when instance is gone after failed delete, got %v", err)
		}
	})
}
