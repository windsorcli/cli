package blueprint

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
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
	Evaluator       *evaluator.ExpressionEvaluator
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

	eval := evaluator.NewExpressionEvaluator(mockConfigHandler, tmpDir, tmpDir)

	rt := &runtime.Runtime{
		ProjectRoot:   tmpDir,
		ConfigRoot:    tmpDir,
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
		Evaluator:     eval,
	}

	mocks := &HandlerTestMocks{
		Shell:           mockShell,
		ConfigHandler:   mockConfigHandler,
		ArtifactBuilder: mockArtifact,
		Evaluator:       eval,
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
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

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
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder, &BaseBlueprintHandler{processor: customProcessor})

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
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder, &BaseBlueprintHandler{composer: customComposer})

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
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder, &BaseBlueprintHandler{writer: customWriter})

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

		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

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

	t.Run("LoadsAndProcessesFeatures", func(t *testing.T) {
		// Given a handler with features
		mocks := setupHandlerMocks(t)
		mocks.Runtime.TemplateRoot = filepath.Join(mocks.Runtime.ProjectRoot, "_template")

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
  name: network
terraform:
  - path: vpc
`
		os.WriteFile(filepath.Join(featuresDir, "network.yaml"), []byte(featureYaml), 0644)

		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

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

	t.Run("MergesUserBlueprintOverPrimary", func(t *testing.T) {
		// Given primary and user blueprints
		mocks := setupHandlerMocks(t)
		mocks.Runtime.TemplateRoot = filepath.Join(mocks.Runtime.ProjectRoot, "_template")

		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)

		primaryYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: primary
terraform:
  - path: vpc
  - path: rds
`
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(primaryYaml), 0644)

		userYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: user
terraform:
  - path: vpc
`
		os.WriteFile(filepath.Join(mocks.Runtime.ConfigRoot, "blueprint.yaml"), []byte(userYaml), 0644)

		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then user should filter components
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if handler.composedBlueprint == nil {
			t.Fatal("Expected composed blueprint")
		}
		if len(handler.composedBlueprint.TerraformComponents) != 1 {
			t.Errorf("Expected 1 component (filtered by user), got %d", len(handler.composedBlueprint.TerraformComponents))
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

		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

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

	t.Run("SetsPrimaryLoader", func(t *testing.T) {
		// Given a handler with local template
		mocks := setupHandlerMocks(t)
		mocks.Runtime.TemplateRoot = filepath.Join(mocks.Runtime.ProjectRoot, "_template")

		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test
`), 0644)

		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then primary loader should be set
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if handler.primaryBlueprintLoader == nil {
			t.Error("Expected primary loader to be set")
		}
	})

	t.Run("ReturnsErrorWhenPrimaryLoadFails", func(t *testing.T) {
		// Given a handler with invalid template
		mocks := setupHandlerMocks(t)
		mocks.Runtime.TemplateRoot = filepath.Join(mocks.Runtime.ProjectRoot, "_template")

		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte("invalid: [yaml"), 0644)

		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid blueprint")
		}
	})

	t.Run("LoadsSourcesFromPrimaryBlueprint", func(t *testing.T) {
		// Given a primary blueprint with sources
		mocks := setupHandlerMocks(t)
		mocks.Runtime.TemplateRoot = filepath.Join(mocks.Runtime.ProjectRoot, "_template")

		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)

		primaryYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: primary
sources:
  - name: shared-modules
    url: oci://example.com/shared:v1.0.0
terraform:
  - path: vpc
    source: shared-modules
`
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(primaryYaml), 0644)

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

		mocks.ArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			return map[string]string{
				"example.com/shared:v1.0.0": sourceCacheDir,
			}, nil
		}
		mocks.ArtifactBuilder.ParseOCIRefFunc = func(ref string) (string, string, string, error) {
			return "example.com", "shared", "v1.0.0", nil
		}

		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then sources should be loaded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.sourceBlueprintLoaders) != 1 {
			t.Errorf("Expected 1 source loader, got %d", len(handler.sourceBlueprintLoaders))
		}
		if _, exists := handler.sourceBlueprintLoaders["shared-modules"]; !exists {
			t.Error("Expected 'shared-modules' source loader")
		}
	})

	t.Run("LoadsSourcesFromUserBlueprint", func(t *testing.T) {
		// Given a user blueprint with sources and a local primary template
		mocks := setupHandlerMocks(t)
		mocks.Runtime.TemplateRoot = filepath.Join(mocks.Runtime.ProjectRoot, "_template")

		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)
		primaryYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: primary
`
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(primaryYaml), 0644)

		userYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: user
