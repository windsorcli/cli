package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var hookCmd = &cobra.Command{
	Use:          "hook",
	Short:        "Prints out shell hook information per platform (zsh,bash,fish,tcsh,powershell).",
	Long:         "Prints out shell hook information for each platform (zsh,bash,fish,tcsh,powershell).",
	SilenceUsage: true,
	PreRunE:      preRunEInitializeCommonComponents,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("No shell name provided")
		}

		shell := controller.ResolveShell()

		return shell.InstallHook(args[0])
	},
}

func init() {
	rootCmd.AddCommand(hookCmd)
}
