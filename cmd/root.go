package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// verbose is a flag for verbose output
var verbose bool

// Define a custom type for context keys
type contextKey string

const injectorKey = contextKey("injector")

var shims = NewShims()

// The Execute function is the main entry point for the Windsor CLI application.
// It provides initialization of core dependencies and command execution,
// establishing the dependency injection container context.
func Execute() error {
	injector := di.NewInjector()
	ctx := context.WithValue(context.Background(), injectorKey, injector)
	return rootCmd.ExecuteContext(ctx)
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "windsor",
	Short: "A command line interface to assist your cloud native development workflow",
	Long:  "A command line interface to assist your cloud native development workflow",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Set context from root command
		ctx := cmd.Root().Context()

		// Add verbose flag to context if set
		if verbose {
			ctx = context.WithValue(ctx, "verbose", true)
		}

		cmd.SetContext(ctx)

		return nil
	},
}

func init() {
	// Define the --verbose flag
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
}

// checkTrust performs trust validation for Windsor CLI commands requiring a trusted project directory.
// It verifies directory trust status by checking if the current project directory is in the trusted file list.
// For the "init" command, or for the "env" command with the --hook flag set, trust validation is skipped.
// Returns an error if the directory is untrusted.
func checkTrust(cmd *cobra.Command, args []string) error {
	if cmd.Name() == "init" {
		return nil
	}

	if cmd.Name() == "env" {
		if hook, _ := cmd.Flags().GetBool("hook"); hook {
			return nil
		}
	}

	if err := shell.CheckTrustedDirectory(); err != nil {
		return fmt.Errorf("not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve")
	}

	return nil
}
