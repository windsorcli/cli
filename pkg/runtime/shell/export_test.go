// The export_test file gives the black-box tests of package shell_test their test seams.
// It exposes private fields for fault injection and state checks. It does not expose private methods.

package shell

// =============================================================================
// Test Seams
// =============================================================================

// SetProjectRoot sets the projectRoot field of the DefaultShell.
func (s *DefaultShell) SetProjectRoot(v string) {
	s.projectRoot = v
}

// SetSessionToken sets the sessionToken field of the DefaultShell.
func (s *DefaultShell) SetSessionToken(v string) {
	s.sessionToken = v
}

// Shims returns the shims field of the DefaultShell.
func (s *DefaultShell) Shims() *Shims {
	return s.shims
}

// SetShims sets the shims field of the DefaultShell.
func (s *DefaultShell) SetShims(v *Shims) {
	s.shims = v
}

// Verbose returns the verbose field of the DefaultShell.
func (s *DefaultShell) Verbose() bool {
	return s.verbose
}
