package runtime_test

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/env"
	"github.com/windsorcli/cli/pkg/runtime/secrets"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestRuntime_HandleSessionReset(t *testing.T) {
	t.Run("ResetsWhenNoSessionToken", func(t *testing.T) {
		// Given a runtime with no session token
		t.Cleanup(func() {
			os.Unsetenv("NO_CACHE")
			os.Unsetenv("WINDSOR_SESSION_TOKEN")
		})
		os.Unsetenv("WINDSOR_SESSION_TOKEN")

		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockShell := mocks.Shell.(*shell.MockShell)
		resetCalled := false
		mockShell.ResetFunc = func(clearSession ...bool) {
			resetCalled = true
		}
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}

		// When HandleSessionReset is called
		err := rt.HandleSessionReset()

		// Then reset should be called

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !resetCalled {
			t.Error("Expected Reset to be called when no session token")
		}
		if os.Getenv("NO_CACHE") != "true" {
			t.Errorf("Expected NO_CACHE to default to true, got %q", os.Getenv("NO_CACHE"))
		}
	})

	t.Run("PreservesExplicitNoCacheFalseOnReset", func(t *testing.T) {
		// Given a caller that explicitly opted back into caching (e.g. `NO_CACHE=false windsor
		// exec -- ...`), and no session token — so a reset is still triggered
		t.Cleanup(func() {
			os.Unsetenv("NO_CACHE")
			os.Unsetenv("WINDSOR_SESSION_TOKEN")
		})
		os.Unsetenv("WINDSOR_SESSION_TOKEN")
		t.Setenv("NO_CACHE", "false")

		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockShell := mocks.Shell.(*shell.MockShell)
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}

		// When HandleSessionReset is called
		err := rt.HandleSessionReset()

		// Then the reset still runs, but the caller's explicit NO_CACHE=false is not
		// force-overwritten to true
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if os.Getenv("NO_CACHE") != "false" {
			t.Errorf("Expected explicit NO_CACHE=false to be preserved, got %q", os.Getenv("NO_CACHE"))
		}
	})

	t.Run("PreservesExplicitNoCacheTrueOnReset", func(t *testing.T) {
		// Given a caller that already set NO_CACHE=true itself, and no session token
		t.Cleanup(func() {
			os.Unsetenv("NO_CACHE")
			os.Unsetenv("WINDSOR_SESSION_TOKEN")
		})
		os.Unsetenv("WINDSOR_SESSION_TOKEN")
		t.Setenv("NO_CACHE", "true")

		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockShell := mocks.Shell.(*shell.MockShell)
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}

		// When HandleSessionReset is called
		err := rt.HandleSessionReset()

		// Then the pre-existing value is left alone (not re-set, though the observable result
		// is the same either way)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if os.Getenv("NO_CACHE") != "true" {
			t.Errorf("Expected NO_CACHE to remain true, got %q", os.Getenv("NO_CACHE"))
		}
	})

	t.Run("ResetsWhenResetFlagSet", func(t *testing.T) {
		// Given a runtime with reset flag set
		t.Cleanup(func() {
			os.Unsetenv("NO_CACHE")
		})

		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockShell := mocks.Shell.(*shell.MockShell)
		resetCalled := false
		mockShell.ResetFunc = func(clearSession ...bool) {
			resetCalled = true
		}
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return true, nil
		}

		// When HandleSessionReset is called
		err := rt.HandleSessionReset()

		// Then reset should be called

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !resetCalled {
			t.Error("Expected Reset to be called when reset flag set")
		}
	})

	t.Run("SkipsResetWhenSessionTokenAndNoResetFlag", func(t *testing.T) {
		// Given a runtime with session token and no reset flag
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		t.Setenv("WINDSOR_SESSION_TOKEN", "test-token")

		mockShell := mocks.Shell.(*shell.MockShell)
		resetCalled := false
		mockShell.ResetFunc = func(clearSession ...bool) {
			resetCalled = true
		}
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}

		// When HandleSessionReset is called
		err := rt.HandleSessionReset()

		// Then reset should not be called

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if resetCalled {
			t.Error("Expected Reset not to be called when session token exists and no reset flag")
		}
	})

	t.Run("ErrorWhenSetNoCacheFails", func(t *testing.T) {
		// Given a reset with no NO_CACHE value and a Setenv that fails
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.Shims.Getenv = func(string) string { return "" }
		rt.Shims.LookupEnv = func(string) (string, bool) { return "", false }
		rt.Shims.Setenv = func(string, string) error {
			return fmt.Errorf("setenv failed")
		}

		// When HandleSessionReset is called
		err := rt.HandleSessionReset()

		// Then an error about NO_CACHE is returned
		if err == nil || !strings.Contains(err.Error(), "failed to set NO_CACHE") {
			t.Errorf("Expected NO_CACHE error, got: %v", err)
		}
	})

	t.Run("PanicsWhenShellNotInitialized", func(t *testing.T) {
		// Given a runtime with nil shell
		rt := &runtime.Runtime{}

		// When HandleSessionReset is called
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when Shell is nil")
			}
		}()
		_ = rt.HandleSessionReset()
	})

	t.Run("ErrorWhenCheckResetFlagsFails", func(t *testing.T) {
		// Given a runtime with a shell that fails to check reset flags
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockShell := mocks.Shell.(*shell.MockShell)
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, fmt.Errorf("check reset flags failed")
		}

		// When HandleSessionReset is called
		err := rt.HandleSessionReset()

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when CheckResetFlags fails")
		}

		if !strings.Contains(err.Error(), "failed to check reset flags") {
			t.Errorf("Expected error about check reset flags, got: %v", err)
		}
	})

}

