package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/env"
)

// getContextCmd represents the get command
var getContextCmd = &cobra.Command{
	Use:          "get",
	Short:        "Get the current context",
	Long:         "Retrieve and display the current context from the configuration",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

		// Resolve config handler
		configHandler := controller.ResolveConfigHandler()

		// Check if config is loaded
		if !configHandler.IsLoaded() {
			return fmt.Errorf("No context is available. Have you run `windsor init`?")
		}

		// Initialize environment components
		if err := controller.CreateEnvComponents(); err != nil {
			return fmt.Errorf("Error initializing environment components: %w", err)
		}

		// Initialize components
		if err := controller.InitializeComponents(); err != nil {
			return fmt.Errorf("Error initializing components: %w", err)
		}

		// Get the current context
		currentContext := configHandler.GetContext()

		// Print the current context
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
		controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

		// Initialize environment components
		if err := controller.CreateEnvComponents(); err != nil {
			return fmt.Errorf("Error initializing environment components: %w", err)
		}

		// Initialize components
		if err := controller.InitializeComponents(); err != nil {
			return fmt.Errorf("Error initializing components: %w", err)
		}

		// Resolve config handler
		configHandler := controller.ResolveConfigHandler()

		// Check if config is loaded
		if !configHandler.IsLoaded() {
			return fmt.Errorf("Configuration is not loaded. Please ensure it is initialized.")
		}

		// Set the context
		contextName := args[0]
		if err := configHandler.SetContext(contextName); err != nil {
			return fmt.Errorf("Error setting context: %w", err)
		}

		// Create a session invalidation signal to force regeneration of the session token
		// since the context has changed
		windsorEnvPrinter := controller.ResolveEnvPrinter("windsorEnv")
		if windsorEnvPrinter != nil {
			// Note: This type assertion makes testing challenging as mock implementations
			// in tests won't pass this check. However, keeping this assertion ensures type safety
			// in production code. The token invalidation functionality is tested at the
			// WindsorEnvPrinter level instead.
			if wEnv, ok := windsorEnvPrinter.(*env.WindsorEnvPrinter); ok {
				if err := wEnv.CreateSessionInvalidationSignal(); err != nil {
					return fmt.Errorf("Warning: Failed to reset session token: %w", err)
				}
			}
		}

		// Print the context
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
