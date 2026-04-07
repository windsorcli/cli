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

func TestGetContextsCmd(t *testing.T) {
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

	t.Run("NoContextsFound", func(t *testing.T) {
		stdout, _ := setup(t)

		rootCmd.SetArgs([]string{"get", "contexts"})

		err := Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		output := stdout.String()
		if !strings.Contains(output, "No contexts found") {
			t.Errorf("Expected 'No contexts found' message, got %q", output)
		}
	})

	t.Run("ListsContextsWithMetadata", func(t *testing.T) {
		stdout, _ := setup(t)
		tmpDir := t.TempDir()

		contextsDir := filepath.Join(tmpDir, "contexts")
		if err := os.MkdirAll(contextsDir, 0755); err != nil {
			t.Fatalf("Failed to create contexts directory: %v", err)
		}

		localDir := filepath.Join(contextsDir, "local")
		if err := os.MkdirAll(localDir, 0755); err != nil {
			t.Fatalf("Failed to create local context directory: %v", err)
		}

		stagingDir := filepath.Join(contextsDir, "staging")
		if err := os.MkdirAll(stagingDir, 0755); err != nil {
			t.Fatalf("Failed to create staging context directory: %v", err)
		}

		windsorYaml := filepath.Join(tmpDir, "windsor.yaml")
		if err := os.WriteFile(windsorYaml, []byte("version: v1alpha1\n"), 0644); err != nil {
			t.Fatalf("Failed to create windsor.yaml: %v", err)
		}

		localConfig := filepath.Join(localDir, "windsor.yaml")
		if err := os.WriteFile(localConfig, []byte("provider: generic\nterraform:\n  backend:\n    type: local\n"), 0644); err != nil {
			t.Fatalf("Failed to create local config: %v", err)
		}

		stagingConfig := filepath.Join(stagingDir, "windsor.yaml")
		if err := os.WriteFile(stagingConfig, []byte("provider: aws\nterraform:\n  backend:\n    type: s3\n"), 0644); err != nil {
			t.Fatalf("Failed to create staging config: %v", err)
		}

		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}

		rtOverride := &runtime.Runtime{
			Shell:       mockShell,
			ProjectRoot: tmpDir,
		}
		rt := runtime.NewRuntime(rtOverride)
		if rt == nil {
			t.Fatal("Failed to create runtime")
		}

		ctx := context.WithValue(context.Background(), runtimeOverridesKey, rt)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"get", "contexts"})

		err := Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		output := stdout.String()
		if !strings.Contains(output, "NAME") {
			t.Errorf("Expected table header with NAME, got %q", output)
		}
		if !strings.Contains(output, "PROVIDER") {
			t.Errorf("Expected table header with PROVIDER, got %q", output)
		}
		if !strings.Contains(output, "BACKEND") {
			t.Errorf("Expected table header with BACKEND, got %q", output)
		}
		if !strings.Contains(output, "CURRENT") {
			t.Errorf("Expected table header with CURRENT, got %q", output)
		}
		if !strings.Contains(output, "local") {
			t.Errorf("Expected 'local' context in output, got %q", output)
		}
		if !strings.Contains(output, "staging") {
			t.Errorf("Expected 'staging' context in output, got %q", output)
		}
	})

	t.Run("ExcludesTemplateDirectory", func(t *testing.T) {
		stdout, _ := setup(t)
		tmpDir := t.TempDir()

		contextsDir := filepath.Join(tmpDir, "contexts")
		if err := os.MkdirAll(contextsDir, 0755); err != nil {
			t.Fatalf("Failed to create contexts directory: %v", err)
		}

		templateDir := filepath.Join(contextsDir, "_template")
		if err := os.MkdirAll(templateDir, 0755); err != nil {
			t.Fatalf("Failed to create _template directory: %v", err)
		}

		localDir := filepath.Join(contextsDir, "local")
		if err := os.MkdirAll(localDir, 0755); err != nil {
			t.Fatalf("Failed to create local context directory: %v", err)
		}

		windsorYaml := filepath.Join(tmpDir, "windsor.yaml")
		if err := os.WriteFile(windsorYaml, []byte("version: v1alpha1\n"), 0644); err != nil {
			t.Fatalf("Failed to create windsor.yaml: %v", err)
		}

		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}

		rtOverride := &runtime.Runtime{
			Shell:       mockShell,
			ProjectRoot: tmpDir,
		}
		rt := runtime.NewRuntime(rtOverride)
		if rt == nil {
			t.Fatal("Failed to create runtime")
		}

		ctx := context.WithValue(context.Background(), runtimeOverridesKey, rt)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"get", "contexts"})

		err := Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		output := stdout.String()
		if strings.Contains(output, "_template") {
			t.Errorf("Expected _template to be excluded from output, got %q", output)
		}
		if !strings.Contains(output, "local") {
			t.Errorf("Expected 'local' context in output, got %q", output)
		}
	})

	t.Run("MarksCurrentContext", func(t *testing.T) {
		stdout, _ := setup(t)
		tmpDir := t.TempDir()

		contextsDir := filepath.Join(tmpDir, "contexts")
		if err := os.MkdirAll(contextsDir, 0755); err != nil {
			t.Fatalf("Failed to create contexts directory: %v", err)
		}

		localDir := filepath.Join(contextsDir, "local")
		if err := os.MkdirAll(localDir, 0755); err != nil {
			t.Fatalf("Failed to create local context directory: %v", err)
		}

		windsorYaml := filepath.Join(tmpDir, "windsor.yaml")
		if err := os.WriteFile(windsorYaml, []byte("version: v1alpha1\n"), 0644); err != nil {
			t.Fatalf("Failed to create windsor.yaml: %v", err)
		}

		windsorDir := filepath.Join(tmpDir, ".windsor")
		if err := os.MkdirAll(windsorDir, 0755); err != nil {
			t.Fatalf("Failed to create .windsor directory: %v", err)
		}

		contextFile := filepath.Join(windsorDir, "context")
		if err := os.WriteFile(contextFile, []byte("local\n"), 0644); err != nil {
			t.Fatalf("Failed to create context file: %v", err)
		}

		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}

		rtOverride := &runtime.Runtime{
			Shell:       mockShell,
			ProjectRoot: tmpDir,
		}
		rt := runtime.NewRuntime(rtOverride)
		if rt == nil {
			t.Fatal("Failed to create runtime")
		}

		ctx := context.WithValue(context.Background(), runtimeOverridesKey, rt)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"get", "contexts"})

		err := Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		output := stdout.String()
		if !strings.Contains(output, "*") {
			t.Errorf("Expected current context marker '*', got %q", output)
		}
	})

	t.Run("DoesNotChangeCurrentContext", func(t *testing.T) {
		stdout, _ := setup(t)
		tmpDir := t.TempDir()

		contextsDir := filepath.Join(tmpDir, "contexts")
		if err := os.MkdirAll(contextsDir, 0755); err != nil {
			t.Fatalf("Failed to create contexts directory: %v", err)
		}

		localDir := filepath.Join(contextsDir, "local")
		if err := os.MkdirAll(localDir, 0755); err != nil {
			t.Fatalf("Failed to create local context directory: %v", err)
		}

		stagingDir := filepath.Join(contextsDir, "staging")
		if err := os.MkdirAll(stagingDir, 0755); err != nil {
			t.Fatalf("Failed to create staging context directory: %v", err)
		}

		windsorYaml := filepath.Join(tmpDir, "windsor.yaml")
		if err := os.WriteFile(windsorYaml, []byte("version: v1alpha1\n"), 0644); err != nil {
			t.Fatalf("Failed to create windsor.yaml: %v", err)
		}

		windsorDir := filepath.Join(tmpDir, ".windsor")
		if err := os.MkdirAll(windsorDir, 0755); err != nil {
			t.Fatalf("Failed to create .windsor directory: %v", err)
		}

		contextFile := filepath.Join(windsorDir, "context")
		if err := os.WriteFile(contextFile, []byte("local\n"), 0644); err != nil {
			t.Fatalf("Failed to create context file: %v", err)
		}

		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}

		rtOverride := &runtime.Runtime{
			Shell:       mockShell,
			ProjectRoot: tmpDir,
		}
		rt := runtime.NewRuntime(rtOverride)
		if rt == nil {
			t.Fatal("Failed to create runtime")
		}

		ctx := context.WithValue(context.Background(), runtimeOverridesKey, rt)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"get", "contexts"})

		err := Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		contextFileAfter := filepath.Join(windsorDir, "context")
		contextData, err := os.ReadFile(contextFileAfter)
		if err != nil {
			t.Fatalf("Failed to read context file after listing: %v", err)
		}

		currentContext := strings.TrimSpace(string(contextData))
		if currentContext != "local" {
			t.Errorf("Expected current context to remain 'local', got '%s'", currentContext)
		}

		output := stdout.String()
		if !strings.Contains(output, "local") {
			t.Errorf("Expected 'local' context in output, got %q", output)
		}
		if !strings.Contains(output, "staging") {
			t.Errorf("Expected 'staging' context in output, got %q", output)
		}
	})
}

