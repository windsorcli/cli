package blueprint

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type ComposerTestMocks struct {
	Shell         *shell.MockShell
	ConfigHandler *config.MockConfigHandler
	Runtime       *runtime.Runtime
}

func setupComposerMocks(t *testing.T) *ComposerTestMocks {
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

	rt := &runtime.Runtime{
		Shims:         runtime.NewShims(),
		ProjectRoot:   tmpDir,
		ConfigRoot:    tmpDir,
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
	}

	mocks := &ComposerTestMocks{
		Shell:         mockShell,
		ConfigHandler: mockConfigHandler,
		Runtime:       rt,
	}

	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
	})

	return mocks
}

func createMockBlueprintLoader(sourceName string, blueprint *blueprintv1alpha1.Blueprint) BlueprintLoader {
	return &mockBlueprintLoaderForComposer{
		sourceName: sourceName,
		blueprint:  blueprint,
	}
}

type mockBlueprintLoaderForComposer struct {
	sourceName string
	blueprint  *blueprintv1alpha1.Blueprint
}

func (m *mockBlueprintLoaderForComposer) Load(sourceName, sourceURL string) error {
	return nil
}

func (m *mockBlueprintLoaderForComposer) GetBlueprint() *blueprintv1alpha1.Blueprint {
	return m.blueprint
}

func (m *mockBlueprintLoaderForComposer) GetFacets() []blueprintv1alpha1.Facet {
	return nil
}

func (m *mockBlueprintLoaderForComposer) GetTemplateData() map[string][]byte {
	return nil
}

