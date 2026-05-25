//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"

	"github.com/windsorcli/cli/integration/helpers"
)

func TestWindsorTest_DefaultFixture(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	env = append(env, "WINDSOR_CONTEXT=test")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"test"}, env)
	if err != nil {
		t.Fatalf("windsor test: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	out := string(stdout) + string(stderr)
	if !strings.Contains(out, "PASS") && !strings.Contains(out, "✓") {
		t.Errorf("expected PASS or ✓ in output: %s", out)
	}
}

// TestWindsorTest_FacetRequiresFixture exercises the declarative test harness against the
// facet-requires fixture's requires.test.yaml. The three cases cover the states a facet
// author cares about: requirements satisfied, requirements missing while the facet is
// active, and the facet not active at all.
func TestWindsorTest_FacetRequiresFixture(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-requires")
	env = append(env, "WINDSOR_CONTEXT=ok")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"test"}, env)
	if err != nil {
		t.Fatalf("windsor test: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	out := string(stdout) + string(stderr)
	if !strings.Contains(out, "PASS") && !strings.Contains(out, "✓") {
		t.Errorf("expected PASS or ✓ in output: %s", out)
	}
}
