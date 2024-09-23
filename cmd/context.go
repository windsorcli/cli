package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// getContextCmd represents the get command
var getContextCmd = &cobra.Command{
	Use:   "get",
	Short: "Get the current context",
	Long:  "Retrieve and display the current context from the configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		context, err := contextInstance.GetContext()
		if err != nil {
			return fmt.Errorf("Error getting context: %w", err)
		}
		fmt.Println(context)
		return nil
	},
}

// setContextCmd represents the set command
var setContextCmd = &cobra.Command{
	Use:   "set [context]",
	Short: "Set the current context",
	Long:  "Set the current context in the configuration and save it",
	Args:  cobra.ExactArgs(1), // Ensure exactly one argument is provided
	RunE: func(cmd *cobra.Command, args []string) error {
		contextName := args[0]
		if err := contextInstance.SetContext(contextName); err != nil {
			return fmt.Errorf("Error setting context: %w", err)
		}
		fmt.Println("Context set to:", contextName)
		return nil
	},
}

// Alias commands
var getContextAliasCmd = &cobra.Command{
	Use:   "get-context",
	Short: "Alias for 'context get'",
	Long:  "Alias for 'context get'",
	RunE: func(cmd *cobra.Command, args []string) error {
		rootCmd.SetArgs(append([]string{"context", "get"}, args...))
		return rootCmd.Execute()
	},
}

var setContextAliasCmd = &cobra.Command{
	Use:   "set-context [context]",
	Short: "Alias for 'context set'",
	Long:  "Alias for 'context set'",
	Args:  cobra.ExactArgs(1), // Ensure exactly one argument is provided
	RunE: func(cmd *cobra.Command, args []string) error {
		rootCmd.SetArgs(append([]string{"context", "set"}, args...))
		return rootCmd.Execute()
	},
}

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "context",
		Short: "Manage contexts",
		Long:  "Manage contexts for the application",
	})

	contextCmd := rootCmd.Commands()[len(rootCmd.Commands())-1]
	contextCmd.AddCommand(getContextCmd)
	contextCmd.AddCommand(setContextCmd)

	// Add alias commands to rootCmd
	rootCmd.AddCommand(getContextAliasCmd)
	rootCmd.AddCommand(setContextAliasCmd)
}
