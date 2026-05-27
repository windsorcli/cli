// render.go produces the markdown for one cobra command. The output shape
// matches the windsorcli.github.io house style for CLI reference: frontmatter
// for the Astro content collection, h1 for the command name, a synopsis fence,
// the cmd.Long body as prose, a flag table (own flags only — inherited globals
// are excluded as noise), an optional examples block, an optional subcommands
// list, and a "See also" section sourced from cmd.Annotations.
//
// Annotations consumed:
//
//	docs.seealso  newline-separated markdown bullets (each line becomes one "- " entry)
//	docs.source   path to the source file (rendered as "Source: [path](github-link)")

package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const sourceURLPrefix = "https://github.com/windsorcli/cli/blob/main/"

func renderCommand(w io.Writer, cmd *cobra.Command) error {
	writeFrontmatter(w, cmd)
	fmt.Fprintf(w, "# %s\n\n", cmd.CommandPath())
	writeSynopsis(w, cmd)
	writeLong(w, cmd)
	writeFlagsTable(w, cmd)
	writeSubcommands(w, cmd)
	writeExamples(w, cmd)
	writeSeeAlso(w, cmd)
	return nil
}

func writeFrontmatter(w io.Writer, cmd *cobra.Command) {
	fmt.Fprintln(w, "---")
	fmt.Fprintf(w, "title: %q\n", cmd.CommandPath())
	if cmd.Short != "" {
		fmt.Fprintf(w, "description: %q\n", cmd.Short)
	}
	fmt.Fprintln(w, "---")
}

func writeSynopsis(w io.Writer, cmd *cobra.Command) {
	fmt.Fprintln(w, "```sh")
	fmt.Fprintln(w, cmd.UseLine())
	fmt.Fprintln(w, "```")
	fmt.Fprintln(w)
}

func writeLong(w io.Writer, cmd *cobra.Command) {
	body := strings.TrimSpace(cmd.Long)
	if body == "" {
		return
	}
	fmt.Fprintln(w, body)
	fmt.Fprintln(w)
}

// writeFlagsTable renders only the command's own flags (cmd.Flags()), excluding
// inherited persistent flags from parents. This matches the hand-written
// reference convention — globals like --verbose / --no-cache are documented
// once at the root rather than duplicated on every page. Cobra's auto-added
// --help is also skipped as boilerplate.
func writeFlagsTable(w io.Writer, cmd *cobra.Command) {
	rows := flagRows(cmd.Flags())
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

// visibleSubcommands returns subcommands that should appear in documentation —
// cobra's auto-added "help" command and any Hidden commands are excluded.
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

// commandFilename returns the on-disk filename for a command's reference page.
// Root command ("windsor") becomes windsor.md; subcommands drop the root and
// join the remaining ancestors with dashes (e.g. "apply terraform" → "apply-terraform.md").
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

// escapePipes escapes the pipe character so it doesn't break the markdown
// table layout when it appears in flag descriptions (e.g. enum lists like
// [foo|bar|baz]).
func escapePipes(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}
