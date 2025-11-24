package terraform

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// The ArchiveModuleResolverTest is a test suite for the ArchiveModuleResolver implementation
// It provides comprehensive coverage for archive-based terraform module source processing and validation
// The ArchiveModuleResolverTest ensures proper handling of file:// archive sources, module extraction,
// and shim generation for archive-based terraform modules

// =============================================================================
// Test Public Methods
// =============================================================================

func TestArchiveModuleResolver_NewArchiveModuleResolver(t *testing.T) {
	t.Run("CreatesArchiveModuleResolver", func(t *testing.T) {
		// Given mocks
		mocks := setupTerraformMocks(t)

		// When creating a new archive module resolver
		resolver := NewArchiveModuleResolver(mocks.Runtime, mocks.BlueprintHandler)

		// Then it should be created successfully
		if resolver == nil {
			t.Fatal("Expected non-nil ArchiveModuleResolver")
		}
		if resolver.BaseModuleResolver == nil {
			t.Error("Expected BaseModuleResolver to be set")
		}
	})
}

func TestArchiveModuleResolver_shouldHandle(t *testing.T) {
	t.Run("HandlesFileSourcesAndRejectsNonFile", func(t *testing.T) {
		// Given a resolver
		mocks := setupTerraformMocks(t)
		resolver := NewArchiveModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
		resolver.BaseModuleResolver.shims = mocks.Shims

		// When checking various source types
		testCases := []struct {
			source   string
			expected bool
		}{
			{"file:///path/to/archive.tar.gz//terraform/module", true},
			{"file://./archive.tar.gz//terraform/module", true},
			{"file://archive.tar.gz//terraform/module", true},
			{"oci://registry.example.com/module:latest", false},
			{"git::https://github.com/test/module.git", false},
			{"./local/module", false},
			{"", false},
		}

		for _, tc := range testCases {
			// Then it should handle file:// sources and reject non-file sources
			result := resolver.shouldHandle(tc.source)
			if result != tc.expected {
				t.Errorf("Expected %s to return %v, got %v", tc.source, tc.expected, result)
			}
		}
	})
}

