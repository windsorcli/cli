package terraform

import (
	"testing"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewShims(t *testing.T) {
	t.Run("InitializesAllFields", func(t *testing.T) {
		// Given a new shims instance
		shims := NewShims()

		// Then the shared and stack shims should be set
		if shims.Stat == nil {
			t.Error("Expected Stat to be initialized")
		}
		if shims.Getwd == nil {
			t.Error("Expected Getwd to be initialized")
		}
		if shims.Setenv == nil {
			t.Error("Expected Setenv to be initialized")
		}
		if shims.Remove == nil {
			t.Error("Expected Remove to be initialized")
		}
		if shims.ReadFile == nil {
			t.Error("Expected ReadFile to be initialized")
		}
		if shims.Chdir == nil {
			t.Error("Expected Chdir to be initialized")
		}
		if shims.TimeSleep == nil {
			t.Error("Expected TimeSleep to be initialized")
		}
	})
}
