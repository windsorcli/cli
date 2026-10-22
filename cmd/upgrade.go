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
	"github.com/windsorcli/cli/internal/werror"
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
			if len(changes) > 0 {
				if err := regenerateTerraform(proj); err != nil {
					return err
				}
			}

			if _, err := proj.Provisioner.Up(cmd.Context(), blueprint); err != nil {
				return werror.Wrap(err, "error applying terraform")
			}

			// Re-generate with deferred substitutions resolved now that terraform
			// outputs are available from the Up step above.
			var resolveErr error
			blueprint, resolveErr = proj.Composer.BlueprintHandler.GenerateResolved()
			if resolveErr != nil {
				return werror.Wrap(resolveErr, "error resolving blueprint substitutions")
			}
			if blueprint == nil {
				return fmt.Errorf("resolved blueprint is not available")
			}

			if err := proj.Provisioner.BeginVersionTransition(blueprint); err != nil {
				return werror.Wrap(err, "error recording version transition")
			}

			if err := proj.Provisioner.Install(cmd.Context(), blueprint, true); err != nil {
				return werror.Wrap(err, "error installing blueprint")
			}

			prunable, err := proj.Provisioner.PrunableKustomizations(blueprint)
			if err != nil {
				return werror.Wrap(err, "error listing kustomizations to prune")
			}
			if extras := unconfirmedPrunes(prunable, confirmedPrune); len(extras) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "Skipping the prune: these kustomizations are no longer declared but were not in the plan you confirmed:\n  %s\nRun `windsor apply --prune` to remove them.\n", strings.Join(extras, "\n  "))
				prunable = nil
			}
			if err := pruneOrphaned(cmd, proj, blueprint, prunable); err != nil {
				return err
			}

			if err := proj.Provisioner.WriteVersionMarker(blueprint); err != nil {
				return werror.Wrap(err, "error recording applied version")
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
			return werror.New("not in a trusted directory", trustDirectoryHint, err)
		}

		if err := rt.ConfigHandler.LoadConfig(); err != nil {
			return err
		}

		if !rt.ConfigHandler.IsLoaded() {
			return werror.New("nothing to upgrade", initProjectHint, nil)
		}

		comp := composer.NewComposer(rt)
		prov := provisioner.NewProvisioner(rt, comp.BlueprintHandler)

		if err := prov.UpgradeNodes(cmd.Context(), upgradeNodes, upgradeImage, powercycle); err != nil {
			return werror.Wrap(err, "node upgrade failed")
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
			return werror.New(fmt.Sprintf("--offline-timeout (%s) cannot exceed --timeout (%s)", upgradeNodeOfflineTimeout, upgradeNodeTimeout), "Set --offline-timeout to a value that is not more than --timeout.", nil)
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
			return werror.New("not in a trusted directory", trustDirectoryHint, err)
		}

		if err := rt.ConfigHandler.LoadConfig(); err != nil {
			return err
		}

		if !rt.ConfigHandler.IsLoaded() {
			return werror.New("nothing to upgrade", initProjectHint, nil)
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
			return werror.Wrap(err, "node upgrade failed")
		}

		return nil
	},
}

// parseRebootMode validates a --reboot-mode value and reports whether it selects powercycle.
// The accepted values are "default", which uses kexec, and "powercycle", which does a full ACPI reset.
// It returns a WindsorError with a hint for any other value.
func parseRebootMode(mode string) (bool, error) {
	switch mode {
	case "", "default":
		return false, nil
	case "powercycle":
		return true, nil
	default:
		return false, werror.New(fmt.Sprintf("invalid --reboot-mode %q", mode), "Set --reboot-mode to default or powercycle.", nil)
	}
}

