package v1alpha1

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/kustomize"
	"github.com/goccy/go-yaml"
	"github.com/windsorcli/cli/pkg/constants"
)

func boolExprPtr(b bool) *BoolExpression {
	return &BoolExpression{Value: &b, IsExpr: false}
}

func intExprPtr(i int) *IntExpression {
	return &IntExpression{Value: &i, IsExpr: false}
}

func TestBlueprint_StrategicMerge(t *testing.T) {
	t.Run("MergesTerraformComponentsStrategically", func(t *testing.T) {
		// Given a base blueprint with terraform components
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Path:      "network/vpc",
					Source:    "core",
					Inputs:    map[string]any{"cidr": "10.0.0.0/16"},
					DependsOn: []string{"backend"},
				},
			},
		}

		// And an overlay with same component (should merge) and new component (should append)
		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Path:      "network/vpc", // Same path+source - should merge
					Source:    "core",
					Inputs:    map[string]any{"enable_dns": true},
					DependsOn: []string{"security"},
				},
				{
					Path:   "cluster/eks", // New component - should append
					Source: "core",
					Inputs: map[string]any{"version": "1.28"},
				},
			},
		}

		// When strategic merging
		base.StrategicMerge(overlay)

		// Then should have 2 components
		if len(base.TerraformComponents) != 2 {
			t.Errorf("Expected 2 terraform components, got %d", len(base.TerraformComponents))
		}

		// And first component should have merged values and dependencies
		vpc := base.TerraformComponents[0]
		if vpc.Path != "network/vpc" {
			t.Errorf("Expected path 'network/vpc', got '%s'", vpc.Path)
		}
		if len(vpc.Inputs) != 2 {
			t.Errorf("Expected 2 inputs, got %d", len(vpc.Inputs))
		}
		if vpc.Inputs["cidr"] != "10.0.0.0/16" {
			t.Errorf("Expected original cidr value preserved")
		}
		if vpc.Inputs["enable_dns"] != true {
			t.Errorf("Expected new enable_dns value added")
		}
		if len(vpc.DependsOn) != 2 {
			t.Errorf("Expected 2 dependencies, got %d", len(vpc.DependsOn))
		}
		if !contains(vpc.DependsOn, "backend") || !contains(vpc.DependsOn, "security") {
			t.Errorf("Expected both backend and security dependencies, got %v", vpc.DependsOn)
		}

		// And second component should be the new one
		eks := base.TerraformComponents[1]
		if eks.Path != "cluster/eks" {
			t.Errorf("Expected path 'cluster/eks', got '%s'", eks.Path)
		}
	})

	t.Run("MergesNamedTerraformComponentsByComponentID", func(t *testing.T) {
		// Given a base blueprint with a named terraform component
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Name:      "cluster",
					Path:      "terraform/cluster",
					Source:    "core",
					Inputs:    map[string]any{"version": "1.28"},
					DependsOn: []string{"network"},
				},
			},
		}

		// And an overlay with same component name (should merge) and new named component (should append)
		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Name:      "cluster",                  // Same name - should merge by componentID
					Path:      "terraform/different-path", // Different path, but name takes precedence
					Source:    "core",
					Inputs:    map[string]any{"node_count": 3},
					DependsOn: []string{"storage"},
				},
				{
					Name:   "network",
					Path:   "terraform/network",
					Source: "core",
					Inputs: map[string]any{"cidr": "10.0.0.0/16"},
				},
			},
		}

		// When strategic merging
		base.StrategicMerge(overlay)

		// Then should have 2 components
		if len(base.TerraformComponents) != 2 {
			t.Errorf("Expected 2 terraform components, got %d", len(base.TerraformComponents))
		}

		// Find components by name (order may change due to dependency sorting)
		var cluster *TerraformComponent
		var network *TerraformComponent
		for i := range base.TerraformComponents {
			if base.TerraformComponents[i].Name == "cluster" {
				cluster = &base.TerraformComponents[i]
			}
			if base.TerraformComponents[i].Name == "network" {
				network = &base.TerraformComponents[i]
			}
		}

		if cluster == nil {
			t.Fatalf("Expected to find cluster component")
		}
		if network == nil {
			t.Fatalf("Expected to find network component")
		}

		// Cluster should have merged values
		if cluster.Path != "terraform/cluster" {
			t.Errorf("Expected original path 'terraform/cluster' to be preserved, got '%s'", cluster.Path)
		}
		if len(cluster.Inputs) != 2 {
			t.Errorf("Expected 2 inputs, got %d", len(cluster.Inputs))
		}
		if cluster.Inputs["version"] != "1.28" {
			t.Errorf("Expected original version value preserved")
		}
		if cluster.Inputs["node_count"] != 3 {
			t.Errorf("Expected new node_count value added")
		}
		if len(cluster.DependsOn) != 2 {
			t.Errorf("Expected 2 dependencies, got %d", len(cluster.DependsOn))
		}
		if !contains(cluster.DependsOn, "network") || !contains(cluster.DependsOn, "storage") {
			t.Errorf("Expected both network and storage dependencies, got %v", cluster.DependsOn)
		}

		// Network should be the new component
		if network.Path != "terraform/network" {
			t.Errorf("Expected network path 'terraform/network', got '%s'", network.Path)
		}

		// Verify dependency order: network should come before cluster
		clusterIndex := -1
		networkIndex := -1
		for i, comp := range base.TerraformComponents {
			if comp.Name == "cluster" {
				clusterIndex = i
			}
			if comp.Name == "network" {
				networkIndex = i
			}
		}
		if networkIndex >= clusterIndex {
			t.Errorf("Expected network (index %d) to come before cluster (index %d) due to dependency", networkIndex, clusterIndex)
		}
	})

	t.Run("MergesNamedComponentWithUnnamedComponentByPath", func(t *testing.T) {
		// Given a base blueprint with an unnamed component
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Path:   "terraform/path",
					Source: "core",
					Inputs: map[string]any{"value": "original"},
				},
			},
		}

		// And an overlay with a named component that has same path
		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Name:   "new-name",
					Path:   "terraform/path",
					Source: "core",
					Inputs: map[string]any{"value": "updated"},
				},
			},
		}

		// When strategic merging
		base.StrategicMerge(overlay)

		// Then should have 2 components (different IDs: path vs name)
		if len(base.TerraformComponents) != 2 {
			t.Errorf("Expected 2 terraform components, got %d", len(base.TerraformComponents))
		}
	})

	t.Run("MergesKustomizationsStrategically", func(t *testing.T) {
		// Given a base blueprint with kustomizations
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:       "ingress",
					Components: []string{"nginx"},
					DependsOn:  []string{"pki"},
				},
			},
		}

		// And an overlay with same kustomization (should merge) and new kustomization (should append)
		overlay := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:       "ingress", // Same name - should merge
					Components: []string{"nginx/tls"},
					DependsOn:  []string{"cert-manager"},
				},
				{
					Name:       "monitoring", // New kustomization - should append
					Components: []string{"prometheus"},
				},
			},
		}

		// When strategic merging
		base.StrategicMerge(overlay)

		// Then should have 2 kustomizations
		if len(base.Kustomizations) != 2 {
			t.Errorf("Expected 2 kustomizations, got %d", len(base.Kustomizations))
		}

		// Components should be ordered by their original order since both have unresolved dependencies
		ingress := base.Kustomizations[0]
		if ingress.Name != "ingress" {
			t.Errorf("Expected name 'ingress' at index 0, got '%s'", ingress.Name)
		}

		// And second kustomization should be monitoring
		monitoring := base.Kustomizations[1]
		if monitoring.Name != "monitoring" {
			t.Errorf("Expected name 'monitoring' at index 1, got '%s'", monitoring.Name)
		}
		if len(ingress.Components) != 2 {
			t.Errorf("Expected 2 components, got %d", len(ingress.Components))
		}
		if !contains(ingress.Components, "nginx") || !contains(ingress.Components, "nginx/tls") {
			t.Errorf("Expected both nginx and nginx/tls components, got %v", ingress.Components)
		}
		if len(ingress.DependsOn) != 2 {
			t.Errorf("Expected 2 dependencies, got %d", len(ingress.DependsOn))
		}
		if !contains(ingress.DependsOn, "pki") || !contains(ingress.DependsOn, "cert-manager") {
			t.Errorf("Expected both pki and cert-manager dependencies, got %v", ingress.DependsOn)
		}

		// Check monitoring component (should have no dependencies)
		if len(monitoring.Components) != 1 {
			t.Errorf("Expected 1 component, got %d", len(monitoring.Components))
		}
		if !contains(monitoring.Components, "prometheus") {
			t.Errorf("Expected prometheus component, got %v", monitoring.Components)
		}
		if len(monitoring.DependsOn) != 0 {
			t.Errorf("Expected no dependencies for monitoring, got %v", monitoring.DependsOn)
		}
	})

	t.Run("MergesKustomizationDestroyOnlyField", func(t *testing.T) {
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:       "cleanup-job",
					Components: []string{"base"},
				},
			},
		}

		destroyOnlyTrue := true
		overlay := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:        "cleanup-job",
					DestroyOnly: &destroyOnlyTrue,
				},
			},
		}

		base.StrategicMerge(overlay)

		if len(base.Kustomizations) != 1 {
			t.Errorf("Expected 1 kustomization, got %d", len(base.Kustomizations))
		}

		kust := base.Kustomizations[0]
		if kust.DestroyOnly == nil || *kust.DestroyOnly != true {
			t.Errorf("Expected DestroyOnly to be true, got %v", kust.DestroyOnly)
		}
		if len(kust.Components) != 1 || kust.Components[0] != "base" {
			t.Errorf("Expected original components preserved, got %v", kust.Components)
		}
	})

	t.Run("MergesKustomizationEnabledField", func(t *testing.T) {
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:       "observability",
					Components: []string{"base"},
				},
			},
		}

		falseVal := false
		overlay := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:    "observability",
					Enabled: &BoolExpression{Value: &falseVal, IsExpr: false},
				},
			},
		}

		base.StrategicMerge(overlay)

		if len(base.Kustomizations) != 1 {
			t.Errorf("Expected 1 kustomization, got %d", len(base.Kustomizations))
		}

		kust := base.Kustomizations[0]
		if kust.Enabled == nil {
			t.Error("Expected Enabled to be set")
		}
		if kust.Enabled.Value == nil || *kust.Enabled.Value != false {
			t.Errorf("Expected Enabled to be false, got %v", kust.Enabled)
		}
		if len(kust.Components) != 1 || kust.Components[0] != "base" {
			t.Errorf("Expected original components preserved, got %v", kust.Components)
		}
	})

	t.Run("MergesTerraformComponentEnabledField", func(t *testing.T) {
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Path:   "cluster/talos",
					Source: "core",
				},
			},
		}

		falseVal := false
		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Path:    "cluster/talos",
					Enabled: &BoolExpression{Value: &falseVal, IsExpr: false},
				},
			},
		}

		base.StrategicMerge(overlay)

		if len(base.TerraformComponents) != 1 {
			t.Errorf("Expected 1 component, got %d", len(base.TerraformComponents))
		}

		component := base.TerraformComponents[0]
		if component.Enabled == nil {
			t.Error("Expected Enabled to be set")
		}
		if component.Enabled.Value == nil || *component.Enabled.Value != false {
			t.Errorf("Expected Enabled to be false, got %v", component.Enabled)
		}
		if component.Path != "cluster/talos" {
			t.Errorf("Expected original path preserved, got %v", component.Path)
		}
	})

	t.Run("MergesKustomizationSubstitutions", func(t *testing.T) {
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name: "observability",
					Substitutions: map[string]string{
						"external_domain": "example.com",
						"timezone":        "UTC",
					},
				},
			},
		}

		overlay := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name: "observability",
					Substitutions: map[string]string{
						"admin_password": "grafana",
					},
				},
			},
		}

		base.StrategicMerge(overlay)

		if len(base.Kustomizations) != 1 {
			t.Errorf("Expected 1 kustomization, got %d", len(base.Kustomizations))
		}

		subs := base.Kustomizations[0].Substitutions
		if subs["external_domain"] != "example.com" {
			t.Errorf("Expected external_domain='example.com', got %q", subs["external_domain"])
		}
		if subs["timezone"] != "UTC" {
			t.Errorf("Expected timezone='UTC', got %q", subs["timezone"])
		}
		if subs["admin_password"] != "grafana" {
			t.Errorf("Expected admin_password='grafana', got %q", subs["admin_password"])
		}
	})

	t.Run("MergesKustomizationSubstitutionsOverwritesExisting", func(t *testing.T) {
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name: "app",
					Substitutions: map[string]string{
						"replicas": "3",
						"env":      "dev",
					},
				},
			},
		}

		overlay := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name: "app",
					Substitutions: map[string]string{
						"replicas": "5",
						"region":   "us-west",
					},
				},
			},
		}

		base.StrategicMerge(overlay)

		subs := base.Kustomizations[0].Substitutions
		if subs["replicas"] != "5" {
			t.Errorf("Expected replicas='5' (overwritten), got %q", subs["replicas"])
		}
		if subs["env"] != "dev" {
			t.Errorf("Expected env='dev' (preserved), got %q", subs["env"])
		}
		if subs["region"] != "us-west" {
			t.Errorf("Expected region='us-west' (added), got %q", subs["region"])
		}
	})

	t.Run("MergesKustomizationSubstitutionsIntoNilMap", func(t *testing.T) {
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name: "app",
				},
			},
		}

		overlay := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name: "app",
					Substitutions: map[string]string{
						"key": "value",
					},
				},
			},
		}

		base.StrategicMerge(overlay)

		subs := base.Kustomizations[0].Substitutions
		if subs == nil {
			t.Fatal("Expected Substitutions to be initialized")
		}
		if subs["key"] != "value" {
			t.Errorf("Expected key='value', got %q", subs["key"])
		}
	})

	t.Run("HandlesDependencyAwareInsertion", func(t *testing.T) {
		// Given a base blueprint with ordered components
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{Path: "backend", Source: "core"},
				{Path: "network", Source: "core"},
			},
		}

		// When adding a component that depends on existing component
		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Path:      "cluster",
					Source:    "core",
					DependsOn: []string{"network"}, // Should be inserted after network
				},
			},
		}

		base.StrategicMerge(overlay)

		// Then component should be inserted in correct order
		if len(base.TerraformComponents) != 3 {
			t.Errorf("Expected 3 components, got %d", len(base.TerraformComponents))
		}

		// Should be: backend, network, cluster (cluster after its dependency)
		if base.TerraformComponents[2].Path != "cluster" {
			t.Errorf("Expected cluster component at index 2, got '%s'", base.TerraformComponents[2].Path)
		}
	})

	t.Run("ReordersExistingComponentsWhenDependenciesChange", func(t *testing.T) {
		// Given a base blueprint with components in wrong order
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{Path: "gitops/flux", Source: "core", DependsOn: []string{"cluster/talos"}},
				{Path: "cluster/talos", Source: "core", Parallelism: intExprPtr(1)},
			},
		}

		// When strategic merging with same components but different order
		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{Path: "cluster/talos", Source: "core", Parallelism: intExprPtr(1)},
				{Path: "gitops/flux", Source: "core", DependsOn: []string{"cluster/talos"}, Destroy: boolExprPtr(false)},
			},
		}

		base.StrategicMerge(overlay)

		// Then components should be reordered according to dependencies
		if len(base.TerraformComponents) != 2 {
			t.Errorf("Expected 2 components, got %d", len(base.TerraformComponents))
		}

		// cluster/talos should come first (dependency), then gitops/flux (dependent)
		if base.TerraformComponents[0].Path != "cluster/talos" {
			t.Errorf("Expected cluster/talos at index 0, got '%s'", base.TerraformComponents[0].Path)
		}
		if base.TerraformComponents[1].Path != "gitops/flux" {
			t.Errorf("Expected gitops/flux at index 1, got '%s'", base.TerraformComponents[1].Path)
		}

		// Verify properties are merged correctly
		cluster := base.TerraformComponents[0]
		parallelism := cluster.Parallelism.ToInt()
		if parallelism == nil || *parallelism != 1 {
			t.Errorf("Expected cluster parallelism to be 1")
		}

		flux := base.TerraformComponents[1]
		destroy := flux.Destroy.ToBool()
		if destroy == nil || *destroy != false {
			t.Errorf("Expected flux destroy to be false")
		}
		if len(flux.DependsOn) != 1 || flux.DependsOn[0] != "cluster/talos" {
			t.Errorf("Expected flux to depend on cluster/talos")
		}
	})

	t.Run("DetectsDependencyCycles", func(t *testing.T) {
		// Given a base blueprint with components that create a cycle
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{Path: "component-a", Source: "core", DependsOn: []string{"component-b"}},
				{Path: "component-b", Source: "core", DependsOn: []string{"component-c"}},
				{Path: "component-c", Source: "core", DependsOn: []string{"component-a"}},
			},
		}

		// When strategic merging (which triggers sorting)
		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{Path: "component-d", Source: "core"},
			},
		}

		err := base.StrategicMerge(overlay)

		// Then should return an error about the cycle
		if err == nil {
			t.Errorf("Expected error for dependency cycle, got nil")
		}
		if !strings.Contains(err.Error(), "dependency cycle detected") {
			t.Errorf("Expected cycle detection error, got: %v", err)
		}
	})

	t.Run("PreservesExistingComponentsNotInOverlay", func(t *testing.T) {
		// Given a base blueprint with existing components
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{Path: "existing-component", Source: "core", Inputs: map[string]any{"key": "value"}},
				{Path: "another-existing", Source: "core", Inputs: map[string]any{"other": "data"}},
			},
		}

		// When strategic merging with overlay that only has one component
		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{Path: "new-component", Source: "core", Inputs: map[string]any{"new": "value"}},
			},
		}

		err := base.StrategicMerge(overlay)

		// Then should preserve all existing components and add new ones
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		if len(base.TerraformComponents) != 3 {
			t.Errorf("Expected 3 components, got %d", len(base.TerraformComponents))
		}

		// Check that existing components are preserved
		foundExisting := false
		foundAnother := false
		foundNew := false

		for _, comp := range base.TerraformComponents {
			switch comp.Path {
			case "existing-component":
				foundExisting = true
				if comp.Inputs["key"] != "value" {
					t.Errorf("Expected existing component inputs to be preserved")
				}
			case "another-existing":
				foundAnother = true
				if comp.Inputs["other"] != "data" {
					t.Errorf("Expected another existing component inputs to be preserved")
				}
			case "new-component":
				foundNew = true
				if comp.Inputs["new"] != "value" {
					t.Errorf("Expected new component inputs to be added")
				}
			}
		}

		if !foundExisting {
			t.Errorf("Expected existing-component to be preserved")
		}
		if !foundAnother {
			t.Errorf("Expected another-existing to be preserved")
		}
		if !foundNew {
			t.Errorf("Expected new-component to be added")
		}
	})

	t.Run("HandlesNilOverlay", func(t *testing.T) {
		// Given a base blueprint
		base := &Blueprint{
			Metadata: Metadata{Name: "test"},
		}

		// When strategic merging with nil overlay
		base.StrategicMerge(nil)

		// Then base should be unchanged
		if base.Metadata.Name != "test" {
			t.Errorf("Expected metadata name preserved")
		}
	})

	t.Run("MergesMetadataAndRepository", func(t *testing.T) {
		secretName := "base-secret"
		updatedSecretName := "updated-secret"
		// Given a base blueprint
		base := &Blueprint{
			Metadata: Metadata{
				Name:        "base",
				Description: "base description",
			},
			Repository: Repository{
				Url:        "base-url",
				Ref:        Reference{Branch: "main"},
				SecretName: &secretName,
			},
		}

		// And an overlay with updated metadata
		overlay := &Blueprint{
			Metadata: Metadata{
				Name:        "updated",
				Description: "updated description",
			},
			Repository: Repository{
				Url:        "updated-url",
				Ref:        Reference{Tag: "v1.0.0"},
				SecretName: &updatedSecretName,
			},
		}

		// When strategic merging
		base.StrategicMerge(overlay)

		// Then metadata should be updated
		if base.Metadata.Name != "updated" {
			t.Errorf("Expected name 'updated', got '%s'", base.Metadata.Name)
		}
		if base.Metadata.Description != "updated description" {
			t.Errorf("Expected description 'updated description', got '%s'", base.Metadata.Description)
		}

		// And repository should be completely replaced (not merged field-by-field)
		if base.Repository.Url != "updated-url" {
			t.Errorf("Expected url 'updated-url', got '%s'", base.Repository.Url)
		}
		if base.Repository.Ref.Tag != "v1.0.0" {
			t.Errorf("Expected tag 'v1.0.0', got '%s'", base.Repository.Ref.Tag)
		}
		if base.Repository.Ref.Branch != "" {
			t.Errorf("Expected branch to be empty (replaced, not merged), got '%s'", base.Repository.Ref.Branch)
		}
		if base.Repository.SecretName == nil || *base.Repository.SecretName != "updated-secret" {
			t.Errorf("Expected secretName to be 'updated-secret', got %v", base.Repository.SecretName)
		}
	})

	t.Run("RepositoryReplacementWhenOverlayHasURL", func(t *testing.T) {
		baseSecretName := "base-secret"
		// Given a base blueprint with repository
		base := &Blueprint{
			Repository: Repository{
				Url:        "base-url",
				Ref:        Reference{Branch: "main", Commit: "abc123"},
				SecretName: &baseSecretName,
			},
		}

		// And an overlay with only URL set (no ref, no secretName)
		overlay := &Blueprint{
			Repository: Repository{
				Url: "overlay-url",
			},
		}

		// When strategic merging
		base.StrategicMerge(overlay)

		// Then repository should be completely replaced
		if base.Repository.Url != "overlay-url" {
			t.Errorf("Expected url 'overlay-url', got '%s'", base.Repository.Url)
		}
		if base.Repository.Ref.Branch != "" {
			t.Errorf("Expected branch to be empty (replaced), got '%s'", base.Repository.Ref.Branch)
		}
		if base.Repository.Ref.Commit != "" {
			t.Errorf("Expected commit to be empty (replaced), got '%s'", base.Repository.Ref.Commit)
		}
		if base.Repository.SecretName != nil {
			t.Errorf("Expected secretName to be nil (replaced), got %v", base.Repository.SecretName)
		}
	})

	t.Run("RepositoryNotReplacedWhenOverlayURLEmpty", func(t *testing.T) {
		baseSecretName := "base-secret"
		// Given a base blueprint with repository
		base := &Blueprint{
			Repository: Repository{
				Url:        "base-url",
				Ref:        Reference{Branch: "main"},
				SecretName: &baseSecretName,
			},
		}

		// And an overlay with empty URL
		overlay := &Blueprint{
			Repository: Repository{
				Url: "",
				Ref: Reference{Tag: "v1.0.0"},
			},
		}

		// When strategic merging
		base.StrategicMerge(overlay)

		// Then repository should remain unchanged
		if base.Repository.Url != "base-url" {
			t.Errorf("Expected url 'base-url' to remain, got '%s'", base.Repository.Url)
		}
		if base.Repository.Ref.Branch != "main" {
			t.Errorf("Expected branch 'main' to remain, got '%s'", base.Repository.Ref.Branch)
		}
		if base.Repository.SecretName == nil || *base.Repository.SecretName != "base-secret" {
			t.Errorf("Expected secretName 'base-secret' to remain, got %v", base.Repository.SecretName)
		}
	})

	t.Run("MergesSourcesUniquely", func(t *testing.T) {
		// Given a base blueprint with sources
		base := &Blueprint{
			Sources: []Source{
				{Name: "source1", Url: "url1"},
			},
		}

		// And an overlay with overlapping and new sources
		overlay := &Blueprint{
			Sources: []Source{
				{Name: "source1", Url: "updated-url1"}, // Should update
				{Name: "source2", Url: "url2"},         // Should add
			},
		}

		// When strategic merging
		base.StrategicMerge(overlay)

		// Then should have both sources with updated values
		if len(base.Sources) != 2 {
			t.Errorf("Expected 2 sources, got %d", len(base.Sources))
		}

		// Check that source1 was updated and source2 was added
		sourceMap := make(map[string]string)
		for _, source := range base.Sources {
			sourceMap[source.Name] = source.Url
		}

		if sourceMap["source1"] != "updated-url1" {
			t.Errorf("Expected source1 url to be updated")
		}
		if sourceMap["source2"] != "url2" {
			t.Errorf("Expected source2 to be added")
		}
	})

	t.Run("EmptyOverlayDoesNothing", func(t *testing.T) {
		// Given a base blueprint with content
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{Path: "test", Source: "core"},
			},
			Kustomizations: []Kustomization{
				{Name: "test"},
			},
		}

		// When strategic merging with empty overlay
		overlay := &Blueprint{}
		base.StrategicMerge(overlay)

		// Then base should be unchanged
		if len(base.TerraformComponents) != 1 {
			t.Errorf("Expected terraform components unchanged")
		}
		if len(base.Kustomizations) != 1 {
			t.Errorf("Expected kustomizations unchanged")
		}
	})

	t.Run("KustomizationDependencyAwareInsertion", func(t *testing.T) {
		// Given a base blueprint with ordered kustomizations
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{Name: "policy", Path: "policy"},
				{Name: "pki", Path: "pki"},
			},
		}

		// When adding a kustomization that depends on existing one
		overlay := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:      "ingress",
					Path:      "ingress",
					DependsOn: []string{"pki"}, // Should be inserted after pki
				},
			},
		}

		base.StrategicMerge(overlay)

		// Then kustomization should be inserted in correct order
		if len(base.Kustomizations) != 3 {
			t.Errorf("Expected 3 kustomizations, got %d", len(base.Kustomizations))
		}

		// Should have ingress after pki (its dependency)
		pkiIndex := -1
		ingressIndex := -1
		for i, k := range base.Kustomizations {
			if k.Name == "pki" {
				pkiIndex = i
			}
			if k.Name == "ingress" {
				ingressIndex = i
			}
		}

		if pkiIndex == -1 {
			t.Errorf("Expected pki kustomization to be present")
		}
		if ingressIndex == -1 {
			t.Errorf("Expected ingress kustomization to be present")
		}
		if pkiIndex >= ingressIndex {
			t.Errorf("Expected ingress (index %d) to come after pki (index %d)", ingressIndex, pkiIndex)
		}
	})

	t.Run("KustomizationUpdatesFieldsSelectively", func(t *testing.T) {
		// Given a base blueprint with a kustomization
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:    "test",
					Path:    "original-path",
					Source:  "original-source",
					Destroy: boolExprPtr(false),
				},
			},
		}

		// When merging with partial updates
		overlay := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:    "test", // Same name - should merge
					Path:    "updated-path",
					Source:  "updated-source",
					Destroy: boolExprPtr(true),
					// Note: not setting Components or DependsOn - should preserve existing
				},
			},
		}

		base.StrategicMerge(overlay)

		// Then should have updated fields
		kustomization := base.Kustomizations[0]
		if kustomization.Path != "updated-path" {
			t.Errorf("Expected path to be updated to 'updated-path', got '%s'", kustomization.Path)
		}
		if kustomization.Source != "updated-source" {
			t.Errorf("Expected source to be updated to 'updated-source', got '%s'", kustomization.Source)
		}
		destroy := kustomization.Destroy.ToBool()
		if destroy == nil || *destroy != true {
			t.Errorf("Expected destroy to be updated to true, got %v", destroy)
		}
	})

	t.Run("KustomizationPreservesExistingComponents", func(t *testing.T) {
		// Given a base blueprint with kustomization that has components
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:       "test",
					Components: []string{"existing1", "existing2"},
					DependsOn:  []string{"dep1"},
				},
			},
		}

		// When merging with additional components and dependencies
		overlay := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:       "test",
					Components: []string{"existing2", "new1"}, // existing2 is duplicate, new1 is new
					DependsOn:  []string{"dep1", "dep2"},      // dep1 is duplicate, dep2 is new
				},
			},
		}

		base.StrategicMerge(overlay)

		// Then should have all unique components and dependencies
		kustomization := base.Kustomizations[0]
		if len(kustomization.Components) != 3 {
			t.Errorf("Expected 3 unique components, got %d: %v", len(kustomization.Components), kustomization.Components)
		}

		expectedComponents := []string{"existing1", "existing2", "new1"}
		for _, expected := range expectedComponents {
			if !contains(kustomization.Components, expected) {
				t.Errorf("Expected component '%s' to be present, got %v", expected, kustomization.Components)
			}
		}

		if len(kustomization.DependsOn) != 2 {
			t.Errorf("Expected 2 unique dependencies, got %d: %v", len(kustomization.DependsOn), kustomization.DependsOn)
		}

		expectedDeps := []string{"dep1", "dep2"}
		for _, expected := range expectedDeps {
			if !contains(kustomization.DependsOn, expected) {
				t.Errorf("Expected dependency '%s' to be present, got %v", expected, kustomization.DependsOn)
			}
		}
	})

	t.Run("KustomizationMultipleDependencyInsertion", func(t *testing.T) {
		// Given a base blueprint with multiple kustomizations
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{Name: "base", Path: "base"},
				{Name: "pki", Path: "pki"},
				{Name: "storage", Path: "storage"},
			},
		}

		// When adding a kustomization that depends on multiple existing ones
		overlay := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:      "app",
					Path:      "app",
					DependsOn: []string{"pki", "storage"}, // Depends on multiple
				},
			},
		}

		base.StrategicMerge(overlay)

		// Then should be inserted after the latest dependency
		if len(base.Kustomizations) != 4 {
			t.Errorf("Expected 4 kustomizations, got %d", len(base.Kustomizations))
		}

		// App should come after its dependencies (pki and storage)
		appIndex := -1
		for i, k := range base.Kustomizations {
			if k.Name == "app" {
				appIndex = i
				break
			}
		}
		if appIndex == -1 {
			t.Errorf("Expected app kustomization to be present")
		}

		// Find indices of dependencies
		pkiIndex := -1
		storageIndex := -1
		for i, k := range base.Kustomizations {
			if k.Name == "pki" {
				pkiIndex = i
			}
			if k.Name == "storage" {
				storageIndex = i
			}
		}

		// App should come after both dependencies
		if appIndex <= pkiIndex || appIndex <= storageIndex {
			t.Errorf("Expected app (index %d) to come after pki (index %d) and storage (index %d)", appIndex, pkiIndex, storageIndex)
		}
	})

	t.Run("ComplexDependencyOrdering", func(t *testing.T) {
		// Test the complex dependency scenario described by the user
		// where pki-* components are separated by dns, but dns depends on both pki-base and ingress

		// Start with a base blueprint that has some kustomizations
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{Name: "policy-base", Path: "policy/base"},
				{Name: "policy-resources", Path: "policy/resources", DependsOn: []string{"policy-base"}},
			},
		}

		// Add kustomizations one by one to trigger strategic merge and sorting
		overlay1 := &Blueprint{
			Kustomizations: []Kustomization{
				{Name: "pki-base", Path: "pki/base", DependsOn: []string{"policy-resources"}},
			},
		}
		base.StrategicMerge(overlay1)

		overlay2 := &Blueprint{
			Kustomizations: []Kustomization{
				{Name: "pki-resources", Path: "pki/resources", DependsOn: []string{"pki-base"}},
			},
		}
		base.StrategicMerge(overlay2)

		overlay3 := &Blueprint{
			Kustomizations: []Kustomization{
				{Name: "ingress", Path: "ingress", DependsOn: []string{"pki-resources"}},
			},
		}
		base.StrategicMerge(overlay3)

		overlay4 := &Blueprint{
			Kustomizations: []Kustomization{
				{Name: "dns", Path: "dns", DependsOn: []string{"pki-base", "ingress"}},
			},
		}
		base.StrategicMerge(overlay4)

		// Expected order: policy-base, policy-resources, pki-base, pki-resources, ingress, dns
		expectedOrder := []string{"policy-base", "policy-resources", "pki-base", "pki-resources", "ingress", "dns"}

		if len(base.Kustomizations) != len(expectedOrder) {
			t.Errorf("Expected %d kustomizations, got %d", len(expectedOrder), len(base.Kustomizations))
		}

		for i, expected := range expectedOrder {
			if i >= len(base.Kustomizations) || base.Kustomizations[i].Name != expected {
				actual := "none"
				if i < len(base.Kustomizations) {
					actual = base.Kustomizations[i].Name
				}
				t.Errorf("Expected '%s' at position %d, got '%s'", expected, i, actual)
			}
		}

		// Verify that dependencies are satisfied
		nameToIndex := make(map[string]int)
		for i, k := range base.Kustomizations {
			nameToIndex[k.Name] = i
		}

		for _, k := range base.Kustomizations {
			for _, dep := range k.DependsOn {
				if depIndex, exists := nameToIndex[dep]; exists {
					if depIndex >= nameToIndex[k.Name] {
						t.Errorf("Dependency violation: '%s' (index %d) depends on '%s' (index %d), but dependency should come first",
							k.Name, nameToIndex[k.Name], dep, depIndex)
					}
				}
			}
		}
	})
}

