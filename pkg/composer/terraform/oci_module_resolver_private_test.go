package terraform

import (
	"archive/tar"
	"errors"
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

func TestOCIModuleResolver_extractModuleFromArtifact(t *testing.T) {
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
		modulePath := "terraform/test-module"
		extractionKey := "registry-module-latest"

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

		// When extracting module from artifact
		err := resolver.extractModuleFromArtifact(artifactData, modulePath, extractionKey)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("HandlesErrors", func(t *testing.T) {
		// Given a resolver with various error conditions
		resolver := setup(t)
		artifactData := []byte("mock artifact data")
		modulePath := "terraform/test-module"
		extractionKey := "registry-module-latest"

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
				expectedError: "failed to create directory",
			},
		}

		for _, tc := range errorCases {
			// Set ProjectRoot for tests that need it
			resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"
			// When extracting with error conditions
			tc.setupMocks(resolver)
			err := resolver.extractModuleFromArtifact(artifactData, modulePath, extractionKey)

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
		modulePath := "terraform/test-module"
		extractionKey := "registry-module-latest"

		// Set ProjectRoot to empty to trigger error
		resolver.BaseModuleResolver.runtime.ProjectRoot = ""

		// When extracting module from artifact
		err := resolver.extractModuleFromArtifact(artifactData, modulePath, extractionKey)

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
		modulePath := "terraform/test-module"
		extractionKey := "registry-module-latest"

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

		// When extracting module from artifact
		err := resolver.extractModuleFromArtifact(artifactData, modulePath, extractionKey)

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
		modulePath := "terraform/test-module"
		extractionKey := "registry-module-latest"

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

		// When extracting module from artifact
		err := resolver.extractModuleFromArtifact(artifactData, modulePath, extractionKey)

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
		modulePath := "terraform/test-module"
		extractionKey := "registry-module-latest"

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

		// When extracting module from artifact
		err := resolver.extractModuleFromArtifact(artifactData, modulePath, extractionKey)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to set file permissions") {
			t.Errorf("Expected chmod error, got: %v", err)
		}
	})

	t.Run("HandlesParentDirectoryCreationError", func(t *testing.T) {
		// Given a resolver with parent directory creation error
		resolver := setup(t)
		resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"
		artifactData := []byte("mock artifact data")
		modulePath := "terraform/test-module"
		extractionKey := "registry-module-latest"

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

		// When extracting module from artifact
		err := resolver.extractModuleFromArtifact(artifactData, modulePath, extractionKey)

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
			result, err := resolver.validateAndSanitizePath(path)
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
		// Use platform-specific absolute paths
		var testCases []string
		if runtime.GOOS == "windows" {
			// On Windows, use Windows-style absolute paths with drive letters
			testCases = []string{
				filepath.Join("C:", "Windows", "System32", "config", "sam"),
				filepath.Join("C:", "Users", "file.tf"),
				filepath.Join("C:", string(filepath.Separator), "tmp", "module", "main.tf"),
			}
		} else {
			// On Unix-like systems, use Unix-style absolute paths
			testCases = []string{
				filepath.Join(string(filepath.Separator), "etc", "passwd"),
				filepath.Join(string(filepath.Separator), "root", "file.tf"),
				filepath.Join(string(filepath.Separator), "tmp", "module", "main.tf"),
			}
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
