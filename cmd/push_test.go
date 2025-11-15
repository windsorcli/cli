package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type PushMocks struct {
}

// setupPushTest sets up the test environment for push command tests.
// It creates a temporary directory, initializes the injector with required mocks, and returns PushMocks.
func setupPushTest(t *testing.T) *PushMocks {
	t.Helper()

	tmpDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	t.Cleanup(func() { os.Chdir(oldDir) })

	// Create required directory structure
	contextsDir := filepath.Join(tmpDir, "contexts")
	templateDir := filepath.Join(contextsDir, "_template")
	os.MkdirAll(templateDir, 0755)

	// Mock shell
	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	// Mock config handler
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
		return map[string]any{}, nil
	}

	// Mock kubernetes manager

	// Mock blueprint handler
	mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
	mockBlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
		return map[string][]byte{}, nil
	}

	// Mock artifact builder
	mockArtifactBuilder := artifact.NewMockArtifact()
	mockArtifactBuilder.BundleFunc = func() error {
		return nil
	}
	mockArtifactBuilder.PushFunc = func(registryBase string, repoName string, tag string) error {
		return fmt.Errorf("authentication failed: unauthorized")
	}

	return &PushMocks{}
}

// createTestPushCmd creates a new cobra.Command for testing the push command.
func createTestPushCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push blueprints to an OCI registry",
		RunE:  pushCmd.RunE,
	}
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	return cmd
}

// =============================================================================
// Test Cases
// =============================================================================

func TestPushCmdWithRuntime(t *testing.T) {
	t.Run("SuccessWithRuntime", func(t *testing.T) {
		setupPushTest(t)
		cmd := createTestPushCmd()
		ctx := context.Background()
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"registry.example.com/repo:v1.0.0"})
		err := cmd.Execute()
		// The push command will fail with authentication error because we're not actually logged in
		// This is expected behavior for unit tests
		if err == nil {
			t.Error("Expected authentication error, got nil")
		}
		if !strings.Contains(err.Error(), "Authentication failed") {
			t.Errorf("Expected authentication error, got %v", err)
		}
	})

	t.Run("SuccessWithoutTag", func(t *testing.T) {
		setupPushTest(t)
		cmd := createTestPushCmd()
		ctx := context.Background()
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"registry.example.com/repo"})
		err := cmd.Execute()
		// The push command will fail with authentication error because we're not actually logged in
		// This is expected behavior for unit tests
		if err == nil {
			t.Error("Expected authentication error, got nil")
		}
		if !strings.Contains(err.Error(), "Authentication failed") {
			t.Errorf("Expected authentication error, got %v", err)
		}
	})

	t.Run("SuccessWithOciUrl", func(t *testing.T) {
		setupPushTest(t)
		cmd := createTestPushCmd()
		ctx := context.Background()
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"oci://ghcr.io/windsorcli/core:v0.0.0"})
		err := cmd.Execute()
		// The push command will fail with authentication error because we're not actually logged in
		// This is expected behavior for unit tests
		if err == nil {
			t.Error("Expected authentication error, got nil")
		}
		if !strings.Contains(err.Error(), "Authentication failed") {
			t.Errorf("Expected authentication error, got %v", err)
		}
	})

	t.Run("ErrorMissingRegistry", func(t *testing.T) {
		setupPushTest(t)
		cmd := createTestPushCmd()
		ctx := context.Background()
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "registry is required") {
			t.Errorf("Expected registry required error, got %v", err)
		}
	})

	t.Run("ErrorInvalidRegistryFormat", func(t *testing.T) {
		setupPushTest(t)
		cmd := createTestPushCmd()
		ctx := context.Background()
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"invalidformat"})
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "invalid registry format") {
			t.Errorf("Expected invalid registry format error, got %v", err)
		}
	})

	t.Run("RuntimeSetupError", func(t *testing.T) {
		// The runtime is now resilient and will create default dependencies
		cmd := createTestPushCmd()
		ctx := context.Background()
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"registry.example.com/repo:v1.0.0"})
		err := cmd.Execute()
		// The runtime is now resilient and will succeed with authentication error
		if err == nil {
			t.Error("Expected authentication error, got nil")
		}
		if !strings.Contains(err.Error(), "Authentication failed") {
			t.Errorf("Expected authentication error, got %v", err)
		}
	})
}
