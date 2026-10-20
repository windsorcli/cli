package blueprint

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
)

// =============================================================================
// Test Private Methods
// =============================================================================

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

	t.Run("IncludesBlueprintSubstitutionsInValuesCommon", func(t *testing.T) {
		// Given a blueprint with top-level Substitutions (set by the processor from facets)
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Substitutions: map[string]string{
				"private_dns": "10.0.0.1",
				"public_dns":  "8.8.8.8",
			},
		}

		// When applying common substitutions
		composer.applyCommonSubstitutions(blueprint)

		// Then values-common should contain the blueprint-level substitutions
		common := blueprint.ConfigMaps["values-common"]
		if common["private_dns"] != "10.0.0.1" {
			t.Errorf("Expected private_dns='10.0.0.1', got '%s'", common["private_dns"])
		}
		if common["public_dns"] != "8.8.8.8" {
			t.Errorf("Expected public_dns='8.8.8.8', got '%s'", common["public_dns"])
		}
	})

	t.Run("ValuesYamlOverridesBlueprintSubstitutions", func(t *testing.T) {
		// Given a blueprint with Substitutions and values.yaml with a conflicting key
		mocks := setupComposerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"substitutions": map[string]any{
					"common": map[string]any{
						"private_dns": "user-override",
					},
				},
			}, nil
		}
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Substitutions: map[string]string{
				"private_dns": "blueprint-default",
			},
		}

		// When applying common substitutions
		composer.applyCommonSubstitutions(blueprint)

		// Then values.yaml should win over blueprint-level substitutions
		common := blueprint.ConfigMaps["values-common"]
		if common["private_dns"] != "user-override" {
			t.Errorf("Expected values.yaml to override blueprint substitutions, got '%s'", common["private_dns"])
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

func TestComposer_discoverContextPatches(t *testing.T) {
	t.Run("ReturnsNilWhenConfigRootEmpty", func(t *testing.T) {
		// Given a composer with empty ConfigRoot
		mocks := setupComposerMocks(t)
		mocks.Runtime.ConfigRoot = ""
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{}

		// When discovering patches
		err := composer.discoverContextPatches(blueprint)

		// Then should return nil without error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsNilWhenPatchesDirectoryDoesNotExist", func(t *testing.T) {
		// Given a composer with ConfigRoot but no patches directory
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{}

		// When discovering patches
		err := composer.discoverContextPatches(blueprint)

		// Then should return nil without error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("DiscoversStrategicMergePatches", func(t *testing.T) {
		// Given a composer with patches directory containing strategic merge patch
		mocks := setupComposerMocks(t)
		patchesDir := mocks.Runtime.ConfigRoot + "/patches/my-app"
		os.MkdirAll(patchesDir, 0755)
		patchContent := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 5
`
		os.WriteFile(patchesDir+"/increase-replicas.yaml", []byte(patchContent), 0644)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "my-app"},
			},
		}

		// When discovering patches
		err := composer.discoverContextPatches(blueprint)

		// Then patch should be added to kustomization
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(blueprint.Kustomizations[0].Patches) != 1 {
			t.Fatalf("Expected 1 patch, got %d", len(blueprint.Kustomizations[0].Patches))
		}
		if blueprint.Kustomizations[0].Patches[0].Patch != patchContent {
			t.Errorf("Expected patch content to match, got %s", blueprint.Kustomizations[0].Patches[0].Patch)
		}
		if blueprint.Kustomizations[0].Patches[0].Target != nil {
			t.Error("Expected target to be nil for strategic merge patch")
		}
	})

	t.Run("DiscoversJSON6902Patches", func(t *testing.T) {
		// Given a composer with patches directory containing JSON 6902 patch
		mocks := setupComposerMocks(t)
		patchesDir := mocks.Runtime.ConfigRoot + "/patches/my-app"
		os.MkdirAll(patchesDir, 0755)
		patchContent := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  namespace: default
patches:
  - op: replace
    path: /spec/replicas
    value: 5
`
		os.WriteFile(patchesDir+"/json-patch.yaml", []byte(patchContent), 0644)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "my-app"},
			},
		}

		// When discovering patches
		err := composer.discoverContextPatches(blueprint)

		// Then JSON 6902 patch should be added with target selector
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(blueprint.Kustomizations[0].Patches) != 1 {
			t.Fatalf("Expected 1 patch, got %d", len(blueprint.Kustomizations[0].Patches))
		}
		patch := blueprint.Kustomizations[0].Patches[0]
		if patch.Target == nil {
			t.Fatal("Expected target selector for JSON 6902 patch")
		}
		if patch.Target.Kind != "Deployment" {
			t.Errorf("Expected target Kind='Deployment', got '%s'", patch.Target.Kind)
		}
		if patch.Target.Name != "my-app" {
			t.Errorf("Expected target Name='my-app', got '%s'", patch.Target.Name)
		}
		if patch.Target.Namespace != "default" {
			t.Errorf("Expected target Namespace='default', got '%s'", patch.Target.Namespace)
		}
		if patch.Patch == "" {
			t.Error("Expected patch content to be set")
		}
	})

	t.Run("DiscoversMultiplePatchesForSameKustomization", func(t *testing.T) {
		// Given a composer with multiple patches for same kustomization
		mocks := setupComposerMocks(t)
		patchesDir := mocks.Runtime.ConfigRoot + "/patches/my-app"
		os.MkdirAll(patchesDir, 0755)
		patch1 := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 1
`
		patch2 := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 2
`
		patch3 := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 3
`
		os.WriteFile(patchesDir+"/patch1.yaml", []byte(patch1), 0644)
		os.WriteFile(patchesDir+"/patch2.yaml", []byte(patch2), 0644)
		os.WriteFile(patchesDir+"/patch3.yml", []byte(patch3), 0644)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "my-app"},
			},
		}

		// When discovering patches
		err := composer.discoverContextPatches(blueprint)

		// Then all patches should be added
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(blueprint.Kustomizations[0].Patches) != 3 {
			t.Fatalf("Expected 3 patches, got %d", len(blueprint.Kustomizations[0].Patches))
		}
	})

	t.Run("DiscoversPatchesForFluxSystemInstallTierName", func(t *testing.T) {
		// Given patches under the compiled install-tier name for a FluxSystem
		mocks := setupComposerMocks(t)
		patchesDir := mocks.Runtime.ConfigRoot + "/patches/cni-install"
		os.MkdirAll(patchesDir, 0755)
		patchContent := "spec:\n  values:\n    key: val\n"
		os.WriteFile(patchesDir+"/values.yaml", []byte(patchContent), 0644)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name:      "cni",
					Install:   &blueprintv1alpha1.Kustomization{Components: []string{"cilium"}},
					Resources: []blueprintv1alpha1.FluxVariant{{Kustomization: blueprintv1alpha1.Kustomization{}}},
				},
			},
		}

		// When discovering patches
		err := composer.discoverContextPatches(blueprint)

		// Then the patch is attached to the install tier
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(blueprint.FluxSystems[0].Install.Patches) != 1 {
			t.Fatalf("Expected 1 patch on install tier, got %d", len(blueprint.FluxSystems[0].Install.Patches))
		}
	})

	t.Run("DiscoversPatchesForFluxSystemResourcesVariantTierName", func(t *testing.T) {
		// Given patches under a named resources variant's compiled tier name
		mocks := setupComposerMocks(t)
		patchesDir := mocks.Runtime.ConfigRoot + "/patches/cni-resources-l2"
		os.MkdirAll(patchesDir, 0755)
		os.WriteFile(patchesDir+"/values.yaml", []byte("spec:\n  key: val\n"), 0644)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name: "cni",
					Resources: []blueprintv1alpha1.FluxVariant{
						{Kustomization: blueprintv1alpha1.Kustomization{}},
						{Kustomization: blueprintv1alpha1.Kustomization{Name: "l2"}},
					},
				},
			},
		}

		// When discovering patches
		err := composer.discoverContextPatches(blueprint)

		// Then the patch is attached to the matching resources variant only
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(blueprint.FluxSystems[0].Resources[1].Patches) != 1 {
			t.Fatalf("Expected 1 patch on 'l2' resources variant, got %d", len(blueprint.FluxSystems[0].Resources[1].Patches))
		}
		if len(blueprint.FluxSystems[0].Resources[0].Patches) != 0 {
			t.Errorf("Expected unnamed resources variant to receive no patches, got %d", len(blueprint.FluxSystems[0].Resources[0].Patches))
		}
	})

	t.Run("DiscoversPatchesForBareFluxSystemNameResolvingToInstallTier", func(t *testing.T) {
		// Given patches directly under the FluxSystem's bare name, with only an install tier
		mocks := setupComposerMocks(t)
		patchesDir := mocks.Runtime.ConfigRoot + "/patches/cni"
		os.MkdirAll(patchesDir, 0755)
		os.WriteFile(patchesDir+"/values.yaml", []byte("spec:\n  key: val\n"), 0644)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{Name: "cni", Install: &blueprintv1alpha1.Kustomization{Components: []string{"cilium"}}},
			},
		}

		// When discovering patches
		err := composer.discoverContextPatches(blueprint)

		// Then the bare name resolves to the install tier
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(blueprint.FluxSystems[0].Install.Patches) != 1 {
			t.Fatalf("Expected 1 patch on install tier via bare name, got %d", len(blueprint.FluxSystems[0].Install.Patches))
		}
	})

	t.Run("DiscoversPatchesForBareFluxSystemNameResolvingToLoneResourcesVariant", func(t *testing.T) {
		// Given patches directly under the FluxSystem's bare name, with no install tier and
		// exactly one resources variant, so the bare name resolves unambiguously
		mocks := setupComposerMocks(t)
		patchesDir := mocks.Runtime.ConfigRoot + "/patches/cni"
		os.MkdirAll(patchesDir, 0755)
		os.WriteFile(patchesDir+"/values.yaml", []byte("spec:\n  key: val\n"), 0644)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name:      "cni",
					Resources: []blueprintv1alpha1.FluxVariant{{Kustomization: blueprintv1alpha1.Kustomization{}}},
				},
			},
		}

		// When discovering patches
		err := composer.discoverContextPatches(blueprint)

		// Then the bare name resolves to the lone resources variant
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(blueprint.FluxSystems[0].Resources[0].Patches) != 1 {
			t.Fatalf("Expected 1 patch on lone resources variant via bare name, got %d", len(blueprint.FluxSystems[0].Resources[0].Patches))
		}
	})

	t.Run("IgnoresBareFluxSystemNameWhenAmbiguousAcrossMultipleResourcesVariants", func(t *testing.T) {
		// Given patches directly under a FluxSystem's bare name, but with no install tier and
		// more than one resources variant, so the bare name cannot be resolved unambiguously
		mocks := setupComposerMocks(t)
		patchesDir := mocks.Runtime.ConfigRoot + "/patches/cni"
		os.MkdirAll(patchesDir, 0755)
		os.WriteFile(patchesDir+"/values.yaml", []byte("spec:\n  key: val\n"), 0644)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name: "cni",
					Resources: []blueprintv1alpha1.FluxVariant{
						{Kustomization: blueprintv1alpha1.Kustomization{Name: "a"}},
						{Kustomization: blueprintv1alpha1.Kustomization{Name: "b"}},
					},
				},
			},
		}

		// When discovering patches
		err := composer.discoverContextPatches(blueprint)

		// Then no variant receives the patch
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(blueprint.FluxSystems[0].Resources[0].Patches) != 0 || len(blueprint.FluxSystems[0].Resources[1].Patches) != 0 {
			t.Error("Expected ambiguous bare name to be left unmatched")
		}
	})

	t.Run("SkipsHiddenDirectoriesUnderPatchesRoot", func(t *testing.T) {
		// Given a hidden directory directly under patches/, alongside a real one
		mocks := setupComposerMocks(t)
		os.MkdirAll(mocks.Runtime.ConfigRoot+"/patches/.git", 0755)
		os.WriteFile(mocks.Runtime.ConfigRoot+"/patches/.git/HEAD", []byte("ref: refs/heads/main"), 0644)
		patchesDir := mocks.Runtime.ConfigRoot + "/patches/my-app"
		os.MkdirAll(patchesDir, 0755)
		os.WriteFile(patchesDir+"/patch.yaml", []byte("valid: patch"), 0644)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "my-app"},
			},
		}

		// When discovering patches
		buf := captureWarnings(t)
		err := composer.discoverContextPatches(blueprint)

		// Then the hidden directory is skipped with no warning, and the real patch applies
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(blueprint.Kustomizations[0].Patches) != 1 {
			t.Fatalf("Expected 1 patch, got %d", len(blueprint.Kustomizations[0].Patches))
		}
		if strings.Contains(buf.String(), ".git") {
			t.Errorf("Expected no warning for the hidden '.git' directory, got: %q", buf.String())
		}
	})

	t.Run("IgnoresPatchesForNonExistentKustomizationAndWarns", func(t *testing.T) {
		// Given a composer with patches for kustomization that doesn't exist
		mocks := setupComposerMocks(t)
		patchesDir := mocks.Runtime.ConfigRoot + "/patches/non-existent"
		os.MkdirAll(patchesDir, 0755)
		os.WriteFile(patchesDir+"/patch.yaml", []byte("patch"), 0644)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "my-app"},
			},
		}

		// When discovering patches
		buf := captureWarnings(t)
		err := composer.discoverContextPatches(blueprint)

		// Then patches should be ignored, but a warning is printed naming the unmatched
		// directory and the valid names it could have used instead
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(blueprint.Kustomizations[0].Patches) != 0 {
			t.Errorf("Expected 0 patches, got %d", len(blueprint.Kustomizations[0].Patches))
		}
		if !strings.Contains(buf.String(), "non-existent") || !strings.Contains(buf.String(), "ignores the patches") {
			t.Errorf("Expected warning naming the unmatched 'non-existent' directory, got: %q", buf.String())
		}
		if !strings.Contains(buf.String(), "Valid names: my-app") {
			t.Errorf("Expected warning to list 'my-app' as a valid name, got: %q", buf.String())
		}
	})

	t.Run("SkipsNonYamlFiles", func(t *testing.T) {
		// Given a composer with non-YAML files in patches directory
		mocks := setupComposerMocks(t)
		patchesDir := mocks.Runtime.ConfigRoot + "/patches/my-app"
		os.MkdirAll(patchesDir, 0755)
		patchContent := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 5
`
		os.WriteFile(patchesDir+"/patch.yaml", []byte(patchContent), 0644)
		os.WriteFile(patchesDir+"/patch.txt", []byte("not a patch"), 0644)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "my-app"},
			},
		}

		// When discovering patches
		err := composer.discoverContextPatches(blueprint)

		// Then only YAML files should be processed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(blueprint.Kustomizations[0].Patches) != 1 {
			t.Fatalf("Expected 1 patch, got %d", len(blueprint.Kustomizations[0].Patches))
		}
	})

	t.Run("SkipsHiddenFiles", func(t *testing.T) {
		// Given a patches directory containing a malformed hidden file alongside a valid
		// patch. A macOS AppleDouble sidecar (._patch.yaml) is one real-world example.
		mocks := setupComposerMocks(t)
		patchesDir := mocks.Runtime.ConfigRoot + "/patches/my-app"
		os.MkdirAll(patchesDir, 0755)
		os.WriteFile(patchesDir+"/patch.yaml", []byte("valid: patch"), 0644)
		os.WriteFile(patchesDir+"/._patch.yaml", []byte{0x00, 0x05, 0x16, 0x07}, 0644)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "my-app"},
			},
		}

		// When discovering patches
		err := composer.discoverContextPatches(blueprint)

		// Then the hidden sidecar file is skipped rather than failing composition, and the
		// real patch is still applied
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(blueprint.Kustomizations[0].Patches) != 1 {
			t.Fatalf("Expected 1 patch, got %d", len(blueprint.Kustomizations[0].Patches))
		}
	})

	t.Run("FailsCompositionOnInvalidPatchYaml", func(t *testing.T) {
		// Given a composer with an invalid YAML patch file
		mocks := setupComposerMocks(t)
		patchesDir := mocks.Runtime.ConfigRoot + "/patches/my-app"
		os.MkdirAll(patchesDir, 0755)
		os.WriteFile(patchesDir+"/invalid.yaml", []byte("invalid: yaml: content: [unclosed"), 0644)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "my-app"},
			},
		}

		// When discovering patches
		err := composer.discoverContextPatches(blueprint)

		// Then composition fails loudly, naming the file and the parse error, rather than
		// silently applying without the operator's patch
		if err == nil {
			t.Fatal("Expected an error for invalid patch YAML")
		}
		if !strings.Contains(err.Error(), "invalid.yaml") {
			t.Errorf("Expected error to name the invalid file, got: %v", err)
		}
	})

	t.Run("HandlesEmptyPatchesDirectory", func(t *testing.T) {
		// Given a composer with empty patches directory
		mocks := setupComposerMocks(t)
		patchesDir := mocks.Runtime.ConfigRoot + "/patches"
		os.MkdirAll(patchesDir, 0755)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "my-app"},
			},
		}

		// When discovering patches
		err := composer.discoverContextPatches(blueprint)

		// Then should return nil without error
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(blueprint.Kustomizations[0].Patches) != 0 {
			t.Errorf("Expected 0 patches, got %d", len(blueprint.Kustomizations[0].Patches))
		}
	})

	t.Run("FailsCompositionOnReadDirErrorForKustomizationDirectory", func(t *testing.T) {
		// Given a composer with patches directory but an unreadable kustomization subdirectory.
		// The read failure is injected via the ReadDir shim rather than real filesystem
		// permissions, since permission bits don't reliably block directory listing on
		// Windows CI runners.
		mocks := setupComposerMocks(t)
		patchesDir := mocks.Runtime.ConfigRoot + "/patches"
		kustomizationDir := filepath.Join(patchesDir, "my-app")
		os.MkdirAll(kustomizationDir, 0755)

		composer := NewBlueprintComposer(mocks.Runtime)
		originalReadDir := composer.shims.ReadDir
		composer.shims.ReadDir = func(path string) ([]os.DirEntry, error) {
			if filepath.Clean(path) == filepath.Clean(kustomizationDir) {
				return nil, os.ErrPermission
			}
			return originalReadDir(path)
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "my-app"},
			},
		}

		// When discovering patches
		err := composer.discoverContextPatches(blueprint)

		// Then composition fails loudly, naming the unreadable directory. A permissions
		// mistake must never look like "no patches configured".
		if err == nil {
			t.Fatal("Expected an error for unreadable kustomization patches directory")
		}
		if !strings.Contains(err.Error(), "my-app") {
			t.Errorf("Expected error to name the unreadable directory, got: %v", err)
		}
	})

	t.Run("FailsCompositionOnReadFileError", func(t *testing.T) {
		// Given a composer with unreadable patch file
		mocks := setupComposerMocks(t)
		patchesDir := filepath.Join(mocks.Runtime.ConfigRoot, "patches", "my-app")
		os.MkdirAll(patchesDir, 0755)
		patchFile := filepath.Join(patchesDir, "patch.yaml")
		os.WriteFile(patchFile, []byte("valid: patch"), 0644)

		composer := NewBlueprintComposer(mocks.Runtime)
		originalReadFile := composer.shims.ReadFile
		composer.shims.ReadFile = func(path string) ([]byte, error) {
			if filepath.Clean(path) == filepath.Clean(patchFile) {
				return nil, os.ErrPermission
			}
			return originalReadFile(path)
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "my-app"},
			},
		}

		// When discovering patches
		err := composer.discoverContextPatches(blueprint)

		// Then composition fails loudly, naming the unreadable file
		if err == nil {
			t.Fatal("Expected an error for unreadable patch file")
		}
		if !strings.Contains(err.Error(), "patch.yaml") {
			t.Errorf("Expected error to name the unreadable file, got: %v", err)
		}
	})

	t.Run("HandlesNilPatchFromParsePatch", func(t *testing.T) {
		// Given a composer with patch that parsePatch returns nil for
		mocks := setupComposerMocks(t)
		patchesDir := mocks.Runtime.ConfigRoot + "/patches/my-app"
		os.MkdirAll(patchesDir, 0755)
		os.WriteFile(patchesDir+"/patch.yaml", []byte(""), 0644)
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "my-app"},
			},
		}

		// When discovering patches
		err := composer.discoverContextPatches(blueprint)

		// Then should handle nil patch gracefully and not add empty patches
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(blueprint.Kustomizations[0].Patches) != 0 {
			t.Errorf("Expected 0 patches for empty file, got %d", len(blueprint.Kustomizations[0].Patches))
		}
	})
}

