package helpers

import "github.com/compose-spec/compose-go/types"

// MockHelper is a mock implementation of the Helper interface
type MockHelper struct {
	// GetContainerConfigFunc is a function that mocks the GetContainerConfig method
	GetComposeConfigFunc func() (*types.Config, error)
}

// NewMockHelper is a constructor for MockHelper
func NewMockHelper() *MockHelper {
	return &MockHelper{}
}

// GetComposeConfig calls the mock GetComposeConfigFunc if it is set, otherwise returns nil
func (m *MockHelper) GetComposeConfig() (*types.Config, error) {
	if m.GetComposeConfigFunc != nil {
		return m.GetComposeConfigFunc()
	}
	return nil, nil
}

// SetGetComposeConfigFunc sets the GetComposeConfigFunc for the mock helper
func (m *MockHelper) SetGetComposeConfigFunc(getComposeConfigFunc func() (*types.Config, error)) {
	m.GetComposeConfigFunc = getComposeConfigFunc
}

// Ensure MockHelper implements Helper interface
var _ Helper = (*MockHelper)(nil)
