package terraform

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestOCIModuleResolver_parseOCIRef(t *testing.T) {
	t.Run("ParsesValidReferences", func(t *testing.T) {
		// Given a resolver
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		resolver := NewOCIModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)
		resolver.BaseModuleResolver.shims = mocks.Shims

		// When parsing various valid OCI references
		testCases := []struct {
			ociRef     string
			registry   string
			repository string
			tag        string
		}{
			{"oci://registry.example.com/module:latest", "registry.example.com", "module", "latest"},
			{"oci://ghcr.io/windsorcli/terraform-modules:v1.0.0", "ghcr.io", "windsorcli/terraform-modules", "v1.0.0"},
		}

		for _, tc := range testCases {
			// Then it should parse correctly
			registry, repository, tag, err := resolver.parseOCIRef(tc.ociRef)
			if err != nil {
				t.Errorf("Expected nil error for %s, got %v", tc.ociRef, err)
			}
			if registry != tc.registry {
				t.Errorf("Expected registry '%s', got '%s'", tc.registry, registry)
			}
			if repository != tc.repository {
				t.Errorf("Expected repository '%s', got '%s'", tc.repository, repository)
			}
			if tag != tc.tag {
				t.Errorf("Expected tag '%s', got '%s'", tc.tag, tag)
			}
		}
	})

	t.Run("HandlesInvalidReferences", func(t *testing.T) {
		// Given a resolver
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		resolver := NewOCIModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)
		resolver.BaseModuleResolver.shims = mocks.Shims

		// When parsing invalid OCI references
		testCases := []string{
			"invalid://reference",
			"oci://invalid-format",
			"oci://registry/module",
		}

		for _, ociRef := range testCases {
			// Then it should return an error
			_, _, _, err := resolver.parseOCIRef(ociRef)
			if err == nil {
				t.Errorf("Expected error for invalid reference %s, got nil", ociRef)
			}
		}
	})

	t.Run("HandlesEdgeCases", func(t *testing.T) {
		// Given a resolver
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		resolver := NewOCIModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)
		resolver.BaseModuleResolver.shims = mocks.Shims

		// When parsing edge case OCI references
		errorCases := []struct {
			name      string
			ociRef    string
			errorText string
		}{
			{"EmptyString", "", "invalid OCI reference format"},
			{"NonOCIPrefix", "https://registry.example.com/module:latest", "invalid OCI reference format"},
			{"OCIOnlyPrefix", "oci://", "invalid OCI reference format"},
			{"MissingTag", "oci://registry.example.com/module", "expected registry/repository:tag"},
			{"MissingRepository", "oci://registry.example.com:", "expected registry/repository:tag"},
			{"MultipleColons", "oci://registry.example.com/module:v1.0.0:extra", "expected registry/repository:tag"},
			{"NoSlash", "oci://registry.example.com-module:latest", "expected registry/repository:tag"},
			{"OnlySlash", "oci:///", "expected registry/repository:tag"},
			{"SlashWithoutRepo", "oci://registry.example.com/", "expected registry/repository:tag"},
		}

		for _, tc := range errorCases {
			// Then it should return appropriate errors
			_, _, _, err := resolver.parseOCIRef(tc.ociRef)
			if err == nil {
				t.Errorf("Expected error for %s (%s), got nil", tc.name, tc.ociRef)
			} else if !strings.Contains(err.Error(), tc.errorText) {
				t.Errorf("Expected error containing '%s' for %s (%s), got: %v", tc.errorText, tc.name, tc.ociRef, err)
			}
		}
	})

	t.Run("HandlesComplexValidReferences", func(t *testing.T) {
		// Given a resolver
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		resolver := NewOCIModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)
		resolver.BaseModuleResolver.shims = mocks.Shims

		// When parsing complex but valid OCI references
		testCases := []struct {
			ociRef     string
			registry   string
			repository string
			tag        string
		}{
			{"oci://ghcr.io/owner/repo-name:sha256-abcdef123456", "ghcr.io", "owner/repo-name", "sha256-abcdef123456"},
			{"oci://registry.io/namespace/module-name:2023.01.01", "registry.io", "namespace/module-name", "2023.01.01"},
			{"oci://docker.io/library/nginx:latest", "docker.io", "library/nginx", "latest"},
			{"oci://quay.io/organization/project:v1.0.0", "quay.io", "organization/project", "v1.0.0"},
		}

		for _, tc := range testCases {
			// Then it should parse correctly
			registry, repository, tag, err := resolver.parseOCIRef(tc.ociRef)
			if err != nil {
				t.Errorf("Expected nil error for %s, got %v", tc.ociRef, err)
			}
			if registry != tc.registry {
				t.Errorf("Expected registry '%s', got '%s' for %s", tc.registry, registry, tc.ociRef)
			}
			if repository != tc.repository {
				t.Errorf("Expected repository '%s', got '%s' for %s", tc.repository, repository, tc.ociRef)
			}
			if tag != tc.tag {
				t.Errorf("Expected tag '%s', got '%s' for %s", tc.tag, tag, tc.ociRef)
			}
		}
	})
}

