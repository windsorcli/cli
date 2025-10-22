package runtime

import (
	"errors"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
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

func TestRuntime_InstallHook(t *testing.T) {
	t.Run("InstallsHookSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded shell
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell()

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
		// Given a runtime without loaded shell (no pre-loaded dependencies)
		runtime := NewRuntime()

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
		// Given a runtime with an existing error (no pre-loaded dependencies)
		runtime := NewRuntime()
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

func TestRuntime_SetContext(t *testing.T) {
	t.Run("SetsContextSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded config handler
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

		// When setting context
		result := runtime.SetContext("test-context")

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected SetContext to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And SetContext should have been called on the config handler
		// (We can't easily track this without modifying the mock, so we just verify no error occurred)
	})

	t.Run("ReturnsErrorWhenConfigHandlerNotLoaded", func(t *testing.T) {
		// Given a runtime without loaded config handler (no pre-loaded dependencies)
		runtime := NewRuntime()

		// When setting context
		result := runtime.SetContext("test-context")

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected SetContext to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error when config handler not loaded")
		}

		expectedError := "config handler not loaded - call LoadConfig() first"
		if runtime.err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, runtime.err.Error())
		}
	})

	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error (no pre-loaded dependencies)
		runtime := NewRuntime()
		runtime.err = errors.New("existing error")

		// When setting context
		result := runtime.SetContext("test-context")

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected SetContext to return the same runtime instance")
		}

		// And original error should be preserved
		if runtime.err.Error() != "existing error" {
			t.Errorf("Expected original error to be preserved, got %v", runtime.err)
		}
	})

	t.Run("PropagatesConfigHandlerError", func(t *testing.T) {
		// Given a runtime with a mock shell that returns an error
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", errors.New("project root error")
		}

		// Create runtime with only the mock shell, no mock config handler
		runtime := NewRuntime()
		runtime.Shell = mockShell
		runtime.Injector.Register("shell", mockShell)
		runtime.LoadConfig()

		// When setting context
		result := runtime.SetContext("test-context")

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected SetContext to return the same runtime instance")
		}

		// And error should be propagated from config handler
		if runtime.err == nil {
			t.Error("Expected error to be propagated from config handler")
		} else {
			expectedError := "failed to load configuration"
			if !strings.Contains(runtime.err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, runtime.err.Error())
			}
		}
	})
}

func TestRuntime_PrintContext(t *testing.T) {
	t.Run("PrintsContextSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded config handler
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return "test-context"
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

		var output string
		outputFunc := func(s string) {
			output = s
		}

		// When printing context
		result := runtime.PrintContext(outputFunc)

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected PrintContext to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And output should be correct
		if output != "test-context" {
			t.Errorf("Expected output 'test-context', got %q", output)
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerNotLoaded", func(t *testing.T) {
		// Given a runtime without loaded config handler (no pre-loaded dependencies)
		runtime := NewRuntime()

		var output string
		outputFunc := func(s string) {
			output = s
		}

		// When printing context
		result := runtime.PrintContext(outputFunc)

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected PrintContext to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error when config handler not loaded")
		}

		expectedError := "config handler not loaded - call LoadConfig() first"
		if runtime.err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, runtime.err.Error())
		}

		// And output should not be set
		if output != "" {
			t.Errorf("Expected no output, got %q", output)
		}
	})

	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error (no pre-loaded dependencies)
		runtime := NewRuntime()
		runtime.err = errors.New("existing error")

		var output string
		outputFunc := func(s string) {
			output = s
		}

		// When printing context
		result := runtime.PrintContext(outputFunc)

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected PrintContext to return the same runtime instance")
		}

		// And original error should be preserved
		if runtime.err.Error() != "existing error" {
			t.Errorf("Expected original error to be preserved, got %v", runtime.err)
		}

		// And output should not be set
		if output != "" {
			t.Errorf("Expected no output, got %q", output)
		}
	})
}

