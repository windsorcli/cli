package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime"
)

// setCmd represents the set command group
var setCmd = &cobra.Command{
	Use:   "set",
	Short: "Set a Windsor resource.",
	Long:  `Set a Windsor resource. Currently supports 'context'.`,
	Annotations: map[string]string{
		"docs.seealso": "[`init`](init.md), [`get`](get.md)",
		"docs.source":  "cmd/set.go",
	},
}

// setContextCmd sets the current context
var setContextCmd = &cobra.Command{
	Use:   "context [context-name]",
	Short: "Switch the current context.",
	Long:  `Switch the current context and persist the choice to the project config. The context directory must already exist (created by 'windsor init').`,
	Example: `windsor set context staging
windsor get context
# → staging`,
	Annotations: map[string]string{
		"docs.seealso": "[`init`](init.md), [`get context`](get-context.md)",
		"docs.source":  "cmd/set.go",
	},
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			rtOpts = []*runtime.Runtime{overridesVal.(*runtime.Runtime)}
		}

		rt := runtime.NewRuntime(rtOpts...)

		contextName := args[0]

		projectRoot, err := rt.Shell.GetProjectRoot()
		if err != nil {
			return werror.Wrap(err, "failed to get project root")
		}

		contextDir := filepath.Join(projectRoot, "contexts", contextName)
		if _, err := os.Stat(contextDir); os.IsNotExist(err) {
			return werror.New(fmt.Sprintf("context %q not found", contextName), fmt.Sprintf("Run windsor init %s to create it.", contextName), nil)
		} else if err != nil {
			return werror.Wrap(err, "failed to check context directory")
		}

		// Load the context being switched to. The current context is the one
		// being left, and its problems are not the user's concern here.
		if err := rt.ConfigHandler.LoadConfigForContext(contextName); err != nil {
			return werror.Wrap(err, "failed to load config")
		}

		if _, err := rt.Shell.WriteResetToken(); err != nil {
			return werror.Wrap(err, "failed to write reset token")
		}

		if err := rt.ConfigHandler.SetContext(contextName); err != nil {
			return werror.Wrap(err, "failed to set context")
		}

		return nil
	},
}

func init() {
	setCmd.AddCommand(setContextCmd)
	rootCmd.AddCommand(setCmd)
}
