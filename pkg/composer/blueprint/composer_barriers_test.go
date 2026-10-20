package blueprint

import (
	"slices"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestComposer_finalizeCrdLayers(t *testing.T) {
	// Sources only carry CRDs when they are install-eligible (their facets were merged), so the
	// per-source tests mark them installed; finalize claims ownership only for install sources.
	install := &blueprintv1alpha1.BoolExpression{Value: func() *bool { v := true; return &v }()}

	t.Run("CollapsesLocalTemplateToTheDefaultCrdsList", func(t *testing.T) {
		// Given CRDs on the local template source with no remote template source declared
		composer := &BaseBlueprintComposer{}
		bp := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{{Name: "template", Crds: []string{"cert-manager-1.16.2"}}},
		}

		composer.finalizeCrdLayers(bp)

		// Then the refs collapse into the default/project list (installed as "crds") and the template
		// source no longer carries them
		if !slices.Equal(bp.Crds, []string{"cert-manager-1.16.2"}) {
			t.Fatalf("expected template collapsed into the default crds list, got %+v", bp.Crds)
		}
		if len(bp.Sources[0].Crds) != 0 {
			t.Errorf("expected template source crds cleared, got %v", bp.Sources[0].Crds)
		}
	})

	t.Run("KeepsTemplateLayerWhenRemoteTemplateSourceExists", func(t *testing.T) {
		// Given a remote template source, "template" is a real fetchable source
		composer := &BaseBlueprintComposer{}
		bp := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{{Name: "template", Url: "oci://example.com/template:1.0.0", Crds: []string{"cert-manager-1.16.2"}}},
		}

		composer.finalizeCrdLayers(bp)

		// Then the template source keeps its own CRDs (it will install as "crds-template")
		if len(bp.Crds) != 0 {
			t.Fatalf("expected no default-source crds, got %+v", bp.Crds)
		}
		if !slices.Equal(bp.Sources[0].Crds, []string{"cert-manager-1.16.2"}) {
			t.Errorf("expected template source to keep its crds, got %v", bp.Sources[0].Crds)
		}
	})

	t.Run("AssignsSharedRefToTheAlphabeticallyFirstSource", func(t *testing.T) {
		// Given two sources that both vendor the same CRD bundle
		composer := &BaseBlueprintComposer{}
		bp := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Install: install, Crds: []string{"cert-manager-1.16.2", "gateway-api-1.5.1"}},
				{Name: "acme-oci", Install: install, Crds: []string{"cert-manager-1.16.2", "acme-crd-2.0.0"}},
			},
		}

		composer.finalizeCrdLayers(bp)

		// Then the shared ref is owned by acme-oci (sorts before core), not by declaration order; refs
		// are emitted sorted
		core := sourceByName(bp, "core")
		acme := sourceByName(bp, "acme-oci")
		if !slices.Equal(acme.Crds, []string{"acme-crd-2.0.0", "cert-manager-1.16.2"}) {
			t.Errorf("expected acme-oci to own the shared ref, got %v", acme.Crds)
		}
		if !slices.Equal(core.Crds, []string{"gateway-api-1.5.1"}) {
			t.Errorf("expected core to drop the shared ref, got %v", core.Crds)
		}
	})

	t.Run("OwnershipIsStableWhenSourcesAreReordered", func(t *testing.T) {
		// Given the same two sources vendoring a shared ref, declared in opposite orders
		composer := &BaseBlueprintComposer{}
		forward := &blueprintv1alpha1.Blueprint{Sources: []blueprintv1alpha1.Source{
			{Name: "core", Install: install, Crds: []string{"cert-manager-1.16.2"}},
			{Name: "acme-oci", Install: install, Crds: []string{"cert-manager-1.16.2"}},
		}}
		reversed := &blueprintv1alpha1.Blueprint{Sources: []blueprintv1alpha1.Source{
			{Name: "acme-oci", Install: install, Crds: []string{"cert-manager-1.16.2"}},
			{Name: "core", Install: install, Crds: []string{"cert-manager-1.16.2"}},
		}}

		composer.finalizeCrdLayers(forward)
		composer.finalizeCrdLayers(reversed)

		// Then both compose to the identical result — acme-oci owns the ref, core drops it, regardless
		// of declaration order
		if !slices.Equal(sourceByName(forward, "acme-oci").Crds, []string{"cert-manager-1.16.2"}) ||
			!slices.Equal(sourceByName(reversed, "acme-oci").Crds, []string{"cert-manager-1.16.2"}) {
			t.Errorf("expected acme-oci to own the ref regardless of order")
		}
		if len(sourceByName(forward, "core").Crds) != 0 || len(sourceByName(reversed, "core").Crds) != 0 {
			t.Errorf("expected core to drop the shared ref regardless of order")
		}
	})

	t.Run("DefaultSourceWinsTiesAndEmptiesNamedSource", func(t *testing.T) {
		// Given the project/default list and a named source both vendoring a ref
		composer := &BaseBlueprintComposer{}
		bp := &blueprintv1alpha1.Blueprint{
			Crds:    []string{"cert-manager-1.16.2"},
			Sources: []blueprintv1alpha1.Source{{Name: "core", Install: install, Crds: []string{"cert-manager-1.16.2"}}},
		}

		composer.finalizeCrdLayers(bp)

		// Then the default list (crds) owns the ref and the named source's copy is emptied, so no second
		// kustomization fights over the same CRD
		if !slices.Equal(bp.Crds, []string{"cert-manager-1.16.2"}) {
			t.Fatalf("expected the default list to keep the ref, got %+v", bp.Crds)
		}
		if len(bp.Sources[0].Crds) != 0 {
			t.Errorf("expected the named source's copy emptied, got %v", bp.Sources[0].Crds)
		}
	})

	t.Run("NonInstallSourceDoesNotStealOwnership", func(t *testing.T) {
		// Given a non-install source (no install signal) and an install source that both vendor a ref,
		// with the non-install one sorting first alphabetically
		composer := &BaseBlueprintComposer{}
		bp := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "aaa-noinstall", Crds: []string{"cert-manager-1.16.2"}},
				{Name: "core", Install: install, Crds: []string{"cert-manager-1.16.2"}},
			},
		}

		composer.finalizeCrdLayers(bp)

		// Then the install source keeps the ref — the non-install source cannot claim ownership and then
		// fail to materialize, which would orphan the CRD
		if !slices.Equal(sourceByName(bp, "core").Crds, []string{"cert-manager-1.16.2"}) {
			t.Errorf("expected the install source to keep the ref, got %v", sourceByName(bp, "core").Crds)
		}
	})
}

