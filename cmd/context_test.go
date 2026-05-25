package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

func TestContextCmd(t *testing.T) {
	setup := func(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
		t.Helper()

		origContext := os.Getenv("WINDSOR_CONTEXT")
		os.Unsetenv("WINDSOR_CONTEXT")
		t.Cleanup(func() {
			if origContext != "" {
				os.Setenv("WINDSOR_CONTEXT", origContext)
			}
		})

		origDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get working directory: %v", err)
		}

		tmpDir := t.TempDir()
		// Anchor GetProjectRoot at tmpDir by dropping a minimal windsor.yaml. Without this,
		// GetProjectRoot walks past tmpDir and falls back to $HOME/.config/windsor, where the
		// developer's global state (.windsor/context) bleeds into the test result.
		if err := os.WriteFile(filepath.Join(tmpDir, "windsor.yaml"), []byte("version: v1alpha1\n"), 0644); err != nil {
			t.Fatalf("Failed to write anchor windsor.yaml: %v", err)
		}
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("Failed to change to temp directory: %v", err)
		}

		t.Cleanup(func() {
			if err := os.Chdir(origDir); err != nil {
				t.Logf("Warning: Failed to change back to original directory: %v", err)
			}
		})

		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)
		t.Cleanup(func() {
			rootCmd.SetOut(nil)
			rootCmd.SetErr(nil)
		})
		return stdout, stderr
	}

	t.Run("GetContext", func(t *testing.T) {
		stdout, _ := setup(t)
		rootCmd.SetContext(context.Background())
		rootCmd.SetArgs([]string{"get", "context"})

		err := Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		output := stdout.String()
		expectedOutput := "local\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetContextNoArgs", func(t *testing.T) {
		_, _ = setup(t)
		setupMocks(t)

		rootCmd.SetArgs([]string{"set", "context"})

		err := Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "accepts 1 arg(s), received 0"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SetContext", func(t *testing.T) {
		_, _ = setup(t)
		mocks := setupMocks(t)
		tmpDir := mocks.TmpDir

		contextsDir := filepath.Join(tmpDir, "contexts")
		if err := os.MkdirAll(contextsDir, 0755); err != nil {
			t.Fatalf("Failed to create contexts directory: %v", err)
		}

		newContextDir := filepath.Join(contextsDir, "new-context")
		if err := os.MkdirAll(newContextDir, 0755); err != nil {
			t.Fatalf("Failed to create new-context directory: %v", err)
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}

		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"set", "context", "new-context"})

		err := Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SetContextMissingContextPrintsSingleError", func(t *testing.T) {
		_, stderr := setup(t)
		mocks := setupMocks(t)
		tmpDir := mocks.TmpDir

		contextsDir := filepath.Join(tmpDir, "contexts")
		if err := os.MkdirAll(contextsDir, 0755); err != nil {
			t.Fatalf("Failed to create contexts directory: %v", err)
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}

		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)
		rootCmd.SetArgs([]string{"set", "context", "missing-context"})

		err := Execute()
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		expected := "Error: context \"missing-context\" not found. Run 'windsor init missing-context' to create it"
		if occurrences := strings.Count(stderr.String(), expected); occurrences != 1 {
			t.Errorf("Expected one error occurrence, got %d. stderr: %q", occurrences, stderr.String())
		}
	})

	t.Run("GetContextAlias", func(t *testing.T) {
		stdout, _ := setup(t)

		rootCmd.SetArgs([]string{"get-context"})

		err := Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		output := stdout.String()
		if output == "" {
			t.Error("Expected some output, got empty string")
		}
	})

	t.Run("SetContextAliasNoArgs", func(t *testing.T) {
		_, _ = setup(t)
		setupMocks(t)

		rootCmd.SetArgs([]string{"set-context"})

		err := Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "accepts 1 arg(s), received 0"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SetContextAlias", func(t *testing.T) {
		_, _ = setup(t)
		mocks := setupMocks(t)
		tmpDir := mocks.TmpDir

		contextsDir := filepath.Join(tmpDir, "contexts")
		if err := os.MkdirAll(contextsDir, 0755); err != nil {
			t.Fatalf("Failed to create contexts directory: %v", err)
		}

		newContextDir := filepath.Join(contextsDir, "new-context")
		if err := os.MkdirAll(newContextDir, 0755); err != nil {
			t.Fatalf("Failed to create new-context directory: %v", err)
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}

		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"set-context", "new-context"})

		err := Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("RemovedContextGroupRejected", func(t *testing.T) {
		_, _ = setup(t)
		rootCmd.SetArgs([]string{"context", "get"})

		err := Execute()

		if err == nil {
			t.Fatal("Expected error for removed 'context' command group, got nil")
		}
		if !strings.Contains(err.Error(), "unknown command \"context\"") {
			t.Errorf("Expected 'unknown command \"context\"' error, got: %v", err)
		}
	})
}

