package helpers

import (
	"github.com/windsor-hotel/cli/internal/shell"
)

// MockHelper is a mock implementation of the Helper interface
type MockHelper struct {
	// GetEnvVarsFunc is a function that mocks the GetEnvVars method
	GetEnvVarsFunc func() (map[string]string, error)
	// Shell is an instance of the shell interface
	Shell shell.Shell
}

// NewMockHelper is a constructor for MockHelper
func NewMockHelper(
	// getEnvVarsFunc is a function that mocks the GetEnvVars method
	getEnvVarsFunc func() (map[string]string, error),
	shell shell.Shell,
) *MockHelper {
	return &MockHelper{
		GetEnvVarsFunc: getEnvVarsFunc,
		Shell:          shell,
	}
}

// GetEnvVars calls the mock GetEnvVarsFunc if it is set, otherwise returns nil
func (m *MockHelper) GetEnvVars() (map[string]string, error) {
	if m.GetEnvVarsFunc != nil {
		return m.GetEnvVarsFunc()
	}
	return nil, nil
}

// PostEnvExec runs any necessary commands after the environment variables have been set.
func (m *MockHelper) PostEnvExec() error {
	return nil
}

// Ensure MockHelper implements Helper interface
var _ Helper = (*MockHelper)(nil)
