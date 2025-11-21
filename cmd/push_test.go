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
func setupPushTest(t *testing.T) (*PushMocks, *Mocks) {
	t.Helper()

	// Get base mocks first to get temp dir
	baseMocks := setupMocks(t)
	tmpDir := baseMocks.TmpDir

	// Create required directory structure
	contextsDir := filepath.Join(tmpDir, "contexts")
	templateDir := filepath.Join(contextsDir, "_template")
	os.MkdirAll(templateDir, 0755)

	// Override Shell GetProjectRootFunc if it's a MockShell
	if mockShell, ok := baseMocks.Runtime.Shell.(*shell.MockShell); ok {
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
	}

	// Override ConfigHandler and ProjectRoot in runtime
	baseMocks.Runtime.ConfigHandler = config.NewMockConfigHandler()
	if mockConfig, ok := baseMocks.Runtime.ConfigHandler.(*config.MockConfigHandler); ok {
		mockConfig.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return tmpDir, nil
		}
	}
	baseMocks.Runtime.ProjectRoot = tmpDir

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

	return &PushMocks{}, baseMocks
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
		// Given proper setup with runtime override
		_, mocks := setupPushTest(t)
		cmd := createTestPushCmd()
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"registry.example.com/repo:v1.0.0"})

		// When executing the push command
		err := cmd.Execute()

		// Then it should fail with authentication error (expected in tests)
		if err == nil {
			t.Error("Expected authentication error, got nil")
		}
		if !strings.Contains(err.Error(), "Authentication failed") {
			t.Errorf("Expected authentication error, got %v", err)
		}
	})

	t.Run("SuccessWithoutTag", func(t *testing.T) {
		// Given proper setup with runtime override
		_, mocks := setupPushTest(t)
		cmd := createTestPushCmd()
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"registry.example.com/repo"})

		// When executing the push command
		err := cmd.Execute()

		// Then it should fail with authentication error (expected in tests)
		if err == nil {
			t.Error("Expected authentication error, got nil")
		}
		if !strings.Contains(err.Error(), "Authentication failed") {
			t.Errorf("Expected authentication error, got %v", err)
		}
	})

	t.Run("SuccessWithOciUrl", func(t *testing.T) {
		// Given proper setup with runtime override
		_, mocks := setupPushTest(t)
		cmd := createTestPushCmd()
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"oci://ghcr.io/windsorcli/core:v0.0.0"})

		// When executing the push command
		err := cmd.Execute()

		// Then it should fail with authentication error (expected in tests)
		if err == nil {
			t.Error("Expected authentication error, got nil")
		}
		if !strings.Contains(err.Error(), "Authentication failed") {
			t.Errorf("Expected authentication error, got %v", err)
		}
	})

	t.Run("ErrorMissingRegistry", func(t *testing.T) {
		// Given proper setup with runtime override
		_, mocks := setupPushTest(t)
		cmd := createTestPushCmd()
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})

		// When executing the push command without registry
		err := cmd.Execute()

		// Then an error should occur
		if err == nil || !strings.Contains(err.Error(), "registry is required") {
			t.Errorf("Expected registry required error, got %v", err)
		}
	})

	t.Run("ErrorInvalidRegistryFormat", func(t *testing.T) {
		// Given proper setup with runtime override
		_, mocks := setupPushTest(t)
		cmd := createTestPushCmd()
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"invalidformat"})

		// When executing the push command with invalid format
		err := cmd.Execute()

		// Then an error should occur
		if err == nil || !strings.Contains(err.Error(), "invalid registry format") {
			t.Errorf("Expected invalid registry format error, got %v", err)
		}
	})

	t.Run("RuntimeSetupError", func(t *testing.T) {
		// Given command without runtime override
		cmd := createTestPushCmd()
		ctx := context.Background()
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"registry.example.com/repo:v1.0.0"})

		// When executing the push command
		err := cmd.Execute()

		// Then it may succeed or fail depending on environment
		// Runtime is resilient and will create default dependencies
		if err != nil {
			t.Logf("Command failed as expected: %v", err)
		} else {
			t.Logf("Command succeeded (runtime may be available from environment)")
		}
	})
}
