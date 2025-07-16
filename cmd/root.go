package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

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
// Intended for use as cobra.Command.PersistentPreRunE, it ensures the command context is configured and
// the current directory is authorized for Windsor operations prior to command execution.
func commandPreflight(cmd *cobra.Command, args []string) error {
	if err := setupGlobalContext(cmd); err != nil {
		return err
	}
	if err := enforceTrustedDirectory(cmd); err != nil {
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

// enforceTrustedDirectory checks if the current working directory is trusted for Windsor operations.
// Enforces trust for a defined set of commands, including "env". For "env" with --hook, exits silently to avoid shell integration noise.
// Returns an error if the directory is not trusted.
func enforceTrustedDirectory(cmd *cobra.Command) error {
	const notTrustedDirMsg = "not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve"
	enforcedCommands := []string{"up", "down", "exec", "install", "env"}
	cmdName := cmd.Name()
	shouldEnforce := slices.Contains(enforcedCommands, cmdName)

	if !shouldEnforce {
		return nil
	}

	currentDir, err := shims.Getwd()
	if err != nil {
		return fmt.Errorf("Error getting current directory: %w", err)
	}

	homeDir, err := shims.UserHomeDir()
	if err != nil {
		return fmt.Errorf("Error getting user home directory: %w", err)
	}

	trustedDirPath := filepath.Join(homeDir, ".config", "windsor")
	trustedFilePath := filepath.Join(trustedDirPath, ".trusted")

	data, err := shims.ReadFile(trustedFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf(notTrustedDirMsg)
		}
		return fmt.Errorf(notTrustedDirMsg)
	}

	iter := strings.SplitSeq(strings.TrimSpace(string(data)), "\n")

	for trustedDir := range iter {
		trustedDir = strings.TrimSpace(trustedDir)
		if trustedDir != "" && strings.HasPrefix(currentDir, trustedDir) {
			return nil
		}
	}

	if cmdName == "env" {
		hook, _ := cmd.Flags().GetBool("hook")
		if hook {
			shims.Exit(0)
		}
	}

	return fmt.Errorf(notTrustedDirMsg)
}
