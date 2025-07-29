package generators

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// patchMockFileInfo implements os.FileInfo for testing
type patchMockFileInfo struct {
	isDir bool
}

func (m *patchMockFileInfo) Name() string       { return "mock" }
func (m *patchMockFileInfo) Size() int64        { return 0 }
func (m *patchMockFileInfo) Mode() os.FileMode  { return 0 }
func (m *patchMockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *patchMockFileInfo) IsDir() bool        { return m.isDir }
func (m *patchMockFileInfo) Sys() interface{}   { return nil }

// =============================================================================
// Test Setup
// =============================================================================

// PatchMocks provides mock dependencies for patch generator tests
type PatchMocks struct {
	Injector      di.Injector
	ConfigHandler *config.MockConfigHandler
	Shell         *shell.MockShell
	Shims         *Shims
}

// setupPatchGeneratorMocks creates mock dependencies for patch generator tests
func setupPatchGeneratorMocks(t *testing.T) *PatchMocks {
	mocks := &PatchMocks{
		Injector:      di.NewInjector(),
		ConfigHandler: &config.MockConfigHandler{},
		Shell:         &shell.MockShell{},
		Shims:         NewShims(),
	}

	// Set up mock file operations
	mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error { return nil }
	mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error { return nil }

	// Create mock blueprint handler with kustomizations that reference patches
	mockBlueprintHandler := &blueprint.MockBlueprintHandler{
		GetKustomizationsFunc: func() []blueprintv1alpha1.Kustomization {
			return []blueprintv1alpha1.Kustomization{
				{
					Name: "dns",
					Patches: []blueprintv1alpha1.BlueprintPatch{
						{
							Path: "patches/dns/coredns.yaml",
						},
					},
				},
				{
					Name: "ingress",
					Patches: []blueprintv1alpha1.BlueprintPatch{
						{
							Path: "patches/ingress/nginx.yaml",
						},
					},
				},
			}
		},
	}

	// Register mocks with injector
	mocks.Injector.Register("configHandler", mocks.ConfigHandler)
	mocks.Injector.Register("shell", mocks.Shell)
	mocks.Injector.Register("shims", mocks.Shims)
	mocks.Injector.Register("blueprintHandler", mockBlueprintHandler)
	mocks.Injector.Register("artifactBuilder", &artifact.MockArtifact{})

	return mocks
}

// createPatchGeneratorWithMocks creates a patch generator with mocked shims
func createPatchGeneratorWithMocks(t *testing.T) (*PatchGenerator, *PatchMocks) {
	mocks := setupPatchGeneratorMocks(t)
	generator := NewPatchGenerator(mocks.Injector)

	// Replace the generator's shims with our mocked ones
	generator.shims = mocks.Shims

	return generator, mocks
}

// =============================================================================
// Constructor Tests
// =============================================================================

func TestNewPatchGenerator(t *testing.T) {
	// Given creating a new patch generator
	mocks := setupPatchGeneratorMocks(t)
	generator := NewPatchGenerator(mocks.Injector)

	// Then the generator should be created successfully
	if generator == nil {
		t.Fatal("expected generator to be created")
	}
}

// =============================================================================
// Initialization Tests
// =============================================================================

