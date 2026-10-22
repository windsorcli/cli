package cmd

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/provisioner/stacklock"
	"github.com/windsorcli/cli/pkg/runtime/tools"
	"github.com/windsorcli/cli/pkg/workstation"
)

var (
	upVmDriver  string
	upPlatform  string
	upBlueprint string
	upSetFlags  []string
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Bring up the local workstation environment.",
	Long: `Start the workstation VM, run Terraform components, then install the Flux blueprint. Workstation contexts only — for non-workstation contexts, use 'windsor apply'. If the current context has no workstation, up exits with a hint and does no work.

Waits until every kustomization is Ready, and fails when one does not become Ready.

If any host-side network or DNS configuration was deferred (because it requires sudo / elevation), up prints a follow-up command as the last section of its output.`,
	Example: `# Bring up the workstation
windsor up

# Override an inline config value at startup
windsor up --set cluster.workers.count=3

# Initialize and bring up with a specific blueprint
windsor up --blueprint=ghcr.io/myorg/blueprint:v1.0.0`,
	Annotations: map[string]string{
		"docs.seealso": "[`down`](down.md), [`apply`](apply.md), [`destroy`](destroy.md)",
		"docs.source":  "cmd/up.go",
	},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		started := time.Now()
		var opts []*project.Project
		if overridesVal := cmd.Context().Value(projectOverridesKey); overridesVal != nil {
			opts = []*project.Project{overridesVal.(*project.Project)}
		}

		proj := project.NewProject("", opts...)

		proj.Runtime.Shell.SetVerbosity(verbose)

		if err := proj.Runtime.Shell.CheckTrustedDirectory(); err != nil {
			return werror.New("not in a trusted directory", trustDirectoryHint, err)
		}

		// Build flag overrides using init's rules so that `windsor up` and
		// `windsor init` share identical bootstrap semantics. Runtime.ResolveConfig
		// applies OS-appropriate workstation.runtime defaults for dev contexts when
		// no flag is given, so we don't re-implement that here.
		flagOverrides, err := buildUpFlagOverrides()
		if err != nil {
			return err
		}

		if err := proj.Configure(flagOverrides); err != nil {
			return err
		}

		if err := proj.Runtime.ConfigHandler.ValidateContextValues(); err != nil {
			return werror.Wrap(err, "invalid configuration")
		}

		blueprintURL, err := resolveBlueprintURL(upBlueprint, upPlatform, proj.Runtime.ContextName, proj.Runtime.TemplateRoot, false, proj.Composer.ArtifactBuilder)
		if err != nil {
			return err
		}

		// `windsor up` brings up the workstation: it starts the container runtime, applies
		// terraform-driven workstation infrastructure, dereferences any 1Password / SOPS-backed
		// values, and (for azure contexts) authenticates to AKS. Request the full set so any
		// of those tools are validated up front.
		proj.SetToolRequirements(tools.AllRequirements())
		if err := proj.Initialize(false, blueprintURL...); err != nil {
			return err
		}

		// Initialize already persisted config with overwrite=false; re-save with
		// overwrite=true only when --set was provided so user values land in
		// values.yaml. Runs before the workstation guard so non-workstation
		// contexts can still receive --set overrides.
		if len(upSetFlags) > 0 {
			if err := proj.Runtime.SaveConfig(true); err != nil {
				return werror.Wrap(err, "failed to save configuration")
			}
		}

		if proj.Workstation == nil {
			fmt.Fprintln(os.Stderr, "windsor up is only applicable when a workstation is enabled; use windsor apply to apply infrastructure")
			return nil
		}

		if err := requireCloudAuth(cmd, proj); err != nil {
			return err
		}

		var halted bool
		var postRunMessages []blueprintv1alpha1.Message
		if err := stacklock.With(cmd.Context(), proj.Runtime, "up", lockTimeout, func() error {
			_, h, err := proj.Up(cmd.Context())
			if err != nil {
				return err
			}
			halted = h
			if halted {
				return nil
			}

			// Re-generate with deferred substitutions resolved now that terraform
			// outputs are available from the Up step above. A failure here surfaces
			// an unresolved expression by name (e.g. "kustomize.dns.substitutions.
			// external_dns_tenant_id: terraform output 'tenant_id' for component
			// cluster not found"), preventing the raw `${...}` source text from
			// reaching Flux ConfigMaps and downstream Helm renders.
			blueprint, err := proj.Composer.BlueprintHandler.GenerateResolved()
			if err != nil {
				return werror.Wrap(err, "error resolving blueprint substitutions")
			}

			// Capture post-run messages to print as an end-of-run summary, mirroring bootstrap.
			postRunMessages = blueprint.Messages

			if err := warnNotReady(os.Stderr, proj.Provisioner.Install(cmd.Context(), blueprint, false), postRunMessages); err != nil {
				return werror.Wrap(err, "error installing blueprint")
			}
			return nil
		}); err != nil {
			return err
		}

		if !halted {
			fmt.Fprintf(os.Stderr, "Windsor environment set up successfully in %s.\n", presenter.FormatDuration(time.Since(started)))
		}
		printPostRunMessages(os.Stderr, postRunMessages)
		printDeferredWork(os.Stderr, proj.Workstation.DeferredWork(), runtime.GOOS)
		return nil
	},
}

