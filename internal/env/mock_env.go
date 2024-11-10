package env

// MockEnvPrinter is a struct that simulates an environment for testing purposes.
type MockEnvPrinter struct {
	BaseEnvPrinter
	InitializeFunc  func() error
	PrintFunc       func() error
	PostEnvHookFunc func() error
	GetEnvVarsFunc  func() (map[string]string, error)
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
func (m *MockEnvPrinter) Print() error {
	if m.PrintFunc != nil {
		return m.PrintFunc()
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

// PostEnvHook simulates running any necessary commands after the environment variables have been set.
// If a custom PostEnvHookFunc is provided, it will use that function instead.
func (m *MockEnvPrinter) PostEnvHook() error {
	if m.PostEnvHookFunc != nil {
		return m.PostEnvHookFunc()
	}
	// Simulate post environment setup without doing anything real
	return nil
}

// Ensure MockEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*MockEnvPrinter)(nil)