func TestComposer_applyCrdLayerBarrier(t *testing.T) {
	depsOf := func(bp *blueprintv1alpha1.Blueprint, name string) []string {
		for _, k := range bp.Kustomizations {
			if k.Name == name {
				return k.DependsOn
			}
		}
		return nil
	}

	ociInstall := func() *blueprintv1alpha1.BoolExpression {
		v := true
		return &blueprintv1alpha1.BoolExpression{Value: &v}
	}

	t.Run("WiresRootsToTheCrdLayer", func(t *testing.T) {
		// Given a blueprint with a local crds: list, a root, and a non-root
		composer := &BaseBlueprintComposer{}
		bp := &blueprintv1alpha1.Blueprint{
			Crds: []string{"gateway-api-1.5.1", "envoy-gateway-1.7.1"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "policy-base", Path: "policy/base"},
				{Name: "gateway-base", Path: "gateway/base", DependsOn: []string{"policy-base"}},
			},
		}

		composer.applyCrdLayerBarrier(bp)

		// Then the root waits on the crds layer; the non-root reaches it transitively
		if !slices.Equal(depsOf(bp, "policy-base"), []string{"crds"}) {
			t.Errorf("expected policy-base wired to crds, got %v", depsOf(bp, "policy-base"))
		}
		if slices.Contains(depsOf(bp, "gateway-base"), "crds") {
			t.Errorf("expected gateway-base to reach crds transitively, not directly, got %v", depsOf(bp, "gateway-base"))
		}
	})

	t.Run("LeavesKustomizationsThatAlreadyDependOnCrds", func(t *testing.T) {
		// Given a facet-authored kustomization that already depends on crds
		composer := &BaseBlueprintComposer{}
		bp := &blueprintv1alpha1.Blueprint{
			Crds: []string{"gateway-api-1.5.1"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "gateway-base", Path: "gateway/base", DependsOn: []string{"crds"}},
			},
		}

		composer.applyCrdLayerBarrier(bp)

		// Then its dependency is untouched (no duplicate crds)
		if !slices.Equal(depsOf(bp, "gateway-base"), []string{"crds"}) {
			t.Errorf("expected gateway-base to keep its single crds dependency, got %v", depsOf(bp, "gateway-base"))
		}
	})

	t.Run("WiresRootsToEveryPerSourceCrdLayer", func(t *testing.T) {
		// Given CRDs vendored by two different install sources
		composer := &BaseBlueprintComposer{}
		bp := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Install: ociInstall(), Crds: []string{"cert-manager-1.16.2"}},
				{Name: "acme-oci", Install: ociInstall(), Crds: []string{"acme-crd-2.0.0"}},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "policy-base", Path: "policy/base"}},
		}

		composer.applyCrdLayerBarrier(bp)

		// Then the root waits on both source layers (alphabetical), so neither source's CRDs are skipped
		if !slices.Equal(depsOf(bp, "policy-base"), []string{"crds-acme-oci", "crds-core"}) {
			t.Errorf("expected policy-base wired to both crds layers, got %v", depsOf(bp, "policy-base"))
		}
	})

	t.Run("NoopWhenNoCrdLayer", func(t *testing.T) {
		composer := &BaseBlueprintComposer{}
		bp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "app", Path: "app"}},
		}

		composer.applyCrdLayerBarrier(bp)

		if len(depsOf(bp, "app")) != 0 {
			t.Errorf("expected no barrier without a CRD layer, got %v", depsOf(bp, "app"))
		}
	})
}

