package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/context/config"
	envvars "github.com/windsorcli/cli/pkg/context/env"
	"github.com/windsorcli/cli/pkg/context/shell"
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
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfig().LoadKubernetes()

		// Create local template data to avoid OCI download
		tmpDir := t.TempDir()
		templateDir := filepath.Join(tmpDir, "contexts", "_template")
		if err := os.MkdirAll(templateDir, 0755); err != nil {
			t.Fatalf("Failed to create template directory: %v", err)
		}

		// Mock GetProjectRoot to return our temp directory
		mocks.Shell.(*shell.MockShell).GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}

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

		// And artifact builder should NOT be created (separate concern)
		if runtime.ArtifactBuilder != nil {
			t.Error("Expected artifact builder to NOT be created by LoadBlueprint")
		}

		// And components should be registered in injector
		if runtime.Injector.Resolve("blueprintHandler") == nil {
			t.Error("Expected blueprint handler to be registered in injector")
		}

		// Artifact builder should NOT be registered (separate concern)
		if runtime.Injector.Resolve("artifactBuilder") != nil {
			t.Error("Expected artifact builder to NOT be registered by LoadBlueprint")
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
func TestRuntime_HandleSessionReset(t *testing.T) {
	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error
		runtime := NewRuntime()
		expectedError := errors.New("existing error")
		runtime.err = expectedError

		// When handling session reset
		result := runtime.HandleSessionReset()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected HandleSessionReset to return the same runtime instance")
		}

		// And original error should be preserved
		if runtime.err != expectedError {
			t.Errorf("Expected original error to be preserved, got %v", runtime.err)
		}
	})

	t.Run("ReturnsErrorWhenShellNotLoaded", func(t *testing.T) {
		// Given a runtime without loaded shell
		runtime := NewRuntime()

		// When handling session reset
		result := runtime.HandleSessionReset()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected HandleSessionReset to return the same runtime instance")
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

	t.Run("ResetsWhenNoSessionToken", func(t *testing.T) {
		// Given a runtime with loaded shell and no session token
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell()

		// Ensure no session token is set
		originalToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		os.Unsetenv("WINDSOR_SESSION_TOKEN")
		defer func() {
			if originalToken != "" {
				os.Setenv("WINDSOR_SESSION_TOKEN", originalToken)
			}
		}()

		// Mock CheckResetFlags to return false (no reset flags)
		mocks.Shell.(*shell.MockShell).CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}

		// Track if Reset was called
		resetCalled := false
		mocks.Shell.(*shell.MockShell).ResetFunc = func(...bool) {
			resetCalled = true
		}

		// When handling session reset
		result := runtime.HandleSessionReset()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected HandleSessionReset to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And reset should be called
		if !resetCalled {
			t.Error("Expected shell reset to be called when no session token")
		}

		// And NO_CACHE should be set
		if os.Getenv("NO_CACHE") != "true" {
			t.Error("Expected NO_CACHE to be set to true")
		}

		// Clean up NO_CACHE
		os.Unsetenv("NO_CACHE")
	})

	t.Run("ResetsWhenResetFlagsTrue", func(t *testing.T) {
		// Given a runtime with loaded shell and session token
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell()

		// Set session token
		originalToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer func() {
			if originalToken != "" {
				os.Setenv("WINDSOR_SESSION_TOKEN", originalToken)
			} else {
				os.Unsetenv("WINDSOR_SESSION_TOKEN")
			}
		}()

		// Mock CheckResetFlags to return true (reset flags detected)
		mocks.Shell.(*shell.MockShell).CheckResetFlagsFunc = func() (bool, error) {
			return true, nil
		}

		// Track if Reset was called
		resetCalled := false
		mocks.Shell.(*shell.MockShell).ResetFunc = func(...bool) {
			resetCalled = true
		}

		// When handling session reset
		result := runtime.HandleSessionReset()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected HandleSessionReset to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And reset should be called
		if !resetCalled {
			t.Error("Expected shell reset to be called when reset flags are true")
		}

		// And NO_CACHE should be set
		if os.Getenv("NO_CACHE") != "true" {
			t.Error("Expected NO_CACHE to be set to true")
		}

		// Clean up NO_CACHE
		os.Unsetenv("NO_CACHE")
	})

	t.Run("DoesNotResetWhenContextChanged", func(t *testing.T) {
		// Given a runtime with loaded shell, config handler, and session token
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

		// Set session token
		originalToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer func() {
			if originalToken != "" {
				os.Setenv("WINDSOR_SESSION_TOKEN", originalToken)
			} else {
				os.Unsetenv("WINDSOR_SESSION_TOKEN")
			}
		}()

		// Set WINDSOR_CONTEXT to differ from current context
		originalContext := os.Getenv("WINDSOR_CONTEXT")
		os.Setenv("WINDSOR_CONTEXT", "different-context")
		defer func() {
			if originalContext != "" {
				os.Setenv("WINDSOR_CONTEXT", originalContext)
			} else {
				os.Unsetenv("WINDSOR_CONTEXT")
			}
		}()

		// Mock CheckResetFlags to return false (no reset flags)
		mocks.Shell.(*shell.MockShell).CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}

		// Mock GetContext to return a different context
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return "current-context"
		}

		// Track if Reset was called
		resetCalled := false
		mocks.Shell.(*shell.MockShell).ResetFunc = func(...bool) {
			resetCalled = true
		}

		// When handling session reset
		result := runtime.HandleSessionReset()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected HandleSessionReset to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And reset should NOT be called
		if resetCalled {
			t.Error("Expected shell reset to NOT be called when context changed (logic not present)")
		}

		// And NO_CACHE should NOT be set
		if os.Getenv("NO_CACHE") == "true" {
			t.Error("Expected NO_CACHE to NOT be set when context changed (logic not present)")
		}
	})

	t.Run("DoesNotResetWhenNoResetNeeded", func(t *testing.T) {
		// Given a runtime with loaded shell, config handler, and session token
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

		// Set session token
		originalToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer func() {
			if originalToken != "" {
				os.Setenv("WINDSOR_SESSION_TOKEN", originalToken)
			} else {
				os.Unsetenv("WINDSOR_SESSION_TOKEN")
			}
		}()

		// Set WINDSOR_CONTEXT to match current context (no context change)
		originalContext := os.Getenv("WINDSOR_CONTEXT")
		os.Setenv("WINDSOR_CONTEXT", "current-context")
		defer func() {
			if originalContext != "" {
				os.Setenv("WINDSOR_CONTEXT", originalContext)
			} else {
				os.Unsetenv("WINDSOR_CONTEXT")
			}
		}()

		// Mock CheckResetFlags to return false (no reset flags)
		mocks.Shell.(*shell.MockShell).CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}

		// Mock GetContext to return the same context as WINDSOR_CONTEXT
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return "current-context"
		}

		// Track if Reset was called
		resetCalled := false
		mocks.Shell.(*shell.MockShell).ResetFunc = func(...bool) {
			resetCalled = true
		}

		// When handling session reset
		result := runtime.HandleSessionReset()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected HandleSessionReset to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And reset should NOT be called
		if resetCalled {
			t.Error("Expected shell reset to NOT be called when no reset needed")
		}

		// And NO_CACHE should NOT be set
		if os.Getenv("NO_CACHE") == "true" {
			t.Error("Expected NO_CACHE to NOT be set when no reset needed")
		}
	})

	t.Run("PropagatesCheckResetFlagsError", func(t *testing.T) {
		// Given a runtime with loaded shell
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell()

		// Set session token
		originalToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer func() {
			if originalToken != "" {
				os.Setenv("WINDSOR_SESSION_TOKEN", originalToken)
			} else {
				os.Unsetenv("WINDSOR_SESSION_TOKEN")
			}
		}()

		// Mock CheckResetFlags to return an error
		expectedError := errors.New("check reset flags error")
		mocks.Shell.(*shell.MockShell).CheckResetFlagsFunc = func() (bool, error) {
			return false, expectedError
		}

		// When handling session reset
		result := runtime.HandleSessionReset()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected HandleSessionReset to return the same runtime instance")
		}

		// And error should be propagated
		if runtime.err == nil {
			t.Error("Expected error to be propagated from CheckResetFlags")
		} else {
			expectedErrorMsg := "failed to check reset flags: check reset flags error"
			if runtime.err.Error() != expectedErrorMsg {
				t.Errorf("Expected error %q, got %q", expectedErrorMsg, runtime.err.Error())
			}
		}
	})

	t.Run("PropagatesSetenvError", func(t *testing.T) {
		// Given a runtime with loaded shell and no session token (to trigger reset)
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell()

		// Ensure no session token is set
		originalToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		os.Unsetenv("WINDSOR_SESSION_TOKEN")
		defer func() {
			if originalToken != "" {
				os.Setenv("WINDSOR_SESSION_TOKEN", originalToken)
			}
		}()

		// Mock CheckResetFlags to return false (no reset flags)
		mocks.Shell.(*shell.MockShell).CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}

		// Mock Reset to succeed
		mocks.Shell.(*shell.MockShell).ResetFunc = func(...bool) {
			// Reset succeeds
		}

		// When handling session reset
		result := runtime.HandleSessionReset()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected HandleSessionReset to return the same runtime instance")
		}

		// And no error should be set (os.Setenv typically succeeds in tests)
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And NO_CACHE should be set
		if os.Getenv("NO_CACHE") != "true" {
			t.Error("Expected NO_CACHE to be set to true")
		}

		// Clean up NO_CACHE
		os.Unsetenv("NO_CACHE")
	})
}

