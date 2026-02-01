package blueprint

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type LoaderTestMocks struct {
	Shell           *shell.MockShell
	ConfigHandler   *config.MockConfigHandler
	ArtifactBuilder *artifact.MockArtifact
	Runtime         *runtime.Runtime
	Shims           *Shims
	TmpDir          string
}

func setupLoaderMocks(t *testing.T) *LoaderTestMocks {
	t.Helper()

	tmpDir := t.TempDir()
	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return tmpDir, nil
	}
	mockConfigHandler.LoadSchemaFromBytesFunc = func(data []byte) error {
		return nil
	}

	mockArtifact := artifact.NewMockArtifact()

	rt := &runtime.Runtime{
		ProjectRoot:   tmpDir,
		ConfigRoot:    tmpDir,
		TemplateRoot:  filepath.Join(tmpDir, "_template"),
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
	}

	mocks := &LoaderTestMocks{
		Shell:           mockShell,
		ConfigHandler:   mockConfigHandler,
		ArtifactBuilder: mockArtifact,
		Runtime:         rt,
		Shims:           NewShims(),
		TmpDir:          tmpDir,
	}

	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
	})

	return mocks
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewBlueprintLoader(t *testing.T) {
	t.Run("CreatesLoaderWithDefaults", func(t *testing.T) {
		// Given a runtime and artifact builder
		mocks := setupLoaderMocks(t)

		// When creating a new loader
		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// Then loader should be created with defaults
		if loader == nil {
			t.Fatal("Expected loader to be created")
		}
		if loader.runtime != mocks.Runtime {
			t.Error("Expected runtime to be set")
		}
		if loader.artifactBuilder != mocks.ArtifactBuilder {
			t.Error("Expected artifact builder to be set")
		}
		if loader.shims == nil {
			t.Error("Expected shims to be initialized")
		}
		if loader.templateData == nil {
			t.Error("Expected templateData map to be initialized")
		}
	})

	t.Run("CreatesLoaderWithArtifactBuilder", func(t *testing.T) {
		// Given a runtime and artifact builder
		mocks := setupLoaderMocks(t)

		// When creating a new loader
		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// Then loader should be created
		if loader == nil {
			t.Fatal("Expected loader to be created")
		}
		if loader.runtime != mocks.Runtime {
			t.Error("Expected runtime to be set")
		}
		if loader.artifactBuilder != mocks.ArtifactBuilder {
			t.Error("Expected artifact builder to be set")
		}
	})

	t.Run("CreatesLoaderWithDefaultShims", func(t *testing.T) {
		// Given a loader
		mocks := setupLoaderMocks(t)

		// When creating a loader
		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)
		_ = loader.Load("test", "")

		// Then loader should have default shims
		if loader.shims == nil {
			t.Error("Expected default shims to be set")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestLoader_GetBlueprint(t *testing.T) {
	t.Run("ReturnsNilWhenNotLoaded", func(t *testing.T) {
		// Given a loader that has not loaded
		mocks := setupLoaderMocks(t)
		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)
		_ = loader.Load("test", "")

		// When getting blueprint
		bp := loader.GetBlueprint()

		// Then should return nil
		if bp != nil {
			t.Error("Expected nil blueprint before load")
		}
	})

	t.Run("ReturnsBlueprintAfterSet", func(t *testing.T) {
		// Given a loader with blueprint set
		mocks := setupLoaderMocks(t)
		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)
		_ = loader.Load("test", "")
		expected := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
		}
		loader.blueprint = expected

		// When getting blueprint
		bp := loader.GetBlueprint()

		// Then should return the blueprint
		if bp != expected {
			t.Error("Expected set blueprint to be returned")
		}
	})
}

func TestLoader_GetBlueprintPath(t *testing.T) {
	t.Run("ReturnsAbsolutePathForUserBlueprintWhenLoaded", func(t *testing.T) {
		mocks := setupLoaderMocks(t)
		blueprintYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: user-blueprint
`
		blueprintPath := filepath.Join(mocks.TmpDir, "blueprint.yaml")
		os.WriteFile(blueprintPath, []byte(blueprintYaml), 0644)
		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		err := loader.Load("user", "")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		path := loader.GetBlueprintPath()
		if path == "" {
			t.Error("Expected non-empty absolute path for user blueprint")
		}
		absExpected, _ := filepath.Abs(blueprintPath)
		if path != absExpected {
			t.Errorf("Expected GetBlueprintPath %q, got %q", absExpected, path)
		}
	})

	t.Run("ReturnsEmptyForTemplateLoader", func(t *testing.T) {
		mocks := setupLoaderMocks(t)
		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)
		blueprintYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test
`
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(blueprintYaml), 0644)
		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		err := loader.Load("primary", "")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		path := loader.GetBlueprintPath()
		if path != "" {
			t.Errorf("Expected empty path for template loader, got %q", path)
		}
	})
}

