package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/project"
)

var (
	installFlag bool // Declare the install flag
	waitFlag    bool // Declare the wait flag
)

var upCmd = &cobra.Command{
	Use:          "up",
	Short:        "Set up the Windsor environment",
	Long:         "Set up the Windsor environment by executing necessary shell commands.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		proj, err := project.NewProject(injector, "")
		if err != nil {
			return err
		}

		if err := proj.Context.Shell.CheckTrustedDirectory(); err != nil {
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

		if proj.Workstation != nil {
			if err := proj.Workstation.Up(); err != nil {
				return fmt.Errorf("error starting workstation: %w", err)
			}
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()
		if err := proj.Provisioner.Up(blueprint); err != nil {
			return fmt.Errorf("error starting infrastructure: %w", err)
		}

		if installFlag {
			if err := proj.Provisioner.Install(blueprint); err != nil {
				return fmt.Errorf("error installing blueprint: %w", err)
			}

			if waitFlag {
				if err := proj.Provisioner.Wait(blueprint); err != nil {
					return fmt.Errorf("error waiting for kustomizations: %w", err)
				}
			}
		}

		return nil
	},
}

func init() {
	upCmd.Flags().BoolVar(&installFlag, "install", false, "Install the blueprint after setting up the environment")
	upCmd.Flags().BoolVar(&waitFlag, "wait", false, "Wait for kustomization resources to be ready")
	rootCmd.AddCommand(upCmd)
}
