// The ColimaVirt is the VirtualMachine that runs the workstation VM with Colima.
// It writes the Colima configuration, and it starts and deletes the VM of the context.
// The VM size comes from the cluster node sizes, or from half the host resources without a cluster.
// The profile name of the VM is windsor-<context>.

package virt

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"net"
	"path/filepath"
	"strings"
	"time"

	colimaConfig "github.com/abiosoft/colima/config"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime"
)

// =============================================================================
// Constants
// =============================================================================

// colimaNoAddressHint is the remediation for a Colima VM that gets no IP address after it starts.
const colimaNoAddressHint = "Run colima list to see the VM state. Then run windsor down --clean and windsor up again."

// testForceMemoryOverflow makes getDefaultValues use the overflow branch. Only tests set it.
var testForceMemoryOverflow = false

// testRetryAttempts is the number of times startColima reads the VM info. Tests lower it.
var testRetryAttempts = 10

// =============================================================================
// Types
// =============================================================================

// ColimaVirt is the VirtualMachine for Colima.
type ColimaVirt struct {
	*BaseVirt
}

// =============================================================================
// Constructor
// =============================================================================

// NewColimaVirt returns a ColimaVirt for rt. It panics when rt is nil.
func NewColimaVirt(rt *runtime.Runtime) *ColimaVirt {
	if rt == nil {
		panic("runtime is required")
	}

	return &ColimaVirt{
		BaseVirt: NewBaseVirt(rt),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Up starts the Colima VM and configures its network settings.
// It initializes the VM with the appropriate configuration and waits for it to become ready.
// If the VM is already running, it skips the start operation and reuses the existing VM.
// The VM address is set in the configuration handler for later use.
// Kills stuck processes before starting if VM is not already running to prevent vmnet/daemon issues.
// Returns an error if the VM fails to start or if the address cannot be set.
func (v *ColimaVirt) Up(ctx context.Context) error {
	info, err := v.getVMInfo()
	if err == nil && info.Address != "" {
		if err := v.configHandler.Set("workstation.address", info.Address); err != nil {
			return werror.Wrap(err, "failed to set VM address in config handler")
		}
		return nil
	}

	return presenter.Track(ctx, "vm:colima", "Creating Colima VM", func(ctx context.Context) error {
		info, err = v.startColima()
		if err != nil {
			return werror.Wrap(err, "failed to start Colima VM")
		}
		if info.Address != "" {
			if err := v.configHandler.Set("workstation.address", info.Address); err != nil {
				return werror.Wrap(err, "failed to set VM address in config handler")
			}
		}
		return nil
	})
}

// Down stops and deletes the Colima VM, ensuring resources are reclaimed.
// Attempts graceful shutdown, then deletes the VM. Returns an error if deletion fails.
func (v *ColimaVirt) Down(ctx context.Context) error {
	return presenter.Track(ctx, "vm:colima", "Deleting Colima VM", func(ctx context.Context) error {
		contextName := v.configHandler.GetContext()
		profileName := fmt.Sprintf("windsor-%s", contextName)
		_, _ = v.shell.ExecSilent("colima", "stop", profileName)
		return v.executeColimaCommand("delete", "--data")
	})
}

// WriteConfig writes ~/.colima/windsor-<context>/colima.yaml from the context and the host resources.
// It writes a temporary file and then renames it to the final file.
// It does nothing when workstation.runtime is not colima.
func (v *ColimaVirt) WriteConfig() error {
	context := v.configHandler.GetContext()

	if v.configHandler.GetString("workstation.runtime") != "colima" {
		return nil
	}

	cpu, disk, memory, hostname, arch := v.getDefaultValues(context)
	vmType := "qemu"
	mountType := "sshfs"
	if v.getArch() == "aarch64" {
		vmType = "vz"
		mountType = "virtiofs"
	}

	cpu = v.configHandler.GetInt("vm.cpu", cpu)
	disk = v.configHandler.GetInt("vm.disk", disk)
	memory = v.configHandler.GetInt("vm.memory", memory)

	archValue := v.configHandler.GetString("vm.arch")
	if archValue != "" {
		arch = archValue
	}

	runtime := "docker"
	nestedVirtualization := false
	if v.configHandler.GetString("platform") == "incus" {
		runtime = "incus"
		nestedVirtualization = true
	}

	colimaConfig := &colimaConfig.Config{
		CPU:      cpu,
		Disk:     disk,
		Memory:   float32(memory),
		Arch:     arch,
		Runtime:  runtime,
		Hostname: hostname,
		Kubernetes: colimaConfig.Kubernetes{
			Enabled: false,
		},
		ActivateRuntime: ptrBool(true),
		Network: func() colimaConfig.Network {
			network := colimaConfig.Network{
				Address:         true,
				DNSResolvers:    []net.IP{},
				DNSHosts:        map[string]string{},
				HostAddresses:   false,
				Mode:            "shared",
				BridgeInterface: "",
				PreferredRoute:  false,
			}
			return network
		}(),
		ForwardAgent:         false,
		VMType:               vmType,
		VZRosetta:            false,
		NestedVirtualization: nestedVirtualization,
		MountType:            mountType,
		MountINotify:         true,
		CPUType:              "",
		Provision:            v.getProvisionScripts(),
		SSHConfig:            true,
		SSHPort:              0,
		Mounts:               colimaMountsForProjectRoot(v.BaseVirt.projectRoot),
		Env:                  map[string]string{},
	}

	homeDir, err := v.BaseVirt.shims.UserHomeDir()
	if err != nil {
		return werror.Wrap(err, "error retrieving user home directory")
	}
	colimaDir := filepath.Join(homeDir, fmt.Sprintf(".colima/windsor-%s", context))
	if err := v.BaseVirt.shims.MkdirAll(colimaDir, 0755); err != nil {
		return werror.Wrap(err, "error creating colima directory")
	}
	tempFilePath := filepath.Join(colimaDir, "colima.yaml.tmp")

	var buf bytes.Buffer
	encoder := v.BaseVirt.shims.NewYAMLEncoder(&buf)
	if err := encoder.Encode(colimaConfig); err != nil {
		return werror.Wrap(err, "error encoding yaml")
	}
	if err := encoder.Close(); err != nil {
		return werror.Wrap(err, "error closing encoder")
	}

	if err := v.BaseVirt.shims.WriteFile(tempFilePath, buf.Bytes(), 0644); err != nil {
		return werror.Wrap(err, "error writing to temporary file")
	}
	defer func() { _ = v.BaseVirt.shims.Remove(tempFilePath) }()

	finalFilePath := filepath.Join(colimaDir, "colima.yaml")
	if err := v.BaseVirt.shims.Rename(tempFilePath, finalFilePath); err != nil {
		return werror.Wrap(err, "error renaming temporary file to colima config file")
	}
	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// colimaMountsForProjectRoot returns Colima mount entries for the given project root.
// When projectRoot is non-empty (after trimming), returns a single mount with Location set to the trimmed and cleaned path and Writable true so bind mounts in the VM see the full repo.
// Returns nil when projectRoot is empty or only whitespace so Colima can apply its default behaviour when no mounts are specified.
func colimaMountsForProjectRoot(projectRoot string) []colimaConfig.Mount {
	trimmed := strings.TrimSpace(projectRoot)
	if trimmed == "" {
		return nil
	}
	location := filepath.Clean(trimmed)
	return []colimaConfig.Mount{
		{Location: location, Writable: true},
	}
}

// getVMInfo returns the information about the Colima VM
// Retrieves the VM details from the Colima CLI and parses the JSON output
// Uses timeout to prevent hanging if colima ls is stuck
// Converts memory and disk values from bytes to gigabytes for easier consumption
// Returns a VMInfo struct with the parsed information or an error if retrieval fails
func (v *ColimaVirt) getVMInfo() (VMInfo, error) {
	contextName := v.configHandler.GetContext()

	command := "colima"
	args := []string{"ls", "--profile", fmt.Sprintf("windsor-%s", contextName), "--json"}
	out, err := v.shell.ExecSilentWithTimeout(command, args, 5*time.Second)
	if err != nil {
		return VMInfo{}, err
	}

	var colimaData struct {
		Address string `json:"address"`
		Arch    string `json:"arch"`
		CPUs    int    `json:"cpus"`
		Disk    int    `json:"disk"`
		Memory  int    `json:"memory"`
		Name    string `json:"name"`
		Runtime string `json:"runtime"`
		Status  string `json:"status"`
	}
	if err := v.BaseVirt.shims.JsonUnmarshal([]byte(out), &colimaData); err != nil {
		return VMInfo{}, err
	}

	memoryGB := colimaData.Memory / (1024 * 1024 * 1024)
	diskGB := colimaData.Disk / (1024 * 1024 * 1024)

	vmInfo := VMInfo{
		Address: colimaData.Address,
		Arch:    colimaData.Arch,
		CPUs:    colimaData.CPUs,
		Disk:    diskGB,
		Memory:  memoryGB,
		Name:    colimaData.Name,
	}

	return vmInfo, nil
}

// getArch returns the host architecture in the Colima format: x86_64 for amd64 and aarch64 for arm64.
func (v *ColimaVirt) getArch() string {
	switch arch := v.shims.GOARCH(); arch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		return arch
	}
}

// getDefaultValues retrieves the default values for the VM properties
// Calculates CPU count and memory based on cluster node resources and services when cluster is enabled
// Falls back to system-based defaults (half of system resources) when cluster is disabled
// Sets a default disk size of 100GB
// Generates a hostname based on the context name
// Returns the calculated values for CPU, disk, memory, hostname, and architecture
func (v *ColimaVirt) getDefaultValues(context string) (int, int, int, string, string) {
	disk := 100
	hostname := fmt.Sprintf("windsor-%s", context)
	arch := v.getArch()

	clusterEnabled := v.configHandler.GetString("cluster.driver", "") != ""
	if values, err := v.configHandler.GetContextValues(); err == nil && values != nil {
		if clusterMap, ok := values["cluster"].(map[string]any); ok {
			if driver, ok := clusterMap["driver"].(string); ok {
				clusterEnabled = driver != ""
			}
		}
	}

	if clusterEnabled {
		cpu, memory := v.calculateVMResources()
		return cpu, disk, memory, hostname, arch
	}

	cpu := v.BaseVirt.shims.NumCPU() / 2
	vmStat, err := v.BaseVirt.shims.VirtualMemory()
	var memory int
	if err != nil {
		memory = 2
	} else {
		totalMemoryGB := vmStat.Total / (1024 * 1024 * 1024)
		halfMemoryGB := totalMemoryGB / 2

		if testForceMemoryOverflow || halfMemoryGB > uint64(math.MaxInt) {
			memory = math.MaxInt
		} else {
			memory = int(halfMemoryGB)
		}
	}

	return cpu, disk, memory, hostname, arch
}

// calculateVMResources returns the Colima VM CPU count and memory in GB for the cluster node topology.
// Explicit cluster.controlplanes and cluster.workers values win. Unset node sizes fall back to the
// constants package, which mirrors the defaults core applies to the nodes; update both together.
// The control plane count defaults to 1. A control plane without workers runs workloads, so it uses the schedulable defaults.
// The VM adds fixed overhead for the base system and services, and warns when it exceeds the host.
func (v *ColimaVirt) calculateVMResources() (int, int) {
	const (
		vmAndServiceCPUOverhead    = 1
		vmAndServiceMemoryOverhead = 3
		minCPU                     = 2
		minMemory                  = 4
		hostMemoryReserveGB        = 4
	)

	controlPlaneCount := v.configHandler.GetInt("cluster.controlplanes.count", 1)
	controlPlaneCPU := v.configHandler.GetInt("cluster.controlplanes.cpu", 0)
	controlPlaneMemory := v.configHandler.GetInt("cluster.controlplanes.memory", 0)

	workerCount := v.configHandler.GetInt("cluster.workers.count", 0)
	workerCPU := v.configHandler.GetInt("cluster.workers.cpu", 0)
	workerMemory := v.configHandler.GetInt("cluster.workers.memory", 0)

	if controlPlaneCPU == 0 {
		if workerCount > 0 {
			controlPlaneCPU = constants.DefaultControlPlaneCPUDedicated
		} else {
			controlPlaneCPU = constants.DefaultControlPlaneCPUSchedulable
		}
	}
	if controlPlaneMemory == 0 {
		if workerCount > 0 {
			controlPlaneMemory = constants.DefaultControlPlaneMemoryDedicated
		} else {
			controlPlaneMemory = constants.DefaultControlPlaneMemorySchedulable
		}
	}
	if workerCount > 0 {
		if workerCPU == 0 {
			workerCPU = constants.DefaultWorkerCPU
		}
		if workerMemory == 0 {
			workerMemory = constants.DefaultWorkerMemory
		}
	}

	totalNodeCPU := (controlPlaneCount * controlPlaneCPU) + (workerCount * workerCPU)
	totalNodeMemory := (controlPlaneCount * controlPlaneMemory) + (workerCount * workerMemory)

	cpu := totalNodeCPU + vmAndServiceCPUOverhead
	memory := totalNodeMemory + vmAndServiceMemoryOverhead

	if cpu < minCPU {
		cpu = minCPU
	}
	if memory < minMemory {
		memory = minMemory
	}

	v.validateVMResources(cpu, memory, hostMemoryReserveGB)

	return cpu, memory
}

// validateVMResources compares the VM resources with the host resources.
// It sends a warning to the default presenter when the VM needs more CPUs or memory than the host has.
func (v *ColimaVirt) validateVMResources(cpu, memory, hostMemoryReserveGB int) {
	hostCPU := v.BaseVirt.shims.NumCPU()
	if cpu > hostCPU {
		presenter.Warn(presenter.Background(), fmt.Sprintf(
			"Cluster configuration requires %d vCPUs, but host only has %d cores.\nReduce cluster.controlplanes.cpu or cluster.workers.cpu, or reduce the node count.",
			cpu, hostCPU))
	}

	vmStat, err := v.BaseVirt.shims.VirtualMemory()
	if err != nil {
		return
	}
	hostMemoryGBUint64 := vmStat.Total / (1024 * 1024 * 1024)
	var hostMemoryGB int
	if hostMemoryGBUint64 > uint64(math.MaxInt) {
		hostMemoryGB = math.MaxInt
	} else {
		hostMemoryGB = int(hostMemoryGBUint64)
	}
	availableMemoryGB := int(math.Max(float64(hostMemoryGB-hostMemoryReserveGB), 0))

	if memory > availableMemoryGB {
		presenter.Warn(presenter.Background(), fmt.Sprintf(
			"Cluster configuration requires %dGB memory, but only %dGB available (host has %dGB, reserving %dGB for OS).\nReduce cluster.controlplanes.memory or cluster.workers.memory, or reduce the node count.",
			memory, availableMemoryGB, hostMemoryGB, hostMemoryReserveGB))
	}
}

// executeColimaCommand runs colima <action> windsor-<context> with additionalArgs and shows progress.
// For the delete action, it adds --force.
// It returns an error with the command output when the command fails.
func (v *ColimaVirt) executeColimaCommand(action string, additionalArgs ...string) error {
	contextName := v.configHandler.GetContext()
	command := "colima"
	args := []string{action, fmt.Sprintf("windsor-%s", contextName)}
	args = append(args, additionalArgs...)
	if action == "delete" {
		args = append(args, "--force")
		output, err := v.shell.ExecProgress("Deleting Colima VM", command, args...)
		if err != nil {
			return werror.Wrap(err, "failed to run %s %v, output: %s", command, args, output)
		}
		return nil
	}
	output, err := v.shell.ExecProgress(fmt.Sprintf("Running %s", command), command, args...)
	if err != nil {
		return werror.Wrap(err, "failed to run %s %v, output: %s", command, args, output)
	}
	return nil
}

// startColima starts the Colima VM and returns the VM info after the VM gets an IP address.
// It runs colima start, then gets the VM info again until the VM has an address or the attempts end.
// If colima start fails, the error includes the command output.
// If the VM gets no address, it returns an error with a remediation hint.
func (v *ColimaVirt) startColima() (VMInfo, error) {
	contextName := v.configHandler.GetContext()

	command := "colima"
	args := []string{"start", fmt.Sprintf("windsor-%s", contextName)}

	output, err := v.shell.ExecProgress(fmt.Sprintf("Running %s", command), command, args...)
	if err != nil {
		return VMInfo{}, werror.Wrap(err, "failed to run %s %v, output: %s", command, args, output)
	}

	var info VMInfo
	var lastErr error
	for i := range make([]int, testRetryAttempts) {
		info, err = v.getVMInfo()
		if err != nil {
			lastErr = werror.Wrap(err, "failed to retrieve Colima info")
			time.Sleep(time.Duration(RETRY_WAIT*(i+1)) * time.Second)
			continue
		}
		if info.Address != "" {
			return info, nil
		}
		time.Sleep(time.Duration(RETRY_WAIT*(i+1)) * time.Second)
	}

	if lastErr != nil {
		return VMInfo{}, lastErr
	}

	return VMInfo{}, werror.New("timed out waiting for Colima VM to get an IP address", colimaNoAddressHint, nil)
}

// getProvisionScripts returns the provision scripts that Colima runs in the VM.
func (v *ColimaVirt) getProvisionScripts() []colimaConfig.Provision {
	return []colimaConfig.Provision{
		{
			Mode:   "system",
			Script: "modprobe br_netfilter",
		},
	}
}

// getProfileName returns the Colima profile name for the current context.
func (v *ColimaVirt) getProfileName() string {
	contextName := v.configHandler.GetContext()
	return fmt.Sprintf("windsor-%s", contextName)
}

// execInVM runs a command in the VM through colima ssh without output and returns its stdout.
// The command discards its own stderr, so the colima ssh connection messages do not appear.
func (v *ColimaVirt) execInVM(command string, args ...string) (string, error) {
	profileName := v.getProfileName()
	fullCommand := command
	if len(args) > 0 {
		fullCommand += " " + strings.Join(args, " ")
	}
	return v.shell.ExecSilent("colima", "ssh", "--profile", profileName, "--", "sh", "-c", fullCommand+" 2>/dev/null </dev/null")
}

// execInVMQuiet runs a command in the VM through colima ssh with a timeout and returns its stdout.
// Callers use it for queries that return large JSON documents.
func (v *ColimaVirt) execInVMQuiet(command string, args []string, timeout time.Duration) (string, error) {
	profileName := v.getProfileName()
	fullCommand := command
	if len(args) > 0 {
		fullCommand += " " + strings.Join(args, " ")
	}
	output, err := v.shell.ExecSilentWithTimeout("colima", []string{"ssh", "--profile", profileName, "--", "sh", "-c", fullCommand + " 2>/dev/null </dev/null"}, timeout)
	return output, err
}

// execInVMProgress runs a command in the VM through colima ssh, shows message as progress, and returns the output.
func (v *ColimaVirt) execInVMProgress(message string, command string, args ...string) (string, error) {
	profileName := v.getProfileName()
	fullCommand := command
	if len(args) > 0 {
		fullCommand += " " + strings.Join(args, " ")
	}
	return v.shell.ExecProgress(message, "colima", "ssh", "--profile", profileName, "--", "sh", "-c", fullCommand)
}

// =============================================================================
// Helpers
// =============================================================================

// ptrBool returns a pointer to b.
func ptrBool(b bool) *bool {
	return &b
}

// =============================================================================
// Interface Compliance
// =============================================================================

// ColimaVirt implements Virt and VirtualMachine.
var _ Virt = (*ColimaVirt)(nil)
var _ VirtualMachine = (*ColimaVirt)(nil)
