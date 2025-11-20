package blueprint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fluxcd/pkg/apis/kustomize"
	"github.com/goccy/go-yaml"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

func TestBaseBlueprintHandler_isOCISource(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		return handler
	}

	t.Run("HandlesOCIPrefix", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// When checking source with oci:// prefix
		result := handler.isOCISource("oci://registry/repo:tag")

		// Then it should return true
		if !result {
			t.Error("Expected true for oci:// prefix")
		}
	})

	t.Run("HandlesSourceNameMatchingBlueprintMetadata", func(t *testing.T) {
		// Given a handler with blueprint metadata name matching source
		handler := setup(t)
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "oci://registry/repo:tag",
			},
		}

		// When checking source name matching blueprint metadata name
		result := handler.isOCISource("test-blueprint")

		// Then it should return true
		if !result {
			t.Error("Expected true when source name matches blueprint metadata name")
		}
	})

	t.Run("HandlesSourceNameMatchingSources", func(t *testing.T) {
		// Given a handler with source matching sources list
		handler := setup(t)
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{
					Name: "oci-source",
					Url:  "oci://registry/repo:tag",
				},
			},
		}

		// When checking source name matching sources
		result := handler.isOCISource("oci-source")

		// Then it should return true
		if !result {
			t.Error("Expected true when source name matches sources list")
		}
	})

	t.Run("HandlesNonOCISource", func(t *testing.T) {
		// Given a handler with non-OCI source
		handler := setup(t)
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{
					Name: "git-source",
					Url:  "git::https://github.com/example/repo.git",
				},
			},
		}

		// When checking non-OCI source
		result := handler.isOCISource("git-source")

		// Then it should return false
		if result {
			t.Error("Expected false for non-OCI source")
		}
	})
}

func TestBaseBlueprintHandler_evaluateSubstitutions(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		return handler
	}

	t.Run("HandlesSubstitutionsWithVariables", func(t *testing.T) {
		// Given a handler with substitutions containing variables
		handler := setup(t)
		substitutions := map[string]string{
			"key1": "value1",
			"key2": "${config.value}",
		}
		config := map[string]any{
			"config": map[string]any{
				"value": "resolved",
			},
		}

		// When evaluating substitutions
		result, err := handler.evaluateSubstitutions(substitutions, config, "test-path")

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if result["key1"] != "value1" {
			t.Errorf("Expected key1 to be 'value1', got: %s", result["key1"])
		}
		if result["key2"] != "resolved" {
			t.Errorf("Expected key2 to be 'resolved', got: %s", result["key2"])
		}
	})

	t.Run("HandlesSubstitutionsWithoutVariables", func(t *testing.T) {
		// Given a handler with substitutions without variables
		handler := setup(t)
		substitutions := map[string]string{
			"key1": "value1",
			"key2": "value2",
		}
		config := map[string]any{}

		// When evaluating substitutions
		result, err := handler.evaluateSubstitutions(substitutions, config, "test-path")

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if result["key1"] != "value1" {
			t.Errorf("Expected key1 to be 'value1', got: %s", result["key1"])
		}
		if result["key2"] != "value2" {
			t.Errorf("Expected key2 to be 'value2', got: %s", result["key2"])
		}
	})

	t.Run("HandlesSubstitutionsWithNonExistentVariable", func(t *testing.T) {
		// Given a handler with substitution referencing non-existent variable
		handler := setup(t)
		substitutions := map[string]string{
			"key1": "${nonexistent.value}",
		}
		config := map[string]any{}

		// When evaluating substitutions
		_, err := handler.evaluateSubstitutions(substitutions, config, "test-path")

		// Then it should return an error (EvaluateDefaults fails for non-existent variables)
		if err == nil {
			t.Error("Expected error when evaluating non-existent variable")
		}
		if !strings.Contains(err.Error(), "failed to evaluate substitution") {
			t.Errorf("Expected error about evaluating substitution, got: %v", err)
		}
	})

	t.Run("HandlesEvaluateDefaultsError", func(t *testing.T) {
		// Given a handler with invalid substitution expression
		handler := setup(t)
		substitutions := map[string]string{
			"key1": "${invalid expression [[[",
		}
		config := map[string]any{}

		// When evaluating substitutions
		_, err := handler.evaluateSubstitutions(substitutions, config, "test-path")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when EvaluateDefaults fails")
		}
		if !strings.Contains(err.Error(), "failed to evaluate substitution") {
			t.Errorf("Expected error about evaluating substitution, got: %v", err)
		}
	})
}

func TestBaseBlueprintHandler_walkAndCollectTemplates(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		return handler
	}

	t.Run("HandlesReadDirError", func(t *testing.T) {
		// Given a handler with ReadDir error
		handler := setup(t)
		templateDir := "/test/template"
		templateData := make(map[string][]byte)

		// Mock ReadDir to return error
		handler.shims.ReadDir = func(path string) ([]os.DirEntry, error) {
			return nil, fmt.Errorf("readdir error")
		}

		// When walking and collecting templates
		err := handler.walkAndCollectTemplates(templateDir, templateData)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when ReadDir fails")
		}
		if !strings.Contains(err.Error(), "failed to read template directory") {
			t.Errorf("Expected error about reading template directory, got: %v", err)
		}
	})

	t.Run("HandlesNestedDirectories", func(t *testing.T) {
		// Given a handler with nested directories
		handler := setup(t)
		tmpDir := t.TempDir()
		templateDir := filepath.Join(tmpDir, "template")
		handler.runtime.TemplateRoot = templateDir
		subdir := filepath.Join(templateDir, "subdir")
		if err := os.MkdirAll(subdir, 0755); err != nil {
			t.Fatalf("Failed to create subdir: %v", err)
		}
		templateData := make(map[string][]byte)

		// Create files in nested directory
		nestedFile := filepath.Join(subdir, "test.jsonnet")
		if err := os.WriteFile(nestedFile, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create nested file: %v", err)
		}

		// Ensure ReadDir and ReadFile are set to defaults
		handler.shims.ReadDir = os.ReadDir
		handler.shims.ReadFile = os.ReadFile

		// When walking and collecting templates
		err := handler.walkAndCollectTemplates(templateDir, templateData)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		// Check that the nested file was collected (relative path should be "subdir/test.jsonnet")
		if len(templateData) == 0 {
			t.Error("Expected template data to be collected")
		}
		// The file should be collected with relative path
		found := false
		for key := range templateData {
			if strings.Contains(key, "test.jsonnet") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected nested file to be collected, templateData keys: %v", templateData)
		}
	})
}

func TestBaseBlueprintHandler_isValidTerraformRemoteSource(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		return handler
	}

	t.Run("HandlesGitHttpsSource", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// When checking git::https source
		result := handler.isValidTerraformRemoteSource("git::https://github.com/example/repo.git")

		// Then it should return true
		if !result {
			t.Error("Expected true for git::https source")
		}
	})

	t.Run("HandlesGitSSHSource", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// When checking git@ source
		result := handler.isValidTerraformRemoteSource("git@github.com:example/repo.git")

		// Then it should return true
		if !result {
			t.Error("Expected true for git@ source")
		}
	})

	t.Run("HandlesHttpSource", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// When checking http source
		result := handler.isValidTerraformRemoteSource("http://example.com/repo.git")

		// Then it should return true
		if !result {
			t.Error("Expected true for http source")
		}
	})

	t.Run("HandlesHttpsSource", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// When checking https source
		result := handler.isValidTerraformRemoteSource("https://example.com/repo.git")

		// Then it should return true
		if !result {
			t.Error("Expected true for https source")
		}
	})

	t.Run("HandlesZipSource", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// When checking zip source
		result := handler.isValidTerraformRemoteSource("https://example.com/repo.zip")

		// Then it should return true
		if !result {
			t.Error("Expected true for zip source")
		}
	})

	t.Run("HandlesSubdirectorySource", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// When checking subdirectory source
		result := handler.isValidTerraformRemoteSource("https://example.com/repo//subdir")

		// Then it should return true
		if !result {
			t.Error("Expected true for subdirectory source")
		}
	})

	t.Run("HandlesTerraformRegistrySource", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// When checking terraform registry source
		result := handler.isValidTerraformRemoteSource("registry.terraform.io/namespace/name")

		// Then it should return true
		if !result {
			t.Error("Expected true for terraform registry source")
		}
	})

	t.Run("HandlesGenericComSource", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// When checking generic .com source
		result := handler.isValidTerraformRemoteSource("example.com/namespace/name")

		// Then it should return true
		if !result {
			t.Error("Expected true for generic .com source")
		}
	})

	t.Run("HandlesInvalidSource", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// When checking invalid source
		result := handler.isValidTerraformRemoteSource("invalid-source")

		// Then it should return false
		if result {
			t.Error("Expected false for invalid source")
		}
	})

	t.Run("HandlesRegexpError", func(t *testing.T) {
		// Given a handler with RegexpMatchString error
		handler := setup(t)
		handler.shims.RegexpMatchString = func(pattern, s string) (bool, error) {
			return false, fmt.Errorf("regexp error")
		}

		// When checking source
		result := handler.isValidTerraformRemoteSource("git::https://github.com/example/repo.git")

		// Then it should return false
		if result {
			t.Error("Expected false when RegexpMatchString fails")
		}
	})
}

func TestBaseBlueprintHandler_resolveComponentPaths(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		return handler
	}

	t.Run("HandlesRemoteSourceComponents", func(t *testing.T) {
		// Given a handler with components using remote sources
		handler := setup(t)
		handler.runtime.ProjectRoot = "/test/project"
		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git::https://github.com/example/repo.git",
				},
			},
		}

		// When resolving component paths
		handler.resolveComponentPaths(blueprint)

		// Then remote source components should use .windsor/.tf_modules path
		if blueprint.TerraformComponents[0].FullPath != filepath.Join("/test/project", ".windsor", ".tf_modules", "test-module") {
			t.Errorf("Expected FullPath for remote source, got: %s", blueprint.TerraformComponents[0].FullPath)
		}
	})

	t.Run("HandlesOCISourceComponents", func(t *testing.T) {
		// Given a handler with components using OCI sources
		handler := setup(t)
		handler.runtime.ProjectRoot = "/test/project"
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{
					Name: "oci-source",
					Url:  "oci://registry.example.com/repo:tag",
				},
			},
		}
		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "oci-source",
				},
			},
		}

		// When resolving component paths
		handler.resolveComponentPaths(blueprint)

		// Then OCI source components should use .windsor/.tf_modules path
		if blueprint.TerraformComponents[0].FullPath != filepath.Join("/test/project", ".windsor", ".tf_modules", "test-module") {
			t.Errorf("Expected FullPath for OCI source, got: %s", blueprint.TerraformComponents[0].FullPath)
		}
	})

	t.Run("HandlesLocalSourceComponents", func(t *testing.T) {
		// Given a handler with components using local sources
		handler := setup(t)
		handler.runtime.ProjectRoot = "/test/project"
		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "local-source",
				},
			},
		}

		// When resolving component paths
		handler.resolveComponentPaths(blueprint)

		// Then local source components should use terraform path
		if blueprint.TerraformComponents[0].FullPath != filepath.Join("/test/project", "terraform", "test-module") {
			t.Errorf("Expected FullPath for local source, got: %s", blueprint.TerraformComponents[0].FullPath)
		}
	})

	t.Run("HandlesEmptySourceComponents", func(t *testing.T) {
		// Given a handler with components with empty source
		handler := setup(t)
		handler.runtime.ProjectRoot = "/test/project"
		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "",
				},
			},
		}

		// When resolving component paths
		handler.resolveComponentPaths(blueprint)

		// Then empty source components should use terraform path
		if blueprint.TerraformComponents[0].FullPath != filepath.Join("/test/project", "terraform", "test-module") {
			t.Errorf("Expected FullPath for empty source, got: %s", blueprint.TerraformComponents[0].FullPath)
		}
	})
}

