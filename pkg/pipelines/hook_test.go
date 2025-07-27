package pipelines

import (
	"context"
	"fmt"
	"testing"
)

// =============================================================================
// Test Setup Infrastructure
// =============================================================================

// HookMocks extends the base Mocks with hook-specific dependencies
type HookMocks struct {
	*Mocks
}

// setupHookShims creates shims for hook pipeline tests
func setupHookShims(t *testing.T) *Shims {
	t.Helper()
	return setupShims(t)
}

// setupHookMocks creates mocks for hook pipeline tests
func setupHookMocks(t *testing.T, opts ...*SetupOptions) *HookMocks {
	t.Helper()

	baseMocks := setupMocks(t, opts...)

	return &HookMocks{
		Mocks: baseMocks,
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewHookPipeline(t *testing.T) {
	t.Run("CreatesWithDefaults", func(t *testing.T) {
		// Given creating a new hook pipeline
		pipeline := NewHookPipeline()

		// Then pipeline should not be nil
		if pipeline == nil {
			t.Fatal("Expected pipeline to not be nil")
		}
	})
}

// =============================================================================
// Test Public Methods - Execute
// =============================================================================

func TestHookPipeline_Execute(t *testing.T) {
	setup := func(t *testing.T) (*HookPipeline, *HookMocks) {
		pipeline := NewHookPipeline()
		mocks := setupHookMocks(t)

		// Set up the pipeline with mocks
		pipeline.shell = mocks.Shell

		return pipeline, mocks
	}

	t.Run("InstallsHookSuccessfully", func(t *testing.T) {
		// Given a hook pipeline with a mock shell
		pipeline, mocks := setup(t)

		installHookCalled := false
		var capturedShellName string
		mocks.Shell.InstallHookFunc = func(shellName string) error {
			installHookCalled = true
			capturedShellName = shellName
			return nil
		}

		ctx := context.WithValue(context.Background(), "shellType", "bash")

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned and hook should be installed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !installHookCalled {
			t.Error("Expected shell.InstallHook to be called")
		}
		if capturedShellName != "bash" {
			t.Errorf("Expected shell name 'bash', got '%s'", capturedShellName)
		}
	})

	t.Run("ReturnsErrorWhenNoShellTypeProvided", func(t *testing.T) {
		// Given a hook pipeline with no shell type in context
		pipeline, _ := setup(t)

		ctx := context.Background()

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "No shell name provided" {
			t.Errorf("Expected 'No shell name provided', got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenShellTypeIsNotString", func(t *testing.T) {
		// Given a hook pipeline with non-string shell type
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "shellType", 123)

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Invalid shell name type" {
			t.Errorf("Expected 'Invalid shell name type', got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenInstallHookFails", func(t *testing.T) {
		// Given a hook pipeline with failing install hook
		pipeline, mocks := setup(t)

		mocks.Shell.InstallHookFunc = func(shellName string) error {
			return fmt.Errorf("install hook failed")
		}

		ctx := context.WithValue(context.Background(), "shellType", "bash")

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "install hook failed" {
			t.Errorf("Expected 'install hook failed', got: %v", err)
		}
	})
}
