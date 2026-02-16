package blueprint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/terraform"
)

// =============================================================================
// Test Setup
// =============================================================================

type HandlerTestMocks struct {
	Shell           *shell.MockShell
	ConfigHandler   *config.MockConfigHandler
	ArtifactBuilder *artifact.MockArtifact
	Evaluator       *evaluator.MockExpressionEvaluator
	Runtime         *runtime.Runtime
	Shims           *Shims
}

func setupHandlerMocks(t *testing.T) *HandlerTestMocks {
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

	mockArtifact := artifact.NewMockArtifact()

	realEvaluator := evaluator.NewExpressionEvaluator(mockConfigHandler, tmpDir, tmpDir)
	mockEvaluator := evaluator.NewMockExpressionEvaluator()

	mockEvaluator.EvaluateFunc = realEvaluator.Evaluate
	mockEvaluator.EvaluateMapFunc = realEvaluator.EvaluateMap
	mockEvaluator.SetTemplateDataFunc = realEvaluator.SetTemplateData
	mockEvaluator.RegisterFunc = realEvaluator.Register

	rt := &runtime.Runtime{
		ProjectRoot:   tmpDir,
		ConfigRoot:    tmpDir,
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
		Evaluator:     mockEvaluator,
	}

	mocks := &HandlerTestMocks{
		Shell:           mockShell,
		ConfigHandler:   mockConfigHandler,
		ArtifactBuilder: mockArtifact,
		Evaluator:       mockEvaluator,
		Runtime:         rt,
		Shims:           NewShims(),
	}

	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
	})

	return mocks
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewBlueprintHandler(t *testing.T) {
	t.Run("CreatesHandlerWithDefaults", func(t *testing.T) {
		// Given a runtime and artifact builder
		mocks := setupHandlerMocks(t)

		// When creating a new handler
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// Then handler should be created with defaults
		if handler == nil {
			t.Fatal("Expected handler to be created")
		}
		if handler.runtime != mocks.Runtime {
			t.Error("Expected runtime to be set")
		}
		if handler.artifactBuilder != mocks.ArtifactBuilder {
			t.Error("Expected artifact builder to be set")
		}
		if handler.processor == nil {
			t.Error("Expected processor to be initialized")
		}
		if handler.composer == nil {
			t.Error("Expected composer to be initialized")
		}
		if handler.writer == nil {
			t.Error("Expected writer to be initialized")
		}
		if handler.shims == nil {
			t.Error("Expected shims to be initialized")
		}
		if handler.sourceBlueprintLoaders == nil {
			t.Error("Expected sourceBlueprintLoaders map to be initialized")
		}
	})

	t.Run("AcceptsProcessorOverride", func(t *testing.T) {
		// Given a custom processor
		mocks := setupHandlerMocks(t)
		customProcessor := NewBlueprintProcessor(mocks.Runtime)

		// When creating handler with override
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder, &BaseBlueprintHandler{processor: customProcessor})

		// Then handler should use custom processor
		if handler.processor != customProcessor {
			t.Error("Expected custom processor to be used")
		}
	})

	t.Run("AcceptsComposerOverride", func(t *testing.T) {
		// Given a custom composer
		mocks := setupHandlerMocks(t)
		customComposer := NewBlueprintComposer(mocks.Runtime)

		// When creating handler with override
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder, &BaseBlueprintHandler{composer: customComposer})

		// Then handler should use custom composer
		if handler.composer != customComposer {
			t.Error("Expected custom composer to be used")
		}
	})

	t.Run("AcceptsWriterOverride", func(t *testing.T) {
		// Given a custom writer
		mocks := setupHandlerMocks(t)
		customWriter := NewBlueprintWriter(mocks.Runtime)

		// When creating handler with override
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder, &BaseBlueprintHandler{writer: customWriter})

		// Then handler should use custom writer
		if handler.writer != customWriter {
			t.Error("Expected custom writer to be used")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestHandler_LoadBlueprint(t *testing.T) {
	t.Run("LoadsFromLocalTemplate", func(t *testing.T) {
		// Given a handler with local template
		mocks := setupHandlerMocks(t)
		mocks.Runtime.TemplateRoot = filepath.Join(mocks.Runtime.ProjectRoot, "_template")

		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)
		blueprintYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test-blueprint
terraform:
  - path: vpc
`
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(blueprintYaml), 0644)

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then blueprint should be composed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if handler.composedBlueprint == nil {
			t.Fatal("Expected composed blueprint")
		}
	})

	t.Run("LoadsAndProcessesFacets", func(t *testing.T) {
		// Given a handler with facets
		mocks := setupHandlerMocks(t)
		mocks.Runtime.TemplateRoot = filepath.Join(mocks.Runtime.ProjectRoot, "_template")

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
  name: network
terraform:
  - path: vpc
`
		os.WriteFile(filepath.Join(facetsDir, "network.yaml"), []byte(facetYaml), 0644)

		userYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
sources:
  - name: template
    install: true
`
		os.WriteFile(filepath.Join(mocks.Runtime.ConfigRoot, "blueprint.yaml"), []byte(userYaml), 0644)

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then features should be processed into components
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if handler.composedBlueprint == nil {
			t.Fatal("Expected composed blueprint")
		}
		if len(handler.composedBlueprint.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component from feature, got %d", len(handler.composedBlueprint.TerraformComponents))
		}
	})

	t.Run("MergesUserBlueprintOverTemplate", func(t *testing.T) {
		// Given template and user blueprints
		mocks := setupHandlerMocks(t)
		mocks.Runtime.TemplateRoot = filepath.Join(mocks.Runtime.ProjectRoot, "_template")

		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)

		templateYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: template
terraform:
  - path: vpc
  - path: rds
`
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(templateYaml), 0644)

		userYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: user
sources:
  - name: template
    install: true
terraform:
  - path: vpc
`
		os.WriteFile(filepath.Join(mocks.Runtime.ConfigRoot, "blueprint.yaml"), []byte(userYaml), 0644)

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then all components should remain (user blueprint acts as override, not filter)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if handler.composedBlueprint == nil {
			t.Fatal("Expected composed blueprint")
		}
		if len(handler.composedBlueprint.TerraformComponents) != 2 {
			t.Errorf("Expected 2 components (no filtering), got %d", len(handler.composedBlueprint.TerraformComponents))
		}
		componentPaths := make(map[string]bool)
		for _, comp := range handler.composedBlueprint.TerraformComponents {
			componentPaths[comp.Path] = true
		}
		if !componentPaths["vpc"] {
			t.Error("Expected 'vpc' component to exist")
		}
		if !componentPaths["rds"] {
			t.Error("Expected 'rds' component to remain (no filtering)")
		}
	})

	t.Run("LoadsFromOCIWhenURLProvided", func(t *testing.T) {
		// Given a handler with OCI URL
		mocks := setupHandlerMocks(t)

		cacheDir := filepath.Join(mocks.Runtime.ProjectRoot, "cache")
		templateDir := filepath.Join(cacheDir, "_template")
		os.MkdirAll(templateDir, 0755)

		blueprintYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: oci-blueprint
terraform:
  - path: vpc
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

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading with OCI URL
		err := handler.LoadBlueprint("oci://example.com/blueprint:v1.0.0")

		// Then OCI blueprint should be loaded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if handler.composedBlueprint == nil {
			t.Fatal("Expected composed blueprint from OCI")
		}
	})

	t.Run("LoadsTemplateWhenInUserSources", func(t *testing.T) {
		// Given a handler with local template and user blueprint referencing it
		mocks := setupHandlerMocks(t)
		mocks.Runtime.TemplateRoot = filepath.Join(mocks.Runtime.ProjectRoot, "_template")

		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test
`), 0644)

		userYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: user
sources:
  - name: template
`
		os.WriteFile(filepath.Join(mocks.Runtime.ConfigRoot, "blueprint.yaml"), []byte(userYaml), 0644)

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then template should be loaded as a source
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := handler.sourceBlueprintLoaders["template"]; !exists {
			t.Error("Expected template source to be loaded")
		}
	})

	t.Run("ReturnsErrorWhenTemplateLoadFails", func(t *testing.T) {
		// Given a handler with invalid template and user blueprint referencing it
		mocks := setupHandlerMocks(t)
		mocks.Runtime.TemplateRoot = filepath.Join(mocks.Runtime.ProjectRoot, "_template")

		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte("invalid: [yaml"), 0644)

		userYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
sources:
  - name: template
`
		os.WriteFile(filepath.Join(mocks.Runtime.ConfigRoot, "blueprint.yaml"), []byte(userYaml), 0644)

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid blueprint")
		}
	})

	t.Run("LoadsSourcesFromUserBlueprint", func(t *testing.T) {
		// Given a user blueprint with sources
		mocks := setupHandlerMocks(t)

		sourceCacheDir := filepath.Join(mocks.Runtime.ProjectRoot, "source-cache")
		sourceTemplateDir := filepath.Join(sourceCacheDir, "_template")
		os.MkdirAll(sourceTemplateDir, 0755)

		sourceYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: shared-modules
terraform:
  - path: vpc
    inputs:
      region: us-east-1
`
		os.WriteFile(filepath.Join(sourceTemplateDir, "blueprint.yaml"), []byte(sourceYaml), 0644)

		userYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: user
sources:
  - name: shared-modules
    url: oci://example.com/shared:v1.0.0
`
		os.WriteFile(filepath.Join(mocks.Runtime.ConfigRoot, "blueprint.yaml"), []byte(userYaml), 0644)

		mocks.ArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			return map[string]string{
				"example.com/shared:v1.0.0": sourceCacheDir,
			}, nil
		}
		mocks.ArtifactBuilder.ParseOCIRefFunc = func(ref string) (string, string, string, error) {
			return "example.com", "shared", "v1.0.0", nil
		}

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then sources should be loaded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.sourceBlueprintLoaders) < 1 {
			t.Errorf("Expected at least 1 source loader, got %d", len(handler.sourceBlueprintLoaders))
		}
		if _, exists := handler.sourceBlueprintLoaders["shared-modules"]; !exists {
			t.Error("Expected 'shared-modules' source loader")
		}
	})

	t.Run("LoadsMultipleSourcesFromUserBlueprint", func(t *testing.T) {
		// Given a user blueprint with multiple sources
		mocks := setupHandlerMocks(t)

		userYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: user
sources:
  - name: primary-source
    url: oci://example.com/primary:v1.0.0
  - name: user-source
    url: oci://example.com/user-modules:v1.0.0
`
		os.WriteFile(filepath.Join(mocks.Runtime.ConfigRoot, "blueprint.yaml"), []byte(userYaml), 0644)

		primaryCacheDir := filepath.Join(mocks.Runtime.ProjectRoot, "primary-cache")
		primaryTemplateDir := filepath.Join(primaryCacheDir, "_template")
		os.MkdirAll(primaryTemplateDir, 0755)
		primaryYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: primary
`
		os.WriteFile(filepath.Join(primaryTemplateDir, "blueprint.yaml"), []byte(primaryYaml), 0644)

		sourceCacheDir := filepath.Join(mocks.Runtime.ProjectRoot, "user-source-cache")
		sourceTemplateDir := filepath.Join(sourceCacheDir, "_template")
		os.MkdirAll(sourceTemplateDir, 0755)

		sourceYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: user-source
`
		os.WriteFile(filepath.Join(sourceTemplateDir, "blueprint.yaml"), []byte(sourceYaml), 0644)

		mocks.ArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			result := make(map[string]string)
			for _, ref := range refs {
				if strings.Contains(ref, "primary") {
					result["example.com/primary:v1.0.0"] = primaryCacheDir
				} else if strings.Contains(ref, "user-modules") {
					result["example.com/user-modules:v1.0.0"] = sourceCacheDir
				}
			}
			return result, nil
		}
		mocks.ArtifactBuilder.ParseOCIRefFunc = func(ref string) (string, string, string, error) {
			if strings.Contains(ref, "primary") {
				return "example.com", "primary", "v1.0.0", nil
			}
			return "example.com", "user-modules", "v1.0.0", nil
		}

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then both sources should be loaded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.sourceBlueprintLoaders) < 2 {
			t.Errorf("Expected at least 2 source loaders, got %d", len(handler.sourceBlueprintLoaders))
		}
	})

	t.Run("ReturnsErrorWhenSourceLoadFails", func(t *testing.T) {
		// Given a user blueprint with invalid source
		mocks := setupHandlerMocks(t)

		userYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
sources:
  - name: bad-source
    url: oci://example.com/bad:v1.0.0
`
		os.WriteFile(filepath.Join(mocks.Runtime.ConfigRoot, "blueprint.yaml"), []byte(userYaml), 0644)

		mocks.ArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			return nil, os.ErrNotExist
		}

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then should return error
		if err == nil {
			t.Error("Expected error when source load fails")
		}
	})

	t.Run("SkipsNonOCISourcesForBlueprintLoading", func(t *testing.T) {
		// Given a user blueprint with both OCI and non-OCI sources
		mocks := setupHandlerMocks(t)

		userYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: user
sources:
  - name: oci-source
    url: oci://example.com/oci:v1.0.0
    install: true
  - name: git-source
    url: https://github.com/org/terraform-modules.git
    ref:
      branch: main
terraform:
  - path: vpc
    source: git-source
`
		os.WriteFile(filepath.Join(mocks.Runtime.ConfigRoot, "blueprint.yaml"), []byte(userYaml), 0644)

		sourceCacheDir := filepath.Join(mocks.Runtime.ProjectRoot, "source-cache")
		sourceTemplateDir := filepath.Join(sourceCacheDir, "_template")
		os.MkdirAll(sourceTemplateDir, 0755)

		sourceYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: oci-source
terraform:
  - path: cluster
`
		os.WriteFile(filepath.Join(sourceTemplateDir, "blueprint.yaml"), []byte(sourceYaml), 0644)

		mocks.ArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			if len(refs) != 1 || refs[0] != "oci://example.com/oci:v1.0.0" {
				return nil, fmt.Errorf("unexpected refs: %v", refs)
			}
			return map[string]string{"example.com/oci:v1.0.0": sourceCacheDir}, nil
		}
		mocks.ArtifactBuilder.ParseOCIRefFunc = func(ref string) (string, string, string, error) {
			return "example.com", "oci", "v1.0.0", nil
		}

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then should succeed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And only OCI source should be loaded as blueprint (recursive loading may add more if oci-source has sources)
		if len(handler.sourceBlueprintLoaders) < 1 {
			t.Errorf("Expected at least 1 source loader, got %d", len(handler.sourceBlueprintLoaders))
		}
		if _, exists := handler.sourceBlueprintLoaders["oci-source"]; !exists {
			t.Error("Expected oci-source to be loaded")
		}
		if _, exists := handler.sourceBlueprintLoaders["git-source"]; exists {
			t.Error("Expected git-source to NOT be loaded as blueprint")
		}

		// And composed blueprint should have components from OCI source and user
		if handler.composedBlueprint == nil {
			t.Fatal("Expected composed blueprint to exist")
		}
		components := handler.GetTerraformComponents()
		if len(components) != 2 {
			t.Errorf("Expected 2 components (cluster from oci-source + vpc from user), got %d", len(components))
		}
		componentPaths := make(map[string]bool)
		for _, comp := range components {
			componentPaths[comp.Path] = true
		}
		if !componentPaths["cluster"] {
			t.Error("Expected 'cluster' component from oci-source")
		}
		if !componentPaths["vpc"] {
			t.Error("Expected 'vpc' component from user blueprint")
		}
	})

	t.Run("LoadsOCISourceAndMergesComponents", func(t *testing.T) {
		// Given a user blueprint with an OCI source
		mocks := setupHandlerMocks(t)

		userYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: user
sources:
  - name: public-blueprint
    url: oci://example.com/public:v1.0.0
    install: true
terraform:
  - path: custom
`
		os.WriteFile(filepath.Join(mocks.Runtime.ConfigRoot, "blueprint.yaml"), []byte(userYaml), 0644)

		publicCacheDir := filepath.Join(mocks.Runtime.ProjectRoot, "public-cache")
		publicTemplateDir := filepath.Join(publicCacheDir, "_template")
		os.MkdirAll(publicTemplateDir, 0755)

		publicYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: public-blueprint
terraform:
  - path: base-component
`
		os.WriteFile(filepath.Join(publicTemplateDir, "blueprint.yaml"), []byte(publicYaml), 0644)

		mocks.ArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			return map[string]string{
				"example.com/public:v1.0.0": publicCacheDir,
			}, nil
		}
		mocks.ArtifactBuilder.ParseOCIRefFunc = func(ref string) (string, string, string, error) {
			return "example.com", "public", "v1.0.0", nil
		}

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then source should be loaded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := handler.sourceBlueprintLoaders["public-blueprint"]; !exists {
			t.Fatal("Expected public-blueprint source to be loaded")
		}

		// And the composed blueprint should have components from both source and user
		if handler.composedBlueprint == nil {
			t.Fatal("Expected composed blueprint to exist")
		}
		componentPaths := make(map[string]bool)
		for _, comp := range handler.composedBlueprint.TerraformComponents {
			componentPaths[comp.Path] = true
		}
		if len(handler.composedBlueprint.TerraformComponents) == 0 {
			t.Errorf("Expected components in composed blueprint, got none. Composed blueprint: %+v", handler.composedBlueprint)
		}
		if !componentPaths["base-component"] {
			t.Errorf("Expected 'base-component' from public-blueprint. Found components: %v", componentPaths)
		}
		if !componentPaths["custom"] {
			t.Errorf("Expected 'custom' from user blueprint. Found components: %v", componentPaths)
		}
	})
}