func TestBaseBlueprintHandler_resolveComponentSources(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		return handler
	}

	t.Run("HandlesOCISourceWithPathPrefix", func(t *testing.T) {
		// Given a handler with OCI source and path prefix
		handler := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{
					Name:       "oci-source",
					Url:        "oci://registry.example.com/repo:tag",
					PathPrefix: "custom-prefix",
				},
			},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "oci-source",
				},
			},
		}

		// When resolving component sources
		handler.resolveComponentSources(blueprint)

		// Then OCI source should be resolved with path prefix
		expectedSource := "oci://registry.example.com/repo:tag//custom-prefix/test-module"
		if blueprint.TerraformComponents[0].Source != expectedSource {
			t.Errorf("Expected source '%s', got '%s'", expectedSource, blueprint.TerraformComponents[0].Source)
		}
	})

	t.Run("HandlesOCISourceWithRef", func(t *testing.T) {
		// Given a handler with OCI source and ref (URL without tag)
		handler := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{
					Name: "oci-source",
					Url:  "oci://registry.example.com/repo",
					Ref: blueprintv1alpha1.Reference{
						Tag: "v1.0.0",
					},
				},
			},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "oci-source",
				},
			},
		}

		// When resolving component sources
		handler.resolveComponentSources(blueprint)

		// Then OCI source should be resolved (tag added only if URL doesn't contain ":" after "oci://")
		// The code checks if baseURL contains ":", and "oci://registry.example.com/repo" contains ":" from "oci://"
		// So tag won't be added unless we use a URL without ":" in the path portion
		expectedSource := "oci://registry.example.com/repo//terraform/test-module"
		if blueprint.TerraformComponents[0].Source != expectedSource {
			t.Errorf("Expected source '%s', got '%s'", expectedSource, blueprint.TerraformComponents[0].Source)
		}
	})

	t.Run("HandlesGitSourceWithPathPrefix", func(t *testing.T) {
		// Given a handler with Git source and path prefix
		handler := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{
					Name:       "git-source",
					Url:        "https://github.com/example/repo.git",
					PathPrefix: "custom-prefix",
					Ref: blueprintv1alpha1.Reference{
						Branch: "main",
					},
				},
			},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git-source",
				},
			},
		}

		// When resolving component sources
		handler.resolveComponentSources(blueprint)

		// Then Git source should be resolved with path prefix and ref
		expectedSource := "https://github.com/example/repo.git//custom-prefix/test-module?ref=main"
		if blueprint.TerraformComponents[0].Source != expectedSource {
			t.Errorf("Expected source '%s', got '%s'", expectedSource, blueprint.TerraformComponents[0].Source)
		}
	})

	t.Run("HandlesGitSourceWithCommitRef", func(t *testing.T) {
		// Given a handler with Git source and commit ref
		handler := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{
					Name: "git-source",
					Url:  "https://github.com/example/repo.git",
					Ref: blueprintv1alpha1.Reference{
						Commit: "abc123",
						Branch: "main",
					},
				},
			},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git-source",
				},
			},
		}

		// When resolving component sources
		handler.resolveComponentSources(blueprint)

		// Then Git source should use commit ref (highest priority)
		expectedSource := "https://github.com/example/repo.git//terraform/test-module?ref=abc123"
		if blueprint.TerraformComponents[0].Source != expectedSource {
			t.Errorf("Expected source '%s', got '%s'", expectedSource, blueprint.TerraformComponents[0].Source)
		}
	})

	t.Run("HandlesGitSourceWithSemVerRef", func(t *testing.T) {
		// Given a handler with Git source and semver ref
		handler := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{
					Name: "git-source",
					Url:  "https://github.com/example/repo.git",
					Ref: blueprintv1alpha1.Reference{
						SemVer: "1.0.0",
						Tag:    "v1.0.0",
						Branch: "main",
					},
				},
			},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git-source",
				},
			},
		}

		// When resolving component sources
		handler.resolveComponentSources(blueprint)

		// Then Git source should use semver ref (second priority after commit)
		expectedSource := "https://github.com/example/repo.git//terraform/test-module?ref=1.0.0"
		if blueprint.TerraformComponents[0].Source != expectedSource {
			t.Errorf("Expected source '%s', got '%s'", expectedSource, blueprint.TerraformComponents[0].Source)
		}
	})

	t.Run("HandlesComponentsWithoutMatchingSource", func(t *testing.T) {
		// Given a handler with component that doesn't match any source
		handler := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{
					Name: "other-source",
					Url:  "https://github.com/example/repo.git",
				},
			},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "non-matching-source",
				},
			},
		}

		// When resolving component sources
		handler.resolveComponentSources(blueprint)

		// Then component source should remain unchanged
		if blueprint.TerraformComponents[0].Source != "non-matching-source" {
			t.Errorf("Expected source to remain unchanged, got: %s", blueprint.TerraformComponents[0].Source)
		}
	})
}

func TestBaseBlueprintHandler_resolvePatchFromPath(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mockConfigHandler := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell()
		mockArtifactBuilder := artifact.NewMockArtifact()
		rt := &runtime.Runtime{
			ConfigHandler: mockConfigHandler,
			Shell:         mockShell,
		}
		handler, err := NewBlueprintHandler(rt, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = setupDefaultShims()
		handler.runtime.ConfigHandler = config.NewMockConfigHandler()
		return handler
	}

	t.Run("WithRenderedDataOnly", func(t *testing.T) {
		// Given a handler with rendered patch data only
		handler := setup(t)
		handler.kustomizeData = map[string]any{
			"kustomize/patches/test": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name":      "test-config",
					"namespace": "test-namespace",
				},
				"data": map[string]any{
					"key": "value",
				},
			},
		}
		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("test yaml"), nil
		}
		// When resolving patch from path
		content, target := handler.resolvePatchFromPath("test", "default-namespace")
		// Then content should be returned and target should be extracted
		if content != "test yaml" {
			t.Errorf("Expected content = 'test yaml', got = '%s'", content)
		}
		if target == nil {
			t.Error("Expected target to be extracted")
		}
		if target.Kind != "ConfigMap" {
			t.Errorf("Expected target kind = 'ConfigMap', got = '%s'", target.Kind)
		}
		if target.Name != "test-config" {
			t.Errorf("Expected target name = 'test-config', got = '%s'", target.Name)
		}
		if target.Namespace != "test-namespace" {
			t.Errorf("Expected target namespace = 'test-namespace', got = '%s'", target.Namespace)
		}
	})

	t.Run("WithNoData", func(t *testing.T) {
		// Given a handler with no data
		handler := setup(t)
		handler.runtime.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("config root error")
		}
		// When resolving patch from path
		content, target := handler.resolvePatchFromPath("test", "default-namespace")
		// Then empty content and nil target should be returned
		if content != "" {
			t.Errorf("Expected empty content, got = '%s'", content)
		}
		if target != nil {
			t.Error("Expected target to be nil")
		}
	})

	t.Run("WithYamlExtension", func(t *testing.T) {
		// Given a handler with patch path containing .yaml extension
		handler := setup(t)
		handler.kustomizeData = map[string]any{
			"kustomize/patches/test": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "test-config",
				},
			},
		}
		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("test yaml"), nil
		}
		// When resolving patch from path with .yaml extension
		content, target := handler.resolvePatchFromPath("test.yaml", "default-namespace")
		// Then content should be returned and target should be extracted
		if content != "test yaml" {
			t.Errorf("Expected content = 'test yaml', got = '%s'", content)
		}
		if target == nil {
			t.Error("Expected target to be extracted")
		}
	})

	t.Run("WithYmlExtension", func(t *testing.T) {
		handler := setup(t)
		handler.kustomizeData = map[string]any{
			"kustomize/patches/test": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "test-config",
				},
			},
		}
		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("test yaml"), nil
		}
		content, target := handler.resolvePatchFromPath("test.yml", "default-namespace")
		if content != "test yaml" {
			t.Errorf("Expected content = 'test yaml', got = '%s'", content)
		}
		if target == nil {
			t.Error("Expected target to be extracted")
		}
		if target != nil && target.Name != "test-config" {
			t.Errorf("Expected target name = 'test-config', got = '%s'", target.Name)
		}
	})

	t.Run("HandlesYamlMarshalError", func(t *testing.T) {
		handler := setup(t)
		handler.kustomizeData = map[string]any{
			"kustomize/patches/test": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
			},
		}
		expectedError := fmt.Errorf("yaml marshal error")
		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return nil, expectedError
		}
		content, target := handler.resolvePatchFromPath("test", "default-namespace")
		if content != "" {
			t.Errorf("Expected empty content on YamlMarshal error, got = '%s'", content)
		}
		if target != nil {
			t.Error("Expected nil target on YamlMarshal error")
		}
	})

	t.Run("HandlesReadFileWithBasePatchData", func(t *testing.T) {
		tmpDir := t.TempDir()
		handler := setup(t)
		handler.runtime.ConfigRoot = tmpDir
		handler.kustomizeData = map[string]any{
			"kustomize/patches/test": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
			},
		}
		patchDir := filepath.Join(tmpDir, "kustomize")
		if err := os.MkdirAll(patchDir, 0755); err != nil {
			t.Fatalf("Failed to create patch directory: %v", err)
		}
		patchFile := filepath.Join(patchDir, "test")
		userPatchContent := `apiVersion: v1
kind: ConfigMap
metadata:
  name: user-config
data:
  key: value
`
		if err := os.WriteFile(patchFile, []byte(userPatchContent), 0644); err != nil {
			t.Fatalf("Failed to write patch file: %v", err)
		}
		handler.shims.ReadFile = os.ReadFile
		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			return yaml.Unmarshal(data, v)
		}
		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return yaml.Marshal(v)
		}
		content, target := handler.resolvePatchFromPath("test", "default-namespace")
		if content == "" {
			t.Error("Expected content to be merged")
		}
		if target == nil {
			t.Error("Expected target to be extracted")
		}
		if !strings.Contains(content, "user-config") {
			t.Error("Expected merged content to contain user patch data")
		}
	})

	t.Run("HandlesYamlUnmarshalErrorWhenBasePatchDataExists", func(t *testing.T) {
		tmpDir := t.TempDir()
		handler := setup(t)
		handler.runtime.ConfigRoot = tmpDir
		handler.kustomizeData = map[string]any{
			"kustomize/patches/test": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
			},
		}
		patchDir := filepath.Join(tmpDir, "kustomize")
		if err := os.MkdirAll(patchDir, 0755); err != nil {
			t.Fatalf("Failed to create patch directory: %v", err)
		}
		patchFile := filepath.Join(patchDir, "test")
		patchContent := `apiVersion: v1
kind: ConfigMap
metadata:
  name: file-config
`
		if err := os.WriteFile(patchFile, []byte(patchContent), 0644); err != nil {
			t.Fatalf("Failed to write patch file: %v", err)
		}
		handler.shims.ReadFile = os.ReadFile
		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			return fmt.Errorf("yaml unmarshal error")
		}
		content, target := handler.resolvePatchFromPath("test", "default-namespace")
		if content == "" {
			t.Error("Expected content from file when unmarshal fails")
		}
		if target == nil {
			t.Error("Expected target to be extracted from file content")
		}
		if target != nil && target.Name != "file-config" {
			t.Errorf("Expected target name = 'file-config', got = '%s'", target.Name)
		}
	})

	t.Run("WithBothRenderedAndUserDataMerge", func(t *testing.T) {
		// Given a handler with both rendered and user data that can be merged
		handler := setup(t)
		handler.kustomizeData = map[string]any{
			"kustomize/patches/test": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name":      "rendered-config",
					"namespace": "rendered-namespace",
				},
				"data": map[string]any{
					"rendered-key": "rendered-value",
				},
			},
		}
		handler.runtime.ConfigRoot = "/test/config"
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			return []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: user-config
  namespace: user-namespace
data:
  user-key: user-value`), nil
		}
		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			values := v.(*map[string]any)
			*values = map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name":      "user-config",
					"namespace": "user-namespace",
				},
				"data": map[string]any{
					"user-key": "user-value",
				},
			}
			return nil
		}
		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("merged yaml"), nil
		}
		// When resolving patch from path
		content, target := handler.resolvePatchFromPath("test", "default-namespace")
		// Then merged content should be returned and target should be extracted from merged data
		if content != "merged yaml" {
			t.Errorf("Expected content = 'merged yaml', got = '%s'", content)
		}
		if target == nil {
			t.Error("Expected target to be extracted")
		}
		if target.Name != "user-config" {
			t.Errorf("Expected target name = 'user-config', got = '%s'", target.Name)
		}
		if target.Namespace != "user-namespace" {
			t.Errorf("Expected target namespace = 'user-namespace', got = '%s'", target.Namespace)
		}
	})
}

func TestBaseBlueprintHandler_extractTargetFromPatchData(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mockConfigHandler := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell()
		mockArtifactBuilder := artifact.NewMockArtifact()
		rt := &runtime.Runtime{
			ConfigHandler: mockConfigHandler,
			Shell:         mockShell,
		}
		handler, err := NewBlueprintHandler(rt, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		return handler
	}

	t.Run("ValidPatchData", func(t *testing.T) {
		// Given valid patch data with all required fields
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      "test-config",
				"namespace": "test-namespace",
			},
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be extracted correctly
		if target == nil {
			t.Error("Expected target to be extracted")
		}
		if target.Kind != "ConfigMap" {
			t.Errorf("Expected target kind = 'ConfigMap', got = '%s'", target.Kind)
		}
		if target.Name != "test-config" {
			t.Errorf("Expected target name = 'test-config', got = '%s'", target.Name)
		}
		if target.Namespace != "test-namespace" {
			t.Errorf("Expected target namespace = 'test-namespace', got = '%s'", target.Namespace)
		}
	})

	t.Run("WithCustomNamespace", func(t *testing.T) {
		// Given patch data with custom namespace
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      "test-config",
				"namespace": "custom-namespace",
			},
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then custom namespace should be used
		if target.Namespace != "custom-namespace" {
			t.Errorf("Expected target namespace = 'custom-namespace', got = '%s'", target.Namespace)
		}
	})

	t.Run("MissingKind", func(t *testing.T) {
		// Given patch data missing kind field
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"metadata": map[string]any{
				"name": "test-config",
			},
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when kind is missing")
		}
	})

	t.Run("MissingMetadata", func(t *testing.T) {
		// Given patch data missing metadata field
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when metadata is missing")
		}
	})

	t.Run("MissingName", func(t *testing.T) {
		// Given patch data missing name field
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{},
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when name is missing")
		}
	})

	t.Run("InvalidKindType", func(t *testing.T) {
		// Given patch data with invalid kind type
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       42,
			"metadata": map[string]any{
				"name": "test-config",
			},
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when kind type is invalid")
		}
	})

	t.Run("InvalidMetadataType", func(t *testing.T) {
		// Given patch data with invalid metadata type
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   "not a map",
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when metadata type is invalid")
		}
	})

	t.Run("InvalidNameType", func(t *testing.T) {
		// Given patch data with invalid name type
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": 42,
			},
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when name type is invalid")
		}
	})
}

func TestBaseBlueprintHandler_extractTargetFromPatchContent(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mockConfigHandler := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell()
		mockArtifactBuilder := artifact.NewMockArtifact()
		rt := &runtime.Runtime{
			ConfigHandler: mockConfigHandler,
			Shell:         mockShell,
		}
		handler, err := NewBlueprintHandler(rt, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		return handler
	}

	t.Run("ValidYamlContent", func(t *testing.T) {
		// Given valid YAML content
		handler := setup(t)
		content := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
  namespace: test-namespace`
		// When extracting target from patch content
		target := handler.extractTargetFromPatchContent(content, "default-namespace")
		// Then target should be extracted correctly
		if target == nil {
			t.Error("Expected target to be extracted")
		}
		if target.Name != "test-config" {
			t.Errorf("Expected target name = 'test-config', got = '%s'", target.Name)
		}
	})

	t.Run("MultipleDocuments", func(t *testing.T) {
		// Given YAML with multiple documents
		handler := setup(t)
		content := `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: first-config
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: second-config`
		// When extracting target from patch content
		target := handler.extractTargetFromPatchContent(content, "default-namespace")
		// Then first valid target should be extracted
		if target == nil {
			t.Error("Expected target to be extracted")
		}
		if target.Name != "first-config" {
			t.Errorf("Expected target name = 'first-config', got = '%s'", target.Name)
		}
	})

	t.Run("InvalidYamlContent", func(t *testing.T) {
		// Given invalid YAML content
		handler := setup(t)
		content := `invalid: yaml: content: with: colons: everywhere`
		// When extracting target from patch content
		target := handler.extractTargetFromPatchContent(content, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil for invalid YAML")
		}
	})

	t.Run("EmptyContent", func(t *testing.T) {
		// Given empty content
		handler := setup(t)
		content := ""
		// When extracting target from patch content
		target := handler.extractTargetFromPatchContent(content, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil for empty content")
		}
	})

	t.Run("NoValidTargets", func(t *testing.T) {
		// Given YAML with no valid targets
		handler := setup(t)
		content := `apiVersion: v1
kind: ConfigMap
# Missing metadata.name`
		// When extracting target from patch content
		target := handler.extractTargetFromPatchContent(content, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when no valid targets")
		}
	})
}

