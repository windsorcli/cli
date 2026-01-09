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

		primaryBp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "primary"},
		}
		loaders := []BlueprintLoader{
			createMockBlueprintLoader("primary", primaryBp),
		}

		// When composing
		result, err := composer.Compose(loaders)

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

func TestComposer_applyCommonSubstitutions(t *testing.T) {
	t.Run("CreatesConfigMapFromCommonSubstitutions", func(t *testing.T) {
		// Given a composer with common substitutions set
		mocks := setupComposerMocks(t)
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}
		mocks.ConfigHandler.GetContextFunc = func() string {
			return ""
		}
		mocks.ConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			return []string{}
		}
		composer := NewBlueprintComposer(mocks.Runtime)
		composer.SetCommonSubstitutions(map[string]string{
			"DOMAIN": "example.com",
			"ENV":    "production",
		})
		blueprint := &blueprintv1alpha1.Blueprint{}

		// When applying common substitutions
		composer.applyCommonSubstitutions(blueprint)

		// Then ConfigMap should be created with values-common
		if blueprint.ConfigMaps == nil {
			t.Fatal("Expected ConfigMaps to be initialized")
		}
		common, exists := blueprint.ConfigMaps["values-common"]
		if !exists {
			t.Fatal("Expected 'values-common' ConfigMap to exist")
		}
		if common["DOMAIN"] != "example.com" {
			t.Errorf("Expected DOMAIN='example.com', got '%s'", common["DOMAIN"])
		}
		if common["ENV"] != "production" {
			t.Errorf("Expected ENV='production', got '%s'", common["ENV"])
		}
	})

	t.Run("ExtractsCommonFromValuesYaml", func(t *testing.T) {
		// Given a composer with values.yaml containing substitutions.common
		mocks := setupComposerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"substitutions": map[string]any{
					"common": map[string]any{
						"KEY1": "value1",
						"KEY2": "value2",
					},
				},
			}, nil
		}
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{}

		// When applying common substitutions
		composer.applyCommonSubstitutions(blueprint)

		// Then ConfigMap should contain values from values.yaml
		common := blueprint.ConfigMaps["values-common"]
		if common["KEY1"] != "value1" {
			t.Errorf("Expected KEY1='value1', got '%s'", common["KEY1"])
		}
		if common["KEY2"] != "value2" {
			t.Errorf("Expected KEY2='value2', got '%s'", common["KEY2"])
		}
	})

	t.Run("MergesCommonSubstitutionsAndValuesYaml", func(t *testing.T) {
		// Given a composer with both commonSubstitutions and values.yaml
		mocks := setupComposerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"substitutions": map[string]any{
					"common": map[string]any{
						"FROM_YAML": "yaml-value",
					},
				},
			}, nil
		}
		composer := NewBlueprintComposer(mocks.Runtime)
		composer.SetCommonSubstitutions(map[string]string{
			"FROM_SET": "set-value",
		})
		blueprint := &blueprintv1alpha1.Blueprint{}

		// When applying common substitutions
		composer.applyCommonSubstitutions(blueprint)

		// Then ConfigMap should contain both
		common := blueprint.ConfigMaps["values-common"]
		if common["FROM_YAML"] != "yaml-value" {
			t.Errorf("Expected FROM_YAML='yaml-value', got '%s'", common["FROM_YAML"])
		}
		if common["FROM_SET"] != "set-value" {
			t.Errorf("Expected FROM_SET='set-value', got '%s'", common["FROM_SET"])
		}
	})

	t.Run("SkipsWhenNoCommonValues", func(t *testing.T) {
		// Given a composer with no common substitutions
		mocks := setupComposerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}
		mocks.ConfigHandler.GetContextFunc = func() string {
			return ""
		}
		mocks.ConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			return []string{}
		}
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{}

		// When applying common substitutions
		composer.applyCommonSubstitutions(blueprint)

		// Then ConfigMap should not be created
		if blueprint.ConfigMaps != nil {
			t.Error("Expected ConfigMaps to be nil when no common values")
		}
	})

	t.Run("HandlesNilRuntime", func(t *testing.T) {
		// Given a composer with nil runtime
		composer := &BaseBlueprintComposer{
			runtime:             nil,
			commonSubstitutions: map[string]string{"KEY": "value"},
		}
		blueprint := &blueprintv1alpha1.Blueprint{}

		// When applying common substitutions
		composer.applyCommonSubstitutions(blueprint)

		// Then should still create ConfigMap from commonSubstitutions
		if blueprint.ConfigMaps == nil {
			t.Fatal("Expected ConfigMaps to be initialized")
		}
		common := blueprint.ConfigMaps["values-common"]
		if common["KEY"] != "value" {
			t.Errorf("Expected KEY='value', got '%s'", common["KEY"])
		}
	})

	t.Run("HandlesNilConfigHandler", func(t *testing.T) {
		// Given a composer with nil ConfigHandler
		mocks := setupComposerMocks(t)
		mocks.Runtime.ConfigHandler = nil
		composer := NewBlueprintComposer(mocks.Runtime)
		composer.SetCommonSubstitutions(map[string]string{"KEY": "value"})
		blueprint := &blueprintv1alpha1.Blueprint{}

		// When applying common substitutions
		composer.applyCommonSubstitutions(blueprint)

		// Then should still create ConfigMap from commonSubstitutions
		common := blueprint.ConfigMaps["values-common"]
		if common["KEY"] != "value" {
			t.Errorf("Expected KEY='value', got '%s'", common["KEY"])
		}
	})

	t.Run("HandlesGetContextValuesError", func(t *testing.T) {
		// Given a composer where GetContextValues returns error
		mocks := setupComposerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return nil, os.ErrNotExist
		}
		composer := NewBlueprintComposer(mocks.Runtime)
		composer.SetCommonSubstitutions(map[string]string{"KEY": "value"})
		blueprint := &blueprintv1alpha1.Blueprint{}

		// When applying common substitutions
		composer.applyCommonSubstitutions(blueprint)

		// Then should still create ConfigMap from commonSubstitutions
		common := blueprint.ConfigMaps["values-common"]
		if common["KEY"] != "value" {
			t.Errorf("Expected KEY='value', got '%s'", common["KEY"])
		}
	})

	t.Run("HandlesMissingSubstitutionsKey", func(t *testing.T) {
		// Given values.yaml without substitutions key
		mocks := setupComposerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}
		mocks.ConfigHandler.GetContextFunc = func() string {
			return ""
		}
		mocks.ConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			return []string{}
		}
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{}

		// When applying common substitutions
		composer.applyCommonSubstitutions(blueprint)

		// Then should not create ConfigMap
		if blueprint.ConfigMaps != nil {
			t.Error("Expected ConfigMaps to be nil")
		}
	})

	t.Run("HandlesMissingCommonKey", func(t *testing.T) {
		// Given values.yaml with substitutions but no common key
		mocks := setupComposerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"substitutions": map[string]any{},
			}, nil
		}
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}
		mocks.ConfigHandler.GetContextFunc = func() string {
			return ""
		}
		mocks.ConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			return []string{}
		}
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{}

		// When applying common substitutions
		composer.applyCommonSubstitutions(blueprint)

		// Then should not create ConfigMap
		if blueprint.ConfigMaps != nil {
			t.Error("Expected ConfigMaps to be nil")
		}
	})
}

