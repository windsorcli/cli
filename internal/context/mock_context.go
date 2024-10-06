package context

import (
	"errors"
)

// MockContext is a mock implementation of the ContextInterface
type MockContext struct {
	GetContextFunc    func() (string, error)     // Function to mock GetContext
	SetContextFunc    func(context string) error // Function to mock SetContext
	GetConfigRootFunc func() (string, error)     // Function to mock GetConfigRoot
}

// NewMockContext creates a new instance of MockContext
func NewMockContext() *MockContext {
	return &MockContext{}
}

// GetContext calls the mock GetContextFunc if set, otherwise returns an error
func (m *MockContext) GetContext() (string, error) {
	if m.GetContextFunc != nil {
		return m.GetContextFunc()
	}
	return "", errors.New("GetContextFunc not implemented")
}

// SetContext calls the mock SetContextFunc if set, otherwise returns an error
func (m *MockContext) SetContext(context string) error {
	if m.SetContextFunc != nil {
		return m.SetContextFunc(context)
	}
	return errors.New("SetContextFunc not implemented")
}

// GetConfigRoot calls the mock GetConfigRootFunc if set, otherwise returns an error
func (m *MockContext) GetConfigRoot() (string, error) {
	if m.GetConfigRootFunc != nil {
		return m.GetConfigRootFunc()
	}
	return "", errors.New("GetConfigRootFunc not implemented")
}

// Ensure MockContext implements the ContextInterface
var _ ContextInterface = (*MockContext)(nil)