func TestHandler_Write(t *testing.T) {
	t.Run("WriteDelegatesToWriter", func(t *testing.T) {
		// Given a handler with composed blueprint
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
		}

		writeCalled := false
		mockWriter := &mockWriterImpl{
			writeFunc: func(bp *blueprintv1alpha1.Blueprint, overwrite bool, initBlueprintURLs ...string) error {
				writeCalled = true
				return nil
			},
		}
		handler.writer = mockWriter

		// When writing
		err := handler.Write()

		// Then writer should be called
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !writeCalled {
			t.Error("Expected writer.Write to be called")
		}
	})

	t.Run("WritePassesOverwriteFlag", func(t *testing.T) {
		// Given a handler
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{}

		var receivedOverwrite bool
		mockWriter := &mockWriterImpl{
			writeFunc: func(bp *blueprintv1alpha1.Blueprint, overwrite bool, initBlueprintURLs ...string) error {
				receivedOverwrite = overwrite
				return nil
			},
		}
		handler.writer = mockWriter

		// When writing with overwrite=true
		_ = handler.Write(true)

		// Then overwrite flag should be passed
		if !receivedOverwrite {
			t.Error("Expected overwrite=true to be passed to writer")
		}
	})
}

func TestHandler_GetTerraformComponents(t *testing.T) {
	t.Run("ReturnsNilWhenNoBlueprint", func(t *testing.T) {
		// Given a handler with no composed blueprint
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When getting components
		components := handler.GetTerraformComponents()

		// Then should return nil
		if components != nil {
			t.Error("Expected nil when no blueprint")
		}
	})

	t.Run("ReturnsComponentsFromComposedBlueprint", func(t *testing.T) {
		// Given a handler with composed blueprint
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "rds"},
			},
		}

		// When getting components
		components := handler.GetTerraformComponents()

		// Then should return components
		if len(components) != 2 {
			t.Errorf("Expected 2 components, got %d", len(components))
		}
	})
}