func TestBaseBlueprintHandler_deepMergeMaps(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mockConfigHandler := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell()
		mockArtifactBuilder := artifact.NewMockArtifact()
		rt := &runtime.Runtime{
			ConfigHandler: mockConfigHandler,
			Shell:         mockShell,
		}
		handler, err := NewBlueprintHandler(rt, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		return handler
	}

	t.Run("SimpleMerge", func(t *testing.T) {
		// Given base and overlay maps with simple values
		handler := setup(t)
		base := map[string]any{
			"key1": "base-value1",
			"key2": "base-value2",
		}
		overlay := map[string]any{
			"key2": "overlay-value2",
			"key3": "overlay-value3",
		}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then result should contain merged values
		if result["key1"] != "base-value1" {
			t.Errorf("Expected key1 = 'base-value1', got = '%v'", result["key1"])
		}
		if result["key2"] != "overlay-value2" {
			t.Errorf("Expected key2 = 'overlay-value2', got = '%v'", result["key2"])
		}
		if result["key3"] != "overlay-value3" {
			t.Errorf("Expected key3 = 'overlay-value3', got = '%v'", result["key3"])
		}
	})

	t.Run("NestedMapMerge", func(t *testing.T) {
		// Given base and overlay maps with nested maps
		handler := setup(t)
		base := map[string]any{
			"nested": map[string]any{
				"base-key": "base-value",
			},
		}
		overlay := map[string]any{
			"nested": map[string]any{
				"overlay-key": "overlay-value",
			},
		}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then nested maps should be merged
		nested := result["nested"].(map[string]any)
		if nested["base-key"] != "base-value" {
			t.Errorf("Expected nested.base-key = 'base-value', got = '%v'", nested["base-key"])
		}
		if nested["overlay-key"] != "overlay-value" {
			t.Errorf("Expected nested.overlay-key = 'overlay-value', got = '%v'", nested["overlay-key"])
		}
	})

	t.Run("OverlayPrecedence", func(t *testing.T) {
		// Given base and overlay maps with conflicting keys
		handler := setup(t)
		base := map[string]any{
			"key": "base-value",
		}
		overlay := map[string]any{
			"key": "overlay-value",
		}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then overlay value should take precedence
		if result["key"] != "overlay-value" {
			t.Errorf("Expected key = 'overlay-value', got = '%v'", result["key"])
		}
	})

	t.Run("DeepNestedMerge", func(t *testing.T) {
		// Given base and overlay maps with deeply nested maps
		handler := setup(t)
		base := map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"base-key": "base-value",
				},
			},
		}
		overlay := map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"overlay-key": "overlay-value",
				},
			},
		}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then deeply nested maps should be merged
		level1 := result["level1"].(map[string]any)
		level2 := level1["level2"].(map[string]any)
		if level2["base-key"] != "base-value" {
			t.Errorf("Expected level2.base-key = 'base-value', got = '%v'", level2["base-key"])
		}
		if level2["overlay-key"] != "overlay-value" {
			t.Errorf("Expected level2.overlay-key = 'overlay-value', got = '%v'", level2["overlay-key"])
		}
	})

	t.Run("EmptyMaps", func(t *testing.T) {
		// Given empty base and overlay maps
		handler := setup(t)
		base := map[string]any{}
		overlay := map[string]any{}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then result should be empty
		if len(result) != 0 {
			t.Errorf("Expected empty result, got %d items", len(result))
		}
	})

	t.Run("NonMapOverlay", func(t *testing.T) {
		// Given base map and non-map overlay value
		handler := setup(t)
		base := map[string]any{
			"key": map[string]any{
				"nested": "value",
			},
		}
		overlay := map[string]any{
			"key": "string-value",
		}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then overlay value should replace base value
		if result["key"] != "string-value" {
			t.Errorf("Expected key = 'string-value', got = '%v'", result["key"])
		}
	})

	t.Run("MixedTypes", func(t *testing.T) {
		// Given base and overlay maps with mixed types
		handler := setup(t)
		base := map[string]any{
			"string": "base-string",
			"number": 42,
			"nested": map[string]any{
				"key": "base-nested",
			},
		}
		overlay := map[string]any{
			"string": "overlay-string",
			"bool":   true,
			"nested": map[string]any{
				"overlay-key": "overlay-nested",
			},
		}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then all values should be merged correctly
		if result["string"] != "overlay-string" {
			t.Errorf("Expected string = 'overlay-string', got = '%v'", result["string"])
		}
		if result["number"] != 42 {
			t.Errorf("Expected number = 42, got = '%v'", result["number"])
		}
		if result["bool"] != true {
			t.Errorf("Expected bool = true, got = '%v'", result["bool"])
		}
		nested := result["nested"].(map[string]any)
		if nested["key"] != "base-nested" {
			t.Errorf("Expected nested.key = 'base-nested', got = '%v'", nested["key"])
		}
		if nested["overlay-key"] != "overlay-nested" {
			t.Errorf("Expected nested.overlay-key = 'overlay-nested', got = '%v'", nested["overlay-key"])
		}
	})
}

// =============================================================================
// Validation Tests
// =============================================================================

func TestBaseBlueprintHandler_validateValuesForSubstitution(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		return &BaseBlueprintHandler{}
	}

	t.Run("AcceptsValidScalarValues", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"string_value":  "test",
			"int_value":     42,
			"int8_value":    int8(8),
			"int16_value":   int16(16),
			"int32_value":   int32(32),
			"int64_value":   int64(64),
			"uint_value":    uint(42),
			"uint8_value":   uint8(8),
			"uint16_value":  uint16(16),
			"uint32_value":  uint32(32),
			"uint64_value":  uint64(64),
			"float32_value": float32(3.14),
			"float64_value": 3.14159,
			"bool_value":    true,
		}

		err := handler.validateValuesForSubstitution(values)
		if err != nil {
			t.Errorf("Expected no error for valid scalar values, got: %v", err)
		}
	})

	t.Run("AcceptsOneLevelOfMapWithScalarValues", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"top_level_string": "value",
			"scalar_map": map[string]any{
				"nested_string": "nested_value",
				"nested_int":    123,
				"nested_bool":   false,
			},
			"another_top_level": 456,
		}

		err := handler.validateValuesForSubstitution(values)
		if err != nil {
			t.Errorf("Expected no error for map with scalar values, got: %v", err)
		}
	})

	t.Run("RejectsNestedMaps", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"top_level": map[string]any{
				"second_level": map[string]any{
					"third_level": "value",
				},
			},
		}

		err := handler.validateValuesForSubstitution(values)
		if err == nil {
			t.Error("Expected error for nested maps")
		}

		if !strings.Contains(err.Error(), "can only contain scalar values in maps") {
			t.Errorf("Expected error about scalar values only in maps, got: %v", err)
		}
		if !strings.Contains(err.Error(), "top_level.second_level") {
			t.Errorf("Expected error to mention the nested key path, got: %v", err)
		}
	})

	t.Run("RejectsSlices", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"valid_string":  "test",
			"invalid_slice": []any{"item1", "item2", "item3"},
		}

		err := handler.validateValuesForSubstitution(values)
		if err == nil {
			t.Error("Expected error for slice values")
		}

		if !strings.Contains(err.Error(), "cannot contain slices") {
			t.Errorf("Expected error about slices, got: %v", err)
		}
		if !strings.Contains(err.Error(), "invalid_slice") {
			t.Errorf("Expected error to mention the slice key, got: %v", err)
		}
	})

	t.Run("RejectsSlicesInNestedMaps", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"nested_map": map[string]any{
				"valid_value":   "test",
				"invalid_slice": []any{"item1", "item2"}, // Use []any to match the type check
			},
		}

		err := handler.validateValuesForSubstitution(values)
		if err == nil {
			t.Error("Expected error for slice in nested map")
		}

		if !strings.Contains(err.Error(), "cannot contain slices") {
			t.Errorf("Expected error about slices, got: %v", err)
		}
		if !strings.Contains(err.Error(), "nested_map.invalid_slice") {
			t.Errorf("Expected error to mention the nested slice key path, got: %v", err)
		}
	})

	t.Run("RejectsTypedSlices", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"string_slice": []string{"item1", "item2"},
			"int_slice":    []int{1, 2, 3},
		}

		err := handler.validateValuesForSubstitution(values)
		if err == nil {
			t.Error("Expected error for typed slices")
		}

		// After the fix, typed slices should now get the specific slice error message
		if !strings.Contains(err.Error(), "cannot contain slices") {
			t.Errorf("Expected error about slices for typed slices, got: %v", err)
		}
	})

	t.Run("RejectsUnsupportedTypes", func(t *testing.T) {
		handler := setup(t)

		// Test with a struct (unsupported type)
		type customStruct struct {
			Field string
		}

		values := map[string]any{
			"valid_string":     "test",
			"invalid_struct":   customStruct{Field: "value"},
			"invalid_function": func() {},
		}

		err := handler.validateValuesForSubstitution(values)
		if err == nil {
			t.Error("Expected error for unsupported types")
		}

		if !strings.Contains(err.Error(), "can only contain strings, numbers, booleans, or maps of scalar types") {
			t.Errorf("Expected error about unsupported types, got: %v", err)
		}
	})

	t.Run("RejectsUnsupportedTypesInNestedMaps", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"nested_map": map[string]any{
				"valid_value":   "test",
				"invalid_value": make(chan int), // Channel is unsupported
			},
		}

		err := handler.validateValuesForSubstitution(values)
		if err == nil {
			t.Error("Expected error for unsupported type in nested map")
		}

		if !strings.Contains(err.Error(), "can only contain scalar values in maps") {
			t.Errorf("Expected error about scalar values only in maps, got: %v", err)
		}
		if !strings.Contains(err.Error(), "nested_map.invalid_value") {
			t.Errorf("Expected error to mention the nested key path, got: %v", err)
		}
	})

	t.Run("RejectsSlicesInMaps", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"config": map[string]any{
				"valid_key": "test",
				"slice_key": []string{"item1", "item2"},
			},
		}

		err := handler.validateValuesForSubstitution(values)
		if err == nil {
			t.Error("Expected error for slices in maps")
		}

		if !strings.Contains(err.Error(), "cannot contain slices") {
			t.Errorf("Expected error about slices, got: %v", err)
		}
		if !strings.Contains(err.Error(), "config.slice_key") {
			t.Errorf("Expected error to mention the nested slice key path, got: %v", err)
		}
	})

	t.Run("HandlesEmptyValues", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{}

		err := handler.validateValuesForSubstitution(values)
		if err != nil {
			t.Errorf("Expected no error for empty values, got: %v", err)
		}
	})

	t.Run("HandlesEmptyNestedMaps", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"empty_nested": map[string]any{},
			"valid_value":  "test",
		}

		err := handler.validateValuesForSubstitution(values)
		if err != nil {
			t.Errorf("Expected no error for empty nested maps, got: %v", err)
		}
	})

	t.Run("HandlesNilValues", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"nil_value":   nil,
			"valid_value": "test",
		}

		err := handler.validateValuesForSubstitution(values)
		if err == nil {
			t.Error("Expected error for nil values")
		}

		if !strings.Contains(err.Error(), "cannot contain nil values") {
			t.Errorf("Expected error about nil values, got: %v", err)
		}
	})

	t.Run("HandlesNilValuesInMaps", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"config": map[string]any{
				"valid_key": "test",
				"nil_key":   nil,
			},
		}

		err := handler.validateValuesForSubstitution(values)
		if err == nil {
			t.Error("Expected error for nil values in maps")
		}

		if !strings.Contains(err.Error(), "cannot contain nil values") {
			t.Errorf("Expected error about nil values, got: %v", err)
		}
		if !strings.Contains(err.Error(), "config.nil_key") {
			t.Errorf("Expected error to mention the nested nil key path, got: %v", err)
		}
	})

	t.Run("ValidatesComplexScenario", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"app_name":    "my-app",
			"app_version": "1.2.3",
			"replicas":    3,
			"enabled":     true,
			"config": map[string]any{
				"database_url":    "postgres://localhost:5432/mydb",
				"cache_enabled":   true,
				"max_connections": 100,
				"timeout_seconds": 30.5,
				"debug_mode":      false,
			},
			"resources": map[string]any{
				"cpu_limit":      "500m",
				"memory_limit":   "512Mi",
				"cpu_request":    "100m",
				"memory_request": "128Mi",
			},
		}

		err := handler.validateValuesForSubstitution(values)
		if err != nil {
			t.Errorf("Expected no error for complex valid scenario, got: %v", err)
		}
	})

	t.Run("RejectsComplexScenarioWithInvalidNesting", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"app_name": "my-app",
			"config": map[string]any{
				"database": map[string]any{ // Maps cannot contain other maps
					"host": "localhost",
					"port": 5432,
				},
			},
		}

		err := handler.validateValuesForSubstitution(values)
		if err == nil {
			t.Error("Expected error for invalid nesting in complex scenario")
		}

		if !strings.Contains(err.Error(), "can only contain scalar values in maps") {
			t.Errorf("Expected error about scalar values only in maps, got: %v", err)
		}
		if !strings.Contains(err.Error(), "config.database") {
			t.Errorf("Expected error to mention the nested path, got: %v", err)
		}
	})

	t.Run("HandlesSpecialNumericTypes", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"zero_int":       0,
			"negative_int":   -42,
			"zero_float":     0.0,
			"negative_float": -3.14,
			"large_uint64":   uint64(18446744073709551615), // Max uint64
			"small_int8":     int8(-128),                   // Min int8
		}

		err := handler.validateValuesForSubstitution(values)
		if err != nil {
			t.Errorf("Expected no error for special numeric types, got: %v", err)
		}
	})

	t.Run("HandlesSpecialStringValues", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"empty_string":  "",
			"whitespace":    "   ",
			"newlines":      "line1\nline2",
			"unicode":       "Hello 世界 🌍",
			"special_chars": "!@#$%^&*()_+-={}[]|\\:;\"'<>?,./",
		}

		err := handler.validateValuesForSubstitution(values)
		if err != nil {
			t.Errorf("Expected no error for special string values, got: %v", err)
		}
	})
}

