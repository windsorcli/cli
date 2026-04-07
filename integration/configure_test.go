//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"

	"github.com/windsorcli/cli/integration/helpers"
)

func TestConfigureNetwork_FailsWhenNotTrusted(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "default")
	env = append(env, "WINDSOR_CONTEXT=default")
	_, stderr, err := helpers.RunCLI(dir, []string{"configure", "network"}, env)
	if err == nil {
		t.Fatal("expected configure network to fail when not in trusted dir")
	}
	if !strings.Contains(string(stderr), "trusted") {
		t.Errorf("expected stderr to contain 'trusted', got: %s", stderr)
	}
}

func TestConfigureNetwork_SucceedsAfterInit(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	env = append(env, "WINDSOR_CONTEXT=default")
	_, stderr, err := helpers.RunCLI(dir, []string{"configure", "network"}, env)
	if err != nil {
		t.Fatalf("configure network: %v\nstderr: %s", err, stderr)
	}
}

func TestConfigureNetwork_SucceedsWithDnsAddressFlag(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	env = append(env, "WINDSOR_CONTEXT=default")
	_, stderr, err := helpers.RunCLI(dir, []string{"configure", "network", "--dns-address=10.5.0.2"}, env)
	if err != nil {
		t.Fatalf("configure network --dns-address: %v\nstderr: %s", err, stderr)
	}
}
