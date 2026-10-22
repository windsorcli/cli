package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner"
	"github.com/windsorcli/cli/pkg/runtime"
)

var (
	nodeHealthTimeout       time.Duration
	nodeHealthNodes         []string
	nodeHealthVersion       string
	k8sEndpoint             string
	checkNodeReady          bool
	nodeHealthSkipServices  []string
	nodeHealthWaitForReboot bool
)

// checkCmd verifies the local tools, their versions and the cloud credentials.
// It runs CheckAuth, which runs the cloud CLI. init and env do not run CheckAuth, because they must not use the cloud CLI.
var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Verify required tools are installed.",
	Long: `Runs the standard preflight in two passes: tool version checks for local CLIs (terraform, kubectl, talosctl, etc.), then a credential check for the platform configured on the current context (e.g. 'aws sts get-caller-identity' for platform aws).

Fails fast if a required tool is missing or at the wrong version, or if credentials don't resolve.`,
	Example: `# Verify the toolchain and cloud credentials
windsor check`,
	Annotations: map[string]string{
		"docs.seealso": "[`upgrade`](upgrade.md), [`up`](up.md)",
		"docs.source":  "cmd/check.go",
	},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			rtOpts = []*runtime.Runtime{overridesVal.(*runtime.Runtime)}
		}

		rt := runtime.NewRuntime(rtOpts...)

		if err := rt.Shell.CheckTrustedDirectory(); err != nil {
			return werror.New("not in a trusted directory", trustDirectoryHint, err)
		}

		if err := rt.ConfigHandler.LoadConfig(); err != nil {
			return err
		}

		if !rt.ConfigHandler.IsLoaded() {
			return werror.New("nothing to check", initProjectHint, nil)
		}

		if err := rt.CheckTools(); err != nil {
			return err
		}

		if err := rt.ToolsManager.CheckAuth(); err != nil {
			return err
		}

		return nil
	},
}

var checkNodeHealthCmd = &cobra.Command{
	Use:   "node-health",
	Short: "Check the health of cluster nodes.",
	Long: `Probe one or more cluster nodes for readiness. Useful after 'windsor upgrade' or for routine monitoring.

At least one of --nodes or --k8s-endpoint must be set.`,
	Example: `# Health-check one node, polling through a reboot
windsor check node-health --nodes=10.0.0.5 --wait-for-reboot

# Verify all nodes report Ready via the configured Kubernetes endpoint
windsor check node-health --k8s-endpoint --ready

# Check a specific Talos version on a set of nodes
windsor check node-health --nodes=10.0.0.5,10.0.0.6 --version=v1.13.3`,
	Annotations: map[string]string{
		"docs.seealso": "[`upgrade`](upgrade.md), [`up`](up.md)",
		"docs.source":  "cmd/check.go",
	},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		if len(nodeHealthNodes) == 0 && k8sEndpoint == "" {
			return werror.New("no health checks specified", "Set --nodes, --k8s-endpoint, or both.", nil)
		}

		if !cmd.Flags().Changed("timeout") {
			nodeHealthTimeout = constants.DefaultNodeHealthCheckTimeout
		}

		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			rtOpts = []*runtime.Runtime{overridesVal.(*runtime.Runtime)}
		}

		rt := runtime.NewRuntime(rtOpts...)

		if err := rt.Shell.CheckTrustedDirectory(); err != nil {
			return werror.New("not in a trusted directory", trustDirectoryHint, err)
		}

		if err := rt.ConfigHandler.LoadConfig(); err != nil {
			return err
		}

		if !rt.ConfigHandler.IsLoaded() {
			return werror.New("nothing to check", initProjectHint, nil)
		}

		comp := composer.NewComposer(rt)
		prov := provisioner.NewProvisioner(rt, comp.BlueprintHandler)

		outputFunc := func(output string) {
			fmt.Fprintln(cmd.OutOrStdout(), output)
		}

		k8sEndpointStr := k8sEndpoint
		if k8sEndpointStr == "" && checkNodeReady {
			k8sEndpointStr = "true"
		}

		options := provisioner.NodeHealthCheckOptions{
			Nodes:               nodeHealthNodes,
			Timeout:             nodeHealthTimeout,
			Version:             nodeHealthVersion,
			K8SEndpoint:         k8sEndpointStr,
			K8SEndpointProvided: k8sEndpoint != "" || checkNodeReady,
			CheckNodeReady:      checkNodeReady,
			SkipServices:        nodeHealthSkipServices,
			WaitForReboot:       nodeHealthWaitForReboot,
		}

		if err := prov.CheckNodeHealth(cmd.Context(), options, outputFunc); err != nil {
			return werror.Wrap(err, "error checking node health")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)
	checkCmd.AddCommand(checkNodeHealthCmd)

	// Add flags for node health check
	checkNodeHealthCmd.Flags().DurationVar(&nodeHealthTimeout, "timeout", 0, "Maximum time to wait for nodes to be ready. Default 5m.")
	checkNodeHealthCmd.Flags().StringSliceVar(&nodeHealthNodes, "nodes", []string{}, "Node addresses to check.")
	checkNodeHealthCmd.Flags().StringVar(&nodeHealthVersion, "version", "", "Expected Talos version. Reports a mismatch if set.")
	checkNodeHealthCmd.Flags().StringVar(&k8sEndpoint, "k8s-endpoint", "", "Probe the Kubernetes API at this URL, or pass without value to use the configured endpoint.")
	checkNodeHealthCmd.Flags().Lookup("k8s-endpoint").NoOptDefVal = "true"
	checkNodeHealthCmd.Flags().BoolVar(&checkNodeReady, "ready", false, "Check Kubernetes node readiness in addition to Talos.")
	checkNodeHealthCmd.Flags().StringSliceVar(&nodeHealthSkipServices, "skip-services", []string{}, "Service names to ignore (e.g., dashboard).")
	checkNodeHealthCmd.Flags().BoolVar(&nodeHealthWaitForReboot, "wait-for-reboot", false, "Poll until the Talos API goes offline (reboot started), then wait for it to come back.")
}