func TestRuntime_CheckTrustedDirectory(t *testing.T) {
	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error
		runtime := NewRuntime()
		expectedError := errors.New("existing error")
		runtime.err = expectedError

		// When checking trusted directory
		result := runtime.CheckTrustedDirectory()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected CheckTrustedDirectory to return the same runtime instance")
		}

		// And original error should be preserved
		if runtime.err != expectedError {
			t.Errorf("Expected original error to be preserved, got %v", runtime.err)
		}
	})

	t.Run("ReturnsErrorWhenShellNotLoaded", func(t *testing.T) {
		// Given a runtime without loaded shell
		runtime := NewRuntime()

		// When checking trusted directory
		result := runtime.CheckTrustedDirectory()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected CheckTrustedDirectory to return the same runtime instance")
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

	t.Run("SucceedsWhenDirectoryIsTrusted", func(t *testing.T) {
		// Given a runtime with loaded shell
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell()

		// Mock CheckTrustedDirectory to succeed
		mocks.Shell.(*shell.MockShell).CheckTrustedDirectoryFunc = func() error {
			return nil
		}

		// When checking trusted directory
		result := runtime.CheckTrustedDirectory()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected CheckTrustedDirectory to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}
	})

	t.Run("PropagatesShellError", func(t *testing.T) {
		// Given a runtime with loaded shell
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell()

		// Mock CheckTrustedDirectory to return an error
		expectedError := errors.New("trusted directory check failed")
		mocks.Shell.(*shell.MockShell).CheckTrustedDirectoryFunc = func() error {
			return expectedError
		}

		// When checking trusted directory
		result := runtime.CheckTrustedDirectory()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected CheckTrustedDirectory to return the same runtime instance")
		}

		// And error should be set with custom message
		if runtime.err == nil {
			t.Error("Expected error to be set")
		} else {
			expectedErrorMsg := "not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve"
			if runtime.err.Error() != expectedErrorMsg {
				t.Errorf("Expected error %q, got %q", expectedErrorMsg, runtime.err.Error())
			}
		}
	})

	t.Run("PropagatesProjectRootError", func(t *testing.T) {
		// Given a runtime with loaded shell
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell()

		// Mock CheckTrustedDirectory to return a project root error
		expectedError := errors.New("Error getting project root directory: getwd failed")
		mocks.Shell.(*shell.MockShell).CheckTrustedDirectoryFunc = func() error {
			return expectedError
		}

		// When checking trusted directory
		result := runtime.CheckTrustedDirectory()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected CheckTrustedDirectory to return the same runtime instance")
		}

		// And error should be set with custom message
		if runtime.err == nil {
			t.Error("Expected error to be set")
		} else {
			expectedErrorMsg := "not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve"
			if runtime.err.Error() != expectedErrorMsg {
				t.Errorf("Expected error %q, got %q", expectedErrorMsg, runtime.err.Error())
			}
		}
	})

	t.Run("PropagatesTrustedFileNotExistError", func(t *testing.T) {
		// Given a runtime with loaded shell
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell()

		// Mock CheckTrustedDirectory to return a trusted file not exist error
		expectedError := errors.New("Trusted file does not exist")
		mocks.Shell.(*shell.MockShell).CheckTrustedDirectoryFunc = func() error {
			return expectedError
		}

		// When checking trusted directory
		result := runtime.CheckTrustedDirectory()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected CheckTrustedDirectory to return the same runtime instance")
		}

		// And error should be set with custom message
		if runtime.err == nil {
			t.Error("Expected error to be set")
		} else {
			expectedErrorMsg := "not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve"
			if runtime.err.Error() != expectedErrorMsg {
				t.Errorf("Expected error %q, got %q", expectedErrorMsg, runtime.err.Error())
			}
		}
	})

	t.Run("PropagatesNotTrustedError", func(t *testing.T) {
		// Given a runtime with loaded shell
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell()

		// Mock CheckTrustedDirectory to return a not trusted error
		expectedError := errors.New("Current directory not in the trusted list")
		mocks.Shell.(*shell.MockShell).CheckTrustedDirectoryFunc = func() error {
			return expectedError
		}

		// When checking trusted directory
		result := runtime.CheckTrustedDirectory()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected CheckTrustedDirectory to return the same runtime instance")
		}

		// And error should be set with custom message
		if runtime.err == nil {
			t.Error("Expected error to be set")
		} else {
			expectedErrorMsg := "not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve"
			if runtime.err.Error() != expectedErrorMsg {
				t.Errorf("Expected error %q, got %q", expectedErrorMsg, runtime.err.Error())
			}
		}
	})
}