// =============================================================================
// Test Error Scenarios
// =============================================================================

func TestContextCmd_ErrorScenarios(t *testing.T) {
	t.Cleanup(func() {
		rootCmd.SetContext(context.Background())
	})

	setup := func(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
		t.Helper()
		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)
		return stdout, stderr
	}

	t.Run("GetContext_HandlesNewRuntimeError", func(t *testing.T) {
		setup(t)

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
	})

	t.Run("GetContext_HandlesLoadConfigError", func(t *testing.T) {
		setup(t)

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error {
			return fmt.Errorf("config load failed")
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mocks := setupMocks(t, &SetupOptions{ConfigHandler: mockConfigHandler})

		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"get", "context"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when LoadConfig fails")
		}

		if !strings.Contains(err.Error(), "failed to load config") {
			t.Errorf("Expected error about config loading, got: %v", err)
		}
	})

	t.Run("SetContext_HandlesNewRuntimeError", func(t *testing.T) {
		setup(t)

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

		rootCmd.SetArgs([]string{"set", "context", "test-context"})

		_ = Execute()
	})

	t.Run("SetContext_HandlesLoadConfigError", func(t *testing.T) {
		setup(t)
		tmpDir := t.TempDir()

		contextsDir := filepath.Join(tmpDir, "contexts")
		if err := os.MkdirAll(contextsDir, 0755); err != nil {
			t.Fatalf("Failed to create contexts directory: %v", err)
		}

		testContextDir := filepath.Join(contextsDir, "test-context")
		if err := os.MkdirAll(testContextDir, 0755); err != nil {
			t.Fatalf("Failed to create test-context directory: %v", err)
		}

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error {
			return fmt.Errorf("config load failed")
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mocks := setupMocks(t, &SetupOptions{ConfigHandler: mockConfigHandler})
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}

		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"set", "context", "test-context"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when LoadConfig fails")
			return
		}

		if !strings.Contains(err.Error(), "failed to load config") {
			t.Errorf("Expected error about config loading, got: %v", err)
		}
	})

	t.Run("SetContext_HandlesWriteResetTokenError", func(t *testing.T) {
		setup(t)
		tmpDir := t.TempDir()

		contextsDir := filepath.Join(tmpDir, "contexts")
		if err := os.MkdirAll(contextsDir, 0755); err != nil {
			t.Fatalf("Failed to create contexts directory: %v", err)
		}

		testContextDir := filepath.Join(contextsDir, "test-context")
		if err := os.MkdirAll(testContextDir, 0755); err != nil {
			t.Fatalf("Failed to create test-context directory: %v", err)
		}

		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		mocks.Shell.WriteResetTokenFunc = func() (string, error) {
			return "", fmt.Errorf("write reset token failed")
		}

		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"set", "context", "test-context"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when WriteResetToken fails")
			return
		}

		if !strings.Contains(err.Error(), "failed to write reset token") {
			t.Errorf("Expected error about reset token, got: %v", err)
		}
	})

	t.Run("SetContext_HandlesSetContextError", func(t *testing.T) {
		setup(t)
		tmpDir := t.TempDir()

		contextsDir := filepath.Join(tmpDir, "contexts")
		if err := os.MkdirAll(contextsDir, 0755); err != nil {
			t.Fatalf("Failed to create contexts directory: %v", err)
		}

		testContextDir := filepath.Join(contextsDir, "test-context")
		if err := os.MkdirAll(testContextDir, 0755); err != nil {
			t.Fatalf("Failed to create test-context directory: %v", err)
		}

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error {
			return nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.SetContextFunc = func(context string) error {
			return fmt.Errorf("set context failed")
		}
		mocks := setupMocks(t, &SetupOptions{ConfigHandler: mockConfigHandler})
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		mocks.Shell.WriteResetTokenFunc = func() (string, error) {
			return "mock-reset-token", nil
		}

		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"set", "context", "test-context"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when SetContext fails")
			return
		}

		if !strings.Contains(err.Error(), "failed to set context") {
			t.Errorf("Expected error about setting context, got: %v", err)
		}
	})
}
