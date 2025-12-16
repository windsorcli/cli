package terraform

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestOCIModuleResolver_NewOCIModuleResolver(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given dependencies
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()

		// When creating a new OCI module resolver
		resolver := NewOCIModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)

		// Then it should be created successfully
		if resolver == nil {
			t.Fatal("Expected non-nil OCIModuleResolver")
		}
		if resolver.BaseModuleResolver == nil {
			t.Error("Expected BaseModuleResolver to be set")
		}
		if resolver.artifactBuilder == nil {
			t.Error("Expected artifactBuilder to be set")
		}
	})
}

func TestOCIModuleResolver_NewOCIModuleResolverWithDependencies(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a resolver with all required dependencies
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		resolver := NewOCIModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)
		resolver.BaseModuleResolver.shims = mocks.Shims

		// Then dependencies should be set
		if resolver.BaseModuleResolver.runtime.Shell == nil {
			t.Error("Expected shell to be set")
		}
		if resolver.artifactBuilder == nil {
			t.Error("Expected artifactBuilder to be set")
		}
		if resolver.BaseModuleResolver.blueprintHandler == nil {
			t.Error("Expected blueprintHandler to be set")
		}
	})
}

func TestOCIModuleResolver_shouldHandle(t *testing.T) {
	t.Run("HandlesOCIAndRejectsNonOCI", func(t *testing.T) {
		// Given a resolver
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		resolver := NewOCIModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)
		resolver.BaseModuleResolver.shims = mocks.Shims

		// When checking various source types
		testCases := []struct {
			source   string
			expected bool
		}{
			{"oci://registry.example.com/module:latest", true},
			{"oci://ghcr.io/windsorcli/terraform-modules:v1.0.0", true},
			{"git::https://github.com/terraform-aws-modules/terraform-aws-vpc.git", false},
			{"./local/module", false},
			{"", false},
		}

		for _, tc := range testCases {
			// Then it should handle OCI sources and reject non-OCI sources
			result := resolver.shouldHandle(tc.source)
			if result != tc.expected {
				t.Errorf("Expected %s to return %v, got %v", tc.source, tc.expected, result)
			}
		}
	})
}

func TestOCIModuleResolver_ProcessModules(t *testing.T) {
	setup := func(t *testing.T) (*OCIModuleResolver, *TerraformTestMocks) {
		t.Helper()
		mocks := setupTerraformMocks(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir

		mockArtifactBuilder := artifact.NewMockArtifact()
		mockArtifactBuilder.ParseOCIRefFunc = func(ociRef string) (registry, repository, tag string, err error) {
			if ociRef == "oci://registry.example.com/module:latest" {
				return "registry.example.com", "module", "latest", nil
			}
			return "", "", "", fmt.Errorf("unexpected OCI ref: %s", ociRef)
		}
		mockArtifactBuilder.GetCacheDirFunc = func(registry, repository, tag string) (string, error) {
			cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)
			extractionKey := strings.ReplaceAll(strings.ReplaceAll(cacheKey, "/", "_"), ":", "_")
			return filepath.Join(tmpDir, ".windsor", ".oci_extracted", extractionKey), nil
		}
		mockArtifactBuilder.PullFunc = func(refs []string) (map[string][]byte, error) {
			artifacts := make(map[string][]byte)
			for _, ref := range refs {
				if strings.HasPrefix(ref, "oci://") {
					pathSeparatorIdx := strings.Index(ref[6:], "//")
					var baseURL string
					if pathSeparatorIdx != -1 {
						baseURL = ref[:6+pathSeparatorIdx]
					} else {
						baseURL = ref
					}
					registry, repository, tag, err := mockArtifactBuilder.ParseOCIRef(baseURL)
					if err == nil {
						cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)
						var buf bytes.Buffer
						tarWriter := tar.NewWriter(&buf)
						content := []byte("resource \"test\" {}")
						header := &tar.Header{
							Name: "terraform/test-module/main.tf",
							Mode: 0644,
							Size: int64(len(content)),
						}
						tarWriter.WriteHeader(header)
						tarWriter.Write(content)
						tarWriter.Close()
						artifacts[cacheKey] = buf.Bytes()

						cacheDir, _ := mockArtifactBuilder.GetCacheDir(registry, repository, tag)
						modulePath := filepath.Join(cacheDir, "terraform/test-module")
						os.MkdirAll(modulePath, 0755)
						os.WriteFile(filepath.Join(modulePath, "main.tf"), content, 0644)
					}
				}
			}
			return artifacts, nil
		}

		resolver := NewOCIModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)
		resolver.BaseModuleResolver.shims = mocks.Shims
		resolver.BaseModuleResolver.runtime.ProjectRoot = tmpDir
		resolver.BaseModuleResolver.shims.Copy = io.Copy
		resolver.BaseModuleResolver.shims.Create = os.Create
		resolver.BaseModuleResolver.shims.MkdirAll = os.MkdirAll
		resolver.BaseModuleResolver.shims.Stat = os.Stat
		resolver.BaseModuleResolver.shims.Rename = os.Rename

		return resolver, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a resolver with OCI components
		resolver, mocks := setup(t)
		tmpDir := mocks.Runtime.ProjectRoot

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "test-module",
					Source:   "oci://registry.example.com/module:latest//terraform/test-module",
					FullPath: filepath.Join(tmpDir, "terraform", "test-module"),
				},
			}
		}

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
		mockArtifactBuilder.PullFunc = func(refs []string) (map[string][]byte, error) {
			return nil, errors.New("artifact pull error")
		}
		resolver.artifactBuilder = mockArtifactBuilder

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
		resolver.BaseModuleResolver.shims.MkdirAll = func(path string, perm os.FileMode) error {
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
