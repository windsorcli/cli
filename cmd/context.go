package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
)

// getContextCmd represents the get command
var getContextCmd = &cobra.Command{
	Use:          "get",
	Short:        "Get the current context",
	Long:         "Retrieve and display the current context from the configuration",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get shared dependency injector from context
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		// Create context pipeline
		pipeline := pipelines.NewContextPipeline()

		// Create output function
		outputFunc := func(output string) {
			fmt.Fprintln(cmd.OutOrStdout(), output)
		}

		// Create execution context with operation and output function
		ctx := context.WithValue(cmd.Context(), "operation", "get")
		ctx = context.WithValue(ctx, "output", outputFunc)

		// Initialize the pipeline
		if err := pipeline.Initialize(injector, ctx); err != nil {
			return fmt.Errorf("Error initializing: %w", err)
		}

		// Execute the pipeline
		if err := pipeline.Execute(ctx); err != nil {
			return fmt.Errorf("Error executing context pipeline: %w", err)
		}

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
		// Get shared dependency injector from context
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		// Create context pipeline
		pipeline := pipelines.NewContextPipeline()

		// Create output function
		outputFunc := func(output string) {
			fmt.Fprintln(cmd.OutOrStdout(), output)
		}

		// Create execution context with operation, context name, and output function
		ctx := context.WithValue(cmd.Context(), "operation", "set")
		ctx = context.WithValue(ctx, "contextName", args[0])
		ctx = context.WithValue(ctx, "output", outputFunc)

		// Initialize the pipeline
		if err := pipeline.Initialize(injector, ctx); err != nil {
			return fmt.Errorf("Error initializing: %w", err)
		}

		// Execute the pipeline
		if err := pipeline.Execute(ctx); err != nil {
			return fmt.Errorf("Error executing context pipeline: %w", err)
		}

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
