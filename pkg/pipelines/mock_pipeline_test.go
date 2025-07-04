package pipelines

import (
	"context"
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewMockBasePipeline(t *testing.T) {
	t.Run("CreatesMockBasePipeline", func(t *testing.T) {
		// Given a new mock base pipeline is created
		mock := NewMockBasePipeline()

		// Then the mock should be created successfully
		if mock == nil {
			t.Fatal("Expected mock to not be nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMockBasePipeline_Initialize(t *testing.T) {
	t.Run("InitializeReturnsNilByDefault", func(t *testing.T) {
		// Given a mock base pipeline with no InitializeFunc set
		mock := NewMockBasePipeline()

		// When initializing the mock
		injector := di.NewInjector()
		err := mock.Initialize(injector)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("InitializeCallsInitializeFunc", func(t *testing.T) {
		// Given a mock base pipeline with InitializeFunc set
		mock := NewMockBasePipeline()
		called := false
		mock.InitializeFunc = func(injector di.Injector) error {
			called = true
			return nil
		}

		// When initializing the mock
		injector := di.NewInjector()
		err := mock.Initialize(injector)

		// Then InitializeFunc should be called
		if !called {
			t.Error("Expected InitializeFunc to be called")
		}

		// And no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("InitializeReturnsErrorFromInitializeFunc", func(t *testing.T) {
		// Given a mock base pipeline with InitializeFunc that returns an error
		mock := NewMockBasePipeline()
		expectedError := fmt.Errorf("test error")
		mock.InitializeFunc = func(injector di.Injector) error {
			return expectedError
		}

		// When initializing the mock
		injector := di.NewInjector()
		err := mock.Initialize(injector)

		// Then the error should be returned
		if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})
}

func TestMockBasePipeline_Execute(t *testing.T) {
	t.Run("ExecuteReturnsNilByDefault", func(t *testing.T) {
		// Given a mock base pipeline with no ExecuteFunc set
		mock := NewMockBasePipeline()

		// When executing the mock
		ctx := context.Background()
		err := mock.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ExecuteCallsExecuteFunc", func(t *testing.T) {
		// Given a mock base pipeline with ExecuteFunc set
		mock := NewMockBasePipeline()
		called := false
		mock.ExecuteFunc = func(ctx context.Context) error {
			called = true
			return nil
		}

		// When executing the mock
		ctx := context.Background()
		err := mock.Execute(ctx)

		// Then ExecuteFunc should be called
		if !called {
			t.Error("Expected ExecuteFunc to be called")
		}

		// And no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ExecuteReturnsErrorFromExecuteFunc", func(t *testing.T) {
		// Given a mock base pipeline with ExecuteFunc that returns an error
		mock := NewMockBasePipeline()
		expectedError := fmt.Errorf("test error")
		mock.ExecuteFunc = func(ctx context.Context) error {
			return expectedError
		}

		// When executing the mock
		ctx := context.Background()
		err := mock.Execute(ctx)

		// Then the error should be returned
		if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})
}