func (m *mockBlueprintLoaderForComposer) GetSourceName() string {
	return m.sourceName
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewBlueprintComposer(t *testing.T) {
	t.Run("CreatesComposerWithDefaults", func(t *testing.T) {
		// Given a runtime
		mocks := setupComposerMocks(t)

		// When creating a new composer
		composer := NewBlueprintComposer(mocks.Runtime)

		// Then composer should be created with defaults
		if composer == nil {
			t.Fatal("Expected composer to be created")
		}
		if composer.runtime != mocks.Runtime {
			t.Error("Expected runtime to be set")
		}
		if composer.commonSubstitutions == nil {
			t.Error("Expected commonSubstitutions to be initialized")
		}
	})

	t.Run("CreatesComposerWithEmptySubstitutions", func(t *testing.T) {
		// Given a composer
		mocks := setupComposerMocks(t)

		// When creating a composer
		composer := NewBlueprintComposer(mocks.Runtime)

		// Then composer should have empty substitutions map
		if composer.commonSubstitutions == nil {
			t.Error("Expected substitutions map to be initialized")
		}
		if len(composer.commonSubstitutions) != 0 {
			t.Error("Expected substitutions map to be empty")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestComposer_Compose(t *testing.T) {
	t.Run("ReturnsEmptyBlueprintWhenNoBlueprintLoaders", func(t *testing.T) {
		// Given a composer with no loaders
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		// When composing
		result, err := composer.Compose(nil, nil, "", nil)

		// Then should return empty blueprint
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil {
			t.Fatal("Expected non-nil blueprint")
		}
	})

	t.Run("MergesTemplateSourceWhenUserBlueprintReferencesIt", func(t *testing.T) {
		// Given template and user loaders, with user referencing template
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		templateBp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "primary"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
			},
		}
		trueVal := true
		userBp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "user"},
			Sources: []blueprintv1alpha1.Source{
				{Name: "template", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("template", templateBp),
			createMockBlueprintLoader("user", userBp),
		}

		// When composing
		result, err := composer.Compose(loaders, nil, "", nil)

		// Then should merge template
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(result.TerraformComponents))
		}
	})

	t.Run("MergesSourcesInOrderFromUserBlueprint", func(t *testing.T) {
		// Given multiple sources and user blueprint specifying order
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		trueVal := true
		templateBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Source: "external"},
			},
		}
		externalBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"region": "us-east-1"}},
				{Path: "rds"},
			},
		}
		userBp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "user"},
			Sources: []blueprintv1alpha1.Source{
				{Name: "external", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
				{Name: "template", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("template", templateBp),
			createMockBlueprintLoader("external", externalBp),
			createMockBlueprintLoader("user", userBp),
		}

		// When composing
		result, err := composer.Compose(loaders, nil, "", nil)

		// Then source components should be merged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) < 1 {
			t.Fatal("Expected at least 1 terraform component")
		}
	})

	t.Run("MergesSourcesWithInstallTrue", func(t *testing.T) {
		// Given a user blueprint with sources that have install:true
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		trueVal := true
		templateBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
			},
		}
		coreSourceBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/eks"},
				{Path: "database/rds"},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "observability"},
			},
		}
		userBp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "user"},
			Sources: []blueprintv1alpha1.Source{
				{
					Name:    "core",
					Url:     "oci://example.com/core:latest",
					Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false},
				},
				{Name: "template", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("template", templateBp),
			createMockBlueprintLoader("core", coreSourceBp),
			createMockBlueprintLoader("user", userBp),
		}

		// When composing
		result, err := composer.Compose(loaders, nil, "", nil)

		// Then core source components should be merged into result
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 3 {
			t.Errorf("Expected 3 terraform components (vpc + eks + rds), got %d", len(result.TerraformComponents))
		}
		if len(result.Kustomizations) != 1 {
			t.Errorf("Expected 1 kustomization, got %d", len(result.Kustomizations))
		}
		componentPaths := make(map[string]bool)
		for _, comp := range result.TerraformComponents {
			componentPaths[comp.Path] = true
		}
		if !componentPaths["vpc"] {
			t.Error("Expected 'vpc' component from template")
		}
		if !componentPaths["cluster/eks"] {
			t.Error("Expected 'cluster/eks' component from source")
		}
		if !componentPaths["database/rds"] {
			t.Error("Expected 'database/rds' component from source")
		}
	})

	t.Run("ReferencingBlueprintComponentsComposeAfterSourceComponents", func(t *testing.T) {
		// Given a core source contributing two addon kustomizations and a referencing (user)
		// blueprint contributing its own addon, with components stamped by source as the real
		// composition flow stamps them (named source -> its name; primary/user -> "").
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		trueVal := true

		coreBp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "core"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "addon-object-store", Path: "addons/object-store", Source: "core"},
				{Name: "addon-private-ca", Path: "addons/private-ca", Source: "core"},
			},
		}
		userBp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "user"},
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "addon-omni", Path: "addons/omni"},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("core", coreBp),
			createMockBlueprintLoader("user", userBp),
		}

		// When composing
		result, err := composer.Compose(loaders, nil, "", nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the referencing blueprint's addon-omni composes after both of core's addons,
		// rather than sorting alphabetically into the middle of them.
		order := make([]string, 0, len(result.Kustomizations))
		for _, k := range result.Kustomizations {
			order = append(order, k.Name)
		}
		idx := func(name string) int {
			for i, n := range order {
				if n == name {
					return i
				}
			}
			return -1
		}
		if idx("addon-omni") < idx("addon-object-store") || idx("addon-omni") < idx("addon-private-ca") {
			t.Errorf("expected addon-omni (referencing blueprint) after core's addons, got order %v", order)
		}
	})

	t.Run("SourceDepthReorderPreservesCrossSourceDependency", func(t *testing.T) {
		// Given a referencing blueprint whose kustomization depends on a source kustomization, the
		// depth reorder must keep the dependency (source, shallower) ahead of its dependent.
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		trueVal := true

		coreBp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "core"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "addon-base", Path: "addons/base", Source: "core"},
			},
		}
		userBp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "user"},
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "addon-extension", Path: "addons/extension", DependsOn: []string{"addon-base"}},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("core", coreBp),
			createMockBlueprintLoader("user", userBp),
		}

		// When composing
		result, err := composer.Compose(loaders, nil, "", nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the dependency (addon-base) still precedes its dependent (addon-extension)
		order := make([]string, 0, len(result.Kustomizations))
		for _, k := range result.Kustomizations {
			order = append(order, k.Name)
		}
		baseIdx, extIdx := -1, -1
		for i, n := range order {
			if n == "addon-base" {
				baseIdx = i
			}
			if n == "addon-extension" {
				extIdx = i
			}
		}
		if baseIdx < 0 || extIdx < 0 || baseIdx > extIdx {
			t.Errorf("expected addon-base before addon-extension, got order %v", order)
		}
	})

	t.Run("SourceDepthReorderIsDeterministicWithCyclicSources", func(t *testing.T) {
		// Given two sources that reference each other, depth resolution must terminate and settle on
		// the same answer regardless of which node the walk reaches first. Cycles are rejected upstream
		// by loader ordering; this guards the depth walk itself against an order-dependent result.
		mocks := setupComposerMocks(t)
		trueVal := true
		install := func(name string) blueprintv1alpha1.Source {
			return blueprintv1alpha1.Source{Name: name, Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}}
		}

		compose := func() []string {
			composer := NewBlueprintComposer(mocks.Runtime)
			aBp := &blueprintv1alpha1.Blueprint{
				Metadata:       blueprintv1alpha1.Metadata{Name: "a"},
				Sources:        []blueprintv1alpha1.Source{install("b")},
				Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "addon-a", Path: "addons/a", Source: "a"}},
			}
			bBp := &blueprintv1alpha1.Blueprint{
				Metadata:       blueprintv1alpha1.Metadata{Name: "b"},
				Sources:        []blueprintv1alpha1.Source{install("a")},
				Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "addon-b", Path: "addons/b", Source: "b"}},
			}
			userBp := &blueprintv1alpha1.Blueprint{
				Metadata:       blueprintv1alpha1.Metadata{Name: "user"},
				Sources:        []blueprintv1alpha1.Source{install("a"), install("b")},
				Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "addon-user", Path: "addons/user"}},
			}
			loaders := []BlueprintLoader{
				createMockBlueprintLoader("a", aBp),
				createMockBlueprintLoader("b", bBp),
				createMockBlueprintLoader("user", userBp),
			}
			result, err := composer.Compose(loaders, nil, "", nil)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			names := make([]string, 0, len(result.Kustomizations))
			for _, k := range result.Kustomizations {
				names = append(names, k.Name)
			}
			return names
		}

		// When composing the same cyclic graph twice
		first, second := compose(), compose()

		// Then the ordering settles identically, and the referencing blueprint still composes last
		if !slices.Equal(first, second) {
			t.Errorf("expected deterministic order across runs, got %v then %v", first, second)
		}
		if len(first) == 0 || first[len(first)-1] != "addon-user" {
			t.Errorf("expected addon-user (referencing blueprint) last, got %v", first)
		}
	})

	t.Run("DoesNotMergeSourcesWithInstallFalse", func(t *testing.T) {
		// Given a user blueprint with a source that has install:false
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		falseVal := false
		trueVal := true
		templateBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
			},
		}
		referenceSourceBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/eks"},
			},
		}
		userBp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "user"},
			Sources: []blueprintv1alpha1.Source{
				{
					Name:    "reference",
					Url:     "oci://example.com/reference:latest",
					Install: &blueprintv1alpha1.BoolExpression{Value: &falseVal, IsExpr: false},
				},
				{
					Name:    "template",
					Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false},
				},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("template", templateBp),
			createMockBlueprintLoader("reference", referenceSourceBp),
			createMockBlueprintLoader("user", userBp),
		}

		// When composing
		result, err := composer.Compose(loaders, nil, "", nil)

		// Then reference source components should NOT be merged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component (only vpc), got %d", len(result.TerraformComponents))
		}
		if len(result.TerraformComponents) > 0 && result.TerraformComponents[0].Path != "vpc" {
			t.Errorf("Expected 'vpc' component, got '%s'", result.TerraformComponents[0].Path)
		}
	})

	t.Run("ComponentsCanReferenceSourcesWithInstallFalse", func(t *testing.T) {
		// Given a template with a component referencing a source that has install:false
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		falseVal := false
		trueVal := true
		templateBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Source: "reference"},
			},
		}
		referenceSourceBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/eks"},
			},
		}
		userBp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "user"},
			Sources: []blueprintv1alpha1.Source{
				{
					Name:    "reference",
					Url:     "oci://example.com/reference:latest",
					Install: &blueprintv1alpha1.BoolExpression{Value: &falseVal, IsExpr: false},
				},
				{Name: "template", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("template", templateBp),
			createMockBlueprintLoader("reference", referenceSourceBp),
			createMockBlueprintLoader("user", userBp),
		}

		// When composing
		result, err := composer.Compose(loaders, nil, "", nil)

		// Then component should be able to reference the source
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		// Component from template should exist (reference source is not merged due to install:false)
		if len(result.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(result.TerraformComponents))
		}
		if result.TerraformComponents[0].Source != "reference" {
			t.Errorf("Expected component to reference 'reference' source, got '%s'", result.TerraformComponents[0].Source)
		}
		if result.Sources[0].Name != "reference" {
			t.Errorf("Expected source 'reference' in Sources array, got '%s'", result.Sources[0].Name)
		}
		// Reference source's components should NOT be unfurled
		componentPaths := make(map[string]bool)
		for _, comp := range result.TerraformComponents {
			componentPaths[comp.Path] = true
		}
		if componentPaths["cluster/eks"] {
			t.Error("Expected 'cluster/eks' component from reference source to NOT be unfurled")
		}
	})

	t.Run("MergesOCISourcesWhenInstallOmittedForBackwardCompat", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		templateBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
			},
		}
		coreSourceBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/eks"},
			},
		}
		trueVal := true
		userBp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "user"},
			Sources: []blueprintv1alpha1.Source{
				{
					Name: "core",
					Url:  "oci://example.com/core:latest",
				},
				{Name: "template", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("template", templateBp),
			createMockBlueprintLoader("core", coreSourceBp),
			createMockBlueprintLoader("user", userBp),
		}

		result, err := composer.Compose(loaders, nil, "", nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 2 {
			t.Errorf("Expected 2 terraform components (vpc + cluster/eks), got %d", len(result.TerraformComponents))
		}
		componentPaths := make(map[string]bool)
		for _, comp := range result.TerraformComponents {
			componentPaths[comp.Path] = true
		}
		if !componentPaths["vpc"] {
			t.Error("Expected 'vpc' component from template")
		}
		if !componentPaths["cluster/eks"] {
			t.Error("Expected 'cluster/eks' from OCI source when install omitted (backward compat)")
		}
	})

	t.Run("DoesNotMergeOCISourceWhenInstallFalse", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		templateBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
			},
		}
		coreSourceBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/eks"},
			},
		}
		trueVal := true
		falseVal := false
		userBp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "user"},
			Sources: []blueprintv1alpha1.Source{
				{
					Name:    "core",
					Url:     "oci://example.com/core:latest",
					Install: &blueprintv1alpha1.BoolExpression{Value: &falseVal, IsExpr: false},
				},
				{Name: "template", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("template", templateBp),
			createMockBlueprintLoader("core", coreSourceBp),
			createMockBlueprintLoader("user", userBp),
		}

		result, err := composer.Compose(loaders, nil, "", nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component (only vpc), got %d", len(result.TerraformComponents))
		}
		componentPaths := make(map[string]bool)
		for _, comp := range result.TerraformComponents {
			componentPaths[comp.Path] = true
		}
		if !componentPaths["vpc"] {
			t.Error("Expected 'vpc' component from template")
		}
		if componentPaths["cluster/eks"] {
			t.Error("Expected 'cluster/eks' to NOT be merged when install: false")
		}
	})

	t.Run("MergesMultipleSources", func(t *testing.T) {
		// Given multiple sources in user blueprint
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		trueVal := true
		coreBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/eks"},
			},
		}
		extrasBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "database/rds"},
			},
		}
		userBp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "user"},
			Sources: []blueprintv1alpha1.Source{
				{
					Name:    "core",
					Url:     "oci://example.com/core:latest",
					Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false},
				},
				{
					Name:    "extras",
					Url:     "oci://example.com/extras:latest",
					Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false},
				},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("core", coreBp),
			createMockBlueprintLoader("extras", extrasBp),
			createMockBlueprintLoader("user", userBp),
		}

		// When composing
		result, err := composer.Compose(loaders, nil, "", nil)

		// Then both sources should be merged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 2 {
			t.Errorf("Expected 2 terraform components, got %d", len(result.TerraformComponents))
		}
		componentPaths := make(map[string]bool)
		for _, comp := range result.TerraformComponents {
			componentPaths[comp.Path] = true
		}
		if !componentPaths["cluster/eks"] {
			t.Error("Expected 'cluster/eks' component from core source")
		}
		if !componentPaths["database/rds"] {
			t.Error("Expected 'database/rds' component from extras source")
		}
	})

	t.Run("UserOverridesSources", func(t *testing.T) {
		// Given template and user loaders with user overriding template values
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		templateBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"region": "us-east-1"}},
			},
		}
		trueVal := true
		userBp := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "template", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
			},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"region": "us-west-2"}},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("template", templateBp),
			createMockBlueprintLoader("user", userBp),
		}

		// When composing
		result, err := composer.Compose(loaders, nil, "", nil)

		// Then user values should override
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(result.TerraformComponents))
		}
		if result.TerraformComponents[0].Inputs["region"] != "us-west-2" {
			t.Errorf("Expected region='us-west-2', got '%v'", result.TerraformComponents[0].Inputs["region"])
		}
	})

	t.Run("ComponentIdentifiedByNameOverPath", func(t *testing.T) {
		// Given components with names
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		templateBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "main-vpc", Path: "network/vpc", Inputs: map[string]any{"cidr": "10.0.0.0/16"}},
			},
		}
		userBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "main-vpc", Inputs: map[string]any{"cidr": "10.1.0.0/16"}},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("template", templateBp),
			createMockBlueprintLoader("user", userBp),
		}

		// When composing
		result, err := composer.Compose(loaders, nil, "", nil)

		// Then should match by name and merge
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(result.TerraformComponents))
		}
		if result.TerraformComponents[0].Inputs["cidr"] != "10.1.0.0/16" {
			t.Errorf("Expected cidr='10.1.0.0/16', got '%v'", result.TerraformComponents[0].Inputs["cidr"])
		}
	})

	t.Run("AddsTemplateSourceWhenTemplateRootExists", func(t *testing.T) {
		// Given a template loader and a template root that the shims report as present
		mocks := setupComposerMocks(t)
		mocks.Runtime.TemplateRoot = filepath.Join(mocks.Runtime.ProjectRoot, "missing-template")
		composer := NewBlueprintComposer(mocks.Runtime)
		composer.shims.Stat = func(name string) (os.FileInfo, error) {
			if name == mocks.Runtime.TemplateRoot {
				return nil, nil
			}
			return os.Stat(name)
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("template", &blueprintv1alpha1.Blueprint{}),
		}

		// When composing
		result, err := composer.Compose(loaders, nil, "", nil)

		// Then the template source should be in the composed blueprint
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.Sources) != 1 || result.Sources[0].Name != "template" {
			t.Errorf("Expected the template source, got %+v", result.Sources)
		}
	})

	t.Run("SkipsTemplateSourceWhenTemplateRootIsMissing", func(t *testing.T) {
		// Given a template loader and a template root that the shims report as missing
		mocks := setupComposerMocks(t)
		mocks.Runtime.TemplateRoot = mocks.Runtime.ProjectRoot
		composer := NewBlueprintComposer(mocks.Runtime)
		composer.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("template", &blueprintv1alpha1.Blueprint{}),
		}

		// When composing
		result, err := composer.Compose(loaders, nil, "", nil)

		// Then the template source should not be in the composed blueprint
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.Sources) != 0 {
			t.Errorf("Expected no sources, got %+v", result.Sources)
		}
	})

	t.Run("SkipsBlueprintLoadersWithNilBlueprint", func(t *testing.T) {
		// Given loaders where one has nil blueprint
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		templateBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
			},
		}
		trueVal := true
		userBp := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "template", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
				{Name: "empty-source"},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("template", templateBp),
			createMockBlueprintLoader("empty-source", nil),
			createMockBlueprintLoader("user", userBp),
		}

		// When composing
		result, err := composer.Compose(loaders, nil, "", nil)

		// Then should still compose successfully with template merged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 1 {
			t.Errorf("Expected 1 component from template, got %d", len(result.TerraformComponents))
		}
	})

	t.Run("CreatesValuesCommonWithLegacyVariables", func(t *testing.T) {
		// Given a composer with legacy config values
		mocks := setupComposerMocks(t)
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "dns.domain":
				return "example.com"
			case "id":
				return "test-id"
			case "network.loadbalancer_ips.start":
				return "10.0.0.1"
			case "network.loadbalancer_ips.end":
				return "10.0.0.10"
			case "docker.registry_url":
				return "registry.example.com"
			default:
				return ""
			}
		}
		mocks.ConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mocks.ConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			if key == "cluster.workers.volumes" {
				return []string{"/host:/container"}
			}
			return []string{}
		}
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		composer := NewBlueprintComposer(mocks.Runtime)

		templateBp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "primary"},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("template", templateBp),
		}

		// When composing
		result, err := composer.Compose(loaders, nil, "", nil)

		// Then values-common ConfigMap should be created with legacy variables
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result.ConfigMaps == nil {
			t.Fatal("Expected ConfigMaps to be initialized")
		}
		common, exists := result.ConfigMaps["values-common"]
		if !exists {
			t.Fatal("Expected 'values-common' ConfigMap to exist")
		}
		if common["DOMAIN"] != "example.com" {
			t.Errorf("Expected DOMAIN='example.com', got '%s'", common["DOMAIN"])
		}
		if common["CONTEXT"] != "test-context" {
			t.Errorf("Expected CONTEXT='test-context', got '%s'", common["CONTEXT"])
		}
		if common["CONTEXT_ID"] != "test-id" {
			t.Errorf("Expected CONTEXT_ID='test-id', got '%s'", common["CONTEXT_ID"])
		}
		if common["LOADBALANCER_IP_RANGE"] != "10.0.0.1-10.0.0.10" {
			t.Errorf("Expected LOADBALANCER_IP_RANGE='10.0.0.1-10.0.0.10', got '%s'", common["LOADBALANCER_IP_RANGE"])
		}
		if common["REGISTRY_URL"] != "registry.example.com" {
			t.Errorf("Expected REGISTRY_URL='registry.example.com', got '%s'", common["REGISTRY_URL"])
		}
		if common["LOCAL_VOLUME_PATH"] != "/container" {
			t.Errorf("Expected LOCAL_VOLUME_PATH='/container', got '%s'", common["LOCAL_VOLUME_PATH"])
		}
		// BUILD_ID may be empty if no build-id file exists, which is fine
	})
}

