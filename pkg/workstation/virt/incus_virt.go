// The IncusVirt is a container runtime implementation
// It provides Incus container management capabilities through the Incus API/CLI
// It serves as the primary container orchestration layer for Incus-based services
// It handles container lifecycle, configuration, and networking for Incus containers and VMs

package virt

import (
	"fmt"
	"maps"
	"path/filepath"
	"sort"
	"strings"

	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/shell/ssh"
	"github.com/windsorcli/cli/pkg/workstation/services"
)

// =============================================================================
// Types
// =============================================================================

// IncusVirt implements the ContainerRuntime interface for Incus
type IncusVirt struct {
	BaseVirt
	services    []services.Service
	vm          VirtualMachine
	secureShell shell.Shell
}

// IncusInstanceConfig represents the complete configuration for creating an Incus instance
type IncusInstanceConfig struct {
	Name      string
	Type      string
	Image     string
	Config    map[string]string
	Devices   map[string]map[string]string
	Profiles  []string
	IPv4      string
	Network   string
	Resources map[string]string
}

// =============================================================================
// Constructor
// =============================================================================

// NewIncusVirt creates a new instance of IncusVirt
func NewIncusVirt(rt *runtime.Runtime, serviceList []services.Service, vm VirtualMachine, sshClient ssh.Client) *IncusVirt {
	var serviceSlice []services.Service
	if serviceList != nil {
		for _, service := range serviceList {
			if service != nil {
				serviceSlice = append(serviceSlice, service)
			}
		}
		sort.Slice(serviceSlice, func(i, j int) bool {
			return serviceSlice[i].GetName() < serviceSlice[j].GetName()
		})
	}

	secureShell := shell.NewSecureShell(sshClient)

	return &IncusVirt{
		BaseVirt:    *NewBaseVirt(rt),
		services:    serviceSlice,
		vm:          vm,
		secureShell: secureShell,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Up creates and starts Incus instances for all services.
// It iterates through services, builds Incus configuration, and creates instances via the Incus CLI.
// Returns an error if any instance creation fails.
func (v *IncusVirt) Up() error {
	vmRuntime := v.configHandler.GetString("vm.runtime", "docker")
	if vmRuntime != "incus" {
		return nil
	}

	remotes := []struct {
		name string
		url  string
	}{
		{"docker", "https://docker.io"},
		{"ghcr", "https://ghcr.io"},
	}

	for _, remote := range remotes {
		if err := v.ensureRemote(remote.name, remote.url); err != nil {
			return fmt.Errorf("failed to ensure %s remote: %w", remote.name, err)
		}
	}

	var networkName string
	if v.vm != nil {
		networkName = v.vm.GetNetworkName()
	} else {
		networkName = v.configHandler.GetString("network.name", "incusbr0")
	}

	for _, service := range v.services {
		incusConfig, err := service.GetIncusConfig()
		if err != nil {
			return fmt.Errorf("failed to get Incus config for service %s: %w", service.GetName(), err)
		}

		if incusConfig == nil {
			continue
		}

		instanceConfig := v.buildInstanceConfig(service, incusConfig, networkName)
		if err := v.createInstance(instanceConfig); err != nil {
			return fmt.Errorf("failed to create Incus instance %s: %w", instanceConfig.Name, err)
		}
	}

	return nil
}

// Down stops and deletes all Incus instances managed by Windsor.
// It iterates through services and removes their corresponding Incus instances.
// Returns an error if any deletion fails.
func (v *IncusVirt) Down() error {
	vmRuntime := v.configHandler.GetString("vm.runtime", "docker")
	if vmRuntime != "incus" {
		return nil
	}

	for _, service := range v.services {
		instanceName := sanitizeInstanceName(service.GetName())
		if err := v.deleteInstance(instanceName); err != nil {
			return fmt.Errorf("failed to delete Incus instance %s: %w", instanceName, err)
		}
	}

	return nil
}

// WriteConfig is a no-op for Incus as there is no manifest file.
// Incus instances are created directly via API/CLI calls.
func (v *IncusVirt) WriteConfig() error {
	return nil
}

// ensureRemote ensures a remote is configured in Incus.
func (v *IncusVirt) ensureRemote(name, url string) error {
	exists, err := v.remoteExists(name)
	if err != nil {
		return fmt.Errorf("failed to check %s remote: %w", name, err)
	}
	if exists {
		return nil
	}

	message := fmt.Sprintf("🔧 Configuring %s remote", strings.ToUpper(name))
	_, err = v.secureShell.ExecProgress(message, "incus", "remote", "add", name, url, "--protocol", "oci", "--public")
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return nil
		}
		return fmt.Errorf("failed to add %s remote: %w", name, err)
	}

	return nil
}

