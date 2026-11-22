package blueprint_test

import (
	"fmt"
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
		// Given a core facet with two patch files, and a template facet that removes one patch by path
		f := newFixture(t)
		f.withCoreSource(t)
		f.publish(t, coreURL, map[string]string{
			"_template/blueprint.yaml":        blueprintHeader + "metadata:\n  name: core\n",
			"_template/facets/web.yaml":       facetHeader + "metadata:\n  name: web\nkustomize:\n- name: web\n  path: web\n  patches:\n  - path: patches/a.yaml\n  - path: patches/b.yaml\n",
			"_template/facets/patches/a.yaml": "kind: ConfigMap\nmetadata:\n  name: a\n",
			"_template/facets/patches/b.yaml": "kind: ConfigMap\nmetadata:\n  name: b\n",
		})
		f.facet(t, "consumer", "kustomize:\n- name: web\n  strategy: remove\n  patches:\n  - path: patches/a.yaml\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then only the other patch remains, with its path relative to the source root
		k := kustomization(bp, "web")
		if k == nil {
			t.Fatal("expected kustomization web")
		}
		if got := patchPaths(k); !slices.Equal(got, []string{"facets/patches/b.yaml"}) {
			t.Errorf("expected patches [facets/patches/b.yaml], got %v", got)
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

	t.Run("RemovesTerraformInputsUnderAnEntryOrdinalOverride", func(t *testing.T) {
		// Given a facet that adds input x, a facet that merges input y with entry ordinal 300,
		// and a facet at ordinal 200 that removes input x
		f := newFixture(t)
		f.project(t)
		f.facet(t, "a-cluster", "ordinal: 100\nterraform:\n- path: cluster\n  inputs:\n    x: 1\n")
		f.facet(t, "b-cluster", "ordinal: 150\nterraform:\n- path: cluster\n  ordinal: 300\n  inputs:\n    y: 2\n")
		f.facet(t, "c-cluster", "ordinal: 200\nterraform:\n- path: cluster\n  strategy: remove\n  inputs:\n    x: null\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then the removal strips x, because x comes from the facet at ordinal 100
		c := component(bp, "cluster")
		if c == nil {
			t.Fatal("expected terraform component cluster")
		}
		if _, ok := c.Inputs["x"]; ok || c.Inputs["y"] == nil {
			t.Errorf("expected inputs with y and without x, got %v", c.Inputs)
		}
	})

	t.Run("RemovesComponentsUnderAnEntryOrdinalOverride", func(t *testing.T) {
		// Given a facet that adds component a, a facet that merges component b with entry ordinal
		// 300, and a facet at ordinal 200 that removes component a
		f := newFixture(t)
		f.project(t)
		f.facet(t, "a-web", "ordinal: 100\nkustomize:\n- name: web\n  path: web\n  components: [a]\n")
		f.facet(t, "b-web", "ordinal: 150\nkustomize:\n- name: web\n  ordinal: 300\n  components: [b]\n")
		f.facet(t, "c-web", "ordinal: 200\nkustomize:\n- name: web\n  strategy: remove\n  components: [a]\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then the removal strips a, because a comes from the facet at ordinal 100
		k := kustomization(bp, "web")
		if k == nil {
			t.Fatal("expected kustomization web")
		}
		if !slices.Equal(k.Components, []string{"b"}) {
			t.Errorf("expected components [b], got %v", k.Components)
		}
	})

	t.Run("RemovesFluxInstallComponentsUnderAnEntryOrdinalOverride", func(t *testing.T) {
		// Given a facet that adds install component envoy, a facet that merges component debug
		// with entry ordinal 300, and a facet at ordinal 200 that removes envoy
		f := newFixture(t)
		f.project(t)
		f.facet(t, "a-gateway", "ordinal: 100\nflux:\n- name: gateway\n  install:\n    components: [envoy]\n")
		f.facet(t, "b-gateway", "ordinal: 150\nflux:\n- name: gateway\n  ordinal: 300\n  install:\n    components: [debug]\n")
		f.facet(t, "c-gateway", "ordinal: 200\nflux:\n- name: gateway\n  strategy: remove\n  install:\n    components: [envoy]\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then the removal strips envoy, because envoy comes from the facet at ordinal 100
		k := fluxTier(bp, "gateway-install")
		if k == nil {
			t.Fatal("expected kustomization gateway-install")
		}
		if !slices.Equal(k.Components, []string{"debug"}) {
			t.Errorf("expected components [debug], got %v", k.Components)
		}
	})

	t.Run("KeepsAFieldThatAHigherEntryOrdinalAddsBack", func(t *testing.T) {
		// Given a facet at ordinal 300 that removes component a, and a facet at ordinal 100 whose
		// entry ordinal 400 adds component a back
		f := newFixture(t)
		f.project(t)
		f.facet(t, "a-web", "ordinal: 100\nkustomize:\n- name: web\n  path: web\n  ordinal: 400\n  components: [a]\n")
		f.facet(t, "b-web", "ordinal: 300\nkustomize:\n- name: web\n  strategy: remove\n  components: [a]\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then component a remains, because its entry ordinal is higher than the removal
		k := kustomization(bp, "web")
		if k == nil {
			t.Fatal("expected kustomization web")
		}
		if !slices.Equal(k.Components, []string{"a"}) {
			t.Errorf("expected components [a], got %v", k.Components)
		}
	})
}

func TestLoadBlueprint_EntryOrdinalInADeeperSource(t *testing.T) {
	t.Run("RanksAnEntryOrdinalAgainstTheFacetOrdinalsOfItsSource", func(t *testing.T) {
		// Given a template that uses a core source, a template facet at ordinal 100 that adds
		// component a, and a template facet at ordinal 50 whose entry ordinal 200 replaces it
		f := newFixture(t)
		f.withCoreSource(t)
		f.facet(t, "a-web", "ordinal: 100\nkustomize:\n- name: web\n  path: web\n  components: [a]\n")
		f.facet(t, "b-web", "ordinal: 50\nkustomize:\n- name: web\n  path: web\n  strategy: replace\n  ordinal: 200\n  components: [b]\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then the replace wins, because its entry ordinal is higher than the facet ordinal 100
		k := kustomization(bp, "web")
		if k == nil {
			t.Fatal("expected kustomization web")
		}
		if !slices.Equal(k.Components, []string{"b"}) {
			t.Errorf("expected components [b], got %v", k.Components)
		}
	})

	t.Run("KeepsAConfigBlockOrdinalAboveTheFacetsOfAShallowerSource", func(t *testing.T) {
		// Given a core facet at ordinal 400 that sets the schema value size to 1, and a template facet
		// whose block ordinal 300 sets size to 2 and passes size to a terraform input
		f := newFixture(t)
		f.withCoreSource(t)
		f.write(t, "contexts/_template/schema.yaml", "$schema: https://json-schema.org/draft/2020-12/schema\ntype: object\nproperties:\n  size:\n    type: integer\n")
		f.publishCore(t, "", map[string]string{"size": "ordinal: 400\nconfig:\n- name: size\n  value: 1\n"})
		f.facet(t, "cluster", "ordinal: 100\nconfig:\n- name: size\n  ordinal: 300\n  value: 2\nterraform:\n- path: cluster\n  inputs:\n    size: ${size}\n")

		// When the blueprint is loaded
		bp := f.mustLoad(t).Generate()

		// Then the template block wins, because a block ordinal does not cross a source boundary
		c := component(bp, "cluster")
		if c == nil {
			t.Fatal("expected terraform component cluster")
		}
		if fmt.Sprint(c.Inputs["size"]) != "2" {
			t.Errorf("expected input size 2, got %v", c.Inputs["size"])
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
