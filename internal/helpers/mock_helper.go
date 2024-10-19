package helpers

import "github.com/compose-spec/compose-go/types"

// MockHelper is a mock implementation of the Helper interface
type MockHelper struct {
	// GetEnvVarsFunc is a function that mocks the GetEnvVars method
	GetEnvVarsFunc func() (map[string]string, error)
	// PostEnvExecFunc is a function that mocks the PostEnvExec method
	PostEnvExecFunc func() error
	// GetContainerConfigFunc is a function that mocks the GetContainerConfig method
	GetContainerConfigFunc func() ([]types.ServiceConfig, error)
	// WriteConfigFunc is a function that mocks the WriteConfig method
	WriteConfigFunc func() error
}

// NewMockHelper is a constructor for MockHelper
func NewMockHelper() *MockHelper {
	return &MockHelper{}
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

// GetContainerConfig calls the mock GetContainerConfigFunc if it is set, otherwise returns nil
func (m *MockHelper) GetContainerConfig() ([]types.ServiceConfig, error) {
	if m.GetContainerConfigFunc != nil {
		return m.GetContainerConfigFunc()
	}
	return nil, nil
}

// WriteConfig calls the mock WriteConfigFunc if it is set, otherwise returns nil
func (m *MockHelper) WriteConfig() error {
	if m.WriteConfigFunc != nil {
		return m.WriteConfigFunc()
	}
	return nil
}

// SetPostEnvExecFunc sets the PostEnvExecFunc for the mock helper
func (m *MockHelper) SetPostEnvExecFunc(postEnvExecFunc func() error) {
	m.PostEnvExecFunc = postEnvExecFunc
}

// SetGetContainerConfigFunc sets the GetContainerConfigFunc for the mock helper
func (m *MockHelper) SetGetContainerConfigFunc(getContainerConfigFunc func() ([]types.ServiceConfig, error)) {
	m.GetContainerConfigFunc = getContainerConfigFunc
}

// SetWriteConfigFunc sets the WriteConfigFunc for the mock helper
func (m *MockHelper) SetWriteConfigFunc(writeConfigFunc func() error) {
	m.WriteConfigFunc = writeConfigFunc
}

// Ensure MockHelper implements Helper interface
var _ Helper = (*MockHelper)(nil)