// pruneOrphaned prints the kustomizations that the blueprint no longer declares, then deletes them.
// The prunable argument is the prune set that the caller computed. An empty set does nothing.
// The caller must wait until the desired set is Ready, so that migrated resources are adopted before a deletion.
// Apply calls it when --prune is set. Upgrade always calls it, because the operator confirms the upgrade or sets --yes.
func pruneOrphaned(cmd *cobra.Command, proj *project.Project, blueprint *blueprintv1alpha1.Blueprint, prunable []string) error {
	if len(prunable) == 0 {
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Pruning kustomizations no longer declared:\n  %s\n", strings.Join(prunable, "\n  "))
	if err := proj.Provisioner.Prune(cmd.Context(), blueprint); err != nil {
		return werror.Wrap(err, "error pruning orphaned kustomizations")
	}
	return nil
}

// upgradeToLatest moves each remote OCI source that is pinned to a semver tag to its latest stable tag.
// It writes the changes to blueprint.yaml and returns the sources that moved.
// It does not change a source that is not OCI, not pinned to a semver tag, or already current.
// It writes nothing when no source moved.
func upgradeToLatest(proj *project.Project) (sourceChanges, error) {
	upgrades, err := proj.Composer.BlueprintHandler.UpgradeSourcesToLatest()
	if err != nil {
		return nil, werror.Wrap(err, "error resolving latest source versions")
	}
	if len(upgrades) == 0 {
		return nil, nil
	}
	if err := proj.Composer.BlueprintHandler.Write(true); err != nil {
		return nil, werror.Wrap(err, "failed to persist source upgrades to blueprint.yaml")
	}
	return upgrades, nil
}

// recomposeBlueprint loads the blueprint from disk again and composes it against the current content of the sources.
// RetargetSource and UpgradeSourcesToLatest change only the source URLs in memory, and Write does not compose again.
// Without this call, Up, Install, and Wait build manifests from the blueprint that was composed before the source change.
func recomposeBlueprint(proj *project.Project) error {
	if err := proj.Composer.BlueprintHandler.LoadBlueprint(); err != nil {
		return werror.Wrap(err, "error recomposing blueprint against updated sources")
	}
	return nil
}

// regenerateTerraform writes the terraform module shims and tfvars again from the recomposed blueprint.
// Initialize writes them before the source change, so they still load the modules of the old version.
// Without this call, Up applies the old modules with the inputs of the new version.
func regenerateTerraform(proj *project.Project) error {
	if err := proj.Composer.Generate(); err != nil {
		return werror.Wrap(err, "error regenerating terraform modules for updated sources")
	}
	return nil
}

// parseSourceSpec splits a --source value into its name and its URL.
// It returns a WindsorError with a hint when the separator, the name, or the URL is missing.
// The downgrade check and the retarget writer both use this parser, so they accept the same specs.
func parseSourceSpec(spec string) (name, url string, err error) {
	name, url, ok := strings.Cut(spec, "=")
	if !ok || name == "" || url == "" {
		return "", "", werror.New(fmt.Sprintf("invalid --source %q", spec), "Set --source in the format name=url.", nil)
	}
	return name, url, nil
}

// retargetSources applies each name=url spec to the declared sources of the context and returns the changes.
// It writes the changes to blueprint.yaml with the writer that init uses.
// An unknown source name or a malformed spec stops the operation before the write, so blueprint.yaml never changes partly.
// checkSourceDowngrades refuses downgrades before this function runs.
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
		return nil, werror.Wrap(err, "failed to persist source changes to blueprint.yaml")
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
		return nil, werror.Wrap(err, "failed to read %s", path)
	}
	return data, nil
}

// restoreFile puts snapshot back at path, or removes the file when the snapshot is nil.
func restoreFile(path string, snapshot []byte) error {
	if snapshot == nil {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return werror.Wrap(err, "failed to remove %s", path)
		}
		return nil
	}
	if err := os.WriteFile(path, snapshot, 0644); err != nil { // #nosec G306 -- blueprint.yaml is a project file, readable like the writer's own output
		return werror.Wrap(err, "failed to restore %s", path)
	}
	return nil
}

// checkSourceDowngrades compares each name=url spec with the declared sources of the context.
// It runs before composition and pulls, so a refused downgrade makes no registry request.
// A spec that moves a declared source to an older semver of the same repository is a downgrade.
// It refuses a downgrade unless allowDowngrade is set, because Windsor does not reverse application data.
// It rejects a malformed spec. retargetSources rejects a spec that names an undeclared source.
//
// It compares with the ref in blueprint.yaml, not with the running version in the cluster marker, so it needs no cluster read.
// Thus it does not flag a move to a version below the running version when blueprint.yaml was edited by hand to a lower ref.
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
		return werror.Wrap(err, "failed to read declared sources")
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