func TestPatchGenerator_Initialize(t *testing.T) {
	// Given a patch generator with mocks
	generator, _ := createPatchGeneratorWithMocks(t)

	t.Run("Success", func(t *testing.T) {
		// When initializing the generator
		err := generator.Initialize()

		// Then it should succeed
		if err != nil {
			t.Fatalf("expected Initialize to succeed, got: %v", err)
		}
	})

	t.Run("BlueprintHandlerNotFound", func(t *testing.T) {
		// Given an injector without blueprint handler but with all base dependencies
		emptyInjector := di.NewInjector()
		// Add required base dependencies
		emptyInjector.Register("configHandler", &config.MockConfigHandler{})
		emptyInjector.Register("shell", &shell.MockShell{})
		emptyInjector.Register("shims", NewShims())
		emptyInjector.Register("artifactBuilder", &artifact.MockArtifact{})
		// Don't register blueprintHandler
		generator := NewPatchGenerator(emptyInjector)

		// When initializing the generator
		err := generator.Initialize()

		// Then it should fail
		if err == nil {
			t.Fatal("expected Initialize to fail with missing blueprint handler")
		}
		if !strings.Contains(err.Error(), "failed to resolve blueprint handler") {
			t.Errorf("expected error about missing blueprint handler, got: %v", err)
		}
	})

	t.Run("BlueprintHandlerWrongType", func(t *testing.T) {
		// Given an injector with wrong type for blueprint handler
		wrongTypeInjector := di.NewInjector()
		// Add required base dependencies
		wrongTypeInjector.Register("configHandler", &config.MockConfigHandler{})
		wrongTypeInjector.Register("shell", &shell.MockShell{})
		wrongTypeInjector.Register("shims", NewShims())
		wrongTypeInjector.Register("artifactBuilder", &artifact.MockArtifact{})
		wrongTypeInjector.Register("blueprintHandler", "not-a-blueprint-handler")
		generator := NewPatchGenerator(wrongTypeInjector)

		// When initializing the generator
		err := generator.Initialize()

		// Then it should fail
		if err == nil {
			t.Fatal("expected Initialize to fail with wrong blueprint handler type")
		}
		if !strings.Contains(err.Error(), "failed to resolve blueprint handler") {
			t.Errorf("expected error about wrong type, got: %v", err)
		}
	})

	t.Run("BaseGeneratorInitializeError", func(t *testing.T) {
		// Given an injector missing required dependencies
		incompleteInjector := di.NewInjector()
		generator := NewPatchGenerator(incompleteInjector)

		// When initializing the generator
		err := generator.Initialize()

		// Then it should fail
		if err == nil {
			t.Fatal("expected Initialize to fail with missing base dependencies")
		}
		if !strings.Contains(err.Error(), "failed to initialize base generator") {
			t.Errorf("expected error about base generator initialization, got: %v", err)
		}
	})
}

// =============================================================================
// Generate Method Tests
// =============================================================================

