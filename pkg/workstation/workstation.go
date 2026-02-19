package workstation

import (
	"fmt"
	"os"
	stdruntime "runtime"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/workstation/network"
	"github.com/windsorcli/cli/pkg/workstation/services"
	"github.com/windsorcli/cli/pkg/workstation/virt"
	"golang.org/x/term"
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
	configHandler config.ConfigHandler
	shell         shell.Shell
	evaluator     evaluator.ExpressionEvaluator
	runtime       *runtime.Runtime

	// Workstation-specific dependencies (created as needed)
	NetworkManager   network.NetworkManager
	Services         []services.Service
	VirtualMachine   virt.VirtualMachine
	ContainerRuntime virt.ContainerRuntime

	// DeferHostGuestSetup when true skips ConfigureGuest/ConfigureHostRoute/ConfigureDNS in Up().
	// Set when the blueprint has a "workstation" Terraform component; host/guest setup runs after that component is applied via the provisioner callback.
	// Temporary: in the future host/guest setup will always run after the "workstation" component and this flag may be removed.
	DeferHostGuestSetup bool
}

// =============================================================================
// Constructor
// =============================================================================

// NewWorkstation creates a new Workstation instance with the provided runtime.
// Other dependencies are created only if not already present via opts.
// Panics if runtime or any required dependencies are nil.
func NewWorkstation(rt *runtime.Runtime, opts ...*Workstation) *Workstation {
	if rt == nil {
		panic("runtime is required")
	}
	if rt.ConfigHandler == nil {
		panic("config handler is required on runtime")
	}
	if rt.Shell == nil {
		panic("shell is required on runtime")
	}
	if rt.Evaluator == nil {
		panic("evaluator is required on runtime")
	}

	workstation := &Workstation{
		configHandler: rt.ConfigHandler,
		shell:         rt.Shell,
		evaluator:     rt.Evaluator,
		runtime:       rt,
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
	}

	return workstation
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
	workstationRuntime := w.configHandler.GetString("workstation.runtime")

	if w.NetworkManager == nil {
		if workstationRuntime == "colima" {
			networkInterfaceProvider := network.NewNetworkInterfaceProvider()
			w.NetworkManager = network.NewColimaNetworkManager(w.runtime, networkInterfaceProvider)
		} else {
			w.NetworkManager = network.NewBaseNetworkManager(w.runtime)
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

	if workstationRuntime == "colima" && w.VirtualMachine == nil {
		w.VirtualMachine = virt.NewColimaVirt(w.runtime)
	}

	if w.configHandler.GetString("provider") == "incus" {
		if w.ContainerRuntime == nil {
			w.ContainerRuntime = virt.NewIncusVirt(w.runtime, w.Services)
			if incusVirt, ok := w.ContainerRuntime.(*virt.IncusVirt); ok {
				w.VirtualMachine = incusVirt.ColimaVirt
			}
		}
	} else {
		if w.runtime.UsesDockerComposeWorkstation() && w.ContainerRuntime == nil {
			w.ContainerRuntime = virt.NewDockerVirt(w.runtime, w.Services)
		}
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

	workstationRuntime := w.configHandler.GetString("workstation.runtime")
	if workstationRuntime == "colima" && w.VirtualMachine != nil {
		if err := w.VirtualMachine.WriteConfig(); err != nil {
			return fmt.Errorf("error writing virtual machine config: %w", err)
		}
		if err := w.VirtualMachine.Up(); err != nil {
			return fmt.Errorf("error running virtual machine Up command: %w", err)
		}
		if w.NetworkManager != nil && w.configHandler.GetString("provider") == "incus" {
			if err := w.NetworkManager.ConfigureGuest(); err != nil {
				return fmt.Errorf("error configuring guest SSH: %w", err)
			}
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

	// Host/guest and DNS are run via the hook after the workstation Terraform component when DeferHostGuestSetup.
	if w.NetworkManager != nil && !w.DeferHostGuestSetup {
		if workstationRuntime == "colima" {
			if err := w.NetworkManager.ConfigureGuest(); err != nil {
				return fmt.Errorf("error configuring guest: %w", err)
			}
			if err := w.NetworkManager.ConfigureHostRoute(); err != nil {
				return fmt.Errorf("error configuring host route: %w", err)
			}
		}
		if dnsEnabled := w.configHandler.GetBool("dns.enabled"); dnsEnabled {
			if err := w.NetworkManager.ConfigureDNS(); err != nil {
				return fmt.Errorf("error configuring DNS: %w", err)
			}
		}
	}

	return nil
}

// PrepareForUp sets DeferHostGuestSetup to defer host/guest and DNS setup until after the
// "workstation" Terraform component is applied (via provisioner hook) when present and
// terraform.enabled is true. Call before Up() when Up will run Terraform with this blueprint.
func (w *Workstation) PrepareForUp(blueprint *blueprintv1alpha1.Blueprint) {
	w.DeferHostGuestSetup = false
	if blueprint == nil || !w.configHandler.GetBool("terraform.enabled", false) {
		return
	}
	for _, c := range blueprint.TerraformComponents {
		if c.GetID() == "workstation" {
			w.DeferHostGuestSetup = true
			break
		}
	}
}

// EnsureNetworkPrivilege ensures the process has (or can obtain) privilege required for network configuration.
// Network manager resolves guest address and other settings from config; prompts for sudo when interactive or on Windows.
func (w *Workstation) EnsureNetworkPrivilege() error {
	if w.NetworkManager == nil || !w.NetworkManager.NeedsPrivilege() {
		return nil
	}
	if term.IsTerminal(int(os.Stdin.Fd())) || stdruntime.GOOS == "windows" {
		if _, err := w.shell.ExecSudo("🔐 Network configuration may require elevated privileges", "true"); err != nil {
			return fmt.Errorf("privileged access required: %w", err)
		}
		return nil
	}
	if os.Geteuid() != 0 {
		if _, err := w.shell.ExecSilent("sudo", "-n", "true"); err != nil {
			return fmt.Errorf("network configuration may require sudo; run from an interactive terminal or with passwordless sudo (e.g. sudo windsor up): %w", err)
		}
	}
	return nil
}

// ConfigureNetwork runs host/guest and DNS setup (same logic as the deferred block in Up).
// Guest address is read from config (workstation.address) by ConfigureHostRoute. DNS: dns.address
// is set from dnsAddressOverride when non-empty, else from workstation.address when unset; DNS
// is attempted only when dns.enabled is true and both dns.domain and dns.address are set. No-op when NetworkManager is nil.
func (w *Workstation) ConfigureNetwork(dnsAddressOverride string) error {
	if w.NetworkManager == nil {
		return nil
	}
	if dnsAddressOverride != "" {
		_ = w.configHandler.Set("dns.address", dnsAddressOverride)
	}
	if w.configHandler.GetString("dns.address") == "" && w.configHandler.GetString("workstation.address") != "" {
		_ = w.configHandler.Set("dns.address", w.configHandler.GetString("workstation.address"))
	}
	workstationRuntime := w.configHandler.GetString("workstation.runtime")
	if workstationRuntime == "colima" {
		if err := w.NetworkManager.ConfigureGuest(); err != nil {
			return fmt.Errorf("error configuring guest: %w", err)
		}
		if err := w.NetworkManager.ConfigureHostRoute(); err != nil {
			return fmt.Errorf("error configuring host route: %w", err)
		}
		fmt.Fprintln(os.Stderr, "network: ready")
	} else {
		fmt.Fprintln(os.Stderr, "network: skipped (not colima)")
	}
	if w.configHandler.GetBool("dns.enabled") && w.configHandler.GetString("dns.domain") != "" && w.configHandler.GetString("dns.address") != "" {
		if err := w.NetworkManager.ConfigureDNS(); err != nil {
			return fmt.Errorf("error configuring DNS: %w", err)
		}
		domain := w.configHandler.GetString("dns.domain")
		addr := w.configHandler.GetString("dns.address")
		fmt.Fprintf(os.Stderr, "dns: %s @ %s\n", domain, addr)
	} else {
		fmt.Fprintln(os.Stderr, "dns: skipped")
	}
	return nil
}

// MakeApplyHook returns a callback for the provisioner's onApply when DeferHostGuestSetup is true.
// The callback configures network after the "workstation" Terraform component is applied, using
// DNS address from Terraform outputs when available. Returns nil when DeferHostGuestSetup is false.
func (w *Workstation) MakeApplyHook() func(componentID string) error {
	if !w.DeferHostGuestSetup {
		return nil
	}
	return func(componentID string) error {
		if componentID != "workstation" {
			return nil
		}
		dnsAddr := ""
		if w.runtime.TerraformProvider != nil {
			outputs, err := w.runtime.TerraformProvider.GetTerraformOutputs("workstation")
			if err == nil {
				for _, key := range []string{"dns_ip", "dns_address"} {
					if v, ok := outputs[key]; ok && v != nil {
						dnsAddr = fmt.Sprint(v)
						break
					}
				}
			}
		}
		return w.ConfigureNetwork(dnsAddr)
	}
}

// Down stops all components of the workstation environment including services, containers, VMs, and networking.
// It gracefully shuts down the container runtime and virtual machine if present. On success, it prints a
// confirmation message to standard error and returns nil. If any step of the teardown fails, it returns an error
// describing the issue.
func (w *Workstation) Down() error {
	workstationRuntime := w.configHandler.GetString("workstation.runtime")
	provider := w.configHandler.GetString("provider")

	if w.NetworkManager != nil && workstationRuntime == "colima" && provider == "incus" {
		if err := w.NetworkManager.ConfigureGuest(); err != nil {
			return fmt.Errorf("error configuring guest: %w", err)
		}
	}

	if w.ContainerRuntime != nil {
		if err := w.ContainerRuntime.Down(); err != nil {
			return fmt.Errorf("Error running container runtime Down command: %w", err)
		}
	}

	if w.VirtualMachine != nil && provider != "incus" {
		if err := w.VirtualMachine.Down(); err != nil {
			return fmt.Errorf("Error running virtual machine Down command: %w", err)
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// createServices creates and registers services based on configuration settings.
func (w *Workstation) createServices() ([]services.Service, error) {
	var serviceList []services.Service

	if !w.runtime.UsesDockerComposeWorkstation() {
		return serviceList, nil
	}

	// DNS Service
	dnsEnabled := w.configHandler.GetBool("dns.enabled", false)
	if dnsEnabled {
		service := services.NewDNSService(w.runtime)
		service.SetName("dns")
		serviceList = append(serviceList, service)
	}

	// Git Livereload Service
	gitEnabled := w.configHandler.GetBool("git.livereload.enabled", false)
	if gitEnabled {
		service := services.NewGitLivereloadService(w.runtime)
		service.SetName("git")
		serviceList = append(serviceList, service)
	}

	// Localstack Service
	awsEnabled := w.configHandler.GetBool("aws.localstack.enabled", false)
	if awsEnabled {
		service := services.NewLocalstackService(w.runtime)
		service.SetName("aws")
		serviceList = append(serviceList, service)
	}

	// Registry Services
	contextConfig := w.configHandler.GetConfig()
	if contextConfig.Docker != nil && contextConfig.Docker.Registries != nil {
		for key := range contextConfig.Docker.Registries {
			service := services.NewRegistryService(w.runtime)
			service.SetName(key)
			serviceList = append(serviceList, service)
		}
	}

	// Cluster Services - only create Talos services for Docker runtime
	// For Incus, Talos nodes are created via blueprint/terraform
	if w.configHandler.GetString("provider") != "incus" {
		clusterDriver := w.configHandler.GetString("cluster.driver", "")
		switch clusterDriver {
		case "talos", "omni":
			controlPlaneCount := w.configHandler.GetInt("cluster.controlplanes.count")
			workerCount := w.configHandler.GetInt("cluster.workers.count")

			for i := 1; i <= controlPlaneCount; i++ {
				service := services.NewTalosService(w.runtime, "controlplane")
				serviceName := fmt.Sprintf("controlplane-%d", i)
				service.SetName(serviceName)
				serviceList = append(serviceList, service)
			}

			for i := 1; i <= workerCount; i++ {
				service := services.NewTalosService(w.runtime, "worker")
				serviceName := fmt.Sprintf("worker-%d", i)
				service.SetName(serviceName)
				serviceList = append(serviceList, service)
			}
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