func TestComposer_mergeLegacySpecialVariables(t *testing.T) {
	t.Run("MergesAllLegacyVariables", func(t *testing.T) {
		// Given a composer with all legacy config values
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
		buildIDDir := os.Getenv("WINDSOR_PROJECT_ROOT")
		if buildIDDir == "" {
			buildIDDir = mocks.Runtime.ProjectRoot
		}
		os.MkdirAll(buildIDDir+"/.windsor", 0755)
		os.WriteFile(buildIDDir+"/.windsor/.build-id", []byte("build-123"), 0644)
		composer := NewBlueprintComposer(mocks.Runtime)
		merged := make(map[string]string)

		// When merging legacy variables
		composer.mergeLegacySpecialVariables(merged)

		// Then all variables should be set
		if merged["DOMAIN"] != "example.com" {
			t.Errorf("Expected DOMAIN='example.com', got '%s'", merged["DOMAIN"])
		}
		if merged["CONTEXT"] != "test-context" {
			t.Errorf("Expected CONTEXT='test-context', got '%s'", merged["CONTEXT"])
		}
		if merged["CONTEXT_ID"] != "test-id" {
			t.Errorf("Expected CONTEXT_ID='test-id', got '%s'", merged["CONTEXT_ID"])
		}
		if merged["LOADBALANCER_IP_RANGE"] != "10.0.0.1-10.0.0.10" {
			t.Errorf("Expected LOADBALANCER_IP_RANGE='10.0.0.1-10.0.0.10', got '%s'", merged["LOADBALANCER_IP_RANGE"])
		}
		if merged["LOADBALANCER_IP_START"] != "10.0.0.1" {
			t.Errorf("Expected LOADBALANCER_IP_START='10.0.0.1', got '%s'", merged["LOADBALANCER_IP_START"])
		}
		if merged["LOADBALANCER_IP_END"] != "10.0.0.10" {
			t.Errorf("Expected LOADBALANCER_IP_END='10.0.0.10', got '%s'", merged["LOADBALANCER_IP_END"])
		}
		if merged["REGISTRY_URL"] != "registry.example.com" {
			t.Errorf("Expected REGISTRY_URL='registry.example.com', got '%s'", merged["REGISTRY_URL"])
		}
		if merged["LOCAL_VOLUME_PATH"] != "/container" {
			t.Errorf("Expected LOCAL_VOLUME_PATH='/container', got '%s'", merged["LOCAL_VOLUME_PATH"])
		}
		if merged["BUILD_ID"] != "build-123" {
			t.Errorf("Expected BUILD_ID='build-123', got '%s'", merged["BUILD_ID"])
		}
	})

	t.Run("SkipsEmptyValues", func(t *testing.T) {
		// Given a composer with empty config values
		mocks := setupComposerMocks(t)
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}
		mocks.ConfigHandler.GetContextFunc = func() string {
			return ""
		}
		mocks.ConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			return []string{}
		}
		composer := NewBlueprintComposer(mocks.Runtime)
		merged := make(map[string]string)

		// When merging legacy variables
		composer.mergeLegacySpecialVariables(merged)

		// Then no variables should be set
		if len(merged) != 0 {
			t.Errorf("Expected empty map, got %d entries", len(merged))
		}
	})

	t.Run("HandlesNilRuntime", func(t *testing.T) {
		// Given a composer with nil runtime
		composer := &BaseBlueprintComposer{runtime: nil}
		merged := make(map[string]string)

		// When merging legacy variables
		composer.mergeLegacySpecialVariables(merged)

		// Then should return early without error
		if len(merged) != 0 {
			t.Errorf("Expected empty map, got %d entries", len(merged))
		}
	})

	t.Run("HandlesNilConfigHandler", func(t *testing.T) {
		// Given a composer with nil ConfigHandler
		mocks := setupComposerMocks(t)
		mocks.Runtime.ConfigHandler = nil
		composer := NewBlueprintComposer(mocks.Runtime)
		merged := make(map[string]string)

		// When merging legacy variables
		composer.mergeLegacySpecialVariables(merged)

		// Then should return early without error
		if len(merged) != 0 {
			t.Errorf("Expected empty map, got %d entries", len(merged))
		}
	})

	t.Run("HandlesLoadBalancerRangeWithDash", func(t *testing.T) {
		// Given a composer with empty load balancer IPs
		mocks := setupComposerMocks(t)
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "network.loadbalancer_ips.start" || key == "network.loadbalancer_ips.end" {
				return ""
			}
			return ""
		}
		composer := NewBlueprintComposer(mocks.Runtime)
		merged := make(map[string]string)

		// When merging legacy variables
		composer.mergeLegacySpecialVariables(merged)

		// Then LOADBALANCER_IP_RANGE should not be set (it would be "-")
		if _, exists := merged["LOADBALANCER_IP_RANGE"]; exists {
			t.Error("Expected LOADBALANCER_IP_RANGE to not be set when range is '-'")
		}
	})

	t.Run("HandlesLocalVolumePathWithoutColon", func(t *testing.T) {
		// Given a composer with volume path without colon
		mocks := setupComposerMocks(t)
		mocks.ConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			if key == "cluster.workers.volumes" {
				return []string{"/host-only"}
			}
			return []string{}
		}
		composer := NewBlueprintComposer(mocks.Runtime)
		merged := make(map[string]string)

		// When merging legacy variables
		composer.mergeLegacySpecialVariables(merged)

		// Then LOCAL_VOLUME_PATH should not be set
		if _, exists := merged["LOCAL_VOLUME_PATH"]; exists {
			t.Error("Expected LOCAL_VOLUME_PATH to not be set when no colon in path")
		}
	})

	t.Run("HandlesMissingBuildID", func(t *testing.T) {
		// Given a composer where build-id file doesn't exist
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		merged := make(map[string]string)

		// When merging legacy variables
		composer.mergeLegacySpecialVariables(merged)

		// Then BUILD_ID should not be set
		if _, exists := merged["BUILD_ID"]; exists {
			t.Error("Expected BUILD_ID to not be set when build-id file doesn't exist")
		}
	})

	t.Run("HandlesEmptyBuildID", func(t *testing.T) {
		// Given a composer where build-id file is empty
		mocks := setupComposerMocks(t)
		buildIDDir := os.Getenv("WINDSOR_PROJECT_ROOT")
		if buildIDDir == "" {
			buildIDDir = mocks.Runtime.ProjectRoot
		}
		os.MkdirAll(buildIDDir+"/.windsor", 0755)
		os.WriteFile(buildIDDir+"/.windsor/.build-id", []byte(""), 0644)
		composer := NewBlueprintComposer(mocks.Runtime)
		merged := make(map[string]string)

		// When merging legacy variables
		composer.mergeLegacySpecialVariables(merged)

		// Then BUILD_ID should not be set
		if _, exists := merged["BUILD_ID"]; exists {
			t.Error("Expected BUILD_ID to not be set when build-id file is empty")
		}
	})
}