func TestComposer_parsePatch(t *testing.T) {
	t.Run("ParsesStrategicMergePatch", func(t *testing.T) {
		// Given a strategic merge patch
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		patchData := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 5
`)

		// When parsing patch
		patch, err := composer.parsePatch(patchData, "patch.yaml")

		// Then should return strategic merge patch
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if patch == nil {
			t.Fatal("Expected non-nil patch")
		}
		if patch.Target != nil {
			t.Error("Expected target to be nil for strategic merge patch")
		}
		if patch.Patch != string(patchData) {
			t.Error("Expected patch content to match input")
		}
	})

	t.Run("ParsesJSON6902PatchWithFullMetadata", func(t *testing.T) {
		// Given a JSON 6902 patch with full metadata
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		patchData := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  namespace: default
patches:
  - op: replace
    path: /spec/replicas
    value: 5
`)

		// When parsing patch
		patch, err := composer.parsePatch(patchData, "patch.yaml")

		// Then should return JSON 6902 patch with target selector
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if patch == nil {
			t.Fatal("Expected non-nil patch")
		}
		if patch.Target == nil {
			t.Fatal("Expected target selector for JSON 6902 patch")
		}
		if patch.Target.Kind != "Deployment" {
			t.Errorf("Expected target Kind='Deployment', got '%s'", patch.Target.Kind)
		}
		if patch.Target.Name != "my-app" {
			t.Errorf("Expected target Name='my-app', got '%s'", patch.Target.Name)
		}
		if patch.Target.Namespace != "default" {
			t.Errorf("Expected target Namespace='default', got '%s'", patch.Target.Namespace)
		}
		if patch.Patch == "" {
			t.Error("Expected patch content to be set")
		}
	})

	t.Run("ParsesJSON6902PatchWithPartialMetadata", func(t *testing.T) {
		// Given a JSON 6902 patch with partial metadata
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		patchData := []byte(`apiVersion: apps/v1
kind: Service
metadata:
  name: my-service
patches:
  - op: add
    path: /spec/type
    value: ClusterIP
`)

		// When parsing patch
		patch, err := composer.parsePatch(patchData, "patch.yaml")

		// Then should return JSON 6902 patch with partial target selector
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if patch == nil {
			t.Fatal("Expected non-nil patch")
		}
		if patch.Target == nil {
			t.Fatal("Expected target selector for JSON 6902 patch")
		}
		if patch.Target.Kind != "Service" {
			t.Errorf("Expected target Kind='Service', got '%s'", patch.Target.Kind)
		}
		if patch.Target.Name != "my-service" {
			t.Errorf("Expected target Name='my-service', got '%s'", patch.Target.Name)
		}
		if patch.Target.Namespace != "" {
			t.Errorf("Expected target Namespace to be empty, got '%s'", patch.Target.Namespace)
		}
	})

	t.Run("ParsesJSON6902PatchWithoutKind", func(t *testing.T) {
		// Given a JSON 6902 patch without kind (should fall back to strategic merge)
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		patchData := []byte(`patches:
  - op: replace
    path: /spec/replicas
    value: 5
`)

		// When parsing patch
		patch, err := composer.parsePatch(patchData, "patch.yaml")

		// Then should treat as strategic merge patch
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if patch == nil {
			t.Fatal("Expected non-nil patch")
		}
		if patch.Target != nil {
			t.Error("Expected target to be nil when kind is missing")
		}
		if patch.Patch != string(patchData) {
			t.Error("Expected patch content to be full YAML when falling back to strategic merge")
		}
	})

	t.Run("ReturnsNilForEmptyData", func(t *testing.T) {
		// Given empty patch data
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		patchData := []byte("")

		// When parsing patch
		patch, err := composer.parsePatch(patchData, "patch.yaml")

		// Then should return nil without error
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if patch != nil {
			t.Error("Expected nil patch for empty data")
		}
	})

	t.Run("ReturnsNilForWhitespaceOnlyData", func(t *testing.T) {
		// Given whitespace-only patch data
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		patchData := []byte("   \n\t  \n  ")

		// When parsing patch
		patch, err := composer.parsePatch(patchData, "patch.yaml")

		// Then should return nil without error
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if patch != nil {
			t.Error("Expected nil patch for whitespace-only data")
		}
	})

	t.Run("ReturnsErrorForInvalidYAML", func(t *testing.T) {
		// Given invalid YAML content
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		patchData := []byte("invalid: yaml: content: [unclosed")

		// When parsing patch
		patch, err := composer.parsePatch(patchData, "patch.yaml")

		// Then should return error
		if err == nil {
			t.Fatal("Expected error for invalid YAML")
		}
		if patch != nil {
			t.Error("Expected nil patch on error")
		}
	})
}

