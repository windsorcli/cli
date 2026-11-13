package virt_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	colimaConfig "github.com/abiosoft/colima/config"
	"github.com/goccy/go-yaml"
	"github.com/shirou/gopsutil/mem"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// =============================================================================
// Test Setup
// =============================================================================

// setupColimaVirt returns a fixture and a ColimaVirt that uses the stub shims of the fixture.
func setupColimaVirt(t *testing.T) (*fixture, *virt.ColimaVirt) {
	t.Helper()
	f := newFixture(t)
	f.values["workstation.runtime"] = "colima"
	colimaVirt := virt.NewColimaVirt(f.runtime)
	colimaVirt.SetShims(f.shims)
	return f, colimaVirt
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewColimaVirt(t *testing.T) {
	t.Run("PanicsWhenRuntimeIsNil", func(t *testing.T) {
		// Given no runtime
		defer func() {
			// Then NewColimaVirt panics
			if recover() == nil {
				t.Error("Expected a panic for a nil runtime")
			}
		}()

		// When creating a ColimaVirt
		virt.NewColimaVirt(nil)
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestColimaVirt_WriteConfig(t *testing.T) {
	t.Run("DoesNothingWhenRuntimeIsNotColima", func(t *testing.T) {
		// Given a context whose workstation runtime is docker-desktop
		f, colimaVirt := setupColimaVirt(t)
		f.values["workstation.runtime"] = "docker-desktop"

		// When writing the config
		err := colimaVirt.WriteConfig()

		// Then no config file exists
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, err := os.Stat(colimaConfigPath(f)); !os.IsNotExist(err) {
			t.Errorf("Expected no config file, got stat error %v", err)
		}
	})

	t.Run("WritesHalfTheHostResourcesWithoutCluster", func(t *testing.T) {
		// Given an arm64 host with 8 CPUs and 16GB of memory and no cluster
		f, colimaVirt := setupColimaVirt(t)

		// When writing the config
		err := colimaVirt.WriteConfig()

		// Then the config uses half the host resources, vz, and a writable project mount
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		cfg := readColimaConfig(t, f)
		if cfg.CPU != 4 || cfg.Memory != 8 || cfg.Disk != 100 {
			t.Errorf("Expected 4 CPUs, 8GB memory, and 100GB disk, got %d, %v, %d", cfg.CPU, cfg.Memory, cfg.Disk)
		}
		if cfg.Arch != "aarch64" || cfg.VMType != "vz" || cfg.MountType != "virtiofs" {
			t.Errorf("Expected aarch64, vz, and virtiofs, got %s, %s, %s", cfg.Arch, cfg.VMType, cfg.MountType)
		}
		if cfg.Hostname != "windsor-local" || cfg.Runtime != "docker" || cfg.NestedVirtualization {
			t.Errorf("Expected hostname windsor-local and runtime docker without nesting, got %+v", cfg)
		}
		wantMounts := []colimaConfig.Mount{{Location: filepath.Clean("/work/project"), Writable: true}}
		if !reflect.DeepEqual(cfg.Mounts, wantMounts) {
			t.Errorf("Expected mounts %+v, got %+v", wantMounts, cfg.Mounts)
		}
		if len(cfg.Provision) != 1 || cfg.Provision[0].Script != "modprobe br_netfilter" {
			t.Errorf("Expected the br_netfilter provision script, got %+v", cfg.Provision)
		}
		if !cfg.Network.Address || cfg.Network.Mode != "shared" {
			t.Errorf("Expected a shared network with an address, got %+v", cfg.Network)
		}
	})

	t.Run("UsesQemuOnAmd64", func(t *testing.T) {
		// Given an amd64 host
		f, colimaVirt := setupColimaVirt(t)
		f.shims.GOARCH = func() string { return "amd64" }

		// When writing the config
		err := colimaVirt.WriteConfig()

		// Then the config uses x86_64, qemu, and sshfs
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		cfg := readColimaConfig(t, f)
		if cfg.Arch != "x86_64" || cfg.VMType != "qemu" || cfg.MountType != "sshfs" {
			t.Errorf("Expected x86_64, qemu, and sshfs, got %s, %s, %s", cfg.Arch, cfg.VMType, cfg.MountType)
		}
	})

	t.Run("UsesVMConfigValues", func(t *testing.T) {
		// Given vm values in the context
		f, colimaVirt := setupColimaVirt(t)
		f.values["vm.cpu"] = 6
		f.values["vm.memory"] = 12
		f.values["vm.disk"] = 50
		f.values["vm.arch"] = "x86_64"

		// When writing the config
		err := colimaVirt.WriteConfig()

		// Then the config uses the vm values
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		cfg := readColimaConfig(t, f)
		if cfg.CPU != 6 || cfg.Memory != 12 || cfg.Disk != 50 || cfg.Arch != "x86_64" {
			t.Errorf("Expected 6 CPUs, 12GB, 50GB, and x86_64, got %d, %v, %d, %s", cfg.CPU, cfg.Memory, cfg.Disk, cfg.Arch)
		}
	})

	t.Run("EnablesNestedVirtualizationForIncus", func(t *testing.T) {
		// Given platform incus
		f, colimaVirt := setupColimaVirt(t)
		f.values["platform"] = "incus"

		// When writing the config
		err := colimaVirt.WriteConfig()

		// Then the config uses the incus runtime with nested virtualization
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		cfg := readColimaConfig(t, f)
		if cfg.Runtime != "incus" || !cfg.NestedVirtualization {
			t.Errorf("Expected runtime incus with nesting, got %s and %v", cfg.Runtime, cfg.NestedVirtualization)
		}
	})

	t.Run("SizesTheVMFromClusterNodes", func(t *testing.T) {
		// Given a cluster with one control plane and one worker of 2 CPUs and 4GB each
		f, colimaVirt := setupColimaVirt(t)
		f.values["cluster.driver"] = "talos"
		f.values["cluster.controlplanes.cpu"] = 2
		f.values["cluster.controlplanes.memory"] = 4
		f.values["cluster.workers.count"] = 1
		f.values["cluster.workers.cpu"] = 2
		f.values["cluster.workers.memory"] = 4

		// When writing the config
		err := colimaVirt.WriteConfig()

		// Then the VM has the node resources plus 1 CPU and 3GB of overhead, and no warning shows
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		cfg := readColimaConfig(t, f)
		if cfg.CPU != 5 || cfg.Memory != 11 {
			t.Errorf("Expected 5 CPUs and 11GB, got %d and %v", cfg.CPU, cfg.Memory)
		}
		if warnings := f.recorder.warnings(); len(warnings) != 0 {
			t.Errorf("Expected no warnings, got %q", warnings)
		}
	})

	t.Run("WarnsWhenClusterExceedsTheHost", func(t *testing.T) {
		// Given a cluster that needs 15 CPUs and 19GB on a host with 8 CPUs and 16GB
		f, colimaVirt := setupColimaVirt(t)
		f.values["cluster.driver"] = "talos"
		f.values["cluster.controlplanes.cpu"] = 2
		f.values["cluster.controlplanes.memory"] = 4
		f.values["cluster.workers.count"] = 3
		f.values["cluster.workers.cpu"] = 4
		f.values["cluster.workers.memory"] = 4

		// When writing the config
		err := colimaVirt.WriteConfig()

		// Then one CPU warning and one memory warning show, and the config is still written
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		warnings := f.recorder.warnings()
		if len(warnings) != 2 {
			t.Fatalf("Expected 2 warnings, got %q", warnings)
		}
		if !strings.Contains(warnings[0], "15 vCPUs") || !strings.Contains(warnings[0], "8 cores") {
			t.Errorf("Expected a CPU warning for 15 vCPUs and 8 cores, got %q", warnings[0])
		}
		if !strings.Contains(warnings[1], "19GB memory") || !strings.Contains(warnings[1], "12GB available") {
			t.Errorf("Expected a memory warning for 19GB and 12GB available, got %q", warnings[1])
		}
		if cfg := readColimaConfig(t, f); cfg.CPU != 15 {
			t.Errorf("Expected 15 CPUs, got %d", cfg.CPU)
		}
	})

	t.Run("ShowsZeroAvailableMemoryOnSmallHost", func(t *testing.T) {
		// Given a cluster on a host with 2GB of memory, less than the 4GB reserve
		f, colimaVirt := setupColimaVirt(t)
		f.values["cluster.driver"] = "talos"
		f.shims.VirtualMemory = func() (*mem.VirtualMemoryStat, error) {
			return &mem.VirtualMemoryStat{Total: 2 * 1024 * 1024 * 1024}, nil
		}

		// When writing the config
		err := colimaVirt.WriteConfig()

		// Then the memory warning shows 0GB available
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		warnings := f.recorder.warnings()
		if len(warnings) != 1 || !strings.Contains(warnings[0], "0GB available") {
			t.Errorf("Expected one warning with 0GB available, got %q", warnings)
		}
	})

	t.Run("ReturnsErrorWhenHomeDirectoryFails", func(t *testing.T) {
		// Given a home directory lookup that fails
		f, colimaVirt := setupColimaVirt(t)
		f.shims.UserHomeDir = func() (string, error) { return "", errors.New("no home") }

		// When writing the config
		err := colimaVirt.WriteConfig()

		// Then the error names the home directory
		if err == nil || !strings.Contains(err.Error(), "error retrieving user home directory") {
			t.Errorf("Expected a home directory error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenWriteFails", func(t *testing.T) {
		// Given a file write that fails
		f, colimaVirt := setupColimaVirt(t)
		f.shims.WriteFile = func(string, []byte, os.FileMode) error { return errors.New("disk full") }

		// When writing the config
		err := colimaVirt.WriteConfig()

		// Then the error names the temporary file
		if err == nil || !strings.Contains(err.Error(), "error writing to temporary file") {
			t.Errorf("Expected a temporary file error, got %v", err)
		}
	})
}

func TestColimaVirt_Up(t *testing.T) {
	t.Run("ReusesARunningVM", func(t *testing.T) {
		// Given a VM that is already running with an address
		f, colimaVirt := setupColimaVirt(t)
		f.shell.ExecSilentWithTimeoutFunc = func(string, []string, time.Duration) (string, error) {
			return `{"address":"192.168.5.2","arch":"aarch64"}`, nil
		}
		started := false
		f.shell.ExecProgressFunc = func(string, string, ...string) (string, error) {
			started = true
			return "", nil
		}

		// When starting the VM
		err := colimaVirt.Up(f.ctx)

		// Then the VM address is stored and colima start does not run
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if f.values["workstation.address"] != "192.168.5.2" {
			t.Errorf("Expected workstation.address 192.168.5.2, got %v", f.values["workstation.address"])
		}
		if started || len(f.recorder.steps()) != 0 {
			t.Errorf("Expected no start and no steps, got started=%v steps=%v", started, f.recorder.steps())
		}
	})

	t.Run("ReusesARunningVMWithTheSameMount", func(t *testing.T) {
		// Given a running VM whose Lima instance mounts the project root
		f, colimaVirt := setupColimaVirt(t)
		f.shell.ExecSilentWithTimeoutFunc = func(string, []string, time.Duration) (string, error) {
			return `{"address":"192.168.5.2"}`, nil
		}
		writeLimaConfig(t, f, "mounts:\n  - location: /cache/colima\n  - location: /work/project/\n    writable: true\n")

		// When starting the VM
		err := colimaVirt.Up(f.ctx)

		// Then the VM is reused without an error
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if f.values["workstation.address"] != "192.168.5.2" {
			t.Errorf("Expected workstation.address 192.168.5.2, got %v", f.values["workstation.address"])
		}
	})

	t.Run("ReturnsErrorWhenRunningMountDiffers", func(t *testing.T) {
		// Given a running VM whose Lima instance mounts an old project root
		f, colimaVirt := setupColimaVirt(t)
		f.shell.ExecSilentWithTimeoutFunc = func(string, []string, time.Duration) (string, error) {
			return `{"address":"192.168.5.2"}`, nil
		}
		writeLimaConfig(t, f, "mounts:\n  - location: /work/old-project/\n    writable: true\n")

		// When starting the VM
		err := colimaVirt.Up(f.ctx)

		// Then the error names the mount and tells the operator to restart the VM
		var wErr *werror.WindsorError
		if !errors.As(err, &wErr) {
			t.Fatalf("Expected a WindsorError, got %v", err)
		}
		if !strings.Contains(wErr.Error(), "/work/project") {
			t.Errorf("Expected the error to name /work/project, got %q", wErr.Error())
		}
		if !strings.Contains(wErr.Remediation, "colima stop windsor-local") {
			t.Errorf("Expected the remediation to name colima stop windsor-local, got %q", wErr.Remediation)
		}
		if _, set := f.values["workstation.address"]; set {
			t.Error("Expected workstation.address not to be set")
		}
	})

	t.Run("StartsTheVMWhenItIsNotRunning", func(t *testing.T) {
		// Given no running VM, and a VM that gets an address after colima start
		f, colimaVirt := setupColimaVirt(t)
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

		// When starting the VM
		err := colimaVirt.Up(f.ctx)

		// Then colima start runs in one step and the new address is stored
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !reflect.DeepEqual(commands, []string{"colima start windsor-local"}) {
			t.Errorf("Expected colima start windsor-local, got %v", commands)
		}
		if f.values["workstation.address"] != "192.168.5.3" {
			t.Errorf("Expected workstation.address 192.168.5.3, got %v", f.values["workstation.address"])
		}
		wantSteps := []string{"applying:Creating Colima VM", "applied:Creating Colima VM"}
		if !reflect.DeepEqual(f.recorder.steps(), wantSteps) {
			t.Errorf("Expected steps %v, got %v", wantSteps, f.recorder.steps())
		}
	})

	t.Run("ReturnsErrorWhenStartFails", func(t *testing.T) {
		// Given no running VM and a colima start that fails
		f, colimaVirt := setupColimaVirt(t)
		f.shell.ExecSilentWithTimeoutFunc = func(string, []string, time.Duration) (string, error) {
			return "", errors.New("profile not found")
		}
		f.shell.ExecProgressFunc = func(string, string, ...string) (string, error) {
			return "boom output", errors.New("exit status 1")
		}

		// When starting the VM
		err := colimaVirt.Up(f.ctx)

		// Then the error names the start, and the step fails
		if err == nil || !strings.Contains(err.Error(), "failed to start Colima VM") {
			t.Fatalf("Expected a start error, got %v", err)
		}
		wantSteps := []string{"applying:Creating Colima VM", "failed:Creating Colima VM"}
		if !reflect.DeepEqual(f.recorder.steps(), wantSteps) {
			t.Errorf("Expected steps %v, got %v", wantSteps, f.recorder.steps())
		}
	})
	t.Run("ReturnsUnlockHintWhenLimaDiskIsLocked", func(t *testing.T) {
		// Given no running VM and a colima start that fails on a stale Lima disk lock
		f, colimaVirt := setupColimaVirt(t)
		f.shell.ExecSilentWithTimeoutFunc = func(string, []string, time.Duration) (string, error) {
			return "", errors.New("profile not found")
		}
		f.shell.ExecProgressFunc = func(string, string, ...string) (string, error) {
			return "level=error msg=\"[hostagent] could not attach disk `colima-windsor-local`: in use by instance `colima-windsor-local`\"", errors.New("exit status 1")
		}

		// When starting the VM
		err := colimaVirt.Up(f.ctx)

		// Then the remediation is the limactl disk unlock command with the disk name and the Lima home
		var wErr *werror.WindsorError
		if !errors.As(err, &wErr) {
			t.Fatalf("Expected a WindsorError, got %v", err)
		}
		limaHome := filepath.Join(f.home, ".colima", "_lima")
		want := "LIMA_HOME=" + limaHome + " limactl disk unlock colima-windsor-local"
		if !strings.Contains(wErr.Remediation, want) {
			t.Errorf("Expected the remediation to contain %q, got %q", want, wErr.Remediation)
		}
	})

	t.Run("UsesTheProfileDiskWhenLimaDoesNotNameTheDisk", func(t *testing.T) {
		// Given a colima start that fails with a disk lock and no disk name
		f, colimaVirt := setupColimaVirt(t)
		f.shell.ExecSilentWithTimeoutFunc = func(string, []string, time.Duration) (string, error) {
			return "", errors.New("profile not found")
		}
		f.shell.ExecProgressFunc = func(string, string, ...string) (string, error) {
			return "in use by instance", errors.New("exit status 1")
		}

		// When starting the VM
		err := colimaVirt.Up(f.ctx)

		// Then the remediation names the data disk of the profile
		var wErr *werror.WindsorError
		if !errors.As(err, &wErr) {
			t.Fatalf("Expected a WindsorError, got %v", err)
		}
		if !strings.Contains(wErr.Remediation, "limactl disk unlock colima-windsor-local") {
			t.Errorf("Expected the remediation to name colima-windsor-local, got %q", wErr.Remediation)
		}
	})
}

func TestColimaVirt_Down(t *testing.T) {
	t.Run("StopsAndDeletesTheVM", func(t *testing.T) {
		// Given a VM for the context
		f, colimaVirt := setupColimaVirt(t)
		var commands []string
		f.shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			commands = append(commands, commandLine(command, args...))
			return "", nil
		}
		f.shell.ExecProgressFunc = func(_ string, command string, args ...string) (string, error) {
			commands = append(commands, commandLine(command, args...))
			return "", nil
		}

		// When stopping the VM
		err := colimaVirt.Down(f.ctx)

		// Then colima stops and deletes the profile in one step
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		want := []string{"colima stop windsor-local", "colima delete windsor-local --data --force"}
		if !reflect.DeepEqual(commands, want) {
			t.Errorf("Expected commands %v, got %v", want, commands)
		}
		wantSteps := []string{"applying:Deleting Colima VM", "applied:Deleting Colima VM"}
		if !reflect.DeepEqual(f.recorder.steps(), wantSteps) {
			t.Errorf("Expected steps %v, got %v", wantSteps, f.recorder.steps())
		}
	})

	t.Run("ReturnsErrorWhenDeleteFails", func(t *testing.T) {
		// Given a colima delete that fails
		f, colimaVirt := setupColimaVirt(t)
		f.shell.ExecProgressFunc = func(string, string, ...string) (string, error) {
			return "busy", errors.New("exit status 1")
		}

		// When stopping the VM
		err := colimaVirt.Down(f.ctx)

		// Then the error includes the command output
		if err == nil || !strings.Contains(err.Error(), "output: busy") {
			t.Errorf("Expected a delete error with the output, got %v", err)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

// colimaConfigPath returns the path of the Colima config file for the fixture context.
func colimaConfigPath(f *fixture) string {
	return filepath.Join(f.home, ".colima", "windsor-local", "colima.yaml")
}

// readColimaConfig reads and decodes the Colima config file of the fixture context.
func readColimaConfig(t *testing.T, f *fixture) colimaConfig.Config {
	t.Helper()
	data, err := os.ReadFile(colimaConfigPath(f))
	if err != nil {
		t.Fatalf("Expected a config file, got %v", err)
	}
	var cfg colimaConfig.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Expected valid YAML, got %v", err)
	}
	return cfg
}