func TestBlueprint_ReplaceTerraformComponent(t *testing.T) {
	t.Run("ReplacesExistingComponent", func(t *testing.T) {
		// Given a base blueprint with a terraform component
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Path:      "network/vpc",
					Source:    "core",
					Inputs:    map[string]any{"cidr": "10.0.0.0/16", "enable_dns": false},
					DependsOn: []string{"backend", "security"},
					Destroy:   boolExprPtr(true),
				},
			},
		}

		// When replacing with a new component
		replacement := TerraformComponent{
			Path:        "network/vpc",
			Source:      "core",
			Inputs:      map[string]any{"cidr": "172.16.0.0/16"},
			DependsOn:   []string{"new-dependency"},
			Destroy:     boolExprPtr(false),
			Parallelism: intExprPtr(5),
		}

		err := base.ReplaceTerraformComponent(replacement)

		// Then should replace the component entirely
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(base.TerraformComponents) != 1 {
			t.Errorf("Expected 1 component, got %d", len(base.TerraformComponents))
		}

		replaced := base.TerraformComponents[0]
		if replaced.Path != "network/vpc" {
			t.Errorf("Expected path 'network/vpc', got '%s'", replaced.Path)
		}
		if len(replaced.Inputs) != 1 {
			t.Errorf("Expected 1 input (replaced), got %d", len(replaced.Inputs))
		}
		if replaced.Inputs["cidr"] != "172.16.0.0/16" {
			t.Errorf("Expected new cidr value, got %v", replaced.Inputs["cidr"])
		}
		if replaced.Inputs["enable_dns"] != nil {
			t.Errorf("Expected old enable_dns to be removed, got %v", replaced.Inputs["enable_dns"])
		}
		if len(replaced.DependsOn) != 1 {
			t.Errorf("Expected 1 dependency (replaced), got %d", len(replaced.DependsOn))
		}
		if replaced.DependsOn[0] != "new-dependency" {
			t.Errorf("Expected new dependency, got %v", replaced.DependsOn)
		}
		destroy := replaced.Destroy.ToBool()
		if destroy == nil || *destroy != false {
			t.Errorf("Expected destroy=false, got %v", destroy)
		}
		parallelism := replaced.Parallelism.ToInt()
		if parallelism == nil || *parallelism != 5 {
			t.Errorf("Expected parallelism=5, got %v", parallelism)
		}
	})

	t.Run("AppendsNewComponent", func(t *testing.T) {
		// Given a base blueprint with existing components
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{Path: "existing", Source: "core"},
			},
		}

		// When replacing with a component that doesn't exist
		newComponent := TerraformComponent{
			Path:   "new-component",
			Source: "core",
			Inputs: map[string]any{"key": "value"},
		}

		err := base.ReplaceTerraformComponent(newComponent)

		// Then should append the new component
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(base.TerraformComponents) != 2 {
			t.Errorf("Expected 2 components, got %d", len(base.TerraformComponents))
		}

		found := false
		for _, comp := range base.TerraformComponents {
			if comp.Path == "new-component" {
				found = true
				if comp.Inputs["key"] != "value" {
					t.Errorf("Expected new component inputs to be preserved")
				}
			}
		}
		if !found {
			t.Errorf("Expected new component to be added")
		}
	})

	t.Run("MaintainsDependencyOrder", func(t *testing.T) {
		// Given a base blueprint with ordered components
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{Path: "backend", Source: "core"},
				{Path: "network", Source: "core"},
				{Path: "cluster", Source: "core", DependsOn: []string{"network"}},
			},
		}

		// When replacing network component with new dependencies
		replacement := TerraformComponent{
			Path:      "network",
			Source:    "core",
			DependsOn: []string{"backend", "new-dep"},
		}

		err := base.ReplaceTerraformComponent(replacement)

		// Then should maintain dependency order
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		networkIndex := -1
		clusterIndex := -1
		for i, comp := range base.TerraformComponents {
			if comp.Path == "network" {
				networkIndex = i
			}
			if comp.Path == "cluster" {
				clusterIndex = i
			}
		}

		if networkIndex == -1 || clusterIndex == -1 {
			t.Fatal("Expected both network and cluster components")
		}

		if networkIndex >= clusterIndex {
			t.Errorf("Expected network (index %d) to come before cluster (index %d) due to dependency", networkIndex, clusterIndex)
		}
	})
}

