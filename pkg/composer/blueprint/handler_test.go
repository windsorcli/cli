package blueprint_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
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
	Shims           *blueprint.Shims
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
		Shims:         runtime.NewShims(),
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
		Shims:           blueprint.NewShims(),
	}

	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
	})

	return mocks
}

// setTestCliVersion temporarily overrides constants.Version for the duration of the calling test,
// restoring the original value via t.Cleanup. constants.Version is process-global, not per-test
// state — any test using this helper, and any sibling subtest in this package that reads
// constants.Version, MUST NOT run under t.Parallel(); a parallel subtest would race this mutation
// against concurrent reads (or another parallel mutation). This package has no t.Parallel() calls
// today; if one is ever added, audit for this helper's callers first.
func setTestCliVersion(t *testing.T, v string) {
	t.Helper()
	original := constants.Version
	constants.Version = v
	t.Cleanup(func() { constants.Version = original })
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewBlueprintHandler(t *testing.T) {
	t.Run("CreatesHandlerWithDefaults", func(t *testing.T) {
		// Given a runtime and artifact builder
		mocks := setupHandlerMocks(t)

		// When creating a new handler
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// Then handler should be created with defaults
		if handler == nil {
			t.Fatal("Expected handler to be created")
		}
		if handler.Shims() == nil {
			t.Error("Expected shims to be initialized")
		}
		if handler.SourceLoaders() == nil {
			t.Error("Expected sourceBlueprintLoaders map to be initialized")
		}
	})

	t.Run("AcceptsProcessorOverride", func(t *testing.T) {
		// Given a custom processor that fails
		mocks := setupHandlerMocks(t)
		customProcessor := &mockProcessorImpl{err: fmt.Errorf("custom processor called")}

		// When loading through a handler created with the override
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder, blueprint.HandlerOverrides(customProcessor, nil, nil))
		handler.SetAllowUninitialized(true)
		err := handler.LoadBlueprint()

		// Then the custom processor is used
		if err == nil || !strings.Contains(err.Error(), "custom processor called") {
			t.Errorf("Expected the custom processor error, got %v", err)
		}
	})

	t.Run("AcceptsComposerOverride", func(t *testing.T) {
		// Given a custom composer that fails
		mocks := setupHandlerMocks(t)
		customComposer := &mockComposerImpl{err: fmt.Errorf("custom composer called")}

		// When loading through a handler created with the override
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder, blueprint.HandlerOverrides(nil, customComposer, nil))
		handler.SetAllowUninitialized(true)
		err := handler.LoadBlueprint()

		// Then the custom composer is used
		if err == nil || !strings.Contains(err.Error(), "custom composer called") {
			t.Errorf("Expected the custom composer error, got %v", err)
		}
	})

	t.Run("AcceptsWriterOverride", func(t *testing.T) {
		// Given a custom writer
		mocks := setupHandlerMocks(t)
		writeCalled := false
		customWriter := &mockWriterImpl{
			writeFunc: func(bp *blueprintv1alpha1.Blueprint, overwrite bool, initBlueprintURLs ...string) error {
				writeCalled = true
				return nil
			},
		}

		// When writing through a handler created with the override
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder, blueprint.HandlerOverrides(nil, nil, customWriter))
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{})
		err := handler.Write()

		// Then the custom writer is used
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !writeCalled {
			t.Error("Expected the custom writer to be called")
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

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then blueprint should be composed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if handler.ComposedBlueprint() == nil {
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

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then features should be processed into components
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if handler.ComposedBlueprint() == nil {
			t.Fatal("Expected composed blueprint")
		}
		if len(handler.ComposedBlueprint().TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component from feature, got %d", len(handler.ComposedBlueprint().TerraformComponents))
		}
	})

	t.Run("LoadsLocalFacetsWhenUserBlueprintListsSources", func(t *testing.T) {
		// Given a user blueprint that lists a source (not "template") alongside a local _template
		// carrying its own facet, which is the downstream-blueprint layout
		mocks := setupHandlerMocks(t)
		mocks.Runtime.TemplateRoot = filepath.Join(mocks.Runtime.ProjectRoot, "_template")

		templateDir := mocks.Runtime.TemplateRoot
		facetsDir := filepath.Join(templateDir, "facets")
		os.MkdirAll(facetsDir, 0755)
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: template
`), 0644)
		os.WriteFile(filepath.Join(facetsDir, "local-network.yaml"), []byte(`kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: local-network
terraform:
  - path: local-net
`), 0644)

		// And an OCI source that contributes nothing conflicting.
		sourceCacheDir := filepath.Join(mocks.Runtime.ProjectRoot, "core-cache")
		sourceTemplateDir := filepath.Join(sourceCacheDir, "_template")
		os.MkdirAll(sourceTemplateDir, 0755)
		os.WriteFile(filepath.Join(sourceTemplateDir, "blueprint.yaml"), []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: core
`), 0644)
		mocks.ArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			return map[string]string{"example.com/core:v1.0.0": sourceCacheDir}, nil
		}
		mocks.ArtifactBuilder.ParseOCIRefFunc = func(ref string) (string, string, string, error) {
			return "example.com", "core", "v1.0.0", nil
		}

		os.WriteFile(filepath.Join(mocks.Runtime.ConfigRoot, "blueprint.yaml"), []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
sources:
  - name: core
    url: oci://example.com/core:v1.0.0
    install: true
`), 0644)

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading the blueprint
		if err := handler.LoadBlueprint(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the local _template facet is loaded despite the sources list, and its component composes.
		if _, exists := handler.SourceLoaders()["template"]; !exists {
			t.Fatal("Expected local 'template' loaded even though the user blueprint lists sources")
		}
		found := false
		for _, comp := range handler.ComposedBlueprint().TerraformComponents {
			if comp.Path == "local-net" {
				found = true
			}
		}
		if !found {
			t.Errorf("Expected local facet's 'local-net' component composed, got %v", handler.ComposedBlueprint().TerraformComponents)
		}
	})

	t.Run("ReloadPullsSourceContentChangedSinceThePreviousLoad", func(t *testing.T) {
		// Given a source pinned to one tag, loaded once
		mocks := setupHandlerMocks(t)

		oldDir := filepath.Join(mocks.Runtime.ProjectRoot, "core-old")
		os.MkdirAll(oldDir, 0755)
		os.WriteFile(filepath.Join(oldDir, "blueprint.yaml"), []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: core
terraform:
  - path: old-path
`), 0644)

		newDir := filepath.Join(mocks.Runtime.ProjectRoot, "core-new")
		os.MkdirAll(newDir, 0755)
		os.WriteFile(filepath.Join(newDir, "blueprint.yaml"), []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: core
terraform:
  - path: new-path
`), 0644)

		currentTag := "v1.0.0"
		mocks.ArtifactBuilder.ParseOCIRefFunc = func(ref string) (string, string, string, error) {
			return "example.com", "core", currentTag, nil
		}
		mocks.ArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			dir := oldDir
			if currentTag == "v2.0.0" {
				dir = newDir
			}
			return map[string]string{"example.com/core:" + currentTag: dir}, nil
		}

		os.WriteFile(filepath.Join(mocks.Runtime.ConfigRoot, "blueprint.yaml"), []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test
sources:
  - name: core
    url: oci://example.com/core:v1.0.0
`), 0644)

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		if err := handler.LoadBlueprint(); err != nil {
			t.Fatalf("Expected no error on first load, got %v", err)
		}
		hasPath := func(path string) bool {
			for _, comp := range handler.ComposedBlueprint().TerraformComponents {
				if comp.Path == path {
					return true
				}
			}
			return false
		}
		if !hasPath("old-path") {
			t.Fatalf("Expected first load to include old-path, got %v", handler.ComposedBlueprint().TerraformComponents)
		}

		// When the source is retargeted to a tag with different content, persisted, and reloaded
		// on the same handler — mirroring windsor upgrade's RetargetSource -> Write -> LoadBlueprint
		if _, err := handler.RetargetSource("core", "oci://example.com/core:v2.0.0"); err != nil {
			t.Fatalf("RetargetSource: %v", err)
		}
		if err := handler.Write(true); err != nil {
			t.Fatalf("Write: %v", err)
		}
		currentTag = "v2.0.0"
		if err := handler.LoadBlueprint(); err != nil {
			t.Fatalf("Expected no error on reload, got %v", err)
		}

		// Then the reload pulls the new source content instead of reusing the loader cached from
		// the first LoadBlueprint call
		if hasPath("old-path") {
			t.Errorf("Expected reload to drop stale content from the old source, got %v", handler.ComposedBlueprint().TerraformComponents)
		}
		if !hasPath("new-path") {
			t.Errorf("Expected reload to pull the new source content, got %v", handler.ComposedBlueprint().TerraformComponents)
		}
	})

	t.Run("DanglingDependencyOnExcludedFacetNamesTheFacetAndCondition", func(t *testing.T) {
		// Given a local template where one facet (excluded by a false when:) would contribute the
		// "cluster" component, and an always-on facet's "cni" depends on it.
		mocks := setupHandlerMocks(t)
		mocks.Runtime.TemplateRoot = filepath.Join(mocks.Runtime.ProjectRoot, "_template")
		facetsDir := filepath.Join(mocks.Runtime.TemplateRoot, "facets")
		os.MkdirAll(facetsDir, 0755)
		os.WriteFile(filepath.Join(mocks.Runtime.TemplateRoot, "blueprint.yaml"), []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: template
`), 0644)
		os.WriteFile(filepath.Join(facetsDir, "provider-hetzner.yaml"), []byte(`kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: provider-hetzner
when: cluster.driver == 'talos'
terraform:
  - path: cluster
`), 0644)
		os.WriteFile(filepath.Join(facetsDir, "cni.yaml"), []byte(`kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: cni
terraform:
  - path: cni
    dependsOn:
      - cluster
`), 0644)

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading, composition fails because "cluster" was excluded.
		err := handler.LoadBlueprint()

		// Then the error names the excluded facet and the condition that excluded it, not just the edge.
		if err == nil {
			t.Fatal("Expected a dangling-dependency error, got nil")
		}
		for _, want := range []string{`facet "provider-hetzner"`, "cluster.driver == 'talos'", "evaluated false"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Expected %q in error, got %v", want, err)
			}
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

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then all components should remain (user blueprint acts as override, not filter)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if handler.ComposedBlueprint() == nil {
			t.Fatal("Expected composed blueprint")
		}
		if len(handler.ComposedBlueprint().TerraformComponents) != 2 {
			t.Errorf("Expected 2 components (no filtering), got %d", len(handler.ComposedBlueprint().TerraformComponents))
		}
		componentPaths := make(map[string]bool)
		for _, comp := range handler.ComposedBlueprint().TerraformComponents {
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

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading with OCI URL
		err := handler.LoadBlueprint("oci://example.com/blueprint:v1.0.0")

		// Then OCI blueprint should be loaded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if handler.ComposedBlueprint() == nil {
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

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then template should be loaded as a source
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := handler.SourceLoaders()["template"]; !exists {
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

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

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

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then sources should be loaded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.SourceLoaders()) < 1 {
			t.Errorf("Expected at least 1 source loader, got %d", len(handler.SourceLoaders()))
		}
		if _, exists := handler.SourceLoaders()["shared-modules"]; !exists {
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

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then both sources should be loaded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.SourceLoaders()) < 2 {
			t.Errorf("Expected at least 2 source loaders, got %d", len(handler.SourceLoaders()))
		}
	})

	t.Run("DuplicateSourceNamesLoadOnlyOnce", func(t *testing.T) {
		// Given a user blueprint listing the same source name twice
		mocks := setupHandlerMocks(t)

		userYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: user
sources:
  - name: dup-source
    url: oci://example.com/dup:v1.0.0
  - name: dup-source
    url: oci://example.com/dup:v1.0.0
`
		os.WriteFile(filepath.Join(mocks.Runtime.ConfigRoot, "blueprint.yaml"), []byte(userYaml), 0644)

		sourceCacheDir := filepath.Join(mocks.Runtime.ProjectRoot, "dup-source-cache")
		sourceTemplateDir := filepath.Join(sourceCacheDir, "_template")
		os.MkdirAll(sourceTemplateDir, 0755)
		sourceYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: dup-source
`
		os.WriteFile(filepath.Join(sourceTemplateDir, "blueprint.yaml"), []byte(sourceYaml), 0644)

		var pullMu sync.Mutex
		pullCalls := 0
		mocks.ArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			pullMu.Lock()
			pullCalls++
			pullMu.Unlock()
			return map[string]string{"example.com/dup:v1.0.0": sourceCacheDir}, nil
		}
		mocks.ArtifactBuilder.ParseOCIRefFunc = func(ref string) (string, string, string, error) {
			return "example.com", "dup", "v1.0.0", nil
		}

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then the duplicate entry is only loaded once, not raced by two concurrent goroutines
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if pullCalls != 1 {
			t.Errorf("Expected exactly 1 Pull call for the duplicate source name, got %d", pullCalls)
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

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

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

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then should succeed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And only OCI source should be loaded as blueprint (recursive loading may add more if oci-source has sources)
		if len(handler.SourceLoaders()) < 1 {
			t.Errorf("Expected at least 1 source loader, got %d", len(handler.SourceLoaders()))
		}
		if _, exists := handler.SourceLoaders()["oci-source"]; !exists {
			t.Error("Expected oci-source to be loaded")
		}
		if _, exists := handler.SourceLoaders()["git-source"]; exists {
			t.Error("Expected git-source to NOT be loaded as blueprint")
		}

		// And composed blueprint should have components from OCI source and user
		if handler.ComposedBlueprint() == nil {
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

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then source should be loaded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := handler.SourceLoaders()["public-blueprint"]; !exists {
			t.Fatal("Expected public-blueprint source to be loaded")
		}

		// And the composed blueprint should have components from both source and user
		if handler.ComposedBlueprint() == nil {
			t.Fatal("Expected composed blueprint to exist")
		}
		componentPaths := make(map[string]bool)
		for _, comp := range handler.ComposedBlueprint().TerraformComponents {
			componentPaths[comp.Path] = true
		}
		if len(handler.ComposedBlueprint().TerraformComponents) == 0 {
			t.Errorf("Expected components in composed blueprint, got none. Composed blueprint: %+v", handler.ComposedBlueprint())
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
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
		})

		writeCalled := false
		mockWriter := &mockWriterImpl{
			writeFunc: func(bp *blueprintv1alpha1.Blueprint, overwrite bool, initBlueprintURLs ...string) error {
				writeCalled = true
				return nil
			},
		}
		handler.SetWriter(mockWriter)

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
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{})

		var receivedOverwrite bool
		mockWriter := &mockWriterImpl{
			writeFunc: func(bp *blueprintv1alpha1.Blueprint, overwrite bool, initBlueprintURLs ...string) error {
				receivedOverwrite = overwrite
				return nil
			},
		}
		handler.SetWriter(mockWriter)

		// When writing with overwrite=true
		_ = handler.Write(true)

		// Then overwrite flag should be passed
		if !receivedOverwrite {
			t.Error("Expected overwrite=true to be passed to writer")
		}
	})

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

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder, blueprint.HandlerOverrides(nil, nil, &mockWriterImpl{}))
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{})

		// When writing the blueprint
		if err := handler.Write(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then repository should be set with local git URL
		if handler.ComposedBlueprint().Repository.Url != "http://git.test/git/myproject" {
			t.Errorf("Expected URL 'http://git.test/git/myproject', got '%s'", handler.ComposedBlueprint().Repository.Url)
		}
		if handler.ComposedBlueprint().Repository.Ref.Branch != "main" {
			t.Errorf("Expected branch 'main', got '%s'", handler.ComposedBlueprint().Repository.Ref.Branch)
		}
		if handler.ComposedBlueprint().Repository.SecretName == nil || *handler.ComposedBlueprint().Repository.SecretName != "flux-system" {
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

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder, blueprint.HandlerOverrides(nil, nil, &mockWriterImpl{}))
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{})

		// When writing the blueprint
		if err := handler.Write(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then repository should be set with normalized host/path URL
		if handler.ComposedBlueprint().Repository.Url != "github.com/test/repo" {
			t.Errorf("Expected URL 'github.com/test/repo', got '%s'", handler.ComposedBlueprint().Repository.Url)
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

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder, blueprint.HandlerOverrides(nil, nil, &mockWriterImpl{}))
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{})

		// When writing the blueprint
		if err := handler.Write(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then repository should be normalized to host/path URL
		if handler.ComposedBlueprint().Repository.Url != "github.com/test/repo" {
			t.Errorf("Expected URL 'github.com/test/repo', got '%s'", handler.ComposedBlueprint().Repository.Url)
		}
	})

	t.Run("NormalizesGitSchemeURL", func(t *testing.T) {
		// Given a git remote origin URL and no dev mode
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "remote.origin.url" {
				return "git://github.com/test/repo.git", nil
			}
			return "", nil
		}

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder, blueprint.HandlerOverrides(nil, nil, &mockWriterImpl{}))
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{})

		// When writing the blueprint
		if err := handler.Write(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the repository URL is the normalized remote
		if handler.ComposedBlueprint().Repository.Url != "github.com/test/repo" {
			t.Errorf("Expected URL 'github.com/test/repo', got '%s'", handler.ComposedBlueprint().Repository.Url)
		}
	})

	t.Run("PreservesFileSchemeURL", func(t *testing.T) {
		// Given a git remote origin URL and no dev mode
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "remote.origin.url" {
				return "file:///tmp/repo", nil
			}
			return "", nil
		}

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder, blueprint.HandlerOverrides(nil, nil, &mockWriterImpl{}))
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{})

		// When writing the blueprint
		if err := handler.Write(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the repository URL is the normalized remote
		if handler.ComposedBlueprint().Repository.Url != "file:///tmp/repo" {
			t.Errorf("Expected URL 'file:///tmp/repo', got '%s'", handler.ComposedBlueprint().Repository.Url)
		}
	})

	t.Run("AddsHTTPSForInvalidSchemePrefix", func(t *testing.T) {
		// Given a git remote origin URL and no dev mode
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "remote.origin.url" {
				return "1invalid://github.com/test/repo.git", nil
			}
			return "", nil
		}

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder, blueprint.HandlerOverrides(nil, nil, &mockWriterImpl{}))
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{})

		// When writing the blueprint
		if err := handler.Write(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the repository URL is the normalized remote
		if handler.ComposedBlueprint().Repository.Url != "github.com/test/repo" {
			t.Errorf("Expected URL 'github.com/test/repo', got '%s'", handler.ComposedBlueprint().Repository.Url)
		}
	})

	t.Run("NormalizesExtendedSchemeURL", func(t *testing.T) {
		// Given a git remote origin URL and no dev mode
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "remote.origin.url" {
				return "git+ssh://github.com/test/repo.git", nil
			}
			return "", nil
		}

		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder, blueprint.HandlerOverrides(nil, nil, &mockWriterImpl{}))
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{})

		// When writing the blueprint
		if err := handler.Write(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the repository URL is the normalized remote
		if handler.ComposedBlueprint().Repository.Url != "github.com/test/repo" {
			t.Errorf("Expected URL 'github.com/test/repo', got '%s'", handler.ComposedBlueprint().Repository.Url)
		}
	})
}

func TestHandler_RetargetSource(t *testing.T) {
	setup := func(t *testing.T) *blueprint.BaseBlueprintHandler {
		t.Helper()
		mocks := setupHandlerMocks(t)
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.3.0", Ref: blueprintv1alpha1.Reference{SemVer: "v0.3.0"}},
			},
		})
		return handler
	}

	t.Run("RepointsDeclaredSourceAndReturnsPrevious", func(t *testing.T) {
		// Given a handler with a declared core source
		handler := setup(t)

		// When retargeting it to a newer tagged URL
		previous, err := handler.RetargetSource("core", "oci://ghcr.io/windsorcli/core:v0.6.0")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the previous URL is returned and the source carries the new URL with its ref cleared
		if previous != "oci://ghcr.io/windsorcli/core:v0.3.0" {
			t.Errorf("Expected previous URL, got %q", previous)
		}
		if handler.ComposedBlueprint().Sources[0].Url != "oci://ghcr.io/windsorcli/core:v0.6.0" {
			t.Errorf("Expected URL repointed, got %q", handler.ComposedBlueprint().Sources[0].Url)
		}
		if handler.ComposedBlueprint().Sources[0].Ref != (blueprintv1alpha1.Reference{}) {
			t.Errorf("Expected ref cleared (tag lives in URL), got %+v", handler.ComposedBlueprint().Sources[0].Ref)
		}
	})

	t.Run("ErrorsOnUndeclaredSource", func(t *testing.T) {
		// Given a handler without an 'addons' source
		handler := setup(t)

		// When retargeting an undeclared source, it errors rather than adding one
		if _, err := handler.RetargetSource("addons", "oci://ghcr.io/acme/addons:v0.5.0"); err == nil {
			t.Error("Expected an error for an undeclared source")
		}
	})

	t.Run("ErrorsOnURLWithoutVersion", func(t *testing.T) {
		// Given a handler with a declared source
		handler := setup(t)

		// When the new URL omits a version tag, it is rejected
		if _, err := handler.RetargetSource("core", "oci://ghcr.io/windsorcli/core"); err == nil {
			t.Error("Expected an error for a URL missing a version")
		}
	})

	t.Run("ErrorsWhenBlueprintNotLoaded", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.SetComposedBlueprint(nil)
		if _, err := handler.RetargetSource("core", "oci://ghcr.io/windsorcli/core:v0.6.0"); err == nil {
			t.Error("Expected an error when no blueprint is loaded")
		}
	})
}

func TestHandler_UpgradeSourcesToLatest(t *testing.T) {
	setup := func(t *testing.T, sources []blueprintv1alpha1.Source, tags map[string][]string) (*blueprint.BaseBlueprintHandler, *artifact.MockArtifact) {
		t.Helper()
		mocks := setupHandlerMocks(t)
		mocks.ArtifactBuilder.ListTagsFunc = func(ociRef string) ([]string, error) {
			return tags[ociRef], nil
		}
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{Sources: sources})
		return handler, mocks.ArtifactBuilder
	}

	t.Run("BumpsOCISemverSourceToLatestStable", func(t *testing.T) {
		// Given a core source at v0.5.0 with newer stable and prerelease tags available
		handler, _ := setup(t,
			[]blueprintv1alpha1.Source{{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.5.0", Ref: blueprintv1alpha1.Reference{SemVer: "v0.5.0"}}},
			map[string][]string{"oci://ghcr.io/windsorcli/core:v0.5.0": {"v0.5.0", "v0.6.0", "v0.7.0-rc.1"}},
		)

		// When upgrading sources to latest
		upgrades, err := handler.UpgradeSourcesToLatest()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it moves to the latest stable (v0.6.0), skipping the rc, and clears the ref
		if len(upgrades) != 1 || upgrades[0].Name != "core" {
			t.Fatalf("Expected one core upgrade, got %v", upgrades)
		}
		if upgrades[0].To != "oci://ghcr.io/windsorcli/core:v0.6.0" {
			t.Errorf("Expected target v0.6.0, got %q", upgrades[0].To)
		}
		if handler.ComposedBlueprint().Sources[0].Url != "oci://ghcr.io/windsorcli/core:v0.6.0" {
			t.Errorf("Expected source URL bumped, got %q", handler.ComposedBlueprint().Sources[0].Url)
		}
		if handler.ComposedBlueprint().Sources[0].Ref != (blueprintv1alpha1.Reference{}) {
			t.Errorf("Expected ref cleared, got %+v", handler.ComposedBlueprint().Sources[0].Ref)
		}
	})

	t.Run("SkipsNonOCIAndNonSemverAndAlreadyLatest", func(t *testing.T) {
		// Given a git source, a mutable-tag OCI source, and an already-latest OCI source
		handler, _ := setup(t,
			[]blueprintv1alpha1.Source{
				{Name: "modules", Url: "github.com/org/modules", Ref: blueprintv1alpha1.Reference{Branch: "main"}},
				{Name: "dev", Url: "oci://ghcr.io/acme/dev:latest"},
				{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.6.0", Ref: blueprintv1alpha1.Reference{SemVer: "v0.6.0"}},
			},
			map[string][]string{
				"oci://ghcr.io/acme/dev:latest":        {"v9.9.9"},
				"oci://ghcr.io/windsorcli/core:v0.6.0": {"v0.5.0", "v0.6.0"},
			},
		)

		// When upgrading
		upgrades, err := handler.UpgradeSourcesToLatest()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then nothing is upgraded (git skipped, :latest not semver, core already latest)
		if len(upgrades) != 0 {
			t.Errorf("Expected no upgrades, got %v", upgrades)
		}
	})

	t.Run("PropagatesListTagsError", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		mocks.ArtifactBuilder.ListTagsFunc = func(ociRef string) ([]string, error) {
			return nil, fmt.Errorf("registry unreachable")
		}
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.5.0"}},
		})
		if _, err := handler.UpgradeSourcesToLatest(); err == nil {
			t.Error("Expected an error when tag listing fails")
		}
	})

	t.Run("ErrorsWhenBlueprintNotLoaded", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.SetComposedBlueprint(nil)
		if _, err := handler.UpgradeSourcesToLatest(); err == nil {
			t.Error("Expected an error when no blueprint is loaded")
		}
	})

	t.Run("LeavesBlueprintUntouchedWhenAListFailsMidway", func(t *testing.T) {
		// Given a resolvable first source and a second source whose tag listing fails
		handler, mock := setup(t,
			[]blueprintv1alpha1.Source{
				{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.5.0"},
				{Name: "addons", Url: "oci://ghcr.io/acme/addons:v0.5.0"},
			},
			nil,
		)
		mock.ListTagsFunc = func(ociRef string) ([]string, error) {
			if ociRef == "oci://ghcr.io/acme/addons:v0.5.0" {
				return nil, fmt.Errorf("registry unreachable")
			}
			return []string{"v0.6.0"}, nil
		}

		// When upgrading, the second source's failure aborts the whole operation
		if _, err := handler.UpgradeSourcesToLatest(); err == nil {
			t.Fatal("Expected an error when a source's tag listing fails")
		}

		// Then the first source is left untouched — no partial in-memory mutation
		if handler.ComposedBlueprint().Sources[0].Url != "oci://ghcr.io/windsorcli/core:v0.5.0" {
			t.Errorf("Expected the first source unmutated, got %q", handler.ComposedBlueprint().Sources[0].Url)
		}
	})

	t.Run("SkipsIncompatibleNewestTagAndSelectsNextCompatible", func(t *testing.T) {
		// Given a running CLI that the newest tag's declared cliVersion excludes, but an
		// older-than-newest (still newer than current) tag admits
		setTestCliVersion(t, "0.8.1")

		handler, mock := setup(t,
			[]blueprintv1alpha1.Source{{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.5.0"}},
			map[string][]string{"oci://ghcr.io/windsorcli/core:v0.5.0": {"v0.5.0", "v0.6.0", "v0.7.0"}},
		)
		mock.GetCliVersionConstraintFunc = func(ociRef string) (string, error) {
			if ociRef == "oci://ghcr.io/windsorcli/core:v0.7.0" {
				return ">=99.0.0", nil
			}
			return "", nil
		}

		// When upgrading sources to latest
		upgrades, err := handler.UpgradeSourcesToLatest()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it skips v0.7.0 and moves to v0.6.0, the highest tag this CLI is compatible with
		if len(upgrades) != 1 || upgrades[0].To != "oci://ghcr.io/windsorcli/core:v0.6.0" {
			t.Fatalf("Expected upgrade to v0.6.0 (skipping incompatible v0.7.0), got %v", upgrades)
		}
	})

	t.Run("SkipsSourceWhenNoNewerTagIsCompatible", func(t *testing.T) {
		// Given every tag newer than current declares a cliVersion constraint this CLI fails
		setTestCliVersion(t, "0.8.1")

		handler, mock := setup(t,
			[]blueprintv1alpha1.Source{{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.5.0"}},
			map[string][]string{"oci://ghcr.io/windsorcli/core:v0.5.0": {"v0.5.0", "v0.6.0"}},
		)
		mock.GetCliVersionConstraintFunc = func(ociRef string) (string, error) {
			if ociRef == "oci://ghcr.io/windsorcli/core:v0.6.0" {
				return ">=99.0.0", nil
			}
			return "", nil
		}

		// When upgrading sources to latest
		upgrades, err := handler.UpgradeSourcesToLatest()

		// Then no upgrade happens — no compatible tag newer than current exists — and this is not
		// treated as an error
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(upgrades) != 0 {
			t.Errorf("Expected no upgrades, got %v", upgrades)
		}
	})

	t.Run("FallsBackToVerifyWhenNewestTagHasNoAnnotation", func(t *testing.T) {
		// Given the newest tag carries no cliVersion manifest annotation (an artifact published
		// before that annotation existed) but its in-tarball metadata.yaml, checked via the
		// fallback, is compatible
		handler, mock := setup(t,
			[]blueprintv1alpha1.Source{{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.5.0"}},
			map[string][]string{"oci://ghcr.io/windsorcli/core:v0.5.0": {"v0.5.0", "v0.6.0"}},
		)
		var verifiedRef string
		mock.VerifyCliVersionCompatibilityFunc = func(ociRef string) error {
			verifiedRef = ociRef
			return nil
		}

		// When upgrading sources to latest
		upgrades, err := handler.UpgradeSourcesToLatest()

		// Then it upgrades to the fallback-verified tag
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(upgrades) != 1 || upgrades[0].To != "oci://ghcr.io/windsorcli/core:v0.6.0" {
			t.Fatalf("Expected upgrade to v0.6.0 via the verify fallback, got %v", upgrades)
		}
		if verifiedRef != "oci://ghcr.io/windsorcli/core:v0.6.0" {
			t.Errorf("Expected the fallback to verify the resolved candidate, got %q", verifiedRef)
		}
	})

	t.Run("PropagatesGetCliVersionConstraintError", func(t *testing.T) {
		// Given a candidate tag whose compatibility cannot be checked (e.g. a registry hiccup)
		handler, mock := setup(t,
			[]blueprintv1alpha1.Source{{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.5.0"}},
			map[string][]string{"oci://ghcr.io/windsorcli/core:v0.5.0": {"v0.5.0", "v0.6.0"}},
		)
		mock.GetCliVersionConstraintFunc = func(ociRef string) (string, error) {
			return "", fmt.Errorf("registry unreachable")
		}

		// When upgrading sources to latest
		if _, err := handler.UpgradeSourcesToLatest(); err == nil {
			t.Error("Expected an error when a candidate's compatibility cannot be checked")
		}
	})
}

func TestHandler_GetTerraformComponents(t *testing.T) {
	t.Run("ReturnsNilWhenNoBlueprint", func(t *testing.T) {
		// Given a handler with no composed blueprint
		mocks := setupHandlerMocks(t)
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

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
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "rds"},
			},
		})

		// When getting components
		components := handler.GetTerraformComponents()

		// Then should return components
		if len(components) != 2 {
			t.Errorf("Expected 2 components, got %d", len(components))
		}
	})

	t.Run("ResolvesSourceNamesToModuleURLs", func(t *testing.T) {
		// Given components that name OCI, git, and local template sources
		mocks := setupHandlerMocks(t)
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "pinned", Url: "oci://ghcr.io/acme/core:v1.0.0", Ref: blueprintv1alpha1.Reference{Tag: "v2.0.0"}},
				{Name: "untagged", Url: "oci://ghcr.io/acme/core", Ref: blueprintv1alpha1.Reference{Tag: "v2.0.0"}},
				{Name: "bare", Url: "oci://ghcr.io/acme/core"},
				{Name: "commit", Url: "https://github.com/acme/core", PathPrefix: "modules", Ref: blueprintv1alpha1.Reference{Commit: "abc123", SemVer: "~1.0", Tag: "v1", Branch: "main"}},
				{Name: "semver", Url: "https://github.com/acme/core", Ref: blueprintv1alpha1.Reference{SemVer: "~1.0", Tag: "v1", Branch: "main"}},
				{Name: "tag", Url: "https://github.com/acme/core", Ref: blueprintv1alpha1.Reference{Tag: "v1", Branch: "main"}},
				{Name: "branch", Url: "https://github.com/acme/core", Ref: blueprintv1alpha1.Reference{Branch: "main"}},
				{Name: "noref", Url: "https://github.com/acme/core"},
				{Name: "template"},
			},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "a", Source: "pinned"},
				{Path: "b", Source: "untagged"},
				{Path: "c", Source: "bare"},
				{Path: "d", Source: "commit"},
				{Path: "e", Source: "semver"},
				{Path: "f", Source: "tag"},
				{Path: "g", Source: "branch"},
				{Path: "h", Source: "noref"},
				{Path: "i", Source: "template"},
				{Path: "j", Source: "undeclared"},
			},
		})

		// When getting components
		components := handler.GetTerraformComponents()

		// Then each source name becomes a module URL with the highest-priority ref
		want := []string{
			"oci://ghcr.io/acme/core:v1.0.0//terraform/a",
			"oci://ghcr.io/acme/core:v2.0.0//terraform/b",
			"oci://ghcr.io/acme/core//terraform/c",
			"https://github.com/acme/core//modules/d?ref=abc123",
			"https://github.com/acme/core//terraform/e?ref=~1.0",
			"https://github.com/acme/core//terraform/f?ref=v1",
			"https://github.com/acme/core//terraform/g?ref=main",
			"https://github.com/acme/core//terraform/h?ref=",
			"",
			"undeclared",
		}
		for i, w := range want {
			if components[i].Source != w {
				t.Errorf("component %s: expected source %q, got %q", components[i].Path, w, components[i].Source)
			}
		}
	})

	t.Run("PlacesSourcedAndNamedComponentsInTheScratchPath", func(t *testing.T) {
		// Given a sourced component, a named component, and a local component
		mocks := setupHandlerMocks(t)
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{{Name: "core", Url: "oci://ghcr.io/acme/core:v1.0.0"}},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "network/vpc", Source: "core"},
				{Path: "cluster", Name: "main-cluster"},
				{Path: "local/app"},
			},
		})

		// When getting components
		components := handler.GetTerraformComponents()

		// Then sourced and named components use the scratch path and local components the project path
		rt := mocks.Runtime
		want := []string{
			filepath.Join(rt.WindsorScratchPath, "terraform", "network/vpc"),
			filepath.Join(rt.WindsorScratchPath, "terraform", "main-cluster"),
			filepath.Join(rt.ProjectRoot, "terraform", "local/app"),
		}
		for i, w := range want {
			if components[i].FullPath != w {
				t.Errorf("component %s: expected full path %q, got %q", components[i].Path, w, components[i].FullPath)
			}
		}
	})

	t.Run("DoesNotChangeTheComposedBlueprint", func(t *testing.T) {
		// Given a component that names a source
		mocks := setupHandlerMocks(t)
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{
			Sources:             []blueprintv1alpha1.Source{{Name: "core", Url: "oci://ghcr.io/acme/core:v1.0.0"}},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Path: "vpc", Source: "core"}},
		})

		// When getting components
		_ = handler.GetTerraformComponents()

		// Then the composed component keeps its source name
		if got := handler.ComposedBlueprint().TerraformComponents[0]; got.Source != "core" || got.FullPath != "" {
			t.Errorf("expected the composed component unchanged, got %+v", got)
		}
	})
}

func TestHandler_Generate(t *testing.T) {
	t.Run("ReturnsNilWhenNoBlueprint", func(t *testing.T) {
		// Given a handler with no composed blueprint
		mocks := setupHandlerMocks(t)
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

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
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "generated"},
		})

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

func TestHandler_GenerateResolved(t *testing.T) {
	t.Run("DoesNotMutateComposedBlueprint", func(t *testing.T) {
		// Given a handler with a deferred substitution
		mocks := setupHandlerMocks(t)
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{
			Substitutions: map[string]string{
				"MY_KEY": "<deferred>",
			},
		})
		handler.SetDeferredPaths(map[string]bool{
			"substitutions.MY_KEY": true,
		})

		// When calling GenerateResolved
		resolved, err := handler.GenerateResolved()
		if err != nil {
			t.Fatalf("Expected no error for resolvable substitution, got %v", err)
		}

		// Then the returned blueprint is a separate object
		if resolved == handler.ComposedBlueprint() {
			t.Error("Expected GenerateResolved to return a deep copy, not the same pointer")
		}

		// And the original composedBlueprint retains the deferred placeholder
		if handler.ComposedBlueprint().Substitutions["MY_KEY"] != "<deferred>" {
			t.Errorf("Expected composedBlueprint to retain '<deferred>', got '%s'", handler.ComposedBlueprint().Substitutions["MY_KEY"])
		}
	})

	t.Run("ReturnsNilWhenNoBlueprint", func(t *testing.T) {
		// Given a handler with no composed blueprint
		mocks := setupHandlerMocks(t)
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When calling GenerateResolved
		result, err := handler.GenerateResolved()
		if err != nil {
			t.Fatalf("Expected no error when no blueprint, got %v", err)
		}

		// Then should return nil
		if result != nil {
			t.Error("Expected nil when no blueprint")
		}
	})

	t.Run("PropagatesEvaluatorErrorWithSubstitutionPath", func(t *testing.T) {
		// A deferred substitution whose expression errors (e.g. terraform_output() referencing
		// a missing module key) surfaces the error here, named by its substitution path.
		mocks := setupHandlerMocks(t)
		mocks.Runtime.Evaluator = &evaluator.MockExpressionEvaluator{
			EvaluateFunc: func(expr string, _ string, _ map[string]any, _ bool) (any, error) {
				return nil, fmt.Errorf("terraform output 'tenant_id' for component cluster not found")
			},
		}
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "dns",
					Substitutions: map[string]string{
						"external_dns_tenant_id": "${terraform_output('cluster', 'tenant_id') ?? ''}",
					},
				},
			},
		})
		handler.SetDeferredPaths(map[string]bool{
			"kustomize.dns.substitutions.external_dns_tenant_id": true,
		})

		// When calling GenerateResolved
		resolved, err := handler.GenerateResolved()

		// Then the error surfaces with the substitution path called out by name —
		// "kustomize.dns.substitutions.external_dns_tenant_id" — so the operator can grep
		// directly from the failure to the offending facet entry.
		if err == nil {
			t.Fatal("Expected GenerateResolved to surface the evaluator error, got nil")
		}
		if !strings.Contains(err.Error(), "kustomize.dns.substitutions.external_dns_tenant_id") {
			t.Errorf("Expected error to name the substitution path, got: %v", err)
		}
		if !strings.Contains(err.Error(), "tenant_id") {
			t.Errorf("Expected error to preserve the underlying evaluator message, got: %v", err)
		}
		if resolved != nil {
			t.Error("Expected nil blueprint on resolve failure (no partial results to consume)")
		}
	})

	t.Run("ResolvesFluxSystemInstallAndResourcesSubstitutions", func(t *testing.T) {
		// Two resources variants share a substitution key name; only "internal"'s is deferred.
		mocks := setupHandlerMocks(t)
		mocks.Runtime.Evaluator = &evaluator.MockExpressionEvaluator{
			EvaluateFunc: func(expr string, _ string, _ map[string]any, _ bool) (any, error) {
				return "resolved-value", nil
			},
		}
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name: "gateway",
					Install: &blueprintv1alpha1.Kustomization{
						Substitutions: map[string]string{
							"cluster_name": "${terraform_output('cluster', 'cluster_name') ?? ''}",
						},
					},
					Resources: []blueprintv1alpha1.FluxVariant{
						{
							Kustomization: blueprintv1alpha1.Kustomization{
								Name: "internal",
								Substitutions: map[string]string{
									"gateway_lb_ip": "${terraform_output('network', 'internal_lb_ip') ?? ''}",
								},
							},
						},
						{
							Kustomization: blueprintv1alpha1.Kustomization{
								Name: "external",
								Substitutions: map[string]string{
									"gateway_lb_ip": "already-resolved",
								},
							},
						},
					},
				},
			},
		})
		handler.SetDeferredPaths(map[string]bool{
			"flux.gateway.install.substitutions.cluster_name":             true,
			"flux.gateway.resources-internal.substitutions.gateway_lb_ip": true,
		})

		// When calling GenerateResolved
		resolved, err := handler.GenerateResolved()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then only the deferred substitutions are resolved
		sys := resolved.FluxSystems[0]
		if sys.Install.Substitutions["cluster_name"] != "resolved-value" {
			t.Errorf("Expected install substitution resolved, got %q", sys.Install.Substitutions["cluster_name"])
		}
		if sys.Resources[0].Substitutions["gateway_lb_ip"] != "resolved-value" {
			t.Errorf("Expected 'internal' variant's deferred substitution resolved, got %q", sys.Resources[0].Substitutions["gateway_lb_ip"])
		}
		if sys.Resources[1].Substitutions["gateway_lb_ip"] != "already-resolved" {
			t.Errorf("Expected 'external' variant's already-resolved substitution left untouched, got %q", sys.Resources[1].Substitutions["gateway_lb_ip"])
		}
	})

	t.Run("RendersPostRunMessageWhenGateTrue", func(t *testing.T) {
		// Given a message gated true whose text interpolates a run value
		mocks := setupHandlerMocks(t)
		mocks.Runtime.Evaluator = &evaluator.MockExpressionEvaluator{
			EvaluateFunc: func(expr string, _ string, _ map[string]any, _ bool) (any, error) {
				if strings.Contains(expr, "public_domain") {
					return true, nil
				}
				return "Delegate ns1.example, ns2.example", nil
			},
		}
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{
			Messages: []blueprintv1alpha1.Message{
				{When: "dns.public_domain != ''", Text: "Delegate ${terraform_output('dns-zone', 'nameservers')}"},
			},
		})

		// When resolving
		resolved, err := handler.GenerateResolved()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the message renders with interpolated text and cleared When
		if len(resolved.Messages) != 1 {
			t.Fatalf("Expected 1 rendered message, got %d", len(resolved.Messages))
		}
		if resolved.Messages[0].Text != "Delegate ns1.example, ns2.example" {
			t.Errorf("Expected interpolated text, got %q", resolved.Messages[0].Text)
		}
		if resolved.Messages[0].When != "" {
			t.Errorf("Expected When cleared after gating, got %q", resolved.Messages[0].When)
		}
	})

	t.Run("DropsPostRunMessageWhenGateFalse", func(t *testing.T) {
		// Given a message whose When evaluates false
		mocks := setupHandlerMocks(t)
		mocks.Runtime.Evaluator = &evaluator.MockExpressionEvaluator{
			EvaluateFunc: func(expr string, _ string, _ map[string]any, _ bool) (any, error) {
				if strings.Contains(expr, "public_domain") {
					return false, nil
				}
				return "should not render", nil
			},
		}
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{
			Messages: []blueprintv1alpha1.Message{
				{When: "dns.public_domain != ''", Text: "delegate ${x}"},
			},
		})

		// When resolving
		resolved, err := handler.GenerateResolved()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the gated-out message is dropped
		if len(resolved.Messages) != 0 {
			t.Errorf("Expected gated-out message dropped, got %v", resolved.Messages)
		}
	})

	t.Run("DeduplicatesIdenticalRenderedMessages", func(t *testing.T) {
		// Given two messages that render to the same text
		mocks := setupHandlerMocks(t)
		mocks.Runtime.Evaluator = &evaluator.MockExpressionEvaluator{
			EvaluateFunc: func(expr string, _ string, _ map[string]any, _ bool) (any, error) {
				return "same note", nil
			},
		}
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{
			Messages: []blueprintv1alpha1.Message{
				{Text: "${a}"},
				{Text: "${b}"},
			},
		})

		// When resolving
		resolved, err := handler.GenerateResolved()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the duplicate rendered text surfaces once
		if len(resolved.Messages) != 1 {
			t.Fatalf("Expected deduplicated to 1 message, got %d", len(resolved.Messages))
		}
		if resolved.Messages[0].Text != "same note" {
			t.Errorf("Expected 'same note', got %q", resolved.Messages[0].Text)
		}
	})

	t.Run("PropagatesMessageWhenEvaluationError", func(t *testing.T) {
		// Given a message whose When expression errors
		mocks := setupHandlerMocks(t)
		mocks.Runtime.Evaluator = &evaluator.MockExpressionEvaluator{
			EvaluateFunc: func(expr string, _ string, _ map[string]any, _ bool) (any, error) {
				return nil, fmt.Errorf("bad when expression")
			},
		}
		handler := blueprint.NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.SetComposedBlueprint(&blueprintv1alpha1.Blueprint{
			Messages: []blueprintv1alpha1.Message{
				{When: "broken(", Text: "x"},
			},
		})

		// When resolving
		resolved, err := handler.GenerateResolved()

		// Then the error surfaces and no partial blueprint is returned
		if err == nil {
			t.Fatal("Expected message When error to surface, got nil")
		}
		if !strings.Contains(err.Error(), "message when") {
			t.Errorf("Expected error to name the message when, got %v", err)
		}
		if resolved != nil {
			t.Error("Expected nil blueprint on message resolve failure")
		}
	})
}

func TestHandler_DeferredSubstitutionsInScope(t *testing.T) {
	t.Run("NoDeferredPaths", func(t *testing.T) {
		handler := &blueprint.BaseBlueprintHandler{}
		if handler.DeferredSubstitutionsInScope(map[string]bool{"my-app": true}) {
			t.Error("Expected false for no deferred paths")
		}
	})

	t.Run("KustomizationInScope", func(t *testing.T) {
		handler := &blueprint.BaseBlueprintHandler{}
		handler.SetDeferredPaths(map[string]bool{"kustomize.my-app.substitutions.cert": true})
		if !handler.DeferredSubstitutionsInScope(map[string]bool{"my-app": true}) {
			t.Error("Expected true when the deferred kustomization is in scope")
		}
	})

	t.Run("KustomizationOutOfScope", func(t *testing.T) {
		handler := &blueprint.BaseBlueprintHandler{}
		handler.SetDeferredPaths(map[string]bool{"kustomize.other-app.substitutions.cert": true})
		if handler.DeferredSubstitutionsInScope(map[string]bool{"my-app": true}) {
			t.Error("Expected false when the deferred kustomization is not in scope")
		}
	})

	t.Run("NamePrefixCollisionDoesNotFalsePositive", func(t *testing.T) {
		// A kustomization named "my-app-2" must not match scope {"my-app"} via a bare
		// string-prefix check on the raw path.
		handler := &blueprint.BaseBlueprintHandler{}
		handler.SetDeferredPaths(map[string]bool{"kustomize.my-app-2.substitutions.cert": true})
		if handler.DeferredSubstitutionsInScope(map[string]bool{"my-app": true}) {
			t.Error("Expected false: 'my-app-2' is a different kustomization than 'my-app'")
		}
	})

	t.Run("GlobalConfigMapAlwaysInScope", func(t *testing.T) {
		handler := &blueprint.BaseBlueprintHandler{}
		handler.SetDeferredPaths(map[string]bool{"configmaps.shared.cert": true})
		if !handler.DeferredSubstitutionsInScope(map[string]bool{}) {
			t.Error("Expected true: blueprint-level ConfigMaps are always in scope")
		}
	})

	t.Run("GlobalSubstitutionAlwaysInScope", func(t *testing.T) {
		handler := &blueprint.BaseBlueprintHandler{}
		handler.SetDeferredPaths(map[string]bool{"substitutions.tenant_id": true})
		if !handler.DeferredSubstitutionsInScope(map[string]bool{}) {
			t.Error("Expected true: global substitutions are always in scope")
		}
	})

	t.Run("UnrecognizedPrefixIgnored", func(t *testing.T) {
		handler := &blueprint.BaseBlueprintHandler{}
		handler.SetDeferredPaths(map[string]bool{"messages.0": true})
		if handler.DeferredSubstitutionsInScope(map[string]bool{"my-app": true}) {
			t.Error("Expected false for a path outside the recognized prefixes")
		}
	})

	t.Run("FluxInstallTierInScope", func(t *testing.T) {
		handler := &blueprint.BaseBlueprintHandler{}
		handler.SetDeferredPaths(map[string]bool{"flux.database.install.substitutions.cert": true})
		if !handler.DeferredSubstitutionsInScope(map[string]bool{"database-install": true}) {
			t.Error("Expected true: the deferred flux install tier's compiled name is in scope")
		}
	})

	t.Run("FluxResourcesTierInScope", func(t *testing.T) {
		handler := &blueprint.BaseBlueprintHandler{}
		handler.SetDeferredPaths(map[string]bool{"flux.database.resources.substitutions.crossplane_gcp_service_account_email": true})
		if !handler.DeferredSubstitutionsInScope(map[string]bool{"database-resources": true}) {
			t.Error("Expected true: the deferred flux resources tier's compiled name is in scope")
		}
	})

	t.Run("FluxResourcesVariantTierInScope", func(t *testing.T) {
		handler := &blueprint.BaseBlueprintHandler{}
		handler.SetDeferredPaths(map[string]bool{"flux.database.resources-primary.substitutions.cert": true})
		if !handler.DeferredSubstitutionsInScope(map[string]bool{"database-resources-primary": true}) {
			t.Error("Expected true: the deferred flux resources variant's compiled name is in scope")
		}
	})

	t.Run("FluxTierOutOfScope", func(t *testing.T) {
		handler := &blueprint.BaseBlueprintHandler{}
		handler.SetDeferredPaths(map[string]bool{"flux.database.install.substitutions.cert": true})
		if handler.DeferredSubstitutionsInScope(map[string]bool{"other-install": true}) {
			t.Error("Expected false when the deferred flux tier is not in scope")
		}
	})
}

func TestIsDowngrade(t *testing.T) {
	t.Run("TrueWhenTargetTagIsOlderInSameRepo", func(t *testing.T) {
		// Given a target that pins an older semver than the previous ref of the same repository
		// When comparing them
		// Then it is reported as a downgrade
		if !blueprint.IsDowngrade("oci://ghcr.io/windsorcli/core:v0.6.0", "oci://ghcr.io/windsorcli/core:v0.3.0") {
			t.Error("Expected v0.3.0 after v0.6.0 to be a downgrade")
		}
	})

	t.Run("FalseWhenTargetTagIsNewer", func(t *testing.T) {
		// Given a target that pins a newer semver
		// When comparing them
		// Then it is not a downgrade
		if blueprint.IsDowngrade("oci://ghcr.io/windsorcli/core:v0.3.0", "oci://ghcr.io/windsorcli/core:v0.6.0") {
			t.Error("Expected v0.6.0 after v0.3.0 to be an upgrade, not a downgrade")
		}
	})

	t.Run("FalseWhenTagsAreEqual", func(t *testing.T) {
		// Given identical tags
		// When comparing them
		// Then it is not a downgrade
		if blueprint.IsDowngrade("oci://ghcr.io/windsorcli/core:v0.6.0", "oci://ghcr.io/windsorcli/core:v0.6.0") {
			t.Error("Expected an unchanged tag not to be a downgrade")
		}
	})

	t.Run("FalseWhenRepositoriesDiffer", func(t *testing.T) {
		// Given a target that points at a different repository, even at a lower tag
		// When comparing them
		// Then it is a re-source, not an ordered downgrade
		if blueprint.IsDowngrade("oci://ghcr.io/windsorcli/core:v0.6.0", "oci://ghcr.io/acme/core:v0.3.0") {
			t.Error("Expected a different repository not to be treated as a downgrade")
		}
	})

	t.Run("FalseWhenEitherTagIsNotSemver", func(t *testing.T) {
		// Given a ref pinned to a branch or mutable tag rather than a semver
		// When comparing them
		// Then the change is not ordered and not a downgrade
		if blueprint.IsDowngrade("oci://ghcr.io/windsorcli/core:main", "oci://ghcr.io/windsorcli/core:v0.3.0") {
			t.Error("Expected a non-semver previous ref not to be ordered")
		}
		if blueprint.IsDowngrade("oci://ghcr.io/windsorcli/core:v0.6.0", "oci://ghcr.io/windsorcli/core:latest") {
			t.Error("Expected a non-semver target ref not to be ordered")
		}
	})

	t.Run("FalseWhenEitherURLIsNotOCI", func(t *testing.T) {
		// Given a ref that is not a parseable OCI reference
		// When comparing them
		// Then no regression can be asserted
		if blueprint.IsDowngrade("https://example.com/core", "oci://ghcr.io/windsorcli/core:v0.3.0") {
			t.Error("Expected a non-OCI previous ref not to be a downgrade")
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

type mockProcessorImpl struct {
	err error
}

func (m *mockProcessorImpl) ProcessGlobally(sources []blueprint.SourceFacetSet) (map[string]any, error) {
	return nil, m.err
}

type mockComposerImpl struct {
	err error
}

func (m *mockComposerImpl) Compose(loaders []blueprint.BlueprintLoader, initLoaderNames []string, userBlueprintPath string, configScope map[string]any) (*blueprintv1alpha1.Blueprint, error) {
	return nil, m.err
}

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
