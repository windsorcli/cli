package artifact

import (
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
)

func TestNewShims(t *testing.T) {
	t.Run("ParseReferenceForwardsOptions", func(t *testing.T) {
		// Given the default shims
		shims := NewShims()

		// When parsing a bare repo:tag with a custom default registry option
		ref, err := shims.ParseReference("repo:tag", name.WithDefaultRegistry("custom.example.com"))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the option should be applied to the parsed reference
		if got := ref.Context().RegistryStr(); got != "custom.example.com" {
			t.Errorf("expected registry custom.example.com, got %s", got)
		}
	})
}
