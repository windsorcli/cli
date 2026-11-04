package inventory_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/composer/inventory"
)

// =============================================================================
// Test Setup
// =============================================================================

func fluxInstance() map[string]any {
	return map[string]any{
		"apiVersion": "fluxcd.controlplane.io/v1",
		"kind":       "FluxInstance",
		"metadata":   map[string]any{"name": "flux", "namespace": "system-gitops"},
		"spec":       map[string]any{"distribution": map[string]any{"version": "2.x"}},
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestFluxInstanceExtractor_Matches(t *testing.T) {
	t.Run("MatchesOnlyTheFluxOperatorKind", func(t *testing.T) {
		// Given an extractor
		m := setupMocks(t)
		extractor := inventory.NewFluxInstanceExtractor(m.runtime, m.shims)

		// When Matches is called on a FluxInstance and on another kind
		matched := extractor.Matches(fluxInstance())
		other := extractor.Matches(map[string]any{"apiVersion": "example.com/v1", "kind": "FluxInstance"})

		// Then only the flux-operator FluxInstance matches
		if !matched || other {
			t.Errorf("expected true and false, got %v and %v", matched, other)
		}
	})
}

func TestFluxInstanceExtractor_Extract(t *testing.T) {
	t.Run("ReturnsTheBuiltObjects", func(t *testing.T) {
		// Given flux-operator build instance prints a Deployment
		m := setupMocks(t)
		var gotArgs []string
		m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			gotArgs = append([]string{command}, args...)
			return "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: source-controller\n", nil
		}
		extractor := inventory.NewFluxInstanceExtractor(m.runtime, m.shims)

		// When Extract is called
		objects, err := extractor.Extract(fluxInstance())

		// Then the build output is returned and the FluxInstance went to the CLI
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(objects) != 1 || objects[0]["kind"] != "Deployment" {
			t.Errorf("unexpected objects: %v", objects)
		}
		if strings.Join(gotArgs, " ") != "flux-operator build instance -f /tmp/inventory/flux-instance.yaml" {
			t.Errorf("unexpected args: %v", gotArgs)
		}
		if !strings.Contains(m.files["/tmp/inventory/flux-instance.yaml"], "kind: FluxInstance") {
			t.Errorf("expected the FluxInstance in the file, got %v", m.files)
		}
	})

	t.Run("FailsWithoutFluxOperator", func(t *testing.T) {
		// Given no flux-operator CLI
		m := setupMocks(t)
		m.shims.LookPath = func(file string) (string, error) { return "", errors.New("not found") }
		extractor := inventory.NewFluxInstanceExtractor(m.runtime, m.shims)

		// When Extract is called
		_, err := extractor.Extract(fluxInstance())

		// Then the error asks for the flux-operator CLI
		if err == nil || !strings.Contains(err.Error(), "flux-operator CLI is required") {
			t.Fatalf("expected a missing CLI error, got %v", err)
		}
	})

	t.Run("ReturnsBuildError", func(t *testing.T) {
		// Given a build that fails
		m := setupMocks(t)
		m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			return "", errors.New("pull failed")
		}
		extractor := inventory.NewFluxInstanceExtractor(m.runtime, m.shims)

		// When Extract is called
		_, err := extractor.Extract(fluxInstance())

		// Then the error is returned
		if err == nil || !strings.Contains(err.Error(), "failed to build FluxInstance") {
			t.Fatalf("expected a build error, got %v", err)
		}
	})
}
