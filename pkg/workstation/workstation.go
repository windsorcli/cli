package workstation

import (
	"errors"
	"fmt"
	"os"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/workstation/network"
	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// ErrClusterPrivilegeRequired is returned from MakeApplyHook when the workstation Terraform
// component has just applied on a VM-backed runtime whose cluster IP is not reachable from the
// host without elevated configuration (host route + in-VM forwarding). It is a clean halt, not a
// failure: the operator runs 'windsor configure network' from an elevated shell and then re-runs
// 'windsor up' to apply the remainder of the blueprint.
var ErrClusterPrivilegeRequired = errors.New(
	"cluster reachability requires running 'windsor configure network' from an elevated shell (one-time per host), then re-run 'windsor up'",
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

	deferredWork []DeferredWorkItem
}

// DeferredWorkItem describes a step the apply skipped because it requires elevation the
// in-process Up() will not request. The end-of-run summary in cmd/up renders these into
// operator-facing guidance. Required items denote a halt (subsequent components were skipped
// and the operator must re-run 'windsor up' after acting); optional items denote work the
// operator can do at their convenience without re-running.
type DeferredWorkItem struct {
	Required bool
	Outcome  string
	Command  string
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
	w.deferredWork = nil
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
		dnsDomain := w.configHandler.GetString("dns.domain")
		if dnsDomain != "" && w.configHandler.GetString("workstation.dns.address") != "" {
			if err := w.NetworkManager.ConfigureDNS(); err != nil {
				return fmt.Errorf("error configuring DNS: %w", err)
			}
			if w.NetworkManager.DNSChanged() {
				if err := w.FlushDNS(); err != nil {
					return fmt.Errorf("error flushing DNS cache: %w", err)
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

// ConfigureNetwork runs host/guest and DNS setup. Workstation address and DNS config are
// expected in the config handler, loaded from .windsor/contexts/<context>/workstation.yaml (written during
// windsor up) or set explicitly. dnsAddressOverride (from --dns-address flag or Terraform
// output) takes priority over config. DNS is configured whenever dns.domain and the resolver
// address are both available — the operator opts in by running 'windsor configure network'.
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
	dnsDomain := w.configHandler.GetString("dns.domain")
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
	return nil
}

// RevertNetwork undoes the host configuration that ConfigureNetwork applied: removes the host
// route + in-VM forwarding on VM-backed runtimes, and removes the per-domain DNS resolver entry.
// Each step is idempotent — the corresponding NetworkManager.Revert* method tolerates missing
// state — so this is safe to call after partial configuration or against contexts that were
// never configured. No-op when NetworkManager is nil. showStatus emits one line per step to
// stderr; suppress it for non-interactive callers.
func (w *Workstation) RevertNetwork(showStatus bool) error {
	if w.NetworkManager == nil {
		return nil
	}
	workstationRuntime := w.configHandler.GetString("workstation.runtime")
	if workstationRuntime == "colima" {
		if err := w.NetworkManager.RevertGuest(); err != nil {
			return fmt.Errorf("error reverting guest: %w", err)
		}
		if err := w.NetworkManager.RevertHostRoute(); err != nil {
			return fmt.Errorf("error reverting host route: %w", err)
		}
		if showStatus {
			fmt.Fprintln(os.Stderr, "network: reverted")
		}
	} else if showStatus {
		fmt.Fprintln(os.Stderr, "network: skipped (not colima)")
	}
	if err := w.NetworkManager.RevertDNS(); err != nil {
		return fmt.Errorf("error reverting DNS: %w", err)
	}
	if showStatus {
		fmt.Fprintln(os.Stderr, "dns: reverted")
	}
	return nil
}

// MakeApplyHook returns a callback for the provisioner's onApply when DeferHostGuestSetup is true.
// The callback persists DNS-related outputs from the just-applied workstation Terraform component,
// then resolves cluster reachability. The hook returns (haltAfter, err): on runtimes that need a
// host route + in-VM forwarding to reach the cluster (colima today), if the process can elevate
// non-interactively (root or cached sudo), the cluster-privilege work runs inline and the hook
// returns (false, nil); otherwise the hook returns (false, ErrClusterPrivilegeRequired) so the
// operator can run 'windsor configure network' and re-run 'windsor up'. DNS resolver configuration
// is not applied here — it's a post-`up` concern handled by cmd/up. Returns nil when
// DeferHostGuestSetup is false.
func (w *Workstation) MakeApplyHook() func(componentID string) (bool, error) {
	if !w.DeferHostGuestSetup {
		return nil
	}
	return func(componentID string) (bool, error) {
		if componentID != "workstation" {
			return false, nil
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
			return false, fmt.Errorf("error writing workstation state: %w", err)
		}
		if w.NetworkManager == nil || !w.NetworkManager.NeedsPrivilegeForCluster() {
			return false, nil
		}
		if !canElevateNonInteractively(w.shell) {
			return false, ErrClusterPrivilegeRequired
		}
		if err := w.NetworkManager.ConfigureGuest(); err != nil {
			return false, fmt.Errorf("error configuring guest: %w", err)
		}
		if err := w.NetworkManager.ConfigureHostRoute(); err != nil {
			return false, fmt.Errorf("error configuring host route: %w", err)
		}
		return false, nil
	}
}

// FlushDNS flushes the DNS cache when DNS is fully configured.
// It is a no-op when the network manager is absent or DNS domain/address are not set.
func (w *Workstation) FlushDNS() error {
	if w.NetworkManager == nil {
		return nil
	}
	dnsDomain := w.configHandler.GetString("dns.domain")
	dnsAddress := w.configHandler.GetString("workstation.dns.address")
	if dnsDomain != "" && dnsAddress != "" {
		return w.NetworkManager.FlushDNS()
	}
	return nil
}

// MakePostApplyHook returns a callback for the provisioner's postApply when DeferHostGuestSetup is true.
// The callback flushes the DNS cache after the "workstation" Terraform component's Done line is printed,
// so the elevated-privilege prompt appears after the spinner completes rather than during it.
// Returns nil when DeferHostGuestSetup is false.
func (w *Workstation) MakePostApplyHook() func(componentID string) error {
	if !w.DeferHostGuestSetup {
		return nil
	}
	return func(componentID string) error {
		if componentID != "workstation" {
			return nil
		}
		if w.NetworkManager == nil || !w.NetworkManager.DNSChanged() {
			return nil
		}
		return w.FlushDNS()
	}
}

// Down reverts host network configuration (when present and elevation is available without
// prompting) and stops the workstation environment: container runtime, then VM. The revert
// step, when it fires, runs FIRST so that RevertGuest can SSH into the still-running VM to
// remove iptables rules; host route and resolver entries are then removed on the host. If the
// process can't elevate without prompting, the revert is skipped and a one-line hint is
// printed at the END (after teardown) so the last thing the operator sees is the actionable
// guidance — surprise sudo prompts during 'windsor down' would undermine the no-prompts
// contract this command exists to support. Revert failures are warned and do not halt
// teardown — the operator's primary intent is to stop the workstation. Workstation state is
// preserved so that 'windsor up' can resume cleanly.
func (w *Workstation) Down() error {
	hintLeftoverHostConfig := false
	if w.NetworkManager != nil && (w.NetworkManager.IsHostRouteInstalled() || w.NetworkManager.IsResolverInstalled()) {
		if canElevateNonInteractively(w.shell) {
			if err := w.RevertNetwork(true); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to revert host network configuration: %v\n", err)
			}
		} else {
			hintLeftoverHostConfig = true
		}
	}

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

	if hintLeftoverHostConfig {
		fmt.Fprintln(os.Stderr, "host network configuration remains; run 'windsor configure network --revert' from an elevated shell to remove")
	}

	return nil
}

// WriteState delegates to ConfigHandler.SaveWorkstationState to persist workstation-managed
// config keys (workstation.*, platform, dns.*) to .windsor/contexts/<context>/workstation.yaml.
func (w *Workstation) WriteState() error {
	return w.configHandler.SaveWorkstationState()
}

// DeferredWork returns the deferred-work items accumulated during the most recent Up().
// The slice is reset at the start of each Up; callers should read it after Up returns.
// Returns nil when nothing was deferred.
func (w *Workstation) DeferredWork() []DeferredWorkItem {
	return w.deferredWork
}

// NetworkChange is one row of structured output for PendingNetworkChanges. Kind is a stable
// kebab-case identifier ("host-route", "vm-forward", "dns-resolver") and Detail is the value
// being installed ("192.168.5.0/24 via 192.168.5.10"). Callers are expected to render these
// as aligned columns rather than baking the kind into a prose sentence.
type NetworkChange struct {
	Kind   string
	Detail string
}

// PendingNetworkChanges returns the host configuration changes that 'windsor configure network'
// would apply for the current context. Returns an empty slice when nothing is pending. Returns
// nil when the workstation has no NetworkManager.
func (w *Workstation) PendingNetworkChanges() []NetworkChange {
	if w.NetworkManager == nil {
		return nil
	}
	var changes []NetworkChange
	if w.NetworkManager.NeedsPrivilegeForCluster() {
		cidr := w.configHandler.GetString("network.cidr_block")
		gateway := w.configHandler.GetString("workstation.address")
		changes = append(changes,
			NetworkChange{Kind: "host-route", Detail: fmt.Sprintf("%s via %s", cidr, gateway)},
			NetworkChange{Kind: "vm-forward", Detail: "col0 -> docker bridge"},
		)
	}
	if w.NetworkManager.NeedsPrivilegeForDNS() {
		domain := w.configHandler.GetString("dns.domain")
		address := w.configHandler.GetString("workstation.dns.address")
		changes = append(changes, NetworkChange{
			Kind:   "dns-resolver",
			Detail: fmt.Sprintf("*.%s -> %s", domain, address),
		})
	}
	return changes
}

// SetGeteuidForTest replaces the package's geteuid implementation for the duration of a test and
// returns a function that restores the previous implementation. Intended for cross-package tests
// (e.g. pkg/project) that need to drive canElevateNonInteractively through a known path.
func SetGeteuidForTest(fn func() int) func() {
	original := geteuidFunc
	geteuidFunc = fn
	return func() { geteuidFunc = original }
}

// =============================================================================
// Private Methods
// =============================================================================

// geteuidFunc is the seam for testing canElevateNonInteractively. Tests override this to simulate
// a root or non-root process without depending on the actual euid of the test runner.
var geteuidFunc = os.Geteuid

// appendDeferredWork records a step that the apply skipped because it requires elevation Up()
// will not request. cmd/up reads these via DeferredWork() after Up returns to render the
// end-of-run summary.
func (w *Workstation) appendDeferredWork(item DeferredWorkItem) {
	w.deferredWork = append(w.deferredWork, item)
}

// canElevateNonInteractively reports whether the current process can run privileged commands
// without prompting the operator: either it is already root (CI as root, sudo windsor up) or
// passwordless sudo is cached for the current user. Used to decide between inline privilege
// application and the operator-facing halt-and-resume model on commands like 'windsor up'.
func canElevateNonInteractively(sh shell.Shell) bool {
	if geteuidFunc() == 0 {
		return true
	}
	_, err := sh.ExecSilent("sudo", "-n", "true")
	return err == nil
}

