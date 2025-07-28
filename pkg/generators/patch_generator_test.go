package generators

import (
	"os"
	"strings"
	"testing"

	bundler "github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

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

	// Register mocks with injector
	mocks.Injector.Register("configHandler", mocks.ConfigHandler)
	mocks.Injector.Register("shell", mocks.Shell)
	mocks.Injector.Register("shims", mocks.Shims)
	mocks.Injector.Register("blueprintHandler", &blueprint.MockBlueprintHandler{})
	mocks.Injector.Register("artifactBuilder", &bundler.MockArtifact{})

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
	generator, mocks := createPatchGeneratorWithMocks(t)

	// Set up mock expectations
	mocks.ConfigHandler.GetContextFunc = func() string { return "test-context" }
	mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) { return "/test/config", nil }

	// When initializing the generator
	err := generator.Initialize()

	// Then it should succeed
	if err != nil {
		t.Fatalf("expected initialization to succeed, got: %v", err)
	}
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
				"coredns-config.jsonnet": map[string]any{
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
}

// =============================================================================
// Helper Tests
// =============================================================================

func TestPatchGenerator_generatePatchFiles(t *testing.T) {
	// Given a patch generator with mocks
	generator, mocks := createPatchGeneratorWithMocks(t)

	if err := generator.Initialize(); err != nil {
		t.Fatalf("failed to initialize generator: %v", err)
	}

	// And patch values
	values := map[string]any{
		"coredns-config.jsonnet": map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      "coredns-custom-test",
				"namespace": "kube-system",
			},
		},
	}

	// And mock file operations
	mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error { return nil }

	// When calling generatePatchFiles
	err := generator.generatePatchFiles("/test/patches", values, false)

	// Then it should succeed
	if err != nil {
		t.Fatalf("expected generatePatchFiles to succeed, got: %v", err)
	}
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
					t.Errorf("expected error about path traversal, got: %v", err)
				}
			})
		}
	})

	t.Run("InvalidCharacters", func(t *testing.T) {
		// When validating names with invalid characters
		testCases := []string{"test<name", "test>name", "test:name", "test\"name", "test|name", "test?name", "test*name"}

		for _, name := range testCases {
			t.Run(name, func(t *testing.T) {
				err := generator.validateKustomizationName(name)

				// Then it should fail
				if err == nil {
					t.Errorf("expected validation to fail for %s", name)
				}
				if !strings.Contains(err.Error(), "invalid characters") {
					t.Errorf("expected error about invalid characters, got: %v", err)
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
		if !strings.Contains(err.Error(), "missing or invalid apiVersion field") {
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
		if !strings.Contains(err.Error(), "missing or invalid apiVersion field") {
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
		if !strings.Contains(err.Error(), "missing or invalid kind field") {
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
		if !strings.Contains(err.Error(), "missing or invalid kind field") {
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
		if !strings.Contains(err.Error(), "missing metadata field") {
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
		if !strings.Contains(err.Error(), "missing or invalid name in metadata") {
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
		if !strings.Contains(err.Error(), "missing or invalid name in metadata") {
			t.Errorf("expected error about missing name, got: %v", err)
		}
	})
}
