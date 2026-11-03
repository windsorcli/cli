package terraform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer/artifact"
)

// =============================================================================
// Test Private Methods
// =============================================================================

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
		path, err := resolver.extractOCIModule(resolvedSource, ociArtifacts)

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
		path, err := resolver.extractOCIModule(resolvedSource, ociArtifacts)

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
			ociArtifacts   map[string]string
			expectedError  string
		}{
			{
				name:           "InvalidOCISourceFormat",
				resolvedSource: "invalid://source",
				ociArtifacts:   map[string]string{},
				expectedError:  "invalid resolved OCI source format",
			},
			{
				name:           "MissingPathSeparator",
				resolvedSource: "oci://registry.example.com/module:latest",
				ociArtifacts:   map[string]string{},
				expectedError:  "missing path separator",
			},
			{
				name:           "ArtifactNotFoundInCache",
				resolvedSource: "oci://registry.example.com/module:latest//terraform/test-module",
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
			_, err := resolver.extractOCIModule(tc.resolvedSource, tc.ociArtifacts)

			// Then it should return appropriate errors
			if err == nil {
				t.Errorf("Expected error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.expectedError) {
				t.Errorf("Expected error containing '%s' for %s, got: %v", tc.expectedError, tc.name, err)
			}
		}
	})

	t.Run("MissingPathSeparatorHasRemediation", func(t *testing.T) {
		// Given a resolved OCI source without a module path
		resolver := setup(t)

		// When extracting the OCI module
		_, err := resolver.extractOCIModule("oci://registry.example.com/module:latest", map[string]string{})

		// Then the error should carry a remediation hint with the source format
		werr := werror.Find(err)
		if werr == nil {
			t.Fatalf("Expected a WindsorError, got: %v", err)
		}
		if !strings.Contains(werr.Remediation, "oci://registry/repository:tag//terraform/module") {
			t.Errorf("Expected remediation with the OCI source format, got: %q", werr.Remediation)
		}
	})

	t.Run("HandlesGetProjectRootError", func(t *testing.T) {
		// Given a resolver with GetProjectRoot error
		resolver := setup(t)
		resolvedSource := "oci://registry.example.com/module:latest//terraform/test-module"
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
		_, err := resolver.extractOCIModule(resolvedSource, ociArtifacts)

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
		ociArtifacts := map[string]string{}

		// Set up ParseOCIRef mock to return error
		resolver.artifactBuilder.(*artifact.MockArtifact).ParseOCIRefFunc = func(ociRef string) (registry, repository, tag string, err error) {
			return "", "", "", fmt.Errorf("invalid OCI reference format, expected registry/repository:tag: %s", ociRef)
		}

		// When extracting OCI module
		_, err := resolver.extractOCIModule(resolvedSource, ociArtifacts)

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
		_, err := resolver.extractOCIModule(resolvedSource, ociArtifacts)

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
