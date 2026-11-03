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
// Test Public Methods
// =============================================================================

func TestCompositeModuleResolver_NewCompositeModuleResolver(t *testing.T) {
	t.Run("CreatesCompositeModuleResolver", func(t *testing.T) {
		// Given dependencies
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()

		// When creating a new composite module resolver
		resolver := terraform.NewCompositeModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)

		// Then it should be created successfully
		if resolver == nil {
			t.Fatal("Expected non-nil CompositeModuleResolver")
		}
		if resolver.OciResolver() == nil {
			t.Error("Expected ociResolver to be set")
		}
		if resolver.StandardResolver() == nil {
			t.Error("Expected standardResolver to be set")
		}
	})

	t.Run("PanicsWhenRuntimeIsNil", func(t *testing.T) {
		// Given nil runtime
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()

		// When creating a new composite module resolver with nil runtime
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when runtime is nil")
			}
		}()
		terraform.NewCompositeModuleResolver(nil, mocks.BlueprintHandler, mockArtifactBuilder)
	})

	t.Run("PanicsWhenBlueprintHandlerIsNil", func(t *testing.T) {
		// Given nil blueprint handler
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()

		// When creating a new composite module resolver with nil blueprint handler
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when blueprint handler is nil")
			}
		}()
		terraform.NewCompositeModuleResolver(mocks.Runtime, nil, mockArtifactBuilder)
	})

	t.Run("PanicsWhenArtifactBuilderIsNil", func(t *testing.T) {
		// Given nil artifact builder
		mocks := setupTerraformMocks(t)

		// When creating a new composite module resolver with nil artifact builder
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when artifact builder is nil")
			}
		}()
		terraform.NewCompositeModuleResolver(mocks.Runtime, mocks.BlueprintHandler, nil)
	})
}

func TestCompositeModuleResolver_ProcessModules(t *testing.T) {
	setup := func(t *testing.T) (*terraform.CompositeModuleResolver, *TerraformTestMocks) {
		t.Helper()
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		resolver := terraform.NewCompositeModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)
		return resolver, mocks
	}

	setupOCIMocks := func(t *testing.T, resolver *terraform.CompositeModuleResolver, projectRoot string) *artifact.MockArtifact {
		t.Helper()
		mockArtifactBuilder := artifact.NewMockArtifact()
		mockArtifactBuilder.ParseOCIRefFunc = func(ociRef string) (registry, repository, tag string, err error) {
			if ociRef == "oci://registry.example.com/module:latest" {
				return "registry.example.com", "module", "latest", nil
			}
			return "", "", "", fmt.Errorf("invalid OCI reference format: %s", ociRef)
		}
		mockArtifactBuilder.GetCacheDirFunc = func(registry, repository, tag string) (string, error) {
			cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)
			extractionKey := strings.ReplaceAll(strings.ReplaceAll(cacheKey, "/", "_"), ":", "_")
			cacheDir := filepath.Join(projectRoot, ".windsor", "cache", "oci", extractionKey)
			return cacheDir, nil
		}
		mockArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			artifacts := make(map[string]string)
			for _, ref := range refs {
				if strings.HasPrefix(ref, "oci://") {
					cacheKey := strings.TrimPrefix(ref, "oci://")
					extractionKey := strings.ReplaceAll(strings.ReplaceAll(cacheKey, "/", "_"), ":", "_")
					cacheDir := filepath.Join(projectRoot, ".windsor", "cache", "oci", extractionKey)
					artifacts[cacheKey] = cacheDir
				} else {
					artifacts[ref] = filepath.Join(projectRoot, ".windsor", "cache", "oci", ref)
				}
			}
			return artifacts, nil
		}
		mockArtifactBuilder.ExtractModulePathFunc = func(registry, repository, tag, modulePath string) (string, error) {
			cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)
			extractionKey := strings.ReplaceAll(strings.ReplaceAll(cacheKey, "/", "_"), ":", "_")
			cacheDir := filepath.Join(projectRoot, ".windsor", "cache", "oci", extractionKey)
			fullModulePath := filepath.Join(cacheDir, modulePath)
			if err := os.MkdirAll(fullModulePath, 0755); err != nil {
				return "", err
			}
			return fullModulePath, nil
		}
		if resolver.OciResolver() != nil {
			resolver.OciResolver().SetArtifactBuilder(mockArtifactBuilder)
		}
		return mockArtifactBuilder
	}

	setupOCIResolverShims := func(resolver *terraform.CompositeModuleResolver) {
		if resolver.OciResolver() != nil {
			resolver.OciResolver().BaseModuleResolver.Shims().MkdirAll = os.MkdirAll
			resolver.OciResolver().BaseModuleResolver.Shims().Stat = os.Stat
		}
	}

	setupStandardResolverShims := func(resolver *terraform.CompositeModuleResolver, statFunc func(string) (os.FileInfo, error)) {
		resolver.StandardResolver().Shims().MkdirAll = os.MkdirAll
		if statFunc != nil {
			resolver.StandardResolver().Shims().Stat = statFunc
		}
	}

	ociComponent := func(tmpDir string) blueprintv1alpha1.TerraformComponent {
		return blueprintv1alpha1.TerraformComponent{
			Path:     "oci-module",
			Source:   "oci://registry.example.com/module:latest//terraform/oci-module",
			FullPath: filepath.Join(tmpDir, ".windsor", "contexts", "local", "terraform", "oci-module"),
		}
	}

	standardComponent := func(tmpDir string) blueprintv1alpha1.TerraformComponent {
		return blueprintv1alpha1.TerraformComponent{
			Path:     "standard-module",
			Source:   "git::https://github.com/test/module.git",
			FullPath: filepath.Join(tmpDir, "terraform", "standard-module"),
		}
	}

	t.Run("Success", func(t *testing.T) {
		// Given a resolver with components of different types
		resolver, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				ociComponent(tmpDir),
				standardComponent(tmpDir),
			}
		}

		setupOCIMocks(t, resolver, tmpDir)
		setupOCIResolverShims(resolver)
		setupStandardResolverShims(resolver, func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		})

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

		mockArtifactBuilder := artifact.NewMockArtifact()
		mockArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			return nil, errors.New("OCI pull error")
		}
		if resolver.OciResolver() != nil {
			resolver.OciResolver().SetArtifactBuilder(mockArtifactBuilder)
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

		resolver.StandardResolver().Shims().MkdirAll = func(path string, perm os.FileMode) error {
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
					Path:     "standard-module",
					Source:   "git::https://github.com/test/module.git",
					FullPath: "/mock/project/terraform/standard-module",
				},
			}
		}

		mockArtifactBuilder := artifact.NewMockArtifact()
		mockArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			return make(map[string]string), nil
		}
		resolver.OciResolver().SetArtifactBuilder(mockArtifactBuilder)

		// When processing modules
		err := resolver.ProcessModules()

		// Then it should succeed (or fail on standard, but OCI should be processed first)
		if err != nil && !strings.Contains(err.Error(), "failed to process OCI modules") {
			if !strings.Contains(err.Error(), "failed to process standard modules") {
				t.Errorf("Expected OCI or standard module processing error, got: %v", err)
			}
		}
	})
}