func TestRuntime_LoadEnvironment(t *testing.T) {
	t.Run("LoadsEnvironmentSuccessfully", func(t *testing.T) {
		// Given a runtime with mocks
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		// When LoadEnvironment is called
		err := rt.LoadEnvironment(false)

		// Then environment should load successfully
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if rt.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv printer to be initialized")
		}

		if len(rt.EnvVars()) == 0 {
			t.Error("Expected environment variables to be loaded")
		}
	})

	t.Run("PanicsWhenConfigHandlerIsNil", func(t *testing.T) {
		// Given a runtime with nil config handler
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		rt.ConfigHandler = nil

		// When LoadEnvironment is called
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when config handler is nil")
			}
		}()
		_ = rt.LoadEnvironment(false)
	})

	t.Run("HandlesEnvPrinterInitializationError", func(t *testing.T) {
		// Given a runtime with mocks
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		// When LoadEnvironment is called
		err := rt.LoadEnvironment(false)

		// Then environment should load successfully
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if rt.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv printer to be initialized")
		}
	})

	t.Run("ErrorWhenSetenvFails", func(t *testing.T) {
		// Given an env printer with a variable and a Setenv that fails
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{"TEST_VAR": "value"}, nil
		}
		rt.EnvPrinters.WindsorEnv = mockEnvPrinter
		rt.Shims.Setenv = func(string, string) error {
			return fmt.Errorf("setenv failed")
		}

		// When LoadEnvironment is called
		err := rt.LoadEnvironment(false)

		// Then an error about setting the variable is returned
		if err == nil || !strings.Contains(err.Error(), "error setting environment variable") {
			t.Errorf("Expected set environment variable error, got: %v", err)
		}
	})

	t.Run("ErrorWhenGetEnvVarsFails", func(t *testing.T) {
		// Given a runtime with an env printer that fails to get env vars
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("failed to get env vars")
		}
		rt.EnvPrinters.WindsorEnv = mockEnvPrinter

		// When LoadEnvironment is called
		err := rt.LoadEnvironment(false)

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when GetEnvVars fails")
		}

		if !strings.Contains(err.Error(), "error getting environment variables") {
			t.Errorf("Expected error about getting environment variables, got: %v", err)
		}
	})

	t.Run("ErrorWhenGetAliasFails", func(t *testing.T) {
		// Given a runtime with an env printer that fails to get aliases
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{}, nil
		}
		mockEnvPrinter.GetAliasFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("failed to get aliases")
		}
		rt.EnvPrinters.WindsorEnv = mockEnvPrinter

		// When LoadEnvironment is called
		err := rt.LoadEnvironment(false)

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when GetAlias fails")
		}

		if !strings.Contains(err.Error(), "error getting aliases") {
			t.Errorf("Expected error about getting aliases, got: %v", err)
		}
	})

	t.Run("ErrorWhenPostEnvHookFails", func(t *testing.T) {
		// Given a runtime with an env printer that fails to execute post env hook
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{}, nil
		}
		mockEnvPrinter.GetAliasFunc = func() (map[string]string, error) {
			return map[string]string{}, nil
		}
		mockEnvPrinter.PostEnvHookFunc = func(directory ...string) error {
			return fmt.Errorf("failed to execute post env hook")
		}
		rt.EnvPrinters.WindsorEnv = mockEnvPrinter

		// When LoadEnvironment is called
		err := rt.LoadEnvironment(false)

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when PostEnvHook fails")
		}

		if !strings.Contains(err.Error(), "failed to execute post env hooks") {
			t.Errorf("Expected error about executing post env hooks, got: %v", err)
		}
	})

	t.Run("LoadsEnvironmentWithSecretsSuccessfully", func(t *testing.T) {
		// Given a runtime with secrets providers
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockSopsProvider := secrets.NewMockProvider()
		mockOnepasswordProvider := secrets.NewMockProvider()
		rt.Resolver = secrets.NewResolver([]secrets.Provider{mockSopsProvider, mockOnepasswordProvider}, mocks.Shell)

		// When LoadEnvironment is called with secrets enabled
		err := rt.LoadEnvironment(true)

		// Then environment should load successfully
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesSecretsLoadError", func(t *testing.T) {
		// Given a runtime with a secrets provider that fails to load
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockProvider := secrets.NewMockProvider()
		mockProvider.LoadSecretsFunc = func() error {
			return errors.New("secrets load failed")
		}
		rt.Resolver = secrets.NewResolver([]secrets.Provider{mockProvider}, mocks.Shell)

		// When LoadEnvironment is called with secrets enabled
		err := rt.LoadEnvironment(true)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "secrets load failed") {
			t.Errorf("Expected secrets load error, got: %v", err)
		}
	})
}

