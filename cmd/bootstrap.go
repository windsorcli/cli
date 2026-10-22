package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
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

// bootstrapCmd brings up a new environment from the start to the end.
// It configures the project as init does, deploys the infrastructure as up does, then waits for the kustomizations.
// When the blueprint has post-run messages, a wait that does not converge is a warning, because a kustomization can wait for that manual step.
// Without post-run messages, a wait failure is an error.
// It runs terraform and the install when the context has no workstation, and it can run in global mode.
// When the blueprint declares a backend, it moves the backend components through local state on each run. If not, it runs one Up pass.
//
// It requests all tool requirements, because each tool can run.
// When the project override has no runtime, it assigns the new runtime to the override, so Configure does not use a nil runtime.
// When there is no project override or the override has no composer, it builds the project with NewProject, so the composer gets its default.
// It reads the config root from the ConfigHandler, so the guard against a second bootstrap does not depend on NewProject.
// No step writes config before the cloud credentials are valid:
//   - With --yes, it checks the credentials and saves the config before it starts.
//   - Without --yes, it does both in the confirmation callback after the user accepts, so a decline changes nothing.
//
// The confirmation prompt runs while bootstrap holds the stack lock.
// It prints the post-run messages as a summary at the end.
var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap [context]",
	Short: "Bootstrap a fresh environment end-to-end.",
	Long: `First-run setup for a context: applies Terraform, installs the Flux blueprint, migrates state to the configured remote backend, and waits for kustomizations. When the blueprint declares post-run notes describing a manual step, a wait that does not converge is reported as a warning rather than a failure; otherwise a wait failure is fatal.

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
		"docs.seealso": "[`init`](init.md), [`apply`](apply.md), [`destroy`](destroy.md)\n" +
			"[Global flags](../global-flags.md) — `--no-cache`, `--lock-timeout`, `--verbose`",
		"docs.source": "cmd/bootstrap.go",
	},
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		started := time.Now()
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
			return werror.Wrap(err, "failed to write reset token")
		}
		if changingContext {
			contextName = args[0]
			if err := tempRt.ConfigHandler.SetContext(contextName); err != nil {
				return werror.Wrap(err, "failed to set context")
			}
		}

		rt := runtime.NewRuntime(rtOpts...)
		rt.Shell.SetVerbosity(verbose)

		if proj != nil && proj.Runtime == nil {
			proj.Runtime = rt
		}

		if err := rt.Shell.AddCurrentDirToTrustedFile(); err != nil {
			return werror.Wrap(err, "failed to add current directory to trusted file")
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
				return werror.New(fmt.Sprintf("invalid --set format: %s", setFlag), setFormatHint, nil)
			}
			flagOverrides[parts[0]] = parts[1]
		}

		if proj == nil || proj.Composer == nil {
			projectOpts := proj
			if projectOpts == nil {
				projectOpts = &project.Project{Runtime: rt}
			}
			if composerOverrideVal := cmd.Context().Value(composerOverridesKey); composerOverrideVal != nil {
				projectOpts.Composer = composerOverrideVal.(*composer.Composer)
			}
			proj = project.NewProject(contextName, projectOpts)
		}

		if !bootstrapYes {
			configRoot, err := proj.Runtime.ConfigHandler.GetConfigRoot()
			if err != nil {
				return werror.Wrap(err, "failed to resolve config root for bootstrap confirmation")
			}
			if err := confirmBootstrapIfContextExists(cmd.InOrStdin(), configRoot, contextName); err != nil {
				return err
			}
		}

		if err := proj.Configure(flagOverrides); err != nil {
			return err
		}

		if err := proj.Runtime.ConfigHandler.ValidateContextValues(); err != nil {
			return werror.Wrap(err, "invalid configuration")
		}

		blueprintURL, err := resolveBlueprintURL(bootstrapBlueprint, bootstrapPlatform, proj.Runtime.ContextName, proj.Runtime.TemplateRoot, true, proj.Composer.ArtifactBuilder)
		if err != nil {
			return err
		}

		proj.SetToolRequirements(tools.AllRequirements())
		if err := proj.Initialize(false, blueprintURL...); err != nil {
			return err
		}

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
					deferredErr = werror.Wrap(err, "failed to save configuration")
					return false
				}
				return true
			}
		} else {
			if err := requireCloudAuth(cmd, proj); err != nil {
				return err
			}
			if err := proj.Runtime.SaveConfig(saveSet); err != nil {
				return werror.Wrap(err, "failed to save configuration")
			}
		}

		var applied, halted bool
		var postRunMessages []blueprintv1alpha1.Message
		if err := stacklock.With(cmd.Context(), proj.Runtime, "bootstrap", lockTimeout, func() error {
			_, ok, h, err := proj.Bootstrap(cmd.Context(), confirmFn)
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
				return werror.Wrap(err, "error resolving blueprint substitutions")
			}

			postRunMessages = blueprint.Messages

			if err := warnNotReady(os.Stderr, proj.Provisioner.Install(cmd.Context(), blueprint, false), postRunMessages); err != nil {
				return werror.Wrap(err, "error installing blueprint")
			}

			if err := proj.Provisioner.WriteVersionMarker(blueprint); err != nil {
				return werror.Wrap(err, "error writing version marker")
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
			fmt.Fprintf(os.Stderr, "Windsor environment bootstrapped successfully in %s.\n", presenter.FormatDuration(time.Since(started)))
		}
		printPostRunMessages(os.Stderr, postRunMessages)
		if proj.Workstation != nil {
			printDeferredWork(os.Stderr, proj.Workstation.DeferredWork(), stdruntime.GOOS)
		}

		return nil
	},
}

// warnNotReady writes a warning to w and returns nil when err reports kustomizations that are not
// Ready and the blueprint declares post-run messages, which describe a manual step a kustomization
// may legitimately wait on (e.g. delegating nameservers). It returns err unchanged otherwise.
func warnNotReady(w io.Writer, err error, messages []blueprintv1alpha1.Message) error {
	if err == nil || !errors.Is(err, provisioner.ErrKustomizationsNotReady) || len(messages) == 0 {
		return err
	}
	fmt.Fprintf(w, "⚠ Applied; some kustomizations are not yet ready: %v\n", err)
	return nil
}

// printPostRunMessages writes the rendered post-run messages to w under a "Notes" header with a
// divider, matching the bootstrap summary's style so the section stands out from the surrounding
// progress rather than blending into it. Messages are already gated and interpolated by
// GenerateResolved; an empty list prints nothing. Multiple messages are separated by a blank line.
func printPostRunMessages(w io.Writer, messages []blueprintv1alpha1.Message) {
	if len(messages) == 0 {
		return
	}
	printSectionHeader(w, "Notes")
	for i, m := range messages {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, m.Text)
	}
}

// printSectionHeader writes a blank line, title, and a divider to w. The end-of-run sections use
// it so each section stands out from the progress output above it.
func printSectionHeader(w io.Writer, title string) {
	fmt.Fprintf(w, "\n%s\n%s\n", title, strings.Repeat("═", 47))
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
		fmt.Fprintf(out, "Note: bootstrap holds the windsor stack lock during this prompt; concurrent windsor commands in this context will fail immediately unless run with --lock-timeout (e.g. --lock-timeout=%s) to wait instead.\n", stacklock.DefaultTimeout)
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

// confirmBootstrapIfContextExists asks the operator to confirm when values.yaml of the context already exists.
// The file exists when an earlier init or bootstrap configured the context.
// It reads one line from in. Only "y" or "yes", in any case, continues.
// Other input, an empty line, or EOF cancels with a WindsorError that names --yes, so a non-interactive caller must set --yes.
// The prompt is important in global mode, where no directory tells the operator which context the command changes.
func confirmBootstrapIfContextExists(in io.Reader, configRoot, contextName string) error {
	valuesPath := filepath.Join(configRoot, "values.yaml")
	if _, err := os.Stat(valuesPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return werror.Wrap(err, "failed to check existing context configuration")
	}

	fmt.Fprintf(os.Stderr, "Context %q is already configured at %s.\n", contextName, configRoot)
	fmt.Fprintln(os.Stderr, "Re-running bootstrap will re-apply terraform, migrate state, and reinstall the blueprint.")
	fmt.Fprint(os.Stderr, "Continue? [y/N]: ")

	reader := bufio.NewReader(in)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		return werror.New("bootstrap cancelled", "Run windsor bootstrap --yes to skip this prompt.", nil)
	}
	return nil
}

func init() {
	bootstrapCmd.Flags().StringVar(&bootstrapPlatform, "platform", "", "Target platform: none, docker, incus, metal, hetzner, aws, azure, gcp, hyperv, vsphere, proxmox.")
	bootstrapCmd.Flags().StringVar(&bootstrapBlueprint, "blueprint", "", "Blueprint OCI reference. Accepts oci://host/org/repo:tag, host/org/repo:tag, or org/repo:tag (host defaults to ghcr.io). Tag is required.")
	bootstrapCmd.Flags().StringSliceVar(&bootstrapSetFlags, "set", []string{}, "Override config values, e.g. --set dns.enabled=false. May be repeated.")
	bootstrapCmd.Flags().BoolVarP(&bootstrapYes, "yes", "y", false, "Skip all confirmation prompts.")
	rootCmd.AddCommand(bootstrapCmd)
}
