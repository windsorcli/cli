package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/provisioner"
	"github.com/windsorcli/cli/pkg/provisioner/stacklock"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

var (
	upgradeNodes          []string
	upgradeImage          string
	upgradeSources        []string
	upgradeYes            bool
	upgradeAllowDowngrade bool

	upgradeNodeAddr    string
	upgradeNodeImage   string
	upgradeNodeTimeout time.Duration
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Move sources to their latest version and reconcile the blueprint.",
	Long: `With no arguments, move every declared OCI source to its latest stable version, then reconcile: apply terraform and the Flux blueprint, wait, and prune kustomizations this context no longer declares. Use --source name=url to move named sources to specific versions instead. Prunes run only after a successful wait and are gated by --yes.

Use the 'cluster' or 'node' subcommand to upgrade Talos nodes instead.`,
	Example: `# Move all sources to their latest stable version and reconcile
windsor upgrade --yes

# Move a specific source to a specific version
windsor upgrade --source core=oci://ghcr.io/windsorcli/core:v0.6.0 --yes

# Upgrade Talos nodes in parallel (see 'upgrade cluster')
windsor upgrade cluster --nodes=10.0.0.5 --image=ghcr.io/siderolabs/installer:v1.13.0`,
	Annotations: map[string]string{
		"docs.seealso": "[`apply`](apply.md), [`bootstrap`](bootstrap.md), [`plan`](plan.md)",
		"docs.source":  "cmd/upgrade.go",
	},
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Configure the project but defer Initialize (compose) until after the confirmation and
		// downgrade gates, so neither a missing --yes nor a refused downgrade pulls or composes
		// any source. `windsor upgrade` runs the full blueprint — terraform components and Flux
		// kustomizations both — so the tool surface is not statically narrowable; mirror `apply`
		// and request AllRequirements(), letting the per-tool config gates decide.
		proj, err := configureProject(cmd)
		if err != nil {
			return err
		}

		if !upgradeYes {
			msg := "upgrade rewrites blueprint.yaml and reconciles the cluster (apply, wait, prune); re-run with --yes to proceed, or use `windsor plan` to preview"
			fmt.Fprintln(cmd.ErrOrStderr(), msg)
			silenceErrorsOnAncestors(cmd)
			return fmt.Errorf("%s", msg)
		}

		if len(upgradeSources) > 0 {
			if err := checkSourceDowngrades(cmd, proj, upgradeSources, upgradeAllowDowngrade); err != nil {
				return err
			}
		}

		proj.SetToolRequirements(tools.AllRequirements())
		if err := proj.Initialize(false); err != nil {
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
		} else {
			if err := upgradeToLatest(cmd, proj); err != nil {
				return err
			}
		}
		blueprint = proj.Composer.BlueprintHandler.Generate()
		if blueprint == nil {
			return fmt.Errorf("blueprint is not available")
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

			prunable, err := proj.Provisioner.PrunableKustomizations(blueprint)
			if err != nil {
				return fmt.Errorf("error listing kustomizations to prune: %w", err)
			}
			if err := confirmAndPrune(cmd, proj, blueprint, prunable, upgradeYes); err != nil {
				return err
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

// confirmAndPrune prunes the kustomizations the blueprint no longer declares, after printing them
// and requiring --yes. prunable is the already-computed prune set (empty → no-op). The caller must
// have waited for the desired set to be Ready first, so any migrated resources are adopted before a
// deletion. Shared by apply and upgrade, which reconcile identically.
func confirmAndPrune(cmd *cobra.Command, proj *project.Project, blueprint *blueprintv1alpha1.Blueprint, prunable []string, yes bool) error {
	if len(prunable) == 0 {
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "The following kustomizations are no longer declared and will be pruned:\n  %s\n", strings.Join(prunable, "\n  "))
	if !yes {
		silenceErrorsOnAncestors(cmd)
		return fmt.Errorf("this would prune %d kustomization(s); re-run with --yes to proceed", len(prunable))
	}
	if err := proj.Provisioner.Prune(blueprint); err != nil {
		return fmt.Errorf("error pruning orphaned kustomizations: %w", err)
	}
	return nil
}

// upgradeToLatest moves every remote OCI source pinned to a semver to its latest stable tag,
// persists the bumps to blueprint.yaml, and prints what changed. Sources that are not OCI, not
// semver-pinned, or already current are left untouched; it reports when nothing moved.
func upgradeToLatest(cmd *cobra.Command, proj *project.Project) error {
	upgrades, err := proj.Composer.BlueprintHandler.UpgradeSourcesToLatest()
	if err != nil {
		return fmt.Errorf("error resolving latest source versions: %w", err)
	}
	if len(upgrades) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "All sources are already at their latest version.")
		return nil
	}
	if err := proj.Composer.BlueprintHandler.Write(true); err != nil {
		return fmt.Errorf("failed to persist source upgrades to blueprint.yaml: %w", err)
	}
	for _, u := range upgrades {
		fmt.Fprintf(cmd.OutOrStdout(), "Upgraded %s from %s to %s\n", u.Name, u.From, u.To)
	}
	return nil
}

// parseSourceSpec splits a `--source` value into its name and URL, rejecting any spec missing the
// separator or either half. Both the downgrade gate and the retarget writer parse the same specs,
// so they share this one parser to stay in lockstep on what a valid spec is.
func parseSourceSpec(spec string) (name, url string, err error) {
	name, url, ok := strings.Cut(spec, "=")
	if !ok || name == "" || url == "" {
		return "", "", fmt.Errorf("invalid --source %q; expected name=url", spec)
	}
	return name, url, nil
}

// retargetSources applies each `name=url` spec to the context's declared sources, persists the bumps
// to blueprint.yaml via the same writer init uses, and prints what changed for the operator to
// commit. An unknown source name or malformed spec aborts before anything is written, so a failed
// retarget never leaves blueprint.yaml half-edited. Downgrades are refused earlier by
// checkSourceDowngrades, before any source is composed.
func retargetSources(cmd *cobra.Command, proj *project.Project, specs []string) error {
	type change struct{ name, previous, target string }
	changes := make([]change, 0, len(specs))
	for _, spec := range specs {
		name, url, err := parseSourceSpec(spec)
		if err != nil {
			return err
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

// checkSourceDowngrades evaluates each `name=url` spec against the context's declared sources before
// anything is composed or pulled, so a refused downgrade never triggers a registry round-trip. A
// spec that moves a declared source to an older semver of the same repository is a downgrade: it is
// refused unless allowDowngrade is set, since Windsor reverts infrastructure declaratively but does
// not reverse application data. Specs naming an undeclared source are left for retargetSources to
// reject; malformed specs are rejected here.
func checkSourceDowngrades(cmd *cobra.Command, proj *project.Project, specs []string, allowDowngrade bool) error {
	type change struct{ name, previous, target string }
	targets := make([]change, 0, len(specs))
	for _, spec := range specs {
		name, url, err := parseSourceSpec(spec)
		if err != nil {
			return err
		}
		targets = append(targets, change{name: name, target: url})
	}

	declared, err := proj.Composer.BlueprintHandler.GetDeclaredSources()
	if err != nil {
		return fmt.Errorf("failed to read declared sources: %w", err)
	}
	declaredURL := make(map[string]string, len(declared))
	for _, s := range declared {
		declaredURL[s.Name] = s.Url
	}

	var downgrades []change
	for _, t := range targets {
		if prev, found := declaredURL[t.name]; found && blueprint.IsDowngrade(prev, t.target) {
			downgrades = append(downgrades, change{name: t.name, previous: prev, target: t.target})
		}
	}
	if len(downgrades) == 0 {
		return nil
	}
	for _, d := range downgrades {
		fmt.Fprintf(cmd.ErrOrStderr(), "Downgrading %s from %s to %s\n", d.name, d.previous, d.target)
	}
	if !allowDowngrade {
		msg := fmt.Sprintf("refusing to downgrade %d source(s); recovery from a bad version is fix-forward (cut a higher version that reverts the change). To revert infrastructure anyway, re-run with --allow-downgrade — application data is NOT reversed", len(downgrades))
		fmt.Fprintln(cmd.ErrOrStderr(), msg)
		silenceErrorsOnAncestors(cmd)
		return fmt.Errorf("%s", msg)
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "Warning: downgrading reverts infrastructure declaratively but does NOT reverse application data; ensure you have backups.")
	return nil
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
	upgradeCmd.AddCommand(upgradeClusterCmd)
	upgradeCmd.AddCommand(upgradeNodeCmd)

	upgradeCmd.Flags().StringArrayVar(&upgradeSources, "source", nil, "Retarget a declared source to a new tagged URL (name=url); repeatable. Persisted to blueprint.yaml.")
	upgradeCmd.Flags().BoolVar(&upgradeYes, "yes", false, "Proceed without confirmation when the upgrade would prune kustomizations.")
	upgradeCmd.Flags().BoolVar(&upgradeAllowDowngrade, "allow-downgrade", false, "Permit moving a source to an older version. Reverts infrastructure declaratively; does NOT reverse application data.")

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