func TestPatchGenerator_Generate(t *testing.T) {
	// Given a patch generator with mocks
	generator, mocks := createPatchGeneratorWithMocks(t)

	// Set up mock expectations
	mocks.ConfigHandler.GetContextFunc = func() string { return "test-context" }
	mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) { return "/test/config", nil }

	if err := generator.Initialize(); err != nil {
		t.Fatalf("failed to initialize generator: %v", err)
	}

	t.Run("InitPipelineData", func(t *testing.T) {
		// Given init pipeline data (no "patches/" prefix)
		data := map[string]any{
			"terraform/vpc": map[string]any{"region": "us-west-2"},
			"blueprint":     map[string]any{"name": "test"},
		}

		// When calling Generate with init pipeline data
		err := generator.Generate(data)

		// Then it should succeed (no-op for init pipeline)
		if err != nil {
			t.Fatalf("expected Generate to succeed with init pipeline data, got: %v", err)
		}
	})

	t.Run("InstallPipelineData", func(t *testing.T) {
		// Given install pipeline data (with "patches/" prefix)
		data := map[string]any{
			"patches/dns": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name":      "coredns-custom-test",
					"namespace": "kube-system",
				},
				"data": map[string]any{
					"custom.server": "test {\n    forward . 8.8.8.8\n}",
				},
			},
		}

		// And mock file operations
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error { return nil }
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error { return nil }

		// When calling Generate with install pipeline data
		err := generator.Generate(data)

		// Then it should succeed
		if err != nil {
			t.Fatalf("expected Generate to succeed with install pipeline data, got: %v", err)
		}
	})

	t.Run("SubdirectoryStructure", func(t *testing.T) {
		// Given install pipeline data with subdirectory structure
		data := map[string]any{
			"patches/ingress/nginx": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name":      "nginx-config",
					"namespace": "default",
				},
				"data": map[string]any{
					"nginx.conf": "server { listen 80; }",
				},
			},
		}

		// And mock file operations that track created directories and files
		var createdDirs []string
		var createdFiles []string
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			createdDirs = append(createdDirs, path)
			return nil
		}
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			createdFiles = append(createdFiles, name)
			return nil
		}

		// When calling Generate with subdirectory structure
		err := generator.Generate(data)

		// Then it should succeed
		if err != nil {
			t.Fatalf("expected Generate to succeed with subdirectory structure, got: %v", err)
		}

		// And it should create the correct file
		expectedFile := "/test/config/patches/ingress/nginx.yaml"
		found := false
		for _, file := range createdFiles {
			// Normalize paths for cross-platform comparison
			normalizedFile := filepath.ToSlash(file)
			normalizedExpected := filepath.ToSlash(expectedFile)
			if strings.HasSuffix(normalizedFile, normalizedExpected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected file %s to be created, but was not found in: %v", expectedFile, createdFiles)
		}
	})

	t.Run("MissingConfigRoot", func(t *testing.T) {
		// Given config root fails
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", os.ErrNotExist
		}

		// And install pipeline data
		data := map[string]any{
			"patches/dns": map[string]any{},
		}

		// When calling Generate
		err := generator.Generate(data)

		// Then it should fail
		if err == nil {
			t.Fatal("expected Generate to fail with missing config root")
		}
	})

	t.Run("NilData", func(t *testing.T) {
		// When calling Generate with nil data
		err := generator.Generate(nil)

		// Then it should fail
		if err == nil {
			t.Fatal("expected Generate to fail with nil data")
		}
		if !strings.Contains(err.Error(), "data cannot be nil") {
			t.Errorf("expected error about nil data, got: %v", err)
		}
	})

	t.Run("NoKustomizations", func(t *testing.T) {
		// Given a blueprint handler with no kustomizations
		mockBlueprintHandler := &blueprint.MockBlueprintHandler{
			GetKustomizationsFunc: func() []blueprintv1alpha1.Kustomization {
				return []blueprintv1alpha1.Kustomization{}
			},
		}
		mocks.Injector.Register("blueprintHandler", mockBlueprintHandler)

		// And proper config root mock
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) { return "/test/config", nil }

		// And patch data with valid content
		data := map[string]any{
			"patches/dns": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "test-config",
				},
			},
		}

		// When calling Generate
		err := generator.Generate(data)

		// Then it should succeed (no-op)
		if err != nil {
			t.Fatalf("expected Generate to succeed with no kustomizations, got: %v", err)
		}
	})

	t.Run("NoPatchReferences", func(t *testing.T) {
		// Given a blueprint handler with kustomizations but no patch references
		mockBlueprintHandler := &blueprint.MockBlueprintHandler{
			GetKustomizationsFunc: func() []blueprintv1alpha1.Kustomization {
				return []blueprintv1alpha1.Kustomization{
					{
						Name:    "dns",
						Patches: []blueprintv1alpha1.BlueprintPatch{},
					},
				}
			},
		}
		mocks.Injector.Register("blueprintHandler", mockBlueprintHandler)

		// And proper config root mock
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) { return "/test/config", nil }

		// And patch data with valid content
		data := map[string]any{
			"patches/dns": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "test-config",
				},
			},
		}

		// When calling Generate
		err := generator.Generate(data)

		// Then it should succeed (no-op)
		if err != nil {
			t.Fatalf("expected Generate to succeed with no patch references, got: %v", err)
		}
	})

	t.Run("InvalidValuesType", func(t *testing.T) {
		// Given proper config root mock
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) { return "/test/config", nil }

		// And patch data with invalid values type
		data := map[string]any{
			"patches/dns": "not-a-map",
		}

		// When calling Generate
		err := generator.Generate(data)

		// Then it should fail
		if err == nil {
			t.Fatal("expected Generate to fail with invalid values type")
		}
		if !strings.Contains(err.Error(), "must be a map") {
			t.Errorf("expected error about map requirement, got: %v", err)
		}
	})

	t.Run("UnreferencedPatch", func(t *testing.T) {
		// Given proper config root mock
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) { return "/test/config", nil }

		// And patch data for an unreferenced kustomization
		data := map[string]any{
			"patches/unreferenced": map[string]any{},
		}

		// When calling Generate
		err := generator.Generate(data)

		// Then it should succeed (skips unreferenced patches)
		if err != nil {
			t.Fatalf("expected Generate to succeed with unreferenced patch, got: %v", err)
		}
	})

	t.Run("OverwriteFlag", func(t *testing.T) {
		// Given proper config root mock
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) { return "/test/config", nil }

		// And patch data
		data := map[string]any{
			"patches/dns": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "test",
				},
			},
		}

		// And mock file operations
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error { return nil }
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error { return nil }

		// When calling Generate with overwrite flag
		err := generator.Generate(data, true)

		// Then it should succeed
		if err != nil {
			t.Fatalf("expected Generate to succeed with overwrite flag, got: %v", err)
		}
	})
}

