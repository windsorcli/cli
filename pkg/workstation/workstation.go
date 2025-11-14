package workstation

import (
	"fmt"
	"os"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/runtime"
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

// WorkstationRuntime holds the execution context for workstation operations.
// It embeds the base Runtime and includes all workstation-specific dependencies.
type WorkstationRuntime struct {
	runtime.Runtime

	// Workstation-specific dependencies (created as needed)
	NetworkManager   network.NetworkManager
	Services         []services.Service
	VirtualMachine   virt.VirtualMachine
	ContainerRuntime virt.ContainerRuntime
	SSHClient        ssh.Client
}

// Workstation manages all workstation functionality including virtualization,
// networking, services, and SSH operations.
// It embeds WorkstationRuntime so all fields are directly accessible.
type Workstation struct {
	*WorkstationRuntime
	injector di.Injector
}

// =============================================================================
// Constructor
// =============================================================================

// NewWorkstation creates a new Workstation instance with the provided execution context and injector.
// The execution context should already have ConfigHandler and Shell set.
// Other dependencies are created only if not already present on the context.
func NewWorkstation(ctx *WorkstationRuntime, injector di.Injector) (*Workstation, error) {
	if ctx == nil {
		return nil, fmt.Errorf("execution context is required")
	}
	if ctx.ConfigHandler == nil {
		return nil, fmt.Errorf("ConfigHandler is required on execution context")
	}
	if ctx.Shell == nil {
		return nil, fmt.Errorf("Shell is required on execution context")
	}
	if injector == nil {
		return nil, fmt.Errorf("injector is required")
	}

	// Create workstation first
	workstation := &Workstation{
		WorkstationRuntime: ctx,
		injector:           injector,
	}

	// Create NetworkManager if not already set
	if workstation.NetworkManager == nil {
		workstation.NetworkManager = network.NewBaseNetworkManager(injector)
	}

	// Create Services if not already set
	if workstation.Services == nil {
		serviceList, err := workstation.createServices()
		if err != nil {
			return nil, fmt.Errorf("failed to create services: %w", err)
		}
		workstation.Services = serviceList
	}

	// Create VirtualMachine if not already set
	if workstation.VirtualMachine == nil {
		vmDriver := workstation.ConfigHandler.GetString("vm.driver", "")
		if vmDriver == "colima" {
			workstation.VirtualMachine = virt.NewColimaVirt(injector)
		}
	}

	// Create ContainerRuntime if not already set
	if workstation.ContainerRuntime == nil {
		dockerEnabled := workstation.ConfigHandler.GetBool("docker.enabled", false)
		if dockerEnabled {
			workstation.ContainerRuntime = virt.NewDockerVirt(injector)
		}
	}

	// Create SSHClient if not already set
	if workstation.SSHClient == nil {
		workstation.SSHClient = ssh.NewSSHClient()
	}

	return workstation, nil
}

// =============================================================================
// Public Methods
// =============================================================================

// Up starts the workstation environment including VMs, containers, networking, and services.
// It sets the NO_CACHE environment variable, launches the virtual machine if the driver is "colima",
// initializes the network manager for all registered services, re-initializes DNS services,
// writes service configurations, initializes and starts the container runtime if enabled,
// configures networking components, and informs the user of successful environment setup.
func (w *Workstation) Up() error {
	if err := os.Setenv("NO_CACHE", "true"); err != nil {
		return fmt.Errorf("Error setting NO_CACHE environment variable: %w", err)
	}

	vmDriver := w.ConfigHandler.GetString("vm.driver")
	if vmDriver == "colima" {
		if w.VirtualMachine == nil {
			return fmt.Errorf("no virtual machine found")
		}
		if err := w.VirtualMachine.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize virtual machine: %w", err)
		}
		if err := w.VirtualMachine.Up(); err != nil {
			return fmt.Errorf("error running virtual machine Up command: %w", err)
		}
	}

	if w.NetworkManager != nil {
		if err := w.NetworkManager.Initialize(w.Services); err != nil {
			return fmt.Errorf("failed to initialize network manager: %w", err)
		}
	}

	for _, service := range w.Services {
		if dnsService, ok := service.(*services.DNSService); ok {
			if err := dnsService.Initialize(); err != nil {
				return fmt.Errorf("failed to re-initialize DNS service: %w", err)
			}
			break
		}
	}

	for _, service := range w.Services {
		if err := service.WriteConfig(); err != nil {
			return fmt.Errorf("Error writing config for service %s: %w", service.GetName(), err)
		}
	}

	containerRuntimeEnabled := w.ConfigHandler.GetBool("docker.enabled")
	if containerRuntimeEnabled {
		if w.ContainerRuntime == nil {
			return fmt.Errorf("no container runtime found")
		}
		if err := w.ContainerRuntime.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize container runtime: %w", err)
		}
		if err := w.ContainerRuntime.WriteConfig(); err != nil {
			return fmt.Errorf("failed to write container runtime config: %w", err)
		}
		if err := w.ContainerRuntime.Up(); err != nil {
			return fmt.Errorf("error running container runtime Up command: %w", err)
		}
	}

	if w.NetworkManager != nil {
		// Only configure guest and host routes for colima
		if vmDriver == "colima" {
			if err := w.NetworkManager.ConfigureGuest(); err != nil {
				return fmt.Errorf("error configuring guest: %w", err)
			}
			if err := w.NetworkManager.ConfigureHostRoute(); err != nil {
				return fmt.Errorf("error configuring host route: %w", err)
			}
		}

		// Configure DNS if enabled
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
		if err := w.ContainerRuntime.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize container runtime: %w", err)
		}
		if err := w.ContainerRuntime.Down(); err != nil {
			return fmt.Errorf("Error running container runtime Down command: %w", err)
		}
	}

	if w.VirtualMachine != nil {
		if err := w.VirtualMachine.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize virtual machine: %w", err)
		}
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
		service := services.NewDNSService(w.injector)
		service.SetName("dns")
		if err := service.Initialize(); err != nil {
			return nil, fmt.Errorf("failed to initialize DNS service: %w", err)
		}
		w.injector.Register("dnsService", service)
		serviceList = append(serviceList, service)
	}

	// Git Livereload Service
	gitEnabled := w.ConfigHandler.GetBool("git.livereload.enabled", false)
	if gitEnabled {
		service := services.NewGitLivereloadService(w.injector)
		service.SetName("git")
		if err := service.Initialize(); err != nil {
			return nil, fmt.Errorf("failed to initialize Git Livereload service: %w", err)
		}
		w.injector.Register("gitLivereloadService", service)
		serviceList = append(serviceList, service)
	}

	// Localstack Service
	awsEnabled := w.ConfigHandler.GetBool("aws.localstack.enabled", false)
	if awsEnabled {
		service := services.NewLocalstackService(w.injector)
		service.SetName("aws")
		if err := service.Initialize(); err != nil {
			return nil, fmt.Errorf("failed to initialize Localstack service: %w", err)
		}
		w.injector.Register("localstackService", service)
		serviceList = append(serviceList, service)
	}

	// Registry Services
	contextConfig := w.ConfigHandler.GetConfig()
	if contextConfig.Docker != nil && contextConfig.Docker.Registries != nil {
		for key := range contextConfig.Docker.Registries {
			service := services.NewRegistryService(w.injector)
			service.SetName(key)
			if err := service.Initialize(); err != nil {
				return nil, fmt.Errorf("failed to initialize Registry service %s: %w", key, err)
			}
			w.injector.Register(fmt.Sprintf("registryService.%s", key), service)
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
			service := services.NewTalosService(w.injector, "controlplane")
			serviceName := fmt.Sprintf("controlplane-%d", i)
			service.SetName(serviceName)
			if err := service.Initialize(); err != nil {
				return nil, fmt.Errorf("failed to initialize Talos controlplane service %s: %w", serviceName, err)
			}
			w.injector.Register(fmt.Sprintf("talosService.%s", serviceName), service)
			serviceList = append(serviceList, service)
		}

		for i := 1; i <= workerCount; i++ {
			service := services.NewTalosService(w.injector, "worker")
			serviceName := fmt.Sprintf("worker-%d", i)
			service.SetName(serviceName)
			if err := service.Initialize(); err != nil {
				return nil, fmt.Errorf("failed to initialize Talos worker service %s: %w", serviceName, err)
			}
			w.injector.Register(fmt.Sprintf("talosService.%s", serviceName), service)
			serviceList = append(serviceList, service)
		}
	}

	return serviceList, nil
}
