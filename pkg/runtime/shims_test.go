package runtime

import (
	"testing"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewShims(t *testing.T) {
	t.Run("SetsSharedAndRuntimeShims", func(t *testing.T) {
		// Given the default shims
		s := NewShims()

		// Then the shared shims and the OpenRoot shim are set
		if s.Getenv == nil || s.Setenv == nil || s.LookupEnv == nil || s.MkdirAll == nil || s.Goos == nil {
			t.Error("Expected the shared shims to be set")
		}
		if s.OpenRoot == nil {
			t.Error("Expected OpenRoot to be set")
		}
	})
}

func TestNewRuntime_Shims(t *testing.T) {
	t.Run("UsesShimsOverride", func(t *testing.T) {
		// Given a Shims override
		mocks := setupRuntimeMocks(t)
		override := NewShims()

		// When NewRuntime is called with the override
		rt := NewRuntime(&Runtime{Shell: mocks.Shell, ConfigHandler: mocks.ConfigHandler, Shims: override})

		// Then the runtime uses the override
		if rt.Shims != override {
			t.Error("Expected the runtime to use the Shims override")
		}
	})
}