// =============================================================================
// Helper Tests
// =============================================================================

func TestPatchGenerator_extractPatchReferences(t *testing.T) {
	// Given a patch generator
	generator, _ := createPatchGeneratorWithMocks(t)

	t.Run("WithValidPatches", func(t *testing.T) {
		// Given kustomizations with valid patch references
		kustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name: "dns",
				Patches: []blueprintv1alpha1.BlueprintPatch{
					{
						Path: "patches/dns/coredns.yaml",
					},
					{
						Path: "patches/dns/dns-config.yaml",
					},
				},
			},
			{
				Name: "ingress",
				Patches: []blueprintv1alpha1.BlueprintPatch{
					{
						Path: "patches/ingress/nginx.yaml",
					},
				},
			},
		}

		// When extracting patch references
		result := generator.extractPatchReferences(kustomizations)

		// Then it should return the expected references
		if len(result) != 2 {
			t.Errorf("expected 2 kustomizations, got %d", len(result))
		}

		if dnsRefs, exists := result["dns"]; !exists {
			t.Error("expected dns kustomization to be present")
		} else if len(dnsRefs) != 2 {
			t.Errorf("expected 2 dns references, got %d", len(dnsRefs))
		}

		if ingressRefs, exists := result["ingress"]; !exists {
			t.Error("expected ingress kustomization to be present")
		} else if len(ingressRefs) != 1 {
			t.Errorf("expected 1 ingress reference, got %d", len(ingressRefs))
		}
	})

	t.Run("WithEmptyPatches", func(t *testing.T) {
		// Given kustomizations with empty patches
		kustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name:    "dns",
				Patches: []blueprintv1alpha1.BlueprintPatch{},
			},
		}

		// When extracting patch references
		result := generator.extractPatchReferences(kustomizations)

		// Then it should return empty result
		if len(result) != 0 {
			t.Errorf("expected empty result, got %d kustomizations", len(result))
		}
	})

	t.Run("WithNilTarget", func(t *testing.T) {
		// Given kustomizations with nil targets
		kustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name: "dns",
				Patches: []blueprintv1alpha1.BlueprintPatch{
					{
						Path: "",
					},
				},
			},
		}

		// When extracting patch references
		result := generator.extractPatchReferences(kustomizations)

		// Then it should return empty result
		if len(result) != 0 {
			t.Errorf("expected empty result, got %d kustomizations", len(result))
		}
	})

	t.Run("WithEmptyTargetName", func(t *testing.T) {
		// Given kustomizations with empty path
		kustomizations := []blueprintv1alpha1.Kustomization{
			{
				Name: "dns",
				Patches: []blueprintv1alpha1.BlueprintPatch{
					{
						Path: "",
					},
				},
			},
		}

		// When extracting patch references
		result := generator.extractPatchReferences(kustomizations)

		// Then it should return empty result
		if len(result) != 0 {
			t.Errorf("expected empty result, got %d kustomizations", len(result))
		}
	})
}

