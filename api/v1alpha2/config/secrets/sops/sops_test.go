package sops

import "testing"

func TestSopsConfig_Merge(t *testing.T) {
	enabled := true
	disabled := false

	t.Run("NilOverlayLeavesBaseUnchanged", func(t *testing.T) {
		base := &SopsConfig{Enabled: &enabled}
		base.Merge(nil)

		if base.Enabled == nil || !*base.Enabled {
			t.Errorf("Expected enabled to stay true, got %+v", base)
		}
	})

	t.Run("OverlayEnabledReplacesBase", func(t *testing.T) {
		base := &SopsConfig{Enabled: &enabled}
		base.Merge(&SopsConfig{Enabled: &disabled})

		if base.Enabled == nil || *base.Enabled {
			t.Errorf("Expected enabled=false, got %+v", base)
		}
	})

	t.Run("UnsetOverlayLeavesBaseUnchanged", func(t *testing.T) {
		base := &SopsConfig{Enabled: &enabled}
		base.Merge(&SopsConfig{})

		if base.Enabled == nil || !*base.Enabled {
			t.Errorf("Expected enabled to stay true, got %+v", base)
		}
	})
}

func TestSopsConfig_DeepCopy(t *testing.T) {
	t.Run("NilReturnsNil", func(t *testing.T) {
		var c *SopsConfig
		if c.DeepCopy() != nil {
			t.Error("Expected nil copy of a nil config")
		}
	})

	t.Run("CopyDoesNotShareThePointer", func(t *testing.T) {
		enabled := true
		original := &SopsConfig{Enabled: &enabled}
		copied := original.DeepCopy()
		*copied.Enabled = false

		if !*original.Enabled {
			t.Error("Expected the original to be unchanged by edits to the copy")
		}
	})
}