func TestLoader_GetFacets(t *testing.T) {
	t.Run("ReturnsEmptyWhenNotLoaded", func(t *testing.T) {
		// Given a loader that has not loaded
		mocks := setupLoaderMocks(t)
		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)
		_ = loader.Load("test", "")

		// When getting facets
		facets := loader.GetFacets()

		// Then should return nil
		if facets != nil {
			t.Error("Expected nil facets before load")
		}
	})

	t.Run("ReturnsFacetsAfterSet", func(t *testing.T) {
		// Given a loader with facets set
		mocks := setupLoaderMocks(t)
		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)
		_ = loader.Load("test", "")
		expected := []blueprintv1alpha1.Facet{
			{Metadata: blueprintv1alpha1.Metadata{Name: "facet1"}},
		}
		loader.facets = expected

		// When getting facets
		facets := loader.GetFacets()

		// Then should return the facets
		if len(facets) != 1 {
			t.Errorf("Expected 1 facet, got %d", len(facets))
		}
	})
}

func TestLoader_GetTemplateData(t *testing.T) {
	t.Run("ReturnsEmptyMapByDefault", func(t *testing.T) {
		// Given a new loader
		mocks := setupLoaderMocks(t)
		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)
		_ = loader.Load("test", "")

		// When getting template data
		data := loader.GetTemplateData()

		// Then should return empty map
		if data == nil {
			t.Error("Expected non-nil template data map")
		}
		if len(data) != 0 {
			t.Errorf("Expected empty map, got %d entries", len(data))
		}
	})

	t.Run("ReturnsTemplateDataAfterSet", func(t *testing.T) {
		// Given a loader with template data set
		mocks := setupLoaderMocks(t)
		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)
		_ = loader.Load("test", "")
		loader.templateData["test.yaml"] = []byte("content")

		// When getting template data
		data := loader.GetTemplateData()

		// Then should return the data
		if len(data) != 1 {
			t.Errorf("Expected 1 entry, got %d", len(data))
		}
	})
}

func TestLoader_GetSourceName(t *testing.T) {
	t.Run("ReturnsSourceName", func(t *testing.T) {
		// Given a loader with source name
		mocks := setupLoaderMocks(t)
		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)
		_ = loader.Load("my-source", "")

		// When getting source name
		name := loader.GetSourceName()

		// Then should return the name
		if name != "my-source" {
			t.Errorf("Expected 'my-source', got '%s'", name)
		}
	})
}

