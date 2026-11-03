// The export_test file gives the black-box tests of package config_test their test seams.
// It exposes private fields for fault injection and state checks. It does not expose private methods.

package config

// =============================================================================
// Test Seams
// =============================================================================

// SensitivePathsOK returns the sensitivePathsOK field of the SchemaValidator.
func (sv *SchemaValidator) SensitivePathsOK() bool {
	return sv.sensitivePathsOK
}
