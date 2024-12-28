package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/shell"
)

var hookCmd = &cobra.Command{
	Use:          "hook",
	Short:        "Prints out shell hook information per platform.",
	Long:         "Prints out shell hook information for each platform (zsh,bash,fish,tcsh, elvish,powershell).",
	SilenceUsage: true,
	PreRunE:      preRunEInitializeCommonComponents,
	RunE: func(cmd *cobra.Command, args []string) error {

		if len(args) == 0 {
			return fmt.Errorf("No shell name provided")
		}

		shell := shell.NewDefaultShell(nil) // Assuming no injector is needed here
		return shell.InstallHook(args[0])

	},
}

func init() {
	rootCmd.AddCommand(hookCmd)
}
