package helpers

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/compose-spec/compose-go/types"
	"github.com/shirou/gopsutil/mem"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// Mockable function for mem.VirtualMemory
var virtualMemory = mem.VirtualMemory

// Test hook to force memory overflow
var testForceMemoryOverflow = false

// getArch retrieves the architecture of the system
var getArch = func() string {
	arch := goArch()
	if arch == "amd64" {
		return "x86_64"
	} else if arch == "arm64" {
		return "aarch64"
	}
	return arch
}

// getDefaultValues retrieves the default values for the VM properties
func getDefaultValues(context string) (int, int, int, string, string) {
	cpu := runtime.NumCPU() / 2
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

		hostname := fmt.Sprintf("windsor-%s", context)
		arch := getArch()
		return cpu, disk, memory, hostname, arch
	}

	hostname := fmt.Sprintf("windsor-%s", context)
	arch := getArch()
	return cpu, disk, memory, hostname, arch
}

// ColimaInfo is a struct that contains the information about the Colima VM
type ColimaInfo struct {
	Address string  `json:"address"`
	Arch    string  `json:"arch"`
	CPUs    int     `json:"cpus"`
	Disk    float64 `json:"disk"`
	Memory  float64 `json:"memory"`
	Name    string  `json:"name"`
	Runtime string  `json:"runtime"`
	Status  string  `json:"status"`
}

// ColimaHelper is a struct that provides various utility functions for working with Colima
type ColimaHelper struct {
	ConfigHandler config.ConfigHandler
	Context       context.ContextInterface
	Shell         shell.Shell
}

// NewColimaHelper is a constructor for ColimaHelper
func NewColimaHelper(di *di.DIContainer) (*ColimaHelper, error) {
	cliConfigHandler, err := di.Resolve("cliConfigHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving cliConfigHandler: %w", err)
	}

	resolvedContext, err := di.Resolve("context")
	if err != nil {
		return nil, fmt.Errorf("error resolving context: %w", err)
	}

	resolvedShell, err := di.Resolve("shell")
	if err != nil {
		return nil, fmt.Errorf("error resolving shell: %w", err)
	}

	return &ColimaHelper{
		ConfigHandler: cliConfigHandler.(config.ConfigHandler),
		Context:       resolvedContext.(context.ContextInterface),
		Shell:         resolvedShell.(shell.Shell),
	}, nil
}

// Initialize performs any necessary initialization for the helper.
func (h *ColimaHelper) Initialize() error {
	// Perform any necessary initialization here
	return nil
}

// GetEnvVars retrieves the environment variables for the Colima command
func (h *ColimaHelper) GetEnvVars() (map[string]string, error) {
	context, err := h.Context.GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	config, err := h.ConfigHandler.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("error retrieving config: %w", err)
	}
	if config.VM == nil || config.VM.Driver == nil || *config.VM.Driver != "colima" {
		return map[string]string{}, nil
	}

	homeDir, err := userHomeDir()
	if err != nil {
		return nil, fmt.Errorf("error retrieving user home directory: %w", err)
	}

	dockerSockPath := filepath.Join(homeDir, ".colima", fmt.Sprintf("windsor-%s", context), "docker.sock")

	envVars := map[string]string{
		"DOCKER_SOCK": dockerSockPath,
	}

	return envVars, nil
}

// GetComposeConfig returns the top-level compose configuration including a list of container data for docker-compose.
func (h *ColimaHelper) GetComposeConfig() (*types.Config, error) {
	// Stub implementation
	return nil, nil
}

// PostEnvExec runs any necessary commands after the environment variables have been set.
func (h *ColimaHelper) PostEnvExec() error {
	// No post environment execution needed for Colima
	return nil
}

// WriteConfig writes any vendor specific configuration files that are needed for the helper.
func (h *ColimaHelper) WriteConfig() error {
	context, err := h.Context.GetContext()
	if err != nil {
		return fmt.Errorf("error retrieving context: %w", err)
	}

	config, err := h.ConfigHandler.GetConfig()
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
	if err := mkdirAll(filepath.Dir(colimaDir), 0755); err != nil {
		return fmt.Errorf("error creating parent directories for colima directory: %w", err)
	}
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

// Up executes necessary commands to instantiate the tool or environment.
func (h *ColimaHelper) Up(verbose ...bool) error {
	if len(verbose) == 0 {
		verbose = append(verbose, false)
	}

	contextConfig, err := h.ConfigHandler.GetConfig()
	if err != nil {
		return fmt.Errorf("error retrieving config: %w", err)
	}

	contextName, err := h.Context.GetContext()
	if err != nil {
		return fmt.Errorf("error retrieving context: %w", err)
	}

	// Check if contextConfig, contextConfig.VM, and contextConfig.VM.Driver are defined
	if contextConfig != nil && contextConfig.VM != nil && contextConfig.VM.Driver != nil {
		// Check the VM.Driver value and start the virtual machine if necessary
		if *contextConfig.VM.Driver == "colima" {
			if err := h.WriteConfig(); err != nil {
				return fmt.Errorf("Error writing colima config: %w", err)
			}

			command := "colima"
			args := []string{"start", fmt.Sprintf("windsor-%s", contextName)}
			output, err := h.Shell.Exec(verbose[0], "Executing colima start command", command, args...)
			if err != nil {
				return fmt.Errorf("Error executing command %s %v: %w\n%s", command, args, err, output)
			}

			// Wait until the Colima VM has an assigned IP address, try three times
			for i := 0; i < 3; i++ {
				info, err := h.Info()
				if err != nil {
					return fmt.Errorf("Error retrieving Colima info: %w", err)
				}
				colimaInfo := info.(*ColimaInfo)
				if colimaInfo.Address != "" {
					break
				}
				time.Sleep(2 * time.Second)
			}
		}
	}

	return nil
}

// Info returns the information about the Colima VM
func (h *ColimaHelper) Info() (interface{}, error) {
	contextName, err := h.Context.GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	command := "colima"
	args := []string{"ls", "--profile", fmt.Sprintf("windsor-%s", contextName), "--json"}
	out, err := h.Shell.Exec(false, "Fetching Colima info", command, args...)
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

	colimaInfo := &ColimaInfo{
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

// Ensure ColimaHelper implements Helper interface
var _ Helper = (*ColimaHelper)(nil)
