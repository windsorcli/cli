package pipelines

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupArtifactPipelineMocks(t *testing.T) (*ArtifactPipeline, *Mocks) {
	t.Helper()
	mocks := setupMocks(t)

	// Create mock artifact builder
	mockArtifactBuilder := artifact.NewMockArtifact()
	mockArtifactBuilder.InitializeFunc = func(injector di.Injector) error { return nil }
	mockArtifactBuilder.AddFileFunc = func(path string, content []byte, mode os.FileMode) error { return nil }
	mockArtifactBuilder.CreateFunc = func(outputPath string, tag string) (string, error) {
		if tag != "" {
			return fmt.Sprintf("test-%s.tar.gz", tag), nil
		}
		return "blueprint-v1.0.0.tar.gz", nil
	}
	mockArtifactBuilder.PushFunc = func(registryBase string, repoName string, tag string) error {
		return nil
	}

	// Create mock bundlers
	mockTemplateBundler := artifact.NewMockBundler()
	mockTemplateBundler.InitializeFunc = func(injector di.Injector) error { return nil }
	mockTemplateBundler.BundleFunc = func(art artifact.Artifact) error { return nil }

	mockKustomizeBundler := artifact.NewMockBundler()
	mockKustomizeBundler.InitializeFunc = func(injector di.Injector) error { return nil }
	mockKustomizeBundler.BundleFunc = func(art artifact.Artifact) error { return nil }

	mockTerraformBundler := artifact.NewMockBundler()
	mockTerraformBundler.InitializeFunc = func(injector di.Injector) error { return nil }
	mockTerraformBundler.BundleFunc = func(art artifact.Artifact) error { return nil }

	// Register components in injector
	mocks.Injector.Register("artifactBuilder", mockArtifactBuilder)
	mocks.Injector.Register("templateBundler", mockTemplateBundler)
	mocks.Injector.Register("kustomizeBundler", mockKustomizeBundler)
	mocks.Injector.Register("terraformBundler", mockTerraformBundler)

	// Create and initialize pipeline
	pipeline := NewArtifactPipeline()
	if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
		t.Fatalf("Failed to initialize pipeline: %v", err)
	}

	return pipeline, mocks
}

// =============================================================================
// Tests
// =============================================================================

func TestArtifactPipeline_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a properly configured environment
		mocks := setupMocks(t)

		// Create mock artifact builder and bundlers
		mockArtifactBuilder := artifact.NewMockArtifact()
		mockArtifactBuilder.InitializeFunc = func(injector di.Injector) error { return nil }

		mockTemplateBundler := artifact.NewMockBundler()
		mockTemplateBundler.InitializeFunc = func(injector di.Injector) error { return nil }

		mockKustomizeBundler := artifact.NewMockBundler()
		mockKustomizeBundler.InitializeFunc = func(injector di.Injector) error { return nil }

		mockTerraformBundler := artifact.NewMockBundler()
		mockTerraformBundler.InitializeFunc = func(injector di.Injector) error { return nil }

		// Register components
		mocks.Injector.Register("artifactBuilder", mockArtifactBuilder)
		mocks.Injector.Register("templateBundler", mockTemplateBundler)
		mocks.Injector.Register("kustomizeBundler", mockKustomizeBundler)
		mocks.Injector.Register("terraformBundler", mockTerraformBundler)

		// When initializing the artifact pipeline
		pipeline := NewArtifactPipeline()
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the pipeline should be properly initialized
		if pipeline.artifactBuilder == nil {
			t.Error("Expected artifact builder to be initialized")
		}
		if len(pipeline.bundlers) == 0 {
			t.Error("Expected bundlers to be initialized")
		}
	})

	t.Run("ErrorInitializingArtifactBuilder", func(t *testing.T) {
		// Given a mock artifact builder that fails to initialize
		mocks := setupMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		mockArtifactBuilder.InitializeFunc = func(injector di.Injector) error {
			return fmt.Errorf("artifact builder initialization failed")
		}

		mockTemplateBundler := artifact.NewMockBundler()
		mockTemplateBundler.InitializeFunc = func(injector di.Injector) error { return nil }

		// Register components
		mocks.Injector.Register("artifactBuilder", mockArtifactBuilder)
		mocks.Injector.Register("templateBundler", mockTemplateBundler)

		// When initializing the artifact pipeline
		pipeline := NewArtifactPipeline()
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to initialize artifact builder") {
			t.Errorf("Expected error about artifact builder initialization, got: %v", err)
		}
	})

	t.Run("ErrorInitializingBundler", func(t *testing.T) {
		// Given a mock bundler that fails to initialize
		mocks := setupMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		mockArtifactBuilder.InitializeFunc = func(injector di.Injector) error { return nil }

		mockTemplateBundler := artifact.NewMockBundler()
		mockTemplateBundler.InitializeFunc = func(injector di.Injector) error {
			return fmt.Errorf("bundler initialization failed")
		}

		// Register components
		mocks.Injector.Register("artifactBuilder", mockArtifactBuilder)
		mocks.Injector.Register("templateBundler", mockTemplateBundler)

		// When initializing the artifact pipeline
		pipeline := NewArtifactPipeline()
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to initialize bundler") {
			t.Errorf("Expected error about bundler initialization, got: %v", err)
		}
	})
}