func TestComposer_applyGlobalDependencyBarrier(t *testing.T) {
	kDeps := func(bp *blueprintv1alpha1.Blueprint, name string) []string {
		for _, k := range bp.Kustomizations {
			if k.Name == name {
				return k.DependsOn
			}
		}
		return nil
	}
	sysDeps := func(bp *blueprintv1alpha1.Blueprint, name string) []string {
		for _, s := range bp.FluxSystems {
			if s.Name == name {
				return s.DependsOn
			}
		}
		return nil
	}
	install := func(components ...string) *blueprintv1alpha1.Kustomization {
		return &blueprintv1alpha1.Kustomization{Components: components}
	}
	resources := func(components ...string) []blueprintv1alpha1.FluxVariant {
		return []blueprintv1alpha1.FluxVariant{{Kustomization: blueprintv1alpha1.Kustomization{Components: components}}}
	}

	t.Run("WiresOutsideSystemsAndKustomizationsToTheResourcesTier", func(t *testing.T) {
		// Given a globalDependency system with a resources tier, an unrelated system, and a flat kustomization
		composer := &BaseBlueprintComposer{}
		bp := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{Name: "policy", GlobalDependency: true, Install: install("kyverno"), Resources: resources("kyverno/policies")},
				{Name: "telemetry", Install: install("prometheus")},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "gitops-webhook", Path: "gitops/flux"}},
		}

		composer.applyGlobalDependencyBarrier(bp)

		// Then everything outside policy waits on its resources tier, and policy does not order after itself
		if !slices.Contains(sysDeps(bp, "telemetry"), "policy-resources") {
			t.Errorf("expected telemetry wired to policy-resources, got %v", sysDeps(bp, "telemetry"))
		}
		if !slices.Contains(kDeps(bp, "gitops-webhook"), "policy-resources") {
			t.Errorf("expected gitops-webhook wired to policy-resources, got %v", kDeps(bp, "gitops-webhook"))
		}
		if slices.Contains(sysDeps(bp, "policy"), "policy-resources") {
			t.Errorf("expected policy not wired to its own tier, got %v", sysDeps(bp, "policy"))
		}
	})

	t.Run("ExcludesSystemsInTheGlobalSystemsClosure", func(t *testing.T) {
		// Given policy depends on telemetry's install tier, so telemetry is in policy's closure
		composer := &BaseBlueprintComposer{}
		bp := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{Name: "policy", GlobalDependency: true, DependsOn: []string{"telemetry-install"}, Install: install("kyverno"), Resources: resources("kyverno/policies")},
				{Name: "telemetry", Install: install("prometheus")},
			},
		}

		composer.applyGlobalDependencyBarrier(bp)

		// Then telemetry is not wired back to policy-resources (that would be a cycle)
		if slices.Contains(sysDeps(bp, "telemetry"), "policy-resources") {
			t.Errorf("expected telemetry (in policy's closure) left unwired, got %v", sysDeps(bp, "telemetry"))
		}
	})

	t.Run("TargetsInstallTierWhenNoResources", func(t *testing.T) {
		// Given a globalDependency system with only an install tier
		composer := &BaseBlueprintComposer{}
		bp := &blueprintv1alpha1.Blueprint{
			FluxSystems:    []blueprintv1alpha1.FluxSystem{{Name: "cni", GlobalDependency: true, Install: install("cilium")}},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "app", Path: "app"}},
		}

		composer.applyGlobalDependencyBarrier(bp)

		if !slices.Equal(kDeps(bp, "app"), []string{"cni-install"}) {
			t.Errorf("expected app wired to cni-install, got %v", kDeps(bp, "app"))
		}
	})

	t.Run("DoesNotDuplicateAnExistingEdge", func(t *testing.T) {
		// Given a consumer that already depends on the terminal tier
		composer := &BaseBlueprintComposer{}
		bp := &blueprintv1alpha1.Blueprint{
			FluxSystems:    []blueprintv1alpha1.FluxSystem{{Name: "policy", GlobalDependency: true, Install: install("kyverno"), Resources: resources("kyverno/policies")}},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "app", Path: "app", DependsOn: []string{"policy-resources"}}},
		}

		composer.applyGlobalDependencyBarrier(bp)

		if !slices.Equal(kDeps(bp, "app"), []string{"policy-resources"}) {
			t.Errorf("expected app to keep a single policy-resources edge, got %v", kDeps(bp, "app"))
		}
	})

	t.Run("ComposesWithTheCrdLayerBarrierWithoutACycle", func(t *testing.T) {
		// Given a crds layer plus a globalDependency policy system and a consumer, both roots
		composer := &BaseBlueprintComposer{}
		bp := &blueprintv1alpha1.Blueprint{
			Crds: []string{"kyverno-crds-1.0.0"},
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{Name: "policy", GlobalDependency: true, Install: install("kyverno"), Resources: resources("kyverno/policies")},
				{Name: "telemetry", Install: install("prometheus")},
			},
		}

		composer.applyCrdLayerBarrier(bp)
		composer.applyGlobalDependencyBarrier(bp)

		// Then policy roots at the crds layer and is not wired to itself
		if !slices.Equal(sysDeps(bp, "policy"), []string{"crds"}) {
			t.Errorf("expected policy to root at crds only, got %v", sysDeps(bp, "policy"))
		}
		// And telemetry waits on both the crds layer (barrier) and policy-resources (global), no duplicate
		if !slices.Equal(sysDeps(bp, "telemetry"), []string{"crds", "policy-resources"}) {
			t.Errorf("expected telemetry wired to crds then policy-resources, got %v", sysDeps(bp, "telemetry"))
		}
		// And the composed graph has no dangling dependency
		if err := composer.validateDependencies(bp); err != nil {
			t.Errorf("expected a valid dependency graph, got %v", err)
		}
	})

	t.Run("NoopWithoutAGlobalDependencySystem", func(t *testing.T) {
		// Given a system with the same shape but globalDependency unset
		composer := &BaseBlueprintComposer{}
		bp := &blueprintv1alpha1.Blueprint{
			FluxSystems:    []blueprintv1alpha1.FluxSystem{{Name: "policy", Install: install("kyverno"), Resources: resources("kyverno/policies")}},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "app", Path: "app"}},
		}

		composer.applyGlobalDependencyBarrier(bp)

		if len(kDeps(bp, "app")) != 0 {
			t.Errorf("expected no barrier without a globalDependency system, got %v", kDeps(bp, "app"))
		}
	})
}

