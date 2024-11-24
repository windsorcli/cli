package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/env"
)

var envCmd = &cobra.Command{
	Use:          "env",
	Short:        "Output commands to set environment variables",
	Long:         "Output commands to set environment variables for the application.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Resolve all environments from the injector
		envInstances, err := injector.ResolveAll((*env.EnvPrinter)(nil))
		if err != nil {
			if verbose {
				return fmt.Errorf("Error resolving environments: %w", err)
			}
			return nil
		}

		// Cast envInstances to a slice of EnvPrinter
		envPrinters := make([]env.EnvPrinter, len(envInstances))
		for i, instance := range envInstances {
			envPrinter, _ := instance.(env.EnvPrinter)
			envPrinters[i] = envPrinter
		}

		// Iterate through all environments and run their Initialize, Print, and PostEnvHook functions
		for _, instance := range envPrinters {
			if err := instance.Initialize(); err != nil {
				if verbose {
					return fmt.Errorf("Error executing Initialize: %w", err)
				}
				return nil
			}
			if err := instance.Print(); err != nil {
				if verbose {
					return fmt.Errorf("Error executing Print: %w", err)
				}
				return nil
			}
			if err := instance.PostEnvHook(); err != nil {
				if verbose {
					return fmt.Errorf("Error executing PostEnvHook: %w", err)
				}
				return nil
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(envCmd)
}
