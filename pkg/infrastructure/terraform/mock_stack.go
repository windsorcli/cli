package terraform

import (
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
)

// The MockStack is a test implementation of the Stack interface.
// It provides function fields that can be set to customize behavior in tests,
// The MockStack acts as a controllable test double for the Stack interface,
// enabling precise control over Initialize and Up behaviors in unit tests.

// =============================================================================
// Types
// =============================================================================

// MockStack is a mock implementation of the Stack interface for testing.
type MockStack struct {
	InitializeFunc func() error
	UpFunc         func(blueprint *blueprintv1alpha1.Blueprint) error
	DownFunc       func(blueprint *blueprintv1alpha1.Blueprint) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockStack creates a new mock stack.
func NewMockStack(injector di.Injector) *MockStack {
	return &MockStack{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize is a mock implementation of the Initialize method.
func (m *MockStack) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
}

// Up is a mock implementation of the Up method.
func (m *MockStack) Up(blueprint *blueprintv1alpha1.Blueprint) error {
	if m.UpFunc != nil {
		return m.UpFunc(blueprint)
	}
	return nil
}

// Down is a mock implementation of the Down method.
func (m *MockStack) Down(blueprint *blueprintv1alpha1.Blueprint) error {
	if m.DownFunc != nil {
		return m.DownFunc(blueprint)
	}
	return nil
}

// Ensure MockStack implements Stack
var _ Stack = (*MockStack)(nil)
