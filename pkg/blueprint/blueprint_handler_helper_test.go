package blueprint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	kustomize "github.com/fluxcd/pkg/apis/kustomize"
	"github.com/windsorcli/cli/api/v1alpha1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/secrets"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mockConfigHandler struct{}

func (m *mockConfigHandler) Initialize() error                                            { return nil }
func (m *mockConfigHandler) LoadConfig(path string) error                                 { return nil }
func (m *mockConfigHandler) LoadConfigString(content string) error                        { return nil }
func (m *mockConfigHandler) LoadContextConfig() error                                     { return nil }
func (m *mockConfigHandler) GetString(key string, defaultValue ...string) string          { return "" }
func (m *mockConfigHandler) GetInt(key string, defaultValue ...int) int                   { return 0 }
func (m *mockConfigHandler) GetBool(key string, defaultValue ...bool) bool                { return false }
func (m *mockConfigHandler) GetStringSlice(key string, defaultValue ...[]string) []string { return nil }
func (m *mockConfigHandler) GetStringMap(key string, defaultValue ...map[string]string) map[string]string {
	return nil
}
func (m *mockConfigHandler) Set(key string, value any) error                     { return nil }
func (m *mockConfigHandler) SetContextValue(key string, value any) error         { return nil }
func (m *mockConfigHandler) Get(key string) any                                  { return nil }
func (m *mockConfigHandler) SaveConfig(overwrite ...bool) error                  { return nil }
func (m *mockConfigHandler) SetDefault(context v1alpha1.Context) error           { return nil }
func (m *mockConfigHandler) GetConfig() *v1alpha1.Context                        { return nil }
func (m *mockConfigHandler) GetContext() string                                  { return "test-context" }
func (m *mockConfigHandler) SetContext(context string) error                     { return nil }
func (m *mockConfigHandler) GetConfigRoot() (string, error)                      { return "/tmp", nil }
func (m *mockConfigHandler) Clean() error                                        { return nil }
func (m *mockConfigHandler) IsLoaded() bool                                      { return true }
func (m *mockConfigHandler) SetSecretsProvider(provider secrets.SecretsProvider) {}
func (m *mockConfigHandler) GenerateContextID() error                            { return nil }
func (m *mockConfigHandler) YamlMarshalWithDefinedPaths(v any) ([]byte, error)   { return nil, nil }

// =============================================================================
// Test Helper Functions
// =============================================================================

func TestTLACode(t *testing.T) {
	// Given a mock Jsonnet VM that returns an error about missing authors
	vm := NewMockJsonnetVM(func(filename, snippet string) (string, error) {
		return "", fmt.Errorf("blueprint has no authors")
	})

	// When evaluating an empty snippet
	_, err := vm.EvaluateAnonymousSnippet("test.jsonnet", "")

	// Then an error about missing authors should be returned
	if err == nil || !strings.Contains(err.Error(), "blueprint has no authors") {
		t.Errorf("expected error containing 'blueprint has no authors', got %v", err)
	}
}

