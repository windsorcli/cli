// The MockEnvPrinter is a mock implementation of the EnvPrinter interface.
// It provides a testable implementation of environment variable management,
// The MockEnvPrinter enables testing of environment-dependent functionality,
// allowing for controlled simulation of environment operations in tests.

package env

// =============================================================================
// Types
// =============================================================================

// MockEnvPrinter is a struct that implements mock environment configuration
type MockEnvPrinter struct {
	BaseEnvPrinter
	PrintFunc           func() error
	PrintAliasFunc      func() error
	PostEnvHookFunc     func(directory ...string) error
	GetEnvVarsFunc      func() (map[string]string, error)
	GetAliasFunc        func() (map[string]string, error)
	GetManagedEnvFunc   func() []string
	GetManagedAliasFunc func() []string
	ResetFunc           func()
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockEnvPrinter creates a new MockEnvPrinter instance
func NewMockEnvPrinter() *MockEnvPrinter {
	return &MockEnvPrinter{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Print simulates printing the provided environment variables.
// If a custom PrintFunc is provided, it will use that function instead.
func (m *MockEnvPrinter) Print() error {
	if m.PrintFunc != nil {
		return m.PrintFunc()
	}
	return nil
}

// PrintAlias simulates printing the shell aliases.
// If a custom PrintAliasFunc is provided, it will use that function instead.
func (m *MockEnvPrinter) PrintAlias() error {
	if m.PrintAliasFunc != nil {
		return m.PrintAliasFunc()
	}
	return nil
}

// GetEnvVars simulates retrieving environment variables.
// If a custom GetEnvVarsFunc is provided, it will use that function instead.
func (m *MockEnvPrinter) GetEnvVars() (map[string]string, error) {
	if m.GetEnvVarsFunc != nil {
		return m.GetEnvVarsFunc()
	}
	// Return an empty map as a placeholder
	return map[string]string{}, nil
}

// GetAlias simulates retrieving the shell alias.
// If a custom GetAliasFunc is provided, it will use that function instead.
func (m *MockEnvPrinter) GetAlias() (map[string]string, error) {
	if m.GetAliasFunc != nil {
		return m.GetAliasFunc()
	}
	// Return an empty map as a placeholder
	return map[string]string{}, nil
}

// PostEnvHook simulates running any necessary commands after the environment variables have been set.
// If a custom PostEnvHookFunc is provided, it will use that function instead.
func (m *MockEnvPrinter) PostEnvHook(directory ...string) error {
	if m.PostEnvHookFunc != nil {
		return m.PostEnvHookFunc(directory...)
	}
	// Simulate post environment setup without doing anything real
	return nil
}

// Reset simulates resetting environment variables.
// If a custom ResetFunc is provided, it will use that function instead.
func (m *MockEnvPrinter) Reset() {
	if m.ResetFunc != nil {
		m.ResetFunc()
		return
	}
}

// GetManagedEnv returns the managed environment variables.
// If a custom GetManagedEnvFunc is provided, it will use that function instead.
func (m *MockEnvPrinter) GetManagedEnv() []string {
	if m.GetManagedEnvFunc != nil {
		return m.GetManagedEnvFunc()
	}
	return m.BaseEnvPrinter.GetManagedEnv()
}

// GetManagedAlias returns the managed aliases.
// If a custom GetManagedAliasFunc is provided, it will use that function instead.
func (m *MockEnvPrinter) GetManagedAlias() []string {
	if m.GetManagedAliasFunc != nil {
		return m.GetManagedAliasFunc()
	}
	return m.BaseEnvPrinter.GetManagedAlias()
}

// Ensure MockEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*MockEnvPrinter)(nil)
