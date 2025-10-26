package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
	"github.com/windsorcli/cli/pkg/runtime"
)

var installWaitFlag bool

var installCmd = &cobra.Command{
	Use:          "install",
	Short:        "Install the blueprint's cluster-level services",
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