func TestOCIModuleResolver_extractOCIModule(t *testing.T) {
	setup := func(t *testing.T) *OCIModuleResolver {
		t.Helper()
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		resolver := NewOCIModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)
		resolver.BaseModuleResolver.shims = mocks.Shims
		return resolver
	}

	t.Run("Success", func(t *testing.T) {
		// Given a resolver with valid OCI source and cached artifact
		resolver := setup(t)
		resolvedSource := "oci://registry.example.com/module:latest//terraform/test-module"
		componentPath := "test-module"
		ociArtifacts := map[string][]byte{
			"registry.example.com/module:latest": []byte("mock artifact data"),
		}

		resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"
		extractionDir := filepath.Join("/test/project", ".windsor", ".oci_extracted", "registry.example.com-module-latest")
		fullModulePath := filepath.Join(extractionDir, "terraform/test-module")

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

		// Mock Stat calls
		statCallCount := 0
		originalStat := resolver.BaseModuleResolver.shims.Stat
		resolver.BaseModuleResolver.shims.Stat = func(name string) (os.FileInfo, error) {
			statCallCount++
			// Before extraction: module path doesn't exist
			if name == fullModulePath && !extractionComplete {
				return nil, os.ErrNotExist
			}
			// After extraction: module path exists
			if name == fullModulePath && extractionComplete {
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

		// When extracting OCI module
		path, err := resolver.extractOCIModule(resolvedSource, componentPath, ociArtifacts)

		// Then it should return the extracted path
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
		if path == "" {
			t.Error("Expected non-empty path")
		}
	})

	t.Run("HandlesCacheHit", func(t *testing.T) {
		// Given a resolver with existing extracted module
		resolver := setup(t)
		resolvedSource := "oci://registry.example.com/module:latest//terraform/test-module"
		componentPath := "test-module"
		ociArtifacts := map[string][]byte{
			"registry.example.com/module:latest": []byte("mock artifact data"),
		}

		// Mock Stat to return success (cache hit)
		resolver.BaseModuleResolver.shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, nil
		}
		resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"

		// When extracting OCI module
		path, err := resolver.extractOCIModule(resolvedSource, componentPath, ociArtifacts)

		// Then it should return the cached path without extraction
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
		if path == "" {
			t.Error("Expected non-empty path")
		}
	})

	t.Run("HandlesExistingExtractionDirectory", func(t *testing.T) {
		// Given a resolver with existing extraction directory (from previous failed extraction)
		resolver := setup(t)
		resolvedSource := "oci://registry.example.com/module:latest//terraform/test-module"
		componentPath := "test-module"
		ociArtifacts := map[string][]byte{
			"registry.example.com/module:latest": []byte("mock artifact data"),
		}

		resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"
		extractionDir := filepath.Join("/test/project", ".windsor", ".oci_extracted", "registry.example.com-module-latest")
		fullModulePath := filepath.Join(extractionDir, "terraform/test-module")

		// Mock tar reader for successful extraction
		callCount := 0
		mockTarReader := &MockTarReader{
			NextFunc: func() (*tar.Header, error) {
				callCount++
				switch callCount {
				case 1:
					return &tar.Header{
						Name:     "metadata.yaml",
						Typeflag: tar.TypeReg,
						Mode:     0644,
					}, nil
				case 2:
					return &tar.Header{
						Name:     "terraform/test-module/",
						Typeflag: tar.TypeDir,
					}, nil
				case 3:
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

		// Mock Stat calls: first check for module path (not found), then check extraction dir (exists),
		// then after cleanup and re-extraction, module path exists
		originalStat := resolver.BaseModuleResolver.shims.Stat
		resolver.BaseModuleResolver.shims.Stat = func(name string) (os.FileInfo, error) {
			// Before extraction: module path doesn't exist
			if name == fullModulePath && !extractionComplete {
				return nil, os.ErrNotExist
			}
			// After extraction: module path exists
			if name == fullModulePath && extractionComplete {
				return nil, nil
			}
			// Tmp extraction dir never exists (we create it fresh)
			if name == tmpExtractionDir {
				return nil, os.ErrNotExist
			}
			// Extraction dir exists initially (from previous failed extraction)
			if name == extractionDir && !extractionComplete {
				return nil, nil
			}
			// After rename, extraction dir exists
			if name == extractionDir && extractionComplete {
				return nil, nil
			}
			return originalStat(name)
		}

		// Mock RemoveAll to clean up existing extraction directory
		removeAllCalled := false
		originalRemoveAll := resolver.BaseModuleResolver.shims.RemoveAll
		resolver.BaseModuleResolver.shims.RemoveAll = func(path string) error {
			if path == extractionDir {
				removeAllCalled = true
			}
			return originalRemoveAll(path)
		}

		// When extracting OCI module
		path, err := resolver.extractOCIModule(resolvedSource, componentPath, ociArtifacts)

		// Then it should clean up existing directory and extract fresh
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
		if path == "" {
			t.Error("Expected non-empty path")
		}
		if !removeAllCalled {
			t.Error("Expected RemoveAll to be called to clean up existing extraction directory")
		}
	})

	t.Run("HandlesErrors", func(t *testing.T) {
		// Given a resolver with various error conditions
		resolver := setup(t)

		errorCases := []struct {
			name           string
			resolvedSource string
			componentPath  string
			ociArtifacts   map[string][]byte
			expectedError  string
		}{
			{
				name:           "InvalidOCISourceFormat",
				resolvedSource: "invalid://source",
				componentPath:  "test-module",
				ociArtifacts:   map[string][]byte{},
				expectedError:  "invalid resolved OCI source format",
			},
			{
				name:           "MissingPathSeparator",
				resolvedSource: "oci://registry.example.com/module:latest",
				componentPath:  "test-module",
				ociArtifacts:   map[string][]byte{},
				expectedError:  "missing path separator",
			},
			{
				name:           "ArtifactNotFoundInCache",
				resolvedSource: "oci://registry.example.com/module:latest//terraform/test-module",
				componentPath:  "test-module",
				ociArtifacts:   map[string][]byte{},
				expectedError:  "not found in cache",
			},
		}

		for _, tc := range errorCases {
			// Set ProjectRoot for cases that need it
			if tc.name != "InvalidOCISourceFormat" && tc.name != "MissingPathSeparator" {
				resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"
			}
			// When extracting OCI module with error conditions
			_, err := resolver.extractOCIModule(tc.resolvedSource, tc.componentPath, tc.ociArtifacts)

			// Then it should return appropriate errors
			if err == nil {
				t.Errorf("Expected error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.expectedError) {
				t.Errorf("Expected error containing '%s' for %s, got: %v", tc.expectedError, tc.name, err)
			}
		}
	})

	t.Run("HandlesGetProjectRootError", func(t *testing.T) {
		// Given a resolver with GetProjectRoot error
		resolver := setup(t)
		resolvedSource := "oci://registry.example.com/module:latest//terraform/test-module"
		componentPath := "test-module"
		ociArtifacts := map[string][]byte{
			"registry.example.com/module:latest": []byte("mock artifact data"),
		}

		// Set ProjectRoot to empty to trigger error
		resolver.BaseModuleResolver.runtime.ProjectRoot = ""

		// When extracting OCI module
		_, err := resolver.extractOCIModule(resolvedSource, componentPath, ociArtifacts)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get project root") {
			t.Errorf("Expected project root error, got: %v", err)
		}
	})

	t.Run("HandlesParseOCIRefError", func(t *testing.T) {
		// Given a resolver with invalid OCI reference format
		resolver := setup(t)
		resolvedSource := "oci://invalid-format//terraform/test-module"
		componentPath := "test-module"
		ociArtifacts := map[string][]byte{}

		// When extracting OCI module
		_, err := resolver.extractOCIModule(resolvedSource, componentPath, ociArtifacts)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse OCI reference") {
			t.Errorf("Expected OCI reference parse error, got: %v", err)
		}
	})

	t.Run("HandlesExtractModuleFromArtifactError", func(t *testing.T) {
		// Given a resolver with extraction error
		resolver := setup(t)
		resolvedSource := "oci://registry.example.com/module:latest//terraform/test-module"
		componentPath := "test-module"
		ociArtifacts := map[string][]byte{
			"registry.example.com/module:latest": []byte("mock artifact data"),
		}

		// Set ProjectRoot to empty to trigger error during extraction
		resolver.BaseModuleResolver.runtime.ProjectRoot = ""

		// When extracting OCI module
		_, err := resolver.extractOCIModule(resolvedSource, componentPath, ociArtifacts)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get project root") {
			t.Errorf("Expected project root error, got: %v", err)
		}
	})
}

func TestOCIModuleResolver_extractArtifactToCache(t *testing.T) {
	setup := func(t *testing.T) *OCIModuleResolver {
		t.Helper()
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		resolver := NewOCIModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)
		resolver.BaseModuleResolver.shims = mocks.Shims
		return resolver
	}

	t.Run("Success", func(t *testing.T) {
		// Given a resolver with valid artifact data
		resolver := setup(t)
		resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"
		artifactData := []byte("mock artifact data")
		registry := "registry"
		repository := "module"
		tag := "latest"

		// Mock successful tar extraction with file and directory
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

		// Mock Rename to succeed
		resolver.BaseModuleResolver.shims.Rename = func(oldpath, newpath string) error {
			return nil
		}

		// When extracting artifact to cache
		err := resolver.extractArtifactToCache(artifactData, registry, repository, tag)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("HandlesErrors", func(t *testing.T) {
		// Given a resolver with various error conditions
		resolver := setup(t)
		artifactData := []byte("mock artifact data")
		registry := "registry"
		repository := "module"
		tag := "latest"

		errorCases := []struct {
			name          string
			setupMocks    func(*OCIModuleResolver)
			expectedError string
		}{
			{
				name: "TarReaderError",
				setupMocks: func(r *OCIModuleResolver) {
					mockTarReader := &MockTarReader{
						NextFunc: func() (*tar.Header, error) {
							return nil, errors.New("tar read error")
						},
					}
					r.shims.NewTarReader = func(reader io.Reader) TarReader {
						return mockTarReader
					}
				},
				expectedError: "failed to read tar header",
			},
			{
				name: "DirectoryCreationError",
				setupMocks: func(r *OCIModuleResolver) {
					mockTarReader := &MockTarReader{
						NextFunc: func() (*tar.Header, error) {
							return &tar.Header{
								Name:     "terraform/test-module/",
								Typeflag: tar.TypeDir,
							}, nil
						},
					}
					r.shims.NewTarReader = func(reader io.Reader) TarReader {
						return mockTarReader
					}
					r.shims.MkdirAll = func(path string, perm os.FileMode) error {
						return errors.New("mkdir error")
					}
				},
				expectedError: "failed to create temporary extraction directory",
			},
		}

		for _, tc := range errorCases {
			// Set ProjectRoot for tests that need it
			resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"
			// When extracting with error conditions
			tc.setupMocks(resolver)
			err := resolver.extractArtifactToCache(artifactData, registry, repository, tag)

			// Then it should return appropriate errors
			if err == nil {
				t.Errorf("Expected error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.expectedError) {
				t.Errorf("Expected error containing '%s' for %s, got: %v", tc.expectedError, tc.name, err)
			}
		}
	})

	t.Run("HandlesGetProjectRootError", func(t *testing.T) {
		// Given a resolver with GetProjectRoot error
		resolver := setup(t)
		artifactData := []byte("mock artifact data")
		registry := "registry"
		repository := "module"
		tag := "latest"

		// Set ProjectRoot to empty to trigger error
		resolver.BaseModuleResolver.runtime.ProjectRoot = ""

		// When extracting artifact to cache
		err := resolver.extractArtifactToCache(artifactData, registry, repository, tag)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get project root") {
			t.Errorf("Expected project root error, got: %v", err)
		}
	})

	t.Run("HandlesFileCreationError", func(t *testing.T) {
		// Given a resolver with file creation error
		resolver := setup(t)
		resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"
		artifactData := []byte("mock artifact data")
		registry := "registry"
		repository := "module"
		tag := "latest"

		// Mock tar reader with file entry
		mockTarReader := &MockTarReader{
			NextFunc: func() (*tar.Header, error) {
				return &tar.Header{
					Name:     "terraform/test-module/main.tf",
					Typeflag: tar.TypeReg,
					Mode:     0644,
				}, nil
			},
		}
		resolver.BaseModuleResolver.shims.NewTarReader = func(r io.Reader) TarReader {
			return mockTarReader
		}

		// Mock Create to return error
		resolver.BaseModuleResolver.shims.Create = func(name string) (*os.File, error) {
			return nil, errors.New("file creation error")
		}

		// When extracting artifact to cache
		err := resolver.extractArtifactToCache(artifactData, registry, repository, tag)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create file") {
			t.Errorf("Expected file creation error, got: %v", err)
		}
	})

	t.Run("HandlesCopyError", func(t *testing.T) {
		// Given a resolver with copy error
		resolver := setup(t)
		resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"
		artifactData := []byte("mock artifact data")
		registry := "registry"
		repository := "module"
		tag := "latest"

		// Mock tar reader with file entry
		mockTarReader := &MockTarReader{
			NextFunc: func() (*tar.Header, error) {
				return &tar.Header{
					Name:     "terraform/test-module/main.tf",
					Typeflag: tar.TypeReg,
					Mode:     0644,
				}, nil
			},
		}
		resolver.BaseModuleResolver.shims.NewTarReader = func(r io.Reader) TarReader {
			return mockTarReader
		}

		// Mock Copy to return error
		resolver.BaseModuleResolver.shims.Copy = func(dst io.Writer, src io.Reader) (int64, error) {
			return 0, errors.New("copy error")
		}

		// When extracting artifact to cache
		err := resolver.extractArtifactToCache(artifactData, registry, repository, tag)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to write file") {
			t.Errorf("Expected file write error, got: %v", err)
		}
	})

	t.Run("HandlesChmodError", func(t *testing.T) {
		// Given a resolver with chmod error
		resolver := setup(t)
		resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"
		artifactData := []byte("mock artifact data")
		registry := "registry"
		repository := "module"
		tag := "latest"

		// Mock tar reader with file entry
		mockTarReader := &MockTarReader{
			NextFunc: func() (*tar.Header, error) {
				return &tar.Header{
					Name:     "terraform/test-module/main.tf",
					Typeflag: tar.TypeReg,
					Mode:     0644,
				}, nil
			},
		}
		resolver.BaseModuleResolver.shims.NewTarReader = func(r io.Reader) TarReader {
			return mockTarReader
		}

		// Mock Chmod to return error
		resolver.BaseModuleResolver.shims.Chmod = func(name string, mode os.FileMode) error {
			return errors.New("chmod error")
		}

		// When extracting artifact to cache
		err := resolver.extractArtifactToCache(artifactData, registry, repository, tag)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to set file permissions") {
			t.Errorf("Expected chmod error, got: %v", err)
		}
	})

	t.Run("CleansUpOnExtractionFailure", func(t *testing.T) {
		// Given a resolver that will fail during extraction
		resolver := setup(t)
		resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"
		artifactData := []byte("mock artifact data")
		registry := "registry"
		repository := "module"
		tag := "latest"

		extractionKey := fmt.Sprintf("%s-%s-%s", registry, repository, tag)
		tmpExtractionDir := filepath.Join("/test/project", ".windsor", ".oci_extracted", extractionKey+".tmp")

		// Mock tar reader that will fail
		callCount := 0
		mockTarReader := &MockTarReader{
			NextFunc: func() (*tar.Header, error) {
				callCount++
				if callCount == 1 {
					return &tar.Header{
						Name:     "terraform/test-module/",
						Typeflag: tar.TypeDir,
					}, nil
				}
				return nil, errors.New("extraction error")
			},
			ReadFunc: func(p []byte) (int, error) {
				return 0, io.EOF
			},
		}
		resolver.BaseModuleResolver.shims.NewTarReader = func(r io.Reader) TarReader {
			return mockTarReader
		}

		// Mock Stat to indicate tmp directory was created
		statCallCount := 0
		resolver.BaseModuleResolver.shims.Stat = func(name string) (os.FileInfo, error) {
			statCallCount++
			if name == tmpExtractionDir {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock RemoveAll to track cleanup
		removeAllCalled := false
		originalRemoveAll := resolver.BaseModuleResolver.shims.RemoveAll
		resolver.BaseModuleResolver.shims.RemoveAll = func(path string) error {
			if path == tmpExtractionDir {
				removeAllCalled = true
			}
			return originalRemoveAll(path)
		}

		// When extraction fails
		err := resolver.extractArtifactToCache(artifactData, registry, repository, tag)

		// Then it should return an error and clean up the directory
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !removeAllCalled {
			t.Error("Expected RemoveAll to be called to clean up on extraction failure")
		}
	})

	t.Run("HandlesParentDirectoryCreationError", func(t *testing.T) {
		// Given a resolver with parent directory creation error
		resolver := setup(t)
		resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"
		artifactData := []byte("mock artifact data")
		registry := "registry"
		repository := "module"
		tag := "latest"

		// Mock tar reader with file entry
		mockTarReader := &MockTarReader{
			NextFunc: func() (*tar.Header, error) {
				return &tar.Header{
					Name:     "terraform/test-module/subdir/main.tf",
					Typeflag: tar.TypeReg,
					Mode:     0644,
				}, nil
			},
		}
		resolver.BaseModuleResolver.shims.NewTarReader = func(r io.Reader) TarReader {
			return mockTarReader
		}

		// Mock MkdirAll to return error for parent directory creation
		callCount := 0
		resolver.BaseModuleResolver.shims.MkdirAll = func(path string, perm os.FileMode) error {
			callCount++
			if callCount > 1 { // First call succeeds, second fails
				return errors.New("parent directory creation error")
			}
			return nil
		}

		// When extracting artifact to cache
		err := resolver.extractArtifactToCache(artifactData, registry, repository, tag)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create parent directory") {
			t.Errorf("Expected parent directory creation error, got: %v", err)
		}
	})
}