func TestComposer_SetCommonSubstitutions(t *testing.T) {
	t.Run("SetsCommonSubstitutions", func(t *testing.T) {
		// Given a composer
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		// When setting common substitutions
		subs := map[string]string{"domain": "example.com"}
		composer.SetCommonSubstitutions(subs)

		// Then substitutions should be set
		if composer.commonSubstitutions["domain"] != "example.com" {
			t.Errorf("Expected domain='example.com', got '%s'", composer.commonSubstitutions["domain"])
		}
	})
}

func TestComposer_FilterToUserSelection(t *testing.T) {
	t.Run("UserBlueprintDoesNotFilterKustomizations", func(t *testing.T) {
		// Given template with multiple kustomizations and user overriding one
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		templateBp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "flux-system", Path: "kustomize/flux-system"},
				{Name: "cert-manager", Path: "kustomize/cert-manager"},
				{Name: "ingress-nginx", Path: "kustomize/ingress-nginx"},
			},
		}
		trueVal := true
		userBp := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "template", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "flux-system", Path: "kustomize/flux-system-custom"},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("template", templateBp),
			createMockBlueprintLoader("user", userBp),
		}

		// When composing
		result, err := composer.Compose(loaders, nil, "", nil)

		// Then all kustomizations should remain (no filtering), with user override applied
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.Kustomizations) != 3 {
			t.Fatalf("Expected 3 kustomizations (no filtering), got %d", len(result.Kustomizations))
		}
		kustMap := make(map[string]blueprintv1alpha1.Kustomization)
		for _, k := range result.Kustomizations {
			kustMap[k.Name] = k
		}
		if flux, exists := kustMap["flux-system"]; !exists {
			t.Error("Expected 'flux-system' kustomization to exist")
		} else if flux.Path != "kustomize/flux-system-custom" {
			t.Errorf("Expected flux-system path to be overridden, got '%s'", flux.Path)
		}
		if _, exists := kustMap["cert-manager"]; !exists {
			t.Error("Expected 'cert-manager' kustomization to remain")
		}
		if _, exists := kustMap["ingress-nginx"]; !exists {
			t.Error("Expected 'ingress-nginx' kustomization to remain")
		}
	})

	t.Run("DoesNotFilterWhenUserHasNoComponents", func(t *testing.T) {
		// Given template with components and user with only sources
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		templateBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "rds"},
			},
		}
		trueVal := true
		userBp := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "template", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("template", templateBp),
			createMockBlueprintLoader("user", userBp),
		}

		// When composing
		result, err := composer.Compose(loaders, nil, "", nil)

		// Then all components should remain (no filtering)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 2 {
			t.Fatalf("Expected 2 terraform components (unfiltered), got %d", len(result.TerraformComponents))
		}
	})

	t.Run("UserBlueprintOverridesBothTerraformAndKustomizations", func(t *testing.T) {
		// Given template with both terraform and kustomizations
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		templateBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"region": "us-east-1"}},
				{Path: "rds", Inputs: map[string]any{"instance": "db.t3.medium"}},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "flux-system", Path: "kustomize/flux-system"},
				{Name: "cert-manager", Path: "kustomize/cert-manager"},
			},
		}
		trueVal := true
		userBp := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "template", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
			},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"region": "us-west-2"}},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "flux-system", Path: "kustomize/flux-system-custom"},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("template", templateBp),
			createMockBlueprintLoader("user", userBp),
		}

		// When composing
		result, err := composer.Compose(loaders, nil, "", nil)

		// Then all components should remain (no filtering), with user overrides applied
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 2 {
			t.Fatalf("Expected 2 terraform components (no filtering), got %d", len(result.TerraformComponents))
		}
		if len(result.Kustomizations) != 2 {
			t.Fatalf("Expected 2 kustomizations (no filtering), got %d", len(result.Kustomizations))
		}
		componentMap := make(map[string]blueprintv1alpha1.TerraformComponent)
		for _, comp := range result.TerraformComponents {
			componentMap[comp.Path] = comp
		}
		if vpc, exists := componentMap["vpc"]; !exists {
			t.Error("Expected 'vpc' component to exist")
		} else if vpc.Inputs["region"] != "us-west-2" {
			t.Errorf("Expected vpc region to be overridden, got %v", vpc.Inputs["region"])
		}
		if _, exists := componentMap["rds"]; !exists {
			t.Error("Expected 'rds' component to remain")
		}
		kustMap := make(map[string]blueprintv1alpha1.Kustomization)
		for _, k := range result.Kustomizations {
			kustMap[k.Name] = k
		}
		if flux, exists := kustMap["flux-system"]; !exists {
			t.Error("Expected 'flux-system' kustomization to exist")
		} else if flux.Path != "kustomize/flux-system-custom" {
			t.Errorf("Expected flux-system path to be overridden, got '%s'", flux.Path)
		}
		if _, exists := kustMap["cert-manager"]; !exists {
			t.Error("Expected 'cert-manager' kustomization to remain")
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestComposer_applyUserBlueprint(t *testing.T) {
	t.Run("ClearsRepositoryWhenUserBlueprintHasNone", func(t *testing.T) {
		// Given a composed blueprint with repository but user blueprint has none
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		result := &blueprintv1alpha1.Blueprint{
			Repository: blueprintv1alpha1.Repository{
				Url: "http://git.test/git/project",
			},
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Url: "github.com/windsorcli/core"},
			},
		}
		user := &blueprintv1alpha1.Blueprint{}

		// When applying user blueprint
		composer.applyUserBlueprint(result, user, "", nil)

		// Then repository should be cleared, but sources should remain (no filtering)
		if result.Repository.Url != "" {
			t.Errorf("Expected empty repository URL, got '%s'", result.Repository.Url)
		}
		if len(result.Sources) != 1 {
			t.Errorf("Expected 1 source (no filtering), got %d", len(result.Sources))
		}
	})

	t.Run("PreservesRepositoryWhenUserBlueprintDefinesIt", func(t *testing.T) {
		// Given a user blueprint that defines repository
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		result := &blueprintv1alpha1.Blueprint{
			Repository: blueprintv1alpha1.Repository{
				Url: "http://primary.test/repo",
			},
		}
		user := &blueprintv1alpha1.Blueprint{
			Repository: blueprintv1alpha1.Repository{
				Url: "http://user.test/repo",
			},
		}

		// When applying user authority
		composer.applyUserBlueprint(result, user, "", nil)

		// Then repository should be merged (user overrides)
		if result.Repository.Url != "http://user.test/repo" {
			t.Errorf("Expected user repository URL, got '%s'", result.Repository.Url)
		}
	})

	t.Run("UserBlueprintOverridesSourcesWithoutFiltering", func(t *testing.T) {
		// Given a user blueprint that overrides a source
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		result := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Url: "github.com/windsorcli/core"},
				{Name: "extras", Url: "github.com/windsorcli/extras"},
			},
		}
		user := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Url: "github.com/user/core-fork"},
			},
		}

		// When applying user blueprint
		composer.applyUserBlueprint(result, user, "", nil)

		// Then all sources should remain (no filtering), with user's override applied
		if len(result.Sources) != 2 {
			t.Errorf("Expected 2 sources (no filtering), got %d", len(result.Sources))
		}
		sourceMap := make(map[string]blueprintv1alpha1.Source)
		for _, s := range result.Sources {
			sourceMap[s.Name] = s
		}
		if core, exists := sourceMap["core"]; !exists {
			t.Error("Expected source 'core' to exist")
		} else if core.Url != "github.com/user/core-fork" {
			t.Errorf("Expected core URL to be overridden, got '%s'", core.Url)
		}
		if _, exists := sourceMap["extras"]; !exists {
			t.Error("Expected source 'extras' to remain")
		}
	})

	t.Run("DoesNothingWhenUserIsNil", func(t *testing.T) {
		// Given no user blueprint
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		result := &blueprintv1alpha1.Blueprint{
			Repository: blueprintv1alpha1.Repository{
				Url: "http://git.test/git/project",
			},
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Url: "github.com/windsorcli/core"},
			},
		}

		// When applying user authority with nil user
		composer.applyUserBlueprint(result, nil, "", nil)

		// Then result should remain unchanged
		if result.Repository.Url != "http://git.test/git/project" {
			t.Errorf("Expected repository preserved, got '%s'", result.Repository.Url)
		}
		if len(result.Sources) != 1 {
			t.Errorf("Expected 1 source, got %d", len(result.Sources))
		}
	})

	t.Run("UserBlueprintOverridesWithoutFiltering", func(t *testing.T) {
		// Given a user blueprint that overrides a component
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		result := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "network/vpc", Inputs: map[string]any{"region": "us-east-1"}},
				{Path: "cluster/eks", Inputs: map[string]any{"version": "1.28"}},
				{Path: "database/rds", Inputs: map[string]any{"instance": "db.t3.medium"}},
			},
		}
		user := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/eks", Inputs: map[string]any{"version": "1.29"}},
			},
		}

		// When applying user blueprint
		composer.applyUserBlueprint(result, user, "", nil)

		// Then all components should remain, with user override applied
		if len(result.TerraformComponents) != 3 {
			t.Errorf("Expected 3 components (no filtering), got %d", len(result.TerraformComponents))
		}
		componentMap := make(map[string]blueprintv1alpha1.TerraformComponent)
		for _, comp := range result.TerraformComponents {
			componentMap[comp.Path] = comp
		}
		if eks, exists := componentMap["cluster/eks"]; !exists {
			t.Error("Expected 'cluster/eks' component to exist")
		} else if eks.Inputs["version"] != "1.29" {
			t.Errorf("Expected version to be overridden to '1.29', got %v", eks.Inputs["version"])
		}
		if _, exists := componentMap["network/vpc"]; !exists {
			t.Error("Expected 'network/vpc' component to remain")
		}
		if _, exists := componentMap["database/rds"]; !exists {
			t.Error("Expected 'database/rds' component to remain")
		}
	})

	t.Run("UserBlueprintCanAddNewComponents", func(t *testing.T) {
		// Given a user blueprint that adds a new component
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		result := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "network/vpc"},
			},
		}
		user := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/eks"},
			},
		}

		// When applying user blueprint
		composer.applyUserBlueprint(result, user, "", nil)

		// Then new component should be added
		if len(result.TerraformComponents) != 2 {
			t.Errorf("Expected 2 components, got %d", len(result.TerraformComponents))
		}
		componentPaths := make(map[string]bool)
		for _, comp := range result.TerraformComponents {
			componentPaths[comp.Path] = true
		}
		if !componentPaths["network/vpc"] {
			t.Error("Expected 'network/vpc' component to remain")
		}
		if !componentPaths["cluster/eks"] {
			t.Error("Expected 'cluster/eks' component to be added")
		}
	})

	t.Run("EvaluatesUserTerraformInputsWhenRuntimeHasEvaluator", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"provider": "aws", "cluster": map[string]any{"name": "my-cluster"}}, nil
		}
		rt := mocks.Runtime
		rt.Evaluator = evaluator.NewExpressionEvaluator(rt.ConfigHandler, rt.ProjectRoot, rt.TemplateRoot)
		composer := NewBlueprintComposer(rt)

		result := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "network/vpc", Inputs: map[string]any{"region": "us-east-1"}},
			},
		}
		user := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/eks", Inputs: map[string]any{"provider_ref": "${provider}", "cluster_ref": "${cluster.name}"}},
			},
		}

		err := composer.applyUserBlueprint(result, user, "", nil)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		componentMap := make(map[string]blueprintv1alpha1.TerraformComponent)
		for _, comp := range result.TerraformComponents {
			componentMap[comp.Path] = comp
		}
		eks, ok := componentMap["cluster/eks"]
		if !ok {
			t.Fatal("Expected 'cluster/eks' component after merge")
		}
		if eks.Inputs["provider_ref"] != "aws" {
			t.Errorf("Expected provider_ref to be evaluated to 'aws', got %v", eks.Inputs["provider_ref"])
		}
		if eks.Inputs["cluster_ref"] != "my-cluster" {
			t.Errorf("Expected cluster_ref to be evaluated to 'my-cluster', got %v", eks.Inputs["cluster_ref"])
		}
	})

	t.Run("ErrorOnEvaluateMapFailure", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		mockEval := evaluator.NewMockExpressionEvaluator()
		mockEval.EvaluateMapFunc = func(values map[string]any, facetPath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			return nil, errors.New("evaluate map failed")
		}
		mocks.Runtime.Evaluator = mockEval
		composer := NewBlueprintComposer(mocks.Runtime)

		result := &blueprintv1alpha1.Blueprint{}
		user := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/eks", Inputs: map[string]any{"key": "value"}},
			},
		}

		err := composer.applyUserBlueprint(result, user, "", nil)

		if err == nil {
			t.Error("Expected error when EvaluateMap fails")
			return
		}
		if !strings.Contains(err.Error(), "evaluate user blueprint terraform inputs") {
			t.Errorf("Expected error to mention terraform inputs, got: %v", err)
		}
	})
}

