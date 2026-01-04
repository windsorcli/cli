package services

import (
	"github.com/compose-spec/compose-go/v2/types"
)

// The MockService is a test implementation of the Service interface
// It provides a mockable implementation for testing service interactions
// The MockService enables isolated testing of service-dependent components
// by allowing test-specific behavior to be injected through function fields

// =============================================================================
// Types
// =============================================================================

// MockService is a mock implementation of the Service interface
type MockService struct {
	BaseService
	GetComposeConfigFunc func() (*types.Config, error)
	WriteConfigFunc      func() error
	SetAddressFunc       func(address string, portAllocator *PortAllocator) error
	GetAddressFunc       func() string
	SetNameFunc          func(name string)
	GetNameFunc          func() string
	GetHostnameFunc      func() string
	SupportsWildcardFunc func() bool
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockService is a constructor for MockService
func NewMockService() *MockService {
	return &MockService{}
}

// =============================================================================
// Public Methods
// =============================================================================

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
func (m *MockService) SetAddress(address string, portAllocator *PortAllocator) error {
	if m.SetAddressFunc != nil {
		return m.SetAddressFunc(address, portAllocator)
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

// GetName calls the mock GetNameFunc if it is set, otherwise returns the stored name
func (m *MockService) GetName() string {
	if m.GetNameFunc != nil {
		return m.GetNameFunc()
	}
	return m.name
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

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure MockService implements Service interface
var _ Service = (*MockService)(nil)