func TestOCIModuleResolver_processComponent(t *testing.T) {
	setup := func(t *testing.T) *OCIModuleResolver {
		t.Helper()
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		resolver := NewOCIModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)
		resolver.BaseModuleResolver.shims = mocks.Shims
		return resolver
	}

	t.Run("Success", func(t *testing.T) {
		// Given a resolver with valid component and artifact
		resolver := setup(t)
		component := blueprintv1alpha1.TerraformComponent{
			Path:     "test-module",
			Source:   "oci://registry.example.com/module:latest//terraform/test-module",
			FullPath: "/mock/project/terraform/test-module",
		}
		ociArtifacts := map[string][]byte{
			"registry.example.com/module:latest": []byte("mock artifact data"),
		}

		resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"
		extractionDir := filepath.Join("/test/project", ".windsor", ".oci_extracted", "registry.example.com-module-latest")
		fullModulePath := filepath.Join(extractionDir, "terraform/test-module")

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

		// Mock Stat calls
		originalStat := resolver.BaseModuleResolver.shims.Stat
		resolver.BaseModuleResolver.shims.Stat = func(name string) (os.FileInfo, error) {
			// Before extraction: module path doesn't exist
			if name == fullModulePath && !extractionComplete {
				return nil, os.ErrNotExist
			}
			// After extraction: module path exists
			if name == fullModulePath && extractionComplete {
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

		// When processing component
		err := resolver.processComponent(component, ociArtifacts)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("HandlesErrors", func(t *testing.T) {
		// Given a resolver with directory creation error
		resolver := setup(t)
		component := blueprintv1alpha1.TerraformComponent{
			Path:     "test-module",
			Source:   "oci://registry.example.com/module:latest//terraform/test-module",
			FullPath: "/mock/project/terraform/test-module",
		}
		ociArtifacts := map[string][]byte{
			"registry.example.com/module:latest": []byte("mock artifact data"),
		}

		// Mock MkdirAll to return error
		resolver.BaseModuleResolver.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return errors.New("mkdir error")
		}

		// When processing component
		err := resolver.processComponent(component, ociArtifacts)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create module directory") {
			t.Errorf("Expected directory creation error, got: %v", err)
		}
	})

	t.Run("HandlesExtractOCIModuleError", func(t *testing.T) {
		// Given a resolver with extract OCI module error
		resolver := setup(t)
		component := blueprintv1alpha1.TerraformComponent{
			Path:     "test-module",
			Source:   "oci://registry.example.com/module:latest//terraform/test-module",
			FullPath: "/mock/project/terraform/test-module",
		}
		ociArtifacts := map[string][]byte{} // Empty artifacts to trigger error

		// When processing component
		err := resolver.processComponent(component, ociArtifacts)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to extract OCI module") {
			t.Errorf("Expected OCI module extraction error, got: %v", err)
		}
	})

	t.Run("HandlesFilepathRelError", func(t *testing.T) {
		// Given a resolver with FilepathRel error
		resolver := setup(t)
		component := blueprintv1alpha1.TerraformComponent{
			Path:     "test-module",
			Source:   "oci://registry.example.com/module:latest//terraform/test-module",
			FullPath: "/mock/project/terraform/test-module",
		}
		ociArtifacts := map[string][]byte{
			"registry.example.com/module:latest": []byte("mock artifact data"),
		}

		resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"

		// Mock Stat to indicate module path already exists (cache hit)
		resolver.BaseModuleResolver.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "terraform/test-module") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock FilepathRel to return error
		resolver.BaseModuleResolver.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return "", errors.New("filepath rel error")
		}

		// When processing component
		err := resolver.processComponent(component, ociArtifacts)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to calculate relative path") {
			t.Errorf("Expected relative path calculation error, got: %v", err)
		}
	})

	t.Run("HandlesWriteShimMainTfError", func(t *testing.T) {
		// Given a resolver with writeShimMainTf error
		resolver := setup(t)
		component := blueprintv1alpha1.TerraformComponent{
			Path:     "test-module",
			Source:   "oci://registry.example.com/module:latest//terraform/test-module",
			FullPath: "/mock/project/terraform/test-module",
		}
		ociArtifacts := map[string][]byte{
			"registry.example.com/module:latest": []byte("mock artifact data"),
		}

		resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"

		// Mock Stat to indicate module path already exists (cache hit)
		resolver.BaseModuleResolver.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "terraform/test-module") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock WriteFile to return error for main.tf
		resolver.BaseModuleResolver.shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(path, "main.tf") {
				return errors.New("write main.tf error")
			}
			return nil
		}

		// When processing component
		err := resolver.processComponent(component, ociArtifacts)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to write main.tf") {
			t.Errorf("Expected main.tf write error, got: %v", err)
		}
	})

	t.Run("HandlesWriteShimVariablesTfError", func(t *testing.T) {
		// Given a resolver with writeShimVariablesTf error
		resolver := setup(t)
		component := blueprintv1alpha1.TerraformComponent{
			Path:     "test-module",
			Source:   "oci://registry.example.com/module:latest//terraform/test-module",
			FullPath: "/mock/project/terraform/test-module",
		}
		ociArtifacts := map[string][]byte{
			"registry.example.com/module:latest": []byte("mock artifact data"),
		}

		// Mock tar reader for successful extraction
		mockTarReader := &MockTarReader{
			NextFunc: func() (*tar.Header, error) {
				return nil, io.EOF
			},
		}
		resolver.BaseModuleResolver.shims.NewTarReader = func(r io.Reader) TarReader {
			return mockTarReader
		}
		resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"

		// The extraction key is built from registry-repository-tag
		extractionDir := filepath.Join("/test/project", ".windsor", ".oci_extracted", "registry.example.com-module-latest")
		modulePath := "terraform/test-module"
		extractedPath := filepath.Join(extractionDir, modulePath)
		variablesTfPath := filepath.Join(extractedPath, "variables.tf")

		// Mock Stat to indicate module path already exists (cache hit)
		resolver.BaseModuleResolver.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "terraform/test-module") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock Glob to return a variables.tf file so writeShimVariablesTf will try to process it
		originalGlob := resolver.BaseModuleResolver.shims.Glob
		resolver.BaseModuleResolver.shims.Glob = func(pattern string) ([]string, error) {
			// Match any pattern that ends with *.tf - be more permissive for cross-platform compatibility
			if strings.HasSuffix(pattern, "*.tf") {
				// Normalize paths for comparison (convert to forward slashes for consistent matching)
				normalizedPattern := filepath.ToSlash(pattern)
				// Check if this pattern is for the extracted module by looking for key identifiers
				if strings.Contains(normalizedPattern, ".oci_extracted") ||
					strings.Contains(normalizedPattern, "test-module") ||
					strings.Contains(normalizedPattern, modulePath) {
					return []string{variablesTfPath}, nil
				}
			}
			return originalGlob(pattern)
		}

		// Mock ReadFile to return variable content
		originalReadFile := resolver.BaseModuleResolver.shims.ReadFile
		resolver.BaseModuleResolver.shims.ReadFile = func(path string) ([]byte, error) {
			// Use normalized path comparison for cross-platform compatibility
			normalizedPath := filepath.ToSlash(filepath.Clean(path))
			normalizedVariablesTfPath := filepath.ToSlash(filepath.Clean(variablesTfPath))
			if normalizedPath == normalizedVariablesTfPath || strings.HasSuffix(normalizedPath, "terraform/test-module/variables.tf") {
				return []byte(`variable "test" { type = string }`), nil
			}
			return originalReadFile(path)
		}

		// Mock WriteFile to return error for variables.tf
		resolver.BaseModuleResolver.shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(path, "variables.tf") {
				return errors.New("write variables.tf error")
			}
			return nil
		}

		// When processing component
		err := resolver.processComponent(component, ociArtifacts)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "failed to write variables.tf") {
			t.Errorf("Expected variables.tf write error, got: %v", err)
		}
	})

	t.Run("HandlesWriteShimOutputsTfError", func(t *testing.T) {
		// Given a resolver with writeShimOutputsTf error
		resolver := setup(t)
		component := blueprintv1alpha1.TerraformComponent{
			Path:     "test-module",
			Source:   "oci://registry.example.com/module:latest//terraform/test-module",
			FullPath: "/mock/project/terraform/test-module",
		}
		ociArtifacts := map[string][]byte{
			"registry.example.com/module:latest": []byte("mock artifact data"),
		}

		resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"

		// Mock Stat to indicate module path already exists (cache hit)
		resolver.BaseModuleResolver.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "terraform/test-module") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock WriteFile to return error for outputs.tf
		resolver.BaseModuleResolver.shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(path, "outputs.tf") {
				return errors.New("write outputs.tf error")
			}
			return nil
		}

		// When processing component
		err := resolver.processComponent(component, ociArtifacts)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to write outputs.tf") {
			t.Errorf("Expected outputs.tf write error, got: %v", err)
		}
	})
}

