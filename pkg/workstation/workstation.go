package workstation

import (
	"fmt"
	"os"

	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/shell/ssh"
	"github.com/windsorcli/cli/pkg/workstation/network"
	"github.com/windsorcli/cli/pkg/workstation/services"
	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// The Workstation is a core component that manages all workstation functionality including virtualization,
// networking, services, and SSH operations.
// It provides a unified interface for starting, stopping, and managing workstation infrastructure,
// The Workstation acts as the central workstation orchestrator for the application,
// coordinating VM lifecycle, container runtime management, network configuration, and service orchestration.

// =============================================================================
// Types
// =============================================================================

// Workstation manages all workstation functionality including virtualization,
// networking, services, and SSH operations.
type Workstation struct {
	*runtime.Runtime

	// Workstation-specific dependencies (created as needed)
	NetworkManager   network.NetworkManager
	Services         []services.Service
	VirtualMachine   virt.VirtualMachine
	ContainerRuntime virt.ContainerRuntime
	SSHClient        ssh.Client
}

// =============================================================================
// Constructor
// =============================================================================

// NewWorkstation creates a new Workstation instance with the provided runtime.
// Other dependencies are created only if not already present via opts.
func NewWorkstation(rt *runtime.Runtime, opts ...*Workstation) (*Workstation, error) {
	if rt == nil {
		return nil, fmt.Errorf("runtime is required")
	}
	if rt.ConfigHandler == nil {
		return nil, fmt.Errorf("ConfigHandler is required on runtime")
	}
	if rt.Shell == nil {
		return nil, fmt.Errorf("Shell is required on runtime")
	}

	workstation := &Workstation{
		Runtime: rt,
	}

	if len(opts) > 0 && opts[0] != nil {
		overrides := opts[0]
		if overrides.NetworkManager != nil {
			workstation.NetworkManager = overrides.NetworkManager
		}
		if overrides.Services != nil {
			workstation.Services = overrides.Services
		}
		if overrides.VirtualMachine != nil {
			workstation.VirtualMachine = overrides.VirtualMachine
		}
		if overrides.ContainerRuntime != nil {
			workstation.ContainerRuntime = overrides.ContainerRuntime
		}
		if overrides.SSHClient != nil {
			workstation.SSHClient = overrides.SSHClient
		}
	}

	if workstation.SSHClient == nil {
		workstation.SSHClient = ssh.NewSSHClient()
	}

	return workstation, nil
}

// =============================================================================
// Public Methods
// =============================================================================

// Prepare creates all workstation components (network manager, services, virtual machine,
// container runtime) and assigns IP addresses to services. This must be called after
// configuration is loaded but before blueprint loading, as blueprint evaluation depends
// on service addresses being set in config. Returns an error if any component creation
// or IP assignment fails.
func (w *Workstation) Prepare() error {
	vmDriver := w.ConfigHandler.GetString("vm.driver")

	if w.NetworkManager == nil {
		if vmDriver == "colima" {
			secureShell := shell.NewSecureShell(w.SSHClient)
			networkInterfaceProvider := network.NewNetworkInterfaceProvider()
			w.NetworkManager = network.NewColimaNetworkManager(w.Runtime, w.SSHClient, secureShell, networkInterfaceProvider)
		} else {
			w.NetworkManager = network.NewBaseNetworkManager(w.Runtime)
		}
	}

	if w.Services == nil {
		serviceList, err := w.createServices()
		if err != nil {
			return fmt.Errorf("failed to create services: %w", err)
		}
		w.Services = serviceList
	}

	if w.NetworkManager != nil {
		if err := w.NetworkManager.AssignIPs(w.Services); err != nil {
			return fmt.Errorf("failed to assign IPs to services: %w", err)
		}
	}

	if vmDriver == "colima" && w.VirtualMachine == nil {
		w.VirtualMachine = virt.NewColimaVirt(w.Runtime)
	}

	containerRuntimeEnabled := w.ConfigHandler.GetBool("docker.enabled")
	if containerRuntimeEnabled && w.ContainerRuntime == nil {
		w.ContainerRuntime = virt.NewDockerVirt(w.Runtime, w.Services)
	}

	return nil
}

// Up initializes the workstation environment: starts VMs, containers, networking, and services.
// Sets NO_CACHE environment variable, starts the virtual machine if configured, writes service
// configs, starts the container runtime if enabled, and configures networking. All components
// must be created via Prepare() before calling Up(). Returns error on any failure.
func (w *Workstation) Up() error {
	if err := os.Setenv("NO_CACHE", "true"); err != nil {
		return fmt.Errorf("Error setting NO_CACHE environment variable: %w", err)
	}

	vmDriver := w.ConfigHandler.GetString("vm.driver")
	if vmDriver == "colima" && w.VirtualMachine != nil {
		if err := w.VirtualMachine.Up(); err != nil {
			return fmt.Errorf("error running virtual machine Up command: %w", err)
		}
	}

	for _, service := range w.Services {
		if err := service.WriteConfig(); err != nil {
			return fmt.Errorf("Error writing config for service %s: %w", service.GetName(), err)
		}
	}

	if w.ContainerRuntime != nil {
		if err := w.ContainerRuntime.WriteConfig(); err != nil {
			return fmt.Errorf("failed to write container runtime config: %w", err)
		}
		if err := w.ContainerRuntime.Up(); err != nil {
			return fmt.Errorf("error running container runtime Up command: %w", err)
		}
	}

	if w.NetworkManager != nil {
		if vmDriver == "colima" {
			if err := w.NetworkManager.ConfigureGuest(); err != nil {
				return fmt.Errorf("error configuring guest: %w", err)
			}
			if err := w.NetworkManager.ConfigureHostRoute(); err != nil {
				return fmt.Errorf("error configuring host route: %w", err)
			}
		}
		if dnsEnabled := w.ConfigHandler.GetBool("dns.enabled"); dnsEnabled {
			if err := w.NetworkManager.ConfigureDNS(); err != nil {
				return fmt.Errorf("error configuring DNS: %w", err)
			}
		}
	}

	return nil
}

// Down stops the workstation environment, including services, containers, VMs, and networking.
// It attempts to gracefully shut down the container runtime and virtual machine if they exist.
// On success, it prints a confirmation message to standard error and returns nil. If any teardown
// step fails, it returns an error describing the issue.
func (w *Workstation) Down() error {
	if w.ContainerRuntime != nil {
		if err := w.ContainerRuntime.Down(); err != nil {
			return fmt.Errorf("Error running container runtime Down command: %w", err)
		}
	}

	if w.VirtualMachine != nil {
		if err := w.VirtualMachine.Down(); err != nil {
			return fmt.Errorf("Error running virtual machine Down command: %w", err)
		}
	}

	fmt.Fprintln(os.Stderr, "Windsor environment torn down successfully.")

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// createServices creates and registers services based on configuration settings.
func (w *Workstation) createServices() ([]services.Service, error) {
	var serviceList []services.Service

	dockerEnabled := w.ConfigHandler.GetBool("docker.enabled", false)
	if !dockerEnabled {
		return serviceList, nil
	}

	// DNS Service
	dnsEnabled := w.ConfigHandler.GetBool("dns.enabled", false)
	if dnsEnabled {
		service := services.NewDNSService(w.Runtime)
		service.SetName("dns")
		serviceList = append(serviceList, service)
	}

	// Git Livereload Service
	gitEnabled := w.ConfigHandler.GetBool("git.livereload.enabled", false)
	if gitEnabled {
		service := services.NewGitLivereloadService(w.Runtime)
		service.SetName("git")
		serviceList = append(serviceList, service)
	}

	// Localstack Service
	awsEnabled := w.ConfigHandler.GetBool("aws.localstack.enabled", false)
	if awsEnabled {
		service := services.NewLocalstackService(w.Runtime)
		service.SetName("aws")
		serviceList = append(serviceList, service)
	}

	// Registry Services
	contextConfig := w.ConfigHandler.GetConfig()
	if contextConfig.Docker != nil && contextConfig.Docker.Registries != nil {
		for key := range contextConfig.Docker.Registries {
			service := services.NewRegistryService(w.Runtime)
			service.SetName(key)
			serviceList = append(serviceList, service)
		}
	}

	// Cluster Services
	clusterDriver := w.ConfigHandler.GetString("cluster.driver", "")
	switch clusterDriver {
	case "talos", "omni":
		controlPlaneCount := w.ConfigHandler.GetInt("cluster.controlplanes.count")
		workerCount := w.ConfigHandler.GetInt("cluster.workers.count")

		for i := 1; i <= controlPlaneCount; i++ {
			service := services.NewTalosService(w.Runtime, "controlplane")
			serviceName := fmt.Sprintf("controlplane-%d", i)
			service.SetName(serviceName)
			serviceList = append(serviceList, service)
		}

		for i := 1; i <= workerCount; i++ {
			service := services.NewTalosService(w.Runtime, "worker")
			serviceName := fmt.Sprintf("worker-%d", i)
			service.SetName(serviceName)
			serviceList = append(serviceList, service)
		}
	}

	// Initialize DNS service with all services after they're all created
	for _, service := range serviceList {
		if dnsService, ok := service.(*services.DNSService); ok {
			dnsService.SetServices(serviceList)
			break
		}
	}

	return serviceList, nil
}
