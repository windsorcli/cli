package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/kubernetes"
	"github.com/windsorcli/cli/pkg/resources/artifact"
	"github.com/windsorcli/cli/pkg/resources/blueprint"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Runtime-based Tests
// =============================================================================

func TestBundleCmdWithRuntime(t *testing.T) {
	t.Run("SuccessWithRuntime", func(t *testing.T) {
		// Set up temporary directory
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		// Create required directory structure
		contextsDir := filepath.Join(tmpDir, "contexts")
		templateDir := filepath.Join(contextsDir, "_template")
		os.MkdirAll(templateDir, 0755)

		// Create injector with required mocks
		injector := di.NewInjector()

		// Mock shell
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		injector.Register("shell", mockShell)

		// Mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		injector.Register("configHandler", mockConfigHandler)

		// Mock kubernetes manager
		mockK8sManager := kubernetes.NewMockKubernetesManager(injector)
		injector.Register("kubernetesManager", mockK8sManager)

		// Mock blueprint handler
		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(injector)
		mockBlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return map[string][]byte{}, nil
		}
		injector.Register("blueprintHandler", mockBlueprintHandler)

		// Mock artifact builder
		mockArtifactBuilder := artifact.NewMockArtifact()
		injector.Register("artifactBuilder", mockArtifactBuilder)

		// Create test command
		cmd := &cobra.Command{
			Use:   "bundle",
			Short: "Bundle blueprints into a .tar.gz archive",
			RunE:  bundleCmd.RunE,
		}
		cmd.Flags().StringP("output", "o", ".", "Output path for bundle archive")
		cmd.Flags().StringP("tag", "t", "", "Tag in 'name:version' format")

		// Set up context
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		// Set arguments
		cmd.SetArgs([]string{"--tag", "test:v1.0.0"})

		// Execute command
		err := cmd.Execute()

		// Verify no error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("RuntimeSetupError", func(t *testing.T) {
		// Set up temporary directory
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		// Create injector without required dependencies
		// The runtime is now resilient and will create default dependencies
		injector := di.NewInjector()

		// Create test command
		cmd := &cobra.Command{
			Use:   "bundle",
			Short: "Bundle blueprints into a .tar.gz archive",
			RunE:  bundleCmd.RunE,
		}
		cmd.Flags().StringP("output", "o", ".", "Output path for bundle archive")
		cmd.Flags().StringP("tag", "t", "", "Tag in 'name:version' format")

		// Set up context
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		// Set arguments
		cmd.SetArgs([]string{"--tag", "test:v1.0.0"})

		// Execute command
		err := cmd.Execute()

		// Verify success - runtime is now resilient and creates default dependencies
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("RuntimeExecutionError", func(t *testing.T) {
		// Set up temporary directory
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		// Create required directory structure
		contextsDir := filepath.Join(tmpDir, "contexts")
		templateDir := filepath.Join(contextsDir, "_template")
		os.MkdirAll(templateDir, 0755)

		// Create injector with mocks that will cause execution to fail
		injector := di.NewInjector()

		// Mock shell
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		injector.Register("shell", mockShell)

		// Mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		injector.Register("configHandler", mockConfigHandler)

		// Mock kubernetes manager
		mockK8sManager := kubernetes.NewMockKubernetesManager(injector)
		injector.Register("kubernetesManager", mockK8sManager)

		// Mock blueprint handler
		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(injector)
		mockBlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return map[string][]byte{}, nil
		}
		injector.Register("blueprintHandler", mockBlueprintHandler)

		// Mock artifact builder that fails during bundle
		mockArtifactBuilder := artifact.NewMockArtifact()
		mockArtifactBuilder.WriteFunc = func(outputPath string, tag string) (string, error) {
			return "", fmt.Errorf("artifact bundle failed")
		}
		injector.Register("artifactBuilder", mockArtifactBuilder)

		// Create test command
		cmd := &cobra.Command{
			Use:   "bundle",
			Short: "Bundle blueprints into a .tar.gz archive",
			RunE:  bundleCmd.RunE,
		}
		cmd.Flags().StringP("output", "o", ".", "Output path for bundle archive")
		cmd.Flags().StringP("tag", "t", "", "Tag in 'name:version' format")

		// Set up context
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		// Set arguments
		cmd.SetArgs([]string{"--tag", "test:v1.0.0"})

		// Execute command
		err := cmd.Execute()

		// Verify error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "artifact bundle failed") {
			t.Errorf("Expected error to contain 'artifact bundle failed', got %v", err)
		}
	})
}
