package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
)

var envCmd = &cobra.Command{
	Use:          "env",
	Short:        "Output commands to set environment variables",
	Long:         "Output commands to set environment variables for the application.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get shared dependency injector from context
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		// Create env pipeline
		pipeline := pipelines.NewEnvPipeline()

		// Get flags
		hook, _ := cmd.Flags().GetBool("hook")
		decrypt, _ := cmd.Flags().GetBool("decrypt")

		// Initialize the pipeline with appropriate configuration
		if err := pipeline.Initialize(injector); err != nil {
			return fmt.Errorf("Error initializing: %w", err)
		}

		// Create execution context with flags
		ctx := cmd.Context()
		if decrypt {
			ctx = context.WithValue(ctx, "decrypt", true)
		}
		if verbose {
			ctx = context.WithValue(ctx, "verbose", true)
		}
		if hook {
			ctx = context.WithValue(ctx, "hook", true)
		}

		// Execute the pipeline
		if err := pipeline.Execute(ctx); err != nil {
			return fmt.Errorf("Error executing env pipeline: %w", err)
		}

		return nil
	},
}

func init() {
	envCmd.Flags().Bool("decrypt", false, "Decrypt secrets before setting environment variables")
	envCmd.Flags().Bool("hook", false, "Flag that indicates the command is being executed by the hook")
	rootCmd.AddCommand(envCmd)
}
