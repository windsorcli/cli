package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/runtime"
)

var hookCmd = &cobra.Command{
	Use:          "hook",
	Short:        "Prints out shell hook information per platform (zsh,bash,fish,tcsh,powershell).",
	Long:         "Prints out shell hook information for each platform (zsh,bash,fish,tcsh,powershell).",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps := &runtime.Dependencies{
			Injector: cmd.Context().Value(injectorKey).(di.Injector),
		}

		// Create Runtime and execute hook installation
		if err := runtime.NewRuntime(deps).
			LoadShell().
			InstallHook(args[0]).
			Do(); err != nil {
			return fmt.Errorf("Error installing hook: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(hookCmd)
}
