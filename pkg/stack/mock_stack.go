package stack

// The MockStack is a test implementation of the Stack interface.
// It provides function fields that can be set to customize behavior in tests,
// The MockStack acts as a controllable test double for the Stack interface,
// enabling precise control over Initialize and Up behaviors in unit tests.

import (
	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Types
// =============================================================================

// MockStack is a mock implementation of the Stack interface for testing purposes.
// It embeds BaseStack to inherit common functionality and adds function fields
// that can be set to customize behavior in tests.
type MockStack struct {
	BaseStack
	InitializeFunc func() error
	UpFunc         func() error
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockStack creates a new instance of MockStack with the provided injector.
// The injector is stored in the embedded BaseStack, and function fields are
// initialized to nil, providing default no-op behavior.
func NewMockStack(injector di.Injector) *MockStack {
	return &MockStack{BaseStack: BaseStack{injector: injector}}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize calls the mock InitializeFunc if set, otherwise returns nil.
// This allows tests to customize the initialization behavior by setting
// InitializeFunc to return specific errors or perform custom actions.
func (m *MockStack) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
}

// Up calls the mock UpFunc if set, otherwise returns nil.
// This allows tests to customize the up behavior by setting
// UpFunc to return specific errors or perform custom actions.
func (m *MockStack) Up() error {
	if m.UpFunc != nil {
		return m.UpFunc()
	}
	return nil
}

// Ensure MockStack implements Stack
var _ Stack = (*MockStack)(nil)
