package generators

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupBlueprintGeneratorMocks(t *testing.T) (*BlueprintGenerator, *Mocks) {
	mocks := setupMocks(t)
	generator := NewBlueprintGenerator(mocks.Injector)
	generator.shims = mocks.Shims

	// Mock GetConfigRoot to return a test path
	mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
		return "/test/context", nil
	}

	if err := generator.Initialize(); err != nil {
		t.Fatalf("failed to initialize BlueprintGenerator: %v", err)
	}

	return generator, mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBlueprintGenerator_NewBlueprintGenerator(t *testing.T) {
	mocks := setupMocks(t)
	generator := NewBlueprintGenerator(mocks.Injector)

	if generator == nil {
		t.Errorf("Expected generator to be non-nil")
	}
}

func TestBlueprintGenerator_Write(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a BlueprintGenerator with mocks
		generator, _ := setupBlueprintGeneratorMocks(t)

		// When Write is called
		err := generator.Write()

		// Then no error should occur (since no data is provided)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithOverwriteTrue", func(t *testing.T) {
		// Given a BlueprintGenerator with mocks
		generator, _ := setupBlueprintGeneratorMocks(t)

		// When Write is called with overwrite true
		err := generator.Write(true)

		// Then no error should occur (since no data is provided)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithOverwriteFalse", func(t *testing.T) {
		// Given a BlueprintGenerator with mocks
		generator, _ := setupBlueprintGeneratorMocks(t)

		// When Write is called with overwrite false
		err := generator.Write(false)

		// Then no error should occur (since no data is provided)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithMultipleOverwriteParams", func(t *testing.T) {
		// Given a BlueprintGenerator with mocks
		generator, _ := setupBlueprintGeneratorMocks(t)

		// When Write is called with multiple overwrite parameters (should use first)
		err := generator.Write(true, false, true)

		// Then no error should occur (since no data is provided)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestBlueprintGenerator_Generate(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a BlueprintGenerator with mocks
		generator, mocks := setupBlueprintGeneratorMocks(t)

		// And mock file operations
		var writtenPath string
		var writtenContent []byte
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			writtenPath = name
			writtenContent = data
			return nil
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist // File doesn't exist
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return []byte("kind: Blueprint\napiVersion: v1alpha1\n"), nil
		}

		// And template data with blueprint configuration
		data := map[string]any{
			"blueprint": map[string]any{
				"metadata": map[string]any{
					"name":        "test-blueprint",
					"description": "A test blueprint",
					"authors":     []any{"test-author"},
				},
				"repository": map[string]any{
					"url": "https://github.com/example/repo",
					"ref": map[string]any{
						"branch": "main",
					},
				},
				"sources": []any{
					map[string]any{
						"name": "test-source",
						"url":  "https://github.com/example/source",
						"ref": map[string]any{
							"tag": "v1.0.0",
						},
					},
				},
				"terraform": []any{
					map[string]any{
						"path":   "cluster",
						"source": "github.com/example/terraform-cluster",
						"values": map[string]any{
							"cluster_name": "test-cluster",
						},
						"destroy": true,
					},
				},
				"kustomize": []any{
					map[string]any{
						"name":       "test-kustomization",
						"path":       "kustomize/test",
						"source":     "github.com/example/kustomize",
						"dependsOn":  []any{"terraform"},
						"components": []any{"component1"},
						"cleanup":    []any{"cleanup1"},
						"wait":       true,
						"force":      false,
						"prune":      true,
					},
				},
			},
		}

		// When Generate is called
		err := generator.Generate(data)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the file should be written to the correct path
		expectedPath := filepath.Join("/test/context", "blueprint.yaml")
		if writtenPath != expectedPath {
			t.Errorf("Expected file path %s, got %s", expectedPath, writtenPath)
		}

		// And the content should be written
		if len(writtenContent) == 0 {
			t.Errorf("Expected content to be written, got empty content")
		}
	})

	t.Run("NoData", func(t *testing.T) {
		// Given a BlueprintGenerator with mocks
		generator, _ := setupBlueprintGeneratorMocks(t)

		// When Generate is called with no data
		err := generator.Generate(nil)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("NoBlueprintKey", func(t *testing.T) {
		// Given a BlueprintGenerator with mocks
		generator, _ := setupBlueprintGeneratorMocks(t)

		// And data without blueprint key
		data := map[string]any{
			"terraform": map[string]any{
				"cluster": map[string]any{
					"cluster_name": "test",
				},
			},
		}

		// When Generate is called
		err := generator.Generate(data)

		// Then no error should occur (blueprint key is optional)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("InvalidBlueprintData", func(t *testing.T) {
		// Given a BlueprintGenerator with mocks
		generator, _ := setupBlueprintGeneratorMocks(t)

		// And data with invalid blueprint data type
		data := map[string]any{
			"blueprint": "invalid-string-data",
		}

		// When Generate is called
		err := generator.Generate(data)

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected error for invalid blueprint data type, got nil")
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		// Given a BlueprintGenerator with mocks
		generator, mocks := setupBlueprintGeneratorMocks(t)

		// And GetConfigRoot returns an error
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("config root error")
		}

		// And valid blueprint data
		data := map[string]any{
			"blueprint": map[string]any{
				"metadata": map[string]any{
					"name": "test-blueprint",
				},
			},
		}

		// When Generate is called
		err := generator.Generate(data)

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected error from GetConfigRoot, got nil")
		}
	})

	t.Run("ErrorCreatingBlueprintFromData", func(t *testing.T) {
		// Given a BlueprintGenerator with mocks
		generator, mocks := setupBlueprintGeneratorMocks(t)

		// And MarshalYAML returns an error
		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("marshal error")
		}

		// And valid blueprint data
		data := map[string]any{
			"blueprint": map[string]any{
				"metadata": map[string]any{
					"name": "test-blueprint",
				},
			},
		}

		// When Generate is called
		err := generator.Generate(data)

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected error from createBlueprintFromData, got nil")
		}
	})

	t.Run("ErrorMarshalingBlueprint", func(t *testing.T) {
		// Given a BlueprintGenerator with mocks
		generator, mocks := setupBlueprintGeneratorMocks(t)

		// And MarshalYAML succeeds for createBlueprintFromData but fails for final marshal
		callCount := 0
		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			callCount++
			if callCount == 1 {
				// First call for createBlueprintFromData
				return []byte("metadata:\n  name: test-blueprint\n"), nil
			}
			// Second call for final marshaling
			return nil, fmt.Errorf("final marshal error")
		}

		// And valid blueprint data
		data := map[string]any{
			"blueprint": map[string]any{
				"metadata": map[string]any{
					"name": "test-blueprint",
				},
			},
		}

		// When Generate is called
		err := generator.Generate(data)

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected error from final MarshalYAML, got nil")
		}
	})

	t.Run("ErrorCreatingDirectory", func(t *testing.T) {
		// Given a BlueprintGenerator with mocks
		generator, mocks := setupBlueprintGeneratorMocks(t)

		// And MkdirAll returns an error
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mkdir error")
		}

		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return []byte("kind: Blueprint\napiVersion: v1alpha1\n"), nil
		}

		// And valid blueprint data
		data := map[string]any{
			"blueprint": map[string]any{
				"metadata": map[string]any{
					"name": "test-blueprint",
				},
			},
		}

		// When Generate is called
		err := generator.Generate(data)

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected error from MkdirAll, got nil")
		}
	})

	t.Run("ErrorWritingFile", func(t *testing.T) {
		// Given a BlueprintGenerator with mocks
		generator, mocks := setupBlueprintGeneratorMocks(t)

		// And WriteFile returns an error
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("write error")
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return []byte("kind: Blueprint\napiVersion: v1alpha1\n"), nil
		}

		// And valid blueprint data
		data := map[string]any{
			"blueprint": map[string]any{
				"metadata": map[string]any{
					"name": "test-blueprint",
				},
			},
		}

		// When Generate is called
		err := generator.Generate(data)

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected error from WriteFile, got nil")
		}
	})

	t.Run("FileExistsNoOverwrite", func(t *testing.T) {
		// Given a BlueprintGenerator with mocks
		generator, mocks := setupBlueprintGeneratorMocks(t)

		// And mock file operations to simulate existing file
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return &mockFileInfo{name: "blueprint.yaml"}, nil // File exists
		}

		// And template data with blueprint configuration
		data := map[string]any{
			"blueprint": map[string]any{
				"metadata": map[string]any{
					"name": "test-blueprint",
				},
			},
		}

		// When Generate is called without overwrite
		err := generator.Generate(data, false)

		// Then no error should occur (file exists, skip generation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("FileExistsWithOverwrite", func(t *testing.T) {
		// Given a BlueprintGenerator with mocks
		generator, mocks := setupBlueprintGeneratorMocks(t)

		// And mock file operations
		var writtenPath string
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			writtenPath = name
			return nil
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return &mockFileInfo{name: "blueprint.yaml"}, nil // File exists
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return []byte("kind: Blueprint\napiVersion: v1alpha1\n"), nil
		}

		// And template data with blueprint configuration
		data := map[string]any{
			"blueprint": map[string]any{
				"metadata": map[string]any{
					"name": "test-blueprint",
				},
			},
		}

		// When Generate is called with overwrite
		err := generator.Generate(data, true)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the file should be written
		expectedPath := filepath.Join("/test/context", "blueprint.yaml")
		if writtenPath != expectedPath {
			t.Errorf("Expected file path %s, got %s", expectedPath, writtenPath)
		}
	})
}

