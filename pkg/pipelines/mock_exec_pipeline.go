package pipelines

import (
	"context"

	"github.com/windsorcli/cli/pkg/di"
)

// MockExecPipeline is a mock implementation of ExecPipeline for testing
type MockExecPipeline struct {
	InitializeFunc func(di.Injector) error
	ExecuteFunc    func(context.Context) error
}

// NewMockExecPipeline creates a new mock exec pipeline
func NewMockExecPipeline() *MockExecPipeline {
	return &MockExecPipeline{
		InitializeFunc: func(di.Injector) error {
			return nil
		},
		ExecuteFunc: func(context.Context) error {
			return nil
		},
	}
}

// Initialize calls the mock function
func (m *MockExecPipeline) Initialize(injector di.Injector) error {
	return m.InitializeFunc(injector)
}

// Execute calls the mock function
func (m *MockExecPipeline) Execute(ctx context.Context) error {
	return m.ExecuteFunc(ctx)
}