func TestLoader_Load(t *testing.T) {
	t.Run("LoadsFromLocalTemplateDirectory", func(t *testing.T) {
		// Given a loader for primary with local template
		mocks := setupLoaderMocks(t)

		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)
		blueprintYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test-blueprint
`
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(blueprintYaml), 0644)

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("primary", "")

		// Then blueprint should be loaded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if loader.blueprint == nil {
			t.Fatal("Expected blueprint to be loaded")
		}
		if loader.blueprint.Metadata.Name != "test-blueprint" {
			t.Errorf("Expected name='test-blueprint', got '%s'", loader.blueprint.Metadata.Name)
		}
	})

	t.Run("LoadsFacetsFromLocalTemplate", func(t *testing.T) {
		// Given a loader with facets directory
		mocks := setupLoaderMocks(t)

		templateDir := mocks.Runtime.TemplateRoot
		facetsDir := filepath.Join(templateDir, "facets")
		os.MkdirAll(facetsDir, 0755)

		blueprintYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test
`
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(blueprintYaml), 0644)

		facetYaml := `kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: vpc-facet
terraform:
  - path: vpc
`
		os.WriteFile(filepath.Join(facetsDir, "vpc.yaml"), []byte(facetYaml), 0644)

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("primary", "")

		// Then facets should be loaded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(loader.facets) != 1 {
			t.Fatalf("Expected 1 facet, got %d", len(loader.facets))
		}
		if loader.facets[0].Metadata.Name != "vpc-facet" {
			t.Errorf("Expected facet name='vpc-facet', got '%s'", loader.facets[0].Metadata.Name)
		}
	})

	t.Run("LoadsFeaturesFromLocalTemplateForBackwardsCompatibility", func(t *testing.T) {
		// Given a loader with features directory (backwards compatibility)
		mocks := setupLoaderMocks(t)

		templateDir := mocks.Runtime.TemplateRoot
		featuresDir := filepath.Join(templateDir, "features")
		os.MkdirAll(featuresDir, 0755)

		blueprintYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test
`
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(blueprintYaml), 0644)

		featureYaml := `kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-feature
terraform:
  - path: vpc
`
		os.WriteFile(filepath.Join(featuresDir, "aws.yaml"), []byte(featureYaml), 0644)

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("primary", "")

		// Then features should be loaded and converted to facets
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(loader.facets) != 1 {
			t.Fatalf("Expected 1 facet, got %d", len(loader.facets))
		}
		if loader.facets[0].Metadata.Name != "aws-feature" {
			t.Errorf("Expected facet name='aws-feature', got '%s'", loader.facets[0].Metadata.Name)
		}
		if loader.facets[0].Kind != "Facet" {
			t.Errorf("Expected kind to be normalized to 'Facet', got '%s'", loader.facets[0].Kind)
		}
	})

	t.Run("LoadsBothFacetsAndFeatures", func(t *testing.T) {
		// Given a loader with both facets and features directories
		mocks := setupLoaderMocks(t)

		templateDir := mocks.Runtime.TemplateRoot
		facetsDir := filepath.Join(templateDir, "facets")
		featuresDir := filepath.Join(templateDir, "features")
		os.MkdirAll(facetsDir, 0755)
		os.MkdirAll(featuresDir, 0755)

		blueprintYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test
`
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(blueprintYaml), 0644)

		facetYaml := `kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: new-facet
terraform:
  - path: network
`
		os.WriteFile(filepath.Join(facetsDir, "network.yaml"), []byte(facetYaml), 0644)

		featureYaml := `kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: legacy-feature
terraform:
  - path: legacy
`
		os.WriteFile(filepath.Join(featuresDir, "legacy.yaml"), []byte(featureYaml), 0644)

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("primary", "")

		// Then both should be loaded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(loader.facets) != 2 {
			t.Fatalf("Expected 2 facets, got %d", len(loader.facets))
		}

		facetNames := make(map[string]bool)
		for _, facet := range loader.facets {
			facetNames[facet.Metadata.Name] = true
			if facet.Kind != "Facet" {
				t.Errorf("Expected all kinds to be 'Facet', got '%s' for %s", facet.Kind, facet.Metadata.Name)
			}
		}

		if !facetNames["new-facet"] {
			t.Error("Expected 'new-facet' to be loaded")
		}
		if !facetNames["legacy-feature"] {
			t.Error("Expected 'legacy-feature' to be loaded")
		}
	})

	t.Run("LoadsUserBlueprintFromConfigRoot", func(t *testing.T) {
		// Given a user loader with blueprint in config root
		mocks := setupLoaderMocks(t)

		blueprintYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: user-blueprint
terraform:
  - path: vpc
`
		os.WriteFile(filepath.Join(mocks.TmpDir, "blueprint.yaml"), []byte(blueprintYaml), 0644)

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("user", "")

		// Then user blueprint should be loaded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if loader.blueprint == nil {
			t.Fatal("Expected blueprint to be loaded")
		}
		if loader.blueprint.Metadata.Name != "user-blueprint" {
			t.Errorf("Expected name='user-blueprint', got '%s'", loader.blueprint.Metadata.Name)
		}
	})

	t.Run("ReturnsNilForUserWhenNoBlueprintExists", func(t *testing.T) {
		// Given a user loader with no blueprint file
		mocks := setupLoaderMocks(t)
		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("user", "")

		// Then should succeed with nil blueprint
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if loader.blueprint != nil {
			t.Error("Expected nil blueprint when file doesn't exist")
		}
	})

	t.Run("PreservesRefWhenOCIURLHasPortInAuthority", func(t *testing.T) {
		mocks := setupLoaderMocks(t)
		blueprintYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test
sources:
  - name: registry
    url: oci://localhost:5000/repo
    ref:
      tag: v1.0.0
`
		os.WriteFile(filepath.Join(mocks.TmpDir, "blueprint.yaml"), []byte(blueprintYaml), 0644)
		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		err := loader.Load("user", "")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		bp := loader.GetBlueprint()
		if bp == nil || len(bp.Sources) != 1 {
			t.Fatalf("Expected one source, got blueprint=%v", bp)
		}
		if bp.Sources[0].Ref.Tag != "v1.0.0" {
			t.Errorf("Expected Ref.Tag preserved (port in URL is not a tag), got %q", bp.Sources[0].Ref.Tag)
		}
	})

	t.Run("CollectsTemplateData", func(t *testing.T) {
		// Given a loader with template files
		mocks := setupLoaderMocks(t)

		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)

		blueprintYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test
`
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(blueprintYaml), 0644)
		os.WriteFile(filepath.Join(templateDir, "schema.yaml"), []byte("type: object"), 0644)

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("primary", "")

		// Then template data should be collected
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(loader.templateData) < 1 {
			t.Errorf("Expected template data to be collected, got %d entries", len(loader.templateData))
		}
	})

	t.Run("ReturnsErrorForInvalidBlueprintYaml", func(t *testing.T) {
		// Given a loader with invalid blueprint yaml
		mocks := setupLoaderMocks(t)

		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte("invalid: [yaml"), 0644)

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("primary", "")

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid YAML")
		}
	})

	t.Run("ReturnsErrorForInvalidFacetYaml", func(t *testing.T) {
		// Given a loader with invalid feature yaml
		mocks := setupLoaderMocks(t)

		templateDir := mocks.Runtime.TemplateRoot
		facetsDir := filepath.Join(templateDir, "facets")
		os.MkdirAll(facetsDir, 0755)

		blueprintYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test
`
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(blueprintYaml), 0644)
		os.WriteFile(filepath.Join(facetsDir, "bad.yaml"), []byte("invalid: [yaml"), 0644)

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("primary", "")

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid feature YAML")
		}
	})

	t.Run("LoadsFromOCIArtifact", func(t *testing.T) {
		// Given a loader with OCI source URL
		mocks := setupLoaderMocks(t)

		cacheDir := filepath.Join(mocks.TmpDir, "cache")
		templateDir := filepath.Join(cacheDir, "_template")
		os.MkdirAll(templateDir, 0755)

		blueprintYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: oci-blueprint
`
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(blueprintYaml), 0644)

		mocks.ArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			return map[string]string{
				"example.com/blueprint:v1.0.0": cacheDir,
			}, nil
		}
		mocks.ArtifactBuilder.ParseOCIRefFunc = func(ref string) (string, string, string, error) {
			return "example.com", "blueprint", "v1.0.0", nil
		}

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("external", "oci://example.com/blueprint:v1.0.0")

		// Then OCI blueprint should be loaded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if loader.blueprint == nil {
			t.Fatal("Expected blueprint to be loaded from OCI")
		}
		if loader.blueprint.Metadata.Name != "oci-blueprint" {
			t.Errorf("Expected name='oci-blueprint', got '%s'", loader.blueprint.Metadata.Name)
		}
	})

	t.Run("ReturnsErrorWhenOCIPullFails", func(t *testing.T) {
		// Given a loader where OCI pull fails
		mocks := setupLoaderMocks(t)

		mocks.ArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			return nil, os.ErrNotExist
		}

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("external", "oci://example.com/blueprint:v1.0.0")

		// Then should return error
		if err == nil {
			t.Error("Expected error when OCI pull fails")
		}
	})

	t.Run("LoadsSchemaIntoConfigHandler", func(t *testing.T) {
		// Given a loader with schema.yaml
		mocks := setupLoaderMocks(t)

		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)

		blueprintYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test
`
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(blueprintYaml), 0644)

		schemaYaml := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  cluster:
    type: object
`
		os.WriteFile(filepath.Join(templateDir, "schema.yaml"), []byte(schemaYaml), 0644)

		schemaLoaded := false
		mocks.ConfigHandler.LoadSchemaFromBytesFunc = func(data []byte) error {
			schemaLoaded = true
			return nil
		}

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("primary", "")

		// Then schema should be loaded into ConfigHandler
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !schemaLoaded {
			t.Error("Expected schema to be loaded into ConfigHandler")
		}
	})

	t.Run("SkipsSchemaLoadingWhenNoSchemaFile", func(t *testing.T) {
		// Given a loader without schema.yaml
		mocks := setupLoaderMocks(t)

		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)

		blueprintYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test
`
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(blueprintYaml), 0644)

		schemaLoaded := false
		mocks.ConfigHandler.LoadSchemaFromBytesFunc = func(data []byte) error {
			schemaLoaded = true
			return nil
		}

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("primary", "")

		// Then schema should NOT be loaded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if schemaLoaded {
			t.Error("Expected schema NOT to be loaded when no schema.yaml exists")
		}
	})

	t.Run("ReturnsErrorWhenSchemaLoadFails", func(t *testing.T) {
		// Given a loader where schema loading fails
		mocks := setupLoaderMocks(t)

		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)

		blueprintYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test
`
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(blueprintYaml), 0644)
		os.WriteFile(filepath.Join(templateDir, "schema.yaml"), []byte("schema: test"), 0644)

		mocks.ConfigHandler.LoadSchemaFromBytesFunc = func(data []byte) error {
			return errors.New("schema load failed")
		}

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("primary", "")

		// Then should return error
		if err == nil {
			t.Error("Expected error when schema load fails")
		}
	})

	t.Run("ReturnsErrorWhenUserBlueprintReadFails", func(t *testing.T) {
		// Given a loader where blueprint file exists but can't be read
		mocks := setupLoaderMocks(t)
		configRoot := mocks.Runtime.ConfigRoot

		os.MkdirAll(configRoot, 0755)
		blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
		os.WriteFile(blueprintPath, []byte("invalid yaml: ["), 0644)

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("user", "")

		// Then should return error
		if err == nil {
			t.Error("Expected error when blueprint YAML is invalid")
		}
	})

	t.Run("SkipsSchemaLoadingWhenConfigHandlerNil", func(t *testing.T) {
		// Given a loader with nil ConfigHandler
		mocks := setupLoaderMocks(t)
		mocks.Runtime.ConfigHandler = nil

		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)

		blueprintYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test
`
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(blueprintYaml), 0644)
		os.WriteFile(filepath.Join(templateDir, "schema.yaml"), []byte("schema: test"), 0644)

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("primary", "")

		// Then should not error (schema loading is skipped when ConfigHandler is nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("LoadsOCIWithTemplateDirectory", func(t *testing.T) {
		// Given an OCI artifact with _template directory
		mocks := setupLoaderMocks(t)

		cacheDir := t.TempDir()
		templateDir := filepath.Join(cacheDir, "_template")
		os.MkdirAll(templateDir, 0755)

		blueprintYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: oci-blueprint
