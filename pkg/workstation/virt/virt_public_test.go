package virt_test

import (
	"testing"

	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewBaseVirt(t *testing.T) {
	f := newFixture(t)
	cases := map[string]*runtime.Runtime{
		"PanicsWhenRuntimeIsNil":       nil,
		"PanicsWhenShellIsNil":         {ConfigHandler: f.config},
		"PanicsWhenConfigHandlerIsNil": {Shell: f.shell},
	}
	for name, rt := range cases {
		t.Run(name, func(t *testing.T) {
			// Given an incomplete runtime
			defer func() {
				// Then the constructor panics
				if recover() == nil {
					t.Error("Expected a panic")
				}
			}()

			// When creating the base virt
			virt.NewBaseVirt(rt)
		})
	}

	t.Run("ReturnsABaseVirtForACompleteRuntime", func(t *testing.T) {
		// Given a complete runtime

		// When creating the base virt
		baseVirt := virt.NewBaseVirt(f.runtime)

		// Then a base virt is returned
		if baseVirt == nil {
			t.Error("Expected a BaseVirt")
		}
	})
}
