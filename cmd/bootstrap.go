package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/provisioner"
	"github.com/windsorcli/cli/pkg/provisioner/stacklock"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Bootstrap Command
// =============================================================================

var (
	bootstrapPlatform  string
	bootstrapBlueprint string
	bootstrapSetFlags  []string
	bootstrapYes       bool
)

// bootstrapCmd stands up a fresh Windsor environment end-to-end. It combines init-style
// project configuration with up-style infrastructure deployment and unconditionally waits
// for kustomizations before returning. Unlike `windsor up`, bootstrap continues through
// terraform + install + wait even when the context does not define a workstation, so it
// is suitable for both local workstation contexts and non-workstation contexts (staging,
// production). Unlike `windsor init`, bootstrap does not anchor the current directory as
// a project root — it is allowed to run in global mode, where directory trust is implicit.
//
// When the blueprint declares a backend tier via Blueprint.Backend, bootstrap pivots the
// tier through local state on every run (always-on, idempotent) so the chicken-and-egg of
// a backend living inside the infrastructure it provisions is resolved without any
// first-run / subsequent-run branching. Without an in-blueprint backend tier, bootstrap
// forwards to a single Up pass against the configured backend.
var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap [context]",
	Short: "Bootstrap a fresh environment end-to-end.",
	Long: `First-run setup for a context: applies Terraform, installs the Flux blueprint, migrates state to the configured remote backend, and waits for kustomizations.

When the blueprint declares a backend Terraform component, bootstrap runs a two-phase apply to resolve the chicken-and-egg case where the remote backend (S3 bucket, DynamoDB table, etc.) lives in infrastructure Terraform must create first:

    1. Override terraform.backend.type to 'local' in memory and apply only the backend component.
    2. Restore the configured backend type and migrate the backend component's state.
    3. Subsequent components init directly against the remote backend.

When no backend component exists, bootstrap falls through to a single apply against whatever backend is configured.

Re-running on an existing context prompts for confirmation. Pass --yes (or -y) to skip the prompt in CI.`,
	Example: `# Bootstrap a new staging context on AWS
windsor bootstrap staging --platform=aws --blueprint=ghcr.io/myorg/blueprint:v1.0.0

# Re-bootstrap an existing context, scripted (skip the prompt)
windsor bootstrap prod --yes`,
	Annotations: map[string]string{
		"docs.seealso": "[`init`](init.md), [`apply`](apply.md), [`destroy`](destroy.md)",
		"docs.source": "cmd/bootstrap.go",
	},
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var proj *project.Project
		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(projectOverridesKey); overridesVal != nil {
			proj = overridesVal.(*project.Project)
			if proj.Runtime != nil {
				rtOpts = []*runtime.Runtime{proj.Runtime}
			}
		} else if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			rtOpts = []*runtime.Runtime{overridesVal.(*runtime.Runtime)}
		}

		contextName := "local"
		changingContext := len(args) > 0
		tempRt := runtime.NewRuntime(rtOpts...)
		if _, err := tempRt.Shell.WriteResetToken(); err != nil {
			return fmt.Errorf("failed to write reset token: %w", err)
		}
		if changingContext {
			contextName = args[0]
			if err := tempRt.ConfigHandler.SetContext(contextName); err != nil {
				return fmt.Errorf("failed to set context: %w", err)
			}
		}

		rt := runtime.NewRuntime(rtOpts...)
		rt.Shell.SetVerbosity(verbose)

		// Wire the freshly constructed runtime into the project override if the caller did not
		// supply one, so that later stages (Configure, Initialize, Up) run against the same
		// runtime that AddCurrentDirToTrustedFile is about to use. Without this a projectOverride
		// without a Runtime would hit a nil field in Configure.
		if proj != nil && proj.Runtime == nil {
			proj.Runtime = rt
		}

		if err := rt.Shell.AddCurrentDirToTrustedFile(); err != nil {
			return fmt.Errorf("failed to add current directory to trusted file: %w", err)
		}

		if !changingContext {
			if currentContext := rt.ConfigHandler.GetContext(); currentContext != "" {
				contextName = currentContext
			}
		}

		flagOverrides := make(map[string]any)
		applyWorkstationFlagOverrides(flagOverrides, "", bootstrapPlatform)
		for _, setFlag := range bootstrapSetFlags {
			parts := strings.SplitN(setFlag, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid --set format, expected key=value: %s", setFlag)
			}
			flagOverrides[parts[0]] = parts[1]
		}

		if proj == nil {
			projectOpts := &project.Project{Runtime: rt}
			if composerOverrideVal := cmd.Context().Value(composerOverridesKey); composerOverrideVal != nil {
				projectOpts.Composer = composerOverrideVal.(*composer.Composer)
			}
			proj = project.NewProject(contextName, projectOpts)
		}

		if !bootstrapYes {
			// Use the canonical ConfigHandler.GetConfigRoot() rather than proj.Runtime.ConfigRoot
			// so the guard doesn't silently depend on NewProject having mutated rt as a side effect
			// — if that ordering ever changes or ConfigRoot is empty for any reason, the guard
			// would pass silently and lose its protection against accidental re-bootstrap.
			configRoot, err := proj.Runtime.ConfigHandler.GetConfigRoot()
			if err != nil {
				return fmt.Errorf("failed to resolve config root for bootstrap confirmation: %w", err)
			}
			if err := confirmBootstrapIfContextExists(cmd.InOrStdin(), configRoot, contextName); err != nil {
				return err
			}
		}

		if err := proj.Configure(flagOverrides); err != nil {
			return err
		}

		if err := proj.Runtime.ConfigHandler.ValidateContextValues(); err != nil {
			return fmt.Errorf("invalid configuration: %w", err)
		}

		blueprintURL, err := resolveBlueprintURL(bootstrapBlueprint, bootstrapPlatform, proj.Runtime.ContextName, proj.Runtime.TemplateRoot, true, proj.Composer.ArtifactBuilder)
		if err != nil {
			return err
		}

		// Bootstrap stands up everything end-to-end: terraform apply, MigrateState, install,
		// and wait. Every tool family may be exercised, so request the full set up front and
		// let CheckAuth (called below) validate cloud credentials separately.
		proj.SetToolRequirements(tools.AllRequirements())
		if err := proj.Initialize(false, blueprintURL...); err != nil {
			return err
		}

		// Cloud auth and SaveConfig are paired by the "auth gates state mutation" invariant:
		// nothing writes config until cloud credentials have been validated. In --yes mode
		// (no prompt to defer behind) both fire upfront — operator wanted automation, fail
		// fast on bad auth before any disk write. In interactive mode both fire inside the
		// wrapped confirm callback after the operator accepts — so declining is a true no-op
		// (no auth burden, no state mutation, no surprise side effects). The wrapped callback
		// stashes any error for surfacing after Bootstrap returns because confirmFn returns
		// bool.
		var confirmFn provisioner.BootstrapConfirmFn
		finishPlan := func(error) {}
		var deferredErr error
		saveSet := len(bootstrapSetFlags) > 0
		if !bootstrapYes {
			promptConfirmFn, fp := makeBootstrapConfirmFn(cmd.InOrStdin(), os.Stderr)
			finishPlan = fp
			confirmFn = func(summary *provisioner.BootstrapSummary) bool {
				if !promptConfirmFn(summary) {
					return false
				}
				if err := requireCloudAuth(cmd, proj); err != nil {
					deferredErr = err
					return false
				}
				if err := proj.Runtime.SaveConfig(saveSet); err != nil {
					deferredErr = fmt.Errorf("failed to save configuration: %w", err)
					return false
				}
				return true
			}
		} else {
			if err := requireCloudAuth(cmd, proj); err != nil {
				return err
			}
			if err := proj.Runtime.SaveConfig(saveSet); err != nil {
				return fmt.Errorf("failed to save configuration: %w", err)
			}
		}

		// The bootstrap confirm prompt fires from inside proj.Bootstrap, so the operator
		// briefly holds the stack lock while answering. Acceptable today since bootstrap
		// is rare; revisit if the prompt grows into a longer flow.
		var applied, halted bool
		if err := stacklock.With(cmd.Context(), proj.Runtime, "bootstrap", func() error {
			_, ok, h, err := proj.Bootstrap(confirmFn)
			finishPlan(err)
			if err != nil {
				return err
			}
			applied = ok
			halted = h
			if !applied || halted {
				return nil
			}

			blueprint, err := proj.Composer.BlueprintHandler.GenerateResolved()
			if err != nil {
				return fmt.Errorf("error resolving blueprint substitutions: %w", err)
			}

			if err := proj.Provisioner.Install(cmd.Context(), blueprint); err != nil {
				return fmt.Errorf("error installing blueprint: %w", err)
			}

			if err := proj.Provisioner.Wait(cmd.Context(), blueprint); err != nil {
				return fmt.Errorf("error waiting for kustomizations: %w", err)
			}

			if err := proj.Provisioner.WriteVersionMarker(blueprint); err != nil {
				return fmt.Errorf("error writing version marker: %w", err)
			}
			return nil
		}); err != nil {
			return err
		}
		if deferredErr != nil {
			return deferredErr
		}
		if !applied {
			fmt.Fprintln(os.Stderr, "Apply skipped. The context is configured — re-run with --yes to apply.")
			return nil
		}

		if !halted {
			fmt.Fprintln(os.Stderr, "Windsor environment bootstrapped successfully.")
		}
		if proj.Workstation != nil {
			printDeferredWork(os.Stderr, proj.Workstation.DeferredWork(), stdruntime.GOOS)
		}

		return nil
	},
}

