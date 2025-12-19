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
		artifactBuilder := artifact.NewArtifactBuilder(mocks.Runtime)
		resolver := NewOCIModuleResolver(mocks.Runtime, mocks.BlueprintHandler, artifactBuilder)
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
			registry, repository, tag, err := resolver.artifactBuilder.ParseOCIRef(tc.ociRef)
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
		artifactBuilder := artifact.NewArtifactBuilder(mocks.Runtime)
		resolver := NewOCIModuleResolver(mocks.Runtime, mocks.BlueprintHandler, artifactBuilder)
		resolver.BaseModuleResolver.shims = mocks.Shims

		// When parsing invalid OCI references
		testCases := []string{
			"invalid://reference",
			"oci://invalid-format",
			"oci://registry/module",
		}

		for _, ociRef := range testCases {
			// Then it should return an error
			_, _, _, err := resolver.artifactBuilder.ParseOCIRef(ociRef)
			if err == nil {
				t.Errorf("Expected error for invalid reference %s, got nil", ociRef)
			}
		}
	})

	t.Run("HandlesEdgeCases", func(t *testing.T) {
		// Given a resolver
		mocks := setupTerraformMocks(t)
		artifactBuilder := artifact.NewArtifactBuilder(mocks.Runtime)
		resolver := NewOCIModuleResolver(mocks.Runtime, mocks.BlueprintHandler, artifactBuilder)
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
			_, _, _, err := resolver.artifactBuilder.ParseOCIRef(tc.ociRef)
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
		artifactBuilder := artifact.NewArtifactBuilder(mocks.Runtime)
		resolver := NewOCIModuleResolver(mocks.Runtime, mocks.BlueprintHandler, artifactBuilder)
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
			registry, repository, tag, err := resolver.artifactBuilder.ParseOCIRef(tc.ociRef)
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
		ociArtifacts := map[string]string{
			"registry.example.com/module:latest": "/test/project/.windsor/cache/oci/registry.example.com_module_latest",
		}

		// Set up ParseOCIRef mock
		mockArtifact := resolver.artifactBuilder.(*artifact.MockArtifact)
		mockArtifact.ParseOCIRefFunc = func(ociRef string) (registry, repository, tag string, err error) {
			if ociRef == "oci://registry.example.com/module:latest" {
				return "registry.example.com", "module", "latest", nil
			}
			return "", "", "", fmt.Errorf("invalid OCI reference format: %s", ociRef)
		}
		mockArtifact.GetCacheDirFunc = func(registry, repository, tag string) (string, error) {
			return "/test/project/.windsor/cache/oci/registry.example.com_module_latest", nil
		}
		mockArtifact.ExtractModulePathFunc = func(registry, repository, tag, modulePath string) (string, error) {
			return "/test/project/.windsor/cache/oci/registry.example.com_module_latest/terraform/test-module", nil
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
		ociArtifacts := map[string]string{
			"registry.example.com/module:latest": "/test/project/.windsor/cache/oci/registry.example.com_module_latest",
		}

		// Set up ParseOCIRef mock
		resolver.artifactBuilder.(*artifact.MockArtifact).ParseOCIRefFunc = func(ociRef string) (registry, repository, tag string, err error) {
			if ociRef == "oci://registry.example.com/module:latest" {
				return "registry.example.com", "module", "latest", nil
			}
			return "", "", "", fmt.Errorf("invalid OCI reference format: %s", ociRef)
		}

		// Set up mocks for cache hit
		cacheDir := filepath.Join("/test/project", ".windsor", "cache", "oci", "registry.example.com_module_latest")
		fullModulePath := filepath.Join(cacheDir, "terraform/test-module")
		mockArtifact := resolver.artifactBuilder.(*artifact.MockArtifact)
		mockArtifact.GetCacheDirFunc = func(registry, repository, tag string) (string, error) {
			return cacheDir, nil
		}
		mockArtifact.ExtractModulePathFunc = func(registry, repository, tag, modulePath string) (string, error) {
			return fullModulePath, nil
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
		if path != fullModulePath {
			t.Errorf("Expected path %s, got %s", fullModulePath, path)
		}
	})

	t.Run("HandlesErrors", func(t *testing.T) {
		// Given a resolver with various error conditions
		resolver := setup(t)

		errorCases := []struct {
			name           string
			resolvedSource string
			componentPath  string
			ociArtifacts   map[string]string
			expectedError  string
		}{
			{
				name:           "InvalidOCISourceFormat",
				resolvedSource: "invalid://source",
				componentPath:  "test-module",
				ociArtifacts:   map[string]string{},
				expectedError:  "invalid resolved OCI source format",
			},
			{
				name:           "MissingPathSeparator",
				resolvedSource: "oci://registry.example.com/module:latest",
				componentPath:  "test-module",
				ociArtifacts:   map[string]string{},
				expectedError:  "missing path separator",
			},
			{
				name:           "ArtifactNotFoundInCache",
				resolvedSource: "oci://registry.example.com/module:latest//terraform/test-module",
				componentPath:  "test-module",
				ociArtifacts:   map[string]string{},
				expectedError:  "not found in cache",
			},
		}

		for _, tc := range errorCases {
			// Set ProjectRoot for cases that need it
			if tc.name != "InvalidOCISourceFormat" && tc.name != "MissingPathSeparator" {
				resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"
			}
			// Set up mocks for valid OCI references
			mockArtifact := resolver.artifactBuilder.(*artifact.MockArtifact)
			if strings.HasPrefix(tc.resolvedSource, "oci://") && tc.name == "ArtifactNotFoundInCache" {
				mockArtifact.ParseOCIRefFunc = func(ociRef string) (registry, repository, tag string, err error) {
					if ociRef == "oci://registry.example.com/module:latest" {
						return "registry.example.com", "module", "latest", nil
					}
					return "", "", "", fmt.Errorf("invalid OCI reference format: %s", ociRef)
				}
				mockArtifact.GetCacheDirFunc = func(registry, repository, tag string) (string, error) {
					return "/test/project/.windsor/cache/oci/registry.example.com_module_latest", nil
				}
				mockArtifact.ExtractModulePathFunc = func(registry, repository, tag, modulePath string) (string, error) {
					return "", fmt.Errorf("extraction error")
				}
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
		ociArtifacts := map[string]string{
			"registry.example.com/module:latest": "/test/project/.windsor/cache/oci/registry.example.com_module_latest",
		}

		// Set up ParseOCIRef mock
		resolver.artifactBuilder.(*artifact.MockArtifact).ParseOCIRefFunc = func(ociRef string) (registry, repository, tag string, err error) {
			if ociRef == "oci://registry.example.com/module:latest" {
				return "registry.example.com", "module", "latest", nil
			}
			return "", "", "", fmt.Errorf("invalid OCI reference format: %s", ociRef)
		}

		// Set up ExtractModulePath mock to return error when project root is empty
		mockArtifact := resolver.artifactBuilder.(*artifact.MockArtifact)
		mockArtifact.ExtractModulePathFunc = func(registry, repository, tag, modulePath string) (string, error) {
			return "", fmt.Errorf("failed to get cache directory: project root is not set")
		}

		// Set ProjectRoot to empty to trigger error
		resolver.BaseModuleResolver.runtime.ProjectRoot = ""

		// When extracting OCI module
		_, err := resolver.extractOCIModule(resolvedSource, componentPath, ociArtifacts)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get cache directory") && !strings.Contains(err.Error(), "project root") {
			t.Errorf("Expected cache directory or project root error, got: %v", err)
		}
	})

	t.Run("HandlesParseOCIRefError", func(t *testing.T) {
		// Given a resolver with invalid OCI reference format
		resolver := setup(t)
		resolvedSource := "oci://invalid-format//terraform/test-module"
		componentPath := "test-module"
		ociArtifacts := map[string]string{}

		// Set up ParseOCIRef mock to return error
		resolver.artifactBuilder.(*artifact.MockArtifact).ParseOCIRefFunc = func(ociRef string) (registry, repository, tag string, err error) {
			return "", "", "", fmt.Errorf("invalid OCI reference format, expected registry/repository:tag: %s", ociRef)
		}

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
		ociArtifacts := map[string]string{
			"registry.example.com/module:latest": "/test/project/.windsor/cache/oci/registry.example.com_module_latest",
		}

		// Set up ParseOCIRef mock
		resolver.artifactBuilder.(*artifact.MockArtifact).ParseOCIRefFunc = func(ociRef string) (registry, repository, tag string, err error) {
			if ociRef == "oci://registry.example.com/module:latest" {
				return "registry.example.com", "module", "latest", nil
			}
			return "", "", "", fmt.Errorf("invalid OCI reference format: %s", ociRef)
		}

		// Set up ExtractModulePath mock to return error
		mockArtifact := resolver.artifactBuilder.(*artifact.MockArtifact)
		mockArtifact.ExtractModulePathFunc = func(registry, repository, tag, modulePath string) (string, error) {
			return "", fmt.Errorf("failed to extract module: extraction error")
		}

		// When extracting OCI module
		_, err := resolver.extractOCIModule(resolvedSource, componentPath, ociArtifacts)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to extract module path") && !strings.Contains(err.Error(), "extraction error") {
			t.Errorf("Expected extraction error, got: %v", err)
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
		ociArtifacts := map[string]string{
			"registry.example.com/module:latest": "/test/project/.windsor/cache/oci/registry.example.com_module_latest",
		}

		// Set up ParseOCIRef mock
		resolver.artifactBuilder.(*artifact.MockArtifact).ParseOCIRefFunc = func(ociRef string) (registry, repository, tag string, err error) {
			if ociRef == "oci://registry.example.com/module:latest" {
				return "registry.example.com", "module", "latest", nil
			}
			return "", "", "", fmt.Errorf("invalid OCI reference format: %s", ociRef)
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
		ociArtifacts := map[string]string{
			"registry.example.com/module:latest": "/test/project/.windsor/cache/oci/registry.example.com_module_latest",
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
		ociArtifacts := map[string]string{} // Empty artifacts to trigger error

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
		ociArtifacts := map[string]string{
			"registry.example.com/module:latest": "/test/project/.windsor/cache/oci/registry.example.com_module_latest",
		}

		// Set up ParseOCIRef mock
		resolver.artifactBuilder.(*artifact.MockArtifact).ParseOCIRefFunc = func(ociRef string) (registry, repository, tag string, err error) {
			if ociRef == "oci://registry.example.com/module:latest" {
				return "registry.example.com", "module", "latest", nil
			}
			return "", "", "", fmt.Errorf("invalid OCI reference format: %s", ociRef)
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
		ociArtifacts := map[string]string{
			"registry.example.com/module:latest": "/test/project/.windsor/cache/oci/registry.example.com_module_latest",
		}

		// Set up ParseOCIRef mock
		resolver.artifactBuilder.(*artifact.MockArtifact).ParseOCIRefFunc = func(ociRef string) (registry, repository, tag string, err error) {
			if ociRef == "oci://registry.example.com/module:latest" {
				return "registry.example.com", "module", "latest", nil
			}
			return "", "", "", fmt.Errorf("invalid OCI reference format: %s", ociRef)
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
		ociArtifacts := map[string]string{
			"registry.example.com/module:latest": "/test/project/.windsor/cache/oci/registry.example.com_module_latest",
		}

		// Set up ParseOCIRef mock
		resolver.artifactBuilder.(*artifact.MockArtifact).ParseOCIRefFunc = func(ociRef string) (registry, repository, tag string, err error) {
			if ociRef == "oci://registry.example.com/module:latest" {
				return "registry.example.com", "module", "latest", nil
			}
			return "", "", "", fmt.Errorf("invalid OCI reference format: %s", ociRef)
		}

		resolver.BaseModuleResolver.runtime.ProjectRoot = "/test/project"

		// The extraction key is built from registry-repository-tag
		cacheDir := filepath.Join("/test/project", ".windsor", "cache", "oci", "registry.example.com_module_latest")
		modulePath := "terraform/test-module"
		extractedPath := filepath.Join(cacheDir, modulePath)
		variablesTfPath := filepath.Join(extractedPath, "variables.tf")

		// Set up ExtractModulePath mock to return the extracted path
		mockArtifact := resolver.artifactBuilder.(*artifact.MockArtifact)
		mockArtifact.ExtractModulePathFunc = func(registry, repository, tag, modulePath string) (string, error) {
			return extractedPath, nil
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
					strings.Contains(normalizedPattern, "registry.example.com_module_latest") ||
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
		ociArtifacts := map[string]string{
			"registry.example.com/module:latest": "/test/project/.windsor/cache/oci/registry.example.com_module_latest",
		}

		// Set up ParseOCIRef mock
		resolver.artifactBuilder.(*artifact.MockArtifact).ParseOCIRefFunc = func(ociRef string) (registry, repository, tag string, err error) {
			if ociRef == "oci://registry.example.com/module:latest" {
				return "registry.example.com", "module", "latest", nil
			}
			return "", "", "", fmt.Errorf("invalid OCI reference format: %s", ociRef)
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
