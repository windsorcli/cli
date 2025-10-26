package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/runtime"
)

var envCmd = &cobra.Command{
	Use:          "env",
	Short:        "Output commands to set environment variables",
	Long:         "Output commands to set environment variables for the application.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get flags
		hook, _ := cmd.Flags().GetBool("hook")
		decrypt, _ := cmd.Flags().GetBool("decrypt")
		verbose, _ := cmd.Flags().GetBool("verbose")

		// Set NO_CACHE=true unless --hook is specified or NO_CACHE is already set
		if !hook && os.Getenv("NO_CACHE") == "" {
			if err := os.Setenv("NO_CACHE", "true"); err != nil {
				return fmt.Errorf("failed to set NO_CACHE environment variable: %w", err)
			}
		}

		// Create dependencies with injector from command context
		deps := &runtime.Dependencies{
			Injector: cmd.Context().Value(injectorKey).(di.Injector),
		}

		// Create output function for environment variables and aliases
		outputFunc := func(output string) {
			fmt.Fprint(cmd.OutOrStdout(), output)
		}

		// Execute the complete workflow
		rt := runtime.NewRuntime(deps).
			LoadShell().
			CheckTrustedDirectory().
			HandleSessionReset().
			LoadConfig().
			LoadSecretsProviders().
			LoadEnvVars(runtime.EnvVarsOptions{
				Decrypt: decrypt,
				Verbose: verbose,
			}).
			PrintEnvVars(runtime.EnvVarsOptions{
				Verbose:    verbose,
				Export:     hook,
				OutputFunc: outputFunc,
			})

		// Only print aliases in hook mode
		if hook {
			rt = rt.PrintAliases(outputFunc)
		}

		if err := rt.ExecutePostEnvHook(verbose).Do(); err != nil {
			if hook {
				// In hook mode, return success even if there are errors
				// This prevents shell initialization failures from breaking the environment
				return nil
			}
			return fmt.Errorf("Error executing environment workflow: %w", err)
		}

		return nil
	},
}

func init() {
	envCmd.Flags().Bool("decrypt", false, "Decrypt secrets before setting environment variables")
	envCmd.Flags().Bool("hook", false, "Flag that indicates the command is being executed by the hook")
	rootCmd.AddCommand(envCmd)
}
