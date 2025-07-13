package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
)

var (
	cleanFlag         bool
	skipK8sFlag       bool
	skipTerraformFlag bool
)

var downCmd = &cobra.Command{
	Use:          "down",
	Short:        "Tear down the Windsor environment",
	Long:         "Tear down the Windsor environment by executing necessary shell commands.",
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

		// Then, run the down pipeline for infrastructure teardown
		downPipeline, err := pipelines.WithPipeline(injector, cmd.Context(), "downPipeline")
		if err != nil {
			return fmt.Errorf("failed to set up down pipeline: %w", err)
		}

		// Create execution context with flags
		ctx := cmd.Context()
		if cleanFlag {
			ctx = context.WithValue(ctx, "clean", true)
		}
		if skipK8sFlag {
			ctx = context.WithValue(ctx, "skipK8s", true)
		}
		if skipTerraformFlag {
			ctx = context.WithValue(ctx, "skipTerraform", true)
		}

		// Execute the down pipeline
		if err := downPipeline.Execute(ctx); err != nil {
			return fmt.Errorf("Error executing down pipeline: %w", err)
		}

		return nil
	},
}

func init() {
	downCmd.Flags().BoolVar(&cleanFlag, "clean", false, "Clean up context specific artifacts")
	downCmd.Flags().BoolVar(&skipK8sFlag, "skip-k8s", false, "Skip Kubernetes cleanup (blueprint cleanup)")
	downCmd.Flags().BoolVar(&skipTerraformFlag, "skip-tf", false, "Skip Terraform cleanup")
	rootCmd.AddCommand(downCmd)
}
