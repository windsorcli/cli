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
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
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
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
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

		// Create runtime and composer with mock artifact builder override
		rt := &runtime.Runtime{
			Injector: injector,
		}
		rt, err := runtime.NewRuntime(rt)
		if err != nil {
			t.Fatalf("Failed to create runtime: %v", err)
		}
		mockComposer := composer.NewComposer(rt, &composer.Composer{
			ArtifactBuilder: mockArtifactBuilder,
		})

		// Create test command
		cmd := &cobra.Command{
			Use:   "bundle",
			Short: "Bundle blueprints into a .tar.gz archive",
			RunE:  bundleCmd.RunE,
		}
		cmd.Flags().StringP("output", "o", ".", "Output path for bundle archive")
		cmd.Flags().StringP("tag", "t", "", "Tag in 'name:version' format")

		// Set up context with composer overrides
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		ctx = context.WithValue(ctx, composerOverridesKey, mockComposer)
		cmd.SetContext(ctx)

		// Set arguments
		cmd.SetArgs([]string{"--tag", "test:v1.0.0"})

		// Execute command
		err = cmd.Execute()

		// Verify error
		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "failed to bundle artifacts") {
			t.Errorf("Expected error to contain 'failed to bundle artifacts', got %v", err)
		}
	})
}
