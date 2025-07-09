package pipelines

import (
	"context"
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupHookPipeline(t *testing.T) *HookPipeline {
	t.Helper()

	pipeline := NewHookPipeline()

	return pipeline
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
// Test Public Methods
// =============================================================================

func TestHookPipeline_Execute(t *testing.T) {
	t.Run("InstallsHookSuccessfully", func(t *testing.T) {
		// Given a hook pipeline with a mock shell
		pipeline := setupHookPipeline(t)

		mockShell := shell.NewMockShell()
		installHookCalled := false
		var capturedShellName string
		mockShell.InstallHookFunc = func(shellName string) error {
			installHookCalled = true
			capturedShellName = shellName
			return nil
		}
		pipeline.shell = mockShell

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
		pipeline := setupHookPipeline(t)

		mockShell := shell.NewMockShell()
		pipeline.shell = mockShell

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
		pipeline := setupHookPipeline(t)

		mockShell := shell.NewMockShell()
		pipeline.shell = mockShell

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
		pipeline := setupHookPipeline(t)

		mockShell := shell.NewMockShell()
		mockShell.InstallHookFunc = func(shellName string) error {
			return fmt.Errorf("install hook failed")
		}
		pipeline.shell = mockShell

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
