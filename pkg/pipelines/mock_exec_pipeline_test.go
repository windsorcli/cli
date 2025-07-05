package pipelines

import (
	"context"
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
)

func TestMockExecPipeline_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		injector := di.NewInjector()
		pipeline := NewMockExecPipeline()

		err := pipeline.Initialize(injector)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("CustomInitializeFunc", func(t *testing.T) {
		injector := di.NewInjector()
		pipeline := NewMockExecPipeline()
		pipeline.InitializeFunc = func(di.Injector) error {
			return fmt.Errorf("custom initialize error")
		}

		err := pipeline.Initialize(injector)

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "custom initialize error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})
}

func TestMockExecPipeline_Execute(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		pipeline := NewMockExecPipeline()

		err := pipeline.Execute(ctx)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("CustomExecuteFunc", func(t *testing.T) {
		ctx := context.Background()
		pipeline := NewMockExecPipeline()
		pipeline.ExecuteFunc = func(context.Context) error {
			return fmt.Errorf("custom execute error")
		}

		err := pipeline.Execute(ctx)

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "custom execute error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})
}
