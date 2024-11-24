package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/context"
)

// getContextCmd represents the get command
var getContextCmd = &cobra.Command{
	Use:   "get",
	Short: "Get the current context",
	Long:  "Retrieve and display the current context from the configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		contextHandler, err := getContextHandler()
		if err != nil {
			return fmt.Errorf("Error getting context handler: %w", err)
		}
		currentContext, err := contextHandler.GetContext()
		if err != nil {
			return fmt.Errorf("Error getting context: %w", err)
		}
		fmt.Println(currentContext)
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
		contextHandler, err := getContextHandler()
		if err != nil {
			return fmt.Errorf("Error getting context handler: %w", err)
		}
		contextName := args[0]
		if err := contextHandler.SetContext(contextName); err != nil {
			return fmt.Errorf("Error setting context: %w", err)
		}
		fmt.Println("Context set to:", contextName)
		return nil
	},
}

// getContextAliasCmd is an alias for the get command
var getContextAliasCmd = &cobra.Command{
	Use:   "get-context",
	Short: "Alias for 'context get'",
	Long:  "Alias for 'context get'",
	RunE: func(cmd *cobra.Command, args []string) error {
		rootCmd.SetArgs(append([]string{"context", "get"}, args...))
		return rootCmd.Execute()
	},
}

// setContextAliasCmd is an alias for the set command
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

// getContextHandler resolves the contextHandler from the injector and returns it as a context.ContextInterface
var getContextHandler = func() (context.ContextInterface, error) {
	instance, err := injector.Resolve("contextHandler")
	if err != nil {
		return nil, fmt.Errorf("Error resolving contextHandler: %w", err)
	}
	contextHandler, ok := instance.(context.ContextInterface)
	if !ok {
		return nil, fmt.Errorf("Error: resolved instance is not of type context.ContextInterface")
	}
	return contextHandler, nil
}
