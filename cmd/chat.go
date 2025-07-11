package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
)

var chatCmd = &cobra.Command{
	Use:          "chat",
	Short:        "Interactive chat interface for Windsor CLI",
	Long:         "Interactive chat interface for Windsor CLI assistance and guidance.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get shared dependency injector from context
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		// Create chat pipeline
		pipeline := pipelines.NewChatPipeline()

		// Initialize the pipeline
		if err := pipeline.Initialize(injector, cmd.Context()); err != nil {
			return fmt.Errorf("Error initializing chat pipeline: %w", err)
		}

		// Create execution context with verbose flag if set
		ctx := cmd.Context()
		if verbose {
			ctx = context.WithValue(ctx, "verbose", true)
		}

		// Execute the pipeline
		if err := pipeline.Execute(ctx); err != nil {
			return fmt.Errorf("Error executing chat pipeline: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(chatCmd)
}
