// The ColimaVirt is a virtual machine implementation
// It provides VM management capabilities through the Colima interface
// It serves as the primary VM orchestration layer for the Windsor CLI
// It handles VM lifecycle, resource allocation, and networking for Colima-based VMs

package virt

import (
	"bytes"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	colimaConfig "github.com/abiosoft/colima/config"
	"github.com/windsorcli/cli/pkg/runtime"
)

// Test hook to force memory overflow
var testForceMemoryOverflow = false

// Test hook to control retry attempts
var testRetryAttempts = 10

// =============================================================================
// Types
// =============================================================================

// ColimaVirt implements the VirtInterface and VMInterface for Colima
type ColimaVirt struct {
	*BaseVirt
}

// =============================================================================
// Constructor
// =============================================================================

// NewColimaVirt creates a new instance of ColimaVirt
func NewColimaVirt(rt *runtime.Runtime) *ColimaVirt {
	return &ColimaVirt{
		BaseVirt: NewBaseVirt(rt),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Up starts the Colima VM and configures its network settings
// Initializes the VM with the appropriate configuration and waits for it to be ready
// Sets the VM address in the configuration handler for later use
// For incus runtime, starts the vmnet daemon and retries getting the address
// Returns an error if the VM fails to start or if the address cannot be set
func (v *ColimaVirt) Up() error {
	info, err := v.startColima()
	if err != nil {
		return fmt.Errorf("failed to start Colima VM: %w", err)
	}

	vmAddress := info.Address

	if vmAddress != "" {
		if err := v.configHandler.Set("vm.address", vmAddress); err != nil {
			return fmt.Errorf("failed to set VM address in config handler: %w", err)
		}
	}

	return nil
}

// Down stops and deletes the Colima VM
// First stops the VM and then deletes it to ensure a clean shutdown
// Returns an error if either the stop or delete operation fails
func (v *ColimaVirt) Down() error {
	if err := v.executeColimaCommand("stop"); err != nil {
		return err
	}

	return v.executeColimaCommand("delete")
}

// getVMInfo returns the information about the Colima VM
// Retrieves the VM details from the Colima CLI and parses the JSON output
// Converts memory and disk values from bytes to gigabytes for easier consumption
// Returns a VMInfo struct with the parsed information or an error if retrieval fails
func (v *ColimaVirt) getVMInfo() (VMInfo, error) {
	contextName := v.configHandler.GetContext()

	command := "colima"
	args := []string{"ls", "--profile", fmt.Sprintf("windsor-%s", contextName), "--json"}
	out, err := v.shell.ExecSilent(command, args...)
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
	if err := v.BaseVirt.shims.UnmarshalJSON([]byte(out), &colimaData); err != nil {
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

// WriteConfig writes the Colima configuration file with VM settings
// Generates a configuration based on the current context and system properties
// Creates a temporary file and then renames it to the final configuration file
// Returns an error if any step of the configuration process fails
func (v *ColimaVirt) WriteConfig() error {
	context := v.configHandler.GetContext()

	if v.configHandler.GetString("vm.driver") != "colima" {
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

	vmRuntime := v.configHandler.GetString("vm.runtime", "docker")
	runtime := vmRuntime
	nestedVirtualization := false

	if vmRuntime == "incus" {
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
		Mounts:               []colimaConfig.Mount{},
		Env:                  map[string]string{},
	}

	homeDir, err := v.BaseVirt.shims.UserHomeDir()
	if err != nil {
		return fmt.Errorf("error retrieving user home directory: %w", err)
	}
	colimaDir := filepath.Join(homeDir, fmt.Sprintf(".colima/windsor-%s", context))
	if err := v.BaseVirt.shims.MkdirAll(colimaDir, 0755); err != nil {
		return fmt.Errorf("error creating colima directory: %w", err)
	}
	tempFilePath := filepath.Join(colimaDir, "colima.yaml.tmp")

	var buf bytes.Buffer
	encoder := v.BaseVirt.shims.NewYAMLEncoder(&buf)
	if err := encoder.Encode(colimaConfig); err != nil {
		return fmt.Errorf("error encoding yaml: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("error closing encoder: %w", err)
	}

	if err := v.BaseVirt.shims.WriteFile(tempFilePath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("error writing to temporary file: %w", err)
	}
	defer os.Remove(tempFilePath)

	finalFilePath := filepath.Join(colimaDir, "colima.yaml")
	if err := v.BaseVirt.shims.Rename(tempFilePath, finalFilePath); err != nil {
		return fmt.Errorf("error renaming temporary file to colima config file: %w", err)
	}
	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// getArch retrieves the architecture of the system
// Maps the Go architecture to the Colima architecture format
// Handles special cases for amd64 and arm64 architectures
// Returns the architecture string in the format expected by Colima
// getArch returns the system architecture string formatted for Colima configuration,
// mapping standard Go architectures to their Colima equivalents using a tagged switch.
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
// Calculates CPU count as half of the system's CPU cores
// Sets a default disk size of 100GB
// Calculates memory as half of the system's total memory, with a fallback to 2GB
// Generates a hostname based on the context name
// Returns the calculated values for CPU, disk, memory, hostname, and architecture
func (v *ColimaVirt) getDefaultValues(context string) (int, int, int, string, string) {
	cpu := v.BaseVirt.shims.NumCPU() / 2
	disk := 100 // Disk size in GB
	vmStat, err := v.BaseVirt.shims.VirtualMemory()
	var memory int
	if err != nil {
		memory = 2 // Default to 2GB
	} else {
		totalMemoryGB := vmStat.Total / (1024 * 1024 * 1024)
		halfMemoryGB := totalMemoryGB / 2

		if testForceMemoryOverflow || halfMemoryGB > uint64(math.MaxInt) {
			memory = math.MaxInt
		} else {
			memory = int(halfMemoryGB)
		}
	}

	hostname := fmt.Sprintf("windsor-%s", context)
	arch := v.getArch()
	return cpu, disk, memory, hostname, arch
}

// executeColimaCommand executes a Colima command with the given action
// Formats the command with the appropriate context name
// Executes the command with progress output
// Returns an error if the command execution fails
func (v *ColimaVirt) executeColimaCommand(action string) error {
	contextName := v.configHandler.GetContext()

	command := "colima"
	args := []string{action, fmt.Sprintf("windsor-%s", contextName)}
	formattedCommand := fmt.Sprintf("%s %s", command, strings.Join(args, " "))
	output, err := v.shell.ExecProgress(fmt.Sprintf("🦙 Running %s", formattedCommand), command, args...)
	if err != nil {
		return fmt.Errorf("Error executing command %s %v: %w\n%s", command, args, err, output)
	}

	return nil
}

// startColima starts the Colima VM and waits for it to have an assigned IP address
// Executes the start command and waits for the VM to be ready
// Retries a configurable number of times to get the VM information
// Returns the VM information or an error if the VM fails to start or get an IP
func (v *ColimaVirt) startColima() (VMInfo, error) {
	contextName := v.configHandler.GetContext()

	command := "colima"
	args := []string{"start", fmt.Sprintf("windsor-%s", contextName)}
	output, err := v.shell.ExecProgress(fmt.Sprintf("🦙 Running %s %s", command, strings.Join(args, " ")), command, args...)
	if err != nil {
		return VMInfo{}, fmt.Errorf("Error executing command %s %v: %w\n%s", command, args, err, output)
	}

	var info VMInfo
	var lastErr error
	vmRuntime := v.configHandler.GetString("vm.runtime", "docker")
	for i := range make([]int, testRetryAttempts) {
		info, err = v.getVMInfo()
		if err != nil {
			lastErr = fmt.Errorf("Error retrieving Colima info: %w", err)
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
	if vmRuntime == "incus" {
		return info, nil
	}
	return VMInfo{}, fmt.Errorf("Timed out waiting for Colima VM to get an IP address")
}

// getProvisionScripts returns the provision scripts to run in the Colima VM
func (v *ColimaVirt) getProvisionScripts() []colimaConfig.Provision {
	return []colimaConfig.Provision{
		{
			Mode:   "system",
			Script: "modprobe br_netfilter",
		},
	}
}

// =============================================================================
// Interface Compliance
// =============================================================================

// GetNetworkName returns the network name for the Colima VM.
func (v *ColimaVirt) GetNetworkName() string {
	return v.configHandler.GetString("network.name", "incusbr0")
}

// GetIncusRemote returns the Incus remote name for the Colima VM.
func (v *ColimaVirt) GetIncusRemote() string {
	return v.configHandler.GetString("incus.remote", "colima-windsor-local")
}

// Ensure ColimaVirt implements Virt and VirtualMachine
var _ Virt = (*ColimaVirt)(nil)
var _ VirtualMachine = (*ColimaVirt)(nil)
