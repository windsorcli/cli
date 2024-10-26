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

		// Check the VM.Driver value and start the virtual machine if necessary
		if *contextConfig.VM.Driver == "colima" {
			command := fmt.Sprintf("colima start windsor-%s", contextName)
			output, err := shellInstance.Exec(command)
			if err != nil {
				if verbose {
					return fmt.Errorf("Error executing command %s: %w", command, err)
				}
				return nil
			}
			fmt.Println(output)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(upCmd)
}
