package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/runtime"
)

// =============================================================================
// Bootstrap Command
// =============================================================================

var (
	bootstrapPlatform  string
	bootstrapBlueprint string
	bootstrapSetFlags  []string
)

// bootstrapCmd stands up a fresh Windsor environment end-to-end. It combines init-style
// project configuration with up-style infrastructure deployment and unconditionally waits
// for kustomizations before returning. Unlike `windsor up`, bootstrap continues through
// terraform + install + wait even when the context does not define a workstation, so it
// is suitable for both local workstation contexts and non-workstation contexts (staging,
// production). Unlike `windsor init`, bootstrap does not anchor the current directory as
// a project root — it is allowed to run in global mode, where directory trust is implicit.
// Adaptive terraform backend handling (local-first apply followed by automatic migration
// to the configured remote backend) is inherited from TerraformStack.Up and runs for all
// callers, so bootstrap does not special-case it here.
var bootstrapCmd = &cobra.Command{
	Use:          "bootstrap [context]",
	Short:        "Bootstrap a fresh Windsor environment end-to-end",
	Long:         "Bootstrap a fresh Windsor environment: configure the project, apply terraform (migrating state from local to the configured remote backend as needed), install the blueprint, and wait for kustomizations to be ready.",
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

		if err := proj.Initialize(false, blueprintURL...); err != nil {
			return err
		}

		if err := proj.Runtime.SaveConfig(len(bootstrapSetFlags) > 0); err != nil {
			return fmt.Errorf("failed to save configuration: %w", err)
		}

		if _, err := proj.Up(); err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.GenerateResolved()

		if err := proj.Provisioner.Install(blueprint); err != nil {
			return fmt.Errorf("error installing blueprint: %w", err)
		}

		if err := proj.Provisioner.Wait(blueprint); err != nil {
			return fmt.Errorf("error waiting for kustomizations: %w", err)
		}

		fmt.Fprintln(os.Stderr, "Windsor environment bootstrapped successfully.")

		return nil
	},
}

func init() {
	bootstrapCmd.Flags().StringVar(&bootstrapPlatform, "platform", "", "Specify the platform to use [none|metal|docker|aws|azure|gcp]")
	bootstrapCmd.Flags().StringVar(&bootstrapBlueprint, "blueprint", "", "Specify the blueprint (OCI reference) to use")
	bootstrapCmd.Flags().StringSliceVar(&bootstrapSetFlags, "set", []string{}, "Override configuration values. Example: --set dns.enabled=false")
	rootCmd.AddCommand(bootstrapCmd)
}
