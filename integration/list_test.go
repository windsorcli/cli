//go:build integration
// +build integration

package integration

import (
	"testing"

	"github.com/windsorcli/cli/integration/helpers"
)

// =============================================================================
// Integration Tests
// =============================================================================

func TestList_MatchesGetContexts(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")

	want, stderr, err := helpers.RunCLI(dir, []string{"get", "contexts"}, env)
	if err != nil {
		t.Fatalf("get contexts: %v\nstderr: %s", err, stderr)
	}
	got, stderr, err := helpers.RunCLI(dir, []string{"list"}, env)
	if err != nil {
		t.Fatalf("list: %v\nstderr: %s", err, stderr)
	}

	if len(want) == 0 {
		t.Fatal("expected get contexts to print output")
	}
	if string(got) != string(want) {
		t.Errorf("list output differs from get contexts.\nlist:\n%s\nget contexts:\n%s", got, want)
	}
}
