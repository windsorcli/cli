package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/runtime"
)

var hookCmd = &cobra.Command{
	Use:          "hook",
	Short:        "Prints out shell hook information per platform (zsh,bash,fish,tcsh,powershell).",
	Long:         "Prints out shell hook information for each platform (zsh,bash,fish,tcsh,powershell).",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			if rt, ok := overridesVal.(*runtime.Runtime); ok {
				rtOpts = []*runtime.Runtime{rt}
			}
		}

		rt := runtime.NewRuntime(rtOpts...)

		if err := rt.Shell.InstallHook(args[0]); err != nil {
			return fmt.Errorf("error installing hook: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(hookCmd)
}