func TestHandler_resolveComponentFullPath(t *testing.T) {
	t.Run("UsesNameForNamedComponents", func(t *testing.T) {
		// Given a handler with a named component
		mocks := setupHandlerMocks(t)
		mocks.Runtime.WindsorScratchPath = "/scratch"
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		component := &blueprintv1alpha1.TerraformComponent{
			Name: "my-component",
			Path: "some/path",
		}

		// When resolving full path
		handler.resolveComponentFullPath(component)

		// Then should use Name in WindsorScratchPath
		expected := "/scratch/terraform/my-component"
		actual := filepath.ToSlash(component.FullPath)
		if actual != expected {
			t.Errorf("Expected '%s', got '%s'", expected, actual)
		}
	})

	t.Run("UsesSourcePathForSourceComponents", func(t *testing.T) {
		// Given a handler with a component having a source
		mocks := setupHandlerMocks(t)
		mocks.Runtime.WindsorScratchPath = "/scratch"
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		component := &blueprintv1alpha1.TerraformComponent{
			Source: "shared-modules",
			Path:   "vpc",
		}

		// When resolving full path
		handler.resolveComponentFullPath(component)

		// Then should use WindsorScratchPath with Path
		expected := "/scratch/terraform/vpc"
		actual := filepath.ToSlash(component.FullPath)
		if actual != expected {
			t.Errorf("Expected '%s', got '%s'", expected, actual)
		}
	})

	t.Run("UsesProjectRootForLocalComponents", func(t *testing.T) {
		// Given a handler with a local component (no name, no source)
		mocks := setupHandlerMocks(t)
		mocks.Runtime.ProjectRoot = "/project"
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		component := &blueprintv1alpha1.TerraformComponent{
			Path: "network/vpc",
		}

		// When resolving full path
		handler.resolveComponentFullPath(component)

		// Then should use ProjectRoot
		expected := "/project/terraform/network/vpc"
		actual := filepath.ToSlash(component.FullPath)
		if actual != expected {
			t.Errorf("Expected '%s', got '%s'", expected, actual)
		}
	})
}

