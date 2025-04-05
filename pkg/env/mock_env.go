package env

// MockEnvPrinter is a struct that simulates an environment for testing purposes.
type MockEnvPrinter struct {
	BaseEnvPrinter
	InitializeFunc  func() error
	PrintFunc       func(customVars ...map[string]string) error
	GetAliasFunc    func() (map[string]string, error)
	PrintAliasFunc  func(customAliases ...map[string]string) error
	PostEnvHookFunc func() error
	GetEnvVarsFunc  func() (map[string]string, error)
	ClearFunc       func() error
}

// NewMockEnvPrinter creates a new instance of MockEnvPrinter.
func NewMockEnvPrinter() *MockEnvPrinter {
	return &MockEnvPrinter{}
}

// Initialize calls the custom InitializeFunc if provided.
func (m *MockEnvPrinter) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
}

// Print simulates printing the provided environment variables.
// If a custom PrintFunc is provided, it will use that function instead.
func (m *MockEnvPrinter) Print(customVars ...map[string]string) error {
	if m.PrintFunc != nil {
		return m.PrintFunc(customVars...)
	}

	// If customVars are provided, use them with BaseEnvPrinter
	if len(customVars) > 0 {
		return m.BaseEnvPrinter.Print(customVars[0])
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

// GetAlias simulates retrieving aliases.
// If a custom GetAliasFunc is provided, it will use that function instead.
func (m *MockEnvPrinter) GetAlias() (map[string]string, error) {
	if m.GetAliasFunc != nil {
		return m.GetAliasFunc()
	}
	// Return an empty map as a placeholder
	return map[string]string{}, nil
}

// PrintAlias simulates printing the provided aliases.
// If a custom PrintAliasFunc is provided, it will use that function instead.
func (m *MockEnvPrinter) PrintAlias(customAliases ...map[string]string) error {
	if m.PrintAliasFunc != nil {
		return m.PrintAliasFunc(customAliases...)
	}

	// If customAliases are provided, use them with BaseEnvPrinter
	if len(customAliases) > 0 {
		return m.BaseEnvPrinter.PrintAlias(customAliases[0])
	}

	return nil
}

// PostEnvHook simulates running any necessary commands after the environment variables have been set.
// If a custom PostEnvHookFunc is provided, it will use that function instead.
func (m *MockEnvPrinter) PostEnvHook() error {
	if m.PostEnvHookFunc != nil {
		return m.PostEnvHookFunc()
	}
	// Simulate post environment setup without doing anything real
	return nil
}

// Clear simulates clearing all tracked environment variables and aliases.
// If a custom ClearFunc is provided, it will use that function instead.
func (m *MockEnvPrinter) Clear() error {
	if m.ClearFunc != nil {
		return m.ClearFunc()
	}
	// Defer to the base implementation if no custom function is provided
	return m.BaseEnvPrinter.Clear()
}

// Ensure MockEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*MockEnvPrinter)(nil)
