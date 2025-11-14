package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/di"
)

var hookCmd = &cobra.Command{
	Use:          "hook",
	Short:        "Prints out shell hook information per platform (zsh,bash,fish,tcsh,powershell).",
	Long:         "Prints out shell hook information for each platform (zsh,bash,fish,tcsh,powershell).",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		execCtx := &runtime.Runtime{
			Injector: injector,
		}

		execCtx, err := runtime.NewRuntime(execCtx)
		if err != nil {
			return fmt.Errorf("failed to initialize context: %w", err)
		}

		if err := execCtx.Shell.InstallHook(args[0]); err != nil {
			return fmt.Errorf("error installing hook: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(hookCmd)
}
