package blueprint

import (
	"os"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
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

func (m *mockBlueprintLoaderForComposer) Load() error {
	return nil
}

func (m *mockBlueprintLoaderForComposer) GetBlueprint() *blueprintv1alpha1.Blueprint {
	return m.blueprint
}

func (m *mockBlueprintLoaderForComposer) GetFeatures() []blueprintv1alpha1.Feature {
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

	t.Run("AcceptsCommonSubstitutionsOverride", func(t *testing.T) {
		// Given custom substitutions
		mocks := setupComposerMocks(t)
		customSubs := map[string]string{"key": "value"}

		// When creating a composer with overrides
		composer := NewBlueprintComposer(mocks.Runtime, &BaseBlueprintComposer{commonSubstitutions: customSubs})

		// Then composer should use custom substitutions
		if composer.commonSubstitutions["key"] != "value" {
			t.Error("Expected custom substitutions to be used")
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
		result, err := composer.Compose(nil)

		// Then should return empty blueprint
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil {
			t.Fatal("Expected non-nil blueprint")
		}
	})

	t.Run("ReturnsPrimaryWhenOnlyPrimaryBlueprintLoader", func(t *testing.T) {
		// Given only a primary loader
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		primaryBp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "primary"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("primary", primaryBp),
		}

		// When composing
		result, err := composer.Compose(loaders)

		// Then should return primary
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result.Metadata.Name != "primary" {
			t.Errorf("Expected name='primary', got '%s'", result.Metadata.Name)
		}
		if len(result.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(result.TerraformComponents))
		}
	})

	t.Run("MergesSourceComponentsIntoPrimary", func(t *testing.T) {
		// Given primary and source loaders
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		primaryBp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "primary"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Source: "external"},
			},
		}
		sourceBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"region": "us-east-1"}},
				{Path: "rds"},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("primary", primaryBp),
			createMockBlueprintLoader("external", sourceBp),
		}

		// When composing
		result, err := composer.Compose(loaders)

		// Then source components should be merged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) < 1 {
			t.Fatal("Expected at least 1 terraform component")
		}
	})

	t.Run("UserOverridesPrimary", func(t *testing.T) {
		// Given primary and user loaders
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		primaryBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"region": "us-east-1"}},
			},
		}
		userBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"region": "us-west-2"}},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("primary", primaryBp),
			createMockBlueprintLoader("user", userBp),
		}

		// When composing
		result, err := composer.Compose(loaders)

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

	t.Run("UserFiltersComponents", func(t *testing.T) {
		// Given primary with multiple components and user referencing subset
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		primaryBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "rds"},
				{Path: "eks"},
			},
		}
		userBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("primary", primaryBp),
			createMockBlueprintLoader("user", userBp),
		}

		// When composing
		result, err := composer.Compose(loaders)

		// Then only user-referenced component should remain
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component (filtered), got %d", len(result.TerraformComponents))
		}
		if result.TerraformComponents[0].Path != "vpc" {
			t.Errorf("Expected path='vpc', got '%s'", result.TerraformComponents[0].Path)
		}
	})

	t.Run("ComponentIdentifiedByNameOverPath", func(t *testing.T) {
		// Given components with names
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		primaryBp := &blueprintv1alpha1.Blueprint{
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
			createMockBlueprintLoader("primary", primaryBp),
			createMockBlueprintLoader("user", userBp),
		}

		// When composing
		result, err := composer.Compose(loaders)

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

	t.Run("SkipsBlueprintLoadersWithNilBlueprint", func(t *testing.T) {
		// Given loaders where one has nil blueprint
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		primaryBp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "primary"},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("primary", primaryBp),
			createMockBlueprintLoader("empty-source", nil),
		}

		// When composing
		result, err := composer.Compose(loaders)

		// Then should still compose successfully
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result.Metadata.Name != "primary" {
			t.Errorf("Expected name='primary', got '%s'", result.Metadata.Name)
		}
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
	t.Run("FiltersKustomizationsToUserSelection", func(t *testing.T) {
		// Given primary with multiple kustomizations and user referencing subset
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		primaryBp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "flux-system"},
				{Name: "cert-manager"},
				{Name: "ingress-nginx"},
			},
		}
		userBp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "flux-system"},
				{Name: "ingress-nginx"},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("primary", primaryBp),
			createMockBlueprintLoader("user", userBp),
		}

		// When composing
		result, err := composer.Compose(loaders)

		// Then only user-referenced kustomizations should remain
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.Kustomizations) != 2 {
			t.Fatalf("Expected 2 kustomizations (filtered), got %d", len(result.Kustomizations))
		}
	})

	t.Run("DoesNotFilterWhenUserHasNoComponents", func(t *testing.T) {
		// Given primary with components and user with empty blueprint
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		primaryBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "rds"},
			},
		}
		userBp := &blueprintv1alpha1.Blueprint{}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("primary", primaryBp),
			createMockBlueprintLoader("user", userBp),
		}

		// When composing
		result, err := composer.Compose(loaders)

		// Then all components should remain (no filtering)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 2 {
			t.Fatalf("Expected 2 terraform components (unfiltered), got %d", len(result.TerraformComponents))
		}
	})

	t.Run("FiltersBothTerraformAndKustomizations", func(t *testing.T) {
		// Given primary with both terraform and kustomizations
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		primaryBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "rds"},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "flux-system"},
				{Name: "cert-manager"},
			},
		}
		userBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "flux-system"},
			},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("primary", primaryBp),
			createMockBlueprintLoader("user", userBp),
		}

		// When composing
		result, err := composer.Compose(loaders)

		// Then both should be filtered
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(result.TerraformComponents))
		}
		if len(result.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(result.Kustomizations))
		}
	})
}

