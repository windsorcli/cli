package terraform_test

import (
	"testing"

	"github.com/windsorcli/cli/pkg/composer/terraform"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewShims(t *testing.T) {
	t.Run("CreatesShimsWithDefaultImplementations", func(t *testing.T) {
		// Given a new shims instance
		shims := terraform.NewShims()

		// When the function fields are read
		fields := map[string]bool{
			"MkdirAll":      shims.MkdirAll != nil,
			"WriteFile":     shims.WriteFile != nil,
			"ReadFile":      shims.ReadFile != nil,
			"Stat":          shims.Stat != nil,
			"Chdir":         shims.Chdir != nil,
			"FilepathRel":   shims.FilepathRel != nil,
			"JsonUnmarshal": shims.JsonUnmarshal != nil,
			"Setenv":        shims.Setenv != nil,
		}

		// Then all function fields are set
		for name, set := range fields {
			if !set {
				t.Errorf("Expected %s to be set", name)
			}
		}
	})

	t.Run("FilepathRelReturnsRelativePath", func(t *testing.T) {
		// Given real shims
		shims := terraform.NewShims()

		// When FilepathRel is called
		rel, err := shims.FilepathRel("/base", "/base/path")

		// Then it returns the relative path
		if err != nil {
			t.Errorf("Expected FilepathRel to work, got error: %v", err)
		}
		if rel != "path" {
			t.Errorf("Expected relative path 'path', got: %s", rel)
		}
	})
}
