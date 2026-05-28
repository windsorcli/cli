package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// brokenWriter fails every Write with the wrapped error. Used to exercise
// the errWriter short-circuit path.
type brokenWriter struct{ err error }

func (b brokenWriter) Write(p []byte) (int, error) { return 0, b.err }

// =============================================================================
// Test Public Methods
// =============================================================================

func TestErrWriter(t *testing.T) {
	t.Run("PassesWritesThroughWhenHealthy", func(t *testing.T) {
		// Given an errWriter wrapping a healthy writer
		var buf strings.Builder
		ew := &errWriter{w: &buf}

		// When several writes go through
		n, err := fmt.Fprint(ew, "hello, ")
		if err != nil || n != len("hello, ") {
			t.Fatalf("first write: n=%d err=%v", n, err)
		}
		_, _ = fmt.Fprint(ew, "world")

		// Then output reaches the underlying writer and no error is captured
		if got := buf.String(); got != "hello, world" {
			t.Errorf("buf = %q, want %q", got, "hello, world")
		}
		if ew.err != nil {
			t.Errorf("expected ew.err to be nil, got %v", ew.err)
		}
	})

	t.Run("CapturesFirstError", func(t *testing.T) {
		// Given an errWriter wrapping a broken writer that always fails
		sentinel := errors.New("disk full")
		ew := &errWriter{w: brokenWriter{err: sentinel}}

		// When the first write fails
		_, err := fmt.Fprint(ew, "hello")
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected first write to surface sentinel, got %v", err)
		}

		// Then the captured error matches the sentinel
		if !errors.Is(ew.err, sentinel) {
			t.Errorf("ew.err = %v, want %v", ew.err, sentinel)
		}
	})

	t.Run("ShortCircuitsAfterFirstErrorWithoutLooping", func(t *testing.T) {
		// Given an errWriter that has already captured an error
		ew := &errWriter{w: brokenWriter{err: errors.New("boom")}}
		_, _ = fmt.Fprint(ew, "first")

		// When subsequent writes happen
		// (fmt.Fprintf would loop on partial writes; this confirms the n=len(p)
		// short-circuit return prevents that)
		n, err := fmt.Fprintf(ew, "second %s third", "value")

		// Then the second write reports success so the caller doesn't retry
		if err != nil {
			t.Errorf("expected nil error from short-circuited write, got %v", err)
		}
		if n == 0 {
			t.Errorf("expected non-zero byte count from short-circuited write, got 0")
		}

		// And the captured error is still the first one
		if ew.err == nil || ew.err.Error() != "boom" {
			t.Errorf("expected ew.err to remain the first error, got %v", ew.err)
		}
	})
}

func TestRenderCommandSurfacesWriteErrors(t *testing.T) {
	t.Run("ReturnsErrorWhenWriterFails", func(t *testing.T) {
		// Given a writer that always fails and any cobra command
		sentinel := errors.New("write failed")
		cmd := &cobra.Command{Use: "windsor", Short: "test"}

		// When renderCommand runs against the broken writer
		err := renderCommand(brokenWriter{err: sentinel}, cmd)

		// Then the underlying write error surfaces — would silently truncate
		// the .md file otherwise
		if !errors.Is(err, sentinel) {
			t.Errorf("renderCommand error = %v, want %v", err, sentinel)
		}
	})

	t.Run("ReturnsNilWhenWriterSucceeds", func(t *testing.T) {
		// Given a healthy writer
		cmd := &cobra.Command{Use: "windsor", Short: "test"}

		// When renderCommand runs
		err := renderCommand(io.Discard, cmd)

		// Then no error is returned
		if err != nil {
			t.Errorf("renderCommand error = %v, want nil", err)
		}
	})
}

func TestCommandFilename(t *testing.T) {
	t.Run("RootCommand", func(t *testing.T) {
		// Given a root cobra command
		root := &cobra.Command{Use: "windsor"}

		// When commandFilename is called
		got := commandFilename(root)

		// Then the filename is just the root name with a .md suffix
		want := "windsor.md"
		if got != want {
			t.Errorf("commandFilename(root) = %q, want %q", got, want)
		}
	})

	t.Run("TopLevelSubcommand", func(t *testing.T) {
		// Given a top-level subcommand
		root := &cobra.Command{Use: "windsor"}
		child := &cobra.Command{Use: "env"}
		root.AddCommand(child)

		// When commandFilename is called
		got := commandFilename(child)

		// Then the root prefix is dropped
		want := "env.md"
		if got != want {
			t.Errorf("commandFilename(env) = %q, want %q", got, want)
		}
	})

	t.Run("NestedSubcommand", func(t *testing.T) {
		// Given a nested subcommand windsor apply terraform
		root := &cobra.Command{Use: "windsor"}
		apply := &cobra.Command{Use: "apply"}
		terraform := &cobra.Command{Use: "terraform <component>"}
		apply.AddCommand(terraform)
		root.AddCommand(apply)

		// When commandFilename is called
		got := commandFilename(terraform)

		// Then ancestors after the root are joined with dashes
		want := "apply-terraform.md"
		if got != want {
			t.Errorf("commandFilename(apply terraform) = %q, want %q", got, want)
		}
	})
}