`
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(blueprintYaml), 0644)

		mocks.ArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			return map[string]string{"ghcr.io/test/blueprint:v1.0.0": cacheDir}, nil
		}
		mocks.ArtifactBuilder.ParseOCIRefFunc = func(ref string) (string, string, string, error) {
			return "ghcr.io", "test/blueprint", "v1.0.0", nil
		}

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("source", "oci://ghcr.io/test/blueprint:v1.0.0")

		// Then should load successfully
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if loader.GetBlueprint() == nil {
			t.Error("Expected blueprint to be loaded")
		}
		if loader.GetBlueprint().Metadata.Name != "oci-blueprint" {
			t.Errorf("Expected name='oci-blueprint', got '%s'", loader.GetBlueprint().Metadata.Name)
		}
	})

	t.Run("LoadsOCIWithoutTemplateDirectory", func(t *testing.T) {
		// Given an OCI artifact without _template directory (flat structure)
		mocks := setupLoaderMocks(t)

		cacheDir := t.TempDir()
		blueprintYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: flat-oci
`
		os.WriteFile(filepath.Join(cacheDir, "blueprint.yaml"), []byte(blueprintYaml), 0644)

		mocks.ArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			return map[string]string{"ghcr.io/test/flat:v1.0.0": cacheDir}, nil
		}
		mocks.ArtifactBuilder.ParseOCIRefFunc = func(ref string) (string, string, string, error) {
			return "ghcr.io", "test/flat", "v1.0.0", nil
		}

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("source", "oci://ghcr.io/test/flat:v1.0.0")

		// Then should load from flat structure
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if loader.GetBlueprint() == nil {
			t.Error("Expected blueprint to be loaded")
		}
	})

	t.Run("ReturnsErrorWhenOCIParseFails", func(t *testing.T) {
		// Given an invalid OCI URL
		mocks := setupLoaderMocks(t)

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("source", "invalid-url")

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid OCI URL")
		}
	})

	t.Run("LoadsOCIWithSchemaAndFacets", func(t *testing.T) {
		// Given an OCI artifact with schema and facets
		mocks := setupLoaderMocks(t)

		cacheDir := t.TempDir()
		templateDir := filepath.Join(cacheDir, "_template")
		facetsDir := filepath.Join(templateDir, "facets")
		os.MkdirAll(facetsDir, 0755)

		blueprintYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: oci-with-features
`
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(blueprintYaml), 0644)
		os.WriteFile(filepath.Join(templateDir, "schema.yaml"), []byte("$schema: test"), 0644)

		facetYaml := `kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`
		os.WriteFile(filepath.Join(facetsDir, "base.yaml"), []byte(facetYaml), 0644)

		mocks.ArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			return map[string]string{"ghcr.io/test/full:v1.0.0": cacheDir}, nil
		}
		mocks.ArtifactBuilder.ParseOCIRefFunc = func(ref string) (string, string, string, error) {
			return "ghcr.io", "test/full", "v1.0.0", nil
		}

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("source", "oci://ghcr.io/test/full:v1.0.0")

		// Then should load schema and facets
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(loader.GetFacets()) != 1 {
			t.Errorf("Expected 1 facet, got %d", len(loader.GetFacets()))
		}
	})

	t.Run("ReturnsErrorWhenOCIPullFails", func(t *testing.T) {
		// Given an OCI URL with failing pull
		mocks := setupLoaderMocks(t)

		mocks.ArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			return nil, errors.New("network error")
		}
		mocks.ArtifactBuilder.ParseOCIRefFunc = func(ref string) (string, string, string, error) {
			return "ghcr.io", "test/repo", "v1.0.0", nil
		}

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("source", "oci://ghcr.io/test/repo:v1.0.0")

		// Then should return error
		if err == nil {
			t.Error("Expected error for failed OCI pull")
		}
	})

	t.Run("ReturnsErrorWhenCacheKeyMissing", func(t *testing.T) {
		// Given an OCI artifact where cache key doesn't match
		mocks := setupLoaderMocks(t)

		mocks.ArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			return map[string]string{"wrong/key:v1.0.0": "/tmp/cache"}, nil
		}
		mocks.ArtifactBuilder.ParseOCIRefFunc = func(ref string) (string, string, string, error) {
			return "ghcr.io", "test/repo", "v1.0.0", nil
		}

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("source", "oci://ghcr.io/test/repo:v1.0.0")

		// Then should return error
		if err == nil {
			t.Error("Expected error for missing cache key")
		}
	})

	t.Run("ReturnsErrorWhenLoadingOCISourceWithoutArtifactBuilder", func(t *testing.T) {
		// Given a loader without artifact builder
		mocks := setupLoaderMocks(t)
		loader := NewBlueprintLoader(mocks.Runtime, nil)

		// When loading an OCI source
		err := loader.Load("source", "oci://ghcr.io/test/repo:v1.0.0")

		// Then should return error
		if err == nil {
			t.Error("Expected error when loading OCI source without artifact builder")
		}
		if !strings.Contains(err.Error(), "artifact builder is required") {
			t.Errorf("Expected error about artifact builder, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenBlueprintReadFails", func(t *testing.T) {
		// Given a local template with unreadable blueprint.yaml
		mocks := setupLoaderMocks(t)
		mocks.Runtime.TemplateRoot = filepath.Join(mocks.TmpDir, "_template")
		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(`kind: Blueprint`), 0644)

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)
		_ = loader.Load("primary", "")
		loader.shims.ReadFile = func(path string) ([]byte, error) {
			return nil, errors.New("permission denied")
		}

		// When loading
		err := loader.Load("primary", "")

		// Then should return error
		if err == nil {
			t.Error("Expected error when blueprint read fails")
		}
	})

	t.Run("ReturnsErrorWhenBlueprintParseFails", func(t *testing.T) {
		// Given a local template with malformed blueprint.yaml
		mocks := setupLoaderMocks(t)
		mocks.Runtime.TemplateRoot = filepath.Join(mocks.TmpDir, "_template")
		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(`{{{invalid yaml`), 0644)

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading
		err := loader.Load("primary", "")

		// Then should return error
		if err == nil {
			t.Error("Expected error when blueprint parse fails")
		}
	})

	t.Run("ReturnsErrorWhenTemplateDataCollectionFails", func(t *testing.T) {
		// Given a template directory with read permission issues
		mocks := setupLoaderMocks(t)
		mocks.Runtime.TemplateRoot = filepath.Join(mocks.TmpDir, "_template")
		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)

		loader := NewBlueprintLoader(mocks.Runtime, mocks.ArtifactBuilder)
		_ = loader.Load("primary", "")
		loader.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			return errors.New("walk failed")
		}

		// When loading
		err := loader.Load("primary", "")

		// Then should return error
		if err == nil {
			t.Error("Expected error when template data collection fails")
		}
	})
}
