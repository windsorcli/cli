package services

import (
	"github.com/compose-spec/compose-go/types"
)

// MockService is a mock implementation of the Service interface
type MockService struct {
	BaseService
	// GetComposeConfigFunc is a function that mocks the GetComposeConfig method
	GetComposeConfigFunc func() (*types.Config, error)
	// WriteConfigFunc is a function that mocks the WriteConfig method
	WriteConfigFunc func() error
	// SetAddressFunc is a function that mocks the SetAddress method
	SetAddressFunc func(address string) error
	// GetAddressFunc is a function that mocks the GetAddress method
	GetAddressFunc func() string
	// InitializeFunc is a function that mocks the Initialize method
	InitializeFunc func() error
	// SetNameFunc is a function that mocks the SetName method
	SetNameFunc func(name string)
	// GetNameFunc is a function that mocks the GetName method
	GetNameFunc func() string
	// GetHostnameFunc is a function that mocks the GetHostname method
	GetHostnameFunc func() string
	// SupportsWildcardFunc is a function that mocks the SupportsWildcard method
	SupportsWildcardFunc func() bool
}

// NewMockService is a constructor for MockService
func NewMockService() *MockService {
	return &MockService{}
}

// Initialize calls the mock InitializeFunc if it is set, otherwise returns nil
func (m *MockService) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
}

// GetComposeConfig calls the mock GetComposeConfigFunc if it is set, otherwise returns nil
func (m *MockService) GetComposeConfig() (*types.Config, error) {
	if m.GetComposeConfigFunc != nil {
		return m.GetComposeConfigFunc()
	}
	return nil, nil
}

// WriteConfig calls the mock WriteConfigFunc if it is set, otherwise returns nil
func (m *MockService) WriteConfig() error {
	if m.WriteConfigFunc != nil {
		return m.WriteConfigFunc()
	}
	return nil
}

// SetAddress calls the mock SetAddressFunc if it is set, otherwise returns nil
func (m *MockService) SetAddress(address string) error {
	if m.SetAddressFunc != nil {
		return m.SetAddressFunc(address)
	}
	return nil
}

// GetAddress calls the mock GetAddressFunc if it is set, otherwise returns an empty string
func (m *MockService) GetAddress() string {
	if m.GetAddressFunc != nil {
		return m.GetAddressFunc()
	}
	return ""
}

// SetName calls the mock SetNameFunc if it is set
func (m *MockService) SetName(name string) {
	if m.SetNameFunc != nil {
		m.SetNameFunc(name)
	}
	m.name = name
}

// GetName calls the mock GetNameFunc if it is set, otherwise returns an empty string
func (m *MockService) GetName() string {
	if m.GetNameFunc != nil {
		return m.GetNameFunc()
	}
	return ""
}

// GetHostname calls the mock GetHostnameFunc if it is set, otherwise returns an empty string
func (m *MockService) GetHostname() string {
	if m.GetHostnameFunc != nil {
		return m.GetHostnameFunc()
	}
	return ""
}

// SupportsWildcard calls the mock SupportsWildcardFunc if it is set, otherwise returns false
func (m *MockService) SupportsWildcard() bool {
	if m.SupportsWildcardFunc != nil {
		return m.SupportsWildcardFunc()
	}
	return false
}

// Ensure MockService implements Service interface
var _ Service = (*MockService)(nil)
