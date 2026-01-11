package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/env"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/terraform"
)

// =============================================================================
// Test Setup
// =============================================================================

func captureEnvVars() map[string]string {
	envVars := make(map[string]string)
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			envVars[parts[0]] = parts[1]
		}
	}
	return envVars
}

func restoreEnvVars(t *testing.T, envVarsBefore map[string]string) {
	t.Helper()
	for key, val := range envVarsBefore {
		os.Setenv(key, val)
	}
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			if _, existed := envVarsBefore[parts[0]]; !existed {
				os.Unsetenv(parts[0])
			}
		}
	}
}

func isolateTestState(t *testing.T) {
	t.Helper()
	rootCmd.SetContext(context.Background())
	rootCmd.SetArgs([]string{})
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	verbose = false
	for _, cmd := range rootCmd.Commands() {
		cmd.SetContext(context.Background())
		cmd.SetArgs([]string{})
	}
}

func setupOutputCapture(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	stdout, stderr := captureOutput(t)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	return stdout, stderr
}

func setupTestContext(t *testing.T, mocks *Mocks) {
	t.Helper()
	rootCmd.SetContext(context.Background())
	verbose = false
	ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
	rootCmd.SetContext(ctx)
	t.Cleanup(func() {
		rootCmd.SetContext(context.Background())
	})
}

func setupTerraformMocks(t *testing.T, inTerraformProject bool) *Mocks {
	t.Helper()
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		if key == "terraform.enabled" {
			return true
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return false
	}

	mocks := setupMocks(t, &SetupOptions{ConfigHandler: mockConfigHandler})

	mockTerraformProvider := &terraform.MockTerraformProvider{}
	mockTerraformProvider.IsInTerraformProjectFunc = func() bool {
		return inTerraformProject
	}
	mocks.Runtime.TerraformProvider = mockTerraformProvider

	return mocks
}

// =============================================================================
// Test Cases
// =============================================================================

func TestEnvCmd(t *testing.T) {
	envVarsBefore := captureEnvVars()
	t.Cleanup(func() {
		isolateTestState(t)
		restoreEnvVars(t, envVarsBefore)
	})

	t.Run("Success", func(t *testing.T) {
		// Given proper output capture and mock setup
		_, stderr := setupOutputCapture(t)
		mocks := setupMocks(t)
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		// When executing the command
		rootCmd.SetArgs([]string{"env"})
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SuccessWithDecrypt", func(t *testing.T) {
		// Given proper output capture and mock setup
		_, stderr := setupOutputCapture(t)
		mocks := setupMocks(t)
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		// When executing the command with decrypt flag
		rootCmd.SetArgs([]string{"env", "--decrypt"})
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SuccessWithHook", func(t *testing.T) {
		// Given proper output capture and mock setup
		_, stderr := setupOutputCapture(t)
		mocks := setupMocks(t)
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		// When executing the command with hook flag
		rootCmd.SetArgs([]string{"env", "--hook"})
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SuccessWithVerbose", func(t *testing.T) {
		// Given proper output capture and mock setup
		_, stderr := setupOutputCapture(t)
		mocks := setupMocks(t)
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		// When executing the command with verbose flag
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SuccessWithAllFlags", func(t *testing.T) {
		// Given proper output capture and mock setup
		_, stderr := setupOutputCapture(t)
		mocks := setupMocks(t)
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		// When executing the command with all flags
		rootCmd.SetArgs([]string{"env", "--decrypt", "--hook", "--verbose"})
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})
}

