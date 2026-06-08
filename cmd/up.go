package cmd

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/provisioner/stacklock"
	"github.com/windsorcli/cli/pkg/runtime/tools"
	"github.com/windsorcli/cli/pkg/workstation"
)

var (
	waitFlag    bool
	upVmDriver  string
	upPlatform  string
	upBlueprint string
	upSetFlags  []string
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Bring up the local workstation environment.",
	Long: `Start the workstation VM, run Terraform components, then install the Flux blueprint. Workstation contexts only — for non-workstation contexts, use 'windsor apply'. If the current context has no workstation, up exits with a hint and does no work.

Returns once the install request has been issued. Pass --wait to block until kustomizations report ready.

If any host-side network or DNS configuration was deferred (because it requires sudo / elevation), up prints a follow-up command at the end so the operator knows what to run next.`,
	Example: `# Bring up the workstation and wait for everything to be ready
windsor up --wait

# Override an inline config value at startup
windsor up --set cluster.workers.count=3

# Initialize and bring up with a specific blueprint
windsor up --blueprint=ghcr.io/myorg/blueprint:v1.0.0`,
	Annotations: map[string]string{
		"docs.seealso": "[`down`](down.md), [`apply`](apply.md), [`destroy`](destroy.md)",
		"docs.source": "cmd/up.go",
	},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var opts []*project.Project
		if overridesVal := cmd.Context().Value(projectOverridesKey); overridesVal != nil {
			opts = []*project.Project{overridesVal.(*project.Project)}
		}

		proj := project.NewProject("", opts...)

		proj.Runtime.Shell.SetVerbosity(verbose)

		if err := proj.Runtime.Shell.CheckTrustedDirectory(); err != nil {
			return fmt.Errorf("not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve")
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
			return fmt.Errorf("invalid configuration: %w", err)
		}

		blueprintURL, err := resolveBlueprintURL(upBlueprint, upPlatform, proj.Runtime.ContextName, proj.Runtime.TemplateRoot, false)
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
				return fmt.Errorf("failed to save configuration: %w", err)
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
		if err := stacklock.With(cmd.Context(), proj.Runtime, "up", func() error {
			_, h, err := proj.Up()
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
				return fmt.Errorf("error resolving blueprint substitutions: %w", err)
			}

			if err := proj.Provisioner.Install(cmd.Context(), blueprint); err != nil {
				return fmt.Errorf("error installing blueprint: %w", err)
			}

			if waitFlag {
				if err := proj.Provisioner.Wait(cmd.Context(), blueprint); err != nil {
					return fmt.Errorf("error waiting for kustomizations: %w", err)
				}
			}
			return nil
		}); err != nil {
			return err
		}

		if !halted {
			fmt.Fprintln(os.Stderr, "Windsor environment set up successfully.")
		}
		printDeferredWork(os.Stderr, proj.Workstation.DeferredWork(), runtime.GOOS)
		return nil
	},
}

// printDeferredWork renders the end-of-run summary for items the apply skipped because they
// require elevation Up() will not request. Required items render as halt sentences ("then
// re-run 'windsor up'"); optional items render as outcome sentences below. Each required item
// folds in the first not-yet-folded optional item that shares its command ("Run 'X' to <outcome>,
// then re-run 'windsor up'.") so the same command is never printed twice for the same outcome.
// Any optional item not folded into a required line — including a second outcome sharing a folded
// command — still renders on its own line, so no outcome is silently dropped. Empty items produce
// no output. goos selects the OS-specific elevation parenthetical: "(Administrator PowerShell)" on
// windows, "(asks for sudo)" elsewhere.
func printDeferredWork(w io.Writer, items []workstation.DeferredWorkItem, goos string) {
	if len(items) == 0 {
		return
	}
	paren := "(asks for sudo)"
	if goos == "windows" {
		paren = "(Administrator PowerShell)"
	}
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

// buildUpFlagOverrides builds a config override map from up's command-line
// flags. The workstation-related mapping is shared with `windsor init` via
// applyWorkstationFlagOverrides; --set is parsed strictly (returning an error
// on malformed entries) to give users clear feedback on typos.
func buildUpFlagOverrides() (map[string]any, error) {
	overrides := make(map[string]any)
	applyWorkstationFlagOverrides(overrides, upVmDriver, upPlatform)
	for _, setFlag := range upSetFlags {
		parts := strings.SplitN(setFlag, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --set format, expected key=value: %s", setFlag)
		}
		overrides[parts[0]] = parts[1]
	}
	return overrides, nil
}

func init() {
	upCmd.Flags().BoolVar(&waitFlag, "wait", false, "Wait for kustomization resources to be ready.")
	upCmd.Flags().StringVar(&upVmDriver, "vm-driver", "", "VM driver: colima, colima-incus, docker-desktop, docker.")
	upCmd.Flags().StringVar(&upPlatform, "platform", "", "Target platform: none, metal, docker, aws, azure, gcp, hyperv.")
	upCmd.Flags().StringVar(&upBlueprint, "blueprint", "", "Blueprint OCI reference or local path.")
	upCmd.Flags().StringSliceVar(&upSetFlags, "set", []string{}, "Override config values, e.g. --set dns.enabled=false. May be repeated.")
	rootCmd.AddCommand(upCmd)
}