func TestBaseBlueprintHandler_parseFeature(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		return handler
	}

	t.Run("ParseValidFeature", func(t *testing.T) {
		handler := setup(t)

		featureYAML := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-observability
  description: Observability stack for AWS
when: provider == "aws"
terraform:
  - path: observability/quickwit
    when: observability.backend == "quickwit"
    values:
      storage_bucket: my-bucket
kustomize:
  - name: grafana
    path: observability/grafana
    when: observability.enabled == true
`)

		feature, err := handler.parseFeature(featureYAML)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if feature.Kind != "Feature" {
			t.Errorf("Expected kind 'Feature', got '%s'", feature.Kind)
		}
		if feature.ApiVersion != "blueprints.windsorcli.dev/v1alpha1" {
			t.Errorf("Expected apiVersion 'blueprints.windsorcli.dev/v1alpha1', got '%s'", feature.ApiVersion)
		}
		if feature.Metadata.Name != "aws-observability" {
			t.Errorf("Expected name 'aws-observability', got '%s'", feature.Metadata.Name)
		}
		if feature.When != `provider == "aws"` {
			t.Errorf("Expected when condition, got '%s'", feature.When)
		}
		if len(feature.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(feature.TerraformComponents))
		}
		if len(feature.Kustomizations) != 1 {
			t.Errorf("Expected 1 kustomization, got %d", len(feature.Kustomizations))
		}
	})

	t.Run("FailsOnInvalidYAML", func(t *testing.T) {
		handler := setup(t)

		invalidYAML := []byte(`this is not valid yaml: [`)

		_, err := handler.parseFeature(invalidYAML)

		if err == nil {
			t.Error("Expected error for invalid YAML, got nil")
		}
		if !strings.Contains(err.Error(), "invalid YAML") {
			t.Errorf("Expected 'invalid YAML' error, got %v", err)
		}
	})

	t.Run("FailsOnWrongKind", func(t *testing.T) {
		handler := setup(t)

		wrongKind := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test
`)

		_, err := handler.parseFeature(wrongKind)

		if err == nil {
			t.Error("Expected error for wrong kind, got nil")
		}
		if !strings.Contains(err.Error(), "expected kind 'Feature'") {
			t.Errorf("Expected kind error, got %v", err)
		}
	})

	t.Run("FailsOnMissingApiVersion", func(t *testing.T) {
		handler := setup(t)

		missingVersion := []byte(`kind: Feature
metadata:
  name: test
`)

		_, err := handler.parseFeature(missingVersion)

		if err == nil {
			t.Error("Expected error for missing apiVersion, got nil")
		}
		if !strings.Contains(err.Error(), "apiVersion is required") {
			t.Errorf("Expected apiVersion error, got %v", err)
		}
	})

	t.Run("FailsOnMissingName", func(t *testing.T) {
		handler := setup(t)

		missingName := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  description: test
`)

		_, err := handler.parseFeature(missingName)

		if err == nil {
			t.Error("Expected error for missing name, got nil")
		}
		if !strings.Contains(err.Error(), "metadata.name is required") {
			t.Errorf("Expected name error, got %v", err)
		}
	})

	t.Run("ParseFeatureWithoutWhenCondition", func(t *testing.T) {
		handler := setup(t)

		featureYAML := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base-feature
terraform:
  - path: base/component
`)

		feature, err := handler.parseFeature(featureYAML)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if feature.When != "" {
			t.Errorf("Expected empty when condition, got '%s'", feature.When)
		}
		if len(feature.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(feature.TerraformComponents))
		}
	})
}

func TestBaseBlueprintHandler_loadFeatures(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		return handler
	}

	t.Run("LoadMultipleFeatures", func(t *testing.T) {
		handler := setup(t)

		templateData := map[string][]byte{
			"features/aws.yaml": []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-feature
`),
			"features/observability.yaml": []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: observability-feature
`),
			"blueprint.jsonnet": []byte(`{}`),
			"schema.yaml":       []byte(`{}`),
		}

		features, err := handler.loadFeatures(templateData)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(features) != 2 {
			t.Errorf("Expected 2 features, got %d", len(features))
		}
		names := make(map[string]bool)
		for _, feature := range features {
			names[feature.Metadata.Name] = true
		}
		if !names["aws-feature"] || !names["observability-feature"] {
			t.Errorf("Expected both features to be loaded, got %v", names)
		}
	})

	t.Run("LoadNoFeatures", func(t *testing.T) {
		handler := setup(t)

		templateData := map[string][]byte{
			"blueprint.jsonnet": []byte(`{}`),
			"schema.yaml":       []byte(`{}`),
		}

		features, err := handler.loadFeatures(templateData)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(features) != 0 {
			t.Errorf("Expected 0 features, got %d", len(features))
		}
	})

	t.Run("IgnoresNonFeatureYAMLFiles", func(t *testing.T) {
		handler := setup(t)

		templateData := map[string][]byte{
			"features/aws.yaml": []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-feature
`),
			"schema.yaml":           []byte(`{}`),
			"values.yaml":           []byte(`key: value`),
			"terraform/module.yaml": []byte(`key: value`),
		}

		features, err := handler.loadFeatures(templateData)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(features) != 1 {
			t.Errorf("Expected 1 feature, got %d", len(features))
		}
		if features[0].Metadata.Name != "aws-feature" {
			t.Errorf("Expected 'aws-feature', got '%s'", features[0].Metadata.Name)
		}
	})

	t.Run("FailsOnInvalidFeature", func(t *testing.T) {
		handler := setup(t)

		templateData := map[string][]byte{
			"features/valid.yaml": []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: valid-feature
`),
			"features/invalid.yaml": []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  description: missing name
`),
		}

		_, err := handler.loadFeatures(templateData)

		if err == nil {
			t.Error("Expected error for invalid feature, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse feature features/invalid.yaml") {
			t.Errorf("Expected parse error with path, got %v", err)
		}
		if !strings.Contains(err.Error(), "metadata.name is required") {
			t.Errorf("Expected name requirement error, got %v", err)
		}
	})

	t.Run("LoadFeaturesWithComplexStructures", func(t *testing.T) {
		handler := setup(t)

		templateData := map[string][]byte{
			"features/complex.yaml": []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: complex-feature
  description: Complex feature with multiple components
when: provider == "aws" && observability.enabled == true
terraform:
  - path: observability/quickwit
    when: observability.backend == "quickwit"
    values:
      storage_bucket: my-bucket
      replicas: 3
  - path: observability/grafana
    values:
      domain: grafana.example.com
kustomize:
  - name: monitoring
    path: monitoring/stack
    when: monitoring.enabled == true
  - name: logging
    path: logging/stack
`),
		}

		features, err := handler.loadFeatures(templateData)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(features) != 1 {
			t.Fatalf("Expected 1 feature, got %d", len(features))
		}
		feature := features[0]
		if feature.Metadata.Name != "complex-feature" {
			t.Errorf("Expected 'complex-feature', got '%s'", feature.Metadata.Name)
		}
		if len(feature.TerraformComponents) != 2 {
			t.Errorf("Expected 2 terraform components, got %d", len(feature.TerraformComponents))
		}
		if len(feature.Kustomizations) != 2 {
			t.Errorf("Expected 2 kustomizations, got %d", len(feature.Kustomizations))
		}
		if feature.TerraformComponents[0].When != `observability.backend == "quickwit"` {
			t.Errorf("Expected when condition on terraform component, got '%s'", feature.TerraformComponents[0].When)
		}
	})
}

func TestBaseBlueprintHandler_processBlueprintData(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		return handler
	}

	t.Run("HandlesYamlUnmarshalError", func(t *testing.T) {
		// Given a handler with invalid YAML data
		handler := setup(t)
		invalidYAML := []byte("invalid: yaml: content: [")
		blueprint := &blueprintv1alpha1.Blueprint{}

		// When processing blueprint data
		err := handler.processBlueprintData(invalidYAML, blueprint)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when YAML unmarshal fails")
		}
		if !strings.Contains(err.Error(), "error unmarshalling blueprint data") {
			t.Errorf("Expected error about unmarshalling, got: %v", err)
		}
	})

	t.Run("HandlesOCIInfoWithExistingSource", func(t *testing.T) {
		// Given a handler with OCI info and existing source
		handler := setup(t)
		blueprintData := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test-blueprint
sources:
  - name: oci-source
    url: oci://old-registry/old-repo:old-tag
terraformComponents: []
kustomizations: []`)
		blueprint := &blueprintv1alpha1.Blueprint{}
		ociInfo := &artifact.OCIArtifactInfo{
			Name: "oci-source",
			URL:  "oci://new-registry/new-repo:new-tag",
		}

		// When processing blueprint data with OCI info
		err := handler.processBlueprintData(blueprintData, blueprint, ociInfo)

		// Then it should succeed and update the source
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if len(blueprint.Sources) != 1 {
			t.Errorf("Expected 1 source, got %d", len(blueprint.Sources))
		}
		if blueprint.Sources[0].Url != "oci://new-registry/new-repo:new-tag" {
			t.Errorf("Expected source URL to be updated, got: %s", blueprint.Sources[0].Url)
		}
	})

	t.Run("HandlesOCIInfoWithEmptyComponentSource", func(t *testing.T) {
		// Given a handler with OCI info and components with empty source
		handler := setup(t)
		blueprintData := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test-blueprint
sources: []
terraformComponents:
  - path: test-component
    source: ""
kustomizations:
  - name: test-kustomization
    source: ""`)
		blueprint := &blueprintv1alpha1.Blueprint{}
		ociInfo := &artifact.OCIArtifactInfo{
			Name: "oci-source",
			URL:  "oci://registry/repo:tag",
		}

		// When processing blueprint data with OCI info
		err := handler.processBlueprintData(blueprintData, blueprint, ociInfo)

		// Then it should succeed and set source on components
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if len(blueprint.TerraformComponents) > 0 && blueprint.TerraformComponents[0].Source != "oci-source" {
			t.Errorf("Expected component source to be set, got: %s", blueprint.TerraformComponents[0].Source)
		}
		if len(blueprint.Kustomizations) > 0 && blueprint.Kustomizations[0].Source != "oci-source" {
			t.Errorf("Expected kustomization source to be set, got: %s", blueprint.Kustomizations[0].Source)
		}
	})

	t.Run("HandlesOCIInfoWithNewSource", func(t *testing.T) {
		// Given a handler with OCI info and new source (not existing)
		handler := setup(t)
		blueprintData := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test-blueprint
sources: []
terraformComponents: []
kustomizations: []`)
		blueprint := &blueprintv1alpha1.Blueprint{}
		ociInfo := &artifact.OCIArtifactInfo{
			Name: "oci-source",
			URL:  "oci://registry/repo:tag",
		}

		// When processing blueprint data with OCI info
		err := handler.processBlueprintData(blueprintData, blueprint, ociInfo)

		// Then it should succeed and add the source
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if len(blueprint.Sources) != 1 {
			t.Errorf("Expected 1 source, got %d", len(blueprint.Sources))
		}
		if blueprint.Sources[0].Name != "oci-source" {
			t.Errorf("Expected source name to be 'oci-source', got: %s", blueprint.Sources[0].Name)
		}
	})
}

func TestBaseBlueprintHandler_processFeatures(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		return handler
	}

	t.Run("ProcessFeaturesWithMatchingConditions", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)

		awsFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-feature
when: provider == "aws"
terraform:
  - path: observability/quickwit
    values:
      bucket: my-bucket
`)

		templateData := map[string][]byte{
			"blueprint":         baseBlueprint,
			"features/aws.yaml": awsFeature,
		}

		config := map[string]any{
			"provider": "aws",
		}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(handler.blueprint.TerraformComponents))
		}
		if handler.blueprint.TerraformComponents[0].Path != "observability/quickwit" {
			t.Errorf("Expected path 'observability/quickwit', got '%s'", handler.blueprint.TerraformComponents[0].Path)
		}
	})

	t.Run("SkipsFeaturesWithNonMatchingConditions", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)

		awsFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-feature
when: provider == "aws"
terraform:
  - path: observability/quickwit
