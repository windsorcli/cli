// commands.go emits docs/reference/commands/*.md from the cobra command tree
// exposed by cmd.RootCmd(). Each cobra command becomes one markdown file
// named <full-command-with-underscores>.md, matching cobra's standard
// GenMarkdownTreeCustom layout. A filePrepender injects Astro-compatible YAML
// frontmatter (title, description) so the windsorcli.github.io content
// collection ingests the output without further processing.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
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

// generateCommands wipes outDir, then walks cmd.RootCmd() and emits one
// markdown file per command. The destination is recreated empty each run so
// renamed or removed commands do not leave stale pages behind — a future CI
// gate that regenerates and asserts `git diff --exit-code` relies on this.
func generateCommands(outDir string) error {
	if err := os.RemoveAll(outDir); err != nil {
		return fmt.Errorf("clear output dir: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	root := cliCmd.RootCmd()
	root.DisableAutoGenTag = true

	// frontmatter closes over root so the emitter and the per-file frontmatter
	// always operate on the same pre-configured command tree. cobra's
	// GenMarkdownTreeCustom passes the destination filename (basename only); we
	// derive the command's full invocation by replacing underscores with spaces
	// and stripping the .md suffix.
	frontmatter := func(filename string) string {
		base := strings.TrimSuffix(filepath.Base(filename), ".md")
		invocation := strings.ReplaceAll(base, "_", " ")
		short := lookupShort(root, base)

		var b strings.Builder
		fmt.Fprintln(&b, "---")
		fmt.Fprintf(&b, "title: %q\n", invocation)
		if short != "" {
			fmt.Fprintf(&b, "description: %q\n", short)
		}
		fmt.Fprintln(&b, "---")
		fmt.Fprintln(&b)
		return b.String()
	}

	return doc.GenMarkdownTreeCustom(root, outDir, frontmatter, linkHandler)
}

// linkHandler rewrites the inter-command links cobra emits at the bottom of
// each page. cobra's default produces `[windsor apply](windsor_apply.md)`;
// we keep that filename (matches what GenMarkdownTreeCustom writes) but
// could rewrite to the published site URL here if needed later.
func linkHandler(name string) string {
	return name
}

// lookupShort walks the command tree to find the cobra.Command whose full
// underscore-joined name matches base, returning its Short text. Used by
// frontmatter so the description field stays in sync with cobra's Short
// without a second source of truth.
func lookupShort(root *cobra.Command, base string) string {
	if base == root.Name() {
		return root.Short
	}
	parts := strings.Split(base, "_")
	if len(parts) == 0 || parts[0] != root.Name() {
		return ""
	}
	c := root
	for _, p := range parts[1:] {
		next, _, err := c.Find([]string{p})
		if err != nil || next == c {
			return ""
		}
		c = next
	}
	return c.Short
}
