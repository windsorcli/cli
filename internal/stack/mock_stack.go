package stack

import (
	"github.com/windsorcli/cli/internal/di"
)

// MockStack is a mock implementation of the Stack interface for testing purposes
type MockStack struct {
	BaseStack
	InitializeFunc func() error
	UpFunc         func() error
}

// NewMockStack creates a new instance of MockStack
func NewMockStack(injector di.Injector) *MockStack {
	return &MockStack{BaseStack: BaseStack{injector: injector}}
}

// Initialize calls the mock InitializeFunc if set, otherwise returns nil
func (m *MockStack) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
}

// Up calls the mock UpFunc if set, otherwise returns nil
func (m *MockStack) Up() error {
	if m.UpFunc != nil {
		return m.UpFunc()
	}
	return nil
}

// Ensure MockStack implements Stack
var _ Stack = (*MockStack)(nil)
