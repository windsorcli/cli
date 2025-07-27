package pipelines

import (
	"context"

	"github.com/windsorcli/cli/pkg/di"
)

// MockBasePipeline is a mock implementation of the Pipeline interface
type MockBasePipeline struct {
	InitializeFunc func(injector di.Injector, ctx context.Context) error
	ExecuteFunc    func(ctx context.Context) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockBasePipeline creates a new MockBasePipeline instance
func NewMockBasePipeline() *MockBasePipeline {
	return &MockBasePipeline{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize calls the mock InitializeFunc if set, otherwise returns nil
func (m *MockBasePipeline) Initialize(injector di.Injector, ctx context.Context) error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc(injector, ctx)
	}
	return nil
}

// Execute calls the mock ExecuteFunc if set, otherwise returns nil
func (m *MockBasePipeline) Execute(ctx context.Context) error {
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(ctx)
	}
	return nil
}

// Ensure MockBasePipeline implements Pipeline
var _ Pipeline = (*MockBasePipeline)(nil)
