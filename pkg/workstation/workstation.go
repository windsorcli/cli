// The Workstation manages the VM, the container runtime, and the host network of a context.
// It starts and stops the VM and the container runtime, and it configures the guest, the host route, and the DNS.
// Terraform in the stack runs the services of the workstation, for example DNS, registries, and Talos.
// When the blueprint has a workstation component, the host network setup runs after that component applies.

package workstation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/shims"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/workstation/network"
	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// =============================================================================
// Types
// =============================================================================

// Workstation manages workstation virtualization, networking, and SSH operations.
type Workstation struct {
	configHandler config.ConfigHandler
	shell         shell.Shell
	evaluator     evaluator.ExpressionEvaluator
	runtime       *runtime.Runtime
	shims         *shims.Shims

	NetworkManager   network.NetworkManager
	VirtualMachine   virt.VirtualMachine
	ContainerRuntime virt.ContainerRuntime

	// DeferHostGuestSetup makes Up skip the guest, host route, and DNS setup. PrepareForUp sets it.
	DeferHostGuestSetup bool

	deferredWork []DeferredWorkItem
}

// DeferredWorkItem is a step that Windsor skipped because it needs elevation that Windsor does not request.
// The end-of-run summary of cmd/up shows each item to the operator.
// A required item stopped the apply, so the operator runs the command and then 'windsor up' again.
// An optional item is work that the operator can do later without a new 'windsor up'.
type DeferredWorkItem struct {
	Required bool
	Outcome  string
	Command  string
}

// NetworkChange is one host change from PendingNetworkChanges.
// Kind is host-route, vm-forward, or dns-resolver. Detail is the value, for example "192.168.5.0/24 via 192.168.5.10".
// Callers show the changes as aligned columns.
type NetworkChange struct {
	Kind   string
	Detail string
}

// =============================================================================
// Constructor
// =============================================================================

// NewWorkstation returns a Workstation for rt. It uses the network manager, the VM, and the
// container runtime from opts when they are set. Prepare creates the ones that are not set.
// It panics when rt, its config handler, its shell, or its evaluator is nil.
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
		shims:         shims.NewShims(),
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

// Prepare creates the network manager, the VM, and the container runtime that the context needs and that are not set.
// Call it after the configuration is loaded.
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

