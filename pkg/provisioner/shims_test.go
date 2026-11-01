package provisioner_test

import (
	"testing"

	"github.com/windsorcli/cli/pkg/provisioner"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewShims(t *testing.T) {
	t.Run("InitializesStat", func(t *testing.T) {
		// Given a new shims instance
		shims := provisioner.NewShims()

		// Then the Stat shim should be set
		if shims.Stat == nil {
			t.Error("Expected Stat to be initialized")
		}
	})
}
