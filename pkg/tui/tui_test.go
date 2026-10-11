package tui_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/windsorcli/cli/pkg/tui"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestSectionHeader(t *testing.T) {
	t.Run("PadsAShortLabelToTheFixedWidth", func(t *testing.T) {
		// Given a short label
		label := "Terraform: cluster"

		// When the header is built
		header := tui.SectionHeader(label)

		// Then it starts with the label and pads to 62 runes
		if !strings.HasPrefix(header, "── Terraform: cluster ─") || utf8.RuneCountInString(header) != 62 {
			t.Errorf("unexpected header %q", header)
		}
	})

	t.Run("KeepsALongLabelWhole", func(t *testing.T) {
		// Given a label longer than the fixed width
		label := strings.Repeat("x", 80)

		// When the header is built
		header := tui.SectionHeader(label)

		// Then the label is kept without padding
		if header != "── "+label+" " {
			t.Errorf("unexpected header %q", header)
		}
	})
}
