package blueprint

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Private Methods
// =============================================================================

// sourceInstalls reports whether a source's components — including its vendored CRDs — are merged and
// installed: an OCI source with install omitted defaults to true for backward compatibility, otherwise
// install must be explicitly true. sourceShouldBeMerged delegates here so the two never diverge.
func sourceInstalls(source blueprintv1alpha1.Source) bool {
	if source.Install == nil {
		return strings.HasPrefix(source.Url, "oci://")
	}
	return source.Install.IsInstalled()
}

// finalizeCrdLayers assigns each CRD reference to exactly one owner before the barrier wires the stack
// to them, so two kustomizations never apply the same CRD object. The default/project list (bp.Crds)
// owns first, then install sources alphabetically by name; a reference claimed by an earlier owner is
// pruned from later ones. Only install-eligible sources (the same sourceInstalls predicate CrdLayers
// materializes on) may claim ownership: a non-install source claiming a ref would strip it from a real
// owner while never installing it itself, silently orphaning the CRD. A local template source collapses
// into the default/project list (the same condition ToFluxKustomization uses), so a purely local
// blueprint installs its CRDs as "crds" rather than a "crds-template" layer. Ownership by name keeps a
// CRD stable across source reordering.
func (c *BaseBlueprintComposer) finalizeCrdLayers(bp *blueprintv1alpha1.Blueprint) {
	if !blueprintv1alpha1.HasRemoteTemplateSource(bp.Sources) {
		for i := range bp.Sources {
			if bp.Sources[i].Name != "template" || len(bp.Sources[i].Crds) == 0 {
				continue
			}
			for _, ref := range bp.Sources[i].Crds {
				if !slices.Contains(bp.Crds, ref) {
					bp.Crds = append(bp.Crds, ref)
				}
			}
			bp.Sources[i].Crds = nil
		}
	}

	claimed := make(map[string]struct{})
	bp.Crds = claimCrdRefs(bp.Crds, claimed)

	idx := make([]int, 0, len(bp.Sources))
	for i := range bp.Sources {
		if len(bp.Sources[i].Crds) > 0 && sourceInstalls(bp.Sources[i]) {
			idx = append(idx, i)
		}
	}
	sort.Slice(idx, func(a, b int) bool { return bp.Sources[idx[a]].Name < bp.Sources[idx[b]].Name })
	for _, i := range idx {
		bp.Sources[i].Crds = claimCrdRefs(bp.Sources[i].Crds, claimed)
	}
}

// claimCrdRefs returns refs minus any already in claimed, recording the survivors as claimed, sorted.
func claimCrdRefs(refs []string, claimed map[string]struct{}) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if _, taken := claimed[ref]; taken {
			continue
		}
		claimed[ref] = struct{}{}
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

// applyCrdLayerBarrier orders the vendored CRD layers ahead of the whole stack: when the blueprint
// implies CRD layers, every root — a plain kustomize: entry or flux: system with no dependsOn of its
// own — is made to depend on every synthesized CRD kustomization. Dependents of those roots reach the
// layers transitively, so the stack reconciles after the CRDs are Established — without any facet
// author naming a CRD in dependsOn, and without the provisioner waiting: Flux honors the edges on
// every reconcile. The CRD kustomizations themselves are materialized by the provisioner from the
// same CrdLayers derivation.
func (c *BaseBlueprintComposer) applyCrdLayerBarrier(bp *blueprintv1alpha1.Blueprint) {
	layers := CrdLayers(bp)
	if len(layers) == 0 {
		return
	}
	names := make([]string, len(layers))
	for i, layer := range layers {
		names[i] = blueprintv1alpha1.CrdKustomizationName(layer.Source)
	}
	for i := range bp.Kustomizations {
		k := &bp.Kustomizations[i]
		if len(k.DependsOn) == 0 {
			k.DependsOn = slices.Clone(names)
		}
	}
	for i := range bp.FluxSystems {
		if len(bp.FluxSystems[i].DependsOn) == 0 {
			bp.FluxSystems[i].DependsOn = slices.Clone(names)
		}
	}
}

// applyGlobalDependencyBarrier wires every kustomization and flux system outside a global-dependency
// system's own dependency closure to depend on that system's terminal tier — its resources tier if it
// has one, else its install tier. It is the inverse of dependsOn: a system declares itself required by
// the whole cluster once (globalDependency: true) instead of every consumer naming it. The closure
// exclusion keeps the system, and anything it transitively depends on, from ordering after itself — the
// same way the crds layer sits ahead of the stack without depending on it. It runs after
// applyCrdLayerBarrier so a global system rooted at the crds layer already carries that edge when its
// closure is computed.
func (c *BaseBlueprintComposer) applyGlobalDependencyBarrier(bp *blueprintv1alpha1.Blueprint) {
	type barrier struct {
		terminals []string
		closure   map[string]struct{}
	}

	depsByName := make(map[string][]string)
	for _, k := range bp.AllKustomizations() {
		depsByName[k.Name] = k.DependsOn
	}

	var barriers []barrier
	for _, sys := range bp.FluxSystems {
		if !sys.GlobalDependency {
			continue
		}
		terminals := terminalTierNames(sys)
		if len(terminals) == 0 {
			continue
		}
		barriers = append(barriers, barrier{
			terminals: terminals,
			closure:   dependencyClosure(sys.TierNames(), depsByName),
		})
	}
	if len(barriers) == 0 {
		return
	}

	for _, b := range barriers {
		for i := range bp.Kustomizations {
			if _, inClosure := b.closure[bp.Kustomizations[i].Name]; inClosure {
				continue
			}
			bp.Kustomizations[i].DependsOn = appendMissing(bp.Kustomizations[i].DependsOn, b.terminals)
		}
		for i := range bp.FluxSystems {
			if fluxSystemInClosure(bp.FluxSystems[i], b.closure) {
				continue
			}
			bp.FluxSystems[i].DependsOn = appendMissing(bp.FluxSystems[i].DependsOn, b.terminals)
		}
	}
}

