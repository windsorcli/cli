package client

import (
	"testing"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewShims(t *testing.T) {
	t.Run("InitializesTheShimsThatTheClientUses", func(t *testing.T) {
		// Given a new shims instance
		shims := NewShims()

		// Then the file and environment shims should be set
		if shims.ReadFile == nil || shims.Getenv == nil || shims.UserHomeDir == nil {
			t.Error("Expected the shims to be initialized")
		}
	})
}