func TestComposer_applyPerKustomizationSubstitutions(t *testing.T) {
	t.Run("ReturnsNilWhenConfigHandlerNil", func(t *testing.T) {
		// Given a composer with nil ConfigHandler
		mocks := setupComposerMocks(t)
		mocks.Runtime.ConfigHandler = nil
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{}

		// When applying per-kustomization substitutions
		err := composer.applyPerKustomizationSubstitutions(blueprint)

		// Then should return nil without error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsNilWhenNoSubstitutionsInValues", func(t *testing.T) {
		// Given a composer with no substitutions in values.yaml
		mocks := setupComposerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{}

		// When applying per-kustomization substitutions
		err := composer.applyPerKustomizationSubstitutions(blueprint)

		// Then should return nil without error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("CreatesConfigMapForKustomizationSubstitutions", func(t *testing.T) {
		// Given a composer with per-kustomization substitutions in values.yaml
		mocks := setupComposerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"substitutions": map[string]any{
					"my-app": map[string]any{
						"REPLICAS": "5",
						"IMAGE":    "nginx:latest",
					},
				},
			}, nil
		}
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "my-app"},
			},
		}

		// When applying per-kustomization substitutions
		err := composer.applyPerKustomizationSubstitutions(blueprint)

		// Then ConfigMap should be created
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if blueprint.ConfigMaps == nil {
			t.Fatal("Expected ConfigMaps to be initialized")
		}
		configMap, exists := blueprint.ConfigMaps["values-my-app"]
		if !exists {
			t.Fatal("Expected 'values-my-app' ConfigMap to exist")
		}
		if configMap["REPLICAS"] != "5" {
			t.Errorf("Expected REPLICAS='5', got '%s'", configMap["REPLICAS"])
		}
		if configMap["IMAGE"] != "nginx:latest" {
			t.Errorf("Expected IMAGE='nginx:latest', got '%s'", configMap["IMAGE"])
		}
	})

	t.Run("AddsSubstitutionsToKustomization", func(t *testing.T) {
		// Given a composer with per-kustomization substitutions
		mocks := setupComposerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"substitutions": map[string]any{
					"my-app": map[string]any{
						"REPLICAS": "5",
					},
				},
			}, nil
		}
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "my-app"},
			},
		}

		// When applying per-kustomization substitutions
		err := composer.applyPerKustomizationSubstitutions(blueprint)

		// Then substitutions should be added to kustomization
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if blueprint.Kustomizations[0].Substitutions == nil {
			t.Fatal("Expected Substitutions to be initialized")
		}
		if blueprint.Kustomizations[0].Substitutions["REPLICAS"] != "5" {
			t.Errorf("Expected REPLICAS='5', got '%s'", blueprint.Kustomizations[0].Substitutions["REPLICAS"])
		}
	})

	t.Run("HandlesMultipleKustomizations", func(t *testing.T) {
		// Given a composer with substitutions for multiple kustomizations
		mocks := setupComposerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"substitutions": map[string]any{
					"app1": map[string]any{
						"KEY1": "value1",
					},
					"app2": map[string]any{
						"KEY2": "value2",
					},
				},
			}, nil
		}
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app1"},
				{Name: "app2"},
			},
		}

		// When applying per-kustomization substitutions
		err := composer.applyPerKustomizationSubstitutions(blueprint)

		// Then ConfigMaps should be created for both
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if blueprint.ConfigMaps["values-app1"]["KEY1"] != "value1" {
			t.Error("Expected values-app1 ConfigMap to contain KEY1")
		}
		if blueprint.ConfigMaps["values-app2"]["KEY2"] != "value2" {
			t.Error("Expected values-app2 ConfigMap to contain KEY2")
		}
		if blueprint.Kustomizations[0].Substitutions["KEY1"] != "value1" {
			t.Error("Expected app1 Substitutions to contain KEY1")
		}
		if blueprint.Kustomizations[1].Substitutions["KEY2"] != "value2" {
			t.Error("Expected app2 Substitutions to contain KEY2")
		}
	})

	t.Run("SkipsKustomizationsWithoutSubstitutions", func(t *testing.T) {
		// Given a composer with substitutions for only one kustomization
		mocks := setupComposerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"substitutions": map[string]any{
					"app1": map[string]any{
						"KEY1": "value1",
					},
				},
			}, nil
		}
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app1"},
				{Name: "app2"},
			},
		}

		// When applying per-kustomization substitutions
		err := composer.applyPerKustomizationSubstitutions(blueprint)

		// Then only app1 should have ConfigMap and substitutions
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := blueprint.ConfigMaps["values-app1"]; !exists {
			t.Error("Expected values-app1 ConfigMap to exist")
		}
		if _, exists := blueprint.ConfigMaps["values-app2"]; exists {
			t.Error("Expected values-app2 ConfigMap to not exist")
		}
		if blueprint.Kustomizations[1].Substitutions != nil {
			t.Error("Expected app2 Substitutions to be nil")
		}
	})

	t.Run("MergesIntoExistingValuesCommonConfigMap", func(t *testing.T) {
		// Given a composer with existing values-common ConfigMap and kustomization named "common"
		mocks := setupComposerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"substitutions": map[string]any{
					"common": map[string]any{
						"CUSTOM_KEY": "custom-value",
					},
				},
			}, nil
		}
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			ConfigMaps: map[string]map[string]string{
				"values-common": {
					"DOMAIN":                "example.com",
					"CONTEXT":               "test-context",
					"LOADBALANCER_IP_RANGE": "10.0.0.1-10.0.0.100",
				},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "common"},
			},
		}

		// When applying per-kustomization substitutions
		err := composer.applyPerKustomizationSubstitutions(blueprint)

		// Then should merge into existing values-common ConfigMap instead of overwriting
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if blueprint.ConfigMaps == nil {
			t.Fatal("Expected ConfigMaps to exist")
		}
		configMap, exists := blueprint.ConfigMaps["values-common"]
		if !exists {
			t.Fatal("Expected 'values-common' ConfigMap to still exist")
		}
		if configMap["DOMAIN"] != "example.com" {
			t.Errorf("Expected DOMAIN='example.com' to be preserved, got '%s'", configMap["DOMAIN"])
		}
		if configMap["CONTEXT"] != "test-context" {
			t.Errorf("Expected CONTEXT='test-context' to be preserved, got '%s'", configMap["CONTEXT"])
		}
		if configMap["LOADBALANCER_IP_RANGE"] != "10.0.0.1-10.0.0.100" {
			t.Errorf("Expected LOADBALANCER_IP_RANGE to be preserved, got '%s'", configMap["LOADBALANCER_IP_RANGE"])
		}
		if configMap["CUSTOM_KEY"] != "custom-value" {
			t.Errorf("Expected CUSTOM_KEY='custom-value' to be merged, got '%s'", configMap["CUSTOM_KEY"])
		}
		if blueprint.Kustomizations[0].Substitutions == nil {
			t.Fatal("Expected Substitutions to be initialized")
		}
		if blueprint.Kustomizations[0].Substitutions["CUSTOM_KEY"] != "custom-value" {
			t.Errorf("Expected CUSTOM_KEY in kustomization Substitutions, got '%s'", blueprint.Kustomizations[0].Substitutions["CUSTOM_KEY"])
		}
	})

	t.Run("HandlesGetContextValuesError", func(t *testing.T) {
		// Given a composer where GetContextValues returns error
		mocks := setupComposerMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return nil, os.ErrNotExist
		}
		composer := NewBlueprintComposer(mocks.Runtime)
		blueprint := &blueprintv1alpha1.Blueprint{}

		// When applying per-kustomization substitutions
		err := composer.applyPerKustomizationSubstitutions(blueprint)

		// Then should return nil without error (error is ignored)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestValidPatchTargetNames(t *testing.T) {
	t.Run("DeduplicatesNameCollidingAcrossBothMaps", func(t *testing.T) {
		// Given a plain kustomization name that collides with a compiled FluxSystem tier name
		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{Name: "myapp", Install: &blueprintv1alpha1.Kustomization{Components: []string{"c"}}},
			},
		}
		kustomizationMap := map[string]*blueprintv1alpha1.Kustomization{
			"myapp-install": {Name: "myapp-install"},
		}
		fluxTierMap := buildFluxTierMap(blueprint)

		// When building the valid-names list
		result := validPatchTargetNames(kustomizationMap, fluxTierMap)

		// Then the colliding name appears exactly once
		if strings.Count(result, "myapp-install") != 1 {
			t.Errorf("Expected 'myapp-install' to appear exactly once, got: %q", result)
		}
	})

	t.Run("ReturnsPlaceholderWhenNoNamesExist", func(t *testing.T) {
		// Given no kustomizations or FluxSystem tiers
		// When building the valid-names list
		result := validPatchTargetNames(nil, nil)

		// Then a placeholder is returned rather than an empty string
		if result != "(none)" {
			t.Errorf("Expected '(none)', got: %q", result)
		}
	})
}

// captureWarnings makes a JSON presenter that writes to the returned buffer the process default
// for the test, and restores the previous default on cleanup.
func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := presenter.Default()
	presenter.SetDefault(presenter.NewJSON(&buf))
	t.Cleanup(func() { presenter.SetDefault(previous) })
	return &buf
}