func TestComposer_dropEmptyCompositionFragments(t *testing.T) {
	t.Run("RemovesEmptyStringFromKustomizationComponents", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app", Components: []string{"a", "", "b", ""}},
			},
		}

		composer.dropEmptyCompositionFragments(blueprint)

		components := blueprint.Kustomizations[0].Components
		if len(components) != 2 {
			t.Errorf("Expected 2 components, got %d: %v", len(components), components)
		}
		if components[0] != "a" || components[1] != "b" {
			t.Errorf("Expected [a b], got %v", components)
		}
	})

	t.Run("RemovesEmptyKeyFromKustomizationSubstitutions", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app", Substitutions: map[string]string{"key": "v", "": "placeholder"}},
			},
		}

		composer.dropEmptyCompositionFragments(blueprint)

		subs := blueprint.Kustomizations[0].Substitutions
		if _, hasEmpty := subs[""]; hasEmpty {
			t.Error("Expected empty key to be removed from Substitutions")
		}
		if subs["key"] != "v" {
			t.Errorf("Expected key='v', got %q", subs["key"])
		}
	})

	t.Run("RemovesEmptyKeyFromTerraformComponentInputs", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"region": "us-east-1", "": "placeholder"}},
			},
		}

		composer.dropEmptyCompositionFragments(blueprint)

		inputs := blueprint.TerraformComponents[0].Inputs
		if _, hasEmpty := inputs[""]; hasEmpty {
			t.Error("Expected empty key to be removed from Inputs")
		}
		if inputs["region"] != "us-east-1" {
			t.Errorf("Expected region='us-east-1', got %v", inputs["region"])
		}
	})

	t.Run("HandlesNilBlueprint", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		composer.dropEmptyCompositionFragments(nil)
	})

	t.Run("HandlesNilSubstitutionsAndInputs", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app", Components: []string{"a"}, Substitutions: nil},
			},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: nil},
			},
		}

		composer.dropEmptyCompositionFragments(blueprint)

		if blueprint.Kustomizations[0].Components[0] != "a" {
			t.Errorf("Expected component 'a', got %q", blueprint.Kustomizations[0].Components[0])
		}
	})

	t.Run("RemovesEmptyValueFromKustomizationSubstitutions", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app", Substitutions: map[string]string{"keep": "v", "drop": ""}},
			},
		}

		composer.dropEmptyCompositionFragments(blueprint)

		subs := blueprint.Kustomizations[0].Substitutions
		if _, hasDrop := subs["drop"]; hasDrop {
			t.Error("Expected key with empty value to be removed from Substitutions")
		}
		if subs["keep"] != "v" {
			t.Errorf("Expected keep='v', got %q", subs["keep"])
		}
	})

	t.Run("RecursivelyPrunesNestedEmptyInTerraformInputs", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{
					Path: "compute",
					Inputs: map[string]any{
						"storage_pools": map[string]any{
							"local":    map[string]any{"driver": "dir"},
							"mayastor": map[string]any{"driver": "null", "source": ""},
						},
					},
				},
			},
		}

		composer.dropEmptyCompositionFragments(blueprint)

		storagePools := blueprint.TerraformComponents[0].Inputs["storage_pools"].(map[string]any)
		mayastor := storagePools["mayastor"].(map[string]any)
		if _, hasSource := mayastor["source"]; hasSource {
			t.Error("Expected nested empty value (source: \"\") to be pruned from Inputs")
		}
		if mayastor["driver"] != "null" {
			t.Errorf("Expected driver='null', got %v", mayastor["driver"])
		}
	})

	t.Run("PrunesConfigMapsEmptyKeysAndValues", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			ConfigMaps: map[string]map[string]string{
				"values-common": {"KEEP": "v", "": "empty-key", "DROP_EMPTY": ""},
			},
		}

		composer.dropEmptyCompositionFragments(blueprint)

		common := blueprint.ConfigMaps["values-common"]
		if _, hasEmpty := common[""]; hasEmpty {
			t.Error("Expected empty key to be removed from ConfigMap")
		}
		if _, hasDrop := common["DROP_EMPTY"]; hasDrop {
			t.Error("Expected key with empty value to be removed from ConfigMap")
		}
		if common["KEEP"] != "v" {
			t.Errorf("Expected KEEP='v', got %q", common["KEEP"])
		}
	})

	t.Run("PrunesTopLevelSubstitutionsEmptyKeysAndValues", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Substitutions: map[string]string{
				"keep":       "10.0.0.1",
				"drop_empty": "",
				"":           "empty-key",
			},
		}

		composer.dropEmptyCompositionFragments(blueprint)

		if _, hasEmpty := blueprint.Substitutions[""]; hasEmpty {
			t.Error("Expected empty key to be removed from Substitutions")
		}
		if _, hasDrop := blueprint.Substitutions["drop_empty"]; hasDrop {
			t.Error("Expected key with empty value to be removed from Substitutions")
		}
		if blueprint.Substitutions["keep"] != "10.0.0.1" {
			t.Errorf("Expected keep='10.0.0.1', got %q", blueprint.Substitutions["keep"])
		}
	})
}

