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
// Test Setup
// =============================================================================

type PushMocks struct {
	Injector di.Injector
}

// setupPushTest sets up the test environment for push command tests.
// It creates a temporary directory, initializes the injector, and returns PushMocks.
func setupPushTest(t *testing.T) *PushMocks {
	t.Helper()

	tmpDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	t.Cleanup(func() { os.Chdir(oldDir) })

	injector := di.NewInjector()
	return &PushMocks{
		Injector: injector,
	}
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

func TestPushCmdWithPipeline(t *testing.T) {
	t.Run("SuccessWithPipeline", func(t *testing.T) {
		mocks := setupPushTest(t)
		mockArtifactPipeline := pipelines.NewMockBasePipeline()
		mockArtifactPipeline.ExecuteFunc = func(ctx context.Context) error {
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
		mocks.Injector.Register("artifactPipeline", mockArtifactPipeline)
		cmd := createTestPushCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"registry.example.com/repo:v1.0.0"})
		err := cmd.Execute()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithoutTag", func(t *testing.T) {
		mocks := setupPushTest(t)
		mockArtifactPipeline := pipelines.NewMockBasePipeline()
		mockArtifactPipeline.ExecuteFunc = func(ctx context.Context) error {
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
		mocks.Injector.Register("artifactPipeline", mockArtifactPipeline)
		cmd := createTestPushCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"registry.example.com/repo"})
		err := cmd.Execute()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithOciUrl", func(t *testing.T) {
		mocks := setupPushTest(t)
		mockArtifactPipeline := pipelines.NewMockBasePipeline()
		mockArtifactPipeline.ExecuteFunc = func(ctx context.Context) error {
			mode, ok := ctx.Value("artifactMode").(string)
			if !ok || mode != "push" {
				return fmt.Errorf("expected artifactMode 'push', got %v", mode)
			}
			registryBase, ok := ctx.Value("registryBase").(string)
			if !ok || registryBase != "ghcr.io" {
				return fmt.Errorf("expected registryBase 'ghcr.io', got %v", registryBase)
			}
			repoName, ok := ctx.Value("repoName").(string)
			if !ok || repoName != "windsorcli/core" {
				return fmt.Errorf("expected repoName 'windsorcli/core', got %v", repoName)
			}
			tag, ok := ctx.Value("tag").(string)
			if !ok || tag != "v0.0.0" {
				return fmt.Errorf("expected tag 'v0.0.0', got %v", tag)
			}
			return nil
		}
		mocks.Injector.Register("artifactPipeline", mockArtifactPipeline)
		cmd := createTestPushCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"oci://ghcr.io/windsorcli/core:v0.0.0"})
		err := cmd.Execute()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorMissingRegistry", func(t *testing.T) {
		mocks := setupPushTest(t)
		cmd := createTestPushCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "registry is required") {
			t.Errorf("Expected registry required error, got %v", err)
		}
	})

	t.Run("ErrorInvalidRegistryFormat", func(t *testing.T) {
		mocks := setupPushTest(t)
		cmd := createTestPushCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"invalidformat"})
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "invalid registry format") {
			t.Errorf("Expected invalid registry format error, got %v", err)
		}
	})

	t.Run("PipelineSetupError", func(t *testing.T) {
		mocks := setupPushTest(t)
		cmd := createTestPushCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"registry.example.com/repo:v1.0.0"})
		// Do not register mock pipeline to force real pipeline setup (which will fail)
		err := cmd.Execute()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		expectedError := "failed to push artifacts: bundling failed: templates directory not found: contexts"
		if err != nil && !strings.HasPrefix(err.Error(), expectedError) {
			t.Errorf("Expected error to start with %q, got %q", expectedError, err.Error())
		}
	})
}