func TestBlueprint_ReplaceKustomization(t *testing.T) {
	t.Run("ReplacesExistingKustomization", func(t *testing.T) {
		// Given a base blueprint with a kustomization
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:       "ingress",
					Path:       "original-path",
					Source:     "original-source",
					Components: []string{"nginx", "cert-manager"},
					DependsOn:  []string{"pki", "dns"},
					Destroy:    boolExprPtr(true),
				},
			},
		}

		// When replacing with a new kustomization
		replacement := Kustomization{
			Name:       "ingress",
			Path:       "new-path",
			Source:     "new-source",
			Components: []string{"traefik"},
			DependsOn:  []string{"new-dependency"},
			Destroy:    boolExprPtr(false),
		}

		err := base.ReplaceKustomization(replacement)

		// Then should replace the kustomization entirely
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(base.Kustomizations) != 1 {
			t.Errorf("Expected 1 kustomization, got %d", len(base.Kustomizations))
		}

		replaced := base.Kustomizations[0]
		if replaced.Name != "ingress" {
			t.Errorf("Expected name 'ingress', got '%s'", replaced.Name)
		}
		if replaced.Path != "new-path" {
			t.Errorf("Expected path 'new-path', got '%s'", replaced.Path)
		}
		if replaced.Source != "new-source" {
			t.Errorf("Expected source 'new-source', got '%s'", replaced.Source)
		}
		if len(replaced.Components) != 1 {
			t.Errorf("Expected 1 component (replaced), got %d", len(replaced.Components))
		}
		if replaced.Components[0] != "traefik" {
			t.Errorf("Expected component 'traefik', got %v", replaced.Components)
		}
		if len(replaced.DependsOn) != 1 {
			t.Errorf("Expected 1 dependency (replaced), got %d", len(replaced.DependsOn))
		}
		if replaced.DependsOn[0] != "new-dependency" {
			t.Errorf("Expected new dependency, got %v", replaced.DependsOn)
		}
		destroy := replaced.Destroy.ToBool()
		if destroy == nil || *destroy != false {
			t.Errorf("Expected destroy=false, got %v", destroy)
		}
	})

	t.Run("AppendsNewKustomization", func(t *testing.T) {
		// Given a base blueprint with existing kustomizations
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{Name: "existing", Path: "existing-path"},
			},
		}

		// When replacing with a kustomization that doesn't exist
		newKustomization := Kustomization{
			Name:       "new-kustomization",
			Path:       "new-path",
			Components: []string{"component1"},
		}

		err := base.ReplaceKustomization(newKustomization)

		// Then should append the new kustomization
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(base.Kustomizations) != 2 {
			t.Errorf("Expected 2 kustomizations, got %d", len(base.Kustomizations))
		}

		found := false
		for i := range base.Kustomizations {
			k := base.Kustomizations[i]
			if k.Name == "new-kustomization" {
				found = true
				if k.Path != "new-path" {
					t.Errorf("Expected new kustomization path to be preserved")
				}
				if len(k.Components) != 1 || k.Components[0] != "component1" {
					t.Errorf("Expected new kustomization components to be preserved")
				}
			}
		}
		if !found {
			t.Errorf("Expected new kustomization to be added")
		}
	})

	t.Run("MaintainsDependencyOrder", func(t *testing.T) {
		// Given a base blueprint with ordered kustomizations
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{Name: "pki", Path: "pki"},
				{Name: "ingress", Path: "ingress", DependsOn: []string{"pki"}},
			},
		}

		// When replacing pki with new dependencies
		replacement := Kustomization{
			Name:      "pki",
			Path:      "pki",
			DependsOn: []string{"base"},
		}

		err := base.ReplaceKustomization(replacement)

		// Then should maintain dependency order
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		pkiIndex := -1
		ingressIndex := -1
		for i, k := range base.Kustomizations {
			if k.Name == "pki" {
				pkiIndex = i
			}
			if k.Name == "ingress" {
				ingressIndex = i
			}
		}

		if pkiIndex == -1 || ingressIndex == -1 {
			t.Fatal("Expected both pki and ingress kustomizations")
		}

		if pkiIndex >= ingressIndex {
			t.Errorf("Expected pki (index %d) to come before ingress (index %d) due to dependency", pkiIndex, ingressIndex)
		}
	})
}

