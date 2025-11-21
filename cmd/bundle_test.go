package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type BundleMocks struct {
	ConfigHandler    config.ConfigHandler
	Shell            *shell.MockShell
	BlueprintHandler *blueprint.MockBlueprintHandler
	ArtifactBuilder  *artifact.MockArtifact
	Composer         *composer.Composer
	Runtime          *Mocks
	TmpDir           string
}

func setupBundleTest(t *testing.T, opts ...*SetupOptions) *BundleMocks {
	t.Helper()

	tmpDir := t.TempDir()
	originalDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	t.Cleanup(func() {
		os.Chdir(originalDir)
	})

	contextsDir := filepath.Join(tmpDir, "contexts")
	templateDir := filepath.Join(contextsDir, "_template")
	os.MkdirAll(templateDir, 0755)

	baseMocks := setupMocks(t, opts...)

	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}
	mockShell.CheckTrustedDirectoryFunc = func() error {
		return nil
	}

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
		return map[string]any{}, nil
	}
	mockConfigHandler.GetContextFunc = func() string {
		return "test-context"
	}

	baseMocks.Runtime.Shell = mockShell
	baseMocks.Runtime.ConfigHandler = mockConfigHandler
	baseMocks.Runtime.ProjectRoot = tmpDir

	mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
	mockBlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
		return map[string][]byte{}, nil
	}

	comp := composer.NewComposer(baseMocks.Runtime)
	comp.BlueprintHandler = mockBlueprintHandler

	return &BundleMocks{
		ConfigHandler:    mockConfigHandler,
		Shell:            mockShell,
		BlueprintHandler: mockBlueprintHandler,
		ArtifactBuilder:  artifact.NewMockArtifact(),
		Composer:         comp,
		Runtime:          baseMocks,
		TmpDir:           tmpDir,
	}
}

// =============================================================================
// Test Cases
// =============================================================================

func TestBundleCmdWithRuntime(t *testing.T) {
	t.Run("SuccessWithRuntime", func(t *testing.T) {
		// Given a properly configured bundle command
		mocks := setupBundleTest(t)

		// When executing the bundle command with tag
		cmd := &cobra.Command{
			Use:   "bundle",
			Short: "Bundle blueprints into a .tar.gz archive",
			RunE:  bundleCmd.RunE,
		}
		cmd.Flags().StringP("output", "o", ".", "Output path for bundle archive")
		cmd.Flags().StringP("tag", "t", "", "Tag in 'name:version' format")

		ctx := context.Background()
		ctx = context.WithValue(ctx, runtimeOverridesKey, mocks.Runtime.Runtime)
		ctx = context.WithValue(ctx, composerOverridesKey, mocks.Composer)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"--tag", "test:v1.0.0"})
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("RuntimeSetupError", func(t *testing.T) {
		// Given a bundle command without runtime setup
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() {
			os.Chdir(originalDir)
		})

		// When executing the bundle command
		cmd := &cobra.Command{
			Use:   "bundle",
			Short: "Bundle blueprints into a .tar.gz archive",
			RunE:  bundleCmd.RunE,
		}
		cmd.Flags().StringP("output", "o", ".", "Output path for bundle archive")
		cmd.Flags().StringP("tag", "t", "", "Tag in 'name:version' format")

		ctx := context.Background()
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"--tag", "test:v1.0.0"})
		err := cmd.Execute()

		// Then no error should occur (runtime is resilient)
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("RuntimeExecutionError", func(t *testing.T) {
		// Given a bundle command with artifact write failure
		mocks := setupBundleTest(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		mockArtifactBuilder.WriteFunc = func(outputPath string, tag string) (string, error) {
			return "", fmt.Errorf("failed to write artifact")
		}
		mocks.Composer.ArtifactBuilder = mockArtifactBuilder

		// When executing the bundle command
		cmd := &cobra.Command{
			Use:   "bundle",
			Short: "Bundle blueprints into a .tar.gz archive",
			RunE:  bundleCmd.RunE,
		}
		cmd.Flags().StringP("output", "o", ".", "Output path for bundle archive")
		cmd.Flags().StringP("tag", "t", "", "Tag in 'name:version' format")

		ctx := context.Background()
		ctx = context.WithValue(ctx, runtimeOverridesKey, mocks.Runtime.Runtime)
		ctx = context.WithValue(ctx, composerOverridesKey, mocks.Composer)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"--tag", "test:v1.0.0"})
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		// And error should contain bundle failure message
		if !strings.Contains(err.Error(), "failed to bundle artifacts") {
			t.Errorf("Expected error to contain 'failed to bundle artifacts', got %v", err)
		}
	})
}
