// commands.go is the cobra entry point for the `gendocs commands` subcommand.
// It walks cmd.RootCmd() and emits one markdown file per command via the
// renderer in render.go. The destination directory is wiped before each run so
// renamed or removed commands do not leave stale pages behind — a future CI
// gate that regenerates and asserts `git diff --exit-code` relies on this.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	cliCmd "github.com/windsorcli/cli/cmd"
)

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

func generateCommands(outDir string) error {
	if err := os.RemoveAll(outDir); err != nil {
		return fmt.Errorf("clear output dir: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	return emitTree(cliCmd.RootCmd(), outDir)
}

// emitTree walks the command tree, writing one file per visible command. The
// root cobra command (`windsor` itself) is skipped — the site renders its own
// index from the file listing rather than relying on a generated index page.
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

func writeCommandFile(cmd *cobra.Command, outDir string) error {
	path := filepath.Join(outDir, commandFilename(cmd))
	// #nosec G304 - outDir is operator-supplied via --out; filename derives from package-controlled cobra command names
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	if err := renderCommand(f, cmd); err != nil {
		return fmt.Errorf("render %s: %w", path, err)
	}
	return nil
}