func TestBlueprint_RemoveTerraformComponent(t *testing.T) {
	t.Run("RemovesInputsFromExistingComponent", func(t *testing.T) {
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Path:      "network/vpc",
					Source:    "core",
					Inputs:    map[string]any{"cidr": "10.0.0.0/16", "enable_dns": true, "keep_this": "value"},
					DependsOn: []string{"backend", "security"},
				},
			},
		}

		removal := TerraformComponent{
			Path:   "network/vpc",
			Source: "core",
			Inputs: map[string]any{"cidr": nil, "enable_dns": nil},
		}

		err := base.RemoveTerraformComponent(removal)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(base.TerraformComponents) != 1 {
			t.Errorf("Expected 1 component, got %d", len(base.TerraformComponents))
		}

		component := base.TerraformComponents[0]
		if len(component.Inputs) != 1 {
			t.Errorf("Expected 1 input remaining, got %d", len(component.Inputs))
		}
		if component.Inputs["keep_this"] != "value" {
			t.Errorf("Expected 'keep_this' input to remain, got %v", component.Inputs)
		}
		if component.Inputs["cidr"] != nil {
			t.Errorf("Expected 'cidr' input to be removed, got %v", component.Inputs["cidr"])
		}
		if component.Inputs["enable_dns"] != nil {
			t.Errorf("Expected 'enable_dns' input to be removed, got %v", component.Inputs["enable_dns"])
		}
	})

	t.Run("RemovesDependenciesFromExistingComponent", func(t *testing.T) {
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Path:      "network/vpc",
					Source:    "core",
					DependsOn: []string{"backend", "security", "keep_dep"},
				},
			},
		}

		removal := TerraformComponent{
			Path:      "network/vpc",
			Source:    "core",
			DependsOn: []string{"backend", "security"},
		}

		err := base.RemoveTerraformComponent(removal)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		component := base.TerraformComponents[0]
		if len(component.DependsOn) != 1 {
			t.Errorf("Expected 1 dependency remaining, got %d: %v", len(component.DependsOn), component.DependsOn)
		}
		if component.DependsOn[0] != "keep_dep" {
			t.Errorf("Expected 'keep_dep' dependency to remain, got %v", component.DependsOn)
		}
	})

	t.Run("RemovesBothInputsAndDependencies", func(t *testing.T) {
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Path:      "network/vpc",
					Source:    "core",
					Inputs:    map[string]any{"cidr": "10.0.0.0/16", "keep_input": "value"},
					DependsOn: []string{"backend", "keep_dep"},
				},
			},
		}

		removal := TerraformComponent{
			Path:      "network/vpc",
			Source:    "core",
			Inputs:    map[string]any{"cidr": nil},
			DependsOn: []string{"backend"},
		}

		err := base.RemoveTerraformComponent(removal)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		component := base.TerraformComponents[0]
		if len(component.Inputs) != 1 {
			t.Errorf("Expected 1 input remaining, got %d", len(component.Inputs))
		}
		if len(component.DependsOn) != 1 {
			t.Errorf("Expected 1 dependency remaining, got %d", len(component.DependsOn))
		}
		if component.Inputs["keep_input"] != "value" {
			t.Errorf("Expected 'keep_input' to remain")
		}
		if component.DependsOn[0] != "keep_dep" {
			t.Errorf("Expected 'keep_dep' to remain")
		}
	})

	t.Run("NoOpWhenComponentNotFound", func(t *testing.T) {
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Path:   "existing",
					Source: "core",
					Inputs: map[string]any{"key": "value"},
				},
			},
		}

		removal := TerraformComponent{
			Path:   "non-existent",
			Source: "core",
			Inputs: map[string]any{"key": nil},
		}

		err := base.RemoveTerraformComponent(removal)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(base.TerraformComponents) != 1 {
			t.Errorf("Expected 1 component unchanged, got %d", len(base.TerraformComponents))
		}
		if base.TerraformComponents[0].Inputs["key"] != "value" {
			t.Errorf("Expected existing component to be unchanged")
		}
	})

	t.Run("PreservesIndexFields", func(t *testing.T) {
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Path:      "network/vpc",
					Source:    "core",
					Inputs:    map[string]any{"cidr": "10.0.0.0/16"},
					DependsOn: []string{"backend"},
				},
			},
		}

		removal := TerraformComponent{
			Path:   "network/vpc",
			Source: "core",
			Inputs: map[string]any{"cidr": nil},
		}

		err := base.RemoveTerraformComponent(removal)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		component := base.TerraformComponents[0]
		if component.Path != "network/vpc" {
			t.Errorf("Expected Path to be preserved, got '%s'", component.Path)
		}
		if component.Source != "core" {
			t.Errorf("Expected Source to be preserved, got '%s'", component.Source)
		}
	})
}

func TestBlueprint_RemoveKustomization(t *testing.T) {
	t.Run("RemovesDependenciesFromExistingKustomization", func(t *testing.T) {
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:       "ingress",
					DependsOn:  []string{"pki", "dns", "keep_dep"},
					Components: []string{"nginx"},
				},
			},
		}

		removal := Kustomization{
			Name:      "ingress",
			DependsOn: []string{"pki", "dns"},
		}

		err := base.RemoveKustomization(removal)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		kustomization := base.Kustomizations[0]
		if len(kustomization.DependsOn) != 1 {
			t.Errorf("Expected 1 dependency remaining, got %d: %v", len(kustomization.DependsOn), kustomization.DependsOn)
		}
		if kustomization.DependsOn[0] != "keep_dep" {
			t.Errorf("Expected 'keep_dep' dependency to remain, got %v", kustomization.DependsOn)
		}
	})

	t.Run("RemovesComponentsFromExistingKustomization", func(t *testing.T) {
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:       "ingress",
					Components: []string{"nginx", "cert-manager", "keep_component"},
				},
			},
		}

		removal := Kustomization{
			Name:       "ingress",
			Components: []string{"nginx", "cert-manager"},
		}

		err := base.RemoveKustomization(removal)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		kustomization := base.Kustomizations[0]
		if len(kustomization.Components) != 1 {
			t.Errorf("Expected 1 component remaining, got %d: %v", len(kustomization.Components), kustomization.Components)
		}
		if kustomization.Components[0] != "keep_component" {
			t.Errorf("Expected 'keep_component' to remain, got %v", kustomization.Components)
		}
	})

	t.Run("RemovesCleanupFromExistingKustomization", func(t *testing.T) {
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:    "ingress",
					Cleanup: []string{"old-resource", "another-resource", "keep_cleanup"},
				},
			},
		}

		removal := Kustomization{
			Name:    "ingress",
			Cleanup: []string{"old-resource", "another-resource"},
		}

		err := base.RemoveKustomization(removal)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		kustomization := base.Kustomizations[0]
		if len(kustomization.Cleanup) != 1 {
			t.Errorf("Expected 1 cleanup remaining, got %d: %v", len(kustomization.Cleanup), kustomization.Cleanup)
		}
		if kustomization.Cleanup[0] != "keep_cleanup" {
			t.Errorf("Expected 'keep_cleanup' to remain, got %v", kustomization.Cleanup)
		}
	})

	t.Run("RemovesPatchesByPath", func(t *testing.T) {
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name: "ingress",
					Patches: []BlueprintPatch{
						{Path: "patches/patch1.yaml"},
						{Path: "patches/patch2.yaml"},
						{Path: "patches/keep.yaml"},
					},
				},
			},
		}

		removal := Kustomization{
			Name: "ingress",
			Patches: []BlueprintPatch{
				{Path: "patches/patch1.yaml"},
				{Path: "patches/patch2.yaml"},
			},
		}

		err := base.RemoveKustomization(removal)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		kustomization := base.Kustomizations[0]
		if len(kustomization.Patches) != 1 {
			t.Errorf("Expected 1 patch remaining, got %d", len(kustomization.Patches))
		}
		if kustomization.Patches[0].Path != "patches/keep.yaml" {
			t.Errorf("Expected 'patches/keep.yaml' to remain, got %v", kustomization.Patches)
		}
	})

	t.Run("RemovesPatchesByContent", func(t *testing.T) {
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name: "ingress",
					Patches: []BlueprintPatch{
						{Patch: "apiVersion: v1\nkind: Service"},
						{Patch: "apiVersion: v1\nkind: Deployment"},
						{Patch: "apiVersion: v1\nkind: Keep"},
					},
				},
			},
		}

		removal := Kustomization{
			Name: "ingress",
			Patches: []BlueprintPatch{
				{Patch: "apiVersion: v1\nkind: Service"},
				{Patch: "apiVersion: v1\nkind: Deployment"},
			},
		}

		err := base.RemoveKustomization(removal)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		kustomization := base.Kustomizations[0]
		if len(kustomization.Patches) != 1 {
			t.Errorf("Expected 1 patch remaining, got %d", len(kustomization.Patches))
		}
		if kustomization.Patches[0].Patch != "apiVersion: v1\nkind: Keep" {
			t.Errorf("Expected 'Keep' patch to remain, got %v", kustomization.Patches)
		}
	})

	t.Run("RemovesSubstitutionsFromExistingKustomization", func(t *testing.T) {
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name: "ingress",
					Substitutions: map[string]string{
						"domain":    "example.com",
						"region":    "us-west-2",
						"keep_this": "value",
					},
				},
			},
		}

		removal := Kustomization{
			Name: "ingress",
			Substitutions: map[string]string{
				"domain": "",
				"region": "",
			},
		}

		err := base.RemoveKustomization(removal)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		kustomization := base.Kustomizations[0]
		if len(kustomization.Substitutions) != 1 {
			t.Errorf("Expected 1 substitution remaining, got %d: %v", len(kustomization.Substitutions), kustomization.Substitutions)
		}
		if kustomization.Substitutions["keep_this"] != "value" {
			t.Errorf("Expected 'keep_this' substitution to remain, got %v", kustomization.Substitutions)
		}
	})

	t.Run("NoOpWhenKustomizationNotFound", func(t *testing.T) {
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:       "existing",
					Components: []string{"component1"},
				},
			},
		}

		removal := Kustomization{
			Name:       "non-existent",
			Components: []string{"component1"},
		}

		err := base.RemoveKustomization(removal)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(base.Kustomizations) != 1 {
			t.Errorf("Expected 1 kustomization unchanged, got %d", len(base.Kustomizations))
		}
		if len(base.Kustomizations[0].Components) != 1 {
			t.Errorf("Expected existing kustomization to be unchanged")
		}
	})

	t.Run("PreservesIndexField", func(t *testing.T) {
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:       "ingress",
					Path:       "original-path",
					Source:     "original-source",
					Components: []string{"nginx"},
				},
			},
		}

		removal := Kustomization{
			Name:       "ingress",
			Components: []string{"nginx"},
		}

		err := base.RemoveKustomization(removal)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		kustomization := base.Kustomizations[0]
		if kustomization.Name != "ingress" {
			t.Errorf("Expected Name to be preserved, got '%s'", kustomization.Name)
		}
		if kustomization.Path != "original-path" {
			t.Errorf("Expected Path to be preserved, got '%s'", kustomization.Path)
		}
		if kustomization.Source != "original-source" {
			t.Errorf("Expected Source to be preserved, got '%s'", kustomization.Source)
		}
	})
}

func TestBlueprint_DeepCopy(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		blueprint := &Blueprint{
			Metadata: Metadata{
				Name: "test-blueprint",
			},
			Sources: []Source{
				{
					Name:       "source1",
					Url:        "https://example.com/repo1.git",
					PathPrefix: "terraform",
					Ref: Reference{
						Branch: "main",
					},
				},
			},
			TerraformComponents: []TerraformComponent{
				{
					Source: "source1",
					Path:   "module/path1",
					Inputs: map[string]any{
						"key1": "value1",
					},
				},
			},
			Kustomizations: []Kustomization{
				{
					Name:       "kustomization1",
					Path:       "kustomize/path1",
					Components: []string{"component1"},
				},
			},
		}
		copy := blueprint.DeepCopy()
		if copy.Metadata.Name != "test-blueprint" {
			t.Errorf("Expected copy to have name %v, but got %v", "test-blueprint", copy.Metadata.Name)
		}
		if len(copy.Sources) != 1 || copy.Sources[0].Name != "source1" {
			t.Errorf("Expected copy to have source %v, but got %v", "source1", copy.Sources)
		}
		if len(copy.TerraformComponents) != 1 || copy.TerraformComponents[0].Source != "source1" {
			t.Errorf("Expected copy to have terraform component source %v, but got %v", "source1", copy.TerraformComponents)
		}
		if copy.TerraformComponents[0].Path != "module/path1" {
			t.Errorf("Expected copy to have terraform component path %v, but got %v", "module/path1", copy.TerraformComponents[0].Path)
		}
		if len(copy.TerraformComponents[0].Inputs) != 1 || copy.TerraformComponents[0].Inputs["key1"] != "value1" {
			t.Errorf("Expected copy to have terraform component input 'key1' with value 'value1', but got %v", copy.TerraformComponents[0].Inputs)
		}
		if len(copy.Kustomizations) != 1 || copy.Kustomizations[0].Name != "kustomization1" {
			t.Errorf("Expected copy to have kustomization 'kustomization1', but got %v", copy.Kustomizations)
		}
		if len(copy.Kustomizations[0].Components) != 1 || copy.Kustomizations[0].Components[0] != "component1" {
			t.Errorf("Expected copy to have kustomization component 'component1', but got %v", copy.Kustomizations[0].Components)
		}
	})

	t.Run("EmptyBlueprint", func(t *testing.T) {
		var blueprint *Blueprint
		copy := blueprint.DeepCopy()
		if copy != nil {
			t.Errorf("Expected copy to be nil, but got non-nil")
		}
	})
}

// Helper function to check if slice contains a value
func contains(slice []string, value string) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}

