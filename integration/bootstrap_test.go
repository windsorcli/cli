//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/integration/helpers"
)

// =============================================================================
// Integration Tests
// =============================================================================

// TestBootstrap_RecognisesFlags verifies bootstrap's minimal flag surface: it
// advertises --platform, --blueprint, and --set, and explicitly does NOT
// advertise flags dropped from the bootstrap design (--wait, --vm-driver,
// --backend, --aws-profile). This guards against accidental flag reintroduction.
func TestBootstrap_RecognisesFlags(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "bootstrap")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"bootstrap", "--help"}, env)
	if err != nil {
		t.Fatalf("bootstrap --help: %v\nstderr: %s", err, stderr)
	}
	help := string(stdout)
	for _, flag := range []string{"--platform", "--blueprint", "--set"} {
		if !strings.Contains(help, flag) {
			t.Errorf("expected bootstrap --help to advertise %s, got:\n%s", flag, help)
		}
	}
	for _, flag := range []string{"--wait", "--vm-driver", "--backend", "--aws-profile", "--aws-endpoint-url"} {
		if strings.Contains(help, flag) {
			t.Errorf("expected bootstrap --help NOT to advertise %s (dropped from flag surface), got:\n%s", flag, help)
		}
	}
}

// TestBootstrap_FailsOnMalformedSetFlag verifies bootstrap rejects --set entries
// missing the = separator with a clear error, matching up's strict --set handling.
func TestBootstrap_FailsOnMalformedSetFlag(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "bootstrap")
	_, stderr, err := helpers.RunCLI(dir, []string{"bootstrap", "--set", "no-equals-here"}, env)
	if err == nil {
		t.Fatal("expected failure for malformed --set, got success")
	}
	if !strings.Contains(string(stderr), "invalid --set format") {
		t.Errorf("expected stderr to mention 'invalid --set format', got: %s", stderr)
	}
}

// TestBootstrap_PersistsSetValues exercises the configuration pipeline end-to-end
// up through SaveConfig. Bootstrap will ultimately fail at the kubernetes Install
// step because the integration harness has no cluster, but --set values must have
// been persisted to values.yaml before that failure. This guards the contract
// that bootstrap persists user overrides regardless of downstream failures.
func TestBootstrap_PersistsSetValues(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "bootstrap")
	// Run bootstrap; failure at the install step is expected and tolerated.
	_, _, _ = helpers.RunCLI(dir, []string{"bootstrap", "staging", "--set", "dns.enabled=false", "--set", "custom.key=hello"}, env)

	valuesPath := filepath.Join(dir, "contexts", "staging", "values.yaml")
	data, err := os.ReadFile(valuesPath)
	if err != nil {
		t.Fatalf("expected values.yaml at %s, got %v", valuesPath, err)
	}
	body := string(data)
	if !strings.Contains(body, "enabled: false") {
		t.Errorf("expected dns.enabled=false persisted, got:\n%s", body)
	}
	if !strings.Contains(body, "key: hello") {
		t.Errorf("expected custom.key=hello persisted, got:\n%s", body)
	}
}

// TestBootstrap_WritesContextFileOnFirstRun verifies the positional context arg
// persists to .windsor/context even when bootstrap later fails at the install
// step. Users on other machines need this file to resolve the same context,
// so context persistence must happen early in the bootstrap sequence.
func TestBootstrap_WritesContextFileOnFirstRun(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "bootstrap")
	// Run bootstrap; failure at the install step is expected and tolerated.
	_, _, _ = helpers.RunCLI(dir, []string{"bootstrap", "staging"}, env)

	contextPath := filepath.Join(dir, ".windsor", "context")
	data, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("expected .windsor/context at %s, got %v", contextPath, err)
	}
	if strings.TrimSpace(string(data)) != "staging" {
		t.Errorf("expected .windsor/context to contain 'staging', got: %q", data)
	}
}