// Up sets NO_CACHE, starts the VM and the container runtime, and configures the network.
// It configures the guest, the host route, and the DNS only when DeferHostGuestSetup is false.
// Otherwise MakeApplyHook and 'windsor configure network' do that work.
// Call Prepare before Up.
func (w *Workstation) Up(ctx context.Context) error {
	w.deferredWork = nil
	if err := w.shims.Setenv("NO_CACHE", "true"); err != nil {
		return werror.Wrap(err, "Error setting NO_CACHE environment variable")
	}

	workstationRuntime := w.configHandler.GetString("workstation.runtime")
	if workstationRuntime == "colima" && w.VirtualMachine != nil {
		if err := w.VirtualMachine.WriteConfig(); err != nil {
			return werror.Wrap(err, "error writing virtual machine config")
		}
		if err := w.VirtualMachine.Up(ctx); err != nil {
			return werror.Wrap(err, "error running virtual machine Up command")
		}
		if err := w.WriteState(); err != nil {
			return werror.Wrap(err, "error writing workstation state")
		}
		if w.NetworkManager != nil && w.configHandler.GetString("platform") == "incus" {
			if err := w.NetworkManager.ConfigureGuest(); err != nil {
				return werror.Wrap(err, "error configuring guest SSH")
			}
		}
	}

	if w.ContainerRuntime != nil {
		if err := w.ContainerRuntime.WriteConfig(); err != nil {
			return werror.Wrap(err, "failed to write container runtime config")
		}
		if err := w.ContainerRuntime.Up(ctx); err != nil {
			return werror.Wrap(err, "error running container runtime Up command")
		}
	}

	if w.NetworkManager != nil && !w.DeferHostGuestSetup {
		if workstationRuntime == "colima" {
			if err := w.NetworkManager.ConfigureGuest(); err != nil {
				return werror.Wrap(err, "error configuring guest")
			}
			if err := w.NetworkManager.ConfigureHostRoute(ctx); err != nil {
				return werror.Wrap(err, "error configuring host route")
			}
		}
		dnsDomain := w.configHandler.GetString("dns.domain")
		if dnsDomain != "" && w.configHandler.GetString("workstation.dns.address") != "" {
			if err := w.NetworkManager.ConfigureDNS(ctx); err != nil {
				return werror.Wrap(err, "error configuring DNS")
			}
			if w.NetworkManager.DNSChanged() {
				if err := w.FlushDNS(ctx); err != nil {
					return werror.Wrap(err, "error flushing DNS cache")
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

// ConfigureNetwork configures the guest and the host route on colima, and then the DNS.
// It reads the addresses from the config, which windsor up saves to workstation.yaml.
// A dnsAddressOverride that is not empty replaces workstation.dns.address.
// It configures the DNS only when dns.domain and the DNS address are both set.
// When showStatus is true, it sends one status message for each step to the presenter.
// It does nothing when NetworkManager is nil.
func (w *Workstation) ConfigureNetwork(ctx context.Context, dnsAddressOverride string, showStatus bool) error {
	if w.NetworkManager == nil {
		return nil
	}
	if dnsAddressOverride != "" {
		_ = w.configHandler.Set("workstation.dns.address", dnsAddressOverride)
	}
	workstationRuntime := w.configHandler.GetString("workstation.runtime")
	if workstationRuntime == "colima" {
		if err := w.NetworkManager.ConfigureGuest(); err != nil {
			return werror.Wrap(err, "error configuring guest")
		}
		if err := w.NetworkManager.ConfigureHostRoute(ctx); err != nil {
			return werror.Wrap(err, "error configuring host route")
		}
		if showStatus {
			presenter.Message(ctx, "network: ready")
		}
	} else if showStatus {
		presenter.Message(ctx, "network: skipped (not colima)")
	}
	dnsDomain := w.configHandler.GetString("dns.domain")
	if dnsDomain != "" && w.configHandler.GetString("workstation.dns.address") != "" {
		if err := w.NetworkManager.ConfigureDNS(ctx); err != nil {
			return werror.Wrap(err, "error configuring DNS")
		}
		if showStatus {
			presenter.Message(ctx, fmt.Sprintf("dns: %s @ %s", w.configHandler.GetString("dns.domain"), w.configHandler.GetString("workstation.dns.address")))
		}
	} else if showStatus {
		presenter.Message(ctx, "dns: skipped (domain or address not set)")
	}
	return nil
}

// RevertNetwork removes the host configuration that ConfigureNetwork installed.
// On colima it removes the guest forwarding and the host route. It always removes the DNS resolver entry.
// Each NetworkManager revert does nothing when its state is absent, so a partial configuration is safe to revert.
// When showStatus is true, it sends one status message for each step to the presenter.
// It does nothing when NetworkManager is nil.
func (w *Workstation) RevertNetwork(ctx context.Context, showStatus bool) error {
	if w.NetworkManager == nil {
		return nil
	}
	workstationRuntime := w.configHandler.GetString("workstation.runtime")
	if workstationRuntime == "colima" {
		if err := w.NetworkManager.RevertGuest(); err != nil {
			return werror.Wrap(err, "error reverting guest")
		}
		if err := w.NetworkManager.RevertHostRoute(); err != nil {
			return werror.Wrap(err, "error reverting host route")
		}
		if showStatus {
			presenter.Message(ctx, "network: reverted")
		}
	} else if showStatus {
		presenter.Message(ctx, "network: skipped (not colima)")
	}
	if err := w.NetworkManager.RevertDNS(); err != nil {
		return werror.Wrap(err, "error reverting DNS")
	}
	if showStatus {
		presenter.Message(ctx, "dns: reverted")
	}
	return nil
}

// MakeApplyHook returns the onApply callback of the provisioner, or nil when DeferHostGuestSetup is false.
// After the workstation component, the callback saves its DNS address output and the workstation state.
// When the cluster needs a host route, it adds a required DeferredWorkItem and returns true to stop the stack.
// When the DNS resolver needs a change, it adds an optional DeferredWorkItem and the stack continues.
// The callback never prompts for sudo.
func (w *Workstation) MakeApplyHook() func(ctx context.Context, componentID string) (bool, error) {
	if !w.DeferHostGuestSetup {
		return nil
	}
	return func(ctx context.Context, componentID string) (bool, error) {
		if componentID != "workstation" {
			return false, nil
		}
		dnsAddr := ""
		if w.runtime.TerraformProvider != nil {
			outputs, err := w.runtime.TerraformProvider.GetTerraformOutputs("workstation")
			if err != nil {
				return false, werror.Wrap(err, "error reading workstation terraform outputs")
			}
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
		if dnsAddr != "" {
			if err := w.configHandler.Set("workstation.dns.address", dnsAddr); err != nil {
				return false, werror.Wrap(err, "error persisting workstation.dns.address from terraform output")
			}
		}
		if err := w.WriteState(); err != nil {
			return false, werror.Wrap(err, "error writing workstation state")
		}
		if w.NetworkManager == nil {
			return false, nil
		}
		halt := false
		if w.NetworkManager.NeedsPrivilegeForCluster() {
			w.appendDeferredWork(DeferredWorkItem{
				Required: true,
				Command:  "windsor configure network",
			})
			halt = true
		}
		if w.NetworkManager.NeedsPrivilegeForDNS() {
			domain := w.configHandler.GetString("dns.domain")
			w.appendDeferredWork(DeferredWorkItem{
				Command: "windsor configure network",
				Outcome: fmt.Sprintf("use *.%s in your browser", domain),
			})
		}
		return halt, nil
	}
}

// FlushDNS flushes the DNS cache when DNS is fully configured.
// It is a no-op when the network manager is absent or DNS domain/address are not set.
func (w *Workstation) FlushDNS(ctx context.Context) error {
	if w.NetworkManager == nil {
		return nil
	}
	dnsDomain := w.configHandler.GetString("dns.domain")
	dnsAddress := w.configHandler.GetString("workstation.dns.address")
	if dnsDomain != "" && dnsAddress != "" {
		return w.NetworkManager.FlushDNS(ctx)
	}
	return nil
}

// MakePostApplyHook returns the postApply callback of the provisioner, or nil when DeferHostGuestSetup is false.
// After the workstation component, the callback flushes the DNS cache when the DNS changed.
func (w *Workstation) MakePostApplyHook() func(ctx context.Context, componentID string) error {
	if !w.DeferHostGuestSetup {
		return nil
	}
	return func(ctx context.Context, componentID string) error {
		if componentID != "workstation" {
			return nil
		}
		if w.NetworkManager == nil || !w.NetworkManager.DNSChanged() {
			return nil
		}
		return w.FlushDNS(ctx)
	}
}

// Down removes the installed host network configuration, and then stops the container runtime and the VM.
// The revert runs first, so RevertGuest can reach the running VM. A revert failure is a warning.
// When Windsor cannot elevate without a prompt, it skips the revert and adds a DeferredWorkItem.
// Down keeps the workstation state, so 'windsor up' can start again.
func (w *Workstation) Down(ctx context.Context) error {
	w.deferredWork = nil
	if w.NetworkManager != nil && (w.NetworkManager.IsHostRouteInstalled() || w.NetworkManager.IsResolverInstalled()) {
		if canElevateNonInteractively(w.shell) {
			if err := w.RevertNetwork(ctx, true); err != nil {
				presenter.Warn(ctx, fmt.Sprintf("failed to revert host network configuration: %v", err))
			}
		} else {
			w.appendDeferredWork(DeferredWorkItem{
				Command: "windsor configure network --revert",
				Outcome: "remove host configuration",
			})
		}
	}

	platform := w.configHandler.GetString("platform")

	if w.ContainerRuntime != nil {
		if err := w.ContainerRuntime.Down(ctx); err != nil {
			return werror.Wrap(err, "Error running container runtime Down command")
		}
	}

	if w.VirtualMachine != nil && platform != "incus" {
		if err := w.VirtualMachine.Down(ctx); err != nil {
			return werror.Wrap(err, "Error running virtual machine Down command")
		}
	}

	return nil
}

// WriteState saves the workstation keys of the config to .windsor/contexts/<context>/workstation.yaml.
func (w *Workstation) WriteState() error {
	return w.configHandler.SaveWorkstationState()
}

// IsProvisioned reports whether 'windsor up' applied the workstation Terraform component of the context.
// It checks for <projectRoot>/.windsor/contexts/<context>/workstation.yaml. WriteState writes this file
// only after MakeApplyHook saves the Terraform outputs of the component.
// 'windsor configure network' uses it as a precondition. It returns false and no error when the file
// does not exist. It returns an error when it cannot get the project root or check the file.
func (w *Workstation) IsProvisioned() (bool, error) {
	projectRoot, err := w.shell.GetProjectRoot()
	if err != nil {
		return false, werror.Wrap(err, "error resolving project root")
	}
	statePath := filepath.Join(projectRoot, ".windsor", "contexts", w.configHandler.GetContext(), "workstation.yaml")
	if _, err := w.shims.Stat(statePath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, werror.Wrap(err, "error checking workstation state")
	}
	return true, nil
}

// DeferredWork returns the deferred work of the last Up, Down, or apply hook, or nil when there is none.
// Up and Down clear it when they start.
func (w *Workstation) DeferredWork() []DeferredWorkItem {
	return w.deferredWork
}

// PendingNetworkChanges returns the host changes that 'windsor configure network' would make for the context.
// It returns nil when nothing is pending or when NetworkManager is nil.
func (w *Workstation) PendingNetworkChanges() []NetworkChange {
	if w.NetworkManager == nil {
		return nil
	}
	var changes []NetworkChange
	if w.NetworkManager.NeedsPrivilegeForCluster() {
		cidr := w.configHandler.GetString("network.cidr_block")
		if cidr == "" {
			cidr = constants.DefaultNetworkCIDR
		}
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

// =============================================================================
// Private Methods
// =============================================================================

// appendDeferredWork adds item to the deferred work.
func (w *Workstation) appendDeferredWork(item DeferredWorkItem) {
	w.deferredWork = append(w.deferredWork, item)
}
