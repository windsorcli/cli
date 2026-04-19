package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/project"
)

var (
	waitFlag    bool // Declare the wait flag
	installFlag bool // Deprecated: no-op, kept for backwards compatibility
	upVmDriver  string
	upPlatform  string
	upBlueprint string
	upSetFlags  []string
)

var upCmd = &cobra.Command{
	Use:          "up",
	Short:        "Bring up the local workstation environment",
	Long:         "Bring up the local workstation environment by starting the VM, applying Terraform, and installing the blueprint.",
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

		blueprintURL, err := resolveBlueprintURL(upBlueprint, upPlatform, proj.Runtime.ContextName, proj.Runtime.TemplateRoot)
		if err != nil {
			return err
		}

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

		if _, err := proj.Up(); err != nil {
			return err
		}

		// Re-generate with deferred substitutions resolved now that terraform
		// outputs are available from the Up step above.
		blueprint := proj.Composer.BlueprintHandler.GenerateResolved()

		if err := proj.Provisioner.Install(blueprint); err != nil {
			return fmt.Errorf("error installing blueprint: %w", err)
		}

		if waitFlag {
			if err := proj.Provisioner.Wait(blueprint); err != nil {
				return fmt.Errorf("error waiting for kustomizations: %w", err)
			}
		}

		fmt.Fprintln(os.Stderr, "Windsor environment set up successfully.")

		return nil
	},
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
	upCmd.Flags().BoolVar(&waitFlag, "wait", false, "Wait for kustomization resources to be ready")
	upCmd.Flags().BoolVar(&installFlag, "install", false, "")
	_ = upCmd.Flags().MarkDeprecated("install", "the --install flag is no longer needed and will be removed in a future release")
	upCmd.Flags().StringVar(&upVmDriver, "vm-driver", "", "VM driver (colima, colima-incus, docker-desktop, docker)")
	upCmd.Flags().StringVar(&upPlatform, "platform", "", "Specify the platform to use [none|metal|docker|aws|azure|gcp]")
	upCmd.Flags().StringVar(&upBlueprint, "blueprint", "", "Specify the blueprint to use")
	upCmd.Flags().StringSliceVar(&upSetFlags, "set", []string{}, "Override configuration values. Example: --set dns.enabled=false")
	rootCmd.AddCommand(upCmd)
}
