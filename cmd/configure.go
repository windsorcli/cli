package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/workstation"
)

var (
	configureNetworkDnsAddress string
	configureNetworkDryRun     bool
	configureNetworkRevert     bool
)

// configureNetworkPreflight is the test seam for the per-OS elevation pre-flight in
// 'windsor configure network'. The default implementation lives in pkg/workstation (the
// privileged-elevation predicate sits next to canElevateNonInteractively); tests override this
// to exercise the gate-error path on platforms that wouldn't otherwise trigger it.
var configureNetworkPreflight = workstation.PreflightConfigureNetwork

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Configure Windsor resources",
	Long:  "Configure Windsor resources such as workstation host/guest networking and DNS.",
}

var configureNetworkCmd = &cobra.Command{
	Use:          "network",
	Short:        "Configure workstation host/guest networking and DNS",
	Long:         "Run after 'windsor up' has provisioned the workstation. Installs the host route + in-VM forwarding required for cluster reachability on VM-backed runtimes, and writes the per-domain DNS resolver entry so '*.<dns.domain>' resolves to the cluster's DNS service. Prompts for sudo on macOS/Linux; must be run from an Administrator PowerShell on Windows. Use --dns-address to override the DNS service address; --dry-run to describe what would change without invoking sudo; --revert to remove the host configuration this command previously installed.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var opts []*project.Project
		if overridesVal := cmd.Context().Value(projectOverridesKey); overridesVal != nil {
			if p, ok := overridesVal.(*project.Project); ok {
				opts = []*project.Project{p}
			}
		}

		proj := project.NewProject("", opts...)
		proj.Runtime.Shell.SetVerbosity(verbose)

		if err := proj.Runtime.Shell.CheckTrustedDirectory(); err != nil {
			return fmt.Errorf("not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve")
		}

		if err := proj.Configure(nil); err != nil {
			return err
		}

		if err := proj.Runtime.ConfigHandler.ValidateContextValues(); err != nil {
			return fmt.Errorf("invalid configuration: %w", err)
		}

		proj.EnsureWorkstation()
		if proj.Workstation == nil {
			fmt.Fprintln(os.Stderr, "workstation disabled")
			return nil
		}

		// Precondition: refuse to run before the workstation Terraform component has applied.
		// MakeApplyHook writes workstation.yaml after the workstation TF outputs are persisted,
		// so its presence is the canonical signal that 'windsor up' has reached the cluster-
		// reachability handoff point and we have real values (DNS address, runtime, etc.) to
		// install on the host.
		if err := ensureWorkstationProvisioned(proj); err != nil {
			return err
		}

		if err := proj.Workstation.Prepare(); err != nil {
			return err
		}
		if proj.Workstation.NetworkManager == nil {
			fmt.Fprintln(os.Stderr, "network: n/a")
			return nil
		}

		if configureNetworkDryRun {
			changes := proj.Workstation.PendingNetworkChanges()
			if len(changes) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "nothing pending")
				return nil
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			for _, c := range changes {
				fmt.Fprintf(tw, "%s\t%s\n", c.Kind, c.Detail)
			}
			return tw.Flush()
		}

		// Privileged work follows: on Windows the process itself must be elevated; on Unix
		// per-op sudo handles each step. The pre-flight runs after --dry-run (which never
		// modifies host state) so operators on non-Admin Windows can still preview changes.
		if err := configureNetworkPreflight(); err != nil {
			return err
		}

		if configureNetworkRevert {
			return proj.Workstation.RevertNetwork(true)
		}

		dnsAddr := configureNetworkDnsAddress
		if err := proj.Workstation.ConfigureNetwork(dnsAddr, true); err != nil {
			return err
		}
		return proj.Workstation.FlushDNS()
	},
}

// ensureWorkstationProvisioned errors when 'windsor up' has not yet reached the apply-hook
// handoff for the current context. Detection is by file presence:
// <projectRoot>/.windsor/contexts/<context>/workstation.yaml is written by Workstation.WriteState
// only after the workstation Terraform component applies. Operator-facing message points at
// 'windsor up' as the remediation.
func ensureWorkstationProvisioned(proj *project.Project) error {
	projectRoot, err := proj.Runtime.Shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("error resolving project root: %w", err)
	}
	context := proj.Runtime.ConfigHandler.GetContext()
	workstationYAML := filepath.Join(projectRoot, ".windsor", "contexts", context, "workstation.yaml")
	if _, err := os.Stat(workstationYAML); os.IsNotExist(err) {
		return fmt.Errorf("workstation has not been provisioned yet for context %q. Run 'windsor up' first, then re-run 'windsor configure network'", context)
	} else if err != nil {
		return fmt.Errorf("error checking workstation state: %w", err)
	}
	return nil
}

func init() {
	configureNetworkCmd.Flags().StringVar(&configureNetworkDnsAddress, "dns-address", "", "DNS service address (e.g. from Terraform workstation output)")
	configureNetworkCmd.Flags().BoolVar(&configureNetworkDryRun, "dry-run", false, "Describe what 'configure network' would do without invoking sudo or modifying host state")
	configureNetworkCmd.Flags().BoolVar(&configureNetworkRevert, "revert", false, "Remove the host route, in-VM forwarding, and DNS resolver entry previously installed by 'configure network'")
	configureCmd.AddCommand(configureNetworkCmd)
	rootCmd.AddCommand(configureCmd)
}