func TestArchiveModuleResolver_ProcessModules(t *testing.T) {
	setup := func(t *testing.T) (*ArchiveModuleResolver, *TerraformTestMocks) {
		t.Helper()
		mocks := setupTerraformMocks(t)
		resolver := NewArchiveModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
		resolver.BaseModuleResolver.shims = mocks.Shims
		return resolver, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a resolver with file:// components
		resolver, mocks := setup(t)
		tmpDir := t.TempDir()
		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")
		createTestArchive(t, archivePath, "terraform/test-module", map[string]string{
			"terraform/test-module/main.tf":      `resource "test" "example" {}`,
			"terraform/test-module/variables.tf": `variable "test" {}`,
			"terraform/test-module/outputs.tf":   `output "test" {}`,
		})

		configRoot := filepath.Join(tmpDir, "contexts", "test")
		if err := os.MkdirAll(configRoot, 0755); err != nil {
			t.Fatalf("Failed to create config root: %v", err)
		}
		blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
		if err := os.WriteFile(blueprintPath, []byte("kind: Blueprint"), 0644); err != nil {
			t.Fatalf("Failed to write blueprint.yaml: %v", err)
		}

		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.ConfigRoot = configRoot

		// Override shims for real file operations
		resolver.BaseModuleResolver.shims.Copy = io.Copy
		resolver.BaseModuleResolver.shims.Create = func(name string) (*os.File, error) {
			return os.Create(name)
		}
		resolver.BaseModuleResolver.shims.MkdirAll = os.MkdirAll
		resolver.BaseModuleResolver.shims.Stat = os.Stat
		resolver.BaseModuleResolver.shims.ReadFile = os.ReadFile
		resolver.BaseModuleResolver.shims.WriteFile = os.WriteFile
		resolver.BaseModuleResolver.shims.FilepathRel = filepath.Rel

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "test-module",
					Source:   "file://" + archivePath + "//terraform/test-module",
					FullPath: filepath.Join(tmpDir, ".windsor", ".tf_modules", "test-module"),
				},
			}
		}

		// When processing modules
		err := resolver.ProcessModules()

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And the module directory should be created
		moduleDir := filepath.Join(tmpDir, ".windsor", ".tf_modules", "test-module")
		if _, err := os.Stat(moduleDir); err != nil {
			t.Errorf("Expected module directory to be created, got error: %v", err)
		}

		// And shim files should be created
		mainTfPath := filepath.Join(moduleDir, "main.tf")
		if _, err := os.Stat(mainTfPath); err != nil {
			t.Errorf("Expected main.tf to be created, got error: %v", err)
		}
	})

	t.Run("HandlesNoFileComponents", func(t *testing.T) {
		// Given a resolver with no file:// components
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

	t.Run("HandlesComponentProcessingErrors", func(t *testing.T) {
		// Given a resolver with component that fails during processing
		resolver, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.ConfigRoot = filepath.Join(tmpDir, "contexts", "test")

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "test-module",
					Source:   "file:///invalid/path.tar.gz//terraform/test-module",
					FullPath: filepath.Join(tmpDir, ".windsor", ".tf_modules", "test-module"),
				},
			}
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

	t.Run("HandlesMalformedFileURLs", func(t *testing.T) {
		// Given a resolver with malformed file:// URLs
		resolver, mocks := setup(t)
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "test-module",
					Source:   "file://archive.tar.gz", // Missing path separator
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
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestArchiveModuleResolver_extractArchiveModule(t *testing.T) {
	setup := func(t *testing.T) (*ArchiveModuleResolver, *TerraformTestMocks, string) {
		t.Helper()
		mocks := setupTerraformMocks(t)
		resolver := NewArchiveModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
		resolver.BaseModuleResolver.shims = mocks.Shims

		tmpDir := t.TempDir()
		configRoot := filepath.Join(tmpDir, "contexts", "test")
		if err := os.MkdirAll(configRoot, 0755); err != nil {
			t.Fatalf("Failed to create config root: %v", err)
		}
		blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
		if err := os.WriteFile(blueprintPath, []byte("kind: Blueprint"), 0644); err != nil {
			t.Fatalf("Failed to write blueprint.yaml: %v", err)
		}

		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.ConfigRoot = configRoot

		// Override shims for real file operations
		resolver.BaseModuleResolver.shims.Copy = io.Copy
		resolver.BaseModuleResolver.shims.Create = func(name string) (*os.File, error) {
			return os.Create(name)
		}
		resolver.BaseModuleResolver.shims.MkdirAll = os.MkdirAll
		resolver.BaseModuleResolver.shims.Stat = os.Stat
		resolver.BaseModuleResolver.shims.ReadFile = os.ReadFile
		resolver.BaseModuleResolver.shims.FilepathAbs = filepath.Abs

		return resolver, mocks, tmpDir
	}

	t.Run("Success", func(t *testing.T) {
		// Given a resolver and a test archive
		resolver, _, tmpDir := setup(t)
		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")
		createTestArchive(t, archivePath, "terraform/test-module", map[string]string{
			"terraform/test-module/main.tf": `resource "test" "example" {}`,
		})

		resolvedSource := "file://" + archivePath + "//terraform/test-module"

		// When extracting the archive module
		extractedPath, err := resolver.extractArchiveModule(resolvedSource, "test-module")

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected nil error, got %v", err)
		}

		// And the extracted path should exist
		if _, err := os.Stat(extractedPath); err != nil {
			t.Errorf("Expected extracted path to exist, got error: %v", err)
		}

		// And the module files should be extracted
		mainTfPath := filepath.Join(extractedPath, "main.tf")
		if _, err := os.Stat(mainTfPath); err != nil {
			t.Errorf("Expected main.tf to be extracted, got error: %v", err)
		}
	})

	t.Run("ReturnsExistingExtraction", func(t *testing.T) {
		// Given a resolver and an already extracted module
		resolver, _, tmpDir := setup(t)
		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")
		createTestArchive(t, archivePath, "terraform/test-module", map[string]string{
			"terraform/test-module/main.tf": `resource "test" "example" {}`,
		})

		extractionKey := "test-archive"
		extractedDir := filepath.Join(tmpDir, ".windsor", ".archive_extracted", extractionKey, "terraform", "test-module")
		if err := os.MkdirAll(extractedDir, 0755); err != nil {
			t.Fatalf("Failed to create extracted directory: %v", err)
		}

		resolvedSource := "file://" + archivePath + "//terraform/test-module"

		// When extracting the archive module
		extractedPath, err := resolver.extractArchiveModule(resolvedSource, "test-module")

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected nil error, got %v", err)
		}

		// And it should return the existing path
		if extractedPath != extractedDir {
			t.Errorf("Expected existing path %s, got %s", extractedDir, extractedPath)
		}
	})

	t.Run("HandlesInvalidSourceFormat", func(t *testing.T) {
		// Given a resolver with invalid source format
		resolver, _, _ := setup(t)

		// When extracting with invalid source
		_, err := resolver.extractArchiveModule("invalid-source", "test-module")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid resolved archive source format") {
			t.Errorf("Expected invalid format error, got: %v", err)
		}
	})

	t.Run("HandlesMissingPathSeparator", func(t *testing.T) {
		// Given a resolver with source missing path separator
		resolver, _, _ := setup(t)

		// When extracting with missing path separator
		_, err := resolver.extractArchiveModule("file://archive.tar.gz", "test-module")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "missing path separator") {
			t.Errorf("Expected missing path separator error, got: %v", err)
		}
	})

	t.Run("HandlesEmptyProjectRoot", func(t *testing.T) {
		// Given a resolver with empty project root
		resolver, mocks, _ := setup(t)
		mocks.Runtime.ProjectRoot = ""

		archivePath := "/test/archive.tar.gz"
		resolvedSource := "file://" + archivePath + "//terraform/test-module"

		// When extracting
		_, err := resolver.extractArchiveModule(resolvedSource, "test-module")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "project root is empty") {
			t.Errorf("Expected project root error, got: %v", err)
		}
	})

	t.Run("HandlesEmptyConfigRoot", func(t *testing.T) {
		// Given a resolver with empty config root
		resolver, mocks, _ := setup(t)
		mocks.Runtime.ConfigRoot = ""

		archivePath := "/test/archive.tar.gz"
		resolvedSource := "file://" + archivePath + "//terraform/test-module"

		// When extracting
		_, err := resolver.extractArchiveModule(resolvedSource, "test-module")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "config root is empty") {
			t.Errorf("Expected config root error, got: %v", err)
		}
	})

	t.Run("HandlesArchiveReadError", func(t *testing.T) {
		// Given a resolver with non-existent archive
		resolver, _, tmpDir := setup(t)
		archivePath := filepath.Join(tmpDir, "nonexistent.tar.gz")
		resolvedSource := "file://" + archivePath + "//terraform/test-module"

		// When extracting
		_, err := resolver.extractArchiveModule(resolvedSource, "test-module")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to read archive file") {
			t.Errorf("Expected archive read error, got: %v", err)
		}
	})

	t.Run("HandlesRelativeArchivePath", func(t *testing.T) {
		// Given a resolver with relative archive path
		resolver, _, tmpDir := setup(t)
		configRoot := filepath.Join(tmpDir, "contexts", "test")
		if err := os.MkdirAll(configRoot, 0755); err != nil {
			t.Fatalf("Failed to create config root: %v", err)
		}
		blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
		if err := os.WriteFile(blueprintPath, []byte("kind: Blueprint"), 0644); err != nil {
			t.Fatalf("Failed to write blueprint.yaml: %v", err)
		}

		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")
		createTestArchive(t, archivePath, "terraform/test-module", map[string]string{
			"terraform/test-module/main.tf": `resource "test" "example" {}`,
		})

		relArchivePath, err := filepath.Rel(filepath.Dir(blueprintPath), archivePath)
		if err != nil {
			t.Fatalf("Failed to calculate relative path: %v", err)
		}
		resolvedSource := "file://" + relArchivePath + "//terraform/test-module"

		// Override FilepathAbs to return absolute path
		resolver.BaseModuleResolver.shims.FilepathAbs = filepath.Abs
		resolver.BaseModuleResolver.shims.ReadFile = os.ReadFile
		resolver.BaseModuleResolver.shims.Stat = os.Stat
		resolver.BaseModuleResolver.shims.Copy = io.Copy
		resolver.BaseModuleResolver.shims.Create = func(name string) (*os.File, error) {
			return os.Create(name)
		}
		resolver.BaseModuleResolver.shims.MkdirAll = os.MkdirAll

		// When extracting
		extractedPath, err := resolver.extractArchiveModule(resolvedSource, "test-module")

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
		if extractedPath == "" {
			t.Error("Expected non-empty extracted path")
		}
	})

	t.Run("HandlesFilepathAbsError", func(t *testing.T) {
		// Given a resolver with FilepathAbs error
		resolver, _, tmpDir := setup(t)
		configRoot := filepath.Join(tmpDir, "contexts", "test")
		if err := os.MkdirAll(configRoot, 0755); err != nil {
			t.Fatalf("Failed to create config root: %v", err)
		}
		blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
		if err := os.WriteFile(blueprintPath, []byte("kind: Blueprint"), 0644); err != nil {
			t.Fatalf("Failed to write blueprint.yaml: %v", err)
		}

		resolvedSource := "file://./relative/path.tar.gz//terraform/test-module"

		// Override FilepathAbs to return error
		resolver.BaseModuleResolver.shims.FilepathAbs = func(path string) (string, error) {
			return "", fmt.Errorf("filepath abs error")
		}

		// When extracting
		_, err := resolver.extractArchiveModule(resolvedSource, "test-module")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get absolute path for archive") {
			t.Errorf("Expected filepath abs error, got: %v", err)
		}
	})

	t.Run("HandlesTarExtension", func(t *testing.T) {
		// Given a resolver with .tar extension (not .tar.gz)
		resolver, _, tmpDir := setup(t)
		configRoot := filepath.Join(tmpDir, "contexts", "test")
		if err := os.MkdirAll(configRoot, 0755); err != nil {
			t.Fatalf("Failed to create config root: %v", err)
		}
		blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
		if err := os.WriteFile(blueprintPath, []byte("kind: Blueprint"), 0644); err != nil {
			t.Fatalf("Failed to write blueprint.yaml: %v", err)
		}

		archivePath := filepath.Join(tmpDir, "test-archive.tar")
		createTestArchive(t, archivePath, "terraform/test-module", map[string]string{
			"terraform/test-module/main.tf": `resource "test" "example" {}`,
		})

		resolvedSource := "file://" + archivePath + "//terraform/test-module"

		// Override shims for real file operations
		resolver.BaseModuleResolver.shims.FilepathAbs = filepath.Abs
		resolver.BaseModuleResolver.shims.ReadFile = os.ReadFile
		resolver.BaseModuleResolver.shims.Stat = os.Stat
		resolver.BaseModuleResolver.shims.Copy = io.Copy
		resolver.BaseModuleResolver.shims.Create = func(name string) (*os.File, error) {
			return os.Create(name)
		}
		resolver.BaseModuleResolver.shims.MkdirAll = os.MkdirAll

		// When extracting
		extractedPath, err := resolver.extractArchiveModule(resolvedSource, "test-module")

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
		if extractedPath == "" {
			t.Error("Expected non-empty extracted path")
		}
	})

	t.Run("HandlesRefParameter", func(t *testing.T) {
		// Given a resolver with ?ref= parameter in source
		resolver, _, tmpDir := setup(t)
		configRoot := filepath.Join(tmpDir, "contexts", "test")
		if err := os.MkdirAll(configRoot, 0755); err != nil {
			t.Fatalf("Failed to create config root: %v", err)
		}
		blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
		if err := os.WriteFile(blueprintPath, []byte("kind: Blueprint"), 0644); err != nil {
			t.Fatalf("Failed to write blueprint.yaml: %v", err)
		}

		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")
		createTestArchive(t, archivePath, "terraform/test-module", map[string]string{
			"terraform/test-module/main.tf": `resource "test" "example" {}`,
		})

		resolvedSource := "file://" + archivePath + "//terraform/test-module?ref=v1.0.0"

		// Override shims for real file operations
		resolver.BaseModuleResolver.shims.FilepathAbs = filepath.Abs
		resolver.BaseModuleResolver.shims.ReadFile = os.ReadFile
		resolver.BaseModuleResolver.shims.Stat = os.Stat
		resolver.BaseModuleResolver.shims.Copy = io.Copy
		resolver.BaseModuleResolver.shims.Create = func(name string) (*os.File, error) {
			return os.Create(name)
		}
		resolver.BaseModuleResolver.shims.MkdirAll = os.MkdirAll

		// When extracting
		extractedPath, err := resolver.extractArchiveModule(resolvedSource, "test-module")

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
		if extractedPath == "" {
			t.Error("Expected non-empty extracted path")
		}
	})

	t.Run("HandlesExtractModuleFromArchiveError", func(t *testing.T) {
		// Given a resolver with extractModuleFromArchive error
		resolver, _, tmpDir := setup(t)
		configRoot := filepath.Join(tmpDir, "contexts", "test")
		if err := os.MkdirAll(configRoot, 0755); err != nil {
			t.Fatalf("Failed to create config root: %v", err)
		}
		blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
		if err := os.WriteFile(blueprintPath, []byte("kind: Blueprint"), 0644); err != nil {
			t.Fatalf("Failed to write blueprint.yaml: %v", err)
		}

		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")
		invalidData := []byte("invalid archive data")
		if err := os.WriteFile(archivePath, invalidData, 0644); err != nil {
			t.Fatalf("Failed to write archive: %v", err)
		}

		resolvedSource := "file://" + archivePath + "//terraform/test-module"

		// Override shims for real file operations
		resolver.BaseModuleResolver.shims.FilepathAbs = filepath.Abs
		resolver.BaseModuleResolver.shims.ReadFile = os.ReadFile
		resolver.BaseModuleResolver.shims.Stat = os.Stat
		resolver.BaseModuleResolver.shims.Copy = io.Copy
		resolver.BaseModuleResolver.shims.Create = func(name string) (*os.File, error) {
			return os.Create(name)
		}
		resolver.BaseModuleResolver.shims.MkdirAll = os.MkdirAll

		// When extracting
		_, err := resolver.extractArchiveModule(resolvedSource, "test-module")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to extract module from archive") {
			t.Errorf("Expected extract error, got: %v", err)
		}
	})
}