func TestIsLocalTemplateSource(t *testing.T) {
	t.Run("ReturnsFalseForNonTemplateSource", func(t *testing.T) {
		source := Source{Name: "other", Url: ""}
		if IsLocalTemplateSource(source) {
			t.Error("Expected false for non-template source")
		}
	})

	t.Run("ReturnsFalseForTemplateWithURL", func(t *testing.T) {
		source := Source{Name: "template", Url: "https://github.com/example/repo.git"}
		if IsLocalTemplateSource(source) {
			t.Error("Expected false for template source with URL")
		}
	})

	t.Run("ReturnsTrueForTemplateWithEmptyURL", func(t *testing.T) {
		source := Source{Name: "template", Url: ""}
		if !IsLocalTemplateSource(source) {
			t.Error("Expected true for template source with empty URL")
		}
	})
}

func TestHasRemoteTemplateSource(t *testing.T) {
	t.Run("ReturnsFalseForEmptySources", func(t *testing.T) {
		if HasRemoteTemplateSource([]Source{}) {
			t.Error("Expected false for empty sources")
		}
	})

	t.Run("ReturnsFalseWhenOnlyLocalTemplate", func(t *testing.T) {
		sources := []Source{{Name: "template", Url: ""}}
		if HasRemoteTemplateSource(sources) {
			t.Error("Expected false when only local template")
		}
	})

	t.Run("ReturnsTrueWhenTemplateHasURL", func(t *testing.T) {
		sources := []Source{
			{Name: "template", Url: "https://github.com/example/repo.git"},
		}
		if !HasRemoteTemplateSource(sources) {
			t.Error("Expected true when template source has URL")
		}
	})

	t.Run("ReturnsTrueWhenMultipleSourcesIncludeRemoteTemplate", func(t *testing.T) {
		sources := []Source{
			{Name: "other", Url: "https://other.com/repo.git"},
			{Name: "template", Url: "https://github.com/example/repo.git"},
		}
		if !HasRemoteTemplateSource(sources) {
			t.Error("Expected true when sources include remote template")
		}
	})
}

func TestKustomization_ToFluxKustomization(t *testing.T) {
	t.Run("BasicConversionWithDefaults", func(t *testing.T) {
		kustomization := &Kustomization{
			Name: "test-kustomization",
			Path: "test/path",
			Substitutions: map[string]string{
				"domain": "example.com",
			},
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{})

		if result.Name != "test-kustomization" {
			t.Errorf("Expected name 'test-kustomization', got '%s'", result.Name)
		}
		if result.Namespace != "test-namespace" {
			t.Errorf("Expected namespace 'test-namespace', got '%s'", result.Namespace)
		}
		if result.Spec.SourceRef.Name != "default-source" {
			t.Errorf("Expected source name 'default-source', got '%s'", result.Spec.SourceRef.Name)
		}
		if result.Spec.SourceRef.Kind != "GitRepository" {
			t.Errorf("Expected source kind 'GitRepository', got '%s'", result.Spec.SourceRef.Kind)
		}
		if result.Spec.Path != "kustomize/test/path" {
			t.Errorf("Expected path 'kustomize/test/path', got '%s'", result.Spec.Path)
		}
		if result.Spec.Interval.Duration != constants.DefaultFluxKustomizationInterval {
			t.Errorf("Expected default interval, got %v", result.Spec.Interval.Duration)
		}
		// PostBuild should have component-specific ConfigMap when substitutions exist
		if result.Spec.PostBuild == nil {
			t.Fatal("Expected PostBuild to be set when substitutions exist")
		}
		if len(result.Spec.PostBuild.SubstituteFrom) != 1 {
			t.Fatalf("Expected 1 SubstituteFrom reference (values-test-kustomization), got %d", len(result.Spec.PostBuild.SubstituteFrom))
		}
		if result.Spec.PostBuild.SubstituteFrom[0].Name != "values-test-kustomization" {
			t.Errorf("Expected SubstituteFrom to be values-test-kustomization, got '%s'", result.Spec.PostBuild.SubstituteFrom[0].Name)
		}
	})

	t.Run("WithAllFieldsSet", func(t *testing.T) {
		interval := DurationString{Duration: 5 * time.Minute}
		retryInterval := DurationString{Duration: 2 * time.Minute}
		timeout := DurationString{Duration: 10 * time.Minute}
		wait := true
		force := false
		prune := true
		destroy := false

		kustomization := &Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "custom-source",
			DependsOn:     []string{"dep1", "dep2"},
			Interval:      &interval,
			RetryInterval: &retryInterval,
			Timeout:       &timeout,
			Wait:          &wait,
			Force:         &force,
			Prune:         &prune,
			Destroy:       boolExprPtr(destroy),
			Components:    []string{"comp1", "comp2"},
			Patches: []BlueprintPatch{
				{
					Patch: "apiVersion: v1\nkind: Service\nmetadata:\n  name: test",
					Target: &kustomize.Selector{
						Kind:      "Service",
						Name:      "test",
						Namespace: "test-ns",
					},
				},
			},
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{})

		if result.Spec.SourceRef.Name != "custom-source" {
			t.Errorf("Expected source name 'custom-source', got '%s'", result.Spec.SourceRef.Name)
		}
		if len(result.Spec.DependsOn) != 2 {
			t.Errorf("Expected 2 dependencies, got %d", len(result.Spec.DependsOn))
		}
		if result.Spec.DependsOn[0].Name != "dep1" || result.Spec.DependsOn[0].Namespace != "test-namespace" {
			t.Errorf("Expected dependency dep1 in test-namespace, got %v", result.Spec.DependsOn[0])
		}
		if result.Spec.Interval.Duration != 5*time.Minute {
			t.Errorf("Expected interval 5m, got %v", result.Spec.Interval.Duration)
		}
		if result.Spec.RetryInterval.Duration != 2*time.Minute {
			t.Errorf("Expected retry interval 2m, got %v", result.Spec.RetryInterval.Duration)
		}
		if result.Spec.Timeout.Duration != 10*time.Minute {
			t.Errorf("Expected timeout 10m, got %v", result.Spec.Timeout.Duration)
		}
		if result.Spec.Wait != wait {
			t.Errorf("Expected wait %v, got %v", wait, result.Spec.Wait)
		}
		if result.Spec.Force != force {
			t.Errorf("Expected force %v, got %v", force, result.Spec.Force)
		}
		if result.Spec.Prune != prune {
			t.Errorf("Expected prune %v, got %v", prune, result.Spec.Prune)
		}
		if result.Spec.DeletionPolicy != "MirrorPrune" {
			t.Errorf("Expected deletion policy 'MirrorPrune', got '%s'", result.Spec.DeletionPolicy)
		}
		if len(result.Spec.Components) != 2 {
			t.Errorf("Expected 2 components, got %d", len(result.Spec.Components))
		}
		if len(result.Spec.Patches) != 1 {
			t.Errorf("Expected 1 patch, got %d", len(result.Spec.Patches))
		}
		if result.Spec.Patches[0].Target.Kind != "Service" {
			t.Errorf("Expected patch target kind 'Service', got '%s'", result.Spec.Patches[0].Target.Kind)
		}
	})

	t.Run("WithSubstitutions", func(t *testing.T) {
		kustomization := &Kustomization{
			Name: "test-kustomization",
			Path: "test/path",
			Substitutions: map[string]string{
				"domain": "example.com",
				"region": "us-west-2",
			},
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{})

		if result.Spec.PostBuild == nil {
			t.Fatal("Expected PostBuild to be set when there are substitutions")
		}
		if len(result.Spec.PostBuild.SubstituteFrom) != 1 {
			t.Fatalf("Expected 1 SubstituteFrom reference (values-test-kustomization), got %d", len(result.Spec.PostBuild.SubstituteFrom))
		}
		if result.Spec.PostBuild.SubstituteFrom[0].Name != "values-test-kustomization" {
			t.Errorf("Expected SubstituteFrom to be values-test-kustomization, got '%s'", result.Spec.PostBuild.SubstituteFrom[0].Name)
		}
	})

	t.Run("WithoutSubstitutions", func(t *testing.T) {
		kustomization := &Kustomization{
			Name: "test-kustomization",
			Path: "test/path",
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{})

		// PostBuild should not be set when there are no substitutions
		if result.Spec.PostBuild != nil {
			t.Errorf("Expected PostBuild to be nil when there are no substitutions, got %v", result.Spec.PostBuild)
		}
	})

	t.Run("WithOCISource", func(t *testing.T) {
		kustomization := &Kustomization{
			Name:   "test-kustomization",
			Path:   "test/path",
			Source: "oci-source",
		}

		sources := []Source{
			{
				Name: "oci-source",
				Url:  "oci://example.com/repo",
			},
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", sources)

		if result.Spec.SourceRef.Kind != "OCIRepository" {
			t.Errorf("Expected source kind 'OCIRepository', got '%s'", result.Spec.SourceRef.Kind)
		}
		if result.Spec.SourceRef.Name != "oci-source" {
			t.Errorf("Expected source name 'oci-source', got '%s'", result.Spec.SourceRef.Name)
		}
	})

	t.Run("WithGitSource", func(t *testing.T) {
		kustomization := &Kustomization{
			Name:   "test-kustomization",
			Path:   "test/path",
			Source: "git-source",
		}

		sources := []Source{
			{
				Name: "git-source",
				Url:  "https://example.com/repo.git",
			},
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", sources)

		if result.Spec.SourceRef.Kind != "GitRepository" {
			t.Errorf("Expected source kind 'GitRepository', got '%s'", result.Spec.SourceRef.Kind)
		}
		if result.Spec.SourceRef.Name != "git-source" {
			t.Errorf("Expected source name 'git-source', got '%s'", result.Spec.SourceRef.Name)
		}
	})

	t.Run("WithEmptyPath", func(t *testing.T) {
		kustomization := &Kustomization{
			Name: "test-kustomization",
			Path: "",
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{})

		if result.Spec.Path != "kustomize" {
			t.Errorf("Expected path 'kustomize', got '%s'", result.Spec.Path)
		}
	})

	t.Run("WithPathBackslashes", func(t *testing.T) {
		kustomization := &Kustomization{
			Name: "test-kustomization",
			Path: "test\\path\\with\\backslashes",
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{})

		if result.Spec.Path != "kustomize/test/path/with/backslashes" {
			t.Errorf("Expected path with forward slashes, got '%s'", result.Spec.Path)
		}
	})

	t.Run("WithDestroyTrue", func(t *testing.T) {
		destroy := true
		kustomization := &Kustomization{
			Name:    "test-kustomization",
			Path:    "test/path",
			Destroy: boolExprPtr(destroy),
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{})

		if result.Spec.DeletionPolicy != "WaitForTermination" {
			t.Errorf("Expected deletion policy 'WaitForTermination', got '%s'", result.Spec.DeletionPolicy)
		}
	})

	t.Run("WithDestroyFalse", func(t *testing.T) {
		destroy := false
		kustomization := &Kustomization{
			Name:    "test-kustomization",
			Path:    "test/path",
			Destroy: boolExprPtr(destroy),
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{})

		if result.Spec.DeletionPolicy != "MirrorPrune" {
			t.Errorf("Expected deletion policy 'MirrorPrune', got '%s'", result.Spec.DeletionPolicy)
		}
	})

	t.Run("WithDestroyNil", func(t *testing.T) {
		kustomization := &Kustomization{
			Name:    "test-kustomization",
			Path:    "test/path",
			Destroy: nil,
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{})

		if result.Spec.DeletionPolicy != "WaitForTermination" {
			t.Errorf("Expected deletion policy 'WaitForTermination' when Destroy is nil, got '%s'", result.Spec.DeletionPolicy)
		}
	})

	t.Run("WithEmptySourceUsesDefault", func(t *testing.T) {
		kustomization := &Kustomization{
			Name:   "test-kustomization",
			Path:   "test/path",
			Source: "",
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{})

		if result.Spec.SourceRef.Name != "default-source" {
			t.Errorf("Expected source name 'default-source', got '%s'", result.Spec.SourceRef.Name)
		}
	})

	t.Run("WithTemplateSourceLocalUsesDefault", func(t *testing.T) {
		kustomization := &Kustomization{
			Name:   "test-kustomization",
			Path:   "test/path",
			Source: "template",
		}
		sources := []Source{{Name: "template", Url: ""}}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", sources)

		if result.Spec.SourceRef.Name != "default-source" {
			t.Errorf("Expected source name 'default-source' when template is local, got '%s'", result.Spec.SourceRef.Name)
		}
	})

	t.Run("WithTemplateSourceRemoteUsesTemplate", func(t *testing.T) {
		kustomization := &Kustomization{
			Name:   "test-kustomization",
			Path:   "test/path",
			Source: "template",
		}
		sources := []Source{{Name: "template", Url: "https://github.com/example/template.git"}}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", sources)

		if result.Spec.SourceRef.Name != "template" {
			t.Errorf("Expected source name 'template' when template is remote, got '%s'", result.Spec.SourceRef.Name)
		}
	})

	t.Run("WithZeroIntervalUsesDefault", func(t *testing.T) {
		zeroInterval := DurationString{Duration: 0}
		kustomization := &Kustomization{
			Name:     "test-kustomization",
			Path:     "test/path",
			Interval: &zeroInterval,
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{})

		if result.Spec.Interval.Duration != constants.DefaultFluxKustomizationInterval {
			t.Errorf("Expected default interval, got %v", result.Spec.Interval.Duration)
		}
	})

	t.Run("WithPatchesWithoutTarget", func(t *testing.T) {
		kustomization := &Kustomization{
			Name: "test-kustomization",
			Path: "test/path",
			Patches: []BlueprintPatch{
				{
					Patch: "apiVersion: v1\nkind: Service\nmetadata:\n  name: test",
				},
			},
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{})

		if len(result.Spec.Patches) != 1 {
			t.Fatalf("Expected 1 patch, got %d", len(result.Spec.Patches))
		}
		if result.Spec.Patches[0].Target != nil {
			t.Error("Expected patch target to be nil")
		}
		if result.Spec.Patches[0].Patch != "apiVersion: v1\nkind: Service\nmetadata:\n  name: test" {
			t.Errorf("Expected patch content, got '%s'", result.Spec.Patches[0].Patch)
		}
	})

	t.Run("WithEmptyPatchIgnored", func(t *testing.T) {
		kustomization := &Kustomization{
			Name: "test-kustomization",
			Path: "test/path",
			Patches: []BlueprintPatch{
				{
					Patch: "",
				},
			},
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{})

		if len(result.Spec.Patches) != 0 {
			t.Errorf("Expected 0 patches (empty patch ignored), got %d", len(result.Spec.Patches))
		}
	})

	t.Run("TypeMetaAndObjectMeta", func(t *testing.T) {
		kustomization := &Kustomization{
			Name: "test-kustomization",
			Path: "test/path",
		}

		result := kustomization.ToFluxKustomization("test-namespace", "default-source", []Source{})

		if result.Kind != "Kustomization" {
			t.Errorf("Expected Kind 'Kustomization', got '%s'", result.Kind)
		}
		if result.APIVersion != "kustomize.toolkit.fluxcd.io/v1" {
			t.Errorf("Expected APIVersion 'kustomize.toolkit.fluxcd.io/v1', got '%s'", result.APIVersion)
		}
		if result.Name != "test-kustomization" {
			t.Errorf("Expected Name 'test-kustomization', got '%s'", result.Name)
		}
		if result.Namespace != "test-namespace" {
			t.Errorf("Expected Namespace 'test-namespace', got '%s'", result.Namespace)
		}
	})
}

func TestTerraformComponent_DeepCopy(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		component := &TerraformComponent{
			Source:      "test-source",
			Path:        "test/path",
			FullPath:    "/full/test/path",
			DependsOn:   []string{"dep1", "dep2"},
			Inputs:      map[string]any{"key1": "value1", "key2": 42},
			Destroy:     boolExprPtr(true),
			Parallelism: intExprPtr(3),
		}

		copy := component.DeepCopy()

		if copy.Source != "test-source" {
			t.Errorf("Expected source 'test-source', got '%s'", copy.Source)
		}
		if copy.Path != "test/path" {
			t.Errorf("Expected path 'test/path', got '%s'", copy.Path)
		}
		if copy.FullPath != "/full/test/path" {
			t.Errorf("Expected full path '/full/test/path', got '%s'", copy.FullPath)
		}
		if len(copy.DependsOn) != 2 {
			t.Errorf("Expected 2 dependencies, got %d", len(copy.DependsOn))
		}
		if copy.DependsOn[0] != "dep1" || copy.DependsOn[1] != "dep2" {
			t.Errorf("Expected dependencies ['dep1', 'dep2'], got %v", copy.DependsOn)
		}
		if len(copy.Inputs) != 2 {
			t.Errorf("Expected 2 inputs, got %d", len(copy.Inputs))
		}
		if copy.Inputs["key1"] != "value1" {
			t.Errorf("Expected input key1='value1', got '%v'", copy.Inputs["key1"])
		}
		if copy.Inputs["key2"] != 42 {
			t.Errorf("Expected input key2=42, got '%v'", copy.Inputs["key2"])
		}
		destroy := copy.Destroy.ToBool()
		if destroy == nil || *destroy != true {
			t.Errorf("Expected destroy=true, got %v", destroy)
		}
		parallelism := copy.Parallelism.ToInt()
		if parallelism == nil || *parallelism != 3 {
			t.Errorf("Expected parallelism=3, got %v", parallelism)
		}

		// Verify it's a deep copy (modifying copy shouldn't affect original)
		copy.Inputs["key3"] = "new-value"
		if _, exists := component.Inputs["key3"]; exists {
			t.Error("Expected modifying copy inputs not to affect original")
		}
		copy.DependsOn = append(copy.DependsOn, "dep3")
		if len(component.DependsOn) != 2 {
			t.Error("Expected modifying copy dependencies not to affect original")
		}
	})

	t.Run("NilComponent", func(t *testing.T) {
		var component *TerraformComponent
		copy := component.DeepCopy()
		if copy != nil {
			t.Errorf("Expected nil copy, got non-nil")
		}
	})

	t.Run("EmptyFields", func(t *testing.T) {
		component := &TerraformComponent{
			Source: "test-source",
			Path:   "test/path",
		}

		copy := component.DeepCopy()

		if copy.Source != "test-source" {
			t.Errorf("Expected source 'test-source', got '%s'", copy.Source)
		}
		if copy.Path != "test/path" {
			t.Errorf("Expected path 'test/path', got '%s'", copy.Path)
		}
		if len(copy.DependsOn) != 0 {
			t.Errorf("Expected empty dependsOn, got %v", copy.DependsOn)
		}
		if len(copy.Inputs) != 0 {
			t.Errorf("Expected empty inputs, got %v", copy.Inputs)
		}
		if copy.Destroy != nil {
			t.Errorf("Expected nil destroy, got %v", copy.Destroy)
		}
		if copy.Parallelism != nil {
			t.Errorf("Expected nil parallelism, got %v", copy.Parallelism)
		}
	})

	t.Run("IncludesNameInDeepCopy", func(t *testing.T) {
		component := &TerraformComponent{
			Name:   "test-component",
			Source: "test-source",
			Path:   "test/path",
		}

		copy := component.DeepCopy()

		if copy.Name != "test-component" {
			t.Errorf("Expected name 'test-component', got '%s'", copy.Name)
		}
	})
}

