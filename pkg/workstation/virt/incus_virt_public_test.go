package virt_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// =============================================================================
// Test Setup
// =============================================================================

// setupIncusVirt returns a fixture for platform incus and an IncusVirt that uses the stub shims of the fixture.
func setupIncusVirt(t *testing.T) (*fixture, *virt.IncusVirt) {
	t.Helper()
	f := newFixture(t)
	f.values["workstation.runtime"] = "colima"
	f.values["platform"] = "incus"
	incusVirt := virt.NewIncusVirt(f.runtime)
	incusVirt.SetShims(f.shims)
	return f, incusVirt
}

// runningVM returns an ExecSilentWithTimeout stub for a running VM with remoteList as the incus remote list output.
func runningVM(remoteList string, remoteErr error) func(string, []string, time.Duration) (string, error) {
	return func(command string, args []string, _ time.Duration) (string, error) {
		if strings.Contains(strings.Join(args, " "), "incus remote list") {
			return remoteList, remoteErr
		}
		return `{"address":"192.168.5.2"}`, nil
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestIncusVirt_Up(t *testing.T) {
	t.Run("StartsTheVMWhenItIsNotRunning", func(t *testing.T) {
		// Given no running VM, and a VM that gets an address after colima start
		f, incusVirt := setupIncusVirt(t)
		running := false
		f.shell.ExecSilentWithTimeoutFunc = func(string, []string, time.Duration) (string, error) {
			if !running {
				return "", errors.New("profile not found")
			}
			return `{"address":"192.168.5.3"}`, nil
		}
		var commands []string
		f.shell.ExecProgressFunc = func(_ string, command string, args ...string) (string, error) {
			commands = append(commands, commandLine(command, args...))
			running = true
			return "", nil
		}

		// When running Up
		err := incusVirt.Up(f.ctx)

		// Then the Colima VM starts and its address is stored
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !reflect.DeepEqual(commands, []string{"colima start windsor-local"}) {
			t.Errorf("Expected colima start windsor-local, got %v", commands)
		}
		if f.values["workstation.address"] != "192.168.5.3" {
			t.Errorf("Expected workstation.address 192.168.5.3, got %v", f.values["workstation.address"])
		}
	})

	t.Run("AddsOnlyTheMissingRemotes", func(t *testing.T) {
		// Given a running VM that has the docker remote and not the ghcr remote
		f, incusVirt := setupIncusVirt(t)
		f.shell.ExecSilentWithTimeoutFunc = runningVM(`{"docker":{"addr":"https://docker.io"}}`, nil)
		var commands []string
		f.shell.ExecProgressFunc = func(_ string, command string, args ...string) (string, error) {
			commands = append(commands, commandLine(command, args...))
			return "", nil
		}

		// When running Up
		err := incusVirt.Up(f.ctx)

		// Then only the ghcr remote is added in the VM
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(commands) != 1 || !strings.Contains(commands[0], "incus remote add ghcr https://ghcr.io --protocol oci --public") {
			t.Errorf("Expected one ghcr remote add, got %v", commands)
		}
	})

	t.Run("ReturnsErrorWhenRemoteListFails", func(t *testing.T) {
		// Given a running VM whose incus remote list fails
		f, incusVirt := setupIncusVirt(t)
		f.shell.ExecSilentWithTimeoutFunc = runningVM("", errors.New("ssh failed"))

		// When running Up
		err := incusVirt.Up(f.ctx)

		// Then the error names the docker remote
		if err == nil || !strings.Contains(err.Error(), "failed to ensure docker remote") {
			t.Errorf("Expected a docker remote error, got %v", err)
		}
	})
}

func TestIncusVirt_Down(t *testing.T) {
	t.Run("DeletesOnlyTheColimaVMOffIncus", func(t *testing.T) {
		// Given platform docker
		f, incusVirt := setupIncusVirt(t)
		f.values["platform"] = "docker"

		// When running Down
		err := incusVirt.Down(f.ctx)

		// Then only the Colima VM step runs
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		wantSteps := []string{"applying:Deleting Colima VM", "applied:Deleting Colima VM"}
		if !reflect.DeepEqual(f.recorder.steps(), wantSteps) {
			t.Errorf("Expected steps %v, got %v", wantSteps, f.recorder.steps())
		}
	})

	t.Run("StopsTheDaemonAndDeletesTheVMOnIncus", func(t *testing.T) {
		// Given platform incus
		f, incusVirt := setupIncusVirt(t)
		var commands []string
		f.shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			commands = append(commands, commandLine(command, args...))
			return "", nil
		}

		// When running Down
		err := incusVirt.Down(f.ctx)

		// Then the daemon stops first, and the Colima VM step runs inside the Incus step
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(commands) == 0 || commands[0] != "colima daemon stop windsor-local" {
			t.Errorf("Expected colima daemon stop first, got %v", commands)
		}
		wantSteps := []string{
			"applying:Deleting Incus VM",
			"applying:Deleting Colima VM",
			"applied:Deleting Colima VM",
			"applied:Deleting Incus VM",
		}
		if !reflect.DeepEqual(f.recorder.steps(), wantSteps) {
			t.Errorf("Expected steps %v, got %v", wantSteps, f.recorder.steps())
		}
	})
	t.Run("ClearsStuckProcessesWhenDeleteFails", func(t *testing.T) {
		// Given a colima delete that fails and a daemon PID file and a vmnet socket that exist
		f, incusVirt := setupIncusVirt(t)
		daemonDir := filepath.Join(f.home, ".colima", "windsor-local", "daemon")
		if err := os.MkdirAll(daemonDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"daemon.pid", "vmnet.sock"} {
			if err := os.WriteFile(filepath.Join(daemonDir, name), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		f.shell.ExecProgressFunc = func(string, string, ...string) (string, error) {
			return "", errors.New("exit status 1")
		}
		var commands []string
		f.shell.ExecSilentWithTimeoutFunc = func(command string, args []string, _ time.Duration) (string, error) {
			commands = append(commands, commandLine(command, args...))
			return "", nil
		}

		// When running Down
		err := incusVirt.Down(f.ctx)

		// Then the delete error returns, the stuck processes stop, and both files are gone
		if err == nil {
			t.Fatal("Expected the delete error")
		}
		want := []string{"colima daemon stop windsor-local", "limactl stop --force colima-windsor-local"}
		if !reflect.DeepEqual(commands, want) {
			t.Errorf("Expected commands %v, got %v", want, commands)
		}
		for _, name := range []string{"daemon.pid", "vmnet.sock"} {
			if _, err := os.Stat(filepath.Join(daemonDir, name)); !os.IsNotExist(err) {
				t.Errorf("Expected %s to be removed, got %v", name, err)
			}
		}
	})
}