func TestComposer_setContextMetadata(t *testing.T) {
	t.Run("SetsMetadataFromContext", func(t *testing.T) {
		// Given a composer with a context name set
		mocks := setupComposerMocks(t)
		mocks.Runtime.ContextName = "production"
		composer := NewBlueprintComposer(mocks.Runtime)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name:        "original",
				Description: "Original description",
			},
		}

		// When setting context metadata
		composer.setContextMetadata(bp)

		// Then metadata should be updated
		if bp.Metadata.Name != "production" {
			t.Errorf("Expected name 'production', got '%s'", bp.Metadata.Name)
		}
		expectedDesc := "Blueprint for the production context"
		if bp.Metadata.Description != expectedDesc {
			t.Errorf("Expected description '%s', got '%s'", expectedDesc, bp.Metadata.Description)
		}
	})

	t.Run("HandlesEmptyContextName", func(t *testing.T) {
		// Given a composer with empty context name
		mocks := setupComposerMocks(t)
		mocks.Runtime.ContextName = ""
		composer := NewBlueprintComposer(mocks.Runtime)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name:        "original",
				Description: "Original description",
			},
		}

		// When setting context metadata
		composer.setContextMetadata(bp)

		// Then metadata should remain unchanged
		if bp.Metadata.Name != "original" {
			t.Errorf("Expected name 'original', got '%s'", bp.Metadata.Name)
		}
	})

	t.Run("HandlesNilRuntime", func(t *testing.T) {
		// Given a composer with nil runtime
		composer := &BaseBlueprintComposer{runtime: nil}
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name:        "original",
				Description: "Original description",
			},
		}

		// When setting context metadata
		composer.setContextMetadata(bp)

		// Then metadata should remain unchanged
		if bp.Metadata.Name != "original" {
			t.Errorf("Expected name 'original', got '%s'", bp.Metadata.Name)
		}
	})
}