func TestTerraformComponent_GetID(t *testing.T) {
	t.Run("ReturnsNameWhenPresent", func(t *testing.T) {
		component := &TerraformComponent{
			Name: "my-component",
			Path: "terraform/path",
		}

		id := component.GetID()

		if id != "my-component" {
			t.Errorf("Expected ID 'my-component', got '%s'", id)
		}
	})

	t.Run("ReturnsPathWhenNameNotPresent", func(t *testing.T) {
		component := &TerraformComponent{
			Path: "terraform/path",
		}

		id := component.GetID()

		if id != "terraform/path" {
			t.Errorf("Expected ID 'terraform/path', got '%s'", id)
		}
	})

	t.Run("ReturnsPathWhenNameIsEmpty", func(t *testing.T) {
		component := &TerraformComponent{
			Name: "",
			Path: "terraform/path",
		}

		id := component.GetID()

		if id != "terraform/path" {
			t.Errorf("Expected ID 'terraform/path', got '%s'", id)
		}
	})
}

func TestKustomization_DeepCopy(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		interval := DurationString{Duration: 5 * time.Minute}
		retryInterval := DurationString{Duration: 2 * time.Minute}
		timeout := DurationString{Duration: 10 * time.Minute}
		wait := true
		force := false
		prune := true
		destroy := false
		destroyOnly := true

		kustomization := &Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "test-source",
			DependsOn:     []string{"dep1", "dep2"},
			Interval:      &interval,
			RetryInterval: &retryInterval,
			Timeout:       &timeout,
			Patches: []BlueprintPatch{
				{
					Patch: "test-patch",
					Target: &kustomize.Selector{
						Kind: "Service",
						Name: "test",
					},
				},
			},
			Wait:          &wait,
			Force:         &force,
			Prune:         &prune,
			Components:    []string{"comp1", "comp2"},
			Cleanup:       []string{"cleanup1"},
			Destroy:       boolExprPtr(destroy),
			DestroyOnly:   &destroyOnly,
			Substitutions: map[string]string{"key1": "value1", "key2": "value2"},
		}

		copy := kustomization.DeepCopy()

		if copy.Name != "test-kustomization" {
			t.Errorf("Expected name 'test-kustomization', got '%s'", copy.Name)
		}
		if copy.Path != "test/path" {
			t.Errorf("Expected path 'test/path', got '%s'", copy.Path)
		}
		if copy.Source != "test-source" {
			t.Errorf("Expected source 'test-source', got '%s'", copy.Source)
		}
		if len(copy.DependsOn) != 2 {
			t.Errorf("Expected 2 dependencies, got %d", len(copy.DependsOn))
		}
		if copy.DependsOn[0] != "dep1" || copy.DependsOn[1] != "dep2" {
			t.Errorf("Expected dependencies ['dep1', 'dep2'], got %v", copy.DependsOn)
		}
		if copy.Interval == nil || copy.Interval.Duration != 5*time.Minute {
			t.Errorf("Expected interval 5m, got %v", copy.Interval)
		}
		if copy.RetryInterval == nil || copy.RetryInterval.Duration != 2*time.Minute {
			t.Errorf("Expected retry interval 2m, got %v", copy.RetryInterval)
		}
		if copy.Timeout == nil || copy.Timeout.Duration != 10*time.Minute {
			t.Errorf("Expected timeout 10m, got %v", copy.Timeout)
		}
		if len(copy.Patches) != 1 {
			t.Errorf("Expected 1 patch, got %d", len(copy.Patches))
		}
		if copy.Patches[0].Patch != "test-patch" {
			t.Errorf("Expected patch 'test-patch', got '%s'", copy.Patches[0].Patch)
		}
		if copy.Wait == nil || *copy.Wait != true {
			t.Errorf("Expected wait=true, got %v", copy.Wait)
		}
		if copy.Force == nil || *copy.Force != false {
			t.Errorf("Expected force=false, got %v", copy.Force)
		}
		if copy.Prune == nil || *copy.Prune != true {
			t.Errorf("Expected prune=true, got %v", copy.Prune)
		}
		if len(copy.Components) != 2 {
			t.Errorf("Expected 2 components, got %d", len(copy.Components))
		}
		if copy.Components[0] != "comp1" || copy.Components[1] != "comp2" {
			t.Errorf("Expected components ['comp1', 'comp2'], got %v", copy.Components)
		}
		if len(copy.Cleanup) != 1 {
			t.Errorf("Expected 1 cleanup, got %d", len(copy.Cleanup))
		}
		if copy.Cleanup[0] != "cleanup1" {
			t.Errorf("Expected cleanup ['cleanup1'], got %v", copy.Cleanup)
		}
		destroyVal := copy.Destroy.ToBool()
		if destroyVal == nil || *destroyVal != false {
			t.Errorf("Expected destroy=false, got %v", destroyVal)
		}
		if copy.DestroyOnly == nil || *copy.DestroyOnly != true {
			t.Errorf("Expected destroyOnly=true, got %v", copy.DestroyOnly)
		}
		if len(copy.Substitutions) != 2 {
			t.Errorf("Expected 2 substitutions, got %d", len(copy.Substitutions))
		}
		if copy.Substitutions["key1"] != "value1" || copy.Substitutions["key2"] != "value2" {
			t.Errorf("Expected substitutions map[key1:value1 key2:value2], got %v", copy.Substitutions)
		}

		// Verify it's a deep copy (modifying copy shouldn't affect original)
		copy.DependsOn = append(copy.DependsOn, "dep3")
		if len(kustomization.DependsOn) != 2 {
			t.Error("Expected modifying copy dependencies not to affect original")
		}
		copy.Components = append(copy.Components, "comp3")
		if len(kustomization.Components) != 2 {
			t.Error("Expected modifying copy components not to affect original")
		}
		copy.Substitutions["key3"] = "value3"
		if _, exists := kustomization.Substitutions["key3"]; exists {
			t.Error("Expected modifying copy substitutions not to affect original")
		}
	})

	t.Run("NilKustomization", func(t *testing.T) {
		var kustomization *Kustomization
		copy := kustomization.DeepCopy()
		if copy != nil {
			t.Errorf("Expected nil copy, got non-nil")
		}
	})

	t.Run("EmptyFields", func(t *testing.T) {
		kustomization := &Kustomization{
			Name:   "test-kustomization",
			Path:   "test/path",
			Source: "test-source",
		}

		copy := kustomization.DeepCopy()

		if copy.Name != "test-kustomization" {
			t.Errorf("Expected name 'test-kustomization', got '%s'", copy.Name)
		}
		if copy.Path != "test/path" {
			t.Errorf("Expected path 'test/path', got '%s'", copy.Path)
		}
		if copy.Source != "test-source" {
			t.Errorf("Expected source 'test-source', got '%s'", copy.Source)
		}
		if len(copy.DependsOn) != 0 {
			t.Errorf("Expected empty dependsOn, got %v", copy.DependsOn)
		}
		if len(copy.Patches) != 0 {
			t.Errorf("Expected empty patches, got %v", copy.Patches)
		}
		if len(copy.Components) != 0 {
			t.Errorf("Expected empty components, got %v", copy.Components)
		}
		if len(copy.Cleanup) != 0 {
			t.Errorf("Expected empty cleanup, got %v", copy.Cleanup)
		}
		if len(copy.Substitutions) != 0 {
			t.Errorf("Expected empty substitutions, got %v", copy.Substitutions)
		}
	})
}

func TestBlueprint_validateTerraformComponents(t *testing.T) {
	t.Run("ValidatesUniqueComponentIDs", func(t *testing.T) {
		blueprint := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Name: "component-a",
					Path: "path/a",
				},
				{
					Name: "component-b",
					Path: "path/b",
				},
			},
		}

		err := blueprint.validateTerraformComponents()

		if err != nil {
			t.Errorf("Expected no error for unique component IDs, got %v", err)
		}
	})

	t.Run("RejectsDuplicateNames", func(t *testing.T) {
		blueprint := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Name: "duplicate",
					Path: "path/a",
				},
				{
					Name: "duplicate",
					Path: "path/b",
				},
			},
		}

		err := blueprint.validateTerraformComponents()

		if err == nil {
			t.Error("Expected error for duplicate component names, got nil")
		}
		if !strings.Contains(err.Error(), "duplicate") {
			t.Errorf("Expected error message to contain 'duplicate', got '%s'", err.Error())
		}
	})

	t.Run("RejectsDuplicatePaths", func(t *testing.T) {
		blueprint := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Path: "duplicate/path",
				},
				{
					Path: "duplicate/path",
				},
			},
		}

		err := blueprint.validateTerraformComponents()

		if err == nil {
			t.Error("Expected error for duplicate component paths, got nil")
		}
		if !strings.Contains(err.Error(), "duplicate") {
			t.Errorf("Expected error message to contain 'duplicate', got '%s'", err.Error())
		}
	})

	t.Run("RejectsNameMatchingPath", func(t *testing.T) {
		blueprint := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Name: "same-id",
					Path: "path/a",
				},
				{
					Path: "same-id",
				},
			},
		}

		err := blueprint.validateTerraformComponents()

		if err == nil {
			t.Error("Expected error when component name matches another component's path, got nil")
		}
		if !strings.Contains(err.Error(), "duplicate") {
			t.Errorf("Expected error message to contain 'duplicate', got '%s'", err.Error())
		}
	})

	t.Run("RejectsSamePathWithDifferentSources", func(t *testing.T) {
		blueprint := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Source: "source-a",
					Path:   "same/path",
				},
				{
					Source: "source-b",
					Path:   "same/path",
				},
			},
		}

		err := blueprint.validateTerraformComponents()

		if err == nil {
			t.Error("Expected error for same path with different sources (Path must be unique regardless of Source), got nil")
		}
		if !strings.Contains(err.Error(), "duplicate") {
			t.Errorf("Expected error message to contain 'duplicate', got '%s'", err.Error())
		}
	})
}