func TestPatchGenerator_isPatchReferenced(t *testing.T) {
	// Given a patch generator
	generator, _ := createPatchGeneratorWithMocks(t)

	t.Run("ExactMatch", func(t *testing.T) {
		// Given patch references
		patchReferences := map[string][]string{
			"dns": {"patches/dns/coredns.yaml", "patches/dns/dns-config.yaml"},
		}

		// When checking exact match
		result := generator.isPatchReferenced("dns", patchReferences)

		// Then it should be referenced
		if !result {
			t.Error("expected patch to be referenced")
		}
	})

	t.Run("SubdirectoryMatch", func(t *testing.T) {
		// Given patch references
		patchReferences := map[string][]string{
			"ingress": {"patches/ingress/nginx.yaml"},
		}

		// When checking subdirectory match
		result := generator.isPatchReferenced("ingress", patchReferences)

		// Then it should be referenced
		if !result {
			t.Error("expected patch to be referenced")
		}
	})

	t.Run("NoMatch", func(t *testing.T) {
		// Given patch references
		patchReferences := map[string][]string{
			"dns": {"patches/dns/coredns.yaml"},
		}

		// When checking no match
		result := generator.isPatchReferenced("unreferenced", patchReferences)

		// Then it should not be referenced
		if result {
			t.Error("expected patch to not be referenced")
		}
	})

	t.Run("EmptyReferences", func(t *testing.T) {
		// Given empty patch references
		patchReferences := map[string][]string{}

		// When checking any patch
		result := generator.isPatchReferenced("dns", patchReferences)

		// Then it should not be referenced
		if result {
			t.Error("expected patch to not be referenced")
		}
	})
}

