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

// TestWindsorTest_TerraformOutputUnregisteredFixture exercises the gap the bug report
// identified: a facet whose terraform component eagerly calls terraform_output() against
// a sibling that is gated out by `when:` must fail windsor test, mirroring the
// "component not found" error windsor bootstrap raises at env-var build time.
//
// The fixture defines two cases: one where the gated component is registered (resolves)
// and one where it is not (must fail). The whole `windsor test` run passes only when
// both cases behave as declared — the failing case is declared with expectError: true.
func TestWindsorTest_TerraformOutputUnregisteredFixture(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "test-tf-output-unregistered")
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

// TestWindsorTest_FluxSystemTiersFixture exercises expect.flux/exclude.flux against the
// facet-tiers fixture's tiers.test.yaml: a system merged across facets (install components
// accumulate, same-ordinal resources variants merge), and a when:-gated system whose install
// tier and resources variant can be asserted present or absent independently, in the author's
// own flux: vocabulary rather than compiled tier names.
func TestWindsorTest_FluxSystemTiersFixture(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-tiers")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"test"}, env)
	if err != nil {
		t.Fatalf("windsor test: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	out := string(stdout) + string(stderr)
	if !strings.Contains(out, "PASS") && !strings.Contains(out, "✓") {
		t.Errorf("expected PASS or ✓ in output: %s", out)
	}
}
