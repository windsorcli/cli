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

func TestNewMockCheckPipeline(t *testing.T) {
	t.Run("CreatesNewMockCheckPipeline", func(t *testing.T) {
		mock := NewMockCheckPipeline()

		if mock == nil {
			t.Fatal("Expected mock to not be nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMockCheckPipeline_Initialize(t *testing.T) {
	t.Run("CallsInitializeFuncWhenSet", func(t *testing.T) {
		mock := NewMockCheckPipeline()
		called := false
		mock.InitializeFunc = func(di.Injector) error {
			called = true
			return nil
		}

		err := mock.Initialize(di.NewMockInjector())

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !called {
			t.Error("Expected InitializeFunc to be called")
		}
	})

	t.Run("ReturnsErrorFromInitializeFunc", func(t *testing.T) {
		mock := NewMockCheckPipeline()
		expectedError := fmt.Errorf("initialize error")
		mock.InitializeFunc = func(di.Injector) error {
			return expectedError
		}

		err := mock.Initialize(di.NewMockInjector())

		if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("ReturnsNilWhenInitializeFuncNotSet", func(t *testing.T) {
		mock := NewMockCheckPipeline()

		err := mock.Initialize(di.NewMockInjector())

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestMockCheckPipeline_Execute(t *testing.T) {
	t.Run("CallsExecuteFuncWhenSet", func(t *testing.T) {
		mock := NewMockCheckPipeline()
		called := false
		mock.ExecuteFunc = func(context.Context) error {
			called = true
			return nil
		}

		err := mock.Execute(context.Background())

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !called {
			t.Error("Expected ExecuteFunc to be called")
		}
	})

	t.Run("ReturnsErrorFromExecuteFunc", func(t *testing.T) {
		mock := NewMockCheckPipeline()
		expectedError := fmt.Errorf("execute error")
		mock.ExecuteFunc = func(context.Context) error {
			return expectedError
		}

		err := mock.Execute(context.Background())

		if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("ReturnsNilWhenExecuteFuncNotSet", func(t *testing.T) {
		mock := NewMockCheckPipeline()

		err := mock.Execute(context.Background())

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}