sources:
  - name: user-source
    url: oci://example.com/user-modules:v1.0.0
terraform:
  - path: custom
    source: user-source
`
		os.WriteFile(filepath.Join(mocks.Runtime.ConfigRoot, "blueprint.yaml"), []byte(userYaml), 0644)

		sourceCacheDir := filepath.Join(mocks.Runtime.ProjectRoot, "user-source-cache")
		sourceTemplateDir := filepath.Join(sourceCacheDir, "_template")
		os.MkdirAll(sourceTemplateDir, 0755)

		sourceYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: user-source
terraform:
  - path: custom
`
		os.WriteFile(filepath.Join(sourceTemplateDir, "blueprint.yaml"), []byte(sourceYaml), 0644)

		mocks.ArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			return map[string]string{
				"example.com/user-modules:v1.0.0": sourceCacheDir,
			}, nil
		}
		mocks.ArtifactBuilder.ParseOCIRefFunc = func(ref string) (string, string, string, error) {
			return "example.com", "user-modules", "v1.0.0", nil
		}

		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then user sources should be loaded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.sourceBlueprintLoaders) != 1 {
			t.Errorf("Expected 1 source loader, got %d", len(handler.sourceBlueprintLoaders))
		}
	})

	t.Run("ReturnsErrorWhenSourceLoadFails", func(t *testing.T) {
		// Given a blueprint with invalid source
		mocks := setupHandlerMocks(t)
		mocks.Runtime.TemplateRoot = filepath.Join(mocks.Runtime.ProjectRoot, "_template")

		templateDir := mocks.Runtime.TemplateRoot
		os.MkdirAll(templateDir, 0755)

		primaryYaml := `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: primary
sources:
  - name: bad-source
    url: oci://example.com/bad:v1.0.0
`
		os.WriteFile(filepath.Join(templateDir, "blueprint.yaml"), []byte(primaryYaml), 0644)

		mocks.ArtifactBuilder.PullFunc = func(refs []string) (map[string]string, error) {
			return nil, os.ErrNotExist
		}

		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading blueprint
		err := handler.LoadBlueprint()

		// Then should return error
		if err == nil {
			t.Error("Expected error when source load fails")
		}
	})
}