func TestRuntime_WriteResetToken(t *testing.T) {
	t.Run("WritesResetTokenSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded shell
		mocks := setupMocks(t)
		mocks.Shell.(*shell.MockShell).WriteResetTokenFunc = func() (string, error) {
			return "/tmp/reset-token", nil
		}
		runtime := NewRuntime(mocks).LoadShell()

		// When writing reset token
		result := runtime.WriteResetToken()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WriteResetToken to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And WriteResetToken should have been called on the shell
		// (We can't easily track this without modifying the mock, so we just verify no error occurred)
	})

	t.Run("ReturnsErrorWhenShellNotLoaded", func(t *testing.T) {
		// Given a runtime without loaded shell (no pre-loaded dependencies)
		runtime := NewRuntime()

		// When writing reset token
		result := runtime.WriteResetToken()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WriteResetToken to return the same runtime instance")
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
		// Given a runtime with an existing error (no pre-loaded dependencies)
		runtime := NewRuntime()
		runtime.err = errors.New("existing error")

		// When writing reset token
		result := runtime.WriteResetToken()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WriteResetToken to return the same runtime instance")
		}

		// And original error should be preserved
		if runtime.err.Error() != "existing error" {
			t.Errorf("Expected original error to be preserved, got %v", runtime.err)
		}
	})

	t.Run("PropagatesShellError", func(t *testing.T) {
		// Given a runtime with loaded shell that returns an error
		mocks := setupMocks(t)
		mocks.Shell.(*shell.MockShell).WriteResetTokenFunc = func() (string, error) {
			return "", errors.New("shell error")
		}
		runtime := NewRuntime(mocks).LoadShell()

		// When writing reset token
		result := runtime.WriteResetToken()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WriteResetToken to return the same runtime instance")
		}

		// And error should be propagated
		if runtime.err == nil {
			t.Error("Expected error to be propagated from shell")
		} else {
			expectedError := "shell error"
			if runtime.err.Error() != expectedError {
				t.Errorf("Expected error %q, got %q", expectedError, runtime.err.Error())
			}
		}
	})
}

func TestRuntime_LoadBlueprint(t *testing.T) {
	t.Run("LoadsBlueprintSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded dependencies
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return "mock-string"
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfig().LoadKubernetes()

		// When loading blueprint
		result := runtime.LoadBlueprint()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadBlueprint to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And blueprint handler should be created and registered
		if runtime.BlueprintHandler == nil {
			t.Error("Expected blueprint handler to be created")
		}

		// And artifact builder should be created and registered
		if runtime.ArtifactBuilder == nil {
			t.Error("Expected artifact builder to be created")
		}

		// And components should be registered in injector
		if runtime.Injector.Resolve("blueprintHandler") == nil {
			t.Error("Expected blueprint handler to be registered in injector")
		}

		if runtime.Injector.Resolve("artifactBuilder") == nil {
			t.Error("Expected artifact builder to be registered in injector")
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerNotLoaded", func(t *testing.T) {
		// Given a runtime without loaded config handler
		runtime := NewRuntime()

		// When loading blueprint
		result := runtime.LoadBlueprint()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadBlueprint to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error when config handler not loaded")
		}

		expectedError := "config handler not loaded - call LoadConfig() first"
		if runtime.err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, runtime.err.Error())
		}

		// And components should not be created
		if runtime.BlueprintHandler != nil {
			t.Error("Expected blueprint handler to not be created")
		}

		if runtime.ArtifactBuilder != nil {
			t.Error("Expected artifact builder to not be created")
		}
	})

	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error
		runtime := NewRuntime()
		runtime.err = errors.New("existing error")

		// When loading blueprint
		result := runtime.LoadBlueprint()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadBlueprint to return the same runtime instance")
		}

		// And original error should be preserved
		if runtime.err.Error() != "existing error" {
			t.Errorf("Expected original error to be preserved, got %v", runtime.err)
		}

		// And components should not be created
		if runtime.BlueprintHandler != nil {
			t.Error("Expected blueprint handler to not be created")
		}

		if runtime.ArtifactBuilder != nil {
			t.Error("Expected artifact builder to not be created")
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
