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

func TestSetContextCmd(t *testing.T) {
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
		return stdout, stderr
	}

	t.Run("SetContext", func(t *testing.T) {
		_, _ = setup(t)
		tmpDir := t.TempDir()

		contextsDir := filepath.Join(tmpDir, "contexts")
		if err := os.MkdirAll(contextsDir, 0755); err != nil {
			t.Fatalf("Failed to create contexts directory: %v", err)
		}

		stagingDir := filepath.Join(contextsDir, "staging")
		if err := os.MkdirAll(stagingDir, 0755); err != nil {
			t.Fatalf("Failed to create staging context directory: %v", err)
		}

		windsorYaml := filepath.Join(tmpDir, "windsor.yaml")
		if err := os.WriteFile(windsorYaml, []byte("version: v1alpha1\n"), 0644); err != nil {
			t.Fatalf("Failed to create windsor.yaml: %v", err)
		}

		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}

		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"set", "context", "staging"})

		err := Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ContextNotFound", func(t *testing.T) {
		_, _ = setup(t)
		tmpDir := t.TempDir()

		windsorYaml := filepath.Join(tmpDir, "windsor.yaml")
		if err := os.WriteFile(windsorYaml, []byte("version: v1alpha1\n"), 0644); err != nil {
			t.Fatalf("Failed to create windsor.yaml: %v", err)
		}

		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}

		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"set", "context", "nonexistent"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when context does not exist")
			return
		}

		if !strings.Contains(err.Error(), "context \"nonexistent\" not found") {
			t.Errorf("Expected error about context not found, got: %v", err)
		}

		if !strings.Contains(err.Error(), "windsor init nonexistent") {
			t.Errorf("Expected error to suggest 'windsor init nonexistent', got: %v", err)
		}
	})

	t.Run("HandlesNewRuntimeError", func(t *testing.T) {
		setup(t)

		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}

		rtOverride := &runtime.Runtime{
			Shell:       mockShell,
			ProjectRoot: "",
		}
		_, err := runtime.NewRuntime(rtOverride)
		if err == nil {
			t.Fatal("Expected NewRuntime to fail with invalid shell")
		}

		ctx := context.WithValue(context.Background(), runtimeOverridesKey, rtOverride)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"set", "context", "test-context"})

		err = Execute()

		if err == nil {
			t.Error("Expected error when NewRuntime fails")
			return
		}

		if !strings.Contains(err.Error(), "failed to initialize runtime") {
			t.Errorf("Expected error about runtime initialization, got: %v", err)
		}
	})

	t.Run("HandlesGetProjectRootError", func(t *testing.T) {
		setup(t)

		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}

		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"set", "context", "test-context"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when GetProjectRoot fails")
			return
		}

		if !strings.Contains(err.Error(), "failed to get project root") {
			t.Errorf("Expected error about project root, got: %v", err)
		}
	})

	t.Run("HandlesLoadConfigError", func(t *testing.T) {
		setup(t)
		tmpDir := t.TempDir()

		contextsDir := filepath.Join(tmpDir, "contexts")
		if err := os.MkdirAll(contextsDir, 0755); err != nil {
			t.Fatalf("Failed to create contexts directory: %v", err)
		}

		stagingDir := filepath.Join(contextsDir, "staging")
		if err := os.MkdirAll(stagingDir, 0755); err != nil {
			t.Fatalf("Failed to create staging context directory: %v", err)
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

		rootCmd.SetArgs([]string{"set", "context", "staging"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when LoadConfig fails")
			return
		}

		if !strings.Contains(err.Error(), "failed to load config") {
			t.Errorf("Expected error about config loading, got: %v", err)
		}
	})

	t.Run("HandlesWriteResetTokenError", func(t *testing.T) {
		setup(t)
		tmpDir := t.TempDir()

		contextsDir := filepath.Join(tmpDir, "contexts")
		if err := os.MkdirAll(contextsDir, 0755); err != nil {
			t.Fatalf("Failed to create contexts directory: %v", err)
		}

		stagingDir := filepath.Join(contextsDir, "staging")
		if err := os.MkdirAll(stagingDir, 0755); err != nil {
			t.Fatalf("Failed to create staging context directory: %v", err)
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

		rootCmd.SetArgs([]string{"set", "context", "staging"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when WriteResetToken fails")
			return
		}

		if !strings.Contains(err.Error(), "failed to write reset token") {
			t.Errorf("Expected error about reset token, got: %v", err)
		}
	})

	t.Run("HandlesSetContextError", func(t *testing.T) {
		setup(t)
		tmpDir := t.TempDir()

		contextsDir := filepath.Join(tmpDir, "contexts")
		if err := os.MkdirAll(contextsDir, 0755); err != nil {
			t.Fatalf("Failed to create contexts directory: %v", err)
		}

		stagingDir := filepath.Join(contextsDir, "staging")
		if err := os.MkdirAll(stagingDir, 0755); err != nil {
			t.Fatalf("Failed to create staging context directory: %v", err)
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

		rootCmd.SetArgs([]string{"set", "context", "staging"})

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

