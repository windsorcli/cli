package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
)

var hookCmd = &cobra.Command{
	Use:          "hook",
	Short:        "Prints out shell hook information per platform (zsh,bash,fish,tcsh,powershell).",
	Long:         "Prints out shell hook information for each platform (zsh,bash,fish,tcsh,powershell).",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("No shell name provided")
		}

		// Get shared dependency injector from context
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		// Create hook pipeline
		pipeline := pipelines.NewHookPipeline()

		// Initialize the pipeline
		if err := pipeline.Initialize(injector); err != nil {
			return fmt.Errorf("Error initializing: %w", err)
		}

		// Create execution context with shell type
		ctx := context.WithValue(cmd.Context(), "shellType", args[0])
		if verbose {
			ctx = context.WithValue(ctx, "verbose", true)
		}

		// Execute the pipeline
		if err := pipeline.Execute(ctx); err != nil {
			return fmt.Errorf("Error executing hook pipeline: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(hookCmd)
}