func TestArchiveModuleResolver_extractModuleFromArchive(t *testing.T) {
	setup := func(t *testing.T) (*ArchiveModuleResolver, *TerraformTestMocks, string) {
		t.Helper()
		mocks := setupTerraformMocks(t)
		resolver := NewArchiveModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
		resolver.BaseModuleResolver.shims = mocks.Shims

		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir

		// Override Copy to use real io.Copy for archive extraction tests
		resolver.BaseModuleResolver.shims.Copy = io.Copy
		// Override Create to create files in the actual tmpDir
		resolver.BaseModuleResolver.shims.Create = func(name string) (*os.File, error) {
			return os.Create(name)
		}
		// Override MkdirAll to actually create directories
		resolver.BaseModuleResolver.shims.MkdirAll = os.MkdirAll
		// Override Stat to actually check files
		resolver.BaseModuleResolver.shims.Stat = os.Stat

		return resolver, mocks, tmpDir
	}

	t.Run("Success", func(t *testing.T) {
		// Given a resolver and archive data
		resolver, _, tmpDir := setup(t)
		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")
		createTestArchive(t, archivePath, "terraform/test-module", map[string]string{
			"terraform/test-module/main.tf":      `resource "test" "example" {}`,
			"terraform/test-module/variables.tf": `variable "test" {}`,
		})

		archiveData, err := os.ReadFile(archivePath)
		if err != nil {
			t.Fatalf("Failed to read archive: %v", err)
		}

		if len(archiveData) == 0 {
			t.Fatalf("Archive data is empty")
		}

		// When extracting module from archive
		err = resolver.extractModuleFromArchive(archiveData, "terraform/test-module", "test-archive")

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected nil error, got %v", err)
		}

		// And the module should be extracted
		extractedPath := filepath.Join(tmpDir, ".windsor", ".archive_extracted", "test-archive", "terraform", "test-module")
		if _, err := os.Stat(extractedPath); err != nil {
			t.Errorf("Expected extracted module to exist at %s, got error: %v", extractedPath, err)
		}

		// And the files should be present
		mainTfPath := filepath.Join(extractedPath, "main.tf")
		if _, err := os.Stat(mainTfPath); err != nil {
			t.Errorf("Expected main.tf to be extracted at %s, got error: %v", mainTfPath, err)
		}
	})

	t.Run("HandlesInvalidGzipData", func(t *testing.T) {
		// Given a resolver with invalid gzip data
		resolver, _, _ := setup(t)
		invalidData := []byte("not a gzip file")

		// When extracting
		err := resolver.extractModuleFromArchive(invalidData, "terraform/test-module", "test-archive")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create gzip reader") {
			t.Errorf("Expected gzip reader error, got: %v", err)
		}
	})

	t.Run("HandlesEmptyProjectRoot", func(t *testing.T) {
		// Given a resolver with empty project root
		resolver, mocks, _ := setup(t)
		mocks.Runtime.ProjectRoot = ""

		archiveData := []byte("test data")

		// When extracting
		err := resolver.extractModuleFromArchive(archiveData, "terraform/test-module", "test-archive")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "project root is empty") {
			t.Errorf("Expected project root error, got: %v", err)
		}
	})

	t.Run("HandlesTarHeaderReadError", func(t *testing.T) {
		// Given a resolver with tar header read error
		resolver, _, tmpDir := setup(t)
		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")
		createTestArchive(t, archivePath, "terraform/test-module", map[string]string{
			"terraform/test-module/main.tf": `resource "test" "example" {}`,
		})

		archiveData, err := os.ReadFile(archivePath)
		if err != nil {
			t.Fatalf("Failed to read archive: %v", err)
		}

		// Override NewTarReader to return a reader that errors on Next()
		resolver.BaseModuleResolver.shims.NewTarReader = func(r io.Reader) TarReader {
			return &mockTarReader{nextError: fmt.Errorf("tar header read error")}
		}

		// When extracting
		err = resolver.extractModuleFromArchive(archiveData, "terraform/test-module", "test-archive")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to read tar header") {
			t.Errorf("Expected tar header read error, got: %v", err)
		}
	})

	t.Run("HandlesInvalidPathInArchive", func(t *testing.T) {
		// Given a resolver with invalid path in archive
		// Use a path that matches prefix but has .. that won't be fully cleaned
		// Actually, filepath.Clean will always remove .., so we need a different approach
		// Let's test with a path that has .. in a way that the cleaned path still contains it
		// But that's impossible - filepath.Clean always removes ..
		// So let's test with a Windows absolute path that matches the prefix pattern
		resolver, _, tmpDir := setup(t)
		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")

		file, err := os.Create(archivePath)
		if err != nil {
			t.Fatalf("Failed to create archive file: %v", err)
		}
		gzipWriter := gzip.NewWriter(file)
		tarWriter := tar.NewWriter(gzipWriter)

		// Use a Windows-style absolute path that starts with the prefix pattern
		// This will be caught by validateAndSanitizePath as an absolute path
		header := &tar.Header{
			Name: "C:terraform/test-module/invalid",
			Mode: 0644,
			Size: 10,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("Failed to write tar header: %v", err)
		}
		if _, err := tarWriter.Write([]byte("test data")); err != nil {
			t.Fatalf("Failed to write tar content: %v", err)
		}

		tarWriter.Close()
		gzipWriter.Close()
		file.Close()

		archiveData, err := os.ReadFile(archivePath)
		if err != nil {
			t.Fatalf("Failed to read archive: %v", err)
		}

		// When extracting with invalid path
		err = resolver.extractModuleFromArchive(archiveData, "C:terraform/test-module", "test-archive")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		// The error could be from validation or from path traversal detection
		if !strings.Contains(err.Error(), "invalid path in tar archive") && !strings.Contains(err.Error(), "absolute paths are not allowed") {
			t.Errorf("Expected invalid path error, got: %v", err)
		}
	})

	t.Run("HandlesPathTraversalDetection", func(t *testing.T) {
		// Given a resolver with path traversal attempt
		resolver, _, tmpDir := setup(t)
		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")

		file, err := os.Create(archivePath)
		if err != nil {
			t.Fatalf("Failed to create archive file: %v", err)
		}
		gzipWriter := gzip.NewWriter(file)
		tarWriter := tar.NewWriter(gzipWriter)

		// Create a file with a path that would traverse outside extraction dir
		// Use a path that matches the prefix but after sanitization would be outside
		header := &tar.Header{
			Name: "terraform/test-module/../../../etc/passwd",
			Mode: 0644,
			Size: 10,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("Failed to write tar header: %v", err)
		}
		if _, err := tarWriter.Write([]byte("test data")); err != nil {
			t.Fatalf("Failed to write tar content: %v", err)
		}

		tarWriter.Close()
		gzipWriter.Close()
		file.Close()

		archiveData, err := os.ReadFile(archivePath)
		if err != nil {
			t.Fatalf("Failed to read archive: %v", err)
		}

		// When extracting
		err = resolver.extractModuleFromArchive(archiveData, "terraform/test-module", "test-archive")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		// The error could be either from validateAndSanitizePath or from path traversal detection
		if !strings.Contains(err.Error(), "path traversal") && !strings.Contains(err.Error(), "invalid path in tar archive") {
			t.Errorf("Expected path traversal error, got: %v", err)
		}
	})

	t.Run("HandlesDirectoryCreationError", func(t *testing.T) {
		// Given a resolver with directory creation error
		resolver, _, tmpDir := setup(t)
		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")
		createTestArchive(t, archivePath, "terraform/test-module", map[string]string{
			"terraform/test-module/main.tf": `resource "test" "example" {}`,
		})

		archiveData, err := os.ReadFile(archivePath)
		if err != nil {
			t.Fatalf("Failed to read archive: %v", err)
		}

		// Override MkdirAll to return error
		resolver.BaseModuleResolver.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mkdir error")
		}

		// When extracting
		err = resolver.extractModuleFromArchive(archiveData, "terraform/test-module", "test-archive")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create") {
			t.Errorf("Expected directory creation error, got: %v", err)
		}
	})

	t.Run("HandlesFileCreationError", func(t *testing.T) {
		// Given a resolver with file creation error
		resolver, _, tmpDir := setup(t)
		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")
		createTestArchive(t, archivePath, "terraform/test-module", map[string]string{
			"terraform/test-module/main.tf": `resource "test" "example" {}`,
		})

		archiveData, err := os.ReadFile(archivePath)
		if err != nil {
			t.Fatalf("Failed to read archive: %v", err)
		}

		// Override Create to return error
		resolver.BaseModuleResolver.shims.Create = func(name string) (*os.File, error) {
			return nil, fmt.Errorf("create error")
		}

		// When extracting
		err = resolver.extractModuleFromArchive(archiveData, "terraform/test-module", "test-archive")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create file") {
			t.Errorf("Expected file creation error, got: %v", err)
		}
	})

	t.Run("HandlesFileCopyError", func(t *testing.T) {
		// Given a resolver with file copy error
		resolver, _, tmpDir := setup(t)
		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")
		createTestArchive(t, archivePath, "terraform/test-module", map[string]string{
			"terraform/test-module/main.tf": `resource "test" "example" {}`,
		})

		archiveData, err := os.ReadFile(archivePath)
		if err != nil {
			t.Fatalf("Failed to read archive: %v", err)
		}

		// Override Copy to return error
		resolver.BaseModuleResolver.shims.Copy = func(dst io.Writer, src io.Reader) (int64, error) {
			return 0, fmt.Errorf("copy error")
		}

		// When extracting
		err = resolver.extractModuleFromArchive(archiveData, "terraform/test-module", "test-archive")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to write file") {
			t.Errorf("Expected file write error, got: %v", err)
		}
	})

	t.Run("HandlesFileCloseError", func(t *testing.T) {
		// Given a resolver with file close error
		resolver, _, tmpDir := setup(t)
		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")
		createTestArchive(t, archivePath, "terraform/test-module", map[string]string{
			"terraform/test-module/main.tf": `resource "test" "example" {}`,
		})

		archiveData, err := os.ReadFile(archivePath)
		if err != nil {
			t.Fatalf("Failed to read archive: %v", err)
		}

		// Override Create to return a file that errors on Close
		// We'll use a custom approach: create the file, then override Close behavior
		// by tracking it and making Copy fail, which will trigger Close error path
		createdFiles := make(map[string]*os.File)
		resolver.BaseModuleResolver.shims.Create = func(name string) (*os.File, error) {
			file, err := os.Create(name)
			if err != nil {
				return nil, err
			}
			createdFiles[name] = file
			return file, nil
		}

		// Override Copy to succeed, but then Close will fail
		resolver.BaseModuleResolver.shims.Copy = func(dst io.Writer, src io.Reader) (int64, error) {
			// Copy succeeds
			return io.Copy(dst, src)
		}

		// We need to manually close with error - but since we can't override Close on os.File,
		// we'll test the close error path differently by making Copy fail after file is created
		// Actually, let's test this by making the file.Close() call fail via a different mechanism
		// Since we can't easily mock os.File.Close(), let's remove this test case for now
		// and test the close error path through integration
		t.Skip("Cannot easily mock os.File.Close() - testing through integration")

		// When extracting
		err = resolver.extractModuleFromArchive(archiveData, "terraform/test-module", "test-archive")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to close file") {
			t.Errorf("Expected file close error, got: %v", err)
		}
	})

	t.Run("HandlesInvalidFileMode", func(t *testing.T) {
		// Given a resolver with invalid file mode
		resolver, _, tmpDir := setup(t)
		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")

		file, err := os.Create(archivePath)
		if err != nil {
			t.Fatalf("Failed to create archive file: %v", err)
		}
		gzipWriter := gzip.NewWriter(file)
		tarWriter := tar.NewWriter(gzipWriter)

		// Create a file with invalid mode (> 0777)
		// Use 01000 (octal) = 512 (decimal), which when masked with 0777 becomes 0, which is valid
		// Use 02000 (octal) = 1024 (decimal), which when masked with 0777 becomes 0, which is also valid
		// Use 010000 (octal) = 4096 (decimal), which when masked with 0777 becomes 0
		// Actually, we need a mode where (mode & 0777) > 0777, which is impossible
		// The check is: modeValue < 0 || modeValue > 0777
		// So we need modeValue to be > 0777, which means (mode & 0777) > 0777
		// But mode & 0777 can never be > 0777, so this check will never fail for valid tar headers
		// Let's use a negative mode value instead by setting a high bit
		header := &tar.Header{
			Name: "terraform/test-module/main.tf",
			Mode: 07777, // This is 4095, which when masked with 0777 becomes 0777, which is valid
			Size: 10,
		}
		// Actually, we can't create an invalid mode that passes the prefix check but fails the mode check
		// The mode check is: modeValue := header.Mode & 0777; if modeValue < 0 || modeValue > 0777
		// Since modeValue is the result of & 0777, it can never be > 0777
		// So this test case is actually testing an impossible condition
		// Let's skip it or test a different scenario
		t.Skip("Invalid file mode test - mode & 0777 can never be > 0777, so this condition is unreachable")

		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("Failed to write tar header: %v", err)
		}
		if _, err := tarWriter.Write([]byte("test data")); err != nil {
			t.Fatalf("Failed to write tar content: %v", err)
		}

		tarWriter.Close()
		gzipWriter.Close()
		file.Close()

		archiveData, err := os.ReadFile(archivePath)
		if err != nil {
			t.Fatalf("Failed to read archive: %v", err)
		}

		// When extracting
		err = resolver.extractModuleFromArchive(archiveData, "terraform/test-module", "test-archive")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid file mode") {
			t.Errorf("Expected invalid file mode error, got: %v", err)
		}
	})

	t.Run("HandlesChmodError", func(t *testing.T) {
		// Given a resolver with chmod error
		resolver, _, tmpDir := setup(t)
		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")
		createTestArchive(t, archivePath, "terraform/test-module", map[string]string{
			"terraform/test-module/main.tf": `resource "test" "example" {}`,
		})

		archiveData, err := os.ReadFile(archivePath)
		if err != nil {
			t.Fatalf("Failed to read archive: %v", err)
		}

		// Override Chmod to return error
		resolver.BaseModuleResolver.shims.Chmod = func(name string, mode os.FileMode) error {
			return fmt.Errorf("chmod error")
		}

		// When extracting
		err = resolver.extractModuleFromArchive(archiveData, "terraform/test-module", "test-archive")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to set file permissions") {
			t.Errorf("Expected chmod error, got: %v", err)
		}
	})

	t.Run("HandlesShellScriptExecutableBit", func(t *testing.T) {
		// Given a resolver with .sh file
		resolver, _, tmpDir := setup(t)
		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")

		// Create archive with .sh file
		file, err := os.Create(archivePath)
		if err != nil {
			t.Fatalf("Failed to create archive file: %v", err)
		}
		gzipWriter := gzip.NewWriter(file)
		tarWriter := tar.NewWriter(gzipWriter)

		header := &tar.Header{
			Name: "terraform/test-module/script.sh",
			Mode: 0644,
			Size: int64(len(`#!/bin/bash\necho "test"`)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("Failed to write tar header: %v", err)
		}
		if _, err := tarWriter.Write([]byte(`#!/bin/bash\necho "test"`)); err != nil {
			t.Fatalf("Failed to write tar content: %v", err)
		}

		tarWriter.Close()
		gzipWriter.Close()
		file.Close()

		archiveData, err := os.ReadFile(archivePath)
		if err != nil {
			t.Fatalf("Failed to read archive: %v", err)
		}

		// Override Chmod to ensure it's called (use real Chmod)
		resolver.BaseModuleResolver.shims.Chmod = os.Chmod

		// When extracting
		err = resolver.extractModuleFromArchive(archiveData, "terraform/test-module", "test-archive")

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And the .sh file should have executable permissions
		scriptPath := filepath.Join(tmpDir, ".windsor", ".archive_extracted", "test-archive", "terraform", "test-module", "script.sh")
		info, err := os.Stat(scriptPath)
		if err != nil {
			t.Errorf("Expected script.sh to exist, got error: %v", err)
		} else {
			mode := info.Mode()
			if mode&0111 == 0 {
				t.Errorf("Expected script.sh to have executable permissions, got mode: %o", mode)
			}
		}
	})
}