func TestKustomization_UnmarshalYAML(t *testing.T) {
	t.Run("UnmarshalsDurationStrings", func(t *testing.T) {
		yamlData := []byte(`name: test-kustomization
path: test/path
interval: 5m
retryInterval: 2m
timeout: 10m
`)

		var k Kustomization
		err := yaml.Unmarshal(yamlData, &k)
		if err != nil {
			t.Fatalf("Failed to unmarshal Kustomization with duration strings: %v", err)
		}

		if k.Name != "test-kustomization" {
			t.Errorf("Expected Name 'test-kustomization', got %q", k.Name)
		}
		if k.Path != "test/path" {
			t.Errorf("Expected Path 'test/path', got %q", k.Path)
		}

		if k.Interval == nil {
			t.Error("Expected Interval to be set, got nil")
		} else if k.Interval.Duration != 5*time.Minute {
			t.Errorf("Expected Interval duration 5m, got %v", k.Interval.Duration)
		}

		if k.RetryInterval == nil {
			t.Error("Expected RetryInterval to be set, got nil")
		} else if k.RetryInterval.Duration != 2*time.Minute {
			t.Errorf("Expected RetryInterval duration 2m, got %v", k.RetryInterval.Duration)
		}

		if k.Timeout == nil {
			t.Error("Expected Timeout to be set, got nil")
		} else if k.Timeout.Duration != 10*time.Minute {
			t.Errorf("Expected Timeout duration 10m, got %v", k.Timeout.Duration)
		}
	})

	t.Run("UnmarshalsDurationObjects", func(t *testing.T) {
		yamlData := []byte(`name: test-kustomization
path: test/path
interval:
  duration: 5m0s
retryInterval:
  duration: 2m0s
timeout:
  duration: 10m0s
`)

		var k Kustomization
		err := yaml.Unmarshal(yamlData, &k)
		if err != nil {
			t.Fatalf("Failed to unmarshal Kustomization with duration objects: %v", err)
		}

		if k.Interval == nil {
			t.Error("Expected Interval to be set, got nil")
		} else if k.Interval.Duration != 5*time.Minute {
			t.Errorf("Expected Interval duration 5m, got %v", k.Interval.Duration)
		}

		if k.RetryInterval == nil {
			t.Error("Expected RetryInterval to be set, got nil")
		} else if k.RetryInterval.Duration != 2*time.Minute {
			t.Errorf("Expected RetryInterval duration 2m, got %v", k.RetryInterval.Duration)
		}

		if k.Timeout == nil {
			t.Error("Expected Timeout to be set, got nil")
		} else if k.Timeout.Duration != 10*time.Minute {
			t.Errorf("Expected Timeout duration 10m, got %v", k.Timeout.Duration)
		}
	})
}

func TestBoolExpression_MarshalYAML(t *testing.T) {
	t.Run("PreservesExpressionInRoundTrip", func(t *testing.T) {
		yamlData := []byte(`destroy: "${shouldDestroy}"`)

		var component struct {
			Destroy *BoolExpression `yaml:"destroy,omitempty"`
		}
		err := yaml.Unmarshal(yamlData, &component)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if component.Destroy == nil {
			t.Fatal("Expected Destroy to be set")
		}
		if !component.Destroy.IsExpr {
			t.Error("Expected IsExpr to be true")
		}
		if component.Destroy.Expr != "${shouldDestroy}" {
			t.Errorf("Expected Expr to be '${shouldDestroy}', got %q", component.Destroy.Expr)
		}
		if component.Destroy.Value != nil {
			t.Error("Expected Value to be nil for expression")
		}

		marshaled, err := yaml.Marshal(&component)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var roundTrip struct {
			Destroy *BoolExpression `yaml:"destroy,omitempty"`
		}
		err = yaml.Unmarshal(marshaled, &roundTrip)
		if err != nil {
			t.Fatalf("Failed to unmarshal round-trip: %v", err)
		}

		if roundTrip.Destroy == nil {
			t.Fatal("Expected Destroy to be preserved after round-trip")
		}
		if !roundTrip.Destroy.IsExpr {
			t.Error("Expected IsExpr to be true after round-trip")
		}
		if roundTrip.Destroy.Expr != "${shouldDestroy}" {
			t.Errorf("Expected Expr to be preserved as '${shouldDestroy}', got %q", roundTrip.Destroy.Expr)
		}
	})

	t.Run("PreservesBooleanValueInRoundTrip", func(t *testing.T) {
		yamlData := []byte(`destroy: true`)

		var component struct {
			Destroy *BoolExpression `yaml:"destroy,omitempty"`
		}
		err := yaml.Unmarshal(yamlData, &component)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if component.Destroy == nil {
			t.Fatal("Expected Destroy to be set")
		}
		if component.Destroy.IsExpr {
			t.Error("Expected IsExpr to be false")
		}
		if component.Destroy.Value == nil || !*component.Destroy.Value {
			t.Error("Expected Value to be true")
		}

		marshaled, err := yaml.Marshal(&component)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var roundTrip struct {
			Destroy *BoolExpression `yaml:"destroy,omitempty"`
		}
		err = yaml.Unmarshal(marshaled, &roundTrip)
		if err != nil {
			t.Fatalf("Failed to unmarshal round-trip: %v", err)
		}

		if roundTrip.Destroy == nil {
			t.Fatal("Expected Destroy to be preserved after round-trip")
		}
		if roundTrip.Destroy.IsExpr {
			t.Error("Expected IsExpr to be false after round-trip")
		}
		if roundTrip.Destroy.Value == nil || !*roundTrip.Destroy.Value {
			t.Error("Expected Value to be true after round-trip")
		}
	})
}

func TestIntExpression_MarshalYAML(t *testing.T) {
	t.Run("PreservesExpressionInRoundTrip", func(t *testing.T) {
		yamlData := []byte(`parallelism: "${cluster.parallelism ?? 10}"`)

		var component struct {
			Parallelism *IntExpression `yaml:"parallelism,omitempty"`
		}
		err := yaml.Unmarshal(yamlData, &component)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if component.Parallelism == nil {
			t.Fatal("Expected Parallelism to be set")
		}
		if !component.Parallelism.IsExpr {
			t.Error("Expected IsExpr to be true")
		}
		if component.Parallelism.Expr != "${cluster.parallelism ?? 10}" {
			t.Errorf("Expected Expr to be '${cluster.parallelism ?? 10}', got %q", component.Parallelism.Expr)
		}
		if component.Parallelism.Value != nil {
			t.Error("Expected Value to be nil for expression")
		}

		marshaled, err := yaml.Marshal(&component)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var roundTrip struct {
			Parallelism *IntExpression `yaml:"parallelism,omitempty"`
		}
		err = yaml.Unmarshal(marshaled, &roundTrip)
		if err != nil {
			t.Fatalf("Failed to unmarshal round-trip: %v", err)
		}

		if roundTrip.Parallelism == nil {
			t.Fatal("Expected Parallelism to be preserved after round-trip")
		}
		if !roundTrip.Parallelism.IsExpr {
			t.Error("Expected IsExpr to be true after round-trip")
		}
		if roundTrip.Parallelism.Expr != "${cluster.parallelism ?? 10}" {
			t.Errorf("Expected Expr to be preserved as '${cluster.parallelism ?? 10}', got %q", roundTrip.Parallelism.Expr)
		}
	})

	t.Run("PreservesIntegerValueInRoundTrip", func(t *testing.T) {
		yamlData := []byte(`parallelism: 5`)

		var component struct {
			Parallelism *IntExpression `yaml:"parallelism,omitempty"`
		}
		err := yaml.Unmarshal(yamlData, &component)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if component.Parallelism == nil {
			t.Fatal("Expected Parallelism to be set")
		}
		if component.Parallelism.IsExpr {
			t.Error("Expected IsExpr to be false")
		}
		if component.Parallelism.Value == nil || *component.Parallelism.Value != 5 {
			t.Error("Expected Value to be 5")
		}

		marshaled, err := yaml.Marshal(&component)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var roundTrip struct {
			Parallelism *IntExpression `yaml:"parallelism,omitempty"`
		}
		err = yaml.Unmarshal(marshaled, &roundTrip)
		if err != nil {
			t.Fatalf("Failed to unmarshal round-trip: %v", err)
		}

		if roundTrip.Parallelism == nil {
			t.Fatal("Expected Parallelism to be preserved after round-trip")
		}
		if roundTrip.Parallelism.IsExpr {
			t.Error("Expected IsExpr to be false after round-trip")
		}
		if roundTrip.Parallelism.Value == nil || *roundTrip.Parallelism.Value != 5 {
			t.Error("Expected Value to be 5 after round-trip")
		}
	})
}

func TestSource_Install(t *testing.T) {
	t.Run("IsNilWhenNotSet", func(t *testing.T) {
		source := Source{
			Name: "test-source",
			Url:  "oci://example.com/repo:tag",
		}

		if source.Install != nil {
			t.Error("Expected Install to be nil when not set")
		}
	})

	t.Run("CanBeSetToFalse", func(t *testing.T) {
		falseVal := false
		source := Source{
			Name:    "test-source",
			Url:     "oci://example.com/repo:tag",
			Install: &BoolExpression{Value: &falseVal, IsExpr: false},
		}

		if source.Install == nil {
			t.Fatal("Expected Install to be set")
		}
		if source.Install.Value == nil || *source.Install.Value {
			t.Error("Expected Install to be false")
		}
	})

	t.Run("SupportsExpression", func(t *testing.T) {
		yamlData := []byte(`name: test-source
url: oci://example.com/repo:tag
install: "${some.condition ?? true}"`)

		var source Source
		err := yaml.Unmarshal(yamlData, &source)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if source.Install == nil {
			t.Fatal("Expected Install to be set")
		}
		if !source.Install.IsExpr {
			t.Error("Expected IsExpr to be true")
		}
		if source.Install.Expr != "${some.condition ?? true}" {
			t.Errorf("Expected Expr to be '${some.condition ?? true}', got %q", source.Install.Expr)
		}
		if source.Install.Value != nil {
			t.Error("Expected Value to be nil for expression")
		}
	})

	t.Run("PreservesValueInRoundTrip", func(t *testing.T) {
		falseVal := false
		source := Source{
			Name:    "test-source",
			Url:     "oci://example.com/repo:tag",
			Install: &BoolExpression{Value: &falseVal, IsExpr: false},
		}

		marshaled, err := yaml.Marshal(&source)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var roundTrip Source
		err = yaml.Unmarshal(marshaled, &roundTrip)
		if err != nil {
			t.Fatalf("Failed to unmarshal round-trip: %v", err)
		}

		if roundTrip.Install == nil {
			t.Fatal("Expected Install to be preserved after round-trip")
		}
		if roundTrip.Install.Value == nil || *roundTrip.Install.Value {
			t.Error("Expected Install to be false after round-trip")
		}
	})
}

func TestTerraformComponent_Enabled(t *testing.T) {
	t.Run("IsNilWhenNotSet", func(t *testing.T) {
		component := TerraformComponent{
			Path: "test/path",
		}

		if component.Enabled != nil {
			t.Error("Expected Enabled to be nil when not set")
		}
	})

	t.Run("CanBeSetToFalse", func(t *testing.T) {
		falseVal := false
		component := TerraformComponent{
			Path:    "test/path",
			Enabled: &BoolExpression{Value: &falseVal, IsExpr: false},
		}

		if component.Enabled == nil {
			t.Fatal("Expected Enabled to be set")
		}
		if component.Enabled.Value == nil || *component.Enabled.Value {
			t.Error("Expected Enabled to be false")
		}
	})

	t.Run("SupportsExpression", func(t *testing.T) {
		yamlData := []byte(`path: test/path
enabled: "${cluster.enabled ?? true}"`)

		var component TerraformComponent
		err := yaml.Unmarshal(yamlData, &component)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if component.Enabled == nil {
			t.Fatal("Expected Enabled to be set")
		}
		if !component.Enabled.IsExpr {
			t.Error("Expected IsExpr to be true")
		}
		if component.Enabled.Expr != "${cluster.enabled ?? true}" {
			t.Errorf("Expected Expr to be '${cluster.enabled ?? true}', got %q", component.Enabled.Expr)
		}
		if component.Enabled.Value != nil {
			t.Error("Expected Value to be nil for expression")
		}
	})

	t.Run("PreservesValueInRoundTrip", func(t *testing.T) {
		falseVal := false
		component := TerraformComponent{
			Path:    "test/path",
			Enabled: &BoolExpression{Value: &falseVal, IsExpr: false},
		}

		marshaled, err := yaml.Marshal(&component)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var roundTrip TerraformComponent
		err = yaml.Unmarshal(marshaled, &roundTrip)
		if err != nil {
			t.Fatalf("Failed to unmarshal round-trip: %v", err)
		}

		if roundTrip.Enabled == nil {
			t.Fatal("Expected Enabled to be preserved after round-trip")
		}
		if roundTrip.Enabled.Value == nil || *roundTrip.Enabled.Value {
			t.Error("Expected Enabled to be false after round-trip")
		}
	})

	t.Run("IsCopiedInDeepCopy", func(t *testing.T) {
		falseVal := false
		original := &TerraformComponent{
			Path:    "test/path",
			Enabled: &BoolExpression{Value: &falseVal, IsExpr: false},
		}

		copy := original.DeepCopy()
		if copy == nil {
			t.Fatal("Expected DeepCopy to return non-nil")
		}
		if copy.Enabled == nil {
			t.Fatal("Expected Enabled to be copied")
		}
		if copy.Enabled.Value == nil || *copy.Enabled.Value {
			t.Error("Expected Enabled to be false in copy")
		}

		trueVal := true
		original.Enabled.Value = &trueVal
		if copy.Enabled == nil || copy.Enabled.Value == nil || *copy.Enabled.Value {
			t.Error("Expected copy to be independent - modifying original should not affect copy")
		}
		if copy.Enabled.Value == nil || *copy.Enabled.Value {
			t.Error("Expected copy to still have false value after modifying original")
		}
	})
}

