package services

import "github.com/compose-spec/compose-go/types"

// MockService is a mock implementation of the Service interface
type MockService struct {
	BaseService
	// GetContainerConfigFunc is a function that mocks the GetContainerConfig method
	GetComposeConfigFunc func() (*types.Config, error)
}

// NewMockService is a constructor for MockService
func NewMockService() *MockService {
	return &MockService{}
}

// GetComposeConfig calls the mock GetComposeConfigFunc if it is set, otherwise returns nil
func (m *MockService) GetComposeConfig() (*types.Config, error) {
	if m.GetComposeConfigFunc != nil {
		return m.GetComposeConfigFunc()
	}
	return nil, nil
}

// SetGetComposeConfigFunc sets the GetComposeConfigFunc for the mock service
func (m *MockService) SetGetComposeConfigFunc(getComposeConfigFunc func() (*types.Config, error)) {
	m.GetComposeConfigFunc = getComposeConfigFunc
}

// Ensure MockService implements Service interface
var _ Service = (*MockService)(nil)