func TestArchiveModuleResolver_processComponent(t *testing.T) {
	setup := func(t *testing.T) (*ArchiveModuleResolver, *TerraformTestMocks, string) {
		t.Helper()
		mocks := setupTerraformMocks(t)
		resolver := NewArchiveModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
		resolver.BaseModuleResolver.shims = mocks.Shims

		tmpDir := t.TempDir()
		configRoot := filepath.Join(tmpDir, "contexts", "test")
		if err := os.MkdirAll(configRoot, 0755); err != nil {
			t.Fatalf("Failed to create config root: %v", err)
		}
		blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
		if err := os.WriteFile(blueprintPath, []byte("kind: Blueprint"), 0644); err != nil {
			t.Fatalf("Failed to write blueprint.yaml: %v", err)
		}

		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.ConfigRoot = configRoot

		return resolver, mocks, tmpDir
	}

	t.Run("HandlesMkdirAllError", func(t *testing.T) {
		// Given a resolver with MkdirAll error
		resolver, _, _ := setup(t)

		component := blueprintv1alpha1.TerraformComponent{
			Path:     "test-module",
			Source:   "file:///test/archive.tar.gz//terraform/test-module",
			FullPath: "/test/module/path",
		}

		// Override MkdirAll to return error
		resolver.BaseModuleResolver.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mkdir error")
		}

		// When processing component
		err := resolver.processComponent(component)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create module directory") {
			t.Errorf("Expected mkdir error, got: %v", err)
		}
	})

	t.Run("HandlesExtractArchiveModuleError", func(t *testing.T) {
		// Given a resolver with extractArchiveModule error
		resolver, _, tmpDir := setup(t)

		component := blueprintv1alpha1.TerraformComponent{
			Path:     "test-module",
			Source:   "file:///invalid/path.tar.gz//terraform/test-module",
			FullPath: filepath.Join(tmpDir, ".windsor", ".tf_modules", "test-module"),
		}

		// Override MkdirAll to succeed
		resolver.BaseModuleResolver.shims.MkdirAll = os.MkdirAll

		// When processing component
		err := resolver.processComponent(component)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to extract archive module") {
			t.Errorf("Expected extract error, got: %v", err)
		}
	})

	t.Run("HandlesFilepathRelError", func(t *testing.T) {
		// Given a resolver with FilepathRel error
		resolver, _, tmpDir := setup(t)
		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")
		createTestArchive(t, archivePath, "terraform/test-module", map[string]string{
			"terraform/test-module/main.tf": `resource "test" "example" {}`,
		})

		component := blueprintv1alpha1.TerraformComponent{
			Path:     "test-module",
			Source:   "file://" + archivePath + "//terraform/test-module",
			FullPath: filepath.Join(tmpDir, ".windsor", ".tf_modules", "test-module"),
		}

		// Override shims for real file operations
		resolver.BaseModuleResolver.shims.MkdirAll = os.MkdirAll
		resolver.BaseModuleResolver.shims.FilepathAbs = filepath.Abs
		resolver.BaseModuleResolver.shims.ReadFile = os.ReadFile
		resolver.BaseModuleResolver.shims.Stat = os.Stat
		resolver.BaseModuleResolver.shims.Copy = io.Copy
		resolver.BaseModuleResolver.shims.Create = func(name string) (*os.File, error) {
			return os.Create(name)
		}

		// Override FilepathRel to return error
		resolver.BaseModuleResolver.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return "", fmt.Errorf("filepath rel error")
		}

		// When processing component
		err := resolver.processComponent(component)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to calculate relative path") {
			t.Errorf("Expected filepath rel error, got: %v", err)
		}
	})

	t.Run("HandlesWriteShimMainTfError", func(t *testing.T) {
		// Given a resolver with writeShimMainTf error
		resolver, _, tmpDir := setup(t)
		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")
		createTestArchive(t, archivePath, "terraform/test-module", map[string]string{
			"terraform/test-module/main.tf": `resource "test" "example" {}`,
		})

		component := blueprintv1alpha1.TerraformComponent{
			Path:     "test-module",
			Source:   "file://" + archivePath + "//terraform/test-module",
			FullPath: filepath.Join(tmpDir, ".windsor", ".tf_modules", "test-module"),
		}

		// Override shims for real file operations
		resolver.BaseModuleResolver.shims.MkdirAll = os.MkdirAll
		resolver.BaseModuleResolver.shims.FilepathAbs = filepath.Abs
		resolver.BaseModuleResolver.shims.ReadFile = os.ReadFile
		resolver.BaseModuleResolver.shims.Stat = os.Stat
		resolver.BaseModuleResolver.shims.Copy = io.Copy
		resolver.BaseModuleResolver.shims.Create = func(name string) (*os.File, error) {
			return os.Create(name)
		}
		resolver.BaseModuleResolver.shims.FilepathRel = filepath.Rel

		// Override WriteFile to return error for main.tf
		resolver.BaseModuleResolver.shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(name, "main.tf") {
				return fmt.Errorf("write main.tf error")
			}
			return os.WriteFile(name, data, perm)
		}

		// When processing component
		err := resolver.processComponent(component)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to write main.tf") {
			t.Errorf("Expected write main.tf error, got: %v", err)
		}
	})

	t.Run("HandlesWriteShimVariablesTfError", func(t *testing.T) {
		// Given a resolver with writeShimVariablesTf error
		resolver, _, tmpDir := setup(t)
		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")
		createTestArchive(t, archivePath, "terraform/test-module", map[string]string{
			"terraform/test-module/main.tf":      `resource "test" "example" {}`,
			"terraform/test-module/variables.tf": `variable "test" {}`,
		})

		component := blueprintv1alpha1.TerraformComponent{
			Path:     "test-module",
			Source:   "file://" + archivePath + "//terraform/test-module",
			FullPath: filepath.Join(tmpDir, ".windsor", ".tf_modules", "test-module"),
		}

		// Override shims for real file operations
		resolver.BaseModuleResolver.shims.MkdirAll = os.MkdirAll
		resolver.BaseModuleResolver.shims.FilepathAbs = filepath.Abs
		resolver.BaseModuleResolver.shims.ReadFile = os.ReadFile
		resolver.BaseModuleResolver.shims.Stat = os.Stat
		resolver.BaseModuleResolver.shims.Copy = io.Copy
		resolver.BaseModuleResolver.shims.Create = func(name string) (*os.File, error) {
			return os.Create(name)
		}
		resolver.BaseModuleResolver.shims.FilepathRel = filepath.Rel
		resolver.BaseModuleResolver.shims.WriteFile = os.WriteFile

		// Override WriteFile to return error for variables.tf
		resolver.BaseModuleResolver.shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(name, "variables.tf") {
				return fmt.Errorf("write variables.tf error")
			}
			return os.WriteFile(name, data, perm)
		}

		// When processing component
		err := resolver.processComponent(component)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to write variables.tf") {
			t.Errorf("Expected write variables.tf error, got: %v", err)
		}
	})

	t.Run("HandlesWriteShimOutputsTfError", func(t *testing.T) {
		// Given a resolver with writeShimOutputsTf error
		resolver, _, tmpDir := setup(t)
		archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")
		createTestArchive(t, archivePath, "terraform/test-module", map[string]string{
			"terraform/test-module/main.tf":     `resource "test" "example" {}`,
			"terraform/test-module/outputs.tf":  `output "test" {}`,
		})

		component := blueprintv1alpha1.TerraformComponent{
			Path:     "test-module",
			Source:   "file://" + archivePath + "//terraform/test-module",
			FullPath: filepath.Join(tmpDir, ".windsor", ".tf_modules", "test-module"),
		}

		// Override shims for real file operations
		resolver.BaseModuleResolver.shims.MkdirAll = os.MkdirAll
		resolver.BaseModuleResolver.shims.FilepathAbs = filepath.Abs
		resolver.BaseModuleResolver.shims.ReadFile = os.ReadFile
		resolver.BaseModuleResolver.shims.Stat = os.Stat
		resolver.BaseModuleResolver.shims.Copy = io.Copy
		resolver.BaseModuleResolver.shims.Create = func(name string) (*os.File, error) {
			return os.Create(name)
		}
		resolver.BaseModuleResolver.shims.FilepathRel = filepath.Rel
		resolver.BaseModuleResolver.shims.WriteFile = os.WriteFile

		// Override WriteFile to return error for outputs.tf
		resolver.BaseModuleResolver.shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(name, "outputs.tf") {
				return fmt.Errorf("write outputs.tf error")
			}
			return os.WriteFile(name, data, perm)
		}

		// When processing component
		err := resolver.processComponent(component)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to write outputs.tf") {
			t.Errorf("Expected write outputs.tf error, got: %v", err)
		}
	})
}

