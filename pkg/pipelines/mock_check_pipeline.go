package pipelines

import (
	"context"

	"github.com/windsorcli/cli/pkg/di"
)

// MockCheckPipeline is a mock implementation of the CheckPipeline for testing purposes.
type MockCheckPipeline struct {
	InitializeFunc func(di.Injector) error
	ExecuteFunc    func(context.Context) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockCheckPipeline creates a new MockCheckPipeline instance.
func NewMockCheckPipeline() *MockCheckPipeline {
	return &MockCheckPipeline{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize calls the mock InitializeFunc if set, otherwise returns nil.
func (m *MockCheckPipeline) Initialize(injector di.Injector) error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc(injector)
	}
	return nil
}

// Execute calls the mock ExecuteFunc if set, otherwise returns nil.
func (m *MockCheckPipeline) Execute(ctx context.Context) error {
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(ctx)
	}
	return nil
}

// Ensure MockCheckPipeline implements Pipeline.
var _ Pipeline = (*MockCheckPipeline)(nil)