func TestComposer_validateSources(t *testing.T) {
	t.Run("ReturnsNilWhenNoInstallSources", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		bp := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "modules", Url: "github.com/org/terraform-modules", Ref: blueprintv1alpha1.Reference{Branch: "main"}},
			},
		}

		err := composer.validateSources(bp)

		if err != nil {
			t.Errorf("Expected no error when no source has install: true, got %v", err)
		}
	})

	t.Run("ReturnsNilWhenOCISourceHasInstallTrue", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		trueVal := true
		bp := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v1.0.0", Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
			},
		}

		err := composer.validateSources(bp)

		if err != nil {
			t.Errorf("Expected no error for OCI source with install: true, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenGitSourceHasInstallTrue", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		trueVal := true
		bp := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "modules", Url: "github.com/org/terraform-modules", Ref: blueprintv1alpha1.Reference{Branch: "main"}, Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false}},
			},
		}

		err := composer.validateSources(bp)

		if err == nil {
			t.Error("Expected error when git source has install: true")
		}
		if !strings.Contains(err.Error(), "install: true") {
			t.Errorf("Expected error to mention install, got: %v", err)
		}
		if !strings.Contains(err.Error(), "modules") {
			t.Errorf("Expected error to mention source name, got: %v", err)
		}
		if !strings.Contains(err.Error(), "OCI") {
			t.Errorf("Expected error to mention OCI, got: %v", err)
		}
	})
}

