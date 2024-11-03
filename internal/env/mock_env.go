package env

import (
	"github.com/windsor-hotel/cli/internal/di"
)

// MockEnv is a struct that simulates an environment for testing purposes.
type MockEnv struct {
	PrintFunc       func(envVars map[string]string)
	PostEnvHookFunc func() error
	diContainer     di.ContainerInterface
}

// NewMockEnv creates a new instance of MockEnv with the provided di container.
func NewMockEnv(diContainer di.ContainerInterface) *MockEnv {
	return &MockEnv{
		diContainer: diContainer,
	}
}

// Print simulates printing the provided environment variables.
// If a custom PrintFunc is provided, it will use that function instead.
func (m *MockEnv) Print(envVars map[string]string) {
	if m.PrintFunc != nil {
		m.PrintFunc(envVars)
		return
	}
	// Simulate printing without doing anything real
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

// Ensure MockEnv implements the EnvInterface
var _ EnvInterface = (*MockEnv)(nil)
