package blueprint_test

import (
	"slices"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Test Setup
// =============================================================================

// publishCore publishes a core artifact at coreURL that holds the core blueprint and the given facets,
// keyed by facet name. It replaces the empty core artifact that withCoreSource publishes.
func (f *fixture) publishCore(t *testing.T, blueprintBody string, facets map[string]string) {
	t.Helper()
	files := map[string]string{"_template/blueprint.yaml": blueprintHeader + "metadata:\n  name: core\n" + blueprintBody}
	for name, body := range facets {
		files["_template/facets/"+name+".yaml"] = facetHeader + "metadata:\n  name: " + name + "\n" + body
	}
	f.publish(t, coreURL, files)
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestLoadBlueprint_RemovalsAcrossSources(t *testing.T) {
	t.Run("RemovesComponentsThatASourceFacetContributes", func(t *testing.T) {
		// Given a core facet that adds components a and b, and a template facet that removes a
		f := newFixture(t)
		f.withCoreSource(t)
		f.publishCore(t, "", map[string]string{"web": "kustomize:\n- name: web\n  path: web\n  components: [a, b]\n"})
		f.facet(t, "consumer", "kustomize:\n- name: web\n  strategy: remove\n  components: [a]\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then only component b remains
		k := kustomization(bp, "web")
		if k == nil {
			t.Fatal("expected kustomization web")
		}
		if !slices.Equal(k.Components, []string{"b"}) {
			t.Errorf("expected components [b], got %v", k.Components)
		}
	})

	t.Run("RemovesComponentsThatASourceBlueprintContributes", func(t *testing.T) {
		// Given a core blueprint with no facets that adds components a and b, and a template facet that removes a
		f := newFixture(t)
		f.withCoreSource(t)
		f.publishCore(t, "kustomize:\n- name: web\n  path: web\n  components: [a, b]\n", nil)
		f.facet(t, "consumer", "kustomize:\n- name: web\n  strategy: remove\n  components: [a]\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then only component b remains
		k := kustomization(bp, "web")
		if k == nil {
			t.Fatal("expected kustomization web")
		}
		if !slices.Equal(k.Components, []string{"b"}) {
			t.Errorf("expected components [b], got %v", k.Components)
		}
	})

	t.Run("RemovesPatchesByPathThatASourceContributes", func(t *testing.T) {
		// Given a core facet with two patches, and a template facet that removes one patch by path
		f := newFixture(t)
		f.withCoreSource(t)
		f.publishCore(t, "", map[string]string{"web": "kustomize:\n- name: web\n  path: web\n  patches:\n  - path: patches/a.yaml\n  - path: patches/b.yaml\n"})
		f.facet(t, "consumer", "kustomize:\n- name: web\n  strategy: remove\n  patches:\n  - path: patches/a.yaml\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then only the other patch remains
		k := kustomization(bp, "web")
		if k == nil {
			t.Fatal("expected kustomization web")
		}
		if got := patchPaths(k); !slices.Equal(got, []string{"patches/b.yaml"}) {
			t.Errorf("expected patches [patches/b.yaml], got %v", got)
		}
	})

	t.Run("RemovesFluxInstallFieldsThatASourceContributes", func(t *testing.T) {
		// Given a core flux system with install components and dependencies, and a template facet that removes one of each
		f := newFixture(t)
		f.withCoreSource(t)
		f.publishCore(t, "", map[string]string{
			"base":    "kustomize:\n- name: base\n  path: base\n- name: certs\n  path: certs\n",
			"gateway": "flux:\n- name: gateway\n  dependsOn: [base, certs]\n  install:\n    components: [envoy, debug]\n",
		})
		f.facet(t, "consumer", "flux:\n- name: gateway\n  strategy: remove\n  dependsOn: [certs]\n  install:\n    components: [debug]\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then the install tier keeps only the fields that the template facet does not remove
		k := fluxTier(bp, "gateway-install")
		if k == nil {
			t.Fatal("expected kustomization gateway-install")
		}
		if !slices.Equal(k.Components, []string{"envoy"}) {
			t.Errorf("expected components [envoy], got %v", k.Components)
		}
		if slices.Contains(k.DependsOn, "certs") || !slices.Contains(k.DependsOn, "base") {
			t.Errorf("expected dependsOn with base and without certs, got %v", k.DependsOn)
		}
	})

	t.Run("KeepsFieldsThatADeeperSourceAdds", func(t *testing.T) {
		// Given a core facet that removes component a, and a template facet that adds a
		f := newFixture(t)
		f.withCoreSource(t)
		f.publishCore(t, "", map[string]string{"web": "kustomize:\n- name: web\n  strategy: remove\n  components: [a]\n"})
		f.facet(t, "consumer", "kustomize:\n- name: web\n  path: web\n  components: [a, b]\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then the removal of the shallower source does not apply to the deeper source
		k := kustomization(bp, "web")
		if k == nil {
			t.Fatal("expected kustomization web")
		}
		if !slices.Equal(k.Components, []string{"a", "b"}) {
			t.Errorf("expected components [a b], got %v", k.Components)
		}
	})

	t.Run("RemovesTerraformFieldsThatASourceContributes", func(t *testing.T) {
		// Given a core terraform component with two inputs and two dependencies, and a template facet that removes one of each
		f := newFixture(t)
		f.withCoreSource(t)
		f.publishCore(t, "", map[string]string{"cluster": "terraform:\n- path: network\n- path: dns\n- path: cluster\n  dependsOn: [network, dns]\n  inputs:\n    size: 3\n    debug: true\n"})
		f.facet(t, "consumer", "terraform:\n- path: cluster\n  strategy: remove\n  dependsOn: [dns]\n  inputs:\n    debug: null\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then the component keeps only the fields that the template facet does not remove
		c := component(bp, "cluster")
		if c == nil {
			t.Fatal("expected terraform component cluster")
		}
		if _, ok := c.Inputs["debug"]; ok || c.Inputs["size"] == nil {
			t.Errorf("expected inputs with size and without debug, got %v", c.Inputs)
		}
		if !slices.Equal(c.DependsOn, []string{"network"}) {
			t.Errorf("expected dependsOn [network], got %v", c.DependsOn)
		}
	})

	t.Run("IgnoresARemovalWhoseConditionIsFalse", func(t *testing.T) {
		// Given a core facet that adds components a and b, and a template removal with a false condition
		f := newFixture(t)
		f.withCoreSource(t)
		f.publishCore(t, "", map[string]string{"web": "kustomize:\n- name: web\n  path: web\n  components: [a, b]\n"})
		f.facet(t, "consumer", "kustomize:\n- name: web\n  when: platform == 'aws'\n  strategy: remove\n  components: [a]\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then both components remain
		k := kustomization(bp, "web")
		if k == nil {
			t.Fatal("expected kustomization web")
		}
		if !slices.Equal(k.Components, []string{"a", "b"}) {
			t.Errorf("expected components [a b], got %v", k.Components)
		}
	})
}

func TestLoadBlueprint_RemovalsInOneSource(t *testing.T) {
	t.Run("RemovesComponentsThatALowerFacetAdds", func(t *testing.T) {
		// Given a facet that adds components a and b, and a higher-ordinal facet that removes a
		f := newFixture(t)
		f.project(t)
		f.facet(t, "web", "ordinal: 100\nkustomize:\n- name: web\n  path: web\n  components: [a, b]\n")
		f.facet(t, "web-trim", "ordinal: 200\nkustomize:\n- name: web\n  strategy: remove\n  components: [a]\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then the kustomization stays with only component b
		k := kustomization(bp, "web")
		if k == nil {
			t.Fatal("expected kustomization web")
		}
		if !slices.Equal(k.Components, []string{"b"}) {
			t.Errorf("expected components [b], got %v", k.Components)
		}
	})

	t.Run("HigherMergeAddsARemovedComponentBack", func(t *testing.T) {
		// Given a facet that removes component a, and a higher-ordinal facet that adds a and b
		f := newFixture(t)
		f.project(t)
		f.facet(t, "web-trim", "ordinal: 100\nkustomize:\n- name: web\n  strategy: remove\n  components: [a]\n")
		f.facet(t, "web", "ordinal: 200\nkustomize:\n- name: web\n  path: web\n  components: [a, b]\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then the higher-ordinal facet wins and both components remain
		k := kustomization(bp, "web")
		if k == nil {
			t.Fatal("expected kustomization web")
		}
		if !slices.Equal(k.Components, []string{"a", "b"}) {
			t.Errorf("expected components [a b], got %v", k.Components)
		}
	})

	t.Run("RemovesTerraformInputsThatALowerFacetAdds", func(t *testing.T) {
		// Given a facet with two terraform inputs, and a higher-ordinal facet that removes one
		f := newFixture(t)
		f.project(t)
		f.facet(t, "cluster", "ordinal: 100\nterraform:\n- path: cluster\n  inputs:\n    size: 3\n    debug: true\n")
		f.facet(t, "cluster-trim", "ordinal: 200\nterraform:\n- path: cluster\n  strategy: remove\n  inputs:\n    debug: null\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then the component stays with only the other input
		c := component(bp, "cluster")
		if c == nil {
			t.Fatal("expected terraform component cluster")
		}
		if _, ok := c.Inputs["debug"]; ok || c.Inputs["size"] == nil {
			t.Errorf("expected inputs with size and without debug, got %v", c.Inputs)
		}
	})

	t.Run("RemovesFluxInstallComponentsThatALowerFacetAdds", func(t *testing.T) {
		// Given a facet with two install components, and a higher-ordinal facet that removes one
		f := newFixture(t)
		f.project(t)
		f.facet(t, "gateway", "ordinal: 100\nflux:\n- name: gateway\n  install:\n    components: [envoy, debug]\n")
		f.facet(t, "gateway-trim", "ordinal: 200\nflux:\n- name: gateway\n  strategy: remove\n  install:\n    components: [debug]\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then the install tier stays with only the other component
		k := fluxTier(bp, "gateway-install")
		if k == nil {
			t.Fatal("expected kustomization gateway-install")
		}
		if !slices.Equal(k.Components, []string{"envoy"}) {
			t.Errorf("expected components [envoy], got %v", k.Components)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

// patchPaths returns the path of each patch of k.
func patchPaths(k *blueprintv1alpha1.Kustomization) []string {
	var paths []string
	for _, p := range k.Patches {
		paths = append(paths, p.Path)
	}
	return paths
}
