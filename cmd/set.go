package cmd

import (
	"fmt"
	"os"
	"path/filepath"

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

		contextName := args[0]

		projectRoot, err := rt.Shell.GetProjectRoot()
		if err != nil {
			return fmt.Errorf("failed to get project root: %w", err)
		}

		contextDir := filepath.Join(projectRoot, "contexts", contextName)
		if _, err := os.Stat(contextDir); os.IsNotExist(err) {
			return fmt.Errorf("context %q not found. Run 'windsor init %s' to create it", contextName, contextName)
		} else if err != nil {
			return fmt.Errorf("failed to check context directory: %w", err)
		}

		if err := rt.ConfigHandler.LoadConfig(); err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if _, err := rt.Shell.WriteResetToken(); err != nil {
			return fmt.Errorf("failed to write reset token: %w", err)
		}

		if err := rt.ConfigHandler.SetContext(contextName); err != nil {
			return fmt.Errorf("failed to set context: %w", err)
		}

		return nil
	},
}

func init() {
	setCmd.AddCommand(setContextCmd)
	rootCmd.AddCommand(setCmd)
}
