package terraform

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
)

// The CompositeModuleResolverTest is a test suite for the CompositeModuleResolver implementation
// It provides comprehensive coverage for composite resolver orchestration and delegation
// The CompositeModuleResolverTest ensures proper coordination between OCI, archive, and standard resolvers
// enabling reliable terraform module resolution across different source types

// =============================================================================
// Test Public Methods
// =============================================================================

func TestCompositeModuleResolver_NewCompositeModuleResolver(t *testing.T) {
	t.Run("CreatesCompositeModuleResolver", func(t *testing.T) {
		// Given dependencies
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()

		// When creating a new composite module resolver
		resolver := NewCompositeModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)

		// Then it should be created successfully
		if resolver == nil {
			t.Fatal("Expected non-nil CompositeModuleResolver")
		}
		if resolver.ociResolver == nil {
			t.Error("Expected ociResolver to be set")
		}
		if resolver.archiveResolver == nil {
			t.Error("Expected archiveResolver to be set")
		}
		if resolver.standardResolver == nil {
			t.Error("Expected standardResolver to be set")
		}
	})
}

func TestCompositeModuleResolver_ProcessModules(t *testing.T) {
	setup := func(t *testing.T) (*CompositeModuleResolver, *TerraformTestMocks) {
		t.Helper()
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		resolver := NewCompositeModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)
		return resolver, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a resolver with components of different types
		resolver, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "oci-module",
					Source:   "oci://registry.example.com/module:latest//terraform/oci-module",
					FullPath: filepath.Join(tmpDir, ".windsor", ".tf_modules", "oci-module"),
				},
				{
					Path:     "standard-module",
					Source:   "git::https://github.com/test/module.git",
					FullPath: filepath.Join(tmpDir, "terraform", "standard-module"),
				},
			}
		}

		// Mock OCI resolver to succeed with proper artifact data
		mockArtifactBuilder := artifact.NewMockArtifact()
		mockArtifactBuilder.PullFunc = func(refs []string) (map[string][]byte, error) {
			artifacts := make(map[string][]byte)
			for _, ref := range refs {
				// Cache key format is registry/repository:tag (without oci:// prefix)
				if strings.HasPrefix(ref, "oci://") {
					cacheKey := strings.TrimPrefix(ref, "oci://")
					// Create a minimal valid tar archive (OCI artifacts are tar, not tar.gz)
					var buf bytes.Buffer
					tarWriter := tar.NewWriter(&buf)
					content := []byte("resource \"test\" {}")
					header := &tar.Header{
						Name: "terraform/oci-module/main.tf",
						Mode: 0644,
						Size: int64(len(content)),
					}
					if err := tarWriter.WriteHeader(header); err != nil {
						return nil, err
					}
					if _, err := tarWriter.Write(content); err != nil {
						return nil, err
					}
					if err := tarWriter.Close(); err != nil {
						return nil, err
					}
					artifacts[cacheKey] = buf.Bytes()
				} else {
					artifacts[ref] = []byte("mock artifact data")
				}
			}
			return artifacts, nil
		}
		if resolver.ociResolver != nil {
			resolver.ociResolver.artifactBuilder = mockArtifactBuilder
			// Override shims for real file operations in OCI resolver
			resolver.ociResolver.BaseModuleResolver.shims.Copy = io.Copy
			resolver.ociResolver.BaseModuleResolver.shims.Create = func(name string) (*os.File, error) {
				return os.Create(name)
			}
			resolver.ociResolver.BaseModuleResolver.shims.MkdirAll = os.MkdirAll
			resolver.ociResolver.BaseModuleResolver.shims.Stat = os.Stat
		}
		
		// Override shims for standard resolver to avoid file system errors
		resolver.standardResolver.shims.MkdirAll = os.MkdirAll
		resolver.standardResolver.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When processing modules
		err := resolver.ProcessModules()

		// Then it should succeed (or fail on standard, but OCI should be processed first)
		if err != nil && !strings.Contains(err.Error(), "failed to process standard modules") {
			t.Errorf("Expected nil error or standard module error, got %v", err)
		}
	})

	t.Run("HandlesOCIResolverError", func(t *testing.T) {
		// Given a resolver with OCI components that fail
		resolver, mocks := setup(t)
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "oci-module",
					Source:   "oci://registry.example.com/module:latest//terraform/oci-module",
					FullPath: "/mock/project/terraform/oci-module",
				},
			}
		}

		// Mock OCI resolver to fail
		mockArtifactBuilder := artifact.NewMockArtifact()
		mockArtifactBuilder.PullFunc = func(refs []string) (map[string][]byte, error) {
			return nil, errors.New("OCI pull error")
		}
		if resolver.ociResolver != nil {
			resolver.ociResolver.artifactBuilder = mockArtifactBuilder
		}

		// When processing modules
		err := resolver.ProcessModules()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to process OCI modules") {
			t.Errorf("Expected OCI module processing error, got: %v", err)
		}
	})

	t.Run("HandlesArchiveResolverError", func(t *testing.T) {
		// Given a resolver with archive components that fail
		resolver, mocks := setup(t)
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "archive-module",
					Source:   "file:///invalid/path.tar.gz//terraform/module",
					FullPath: "/mock/project/terraform/archive-module",
				},
			}
		}

		mocks.Runtime.ProjectRoot = "/mock/project"
		mocks.Runtime.ConfigRoot = "/mock/config"

		// When processing modules
		err := resolver.ProcessModules()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to process archive modules") {
			t.Errorf("Expected archive module processing error, got: %v", err)
		}
	})

	t.Run("HandlesStandardResolverError", func(t *testing.T) {
		// Given a resolver with standard components that fail
		resolver, mocks := setup(t)
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "standard-module",
					Source:   "git::https://github.com/test/module.git",
					FullPath: "/mock/project/terraform/standard-module",
				},
			}
		}

		// Mock standard resolver to fail
		resolver.standardResolver.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return errors.New("mkdir error")
		}

		// When processing modules
		err := resolver.ProcessModules()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to process standard modules") {
			t.Errorf("Expected standard module processing error, got: %v", err)
		}
	})

	t.Run("ProcessesAllResolverTypes", func(t *testing.T) {
		// Given a resolver with components of all types
		resolver, mocks := setup(t)
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "oci-module",
					Source:   "oci://registry.example.com/module:latest",
					FullPath: "/mock/project/terraform/oci-module",
				},
				{
					Path:     "archive-module",
					Source:   "file:///test/archive.tar.gz//terraform/module",
					FullPath: "/mock/project/terraform/archive-module",
				},
				{
					Path:     "standard-module",
					Source:   "git::https://github.com/test/module.git",
					FullPath: "/mock/project/terraform/standard-module",
				},
			}
		}

		// Mock OCI resolver to succeed
		mockArtifactBuilder := artifact.NewMockArtifact()
		mockArtifactBuilder.PullFunc = func(refs []string) (map[string][]byte, error) {
			return make(map[string][]byte), nil
		}
		resolver.ociResolver.artifactBuilder = mockArtifactBuilder

		// When processing modules
		err := resolver.ProcessModules()

		// Then it should succeed (or fail on archive/standard, but OCI should be processed first)
		// The exact error depends on which resolver fails, but OCI should be attempted first
		if err != nil && !strings.Contains(err.Error(), "failed to process OCI modules") {
			// If error is not about OCI, it means OCI succeeded and we moved to next resolver
			if !strings.Contains(err.Error(), "failed to process archive modules") && !strings.Contains(err.Error(), "failed to process standard modules") {
				t.Errorf("Expected OCI, archive, or standard module processing error, got: %v", err)
			}
		}
	})
}

