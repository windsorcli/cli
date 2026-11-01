// The export_test file gives the black-box tests of the workstation package access to its shims.
// It compiles only into the test binary, so the production API does not change.
// Tests replace the shims to simulate the environment and the file system.
// It holds no test cases.

package workstation

import "github.com/windsorcli/cli/internal/shims"

// =============================================================================
// Test Helpers
// =============================================================================

// SetShims replaces the shims of w.
func (w *Workstation) SetShims(s *shims.Shims) {
	w.shims = s
}
