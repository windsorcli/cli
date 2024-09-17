package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var goos = runtime.GOOS

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Output commands to set environment variables",
	Long:  "Output commands to set environment variables for the application.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get the current context value
		contextName, err := configHandler.GetConfigValue("context")
		if err != nil {
			return fmt.Errorf("Error getting config value: %w", err)
		}

		// Determine the command based on the OS
		var command string
		if goos == "windows" {
			command = fmt.Sprintf("set WINDSORCONTEXT=%s", contextName)
		} else {
			command = fmt.Sprintf("export WINDSORCONTEXT=%s", contextName)
		}
		fmt.Println(command)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(envCmd)
}
