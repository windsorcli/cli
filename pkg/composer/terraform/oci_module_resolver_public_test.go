package terraform

import (
	"archive/tar"
	"errors"
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
		mockArtifactBuilder := artifact.NewMockArtifact()
		resolver := NewOCIModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)
		resolver.BaseModuleResolver.shims = mocks.Shims
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
		mockArtifactBuilder.PullFunc = func(refs []string) (map[string][]byte, error) {
			artifacts := make(map[string][]byte)
			for _, ref := range refs {
				// Cache key format is registry/repository:tag (without oci:// prefix)
				if strings.HasPrefix(ref, "oci://") {
					cacheKey := strings.TrimPrefix(ref, "oci://")
					artifacts[cacheKey] = []byte("mock artifact data")
				} else {
					artifacts[ref] = []byte("mock artifact data")
				}
			}
			return artifacts, nil
		}
		resolver.artifactBuilder = mockArtifactBuilder

		resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"
		extractionDir := filepath.Join("/test/project", ".windsor", ".oci_extracted", "registry.example.com_module_latest")

		// Mock tar reader for successful extraction
		callCount := 0
		mockTarReader := &MockTarReader{
			NextFunc: func() (*tar.Header, error) {
				callCount++
				switch callCount {
				case 1:
					return &tar.Header{
						Name:     "terraform/test-module/",
						Typeflag: tar.TypeDir,
					}, nil
				case 2:
					return &tar.Header{
						Name:     "terraform/test-module/main.tf",
						Typeflag: tar.TypeReg,
						Mode:     0644,
					}, nil
				default:
					return nil, io.EOF
				}
			},
			ReadFunc: func(p []byte) (int, error) {
				return 0, io.EOF
			},
		}
		resolver.BaseModuleResolver.shims.NewTarReader = func(r io.Reader) TarReader {
			return mockTarReader
		}

		// Mock Stat and Rename with shared state
		extractionComplete := false
		tmpExtractionDir := extractionDir + ".tmp"

		// Mock Rename to succeed and mark extraction as complete
		resolver.BaseModuleResolver.shims.Rename = func(oldpath, newpath string) error {
			if oldpath == tmpExtractionDir && newpath == extractionDir {
				extractionComplete = true
			}
			return nil
		}

		// Mock Stat to check extraction directory and module path
		originalStat := resolver.BaseModuleResolver.shims.Stat
		resolver.BaseModuleResolver.shims.Stat = func(name string) (os.FileInfo, error) {
			// Before extraction: module path doesn't exist
			if strings.Contains(name, "terraform/test-module") && !extractionComplete {
				return nil, os.ErrNotExist
			}
			// After extraction: module path exists
			if strings.Contains(name, "terraform/test-module") && extractionComplete {
				return nil, nil
			}
			// Tmp extraction dir never exists (we create it fresh)
			if name == tmpExtractionDir {
				return nil, os.ErrNotExist
			}
			// Extraction dir doesn't exist before rename
			if name == extractionDir && !extractionComplete {
				return nil, os.ErrNotExist
			}
			// After rename, extraction dir exists
			if name == extractionDir && extractionComplete {
				return nil, nil
			}
			return originalStat(name)
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
