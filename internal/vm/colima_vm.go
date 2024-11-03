package vm

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// Test hook to force memory overflow
var testForceMemoryOverflow = false

// ColimaVM implements the VMInterface for Colima
type ColimaVM struct {
	container di.ContainerInterface
}

// NewColimaVM creates a new instance of ColimaVM using a DI container
func NewColimaVM(container di.ContainerInterface) *ColimaVM {
	return &ColimaVM{
		container: container,
	}
}

// Up starts the Colima VM
func (vm *ColimaVM) Up(verbose ...bool) error {
	if len(verbose) == 0 {
		verbose = append(verbose, false)
	}

	if err := vm.configureColima(); err != nil {
		return err
	}

	if err := vm.startColimaVM(verbose[0]); err != nil {
		return err
	}

	return nil
}

// Down stops the Colima VM
func (vm *ColimaVM) Down(verbose ...bool) error {
	if len(verbose) == 0 {
		verbose = append(verbose, false)
	}

	return vm.executeColimaCommand("stop", verbose[0])
}

// Info returns the information about the Colima VM
func (vm *ColimaVM) Info() (interface{}, error) {
	contextInstance, err := vm.container.Resolve("contextInstance")
	if err != nil {
		return nil, fmt.Errorf("error resolving context: %w", err)
	}

	shellInstance, err := vm.container.Resolve("shell")
	if err != nil {
		return nil, fmt.Errorf("error resolving shell: %w", err)
	}

	contextName, err := contextInstance.(context.ContextInterface).GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	command := "colima"
	args := []string{"ls", "--profile", fmt.Sprintf("windsor-%s", contextName), "--json"}
	out, err := shellInstance.(shell.Shell).Exec(false, "Fetching Colima info", command, args...)
	if err != nil {
		return nil, err
	}

	var colimaData struct {
		Address string `json:"address"`
		Arch    string `json:"arch"`
		CPUs    int    `json:"cpus"`
		Disk    int64  `json:"disk"`
		Memory  int64  `json:"memory"`
		Name    string `json:"name"`
		Runtime string `json:"runtime"`
		Status  string `json:"status"`
	}
	if err := jsonUnmarshal([]byte(out), &colimaData); err != nil {
		return nil, err
	}

	colimaInfo := &VMInfo{
		Address: colimaData.Address,
		Arch:    colimaData.Arch,
		CPUs:    colimaData.CPUs,
		Disk:    float64(colimaData.Disk) / (1024 * 1024 * 1024),
		Memory:  float64(colimaData.Memory) / (1024 * 1024 * 1024),
		Name:    colimaData.Name,
		Runtime: colimaData.Runtime,
		Status:  colimaData.Status,
	}

	return colimaInfo, nil
}

// Delete removes the Colima VM
func (vm *ColimaVM) Delete(verbose ...bool) error {
	if len(verbose) == 0 {
		verbose = append(verbose, false)
	}

	return vm.executeColimaCommand("delete", verbose[0])
}

// Ensure ColimaVM implements VMInterface
var _ VMInterface = (*ColimaVM)(nil)

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
	disk := 60

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
func (vm *ColimaVM) executeColimaCommand(action string, verbose bool) error {
	contextInstance, err := vm.container.Resolve("contextInstance")
	if err != nil {
		return fmt.Errorf("error resolving context: %w", err)
	}

	shellInstance, err := vm.container.Resolve("shell")
	if err != nil {
		return fmt.Errorf("error resolving shell: %w", err)
	}

	contextName, err := contextInstance.(context.ContextInterface).GetContext()
	if err != nil {
		return fmt.Errorf("error retrieving context: %w", err)
	}

	command := "colima"
	args := []string{action, fmt.Sprintf("windsor-%s", contextName)}
	output, err := shellInstance.(shell.Shell).Exec(verbose, fmt.Sprintf("Executing colima %s command", action), command, args...)
	if err != nil {
		return fmt.Errorf("Error executing command %s %v: %w\n%s", command, args, err, output)
	}

	return nil
}

// configureColima writes the Colima configuration file if necessary
func (vm *ColimaVM) configureColima() error {
	cliConfigHandler, err := vm.container.Resolve("cliConfigHandler")
	if err != nil {
		return fmt.Errorf("error resolving cliConfigHandler: %w", err)
	}

	contextConfig, err := cliConfigHandler.(config.ConfigHandler).GetConfig()
	if err != nil {
		return fmt.Errorf("error retrieving config: %w", err)
	}

	if contextConfig == nil || contextConfig.VM == nil || contextConfig.VM.Driver == nil || *contextConfig.VM.Driver != "colima" {
		return nil
	}

	return vm.writeConfig()
}

