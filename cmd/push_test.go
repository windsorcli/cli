package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
)

// =============================================================================
// Pipeline-based Tests
// =============================================================================

func TestPushCmdWithPipeline(t *testing.T) {
	t.Run("SuccessWithPipeline", func(t *testing.T) {
		// Set up temporary directory
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		// Create injector and mock artifact pipeline
		injector := di.NewInjector()
		mockArtifactPipeline := pipelines.NewMockBasePipeline()
		mockArtifactPipeline.ExecuteFunc = func(ctx context.Context) error {
			// Verify context values
			mode, ok := ctx.Value("artifactMode").(string)
			if !ok || mode != "push" {
				return fmt.Errorf("expected artifactMode 'push', got %v", mode)
			}
			registryBase, ok := ctx.Value("registryBase").(string)
			if !ok || registryBase != "registry.example.com" {
				return fmt.Errorf("expected registryBase 'registry.example.com', got %v", registryBase)
			}
			repoName, ok := ctx.Value("repoName").(string)
			if !ok || repoName != "repo" {
				return fmt.Errorf("expected repoName 'repo', got %v", repoName)
			}
			tag, ok := ctx.Value("tag").(string)
			if !ok || tag != "v1.0.0" {
				return fmt.Errorf("expected tag 'v1.0.0', got %v", tag)
			}
			return nil
		}

		// Register the mock pipeline
		injector.Register("artifactPipeline", mockArtifactPipeline)

		// Create test command
		cmd := &cobra.Command{
			Use:   "push",
			Short: "Push blueprints to an OCI registry",
			RunE:  pushCmd.RunE,
		}

		// Set up context
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		// Set arguments
		cmd.SetArgs([]string{"registry.example.com/repo:v1.0.0"})

		// Execute command
		err := cmd.Execute()

		// Verify no error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithoutTag", func(t *testing.T) {
		// Set up temporary directory
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		// Create injector and mock artifact pipeline
		injector := di.NewInjector()
		mockArtifactPipeline := pipelines.NewMockBasePipeline()
		mockArtifactPipeline.ExecuteFunc = func(ctx context.Context) error {
			// Verify context values
			mode, ok := ctx.Value("artifactMode").(string)
			if !ok || mode != "push" {
				return fmt.Errorf("expected artifactMode 'push', got %v", mode)
			}
			registryBase, ok := ctx.Value("registryBase").(string)
			if !ok || registryBase != "registry.example.com" {
				return fmt.Errorf("expected registryBase 'registry.example.com', got %v", registryBase)
			}
			repoName, ok := ctx.Value("repoName").(string)
			if !ok || repoName != "repo" {
				return fmt.Errorf("expected repoName 'repo', got %v", repoName)
			}
			tag, ok := ctx.Value("tag").(string)
			if !ok || tag != "" {
				return fmt.Errorf("expected empty tag, got %v", tag)
			}
			return nil
		}

		// Register the mock pipeline
		injector.Register("artifactPipeline", mockArtifactPipeline)

		// Create test command
		cmd := &cobra.Command{
			Use:   "push",
			Short: "Push blueprints to an OCI registry",
			RunE:  pushCmd.RunE,
		}

		// Set up context
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		// Set arguments
		cmd.SetArgs([]string{"registry.example.com/repo"})

		// Execute command
		err := cmd.Execute()

		// Verify no error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorMissingRegistry", func(t *testing.T) {
		// Set up temporary directory
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		// Create injector
		injector := di.NewInjector()

		// Create test command
		cmd := &cobra.Command{
			Use:   "push",
			Short: "Push blueprints to an OCI registry",
			RunE:  pushCmd.RunE,
		}

		// Set up context
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		// Set arguments (no registry provided)
		cmd.SetArgs([]string{})

		// Execute command
		err := cmd.Execute()

		// Verify error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		expectedError := "registry is required: windsor push registry/repo[:tag]"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorInvalidRegistryFormat", func(t *testing.T) {
		// Set up temporary directory
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		// Create injector
		injector := di.NewInjector()

		// Create test command
		cmd := &cobra.Command{
			Use:   "push",
			Short: "Push blueprints to an OCI registry",
			RunE:  pushCmd.RunE,
		}

		// Set up context
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		// Set arguments (invalid registry format)
		cmd.SetArgs([]string{"registry.example.com"})

		// Execute command
		err := cmd.Execute()

		// Verify error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		expectedError := "invalid registry format: must include repository path (e.g., registry.com/namespace/repo)"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("PipelineSetupError", func(t *testing.T) {
		// Set up temporary directory
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		// Create injector without registering the pipeline
		// This will cause WithPipeline to try to create a new one, which will fail
		// because it requires the contexts/_template directory
		injector := di.NewInjector()

		// Create test command
		cmd := &cobra.Command{
			Use:   "push",
			Short: "Push blueprints to an OCI registry",
			RunE:  pushCmd.RunE,
		}

		// Set up context
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		// Set arguments
		cmd.SetArgs([]string{"registry.example.com/repo:v1.0.0"})

		// Execute command
		err := cmd.Execute()

		// Verify error - the pipeline setup should fail because the real artifact pipeline
		// requires the contexts/_template directory which doesn't exist in the test
		if err == nil {
			t.Error("Expected error, got nil")
		}
		expectedError := "failed to push artifacts: bundling failed: templates directory not found: contexts"
		if err.Error()[:len(expectedError)] != expectedError {
			t.Errorf("Expected error to start with %q, got %q", expectedError, err.Error())
		}
		// Verify the path separator is correct for the platform
		if !strings.Contains(err.Error(), "contexts") {
			t.Errorf("Expected error to contain 'contexts', got %q", err.Error())
		}
	})

	t.Run("PipelineExecutionError", func(t *testing.T) {
		// Set up temporary directory
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		// Create injector and mock artifact pipeline that fails
		injector := di.NewInjector()
		mockArtifactPipeline := pipelines.NewMockBasePipeline()
		mockArtifactPipeline.ExecuteFunc = func(ctx context.Context) error {
			return fmt.Errorf("pipeline execution failed")
		}

		// Register the mock pipeline
		injector.Register("artifactPipeline", mockArtifactPipeline)

		// Create test command
		cmd := &cobra.Command{
			Use:   "push",
			Short: "Push blueprints to an OCI registry",
			RunE:  pushCmd.RunE,
		}

		// Set up context
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		// Set arguments
		cmd.SetArgs([]string{"registry.example.com/repo:v1.0.0"})

		// Execute command
		err := cmd.Execute()

		// Verify error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		expectedError := "failed to push artifacts: pipeline execution failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})
}