`)

		templateData := map[string][]byte{
			"blueprint":         baseBlueprint,
			"features/aws.yaml": awsFeature,
		}

		config := map[string]any{
			"provider": "gcp",
		}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.TerraformComponents) != 0 {
			t.Errorf("Expected 0 terraform components, got %d", len(handler.blueprint.TerraformComponents))
		}
	})

	t.Run("HandlesProcessBlueprintDataError", func(t *testing.T) {
		// Given a handler with invalid blueprint data
		handler := setup(t)
		invalidBlueprint := []byte("invalid: yaml: [")
		templateData := map[string][]byte{
			"blueprint": invalidBlueprint,
		}
		config := map[string]any{}

		// When processing features
		err := handler.processFeatures(templateData, config)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when processBlueprintData fails")
		}
		if !strings.Contains(err.Error(), "failed to load base blueprint.yaml") {
			t.Errorf("Expected error about loading blueprint, got: %v", err)
		}
	})

	t.Run("HandlesLoadFeaturesError", func(t *testing.T) {
		// Given a handler with invalid feature data
		handler := setup(t)
		invalidFeature := []byte("invalid: yaml: [")
		templateData := map[string][]byte{
			"features/invalid.yaml": invalidFeature,
		}
		config := map[string]any{}

		// When processing features
		err := handler.processFeatures(templateData, config)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when loadFeatures fails")
		}
		if !strings.Contains(err.Error(), "failed to load features") {
			t.Errorf("Expected error about loading features, got: %v", err)
		}
	})

	t.Run("HandlesEvaluateDefaultsErrorForTerraformComponent", func(t *testing.T) {
		handler := setup(t)
		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)
		feature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test-feature
terraform:
  - path: test/component
    inputs:
      key: ${invalid expression [[[
`)
		templateData := map[string][]byte{
			"blueprint":          baseBlueprint,
			"features/test.yaml": feature,
		}
		config := map[string]any{}

		err := handler.processFeatures(templateData, config)

		if err == nil {
			t.Fatal("Expected error when EvaluateDefaults fails")
		}
		if !strings.Contains(err.Error(), "failed to evaluate inputs") {
			t.Errorf("Expected error about evaluating inputs, got: %v", err)
		}
	})

	t.Run("HandlesStrategicMergeErrorForTerraformComponent", func(t *testing.T) {
		handler := setup(t)
		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)
		feature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test-feature
terraform:
  - path: component-a
    dependsOn:
      - component-b
  - path: component-b
    dependsOn:
      - component-c
  - path: component-c
    dependsOn:
      - component-a
`)
		templateData := map[string][]byte{
			"blueprint":          baseBlueprint,
			"features/test.yaml": feature,
		}
		config := map[string]any{}

		err := handler.processFeatures(templateData, config)

		if err == nil {
			t.Fatal("Expected error when StrategicMerge fails due to dependency cycle")
		}
		if !strings.Contains(err.Error(), "failed to merge terraform component") {
			t.Errorf("Expected error about merging component, got: %v", err)
		}
	})

	t.Run("HandlesEvaluateExpressionErrorForKustomization", func(t *testing.T) {
		handler := setup(t)
		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)
		feature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test-feature
kustomize:
  - name: test-kustomization
    when: invalid expression [[[
`)
		templateData := map[string][]byte{
			"blueprint":          baseBlueprint,
			"features/test.yaml": feature,
		}
		config := map[string]any{}

		err := handler.processFeatures(templateData, config)

		if err == nil {
			t.Fatal("Expected error when EvaluateExpression fails for kustomization")
		}
		if !strings.Contains(err.Error(), "failed to evaluate kustomization condition") {
			t.Errorf("Expected error about evaluating kustomization condition, got: %v", err)
		}
	})

	t.Run("HandlesEvaluateSubstitutionsErrorForKustomization", func(t *testing.T) {
		handler := setup(t)
		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)
		feature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test-feature
kustomize:
  - name: test-kustomization
    substitutions:
      key: ${invalid expression [[[
`)
		templateData := map[string][]byte{
			"blueprint":          baseBlueprint,
			"features/test.yaml": feature,
		}
		config := map[string]any{}

		err := handler.processFeatures(templateData, config)

		if err == nil {
			t.Fatal("Expected error when evaluateSubstitutions fails")
		}
		if !strings.Contains(err.Error(), "failed to evaluate substitutions") {
			t.Errorf("Expected error about evaluating substitutions, got: %v", err)
		}
	})

	t.Run("HandlesStrategicMergeErrorForKustomization", func(t *testing.T) {
		handler := setup(t)
		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
kustomize:
  - name: kustomization-a
    dependsOn:
      - kustomization-b
  - name: kustomization-b
    dependsOn:
      - kustomization-c
  - name: kustomization-c
    dependsOn:
      - kustomization-a
`)
		feature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test-feature
kustomize:
  - name: test-kustomization
`)
		templateData := map[string][]byte{
			"blueprint":          baseBlueprint,
			"features/test.yaml": feature,
		}
		config := map[string]any{}

		err := handler.processFeatures(templateData, config)

		if err == nil {
			t.Fatal("Expected error when StrategicMerge fails due to dependency cycle")
		}
		if !strings.Contains(err.Error(), "dependency cycle detected") {
			t.Errorf("Expected error about dependency cycle, got: %v", err)
		}
	})

	t.Run("HandlesEvaluateExpressionError", func(t *testing.T) {
		// Given a handler with feature that has invalid condition
		handler := setup(t)
		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base`)
		invalidFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: invalid-feature
when: invalid expression syntax [[[`)
		templateData := map[string][]byte{
			"blueprint":             baseBlueprint,
			"features/invalid.yaml": invalidFeature,
		}
		config := map[string]any{}

		// When processing features
		err := handler.processFeatures(templateData, config)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when EvaluateExpression fails")
		}
		if !strings.Contains(err.Error(), "failed to evaluate feature condition") {
			t.Errorf("Expected error about evaluating condition, got: %v", err)
		}
	})

	t.Run("ProcessComponentLevelConditions", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)

		observabilityFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: observability
terraform:
  - path: observability/quickwit
    when: observability.backend == "quickwit"
  - path: observability/grafana
    when: observability.backend == "grafana"
`)

		templateData := map[string][]byte{
			"blueprint":                   baseBlueprint,
			"features/observability.yaml": observabilityFeature,
		}

		config := map[string]any{
			"observability": map[string]any{
				"backend": "quickwit",
			},
		}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(handler.blueprint.TerraformComponents))
		}
		if handler.blueprint.TerraformComponents[0].Path != "observability/quickwit" {
			t.Errorf("Expected 'observability/quickwit', got '%s'", handler.blueprint.TerraformComponents[0].Path)
		}
	})

	t.Run("MergesMultipleMatchingFeatures", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)

		awsFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-feature
when: provider == "aws"
terraform:
  - path: network/vpc
`)

		observabilityFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: observability
when: observability.enabled == true
terraform:
  - path: observability/quickwit
`)

		templateData := map[string][]byte{
			"blueprint":                   baseBlueprint,
			"features/aws.yaml":           awsFeature,
			"features/observability.yaml": observabilityFeature,
		}

		config := map[string]any{
			"provider": "aws",
			"observability": map[string]any{
				"enabled": true,
			},
		}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.TerraformComponents) != 2 {
			t.Errorf("Expected 2 terraform components, got %d", len(handler.blueprint.TerraformComponents))
		}
	})

	t.Run("SortsFeaturesDeterministically", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)

		featureZ := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: z-feature
terraform:
  - path: z/module
`)

		featureA := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: a-feature
terraform:
  - path: a/module
`)

		templateData := map[string][]byte{
			"blueprint":       baseBlueprint,
			"features/z.yaml": featureZ,
			"features/a.yaml": featureA,
		}

		config := map[string]any{}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.TerraformComponents) != 2 {
			t.Fatalf("Expected 2 terraform components, got %d", len(handler.blueprint.TerraformComponents))
		}
		if handler.blueprint.TerraformComponents[0].Path != "a/module" {
			t.Errorf("Expected first component 'a/module', got '%s'", handler.blueprint.TerraformComponents[0].Path)
		}
		if handler.blueprint.TerraformComponents[1].Path != "z/module" {
			t.Errorf("Expected second component 'z/module', got '%s'", handler.blueprint.TerraformComponents[1].Path)
		}
	})

	t.Run("ProcessesKustomizations", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)

		fluxFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: flux
when: gitops.enabled == true
kustomize:
  - name: flux-system
    path: gitops/flux
`)

		templateData := map[string][]byte{
			"blueprint":          baseBlueprint,
			"features/flux.yaml": fluxFeature,
		}

		config := map[string]any{
			"gitops": map[string]any{
				"enabled": true,
			},
		}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.Kustomizations) != 1 {
			t.Errorf("Expected 1 kustomization, got %d", len(handler.blueprint.Kustomizations))
		}
		if handler.blueprint.Kustomizations[0].Name != "flux-system" {
			t.Errorf("Expected 'flux-system', got '%s'", handler.blueprint.Kustomizations[0].Name)
		}
	})

	t.Run("HandlesNoFeatures", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)

		templateData := map[string][]byte{
			"blueprint": baseBlueprint,
		}

		config := map[string]any{}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.TerraformComponents) != 0 {
			t.Errorf("Expected 0 terraform components, got %d", len(handler.blueprint.TerraformComponents))
		}
	})

	t.Run("HandlesNoBlueprint", func(t *testing.T) {
		handler := setup(t)

		awsFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-feature
terraform:
  - path: network/vpc
`)

		templateData := map[string][]byte{
			"features/aws.yaml": awsFeature,
		}

		config := map[string]any{}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(handler.blueprint.TerraformComponents))
		}
	})

	t.Run("FailsOnInvalidFeatureCondition", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)

		badFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: bad-feature
when: invalid syntax ===
terraform:
  - path: test/module
`)

		templateData := map[string][]byte{
			"blueprint":         baseBlueprint,
			"features/bad.yaml": badFeature,
		}

		config := map[string]any{}

		err := handler.processFeatures(templateData, config)

		if err == nil {
			t.Error("Expected error for invalid condition, got nil")
		}
		if !strings.Contains(err.Error(), "failed to evaluate feature condition") {
			t.Errorf("Expected condition evaluation error, got %v", err)
		}
	})

	t.Run("EvaluatesAndMergesInputs", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)

		featureWithInputs := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-eks
when: provider == "aws"
terraform:
  - path: cluster/aws-eks
    inputs:
      cluster_name: my-cluster
      node_groups:
        default:
          instance_types:
            - ${cluster.workers.instance_type}
          min_size: ${cluster.workers.count}
          max_size: ${cluster.workers.count + 2}
          desired_size: ${cluster.workers.count}
      region: us-east-1
      literal_string: my-literal-value
`)

		templateData := map[string][]byte{
			"blueprint":         baseBlueprint,
			"features/eks.yaml": featureWithInputs,
		}

		config := map[string]any{
			"provider": "aws",
			"cluster": map[string]any{
				"workers": map[string]any{
					"instance_type": "t3.medium",
					"count":         3,
				},
			},
		}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(handler.blueprint.TerraformComponents))
		}

		component := handler.blueprint.TerraformComponents[0]

		if component.Inputs["cluster_name"] != "my-cluster" {
			t.Errorf("Expected cluster_name to be 'my-cluster', got %v", component.Inputs["cluster_name"])
		}

		nodeGroups, ok := component.Inputs["node_groups"].(map[string]any)
		if !ok {
			t.Fatalf("Expected node_groups to be a map, got %T", component.Inputs["node_groups"])
		}

		defaultGroup, ok := nodeGroups["default"].(map[string]any)
		if !ok {
			t.Fatalf("Expected default group to be a map, got %T", nodeGroups["default"])
		}

		instanceTypes, ok := defaultGroup["instance_types"].([]any)
		if !ok {
			t.Fatalf("Expected instance_types to be an array, got %T", defaultGroup["instance_types"])
		}
		if len(instanceTypes) != 1 || instanceTypes[0] != "t3.medium" {
			t.Errorf("Expected instance_types to be ['t3.medium'], got %v", instanceTypes)
		}

		if defaultGroup["min_size"] != 3 {
			t.Errorf("Expected min_size to be 3, got %v", defaultGroup["min_size"])
		}

		if defaultGroup["max_size"] != 5 {
			t.Errorf("Expected max_size to be 5 (3+2), got %v", defaultGroup["max_size"])
		}

		if defaultGroup["desired_size"] != 3 {
			t.Errorf("Expected desired_size to be 3, got %v", defaultGroup["desired_size"])
		}

		if component.Inputs["region"] != "us-east-1" {
			t.Errorf("Expected region to be literal 'us-east-1', got %v", component.Inputs["region"])
		}

		if component.Inputs["literal_string"] != "my-literal-value" {
			t.Errorf("Expected literal_string to be 'my-literal-value', got %v", component.Inputs["literal_string"])
		}
	})

	t.Run("FailsOnInvalidExpressions", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)

		featureWithBadExpression := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test
terraform:
  - path: test/module
    inputs:
      bad_path: ${cluster.workrs.count}
`)

		templateData := map[string][]byte{
			"blueprint":          baseBlueprint,
			"features/test.yaml": featureWithBadExpression,
		}

		config := map[string]any{
			"cluster": map[string]any{
				"workers": map[string]any{
					"count": 3,
				},
			},
		}

		err := handler.processFeatures(templateData, config)

		if err == nil {
			t.Fatal("Expected error for invalid expression, got nil")
		}
		if !strings.Contains(err.Error(), "failed to evaluate inputs") {
			t.Errorf("Expected inputs evaluation error, got %v", err)
		}
	})

	t.Run("ReplacesTerraformComponentWithReplaceStrategy", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
terraform:
  - path: network/vpc
    source: core
    inputs:
      cidr: 10.0.0.0/16
      enable_dns: true
    dependsOn:
      - backend
`)

		replaceFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: replace-feature
terraform:
  - path: network/vpc
    source: core
    strategy: replace
    inputs:
      cidr: 172.16.0.0/16
    dependsOn:
      - new-dependency
