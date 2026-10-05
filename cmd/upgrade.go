package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	"golang.org/x/term"
)

var (
	upgradeNodes          []string
	upgradeImage          string
	upgradeSources        []string
	upgradeYes            bool
	upgradeAllowDowngrade bool
	upgradeRebootMode     string

	upgradeNodeAddr           string
	upgradeNodeImage          string
	upgradeNodeTimeout        time.Duration
	upgradeNodeOfflineTimeout time.Duration
	upgradeNodeRebootMode     string
)

// sourceChanges lists the sources an upgrade moves, each with its previous and new URL.
type sourceChanges = []blueprint.SourceUpgrade

// upgradeStdinIsTerminal reports whether upgrade can ask for confirmation. Tests replace it.
var upgradeStdinIsTerminal = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) // #nosec G115 -- file descriptors are small, safe to cast to int
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Move sources to their latest version and reconcile the blueprint.",
	Long: `With no arguments, move every declared OCI source to its latest stable version, then reconcile: apply terraform and the Flux blueprint, wait, and prune kustomizations this context no longer declares. Use --source name=url to move named sources to specific versions instead. In a terminal, upgrade first prints the source moves and the kustomizations it would prune, then asks to proceed; --yes skips the prompt. Without a terminal, --yes is required.

Use the 'cluster' or 'node' subcommand to upgrade Talos nodes instead.`,
	Example: `# Review the plan, confirm at the prompt, then move all sources to their latest stable version and reconcile
windsor upgrade

# Same, without the prompt (required in CI)
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
		// Configure the project but defer Initialize (compose) until after the --yes and
		// downgrade gates, so neither a missing --yes nor a refused downgrade pulls or composes
		// any source. The interactive prompt follows composition, because its plan needs the
		// recomposed blueprint. `windsor upgrade` runs the full blueprint — terraform components and Flux
		// kustomizations both — so the tool surface is not statically narrowable; mirror `apply`
		// and request AllRequirements(), letting the per-tool config gates decide.
		proj, err := configureProject(cmd)
		if err != nil {
			return err
		}

		interactive := !upgradeYes && upgradeStdinIsTerminal()
		if !upgradeYes && !interactive {
			msg := "upgrade rewrites blueprint.yaml and reconciles the cluster (apply, wait, prune); run it in a terminal to confirm, re-run with --yes to proceed, or use `windsor plan` to preview"
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

		blueprintPath := filepath.Join(proj.Runtime.ConfigRoot, "blueprint.yaml")
		var snapshot []byte
		if interactive {
			snapshot, err = snapshotFile(blueprintPath)
			if err != nil {
				return err
			}
		}
		abort := func(cause error) error {
			if !interactive {
				return cause
			}
			if restoreErr := restoreFile(blueprintPath, snapshot); restoreErr != nil {
				return fmt.Errorf("%w; also failed to restore blueprint.yaml: %v", cause, restoreErr)
			}
			return cause
		}

		var changes sourceChanges
		if len(upgradeSources) > 0 {
			changes, err = retargetSources(proj, upgradeSources)
		} else {
			changes, err = upgradeToLatest(proj)
		}
		if err != nil {
			return abort(err)
		}
		if err := recomposeBlueprint(proj); err != nil {
			return abort(err)
		}
		blueprint = proj.Composer.BlueprintHandler.Generate()
		if blueprint == nil {
			return abort(fmt.Errorf("blueprint is not available"))
		}

		var confirmedPrune map[string]bool
		if interactive {
			prunable, pruneErr := proj.Provisioner.PrunableKustomizations(blueprint)
			if pruneErr == nil {
				confirmedPrune = make(map[string]bool, len(prunable))
				for _, name := range prunable {
					confirmedPrune[name] = true
				}
			}
			if !confirmUpgrade(cmd.InOrStdin(), cmd.ErrOrStderr(), proj.Runtime.ContextName, changes, prunable, pruneErr) {
				silenceErrorsOnAncestors(cmd)
				return abort(fmt.Errorf("upgrade cancelled; blueprint.yaml is unchanged"))
			}
		} else {
			printSourceChanges(cmd.OutOrStdout(), changes, len(upgradeSources) > 0)
		}

		return stacklock.With(cmd.Context(), proj.Runtime, "upgrade", lockTimeout, func() error {
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

			if err := proj.Provisioner.Install(cmd.Context(), blueprint, true); err != nil {
				return fmt.Errorf("error installing blueprint: %w", err)
			}

			if err := proj.Provisioner.Wait(cmd.Context(), blueprint); err != nil {
				return fmt.Errorf("error waiting for kustomizations: %w", err)
			}

			prunable, err := proj.Provisioner.PrunableKustomizations(blueprint)
			if err != nil {
				return fmt.Errorf("error listing kustomizations to prune: %w", err)
			}
			if extras := unconfirmedPrunes(prunable, confirmedPrune); len(extras) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "Skipping the prune: these kustomizations are no longer declared but were not in the plan you confirmed:\n  %s\nRun `windsor apply --prune` to remove them.\n", strings.Join(extras, "\n  "))
				prunable = nil
			}
			if err := pruneOrphaned(cmd, proj, blueprint, prunable); err != nil {
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
		powercycle, err := parseRebootMode(upgradeRebootMode)
		if err != nil {
			return err
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

		if err := prov.UpgradeNodes(cmd.Context(), upgradeNodes, upgradeImage, powercycle); err != nil {
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

# Same with a longer overall timeout for slow rebooters
windsor upgrade node --node=10.0.0.5 --image=ghcr.io/siderolabs/installer:v1.13.0 --timeout=20m

# Nested-virtualized platforms can take longer than 3m to confirm the reboot
windsor upgrade node --node=10.0.0.5 --image=ghcr.io/siderolabs/installer:v1.13.0 --offline-timeout=8m

# Some platforms don't reliably register kexec as an offline transition; powercycle is slower but more reliable
windsor upgrade node --node=10.0.0.5 --image=ghcr.io/siderolabs/installer:v1.13.0 --reboot-mode=powercycle`,
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
		if !cmd.Flags().Changed("offline-timeout") {
			upgradeNodeOfflineTimeout = constants.DefaultNodeOfflineTimeout
		}
		if upgradeNodeTimeout > 0 && upgradeNodeOfflineTimeout > upgradeNodeTimeout {
			return fmt.Errorf("--offline-timeout (%s) cannot exceed --timeout (%s)", upgradeNodeOfflineTimeout, upgradeNodeTimeout)
		}

		powercycle, err := parseRebootMode(upgradeNodeRebootMode)
		if err != nil {
			return err
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

		if err := prov.UpgradeNode(ctx, upgradeNodeAddr, upgradeNodeImage, upgradeNodeOfflineTimeout, powercycle, outputFunc); err != nil {
			return fmt.Errorf("node upgrade failed: %w", err)
		}

		return nil
	},
}

