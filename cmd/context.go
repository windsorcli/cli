package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// removedContextCmd intercepts `windsor context …` and returns a migration hint
// instead of cobra's default "unknown command". The legacy hidden group was
// removed in v0.9.0; this stub exists only to give pre-v0.9.0 scripts an
// actionable error message and can be deleted in v0.10.0.
var removedContextCmd = &cobra.Command{
	Use:                "context",
	Short:              "Removed: use 'windsor get context' / 'windsor set context'",
	Hidden:             true,
	SilenceUsage:       true,
	DisableFlagParsing: true,
	Args:               cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		replacement := "windsor get context"
		if len(args) > 0 && args[0] == "set" {
			replacement = "windsor set context"
		}
		return fmt.Errorf("'windsor context' was removed in v0.9.0; use '%s' instead. See the v0.9.0 release notes for migration", replacement)
	},
}

var getContextAliasCmd = &cobra.Command{
	Use:          "get-context",
	Short:        "Get the current context",
	Long:         "Get the current context (alias for 'get context').",
	Hidden:       true,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return getContextCmd.RunE(cmd, args)
	},
}

var setContextAliasCmd = &cobra.Command{
	Use:          "set-context [context]",
	Short:        "Set the current context",
	Long:         "Set the current context (alias for 'set context').",
	Hidden:       true,
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setContextCmd.RunE(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(removedContextCmd)
	rootCmd.AddCommand(getContextAliasCmd)
	rootCmd.AddCommand(setContextAliasCmd)
}
