package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
	"github.com/windsorcli/cli/pkg/runtime"
)

var (
	installFlag bool // Declare the install flag
	waitFlag    bool // Declare the wait flag
)

var upCmd = &cobra.Command{
	Use:          "up",
	Short:        "Set up the Windsor environment",
	Long:         "Set up the Windsor environment by executing necessary shell commands.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get shared dependency injector from context
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		// First, set up environment variables using runtime
		deps := &runtime.Dependencies{
			Injector: injector,
		}
		if err := runtime.NewRuntime(deps).
			LoadShell().
			CheckTrustedDirectory().
			LoadConfig().
			LoadSecretsProviders().
			LoadEnvVars(runtime.EnvVarsOptions{
				Decrypt: true,
				Verbose: verbose,
			}).
			ExecutePostEnvHook(verbose).
			Do(); err != nil {
			return fmt.Errorf("failed to set up environment: %w", err)
		}

		// Then, run the init pipeline to initialize the environment
		var initPipeline pipelines.Pipeline
		initPipeline, err := pipelines.WithPipeline(injector, cmd.Context(), "initPipeline")
		if err != nil {
			return fmt.Errorf("failed to set up init pipeline: %w", err)
		}
		if err := initPipeline.Execute(cmd.Context()); err != nil {
			return fmt.Errorf("failed to initialize environment: %w", err)
		}

		// Finally, run the up pipeline for infrastructure setup
		upPipeline, err := pipelines.WithPipeline(injector, cmd.Context(), "upPipeline")
		if err != nil {
			return fmt.Errorf("failed to set up up pipeline: %w", err)
		}

		// Create execution context with flags
		ctx := cmd.Context()
		if installFlag {
			ctx = context.WithValue(ctx, "install", true)
		}
		if waitFlag {
			ctx = context.WithValue(ctx, "wait", true)
		}

		// Execute the up pipeline
		if err := upPipeline.Execute(ctx); err != nil {
			return fmt.Errorf("Error executing up pipeline: %w", err)
		}

		// Run install pipeline if requested
		if installFlag {
			installPipeline, err := pipelines.WithPipeline(injector, cmd.Context(), "installPipeline")
			if err != nil {
				return fmt.Errorf("failed to set up install pipeline: %w", err)
			}

			// Create installation context with wait flag if needed
			installCtx := cmd.Context()
			if waitFlag {
				installCtx = context.WithValue(installCtx, "wait", true)
			}

			if err := installPipeline.Execute(installCtx); err != nil {
				return fmt.Errorf("Error executing install pipeline: %w", err)
			}
		}

		return nil
	},
}

func init() {
	upCmd.Flags().BoolVar(&installFlag, "install", false, "Install the blueprint after setting up the environment")
	upCmd.Flags().BoolVar(&waitFlag, "wait", false, "Wait for kustomization resources to be ready")
	rootCmd.AddCommand(upCmd)
}