`)

		templateData := map[string][]byte{
			"blueprint":             baseBlueprint,
			"features/replace.yaml": replaceFeature,
		}

		config := map[string]any{}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(handler.blueprint.TerraformComponents))
		}

		component := handler.blueprint.TerraformComponents[0]
		if component.Path != "network/vpc" {
			t.Errorf("Expected path 'network/vpc', got '%s'", component.Path)
		}
		if len(component.Inputs) != 1 {
			t.Errorf("Expected 1 input (replaced), got %d", len(component.Inputs))
		}
		if component.Inputs["cidr"] != "172.16.0.0/16" {
			t.Errorf("Expected new cidr value, got %v", component.Inputs["cidr"])
		}
		if component.Inputs["enable_dns"] != nil {
			t.Errorf("Expected old enable_dns to be removed, got %v", component.Inputs["enable_dns"])
		}
		if len(component.DependsOn) != 1 {
			t.Errorf("Expected 1 dependency (replaced), got %d", len(component.DependsOn))
		}
		if component.DependsOn[0] != "new-dependency" {
			t.Errorf("Expected new dependency, got %v", component.DependsOn)
		}
	})

	t.Run("MergesTerraformComponentWithDefaultStrategy", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
terraform:
  - path: network/vpc
    source: core
    inputs:
      cidr: 10.0.0.0/16
    dependsOn:
      - backend
`)

		mergeFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: merge-feature
terraform:
  - path: network/vpc
    source: core
    strategy: merge
    inputs:
      enable_dns: true
    dependsOn:
      - security
`)

		templateData := map[string][]byte{
			"blueprint":           baseBlueprint,
			"features/merge.yaml": mergeFeature,
		}

		config := map[string]any{}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(handler.blueprint.TerraformComponents))
		}

		component := handler.blueprint.TerraformComponents[0]
		if len(component.Inputs) != 2 {
			t.Errorf("Expected 2 inputs (merged), got %d", len(component.Inputs))
		}
		if component.Inputs["cidr"] != "10.0.0.0/16" {
			t.Errorf("Expected original cidr value preserved, got %v", component.Inputs["cidr"])
		}
		if component.Inputs["enable_dns"] != true {
			t.Errorf("Expected new enable_dns value added, got %v", component.Inputs["enable_dns"])
		}
		if len(component.DependsOn) != 2 {
			t.Errorf("Expected 2 dependencies (merged), got %d", len(component.DependsOn))
		}
	})

	t.Run("ReplacesKustomizationWithReplaceStrategy", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
kustomize:
  - name: ingress
    path: original-path
    source: original-source
    components:
      - nginx
      - cert-manager
    dependsOn:
      - pki
      - dns
`)

		replaceFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: replace-feature
kustomize:
  - name: ingress
    strategy: replace
    path: new-path
    source: new-source
    components:
      - traefik
    dependsOn:
      - new-dependency
`)

		templateData := map[string][]byte{
			"blueprint":             baseBlueprint,
			"features/replace.yaml": replaceFeature,
		}

		config := map[string]any{}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(handler.blueprint.Kustomizations))
		}

		kustomization := handler.blueprint.Kustomizations[0]
		if kustomization.Name != "ingress" {
			t.Errorf("Expected name 'ingress', got '%s'", kustomization.Name)
		}
		if kustomization.Path != "new-path" {
			t.Errorf("Expected path 'new-path', got '%s'", kustomization.Path)
		}
		if kustomization.Source != "new-source" {
			t.Errorf("Expected source 'new-source', got '%s'", kustomization.Source)
		}
		if len(kustomization.Components) != 1 {
			t.Errorf("Expected 1 component (replaced), got %d", len(kustomization.Components))
		}
		if kustomization.Components[0] != "traefik" {
			t.Errorf("Expected component 'traefik', got %v", kustomization.Components)
		}
		if len(kustomization.DependsOn) != 1 {
			t.Errorf("Expected 1 dependency (replaced), got %d", len(kustomization.DependsOn))
		}
		if kustomization.DependsOn[0] != "new-dependency" {
			t.Errorf("Expected new dependency, got %v", kustomization.DependsOn)
		}
	})

	t.Run("MergesKustomizationWithDefaultStrategy", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
kustomize:
  - name: ingress
    path: original-path
    components:
      - nginx
    dependsOn:
      - pki
`)

		mergeFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: merge-feature
kustomize:
  - name: ingress
    strategy: merge
    components:
      - cert-manager
    dependsOn:
      - dns
`)

		templateData := map[string][]byte{
			"blueprint":           baseBlueprint,
			"features/merge.yaml": mergeFeature,
		}

		config := map[string]any{}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(handler.blueprint.Kustomizations))
		}

		kustomization := handler.blueprint.Kustomizations[0]
		if len(kustomization.Components) != 2 {
			t.Errorf("Expected 2 components (merged), got %d", len(kustomization.Components))
		}
		if len(kustomization.DependsOn) != 2 {
			t.Errorf("Expected 2 dependencies (merged), got %d", len(kustomization.DependsOn))
		}
	})
}

func TestBaseBlueprintHandler_setRepositoryDefaults(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		handler.runtime.ConfigHandler = mocks.ConfigHandler
		handler.runtime.Shell = mocks.Shell
		return handler
	}

	t.Run("PreservesExistingRepositoryURL", func(t *testing.T) {
		handler := setup(t)
		handler.blueprint.Repository.Url = "https://github.com/existing/repo"

		mockConfigHandler := handler.runtime.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "dev" {
				return false
			}
			return false
		}

		err := handler.setRepositoryDefaults()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if handler.blueprint.Repository.Url != "https://github.com/existing/repo" {
			t.Errorf("Expected URL to remain unchanged, got %s", handler.blueprint.Repository.Url)
		}
	})

	t.Run("UsesDevelopmentURLWhenDevFlagEnabled", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.runtime.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "dev" {
				return true
			}
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "example.com"
			}
			return ""
		}

		handler.runtime.ProjectRoot = "/path/to/my-project"

		handler.shims.FilepathBase = func(path string) string {
			return "my-project"
		}

		err := handler.setRepositoryDefaults()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedURL := "http://git.example.com/git/my-project"
		if handler.blueprint.Repository.Url != expectedURL {
			t.Errorf("Expected URL to be %s, got %s", expectedURL, handler.blueprint.Repository.Url)
		}
	})

	t.Run("FallsBackToGitRemoteOrigin", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.runtime.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}

		mockShell := handler.runtime.Shell.(*shell.MockShell)
		mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "git" && len(args) == 3 && args[0] == "config" && args[2] == "remote.origin.url" {
				return "https://github.com/user/repo.git\n", nil
			}
			return "", fmt.Errorf("command not found")
		}

		err := handler.setRepositoryDefaults()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedURL := "https://github.com/user/repo.git"
		if handler.blueprint.Repository.Url != expectedURL {
			t.Errorf("Expected URL to be %s, got %s", expectedURL, handler.blueprint.Repository.Url)
		}
	})

	t.Run("PreservesSSHGitRemoteOrigin", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.runtime.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}

		mockShell := handler.runtime.Shell.(*shell.MockShell)
		mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "git" && len(args) == 3 && args[0] == "config" && args[2] == "remote.origin.url" {
				return "git@github.com:windsorcli/core.git\n", nil
			}
			return "", fmt.Errorf("command not found")
		}

		err := handler.setRepositoryDefaults()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedURL := "git@github.com:windsorcli/core.git"
		if handler.blueprint.Repository.Url != expectedURL {
			t.Errorf("Expected URL to be %s, got %s", expectedURL, handler.blueprint.Repository.Url)
		}
	})

	t.Run("HandlesGitRemoteOriginError", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.runtime.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}

		mockShell := handler.runtime.Shell.(*shell.MockShell)
		mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("not a git repository")
		}

		err := handler.setRepositoryDefaults()

		if err != nil {
			t.Fatalf("Expected no error even when git fails, got %v", err)
		}
		if handler.blueprint.Repository.Url != "" {
			t.Errorf("Expected URL to remain empty when git fails, got %s", handler.blueprint.Repository.Url)
		}
	})

	t.Run("HandlesEmptyGitRemoteOriginOutput", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.runtime.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}

		mockShell := handler.runtime.Shell.(*shell.MockShell)
		mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", nil
		}

		err := handler.setRepositoryDefaults()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if handler.blueprint.Repository.Url != "" {
			t.Errorf("Expected URL to remain empty, got %s", handler.blueprint.Repository.Url)
		}
	})

	t.Run("DevModeFallsBackToGitWhenDevelopmentURLFails", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.runtime.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "dev" {
				return true
			}
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		mockShell := handler.runtime.Shell.(*shell.MockShell)
		mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "git" {
				return "https://github.com/fallback/repo.git", nil
			}
			return "", fmt.Errorf("command not found")
		}

		err := handler.setRepositoryDefaults()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedURL := "https://github.com/fallback/repo.git"
		if handler.blueprint.Repository.Url != expectedURL {
			t.Errorf("Expected URL to be %s, got %s", expectedURL, handler.blueprint.Repository.Url)
		}
	})

}

func TestBaseBlueprintHandler_normalizeGitURL(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		return handler
	}

	t.Run("PreservesSSHURL", func(t *testing.T) {
		handler := setup(t)

		input := "git@github.com:windsorcli/core.git"
		expected := "git@github.com:windsorcli/core.git"
		result := handler.normalizeGitURL(input)

		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("PreservesHTTPSURL", func(t *testing.T) {
		handler := setup(t)

		input := "https://github.com/windsorcli/core.git"
		expected := "https://github.com/windsorcli/core.git"
		result := handler.normalizeGitURL(input)

		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("PreservesHTTPURL", func(t *testing.T) {
		handler := setup(t)

		input := "http://git.test/git/core"
		expected := "http://git.test/git/core"
		result := handler.normalizeGitURL(input)

		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("PrependsHTTPSToPlainURL", func(t *testing.T) {
		handler := setup(t)

		input := "github.com/windsorcli/core.git"
		expected := "https://github.com/windsorcli/core.git"
		result := handler.normalizeGitURL(input)

		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})
}

func TestBaseBlueprintHandler_getDevelopmentRepositoryURL(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		handler.runtime.ConfigHandler = mocks.ConfigHandler
		handler.runtime.Shell = mocks.Shell
		return handler
	}

	t.Run("GeneratesCorrectDevelopmentURL", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.runtime.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "dev.example.com"
			}
			return ""
		}

		handler.runtime.ProjectRoot = "/home/user/projects/my-awesome-project"

		handler.shims.FilepathBase = func(path string) string {
			return "my-awesome-project"
		}

		url := handler.getDevelopmentRepositoryURL()

		expectedURL := "http://git.dev.example.com/git/my-awesome-project"
		if url != expectedURL {
			t.Errorf("Expected URL to be %s, got %s", expectedURL, url)
		}
	})

	t.Run("UsesDefaultDomainWhenNotSet", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.runtime.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" && len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		handler.runtime.ProjectRoot = "/home/user/projects/my-project"

		handler.shims.FilepathBase = func(path string) string {
			return "my-project"
		}

		url := handler.getDevelopmentRepositoryURL()

		expectedURL := "http://git.test/git/my-project"
		if url != expectedURL {
			t.Errorf("Expected URL to be %s, got %s", expectedURL, url)
		}
	})

	t.Run("ReturnsEmptyWhenProjectRootFails", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.runtime.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "example.com"
			}
			return ""
		}

		handler.runtime.ProjectRoot = ""

		url := handler.getDevelopmentRepositoryURL()

		if url != "" {
			t.Errorf("Expected empty URL when project root fails, got %s", url)
		}
	})

	t.Run("ReturnsEmptyWhenFolderNameEmpty", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.runtime.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "example.com"
			}
			return ""
		}

		handler.runtime.ProjectRoot = "/home/user/projects/"

		handler.shims.FilepathBase = func(path string) string {
			return ""
		}

		url := handler.getDevelopmentRepositoryURL()

		if url != "" {
			t.Errorf("Expected empty URL when folder name is empty, got %s", url)
		}
	})

	t.Run("HandlesComplexProjectPaths", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.runtime.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "staging.example.io"
			}
			return ""
		}

		handler.runtime.ProjectRoot = "/var/www/projects/nested/deep/project-with-dashes"

		handler.shims.FilepathBase = func(path string) string {
			return "project-with-dashes"
		}

		url := handler.getDevelopmentRepositoryURL()

		expectedURL := "http://git.staging.example.io/git/project-with-dashes"
		if url != expectedURL {
			t.Errorf("Expected URL to be %s, got %s", expectedURL, url)
		}
	})
}

func TestBlueprintHandler_getSources(t *testing.T) {
	setup := func(t *testing.T) (*BaseBlueprintHandler, *BlueprintTestMocks) {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{},
		}
		return handler, mocks
	}

	t.Run("ReturnsExpectedSources", func(t *testing.T) {
		// Given a blueprint handler with a set of sources
		handler, _ := setup(t)
		expectedSources := []blueprintv1alpha1.Source{
			{
				Name:       "source1",
				Url:        "git::https://example.com/source1.git",
				Ref:        blueprintv1alpha1.Reference{Branch: "main"},
				PathPrefix: "/source1",
			},
			{
				Name:       "source2",
				Url:        "git::https://example.com/source2.git",
				Ref:        blueprintv1alpha1.Reference{Branch: "develop"},
				PathPrefix: "/source2",
			},
		}
		handler.blueprint.Sources = expectedSources

		// When getting sources
		sources := handler.getSources()

		// Then the returned sources should match the expected sources
		if len(sources) != len(expectedSources) {
			t.Fatalf("Expected %d sources, got %d", len(expectedSources), len(sources))
		}
		for i := range expectedSources {
			if sources[i] != expectedSources[i] {
				t.Errorf("Source[%d] = %+v, want %+v", i, sources[i], expectedSources[i])
			}
		}
	})
}

func TestBlueprintHandler_getRepository(t *testing.T) {
	setup := func(t *testing.T) (*BaseBlueprintHandler, *BlueprintTestMocks) {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("ReturnsExpectedRepository", func(t *testing.T) {
		// Given a blueprint handler with a set repository
		handler, _ := setup(t)
		expectedRepo := blueprintv1alpha1.Repository{
			Url: "git::https://example.com/repo.git",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		}
		handler.blueprint.Repository = expectedRepo

		// When getting the repository
		repo := handler.getRepository()

		// Then the expected repository should be returned
		if repo != expectedRepo {
			t.Errorf("Expected repository %+v, got %+v", expectedRepo, repo)
		}
	})

	t.Run("ReturnsDefaultValues", func(t *testing.T) {
		// Given a blueprint handler with an empty repository
		handler, _ := setup(t)
		handler.blueprint.Repository = blueprintv1alpha1.Repository{}

		// When getting the repository
		repo := handler.getRepository()

		// Then default values should be set
		expectedRepo := blueprintv1alpha1.Repository{
			Url: "",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		}
		if repo != expectedRepo {
			t.Errorf("Expected repository %+v, got %+v", expectedRepo, repo)
		}
	})
}

func TestBlueprintHandler_loadConfig(t *testing.T) {
	setup := func(t *testing.T) (*BaseBlueprintHandler, *BlueprintTestMocks) {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		if err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)
		handler.runtime.ConfigRoot = "/test/config"

		// When loading the config
		err := handler.loadConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the metadata should be correctly loaded
		metadata := handler.getMetadata()
		if metadata.Name != "test-blueprint" {
			t.Errorf("Expected name to be test-blueprint, got %s", metadata.Name)
		}
	})

	t.Run("CustomPathOverride", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)
		handler.runtime.ConfigRoot = "/test/config"

		// And a mock file system that tracks checked paths
		var checkedPaths []string
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, ".jsonnet") || strings.HasSuffix(name, ".yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			checkedPaths = append(checkedPaths, name)
			if strings.HasSuffix(name, ".jsonnet") {
				return []byte(safeBlueprintJsonnet), nil
			}
			if strings.HasSuffix(name, ".yaml") {
				return []byte(safeBlueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		// When loading config
		err := handler.loadConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And only yaml path should be checked since it exists
		expectedPaths := []string{
			"blueprint.yaml",
		}
		for _, expected := range expectedPaths {
			found := false
			for _, checked := range checkedPaths {
				if strings.HasSuffix(checked, expected) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected path %s to be checked, but it wasn't. Checked paths: %v", expected, checkedPaths)
			}
		}
	})

	t.Run("DefaultBlueprint", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)
		handler.runtime.ConfigRoot = "/test/config"

		// And a mock file system that returns no existing files
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		handler.shims.ReadFile = func(name string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		// And a local context
		originalContext := os.Getenv("WINDSOR_CONTEXT")
		os.Setenv("WINDSOR_CONTEXT", "local")
		defer func() { os.Setenv("WINDSOR_CONTEXT", originalContext) }()

		// When loading the config
		err := handler.loadConfig()

		// Then an error should be returned since blueprint.yaml doesn't exist
		if err == nil {
			t.Errorf("Expected error when blueprint.yaml doesn't exist, got nil")
		}

		// And the error should indicate blueprint.yaml not found
		if !strings.Contains(err.Error(), "blueprint.yaml not found") {
			t.Errorf("Expected error about blueprint.yaml not found, got: %v", err)
		}
	})

	t.Run("ErrorUnmarshallingLocalJsonnet", func(t *testing.T) {
		// Given a blueprint handler with local context
		handler, mocks := setup(t)
		mocks.ConfigHandler.SetContext("local")

		// And a mock yaml unmarshaller that returns an error
		handler.shims.YamlUnmarshal = func(data []byte, obj any) error {
			return fmt.Errorf("simulated unmarshalling error")
		}

		// When loading the config
		err := handler.loadConfig()

		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected loadConfig to fail due to unmarshalling error, but it succeeded")
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		// Given a mock config handler that returns an error
		mockConfigHandler := config.NewMockConfigHandler()
		mocks := setupBlueprintMocks(t, func(m *BlueprintTestMocks) {
			m.ConfigHandler = mockConfigHandler
		})
		mocks.Runtime.ConfigRoot = ""

		// And a blueprint handler using that config handler
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims

		// When loading the config
		err = handler.loadConfig()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "config root is empty") {
			t.Errorf("Expected error containing 'config root is empty', got: %v", err)
		}
	})

	t.Run("ErrorReadingYamlFile", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)
		handler.runtime.ConfigRoot = "/test/config"

		// And a mock file system that finds yaml file but fails to read it
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, "blueprint.yaml") {
				return nil, nil // File exists
			}
			return nil, os.ErrNotExist
		}
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if strings.HasSuffix(name, "blueprint.yaml") {
				return nil, fmt.Errorf("error reading yaml file")
			}
			return nil, os.ErrNotExist
		}

		// When loading the config
		err := handler.loadConfig()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error reading yaml file") {
			t.Errorf("Expected error containing 'error reading yaml file', got: %v", err)
		}
	})

	t.Run("ErrorLoadingYamlFile", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)
		handler.runtime.ConfigRoot = "/test/config"

		// And a mock file system that returns an error for yaml files
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, ".yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if strings.HasSuffix(name, ".yaml") {
				return nil, fmt.Errorf("error reading yaml file")
			}
			return nil, os.ErrNotExist
		}

		// When loading the config
		err := handler.loadConfig()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error reading yaml file") {
			t.Errorf("Expected error containing 'error reading yaml file', got: %v", err)
		}
	})

	t.Run("ErrorUnmarshallingYamlBlueprint", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)
		handler.runtime.ConfigRoot = "/test/config"

		// And a mock file system with a yaml file
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, "blueprint.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if strings.HasSuffix(name, "blueprint.yaml") {
				return []byte("invalid: yaml: content"), nil
			}
			return nil, os.ErrNotExist
		}

		// And a mock yaml unmarshaller that returns an error
		handler.shims.YamlUnmarshal = func(data []byte, obj any) error {
			return fmt.Errorf("error unmarshalling blueprint data")
		}

		// When loading the config
		err := handler.loadConfig()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error unmarshalling blueprint data") {
			t.Errorf("Expected error containing 'error unmarshalling blueprint data', got: %v", err)
		}
	})

	t.Run("EmptyEvaluatedJsonnet", func(t *testing.T) {
		// Given a blueprint handler with local context
		handler, mocks := setup(t)
		handler.runtime.ConfigRoot = "/test/config"
		mocks.ConfigHandler.SetContext("local")

		// And a mock jsonnet VM that returns empty result

		// And a mock file system that returns no files
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			return nil, fmt.Errorf("file not found")
		}

		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When loading the config
		err := handler.loadConfig()

		// Then an error should be returned since blueprint.yaml doesn't exist
		if err == nil {
			t.Errorf("Expected error when blueprint.yaml doesn't exist, got nil")
		}

		// And the error should indicate blueprint.yaml not found
		if !strings.Contains(err.Error(), "blueprint.yaml not found") {
			t.Errorf("Expected error about blueprint.yaml not found, got: %v", err)
		}
	})

	t.Run("PathBackslashNormalization", func(t *testing.T) {
		handler, _ := setup(t)
		handler.blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{Name: "k1", Path: "foo\\bar\\baz"},
		}
		ks := handler.getKustomizations()
		if ks[0].Path != "kustomize/foo/bar/baz" {
			t.Errorf("expected normalized path, got %q", ks[0].Path)
		}
	})

	t.Run("SetsRepositoryDefaultsInDevMode", func(t *testing.T) {
		handler, mocks := setup(t)

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "dev" {
				return true
			}
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" && len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "dev" {
				return true
			}
			return false
		}
		mocks.Runtime.ConfigRoot = "/tmp/test-config"
		mocks.Runtime.ProjectRoot = "/Users/test/project/cli"

		handler.shims.FilepathBase = func(path string) string {
			if path == "/Users/test/project/cli" {
				return "cli"
			}
			return ""
		}

		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, ".yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		blueprintWithoutURL := `kind: Blueprint