func TestArtifactPipeline_Execute_BundleMode(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given an artifact pipeline with mocks
		pipeline, mocks := setupArtifactPipelineMocks(t)

		// And bundle mode context
		ctx := context.WithValue(context.Background(), "artifactMode", "bundle")
		ctx = context.WithValue(ctx, "outputPath", "/tmp/test.tar.gz")
		ctx = context.WithValue(ctx, "tag", "v1.0.0")

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the artifact builder should have been called
		mockArtifactBuilder := mocks.Injector.Resolve("artifactBuilder").(*artifact.MockArtifact)
		if mockArtifactBuilder.CreateFunc == nil {
			t.Error("Expected artifact builder Create method to be called")
		}
	})

	t.Run("ErrorNoArtifactMode", func(t *testing.T) {
		// Given an artifact pipeline with mocks
		pipeline, _ := setupArtifactPipelineMocks(t)

		// And context without artifact mode
		ctx := context.Background()

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "artifact mode not specified in context") {
			t.Errorf("Expected error about missing artifact mode, got: %v", err)
		}
	})

	t.Run("ErrorUnknownMode", func(t *testing.T) {
		// Given an artifact pipeline with mocks
		pipeline, _ := setupArtifactPipelineMocks(t)

		// And context with unknown mode
		ctx := context.WithValue(context.Background(), "artifactMode", "unknown")

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "unknown artifact mode: unknown") {
			t.Errorf("Expected error about unknown mode, got: %v", err)
		}
	})

	t.Run("ErrorNoOutputPath", func(t *testing.T) {
		// Given an artifact pipeline with mocks
		pipeline, _ := setupArtifactPipelineMocks(t)

		// And bundle mode context without output path
		ctx := context.WithValue(context.Background(), "artifactMode", "bundle")
		ctx = context.WithValue(ctx, "tag", "v1.0.0")

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "output path not specified in context for bundle mode") {
			t.Errorf("Expected error about missing output path, got: %v", err)
		}
	})

	t.Run("ErrorArtifactCreationFails", func(t *testing.T) {
		// Given an artifact pipeline with failing artifact creation
		pipeline, mocks := setupArtifactPipelineMocks(t)
		mockArtifactBuilder := mocks.Injector.Resolve("artifactBuilder").(*artifact.MockArtifact)
		mockArtifactBuilder.CreateFunc = func(outputPath string, tag string) (string, error) {
			return "", fmt.Errorf("artifact creation failed")
		}

		// And bundle mode context
		ctx := context.WithValue(context.Background(), "artifactMode", "bundle")
		ctx = context.WithValue(ctx, "outputPath", "/tmp/test.tar.gz")
		ctx = context.WithValue(ctx, "tag", "v1.0.0")

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create artifact") {
			t.Errorf("Expected error about artifact creation failure, got: %v", err)
		}
	})

	t.Run("ErrorBundlingFails", func(t *testing.T) {
		// Given an artifact pipeline with failing bundler
		pipeline, mocks := setupArtifactPipelineMocks(t)
		mockTemplateBundler := mocks.Injector.Resolve("templateBundler").(*artifact.MockBundler)
		mockTemplateBundler.BundleFunc = func(art artifact.Artifact) error {
			return fmt.Errorf("bundling failed")
		}

		// And bundle mode context
		ctx := context.WithValue(context.Background(), "artifactMode", "bundle")
		ctx = context.WithValue(ctx, "outputPath", "/tmp/test.tar.gz")
		ctx = context.WithValue(ctx, "tag", "v1.0.0")

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "bundling failed") {
			t.Errorf("Expected error about bundling failure, got: %v", err)
		}
	})
}

