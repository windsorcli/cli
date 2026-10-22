// The command renderer writes the markdown reference page for one cobra command.
// It provides the page sections in the house style of windsorcli.github.io.
// A page has frontmatter, a synopsis, the Long text, a flag table, the subcommands, the examples and a See also section.
// The title is only in the frontmatter, so no H1 in the body repeats it.
//
// The renderer reads these annotations from the command:
//
//	docs.seealso  markdown bullets, one on each line, for the See also section
//	docs.source   the path of the source file, which the See also section links to

package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// =============================================================================
// Constants
// =============================================================================

// sourceURLPrefix is the GitHub URL prefix for the source link on a command page.
const sourceURLPrefix = "https://github.com/windsorcli/cli/blob/main/"

// =============================================================================
// Types
// =============================================================================

// errWriter is an io.Writer that keeps the first write error and ignores all writes after it.
// The section functions can then call fmt.Fprintf without an error check after each call.
// renderCommand returns the kept error, so a full disk gives an error and not a short file.
type errWriter struct {
	w   io.Writer
	err error
}

// =============================================================================
// Public Methods
// =============================================================================

// Write writes p to the wrapped writer and keeps the first error.
// After an error, it reports success and writes nothing, so fmt.Fprintf does not retry the rest of its output.
func (e *errWriter) Write(p []byte) (int, error) {
	if e.err != nil {
		return len(p), nil
	}
	n, err := e.w.Write(p)
	if err != nil {
		e.err = err
	}
	return n, err
}

// =============================================================================
// Private Methods
// =============================================================================

// renderCommand writes the full page for cmd to w and returns the first write error.
func renderCommand(w io.Writer, cmd *cobra.Command) error {
	ew := &errWriter{w: w}
	writeFrontmatter(ew, cmd)
	fmt.Fprintln(ew)
	writeSynopsis(ew, cmd)
	writeLong(ew, cmd)
	writeFlagsTable(ew, cmd)
	writeSubcommands(ew, cmd)
	writeExamples(ew, cmd)
	writeSeeAlso(ew, cmd)
	return ew.err
}

// writeFrontmatter writes the frontmatter, with the command path as the title and Short as the description.
func writeFrontmatter(w io.Writer, cmd *cobra.Command) {
	fmt.Fprintln(w, "---")
	fmt.Fprintf(w, "title: %q\n", cmd.CommandPath())
	if cmd.Short != "" {
		fmt.Fprintf(w, "description: %q\n", cmd.Short)
	}
	fmt.Fprintln(w, "---")
}

// writeSynopsis writes the usage line of cmd in a sh code block.
func writeSynopsis(w io.Writer, cmd *cobra.Command) {
	fmt.Fprintln(w, "```sh")
	fmt.Fprintln(w, cmd.UseLine())
	fmt.Fprintln(w, "```")
	fmt.Fprintln(w)
}

// writeLong writes the Long text of cmd. It writes nothing when Long is empty.
func writeLong(w io.Writer, cmd *cobra.Command) {
	body := strings.TrimSpace(cmd.Long)
	if body == "" {
		return
	}
	fmt.Fprintln(w, body)
	fmt.Fprintln(w)
}

// writeFlagsTable writes a table of the flags in cmd.LocalFlags(), without the hidden flags and --help.
// LocalFlags has the flags that cmd declares, and not the persistent flags of its parents.
// The parent page documents those flags, so each page does not repeat --verbose and the other global flags.
// It does not use Flags(), because only the cobra Execute function merges the persistent flags into it.
func writeFlagsTable(w io.Writer, cmd *cobra.Command) {
	rows := flagRows(cmd.LocalFlags())
	if len(rows) == 0 {
		return
	}
	fmt.Fprintln(w, "## Flags")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Flag | Default | Description |")
	fmt.Fprintln(w, "|------|---------|-------------|")
	for _, r := range rows {
		fmt.Fprintln(w, r)
	}
	fmt.Fprintln(w)
}

// flagRows returns one table row for each flag in set that is not hidden and is not --help.
func flagRows(set *pflag.FlagSet) []string {
	var rows []string
	set.VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" {
			return
		}
		name := "`--" + f.Name + "`"
		if f.Shorthand != "" {
			name = "`-" + f.Shorthand + "`, " + name
		}
		def := f.DefValue
		if def == "" {
			def = `""`
		}
		rows = append(rows, fmt.Sprintf("| %s | `%s` | %s |", name, def, escapePipes(f.Usage)))
	})
	return rows
}

// writeSubcommands writes a list of links to the visible subcommands of cmd, with the Short text of each one.
func writeSubcommands(w io.Writer, cmd *cobra.Command) {
	subs := visibleSubcommands(cmd)
	if len(subs) == 0 {
		return
	}
	fmt.Fprintln(w, "## Subcommands")
	fmt.Fprintln(w)
	for _, c := range subs {
		fmt.Fprintf(w, "- [`%s`](%s) — %s\n", c.CommandPath(), commandFilename(c), c.Short)
	}
	fmt.Fprintln(w)
}

// writeExamples writes the Example text of cmd in a sh code block. It writes nothing when Example is empty.
func writeExamples(w io.Writer, cmd *cobra.Command) {
	ex := strings.TrimSpace(cmd.Example)
	if ex == "" {
		return
	}
	fmt.Fprintln(w, "## Examples")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "```sh")
	fmt.Fprintln(w, ex)
	fmt.Fprintln(w, "```")
	fmt.Fprintln(w)
}

// writeSeeAlso writes the See also section from the docs.seealso and docs.source annotations.
// It writes nothing when both annotations are empty.
func writeSeeAlso(w io.Writer, cmd *cobra.Command) {
	seealso := strings.TrimSpace(cmd.Annotations["docs.seealso"])
	source := strings.TrimSpace(cmd.Annotations["docs.source"])
	if seealso == "" && source == "" {
		return
	}
	fmt.Fprintln(w, "## See also")
	fmt.Fprintln(w)
	if seealso != "" {
		for _, line := range strings.Split(seealso, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			fmt.Fprintf(w, "- %s\n", line)
		}
	}
	if source != "" {
		fmt.Fprintf(w, "- Source: [%s](%s%s)\n", source, sourceURLPrefix, source)
	}
}

// =============================================================================
// Helpers
// =============================================================================

// visibleSubcommands returns the subcommands of cmd without the hidden commands and the cobra help command.
func visibleSubcommands(cmd *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, c := range cmd.Commands() {
		if c.Hidden || c.Name() == "help" {
			continue
		}
		out = append(out, c)
	}
	return out
}

// commandFilename returns the file name of the page for cmd.
// The root command gives windsor.md. A subcommand drops the root and joins the other names with dashes.
// For example, "windsor apply terraform" gives apply-terraform.md.
func commandFilename(cmd *cobra.Command) string {
	parts := strings.Fields(cmd.CommandPath())
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0] + ".md"
	}
	return strings.Join(parts[1:], "-") + ".md"
}

// escapePipes escapes each pipe character in s, so that a flag description such as [a|b] does not break the table.
func escapePipes(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}
