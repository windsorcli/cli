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
	"github.com/windsorcli/cli/pkg/workstation/virt"
	"golang.org/x/term"
)

// The Workstation is a core component that manages workstation virtualization, networking, and SSH operations.
// It provides a unified interface for starting, stopping, and managing the VM and container runtime layer.
// Service orchestration (DNS, registries, localstack, Talos, etc.) is handled by Terraform in the stack.

// =============================================================================
// Types
// =============================================================================

// Workstation manages workstation virtualization, networking, and SSH operations.
type Workstation struct {
	configHandler config.ConfigHandler
	shell         shell.Shell
	evaluator     evaluator.ExpressionEvaluator
	runtime       *runtime.Runtime

	// Workstation-specific dependencies (created as needed)
	NetworkManager   network.NetworkManager
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

// Prepare creates workstation components (network manager, virtual machine, container runtime).
// Call after configuration is loaded, then returns an error if any component creation fails.
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

	if workstationRuntime == "colima" && w.VirtualMachine == nil {
		w.VirtualMachine = virt.NewColimaVirt(w.runtime)
	}

	platform := w.configHandler.GetString("platform")
	if platform == "incus" {
		if w.ContainerRuntime == nil {
			w.ContainerRuntime = virt.NewIncusVirt(w.runtime)
			if incusVirt, ok := w.ContainerRuntime.(*virt.IncusVirt); ok {
				w.VirtualMachine = incusVirt.ColimaVirt
			}
		}
	} else if platform == "docker" && w.ContainerRuntime == nil {
		w.ContainerRuntime = virt.NewDockerVirt(w.runtime)
	}

	return nil
}

// Up initializes the workstation environment: starts VMs, container runtime, and networking.
// Sets NO_CACHE, starts the virtual machine if configured, writes container runtime config,
// and configures networking. All components must be created via Prepare() before calling Up().
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
		if err := w.WriteState(); err != nil {
			return fmt.Errorf("error writing workstation state: %w", err)
		}
		if w.NetworkManager != nil && w.configHandler.GetString("platform") == "incus" {
			if err := w.NetworkManager.ConfigureGuest(); err != nil {
				return fmt.Errorf("error configuring guest SSH: %w", err)
			}
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
		dnsEnabled := w.configHandler.Get("dns.enabled")
		dnsDomain := w.configHandler.GetString("dns.domain")
		if dnsEnabled == nil || dnsEnabled == true {
			if dnsDomain != "" && w.configHandler.GetString("workstation.dns.address") != "" {
				if err := w.NetworkManager.ConfigureDNS(); err != nil {
					return fmt.Errorf("error configuring DNS: %w", err)
				}
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
	if term.IsTerminal(int(os.Stdin.Fd())) || stdruntime.GOOS == "windows" { // #nosec G115 -- file descriptors are small, safe to cast to int
		if os.Geteuid() != 0 {
			if _, err := w.shell.ExecSilent("sudo", "-n", "true"); err != nil {
				fmt.Fprintf(os.Stderr, "\033[33m⚠\033[0m Network configuration may require elevated privileges\n")
			}
		}
		if _, err := w.shell.ExecSudo("", "true"); err != nil {
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

// ConfigureNetwork runs host/guest and DNS setup. Workstation address and DNS config are
// expected in the config handler, loaded from .windsor/contexts/<context>/workstation.yaml (written during
// windsor up) or set explicitly. dnsAddressOverride (from --dns-address flag or Terraform
// output) takes priority over config. DNS is configured unless dns.enabled is explicitly false.
// No-op when NetworkManager is nil.
func (w *Workstation) ConfigureNetwork(dnsAddressOverride string, showStatus bool) error {
	if w.NetworkManager == nil {
		return nil
	}
	if dnsAddressOverride != "" {
		_ = w.configHandler.Set("workstation.dns.address", dnsAddressOverride)
	}
	workstationRuntime := w.configHandler.GetString("workstation.runtime")
	if workstationRuntime == "colima" {
		if err := w.NetworkManager.ConfigureGuest(); err != nil {
			return fmt.Errorf("error configuring guest: %w", err)
		}
		if err := w.NetworkManager.ConfigureHostRoute(); err != nil {
			return fmt.Errorf("error configuring host route: %w", err)
		}
		if showStatus {
			fmt.Fprintln(os.Stderr, "network: ready")
		}
	} else if showStatus {
		fmt.Fprintln(os.Stderr, "network: skipped (not colima)")
	}
	dnsEnabled := w.configHandler.Get("dns.enabled")
	dnsDomain := w.configHandler.GetString("dns.domain")
	if dnsEnabled == nil || dnsEnabled == true {
		if dnsDomain != "" && w.configHandler.GetString("workstation.dns.address") != "" {
			if err := w.NetworkManager.ConfigureDNS(); err != nil {
				return fmt.Errorf("error configuring DNS: %w", err)
			}
			if showStatus {
				fmt.Fprintf(os.Stderr, "dns: %s @ %s\n", w.configHandler.GetString("dns.domain"), w.configHandler.GetString("workstation.dns.address"))
			}
		} else if showStatus {
			fmt.Fprintln(os.Stderr, "dns: skipped (domain or address not set)")
		}
	} else if showStatus {
		fmt.Fprintln(os.Stderr, "dns: disabled")
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
						s := fmt.Sprint(v)
						if s != "" {
							dnsAddr = s
							break
						}
					}
				}
			}
		}
		if dnsAddr != "" {
			_ = w.configHandler.Set("workstation.dns.address", dnsAddr)
		}
		if err := w.WriteState(); err != nil {
			return fmt.Errorf("error writing workstation state: %w", err)
		}
		return w.ConfigureNetwork("", false)
	}
}

// Down stops the workstation environment: container runtime, then VM and networking.
// Gracefully shuts down the container runtime and virtual machine if present.
func (w *Workstation) Down() error {
	platform := w.configHandler.GetString("platform")

	if w.ContainerRuntime != nil {
		if err := w.ContainerRuntime.Down(); err != nil {
			return fmt.Errorf("Error running container runtime Down command: %w", err)
		}
	}

	if w.VirtualMachine != nil && platform != "incus" {
		if err := w.VirtualMachine.Down(); err != nil {
			return fmt.Errorf("Error running virtual machine Down command: %w", err)
		}
	}

	return nil
}

// WriteState delegates to ConfigHandler.SaveWorkstationState to persist workstation-managed
// config keys (workstation.*, platform, dns.*) to .windsor/contexts/<context>/workstation.yaml.
func (w *Workstation) WriteState() error {
	return w.configHandler.SaveWorkstationState()
}

