package pipelines

import (
	"context"
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/context/config"
)

// =============================================================================
// Test Setup
// =============================================================================

type ExecMocks struct {
	*Mocks
}

func setupExecMocks(t *testing.T, opts ...*SetupOptions) *ExecMocks {
	t.Helper()

	// Create setup options, preserving any provided options
	setupOptions := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		setupOptions = opts[0]
	}

	// Only create a default mock config handler if one wasn't provided
	if setupOptions.ConfigHandler == nil {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error { return nil }
		mockConfigHandler.IsLoadedFunc = func() bool { return true } // Default to loaded
		setupOptions.ConfigHandler = mockConfigHandler
	}

	baseMocks := setupMocks(t, setupOptions)

	return &ExecMocks{
		Mocks: baseMocks,
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewExecPipeline(t *testing.T) {
	t.Run("CreatesWithDefaults", func(t *testing.T) {
		// Given creating a new exec pipeline
		pipeline := NewExecPipeline()

		// Then pipeline should not be nil
		if pipeline == nil {
			t.Fatal("Expected pipeline to not be nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestExecPipeline_Execute(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*ExecPipeline, *ExecMocks) {
		t.Helper()
		pipeline := NewExecPipeline()
		mocks := setupExecMocks(t, opts...)

		// Initialize the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		return pipeline, mocks
	}

	t.Run("ExecutesCommandSuccessfully", func(t *testing.T) {
		// Given an exec pipeline with a mock shell
		pipeline, mocks := setup(t)

		execCalled := false
		var capturedCommand string
		var capturedArgs []string
		mocks.Shell.ExecFunc = func(command string, args ...string) (string, error) {
			execCalled = true
			capturedCommand = command
			capturedArgs = args
			return "command output", nil
		}

		ctx := context.WithValue(context.Background(), "command", "test-command")
		ctx = context.WithValue(ctx, "args", []string{"arg1", "arg2"})

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned and command should be executed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !execCalled {
			t.Error("Expected shell.Exec to be called")
		}
		if capturedCommand != "test-command" {
			t.Errorf("Expected command 'test-command', got '%s'", capturedCommand)
		}
		if len(capturedArgs) != 2 || capturedArgs[0] != "arg1" || capturedArgs[1] != "arg2" {
			t.Errorf("Expected args ['arg1', 'arg2'], got %v", capturedArgs)
		}
	})

	t.Run("ExecutesCommandWithoutArgs", func(t *testing.T) {
		// Given an exec pipeline with a mock shell and no args
		pipeline, mocks := setup(t)

		execCalled := false
		var capturedCommand string
		var capturedArgs []string
		mocks.Shell.ExecFunc = func(command string, args ...string) (string, error) {
			execCalled = true
			capturedCommand = command
			capturedArgs = args
			return "command output", nil
		}

		ctx := context.WithValue(context.Background(), "command", "test-command")

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned and command should be executed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !execCalled {
			t.Error("Expected shell.Exec to be called")
		}
		if capturedCommand != "test-command" {
			t.Errorf("Expected command 'test-command', got '%s'", capturedCommand)
		}
		if len(capturedArgs) != 0 {
			t.Errorf("Expected no args, got %v", capturedArgs)
		}
	})

	t.Run("ReturnsErrorWhenNoCommandProvided", func(t *testing.T) {
		// Given an exec pipeline with no command in context
		pipeline, _ := setup(t)

		ctx := context.Background()

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "no command provided in context" {
			t.Errorf("Expected 'no command provided in context', got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenCommandIsEmpty", func(t *testing.T) {
		// Given an exec pipeline with empty command
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "command", "")

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "no command provided in context" {
			t.Errorf("Expected 'no command provided in context', got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenCommandIsNotString", func(t *testing.T) {
		// Given an exec pipeline with non-string command
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "command", 123)

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "no command provided in context" {
			t.Errorf("Expected 'no command provided in context', got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenShellExecFails", func(t *testing.T) {
		// Given an exec pipeline with failing shell exec
		pipeline, mocks := setup(t)

		mocks.Shell.ExecFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("exec failed")
		}

		ctx := context.WithValue(context.Background(), "command", "test-command")

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "command execution failed: exec failed" {
			t.Errorf("Expected 'command execution failed: exec failed', got: %v", err)
		}
	})

	t.Run("HandlesArgsAsNonSliceType", func(t *testing.T) {
		// Given an exec pipeline with args as non-slice type
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "command", "test-command")
		ctx = context.WithValue(ctx, "args", "not-a-slice")

		// When executing the pipeline
		// Then it should panic due to invalid type assertion
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic due to invalid type assertion")
			}
		}()

		pipeline.Execute(ctx)
	})
}
