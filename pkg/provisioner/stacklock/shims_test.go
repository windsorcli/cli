package stacklock

import (
	"testing"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewShims(t *testing.T) {
	t.Run("initializes the file shims", func(t *testing.T) {
		// Given a new shims instance
		shims := NewShims()

		// Then the file shims should be set
		if shims.Stat == nil || shims.ReadFile == nil || shims.WriteFile == nil || shims.Rename == nil || shims.Remove == nil || shims.MkdirAll == nil {
			t.Error("Expected the file shims to be initialized")
		}
	})
}