func TestHandler_Write(t *testing.T) {
	t.Run("WriteDelegatesToWriter", func(t *testing.T) {
		// Given a handler with composed blueprint
		mocks := setupHandlerMocks(t)
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
		}

		writeCalled := false
		mockWriter := &mockWriterImpl{
			writeFunc: func(bp *blueprintv1alpha1.Blueprint, overwrite bool) error {
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
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{}

		var receivedOverwrite bool
		mockWriter := &mockWriterImpl{
			writeFunc: func(bp *blueprintv1alpha1.Blueprint, overwrite bool) error {
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
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

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
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
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
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		component := &blueprintv1alpha1.TerraformComponent{
			Name: "my-component",
			Path: "some/path",
		}

		// When resolving full path
		handler.resolveComponentFullPath(component)

		// Then should use Name in WindsorScratchPath
		expected := "/scratch/terraform/my-component"
		if component.FullPath != expected {
			t.Errorf("Expected '%s', got '%s'", expected, component.FullPath)
		}
	})

	t.Run("UsesSourcePathForSourceComponents", func(t *testing.T) {
		// Given a handler with a component having a source
		mocks := setupHandlerMocks(t)
		mocks.Runtime.WindsorScratchPath = "/scratch"
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		component := &blueprintv1alpha1.TerraformComponent{
			Source: "shared-modules",
			Path:   "vpc",
		}

		// When resolving full path
		handler.resolveComponentFullPath(component)

		// Then should use WindsorScratchPath with Path
		expected := "/scratch/terraform/vpc"
		if component.FullPath != expected {
			t.Errorf("Expected '%s', got '%s'", expected, component.FullPath)
		}
	})

	t.Run("UsesProjectRootForLocalComponents", func(t *testing.T) {
		// Given a handler with a local component (no name, no source)
		mocks := setupHandlerMocks(t)
		mocks.Runtime.ProjectRoot = "/project"
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		component := &blueprintv1alpha1.TerraformComponent{
			Path: "network/vpc",
		}

		// When resolving full path
		handler.resolveComponentFullPath(component)

		// Then should use ProjectRoot
		expected := "/project/terraform/network/vpc"
		if component.FullPath != expected {
			t.Errorf("Expected '%s', got '%s'", expected, component.FullPath)
		}
	})
}

func TestHandler_resolveComponentSource(t *testing.T) {
	t.Run("ResolvesOCISourceWithTag", func(t *testing.T) {
		// Given a handler with a composed blueprint containing an OCI source
		mocks := setupHandlerMocks(t)
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
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
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
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
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
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

	t.Run("DefaultsToMainRef", func(t *testing.T) {
		// Given a source with no ref specified
		mocks := setupHandlerMocks(t)
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "lib", Url: "https://github.com/org/lib"},
			},
		}
		component := &blueprintv1alpha1.TerraformComponent{Source: "lib", Path: "utils"}

		// When resolving component source
		handler.resolveComponentSource(component)

		// Then source should default to main ref
		expected := "https://github.com/org/lib//terraform/utils?ref=main"
		if component.Source != expected {
			t.Errorf("Expected '%s', got '%s'", expected, component.Source)
		}
	})

	t.Run("LeavesUnmatchedSourceUnchanged", func(t *testing.T) {
		// Given a component referencing a non-existent source
		mocks := setupHandlerMocks(t)
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
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
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
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
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
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
	t.Run("ReturnsTag", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		source := blueprintv1alpha1.Source{Ref: blueprintv1alpha1.Reference{Tag: "v1.0.0", Branch: "main"}}

		ref := handler.getSourceRef(source)

		if ref != "v1.0.0" {
			t.Errorf("Expected 'v1.0.0', got '%s'", ref)
		}
	})

	t.Run("ReturnsBranchWhenNoTag", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		source := blueprintv1alpha1.Source{Ref: blueprintv1alpha1.Reference{Branch: "develop"}}

		ref := handler.getSourceRef(source)

		if ref != "develop" {
			t.Errorf("Expected 'develop', got '%s'", ref)
		}
	})

	t.Run("ReturnsCommit", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		source := blueprintv1alpha1.Source{Ref: blueprintv1alpha1.Reference{Commit: "abc123"}}

		ref := handler.getSourceRef(source)

		if ref != "abc123" {
			t.Errorf("Expected 'abc123', got '%s'", ref)
		}
	})

	t.Run("DefaultsToMain", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		source := blueprintv1alpha1.Source{}

		ref := handler.getSourceRef(source)

		if ref != "main" {
			t.Errorf("Expected 'main', got '%s'", ref)
		}
	})
}