func TestComposer_validateReservedNames(t *testing.T) {
	t.Run("RejectsUserKustomizationNamedCrds", func(t *testing.T) {
		// Given a user-authored kustomization that claims the reserved CRD-layer name
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		bp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: blueprintv1alpha1.CrdLayerName, Path: "crds"}},
		}

		// When validating
		err := composer.validateReservedNames(bp)

		// Then it is rejected as reserved
		if err == nil || !strings.Contains(err.Error(), "reserved for the CRD layer") {
			t.Errorf("expected reserved-name error, got %v", err)
		}
	})

	t.Run("AllowsTheCrdsSectionItself", func(t *testing.T) {
		// Given the CRD layer expressed through the crds: section (not a kustomization)
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		bp := &blueprintv1alpha1.Blueprint{
			Crds:           []string{"cert-manager-1.16.2"},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "cert-manager", Path: "pki/cert-manager"}},
		}

		// When validating
		if err := composer.validateReservedNames(bp); err != nil {
			t.Errorf("expected no error for crds: section, got %v", err)
		}
	})

	t.Run("RejectsUserKustomizationInCrdsNamespace", func(t *testing.T) {
		// Given a user-authored kustomization that claims a per-source CRD-layer name
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		bp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "crds-core", Path: "crds"}},
		}

		// When validating
		err := composer.validateReservedNames(bp)

		// Then the whole crds-<source> namespace is reserved
		if err == nil || !strings.Contains(err.Error(), "reserved for the CRD layer") {
			t.Errorf("expected reserved-name error for crds-core, got %v", err)
		}
	})
}
