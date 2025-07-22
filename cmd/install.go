package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
)

var installWaitFlag bool

var installCmd = &cobra.Command{
	Use:          "install",
	Short:        "Install the blueprint's cluster-level services",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get shared dependency injector from context
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		// First, run the env pipeline in quiet mode to set up environment variables
		envPipeline, err := pipelines.WithPipeline(injector, cmd.Context(), "envPipeline")
		if err != nil {
			return fmt.Errorf("failed to set up env pipeline: %w", err)
		}
		envCtx := context.WithValue(cmd.Context(), "quiet", true)
		envCtx = context.WithValue(envCtx, "decrypt", true)
		if err := envPipeline.Execute(envCtx); err != nil {
			return fmt.Errorf("failed to set up environment: %w", err)
		}

		// Then, run the install pipeline for blueprint installation
		installPipeline, err := pipelines.WithPipeline(injector, cmd.Context(), "installPipeline")
		if err != nil {
			return fmt.Errorf("failed to set up install pipeline: %w", err)
		}

		// Create execution context with flags
		ctx := cmd.Context()
		if installWaitFlag {
			ctx = context.WithValue(ctx, "wait", true)
		}

		// Execute the install pipeline
		if err := installPipeline.Execute(ctx); err != nil {
			return fmt.Errorf("Error executing install pipeline: %w", err)
		}

		return nil
	},
}

func init() {
	installCmd.Flags().BoolVar(&installWaitFlag, "wait", false, "Wait for kustomizations to be ready")
	rootCmd.AddCommand(installCmd)
}
