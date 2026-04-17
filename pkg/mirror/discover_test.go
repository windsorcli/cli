package mirror

import (
	"errors"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestDiscoverTarget(t *testing.T) {
	t.Run("ReturnsPublishedEndpointWhenContainerExposesPort", func(t *testing.T) {
		// Given a running mirror container with a host-published 5000/tcp
		// mapping on 0.0.0.0:55000
		m := shell.NewMockShell()
		m.ExecSilentFunc = func(cmd string, args ...string) (string, error) {
			if cmd == "docker" && len(args) > 0 && args[0] == "ps" {
				return "abc123\n", nil
			}
			if cmd == "docker" && len(args) > 0 && args[0] == "port" {
				return "0.0.0.0:55000\n", nil
			}
			return "", nil
		}

		// When discovering the target
		got, err := DiscoverTarget(m, "local")

		// Then the published port is returned with 0.0.0.0 rewritten to localhost
		if err != nil {
			t.Fatalf("DiscoverTarget: %v", err)
		}
		if got != "localhost:55000" {
			t.Errorf("got %q, want localhost:55000", got)
		}
	})

	t.Run("ErrorsWhenPortUnpublished", func(t *testing.T) {
		// Given a mirror container with no host port mapping
		m := shell.NewMockShell()
		m.ExecSilentFunc = func(cmd string, args ...string) (string, error) {
			if cmd == "docker" && len(args) > 0 && args[0] == "ps" {
				return "abc123\n", nil
			}
			if cmd == "docker" && len(args) > 0 && args[0] == "port" {
				return "", errors.New("no port")
			}
			return "", nil
		}

		// When discovering
		got, err := DiscoverTarget(m, "local")

		// Then an error is returned indicating the missing port mapping
		if err == nil {
			t.Fatal("expected error for unpublished port, got nil")
		}
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("ReturnsEmptyWhenNoContainerMatches", func(t *testing.T) {
		// Given docker ps returns no matching container
		m := shell.NewMockShell()
		m.ExecSilentFunc = func(cmd string, args ...string) (string, error) {
			return "", nil
		}

		// When discovering
		got, err := DiscoverTarget(m, "local")

		// Then the empty string is returned without error so the caller
		// can fall back to the self-hosted registry path
		if err != nil {
			t.Fatalf("DiscoverTarget: %v", err)
		}
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("ReturnsEmptyWhenDockerPsFails", func(t *testing.T) {
		// Given docker ps errors (e.g. docker daemon not running)
		m := shell.NewMockShell()
		m.ExecSilentFunc = func(cmd string, args ...string) (string, error) {
			if cmd == "docker" && len(args) > 0 && args[0] == "ps" {
				return "", errors.New("cannot connect to docker daemon")
			}
			return "", nil
		}

		// When discovering
		got, err := DiscoverTarget(m, "local")

		// Then the caller silently falls back; discovery is best-effort
		if err != nil {
			t.Fatalf("DiscoverTarget: %v", err)
		}
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("ReturnsEmptyWhenContextBlank", func(t *testing.T) {
		// Given an empty context (e.g. no windsor context selected)
		m := shell.NewMockShell()
		called := false
		m.ExecSilentFunc = func(cmd string, args ...string) (string, error) {
			called = true
			return "", nil
		}

		// When discovering
		got, err := DiscoverTarget(m, "")

		// Then discovery short-circuits without shelling out
		if err != nil {
			t.Fatalf("DiscoverTarget: %v", err)
		}
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
		if called {
			t.Error("expected no docker invocation with blank context")
		}
	})

	t.Run("FiltersByRoleAndContextLabels", func(t *testing.T) {
		// Given a discover call
		m := shell.NewMockShell()
		var psArgs []string
		m.ExecSilentFunc = func(cmd string, args ...string) (string, error) {
			if cmd == "docker" && len(args) > 0 && args[0] == "ps" {
				psArgs = append([]string(nil), args...)
			}
			return "", nil
		}

		// When discovering against context "demo"
		_, _ = DiscoverTarget(m, "demo")

		// Then docker ps is invoked with both the role=mirror and
		// context=demo label filters so sibling registry containers
		// are never returned as mirror targets
		wantRole := "label=role=mirror"
		wantCtx := "label=context=demo"
		hasRole, hasCtx := false, false
		for _, a := range psArgs {
			if a == wantRole {
				hasRole = true
			}
			if a == wantCtx {
				hasCtx = true
			}
		}
		if !hasRole || !hasCtx {
			t.Errorf("expected filters %q and %q, got %v", wantRole, wantCtx, psArgs)
		}
	})
}
