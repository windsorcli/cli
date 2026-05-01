package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/provisioner"
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
// To handle the chicken-and-egg case where a configured remote backend (e.g. an S3 bucket
// or kubernetes Secret) lives in infrastructure that terraform must create first, bootstrap
// uses a two-phase apply when the blueprint declares a "backend" terraform component:
//
//  1. Override terraform.backend.type to "local" in-memory and apply only the backend
//     component, materializing the remote state store (bucket, dynamodb table, etc.).
//  2. Restore the configured backend type and migrate just the backend component's state
//     to remote via MigrateComponentState. Subsequent components in the next Up pass init
//     directly against the configured remote backend with no migration needed, since they
//     have not been applied yet.
//
// When the blueprint has no backend component, bootstrap falls through to a single Up
// pass against whatever backend type is configured (typically "local" for non-cloud
// contexts, or a backend whose bucket exists out-of-band). The on-disk config
// (values.yaml) is never mutated during the override window.
var bootstrapCmd = &cobra.Command{
	Use:          "bootstrap [context]",
	Short:        "Bootstrap a fresh environment end-to-end",
	Long:         "First-run setup for a context: applies Terraform, installs the Flux blueprint, and migrates state to the configured remote backend. Use `windsor apply` for everything after.",
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

		blueprintURL, err := resolveBlueprintURL(bootstrapBlueprint, bootstrapPlatform, proj.Runtime.ContextName, proj.Runtime.TemplateRoot, true)
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

		// Validate cloud credentials before any infrastructure-touching work runs. CheckAuth
		// is intentionally NOT part of Initialize/PrepareTools (which fire from `windsor init`
		// where the operator has no obligation to be authed yet); bootstrap is the first
		// command that will exercise credentials, so failing here gives the operator the
		// vendor's own error (expired SSO, profile not found, etc.) up front rather than
		// minutes into a `terraform apply`. Routed through requireCloudAuth so the calm
		// output pattern (just the hint, no scary "Error:" prefix, no stacked wrappers) is
		// consistent across all preflight call sites.
		if err := requireCloudAuth(cmd, proj); err != nil {
			return err
		}

		if err := proj.Runtime.SaveConfig(len(bootstrapSetFlags) > 0); err != nil {
			return fmt.Errorf("failed to save configuration: %w", err)
		}

		var confirmFn provisioner.BootstrapConfirmFn
		finishPlan := func(error) {}
		if proj.Runtime.Global && !bootstrapYes {
			confirmFn, finishPlan = makeBootstrapConfirmFn(cmd.InOrStdin(), os.Stderr)
		}

		_, applied, err := proj.Bootstrap(confirmFn)
		finishPlan(err)
		if err != nil {
			return err
		}
		if !applied {
			fmt.Fprintln(os.Stderr, "Apply skipped. The context is configured — re-run with --yes to apply.")
			return nil
		}

		blueprint := proj.Composer.BlueprintHandler.GenerateResolved()

		if err := proj.Provisioner.Install(cmd.Context(), blueprint); err != nil {
			return fmt.Errorf("error installing blueprint: %w", err)
		}

		if err := proj.Provisioner.Wait(blueprint); err != nil {
			return fmt.Errorf("error waiting for kustomizations: %w", err)
		}

		fmt.Fprintln(os.Stderr, "Windsor environment bootstrapped successfully.")

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
// --yes. Reserved for global mode by the caller; in local project mode the
// operator has the directory-level cue and can run `windsor plan` separately.
func makeBootstrapConfirmFn(in io.Reader, out io.Writer) (provisioner.BootstrapConfirmFn, func(error)) {
	resolved := false
	confirmFn := func(summary *provisioner.BootstrapSummary) bool {
		resolved = true
		printBootstrapSummary(out, summary)
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
	bootstrapCmd.Flags().StringVar(&bootstrapPlatform, "platform", "", "Target platform [none|metal|docker|aws|azure|gcp]")
	bootstrapCmd.Flags().StringVar(&bootstrapBlueprint, "blueprint", "", "Blueprint OCI reference (oci://ghcr.io/org/repo:tag, ghcr.io/org/repo:tag, or org/repo:tag — host defaults to ghcr.io; tag is required)")
	bootstrapCmd.Flags().StringSliceVar(&bootstrapSetFlags, "set", []string{}, "Override config values, e.g. --set dns.enabled=false")
	bootstrapCmd.Flags().BoolVarP(&bootstrapYes, "yes", "y", false, "Skip all confirmation prompts")
	rootCmd.AddCommand(bootstrapCmd)
}