func TestBaseBlueprintHandler_calculateMaxWaitTime(t *testing.T) {
	t.Run("EmptyKustomizations", func(t *testing.T) {
		// Given a blueprint handler with no kustomizations
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{},
			},
		}

		// When calculating max wait time
		waitTime := handler.calculateMaxWaitTime()

		// Then it should return 0 since there are no kustomizations
		if waitTime != 0 {
			t.Errorf("expected 0 duration, got %v", waitTime)
		}
	})

	t.Run("SingleKustomization", func(t *testing.T) {
		// Given a blueprint handler with a single kustomization
		customTimeout := 2 * time.Minute
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "test-kustomization",
						Timeout: &metav1.Duration{
							Duration: customTimeout,
						},
					},
				},
			},
			shims:         NewShims(),
			configHandler: &mockConfigHandler{},
			projectRoot:   "/tmp",
		}

		// When calculating max wait time
		waitTime := handler.calculateMaxWaitTime()

		// Then it should return the kustomization's timeout
		if waitTime != customTimeout {
			t.Errorf("expected timeout %v, got %v", customTimeout, waitTime)
		}
	})

	t.Run("LinearDependencies", func(t *testing.T) {
		// Given a blueprint handler with linear dependencies
		timeout1 := 1 * time.Minute
		timeout2 := 2 * time.Minute
		timeout3 := 3 * time.Minute
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "kustomization-1",
						Timeout: &metav1.Duration{
							Duration: timeout1,
						},
						DependsOn: []string{"kustomization-2"},
					},
					{
						Name: "kustomization-2",
						Timeout: &metav1.Duration{
							Duration: timeout2,
						},
						DependsOn: []string{"kustomization-3"},
					},
					{
						Name: "kustomization-3",
						Timeout: &metav1.Duration{
							Duration: timeout3,
						},
					},
				},
			},
			shims:         NewShims(),
			configHandler: &mockConfigHandler{},
			projectRoot:   "/tmp",
		}

		// When calculating max wait time
		waitTime := handler.calculateMaxWaitTime()

		// Then it should return the sum of all timeouts
		expectedTime := timeout1 + timeout2 + timeout3
		if waitTime != expectedTime {
			t.Errorf("expected timeout %v, got %v", expectedTime, waitTime)
		}
	})

	t.Run("BranchingDependencies", func(t *testing.T) {
		// Given a blueprint handler with branching dependencies
		timeout1 := 1 * time.Minute
		timeout2 := 2 * time.Minute
		timeout3 := 3 * time.Minute
		timeout4 := 4 * time.Minute
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "kustomization-1",
						Timeout: &metav1.Duration{
							Duration: timeout1,
						},
						DependsOn: []string{"kustomization-2", "kustomization-3"},
					},
					{
						Name: "kustomization-2",
						Timeout: &metav1.Duration{
							Duration: timeout2,
						},
						DependsOn: []string{"kustomization-4"},
					},
					{
						Name: "kustomization-3",
						Timeout: &metav1.Duration{
							Duration: timeout3,
						},
						DependsOn: []string{"kustomization-4"},
					},
					{
						Name: "kustomization-4",
						Timeout: &metav1.Duration{
							Duration: timeout4,
						},
					},
				},
			},
			shims:         NewShims(),
			configHandler: &mockConfigHandler{},
			projectRoot:   "/tmp",
		}

		// When calculating max wait time
		waitTime := handler.calculateMaxWaitTime()

		// Then it should return the longest path (1 -> 3 -> 4)
		expectedTime := timeout1 + timeout3 + timeout4
		if waitTime != expectedTime {
			t.Errorf("expected timeout %v, got %v", expectedTime, waitTime)
		}
	})

	t.Run("CircularDependencies", func(t *testing.T) {
		// Given a blueprint handler with circular dependencies
		timeout1 := 1 * time.Minute
		timeout2 := 2 * time.Minute
		timeout3 := 3 * time.Minute
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "kustomization-1",
						Timeout: &metav1.Duration{
							Duration: timeout1,
						},
						DependsOn: []string{"kustomization-2"},
					},
					{
						Name: "kustomization-2",
						Timeout: &metav1.Duration{
							Duration: timeout2,
						},
						DependsOn: []string{"kustomization-3"},
					},
					{
						Name: "kustomization-3",
						Timeout: &metav1.Duration{
							Duration: timeout3,
						},
						DependsOn: []string{"kustomization-1"},
					},
				},
			},
			shims:         NewShims(),
			configHandler: &mockConfigHandler{},
			projectRoot:   "/tmp",
		}

		// When calculating max wait time
		waitTime := handler.calculateMaxWaitTime()

		// Then it should return the sum of all timeouts in the cycle (1+2+3+3)
		expectedTime := timeout1 + timeout2 + timeout3 + timeout3
		if waitTime != expectedTime {
			t.Errorf("expected timeout %v, got %v", expectedTime, waitTime)
		}
	})
}