// Ensure IncusVirt implements ContainerRuntime
var _ ContainerRuntime = (*IncusVirt)(nil)

// =============================================================================
// Private Methods
// =============================================================================

// instanceExists checks if an Incus instance exists.
func (v *IncusVirt) instanceExists(name string) (bool, error) {
	output, err := v.secureShell.ExecSilent("incus", "list", "--format", "csv", name)
	if err != nil {
		return false, err
	}
	return strings.Contains(output, name+","), nil
}

// instanceIsRunning checks if an Incus instance is running.
func (v *IncusVirt) instanceIsRunning(name string) (bool, error) {
	output, err := v.secureShell.ExecSilent("incus", "list", "--format", "csv", name)
	if err != nil {
		return false, err
	}
	return strings.Contains(output, "RUNNING"), nil
}

// deviceExists checks if a device exists on an Incus instance.
func (v *IncusVirt) deviceExists(instanceName, deviceName string) (bool, error) {
	output, err := v.secureShell.ExecSilent("incus", "config", "device", "list", instanceName)
	if err != nil {
		return false, err
	}
	return strings.Contains(output, deviceName), nil
}

// remoteExists checks if an Incus remote exists.
func (v *IncusVirt) remoteExists(remoteName string) (bool, error) {
	output, err := v.secureShell.ExecSilent("incus", "remote", "list", "--format", "csv")
	if err != nil {
		return false, err
	}
	return strings.Contains(output, remoteName+","), nil
}

// ensureFileExists ensures a file or directory exists in the VM by checking if it's accessible.
// For directories, it creates them if they don't exist. For files, it ensures the parent directory exists.
func (v *IncusVirt) ensureFileExists(filePath string) error {
	_, err := v.secureShell.ExecSilent("test", "-e", filePath)
	if err == nil {
		return nil
	}
	parentDir := filepath.Dir(filePath)
	_, err = v.secureShell.ExecSilent("mkdir", "-p", parentDir)
	if err != nil {
		return fmt.Errorf("failed to create parent directory in VM: %s", parentDir)
	}
	_, err = v.secureShell.ExecSilent("mkdir", "-p", filePath)
	if err == nil {
		return nil
	}
	_, err = v.secureShell.ExecSilent("test", "-e", filePath)
	if err != nil {
		return fmt.Errorf("file or directory does not exist or is not accessible in VM: %s", filePath)
	}
	return nil
}

// buildInstanceConfig builds a complete IncusInstanceConfig from service metadata and IncusConfig.
func (v *IncusVirt) buildInstanceConfig(service services.Service, incusConfig *services.IncusConfig, networkName string) *IncusInstanceConfig {
	hostname := service.GetHostname()
	address := service.GetAddress()

	config := maps.Clone(incusConfig.Config)
	config["user.hostname"] = hostname

	profiles := incusConfig.Profiles
	if len(profiles) == 0 {
		profiles = []string{"default"}
	}

	instanceName := sanitizeInstanceName(service.GetName())

	return &IncusInstanceConfig{
		Name:      instanceName,
		Type:      incusConfig.Type,
		Image:     incusConfig.Image,
		Config:    config,
		Devices:   incusConfig.Devices,
		IPv4:      address,
		Network:   networkName,
		Profiles:  profiles,
		Resources: incusConfig.Resources,
	}
}

// sanitizeInstanceName converts a service name to a valid Incus instance name.
// Incus instance names can only contain alphanumeric and hyphen characters.
func sanitizeInstanceName(name string) string {
	result := strings.Builder{}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		} else {
			result.WriteRune('-')
		}
	}
	return result.String()
}

