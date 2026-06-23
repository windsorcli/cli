package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/provisioner"
	"github.com/windsorcli/cli/pkg/provisioner/stacklock"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

var (
	upgradeNodes   []string
	upgradeImage   string
	upgradeSources []string

	upgradeNodeAddr    string
	upgradeNodeImage   string
	upgradeNodeTimeout time.Duration
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade the blueprint, pruning kustomizations it no longer declares.",
	Long: `Apply terraform and the Flux blueprint, wait for kustomizations to be ready, then prune any kustomizations this context no longer declares. Pruning runs only after a successful wait, so resources are never deleted before the desired set has reconciled.

Use the 'cluster' or 'node' subcommand to upgrade Talos nodes instead.`,
	Example: `# Upgrade the blueprint and prune orphaned kustomizations
windsor upgrade

# Upgrade Talos nodes in parallel (see 'upgrade cluster')
windsor upgrade cluster --nodes=10.0.0.5 --image=ghcr.io/siderolabs/installer:v1.13.0`,
	Annotations: map[string]string{
		"docs.seealso": "[`apply`](apply.md), [`bootstrap`](bootstrap.md), [`plan`](plan.md)",
		"docs.source":  "cmd/upgrade.go",
	},
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// `windsor upgrade` runs the full blueprint — terraform components and Flux
		// kustomizations both — so the tool surface is not statically narrowable; mirror
		// `apply` and request AllRequirements(), letting the per-tool config gates decide.
		proj, err := prepareProject(cmd, tools.AllRequirements())
		if err != nil {
			return err
		}

		if err := requireCloudAuth(cmd, proj); err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()
		if blueprint == nil {
			return fmt.Errorf("blueprint is not available")
		}

		if len(upgradeSources) > 0 {
			if err := retargetSources(cmd, proj, upgradeSources); err != nil {
				return err
			}
			blueprint = proj.Composer.BlueprintHandler.Generate()
			if blueprint == nil {
				return fmt.Errorf("blueprint is not available")
			}
		}

		return stacklock.With(cmd.Context(), proj.Runtime, "upgrade", func() error {
			if _, err := proj.Provisioner.Up(blueprint); err != nil {
				return fmt.Errorf("error applying terraform: %w", err)
			}

			// Re-generate with deferred substitutions resolved now that terraform
			// outputs are available from the Up step above.
			var resolveErr error
			blueprint, resolveErr = proj.Composer.BlueprintHandler.GenerateResolved()
			if resolveErr != nil {
				return fmt.Errorf("error resolving blueprint substitutions: %w", resolveErr)
			}
			if blueprint == nil {
				return fmt.Errorf("resolved blueprint is not available")
			}

			if err := proj.Provisioner.BeginVersionTransition(blueprint); err != nil {
				return fmt.Errorf("error recording version transition: %w", err)
			}

			if err := proj.Provisioner.Install(cmd.Context(), blueprint); err != nil {
				return fmt.Errorf("error installing blueprint: %w", err)
			}

			if err := proj.Provisioner.Wait(cmd.Context(), blueprint); err != nil {
				return fmt.Errorf("error waiting for kustomizations: %w", err)
			}

			if err := proj.Provisioner.Prune(blueprint); err != nil {
				return fmt.Errorf("error pruning orphaned kustomizations: %w", err)
			}

			if err := proj.Provisioner.WriteVersionMarker(blueprint); err != nil {
				return fmt.Errorf("error recording applied version: %w", err)
			}

			return nil
		})
	},
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

// retargetSources applies each `name=url` spec to the context's declared sources, persists the bumps
// to blueprint.yaml via the same writer init uses, and prints what changed for the operator to
// commit. An unknown source name or malformed spec aborts before anything is written, so a failed
// retarget never leaves blueprint.yaml half-edited.
func retargetSources(cmd *cobra.Command, proj *project.Project, specs []string) error {
	type change struct{ name, previous, target string }
	changes := make([]change, 0, len(specs))
	for _, spec := range specs {
		name, url, ok := strings.Cut(spec, "=")
		if !ok || name == "" || url == "" {
			return fmt.Errorf("invalid --source %q; expected name=url", spec)
		}
		previous, err := proj.Composer.BlueprintHandler.RetargetSource(name, url)
		if err != nil {
			return err
		}
		changes = append(changes, change{name: name, previous: previous, target: url})
	}

	if err := proj.Composer.BlueprintHandler.Write(true); err != nil {
		return fmt.Errorf("failed to persist source changes to blueprint.yaml: %w", err)
	}

	for _, c := range changes {
		fmt.Fprintf(cmd.OutOrStdout(), "Retargeted %s from %s to %s\n", c.name, c.previous, c.target)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
	upgradeCmd.AddCommand(upgradeClusterCmd)
	upgradeCmd.AddCommand(upgradeNodeCmd)

	upgradeCmd.Flags().StringArrayVar(&upgradeSources, "source", nil, "Retarget a declared source to a new tagged URL (name=url); repeatable. Persisted to blueprint.yaml.")

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
