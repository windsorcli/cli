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
	PreRunE:      preRunEInitializeCommonComponents,
	RunE: func(cmd *cobra.Command, args []string) error {

		// currentDir, err := os.Getwd()
		// if err != nil {
		// 	return fmt.Errorf("Error getting current directory: %w", err)
		// }

		// usr, err := user.Current()
		// if err != nil {
		// 	return fmt.Errorf("Error getting user home directory: %w", err)
		// }

		// trustedFilePath := path.Join(usr.HomeDir, ".config", "windsor", ".trusted")
		// file, err := os.Open(trustedFilePath)
		// if err != nil {
		// 	return fmt.Errorf("Error opening trusted file: %w", err)
		// }
		// defer file.Close()

		// scanner := bufio.NewScanner(file)
		// isTrusted := false
		// for scanner.Scan() {
		// 	if strings.TrimSpace(scanner.Text()) == currentDir {
		// 		isTrusted = true
		// 		break
		// 	}
		// }

		// if !isTrusted {
		// 	return nil
		// }

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
		envPrinters := controller.ResolveAllEnvPrinters()
		if len(envPrinters) == 0 && verbose {
			return fmt.Errorf("Error resolving environment printers: no printers returned")
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
