package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var installWaitFlag bool

var installCmd = &cobra.Command{
	Use:          "install",
	Short:        "Install the blueprint's cluster-level services",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := prepareProject(cmd)
		if err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.GenerateResolved()
		if err := proj.Provisioner.Install(cmd.Context(), blueprint); err != nil {
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
