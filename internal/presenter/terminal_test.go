package presenter

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestTTYModes(t *testing.T) {
	t.Run("TogglesTheCursorAndKeepsEchoForANonTerminalInput", func(t *testing.T) {
		// Given terminal modes whose input is a regular file
		in, err := os.Create(filepath.Join(t.TempDir(), "in"))
		if err != nil {
			t.Fatalf("failed to create input file: %v", err)
		}
		defer in.Close()
		var out bytes.Buffer
		modes := newTTYModes(in, &out)

		// When the modes enter and leave
		modes.Enter()
		modes.Leave()

		// Then the cursor hides and shows, and no echo restore was recorded
		if out.String() != ansiHideCursor+ansiShowCursor {
			t.Errorf("unexpected output %q", out.String())
		}
		if modes.restore != nil {
			t.Error("expected no echo restore for a non-terminal input")
		}
	})
}
