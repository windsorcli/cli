// The IncusVirt is a container runtime implementation
// It provides Incus container management capabilities through the Incus API/CLI
// It serves as the primary container orchestration layer for Incus-based services
// It handles container lifecycle, configuration, and networking for Incus containers and VMs

package virt

import (
	"fmt"
	"maps"
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
		if err := v.deleteInstance(service.GetName()); err != nil {
			return fmt.Errorf("failed to delete Incus instance %s: %w", service.GetName(), err)
		}
	}

	return nil
}

// WriteConfig is a no-op for Incus as there is no manifest file.
// Incus instances are created directly via API/CLI calls.
func (v *IncusVirt) WriteConfig() error {
	return nil
}

// Ensure IncusVirt implements ContainerRuntime
var _ ContainerRuntime = (*IncusVirt)(nil)

// =============================================================================
// Private Methods
// =============================================================================

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

	return &IncusInstanceConfig{
		Name:      service.GetName(),
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

// createInstance creates an Incus instance using the provided configuration.
// Commands are executed inside the Colima VM via SSH.
func (v *IncusVirt) createInstance(config *IncusInstanceConfig) error {
	if v.vm == nil {
		return fmt.Errorf("virtual machine is required for Incus operations")
	}

	args := []string{"launch", config.Image, config.Name, "--type", config.Type}

	if config.Network != "" {
		if config.IPv4 != "" {
			networkDevice := fmt.Sprintf("eth0,type=nic,network=%s,ipv4.address=%s", config.Network, config.IPv4)
			args = append(args, "--device", networkDevice)
		} else {
			args = append(args, "--network", config.Network)
		}
	}

	for key, value := range config.Config {
		args = append(args, "--config", fmt.Sprintf("%s=%s", key, value))
	}

	for deviceName, deviceConfig := range config.Devices {
		if deviceName == "eth0" && config.Network != "" && config.IPv4 != "" {
			continue
		}
		deviceArgs := []string{}
		for k, v := range deviceConfig {
			deviceArgs = append(deviceArgs, fmt.Sprintf("%s=%s", k, v))
		}
		deviceSpec := deviceName
		if len(deviceArgs) > 0 {
			deviceSpec += "," + strings.Join(deviceArgs, ",")
		}
		args = append(args, "--device", deviceSpec)
	}

	if len(config.Resources) > 0 {
		for key, value := range config.Resources {
			args = append(args, "--config", fmt.Sprintf("%s=%s", key, value))
		}
	}

	for _, profile := range config.Profiles {
		args = append(args, "--profile", profile)
	}

	message := fmt.Sprintf("📦 Creating Incus instance %s", config.Name)
	_, err := v.secureShell.ExecProgress(message, "incus", args...)
	if err != nil {
		return fmt.Errorf("failed to launch Incus instance: %w", err)
	}

	return nil
}

// deleteInstance deletes an Incus instance by name.
// Commands are executed inside the VM via SSH.
func (v *IncusVirt) deleteInstance(name string) error {
	if v.vm == nil {
		return fmt.Errorf("virtual machine is required for Incus operations")
	}

	args := []string{"delete", name, "--force"}

	message := fmt.Sprintf("📦 Deleting Incus instance %s", name)
	_, err := v.secureShell.ExecProgress(message, "incus", args...)
	if err != nil {
		return fmt.Errorf("failed to delete Incus instance: %w", err)
	}

	return nil
}