// startColimaVM starts the Colima VM and waits for it to have an assigned IP address
func (vm *ColimaVM) startColimaVM(verbose bool) error {
	contextInstance, err := vm.container.Resolve("contextInstance")
	if err != nil {
		return fmt.Errorf("error resolving context: %w", err)
	}

	shellInstance, err := vm.container.Resolve("shell")
	if err != nil {
		return fmt.Errorf("error resolving shell: %w", err)
	}

	contextName, err := contextInstance.(context.ContextInterface).GetContext()
	if err != nil {
		return fmt.Errorf("error retrieving context: %w", err)
	}

	command := "colima"
	args := []string{"start", fmt.Sprintf("windsor-%s", contextName)}
	output, err := shellInstance.(shell.Shell).Exec(verbose, "Executing colima start command", command, args...)
	if err != nil {
		return fmt.Errorf("Error executing command %s %v: %w\n%s", command, args, err, output)
	}

	// Wait until the Colima VM has an assigned IP address, try three times
	for i := 0; i < 3; i++ {
		info, err := vm.Info()
		if err != nil {
			return fmt.Errorf("Error retrieving Colima info: %w", err)
		}
		colimaInfo := info.(*VMInfo)
		if colimaInfo.Address != "" {
			break
		}
		time.Sleep(2 * time.Second)
	}

	return nil
}

// writeConfig writes the Colima configuration file
func (vm *ColimaVM) writeConfig() error {
	contextInstance, err := vm.container.Resolve("contextInstance")
	if err != nil {
		return fmt.Errorf("error resolving context: %w", err)
	}

	cliConfigHandler, err := vm.container.Resolve("cliConfigHandler")
	if err != nil {
		return fmt.Errorf("error resolving cliConfigHandler: %w", err)
	}

	context, err := contextInstance.(context.ContextInterface).GetContext()
	if err != nil {
		return fmt.Errorf("error retrieving context: %w", err)
	}

	config, err := cliConfigHandler.(config.ConfigHandler).GetConfig()
	if err != nil {
		return fmt.Errorf("error retrieving config: %w", err)
	}

	// Check if VM is defined and if the vm driver is colima
	if config.VM == nil || config.VM.Driver == nil || *config.VM.Driver != "colima" {
		return nil
	}

	// Get default values
	cpu, disk, memory, hostname, arch := getDefaultValues(context)
	vmType := "qemu"
	if getArch() == "aarch64" {
		vmType = "vz"
	}

	// Helper function to override default values with context-specific values if provided
	overrideValue := func(defaultValue *int, configValue *int) {
		if configValue != nil && *configValue != 0 {
			*defaultValue = *configValue
		}
	}

	overrideValue(&cpu, config.VM.CPU)
	overrideValue(&disk, config.VM.Disk)
	overrideValue(&memory, config.VM.Memory)

	if config.VM.Arch != nil && *config.VM.Arch != "" {
		arch = *config.VM.Arch
	}

	configData := map[string]interface{}{
		"cpu":      cpu,
		"disk":     disk,
		"memory":   memory,
		"arch":     arch,
		"runtime":  "docker",
		"hostname": hostname,
		"kubernetes": map[string]interface{}{
			"enabled": false,
		},
		"autoActivate": true,
		"network": map[string]interface{}{
			"address":       true,
			"dns":           []interface{}{},
			"dnsHosts":      map[string]interface{}{},
			"hostAddresses": false,
		},
		"forwardAgent":         false,
		"docker":               map[string]interface{}{},
		"vmType":               vmType,
		"rosetta":              false,
		"nestedVirtualization": false,
		"mountType":            "sshfs",
		"mountInotify":         true,
		"cpuType":              "",
		"provision":            []interface{}{},
		"sshConfig":            true,
		"sshPort":              0,
		"mounts":               []interface{}{},
		"env":                  map[string]interface{}{},
	}

	headerComment := "# This file was generated by the Windsor CLI. Do not alter.\n\n"

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
	buf.WriteString(headerComment)
	encoder := newYAMLEncoder(&buf)
	if err := encoder.Encode(configData); err != nil {
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
