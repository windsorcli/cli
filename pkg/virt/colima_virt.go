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
	"github.com/windsorcli/cli/pkg/di"
)

// Test hook to force memory overflow
var testForceMemoryOverflow = false

// ColimaVirt implements the VirtInterface and VMInterface for Colima
type ColimaVirt struct {
	BaseVirt
}

// NewColimaVirt creates a new instance of ColimaVirt using a DI injector
func NewColimaVirt(injector di.Injector) *ColimaVirt {
	return &ColimaVirt{
		BaseVirt: BaseVirt{
			injector: injector,
		},
	}
}

// Up starts the Colima VM
func (v *ColimaVirt) Up() error {
	// Start the Colima VM
	info, err := v.startColima()
	if err != nil {
		return fmt.Errorf("failed to start Colima VM: %w", err)
	}

	// Set the VM address in the config handler
	if err := v.configHandler.SetContextValue("vm.address", info.Address); err != nil {
		return fmt.Errorf("failed to set VM address in config handler: %w", err)
	}

	return nil
}

// Down stops and deletes the Colima VM
func (v *ColimaVirt) Down() error {
	// Stop the Colima VM
	if err := v.executeColimaCommand("stop"); err != nil {
		return err
	}

	// Delete the Colima VM
	return v.executeColimaCommand("delete")
}

// GetVMInfo returns the information about the Colima VM
func (v *ColimaVirt) GetVMInfo() (VMInfo, error) {
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
	if err := jsonUnmarshal([]byte(out), &colimaData); err != nil {
		return VMInfo{}, err
	}

	// Convert memory and disk from bytes to GB
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

// WriteConfig writes the Colima configuration file
func (v *ColimaVirt) WriteConfig() error {
	context := v.configHandler.GetContext()

	if v.configHandler.GetString("vm.driver") != "colima" {
		return nil
	}

	// Get default values
	cpu, disk, memory, hostname, arch := getDefaultValues(context)
	vmType := "qemu"
	mountType := "sshfs"
	if getArch() == "aarch64" {
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

	// Use the config package to create a new Colima configuration
	colimaConfig := colimaConfig.Config{
		CPU:      cpu,
		Disk:     disk,
		Memory:   float32(memory),
		Arch:     arch,
		Runtime:  "docker",
		Hostname: hostname,
		Kubernetes: colimaConfig.Kubernetes{
			Enabled: false,
		},
		ActivateRuntime: ptrBool(true),
		Network: colimaConfig.Network{
			Address:       true,
			DNSResolvers:  []net.IP{},
			DNSHosts:      map[string]string{},
			HostAddresses: false,
		},
		ForwardAgent:         false,
		VMType:               vmType,
		VZRosetta:            false,
		NestedVirtualization: false,
		MountType:            mountType,
		MountINotify:         true,
		CPUType:              "",
		Provision: []colimaConfig.Provision{
			{
				Mode:   "system",
				Script: "modprobe br_netfilter",
			},
		},
		SSHConfig: true,
		SSHPort:   0,
		Mounts:    []colimaConfig.Mount{},
		Env:       map[string]string{},
	}

	// Create a temporary file path next to the target file
	homeDir, err := userHomeDir()
	if err != nil {
		return fmt.Errorf("error retrieving user home directory: %w", err)
	}
	colimaDir := filepath.Join(homeDir, fmt.Sprintf(".colima/windsor-%s", context))
	if err := mkdirAll(colimaDir, 0755); err != nil {
		return fmt.Errorf("error creating colima directory: %w", err)
	}
	tempFilePath := filepath.Join(colimaDir, "colima.yaml.tmp")

	// Encode the YAML content to a byte slice
	var buf bytes.Buffer
	encoder := newYAMLEncoder(&buf)
	if err := encoder.Encode(colimaConfig); err != nil {
		return fmt.Errorf("error encoding yaml: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("error closing encoder: %w", err)
	}

	// Write the encoded content to the temporary file
	if err := writeFile(tempFilePath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("error writing to temporary file: %w", err)
	}
	defer os.Remove(tempFilePath)

	// Rename the temporary file to the target file
	finalFilePath := filepath.Join(colimaDir, "colima.yaml")
	if err := rename(tempFilePath, finalFilePath); err != nil {
		return fmt.Errorf("error renaming temporary file to colima config file: %w", err)
	}
	return nil
}

// PrintInfo prints the information about the Colima VM
func (v *ColimaVirt) PrintInfo() error {
	info, err := v.GetVMInfo()
	if err != nil {
		return fmt.Errorf("error retrieving Colima info: %w", err)
	}
	fmt.Printf("%-15s %-10s %-10s %-10s %-10s %-15s\n", "VM NAME", "ARCH", "CPUS", "MEMORY", "DISK", "ADDRESS")
	fmt.Printf("%-15s %-10s %-10d %-10s %-10s %-15s\n", info.Name, info.Arch, info.CPUs, fmt.Sprintf("%dGiB", info.Memory), fmt.Sprintf("%dGiB", info.Disk), info.Address)
	fmt.Println()

	return nil
}

// Ensure ColimaVirt implements Virt and VirtualMachine
var _ Virt = (*ColimaVirt)(nil)
var _ VirtualMachine = (*ColimaVirt)(nil)

// getArch retrieves the architecture of the system
var getArch = func() string {
	arch := goArch
	if arch == "amd64" {
		return "x86_64"
	} else if arch == "arm64" {
		return "aarch64"
	}
	return arch
}

// getDefaultValues retrieves the default values for the VM properties
func getDefaultValues(context string) (int, int, int, string, string) {
	cpu := numCPU() / 2
	disk := 60 // Disk size in GB

	// Use the mockable function to get the total system memory
	vmStat, err := virtualMemory()
	var memory int
	if err != nil {
		// Fallback to a default value if memory retrieval fails
		memory = 2 // Default to 2GB
	} else {
		// Convert total system memory from bytes to gigabytes
		totalMemoryGB := vmStat.Total / (1024 * 1024 * 1024)
		halfMemoryGB := totalMemoryGB / 2

		// Use the test hook to force the overflow condition
		if testForceMemoryOverflow || halfMemoryGB > uint64(math.MaxInt) {
			memory = math.MaxInt
		} else {
			memory = int(halfMemoryGB)
		}
	}

	hostname := fmt.Sprintf("windsor-%s", context)
	arch := getArch()
	return cpu, disk, memory, hostname, arch
}

// executeColimaCommand executes a Colima command with the given action
func (v *ColimaVirt) executeColimaCommand(action string) error {
	// Get the context name
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
func (v *ColimaVirt) startColima() (VMInfo, error) {
	// Get the context name
	contextName := v.configHandler.GetContext()

	command := "colima"
	args := []string{"start", fmt.Sprintf("windsor-%s", contextName)}
	output, err := v.shell.ExecProgress(fmt.Sprintf("🦙 Running %s %s", command, strings.Join(args, " ")), command, args...)
	if err != nil {
		return VMInfo{}, fmt.Errorf("Error executing command %s %v: %w\n%s", command, args, err, output)
	}

	// Wait until the Colima VM has an assigned IP address, try three times
	var info VMInfo
	for i := 0; i < 3; i++ {
		info, err = v.GetVMInfo()
		if err != nil {
			return VMInfo{}, fmt.Errorf("Error retrieving Colima info: %w", err)
		}
		if info.Address != "" {
			return info, nil
		}

		time.Sleep(time.Duration(RETRY_WAIT) * time.Second)
	}

	return VMInfo{}, fmt.Errorf("Failed to retrieve VM info with a valid address after multiple attempts")
}