func TestPatchGenerator_generatePatchFiles(t *testing.T) {
	// Given a patch generator with mocks
	generator, mocks := createPatchGeneratorWithMocks(t)

	t.Run("Success", func(t *testing.T) {
		// Given valid patch files
		values := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "test-config",
			},
			"data": map[string]any{
				"config": "value",
			},
		}

		// And mock file operations
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error { return nil }
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error { return nil }

		// When generating patch files
		err := generator.generatePatchFiles("/test/patches/test.yaml", values, false)

		// Then it should succeed
		if err != nil {
			t.Fatalf("expected generatePatchFiles to succeed, got: %v", err)
		}
	})

	t.Run("MkdirAllError", func(t *testing.T) {
		// Given mock that fails on MkdirAll
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return os.ErrPermission
		}

		// And valid patch files
		values := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "test",
			},
		}

		// When generating patch files
		err := generator.generatePatchFiles("/test/patches/test.yaml", values, false)

		// Then it should fail
		if err == nil {
			t.Fatal("expected generatePatchFiles to fail with MkdirAll error")
		}
		if !strings.Contains(err.Error(), "failed to create directory") {
			t.Errorf("expected error about directory creation, got: %v", err)
		}
	})

	t.Run("WriteFileError", func(t *testing.T) {
		// Given mock that fails on WriteFile
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error { return nil }
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return os.ErrPermission
		}

		// And valid patch files
		values := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "test",
			},
		}

		// When generating patch files
		err := generator.generatePatchFiles("/test/patches/test.yaml", values, false)

		// Then it should fail
		if err == nil {
			t.Fatal("expected generatePatchFiles to fail with WriteFile error")
		}
		if !strings.Contains(err.Error(), "failed to write patch file") {
			t.Errorf("expected error about file writing, got: %v", err)
		}
	})

	t.Run("MarshalYAMLError", func(t *testing.T) {
		// Given mock that fails on MarshalYAML
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error { return nil }
		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return nil, os.ErrInvalid
		}

		// And valid patch files
		values := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "test",
			},
		}

		// When generating patch files
		err := generator.generatePatchFiles("/test/patches/test.yaml", values, false)

		// Then it should fail
		if err == nil {
			t.Fatal("expected generatePatchFiles to fail with MarshalYAML error")
		}
		if !strings.Contains(err.Error(), "failed to marshal content to YAML") {
			t.Errorf("expected error about YAML marshalling, got: %v", err)
		}
	})

	t.Run("InvalidManifest", func(t *testing.T) {
		// Given invalid manifest (missing required fields)
		values := map[string]any{
			"apiVersion": "v1",
			// Missing kind and metadata
		}

		// And mock file operations
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error { return nil }

		// When generating patch files
		err := generator.generatePatchFiles("/test/patches/test.yaml", values, false)

		// Then it should fail
		if err == nil {
			t.Fatal("expected generatePatchFiles to fail with invalid manifest")
		}
		if !strings.Contains(err.Error(), "invalid Kubernetes manifest") {
			t.Errorf("expected error about invalid manifest, got: %v", err)
		}
	})

	t.Run("AutoAppendYamlExtension", func(t *testing.T) {
		// Given patch files without .yaml extension
		values := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "test-config",
			},
		}

		// And mock file operations that track written files
		var writtenFiles []string
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error { return nil }
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) { return []byte("test yaml"), nil }
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			writtenFiles = append(writtenFiles, name)
			return nil
		}

		// When generating patch files
		err := generator.generatePatchFiles("/test/patches/test", values, false)

		// Then it should succeed
		if err != nil {
			t.Fatalf("expected generatePatchFiles to succeed, got: %v", err)
		}

		// And it should append .yaml extension
		expectedFile := "/test/patches/test.yaml"
		found := false
		for _, file := range writtenFiles {
			if strings.HasSuffix(file, expectedFile) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected file %s to be written, but was not found in: %v", expectedFile, writtenFiles)
		}
	})

	t.Run("SkipExistingFiles", func(t *testing.T) {
		// Given patch files
		values := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "test",
			},
		}

		// And mock that indicates file exists
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error { return nil }
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			// Return directory for /test/patches
			if name == "/test/patches" {
				return &patchMockFileInfo{isDir: true}, nil
			}
			// Return file exists for individual files
			return nil, nil // File exists
		}
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			t.Error("WriteFile should not be called when file exists and overwrite is false")
			return nil
		}

		// When generating patch files with overwrite=false
		err := generator.generatePatchFiles("/test/patches", values, false)

		// Then it should succeed (skips existing files)
		if err != nil {
			t.Fatalf("expected generatePatchFiles to succeed, got: %v", err)
		}
	})

	t.Run("OverwriteExistingFiles", func(t *testing.T) {
		// Given patch files
		values := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "test",
			},
		}

		// And mock that indicates file exists
		var writeFileCalled bool
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error { return nil }
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			// Return directory for /test/patches
			if name == "/test/patches" {
				return &patchMockFileInfo{isDir: true}, nil
			}
			// Return file exists for individual files
			return nil, nil // File exists
		}
		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) { return []byte("test yaml"), nil }
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			writeFileCalled = true
			return nil
		}

		// When generating patch files with overwrite=true
		err := generator.generatePatchFiles("/test/patches", values, true)

		// Then it should succeed
		if err != nil {
			t.Fatalf("expected generatePatchFiles to succeed, got: %v", err)
		}

		// And WriteFile should be called (overwrites existing files)
		if !writeFileCalled {
			t.Error("expected WriteFile to be called when overwrite is true")
		}
	})
}

// =============================================================================
// Validation Method Tests
// =============================================================================

