package helpers

import (
	"github.com/windsor-hotel/cli/internal/shell"
)

// MockHelper is a mock implementation of the Helper interface
type MockHelper struct {
	GetEnvVarsFunc func() (map[string]string, error)
	Shell          shell.Shell
}

// NewMockHelper is a constructor for MockHelper
func NewMockHelper(
	getEnvVarsFunc func() (map[string]string, error),
	shell shell.Shell,
) *MockHelper {
	return &MockHelper{
		GetEnvVarsFunc: getEnvVarsFunc,
		Shell:          shell,
	}
}

func (m *MockHelper) GetEnvVars() (map[string]string, error) {
	if m.GetEnvVarsFunc != nil {
		return m.GetEnvVarsFunc()
	}
	return nil, nil
}

// Ensure MockHelper implements Helper interface
var _ Helper = (*MockHelper)(nil)