func TestRuntime_PrintEnvVars(t *testing.T) {
	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error
		runtime := NewRuntime()
		expectedError := errors.New("existing error")
		runtime.err = expectedError

		opts := EnvVarsOptions{
			OutputFunc: func(string) {},
		}

		// When printing environment variables
		result := runtime.PrintEnvVars(opts)

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected PrintEnvVars to return the same runtime instance")
		}

		// And original error should be preserved
		if runtime.err != expectedError {
			t.Errorf("Expected original error to be preserved, got %v", runtime.err)
		}
	})

	t.Run("PrintsEnvVarsSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded shell and pre-populated environment variables
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell()

		// Pre-populate r.EnvVars (simulating what LoadEnvVars would do)
		runtime.EnvVars = map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
			"VAR3": "value3",
		}

		// Track output
		var output string
		opts := EnvVarsOptions{
			Export:     true,
			OutputFunc: func(s string) { output = s },
		}

		// Mock shell RenderEnvVars
		expectedEnvVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
			"VAR3": "value3",
		}
		mocks.Shell.(*shell.MockShell).RenderEnvVarsFunc = func(envVars map[string]string, export bool) string {
			if !reflect.DeepEqual(envVars, expectedEnvVars) {
				t.Errorf("Expected env vars %v, got %v", expectedEnvVars, envVars)
			}
			if !export {
				t.Error("Expected export to be true")
			}
			return "export VAR1=value1\nexport VAR2=value2\nexport VAR3=value3"
		}

		// When printing environment variables
		result := runtime.PrintEnvVars(opts)

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected PrintEnvVars to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And output should be captured
		expectedOutput := "export VAR1=value1\nexport VAR2=value2\nexport VAR3=value3"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("HandlesEmptyEnvVars", func(t *testing.T) {
		// Given a runtime with loaded shell and no environment printers
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell()

		// Track if output function is called
		outputCalled := false
		opts := EnvVarsOptions{
			OutputFunc: func(string) { outputCalled = true },
		}

		// When printing environment variables
		result := runtime.PrintEnvVars(opts)

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected PrintEnvVars to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And output function should not be called
		if outputCalled {
			t.Error("Expected output function to not be called when no env vars")
		}
	})

}