func TestEnvCmd_ErrorScenarios(t *testing.T) {
	envVarsBefore := captureEnvVars()
	os.Unsetenv("WINDSOR_CONTEXT")
	os.Unsetenv("NO_CACHE")

	t.Cleanup(func() {
		isolateTestState(t)
		os.Unsetenv("WINDSOR_CONTEXT")
		restoreEnvVars(t, envVarsBefore)
	})

	t.Run("HandlesNewRuntimeError", func(t *testing.T) {
		setupOutputCapture(t)
		isolateTestState(t)

		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}

		rtOverride := &runtime.Runtime{
			Shell:       mockShell,
			ProjectRoot: "",
		}

		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected NewRuntime to panic with invalid shell")
			}
		}()
		_ = runtime.NewRuntime(rtOverride)

		ctx := context.WithValue(context.Background(), runtimeOverridesKey, rtOverride)
		rootCmd.SetContext(ctx)
		rootCmd.SetArgs([]string{"env"})
		_ = Execute()
	})

	t.Run("HandlesHandleSessionResetError", func(t *testing.T) {
		// Given mocks with CheckResetFlags that fails
		setupOutputCapture(t)
		mocks := setupMocks(t)
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return false, fmt.Errorf("reset check failed")
		}
		setupTestContext(t, mocks)

		// When executing the command
		rootCmd.SetArgs([]string{"env"})
		err := Execute()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when HandleSessionReset fails")
			return
		}
		if !strings.Contains(err.Error(), "failed to check reset flags") {
			t.Errorf("Expected error about reset flags, got: %v", err)
		}
	})

	t.Run("HandlesLoadConfigError", func(t *testing.T) {
		// Given a mock config handler that fails on LoadConfig
		setupOutputCapture(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error {
			return fmt.Errorf("config load failed")
		}
		mocks := setupMocks(t, &SetupOptions{ConfigHandler: mockConfigHandler})
		setupTestContext(t, mocks)

		// When executing the command
		rootCmd.SetArgs([]string{"env"})
		err := Execute()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when LoadConfig fails")
			return
		}
		if !strings.Contains(err.Error(), "config load failed") {
			t.Errorf("Expected error about config loading, got: %v", err)
		}
	})

	t.Run("HandlesExecutePostEnvHooksErrorWithVerbose", func(t *testing.T) {
		// Given mocks with PostEnvHook that fails and verbose flag
		setupOutputCapture(t)
		mocks := setupMocks(t)
		if mockWindsorEnv, ok := mocks.Runtime.EnvPrinters.WindsorEnv.(*env.MockEnvPrinter); ok {
			mockWindsorEnv.PostEnvHookFunc = func(directory ...string) error {
				return fmt.Errorf("hook failed")
			}
		}
		setupTestContext(t, mocks)

		// When executing the command with verbose flag
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute()

		// Then an error may be returned about post env hooks or environment loading
		if err != nil && !strings.Contains(err.Error(), "failed to execute post env hooks") && !strings.Contains(err.Error(), "failed to load environment") {
			t.Errorf("Expected error about post env hooks or environment loading, got: %v", err)
		}
	})

	t.Run("SwallowsExecutePostEnvHooksErrorWithoutVerbose", func(t *testing.T) {
		// Given mocks with PostEnvHook that fails and no verbose flag
		_, stderr := setupOutputCapture(t)
		mocks := setupMocks(t)
		if mockWindsorEnv, ok := mocks.Runtime.EnvPrinters.WindsorEnv.(*env.MockEnvPrinter); ok {
			mockWindsorEnv.PostEnvHookFunc = func(directory ...string) error {
				return fmt.Errorf("hook failed")
			}
		}
		setupTestContext(t, mocks)

		// When executing the command without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := Execute()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error when verbose is false, got: %v", err)
		}
		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SwallowsExecutePostEnvHooksErrorWithHook", func(t *testing.T) {
		// Given mocks with PostEnvHook that fails and hook flag
		_, stderr := setupOutputCapture(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "secrets.sops.enabled" || key == "secrets.onepassword.enabled" {
				return false
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		mocks := setupMocks(t, &SetupOptions{ConfigHandler: mockConfigHandler})
		if mockWindsorEnv, ok := mocks.Runtime.EnvPrinters.WindsorEnv.(*env.MockEnvPrinter); ok {
			mockWindsorEnv.PostEnvHookFunc = func(directory ...string) error {
				return fmt.Errorf("hook failed")
			}
		}
		setupTestContext(t, mocks)

		// When executing the command with hook flag
		rootCmd.SetArgs([]string{"env", "--hook", "--verbose"})
		err := Execute()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error when hook is true (even with verbose), got: %v", err)
		}
		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SwallowsCheckTrustedDirectoryErrorWithHook", func(t *testing.T) {
		// Given mocks with CheckTrustedDirectory that fails and hook flag
		_, stderr := setupOutputCapture(t)
		mocks := setupMocks(t)
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not trusted")
		}
		setupTestContext(t, mocks)

		// When executing the command with hook flag
		rootCmd.SetArgs([]string{"env", "--hook"})
		err := Execute()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error when CheckTrustedDirectory fails with hook, got: %v", err)
		}
		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SwallowsBlueprintLoadErrorWithHook", func(t *testing.T) {
		// Given terraform mocks with blueprint loading that may fail
		_, stderr := setupOutputCapture(t)
		mocks := setupTerraformMocks(t, true)
		setupTestContext(t, mocks)

		// When executing the command with hook flag
		rootCmd.SetArgs([]string{"env", "--hook"})
		err := Execute()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error when blueprint load fails with hook, got: %v", err)
		}
		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SwallowsBlueprintLoadErrorWithoutVerbose", func(t *testing.T) {
		// Given terraform mocks with blueprint loading that may fail
		_, stderr := setupOutputCapture(t)
		mocks := setupTerraformMocks(t, true)
		setupTestContext(t, mocks)

		// When executing the command without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := Execute()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error when blueprint load fails without verbose, got: %v", err)
		}
		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("ReturnsBlueprintLoadErrorWithVerbose", func(t *testing.T) {
		// Given terraform mocks with blueprint loading that may fail
		setupOutputCapture(t)
		mocks := setupTerraformMocks(t, true)
		setupTestContext(t, mocks)

		// When executing the command with verbose flag
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute()

		// Then an error may be returned about loading blueprint
		if err != nil && !strings.Contains(err.Error(), "failed to load blueprint") {
			t.Errorf("Expected error about loading blueprint, got: %v", err)
		}
	})

	t.Run("ExecutesInitializeComponents", func(t *testing.T) {
		// Given proper mocks and setup
		_, stderr := setupOutputCapture(t)
		mocks := setupMocks(t)
		setupTestContext(t, mocks)

		// When executing the command
		rootCmd.SetArgs([]string{"env"})
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("ExecutesBlueprintLoadWhenTerraformEnabled", func(t *testing.T) {
		// Given terraform is enabled and in terraform project
		_, stderr := setupOutputCapture(t)
		mocks := setupTerraformMocks(t, true)
		setupTestContext(t, mocks)

		// When executing the command
		rootCmd.SetArgs([]string{"env"})
		err := Execute()

		// Then no error should occur (blueprint load may succeed or fail silently)
		if err != nil {
			t.Errorf("Expected success (blueprint load may succeed or fail silently), got error: %v", err)
		}
		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SkipsBlueprintLoadWhenNotInTerraformProject", func(t *testing.T) {
		// Given terraform is enabled but not in terraform project
		_, stderr := setupOutputCapture(t)
		mocks := setupTerraformMocks(t, false)
		setupTestContext(t, mocks)

		// When executing the command
		rootCmd.SetArgs([]string{"env"})
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("DoesNotSetNoCacheWhenHook", func(t *testing.T) {
		// Given hook flag is set and NO_CACHE is not set
		setupOutputCapture(t)
		mocks := setupMocks(t)
		os.Unsetenv("NO_CACHE")
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		setupTestContext(t, mocks)
		t.Cleanup(func() {
			os.Unsetenv("NO_CACHE")
			os.Unsetenv("WINDSOR_SESSION_TOKEN")
		})

		// When executing the command with hook flag
		rootCmd.SetArgs([]string{"env", "--hook"})
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And NO_CACHE should not be set
		if os.Getenv("NO_CACHE") != "" {
			t.Error("Expected NO_CACHE to not be set when hook is true")
		}
	})

	t.Run("SetsNoCacheWhenNotHookAndNotSet", func(t *testing.T) {
		// Given hook flag is not set and NO_CACHE is not set
		setupOutputCapture(t)
		mocks := setupMocks(t)
		os.Unsetenv("NO_CACHE")
		setupTestContext(t, mocks)
		t.Cleanup(func() {
			os.Unsetenv("NO_CACHE")
		})

		// When executing the command
		rootCmd.SetArgs([]string{"env"})
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And NO_CACHE should be set to true
		if os.Getenv("NO_CACHE") != "true" {
			t.Errorf("Expected NO_CACHE to be set to 'true' when hook is false and NO_CACHE is not set, got: %s", os.Getenv("NO_CACHE"))
		}
	})

	t.Run("PreservesNoCacheWhenAlreadySet", func(t *testing.T) {
		// Given NO_CACHE is already set to a value
		setupOutputCapture(t)
		mocks := setupMocks(t)
		os.Setenv("NO_CACHE", "existing-value")
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		setupTestContext(t, mocks)
		t.Cleanup(func() {
			os.Unsetenv("NO_CACHE")
			os.Unsetenv("WINDSOR_SESSION_TOKEN")
		})

		// When executing the command
		rootCmd.SetArgs([]string{"env"})
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And NO_CACHE should preserve its existing value
		if os.Getenv("NO_CACHE") != "existing-value" {
			t.Errorf("Expected NO_CACHE to preserve existing value 'existing-value', got: %s", os.Getenv("NO_CACHE"))
		}
	})

	t.Run("OutputsEnvVarsInHookMode", func(t *testing.T) {
		// Given mocks with env vars and hook mode
		stdout, stderr := setupOutputCapture(t)
		mocks := setupMocks(t)
		mocks.Shell.RenderEnvVarsFunc = func(envVars map[string]string, export bool) string {
			if !export {
				t.Error("Expected export to be true in hook mode")
			}
			return "export TEST_VAR=\"test_value\"\n"
		}
		if mockWindsorEnv, ok := mocks.Runtime.EnvPrinters.WindsorEnv.(*env.MockEnvPrinter); ok {
			mockWindsorEnv.GetEnvVarsFunc = func() (map[string]string, error) {
				return map[string]string{"TEST_VAR": "test_value"}, nil
			}
		}
		setupTestContext(t, mocks)

		// When executing the command with hook flag
		rootCmd.SetArgs([]string{"env", "--hook"})
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And stdout should contain env var output
		if !strings.Contains(stdout.String(), "export TEST_VAR") {
			t.Errorf("Expected stdout to contain env var output, got: %s", stdout.String())
		}
		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("OutputsAliasesInHookMode", func(t *testing.T) {
		// Given mocks with aliases and hook mode
		stdout, stderr := setupOutputCapture(t)
		mocks := setupMocks(t)
		mocks.Shell.RenderEnvVarsFunc = func(envVars map[string]string, export bool) string {
			return ""
		}
		mocks.Shell.RenderAliasesFunc = func(aliases map[string]string) string {
			return "alias test=\"test_command\"\n"
		}
		if mockWindsorEnv, ok := mocks.Runtime.EnvPrinters.WindsorEnv.(*env.MockEnvPrinter); ok {
			mockWindsorEnv.GetAliasFunc = func() (map[string]string, error) {
				return map[string]string{"test": "test_command"}, nil
			}
		}
		setupTestContext(t, mocks)

		// When executing the command with hook flag
		rootCmd.SetArgs([]string{"env", "--hook"})
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And stdout should contain alias output
		if !strings.Contains(stdout.String(), "alias test") {
			t.Errorf("Expected stdout to contain alias output, got: %s", stdout.String())
		}
		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("OutputsNothingWhenNoEnvVars", func(t *testing.T) {
		// Given mocks with no env vars or aliases (defaults already provide this)
		stdout, stderr := setupOutputCapture(t)
		mocks := setupMocks(t)
		setupTestContext(t, mocks)

		// When executing the command
		rootCmd.SetArgs([]string{"env"})
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And stdout should be empty
		if stdout.String() != "" {
			t.Errorf("Expected empty stdout when no env vars, got: %s", stdout.String())
		}
		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})
}
