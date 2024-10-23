package helpers

import "github.com/compose-spec/compose-go/types"

// MockHelper is a mock implementation of the Helper interface
type MockHelper struct {
	// GetEnvVarsFunc is a function that mocks the GetEnvVars method
	GetEnvVarsFunc func() (map[string]string, error)
	// PostEnvExecFunc is a function that mocks the PostEnvExec method
	PostEnvExecFunc func() error
	// GetContainerConfigFunc is a function that mocks the GetContainerConfig method
	GetComposeConfigFunc func() (*types.Config, error)
	// WriteConfigFunc is a function that mocks the WriteConfig method
	WriteConfigFunc func() error
	// InitializeFunc is a function that mocks the Initialize method
	InitializeFunc func() error
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

// GetComposeConfig calls the mock GetComposeConfigFunc if it is set, otherwise returns nil
func (m *MockHelper) GetComposeConfig() (*types.Config, error) {
	if m.GetComposeConfigFunc != nil {
		return m.GetComposeConfigFunc()
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

// Initialize calls the mock InitializeFunc if it is set, otherwise returns nil
func (m *MockHelper) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
}

// SetPostEnvExecFunc sets the PostEnvExecFunc for the mock helper
func (m *MockHelper) SetPostEnvExecFunc(postEnvExecFunc func() error) {
	m.PostEnvExecFunc = postEnvExecFunc
}

// SetGetComposeConfigFunc sets the GetComposeConfigFunc for the mock helper
func (m *MockHelper) SetGetComposeConfigFunc(getComposeConfigFunc func() (*types.Config, error)) {
	m.GetComposeConfigFunc = getComposeConfigFunc
}

// SetWriteConfigFunc sets the WriteConfigFunc for the mock helper
func (m *MockHelper) SetWriteConfigFunc(writeConfigFunc func() error) {
	m.WriteConfigFunc = writeConfigFunc
}

// SetInitializeFunc sets the InitializeFunc for the mock helper
func (m *MockHelper) SetInitializeFunc(initializeFunc func() error) {
	m.InitializeFunc = initializeFunc
}

// Ensure MockHelper implements Helper interface
var _ Helper = (*MockHelper)(nil)
