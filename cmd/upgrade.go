package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner"
	"github.com/windsorcli/cli/pkg/runtime"
)

var (
	upgradeNodes []string
	upgradeImage string

	upgradeNodeAddr    string
	upgradeNodeImage   string
	upgradeNodeTimeout time.Duration
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade cluster components.",
	Long:  `Upgrade cluster components. Currently supports Talos node upgrades via the 'cluster' (parallel) and 'node' (one-at-a-time, with health verification) subcommands.`,
	Annotations: map[string]string{
		"docs.seealso": "[`check node-health`](check-node-health.md)",
		"docs.source":  "cmd/upgrade.go",
	},
	SilenceUsage: true,
}

var upgradeClusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Upgrade cluster nodes in parallel.",
	Long: `Initiate a Talos upgrade on the named nodes in parallel. Returns once the upgrade requests are accepted; nodes reboot asynchronously.

Use 'windsor check node-health --wait-for-reboot' afterward to verify each node comes back healthy.`,
	Example: `# Upgrade all controlplane nodes in parallel
windsor upgrade cluster \
  --nodes=10.0.0.5,10.0.0.6,10.0.0.7 \
  --image=ghcr.io/siderolabs/installer:v1.13.0`,
	Annotations: map[string]string{
		"docs.seealso": "[`upgrade node`](upgrade-node.md)\n" +
			"[`check node-health`](check-node-health.md)",
		"docs.source": "cmd/upgrade.go",
	},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			rtOpts = []*runtime.Runtime{overridesVal.(*runtime.Runtime)}
		}

		rt := runtime.NewRuntime(rtOpts...)

		if err := rt.Shell.CheckTrustedDirectory(); err != nil {
			return fmt.Errorf("not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve")
		}

		if err := rt.ConfigHandler.LoadConfig(); err != nil {
			return err
		}

		if !rt.ConfigHandler.IsLoaded() {
			return fmt.Errorf("Nothing to upgrade. Have you run \033[1mwindsor init\033[0m?")
		}

		comp := composer.NewComposer(rt)
		prov := provisioner.NewProvisioner(rt, comp.BlueprintHandler)

		if err := prov.UpgradeNodes(cmd.Context(), upgradeNodes, upgradeImage); err != nil {
			return fmt.Errorf("node upgrade failed: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Successfully initiated upgrade for %d nodes to image %s\n", len(upgradeNodes), upgradeImage)

		return nil
	},
}

var upgradeNodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Upgrade a single cluster node and wait for it to rejoin.",
	Long:  `Send an upgrade request to a single Talos node, wait for it to reboot, then verify it is healthy. Suitable for rolling upgrades one node at a time.`,
	Example: `# Roll one node, blocking until it is healthy
windsor upgrade node --node=10.0.0.5 --image=ghcr.io/siderolabs/installer:v1.13.0

# Same with a longer timeout for slow rebooters
windsor upgrade node --node=10.0.0.5 --image=ghcr.io/siderolabs/installer:v1.13.0 --timeout=20m`,
	Annotations: map[string]string{
		"docs.seealso": "[`upgrade cluster`](upgrade-cluster.md)\n" +
			"[`check node-health`](check-node-health.md)",
		"docs.source": "cmd/upgrade.go",
	},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !cmd.Flags().Changed("timeout") {
			upgradeNodeTimeout = constants.DefaultNodeUpgradeTimeout
		}

		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			rtOpts = []*runtime.Runtime{overridesVal.(*runtime.Runtime)}
		}

		rt := runtime.NewRuntime(rtOpts...)

		if err := rt.Shell.CheckTrustedDirectory(); err != nil {
			return fmt.Errorf("not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve")
		}

		if err := rt.ConfigHandler.LoadConfig(); err != nil {
			return err
		}

		if !rt.ConfigHandler.IsLoaded() {
			return fmt.Errorf("Nothing to upgrade. Have you run \033[1mwindsor init\033[0m?")
		}

		comp := composer.NewComposer(rt)
		prov := provisioner.NewProvisioner(rt, comp.BlueprintHandler)

		ctx := cmd.Context()
		if upgradeNodeTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(cmd.Context(), upgradeNodeTimeout)
			defer cancel()
		}

		outputFunc := func(output string) {
			fmt.Fprintln(cmd.OutOrStdout(), output)
		}

		if err := prov.UpgradeNode(ctx, upgradeNodeAddr, upgradeNodeImage, constants.DefaultNodeOfflineTimeout, outputFunc); err != nil {
			return fmt.Errorf("node upgrade failed: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
	upgradeCmd.AddCommand(upgradeClusterCmd)
	upgradeCmd.AddCommand(upgradeNodeCmd)

	upgradeClusterCmd.Flags().StringSliceVar(&upgradeNodes, "nodes", []string{}, "Node addresses to upgrade. Required.")
	upgradeClusterCmd.Flags().StringVar(&upgradeImage, "image", "", "Talos image to upgrade to. Required.")
	_ = upgradeClusterCmd.MarkFlagRequired("nodes")
	_ = upgradeClusterCmd.MarkFlagRequired("image")

	upgradeNodeCmd.Flags().StringVar(&upgradeNodeAddr, "node", "", "Node IP address to upgrade. Required.")
	upgradeNodeCmd.Flags().StringVar(&upgradeNodeImage, "image", "", "Talos image to upgrade to. Required.")
	upgradeNodeCmd.Flags().DurationVar(&upgradeNodeTimeout, "timeout", 0, "Overall timeout. Default 10m.")
	_ = upgradeNodeCmd.MarkFlagRequired("node")
	_ = upgradeNodeCmd.MarkFlagRequired("image")
}
