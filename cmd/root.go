package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
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
