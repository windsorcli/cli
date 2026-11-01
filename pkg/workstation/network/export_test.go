// The export_test file gives the black-box tests of the network package access to its shims.
// It compiles only into the test binary, so the production API does not change.
// Tests replace the shims to simulate the resolver files and the temporary directories of the host.
// It holds no test cases.

package network

// =============================================================================
// Test Helpers
// =============================================================================

// SetShims replaces the shims of n.
func (n *BaseNetworkManager) SetShims(shims *Shims) {
	n.shims = shims
}