// terminalTierNames returns the compiled kustomization names at which a system is fully reconciled: its
// resources tier names when it has a resources tier, otherwise its install tier. These are the targets a
// globalDependency system's consumers wait on.
func terminalTierNames(sys blueprintv1alpha1.FluxSystem) []string {
	names := sys.TierNames()
	if len(sys.Resources) == 0 {
		return names
	}
	installName := sys.Name + "-install"
	terminals := make([]string, 0, len(names))
	for _, n := range names {
		if n != installName {
			terminals = append(terminals, n)
		}
	}
	return terminals
}

// dependencyClosure returns the set of kustomization names reachable from seeds by following dependsOn
// edges (seeds included), over the compiled dependency graph depsByName.
func dependencyClosure(seeds []string, depsByName map[string][]string) map[string]struct{} {
	closure := make(map[string]struct{})
	queue := slices.Clone(seeds)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if _, seen := closure[name]; seen {
			continue
		}
		closure[name] = struct{}{}
		queue = append(queue, depsByName[name]...)
	}
	return closure
}

// fluxSystemInClosure reports whether any of a system's compiled tiers is in closure. It is how a
// globalDependency barrier skips the system it depends on (wiring it would form a cycle), the global
// system itself included, since a system's own tiers are always in its closure.
func fluxSystemInClosure(sys blueprintv1alpha1.FluxSystem, closure map[string]struct{}) bool {
	for _, name := range sys.TierNames() {
		if _, ok := closure[name]; ok {
			return true
		}
	}
	return false
}

// appendMissing returns deps with each addition not already present appended in order, so the barrier
// never duplicates an edge a consumer already declares.
func appendMissing(deps []string, additions []string) []string {
	for _, add := range additions {
		if !slices.Contains(deps, add) {
			deps = append(deps, add)
		}
	}
	return deps
}

// resolveTierDependencies rewrites a dependsOn reference to a vendor's bare name into its install
// tier when the vendor was authored with install/resources tiers. A facet that depends on
// "cert-manager" resolves to "cert-manager-install" when no kustomization named "cert-manager"
// exists but "cert-manager-install" does, so "depend on cert-manager" means "wait for its
// controller". An exact name match always wins, so the legacy flat form is unaffected.
func (c *BaseBlueprintComposer) resolveTierDependencies(bp *blueprintv1alpha1.Blueprint) {
	names := make(map[string]struct{}, len(bp.Kustomizations)+len(bp.FluxSystems))
	for _, k := range bp.Kustomizations {
		names[k.Name] = struct{}{}
	}
	for _, sys := range bp.FluxSystems {
		if sys.Install != nil {
			names[sys.Name+"-install"] = struct{}{}
		}
	}
	resolve := func(deps []string) {
		for j, dep := range deps {
			if _, ok := names[dep]; ok {
				continue
			}
			if _, ok := names[dep+"-install"]; ok {
				deps[j] = dep + "-install"
			}
		}
	}
	for i := range bp.Kustomizations {
		resolve(bp.Kustomizations[i].DependsOn)
	}
	for i := range bp.FluxSystems {
		resolve(bp.FluxSystems[i].DependsOn)
	}
}

// validateDependencies checks that all component dependencies reference components that exist in the final blueprint.
// This validation happens after all composition is complete to ensure dependencies are valid in the final state.
func (c *BaseBlueprintComposer) validateDependencies(bp *blueprintv1alpha1.Blueprint) error {
	tfIDs := make(map[string]struct{})
	for _, tf := range bp.TerraformComponents {
		tfIDs[tf.GetID()] = struct{}{}
	}

	allK := bp.AllKustomizations()
	kNames := make(map[string]struct{}, len(allK)+len(bp.Sources)+1)
	for _, k := range allK {
		kNames[k.Name] = struct{}{}
	}
	for _, layer := range CrdLayers(bp) {
		kNames[blueprintv1alpha1.CrdKustomizationName(layer.Source)] = struct{}{}
	}

	for _, tf := range bp.TerraformComponents {
		for _, dep := range tf.DependsOn {
			if _, exists := tfIDs[dep]; !exists {
				return fmt.Errorf("terraform component %q depends on %s", tf.GetID(), c.describeMissingDependency(dep, "component"))
			}
		}
	}

	for _, k := range allK {
		for _, dep := range k.DependsOn {
			if _, exists := kNames[dep]; !exists {
				return fmt.Errorf("kustomization %q depends on %s", k.Name, c.describeMissingDependency(dep, "kustomization"))
			}
		}
	}

	return nil
}

// describeMissingDependency renders the tail of a dangling-dependency error. When an excluded facet
// would have provided the missing name, it names that facet and the when: condition that excluded it,
// turning a bare "non-existent X" into an actionable diagnostic. Otherwise it falls back to the plain
// "non-existent X" message. kind is "component" or "kustomization".
func (c *BaseBlueprintComposer) describeMissingDependency(dep, kind string) string {
	for _, excluded := range c.excludedFacets {
		if !slices.Contains(excluded.Provides, dep) {
			continue
		}
		if excluded.When != "" {
			return fmt.Sprintf("%q, which is not in the composition: it is contributed by facet %q, excluded because its when condition (%s) evaluated false", dep, excluded.Name, excluded.When)
		}
		return fmt.Sprintf("%q, which is not in the composition: it is contributed by facet %q, which was excluded", dep, excluded.Name)
	}
	return fmt.Sprintf("non-existent %s %q", kind, dep)
}