func TestHandler_resolveComponentSource(t *testing.T) {
	t.Run("ResolvesOCISourceWithTag", func(t *testing.T) {
		// Given a handler with a composed blueprint containing an OCI source
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Url: "oci://ghcr.io/windsorcli/core", Ref: blueprintv1alpha1.Reference{Tag: "v1.0.0"}},
			},
		}
		component := &blueprintv1alpha1.TerraformComponent{Source: "core", Path: "cluster/talos"}

		// When resolving component source
		handler.resolveComponentSource(component)

		// Then source should be expanded to full OCI URL
		expected := "oci://ghcr.io/windsorcli/core:v1.0.0//terraform/cluster/talos"
		if component.Source != expected {
			t.Errorf("Expected '%s', got '%s'", expected, component.Source)
		}
	})

	t.Run("ResolvesGitSourceWithBranch", func(t *testing.T) {
		// Given a handler with a composed blueprint containing a Git source
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "infra", Url: "https://github.com/org/infra", Ref: blueprintv1alpha1.Reference{Branch: "develop"}},
			},
		}
		component := &blueprintv1alpha1.TerraformComponent{Source: "infra", Path: "vpc"}

		// When resolving component source
		handler.resolveComponentSource(component)

		// Then source should be expanded to Git URL format
		expected := "https://github.com/org/infra//terraform/vpc?ref=develop"
		if component.Source != expected {
			t.Errorf("Expected '%s', got '%s'", expected, component.Source)
		}
	})

	t.Run("UsesPathPrefixFromSource", func(t *testing.T) {
		// Given a source with a custom path prefix
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "modules", Url: "oci://ghcr.io/org/modules", PathPrefix: "modules/tf", Ref: blueprintv1alpha1.Reference{Tag: "latest"}},
			},
		}
		component := &blueprintv1alpha1.TerraformComponent{Source: "modules", Path: "network"}

		// When resolving component source
		handler.resolveComponentSource(component)

		// Then source should use the custom path prefix
		expected := "oci://ghcr.io/org/modules:latest//modules/tf/network"
		if component.Source != expected {
			t.Errorf("Expected '%s', got '%s'", expected, component.Source)
		}
	})

	t.Run("UsesEmptyRefWhenNoRefSpecified", func(t *testing.T) {
		// Given a source with no ref specified
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "lib", Url: "https://github.com/org/lib"},
			},
		}
		component := &blueprintv1alpha1.TerraformComponent{Source: "lib", Path: "utils"}

		// When resolving component source
		handler.resolveComponentSource(component)

		// Then source should use empty ref
		expected := "https://github.com/org/lib//terraform/utils?ref="
		if component.Source != expected {
			t.Errorf("Expected '%s', got '%s'", expected, component.Source)
		}
	})

	t.Run("LeavesUnmatchedSourceUnchanged", func(t *testing.T) {
		// Given a component referencing a non-existent source
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{},
		}
		component := &blueprintv1alpha1.TerraformComponent{Source: "missing", Path: "vpc"}

		// When resolving component source
		handler.resolveComponentSource(component)

		// Then source should remain unchanged
		if component.Source != "missing" {
			t.Errorf("Expected 'missing', got '%s'", component.Source)
		}
	})

	t.Run("SkipsEmptySource", func(t *testing.T) {
		// Given a component with no source
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{}
		component := &blueprintv1alpha1.TerraformComponent{Path: "local"}

		// When resolving component source
		handler.resolveComponentSource(component)

		// Then source should remain empty
		if component.Source != "" {
			t.Errorf("Expected empty source, got '%s'", component.Source)
		}
	})

	t.Run("SkipsWhenNoBlueprintComposed", func(t *testing.T) {
		// Given a handler with no composed blueprint
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		component := &blueprintv1alpha1.TerraformComponent{Source: "core", Path: "vpc"}

		// When resolving component source
		handler.resolveComponentSource(component)

		// Then source should remain unchanged
		if component.Source != "core" {
			t.Errorf("Expected 'core', got '%s'", component.Source)
		}
	})
}

