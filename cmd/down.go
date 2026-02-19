package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/project"
)

var (
	cleanFlag         bool
	skipK8sFlag       bool
	skipTerraformFlag bool
	skipDockerFlag    bool
)

var downCmd = &cobra.Command{
	Use:          "down",
	Short:        "Tear down the Windsor environment",
	Long:         "Tear down the Windsor environment by executing necessary shell commands.",
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

		if err := proj.Initialize(false); err != nil {
			return err
		}

		if !skipK8sFlag {
			blueprint := proj.Composer.BlueprintHandler.Generate()
			if err := proj.Provisioner.Uninstall(blueprint); err != nil {
				return fmt.Errorf("error running blueprint cleanup: %w", err)
			}
		} else {
			fmt.Fprintln(os.Stderr, "Skipping Kubernetes cleanup (--skip-k8s set)")
		}

		if !skipTerraformFlag {
			blueprint := proj.Composer.BlueprintHandler.Generate()
			if err := proj.Provisioner.Down(blueprint); err != nil {
				return fmt.Errorf("error tearing down infrastructure: %w", err)
			}
		} else {
			fmt.Fprintln(os.Stderr, "Skipping Terraform cleanup (--skip-tf set)")
		}

		if !skipDockerFlag {
			if proj.Workstation != nil {
				if err := proj.Workstation.Down(); err != nil {
					return fmt.Errorf("error tearing down workstation: %w", err)
				}
			}
		} else {
			fmt.Fprintln(os.Stderr, "Skipping Docker container cleanup (--skip-docker set)")
		}

		if cleanFlag {
			if err := proj.PerformCleanup(); err != nil {
				return fmt.Errorf("error performing cleanup: %w", err)
			}
		}

		fmt.Fprintln(os.Stderr, "Windsor environment torn down successfully.")
		return nil
	},
}

func init() {
	downCmd.Flags().BoolVar(&cleanFlag, "clean", false, "Clean up context specific artifacts")
	downCmd.Flags().BoolVar(&skipK8sFlag, "skip-k8s", false, "Skip Kubernetes cleanup (blueprint cleanup)")
	downCmd.Flags().BoolVar(&skipTerraformFlag, "skip-tf", false, "Skip Terraform cleanup")
	downCmd.Flags().BoolVar(&skipDockerFlag, "skip-docker", false, "Skip Docker container cleanup")
	rootCmd.AddCommand(downCmd)
}