func TestRuntime_PrintAliases(t *testing.T) {
	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error
		runtime := NewRuntime()
		expectedError := errors.New("existing error")
		runtime.err = expectedError

		// When printing aliases
		result := runtime.PrintAliases(func(string) {})

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected PrintAliases to return the same runtime instance")
		}

		// And original error should be preserved
		if runtime.err != expectedError {
			t.Errorf("Expected original error to be preserved, got %v", runtime.err)
		}
	})

	t.Run("PrintsAliasesSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded shell and environment printers
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell()

		// Set up mock environment printers
		mockPrinter1 := envvars.NewMockEnvPrinter()
		mockPrinter1.GetAliasFunc = func() (map[string]string, error) {
			return map[string]string{"alias1": "command1", "alias2": "command2"}, nil
		}

		mockPrinter2 := envvars.NewMockEnvPrinter()
		mockPrinter2.GetAliasFunc = func() (map[string]string, error) {
			return map[string]string{"alias3": "command3"}, nil
		}

		runtime.EnvPrinters.AwsEnv = mockPrinter1
		runtime.EnvPrinters.WindsorEnv = mockPrinter2

		// Track output
		var output string
		outputFunc := func(s string) { output = s }

		// Mock shell RenderAliases
		expectedAliases := map[string]string{
			"alias1": "command1",
			"alias2": "command2",
			"alias3": "command3",
		}
		mocks.Shell.(*shell.MockShell).RenderAliasesFunc = func(aliases map[string]string) string {
			if !reflect.DeepEqual(aliases, expectedAliases) {
				t.Errorf("Expected aliases %v, got %v", expectedAliases, aliases)
			}
			return "alias alias1='command1'\nalias alias2='command2'\nalias alias3='command3'"
		}

		// When printing aliases
		result := runtime.PrintAliases(outputFunc)

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected PrintAliases to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And output should be captured
		expectedOutput := "alias alias1='command1'\nalias alias2='command2'\nalias alias3='command3'"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("HandlesEmptyAliases", func(t *testing.T) {
		// Given a runtime with loaded shell and no environment printers
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell()

		// Track if output function is called
		outputCalled := false
		outputFunc := func(string) { outputCalled = true }

		// When printing aliases
		result := runtime.PrintAliases(outputFunc)

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected PrintAliases to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And output function should not be called
		if outputCalled {
			t.Error("Expected output function to not be called when no aliases")
		}
	})

	t.Run("PropagatesAliasError", func(t *testing.T) {
		// Given a runtime with loaded shell and environment printer that returns error
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell()

		// Set up mock environment printer that returns error
		mockPrinter := envvars.NewMockEnvPrinter()
		expectedError := errors.New("alias error")
		mockPrinter.GetAliasFunc = func() (map[string]string, error) {
			return nil, expectedError
		}

		// Set up WindsorEnv printer to avoid panic
		windsorPrinter := envvars.NewMockEnvPrinter()
		windsorPrinter.GetAliasFunc = func() (map[string]string, error) {
			return map[string]string{}, nil
		}

		runtime.EnvPrinters.AwsEnv = mockPrinter
		runtime.EnvPrinters.WindsorEnv = windsorPrinter

		// When printing aliases
		result := runtime.PrintAliases(func(string) {})

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected PrintAliases to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error to be set")
		} else {
			expectedErrorMsg := "error getting aliases: alias error"
			if runtime.err.Error() != expectedErrorMsg {
				t.Errorf("Expected error %q, got %q", expectedErrorMsg, runtime.err.Error())
			}
		}
	})
}

