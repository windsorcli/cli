package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

// verbose is a flag for verbose output
var verbose bool

// Define a custom type for context keys
type contextKey string

const projectOverridesKey = contextKey("projectOverrides")
const composerOverridesKey = contextKey("composerOverrides")
const runtimeOverridesKey = contextKey("runtimeOverrides")

var shims = NewShims()

// Execute is the main entry point for the Windsor CLI application.
// It executes the root command with the provided context or a new background context.
func Execute() error {
	ctx := rootCmd.Context()
	if ctx != nil {
		return rootCmd.ExecuteContext(ctx)
	}
	return rootCmd.ExecuteContext(context.Background())
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:               "windsor",
	Short:             "A command line interface to assist your cloud native development workflow",
	Long:              "A command line interface to assist your cloud native development workflow",
	PersistentPreRunE: commandPreflight,
}

func init() {
	// Define the --verbose flag
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
}

// commandPreflight orchestrates global CLI preflight checks and context initialization for all commands.
// Intended for use as cobra.Command.PersistentPreRunE, it ensures the command context is configured
// prior to command execution. Trust checking is now handled by individual commands through the runtime.
func commandPreflight(cmd *cobra.Command, args []string) error {
	if err := setupGlobalContext(cmd); err != nil {
		return err
	}
	return nil
}

// setupGlobalContext injects global flags and context values into the command's context.
// It sets the verbose flag in the context if enabled.
func setupGlobalContext(cmd *cobra.Command) error {
	ctx := cmd.Root().Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if verbose {
		ctx = context.WithValue(ctx, "verbose", true)
	}
	cmd.SetContext(ctx)
	return nil
}
