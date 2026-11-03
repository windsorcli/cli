// The export_test file gives the black-box tests of package project_test their test seams.
// It exposes private fields for fault injection. It does not expose private methods.

package project

import "github.com/windsorcli/cli/internal/shims"

// =============================================================================
// Test Seams
// =============================================================================

// Shims returns the shims field of the Project.
func (p *Project) Shims() *shims.Shims {
	return p.shims
}