func TestHandler_getSourceRef(t *testing.T) {
	t.Run("ReturnsCommitWhenPresent", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		source := blueprintv1alpha1.Source{Ref: blueprintv1alpha1.Reference{Commit: "abc123", SemVer: "1.0.0", Tag: "v1.0.0", Branch: "main"}}

		ref := handler.getSourceRef(source)

		if ref != "abc123" {
			t.Errorf("Expected 'abc123', got '%s'", ref)
		}
	})

	t.Run("ReturnsSemVerWhenCommitNotPresent", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		source := blueprintv1alpha1.Source{Ref: blueprintv1alpha1.Reference{SemVer: "1.0.0", Tag: "v1.0.0", Branch: "main"}}

		ref := handler.getSourceRef(source)

		if ref != "1.0.0" {
			t.Errorf("Expected '1.0.0', got '%s'", ref)
		}
	})

	t.Run("ReturnsTagWhenCommitAndSemVerNotPresent", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		source := blueprintv1alpha1.Source{Ref: blueprintv1alpha1.Reference{Tag: "v1.0.0", Branch: "main"}}

		ref := handler.getSourceRef(source)

		if ref != "v1.0.0" {
			t.Errorf("Expected 'v1.0.0', got '%s'", ref)
		}
	})

	t.Run("ReturnsBranchWhenOnlyBranchPresent", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		source := blueprintv1alpha1.Source{Ref: blueprintv1alpha1.Reference{Branch: "develop"}}

		ref := handler.getSourceRef(source)

		if ref != "develop" {
			t.Errorf("Expected 'develop', got '%s'", ref)
		}
	})

	t.Run("ReturnsEmptyStringWhenNoRefFieldsPresent", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		source := blueprintv1alpha1.Source{}

		ref := handler.getSourceRef(source)

		if ref != "" {
			t.Errorf("Expected empty string, got '%s'", ref)
		}
	})
}

func TestHandler_GetLocalTemplateData(t *testing.T) {
	t.Run("ReturnsNilWhenNoTemplateSource", func(t *testing.T) {
		// Given a handler with no template source
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When getting template data
		data, err := handler.GetLocalTemplateData()

		// Then should return nil
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if data != nil {
			t.Error("Expected nil when no template source")
		}
	})

	t.Run("ReturnsDataFromTemplateSource", func(t *testing.T) {
		// Given a handler with template source
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		mockLoader := &mockLoaderImpl{
			getTemplateDataFunc: func() map[string][]byte {
				return map[string][]byte{"test.yaml": []byte("content")}
			},
		}
		handler.sourceBlueprintLoaders["template"] = mockLoader

		// When getting template data
		data, err := handler.GetLocalTemplateData()

		// Then should return data
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(data) != 1 {
			t.Errorf("Expected 1 entry, got %d", len(data))
		}
	})
}

