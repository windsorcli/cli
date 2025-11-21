package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/project"
)

var installWaitFlag bool

var installCmd = &cobra.Command{
	Use:          "install",
	Short:        "Install the blueprint's cluster-level services",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var opts []*project.Project
		if overridesVal := cmd.Context().Value(projectOverridesKey); overridesVal != nil {
			opts = []*project.Project{overridesVal.(*project.Project)}
		}

		proj, err := project.NewProject("", opts...)
		if err != nil {
			return err
		}

		proj.Runtime.Shell.SetVerbosity(verbose)

		if err := proj.Runtime.Shell.CheckTrustedDirectory(); err != nil {
			return fmt.Errorf("not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve")
		}

		if err := proj.Configure(nil); err != nil {
			return err
		}

		if err := proj.Initialize(false); err != nil {
			if !verbose {
				return nil
			}
			return err
		}

		blueprint, err := proj.Composer.GenerateBlueprint()
		if err != nil {
			return fmt.Errorf("error generating blueprint: %w", err)
		}
		if err := proj.Provisioner.Install(blueprint); err != nil {
			return fmt.Errorf("error installing blueprint: %w", err)
		}

		if installWaitFlag {
			if err := proj.Provisioner.Wait(blueprint); err != nil {
				return fmt.Errorf("error waiting for kustomizations: %w", err)
			}
		}

		return nil
	},
}

func init() {
	installCmd.Flags().BoolVar(&installWaitFlag, "wait", false, "Wait for kustomizations to be ready")
	rootCmd.AddCommand(installCmd)
}