func TestComposer_resolveTierDependencies(t *testing.T) {
	depsOf := func(bp *blueprintv1alpha1.Blueprint, name string) []string {
		for _, k := range bp.Kustomizations {
			if k.Name == name {
				return k.DependsOn
			}
		}
		return nil
	}

	t.Run("RewritesBareVendorNameToInstallTier", func(t *testing.T) {
		// Given a consumer that depends on a vendor by its bare name, where the vendor was tiered
		composer := &BaseBlueprintComposer{}
		bp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "cert-manager-install"},
				{Name: "cert-manager-resources", DependsOn: []string{"cert-manager-install"}},
				{Name: "dns", DependsOn: []string{"cert-manager"}},
			},
		}

		// When resolving tier dependencies
		composer.resolveTierDependencies(bp)

		// Then the bare-name dependency points at the install tier
		if !slices.Contains(depsOf(bp, "dns"), "cert-manager-install") {
			t.Errorf("Expected dns to depend on cert-manager-install, got %v", depsOf(bp, "dns"))
		}
	})

	t.Run("ExactNameMatchWins", func(t *testing.T) {
		// Given both a flat "foo" and a "foo-install" exist
		composer := &BaseBlueprintComposer{}
		bp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "foo"},
				{Name: "foo-install"},
				{Name: "bar", DependsOn: []string{"foo"}},
			},
		}

		// When resolving
		composer.resolveTierDependencies(bp)

		// Then the exact flat name is preserved (legacy form unaffected)
		if !slices.Contains(depsOf(bp, "bar"), "foo") || slices.Contains(depsOf(bp, "bar"), "foo-install") {
			t.Errorf("Expected bar to keep dependency on foo, got %v", depsOf(bp, "bar"))
		}
	})

	t.Run("LeavesUnresolvableDependencyUntouched", func(t *testing.T) {
		// Given a dependency with neither an exact nor an install-tier match
		composer := &BaseBlueprintComposer{}
		bp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app", DependsOn: []string{"missing"}},
			},
		}

		// When resolving
		composer.resolveTierDependencies(bp)

		// Then the dependency is left as-is (validateDependencies surfaces the error later)
		if !slices.Contains(depsOf(bp, "app"), "missing") {
			t.Errorf("Expected unresolved dependency to be left untouched, got %v", depsOf(bp, "app"))
		}
	})
}