func TestHandler_Generate(t *testing.T) {
	t.Run("ReturnsNilWhenNoBlueprint", func(t *testing.T) {
		// Given a handler with no composed blueprint
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When generating
		result := handler.Generate()

		// Then should return nil
		if result != nil {
			t.Error("Expected nil when no blueprint")
		}
	})

	t.Run("ReturnsComposedBlueprint", func(t *testing.T) {
		// Given a handler with composed blueprint
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "generated"},
		}

		// When generating
		result := handler.Generate()

		// Then should return blueprint
		if result == nil {
			t.Fatal("Expected non-nil blueprint")
		}
		if result.Metadata.Name != "generated" {
			t.Errorf("Expected name='generated', got '%s'", result.Metadata.Name)
		}
	})
}

func TestHandler_getConfigValues(t *testing.T) {
	t.Run("ReturnsNilWhenConfigHandlerNil", func(t *testing.T) {
		// Given a handler with nil ConfigHandler
		mocks := setupHandlerMocks(t)
		mocks.Runtime.ConfigHandler = nil
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When getting config values
		values := handler.getConfigValues()

		// Then should return nil
		if values != nil {
			t.Error("Expected nil when ConfigHandler is nil")
		}
	})

	t.Run("ReturnsNilWhenGetContextValuesErrors", func(t *testing.T) {
		// Given a handler where GetContextValues returns error
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return nil, errors.New("config error")
		}
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When getting config values
		values := handler.getConfigValues()

		// Then should return nil
		if values != nil {
			t.Error("Expected nil when GetContextValues errors")
		}
	})

	t.Run("ReturnsValuesFromConfigHandler", func(t *testing.T) {
		// Given a handler with working ConfigHandler
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"key": "value"}, nil
		}
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When getting config values
		values := handler.getConfigValues()

		// Then should return values
		if values == nil {
			t.Fatal("Expected non-nil values")
		}
		if values["key"] != "value" {
			t.Errorf("Expected key='value', got '%v'", values["key"])
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

type mockWriterImpl struct {
	writeFunc func(bp *blueprintv1alpha1.Blueprint, overwrite bool, initBlueprintURLs ...string) error
}

func (m *mockWriterImpl) Write(bp *blueprintv1alpha1.Blueprint, overwrite bool, initBlueprintURLs ...string) error {
	if m.writeFunc != nil {
		return m.writeFunc(bp, overwrite, initBlueprintURLs...)
	}
	return nil
}

type mockLoaderImpl struct {
	loadFunc            func(sourceName, sourceURL string) error
	getBlueprintFunc    func() *blueprintv1alpha1.Blueprint
	getFacetsFunc       func() []blueprintv1alpha1.Facet
	getTemplateDataFunc func() map[string][]byte
	getSourceNameFunc   func() string
}

func (m *mockLoaderImpl) Load(sourceName, sourceURL string) error {
	if m.loadFunc != nil {
		return m.loadFunc(sourceName, sourceURL)
	}
	return nil
}

func (m *mockLoaderImpl) GetBlueprint() *blueprintv1alpha1.Blueprint {
	if m.getBlueprintFunc != nil {
		return m.getBlueprintFunc()
	}
	return nil
}

func (m *mockLoaderImpl) GetFacets() []blueprintv1alpha1.Facet {
	if m.getFacetsFunc != nil {
		return m.getFacetsFunc()
	}
	return nil
}

func (m *mockLoaderImpl) GetTemplateData() map[string][]byte {
	if m.getTemplateDataFunc != nil {
		return m.getTemplateDataFunc()
	}
	return nil
}

func (m *mockLoaderImpl) GetSourceName() string {
	if m.getSourceNameFunc != nil {
		return m.getSourceNameFunc()
	}
	return ""
}

func TestHandler_setRepositoryDefaults(t *testing.T) {
	t.Run("SetsDevRepositoryURLInDevMode", func(t *testing.T) {
		// Given a handler in dev mode with domain and project configured, no user blueprint
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return key == "dev"
		}
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "test"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mocks.Runtime.ProjectRoot = "/path/to/myproject"

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.shims = &Shims{
			FilepathBase: func(s string) string { return "myproject" },
			TrimSpace:    func(s string) string { return s },
			HasPrefix:    func(s, prefix string) bool { return false },
			Contains:     func(s, substr string) bool { return false },
			Replace:      func(s, old, new string, n int) string { return s },
		}
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{}
		handler.userBlueprintLoader = nil

		// When setting repository defaults
		handler.setRepositoryDefaults()

		// Then repository should be set with local git URL
		if handler.composedBlueprint.Repository.Url != "http://git.test/git/myproject" {
			t.Errorf("Expected URL 'http://git.test/git/myproject', got '%s'", handler.composedBlueprint.Repository.Url)
		}
		if handler.composedBlueprint.Repository.Ref.Branch != "main" {
			t.Errorf("Expected branch 'main', got '%s'", handler.composedBlueprint.Repository.Ref.Branch)
		}
		if handler.composedBlueprint.Repository.SecretName == nil || *handler.composedBlueprint.Repository.SecretName != "flux-system" {
			t.Errorf("Expected secretName 'flux-system'")
		}
	})

	t.Run("FallsBackToGitRemoteWhenNotDevMode", func(t *testing.T) {
		// Given a handler not in dev mode with no user blueprint
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "remote.origin.url" {
				return "https://github.com/test/repo.git", nil
			}
			return "", nil
		}

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.shims = &Shims{
			TrimSpace: func(s string) string { return s },
			HasPrefix: func(s, prefix string) bool { return false },
			Contains:  func(s, substr string) bool { return false },
			Replace:   func(s, old, new string, n int) string { return s },
		}
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{}
		handler.userBlueprintLoader = nil

		// When setting repository defaults
		handler.setRepositoryDefaults()

		// Then repository should be set with git remote URL
		if handler.composedBlueprint.Repository.Url != "https://github.com/test/repo.git" {
			t.Errorf("Expected URL 'https://github.com/test/repo.git', got '%s'", handler.composedBlueprint.Repository.Url)
		}
	})

	t.Run("NormalizesSSHGitURL", func(t *testing.T) {
		// Given a handler with SSH-style git URL and no user blueprint
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "remote.origin.url" {
				return "git@github.com:test/repo.git", nil
			}
			return "", nil
		}

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.shims = NewShims()
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{}
		handler.userBlueprintLoader = nil

		// When setting repository defaults
		handler.setRepositoryDefaults()

		// Then repository should be normalized to SSH URL
		if handler.composedBlueprint.Repository.Url != "ssh://git@github.com/test/repo.git" {
			t.Errorf("Expected URL 'ssh://git@github.com/test/repo.git', got '%s'", handler.composedBlueprint.Repository.Url)
		}
	})

	t.Run("SetsDefaultsWhenRepositoryEmptyEvenWithUserBlueprint", func(t *testing.T) {
		// Given a handler with user blueprint but composed Repository.Url empty (e.g. user file has no repository)
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return key == "dev"
		}
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "test"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mocks.Runtime.ProjectRoot = "/path/to/myproject"

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.shims = &Shims{
			FilepathBase: func(s string) string { return "myproject" },
		}
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{}
		handler.userBlueprintLoader = &mockLoaderImpl{
			getBlueprintFunc: func() *blueprintv1alpha1.Blueprint { return &blueprintv1alpha1.Blueprint{} },
		}

		// When setting repository defaults
		handler.setRepositoryDefaults()

		// Then repository should still be set from dev defaults so rendered blueprint has repository section
		if handler.composedBlueprint.Repository.Url != "http://git.test/git/myproject" {
			t.Errorf("Expected URL 'http://git.test/git/myproject' when Repository empty, got '%s'", handler.composedBlueprint.Repository.Url)
		}
	})

	t.Run("HandlesNilBlueprint", func(t *testing.T) {
		// Given a handler with nil composed blueprint
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = nil

		// When setting repository defaults
		handler.setRepositoryDefaults()

		// Then no panic should occur
	})
}