func TestHandler_GetLocalTemplateData(t *testing.T) {
	t.Run("ReturnsNilWhenNoPrimaryLoader", func(t *testing.T) {
		// Given a handler with no primary loader
		mocks := setupHandlerMocks(t)
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When getting template data
		data, err := handler.GetLocalTemplateData()

		// Then should return nil
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if data != nil {
			t.Error("Expected nil when no primary loader")
		}
	})

	t.Run("ReturnsDataFromPrimaryLoader", func(t *testing.T) {
		// Given a handler with primary loader
		mocks := setupHandlerMocks(t)
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		mockLoader := &mockLoaderImpl{
			getTemplateDataFunc: func() map[string][]byte {
				return map[string][]byte{"test.yaml": []byte("content")}
			},
		}
		handler.primaryBlueprintLoader = mockLoader

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
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

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
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
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
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

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
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

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
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

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

func TestHandler_loadSourcesFromBlueprint(t *testing.T) {
	t.Run("SkipsLoadingWhenLoaderNil", func(t *testing.T) {
		// Given a handler
		mocks := setupHandlerMocks(t)
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)

		// When loading sources from nil loader
		err := handler.loadSourcesFromBlueprint(nil)

		// Then should not error
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("SkipsLoadingWhenBlueprintNil", func(t *testing.T) {
		// Given a handler with loader returning nil blueprint
		mocks := setupHandlerMocks(t)
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		mockLoader := &mockLoaderImpl{
			getBlueprintFunc: func() *blueprintv1alpha1.Blueprint {
				return nil
			},
		}

		// When loading sources
		err := handler.loadSourcesFromBlueprint(mockLoader)

		// Then should not error
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("SkipsDuplicateSources", func(t *testing.T) {
		// Given a handler with existing source loader
		mocks := setupHandlerMocks(t)
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.sourceBlueprintLoaders["existing-source"] = &mockLoaderImpl{}

		mockLoader := &mockLoaderImpl{
			getBlueprintFunc: func() *blueprintv1alpha1.Blueprint {
				return &blueprintv1alpha1.Blueprint{
					Sources: []blueprintv1alpha1.Source{
						{Name: "existing-source", Url: "oci://example.com/test:v1"},
					},
				}
			},
		}

		// When loading sources
		err := handler.loadSourcesFromBlueprint(mockLoader)

		// Then should skip duplicate and not error
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.sourceBlueprintLoaders) != 1 {
			t.Errorf("Expected 1 source loader, got %d", len(handler.sourceBlueprintLoaders))
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

type mockWriterImpl struct {
	writeFunc func(bp *blueprintv1alpha1.Blueprint, overwrite bool) error
}

func (m *mockWriterImpl) Write(bp *blueprintv1alpha1.Blueprint, overwrite bool) error {
	if m.writeFunc != nil {
		return m.writeFunc(bp, overwrite)
	}
	return nil
}

type mockLoaderImpl struct {
	loadFunc            func() error
	getBlueprintFunc    func() *blueprintv1alpha1.Blueprint
	getFeaturesFunc     func() []blueprintv1alpha1.Feature
	getTemplateDataFunc func() map[string][]byte
	getSourceNameFunc   func() string
}

func (m *mockLoaderImpl) Load() error {
	if m.loadFunc != nil {
		return m.loadFunc()
	}
	return nil
}

func (m *mockLoaderImpl) GetBlueprint() *blueprintv1alpha1.Blueprint {
	if m.getBlueprintFunc != nil {
		return m.getBlueprintFunc()
	}
	return nil
}

func (m *mockLoaderImpl) GetFeatures() []blueprintv1alpha1.Feature {
	if m.getFeaturesFunc != nil {
		return m.getFeaturesFunc()
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

		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
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

		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
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

		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
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

	t.Run("SkipsDefaultsWhenUserBlueprintExists", func(t *testing.T) {
		// Given a handler with a user blueprint (even if empty)
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return key == "dev"
		}

		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.shims = &Shims{
			FilepathBase: func(s string) string { return "myproject" },
		}
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{}

		userBp := &blueprintv1alpha1.Blueprint{}
		handler.userBlueprintLoader = &mockLoaderImpl{
			getBlueprintFunc: func() *blueprintv1alpha1.Blueprint { return userBp },
		}

		// When setting repository defaults
		handler.setRepositoryDefaults()

		// Then no defaults should be set (user blueprint exists)
		if handler.composedBlueprint.Repository.Url != "" {
			t.Errorf("Expected empty repository URL when user blueprint exists, got '%s'", handler.composedBlueprint.Repository.Url)
		}
	})

	t.Run("HandlesNilBlueprint", func(t *testing.T) {
		// Given a handler with nil composed blueprint
		mocks := setupHandlerMocks(t)
		handler, _ := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = nil

		// When setting repository defaults
		handler.setRepositoryDefaults()

		// Then no panic should occur
	})
}
