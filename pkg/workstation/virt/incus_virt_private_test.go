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
// Test Private Methods
// =============================================================================

func TestIncusVirt_ensureRemote(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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

func TestIncusVirt_instanceIsBusy(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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

func TestIncusVirt_waitForInstanceReady(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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

func TestIncusVirt_ensureFileExists(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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

func TestIncusVirt_handleInstanceLaunchError(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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

func TestIncusVirt_updateNetworkDevice(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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

func TestIncusVirt_pollUntilNotBusy(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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
		if callCount < 1 {
			t.Errorf("Expected at least 1 call before timeout, got %d", callCount)
		}
	})
}

func TestIncusVirt_stopInstanceIfRunning(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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

func TestIncusVirt_addDevice(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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

func TestIncusVirt_addNonNetworkDevices(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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

func TestIncusVirt_ensureInstanceRunning(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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

func TestIncusVirt_launchInstance(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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

func TestIncusVirt_configureNetworkDevice(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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

func TestIncusVirt_buildInstanceConfig(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{mocks.Service})
		return incusVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given an IncusVirt with service
		incusVirt, mocks := setup(t)

		// When building instance config
		config := incusVirt.buildInstanceConfig(mocks.Service, &services.IncusConfig{
			Type:     "container",
			Image:    "docker:alpine:latest",
			Config:   map[string]string{"key": "value"},
			Devices:  map[string]map[string]string{},
			Profiles: []string{"default"},
		}, "incusbr0")

		// Then config should be built correctly
		if config == nil {
			t.Fatal("Expected config, got nil")
		}
		if config.Name != "test-service" {
			t.Errorf("Expected name test-service, got %s", config.Name)
		}
		if config.Type != "container" {
			t.Errorf("Expected type container, got %s", config.Type)
		}
		if config.Image != "docker:alpine:latest" {
			t.Errorf("Expected image docker:alpine:latest, got %s", config.Image)
		}
		if config.IPv4 != "10.0.0.10" {
			t.Errorf("Expected IPv4 10.0.0.10, got %s", config.IPv4)
		}
		if config.Network != "incusbr0" {
			t.Errorf("Expected network incusbr0, got %s", config.Network)
		}
	})

	t.Run("EmptyProfilesWithAddress", func(t *testing.T) {
		// Given an IncusVirt with service that has address
		incusVirt, mocks := setup(t)

		// When building instance config with empty profiles but address
		config := incusVirt.buildInstanceConfig(mocks.Service, &services.IncusConfig{
			Type:     "container",
			Image:    "docker:alpine:latest",
			Config:   map[string]string{},
			Devices:  map[string]map[string]string{},
			Profiles: []string{},
		}, "incusbr0")

		// Then profiles should remain empty
		if len(config.Profiles) != 0 {
			t.Errorf("Expected empty profiles, got %v", config.Profiles)
		}
	})

	t.Run("EmptyProfilesWithoutAddress", func(t *testing.T) {
		// Given an IncusVirt with service without address
		incusVirt, mocks := setup(t)
		mocks.Service.GetAddressFunc = func() string {
			return ""
		}

		// When building instance config with empty profiles and no address
		config := incusVirt.buildInstanceConfig(mocks.Service, &services.IncusConfig{
			Type:     "container",
			Image:    "docker:alpine:latest",
			Config:   map[string]string{},
			Devices:  map[string]map[string]string{},
			Profiles: []string{},
		}, "incusbr0")

		// Then default profile should be added
		if len(config.Profiles) != 1 || config.Profiles[0] != "default" {
			t.Errorf("Expected default profile, got %v", config.Profiles)
		}
	})
}

func TestIncusVirt_instanceIsRunning(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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

func TestIncusVirt_deviceExists(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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

func TestIncusVirt_getInstances(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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

func TestIncusVirt_instanceExists(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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

func TestIncusVirt_remoteExists(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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

func TestIncusVirt_startInstance(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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

func TestIncusVirt_ensureInstanceNotBusy(t *testing.T) {
	setup := func(t *testing.T) (*IncusVirt, *IncusTestMocks) {
		t.Helper()
		mocks := setupIncusMocks(t)
		incusVirt := NewIncusVirt(mocks.Runtime, []services.Service{})
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
