package cmd

import (
	"fmt"
	"os"
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
	Short: "Configure workstation resources.",
	Long:  `Configure workstation host/guest resources. Currently supports networking and DNS via the 'network' subcommand.`,
	Annotations: map[string]string{
		"docs.seealso": "[Workstation overview](https://www.windsorcli.dev/docs/workstation/overview)\n" +
			"[`up`](up.md)",
		"docs.source": "cmd/configure.go",
	},
}

var configureNetworkCmd = &cobra.Command{
	Use:   "network",
	Short: "Configure workstation host/guest networking and DNS.",
	Long: `Run after 'windsor up' has provisioned the workstation. Installs the host route and in-VM forwarding required for cluster reachability on VM-backed runtimes, and writes the per-domain DNS resolver entry so '*.<dns.domain>' resolves to the cluster's DNS service.

Prompts for sudo on macOS/Linux; must be run from an Administrator PowerShell on Windows. Use --dry-run to preview without modifying host state, or --revert to remove the host configuration this command previously installed.

If the current context has no workstation enabled, the command is a no-op and prints 'workstation disabled'.`,
	Example: `# Wire network using the DNS address from Terraform workstation output
windsor configure network

# Preview the changes without invoking sudo
windsor configure network --dry-run

# Wire network and explicitly set the DNS service address
windsor configure network --dns-address=10.5.0.2

# Remove the host configuration installed by this command
windsor configure network --revert`,
	Annotations: map[string]string{
		"docs.seealso": "[Workstation overview](https://www.windsorcli.dev/docs/workstation/overview)\n" +
			"[`up`](up.md)",
		"docs.source": "cmd/configure.go",
	},
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
		// IsProvisioned reads the canonical state file written by MakeApplyHook after the TF
		// outputs are persisted — its presence means 'windsor up' has reached the cluster-
		// reachability handoff and we have real values (DNS address, runtime, etc.) on hand.
		provisioned, err := proj.Workstation.IsProvisioned()
		if err != nil {
			return err
		}
		if !provisioned {
			return fmt.Errorf("workstation has not been provisioned yet for context %q. Run 'windsor up' first, then re-run 'windsor configure network'", proj.Runtime.ConfigHandler.GetContext())
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

func init() {
	configureNetworkCmd.Flags().StringVar(&configureNetworkDnsAddress, "dns-address", "", "DNS service address (e.g. from Terraform workstation output)")
	configureNetworkCmd.Flags().BoolVar(&configureNetworkDryRun, "dry-run", false, "Describe what 'configure network' would do without invoking sudo or modifying host state")
	configureNetworkCmd.Flags().BoolVar(&configureNetworkRevert, "revert", false, "Remove the host route, in-VM forwarding, and DNS resolver entry previously installed by 'configure network'")
	configureCmd.AddCommand(configureNetworkCmd)
	rootCmd.AddCommand(configureCmd)
}