func TestComposer_validateDependencies(t *testing.T) {
	t.Run("ReturnsNilForValidTerraformDependencies", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "network", Path: "network/vpc"},
				{Name: "cluster", Path: "cluster/eks", DependsOn: []string{"network"}},
			},
		}

		err := composer.validateDependencies(bp)

		if err != nil {
			t.Errorf("Expected no error for valid dependencies, got %v", err)
		}
	})

	t.Run("ReturnsErrorForInvalidTerraformDependency", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Path: "cluster/eks", DependsOn: []string{"nonexistent"}},
			},
		}

		err := composer.validateDependencies(bp)

		if err == nil {
			t.Error("Expected error for invalid terraform dependency")
		}
		if !strings.Contains(err.Error(), "depends on non-existent component") {
			t.Errorf("Expected error message about non-existent component, got: %v", err)
		}
		if !strings.Contains(err.Error(), "cluster") {
			t.Errorf("Expected error to mention component name, got: %v", err)
		}
	})

	t.Run("NamesExcludedFacetAndConditionForDanglingTerraformDependency", func(t *testing.T) {
		// Given the missing component is one an excluded facet would have contributed
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		composer.SetExcludedFacets([]ExcludedFacet{
			{Name: "provider-hetzner", When: "cluster.driver == 'talos'", Provides: []string{"cluster"}},
		})
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Path: "cni", DependsOn: []string{"cluster"}}},
		}

		// When validating, the error names the excluded facet and the condition that excluded it
		err := composer.validateDependencies(bp)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		for _, want := range []string{`facet "provider-hetzner"`, "cluster.driver == 'talos'", "evaluated false", `"cluster"`} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Expected %q in error, got %v", want, err)
			}
		}
	})

	t.Run("NamesExcludedFacetForDanglingKustomizationDependency", func(t *testing.T) {
		// Given a kustomization depending on a name an excluded facet would have contributed
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		composer.SetExcludedFacets([]ExcludedFacet{
			{Name: "addon-private-dns", When: "addons.private_dns.enabled == true", Provides: []string{"dns-install"}},
		})
		bp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "gateway-resources", DependsOn: []string{"dns-install"}}},
		}

		// When validating, the error attributes the missing kustomization to the excluded facet
		err := composer.validateDependencies(bp)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		for _, want := range []string{`facet "addon-private-dns"`, "addons.private_dns.enabled == true", `"dns-install"`} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Expected %q in error, got %v", want, err)
			}
		}
	})

	t.Run("ReportsExclusionWithoutConditionWhenExcludedFacetHasNoWhen", func(t *testing.T) {
		// Given the providing facet was excluded but carries no when: expression
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		composer.SetExcludedFacets([]ExcludedFacet{{Name: "options-extra", Provides: []string{"cluster"}}})
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Path: "cni", DependsOn: []string{"cluster"}}},
		}

		// When validating, it names the facet without a false-condition clause
		err := composer.validateDependencies(bp)
		if err == nil || !strings.Contains(err.Error(), `facet "options-extra"`) {
			t.Fatalf("Expected excluded-facet attribution, got %v", err)
		}
		if strings.Contains(err.Error(), "evaluated false") {
			t.Errorf("Did not expect a condition clause for a when-less facet, got %v", err)
		}
	})

	t.Run("ReturnsNilForValidKustomizationDependencies", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		bp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "base"},
				{Name: "app", DependsOn: []string{"base"}},
			},
		}

		err := composer.validateDependencies(bp)

		if err != nil {
			t.Errorf("Expected no error for valid dependencies, got %v", err)
		}
	})

	t.Run("ReturnsErrorForInvalidKustomizationDependency", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		bp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app", DependsOn: []string{"nonexistent"}},
			},
		}

		err := composer.validateDependencies(bp)

		if err == nil {
			t.Error("Expected error for invalid kustomization dependency")
		}
		if !strings.Contains(err.Error(), "depends on non-existent kustomization") {
			t.Errorf("Expected error message about non-existent kustomization, got: %v", err)
		}
		if !strings.Contains(err.Error(), "app") {
			t.Errorf("Expected error to mention kustomization name, got: %v", err)
		}
	})

	t.Run("ReturnsNilForEmptyBlueprint", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		bp := &blueprintv1alpha1.Blueprint{}

		err := composer.validateDependencies(bp)

		if err != nil {
			t.Errorf("Expected no error for empty blueprint, got %v", err)
		}
	})

	t.Run("ReturnsNilForComponentsWithNoDependencies", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "network", Path: "network/vpc"},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app"},
			},
		}

		err := composer.validateDependencies(bp)

		if err != nil {
			t.Errorf("Expected no error for components with no dependencies, got %v", err)
		}
	})

	t.Run("ReturnsErrorForMultipleInvalidDependencies", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Path: "cluster/eks", DependsOn: []string{"missing1", "missing2"}},
			},
		}

		err := composer.validateDependencies(bp)

		if err == nil {
			t.Error("Expected error for invalid dependencies")
		}
		if !strings.Contains(err.Error(), "missing1") && !strings.Contains(err.Error(), "missing2") {
			t.Errorf("Expected error to mention at least one missing dependency, got: %v", err)
		}
	})

	t.Run("ValidatesTerraformDependenciesByID", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "network/vpc"},
				{Path: "cluster/eks", DependsOn: []string{"network/vpc"}},
			},
		}

		err := composer.validateDependencies(bp)

		if err != nil {
			t.Errorf("Expected no error when dependency matches by path (ID), got %v", err)
		}
	})

	t.Run("ValidatesKustomizationDependenciesByName", func(t *testing.T) {
		mocks := setupComposerMocks(t)
		composer := NewBlueprintComposer(mocks.Runtime)
		bp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "base", Path: "base"},
				{Name: "app", Path: "app", DependsOn: []string{"base"}},
			},
		}

		err := composer.validateDependencies(bp)

		if err != nil {
			t.Errorf("Expected no error when dependency matches by name, got %v", err)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func sourceByName(bp *blueprintv1alpha1.Blueprint, name string) blueprintv1alpha1.Source {
	for _, s := range bp.Sources {
		if s.Name == name {
			return s
		}
	}
	return blueprintv1alpha1.Source{}
}