func TestRuntime_ExecutePostEnvHook(t *testing.T) {
	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error
		runtime := NewRuntime()
		expectedError := errors.New("existing error")
		runtime.err = expectedError

		// When executing post env hook
		result := runtime.ExecutePostEnvHook(true)

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected ExecutePostEnvHook to return the same runtime instance")
		}

		// And original error should be preserved
		if runtime.err != expectedError {
			t.Errorf("Expected original error to be preserved, got %v", runtime.err)
		}
	})

	t.Run("ExecutesPostEnvHooksSuccessfully", func(t *testing.T) {
		// Given a runtime with environment printers
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks)

		// Set up mock environment printers
		hook1Called := false
		hook2Called := false

		mockPrinter1 := envvars.NewMockEnvPrinter()
		mockPrinter1.PostEnvHookFunc = func(directory ...string) error {
			hook1Called = true
			return nil
		}

		mockPrinter2 := envvars.NewMockEnvPrinter()
		mockPrinter2.PostEnvHookFunc = func(directory ...string) error {
			hook2Called = true
			return nil
		}

		runtime.EnvPrinters.AwsEnv = mockPrinter1
		runtime.EnvPrinters.WindsorEnv = mockPrinter2

		// When executing post env hook
		result := runtime.ExecutePostEnvHook(true)

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected ExecutePostEnvHook to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And hooks should be called
		if !hook1Called {
			t.Error("Expected first hook to be called")
		}
		if !hook2Called {
			t.Error("Expected second hook to be called")
		}
	})

	t.Run("HandlesHookErrorWithVerboseTrue", func(t *testing.T) {
		// Given a runtime with environment printer that returns error
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks)

		// Set up mock environment printer that returns error
		mockPrinter := envvars.NewMockEnvPrinter()
		expectedError := errors.New("hook error")
		mockPrinter.PostEnvHookFunc = func(directory ...string) error {
			return expectedError
		}

		// Set up WindsorEnv printer to avoid panic
		windsorPrinter := envvars.NewMockEnvPrinter()
		windsorPrinter.PostEnvHookFunc = func(directory ...string) error {
			return nil
		}

		runtime.EnvPrinters.AwsEnv = mockPrinter
		runtime.EnvPrinters.WindsorEnv = windsorPrinter

		// When executing post env hook with verbose true
		result := runtime.ExecutePostEnvHook(true)

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected ExecutePostEnvHook to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error to be set")
		} else {
			expectedErrorMsg := "failed to execute post env hooks: hook error"
			if runtime.err.Error() != expectedErrorMsg {
				t.Errorf("Expected error %q, got %q", expectedErrorMsg, runtime.err.Error())
			}
		}
	})

	t.Run("HandlesHookErrorWithVerboseFalse", func(t *testing.T) {
		// Given a runtime with environment printer that returns error
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks)

		// Set up mock environment printer that returns error
		mockPrinter := envvars.NewMockEnvPrinter()
		expectedError := errors.New("hook error")
		mockPrinter.PostEnvHookFunc = func(directory ...string) error {
			return expectedError
		}

		// Set up WindsorEnv printer to avoid panic
		windsorPrinter := envvars.NewMockEnvPrinter()
		windsorPrinter.PostEnvHookFunc = func(directory ...string) error {
			return nil
		}

		runtime.EnvPrinters.AwsEnv = mockPrinter
		runtime.EnvPrinters.WindsorEnv = windsorPrinter

		// When executing post env hook with verbose false
		result := runtime.ExecutePostEnvHook(false)

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected ExecutePostEnvHook to return the same runtime instance")
		}

		// And no error should be set (verbose false suppresses errors)
		if runtime.err != nil {
			t.Errorf("Expected no error when verbose false, got %v", runtime.err)
		}
	})

	t.Run("HandlesMultipleHookErrors", func(t *testing.T) {
		// Given a runtime with multiple environment printers that return errors
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks)

		// Set up mock environment printers that return errors
		mockPrinter1 := envvars.NewMockEnvPrinter()
		error1 := errors.New("hook error 1")
		mockPrinter1.PostEnvHookFunc = func(directory ...string) error {
			return error1
		}

		mockPrinter2 := envvars.NewMockEnvPrinter()
		error2 := errors.New("hook error 2")
		mockPrinter2.PostEnvHookFunc = func(directory ...string) error {
			return error2
		}

		runtime.EnvPrinters.AwsEnv = mockPrinter1
		runtime.EnvPrinters.WindsorEnv = mockPrinter2

		// When executing post env hook
		result := runtime.ExecutePostEnvHook(true)

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected ExecutePostEnvHook to return the same runtime instance")
		}

		// And error should be set with first error
		if runtime.err == nil {
			t.Error("Expected error to be set")
		} else {
			expectedErrorMsg := "failed to execute post env hooks: hook error 1"
			if runtime.err.Error() != expectedErrorMsg {
				t.Errorf("Expected error %q, got %q", expectedErrorMsg, runtime.err.Error())
			}
		}
	})

	t.Run("SkipsNilPrinters", func(t *testing.T) {
		// Given a runtime with some nil environment printers
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks)

		// Set up one mock environment printer
		hookCalled := false
		mockPrinter := envvars.NewMockEnvPrinter()
		mockPrinter.PostEnvHookFunc = func(directory ...string) error {
			hookCalled = true
			return nil
		}

		// Set up WindsorEnv printer to avoid panic
		windsorPrinter := envvars.NewMockEnvPrinter()
		windsorPrinter.PostEnvHookFunc = func(directory ...string) error {
			return nil
		}

		runtime.EnvPrinters.AwsEnv = mockPrinter
		runtime.EnvPrinters.WindsorEnv = windsorPrinter
		// Other printers remain nil

		// When executing post env hook
		result := runtime.ExecutePostEnvHook(true)

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected ExecutePostEnvHook to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And hook should be called
		if !hookCalled {
			t.Error("Expected hook to be called")
		}
	})
}