func TestBaseBlueprintHandler_discoverKustomizationPatches(t *testing.T) {
	t.Run("DirectoryNotExists", func(t *testing.T) {
		// Given a blueprint handler with a non-existent patches directory
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: &mockConfigHandler{},
			projectRoot:   "/tmp",
		}

		// When discovering patches for a kustomization
		patches, err := handler.discoverKustomizationPatches("test-kustomization")

		// Then it should return no patches and no error
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if len(patches) != 0 {
			t.Errorf("expected no patches, got %d", len(patches))
		}
	})

	t.Run("ValidPatchFile", func(t *testing.T) {
		// Given a blueprint handler with a valid patch file
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: &mockConfigHandler{},
			projectRoot:   "/tmp",
		}

		// Create a temporary directory structure
		tempDir := t.TempDir()
		patchesDir := filepath.Join(tempDir, "contexts", "test-context", "patches", "test-kustomization")
		if err := os.MkdirAll(patchesDir, 0755); err != nil {
			t.Fatalf("failed to create patches directory: %v", err)
		}

		// Create a valid patch file
		patchContent := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  key: value`
		patchFile := filepath.Join(patchesDir, "test-patch.yaml")
		if err := os.WriteFile(patchFile, []byte(patchContent), 0644); err != nil {
			t.Fatalf("failed to write patch file: %v", err)
		}

		// Override project root for this test
		handler.projectRoot = tempDir

		// When discovering patches for the kustomization
		patches, err := handler.discoverKustomizationPatches("test-kustomization")

		// Then it should return the patch
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if len(patches) != 1 {
			t.Errorf("expected 1 patch, got %d", len(patches))
		}
		if patches[0].Patch != patchContent {
			t.Errorf("expected patch content to match, got %s", patches[0].Patch)
		}
	})

	t.Run("InvalidYAMLFile", func(t *testing.T) {
		// Given a blueprint handler with an invalid YAML file
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: &mockConfigHandler{},
			projectRoot:   "/tmp",
		}

		// Create a temporary directory structure
		tempDir := t.TempDir()
		patchesDir := filepath.Join(tempDir, "contexts", "test-context", "patches", "test-kustomization")
		if err := os.MkdirAll(patchesDir, 0755); err != nil {
			t.Fatalf("failed to create patches directory: %v", err)
		}

		// Create an invalid YAML file
		invalidContent := `invalid: yaml: content:`
		patchFile := filepath.Join(patchesDir, "invalid-patch.yaml")
		if err := os.WriteFile(patchFile, []byte(invalidContent), 0644); err != nil {
			t.Fatalf("failed to write patch file: %v", err)
		}

		// Override project root for this test
		handler.projectRoot = tempDir

		// When discovering patches for the kustomization
		_, err := handler.discoverKustomizationPatches("test-kustomization")

		// Then it should return an error
		if err == nil {
			t.Error("expected error for invalid YAML, got nil")
		}
		if !strings.Contains(err.Error(), "invalid YAML") {
			t.Errorf("expected error about invalid YAML, got %v", err)
		}
	})

	t.Run("MissingKindField", func(t *testing.T) {
		// Given a blueprint handler with a patch file missing kind field
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: &mockConfigHandler{},
			projectRoot:   "/tmp",
		}

		// Create a temporary directory structure
		tempDir := t.TempDir()
		patchesDir := filepath.Join(tempDir, "contexts", "test-context", "patches", "test-kustomization")
		if err := os.MkdirAll(patchesDir, 0755); err != nil {
			t.Fatalf("failed to create patches directory: %v", err)
		}

		// Create a patch file missing kind field
		patchContent := `apiVersion: v1
metadata:
  name: test-config
data:
  key: value`
		patchFile := filepath.Join(patchesDir, "missing-kind.yaml")
		if err := os.WriteFile(patchFile, []byte(patchContent), 0644); err != nil {
			t.Fatalf("failed to write patch file: %v", err)
		}

		// Override project root for this test
		handler.projectRoot = tempDir

		// When discovering patches for the kustomization
		_, err := handler.discoverKustomizationPatches("test-kustomization")

		// Then it should return an error
		if err == nil {
			t.Error("expected error for missing kind field, got nil")
		}
		if !strings.Contains(err.Error(), "missing or invalid 'kind' field") {
			t.Errorf("expected error about missing kind field, got %v", err)
		}
	})

	t.Run("MissingMetadataField", func(t *testing.T) {
		// Given a blueprint handler with a patch file missing metadata field
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: &mockConfigHandler{},
			projectRoot:   "/tmp",
		}

		// Create a temporary directory structure
		tempDir := t.TempDir()
		patchesDir := filepath.Join(tempDir, "contexts", "test-context", "patches", "test-kustomization")
		if err := os.MkdirAll(patchesDir, 0755); err != nil {
			t.Fatalf("failed to create patches directory: %v", err)
		}

		// Create a patch file missing metadata field
		patchContent := `apiVersion: v1
kind: ConfigMap
data:
  key: value`
		patchFile := filepath.Join(patchesDir, "missing-metadata.yaml")
		if err := os.WriteFile(patchFile, []byte(patchContent), 0644); err != nil {
			t.Fatalf("failed to write patch file: %v", err)
		}

		// Override project root for this test
		handler.projectRoot = tempDir

		// When discovering patches for the kustomization
		_, err := handler.discoverKustomizationPatches("test-kustomization")

		// Then it should return an error
		if err == nil {
			t.Error("expected error for missing metadata field, got nil")
		}
		if !strings.Contains(err.Error(), "missing 'metadata' field") {
			t.Errorf("expected error about missing metadata field, got %v", err)
		}
	})

	t.Run("MissingNameField", func(t *testing.T) {
		// Given a blueprint handler with a patch file missing name field
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: &mockConfigHandler{},
			projectRoot:   "/tmp",
		}

		// Create a temporary directory structure
		tempDir := t.TempDir()
		patchesDir := filepath.Join(tempDir, "contexts", "test-context", "patches", "test-kustomization")
		if err := os.MkdirAll(patchesDir, 0755); err != nil {
			t.Fatalf("failed to create patches directory: %v", err)
		}

		// Create a patch file missing name field
		patchContent := `apiVersion: v1
kind: ConfigMap
metadata:
  labels:
    app: test
data:
  key: value`
		patchFile := filepath.Join(patchesDir, "missing-name.yaml")
		if err := os.WriteFile(patchFile, []byte(patchContent), 0644); err != nil {
			t.Fatalf("failed to write patch file: %v", err)
		}

		// Override project root for this test
		handler.projectRoot = tempDir

		// When discovering patches for the kustomization
		_, err := handler.discoverKustomizationPatches("test-kustomization")

		// Then it should return an error
		if err == nil {
			t.Error("expected error for missing name field, got nil")
		}
		if !strings.Contains(err.Error(), "missing or invalid 'metadata.name' field") {
			t.Errorf("expected error about missing name field, got %v", err)
		}
	})

	t.Run("NonYAMLFile", func(t *testing.T) {
		// Given a blueprint handler with a non-YAML file
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: &mockConfigHandler{},
			projectRoot:   "/tmp",
		}

		// Create a temporary directory structure
		tempDir := t.TempDir()
		patchesDir := filepath.Join(tempDir, "contexts", "test-context", "patches", "test-kustomization")
		if err := os.MkdirAll(patchesDir, 0755); err != nil {
			t.Fatalf("failed to create patches directory: %v", err)
		}

		// Create a non-YAML file
		nonYamlContent := `This is not YAML content`
		nonYamlFile := filepath.Join(patchesDir, "not-yaml.txt")
		if err := os.WriteFile(nonYamlFile, []byte(nonYamlContent), 0644); err != nil {
			t.Fatalf("failed to write non-YAML file: %v", err)
		}

		// Override project root for this test
		handler.projectRoot = tempDir

		// When discovering patches for the kustomization
		patches, err := handler.discoverKustomizationPatches("test-kustomization")

		// Then it should return no patches and no error (non-YAML files are ignored)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if len(patches) != 0 {
			t.Errorf("expected no patches, got %d", len(patches))
		}
	})
}

func TestBaseBlueprintHandler_getKustomizations(t *testing.T) {
	t.Run("NoKustomizations", func(t *testing.T) {
		// Given a blueprint handler with no kustomizations
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: &mockConfigHandler{},
			projectRoot:   "/tmp",
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: nil,
			},
		}

		// When getting kustomizations
		result := handler.getKustomizations()

		// Then it should return nil
		if result != nil {
			t.Errorf("expected nil result, got %v", result)
		}
	})

	t.Run("KustomizationWithNoPatches", func(t *testing.T) {
		// Given a blueprint handler with a kustomization that has no patches
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: &mockConfigHandler{},
			projectRoot:   "/tmp",
			blueprint: blueprintv1alpha1.Blueprint{
				Metadata: blueprintv1alpha1.Metadata{
					Name: "test-blueprint",
				},
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "test-kustomization",
					},
				},
			},
		}

		// When getting kustomizations
		result := handler.getKustomizations()

		// Then it should return the kustomization with default values
		if len(result) != 1 {
			t.Fatalf("expected 1 kustomization, got %d", len(result))
		}
		if result[0].Name != "test-kustomization" {
			t.Errorf("expected name 'test-kustomization', got %s", result[0].Name)
		}
		if result[0].Source != "test-blueprint" {
			t.Errorf("expected source 'test-blueprint', got %s", result[0].Source)
		}
		if result[0].Path != "kustomize" {
			t.Errorf("expected path 'kustomize', got %s", result[0].Path)
		}
		if result[0].Patches != nil {
			t.Errorf("expected nil patches, got %v", result[0].Patches)
		}
	})

	t.Run("KustomizationWithExistingPatches", func(t *testing.T) {
		// Given a blueprint handler with a kustomization that has existing patches
		existingPatches := []kustomize.Patch{
			{Patch: "existing-patch-1"},
			{Patch: "existing-patch-2"},
		}
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: &mockConfigHandler{},
			projectRoot:   "/tmp",
			blueprint: blueprintv1alpha1.Blueprint{
				Metadata: blueprintv1alpha1.Metadata{
					Name: "test-blueprint",
				},
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name:    "test-kustomization",
						Patches: existingPatches,
					},
				},
			},
		}

		// When getting kustomizations
		result := handler.getKustomizations()

		// Then it should return the kustomization with existing patches preserved
		if len(result) != 1 {
			t.Fatalf("expected 1 kustomization, got %d", len(result))
		}
		if len(result[0].Patches) != 2 {
			t.Errorf("expected 2 patches, got %d", len(result[0].Patches))
		}
		if result[0].Patches[0].Patch != "existing-patch-1" {
			t.Errorf("expected first patch 'existing-patch-1', got %s", result[0].Patches[0].Patch)
		}
		if result[0].Patches[1].Patch != "existing-patch-2" {
			t.Errorf("expected second patch 'existing-patch-2', got %s", result[0].Patches[1].Patch)
		}
	})

	t.Run("KustomizationWithDiscoveredPatches", func(t *testing.T) {
		// Given a blueprint handler with a kustomization that will have discovered patches
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: &mockConfigHandler{},
			projectRoot:   "/tmp",
			blueprint: blueprintv1alpha1.Blueprint{
				Metadata: blueprintv1alpha1.Metadata{
					Name: "test-blueprint",
				},
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "test-kustomization",
					},
				},
			},
		}

		// Create a temporary directory structure with patch files
		tempDir := t.TempDir()
		patchesDir := filepath.Join(tempDir, "contexts", "test-context", "patches", "test-kustomization")
		if err := os.MkdirAll(patchesDir, 0755); err != nil {
			t.Fatalf("failed to create patches directory: %v", err)
		}

		// Create patch files
		patch1Content := `apiVersion: v1
kind: ConfigMap
metadata:
  name: discovered-patch-1
data:
  key1: value1`
		patch1File := filepath.Join(patchesDir, "patch1.yaml")
		if err := os.WriteFile(patch1File, []byte(patch1Content), 0644); err != nil {
			t.Fatalf("failed to write patch file: %v", err)
		}

		patch2Content := `apiVersion: v1
kind: ConfigMap
metadata:
  name: discovered-patch-2
data:
  key2: value2`
		patch2File := filepath.Join(patchesDir, "patch2.yaml")
		if err := os.WriteFile(patch2File, []byte(patch2Content), 0644); err != nil {
			t.Fatalf("failed to write patch file: %v", err)
		}

		// Override project root for this test
		handler.projectRoot = tempDir

		// When getting kustomizations
		result := handler.getKustomizations()

		// Then it should return the kustomization with discovered patches
		if len(result) != 1 {
			t.Fatalf("expected 1 kustomization, got %d", len(result))
		}
		if len(result[0].Patches) != 2 {
			t.Errorf("expected 2 discovered patches, got %d", len(result[0].Patches))
		}
		if result[0].Patches[0].Patch != patch1Content {
			t.Errorf("expected first patch content to match, got %s", result[0].Patches[0].Patch)
		}
		if result[0].Patches[1].Patch != patch2Content {
			t.Errorf("expected second patch content to match, got %s", result[0].Patches[1].Patch)
		}
	})

	t.Run("KustomizationWithExistingAndDiscoveredPatches", func(t *testing.T) {
		// Given a blueprint handler with a kustomization that has both existing and discovered patches
		existingPatches := []kustomize.Patch{
			{Patch: "existing-patch-1"},
			{Patch: "existing-patch-2"},
		}
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: &mockConfigHandler{},
			projectRoot:   "/tmp",
			blueprint: blueprintv1alpha1.Blueprint{
				Metadata: blueprintv1alpha1.Metadata{
					Name: "test-blueprint",
				},
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name:    "test-kustomization",
						Patches: existingPatches,
					},
				},
			},
		}

		// Create a temporary directory structure with patch files
		tempDir := t.TempDir()
		patchesDir := filepath.Join(tempDir, "contexts", "test-context", "patches", "test-kustomization")
		if err := os.MkdirAll(patchesDir, 0755); err != nil {
			t.Fatalf("failed to create patches directory: %v", err)
		}

		// Create discovered patch file
		discoveredPatchContent := `apiVersion: v1
kind: ConfigMap
metadata:
  name: discovered-patch
data:
  key: value`
		discoveredPatchFile := filepath.Join(patchesDir, "discovered-patch.yaml")
		if err := os.WriteFile(discoveredPatchFile, []byte(discoveredPatchContent), 0644); err != nil {
			t.Fatalf("failed to write patch file: %v", err)
		}

		// Override project root for this test
		handler.projectRoot = tempDir

		// When getting kustomizations
		result := handler.getKustomizations()

		// Then it should return the kustomization with both existing and discovered patches
		if len(result) != 1 {
			t.Fatalf("expected 1 kustomization, got %d", len(result))
		}
		if len(result[0].Patches) != 3 {
			t.Errorf("expected 3 patches (2 existing + 1 discovered), got %d", len(result[0].Patches))
		}
		// Existing patches should come first
		if result[0].Patches[0].Patch != "existing-patch-1" {
			t.Errorf("expected first patch 'existing-patch-1', got %s", result[0].Patches[0].Patch)
		}
		if result[0].Patches[1].Patch != "existing-patch-2" {
			t.Errorf("expected second patch 'existing-patch-2', got %s", result[0].Patches[1].Patch)
		}
		// Discovered patch should come last
		if result[0].Patches[2].Patch != discoveredPatchContent {
			t.Errorf("expected third patch to match discovered content, got %s", result[0].Patches[2].Patch)
		}
	})

	t.Run("KustomizationWithPatchDiscoveryError", func(t *testing.T) {
		// Given a blueprint handler with a kustomization that will have patch discovery errors
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: &mockConfigHandler{},
			projectRoot:   "/tmp",
			blueprint: blueprintv1alpha1.Blueprint{
				Metadata: blueprintv1alpha1.Metadata{
					Name: "test-blueprint",
				},
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "test-kustomization",
					},
				},
			},
		}

		// Create a temporary directory structure with invalid patch files
		tempDir := t.TempDir()
		patchesDir := filepath.Join(tempDir, "contexts", "test-context", "patches", "test-kustomization")
		if err := os.MkdirAll(patchesDir, 0755); err != nil {
			t.Fatalf("failed to create patches directory: %v", err)
		}

		// Create an invalid patch file (missing required fields)
		invalidPatchContent := `apiVersion: v1
# Missing kind and metadata
data:
  key: value`
		invalidPatchFile := filepath.Join(patchesDir, "invalid-patch.yaml")
		if err := os.WriteFile(invalidPatchFile, []byte(invalidPatchContent), 0644); err != nil {
			t.Fatalf("failed to write patch file: %v", err)
		}

		// Override project root for this test
		handler.projectRoot = tempDir

		// When getting kustomizations
		result := handler.getKustomizations()

		// Then it should return the kustomization without patches (error is logged but not returned)
		if len(result) != 1 {
			t.Fatalf("expected 1 kustomization, got %d", len(result))
		}
		if result[0].Patches != nil {
			t.Errorf("expected nil patches due to discovery error, got %v", result[0].Patches)
		}
	})

	t.Run("MultipleKustomizations", func(t *testing.T) {
		// Given a blueprint handler with multiple kustomizations
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: &mockConfigHandler{},
			projectRoot:   "/tmp",
			blueprint: blueprintv1alpha1.Blueprint{
				Metadata: blueprintv1alpha1.Metadata{
					Name: "test-blueprint",
				},
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "kustomization-1",
						Patches: []kustomize.Patch{
							{Patch: "existing-patch-1"},
						},
					},
					{
						Name: "kustomization-2",
					},
				},
			},
		}

		// Create a temporary directory structure with patch files for second kustomization
		tempDir := t.TempDir()
		patchesDir := filepath.Join(tempDir, "contexts", "test-context", "patches", "kustomization-2")
		if err := os.MkdirAll(patchesDir, 0755); err != nil {
			t.Fatalf("failed to create patches directory: %v", err)
		}

		// Create patch file for second kustomization
		patchContent := `apiVersion: v1
kind: ConfigMap
metadata:
  name: discovered-patch
data:
  key: value`
		patchFile := filepath.Join(patchesDir, "discovered-patch.yaml")
		if err := os.WriteFile(patchFile, []byte(patchContent), 0644); err != nil {
			t.Fatalf("failed to write patch file: %v", err)
		}

		// Override project root for this test
		handler.projectRoot = tempDir

		// When getting kustomizations
		result := handler.getKustomizations()

		// Then it should return both kustomizations with appropriate patches
		if len(result) != 2 {
			t.Fatalf("expected 2 kustomizations, got %d", len(result))
		}

		// First kustomization should have existing patch
		if result[0].Name != "kustomization-1" {
			t.Errorf("expected first kustomization name 'kustomization-1', got %s", result[0].Name)
		}
		if len(result[0].Patches) != 1 {
			t.Errorf("expected 1 patch for first kustomization, got %d", len(result[0].Patches))
		}
		if result[0].Patches[0].Patch != "existing-patch-1" {
			t.Errorf("expected first kustomization patch 'existing-patch-1', got %s", result[0].Patches[0].Patch)
		}

		// Second kustomization should have discovered patch
		if result[1].Name != "kustomization-2" {
			t.Errorf("expected second kustomization name 'kustomization-2', got %s", result[1].Name)
		}
		if len(result[1].Patches) != 1 {
			t.Errorf("expected 1 patch for second kustomization, got %d", len(result[1].Patches))
		}
		if result[1].Patches[0].Patch != patchContent {
			t.Errorf("expected second kustomization patch to match discovered content, got %s", result[1].Patches[0].Patch)
		}
	})
}

func TestBaseBlueprintHandler_loadFileData(t *testing.T) {
	t.Run("func", func(t *testing.T) {
		// Test cases will go here
	})
}
