package cmd

import (
	"github.com/spf13/cobra"
)

// contextCmd represents the context command group (legacy, kept for backward compatibility)
var contextCmd = &cobra.Command{
	Use:    "context",
	Short:  "Manage contexts (legacy)",
	Long:   "Manage contexts for the application. This command is kept for backward compatibility. Use 'windsor get contexts' and 'windsor set context' instead.",
	Hidden: true,
}

// contextGetCmd routes to the new get context command
var contextGetCmd = &cobra.Command{
	Use:          "get",
	Short:        "Get the current context",
	Long:         "Retrieve and display the current context from the configuration",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		rootCmd.SetArgs(append([]string{"get", "context"}, args...))
		return rootCmd.Execute()
	},
}

// contextSetCmd routes to the new set context command
var contextSetCmd = &cobra.Command{
	Use:          "set [context]",
	Short:        "Set the current context",
	Long:         "Set the current context in the configuration and save it",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		rootCmd.SetArgs(append([]string{"set", "context"}, args...))
		return rootCmd.Execute()
	},
}

// getContextAliasCmd is an alias for the get command
var getContextAliasCmd = &cobra.Command{
	Use:          "get-context",
	Short:        "Alias for 'get context'",
	Long:         "Alias for 'get context'",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		rootCmd.SetArgs(append([]string{"get", "context"}, args...))
		return rootCmd.Execute()
	},
}

// setContextAliasCmd is an alias for the set command
var setContextAliasCmd = &cobra.Command{
	Use:          "set-context [context]",
	Short:        "Alias for 'set context'",
	SilenceUsage: true,
	Long:         "Alias for 'set context'",
	Args:         cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rootCmd.SetArgs(append([]string{"set", "context"}, args...))
		return rootCmd.Execute()
	},
}

func init() {
	contextCmd.AddCommand(contextGetCmd)
	contextCmd.AddCommand(contextSetCmd)
	rootCmd.AddCommand(contextCmd)

	rootCmd.AddCommand(getContextAliasCmd)
	rootCmd.AddCommand(setContextAliasCmd)
}