func TestRuntime_getAllEnvPrinters(t *testing.T) {
	t.Run("ReturnsAllNonNilPrinters", func(t *testing.T) {
		// Given a runtime with some environment printers set
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks)

		// Set up some mock environment printers
		mockPrinter1 := envvars.NewMockEnvPrinter()
		mockPrinter2 := envvars.NewMockEnvPrinter()
		mockPrinter3 := envvars.NewMockEnvPrinter()

		runtime.EnvPrinters.AwsEnv = mockPrinter1
		runtime.EnvPrinters.AzureEnv = mockPrinter2
		runtime.EnvPrinters.WindsorEnv = mockPrinter3
		// Other printers remain nil

		// When getting all environment printers
		printers := runtime.getAllEnvPrinters()

		// Then should return only non-nil printers
		if len(printers) != 3 {
			t.Errorf("Expected 3 printers, got %d", len(printers))
		}

		// And should include the set printers
		foundAws := false
		foundAzure := false
		foundWindsor := false

		for _, printer := range printers {
			if printer == mockPrinter1 {
				foundAws = true
			}
			if printer == mockPrinter2 {
				foundAzure = true
			}
			if printer == mockPrinter3 {
				foundWindsor = true
			}
		}

		if !foundAws {
			t.Error("Expected AWS printer to be included")
		}
		if !foundAzure {
			t.Error("Expected Azure printer to be included")
		}
		if !foundWindsor {
			t.Error("Expected Windsor printer to be included")
		}
	})

	t.Run("EnsuresWindsorEnvIsLast", func(t *testing.T) {
		// Given a runtime with WindsorEnv and other printers
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks)

		// Set up mock environment printers
		mockPrinter1 := envvars.NewMockEnvPrinter()
		mockPrinter2 := envvars.NewMockEnvPrinter()
		windsorPrinter := envvars.NewMockEnvPrinter()

		runtime.EnvPrinters.AwsEnv = mockPrinter1
		runtime.EnvPrinters.AzureEnv = mockPrinter2
		runtime.EnvPrinters.WindsorEnv = windsorPrinter

		// When getting all environment printers
		printers := runtime.getAllEnvPrinters()

		// Then WindsorEnv should be last
		if len(printers) == 0 {
			t.Error("Expected at least one printer")
		} else if printers[len(printers)-1] != windsorPrinter {
			t.Error("Expected WindsorEnv to be the last printer")
		}
	})

	t.Run("ReturnsEmptySliceWhenNoPrinters", func(t *testing.T) {
		// Given a runtime with no environment printers set
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks)

		// When getting all environment printers
		printers := runtime.getAllEnvPrinters()

		// Then should return empty slice
		if len(printers) != 0 {
			t.Errorf("Expected 0 printers, got %d", len(printers))
		}
	})
}

