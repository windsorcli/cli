// The export_test file gives the black-box tests of the virt package access to its shims.
// It compiles only into the test binary, so the production API does not change.
// Tests replace the shims to simulate the host, the file system, and the YAML encoder.
// It holds no test cases.

package virt

// =============================================================================
// Test Helpers
// =============================================================================

// SetShims replaces the shims of v.
func (v *BaseVirt) SetShims(shims *Shims) {
	v.shims = shims
}
