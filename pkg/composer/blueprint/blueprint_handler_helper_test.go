package blueprint

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// =============================================================================
// Test Helper Functions
// =============================================================================

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
			configHandler: config.NewMockConfigHandler(),
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
			configHandler: config.NewMockConfigHandler(),
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
			configHandler: config.NewMockConfigHandler(),
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
			configHandler: config.NewMockConfigHandler(),
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

func TestBaseBlueprintHandler_GetKustomizations(t *testing.T) {
	t.Run("NoKustomizations", func(t *testing.T) {
		// Given a blueprint handler with no kustomizations
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: config.NewMockConfigHandler(),
			projectRoot:   "/tmp",
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: nil,
			},
		}

		// When getting kustomizations
		result := handler.GetKustomizations()

		// Then it should return empty slice
		if len(result) != 0 {
			t.Errorf("expected empty slice, got %v", result)
		}
	})

	t.Run("KustomizationWithNoPatches", func(t *testing.T) {
		// Given a blueprint handler with a kustomization that has no patches
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: config.NewMockConfigHandler(),
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
		result := handler.GetKustomizations()

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
		existingPatches := []blueprintv1alpha1.BlueprintPatch{
			{
				Path: "kustomize/test-patch.yaml",
			},
		}
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: config.NewMockConfigHandler(),
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
		result := handler.GetKustomizations()

		// Then it should return the kustomization with existing patches preserved
		if len(result) != 1 {
			t.Fatalf("expected 1 kustomization, got %d", len(result))
		}
		if len(result[0].Patches) != 1 {
			t.Errorf("expected 1 patch, got %d", len(result[0].Patches))
		}
		if result[0].Patches[0].Path != "kustomize/test-patch.yaml" {
			t.Errorf("expected patch path to match, got %s", result[0].Patches[0].Path)
		}
	})

	t.Run("KustomizationWithDiscoveredPatches", func(t *testing.T) {
		// Given a blueprint handler with a kustomization that will have discovered patches
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: config.NewMockConfigHandler(),
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

		// Create patch files (these should not be auto-discovered)
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
		result := handler.GetKustomizations()

		// Then it should return the kustomization with no patches (auto-discovery disabled)
		if len(result) != 1 {
			t.Fatalf("expected 1 kustomization, got %d", len(result))
		}
		if len(result[0].Patches) != 0 {
			t.Errorf("expected 0 discovered patches (auto-discovery disabled), got %d", len(result[0].Patches))
		}
	})

	t.Run("KustomizationWithExistingAndDiscoveredPatches", func(t *testing.T) {
		// Given a blueprint handler with a kustomization that has both existing and discovered patches
		existingPatches := []blueprintv1alpha1.BlueprintPatch{
			{
				Path: "kustomize/existing-patch-1.yaml",
			},
			{
				Path: "kustomize/existing-patch-2.yaml",
			},
		}
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: config.NewMockConfigHandler(),
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

		// Create discovered patch file (this should not be auto-discovered)
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
		result := handler.GetKustomizations()

		// Then it should return the kustomization with only existing patches (auto-discovery disabled)
		if len(result) != 1 {
			t.Fatalf("expected 1 kustomization, got %d", len(result))
		}
		if len(result[0].Patches) != 2 {
			t.Errorf("expected 2 patches (only existing, no auto-discovery), got %d", len(result[0].Patches))
		}
		// Existing patches should be preserved
		if result[0].Patches[0].Path != "kustomize/existing-patch-1.yaml" {
			t.Errorf("expected first patch 'kustomize/existing-patch-1.yaml', got %s", result[0].Patches[0].Path)
		}
		if result[0].Patches[1].Path != "kustomize/existing-patch-2.yaml" {
			t.Errorf("expected second patch 'kustomize/existing-patch-2.yaml', got %s", result[0].Patches[1].Path)
		}
	})

	t.Run("KustomizationWithPatchDiscoveryError", func(t *testing.T) {
		// Given a blueprint handler with a kustomization that will have patch discovery errors
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: config.NewMockConfigHandler(),
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

		// Create an invalid patch file (missing required fields) - this should not be auto-discovered
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
		result := handler.GetKustomizations()

		// Then it should return the kustomization without patches (auto-discovery disabled)
		if len(result) != 1 {
			t.Fatalf("expected 1 kustomization, got %d", len(result))
		}
		if len(result[0].Patches) != 0 {
			t.Errorf("expected 0 patches (auto-discovery disabled), got %d", len(result[0].Patches))
		}
	})

	t.Run("MultipleKustomizations", func(t *testing.T) {
		// Given a blueprint handler with multiple kustomizations
		handler := &BaseBlueprintHandler{
			shims:         NewShims(),
			configHandler: config.NewMockConfigHandler(),
			projectRoot:   "/tmp",
			blueprint: blueprintv1alpha1.Blueprint{
				Metadata: blueprintv1alpha1.Metadata{
					Name: "test-blueprint",
				},
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "kustomization-1",
						Patches: []blueprintv1alpha1.BlueprintPatch{
							{
								Path: "kustomize/existing-patch-1.yaml",
							},
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

		// Create patch file for second kustomization (this should not be auto-discovered)
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
		result := handler.GetKustomizations()

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
		if result[0].Patches[0].Path != "kustomize/existing-patch-1.yaml" {
			t.Errorf("expected first kustomization patch 'kustomize/existing-patch-1.yaml', got %s", result[0].Patches[0].Path)
		}

		// Second kustomization should have no patches (auto-discovery disabled)
		if result[1].Name != "kustomization-2" {
			t.Errorf("expected second kustomization name 'kustomization-2', got %s", result[1].Name)
		}
		if len(result[1].Patches) != 0 {
			t.Errorf("expected 0 patches for second kustomization (auto-discovery disabled), got %d", len(result[1].Patches))
		}
	})
}

func TestBaseBlueprintHandler_loadFileData(t *testing.T) {
	t.Run("func", func(t *testing.T) {
		// Test cases will go here
	})
}
