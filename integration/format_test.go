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

// TestFormat_TextRendersErrorsWithTheErrorPrefix verifies that the default text format prints
// one "error:" line on stderr.
func TestFormat_TextRendersErrorsWithTheErrorPrefix(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")

	_, stderr, err := helpers.RunCLI(dir, []string{"plan", "terraform", "missing"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if strings.Count(string(stderr), "error: ") != 1 {
		t.Errorf("expected exactly one error: line, got:\n%s", stderr)
	}
}

// TestFormat_RejectsAnUnknownFormat verifies that an unknown --format value fails with a hint.
func TestFormat_RejectsAnUnknownFormat(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")

	_, stderr, err := helpers.RunCLI(dir, []string{"env", "--format", "yaml"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "error: Unknown output format") || !strings.Contains(string(stderr), "hint: Run the command again with --format text or --format json.") {
		t.Errorf("expected the error and its hint, got:\n%s", stderr)
	}
}

// TestFormat_JSONReportsTerraformProgress verifies that applying a terraform component under
// --format json writes an applying and an applied event for that component to stderr.
func TestFormat_JSONReportsTerraformProgress(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	helpers.MarkAsGitRepo(t, dir)
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env)
	if err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")

	_, stderr, err = helpers.RunCLI(dir, []string{"apply", "terraform", "null", "--format", "json"}, env)
	if err != nil {
		t.Fatalf("apply terraform null --format json: %v\nstderr: %s", err, stderr)
	}
	var kinds []string
	for _, line := range strings.Split(strings.TrimSpace(string(stderr)), "\n") {
		var event struct {
			Kind    string `json:"kind"`
			Subject string `json:"subject"`
		}
		if json.Unmarshal([]byte(line), &event) == nil && event.Subject == "terraform:null" {
			kinds = append(kinds, event.Kind)
		}
	}
	if strings.Join(kinds, ",") != "applying,applied" {
		t.Errorf("expected applying,applied for terraform:null, got %v\nstderr: %s", kinds, stderr)
	}
}
