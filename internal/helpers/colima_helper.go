package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/shell"
)

// ColimaHelper is a struct that provides various utility functions for working with Colima
type ColimaHelper struct {
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Context       context.ContextInterface
}

// NewColimaHelper is a constructor for ColimaHelper
func NewColimaHelper(configHandler config.ConfigHandler, shell shell.Shell, ctx context.ContextInterface) *ColimaHelper {
	return &ColimaHelper{
		ConfigHandler: configHandler,
		Shell:         shell,
		Context:       ctx,
	}
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
	memory := int(float64(os.Getpagesize()) * float64(runtime.NumCPU()) / (1024.0 * 1024.0 * 1024.0) / 2)
	hostname := fmt.Sprintf("windsor-%s", context)
	arch := getArch()
	return cpu, disk, memory, hostname, arch
}

// GetEnvVars retrieves the environment variables for the Colima command
func (h *ColimaHelper) GetEnvVars() (map[string]string, error) {
	// Colima does not use environment variables
	return map[string]string{}, nil
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

	cpu, disk, memory, _, arch := getDefaultValues(context)

	switch key {
	case "driver":
		if value == "colima" {
			if err = h.ConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.vm.driver", context), value); err != nil {
				return fmt.Errorf("error setting colima config: %w", err)
			}
		}
	case "cpu":
		if value == "" {
			value = strconv.Itoa(cpu)
		}
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		if err = h.ConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.vm.%s", context, key), value); err != nil {
			return fmt.Errorf("error setting colima config: %w", err)
		}
	case "disk":
		if value == "" {
			value = strconv.Itoa(disk)
		}
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		if err = h.ConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.vm.%s", context, key), value); err != nil {
			return fmt.Errorf("error setting colima config: %w", err)
		}
	case "memory":
		if value == "" {
			value = strconv.Itoa(memory)
		}
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		if err = h.ConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.vm.%s", context, key), value); err != nil {
			return fmt.Errorf("error setting colima config: %w", err)
		}
	case "arch":
		if value == "" {
			value = arch
		}
		if value != "amd64" && value != "arm64" {
			return fmt.Errorf("invalid value for arch: %s", value)
		}
		if err = h.ConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.vm.arch", context), value); err != nil {
			return fmt.Errorf("error setting colima config: %w", err)
		}
	default:
		return fmt.Errorf("unsupported config key: %s", key)
	}

	return generateColimaConfig(context, h.ConfigHandler)
}

// Ensure ColimaHelper implements Helper interface
var _ Helper = (*ColimaHelper)(nil)

// generateColimaConfig generates the colima.yaml configuration file based on the Windsor context
func generateColimaConfig(context string, configHandler config.ConfigHandler) error {
	colimaConfigDir := filepath.Join(os.Getenv("HOME"), fmt.Sprintf(".colima/windsor-%s", context))
	colimaConfigPath := filepath.Join(colimaConfigDir, "colima.yaml")

	if err := mkdirAll(colimaConfigDir, os.ModePerm); err != nil {
		return fmt.Errorf("error creating colima config directory: %w", err)
	}

	// Get default values
	cpu, disk, memory, hostname, arch := getDefaultValues(context)

	vmType := "qemu"
	if runtime.GOOS == "darwin" && arch == "aarch64" {
		vmType = "vz"
	}

	// Override default values with context-specific values if provided
	if val, err := configHandler.GetConfigValue(fmt.Sprintf("contexts.%s.vm.cpu", context)); err == nil {
		if cpuVal, err := strconv.Atoi(val); err == nil {
			cpu = cpuVal
		}
	}
	if val, err := configHandler.GetConfigValue(fmt.Sprintf("contexts.%s.vm.disk", context)); err == nil {
		if diskVal, err := strconv.Atoi(val); err == nil {
			disk = diskVal
		}
	}
	if val, err := configHandler.GetConfigValue(fmt.Sprintf("contexts.%s.vm.memory", context)); err == nil {
		if memoryVal, err := strconv.Atoi(val); err == nil {
			memory = memoryVal
		}
	}
	if val, err := configHandler.GetConfigValue(fmt.Sprintf("contexts.%s.vm.arch", context)); err == nil {
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

	file, err := osCreate(colimaConfigPath)
	if err != nil {
		return fmt.Errorf("error creating colima config file: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(headerComment); err != nil {
		return fmt.Errorf("error writing header comment: %w", err)
	}

	// Create a temporary file to write the YAML content
	tempFile, err := osCreateTemp("", "colima_config_*.yaml")
	if err != nil {
		return fmt.Errorf("error creating temporary file: %w", err)
	}
	defer os.Remove(tempFile.Name())

	encoder := newYAMLEncoder(tempFile)
	defer encoder.Close()

	if err := encoder.Encode(configData); err != nil {
		return fmt.Errorf("error encoding yaml: %w", err)
	}

	// Copy the content from the temporary file to the actual file
	tempFile.Seek(0, 0)
	if _, err := tempFile.WriteTo(file); err != nil {
		return fmt.Errorf("error writing to colima config file: %w", err)
	}

	return nil
}