func TestHandler_clearLocalTemplateSource(t *testing.T) {
	t.Run("ClearsTemplateSourceOnComponentsAndKustomizationsWhenTemplateIsLocal", func(t *testing.T) {
		// Given a blueprint with local template source and components/kustomizations referencing template
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{{Name: "template", Url: ""}},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Source: "template", Path: "vpc"},
				{Source: "other", Path: "rds"},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "k1", Source: "template", Path: "app"},
				{Name: "k2", Source: "other", Path: "base"},
			},
		}

		// When clearing local template source
		handler.clearLocalTemplateSource(blueprint)

		// Then template-referencing components and kustomizations have Source cleared
		if blueprint.TerraformComponents[0].Source != "" {
			t.Errorf("Expected TerraformComponent Source cleared, got '%s'", blueprint.TerraformComponents[0].Source)
		}
		if blueprint.TerraformComponents[1].Source != "other" {
			t.Errorf("Expected non-template Source unchanged, got '%s'", blueprint.TerraformComponents[1].Source)
		}
		if blueprint.Kustomizations[0].Source != "" {
			t.Errorf("Expected Kustomization Source cleared, got '%s'", blueprint.Kustomizations[0].Source)
		}
		if blueprint.Kustomizations[1].Source != "other" {
			t.Errorf("Expected non-template Kustomization Source unchanged, got '%s'", blueprint.Kustomizations[1].Source)
		}
	})

	t.Run("NoOpWhenTemplateIsRemote", func(t *testing.T) {
		// Given a blueprint with remote template source
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources:             []blueprintv1alpha1.Source{{Name: "template", Url: "https://github.com/example/repo.git"}},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Source: "template", Path: "vpc"}},
			Kustomizations:      []blueprintv1alpha1.Kustomization{{Name: "k1", Source: "template", Path: "app"}},
		}

		// When clearing local template source
		handler.clearLocalTemplateSource(blueprint)

		if blueprint.TerraformComponents[0].Source != "template" {
			t.Errorf("Expected Source unchanged when template is remote, got '%s'", blueprint.TerraformComponents[0].Source)
		}
		// Then Source remains unchanged
		if blueprint.Kustomizations[0].Source != "template" {
			t.Errorf("Expected Kustomization Source unchanged when template is remote, got '%s'", blueprint.Kustomizations[0].Source)
		}
	})

	t.Run("ClearsUserSourceSoComponentsResolveToProjectPath", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources:             []blueprintv1alpha1.Source{{Name: "template", Url: "https://example.com/oci"}},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Source: "user", Path: "cluster"}},
			Kustomizations:      []blueprintv1alpha1.Kustomization{{Name: "app", Source: "user", Path: "app"}},
		}

		handler.clearLocalTemplateSource(blueprint)

		if blueprint.TerraformComponents[0].Source != "" {
			t.Errorf("Expected user Source cleared so FullPath uses project path, got '%s'", blueprint.TerraformComponents[0].Source)
		}
		if blueprint.Kustomizations[0].Source != "" {
			t.Errorf("Expected user Kustomization Source cleared, got '%s'", blueprint.Kustomizations[0].Source)
		}
	})

	t.Run("NoOpWhenBlueprintNil", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.clearLocalTemplateSource(nil)
	})
}

