package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/tui"
)

// verbose is a flag for verbose output
var verbose bool

// Define a custom type for context keys
type contextKey string

const projectOverridesKey = contextKey("projectOverrides")
const composerOverridesKey = contextKey("composerOverrides")
const runtimeOverridesKey = contextKey("runtimeOverrides")
const testRunnerOverridesKey = contextKey("testRunnerOverrides")

var shims = NewShims()

// Execute is the main entry point for the Windsor CLI application.
// It executes the root command with the provided context or a new background context.
// Sets the root command's context before execution so cmd.Root().Context() is correct
// when RunE runs (Cobra does not always propagate context to root on subsequent runs).
func Execute() error {
	ctx := rootCmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	rootCmd.SetContext(ctx)
	return rootCmd.ExecuteContext(ctx)
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

// configureProject creates a project for the given command and runs setup through Configure.
// It reads any test overrides from the command context, sets shell verbosity, checks for a
// trusted directory, and configures the project. Commands that need additional steps after
// Configure (e.g. ComposeBlueprint, GetContextValues) call this directly.
func configureProject(cmd *cobra.Command) (*project.Project, error) {
	var opts []*project.Project
	if overridesVal := cmd.Context().Value(projectOverridesKey); overridesVal != nil {
		opts = []*project.Project{overridesVal.(*project.Project)}
	}
	proj := project.NewProject("", opts...)
	proj.Runtime.Shell.SetVerbosity(verbose)
	if err := proj.Runtime.Shell.CheckTrustedDirectory(); err != nil {
		return nil, fmt.Errorf("not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve")
	}
	if err := proj.Configure(nil); err != nil {
		return nil, err
	}
	return proj, nil
}

// prepareProject creates and fully initializes a project for the given command. It delegates
// setup through Configure to configureProject, then runs Initialize with blueprint
// structural validation enabled. Use this for write/deploy commands (apply, plan, install)
// where misorderings should surface before terraform runs.
//
// Commands that need additional steps between Configure and Initialize (e.g.
// ValidateContextValues) should not use this helper — call configureProject directly
// instead. Teardown/read commands that must tolerate a deployed-but-misordered blueprint
// should use prepareProjectSkipValidation.
func prepareProject(cmd *cobra.Command) (*project.Project, error) {
	proj, err := configureProject(cmd)
	if err != nil {
		return nil, err
	}
	if err := proj.Initialize(false); err != nil {
		return nil, err
	}
	return proj, nil
}

// prepareProjectSkipValidation is prepareProject with blueprint structural validation
// disabled. Used by teardown/read commands (destroy, down, env) that must operate against
// a deployed blueprint whose structure may not satisfy the validator's invariants —
// otherwise an operator with a misordered backend component cannot run windsor destroy to
// clean up. The skip is explicit at the call site so a future write/deploy command using
// this helper would have to consciously opt out of validation rather than inheriting the
// behavior from a generic prepareProject.
func prepareProjectSkipValidation(cmd *cobra.Command) (*project.Project, error) {
	proj, err := configureProject(cmd)
	if err != nil {
		return nil, err
	}
	proj.Composer.BlueprintHandler.SetSkipValidation(true)
	if err := proj.Initialize(false); err != nil {
		return nil, err
	}
	return proj, nil
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
	tui.Init(verbose)
	return nil
}
