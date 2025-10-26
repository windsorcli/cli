package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
	"github.com/windsorcli/cli/pkg/runtime"
)

// execCmd represents the exec command
var execCmd = &cobra.Command{
	Use:          "exec [command] [args...]",
	Short:        "Execute a command with environment variables",
	Long:         "Execute a command with environment variables loaded from configuration and secrets",
	Args:         cobra.MinimumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Safety check for arguments
		if len(args) == 0 {
			return fmt.Errorf("no command provided")
		}

		verbose, _ := cmd.Flags().GetBool("verbose")

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

		// Then, run the exec pipeline to execute the command
		execPipeline, err := pipelines.WithPipeline(injector, cmd.Context(), "execPipeline")
		if err != nil {
			return fmt.Errorf("failed to set up exec pipeline: %w", err)
		}

		// Create execution context with command and arguments
		execCtx := context.WithValue(cmd.Context(), "command", args[0])
		if len(args) > 1 {
			execCtx = context.WithValue(execCtx, "args", args[1:])
		}

		// Execute the command
		if err := execPipeline.Execute(execCtx); err != nil {
			return fmt.Errorf("failed to execute command: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
}
