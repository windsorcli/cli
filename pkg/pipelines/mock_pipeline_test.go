package pipelines

import (
	"context"
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupMockBasePipeline(t *testing.T) *MockBasePipeline {
	t.Helper()

	pipeline := NewMockBasePipeline()

	return pipeline
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewMockBasePipeline(t *testing.T) {
	t.Run("CreatesWithDefaults", func(t *testing.T) {
		// Given creating a new mock base pipeline
		pipeline := NewMockBasePipeline()

		// Then pipeline should not be nil
		if pipeline == nil {
			t.Fatal("Expected pipeline to not be nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMockBasePipeline_Initialize(t *testing.T) {
	t.Run("CallsInitializeFuncWhenSet", func(t *testing.T) {
		// Given a mock pipeline with custom initialize function
		pipeline := setupMockBasePipeline(t)

		initializeCalled := false
		var capturedInjector di.Injector
		var capturedCtx context.Context
		pipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error {
			initializeCalled = true
			capturedInjector = injector
			capturedCtx = ctx
			return nil
		}

		injector := di.NewInjector()
		ctx := context.Background()

		// When initializing the pipeline
		err := pipeline.Initialize(injector, ctx)

		// Then the custom function should be called
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !initializeCalled {
			t.Error("Expected InitializeFunc to be called")
		}
		if capturedInjector != injector {
			t.Error("Expected injector to be passed to InitializeFunc")
		}
		if capturedCtx != ctx {
			t.Error("Expected context to be passed to InitializeFunc")
		}
	})

	t.Run("ReturnsErrorWhenInitializeFuncFails", func(t *testing.T) {
		// Given a mock pipeline with failing initialize function
		pipeline := setupMockBasePipeline(t)

		pipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error {
			return fmt.Errorf("initialize failed")
		}

		injector := di.NewInjector()
		ctx := context.Background()

		// When initializing the pipeline
		err := pipeline.Initialize(injector, ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "initialize failed" {
			t.Errorf("Expected 'initialize failed', got: %v", err)
		}
	})

	t.Run("ReturnsNilWhenInitializeFuncNotSet", func(t *testing.T) {
		// Given a mock pipeline without custom initialize function
		pipeline := setupMockBasePipeline(t)

		injector := di.NewInjector()
		ctx := context.Background()

		// When initializing the pipeline
		err := pipeline.Initialize(injector, ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestMockBasePipeline_Execute(t *testing.T) {
	t.Run("CallsExecuteFuncWhenSet", func(t *testing.T) {
		// Given a mock pipeline with custom execute function
		pipeline := setupMockBasePipeline(t)

		executeCalled := false
		var capturedCtx context.Context
		pipeline.ExecuteFunc = func(ctx context.Context) error {
			executeCalled = true
			capturedCtx = ctx
			return nil
		}

		ctx := context.Background()

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then the custom function should be called
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !executeCalled {
			t.Error("Expected ExecuteFunc to be called")
		}
		if capturedCtx != ctx {
			t.Error("Expected context to be passed to ExecuteFunc")
		}
	})

	t.Run("ReturnsErrorWhenExecuteFuncFails", func(t *testing.T) {
		// Given a mock pipeline with failing execute function
		pipeline := setupMockBasePipeline(t)

		pipeline.ExecuteFunc = func(ctx context.Context) error {
			return fmt.Errorf("execute failed")
		}

		ctx := context.Background()

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "execute failed" {
			t.Errorf("Expected 'execute failed', got: %v", err)
		}
	})

	t.Run("ReturnsNilWhenExecuteFuncNotSet", func(t *testing.T) {
		// Given a mock pipeline without custom execute function
		pipeline := setupMockBasePipeline(t)

		ctx := context.Background()

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}