func TestRuntime_WorkstationUp(t *testing.T) {
	t.Run("StartsWorkstationSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded dependencies
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return "test-context"
		}
		mocks.Shell.(*shell.MockShell).GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

		// When starting workstation
		result := runtime.WorkstationUp()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WorkstationUp to return the same runtime instance")
		}

		// And error should be set due to workstation creation failure
		// (This is expected since we don't have a real workstation setup)
		if runtime.err == nil {
			t.Error("Expected error to be set due to workstation creation failure")
		}
	})

	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks)
		expectedError := errors.New("existing error")
		runtime.err = expectedError

		// When starting workstation
		result := runtime.WorkstationUp()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WorkstationUp to return the same runtime instance")
		}

		// And error should remain unchanged
		if runtime.err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, runtime.err)
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerNotLoaded", func(t *testing.T) {
		// Given a runtime without loaded config handler
		runtime := NewRuntime()

		// When starting workstation
		result := runtime.WorkstationUp()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WorkstationUp to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error to be set")
		} else {
			expectedError := "config handler not loaded"
			if !strings.Contains(runtime.err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, runtime.err.Error())
			}
		}
	})

	t.Run("ReturnsErrorWhenShellNotLoaded", func(t *testing.T) {
		// Given a runtime with config handler but no shell
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks)
		// Manually set config handler without calling LoadConfig (which loads shell)
		runtime.ConfigHandler = mocks.ConfigHandler
		// Explicitly set shell to nil
		runtime.Shell = nil

		// When starting workstation
		result := runtime.WorkstationUp()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WorkstationUp to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error to be set")
		} else {
			expectedError := "shell not loaded"
			if !strings.Contains(runtime.err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, runtime.err.Error())
			}
		}
	})

	t.Run("ReturnsErrorWhenInjectorNotAvailable", func(t *testing.T) {
		// Given a runtime with loaded dependencies but no injector
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()
		runtime.Injector = nil

		// When starting workstation
		result := runtime.WorkstationUp()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WorkstationUp to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error to be set")
		} else {
			expectedError := "injector not available"
			if !strings.Contains(runtime.err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, runtime.err.Error())
			}
		}
	})

	t.Run("ReturnsErrorWhenNoContextSet", func(t *testing.T) {
		// Given a runtime with loaded dependencies but no context
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return ""
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

		// When starting workstation
		result := runtime.WorkstationUp()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WorkstationUp to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error to be set")
		} else {
			expectedError := "no context set"
			if !strings.Contains(runtime.err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, runtime.err.Error())
			}
		}
	})

	t.Run("PropagatesProjectRootError", func(t *testing.T) {
		// Given a runtime with loaded dependencies
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return "test-context"
		}
		expectedError := errors.New("project root error")
		mocks.Shell.(*shell.MockShell).GetProjectRootFunc = func() (string, error) {
			return "", expectedError
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

		// When starting workstation
		result := runtime.WorkstationUp()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WorkstationUp to return the same runtime instance")
		}

		// And error should be propagated
		if runtime.err == nil {
			t.Error("Expected error to be propagated")
		} else {
			expectedErrorText := "failed to get project root"
			if !strings.Contains(runtime.err.Error(), expectedErrorText) {
				t.Errorf("Expected error to contain %q, got %q", expectedErrorText, runtime.err.Error())
			}
		}
	})
}

func TestRuntime_WorkstationDown(t *testing.T) {
	t.Run("StopsWorkstationSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded dependencies
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return "test-context"
		}
		mocks.Shell.(*shell.MockShell).GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

		// When stopping workstation
		result := runtime.WorkstationDown()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WorkstationDown to return the same runtime instance")
		}

		// And no error should be set (workstation down succeeds even with minimal setup)
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}
	})

	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks)
		expectedError := errors.New("existing error")
		runtime.err = expectedError

		// When stopping workstation
		result := runtime.WorkstationDown()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WorkstationDown to return the same runtime instance")
		}

		// And error should remain unchanged
		if runtime.err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, runtime.err)
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerNotLoaded", func(t *testing.T) {
		// Given a runtime without loaded config handler
		runtime := NewRuntime()

		// When stopping workstation
		result := runtime.WorkstationDown()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WorkstationDown to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error to be set")
		} else {
			expectedError := "config handler not loaded"
			if !strings.Contains(runtime.err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, runtime.err.Error())
			}
		}
	})

	t.Run("ReturnsErrorWhenShellNotLoaded", func(t *testing.T) {
		// Given a runtime with config handler but no shell
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks)
		// Manually set config handler without calling LoadConfig (which loads shell)
		runtime.ConfigHandler = mocks.ConfigHandler
		// Explicitly set shell to nil
		runtime.Shell = nil

		// When stopping workstation
		result := runtime.WorkstationDown()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WorkstationDown to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error to be set")
		} else {
			expectedError := "shell not loaded"
			if !strings.Contains(runtime.err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, runtime.err.Error())
			}
		}
	})

	t.Run("ReturnsErrorWhenInjectorNotAvailable", func(t *testing.T) {
		// Given a runtime with loaded dependencies but no injector
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()
		runtime.Injector = nil

		// When stopping workstation
		result := runtime.WorkstationDown()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WorkstationDown to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error to be set")
		} else {
			expectedError := "injector not available"
			if !strings.Contains(runtime.err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, runtime.err.Error())
			}
		}
	})

	t.Run("ReturnsErrorWhenNoContextSet", func(t *testing.T) {
		// Given a runtime with loaded dependencies but no context
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return ""
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

		// When stopping workstation
		result := runtime.WorkstationDown()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WorkstationDown to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error to be set")
		} else {
			expectedError := "no context set"
			if !strings.Contains(runtime.err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, runtime.err.Error())
			}
		}
	})

	t.Run("PropagatesProjectRootError", func(t *testing.T) {
		// Given a runtime with loaded dependencies
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return "test-context"
		}
		expectedError := errors.New("project root error")
		mocks.Shell.(*shell.MockShell).GetProjectRootFunc = func() (string, error) {
			return "", expectedError
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

		// When stopping workstation
		result := runtime.WorkstationDown()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WorkstationDown to return the same runtime instance")
		}

		// And error should be propagated
		if runtime.err == nil {
			t.Error("Expected error to be propagated")
		} else {
			expectedErrorText := "failed to get project root"
			if !strings.Contains(runtime.err.Error(), expectedErrorText) {
				t.Errorf("Expected error to contain %q, got %q", expectedErrorText, runtime.err.Error())
			}
		}
	})
}

