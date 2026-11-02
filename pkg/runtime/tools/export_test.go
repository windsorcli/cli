// The export_test file gives the black-box tests of package tools_test their test seams.
// It exposes private fields for fault injection and state checks. It does not expose private methods.

package tools

// =============================================================================
// Test Seams
// =============================================================================

// SetShims sets the shims field of the BaseToolsManager.
func (t *BaseToolsManager) SetShims(v *Shims) {
	t.shims = v
}