func TestPatchGenerator_validateKustomizationName(t *testing.T) {
	// Given a patch generator
	generator, _ := createPatchGeneratorWithMocks(t)

	t.Run("ValidName", func(t *testing.T) {
		// When validating a valid kustomization name
		err := generator.validateKustomizationName("valid-name")

		// Then it should succeed
		if err != nil {
			t.Errorf("expected validation to succeed, got: %v", err)
		}
	})

	t.Run("ValidSubdirectoryPath", func(t *testing.T) {
		// When validating a valid subdirectory path
		err := generator.validateKustomizationName("ingress/nginx")

		// Then it should succeed
		if err != nil {
			t.Errorf("expected validation to succeed for subdirectory path, got: %v", err)
		}
	})

	t.Run("ValidNestedSubdirectoryPath", func(t *testing.T) {
		// When validating a valid nested subdirectory path
		err := generator.validateKustomizationName("ingress/nginx/config")

		// Then it should succeed
		if err != nil {
			t.Errorf("expected validation to succeed for nested subdirectory path, got: %v", err)
		}
	})

	t.Run("EmptyPathComponent", func(t *testing.T) {
		// When validating a path with empty components
		testCases := []string{"//test", "test//", "test//name", "/test"}

		for _, name := range testCases {
			t.Run(name, func(t *testing.T) {
				err := generator.validateKustomizationName(name)

				// Then it should fail
				if err == nil {
					t.Error("expected validation to fail for path with empty components")
				}
				if !strings.Contains(err.Error(), "empty path components") {
					t.Errorf("expected error about empty path components, got: %v", err)
				}
			})
		}
	})

	t.Run("EmptyName", func(t *testing.T) {
		// When validating an empty name
		err := generator.validateKustomizationName("")

		// Then it should fail
		if err == nil {
			t.Error("expected validation to fail for empty name")
		}
		if !strings.Contains(err.Error(), "cannot be empty") {
			t.Errorf("expected error about empty name, got: %v", err)
		}
	})

	t.Run("PathTraversalCharacters", func(t *testing.T) {
		// When validating names with path traversal characters
		testCases := []string{"../test", "test/../", "test\\..", "..\\test"}

		for _, name := range testCases {
			t.Run(name, func(t *testing.T) {
				err := generator.validateKustomizationName(name)

				// Then it should fail
				if err == nil {
					t.Errorf("expected validation to fail for %s", name)
				}
				if !strings.Contains(err.Error(), "path traversal characters") {
					t.Errorf("expected error about path traversal characters, got: %v", err)
				}
			})
		}
	})

	t.Run("InvalidCharacters", func(t *testing.T) {
		// When validating names with invalid character
		testCases := []string{"test<name", "test>name", "test:name", "test\"name", "test|name", "test?name", "test*name"}

		for _, name := range testCases {
			t.Run(name, func(t *testing.T) {
				err := generator.validateKustomizationName(name)

				// Then it should fail
				if err == nil {
					t.Errorf("expected validation to fail for %s", name)
				}
				if !strings.Contains(err.Error(), "invalid character") {
					t.Errorf("expected error about invalid character, got: %v", err)
				}
			})
		}
	})
}

func TestPatchGenerator_validatePath(t *testing.T) {
	// Given a patch generator
	generator, _ := createPatchGeneratorWithMocks(t)

	t.Run("ValidPath", func(t *testing.T) {
		// When validating a valid path within base directory
		err := generator.validatePath("/base/path/subdir", "/base/path")

		// Then it should succeed
		if err != nil {
			t.Errorf("expected validation to succeed, got: %v", err)
		}
	})

	t.Run("PathOutsideBase", func(t *testing.T) {
		// When validating a path outside the base directory
		err := generator.validatePath("/different/path", "/base/path")

		// Then it should fail
		if err == nil {
			t.Error("expected validation to fail for path outside base")
		}
		if !strings.Contains(err.Error(), "outside base path") {
			t.Errorf("expected error about path outside base, got: %v", err)
		}
	})

	t.Run("SamePath", func(t *testing.T) {
		// When validating the same path as base
		err := generator.validatePath("/base/path", "/base/path")

		// Then it should succeed
		if err != nil {
			t.Errorf("expected validation to succeed, got: %v", err)
		}
	})
}

