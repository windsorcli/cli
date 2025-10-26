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
	cleanFlag         bool
	skipK8sFlag       bool
	skipTerraformFlag bool
	skipDockerFlag    bool
)

var downCmd = &cobra.Command{
	Use:          "down",
	Short:        "Tear down the Windsor environment",
	Long:         "Tear down the Windsor environment by executing necessary shell commands.",
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

		// Finally, run the down pipeline for infrastructure teardown
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
		if skipDockerFlag {
			ctx = context.WithValue(ctx, "skipDocker", true)
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
	downCmd.Flags().BoolVar(&skipDockerFlag, "skip-docker", false, "Skip Docker container cleanup")
	rootCmd.AddCommand(downCmd)
}
