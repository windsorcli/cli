package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
)

// execCmd represents the exec command
var execCmd = &cobra.Command{
	Use:               "exec [command] [args...]",
	Short:             "Execute a command with environment variables",
	Long:              "Execute a command with environment variables loaded from configuration and secrets",
	Args:              cobra.MinimumNArgs(1),
	PersistentPreRunE: checkTrust,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Safety check for arguments
		if len(args) == 0 {
			return fmt.Errorf("no command provided")
		}

		// Get shared dependency injector from context
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		// First, run the env pipeline in quiet mode to set up environment variables
		envPipeline, err := pipelines.WithPipeline(injector, cmd.Context(), "envPipeline")
		if err != nil {
			return fmt.Errorf("failed to set up env pipeline: %w", err)
		}

		// Execute env pipeline in quiet mode (inject environment variables without printing)
		envCtx := context.WithValue(cmd.Context(), "quiet", true)
		envCtx = context.WithValue(envCtx, "decrypt", true)
		if err := envPipeline.Execute(envCtx); err != nil {
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