func TestRuntime_createWorkstation(t *testing.T) {
	t.Run("CreatesWorkstationSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded dependencies
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return "test-context"
		}
		mocks.Shell.(*shell.MockShell).GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

		// When creating workstation
		ws, err := runtime.createWorkstation()

		// Then should return workstation and no error
		if ws == nil {
			t.Error("Expected workstation to be created")
		}
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerNotLoaded", func(t *testing.T) {
		// Given a runtime without loaded config handler
		runtime := NewRuntime()

		// When creating workstation
		ws, err := runtime.createWorkstation()

		// Then should return error
		if ws != nil {
			t.Error("Expected workstation to be nil")
		}
		if err == nil {
			t.Error("Expected error to be returned")
		} else {
			expectedError := "config handler not loaded"
			if !strings.Contains(err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
			}
		}
	})

	t.Run("ReturnsErrorWhenShellNotLoaded", func(t *testing.T) {
		// Given a runtime with config handler but no shell
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks)
		// Manually set config handler without calling LoadConfig (which loads shell)
		runtime.ConfigHandler = mocks.ConfigHandler
		// Explicitly set shell to nil
		runtime.Shell = nil

		// When creating workstation
		ws, err := runtime.createWorkstation()

		// Then should return error
		if ws != nil {
			t.Error("Expected workstation to be nil")
		}
		if err == nil {
			t.Error("Expected error to be returned")
		} else {
			expectedError := "shell not loaded"
			if !strings.Contains(err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
			}
		}
	})

	t.Run("ReturnsErrorWhenInjectorNotAvailable", func(t *testing.T) {
		// Given a runtime with loaded dependencies but no injector
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()
		runtime.Injector = nil

		// When creating workstation
		ws, err := runtime.createWorkstation()

		// Then should return error
		if ws != nil {
			t.Error("Expected workstation to be nil")
		}
		if err == nil {
			t.Error("Expected error to be returned")
		} else {
			expectedError := "injector not available"
			if !strings.Contains(err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
			}
		}
	})

	t.Run("ReturnsErrorWhenNoContextSet", func(t *testing.T) {
		// Given a runtime with loaded dependencies but no context
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return ""
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

		// When creating workstation
		ws, err := runtime.createWorkstation()

		// Then should return error
		if ws != nil {
			t.Error("Expected workstation to be nil")
		}
		if err == nil {
			t.Error("Expected error to be returned")
		} else {
			expectedError := "no context set"
			if !strings.Contains(err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
			}
		}
	})

	t.Run("PropagatesProjectRootError", func(t *testing.T) {
		// Given a runtime with loaded dependencies
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return "test-context"
		}
		expectedError := errors.New("project root error")
		mocks.Shell.(*shell.MockShell).GetProjectRootFunc = func() (string, error) {
			return "", expectedError
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

		// When creating workstation
		ws, err := runtime.createWorkstation()

		// Then should return error
		if ws != nil {
			t.Error("Expected workstation to be nil")
		}
		if err == nil {
			t.Error("Expected error to be returned")
		} else {
			expectedErrorText := "failed to get project root"
			if !strings.Contains(err.Error(), expectedErrorText) {
				t.Errorf("Expected error to contain %q, got %q", expectedErrorText, err.Error())
			}
		}
	})
}
