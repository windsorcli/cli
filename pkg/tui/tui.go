// The tui package holds terminal formatting helpers that are not presenter output.
// It provides section headers for streamed tool output, such as a terraform plan.
// Progress, logs, and errors go through internal/presenter instead.
// The plan subpackage renders plan summaries.

package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// =============================================================================
// Public Methods
// =============================================================================

// SectionHeader returns a fixed-width section label padded with ─ characters.
// Example: SectionHeader("Kustomize: gitops") → "── Kustomize: gitops ──────────────────────────────────────"
func SectionHeader(label string) string {
	s := fmt.Sprintf("── %s ", label)
	const width = 62
	if n := width - utf8.RuneCountInString(s); n > 0 {
		s += strings.Repeat("─", n)
	}
	return s
}
