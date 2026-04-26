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
// To handle the chicken-and-egg case where a configured remote backend (e.g. the kubernetes
// backend) lives in infrastructure that terraform must create first, bootstrap temporarily
// overrides the configured backend to "local" in-memory, runs the apply pass, then restores
// the configured backend and calls Provisioner.MigrateState to move each component's state
// to the real backend. The on-disk config (values.yaml) is never mutated during this window.
var bootstrapCmd = &cobra.Command{
	Use:          "bootstrap [context]",
	Short:        "Bootstrap a fresh Windsor environment end-to-end",
	Long:         "Bootstrap a fresh Windsor environment: configure the project, apply terraform with local state first, migrate state to the configured remote backend, install the blueprint, and wait for kustomizations to be ready.",
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
		// minutes into a `terraform apply`.
		if err := proj.Runtime.ToolsManager.CheckAuth(); err != nil {
			return fmt.Errorf("error validating credentials: %w", err)
		}

		if err := proj.Runtime.SaveConfig(len(bootstrapSetFlags) > 0); err != nil {
			return fmt.Errorf("failed to save configuration: %w", err)
		}

		// Temporarily pin the backend to local for the first-run apply pass. The configured
		// value is captured first and restored via defer so any panic or early error still
		// unwinds the in-memory override — guarding against future code in this RunE that
		// might persist config while the override is active. SaveConfig was called above
		// with the real backend, so values.yaml on disk is unaffected by this override.
		ch := proj.Runtime.ConfigHandler
		originalBackend := ch.GetString("terraform.backend.type", "local")
		if err := ch.Set("terraform.backend.type", "local"); err != nil {
			return fmt.Errorf("failed to override backend for bootstrap apply: %w", err)
		}
		defer func() {
			_ = ch.Set("terraform.backend.type", originalBackend)
		}()

		if _, err := proj.Up(); err != nil {
			return err
		}

		// Restore the configured backend before migration so MigrateState reads the real
		// config. The deferred restore above remains a safety net for any exit path below.
		// MigrateState is a no-op per component when state is already on the configured
		// backend, so re-runs (e.g. re-bootstrapping after success) stay safe.
		if err := ch.Set("terraform.backend.type", originalBackend); err != nil {
			return fmt.Errorf("failed to restore backend after bootstrap apply: %w", err)
		}
		// Intentionally pass the freshly-generated blueprint without a nil guard. If Generate
		// returns nil here, that's an anomaly (proj.Up just succeeded against a blueprint a
		// few lines up) and MigrateState's own nil check will surface it as a loud error
		// rather than silently skipping migration — a skipped migration would leave state on
		// local disk while the on-disk config points at the real remote backend, a genuinely
		// wrong outcome that subsequent `windsor apply` runs would discover inconsistently.
		if err := proj.Provisioner.MigrateState(proj.Composer.BlueprintHandler.Generate()); err != nil {
			return err
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
	bootstrapCmd.Flags().StringVar(&bootstrapPlatform, "platform", "", "Specify the platform to use [none|metal|docker|aws|azure|gcp]")
	bootstrapCmd.Flags().StringVar(&bootstrapBlueprint, "blueprint", "", "Specify the blueprint (OCI reference) to use")
	bootstrapCmd.Flags().StringSliceVar(&bootstrapSetFlags, "set", []string{}, "Override configuration values. Example: --set dns.enabled=false")
	bootstrapCmd.Flags().BoolVarP(&bootstrapYes, "yes", "y", false, "Skip the confirmation prompt when re-bootstrapping an existing context")
	rootCmd.AddCommand(bootstrapCmd)
}
