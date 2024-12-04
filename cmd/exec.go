package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:          "exec -- [command]",
	Short:        "Execute a shell command with environment variables",
	Long:         "Execute a shell command with environment variables set for the application.",
	SilenceUsage: true,
	PreRunE:      preRunEInitializeCommonComponents,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("no command provided")
		}

		// Create environment components
		if err := controller.CreateEnvComponents(); err != nil {
			return fmt.Errorf("Error creating environment components: %w", err)
		}

		// Initialize components
		if err := controller.InitializeComponents(); err != nil {
			return fmt.Errorf("Error initializing components: %w", err)
		}

		// Resolve all environment printers using the controller
		envPrinters := controller.ResolveAllEnvPrinters()
		if len(envPrinters) == 0 {
			return fmt.Errorf("Error resolving environment printers: no printers returned")
		}

		// Collect environment variables from all printers
		envVars := make(map[string]string)
		for _, envPrinter := range envPrinters {
			if err := envPrinter.Print(); err != nil {
				return fmt.Errorf("Error executing Print: %w", err)
			}
			vars, err := envPrinter.GetEnvVars()
			if err != nil {
				return fmt.Errorf("Error getting environment variables: %w", err)
			}
			for k, v := range vars {
				envVars[k] = v
			}
			if err := envPrinter.PostEnvHook(); err != nil {
				return fmt.Errorf("Error executing PostEnvHook: %w", err)
			}
		}

		// Set environment variables for the command
		for k, v := range envVars {
			if err := osSetenv(k, v); err != nil {
				return fmt.Errorf("Error setting environment variable %s: %w", k, err)
			}
		}

		// Resolve the shell instance using the controller
		shellInstance := controller.ResolveShell()
		if shellInstance == nil {
			return fmt.Errorf("Error: no shell found")
		}

		// Execute the command using the resolved shell instance
		output, err := shellInstance.Exec(false, "", args[0], args[1:]...)
		if err != nil {
			return fmt.Errorf("command execution failed: %w", err)
		}

		// Print the command output
		fmt.Println(output)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
}