func TestHandler_processAndCompose(t *testing.T) {
	t.Run("CallsSetTerraformComponentsWhenProviderExists", func(t *testing.T) {
		// Given a handler with terraform provider and composed blueprint with components
		mocks := setupHandlerMocks(t)
		mockTerraformProvider := &terraform.MockTerraformProvider{}
		var setComponentsCalled bool
		var receivedComponents []blueprintv1alpha1.TerraformComponent
		mockTerraformProvider.SetTerraformComponentsFunc = func(components []blueprintv1alpha1.TerraformComponent) {
			setComponentsCalled = true
			receivedComponents = components
		}
		mocks.Runtime.TerraformProvider = mockTerraformProvider
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		trueVal := true
		templateBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "rds"},
			},
		}
		handler.sourceBlueprintLoaders["template"] = &mockLoaderImpl{
			getBlueprintFunc:  func() *blueprintv1alpha1.Blueprint { return templateBp },
			getSourceNameFunc: func() string { return "template" },
		}
		handler.userBlueprintLoader = &mockLoaderImpl{
			getBlueprintFunc: func() *blueprintv1alpha1.Blueprint {
				return &blueprintv1alpha1.Blueprint{
					Sources: []blueprintv1alpha1.Source{
						{Name: "template", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
					},
				}
			},
			getSourceNameFunc: func() string { return "user" },
		}

		// When processing and composing
		err := handler.processAndCompose()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		// And SetTerraformComponents should be called
		if !setComponentsCalled {
			t.Error("Expected SetTerraformComponents to be called")
		}
		// And components should be passed correctly
		if len(receivedComponents) != 2 {
			t.Errorf("Expected 2 components to be set, got %d", len(receivedComponents))
		}
	})

	t.Run("SkipsSetTerraformComponentsWhenProviderIsNil", func(t *testing.T) {
		// Given a handler with no terraform provider
		mocks := setupHandlerMocks(t)
		mocks.Runtime.TerraformProvider = nil
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		templateBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
			},
		}
		trueVal := true
		handler.sourceBlueprintLoaders["template"] = &mockLoaderImpl{
			getBlueprintFunc:  func() *blueprintv1alpha1.Blueprint { return templateBp },
			getSourceNameFunc: func() string { return "template" },
		}
		handler.userBlueprintLoader = &mockLoaderImpl{
			getBlueprintFunc: func() *blueprintv1alpha1.Blueprint {
				return &blueprintv1alpha1.Blueprint{
					Sources: []blueprintv1alpha1.Source{
						{Name: "template", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
					},
				}
			},
			getSourceNameFunc: func() string { return "user" },
		}

		// When processing and composing
		err := handler.processAndCompose()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		// And composed blueprint should be set
		if handler.composedBlueprint == nil {
			t.Error("Expected composed blueprint to be set")
		}
	})

	t.Run("ResolvesConfigBlockRefInTerraformInputsWhenTemplateHasFacetWithConfig", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		templateBp := &blueprintv1alpha1.Blueprint{}
		handler.sourceBlueprintLoaders["template"] = &mockLoaderImpl{
			getBlueprintFunc:  func() *blueprintv1alpha1.Blueprint { return templateBp },
			getSourceNameFunc: func() string { return "template" },
			getFacetsFunc: func() []blueprintv1alpha1.Facet {
				return []blueprintv1alpha1.Facet{
					{
						Metadata: blueprintv1alpha1.Metadata{Name: "facet-with-config"},
						Config: []blueprintv1alpha1.ConfigBlock{
							{
								Name: "talos_common_docker",
								Body: map[string]any{
									"patchVars":    map[string]any{"key": "val"},
									"common_patch": "${string(talos_common_docker.patchVars)}",
									"patches":      "${string(talos_common_docker.common_patch)}",
								},
							},
						},
						TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
							{
								TerraformComponent: blueprintv1alpha1.TerraformComponent{
									Path:   "cluster/talos",
									Inputs: map[string]any{"common_config_patches": "${talos_common_docker.patches}"},
								},
							},
						},
					},
				}
			},
		}
		handler.userBlueprintLoader = &mockLoaderImpl{
			getBlueprintFunc:  func() *blueprintv1alpha1.Blueprint { return &blueprintv1alpha1.Blueprint{} },
			getSourceNameFunc: func() string { return "user" },
		}

		err := handler.processAndCompose()
		if err != nil {
			t.Fatalf("processAndCompose failed: %v", err)
		}
		if handler.composedBlueprint == nil {
			t.Fatal("Expected composed blueprint to be set")
		}
		if len(handler.composedBlueprint.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(handler.composedBlueprint.TerraformComponents))
		}
		commonConfigPatches := handler.composedBlueprint.TerraformComponents[0].Inputs["common_config_patches"]
		if commonConfigPatches == nil {
			t.Error("Expected common_config_patches to be resolved, got nil")
		}
		if s, ok := commonConfigPatches.(string); ok && strings.Contains(s, "${") {
			t.Errorf("Expected common_config_patches to be resolved (no expression), got %q", s)
		}
	})

	t.Run("CallsSetConfigScopeWhenProviderExists", func(t *testing.T) {
		// Given a handler with terraform provider and loaders
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		var setConfigScopeCalled bool
		mockTerraformProvider := &terraform.MockTerraformProvider{}
		mockTerraformProvider.SetTerraformComponentsFunc = func([]blueprintv1alpha1.TerraformComponent) {}
		mockTerraformProvider.SetConfigScopeFunc = func(map[string]any) {
			setConfigScopeCalled = true
		}
		mocks.Runtime.TerraformProvider = mockTerraformProvider
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		templateBp := &blueprintv1alpha1.Blueprint{}
		handler.sourceBlueprintLoaders["template"] = &mockLoaderImpl{
			getBlueprintFunc:  func() *blueprintv1alpha1.Blueprint { return templateBp },
			getSourceNameFunc: func() string { return "template" },
		}
		trueVal := true
		handler.userBlueprintLoader = &mockLoaderImpl{
			getBlueprintFunc: func() *blueprintv1alpha1.Blueprint {
				return &blueprintv1alpha1.Blueprint{
					Sources: []blueprintv1alpha1.Source{
						{Name: "template", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
					},
				}
			},
			getSourceNameFunc: func() string { return "user" },
		}

		// When processing and composing
		err := handler.processAndCompose()

		// Then SetConfigScope should be called when provider exists
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !setConfigScopeCalled {
			t.Error("Expected SetConfigScope to be called when provider exists")
		}
	})
}