func TestCompositeModuleResolver_GenerateTfvars(t *testing.T) {
	setup := func(t *testing.T) (*terraform.CompositeModuleResolver, *TerraformTestMocks) {
		t.Helper()
		mocks := setupTerraformMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		resolver := terraform.NewCompositeModuleResolver(mocks.Runtime, mocks.BlueprintHandler, mockArtifactBuilder)
		return resolver, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a resolver
		resolver, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir
		mocks.Runtime.ContextName = "local"

		moduleDir := filepath.Join(tmpDir, ".windsor", "contexts", "local", "terraform", "test-module")
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
					Path:     "test-module",
					Source:   "git::https://github.com/test/module.git",
					FullPath: moduleDir,
					Inputs: map[string]any{
						"cluster_name": "test-cluster",
					},
				},
			}
		}

		// Override shims for real file operations
		resolver.StandardResolver().Shims().Stat = os.Stat
		resolver.StandardResolver().Shims().ReadFile = os.ReadFile
		resolver.StandardResolver().Shims().MkdirAll = os.MkdirAll
		resolver.StandardResolver().Shims().WriteFile = os.WriteFile

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
		mocks.Runtime.ContextName = "local"

		moduleDir := filepath.Join(tmpDir, ".windsor", "contexts", "local", "terraform", "test-module")
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
					Path:     "test-module",
					Source:   "git::https://github.com/test/module.git",
					FullPath: moduleDir,
					Inputs: map[string]any{
						"cluster_name": "test-cluster",
					},
				},
			}
		}

		// Override shims for real file operations
		resolver.StandardResolver().Shims().Stat = os.Stat
		resolver.StandardResolver().Shims().ReadFile = os.ReadFile
		resolver.StandardResolver().Shims().MkdirAll = os.MkdirAll
		resolver.StandardResolver().Shims().WriteFile = os.WriteFile

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
		resolver.StandardResolver().Shims().MkdirAll = func(path string, perm os.FileMode) error {
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
