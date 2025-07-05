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

func TestNewMockContextPipeline(t *testing.T) {
	t.Run("CreatesMockContextPipeline", func(t *testing.T) {
		mock := NewMockContextPipeline()

		if mock == nil {
			t.Fatal("Expected mock to not be nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMockContextPipeline_Initialize(t *testing.T) {
	t.Run("InitializeReturnsNilByDefault", func(t *testing.T) {
		mock := NewMockContextPipeline()

		injector := di.NewInjector()
		err := mock.Initialize(injector)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("InitializeCallsInitializeFunc", func(t *testing.T) {
		mock := NewMockContextPipeline()
		called := false
		mock.InitializeFunc = func(injector di.Injector) error {
			called = true
			return nil
		}

		injector := di.NewInjector()
		err := mock.Initialize(injector)

		if !called {
			t.Error("Expected InitializeFunc to be called")
		}

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("InitializeReturnsErrorFromInitializeFunc", func(t *testing.T) {
		mock := NewMockContextPipeline()
		expectedError := fmt.Errorf("test error")
		mock.InitializeFunc = func(injector di.Injector) error {
			return expectedError
		}

		injector := di.NewInjector()
		err := mock.Initialize(injector)

		if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})
}

func TestMockContextPipeline_Execute(t *testing.T) {
	t.Run("ExecuteReturnsNilByDefault", func(t *testing.T) {
		mock := NewMockContextPipeline()

		ctx := context.Background()
		err := mock.Execute(ctx)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ExecuteCallsExecuteFunc", func(t *testing.T) {
		mock := NewMockContextPipeline()
		called := false
		mock.ExecuteFunc = func(ctx context.Context) error {
			called = true
			return nil
		}

		ctx := context.Background()
		err := mock.Execute(ctx)

		if !called {
			t.Error("Expected ExecuteFunc to be called")
		}

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ExecuteReturnsErrorFromExecuteFunc", func(t *testing.T) {
		mock := NewMockContextPipeline()
		expectedError := fmt.Errorf("test error")
		mock.ExecuteFunc = func(ctx context.Context) error {
			return expectedError
		}

		ctx := context.Background()
		err := mock.Execute(ctx)

		if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})
}
