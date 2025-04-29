package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	ctrl "github.com/windsorcli/cli/pkg/controller"
)

// getContextCmd represents the get command
var getContextCmd = &cobra.Command{
	Use:          "get",
	Short:        "Get the current context",
	Long:         "Retrieve and display the current context from the configuration",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

		// Initialize environment components
		if err := controller.InitializeWithRequirements(ctrl.Requirements{
			Env:         true,
			CommandName: cmd.Name(),
		}); err != nil {
			return fmt.Errorf("Error initializing environment components: %w", err)
		}

		// Resolve config handler
		configHandler := controller.ResolveConfigHandler()

		// Check if config is loaded
		if !configHandler.IsLoaded() {
			return fmt.Errorf("No context is available. Have you run `windsor init`?")
		}

		// Set the environment variables internally in the process
		if err := controller.SetEnvironmentVariables(); err != nil {
			return fmt.Errorf("Error setting environment variables: %w", err)
		}

		// Get the current context
		currentContext := configHandler.GetContext()

		// Print the current context
		fmt.Fprintln(cmd.OutOrStdout(), currentContext)
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
		controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

		// Initialize environment components
		if err := controller.InitializeWithRequirements(ctrl.Requirements{
			ConfigLoaded: true,
			Env:          true,
			CommandName:  cmd.Name(),
		}); err != nil {
			return fmt.Errorf("Error initializing environment components: %w", err)
		}

		// Set the environment variables internally in the process
		if err := controller.SetEnvironmentVariables(); err != nil {
			return fmt.Errorf("Error setting environment variables: %w", err)
		}

		// Resolve config handler
		configHandler := controller.ResolveConfigHandler()

		// Write a reset token to reset the session
		shell := controller.ResolveShell()
		if _, err := shell.WriteResetToken(); err != nil {
			return fmt.Errorf("Error writing reset token: %w", err)
		}

		// Set the context
		contextName := args[0]
		if err := configHandler.SetContext(contextName); err != nil {
			return fmt.Errorf("Error setting context: %w", err)
		}

		// Print the context
		fmt.Fprintln(cmd.OutOrStdout(), "Context set to:", contextName)
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
