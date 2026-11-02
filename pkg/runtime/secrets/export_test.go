// The export_test file gives the black-box tests of package secrets_test their test seams.
// It exposes private fields for fault injection and state checks. It does not expose private methods.

package secrets

// =============================================================================
// Test Seams
// =============================================================================

// Shims returns the shims field of the OnePasswordCLIProvider.
func (s *OnePasswordCLIProvider) Shims() *Shims {
	return s.shims
}

// Unlocked returns the unlocked field of the OnePasswordCLIProvider.
func (s *OnePasswordCLIProvider) Unlocked() bool {
	return s.unlocked
}

// SetUnlocked sets the unlocked field of the OnePasswordCLIProvider.
func (s *OnePasswordCLIProvider) SetUnlocked(v bool) {
	s.unlocked = v
}

// Unlocked returns the unlocked field of the OnePasswordSDKProvider.
func (s *OnePasswordSDKProvider) Unlocked() bool {
	return s.unlocked
}

// ConfigPath returns the configPath field of the SopsProvider.
func (s *SopsProvider) ConfigPath() string {
	return s.configPath
}

// Unlocked returns the unlocked field of the SopsProvider.
func (s *SopsProvider) Unlocked() bool {
	return s.unlocked
}
