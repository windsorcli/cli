package helpers

import (
	"github.com/windsor-hotel/cli/internal/shell"
)

// MockHelper is a mock implementation of the Helper interface
type MockHelper struct {
	// GetEnvVarsFunc is a function that mocks the GetEnvVars method
	GetEnvVarsFunc func() (map[string]string, error)
	// PostEnvExecFunc is a function that mocks the PostEnvExec method
	PostEnvExecFunc func() error
	// SetConfigFunc is a function that mocks the SetConfig method
	SetConfigFunc func(key, value string) error
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

// PostEnvExec calls the mock PostEnvExecFunc if it is set, otherwise returns nil
func (m *MockHelper) PostEnvExec() error {
	if m.PostEnvExecFunc != nil {
		return m.PostEnvExecFunc()
	}
	return nil
}

// SetConfig calls the mock SetConfigFunc if it is set, otherwise returns nil
func (m *MockHelper) SetConfig(key, value string) error {
	if m.SetConfigFunc != nil {
		return m.SetConfigFunc(key, value)
	}
	return nil
}

// SetSetConfigFunc sets the SetConfigFunc for the mock helper
func (m *MockHelper) SetSetConfigFunc(setConfigFunc func(key, value string) error) {
	m.SetConfigFunc = setConfigFunc
}

// SetPostEnvExecFunc sets the PostEnvExecFunc for the mock helper
func (m *MockHelper) SetPostEnvExecFunc(postEnvExecFunc func() error) {
	m.PostEnvExecFunc = postEnvExecFunc
}

// Ensure MockHelper implements Helper interface
var _ Helper = (*MockHelper)(nil)
