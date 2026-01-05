package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/runtime"
)

// setCmd represents the set command group
var setCmd = &cobra.Command{
	Use:   "set",
	Short: "Set a resource",
	Long:  "Set a resource",
}

// setContextCmd sets the current context
var setContextCmd = &cobra.Command{
	Use:          "context [context-name]",
	Short:        "Set the current context",
	Long:         "Set the current context in the configuration and save it",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			rtOpts = []*runtime.Runtime{overridesVal.(*runtime.Runtime)}
		}

		rt, err := runtime.NewRuntime(rtOpts...)
		if err != nil {
			return fmt.Errorf("failed to initialize runtime: %w", err)
		}

		if err := rt.ConfigHandler.LoadConfig(); err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if _, err := rt.Shell.WriteResetToken(); err != nil {
			return fmt.Errorf("failed to write reset token: %w", err)
		}

		if err := rt.ConfigHandler.SetContext(args[0]); err != nil {
			return fmt.Errorf("failed to set context: %w", err)
		}

		return nil
	},
}

func init() {
	setCmd.AddCommand(setContextCmd)
	rootCmd.AddCommand(setCmd)
}
