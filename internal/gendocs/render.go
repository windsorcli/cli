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

// errWriter wraps an io.Writer and captures the first write error encountered,
// turning subsequent writes into no-ops that report success. This lets the
// per-section helpers continue using the bare fmt.Fprintf style instead of
// branching on errors after every call; renderCommand surfaces the captured
// error at the end so a full disk or broken pipe during doc generation
// produces a non-nil error rather than a silently truncated .md file.
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) Write(p []byte) (int, error) {
	if e.err != nil {
		// Report success after the first error so fmt.Fprintf doesn't loop
		// retrying the rest of the format string. renderCommand checks e.err
		// at the end of the section sequence.
		return len(p), nil
	}
	n, err := e.w.Write(p)
	if err != nil {
		e.err = err
	}
	return n, err
}

func renderCommand(w io.Writer, cmd *cobra.Command) error {
	ew := &errWriter{w: w}
	writeFrontmatter(ew, cmd)
	fmt.Fprintf(ew, "# %s\n\n", cmd.CommandPath())
	writeSynopsis(ew, cmd)
	writeLong(ew, cmd)
	writeFlagsTable(ew, cmd)
	writeSubcommands(ew, cmd)
	writeExamples(ew, cmd)
	writeSeeAlso(ew, cmd)
	return ew.err
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

// writeFlagsTable renders only the command's own flags via cmd.LocalFlags() —
// the local non-persistent flagset plus any persistent flags declared on this
// command. Inherited persistent flags from ancestors are excluded so globals
// like --verbose / --no-cache aren't duplicated on every page; a parent
// command's persistent flags are documented on the parent page and noted as
// inherited in the subcommand's Long. Cobra's auto-added --help is also
// skipped as boilerplate.
//
// We use LocalFlags() rather than Flags() because Flags() relies on cobra's
// Execute() to merge persistent flags, which never runs during doc generation.
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
