package env

import (
	"github.com/windsor-hotel/cli/internal/di"
)

// MockEnv is a struct that simulates an environment for testing purposes.
type MockEnv struct {
	Env
	PrintFunc       func() error
	PostEnvHookFunc func() error
	GetEnvVarsFunc  func() (map[string]string, error)
}

// NewMockEnv creates a new instance of MockEnv with the provided di container.
func NewMockEnv(diContainer di.ContainerInterface) *MockEnv {
	return &MockEnv{
		Env: Env{
			diContainer: diContainer,
		},
	}
}

// Print simulates printing the provided environment variables.
// If a custom PrintFunc is provided, it will use that function instead.
func (m *MockEnv) Print() error {
	if m.PrintFunc != nil {
		return m.PrintFunc()
	}
	return nil
}

// GetEnvVars simulates retrieving environment variables.
// If a custom GetEnvVarsFunc is provided, it will use that function instead.
func (m *MockEnv) GetEnvVars() (map[string]string, error) {
	if m.GetEnvVarsFunc != nil {
		return m.GetEnvVarsFunc()
	}
	// Return an empty map as a placeholder
	return map[string]string{}, nil
}

// PostEnvHook simulates running any necessary commands after the environment variables have been set.
// If a custom PostEnvHookFunc is provided, it will use that function instead.
func (m *MockEnv) PostEnvHook() error {
	if m.PostEnvHookFunc != nil {
		return m.PostEnvHookFunc()
	}
	// Simulate post environment setup without doing anything real
	return nil
}

// Ensure MockEnv implements the EnvPrinter interface
var _ EnvPrinter = (*MockEnv)(nil)