// makeBootstrapConfirmFn builds the confirm callback the provisioner calls
// with a BootstrapSummary describing what bootstrap is about to apply. The
// summary is sourced from the blueprint and config — no terraform invocation
// — and is rendered for the operator before they confirm.
//
// The returned finish func must be called once Bootstrap returns; it is a
// no-op when the prompt has already resolved.
//
// Anything other than "y" or "yes" (case-insensitive) at the prompt returns
// false — including empty input and EOF, so non-interactive callers must pass
// --yes. Fires in both global and local project modes; the directory-level
// cue alone isn't enough to tell an operator which components and
// kustomizations are about to move, especially when bootstrap re-applies on
// top of partial state.
func makeBootstrapConfirmFn(in io.Reader, out io.Writer) (provisioner.BootstrapConfirmFn, func(error)) {
	resolved := false
	confirmFn := func(summary *provisioner.BootstrapSummary) bool {
		resolved = true
		printBootstrapSummary(out, summary)
		fmt.Fprintf(out, "Note: bootstrap holds the windsor stack lock during this prompt; concurrent windsor commands in this context will wait up to %s before failing.\n", stacklock.DefaultTimeout)
		fmt.Fprint(out, "Continue? [y/N]: ")
		reader := bufio.NewReader(in)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		return answer == "y" || answer == "yes"
	}
	finish := func(err error) {
		if resolved {
			return
		}
		resolved = true
	}
	return confirmFn, finish
}

