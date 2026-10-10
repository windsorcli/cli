//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/windsorcli/cli/integration/helpers"
)

// =============================================================================
// Integration Tests
// =============================================================================

// TestFormat_JSONSucceedsAndKeepsStdout verifies that --format json is accepted and that
// command output on stdout is unchanged.
func TestFormat_JSONSucceedsAndKeepsStdout(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")

	stdout, stderr, err := helpers.RunCLI(dir, []string{"env", "--format", "json"}, env)
	if err != nil {
		t.Fatalf("env --format json: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(string(stdout), "WINDSOR_CONTEXT") {
		t.Errorf("expected env output on stdout, got:\n%s", stdout)
	}
}

// TestFormat_JSONRendersErrorsAsJSONLines verifies that a failing command under --format json
// writes its error to stderr as one JSON failed event.
func TestFormat_JSONRendersErrorsAsJSONLines(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")

	_, stderr, err := helpers.RunCLI(dir, []string{"plan", "terraform", "missing", "--format", "json"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	lines := strings.Split(strings.TrimSpace(string(stderr)), "\n")
	var event struct {
		Kind  string            `json:"kind"`
		Error map[string]string `json:"error"`
	}
	if jsonErr := json.Unmarshal([]byte(lines[len(lines)-1]), &event); jsonErr != nil {
		t.Fatalf("expected a JSON line last on stderr, got:\n%s", stderr)
	}
	if event.Kind != "failed" || !strings.Contains(event.Error["message"], "missing") {
		t.Errorf("unexpected failed event %+v", event)
	}
}

// TestFormat_TextRendersErrorsWithThePlainPrefix verifies that the default text format keeps
// the "Error:" prefix on stderr.
func TestFormat_TextRendersErrorsWithThePlainPrefix(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")

	_, stderr, err := helpers.RunCLI(dir, []string{"plan", "terraform", "missing"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if strings.Count(string(stderr), "Error: ") != 1 {
		t.Errorf("expected exactly one Error: line, got:\n%s", stderr)
	}
}

// TestFormat_RejectsAnUnknownFormat verifies that an unknown --format value fails with the
// CLI-001 code and a remediation.
func TestFormat_RejectsAnUnknownFormat(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")

	_, stderr, err := helpers.RunCLI(dir, []string{"env", "--format", "yaml"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "Error [CLI-001]") || !strings.Contains(string(stderr), "--format text or --format json") {
		t.Errorf("expected CLI-001 with remediation, got:\n%s", stderr)
	}
}