func TestFlagRows(t *testing.T) {
	t.Run("RendersOwnFlagWithDefault", func(t *testing.T) {
		// Given a flagset with one flag and a default
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.Bool("wait", false, "Wait for things.")

		// When flagRows is called
		rows := flagRows(fs)

		// Then a single row is rendered with the default value in code-fenced form
		if len(rows) != 1 {
			t.Fatalf("expected 1 row, got %d: %v", len(rows), rows)
		}
		if !strings.Contains(rows[0], "`--wait`") || !strings.Contains(rows[0], "`false`") {
			t.Errorf("row missing flag name or default: %q", rows[0])
		}
	})

	t.Run("RendersShorthand", func(t *testing.T) {
		// Given a flag with a shorthand
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.StringP("output", "o", ".", "Output path.")

		// When flagRows is called
		rows := flagRows(fs)

		// Then the row contains both the shorthand and the long form
		if len(rows) != 1 {
			t.Fatalf("expected 1 row, got %d: %v", len(rows), rows)
		}
		if !strings.Contains(rows[0], "`-o`") || !strings.Contains(rows[0], "`--output`") {
			t.Errorf("row missing shorthand or long form: %q", rows[0])
		}
	})

	t.Run("ExcludesHelpAndHidden", func(t *testing.T) {
		// Given a flagset with a visible flag, a hidden flag, and --help
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.Bool("visible", false, "Visible flag.")
		fs.Bool("hidden", false, "Hidden flag.")
		_ = fs.MarkHidden("hidden")
		fs.BoolP("help", "h", false, "Help.")

		// When flagRows is called
		rows := flagRows(fs)

		// Then only the visible flag is rendered
		if len(rows) != 1 {
			t.Fatalf("expected 1 row, got %d: %v", len(rows), rows)
		}
		if !strings.Contains(rows[0], "`--visible`") {
			t.Errorf("expected visible flag in row, got %q", rows[0])
		}
	})

	t.Run("EscapesPipesInDescription", func(t *testing.T) {
		// Given a flag whose description contains a pipe (enum-style)
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.String("platform", "", "Platform: aws|azure|gcp.")

		// When flagRows is called
		rows := flagRows(fs)

		// Then pipes are escaped so the markdown table layout survives
		if len(rows) != 1 {
			t.Fatalf("expected 1 row, got %d: %v", len(rows), rows)
		}
		if strings.Contains(rows[0], "aws|azure") {
			t.Errorf("expected pipes to be escaped, got %q", rows[0])
		}
		if !strings.Contains(rows[0], `aws\|azure`) {
			t.Errorf("expected escaped pipes in row, got %q", rows[0])
		}
	})

	t.Run("EmptyDefaultRendersAsQuotedEmpty", func(t *testing.T) {
		// Given a string flag with no default
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.String("name", "", "Name.")

		// When flagRows is called
		rows := flagRows(fs)

		// Then the empty default renders as "" (not as a blank cell)
		if len(rows) != 1 {
			t.Fatalf("expected 1 row, got %d: %v", len(rows), rows)
		}
		if !strings.Contains(rows[0], `""`) {
			t.Errorf("expected empty default to render as \"\", got %q", rows[0])
		}
	})
}

func TestVisibleSubcommands(t *testing.T) {
	t.Run("ExcludesHiddenAndHelp", func(t *testing.T) {
		// Given a tree with a visible child, a hidden child, and a help subcommand
		root := &cobra.Command{Use: "root"}
		root.AddCommand(&cobra.Command{Use: "visible"})
		root.AddCommand(&cobra.Command{Use: "secret", Hidden: true})
		root.AddCommand(&cobra.Command{Use: "help"})

		// When visibleSubcommands is called
		got := visibleSubcommands(root)

		// Then only the visible child is returned
		if len(got) != 1 || got[0].Name() != "visible" {
			names := make([]string, 0, len(got))
			for _, c := range got {
				names = append(names, c.Name())
			}
			t.Errorf("visibleSubcommands = %v, want [visible]", names)
		}
	})
}

func TestGenerateCommands(t *testing.T) {
	t.Run("WritesFilesForVisibleCommands", func(t *testing.T) {
		// Given a temporary output directory
		out := t.TempDir()

		// When generateCommands is called against the real windsor command tree
		if err := generateCommands(out); err != nil {
			t.Fatalf("generateCommands: %v", err)
		}

		// Then a stable, low-churn command (version) has a file emitted with valid
		// frontmatter and the expected h1. Using version because its surface is
		// minimal — any other command would also work but version is least likely
		// to change shape over time.
		raw, err := os.ReadFile(filepath.Join(out, "version.md"))
		if err != nil {
			t.Fatalf("read version.md: %v", err)
		}
		body := string(raw)
		if !strings.HasPrefix(body, "---\n") {
			t.Errorf("version.md missing frontmatter prefix, got: %q", body[:min(40, len(body))])
		}
		if !strings.Contains(body, `title: "windsor version"`) {
			t.Error("version.md missing expected title in frontmatter")
		}
		if !strings.Contains(body, "# windsor version\n") {
			t.Error("version.md missing expected h1 heading")
		}
	})

	t.Run("WipesStaleFilesBeforeRegenerating", func(t *testing.T) {
		// Given an output directory with a stale file from a previous run
		out := t.TempDir()
		stalePath := filepath.Join(out, "stale-deleted-command.md")
		if err := os.WriteFile(stalePath, []byte("stale\n"), 0o600); err != nil {
			t.Fatalf("seed stale file: %v", err)
		}

		// When generateCommands runs
		if err := generateCommands(out); err != nil {
			t.Fatalf("generateCommands: %v", err)
		}

		// Then the stale file is gone — required so the CI gate's diff is meaningful
		if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
			t.Errorf("expected stale file to be removed; got err=%v", err)
		}
	})
}