// printBootstrapSummary writes the bootstrap intent description to w. The
// header block lists Context and Backend (just the type — no editorial about
// what windsor does internally with state migration). The Terraform section
// lists the path of each enabled component (falling back to ComponentID when
// Path is empty). The Kustomize section lists names, one per line, in
// blueprint order. No status column, no counts — this is intent, not diff.
func printBootstrapSummary(w io.Writer, summary *provisioner.BootstrapSummary) {
	headerWidth := 47
	for _, e := range summary.Terraform {
		if n := len(bootstrapTerraformDisplay(e)); n > headerWidth {
			headerWidth = n
		}
	}
	for _, name := range summary.Kustomize {
		if len(name) > headerWidth {
			headerWidth = len(name)
		}
	}
	sep := strings.Repeat("═", headerWidth)
	fmt.Fprintf(w, "\nWindsor Bootstrap Summary\n%s\n", sep)
	fmt.Fprintf(w, "Context  %s\n", summary.ContextName)
	fmt.Fprintf(w, "Backend  %s\n", summary.BackendType)

	if len(summary.Terraform) > 0 {
		fmt.Fprintln(w, "\nTerraform")
		for _, e := range summary.Terraform {
			fmt.Fprintf(w, "  %s\n", bootstrapTerraformDisplay(e))
		}
	}
	if len(summary.Kustomize) > 0 {
		fmt.Fprintln(w, "\nKustomize")
		for _, name := range summary.Kustomize {
			fmt.Fprintf(w, "  %s\n", name)
		}
	}
	fmt.Fprintln(w)
}

// bootstrapTerraformDisplay returns the path-or-ID identifier shown for a
// Terraform component row in the bootstrap summary. Mirrors the display rule
// used by `windsor plan`: prefer Path (informative module location like
// "cluster/aws-eks"), fall back to ComponentID.
func bootstrapTerraformDisplay(e provisioner.BootstrapTerraformEntry) string {
	if e.Path != "" {
		return e.Path
	}
	return e.ComponentID
}

// confirmBootstrapIfContextExists prompts the user to confirm when the context's values.yaml
// already exists (i.e. the context has been configured by a prior init or bootstrap). Returns
// nil to proceed, an error to abort. Reads a single line from in; anything other than "y" or
// "yes" (case-insensitive) cancels — including empty input and EOF, which means non-interactive
// callers must pass --yes explicitly rather than silently re-bootstrapping. The prompt is
// especially important in global mode where users have no directory-level cue that they're
// about to touch a context they may not have intended to run against.
func confirmBootstrapIfContextExists(in io.Reader, configRoot, contextName string) error {
	valuesPath := filepath.Join(configRoot, "values.yaml")
	if _, err := os.Stat(valuesPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to check existing context configuration: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Context %q is already configured at %s.\n", contextName, configRoot)
	fmt.Fprintln(os.Stderr, "Re-running bootstrap will re-apply terraform, migrate state, and reinstall the blueprint.")
	fmt.Fprint(os.Stderr, "Continue? [y/N]: ")

	reader := bufio.NewReader(in)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		return fmt.Errorf("bootstrap cancelled (pass --yes to skip this prompt on re-runs)")
	}
	return nil
}

func init() {
	bootstrapCmd.Flags().StringVar(&bootstrapPlatform, "platform", "", "Target platform: none, docker, incus, metal, hetzner, aws, azure, gcp, hyperv, vsphere.")
	bootstrapCmd.Flags().StringVar(&bootstrapBlueprint, "blueprint", "", "Blueprint OCI reference. Accepts oci://host/org/repo:tag, host/org/repo:tag, or org/repo:tag (host defaults to ghcr.io). Tag is required.")
	bootstrapCmd.Flags().StringSliceVar(&bootstrapSetFlags, "set", []string{}, "Override config values, e.g. --set dns.enabled=false. May be repeated.")
	bootstrapCmd.Flags().BoolVarP(&bootstrapYes, "yes", "y", false, "Skip all confirmation prompts.")
	rootCmd.AddCommand(bootstrapCmd)
}
