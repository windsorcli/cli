package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

var installWaitFlag bool

var installCmd = &cobra.Command{
	Use:          "install",
	Short:        "Install the blueprint's cluster-level services",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// `install` applies the blueprint to the cluster (Flux + kustomizations); it does not
		// invoke terraform or the local container runtime.
		proj, err := prepareProject(cmd, tools.Requirements{Secrets: true, Kubelogin: true})
		if err != nil {
			return err
		}

		blueprint, err := proj.Composer.BlueprintHandler.GenerateResolved()
		if err != nil {
			return fmt.Errorf("error resolving blueprint substitutions: %w", err)
		}
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
