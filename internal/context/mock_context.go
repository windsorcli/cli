package context

import (
	"errors"
)

type MockContext struct {
	GetContextFunc    func() (string, error)
	SetContextFunc    func(context string) error
	GetConfigRootFunc func() (string, error)
}

func NewMockContext() *MockContext {
	return &MockContext{}
}

func (m *MockContext) GetContext() (string, error) {
	if m.GetContextFunc != nil {
		return m.GetContextFunc()
	}
	return "", errors.New("GetContextFunc not implemented")
}

func (m *MockContext) SetContext(context string) error {
	if m.SetContextFunc != nil {
		return m.SetContextFunc(context)
	}
	return errors.New("SetContextFunc not implemented")
}

func (m *MockContext) GetConfigRoot() (string, error) {
	if m.GetConfigRootFunc != nil {
		return m.GetConfigRootFunc()
	}
	return "", errors.New("GetConfigRootFunc not implemented")
}

// Ensure MockContext implements the ContextInterface
var _ ContextInterface = (*MockContext)(nil)