// printDeferredWork prints the items Up skipped because they need elevation, as the last section
// of the run. The header is "Action required" when any item is required, and "Next steps" otherwise.
// A required item renders as a halt sentence ending "then re-run 'windsor up'" and folds in the
// first unfolded optional item with the same command. Every other optional item renders as its
// own outcome sentence. goos selects "(Administrator PowerShell)" on windows and "(asks for sudo)"
// elsewhere. Empty items print nothing.
func printDeferredWork(w io.Writer, items []workstation.DeferredWorkItem, goos string) {
	if len(items) == 0 {
		return
	}
	paren := "(asks for sudo)"
	if goos == "windows" {
		paren = "(Administrator PowerShell)"
	}
	title := "Next steps"
	for _, item := range items {
		if item.Required {
			title = "Action required"
			break
		}
	}
	printSectionHeader(w, title)
	folded := make(map[int]bool)
	for _, item := range items {
		if !item.Required {
			continue
		}
		outcome := ""
		for j, other := range items {
			if !other.Required && !folded[j] && other.Command == item.Command && other.Outcome != "" {
				outcome = other.Outcome
				folded[j] = true
				break
			}
		}
		if outcome != "" {
			fmt.Fprintf(w, "Run '%s' %s to %s, then re-run 'windsor up'.\n", item.Command, paren, outcome)
		} else {
			fmt.Fprintf(w, "Run '%s' %s, then re-run 'windsor up'.\n", item.Command, paren)
		}
	}
	for j, item := range items {
		if !item.Required && !folded[j] {
			fmt.Fprintf(w, "Run '%s' %s to %s.\n", item.Command, paren, item.Outcome)
		}
	}
}

// buildUpFlagOverrides builds the config override map from the flags of up.
// It shares the workstation flag mapping with windsor init through applyWorkstationFlagOverrides.
// It returns an error for a malformed --set entry.
func buildUpFlagOverrides() (map[string]any, error) {
	overrides := make(map[string]any)
	applyWorkstationFlagOverrides(overrides, upVmDriver, upPlatform)
	for _, setFlag := range upSetFlags {
		parts := strings.SplitN(setFlag, "=", 2)
		if len(parts) != 2 {
			return nil, werror.New(fmt.Sprintf("invalid --set format: %s", setFlag), setFormatHint, nil)
		}
		overrides[parts[0]] = parts[1]
	}
	return overrides, nil
}

func init() {
	addDeprecatedWaitFlag(upCmd)
	upCmd.Flags().StringVar(&upVmDriver, "vm-driver", "", "VM driver: colima, colima-incus, docker-desktop, docker.")
	upCmd.Flags().StringVar(&upPlatform, "platform", "", "Target platform: none, docker, incus, metal, hetzner, aws, azure, gcp, hyperv, vsphere, proxmox.")
	upCmd.Flags().StringVar(&upBlueprint, "blueprint", "", "Blueprint OCI reference or local path.")
	upCmd.Flags().StringSliceVar(&upSetFlags, "set", []string{}, "Override config values, e.g. --set dns.enabled=false. May be repeated.")
	rootCmd.AddCommand(upCmd)
}