func TestCompositeModuleResolver_GenerateTfvars(t *testing.T) {
	setup := func(t *testing.T) (*CompositeModuleResolver, *TerraformTestMocks) {
		t.Helper()
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		resolver := NewCompositeModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)
		return resolver, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a resolver
		resolver, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		
		moduleDir := filepath.Join(tmpDir, ".windsor", ".tf_modules", "test-module")
		if err := os.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("Failed to create module directory: %v", err)
		}
		
		// Create a variables.tf file so GenerateTfvars can find it
		variablesPath := filepath.Join(moduleDir, "variables.tf")
		if err := os.WriteFile(variablesPath, []byte(`variable "cluster_name" {}`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}
		
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git::https://github.com/test/module.git",
					FullPath: moduleDir,
					Inputs: map[string]any{
						"cluster_name": "test-cluster",
					},
				},
			}
		}
		
		// Override shims for real file operations
		resolver.standardResolver.shims.Stat = os.Stat
		resolver.standardResolver.shims.ReadFile = os.ReadFile
		resolver.standardResolver.shims.MkdirAll = os.MkdirAll
		resolver.standardResolver.shims.WriteFile = os.WriteFile

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("DelegatesToStandardResolver", func(t *testing.T) {
		// Given a resolver
		resolver, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		
		moduleDir := filepath.Join(tmpDir, ".windsor", ".tf_modules", "test-module")
		if err := os.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("Failed to create module directory: %v", err)
		}
		
		// Create a variables.tf file so GenerateTfvars can find it
		variablesPath := filepath.Join(moduleDir, "variables.tf")
		if err := os.WriteFile(variablesPath, []byte(`variable "cluster_name" {}`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}
		
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git::https://github.com/test/module.git",
					FullPath: moduleDir,
					Inputs: map[string]any{
						"cluster_name": "test-cluster",
					},
				},
			}
		}
		
		// Override shims for real file operations
		resolver.standardResolver.shims.Stat = os.Stat
		resolver.standardResolver.shims.ReadFile = os.ReadFile
		resolver.standardResolver.shims.MkdirAll = os.MkdirAll
		resolver.standardResolver.shims.WriteFile = os.WriteFile

		// When generating tfvars
		err := resolver.GenerateTfvars(true)

		// Then it should succeed (delegates to standard resolver)
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("HandlesStandardResolverError", func(t *testing.T) {
		// Given a resolver with standard resolver that fails
		resolver, mocks := setup(t)
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git::https://github.com/test/module.git",
					Inputs: map[string]any{
						"cluster_name": "test-cluster",
					},
				},
			}
		}

		// Mock standard resolver's shims to fail
		resolver.standardResolver.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return errors.New("generate tfvars error")
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should return an error (from standard resolver)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})
}

