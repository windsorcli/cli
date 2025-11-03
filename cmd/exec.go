package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/di"
)

// execCmd represents the exec command
var execCmd = &cobra.Command{
	Use:          "exec [command] [args...]",
	Short:        "Execute a command with environment variables",
	Long:         "Execute a command with environment variables loaded from configuration and secrets",
	Args:         cobra.MinimumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("no command provided")
		}

		verbose, _ := cmd.Flags().GetBool("verbose")

		injector := cmd.Context().Value(injectorKey).(di.Injector)

		execCtx := &context.ExecutionContext{
			Injector: injector,
		}

		execCtx, err := context.NewContext(execCtx)
		if err != nil {
			return fmt.Errorf("failed to initialize context: %w", err)
		}

		if err := execCtx.CheckTrustedDirectory(); err != nil {
			return fmt.Errorf("not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve")
		}

		if err := execCtx.HandleSessionReset(); err != nil {
			return err
		}

		if err := execCtx.LoadConfig(); err != nil {
			return err
		}

		if err := execCtx.LoadEnvironment(true); err != nil {
			return fmt.Errorf("failed to load environment: %w", err)
		}

		if err := execCtx.ExecutePostEnvHooks(); err != nil {
			if !verbose {
				return nil
			}
			return err
		}

		for key, value := range execCtx.GetEnvVars() {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("failed to set environment variable %s: %w", key, err)
			}
		}

		command := args[0]
		var commandArgs []string
		if len(args) > 1 {
			commandArgs = args[1:]
		}

		if _, err := execCtx.Shell.Exec(command, commandArgs...); err != nil {
			return fmt.Errorf("failed to execute command: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
}