func TestRuntime_GetEnvVars(t *testing.T) {
	t.Run("ReturnsCopyOfEnvVars", func(t *testing.T) {
		// Given a runtime with environment variables
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		original := map[string]string{
			"TEST_VAR1": "value1",
			"TEST_VAR2": "value2",
		}
		rt.SetEnvVars(original)

		// When GetEnvVars is called
		copy := rt.GetEnvVars()

		// Then a copy should be returned that doesn't affect the original
		if len(copy) != len(original) {
			t.Error("Expected copy to have same length as original")
		}

		copy["NEW_VAR"] = "new_value"

		if len(rt.EnvVars()) != len(original) {
			t.Error("Expected original to be unchanged")
		}
	})
}

func TestRuntime_GetAliases(t *testing.T) {
	t.Run("ReturnsCopyOfAliases", func(t *testing.T) {
		// Given a runtime with aliases
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		original := map[string]string{
			"test1": "echo test1",
			"test2": "echo test2",
		}
		rt.SetAliases(original)

		// When GetAliases is called
		copy := rt.GetAliases()

		// Then a copy should be returned that doesn't affect the original
		if len(copy) != len(original) {
			t.Error("Expected copy to have same length as original")
		}

		copy["new"] = "echo new"

		if len(rt.Aliases()) != len(original) {
			t.Error("Expected original to be unchanged")
		}
	})
}
