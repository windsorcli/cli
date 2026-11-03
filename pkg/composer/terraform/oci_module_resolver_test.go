package terraform_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/composer/terraform"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestOCIModuleResolver_NewOCIModuleResolver(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given dependencies
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()

		// When creating a new OCI module resolver
		resolver := terraform.NewOCIModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)

		// Then it should be created successfully
		if resolver == nil {
			t.Fatal("Expected non-nil OCIModuleResolver")
		}
		if resolver.BaseModuleResolver == nil {
			t.Error("Expected BaseModuleResolver to be set")
		}
		if resolver.ArtifactBuilder() == nil {
			t.Error("Expected artifactBuilder to be set")
		}
	})
}

func TestOCIModuleResolver_NewOCIModuleResolverWithDependencies(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a resolver with all required dependencies
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		resolver := terraform.NewOCIModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)
		resolver.BaseModuleResolver.SetShims(mocks.Shims)

		// Then dependencies should be set
		if resolver.BaseModuleResolver.Runtime().Shell == nil {
			t.Error("Expected shell to be set")
		}
		if resolver.ArtifactBuilder() == nil {
			t.Error("Expected artifactBuilder to be set")
		}
		if resolver.BaseModuleResolver.BlueprintHandler() == nil {
			t.Error("Expected blueprintHandler to be set")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestOCIModuleResolver_ProcessModules(t *testing.T) {
	setup := func(t *testing.T) (*terraform.OCIModuleResolver, *TerraformTestMocks) {
		t.Helper()
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		resolver := terraform.NewOCIModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)
		resolver.BaseModuleResolver.SetShims(mocks.Shims)
		return resolver, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a resolver with OCI components
		resolver, mocks := setup(t)
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "test-module",
					Source:   "oci://registry.example.com/module:latest//terraform/test-module",
					FullPath: "/mock/project/terraform/test-module",
				},
			}
		}

		// Set up artifact builder to return mock data with correct cache key
		mockArtifactBuilder := artifact.NewMockArtifact()
		mockArtifactBuilder.ParseOCIRefFunc = func(ociRef string) (registry, repository, tag string, err error) {
			if ociRef == "oci://registry.example.com/module:latest" {
				return "registry.example.com", "module", "latest", nil
			}
			return "", "", "", fmt.Errorf("invalid OCI reference format: %s", ociRef)
		}
		mockArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			artifacts := make(map[string]string)
			for _, ref := range refs {
				// Cache key format is registry/repository:tag (without oci:// prefix)
				if strings.HasPrefix(ref, "oci://") {
					cacheKey := strings.TrimPrefix(ref, "oci://")
					extractionKey := strings.ReplaceAll(strings.ReplaceAll(cacheKey, "/", "_"), ":", "_")
					cacheDir := filepath.Join("/test/project", ".windsor", "cache", "oci", extractionKey)
					artifacts[cacheKey] = cacheDir
				} else {
					artifacts[ref] = filepath.Join("/test/project", ".windsor", "cache", "oci", ref)
				}
			}
			return artifacts, nil
		}
		resolver.SetArtifactBuilder(mockArtifactBuilder)

		resolver.BaseModuleResolver.Runtime().ProjectRoot = "/test/project"

		// When processing modules
		err := resolver.ProcessModules()

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("HandlesNoOCIComponents", func(t *testing.T) {
		// Given a resolver with no OCI components
		resolver, mocks := setup(t)
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "test-module",
					Source:   "git::https://github.com/test/module.git",
					FullPath: "/mock/project/terraform/test-module",
				},
			}
		}

		// When processing modules
		err := resolver.ProcessModules()

		// Then it should succeed without processing
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("HandlesErrors", func(t *testing.T) {
		// Given a resolver with artifact pull error
		resolver, mocks := setup(t)
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "test-module",
					Source:   "oci://registry.example.com/module:latest//terraform/test-module",
					FullPath: "/mock/project/terraform/test-module",
				},
			}
		}

		// Mock artifact builder to return error
		mockArtifactBuilder := artifact.NewMockArtifact()
		mockArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			return nil, errors.New("artifact pull error")
		}
		resolver.SetArtifactBuilder(mockArtifactBuilder)

		// When processing modules
		err := resolver.ProcessModules()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to preload OCI artifacts") {
			t.Errorf("Expected artifact pull error, got: %v", err)
		}
	})

	t.Run("HandlesMalformedOCIURLs", func(t *testing.T) {
		// Given a resolver with malformed OCI URLs
		resolver, mocks := setup(t)
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "test-module",
					Source:   "oci://registry.example.com/module:latest", // Missing path separator
					FullPath: "/mock/project/terraform/test-module",
				},
			}
		}

		// When processing modules
		err := resolver.ProcessModules()

		// Then it should succeed (malformed URLs are skipped during URL extraction)
		if err != nil {
			t.Errorf("Expected nil error for malformed URL, got %v", err)
		}
	})

	t.Run("HandlesComponentProcessingErrors", func(t *testing.T) {
		// Given a resolver with component that fails during processing
		resolver, mocks := setup(t)
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "test-module",
					Source:   "oci://registry.example.com/module:latest//terraform/test-module",
					FullPath: "/mock/project/terraform/test-module",
				},
			}
		}

		// Mock MkdirAll to fail for component processing
		resolver.BaseModuleResolver.Shims().MkdirAll = func(path string, perm os.FileMode) error {
			return errors.New("mkdir error")
		}

		// When processing modules
		err := resolver.ProcessModules()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to process component") {
			t.Errorf("Expected component processing error, got: %v", err)
		}
	})
}