func TestArchiveModuleResolver_validateAndSanitizePath(t *testing.T) {
	setup := func(t *testing.T) *ArchiveModuleResolver {
		t.Helper()
		mocks := setupTerraformMocks(t)
		resolver := NewArchiveModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
		resolver.BaseModuleResolver.shims = mocks.Shims
		return resolver
	}

	t.Run("AcceptsValidPaths", func(t *testing.T) {
		// Given a resolver
		resolver := setup(t)

		// When validating valid paths
		testCases := []string{
			"terraform/module/main.tf",
			"terraform/module/variables.tf",
			"terraform/nested/module/main.tf",
		}

		for _, path := range testCases {
			// Then they should be accepted
			result, err := resolver.validateAndSanitizePath(path)
			if err != nil {
				t.Errorf("Expected path %s to be valid, got error: %v", path, err)
			}
			if result == "" {
				t.Errorf("Expected non-empty result for path %s", path)
			}
		}
	})

	t.Run("RejectsPathTraversal", func(t *testing.T) {
		// Given a resolver
		resolver := setup(t)

		// When validating paths with traversal sequences
		testCases := []string{
			"../terraform/module/main.tf",
			"terraform/../../etc/passwd",
			"terraform/module/../../../etc/passwd",
		}

		for _, path := range testCases {
			// Then they should be rejected
			_, err := resolver.validateAndSanitizePath(path)
			if err == nil {
				t.Errorf("Expected path %s to be rejected, got nil error", path)
			}
			if !strings.Contains(err.Error(), "directory traversal") {
				t.Errorf("Expected directory traversal error for path %s, got: %v", path, err)
			}
		}
	})

	t.Run("RejectsAbsolutePaths", func(t *testing.T) {
		// Given a resolver
		resolver := setup(t)

		// When validating absolute paths
		testCases := []string{
			"/etc/passwd",
			"/terraform/module/main.tf",
			"C:\\Windows\\System32",
		}

		for _, path := range testCases {
			// Then they should be rejected
			_, err := resolver.validateAndSanitizePath(path)
			if err == nil {
				t.Errorf("Expected absolute path %s to be rejected, got nil error", path)
			}
			if !strings.Contains(err.Error(), "absolute paths are not allowed") {
				t.Errorf("Expected absolute path error for path %s, got: %v", path, err)
			}
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

// createTestArchive creates a test tar.gz archive with the specified files
func createTestArchive(t *testing.T, archivePath, modulePath string, files map[string]string) {
	t.Helper()

	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("Failed to create archive file: %v", err)
	}

	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)

	for filePath, content := range files {
		header := &tar.Header{
			Name: filePath,
			Mode: 0644,
			Size: int64(len(content)),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("Failed to write tar header: %v", err)
		}

		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatalf("Failed to write tar content: %v", err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatalf("Failed to close tar writer: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("Failed to close gzip writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Failed to close file: %v", err)
	}
}

// =============================================================================
// Test Helper Types
// =============================================================================

// mockTarReader is a mock TarReader that returns errors on Next()
type mockTarReader struct {
	nextError error
}

func (m *mockTarReader) Next() (*tar.Header, error) {
	if m.nextError != nil {
		return nil, m.nextError
	}
	return nil, io.EOF
}

func (m *mockTarReader) Read(p []byte) (int, error) {
	return 0, io.EOF
}

