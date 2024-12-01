package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var envCmd = &cobra.Command{
	Use:          "env",
	Short:        "Output commands to set environment variables",
	Long:         "Output commands to set environment variables for the application.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Initialize the controller
		if err := controller.Initialize(); err != nil {
			if verbose {
				return fmt.Errorf("Error initializing controller: %w", err)
			}
			return nil
		}

		// Create environment components
		if err := controller.CreateEnvComponents(); err != nil {
			if verbose {
				return fmt.Errorf("Error creating environment components: %w", err)
			}
			return nil
		}

		// Initialize components
		if err := controller.InitializeComponents(); err != nil {
			if verbose {
				return fmt.Errorf("Error initializing components: %w", err)
			}
			return nil
		}

		// Resolve all environment printers using the controller
		envPrinters, err := controller.ResolveAllEnvPrinters()
		if err != nil {
			if verbose {
				return fmt.Errorf("Error resolving environment printers: %w", err)
			}
			return nil
		}

		// Iterate through all environment printers and run their Print and PostEnvHook functions
		for _, envPrinter := range envPrinters {
			if err := envPrinter.Print(); err != nil {
				if verbose {
					return fmt.Errorf("Error executing Print: %w", err)
				}
				return nil
			}
			if err := envPrinter.PostEnvHook(); err != nil {
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