func TestGetContextCmd(t *testing.T) {
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

	t.Run("GetCurrentContext", func(t *testing.T) {
		stdout, _ := setup(t)

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
		// NewRuntime will panic with invalid shell, so we test that
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected NewRuntime to panic with invalid shell")
			}
		}()
		_ = runtime.NewRuntime(rtOverride)

		ctx := context.WithValue(context.Background(), runtimeOverridesKey, rtOverride)
		rootCmd.SetContext(ctx)

		// Note: NewRuntime will panic, so Execute won't be reached
		// This test needs to be updated to test for panics instead
		rootCmd.SetArgs([]string{"get", "context"})
		_ = Execute()
	})

	t.Run("HandlesLoadConfigError", func(t *testing.T) {
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
}

func TestGetContextsCmd_ErrorScenarios(t *testing.T) {
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
		// NewRuntime will panic with invalid shell, so we test that
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected NewRuntime to panic with invalid shell")
			}
		}()
		_ = runtime.NewRuntime(rtOverride)

		ctx := context.WithValue(context.Background(), runtimeOverridesKey, rtOverride)
		rootCmd.SetContext(ctx)

		// Note: NewRuntime will panic, so Execute won't be reached
		// This test needs to be updated to test for panics instead
		rootCmd.SetArgs([]string{"get", "contexts"})
		_ = Execute()
	})

	t.Run("HandlesGetProjectRootError", func(t *testing.T) {
		setup(t)

		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}

		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"get", "contexts"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when GetProjectRoot fails")
			return
		}

		if !strings.Contains(err.Error(), "failed to get project root") {
			t.Errorf("Expected error about project root, got: %v", err)
		}
	})
}
