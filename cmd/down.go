package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:          "down",
	Short:        "Stop the local workstation environment",
	Long:         "Stop the local workstation environment by tearing down the VM, stopping container runtimes, and cleaning up context artifacts.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := prepareProject(cmd)
		if err != nil {
			return err
		}

		if proj.Workstation == nil {
			fmt.Fprintln(os.Stderr, "windsor down is only applicable when a workstation is enabled; use windsor destroy to tear down live environments")
			return nil
		}

		if err := proj.Workstation.Down(); err != nil {
			return fmt.Errorf("error tearing down workstation: %w", err)
		}

		if err := proj.PerformCleanup(); err != nil {
			return fmt.Errorf("error performing cleanup: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(downCmd)
}