func TestComposer_SetContextMetadata(t *testing.T) {
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

func TestComposer_ApplyUserBlueprint(t *testing.T) {
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

		// When applying user authority
		composer.applyUserBlueprint(result, user)

		// Then repository and sources should be cleared
		if result.Repository.Url != "" {
			t.Errorf("Expected empty repository URL, got '%s'", result.Repository.Url)
		}
		if len(result.Sources) != 0 {
			t.Errorf("Expected no sources, got %d", len(result.Sources))
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
		composer.applyUserBlueprint(result, user)

		// Then repository should be merged (user overrides)
		if result.Repository.Url != "http://user.test/repo" {
			t.Errorf("Expected user repository URL, got '%s'", result.Repository.Url)
		}
	})

	t.Run("FiltersSourcesToUserSelection", func(t *testing.T) {
		// Given a user blueprint that selects specific sources
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

		// When applying user authority
		composer.applyUserBlueprint(result, user)

		// Then only user-selected source should remain, with user's URL
		if len(result.Sources) != 1 {
			t.Errorf("Expected 1 source after filtering, got %d", len(result.Sources))
		}
		if result.Sources[0].Name != "core" {
			t.Errorf("Expected source 'core', got '%s'", result.Sources[0].Name)
		}
		if result.Sources[0].Url != "github.com/user/core-fork" {
			t.Errorf("Expected user's URL, got '%s'", result.Sources[0].Url)
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
		composer.applyUserBlueprint(result, nil)

		// Then result should remain unchanged
		if result.Repository.Url != "http://git.test/git/project" {
			t.Errorf("Expected repository preserved, got '%s'", result.Repository.Url)
		}
		if len(result.Sources) != 1 {
			t.Errorf("Expected 1 source, got %d", len(result.Sources))
		}
	})

	t.Run("FiltersComponentsToUserSelection", func(t *testing.T) {
		// Given a user blueprint that selects specific components
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)

		result := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "network/vpc"},
				{Path: "cluster/eks"},
				{Path: "database/rds"},
			},
		}
		user := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/eks"},
			},
		}

		// When applying user authority
		composer.applyUserBlueprint(result, user)

		// Then only selected component should remain
		if len(result.TerraformComponents) != 1 {
			t.Errorf("Expected 1 component, got %d", len(result.TerraformComponents))
		}
		if result.TerraformComponents[0].Path != "cluster/eks" {
			t.Errorf("Expected 'cluster/eks', got '%s'", result.TerraformComponents[0].Path)
		}
	})
}
