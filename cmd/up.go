package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Set up the Windsor environment",
	Long:  "Set up the Windsor environment by executing necessary shell commands.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get the context
		contextName, err := contextInstance.GetContext()
		if err != nil {
			return fmt.Errorf("Error getting context: %w", err)
		}

		// Get the context configuration
		contextConfig, err := cliConfigHandler.GetConfig()
		if err != nil {
			if verbose {
				return fmt.Errorf("Error getting context configuration: %w", err)
			}
			return nil
		}

		// Ensure VM is set before continuing
		if contextConfig.VM == nil {
			if verbose {
				fmt.Println("VM configuration is not set, skipping VM start")
			}
			return nil
		}

		// Collect environment variables
		envVars, err := collectEnvVars()
		if err != nil {
			return err
		}

		// Set environment variables for the command
		for k, v := range envVars {
			if err := osSetenv(k, v); err != nil {
				return fmt.Errorf("Error setting environment variable %s: %w", k, err)
			}
		}

		// Check the VM.Driver value and start the virtual machine if necessary
		if *contextConfig.VM.Driver == "colima" {
			command := "colima"
			args := []string{"start", fmt.Sprintf("windsor-%s", contextName)}
			output, err := shellInstance.Exec(command, args...)
			if err != nil {
				return fmt.Errorf("Error executing command %s %v: %w", command, args, err)
			}

			// Print the command output
			fmt.Println(output)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(upCmd)
}
