package runtime

import (
	"errors"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// The RuntimeTest is a test suite for the Runtime struct and its chaining methods.
// It provides comprehensive test coverage for dependency loading, error propagation,
// and command execution in the Windsor CLI runtime system.
// The RuntimeTest acts as a validation framework for runtime functionality,
// ensuring reliable dependency management, proper error handling, and method chaining.

// =============================================================================
// Test Setup
// =============================================================================

// setupMocks creates a new set of mocks for testing
func setupMocks(t *testing.T) *Dependencies {
	t.Helper()

	return &Dependencies{
		Injector:      di.NewInjector(),
		Shell:         shell.NewMockShell(),
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestRuntime_NewRuntime(t *testing.T) {
	t.Run("CreatesRuntimeWithDependencies", func(t *testing.T) {
		// Given dependencies
		mocks := setupMocks(t)

		// When creating a new runtime
		runtime := NewRuntime(mocks)

		// Then runtime should be created successfully
		if runtime == nil {
			t.Error("Expected runtime to be created")
		}

		if runtime.Injector != mocks.Injector {
			t.Error("Expected injector to be set")
		}
	})
}

func TestRuntime_LoadShell(t *testing.T) {
	t.Run("LoadsShellSuccessfully", func(t *testing.T) {
		// Given a runtime with dependencies
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks)

		// When loading shell
		result := runtime.LoadShell()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadShell to return the same runtime instance")
		}

		// And shell should be loaded
		if runtime.Shell == nil {
			t.Error("Expected shell to be loaded")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}
	})

	t.Run("CreatesNewShellWhenNoneExists", func(t *testing.T) {
		// Given a runtime without pre-loaded shell
		runtime := NewRuntime()

		// When loading shell
		result := runtime.LoadShell()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadShell to return the same runtime instance")
		}

		// And shell should be loaded
		if runtime.Shell == nil {
			t.Error("Expected shell to be loaded")
		}

		// And shell should be registered in injector
		resolvedShell := runtime.Injector.Resolve("shell")
		if resolvedShell == nil {
			t.Error("Expected shell to be registered in injector")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}
	})

	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error (no pre-loaded dependencies)
		runtime := NewRuntime()
		runtime.err = errors.New("existing error")

		// When loading shell
		result := runtime.LoadShell()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadShell to return the same runtime instance")
		}

		// And shell should not be loaded
		if runtime.Shell != nil {
			t.Error("Expected shell to not be loaded when error exists")
		}

		// And original error should be preserved
		if runtime.err.Error() != "existing error" {
			t.Errorf("Expected original error to be preserved, got %v", runtime.err)
		}
	})
}

func TestRuntime_InstallHook(t *testing.T) {
	t.Run("InstallsHookSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded shell
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks.Injector).LoadShell()

		// When installing hook
		result := runtime.InstallHook("bash")

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected InstallHook to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}
	})

	t.Run("ReturnsErrorWhenShellNotLoaded", func(t *testing.T) {
		// Given a runtime without loaded shell
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks.Injector)

		// When installing hook
		result := runtime.InstallHook("bash")

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected InstallHook to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error when shell not loaded")
		}

		expectedError := "shell not loaded - call LoadShell() first"
		if runtime.err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, runtime.err.Error())
		}
	})

	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks.Injector)
		runtime.err = errors.New("existing error")

		// When installing hook
		result := runtime.InstallHook("bash")

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected InstallHook to return the same runtime instance")
		}

		// And original error should be preserved
		if runtime.err.Error() != "existing error" {
			t.Errorf("Expected original error to be preserved, got %v", runtime.err)
		}
	})
}

func TestRuntime_Do(t *testing.T) {
	t.Run("ReturnsNilWhenNoError", func(t *testing.T) {
		// Given a runtime with no error
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks)

		// When calling Do
		err := runtime.Do()

		// Then should return nil
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenErrorSet", func(t *testing.T) {
		// Given a runtime with an error
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks)
		expectedError := errors.New("test error")
		runtime.err = expectedError

		// When calling Do
		err := runtime.Do()

		// Then should return the error
		if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})
}
