package cmd

import (
	"github.com/spf13/cobra"
)

var getContextAliasCmd = &cobra.Command{
	Use:          "get-context",
	Short:        "Alias for 'get context'",
	Long:         "Alias for 'get context'",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return getContextCmd.RunE(cmd, args)
	},
}

var setContextAliasCmd = &cobra.Command{
	Use:          "set-context [context]",
	Short:        "Alias for 'set context'",
	Long:         "Alias for 'set context'",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setContextCmd.RunE(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(getContextAliasCmd)
	rootCmd.AddCommand(setContextAliasCmd)
}