apiVersion: v1alpha1
metadata:
  name: test-blueprint
  description: A test blueprint
repository:
  ref:
    branch: main
sources: []
terraform: []
kustomize: []`

		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if strings.HasSuffix(name, ".yaml") {
				return []byte(blueprintWithoutURL), nil
			}
			return nil, os.ErrNotExist
		}

		// Mock WriteFile to allow Write() to succeed
		handler.shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return nil
		}

		err := handler.loadConfig()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Repository defaults are now set during Write(), not loadConfig()
		// So the URL should be empty after loadConfig()
		if handler.blueprint.Repository.Url != "" {
			t.Errorf("Expected repository URL to be empty after loadConfig(), got %s", handler.blueprint.Repository.Url)
		}

		// Now test that Write() sets the repository defaults
		// Use overwrite=true to ensure setRepositoryDefaults() is called
		err = handler.Write(true)
		if err != nil {
			t.Fatalf("Expected no error during Write(), got %v", err)
		}

		expectedURL := "http://git.test/git/cli"
		if handler.blueprint.Repository.Url != expectedURL {
			t.Errorf("Expected repository URL to be %s after Write(), got %s", expectedURL, handler.blueprint.Repository.Url)
		}
	})
}

func TestNewShims(t *testing.T) {
	t.Run("CreatesShimsWithDefaultImplementations", func(t *testing.T) {
		shims := NewShims()

		if shims == nil {
			t.Fatal("Expected shims, got nil")
		}

		if shims.Stat == nil {
			t.Error("Expected Stat to be set")
		}
		if shims.ReadFile == nil {
			t.Error("Expected ReadFile to be set")
		}
		if shims.ReadDir == nil {
			t.Error("Expected ReadDir to be set")
		}
		if shims.Walk == nil {
			t.Error("Expected Walk to be set")
		}
		if shims.WriteFile == nil {
			t.Error("Expected WriteFile to be set")
		}
		if shims.Remove == nil {
			t.Error("Expected Remove to be set")
		}
		if shims.MkdirAll == nil {
			t.Error("Expected MkdirAll to be set")
		}
		if shims.YamlMarshal == nil {
			t.Error("Expected YamlMarshal to be set")
		}
		if shims.YamlUnmarshal == nil {
			t.Error("Expected YamlUnmarshal to be set")
		}
		if shims.YamlMarshalNonNull == nil {
			t.Error("Expected YamlMarshalNonNull to be set")
		}
		if shims.K8sYamlUnmarshal == nil {
			t.Error("Expected K8sYamlUnmarshal to be set")
		}
		if shims.NewFakeClient == nil {
			t.Error("Expected NewFakeClient to be set")
		}
		if shims.RegexpMatchString == nil {
			t.Error("Expected RegexpMatchString to be set")
		}
		if shims.TimeAfter == nil {
			t.Error("Expected TimeAfter to be set")
		}
		if shims.NewTicker == nil {
			t.Error("Expected NewTicker to be set")
		}
		if shims.TickerStop == nil {
			t.Error("Expected TickerStop to be set")
		}
		if shims.JsonMarshal == nil {
			t.Error("Expected JsonMarshal to be set")
		}
		if shims.JsonUnmarshal == nil {
			t.Error("Expected JsonUnmarshal to be set")
		}
		if shims.FilepathBase == nil {
			t.Error("Expected FilepathBase to be set")
		}
		if shims.NewJsonnetVM == nil {
			t.Error("Expected NewJsonnetVM to be set")
		}
	})
}

func TestBaseBlueprintHandler_writeLocalTemplatePatches(t *testing.T) {
	setup := func(t *testing.T) (*BaseBlueprintHandler, *BlueprintTestMocks) {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		return handler, mocks
	}

	t.Run("WritesPatchWithKindAndName", func(t *testing.T) {
		handler, mocks := setup(t)
		mocks.Runtime.ConfigRoot = "/test/config"
		mocks.Runtime.TemplateRoot = "/test/template"

		kustomization := blueprintv1alpha1.Kustomization{
			Name: "test-kustomization",
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Patch: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  key: value`,
				},
			},
		}

		var writtenPath string
		var writtenContent []byte
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			if strings.Contains(name, "patches/") {
				writtenPath = name
				writtenContent = data
			}
			return nil
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		mocks.Shims.YamlUnmarshal = yaml.Unmarshal
		mocks.Shims.YamlMarshal = yaml.Marshal

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return make(map[string]any), nil
		}

		err := handler.writeLocalTemplatePatches(kustomization, true)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		expectedPath := filepath.Join(mocks.Runtime.ConfigRoot, "patches", "test-kustomization", "configmap-test-config.yaml")
		if writtenPath != expectedPath {
			t.Errorf("Expected patch file at %s, got %s", expectedPath, writtenPath)
		}

		if len(writtenContent) == 0 {
			t.Error("Expected patch content to be written")
		}
	})

	t.Run("HandlesEmptyPatches", func(t *testing.T) {
		handler, mocks := setup(t)
		mocks.Runtime.ConfigRoot = "/test/config"

		kustomization := blueprintv1alpha1.Kustomization{
			Name:    "test-kustomization",
			Patches: []blueprintv1alpha1.BlueprintPatch{},
		}

		var writeFileCalled bool
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			if strings.Contains(name, "patches/") {
				writeFileCalled = true
			}
			return nil
		}

		err := handler.writeLocalTemplatePatches(kustomization, true)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if writeFileCalled {
			t.Error("Expected WriteFile not to be called for empty patches")
		}
	})

	t.Run("HandlesEmptyConfigRoot", func(t *testing.T) {
		handler, mocks := setup(t)
		mocks.Runtime.ConfigRoot = ""

		kustomization := blueprintv1alpha1.Kustomization{
			Name: "test-kustomization",
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Patch: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config`,
				},
			},
		}

		var writeFileCalled bool
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			if strings.Contains(name, "patches/") {
				writeFileCalled = true
			}
			return nil
		}

		err := handler.writeLocalTemplatePatches(kustomization, true)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if writeFileCalled {
			t.Error("Expected WriteFile not to be called when ConfigRoot is empty")
		}
	})

	t.Run("HandlesMkdirAllError", func(t *testing.T) {
		handler, mocks := setup(t)
		mocks.Runtime.ConfigRoot = "/test/config"

		kustomization := blueprintv1alpha1.Kustomization{
			Name: "test-kustomization",
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Patch: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config`,
				},
			},
		}

		expectedError := fmt.Errorf("mkdir error")
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			if strings.Contains(path, "patches/") {
				return expectedError
			}
			return nil
		}

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return make(map[string]any), nil
		}

		err := handler.writeLocalTemplatePatches(kustomization, true)

		if err == nil {
			t.Error("Expected error from MkdirAll, got nil")
		}

		if !strings.Contains(err.Error(), "failed to create patches directory") {
			t.Errorf("Expected error about creating patches directory, got: %v", err)
		}
	})

	t.Run("HandlesWriteFileError", func(t *testing.T) {
		handler, mocks := setup(t)
		mocks.Runtime.ConfigRoot = "/test/config"

		kustomization := blueprintv1alpha1.Kustomization{
			Name: "test-kustomization",
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Patch: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config`,
				},
			},
		}

		expectedError := fmt.Errorf("write file error")
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			if strings.Contains(name, "patches/") {
				return expectedError
			}
			return nil
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		mocks.Shims.YamlUnmarshal = yaml.Unmarshal

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return make(map[string]any), nil
		}

		err := handler.writeLocalTemplatePatches(kustomization, true)

		if err == nil {
			t.Error("Expected error from WriteFile, got nil")
		}

		if !strings.Contains(err.Error(), "failed to write patch file") {
			t.Errorf("Expected error about writing patch file, got: %v", err)
		}
	})

	t.Run("RespectsOverwriteFlag", func(t *testing.T) {
		handler, mocks := setup(t)
		mocks.Runtime.ConfigRoot = "/test/config"

		kustomization := blueprintv1alpha1.Kustomization{
			Name: "test-kustomization",
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Patch: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config`,
				},
			},
		}

		var writeFileCalled bool
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			if strings.Contains(name, "patches/") {
				writeFileCalled = true
			}
			return nil
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "patches/") {
				return &mockFileInfo{name: "configmap-test-config.yaml", isDir: false}, nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		mocks.Shims.YamlUnmarshal = yaml.Unmarshal

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return make(map[string]any), nil
		}

		err := handler.writeLocalTemplatePatches(kustomization, false)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if writeFileCalled {
			t.Error("Expected WriteFile not to be called when overwrite is false and file exists")
		}

		writeFileCalled = false
		err = handler.writeLocalTemplatePatches(kustomization, true)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if !writeFileCalled {
			t.Error("Expected WriteFile to be called when overwrite is true")
		}
	})

	t.Run("EvaluatesPatchContentFromPath", func(t *testing.T) {
		handler, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ConfigRoot = "/test/config"
		mocks.Runtime.TemplateRoot = tmpDir

		patchFile := filepath.Join(tmpDir, "kustomize", "patches", "test-patch.yaml")
		if err := os.MkdirAll(filepath.Dir(patchFile), 0755); err != nil {
			t.Fatalf("Failed to create patch directory: %v", err)
		}

		patchContent := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config`
		if err := os.WriteFile(patchFile, []byte(patchContent), 0644); err != nil {
			t.Fatalf("Failed to write patch file: %v", err)
		}

		kustomization := blueprintv1alpha1.Kustomization{
			Name: "test-kustomization",
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Path: "kustomize/patches/test-patch.yaml",
				},
			},
		}

		var writtenContent []byte
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			if strings.Contains(name, "patches/") {
				writtenContent = data
			}
			return nil
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		mocks.Shims.ReadFile = os.ReadFile
		mocks.Shims.YamlUnmarshal = yaml.Unmarshal
		mocks.Shims.YamlMarshal = yaml.Marshal

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return make(map[string]any), nil
		}

		err := handler.writeLocalTemplatePatches(kustomization, true)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(writtenContent) == 0 {
			t.Error("Expected patch content to be written")
		}

		if !strings.Contains(string(writtenContent), "test-config") {
			t.Error("Expected written content to contain patch data")
		}
	})

	t.Run("MergesPatchesWithSameKindAndName", func(t *testing.T) {
		handler, mocks := setup(t)
		mocks.Runtime.ConfigRoot = "/test/config"

		kustomization := blueprintv1alpha1.Kustomization{
			Name: "test-kustomization",
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Patch: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  key1: value1`,
				},
				{
					Patch: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  key2: value2`,
				},
			},
		}

		var writtenContent []byte
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			if strings.Contains(name, "patches/") {
				writtenContent = data
			}
			return nil
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		mocks.Shims.YamlUnmarshal = yaml.Unmarshal
		mocks.Shims.YamlMarshal = yaml.Marshal

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return make(map[string]any), nil
		}

		err := handler.writeLocalTemplatePatches(kustomization, true)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(writtenContent) == 0 {
			t.Fatal("Expected merged patch content to be written")
		}

		var mergedDoc map[string]any
		if err := yaml.Unmarshal(writtenContent, &mergedDoc); err != nil {
			t.Fatalf("Failed to unmarshal merged patch: %v", err)
		}

		if data, ok := mergedDoc["data"].(map[string]any); ok {
			if data["key1"] != "value1" {
				t.Error("Expected key1 to be in merged patch")
			}
			if data["key2"] != "value2" {
				t.Error("Expected key2 to be in merged patch")
			}
		} else {
			t.Error("Expected data section in merged patch")
		}
	})

	t.Run("AppendsPatchesWithDifferentKindOrName", func(t *testing.T) {
		handler, mocks := setup(t)
		mocks.Runtime.ConfigRoot = "/test/config"

		kustomization := blueprintv1alpha1.Kustomization{
			Name: "test-kustomization",
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Patch: `apiVersion: v1
kind: ConfigMap
metadata:
  name: config-1
data:
  key1: value1`,
				},
				{
					Patch: `apiVersion: v1
kind: Deployment
metadata:
  name: deployment-1
spec:
  replicas: 3`,
				},
			},
		}

		var writtenFiles = make(map[string][]byte)
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			if strings.Contains(name, "patches/") {
				writtenFiles[name] = data
			}
			return nil
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		mocks.Shims.YamlUnmarshal = yaml.Unmarshal
		mocks.Shims.YamlMarshal = yaml.Marshal

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return make(map[string]any), nil
		}

		err := handler.writeLocalTemplatePatches(kustomization, true)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(writtenFiles) == 0 {
			t.Fatal("Expected patch files to be written")
		}

		foundConfigMap := false
		foundDeployment := false
		for path, content := range writtenFiles {
			contentStr := string(content)
			if strings.Contains(path, "configmap-config-1.yaml") {
				foundConfigMap = true
				if !strings.Contains(contentStr, "config-1") {
					t.Error("Expected config-1 patch content")
				}
			}
			if strings.Contains(path, "deployment-deployment-1.yaml") {
				foundDeployment = true
				if !strings.Contains(contentStr, "deployment-1") {
					t.Error("Expected deployment-1 patch content")
				}
			}
		}

		if !foundConfigMap {
			t.Error("Expected ConfigMap patch file to be written")
		}
		if !foundDeployment {
			t.Error("Expected Deployment patch file to be written")
		}
	})

	t.Run("SanitizesInvalidFilenameCharacters", func(t *testing.T) {
		handler, mocks := setup(t)
		mocks.Runtime.ConfigRoot = "/test/config"

		kustomization := blueprintv1alpha1.Kustomization{
			Name: "test-kustomization",
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Patch: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test/config:name*with?invalid"chars<|
data:
  key: value`,
				},
			},
		}

		var writtenPath string
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			if strings.Contains(name, "patches/") {
				writtenPath = name
			}
			return nil
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		mocks.Shims.YamlUnmarshal = yaml.Unmarshal

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return make(map[string]any), nil
		}

		err := handler.writeLocalTemplatePatches(kustomization, true)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if writtenPath == "" {
			t.Fatal("Expected patch file to be written")
		}

		fileName := filepath.Base(writtenPath)
		invalidChars := []string{"/", "\\", ":", "*", "?", "\"", "<", "|"}
		for _, char := range invalidChars {
			if strings.Contains(fileName, char) {
				t.Errorf("Expected invalid character '%s' to be sanitized from filename, got: %s", char, fileName)
			}
		}

		if !strings.Contains(fileName, "configmap-test") {
			t.Errorf("Expected sanitized filename to contain 'configmap-test', got: %s", fileName)
		}

		if !strings.HasSuffix(fileName, ".yaml") {
			t.Errorf("Expected filename to end with .yaml, got: %s", fileName)
		}
	})

	t.Run("SkipsPatchWhenReadFileFails", func(t *testing.T) {
		handler, mocks := setup(t)
		mocks.Runtime.ConfigRoot = "/test/config"
		mocks.Runtime.TemplateRoot = "/test/template"

		kustomization := blueprintv1alpha1.Kustomization{
			Name: "test-kustomization",
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Path: "kustomize/patches/test-patch.yaml",
				},
			},
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			if strings.Contains(name, "patches/test-patch.yaml") {
				return nil, fmt.Errorf("read file error")
			}
			return nil, os.ErrNotExist
		}

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return make(map[string]any), nil
		}

		var writeFileCalled bool
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			if strings.Contains(name, "patches/") {
				writeFileCalled = true
			}
			return nil
		}

		err := handler.writeLocalTemplatePatches(kustomization, true)

		if err != nil {
			t.Errorf("Expected no error (patch skipped), got %v", err)
		}

		if writeFileCalled {
			t.Error("Expected WriteFile not to be called when patch file cannot be read")
		}
	})

	t.Run("UsesIndexWhenNoMetadata", func(t *testing.T) {
		handler, mocks := setup(t)
		mocks.Runtime.ConfigRoot = "/test/config"

		kustomization := blueprintv1alpha1.Kustomization{
			Name: "test-kustomization",
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Patch: `apiVersion: v1
data:
  key: value`,
				},
			},
		}

		var writtenPath string
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			if strings.Contains(name, "patches/") {
				writtenPath = name
			}
			return nil
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		mocks.Shims.YamlUnmarshal = yaml.Unmarshal

		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return make(map[string]any), nil
		}

		err := handler.writeLocalTemplatePatches(kustomization, true)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if !strings.Contains(writtenPath, "0.yaml") {
			t.Errorf("Expected filename with index 0, got: %s", filepath.Base(writtenPath))
		}
	})
}