// createInstance creates an Incus instance using the provided configuration.
// Commands are executed inside the Colima VM via SSH.
func (v *IncusVirt) createInstance(config *IncusInstanceConfig) error {
	if v.vm == nil {
		return fmt.Errorf("virtual machine is required for Incus operations")
	}

	args := []string{"launch", config.Image, config.Name}

	if config.Type == "vm" {
		args = append(args, "--vm")
	}

	if config.Network != "" && config.IPv4 == "" {
		args = append(args, "--network", config.Network)
	}

	for key, value := range config.Config {
		configValue := fmt.Sprintf("%s=%s", key, value)
		if strings.Contains(value, " ") {
			configValue = fmt.Sprintf("%s=\"%s\"", key, value)
		}
		args = append(args, "--config", configValue)
	}

	if len(config.Resources) > 0 {
		for key, value := range config.Resources {
			args = append(args, "--config", fmt.Sprintf("%s=%s", key, value))
		}
	}

	for _, profile := range config.Profiles {
		args = append(args, "--profile", profile)
	}

	exists, err := v.instanceExists(config.Name)
	if err != nil {
		return fmt.Errorf("failed to check if instance exists: %w", err)
	}
	if exists {
		running, err := v.instanceIsRunning(config.Name)
		if err != nil {
			return fmt.Errorf("failed to check if instance is running: %w", err)
		}
		if !running {
			_, err := v.secureShell.ExecProgress(fmt.Sprintf("📦 Starting instance %s", config.Name), "incus", "start", config.Name)
			if err != nil {
				if strings.Contains(err.Error(), "already running") {
					return nil
				}
				return fmt.Errorf("failed to start instance: %w", err)
			}
		}
		return nil
	}

	message := fmt.Sprintf("📦 Creating Incus instance %s", config.Name)
	_, err = v.secureShell.ExecProgress(message, "incus", args...)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return nil
		}
		return fmt.Errorf("failed to launch Incus instance: %w", err)
	}

	needsDeviceConfig := (config.Network != "" && config.IPv4 != "") || len(config.Devices) > 0
	if needsDeviceConfig {
		_, err := v.secureShell.ExecSilent("incus", "stop", config.Name)
		if err != nil {
			return fmt.Errorf("failed to stop instance for device configuration: %w", err)
		}

		if config.Network != "" && config.IPv4 != "" {
			exists, err := v.deviceExists(config.Name, "eth0")
			if err != nil {
				return fmt.Errorf("failed to check if network device exists: %w", err)
			}
			if !exists {
				deviceArgs := []string{"config", "device", "add", config.Name, "eth0", "nic", fmt.Sprintf("network=%s", config.Network), fmt.Sprintf("ipv4.address=%s", config.IPv4)}
				_, err := v.secureShell.ExecProgress(fmt.Sprintf("📦 Adding network device to %s", config.Name), "incus", deviceArgs...)
				if err != nil {
					if strings.Contains(err.Error(), "already exists") {
					} else {
						return fmt.Errorf("failed to add network device: %w", err)
					}
				}
			}
		}
	}

	for deviceName, deviceConfig := range config.Devices {
		if deviceName == "eth0" {
			continue
		}
		deviceType := deviceConfig["type"]
		if deviceType == "" {
			continue
		}
		exists, err := v.deviceExists(config.Name, deviceName)
		if err != nil {
			return fmt.Errorf("failed to check if device %s exists: %w", deviceName, err)
		}
		if exists {
			continue
		}
		if deviceType == "disk" {
			if source, ok := deviceConfig["source"]; ok {
				if err := v.ensureFileExists(source); err != nil {
					return fmt.Errorf("failed to ensure source file exists for device %s: %w", deviceName, err)
				}
			}
		}
		deviceArgs := []string{"config", "device", "add", config.Name, deviceName, deviceType}
		for k, v := range deviceConfig {
			if k != "type" {
				deviceArgs = append(deviceArgs, fmt.Sprintf("%s=%s", k, v))
			}
		}
		_, err = v.secureShell.ExecProgress(fmt.Sprintf("📦 Adding device %s to %s", deviceName, config.Name), "incus", deviceArgs...)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				continue
			}
			return fmt.Errorf("failed to add device %s: %w", deviceName, err)
		}
	}

	if needsDeviceConfig {
		running, err := v.instanceIsRunning(config.Name)
		if err != nil {
			return fmt.Errorf("failed to check if instance is running: %w", err)
		}
		if !running {
			_, err := v.secureShell.ExecProgress(fmt.Sprintf("📦 Starting instance %s", config.Name), "incus", "start", config.Name)
			if err != nil {
				if strings.Contains(err.Error(), "already running") {
				} else {
					return fmt.Errorf("failed to start instance: %w", err)
				}
			}
		}
	}

	return nil
}

// deleteInstance deletes an Incus instance by name.
// Commands are executed inside the VM via SSH.
func (v *IncusVirt) deleteInstance(name string) error {
	if v.vm == nil {
		return fmt.Errorf("virtual machine is required for Incus operations")
	}

	exists, err := v.instanceExists(name)
	if err != nil {
		return fmt.Errorf("failed to check if instance exists: %w", err)
	}
	if !exists {
		return nil
	}

	args := []string{"delete", name, "--force"}

	message := fmt.Sprintf("📦 Deleting Incus instance %s", name)
	_, err = v.secureShell.ExecProgress(message, "incus", args...)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to delete Incus instance: %w", err)
	}

	return nil
}
