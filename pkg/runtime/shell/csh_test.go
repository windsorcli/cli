package shell_test

import (
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestRenderCsh(t *testing.T) {
	t.Run("RendersSortedSetenvAndAliasLines", func(t *testing.T) {
		// Given env vars and aliases, with empty values to remove
		envVars := map[string]string{"B": "two", "A": "one", "C": ""}
		aliases := map[string]string{"k": "kubectl get", "old": ""}

		// When rendering them for tcsh
		got := shell.RenderCsh(envVars, aliases)

		// Then each command is on its own line, sorted, and ends in a semicolon
		want := "setenv A 'one';\n" +
			"setenv B 'two';\n" +
			"unsetenv C;\n" +
			"alias k 'kubectl get';\n" +
			"unalias old;\n"
		if got != want {
			t.Errorf("Expected:\n%s\ngot:\n%s", want, got)
		}
	})

	t.Run("EscapesQuotesHistoryAndNewlines", func(t *testing.T) {
		// Given a value with a single quote, a history character, and a newline
		envVars := map[string]string{"V": "it's !x\nend"}

		// When rendering it for tcsh
		got := shell.RenderCsh(envVars, nil)

		// Then the quote and the history character are escaped and the newline is a space
		want := `setenv V 'it'\''s \!x end';` + "\n"
		if got != want {
			t.Errorf("Expected %q, got %q", want, got)
		}
	})

	t.Run("ReturnsEmptyForNoValues", func(t *testing.T) {
		// Given no env vars and no aliases
		// When rendering them for tcsh
		got := shell.RenderCsh(nil, nil)

		// Then the output is empty
		if got != "" {
			t.Errorf("Expected empty output, got %q", got)
		}
	})
}

func TestWithRemoved(t *testing.T) {
	t.Run("AddsEmptyValuesForMissingNames", func(t *testing.T) {
		// Given values and names, one of which values already has
		values := map[string]string{"A": "one"}

		// When the names are added as removed
		got := shell.WithRemoved(values, []string{"A", "B"})

		// Then only the missing name gets an empty value, and values is unchanged
		if got["A"] != "one" || got["B"] != "" || len(got) != 2 {
			t.Errorf("Expected A=one and B empty, got %v", got)
		}
		if _, ok := values["B"]; ok {
			t.Error("Expected the input map to stay unchanged")
		}
	})

	t.Run("HandlesNilValues", func(t *testing.T) {
		// Given no values
		// When a name is added as removed
		got := shell.WithRemoved(nil, []string{"A"})

		// Then the result has the name with an empty value
		if v, ok := got["A"]; !ok || v != "" {
			t.Errorf("Expected A with an empty value, got %v", got)
		}
	})
}
