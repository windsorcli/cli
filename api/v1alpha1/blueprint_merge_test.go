package v1alpha1

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
)

// =============================================================================
// Test Public Methods
// =============================================================================

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

	t.Run("PreservesNestedInputOriginsAcrossDeepMergedTopLevelKeys", func(t *testing.T) {
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Name:   "cluster",
					Path:   "terraform/cluster",
					Source: "core",
					Inputs: map[string]any{
						"config": map[string]any{
							"from_base": "${file(\"./base.yaml\")}",
						},
					},
					InputOrigins: map[string]string{
						"config": "facets/base/facet.yaml",
					},
				},
			},
		}

		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Name:   "cluster",
					Path:   "terraform/cluster",
					Source: "core",
					Inputs: map[string]any{
						"config": map[string]any{
							"from_overlay": "${file(\"./overlay.yaml\")}",
						},
					},
					InputOrigins: map[string]string{
						"config": "facets/overlay/facet.yaml",
					},
				},
			},
		}

		base.StrategicMerge(overlay)

		if len(base.TerraformComponents) != 1 {
			t.Fatalf("Expected one merged terraform component, got %d", len(base.TerraformComponents))
		}

		merged := base.TerraformComponents[0]
		config, ok := merged.Inputs["config"].(map[string]any)
		if !ok {
			t.Fatalf("Expected merged input key 'config' to be a map, got %T", merged.Inputs["config"])
		}

		if _, ok := config["from_base"]; !ok {
			t.Fatalf("Expected merged config map to preserve base key")
		}
		if _, ok := config["from_overlay"]; !ok {
			t.Fatalf("Expected merged config map to include overlay key")
		}

		if got := merged.InputOrigins["config.from_base"]; got != "facets/base/facet.yaml" {
			t.Fatalf("Expected origin for config.from_base to be facets/base/facet.yaml, got %q", got)
		}
		if got := merged.InputOrigins["config.from_overlay"]; got != "facets/overlay/facet.yaml" {
			t.Fatalf("Expected origin for config.from_overlay to be facets/overlay/facet.yaml, got %q", got)
		}
		if _, hasTopLevel := merged.InputOrigins["config"]; hasTopLevel {
			t.Fatalf("Expected top-level config origin entry to be expanded and removed")
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

	t.Run("TerraformFiltersEmptyStringDependsOn", func(t *testing.T) {
		// Given a base blueprint with a terraform component
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{Path: "network", Source: "core", DependsOn: []string{"backend"}},
			},
		}

		// When merging with an overlay that has an empty-string dependsOn
		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{Path: "network", Source: "core", DependsOn: []string{"", "security"}},
			},
		}

		base.StrategicMerge(overlay)

		// Then the empty string should not be added to dependsOn
		comp := base.TerraformComponents[0]
		for _, dep := range comp.DependsOn {
			if dep == "" {
				t.Errorf("Expected empty string to be filtered from dependsOn, got %v", comp.DependsOn)
			}
		}
		if len(comp.DependsOn) != 2 {
			t.Errorf("Expected 2 dependencies, got %d: %v", len(comp.DependsOn), comp.DependsOn)
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

	t.Run("MergesKustomizationPruneWaitForceFields", func(t *testing.T) {
		// Given a base kustomization with prune/wait/force unset
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{Name: "crds"},
			},
		}

		// And an overlay that sets prune=false, wait=true, force=true on the same name
		prune := false
		wait := true
		force := true
		overlay := &Blueprint{
			Kustomizations: []Kustomization{
				{Name: "crds", Prune: &prune, Wait: &wait, Force: &force},
			},
		}

		// When strategic merging
		base.StrategicMerge(overlay)

		// Then the overlay's pointer fields are adopted (not silently dropped)
		merged := base.Kustomizations[0]
		if merged.Prune == nil || *merged.Prune != false {
			t.Errorf("Expected prune=false to survive merge, got %v", merged.Prune)
		}
		if merged.Wait == nil || *merged.Wait != true {
			t.Errorf("Expected wait=true to survive merge, got %v", merged.Wait)
		}
		if merged.Force == nil || *merged.Force != true {
			t.Errorf("Expected force=true to survive merge, got %v", merged.Force)
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

	t.Run("MergesBackendWhenOverlaySetsIt", func(t *testing.T) {
		// Given a base blueprint without a backend declaration
		base := &Blueprint{}

		// And an overlay that names a backend component
		overlay := &Blueprint{Backend: "cluster"}

		base.StrategicMerge(overlay)

		if base.Backend != "cluster" {
			t.Errorf("Expected Backend=\"cluster\" after merge, got %q", base.Backend)
		}
	})

	t.Run("OverwritesBackendWhenOverlaySetsIt", func(t *testing.T) {
		// Given a base blueprint with one backend
		base := &Blueprint{Backend: "old-backend"}

		// And an overlay that names a different backend
		overlay := &Blueprint{Backend: "cluster"}

		base.StrategicMerge(overlay)

		if base.Backend != "cluster" {
			t.Errorf("Expected Backend=\"cluster\" after merge, got %q", base.Backend)
		}
	})

	t.Run("PreservesBackendWhenOverlayLeavesItEmpty", func(t *testing.T) {
		// Given a base blueprint with a backend
		base := &Blueprint{Backend: "cluster"}

		// And an overlay that does not set Backend
		overlay := &Blueprint{}

		base.StrategicMerge(overlay)

		if base.Backend != "cluster" {
			t.Errorf("Expected Backend=\"cluster\" preserved, got %q", base.Backend)
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

	t.Run("KustomizationFiltersEmptyStringDependsOn", func(t *testing.T) {
		// Given a base blueprint with a kustomization
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{Name: "test", DependsOn: []string{"dep1"}},
			},
		}

		// When merging with an overlay that has an empty-string dependsOn
		overlay := &Blueprint{
			Kustomizations: []Kustomization{
				{Name: "test", DependsOn: []string{"", "dep2"}},
			},
		}

		base.StrategicMerge(overlay)

		// Then the empty string should not be added to dependsOn
		kustomization := base.Kustomizations[0]
		for _, dep := range kustomization.DependsOn {
			if dep == "" {
				t.Errorf("Expected empty string to be filtered from dependsOn, got %v", kustomization.DependsOn)
			}
		}
		if len(kustomization.DependsOn) != 2 {
			t.Errorf("Expected 2 dependencies, got %d: %v", len(kustomization.DependsOn), kustomization.DependsOn)
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

	t.Run("MergesTopLevelSubstitutionsFromNilBase", func(t *testing.T) {
		// Given a base blueprint with no Substitutions and an overlay with values
		base := &Blueprint{}
		overlay := &Blueprint{
			Substitutions: map[string]string{
				"private_dns": "10.0.0.1",
				"public_dns":  "8.8.8.8",
			},
		}

		// When strategic merging
		base.StrategicMerge(overlay)

		// Then base should have the overlay substitutions
		if base.Substitutions == nil {
			t.Fatal("Expected Substitutions to be initialized")
		}
		if base.Substitutions["private_dns"] != "10.0.0.1" {
			t.Errorf("Expected private_dns='10.0.0.1', got '%s'", base.Substitutions["private_dns"])
		}
		if base.Substitutions["public_dns"] != "8.8.8.8" {
			t.Errorf("Expected public_dns='8.8.8.8', got '%s'", base.Substitutions["public_dns"])
		}
	})

	t.Run("OverlaySubstitutionsOverrideBase", func(t *testing.T) {
		// Given a base blueprint with Substitutions and an overlay with overlapping key
		base := &Blueprint{
			Substitutions: map[string]string{
				"private_dns": "base-value",
				"keep_me":     "unchanged",
			},
		}
		overlay := &Blueprint{
			Substitutions: map[string]string{
				"private_dns": "overlay-value",
			},
		}

		// When strategic merging
		base.StrategicMerge(overlay)

		// Then overlay wins on conflicts, base keys not in overlay are preserved
		if base.Substitutions["private_dns"] != "overlay-value" {
			t.Errorf("Expected overlay to win, got '%s'", base.Substitutions["private_dns"])
		}
		if base.Substitutions["keep_me"] != "unchanged" {
			t.Errorf("Expected base-only key to be preserved, got '%s'", base.Substitutions["keep_me"])
		}
	})

	t.Run("MergesSameNameFluxSystemsAcrossSourcesInsteadOfReplacing", func(t *testing.T) {
		// Given a base blueprint (one source's already-composed facets) with a cert-manager
		// system carrying an install and one resources variant
		base := &Blueprint{
			FluxSystems: []FluxSystem{
				{
					Name:      "cert-manager",
					Install:   &Kustomization{Components: []string{"helm-release"}},
					Resources: []FluxVariant{{Kustomization: Kustomization{Components: []string{"private-issuer/ca"}}}},
				},
			},
		}

		// And an overlay (a second source's already-composed facets) contributing to the same
		// system name with an extra install component and a new named resources variant
		overlay := &Blueprint{
			FluxSystems: []FluxSystem{
				{
					Name:      "cert-manager",
					Install:   &Kustomization{Components: []string{"helm-release-extra-metrics"}},
					Resources: []FluxVariant{{Kustomization: Kustomization{Name: "extra", Components: []string{"public-issuer"}}}},
				},
			},
		}

		// When strategic merging (the path composer.Compose uses to combine per-source blueprints)
		if err := base.StrategicMerge(overlay); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Then the two sources' contributions merge into one system instead of the overlay's
		// source wholesale-replacing the base source's install/resources
		if len(base.FluxSystems) != 1 {
			t.Fatalf("expected one merged cert-manager system, not two colliding sources' entries, got %+v", base.FluxSystems)
		}
		merged := base.FluxSystems[0]
		if merged.Install == nil || !contains(merged.Install.Components, "helm-release") || !contains(merged.Install.Components, "helm-release-extra-metrics") {
			t.Errorf("expected install components to union across sources, got %+v", merged.Install)
		}
		if len(merged.Resources) != 2 {
			t.Fatalf("expected both sources' resources variants to survive, got %+v", merged.Resources)
		}
	})
}

func TestBlueprint_StrategicMerge_Crds(t *testing.T) {
	t.Run("UnionsOverlayRefsDeduped", func(t *testing.T) {
		// Given a base and overlay sharing one local ref
		base := &Blueprint{Crds: []string{"cert-manager-1.16.2"}}
		overlay := &Blueprint{Crds: []string{"cert-manager-1.16.2", "gateway-api-1.5.1"}}

		// When merged
		if err := base.StrategicMerge(overlay); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Then the refs union without duplicating the shared one
		if len(base.Crds) != 2 || base.Crds[0] != "cert-manager-1.16.2" || base.Crds[1] != "gateway-api-1.5.1" {
			t.Errorf("expected deduped union, got %+v", base.Crds)
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

func TestBlueprint_RemoveFluxSystem(t *testing.T) {
	t.Run("RemovesDependsOnFromExistingSystem", func(t *testing.T) {
		base := &Blueprint{
			FluxSystems: []FluxSystem{
				{
					Name:      "gateway",
					DependsOn: []string{"pki-install", "lb-install", "keep_dep"},
				},
			},
		}

		removal := FluxSystem{
			Name:      "gateway",
			DependsOn: []string{"pki-install", "lb-install"},
		}

		err := base.RemoveFluxSystem(removal)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		sys := base.FluxSystems[0]
		if len(sys.DependsOn) != 1 || sys.DependsOn[0] != "keep_dep" {
			t.Errorf("Expected only 'keep_dep' to remain, got %v", sys.DependsOn)
		}
	})

	t.Run("RemovesInstallComponentsFromExistingSystem", func(t *testing.T) {
		base := &Blueprint{
			FluxSystems: []FluxSystem{
				{
					Name:    "gateway",
					Install: &Kustomization{Components: []string{"envoy", "envoy/prometheus", "keep_component"}},
				},
			},
		}

		removal := FluxSystem{
			Name:    "gateway",
			Install: &Kustomization{Components: []string{"envoy", "envoy/prometheus"}},
		}

		err := base.RemoveFluxSystem(removal)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		sys := base.FluxSystems[0]
		if sys.Install == nil || len(sys.Install.Components) != 1 || sys.Install.Components[0] != "keep_component" {
			t.Errorf("Expected only 'keep_component' to remain, got %+v", sys.Install)
		}
	})

	t.Run("LeavesInstallUnchangedWhenRemovalHasNoInstall", func(t *testing.T) {
		base := &Blueprint{
			FluxSystems: []FluxSystem{
				{
					Name:    "gateway",
					Install: &Kustomization{Components: []string{"envoy"}},
				},
			},
		}

		removal := FluxSystem{
			Name:      "gateway",
			DependsOn: []string{"pki-install"},
		}

		err := base.RemoveFluxSystem(removal)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		sys := base.FluxSystems[0]
		if sys.Install == nil || len(sys.Install.Components) != 1 || sys.Install.Components[0] != "envoy" {
			t.Errorf("Expected install to be unchanged, got %+v", sys.Install)
		}
	})

	t.Run("DropsNamedResourcesVariant", func(t *testing.T) {
		base := &Blueprint{
			FluxSystems: []FluxSystem{
				{
					Name: "gateway",
					Resources: []FluxVariant{
						{Kustomization: Kustomization{Name: "internal", Components: []string{"internal"}}},
						{Kustomization: Kustomization{Name: "external", Components: []string{"external"}}},
					},
				},
			},
		}

		removal := FluxSystem{
			Name:      "gateway",
			Resources: []FluxVariant{{Kustomization: Kustomization{Name: "internal"}}},
		}

		err := base.RemoveFluxSystem(removal)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		sys := base.FluxSystems[0]
		if len(sys.Resources) != 1 || sys.Resources[0].Name != "external" {
			t.Errorf("Expected only 'external' variant to remain, got %+v", sys.Resources)
		}
	})

	t.Run("DropsUnnamedResourcesVariant", func(t *testing.T) {
		base := &Blueprint{
			FluxSystems: []FluxSystem{
				{
					Name: "dns",
					Resources: []FluxVariant{
						{Kustomization: Kustomization{Components: []string{"coredns"}}},
					},
				},
			},
		}

		removal := FluxSystem{
			Name:      "dns",
			Resources: []FluxVariant{{}},
		}

		err := base.RemoveFluxSystem(removal)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		sys := base.FluxSystems[0]
		if len(sys.Resources) != 0 {
			t.Errorf("Expected the unnamed variant to be dropped, got %+v", sys.Resources)
		}
	})

	t.Run("NoOpWhenSystemNotFound", func(t *testing.T) {
		base := &Blueprint{
			FluxSystems: []FluxSystem{
				{
					Name:    "existing",
					Install: &Kustomization{Components: []string{"component1"}},
				},
			},
		}

		removal := FluxSystem{
			Name:    "non-existent",
			Install: &Kustomization{Components: []string{"component1"}},
		}

		err := base.RemoveFluxSystem(removal)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(base.FluxSystems) != 1 {
			t.Errorf("Expected 1 system unchanged, got %d", len(base.FluxSystems))
		}
		if len(base.FluxSystems[0].Install.Components) != 1 {
			t.Errorf("Expected existing system to be unchanged")
		}
	})

	t.Run("PreservesIndexField", func(t *testing.T) {
		base := &Blueprint{
			FluxSystems: []FluxSystem{
				{
					Name:    "gateway",
					Path:    "original-path",
					Source:  "original-source",
					Install: &Kustomization{Components: []string{"envoy"}},
				},
			},
		}

		removal := FluxSystem{
			Name:    "gateway",
			Install: &Kustomization{Components: []string{"envoy"}},
		}

		err := base.RemoveFluxSystem(removal)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		sys := base.FluxSystems[0]
		if sys.Name != "gateway" {
			t.Errorf("Expected Name to be preserved, got '%s'", sys.Name)
		}
		if sys.Path != "original-path" {
			t.Errorf("Expected Path to be preserved, got '%s'", sys.Path)
		}
		if sys.Source != "original-source" {
			t.Errorf("Expected Source to be preserved, got '%s'", sys.Source)
		}
	})
}

func TestBlueprint_UpsertFluxSystem(t *testing.T) {
	t.Run("AppendsWhenNoExistingSystemOfThatName", func(t *testing.T) {
		// Given a blueprint with no cert-manager system yet
		b := &Blueprint{}

		// When a system is upserted
		if err := b.UpsertFluxSystem(FluxSystem{Name: "cert-manager"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Then it is appended
		if len(b.FluxSystems) != 1 || b.FluxSystems[0].Name != "cert-manager" {
			t.Fatalf("expected cert-manager appended, got %+v", b.FluxSystems)
		}
	})

	t.Run("MergesInsteadOfReplacingOnNameCollision", func(t *testing.T) {
		// Given a blueprint already carrying a cert-manager system with an install and one
		// resources variant (as if written by one source's already-merged facets)
		b := &Blueprint{
			FluxSystems: []FluxSystem{
				{
					Name:      "cert-manager",
					DependsOn: []string{"pki-base"},
					Install:   &Kustomization{Components: []string{"helm-release"}},
					Resources: []FluxVariant{{Kustomization: Kustomization{Components: []string{"private-issuer/ca"}}}},
				},
			},
		}

		// When a second same-named system (as if from another source) is upserted, contributing
		// an additional install component, an extra dependsOn edge, and a new resources variant
		if err := b.UpsertFluxSystem(FluxSystem{
			Name:      "cert-manager",
			DependsOn: []string{"lb-base"},
			Install:   &Kustomization{Components: []string{"helm-release-extra-metrics"}},
			Resources: []FluxVariant{{Kustomization: Kustomization{Name: "extra", Components: []string{"public-issuer"}}}},
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Then the two contributions merge into one entry instead of the second wholesale
		// replacing the first
		if len(b.FluxSystems) != 1 {
			t.Fatalf("expected one merged cert-manager entry, not two colliding entries, got %+v", b.FluxSystems)
		}
		merged := b.FluxSystems[0]
		if !contains(merged.DependsOn, "pki-base") || !contains(merged.DependsOn, "lb-base") {
			t.Errorf("expected dependsOn to union, got %v", merged.DependsOn)
		}
		if merged.Install == nil || !contains(merged.Install.Components, "helm-release") || !contains(merged.Install.Components, "helm-release-extra-metrics") {
			t.Errorf("expected install components to union across the collision, got %+v", merged.Install)
		}
		if len(merged.Resources) != 2 {
			t.Fatalf("expected the original unnamed variant and the new named variant to both survive, got %+v", merged.Resources)
		}
	})

	t.Run("PreservesSecretsContributedToAnExistingSystem", func(t *testing.T) {
		// Given a blueprint already carrying a telemetry system with no secrets (as if from a base source)
		b := &Blueprint{
			FluxSystems: []FluxSystem{{
				Name:    "telemetry",
				Install: &Kustomization{Components: []string{"prometheus"}},
			}},
		}

		// When another source's same-named system contributes a secret
		if err := b.UpsertFluxSystem(FluxSystem{
			Name:    "telemetry",
			Secrets: map[string]SecretEntry{"alertmanager-slack": {Data: map[string]string{"webhook_url": `${secret("Developer", "slack_webhooks", "url")}`}}},
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Then the merged system carries the secret instead of dropping it
		if len(b.FluxSystems) != 1 {
			t.Fatalf("expected one merged telemetry entry, got %+v", b.FluxSystems)
		}
		got := b.FluxSystems[0].Secrets["alertmanager-slack"].Data["webhook_url"]
		if got != `${secret("Developer", "slack_webhooks", "url")}` {
			t.Errorf("expected the contributed secret to survive the merge, got %q (all: %+v)", got, b.FluxSystems[0].Secrets)
		}
	})

	t.Run("UnionsSecretsAcrossBothSidesOnNameCollision", func(t *testing.T) {
		// Given a system that already carries one secret
		b := &Blueprint{
			FluxSystems: []FluxSystem{{
				Name:    "telemetry",
				Secrets: map[string]SecretEntry{"creds": {Data: map[string]string{"a": "${x.a}"}}},
			}},
		}

		// When a same-named system contributes another secret data key
		if err := b.UpsertFluxSystem(FluxSystem{
			Name:    "telemetry",
			Secrets: map[string]SecretEntry{"creds": {Data: map[string]string{"b": "${x.b}"}}},
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Then both data keys survive under the same secret name
		creds := b.FluxSystems[0].Secrets["creds"].Data
		if creds["a"] != "${x.a}" || creds["b"] != "${x.b}" {
			t.Errorf("expected secret data keys to union, got %+v", creds)
		}
	})

	t.Run("UnionsSecretNamespacesAcrossBothSidesOnNameCollision", func(t *testing.T) {
		// Given a system whose secret already targets one namespace
		b := &Blueprint{
			FluxSystems: []FluxSystem{{
				Name:    "pki",
				Secrets: map[string]SecretEntry{"acme-dns": {Namespaces: []string{"system-pki"}, Data: map[string]string{"token": "${pki.token}"}}},
			}},
		}

		// When a same-named system contributes the same secret targeting another namespace
		if err := b.UpsertFluxSystem(FluxSystem{
			Name:    "pki",
			Secrets: map[string]SecretEntry{"acme-dns": {Namespaces: []string{"system-dns"}, Data: map[string]string{"token": "${pki.token}"}}},
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Then the secret fans out to both namespaces without duplication
		got := b.FluxSystems[0].Secrets["acme-dns"].Namespaces
		if len(got) != 2 || !slices.Contains(got, "system-pki") || !slices.Contains(got, "system-dns") {
			t.Errorf("expected namespaces to union to [system-pki system-dns], got %+v", got)
		}
	})
}

func TestKustomization_Decryption_DeepCopyAndMerge(t *testing.T) {
	t.Run("DeepCopyIndependentOfOriginal", func(t *testing.T) {
		// Given a kustomization with decryption set
		original := &Kustomization{
			Name:       "k",
			Decryption: &Decryption{Provider: "sops", SecretRef: &DecryptionSecretRef{Name: "sops-age"}},
		}

		// When deep-copied and the copy mutated
		clone := original.DeepCopy()
		clone.Decryption.Provider = "vault"
		clone.Decryption.SecretRef.Name = "other"

		// Then the original is unchanged (no shared pointer)
		if original.Decryption.Provider != "sops" || original.Decryption.SecretRef.Name != "sops-age" {
			t.Errorf("Expected original unchanged, got %+v / %+v", original.Decryption, original.Decryption.SecretRef)
		}
	})

	t.Run("NilDecryptionDeepCopiesToNil", func(t *testing.T) {
		// Given a kustomization without decryption
		original := &Kustomization{Name: "k"}

		// When deep-copied
		clone := original.DeepCopy()

		// Then the clone's Decryption stays nil
		if clone.Decryption != nil {
			t.Errorf("Expected nil Decryption on clone, got %+v", clone.Decryption)
		}
	})

	t.Run("OverlayDecryptionWinsAndIsCopied", func(t *testing.T) {
		// Given a base with no decryption and an overlay that sets it
		base := Kustomization{Name: "k"}
		overlay := Kustomization{Name: "k", Decryption: &Decryption{Provider: "sops", SecretRef: &DecryptionSecretRef{Name: "sops-age"}}}

		// When merged
		merged := MergeKustomizationFields(base, overlay)

		// Then the overlay's decryption is applied
		if merged.Decryption == nil || merged.Decryption.Provider != "sops" || merged.Decryption.SecretRef.Name != "sops-age" {
			t.Fatalf("Expected overlay decryption applied, got %+v", merged.Decryption)
		}

		// And it is a copy, not the overlay's pointer, so mutating the overlay does not bleed through
		overlay.Decryption.SecretRef.Name = "mutated"
		if merged.Decryption.SecretRef.Name != "sops-age" {
			t.Errorf("Expected merged decryption independent of overlay, got '%s'", merged.Decryption.SecretRef.Name)
		}
	})

	t.Run("NilOverlayDecryptionLeavesBaseIntact", func(t *testing.T) {
		// Given a base that already has decryption and an overlay that does not set it
		base := Kustomization{Name: "k", Decryption: &Decryption{Provider: "sops", SecretRef: &DecryptionSecretRef{Name: "sops-age"}}}
		overlay := Kustomization{Name: "k"}

		// When merged
		merged := MergeKustomizationFields(base, overlay)

		// Then the base decryption survives
		if merged.Decryption == nil || merged.Decryption.Provider != "sops" {
			t.Errorf("Expected base decryption preserved, got %+v", merged.Decryption)
		}
	})
}

func TestKustomization_DeleteTimeout_Merge(t *testing.T) {
	t.Run("OverlayDeleteTimeoutWins", func(t *testing.T) {
		// Given a base with no delete timeout and an overlay that sets one
		base := Kustomization{Name: "k"}
		overlay := Kustomization{Name: "k", DeleteTimeout: &DurationString{Duration: 45 * time.Minute}}

		// When merged
		merged := MergeKustomizationFields(base, overlay)

		// Then the overlay's delete timeout is applied
		if merged.DeleteTimeout == nil || merged.DeleteTimeout.Duration != 45*time.Minute {
			t.Fatalf("Expected overlay delete timeout applied, got %+v", merged.DeleteTimeout)
		}
	})

	t.Run("NilOverlayDeleteTimeoutLeavesBaseIntact", func(t *testing.T) {
		// Given a base that already has a delete timeout and an overlay that does not set one
		base := Kustomization{Name: "k", DeleteTimeout: &DurationString{Duration: 45 * time.Minute}}
		overlay := Kustomization{Name: "k"}

		// When merged
		merged := MergeKustomizationFields(base, overlay)

		// Then the base delete timeout survives
		if merged.DeleteTimeout == nil || merged.DeleteTimeout.Duration != 45*time.Minute {
			t.Errorf("Expected base delete timeout preserved, got %+v", merged.DeleteTimeout)
		}
	})
}

func TestDeepMergeMaps_EmptyOverlayDoesNotOverwritePopulated(t *testing.T) {
	t.Run("EmptyStringOverlayDoesNotOverwriteNonEmptyBase", func(t *testing.T) {
		base := map[string]any{"key": "populated"}
		overlay := map[string]any{"key": ""}
		got := DeepMergeMaps(base, overlay)
		if got["key"] != "populated" {
			t.Errorf("Expected populated value preserved, got %v", got["key"])
		}
	})
	t.Run("EmptySliceOverlayDoesNotOverwriteNonEmptyBase", func(t *testing.T) {
		base := map[string]any{"patches": []any{"a", "b"}}
		overlay := map[string]any{"patches": []any{}}
		got := DeepMergeMaps(base, overlay)
		sl, ok := got["patches"].([]any)
		if !ok {
			t.Fatalf("Expected []any, got %T", got["patches"])
		}
		if len(sl) != 2 || sl[0] != "a" || sl[1] != "b" {
			t.Errorf("Expected [a b] preserved, got %v", sl)
		}
	})
	t.Run("EmptyOverlayValueForKeyMissingInBaseIsAdded", func(t *testing.T) {
		base := map[string]any{"a": 1}
		overlay := map[string]any{"new_empty": "", "new_slice": []any{}, "new_map": map[string]any{}}
		got := DeepMergeMaps(base, overlay)
		if got["new_empty"] != "" {
			t.Errorf("Expected new key with empty string to be added, got %v", got["new_empty"])
		}
		if sl, ok := got["new_slice"].([]any); !ok || len(sl) != 0 {
			t.Errorf("Expected new key with empty slice to be added, got %v", got["new_slice"])
		}
		if m, ok := got["new_map"].(map[string]any); !ok || len(m) != 0 {
			t.Errorf("Expected new key with empty map to be added, got %v", got["new_map"])
		}
	})
	t.Run("EmptyOverlayOverwritesBaseKeyWithNilValue", func(t *testing.T) {
		base := map[string]any{"key": nil}
		overlay := map[string]any{"key": ""}
		got := DeepMergeMaps(base, overlay)
		if got["key"] != "" {
			t.Errorf("Expected empty overlay to be applied when base value is nil, got %v", got["key"])
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

// =============================================================================
// Test Private Methods
// =============================================================================

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