func TestKustomization_Enabled(t *testing.T) {
	t.Run("IsNilWhenNotSet", func(t *testing.T) {
		kustomization := Kustomization{
			Name: "test-kustomization",
		}

		if kustomization.Enabled != nil {
			t.Error("Expected Enabled to be nil when not set")
		}
	})

	t.Run("CanBeSetToFalse", func(t *testing.T) {
		falseVal := false
		kustomization := Kustomization{
			Name:    "test-kustomization",
			Enabled: &BoolExpression{Value: &falseVal, IsExpr: false},
		}

		if kustomization.Enabled == nil {
			t.Fatal("Expected Enabled to be set")
		}
		if kustomization.Enabled.Value == nil || *kustomization.Enabled.Value {
			t.Error("Expected Enabled to be false")
		}
	})

	t.Run("SupportsExpression", func(t *testing.T) {
		yamlData := []byte(`name: test-kustomization
enabled: "${observability.enabled ?? true}"`)

		var kustomization Kustomization
		err := yaml.Unmarshal(yamlData, &kustomization)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if kustomization.Enabled == nil {
			t.Fatal("Expected Enabled to be set")
		}
		if !kustomization.Enabled.IsExpr {
			t.Error("Expected IsExpr to be true")
		}
		if kustomization.Enabled.Expr != "${observability.enabled ?? true}" {
			t.Errorf("Expected Expr to be '${observability.enabled ?? true}', got %q", kustomization.Enabled.Expr)
		}
		if kustomization.Enabled.Value != nil {
			t.Error("Expected Value to be nil for expression")
		}
	})

	t.Run("PreservesValueInRoundTrip", func(t *testing.T) {
		falseVal := false
		kustomization := Kustomization{
			Name:    "test-kustomization",
			Enabled: &BoolExpression{Value: &falseVal, IsExpr: false},
		}

		marshaled, err := yaml.Marshal(&kustomization)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var roundTrip Kustomization
		err = yaml.Unmarshal(marshaled, &roundTrip)
		if err != nil {
			t.Fatalf("Failed to unmarshal round-trip: %v", err)
		}

		if roundTrip.Enabled == nil {
			t.Fatal("Expected Enabled to be preserved after round-trip")
		}
		if roundTrip.Enabled.Value == nil || *roundTrip.Enabled.Value {
			t.Error("Expected Enabled to be false after round-trip")
		}
	})

	t.Run("IsCopiedInDeepCopy", func(t *testing.T) {
		falseVal := false
		original := &Kustomization{
			Name:    "test-kustomization",
			Enabled: &BoolExpression{Value: &falseVal, IsExpr: false},
		}

		copy := original.DeepCopy()
		if copy == nil {
			t.Fatal("Expected DeepCopy to return non-nil")
		}
		if copy.Enabled == nil {
			t.Fatal("Expected Enabled to be copied")
		}
		if copy.Enabled.Value == nil || *copy.Enabled.Value {
			t.Error("Expected Enabled to be false in copy")
		}

		trueVal := true
		original.Enabled.Value = &trueVal
		if copy.Enabled == nil || copy.Enabled.Value == nil || *copy.Enabled.Value {
			t.Error("Expected copy to be independent - modifying original should not affect copy")
		}
		if copy.Enabled.Value == nil || *copy.Enabled.Value {
			t.Error("Expected copy to still have false value after modifying original")
		}
	})
}

func TestBoolExpression_IsEnabled(t *testing.T) {
	t.Run("ReturnsTrueWhenNil", func(t *testing.T) {
		var expr *BoolExpression
		if !expr.IsEnabled() {
			t.Error("Expected IsEnabled to return true when nil")
		}
	})

	t.Run("ReturnsTrueWhenValueIsTrue", func(t *testing.T) {
		trueVal := true
		expr := &BoolExpression{Value: &trueVal, IsExpr: false}
		if !expr.IsEnabled() {
			t.Error("Expected IsEnabled to return true when value is true")
		}
	})

	t.Run("ReturnsFalseWhenValueIsFalse", func(t *testing.T) {
		falseVal := false
		expr := &BoolExpression{Value: &falseVal, IsExpr: false}
		if expr.IsEnabled() {
			t.Error("Expected IsEnabled to return false when value is false")
		}
	})

	t.Run("ReturnsTrueWhenIsExpression", func(t *testing.T) {
		expr := &BoolExpression{IsExpr: true, Expr: "${some.condition ?? true}"}
		if !expr.IsEnabled() {
			t.Error("Expected IsEnabled to return true when IsExpr is true")
		}
	})

	t.Run("ReturnsTrueWhenValueIsNilButNotExpression", func(t *testing.T) {
		expr := &BoolExpression{Value: nil, IsExpr: false}
		if !expr.IsEnabled() {
			t.Error("Expected IsEnabled to return true when Value is nil and not expression")
		}
	})
}

func TestBoolExpression_IsInstalled(t *testing.T) {
	t.Run("ReturnsFalseWhenNil", func(t *testing.T) {
		var expr *BoolExpression
		if expr.IsInstalled() {
			t.Error("Expected IsInstalled to return false when nil")
		}
	})

	t.Run("ReturnsTrueWhenValueIsTrue", func(t *testing.T) {
		trueVal := true
		expr := &BoolExpression{Value: &trueVal, IsExpr: false}
		if !expr.IsInstalled() {
			t.Error("Expected IsInstalled to return true when value is true")
		}
	})

	t.Run("ReturnsFalseWhenValueIsFalse", func(t *testing.T) {
		falseVal := false
		expr := &BoolExpression{Value: &falseVal, IsExpr: false}
		if expr.IsInstalled() {
			t.Error("Expected IsInstalled to return false when value is false")
		}
	})

	t.Run("ReturnsFalseWhenIsExpression", func(t *testing.T) {
		expr := &BoolExpression{IsExpr: true, Expr: "${some.condition ?? true}"}
		if expr.IsInstalled() {
			t.Error("Expected IsInstalled to return false when IsExpr is true")
		}
	})

	t.Run("ReturnsFalseWhenValueIsNilButNotExpression", func(t *testing.T) {
		expr := &BoolExpression{Value: nil, IsExpr: false}
		if expr.IsInstalled() {
			t.Error("Expected IsInstalled to return false when Value is nil and not expression")
		}
	})
}

func TestBlueprint_DeepCopy_WithNewFields(t *testing.T) {
	t.Run("CopiesSourceInstallField", func(t *testing.T) {
		falseVal := false
		blueprint := &Blueprint{
			Sources: []Source{
				{
					Name:    "test-source",
					Url:     "oci://example.com/repo:tag",
					Install: &BoolExpression{Value: &falseVal, IsExpr: false},
				},
			},
		}

		copy := blueprint.DeepCopy()
		if len(copy.Sources) != 1 {
			t.Fatalf("Expected 1 source, got %d", len(copy.Sources))
		}
		if copy.Sources[0].Install == nil {
			t.Fatal("Expected Install to be copied")
		}
		if copy.Sources[0].Install.Value == nil || *copy.Sources[0].Install.Value {
			t.Error("Expected Install to be false in copy")
		}
	})

	t.Run("CopiesTerraformComponentEnabledField", func(t *testing.T) {
		falseVal := false
		blueprint := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Path:    "test/path",
					Enabled: &BoolExpression{Value: &falseVal, IsExpr: false},
				},
			},
		}

		copy := blueprint.DeepCopy()
		if len(copy.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 component, got %d", len(copy.TerraformComponents))
		}
		if copy.TerraformComponents[0].Enabled == nil {
			t.Fatal("Expected Enabled to be copied")
		}
		if copy.TerraformComponents[0].Enabled.Value == nil || *copy.TerraformComponents[0].Enabled.Value {
			t.Error("Expected Enabled to be false in copy")
		}
	})

	t.Run("CopiesKustomizationEnabledField", func(t *testing.T) {
		falseVal := false
		blueprint := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:    "test-kustomization",
					Enabled: &BoolExpression{Value: &falseVal, IsExpr: false},
				},
			},
		}

		copy := blueprint.DeepCopy()
		if len(copy.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(copy.Kustomizations))
		}
		if copy.Kustomizations[0].Enabled == nil {
			t.Fatal("Expected Enabled to be copied")
		}
		if copy.Kustomizations[0].Enabled.Value == nil || *copy.Kustomizations[0].Enabled.Value {
			t.Error("Expected Enabled to be false in copy")
		}
	})
}

func TestToMapStringAny(t *testing.T) {
	t.Run("ReturnsNilForNil", func(t *testing.T) {
		if ToMapStringAny(nil) != nil {
			t.Error("Expected nil for nil input")
		}
	})
	t.Run("ReturnsNilForNonMap", func(t *testing.T) {
		if ToMapStringAny("string") != nil {
			t.Error("Expected nil for string")
		}
		if ToMapStringAny(42) != nil {
			t.Error("Expected nil for int")
		}
	})
	t.Run("ConvertsMapStringAnyIdentity", func(t *testing.T) {
		in := map[string]any{"a": 1, "b": "x"}
		out := ToMapStringAny(in)
		if out == nil {
			t.Fatal("Expected non-nil map")
		}
		if out["a"] != 1 || out["b"] != "x" {
			t.Errorf("Expected a=1, b=x, got %v", out)
		}
	})
	t.Run("ConvertsYAMLMapInterfaceToMapStringAny", func(t *testing.T) {
		var v any
		if err := yaml.Unmarshal([]byte("a: 1\nb: hello\nnested:\n  x: 10"), &v); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		out := ToMapStringAny(v)
		if out == nil {
			t.Fatal("Expected non-nil map")
		}
		if fmt.Sprint(out["a"]) != "1" {
			t.Errorf("Expected a=1, got %v", out["a"])
		}
		if out["b"] != "hello" {
			t.Errorf("Expected b=hello, got %v", out["b"])
		}
		nested, ok := out["nested"].(map[string]any)
		if !ok {
			t.Errorf("Expected nested to be map[string]any, got %T", out["nested"])
		} else if fmt.Sprint(nested["x"]) != "10" {
			t.Errorf("Expected nested.x=10, got %v", nested["x"])
		}
	})
	t.Run("ConvertsNestedMapAndSlice", func(t *testing.T) {
		var v any
		if err := yaml.Unmarshal([]byte("list:\n  - 1\n  - two"), &v); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		out := ToMapStringAny(v)
		if out == nil {
			t.Fatal("Expected non-nil map")
		}
		sl, ok := out["list"].([]any)
		if !ok {
			t.Errorf("Expected list to be []any, got %T", out["list"])
		} else if len(sl) != 2 {
			t.Errorf("Expected list len 2, got %d", len(sl))
		} else if fmt.Sprint(sl[0]) != "1" || sl[1] != "two" {
			t.Errorf("Expected list [1, two], got %v", sl)
		}
	})
}

func TestToSliceAny(t *testing.T) {
	t.Run("ReturnsNilForNil", func(t *testing.T) {
		if ToSliceAny(nil) != nil {
			t.Error("Expected nil for nil input")
		}
	})
	t.Run("ReturnsNilForNonSlice", func(t *testing.T) {
		if ToSliceAny("string") != nil {
			t.Error("Expected nil for string")
		}
		if ToSliceAny(map[string]any{}) != nil {
			t.Error("Expected nil for map")
		}
	})
	t.Run("ConvertsSliceIdentity", func(t *testing.T) {
		in := []any{1, "x", true}
		out := ToSliceAny(in)
		if out == nil {
			t.Fatal("Expected non-nil slice")
		}
		if len(out) != 3 || out[0] != 1 || out[1] != "x" || out[2] != true {
			t.Errorf("Expected [1, x, true], got %v", out)
		}
	})
	t.Run("ConvertsYAMLSliceToSliceAny", func(t *testing.T) {
		var v any
		if err := yaml.Unmarshal([]byte("[1, two, 3]"), &v); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		out := ToSliceAny(v)
		if out == nil {
			t.Fatal("Expected non-nil slice")
		}
		if len(out) != 3 {
			t.Errorf("Expected len 3, got %d", len(out))
		} else if fmt.Sprint(out[0]) != "1" || out[1] != "two" || fmt.Sprint(out[2]) != "3" {
			t.Errorf("Expected [1, two, 3], got %v", out)
		}
	})
	t.Run("ConvertsNestedSliceAndMap", func(t *testing.T) {
		var v any
		if err := yaml.Unmarshal([]byte("- a: 1\n  b: 2\n- 3"), &v); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		out := ToSliceAny(v)
		if out == nil {
			t.Fatal("Expected non-nil slice")
		}
		if len(out) != 2 {
			t.Fatalf("Expected len 2, got %d", len(out))
		}
		first, ok := out[0].(map[string]any)
		if !ok {
			t.Errorf("Expected first element map[string]any, got %T", out[0])
		} else if fmt.Sprint(first["a"]) != "1" || fmt.Sprint(first["b"]) != "2" {
			t.Errorf("Expected first map a=1 b=2, got %v", first)
		}
		if fmt.Sprint(out[1]) != "3" {
			t.Errorf("Expected second element 3, got %v", out[1])
		}
	})
}