// parseRebootMode validates a --reboot-mode value and reports whether it requests powercycle.
// "default" (kexec, fast) and "powercycle" (full ACPI reset) are the only accepted values.
func parseRebootMode(mode string) (bool, error) {
	switch mode {
	case "", "default":
		return false, nil
	case "powercycle":
		return true, nil
	default:
		return false, fmt.Errorf("invalid --reboot-mode %q: must be \"default\" or \"powercycle\"", mode)
	}
}

// pruneOrphaned deletes the kustomizations the blueprint no longer declares, after printing them.
// prunable is the already-computed prune set (empty → no-op). The caller must have waited for the
// desired set to be Ready first, so any migrated resources are adopted before a deletion. Shared by
// apply (behind --prune) and upgrade (unconditional, since upgrade is confirmed or passed --yes before it starts).
func pruneOrphaned(cmd *cobra.Command, proj *project.Project, blueprint *blueprintv1alpha1.Blueprint, prunable []string) error {
	if len(prunable) == 0 {
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Pruning kustomizations no longer declared:\n  %s\n", strings.Join(prunable, "\n  "))
	if err := proj.Provisioner.Prune(blueprint); err != nil {
		return fmt.Errorf("error pruning orphaned kustomizations: %w", err)
	}
	return nil
}

// upgradeToLatest moves every remote OCI source pinned to a semver to its latest stable tag and
// persists the bumps to blueprint.yaml. It returns what moved. Sources that are not OCI, not
// semver-pinned, or already current are left untouched, and nothing is written when none moved.
func upgradeToLatest(proj *project.Project) (sourceChanges, error) {
	upgrades, err := proj.Composer.BlueprintHandler.UpgradeSourcesToLatest()
	if err != nil {
		return nil, fmt.Errorf("error resolving latest source versions: %w", err)
	}
	if len(upgrades) == 0 {
		return nil, nil
	}
	if err := proj.Composer.BlueprintHandler.Write(true); err != nil {
		return nil, fmt.Errorf("failed to persist source upgrades to blueprint.yaml: %w", err)
	}
	return upgrades, nil
}

// recomposeBlueprint reloads the blueprint from disk and recomposes it against the sources'
// current content. RetargetSource and UpgradeSourcesToLatest only change source URLs in memory.
// Write persists those URLs to blueprint.yaml but does not recompose. Without this call, Up,
// Install, and Wait would build manifests from the blueprint composed before the source change.
func recomposeBlueprint(proj *project.Project) error {
	if err := proj.Composer.BlueprintHandler.LoadBlueprint(); err != nil {
		return fmt.Errorf("error recomposing blueprint against updated sources: %w", err)
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
// to blueprint.yaml via the same writer init uses, and returns what changed. An unknown source name
// or malformed spec aborts before anything is written, so a failed retarget never leaves
// blueprint.yaml half-edited. Downgrades are refused earlier by checkSourceDowngrades.
func retargetSources(proj *project.Project, specs []string) (sourceChanges, error) {
	changes := make(sourceChanges, 0, len(specs))
	for _, spec := range specs {
		name, url, err := parseSourceSpec(spec)
		if err != nil {
			return nil, err
		}
		previous, err := proj.Composer.BlueprintHandler.RetargetSource(name, url)
		if err != nil {
			return nil, err
		}
		changes = append(changes, blueprint.SourceUpgrade{Name: name, From: previous, To: url})
	}

	if err := proj.Composer.BlueprintHandler.Write(true); err != nil {
		return nil, fmt.Errorf("failed to persist source changes to blueprint.yaml: %w", err)
	}
	return changes, nil
}

// printSourceChanges reports the sources an upgrade moved, or that every source is already current.
func printSourceChanges(w io.Writer, changes sourceChanges, retargeted bool) {
	if len(changes) == 0 {
		fmt.Fprintln(w, "All sources are already at their latest version.")
		return
	}
	verb := "Upgraded"
	if retargeted {
		verb = "Retargeted"
	}
	for _, c := range changes {
		fmt.Fprintf(w, "%s %s from %s to %s\n", verb, c.Name, c.From, c.To)
	}
}

// confirmUpgrade prints the upgrade plan for the context and reads one line from r. Only "y" or
// "yes" (any case) confirms; anything else, including closed input, declines. A failed prune
// listing is reported in the plan instead of aborting, so the operator still decides.
func confirmUpgrade(r io.Reader, w io.Writer, contextName string, changes sourceChanges, prunable []string, pruneErr error) bool {
	fmt.Fprintf(w, "Upgrade plan for context %s:\n", contextName)
	if len(changes) == 0 {
		fmt.Fprintln(w, "  Sources: no changes")
	} else {
		fmt.Fprintln(w, "  Sources:")
		for _, c := range changes {
			fmt.Fprintf(w, "    %s: %s -> %s\n", c.Name, c.From, c.To)
		}
	}
	switch {
	case pruneErr != nil:
		fmt.Fprintf(w, "  Kustomizations to prune: unknown (%v)\n", pruneErr)
	case len(prunable) == 0:
		fmt.Fprintln(w, "  Kustomizations to prune: none")
	default:
		fmt.Fprintf(w, "  Kustomizations to prune:\n    %s\n", strings.Join(prunable, "\n    "))
	}
	fmt.Fprint(w, "Proceed with the upgrade? [y/N]: ")
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes"
}

// unconfirmedPrunes returns the names in prunable that the operator did not see in the confirmed
// plan. A nil confirmed map means no plan was shown (--yes, or the listing failed), so it returns nil.
func unconfirmedPrunes(prunable []string, confirmed map[string]bool) []string {
	if confirmed == nil {
		return nil
	}
	var extras []string
	for _, name := range prunable {
		if !confirmed[name] {
			extras = append(extras, name)
		}
	}
	return extras
}

// snapshotFile returns the bytes of path, or nil when the file does not exist.
func snapshotFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is the context's own blueprint.yaml
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	return data, nil
}

// restoreFile puts snapshot back at path, or removes the file when the snapshot is nil.
func restoreFile(path string, snapshot []byte) error {
	if snapshot == nil {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove %s: %w", path, err)
		}
		return nil
	}
	if err := os.WriteFile(path, snapshot, 0644); err != nil { // #nosec G306 -- blueprint.yaml is a project file, readable like the writer's own output
		return fmt.Errorf("failed to restore %s: %w", path, err)
	}
	return nil
}

// checkSourceDowngrades evaluates each `name=url` spec against the context's declared sources before
// anything is composed or pulled, so a refused downgrade never triggers a registry round-trip. A
// spec that moves a declared source to an older semver of the same repository is a downgrade: it is
// refused unless allowDowngrade is set, since Windsor reverts infrastructure declaratively but does
// not reverse application data. Specs naming an undeclared source are left for retargetSources to
// reject; malformed specs are rejected here.
//
// The comparison baseline is the ref declared in blueprint.yaml, not the running version recorded in
// the cluster marker — a deliberate trade-off that keeps this a cheap, offline pre-flight (the marker
// would require a cluster read). The normal flow keeps declared and applied in lockstep; the gap is a
// blueprint.yaml hand-edited below the running version, where a move that is forward relative to the
// declared ref but still behind what is applied would not be flagged.
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
	upgradeCmd.Flags().BoolVar(&upgradeYes, "yes", false, "Skip the confirmation prompt.")
	upgradeCmd.Flags().BoolVar(&upgradeAllowDowngrade, "allow-downgrade", false, "Permit moving a source to an older version. Reverts infrastructure declaratively; does NOT reverse application data.")

	upgradeClusterCmd.Flags().StringSliceVar(&upgradeNodes, "nodes", []string{}, "Node addresses to upgrade. Required.")
	upgradeClusterCmd.Flags().StringVar(&upgradeImage, "image", "", "Talos image to upgrade to. Required.")
	upgradeClusterCmd.Flags().StringVar(&upgradeRebootMode, "reboot-mode", "default", "Reboot mode: \"default\" (kexec, fast) or \"powercycle\" (full ACPI reset). Use powercycle on platforms where kexec doesn't reliably register as offline (e.g. nested virtualization).")
	_ = upgradeClusterCmd.MarkFlagRequired("nodes")
	_ = upgradeClusterCmd.MarkFlagRequired("image")

	upgradeNodeCmd.Flags().StringVar(&upgradeNodeAddr, "node", "", "Node IP address to upgrade. Required.")
	upgradeNodeCmd.Flags().StringVar(&upgradeNodeImage, "image", "", "Talos image to upgrade to. Required.")
	upgradeNodeCmd.Flags().DurationVar(&upgradeNodeTimeout, "timeout", 0, "Overall timeout for the whole upgrade, including the offline wait (see --offline-timeout). Default 10m.")
	upgradeNodeCmd.Flags().DurationVar(&upgradeNodeOfflineTimeout, "offline-timeout", 0, "Timeout for the node to confirm it rebooted (kernel boot ID changed) after the upgrade request. Raise this on slow-rebooting or nested-virtualized platforms. Default 3m.")
	upgradeNodeCmd.Flags().StringVar(&upgradeNodeRebootMode, "reboot-mode", "default", "Reboot mode: \"default\" (kexec, fast) or \"powercycle\" (full ACPI reset). Use powercycle on platforms where kexec doesn't reliably register as offline (e.g. nested virtualization).")
	_ = upgradeNodeCmd.MarkFlagRequired("node")
	_ = upgradeNodeCmd.MarkFlagRequired("image")
}
