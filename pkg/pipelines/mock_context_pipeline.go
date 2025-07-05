package pipelines

import (
	"context"

	"github.com/windsorcli/cli/pkg/di"
)

// MockContextPipeline is a mock implementation of the ContextPipeline
type MockContextPipeline struct {
	InitializeFunc func(injector di.Injector) error
	ExecuteFunc    func(ctx context.Context) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockContextPipeline creates a new MockContextPipeline instance
func NewMockContextPipeline() *MockContextPipeline {
	return &MockContextPipeline{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize calls the mock InitializeFunc if set, otherwise returns nil
func (m *MockContextPipeline) Initialize(injector di.Injector) error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc(injector)
	}
	return nil
}

// Execute calls the mock ExecuteFunc if set, otherwise returns nil
func (m *MockContextPipeline) Execute(ctx context.Context) error {
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(ctx)
	}
	return nil
}

// Ensure MockContextPipeline implements Pipeline
var _ Pipeline = (*MockContextPipeline)(nil)