func TestBlueprintGenerator_createBlueprintFromData(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a BlueprintGenerator
		generator, _ := setupBlueprintGeneratorMocks(t)

		// And complete blueprint data
		data := map[string]any{
			"metadata": map[string]any{
				"name":        "test-blueprint",
				"description": "A test blueprint",
				"authors":     []any{"author1", "author2"},
			},
			"repository": map[string]any{
				"url": "https://github.com/example/repo",
				"ref": map[string]any{
					"branch": "main",
				},
				"secretName": "repo-secret",
			},
			"sources": []any{
				map[string]any{
					"name":       "test-source",
					"url":        "https://github.com/example/source",
					"pathPrefix": "terraform",
					"ref": map[string]any{
						"tag": "v1.0.0",
					},
					"secretName": "source-secret",
				},
			},
			"terraform": []any{
				map[string]any{
					"path":   "cluster",
					"source": "github.com/example/terraform-cluster",
					"values": map[string]any{
						"cluster_name": "test-cluster",
					},
					"destroy": true,
				},
			},
			"kustomize": []any{
				map[string]any{
					"name":       "test-kustomization",
					"path":       "kustomize/test",
					"source":     "github.com/example/kustomize",
					"dependsOn":  []any{"terraform"},
					"components": []any{"component1"},
					"cleanup":    []any{"cleanup1"},
					"wait":       true,
					"force":      false,
					"prune":      true,
				},
			},
		}

		// When createBlueprintFromData is called
		blueprint, err := generator.createBlueprintFromData(data)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the blueprint should be properly structured
		if blueprint.Kind != "Blueprint" {
			t.Errorf("Expected Kind to be 'Blueprint', got %s", blueprint.Kind)
		}

		if blueprint.ApiVersion != "v1alpha1" {
			t.Errorf("Expected ApiVersion to be 'v1alpha1', got %s", blueprint.ApiVersion)
		}

		if blueprint.Metadata.Name != "test-blueprint" {
			t.Errorf("Expected metadata name to be 'test-blueprint', got %s", blueprint.Metadata.Name)
		}

		if blueprint.Metadata.Description != "A test blueprint" {
			t.Errorf("Expected metadata description to be 'A test blueprint', got %s", blueprint.Metadata.Description)
		}

		if len(blueprint.Metadata.Authors) != 2 {
			t.Errorf("Expected 2 authors, got %d", len(blueprint.Metadata.Authors))
		}

		if blueprint.Repository.Url != "https://github.com/example/repo" {
			t.Errorf("Expected repository URL to be 'https://github.com/example/repo', got %s", blueprint.Repository.Url)
		}

		if blueprint.Repository.Ref.Branch != "main" {
			t.Errorf("Expected repository branch to be 'main', got %s", blueprint.Repository.Ref.Branch)
		}

		if len(blueprint.Sources) != 1 {
			t.Errorf("Expected 1 source, got %d", len(blueprint.Sources))
		}

		if blueprint.Sources[0].Name != "test-source" {
			t.Errorf("Expected source name to be 'test-source', got %s", blueprint.Sources[0].Name)
		}

		if len(blueprint.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(blueprint.TerraformComponents))
		}

		if blueprint.TerraformComponents[0].Path != "cluster" {
			t.Errorf("Expected terraform component path to be 'cluster', got %s", blueprint.TerraformComponents[0].Path)
		}

		if len(blueprint.Kustomizations) != 1 {
			t.Errorf("Expected 1 kustomization, got %d", len(blueprint.Kustomizations))
		}

		if blueprint.Kustomizations[0].Name != "test-kustomization" {
			t.Errorf("Expected kustomization name to be 'test-kustomization', got %s", blueprint.Kustomizations[0].Name)
		}
	})

	t.Run("EmptyData", func(t *testing.T) {
		// Given a BlueprintGenerator
		generator, _ := setupBlueprintGeneratorMocks(t)

		// And empty data
		data := map[string]any{}

		// When createBlueprintFromData is called
		blueprint, err := generator.createBlueprintFromData(data)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the blueprint should have default values
		if blueprint.Kind != "Blueprint" {
			t.Errorf("Expected Kind to be 'Blueprint', got %s", blueprint.Kind)
		}

		if blueprint.ApiVersion != "v1alpha1" {
			t.Errorf("Expected ApiVersion to be 'v1alpha1', got %s", blueprint.ApiVersion)
		}
	})

	t.Run("ErrorMarshalingData", func(t *testing.T) {
		// Given a BlueprintGenerator with mocks
		generator, mocks := setupBlueprintGeneratorMocks(t)

		// And MarshalYAML returns an error
		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("marshal error")
		}

		// And valid data
		data := map[string]any{
			"metadata": map[string]any{
				"name": "test-blueprint",
			},
		}

		// When createBlueprintFromData is called
		blueprint, err := generator.createBlueprintFromData(data)

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected error from MarshalYAML, got nil")
		}

		// And blueprint should be nil
		if blueprint != nil {
			t.Errorf("Expected blueprint to be nil, got %v", blueprint)
		}
	})

	t.Run("ErrorUnmarshalingYAML", func(t *testing.T) {
		// Given a BlueprintGenerator with mocks
		generator, mocks := setupBlueprintGeneratorMocks(t)

		// And MarshalYAML succeeds but YamlUnmarshal fails
		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return []byte("valid-yaml"), nil
		}

		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			return fmt.Errorf("unmarshal error")
		}

		// And valid data
		data := map[string]any{
			"metadata": map[string]any{
				"name": "test-blueprint",
			},
		}

		// When createBlueprintFromData is called
		blueprint, err := generator.createBlueprintFromData(data)

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected error from YamlUnmarshal, got nil")
		}

		// And blueprint should be nil
		if blueprint != nil {
			t.Errorf("Expected blueprint to be nil, got %v", blueprint)
		}
	})

	t.Run("PartialData", func(t *testing.T) {
		// Given a BlueprintGenerator
		generator, _ := setupBlueprintGeneratorMocks(t)

		// And partial blueprint data (only metadata)
		data := map[string]any{
			"metadata": map[string]any{
				"name": "partial-blueprint",
			},
		}

		// When createBlueprintFromData is called
		blueprint, err := generator.createBlueprintFromData(data)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the blueprint should have the provided data
		if blueprint.Metadata.Name != "partial-blueprint" {
			t.Errorf("Expected metadata name to be 'partial-blueprint', got %s", blueprint.Metadata.Name)
		}

		// And default values should be set
		if blueprint.Kind != "Blueprint" {
			t.Errorf("Expected Kind to be 'Blueprint', got %s", blueprint.Kind)
		}

		if blueprint.ApiVersion != "v1alpha1" {
			t.Errorf("Expected ApiVersion to be 'v1alpha1', got %s", blueprint.ApiVersion)
		}

		// And optional fields should be empty
		if len(blueprint.Sources) != 0 {
			t.Errorf("Expected 0 sources, got %d", len(blueprint.Sources))
		}

		if len(blueprint.TerraformComponents) != 0 {
			t.Errorf("Expected 0 terraform components, got %d", len(blueprint.TerraformComponents))
		}
	})

	t.Run("ComplexNestedData", func(t *testing.T) {
		// Given a BlueprintGenerator
		generator, _ := setupBlueprintGeneratorMocks(t)

		// And complex nested blueprint data
		data := map[string]any{
			"metadata": map[string]any{
				"name":        "complex-blueprint",
				"description": "A complex blueprint with nested structures",
				"authors":     []any{"author1", "author2", "author3"},
			},
			"repository": map[string]any{
				"url": "https://github.com/complex/repo",
				"ref": map[string]any{
					"tag":    "v2.0.0",
					"commit": "abc123",
				},
				"secretName": "complex-secret",
			},
			"sources": []any{
				map[string]any{
					"name":       "source1",
					"url":        "https://github.com/example/source1",
					"pathPrefix": "modules",
					"ref": map[string]any{
						"branch": "develop",
					},
				},
				map[string]any{
					"name": "source2",
					"url":  "https://github.com/example/source2",
					"ref": map[string]any{
						"semver": ">=1.0.0",
					},
				},
			},
			"terraform": []any{
				map[string]any{
					"path":   "networking",
					"source": "github.com/example/terraform-network",
					"values": map[string]any{
						"vpc_cidr":     "10.0.0.0/16",
						"subnet_count": 3,
						"enable_nat":   true,
					},
					"destroy": false,
				},
				map[string]any{
					"path": "compute",
					"values": map[string]any{
						"instance_type": "t3.medium",
						"min_size":      2,
						"max_size":      10,
					},
				},
			},
		}

		// When createBlueprintFromData is called
		blueprint, err := generator.createBlueprintFromData(data)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the complex data should be properly processed
		if blueprint.Metadata.Name != "complex-blueprint" {
			t.Errorf("Expected metadata name to be 'complex-blueprint', got %s", blueprint.Metadata.Name)
		}

		if len(blueprint.Metadata.Authors) != 3 {
			t.Errorf("Expected 3 authors, got %d", len(blueprint.Metadata.Authors))
		}

		if len(blueprint.Sources) != 2 {
			t.Errorf("Expected 2 sources, got %d", len(blueprint.Sources))
		}

		if len(blueprint.TerraformComponents) != 2 {
			t.Errorf("Expected 2 terraform components, got %d", len(blueprint.TerraformComponents))
		}

		// Verify nested values are preserved
		if blueprint.TerraformComponents[0].Values["vpc_cidr"] != "10.0.0.0/16" {
			t.Errorf("Expected vpc_cidr to be '10.0.0.0/16', got %v", blueprint.TerraformComponents[0].Values["vpc_cidr"])
		}

		if blueprint.TerraformComponents[0].Destroy == nil || *blueprint.TerraformComponents[0].Destroy != false {
			t.Errorf("Expected destroy to be false, got %v", blueprint.TerraformComponents[0].Destroy)
		}
	})
}

// =============================================================================
// Interface Compliance
// =============================================================================

func TestBlueprintGenerator_ImplementsGenerator(t *testing.T) {
	mocks := setupMocks(t)
	generator := NewBlueprintGenerator(mocks.Injector)

	// Test that BlueprintGenerator implements Generator interface
	var _ Generator = generator
}
