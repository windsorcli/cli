package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
)

// verbose is a flag for verbose output
var verbose bool

// Define a custom type for context keys
type contextKey string

const controllerKey = contextKey("controller")
const injectorKey = contextKey("injector")

var shims = NewShims()

// The Execute function is the main entry point for the Windsor CLI application.
// It provides initialization of core dependencies and command execution,
// The Execute function serves as the bootstrap mechanism for the CLI,
// establishing the dependency injection container and controller context.
func Execute(controllers ...controller.Controller) error {
	var ctrl controller.Controller
	var injector di.Injector
	if len(controllers) > 0 {
		ctrl = controllers[0]
		// Extract injector from controller if possible
		// For now, we'll create a new one since the controller interface doesn't expose the injector
		injector = di.NewInjector()
	} else {
		injector = di.NewInjector()
		ctrl = controller.NewController(injector)
	}
	ctx := context.WithValue(context.Background(), controllerKey, ctrl)
	ctx = context.WithValue(ctx, injectorKey, injector)
	return rootCmd.ExecuteContext(ctx)
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "windsor",
	Short: "A command line interface to assist your cloud native development workflow",
	Long:  "A command line interface to assist your cloud native development workflow",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Set context from root command
		cmd.SetContext(cmd.Root().Context())

		return nil
	},
}

func init() {
	// Define the --verbose flag
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
}
