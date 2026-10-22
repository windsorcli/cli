// The commands generator writes one markdown reference page for each windsor command.
// It provides the `gendocs commands` subcommand, which reads the command tree from cmd.RootCmd().
// It deletes the output directory before each run, so a renamed or removed command leaves no old page.

package main

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	cliCmd "github.com/windsorcli/cli/cmd"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constructor
// =============================================================================

// commandsCmd returns the `gendocs commands` subcommand. The --out flag sets the output directory.
func commandsCmd() *cobra.Command {
	var outDir string
	cmd := &cobra.Command{
		Use:   "commands",
		Short: "Generate markdown reference for windsor commands",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return generateCommands(outDir)
		},
	}
	cmd.Flags().StringVar(&outDir, "out", "docs/reference/commands", "output directory")
	return cmd
}

// =============================================================================
// Private Methods
// =============================================================================

// generateCommands deletes and creates outDir, then writes a page for each visible command.
func generateCommands(outDir string) error {
	if err := os.RemoveAll(outDir); err != nil {
		return werror.Wrap(err, "clear output dir")
	}
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return werror.Wrap(err, "create output dir")
	}

	return emitTree(cliCmd.RootCmd(), outDir)
}

// emitTree writes one file for each visible command in the tree of cmd.
// It skips the root command, because the docs site makes its own index from the file list.
// It also skips the hidden commands and the help command.
func emitTree(cmd *cobra.Command, outDir string) error {
	if !cmd.Hidden && cmd.HasParent() {
		if err := writeCommandFile(cmd, outDir); err != nil {
			return err
		}
	}
	for _, sub := range cmd.Commands() {
		if sub.Hidden || sub.Name() == "help" {
			continue
		}
		if err := emitTree(sub, outDir); err != nil {
			return err
		}
	}
	return nil
}

// writeCommandFile renders the page for cmd into a file in outDir.
func writeCommandFile(cmd *cobra.Command, outDir string) error {
	path := filepath.Join(outDir, commandFilename(cmd))
	// #nosec G304 - outDir is operator-supplied via --out; filename derives from package-controlled cobra command names
	f, err := os.Create(path)
	if err != nil {
		return werror.Wrap(err, "create %s", path)
	}
	defer f.Close()
	if err := renderCommand(f, cmd); err != nil {
		return werror.Wrap(err, "render %s", path)
	}
	return nil
}