func TestArtifactPipeline_Execute_PushMode(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given an artifact pipeline with mocks
		pipeline, mocks := setupArtifactPipelineMocks(t)

		// And push mode context
		ctx := context.WithValue(context.Background(), "artifactMode", "push")
		ctx = context.WithValue(ctx, "registryBase", "ghcr.io/test")
		ctx = context.WithValue(ctx, "repoName", "myblueprint")
		ctx = context.WithValue(ctx, "tag", "v1.0.0")

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the artifact builder should have been called
		mockArtifactBuilder := mocks.Injector.Resolve("artifactBuilder").(*artifact.MockArtifact)
		if mockArtifactBuilder.PushFunc == nil {
			t.Error("Expected artifact builder Push method to be called")
		}
	})

	t.Run("ErrorNoRegistryBase", func(t *testing.T) {
		// Given an artifact pipeline with mocks
		pipeline, _ := setupArtifactPipelineMocks(t)

		// And push mode context without registry base
		ctx := context.WithValue(context.Background(), "artifactMode", "push")
		ctx = context.WithValue(ctx, "repoName", "myblueprint")
		ctx = context.WithValue(ctx, "tag", "v1.0.0")

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "registry base not specified in context for push mode") {
			t.Errorf("Expected error about missing registry base, got: %v", err)
		}
	})

	t.Run("ErrorNoRepoName", func(t *testing.T) {
		// Given an artifact pipeline with mocks
		pipeline, _ := setupArtifactPipelineMocks(t)

		// And push mode context without repo name
		ctx := context.WithValue(context.Background(), "artifactMode", "push")
		ctx = context.WithValue(ctx, "registryBase", "ghcr.io/test")
		ctx = context.WithValue(ctx, "tag", "v1.0.0")

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "repository name not specified in context for push mode") {
			t.Errorf("Expected error about missing repository name, got: %v", err)
		}
	})

	t.Run("ErrorPushFails", func(t *testing.T) {
		// Given an artifact pipeline with failing push
		pipeline, mocks := setupArtifactPipelineMocks(t)
		mockArtifactBuilder := mocks.Injector.Resolve("artifactBuilder").(*artifact.MockArtifact)
		mockArtifactBuilder.PushFunc = func(registryBase string, repoName string, tag string) error {
			return fmt.Errorf("push failed")
		}

		// And push mode context
		ctx := context.WithValue(context.Background(), "artifactMode", "push")
		ctx = context.WithValue(ctx, "registryBase", "ghcr.io/test")
		ctx = context.WithValue(ctx, "repoName", "myblueprint")
		ctx = context.WithValue(ctx, "tag", "v1.0.0")

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to push artifact") {
			t.Errorf("Expected error about push failure, got: %v", err)
		}
	})
}

func TestArtifactPipeline_Execute_NoArtifactBuilder(t *testing.T) {
	t.Run("ErrorNoArtifactBuilder", func(t *testing.T) {
		// Given an artifact pipeline without artifact builder
		pipeline := NewArtifactPipeline()
		// Don't register artifact builder

		// And bundle mode context
		ctx := context.WithValue(context.Background(), "artifactMode", "bundle")
		ctx = context.WithValue(ctx, "outputPath", "/tmp/test.tar.gz")

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "artifact builder not available") {
			t.Errorf("Expected error about missing artifact builder, got: %v", err)
		}
	})
}

func TestArtifactPipeline_VerifyAllBundlersCalled(t *testing.T) {
	t.Run("AllBundlersExecuted", func(t *testing.T) {
		// Given an artifact pipeline with mocks
		pipeline, mocks := setupArtifactPipelineMocks(t)

		// Track which bundlers were called
		templateBundlerCalled := false
		kustomizeBundlerCalled := false
		terraformBundlerCalled := false

		mockTemplateBundler := mocks.Injector.Resolve("templateBundler").(*artifact.MockBundler)
		mockTemplateBundler.BundleFunc = func(art artifact.Artifact) error {
			templateBundlerCalled = true
			return nil
		}

		mockKustomizeBundler := mocks.Injector.Resolve("kustomizeBundler").(*artifact.MockBundler)
		mockKustomizeBundler.BundleFunc = func(art artifact.Artifact) error {
			kustomizeBundlerCalled = true
			return nil
		}

		mockTerraformBundler := mocks.Injector.Resolve("terraformBundler").(*artifact.MockBundler)
		mockTerraformBundler.BundleFunc = func(art artifact.Artifact) error {
			terraformBundlerCalled = true
			return nil
		}

		// And bundle mode context
		ctx := context.WithValue(context.Background(), "artifactMode", "bundle")
		ctx = context.WithValue(ctx, "outputPath", "/tmp/test.tar.gz")

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And all bundlers should be called
		if !templateBundlerCalled {
			t.Error("Expected template bundler to be called")
		}
		if !kustomizeBundlerCalled {
			t.Error("Expected kustomize bundler to be called")
		}
		if !terraformBundlerCalled {
			t.Error("Expected terraform bundler to be called")
		}
	})
}
