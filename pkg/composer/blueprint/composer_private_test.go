package blueprint

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

	t.Run("KeepsComposedBackendOverASavedUserBackend", func(t *testing.T) {
		// Given facets that declare backend "backend", and a context blueprint.yaml that an
		// older CLI saved with backend "cluster"
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		result := &blueprintv1alpha1.Blueprint{Backend: "backend"}
		user := &blueprintv1alpha1.Blueprint{Backend: "cluster"}
		userPath := filepath.Join(mocks.Runtime.ProjectRoot, "contexts", "local", "blueprint.yaml")

		// When applying the user blueprint
		buf := captureWarnings(t)
		if err := composer.applyUserBlueprint(result, user, userPath, nil); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the facet backend wins
		if result.Backend != "backend" {
			t.Errorf("Expected backend %q, got %q", "backend", result.Backend)
		}

		// And a warning names the ignored value and tells the operator to remove the line
		var event struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(buf.Bytes(), &event); err != nil {
			t.Fatalf("Expected one JSON warning event, got %q: %v", buf.String(), err)
		}
		warning := event.Message
		for _, want := range []string{"ignores backend: cluster, because a source or facet declares it.", "Remove backend: cluster from " + filepath.Join("contexts", "local", "blueprint.yaml") + "."} {
			if !strings.Contains(warning, want) {
				t.Errorf("Expected warning to contain %q, got: %q", want, warning)
			}
		}
	})

	t.Run("DoesNotWarnWhenUserBackendMatchesComposedBackend", func(t *testing.T) {
		// Given a user blueprint whose backend matches the facet backend
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		result := &blueprintv1alpha1.Blueprint{Backend: "backend"}
		user := &blueprintv1alpha1.Blueprint{Backend: "backend"}

		// When applying the user blueprint
		buf := captureWarnings(t)
		if err := composer.applyUserBlueprint(result, user, "", nil); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the backend is unchanged and no warning is printed
		if result.Backend != "backend" {
			t.Errorf("Expected backend %q, got %q", "backend", result.Backend)
		}
		if strings.Contains(buf.String(), "backend") {
			t.Errorf("Expected no warning, got: %q", buf.String())
		}
	})

	t.Run("AppliesUserBackendWhenFacetsDeclareNone", func(t *testing.T) {
		// Given facets that declare no backend, and a user blueprint that names one
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		result := &blueprintv1alpha1.Blueprint{}
		user := &blueprintv1alpha1.Blueprint{Backend: "state"}

		// When applying the user blueprint
		if err := composer.applyUserBlueprint(result, user, "", nil); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the user backend applies
		if result.Backend != "state" {
			t.Errorf("Expected backend %q, got %q", "state", result.Backend)
		}
	})

	t.Run("DoesNotModifyTheUserBlueprintBackend", func(t *testing.T) {
		// Given facets that declare a backend, and a user blueprint that names another
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		result := &blueprintv1alpha1.Blueprint{Backend: "backend"}
		user := &blueprintv1alpha1.Blueprint{Backend: "cluster"}

		// When applying the user blueprint
		if err := composer.applyUserBlueprint(result, user, "", nil); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the loaded user blueprint keeps its own value
		if user.Backend != "cluster" {
			t.Errorf("Expected user backend %q, got %q", "cluster", user.Backend)
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
