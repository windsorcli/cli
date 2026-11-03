// The export_test file gives the black-box tests of package runtime_test their test seams.
// It exposes private fields for fault injection and state checks. It does not expose private methods.

package runtime

// =============================================================================
// Test Seams
// =============================================================================

// Aliases returns the aliases field of the Runtime.
func (rt *Runtime) Aliases() map[string]string {
	return rt.aliases
}

// SetAliases sets the aliases field of the Runtime.
func (rt *Runtime) SetAliases(v map[string]string) {
	rt.aliases = v
}

// EnvVars returns the envVars field of the Runtime.
func (rt *Runtime) EnvVars() map[string]string {
	return rt.envVars
}

// SetEnvVars sets the envVars field of the Runtime.
func (rt *Runtime) SetEnvVars(v map[string]string) {
	rt.envVars = v
}
