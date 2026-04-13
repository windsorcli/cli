package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/project"
)

var (
	waitFlag    bool // Declare the wait flag
	installFlag bool // Deprecated: no-op, kept for backwards compatibility
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

		if err := proj.Configure(nil); err != nil {
			return err
		}

		if err := proj.Runtime.ConfigHandler.ValidateContextValues(); err != nil {
			return fmt.Errorf("invalid configuration: %w", err)
		}

		if err := proj.Initialize(false); err != nil {
			return err
		}

		if proj.Workstation == nil {
			fmt.Fprintln(os.Stderr, "windsor up is only applicable when a workstation is enabled; use windsor apply to apply infrastructure")
			return nil
		}

		blueprint, err := proj.Up()
		if err != nil {
			return err
		}

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

func init() {
	upCmd.Flags().BoolVar(&waitFlag, "wait", false, "Wait for kustomization resources to be ready")
	upCmd.Flags().BoolVar(&installFlag, "install", false, "")
	_ = upCmd.Flags().MarkDeprecated("install", "the --install flag is no longer needed and will be removed in a future release")
	rootCmd.AddCommand(upCmd)
}
