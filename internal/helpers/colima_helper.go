package helpers

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/compose-spec/compose-go/types"
	"github.com/shirou/gopsutil/mem"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

// Mockable function for mem.VirtualMemory
var virtualMemory = mem.VirtualMemory

// Test hook to force memory overflow
var testForceMemoryOverflow = false

// ColimaHelper is a struct that provides various utility functions for working with Colima
type ColimaHelper struct {
	ConfigHandler config.ConfigHandler
	Context       context.ContextInterface
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

	return &ColimaHelper{
		ConfigHandler: cliConfigHandler.(config.ConfigHandler),
		Context:       resolvedContext.(context.ContextInterface),
	}, nil
}

type YAMLEncoder interface {
	Encode(v interface{}) error
	Close() error
}

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

// GetEnvVars retrieves the environment variables for the Colima command
func (h *ColimaHelper) GetEnvVars() (map[string]string, error) {
	context, err := h.Context.GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	driver, err := h.ConfigHandler.GetConfigValue(fmt.Sprintf("contexts.%s.vm.driver", context), "")
	if err != nil {
		return nil, fmt.Errorf("error retrieving vm driver: %w", err)
	}

	if driver != "colima" {
		return nil, nil
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

// PostEnvExec runs any necessary commands after the environment variables have been set.
func (h *ColimaHelper) PostEnvExec() error {
	// No post environment execution needed for Colima
	return nil
}

// SetConfig sets the configuration value for the given key
func (h *ColimaHelper) SetConfig(key, value string) error {
	context, err := h.Context.GetContext()
	if err != nil {
		return fmt.Errorf("error retrieving context: %w", err)
	}

	writeColimaConfig := false

	switch key {
	case "driver":
		if value == "colima" {
			if err = h.ConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.vm.driver", context), value); err != nil {
				return fmt.Errorf("error setting colima config: %w", err)
			}
			writeColimaConfig = true
		}
	case "cpu":
		if value != "" {
			cpuValue, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid value for %s: %w", key, err)
			}
			if err = h.ConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.vm.%s", context, key), cpuValue); err != nil {
				return fmt.Errorf("error setting colima config: %w", err)
			}
			writeColimaConfig = true
		}
	case "disk":
		if value != "" {
			diskValue, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid value for %s: %w", key, err)
			}
			if err = h.ConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.vm.%s", context, key), diskValue); err != nil {
				return fmt.Errorf("error setting colima config: %w", err)
			}
			writeColimaConfig = true
		}
	case "memory":
		if value != "" {
			memoryValue, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid value for %s: %w", key, err)
			}
			if err = h.ConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.vm.%s", context, key), memoryValue); err != nil {
				return fmt.Errorf("error setting colima config: %w", err)
			}
			writeColimaConfig = true
		}
	case "arch":
		if value != "" {
			if value != "aarch64" && value != "x86_64" {
				return fmt.Errorf("invalid value for arch: %s", value)
			}
			if err = h.ConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.vm.arch", context), value); err != nil {
				return fmt.Errorf("error setting colima config: %w", err)
			}
			writeColimaConfig = true
		}
	default:
		return fmt.Errorf("unsupported config key: %s", key)
	}

	if writeColimaConfig {
		return generateColimaConfig(context, h.ConfigHandler)
	}

	return nil
}

// Ensure ColimaHelper implements Helper interface
var _ Helper = (*ColimaHelper)(nil)

// generateColimaConfig generates the colima.yaml configuration file based on the Windsor context
func generateColimaConfig(context string, cliConfigHandler config.ConfigHandler) error {
	colimaConfigDir := filepath.Join(os.Getenv("HOME"), fmt.Sprintf(".colima/windsor-%s", context))
	colimaConfigPath := filepath.Join(colimaConfigDir, "colima.yaml")

	if err := mkdirAll(colimaConfigDir, os.ModePerm); err != nil {
		return fmt.Errorf("error creating colima config directory: %w", err)
	}

	// Get default values
	cpu, disk, memory, hostname, arch := getDefaultValues(context)
	vmType := "qemu"
	if getArch() == "aarch64" {
		vmType = "vz"
	}

	// Override default values with context-specific values if provided
	if val, err := cliConfigHandler.GetConfigValue(fmt.Sprintf("contexts.%s.vm.cpu", context)); err == nil && val != "" {
		if cpuVal, err := strconv.Atoi(val); err == nil {
			cpu = cpuVal
		}
	}
	if val, err := cliConfigHandler.GetConfigValue(fmt.Sprintf("contexts.%s.vm.disk", context)); err == nil && val != "" {
		if diskVal, err := strconv.Atoi(val); err == nil {
			disk = diskVal
		}
	}
	if val, err := cliConfigHandler.GetConfigValue(fmt.Sprintf("contexts.%s.vm.memory", context)); err == nil && val != "" {
		if memoryVal, err := strconv.Atoi(val); err == nil {
			memory = memoryVal
		}
	}
	if val, err := cliConfigHandler.GetConfigValue(fmt.Sprintf("contexts.%s.vm.arch", context)); err == nil && val != "" {
		arch = val
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
	tempFilePath := colimaConfigPath + ".tmp"

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
	if err := rename(tempFilePath, colimaConfigPath); err != nil {
		return fmt.Errorf("error renaming temporary file to colima config file: %w", err)
	}
	return nil
}

// WriteConfig writes any vendor specific configuration files that are needed for the helper.
func (h *ColimaHelper) WriteConfig() error {
	return nil
}

// GetContainerConfig returns a list of container data for docker-compose.
func (h *ColimaHelper) GetContainerConfig() ([]types.ServiceConfig, error) {
	// Stub implementation
	return nil, nil
}