func TestPatchGenerator_validateKubernetesManifest(t *testing.T) {
	// Given a patch generator
	generator, _ := createPatchGeneratorWithMocks(t)

	t.Run("ValidManifest", func(t *testing.T) {
		// When validating a valid Kubernetes manifest
		manifest := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "test-config",
			},
		}

		err := generator.validateKubernetesManifest(manifest)

		// Then it should succeed
		if err != nil {
			t.Errorf("expected validation to succeed, got: %v", err)
		}
	})

	t.Run("NotMap", func(t *testing.T) {
		// When validating non-map content
		err := generator.validateKubernetesManifest("not-a-map")

		// Then it should fail
		if err == nil {
			t.Error("expected validation to fail for non-map content")
		}
		if !strings.Contains(err.Error(), "must be a map") {
			t.Errorf("expected error about map requirement, got: %v", err)
		}
	})

	t.Run("MissingAPIVersion", func(t *testing.T) {
		// When validating manifest missing apiVersion
		manifest := map[string]any{
			"kind": "ConfigMap",
			"metadata": map[string]any{
				"name": "test-config",
			},
		}

		err := generator.validateKubernetesManifest(manifest)

		// Then it should fail
		if err == nil {
			t.Error("expected validation to fail for missing apiVersion")
		}
		if !strings.Contains(err.Error(), "missing or invalid 'apiVersion' field") {
			t.Errorf("expected error about missing apiVersion, got: %v", err)
		}
	})

	t.Run("EmptyAPIVersion", func(t *testing.T) {
		// When validating manifest with empty apiVersion
		manifest := map[string]any{
			"apiVersion": "",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "test-config",
			},
		}

		err := generator.validateKubernetesManifest(manifest)

		// Then it should fail
		if err == nil {
			t.Error("expected validation to fail for empty apiVersion")
		}
		if !strings.Contains(err.Error(), "missing or invalid 'apiVersion' field") {
			t.Errorf("expected error about missing apiVersion, got: %v", err)
		}
	})

	t.Run("MissingKind", func(t *testing.T) {
		// When validating manifest missing kind
		manifest := map[string]any{
			"apiVersion": "v1",
			"metadata": map[string]any{
				"name": "test-config",
			},
		}

		err := generator.validateKubernetesManifest(manifest)

		// Then it should fail
		if err == nil {
			t.Error("expected validation to fail for missing kind")
		}
		if !strings.Contains(err.Error(), "missing or invalid 'kind' field") {
			t.Errorf("expected error about missing kind, got: %v", err)
		}
	})

	t.Run("EmptyKind", func(t *testing.T) {
		// When validating manifest with empty kind
		manifest := map[string]any{
			"apiVersion": "v1",
			"kind":       "",
			"metadata": map[string]any{
				"name": "test-config",
			},
		}

		err := generator.validateKubernetesManifest(manifest)

		// Then it should fail
		if err == nil {
			t.Error("expected validation to fail for empty kind")
		}
		if !strings.Contains(err.Error(), "missing or invalid 'kind' field") {
			t.Errorf("expected error about missing kind, got: %v", err)
		}
	})

	t.Run("MissingMetadata", func(t *testing.T) {
		// When validating manifest missing metadata
		manifest := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
		}

		err := generator.validateKubernetesManifest(manifest)

		// Then it should fail
		if err == nil {
			t.Error("expected validation to fail for missing metadata")
		}
		if !strings.Contains(err.Error(), "missing 'metadata' field") {
			t.Errorf("expected error about missing metadata, got: %v", err)
		}
	})

	t.Run("MissingName", func(t *testing.T) {
		// When validating manifest missing name
		manifest := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"labels": map[string]any{
					"app": "test",
				},
			},
		}

		err := generator.validateKubernetesManifest(manifest)

		// Then it should fail
		if err == nil {
			t.Error("expected validation to fail for missing name")
		}
		if !strings.Contains(err.Error(), "missing or invalid 'metadata.name' field") {
			t.Errorf("expected error about missing name, got: %v", err)
		}
	})

	t.Run("EmptyName", func(t *testing.T) {
		// When validating manifest with empty name
		manifest := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "",
			},
		}

		err := generator.validateKubernetesManifest(manifest)

		// Then it should fail
		if err == nil {
			t.Error("expected validation to fail for empty name")
		}
		if !strings.Contains(err.Error(), "missing or invalid 'metadata.name' field") {
			t.Errorf("expected error about missing name, got: %v", err)
		}
	})
}
