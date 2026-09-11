package client

import (
	"testing"

	"k8s.io/client-go/rest"

	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Private Methods
// =============================================================================

func TestWarningHandlerFor(t *testing.T) {
	t.Run("SuppressesWhenNotVerbose", func(t *testing.T) {
		mockShell := shell.NewMockShell()
		mockShell.IsVerboseFunc = func() bool { return false }

		handler := warningHandlerFor(mockShell)

		if _, ok := handler.(rest.NoWarnings); !ok {
			t.Errorf("Expected rest.NoWarnings, got %T", handler)
		}
	})

	t.Run("PassesThroughWhenVerbose", func(t *testing.T) {
		mockShell := shell.NewMockShell()
		mockShell.IsVerboseFunc = func() bool { return true }

		handler := warningHandlerFor(mockShell)

		if handler != nil {
			t.Errorf("Expected nil handler, got %T", handler)
		}
	})

	t.Run("SuppressesWhenShellNil", func(t *testing.T) {
		handler := warningHandlerFor(nil)

		if _, ok := handler.(rest.NoWarnings); !ok {
			t.Errorf("Expected rest.NoWarnings, got %T", handler)
		}
	})
}
