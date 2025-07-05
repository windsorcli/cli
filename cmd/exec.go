package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
)

var execCmd = &cobra.Command{
	Use:          "exec -- [command]",
	Short:        "Execute a shell command with environment variables",
	Long:         "Execute a shell command with environment variables set for the application.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("no command provided")
		}

		injector := cmd.Context().Value(injectorKey).(di.Injector)

		var pipeline pipelines.Pipeline
		if existingPipeline := injector.Resolve("execPipeline"); existingPipeline != nil {
			pipeline = existingPipeline.(pipelines.Pipeline)
		} else {
			pipeline = pipelines.NewExecPipeline()
			if err := pipeline.Initialize(injector); err != nil {
				return fmt.Errorf("failed to initialize exec pipeline: %w", err)
			}
		}

		ctx := context.Background()
		ctx = context.WithValue(ctx, "command", args[0])
		if len(args) > 1 {
			ctx = context.WithValue(ctx, "args", args[1:])
		}

		return pipeline.Execute(ctx)
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
}
