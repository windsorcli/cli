//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"

	"github.com/windsorcli/cli/integration/helpers"
)

// =============================================================================
// Integration Tests
// =============================================================================

// Error path — running `windsor mirror` in a project whose composed blueprint
// has no oci:// sources fails fast with a clear message rather than starting a
// registry container. This is the default fixture, which has no source
// dependencies to mirror.
func TestMirror_FailsWhenNoOCISourcesPresent(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")

	_, stderr, err := helpers.RunCLI(dir, []string{"mirror"}, env)
	if err == nil {
		t.Fatal("expected mirror to fail with no OCI sources, got success")
	}
	if !strings.Contains(string(stderr), "no oci:// sources") &&
		!strings.Contains(string(stderr), "nothing to mirror") {
		t.Errorf("expected 'no oci:// sources' message, got stderr: %s", stderr)
	}
}