func TestOCIModuleResolver_validateAndSanitizePath(t *testing.T) {
	setup := func(t *testing.T) *OCIModuleResolver {
		t.Helper()
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		resolver := NewOCIModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)
		resolver.BaseModuleResolver.shims = mocks.Shims
		return resolver
	}

	t.Run("HandlesValidPaths", func(t *testing.T) {
		// Given a resolver
		resolver := setup(t)

		// When validating valid paths
		testCases := []string{
			"terraform/module/main.tf",
			"terraform/module/subdir/file.tf",
			"module/file.tf",
		}

		for _, path := range testCases {
			// Then it should succeed
			result, err := resolver.BaseModuleResolver.validateAndSanitizePath(path)
			if err != nil {
				t.Errorf("Expected no error for %s, got %v", path, err)
			}
			if result == "" {
				t.Errorf("Expected non-empty result for %s", path)
			}
		}
	})

	t.Run("HandlesDirectoryTraversal", func(t *testing.T) {
		// Given a resolver
		resolver := setup(t)

		// When validating paths with directory traversal
		testCases := []string{
			"../../etc/passwd",
			"terraform/../../../etc/passwd",
			"../module/file.tf",
			"module/../../file.tf",
		}

		for _, path := range testCases {
			// Then it should return an error
			_, err := resolver.validateAndSanitizePath(path)
			if err == nil {
				t.Errorf("Expected error for path with traversal %s, got nil", path)
			}
			if !strings.Contains(err.Error(), "directory traversal") {
				t.Errorf("Expected directory traversal error for %s, got: %v", path, err)
			}
		}
	})

	t.Run("HandlesAbsolutePaths", func(t *testing.T) {
		// Given a resolver
		resolver := setup(t)

		// When validating absolute paths
		// Tar archives use Unix-style paths (forward slashes) regardless of OS,
		// so test with both Unix-style and Windows-style absolute paths
		testCases := []string{
			// Unix-style absolute paths (what would come from tar archives)
			"/etc/passwd",
			"/root/file.tf",
			"/tmp/module/main.tf",
		}

		// Also test Windows-style absolute paths
		if runtime.GOOS == "windows" {
			testCases = append(testCases,
				filepath.Join("C:", string(filepath.Separator), "Windows", "System32", "config", "sam"),
				filepath.Join("C:", string(filepath.Separator), "Users", "file.tf"),
			)
		}

		for _, path := range testCases {
			// Then it should return an error
			_, err := resolver.validateAndSanitizePath(path)
			if err == nil {
				t.Errorf("Expected error for absolute path %s, got nil", path)
				continue
			}
			if !strings.Contains(err.Error(), "absolute paths are not allowed") {
				t.Errorf("Expected absolute path error for %s, got: %v", path, err)
			}
		}
	})

	t.Run("HandlesCleanPathWithTraversal", func(t *testing.T) {
		// Given a resolver
		resolver := setup(t)

		// When validating paths that clean to contain traversal
		path := "terraform/../module/../../etc/passwd"

		// Then it should return an error
		_, err := resolver.validateAndSanitizePath(path)
		if err == nil {
			t.Error("Expected error for path that cleans to traversal, got nil")
		}
	})
}
