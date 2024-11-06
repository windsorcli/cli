package helpers

import "github.com/compose-spec/compose-go/types"

// MockHelper is a mock implementation of the Helper interface
type MockHelper struct {
	// GetContainerConfigFunc is a function that mocks the GetContainerConfig method
	GetComposeConfigFunc func() (*types.Config, error)
	// WriteConfigFunc is a function that mocks the WriteConfig method
	WriteConfigFunc func() error
	// UpFunc is a function that mocks the Up method
	UpFunc func() error
	// InitializeFunc is a function that mocks the Initialize method
	InitializeFunc func() error
	// InfoFunc is a function that mocks the Info method
	InfoFunc func() (interface{}, error)
}

// NewMockHelper is a constructor for MockHelper
func NewMockHelper() *MockHelper {
	return &MockHelper{}
}

// Initialize calls the mock InitializeFunc if it is set, otherwise returns nil
func (m *MockHelper) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
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

// Up calls the mock UpFunc if it is set, otherwise returns nil
func (m *MockHelper) Up(verbose ...bool) error {
	if m.UpFunc != nil {
		return m.UpFunc()
	}
	return nil
}

// Info returns information about the helper.
func (m *MockHelper) Info() (interface{}, error) {
	if m.InfoFunc != nil {
		return m.InfoFunc()
	}
	return nil, nil
}

// SetInitializeFunc sets the InitializeFunc for the mock helper
func (m *MockHelper) SetInitializeFunc(initializeFunc func() error) {
	m.InitializeFunc = initializeFunc
}

// SetGetComposeConfigFunc sets the GetComposeConfigFunc for the mock helper
func (m *MockHelper) SetGetComposeConfigFunc(getComposeConfigFunc func() (*types.Config, error)) {
	m.GetComposeConfigFunc = getComposeConfigFunc
}

// SetWriteConfigFunc sets the WriteConfigFunc for the mock helper
func (m *MockHelper) SetWriteConfigFunc(writeConfigFunc func() error) {
	m.WriteConfigFunc = writeConfigFunc
}

// SetUpFunc sets the UpFunc for the mock helper
func (m *MockHelper) SetUpFunc(upFunc func() error) {
	m.UpFunc = upFunc
}

// SetInfoFunc sets the InfoFunc for the mock helper
func (m *MockHelper) SetInfoFunc(infoFunc func() (interface{}, error)) {
	m.InfoFunc = infoFunc
}

// Ensure MockHelper implements Helper interface
var _ Helper = (*MockHelper)(nil)
