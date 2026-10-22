package composer

import (
	"testing"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewShims(t *testing.T) {
	t.Run("InitializesTheFileShims", func(t *testing.T) {
		// Given a new shims instance
		shims := NewShims()

		// Then the file shims that the composer uses should be set
		if shims.Stat == nil || shims.ReadFile == nil || shims.WriteFile == nil || shims.MkdirAll == nil || shims.ReadDir == nil {
			t.Error("Expected the file shims to be initialized")
		}
	})
}