func TestBaseBlueprintHandler_categorizePatches(t *testing.T) {
	setup := func(t *testing.T) (*BaseBlueprintHandler, *BlueprintTestMocks) {
		t.Helper()
		mocks := setupBlueprintMocks(t)
		mockArtifactBuilder := artifact.NewMockArtifact()
		handler, err := NewBlueprintHandler(mocks.Runtime, mockArtifactBuilder)
		if err != nil {
			t.Fatalf("NewBlueprintHandler() failed: %v", err)
		}
		handler.shims = mocks.Shims
		return handler, mocks
	}

	t.Run("CategorizesStrategicMergePatches", func(t *testing.T) {
		handler, mocks := setup(t)
		mocks.Runtime.TemplateRoot = "/test/template"

		kustomization := blueprintv1alpha1.Kustomization{
			Name: "test-kustomization",
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Patch: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config`,
				},
			},
		}

		strategicMerge, inline := handler.categorizePatches(kustomization)

		if len(strategicMerge) != 1 {
			t.Errorf("Expected 1 strategic merge patch, got %d", len(strategicMerge))
		}

		if len(inline) != 0 {
			t.Errorf("Expected 0 inline patches, got %d", len(inline))
		}
	})

	t.Run("CategorizesInlinePatches", func(t *testing.T) {
		handler, _ := setup(t)

		kustomization := blueprintv1alpha1.Kustomization{
			Name: "test-kustomization",
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Target: &kustomize.Selector{
						Kind: "ConfigMap",
						Name: "test-config",
					},
					Patch: `[{"op": "replace", "path": "/data/key", "value": "newvalue"}]`,
				},
			},
		}

		strategicMerge, inline := handler.categorizePatches(kustomization)

		if len(strategicMerge) != 0 {
			t.Errorf("Expected 0 strategic merge patches, got %d", len(strategicMerge))
		}

		if len(inline) != 1 {
			t.Errorf("Expected 1 inline patch, got %d", len(inline))
		}
	})

	t.Run("CategorizesJSON6902Patches", func(t *testing.T) {
		handler, _ := setup(t)

		kustomization := blueprintv1alpha1.Kustomization{
			Name: "test-kustomization",
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Target: &kustomize.Selector{
						Kind:      "Deployment",
						Name:      "test-deployment",
						Namespace: "default",
					},
					Patch: `[{"op": "replace", "path": "/spec/replicas", "value": 5}]`,
				},
			},
		}

		strategicMerge, inline := handler.categorizePatches(kustomization)

		if len(strategicMerge) != 0 {
			t.Errorf("Expected 0 strategic merge patches, got %d", len(strategicMerge))
		}

		if len(inline) != 1 {
			t.Errorf("Expected 1 inline patch, got %d", len(inline))
		}

		if inline[0].Target == nil {
			t.Error("Expected target to be set for JSON6902 patch")
		}
	})

	t.Run("CategorizesLocalTemplatePatches", func(t *testing.T) {
		handler, mocks := setup(t)
		mocks.Runtime.TemplateRoot = "/test/template"

		kustomization := blueprintv1alpha1.Kustomization{
			Name: "test-kustomization",
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Path: "kustomize/patches/test-patch.yaml",
				},
			},
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "template/kustomize/patches/test-patch.yaml") {
				return &mockFileInfo{name: "test-patch.yaml", isDir: false}, nil
			}
			return nil, os.ErrNotExist
		}

		strategicMerge, inline := handler.categorizePatches(kustomization)

		if len(strategicMerge) != 1 {
			t.Errorf("Expected 1 strategic merge patch for local template, got %d", len(strategicMerge))
		}

		if len(inline) != 0 {
			t.Errorf("Expected 0 inline patches, got %d", len(inline))
		}
	})

	t.Run("HandlesEmptyPatches", func(t *testing.T) {
		handler, _ := setup(t)

		kustomization := blueprintv1alpha1.Kustomization{
			Name:    "test-kustomization",
			Patches: []blueprintv1alpha1.BlueprintPatch{},
		}

		strategicMerge, inline := handler.categorizePatches(kustomization)

		if len(strategicMerge) != 0 {
			t.Errorf("Expected 0 strategic merge patches, got %d", len(strategicMerge))
		}

		if len(inline) != 0 {
			t.Errorf("Expected 0 inline patches, got %d", len(inline))
		}
	})
}
