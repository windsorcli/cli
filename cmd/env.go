package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/di"
)

var envCmd = &cobra.Command{
	Use:          "env",
	Short:        "Output commands to set environment variables",
	Long:         "Output commands to set environment variables for the application.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		hook, _ := cmd.Flags().GetBool("hook")
		decrypt, _ := cmd.Flags().GetBool("decrypt")
		verbose, _ := cmd.Flags().GetBool("verbose")

		if !hook && os.Getenv("NO_CACHE") == "" {
			if err := os.Setenv("NO_CACHE", "true"); err != nil {
				return fmt.Errorf("failed to set NO_CACHE environment variable: %w", err)
			}
		}

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

		if err := execCtx.LoadEnvironment(decrypt); err != nil {
			return fmt.Errorf("failed to load environment: %w", err)
		}

		outputFunc := func(output string) {
			if output != "" {
				fmt.Fprint(cmd.OutOrStdout(), output)
			}
		}

		if hook {
			outputFunc(execCtx.PrintEnvVarsExport())
			outputFunc(execCtx.PrintAliases())
		} else {
			outputFunc(execCtx.PrintEnvVars())
		}

		if err := execCtx.ExecutePostEnvHooks(); err != nil {
			if hook || !verbose {
				return nil
			}
			return err
		}

		return nil
	},
}

func init() {
	envCmd.Flags().Bool("decrypt", false, "Decrypt secrets before setting environment variables")
	envCmd.Flags().Bool("hook", false, "Flag that indicates the command is being executed by the hook")
	envCmd.Flags().Bool("verbose", false, "Show verbose error output")
	rootCmd.AddCommand(envCmd)
}
