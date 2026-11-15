// The processor_collect file holds the component collection steps of the BlueprintProcessor.
// It provides collection of terraform components, kustomizations, and flux systems from a facet.
// It evaluates the conditions and values of each entry against the facet scope.
// It validates CRD references and flux system secrets, and rejects sensitive substitutions.

package blueprint

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Constants
// =============================================================================

// secretTypeOpaque and secretTypeTLS are the Secret types a flux system secrets: entry can set.
const (
	secretTypeOpaque = "Opaque"
	secretTypeTLS    = "kubernetes.io/tls" // #nosec G101 -- Secret type name, not a credential
)

// =============================================================================
// Types
// =============================================================================

// entryRef locates one entry of one kind in a list of facets, with the effective ordinal of the entry.
type entryRef struct {
	facet   int
	entry   int
	ordinal int
}

// =============================================================================
// Private Methods
// =============================================================================

// collectTerraformComponent collects one terraform component of a facet into collected. It
// evaluates the when condition, inputs, and source of the component, and merges it by strategy.
// A remove component goes to addTerraformRemoval. An equal or higher-ordinal removal strips its fields
// from a merge or replace component. A component with no when takes the facet condition. Expressions
// can read config block values. It records input traces only with a trace collector.
func (p *BaseBlueprintProcessor) collectTerraformComponent(
	facet blueprintv1alpha1.Facet,
	tc blueprintv1alpha1.ConditionalTerraformComponent,
	sourceName []string,
	collected collectedComponents,
	facetScope map[string]any,
) error {
	terraformByID := collected.terraformByID
	componentWhen := tc.When
	if componentWhen == "" && facet.When != "" {
		componentWhen = facet.When
	}

	shouldInclude, err := p.shouldIncludeComponent(componentWhen, facet.Path, facetScope)
	if err != nil {
		return werror.Wrap(err, "error evaluating terraform component condition")
	}
	if !shouldInclude {
		return nil
	}

	processed := tc
	processed.When = componentWhen
	if processed.Inputs != nil {
		evaluated, err := p.evaluator.EvaluateMap(processed.Inputs, facet.Path, facetScope, false)
		if err != nil {
			return werror.Wrap(err, "error evaluating inputs for component '%s'", processed.GetID())
		}
		normalized := make(map[string]any, len(evaluated))
		origins := make(map[string]string, len(evaluated))
		componentID := processed.GetID()
		for k, v := range evaluated {
			origins[k] = facet.Path
			composedPath := "terraform." + componentID + ".inputs." + k
			if originalVal, exists := processed.Inputs[k]; exists {
				if s, ok := originalVal.(string); ok && evaluator.ContainsExpression(s) {
					probe, probeErr := p.evaluator.Evaluate(s, facet.Path, facetScope, false)
					if probeErr == nil && evaluator.IsDeferredValue(probe) {
						p.markDeferredPath(composedPath)
						if expr, ok := evaluator.DeferredExpression(probe); ok {
							v = expr
						}
					}
				}
			}
			v = normalizeDeferredValue(v)
			if m := blueprintv1alpha1.ToMapStringAny(v); m != nil {
				normalized[k] = m
			} else if s := blueprintv1alpha1.ToSliceAny(v); s != nil {
				normalized[k] = s
			} else {
				normalized[k] = v
			}
		}
		processed.Inputs = normalized
		processed.InputOrigins = origins
	}
	if len(processed.DependsOn) > 0 {
		evaluated, err := p.evaluateStringSlice(processed.DependsOn, facet.Path, facetScope)
		if err != nil {
			return werror.Wrap(err, "error evaluating dependsOn for component '%s'", processed.GetID())
		}
		processed.DependsOn = slices.DeleteFunc(evaluated, func(s string) bool { return s == "" })
	}
	if processed.Destroy != nil && processed.Destroy.IsExpr {
		evaluated, err := p.evaluateBooleanExpression(processed.Destroy.Expr, facet.Path, facetScope)
		if err != nil {
			return werror.Wrap(err, "error evaluating destroy for component '%s'", processed.GetID())
		}
		processed.Destroy = &blueprintv1alpha1.BoolExpression{Value: evaluated, IsExpr: false}
	}
	if processed.Parallelism != nil && processed.Parallelism.IsExpr {
		evaluated, err := p.evaluateIntegerExpression(processed.Parallelism.Expr, facet.Path, facetScope)
		if err != nil {
			return werror.Wrap(err, "error evaluating parallelism for component '%s'", processed.GetID())
		}
		processed.Parallelism = &blueprintv1alpha1.IntExpression{Value: evaluated, IsExpr: false}
	}
	if processed.Source == "" {
		processed.Source = resolveSourceName(sourceName)
	}

	effectiveOrdinal := resolveOrdinal(facet, processed.Ordinal)
	processed.Ordinal = &effectiveOrdinal

	strategy := processed.Strategy
	if strategy == "" {
		strategy = "merge"
	}

	componentID := processed.GetID()
	sn := ""
	if len(sourceName) > 0 {
		sn = sourceName[0]
	}
	entryKey := traceEntryKey(sn, "terraform", componentID)
	if p.traceCollector != nil {
		for k, v := range tc.Inputs {
			composedPath := "terraform." + componentID + ".inputs." + k
			p.recordTrace(composedPath, TraceContribution{
				FacetPath:  facet.Path,
				SourceName: sn,
				Ordinal:    effectiveOrdinal,
				Strategy:   strategy,
				Line:       yamlNodeLine(facet.Path, "terraform", namedItem(componentID), "inputs", k),
				RawValue:   deepCopyValue(v),
				Entry:      entryKey,
			})
		}
	}

	if strategy == "remove" {
		if err := p.addTerraformRemoval(collected, componentID, processed, entryKey); err != nil {
			return err
		}
		return nil
	}
	stripped, err := stripTerraformComponent(processed.TerraformComponent, removalItems(collected.terraformRemovals[componentID], effectiveOrdinal))
	if err != nil {
		return werror.Wrap(err, "error removing fields from terraform component '%s'", componentID)
	}
	processed.TerraformComponent = stripped

	previous := terraformByID[componentID]
	if previous == nil {
		processed.Strategy = strategy
		terraformByID[componentID] = &processed
	} else if err := p.updateTerraformComponentEntry(componentID, &processed, strategy, terraformByID, facetScope); err != nil {
		return err
	}
	settleEntryTrace(p, entryKey, previous, &processed, terraformByID[componentID])
	return nil
}

// collectByEntryOrdinal collects the terraform components, kustomizations, and flux systems of the
// facets of one source into collected. It collects each kind in the order of the effective ordinal
// of each entry, so a per-entry ordinal moves the entry in the merge. Entries with equal ordinals
// keep the facet order. Thus each removal gets only the entries with an equal or lower ordinal.
func (p *BaseBlueprintProcessor) collectByEntryOrdinal(facets []blueprintv1alpha1.Facet, sourceName []string, collected collectedComponents, scope map[string]any) error {
	terraformOrder := entriesByOrdinal(facets,
		func(f blueprintv1alpha1.Facet) []blueprintv1alpha1.ConditionalTerraformComponent {
			return f.TerraformComponents
		},
		func(e blueprintv1alpha1.ConditionalTerraformComponent) *int { return e.Ordinal })
	for _, ref := range terraformOrder {
		facet := facets[ref.facet]
		if err := p.collectTerraformComponent(facet, facet.TerraformComponents[ref.entry], sourceName, collected, scope); err != nil {
			return err
		}
	}
	kustomizationOrder := entriesByOrdinal(facets,
		func(f blueprintv1alpha1.Facet) []blueprintv1alpha1.ConditionalKustomization { return f.Kustomizations },
		func(e blueprintv1alpha1.ConditionalKustomization) *int { return e.Ordinal })
	for _, ref := range kustomizationOrder {
		facet := facets[ref.facet]
		if err := p.collectKustomization(facet, facet.Kustomizations[ref.entry], sourceName, collected, scope); err != nil {
			return err
		}
	}
	fluxOrder := entriesByOrdinal(facets,
		func(f blueprintv1alpha1.Facet) []blueprintv1alpha1.FluxSystem { return f.FluxSystems },
		func(e blueprintv1alpha1.FluxSystem) *int { return e.Ordinal })
	for _, ref := range fluxOrder {
		if err := p.collectFluxSystem(facets[ref.facet], ref.entry, sourceName, collected, scope); err != nil {
			return err
		}
	}
	return nil
}

// recordKustomizationTraces records the source of each substitution and component of a kustomization,
// so `windsor explain` can name the facet, source, and line of each value. It does nothing without a collector.
func (p *BaseBlueprintProcessor) recordKustomizationTraces(name string, rawComponents []string, rawSubstitutions map[string]string, facetPath, sourceName string, ordinal int, strategy string) {
	if p.traceCollector == nil {
		return
	}
	entryKey := traceEntryKey(sourceName, "kustomize", name)
	for sk, sv := range rawSubstitutions {
		p.recordTrace(kustomizeSubstitutionPrefix(name)+sk, TraceContribution{
			FacetPath:  facetPath,
			SourceName: sourceName,
			Ordinal:    ordinal,
			Strategy:   strategy,
			Line:       yamlNodeLine(facetPath, "kustomize", namedItem(name), "substitutions", sk),
			RawValue:   deepCopyValue(sv),
			Entry:      entryKey,
		})
	}
	if len(rawComponents) > 0 {
		p.recordTrace("kustomize."+name+".components", TraceContribution{
			FacetPath:     facetPath,
			SourceName:    sourceName,
			Ordinal:       ordinal,
			Strategy:      strategy,
			Line:          yamlNodeLine(facetPath, "kustomize", namedItem(name), "components"),
			RawComponents: slices.Clone(rawComponents),
			Entry:         entryKey,
		})
	}
}

// collectKustomization collects one kustomization of a facet into collected. It evaluates the when
// condition, substitutions, and source of the kustomization. Entries merge by ordinal, then replace >
// merge. A remove entry goes to addKustomizationRemoval. A removal with an equal or higher ordinal
// strips its fields from a merge or replace entry. A kustomization with no when takes the facet
// condition. When facetScope is not nil, expressions can read config block values. Evaluation errors return.
func (p *BaseBlueprintProcessor) collectKustomization(facet blueprintv1alpha1.Facet, k blueprintv1alpha1.ConditionalKustomization, sourceName []string, collected collectedComponents, facetScope map[string]any) error {
	kustomizationByName := collected.kustomizationByName
	componentWhen := k.When
	if componentWhen == "" && facet.When != "" {
		componentWhen = facet.When
	}

	shouldInclude, err := p.shouldIncludeComponent(componentWhen, facet.Path, facetScope)
	if err != nil {
		return werror.Wrap(err, "error evaluating kustomization condition")
	}
	if !shouldInclude {
		return nil
	}

	processed := k
	processed.When = componentWhen
	if err := p.evalKustomizationSubstitutions(&processed.Kustomization, facet.Path, kustomizeSubstitutionPrefix(processed.Name), facetScope); err != nil {
		return werror.Wrap(err, "error evaluating substitutions for kustomization '%s'", processed.Name)
	}
	if evaluator.ContainsExpression(processed.Path) {
		evaluatedPath, err := p.evaluator.Evaluate(processed.Path, facet.Path, facetScope, false)
		if err != nil {
			return werror.Wrap(err, "error evaluating path for kustomization '%s'", processed.Name)
		}
		if containsDeferredInValue(evaluatedPath) {
			expr, _ := evaluator.DeferredExpression(evaluatedPath)
			return werror.New(fmt.Sprintf("kustomization '%s' path references a value that is not yet available: %s", processed.Name, expr), "Use a kustomization path that Windsor can resolve at compose time.", nil)
		}
		if pathStr, ok := evaluatedPath.(string); ok {
			processed.Path = pathStr
		} else if evaluatedPath != nil {
			processed.Path = fmt.Sprintf("%v", evaluatedPath)
		}
	}
	dependsOn, err := p.evalDropEmpty(processed.DependsOn, facet.Path, facetScope)
	if err != nil {
		return werror.Wrap(err, "error evaluating dependsOn for kustomization '%s'", processed.Name)
	}
	processed.DependsOn = dependsOn
	if len(processed.Components) > 0 {
		evaluated, err := p.evaluateStringSlice(processed.Components, facet.Path, facetScope)
		if err != nil {
			return werror.Wrap(err, "error evaluating components for kustomization '%s'", processed.Name)
		}
		processed.Components = evaluated
	}
	if processed.Destroy != nil && processed.Destroy.IsExpr {
		evaluated, err := p.evaluateBooleanExpression(processed.Destroy.Expr, facet.Path, facetScope)
		if err != nil {
			return werror.Wrap(err, "error evaluating destroy for kustomization '%s'", processed.Name)
		}
		processed.Destroy = &blueprintv1alpha1.BoolExpression{Value: evaluated, IsExpr: false}
	}
	if processed.Source == "" {
		processed.Source = resolveSourceName(sourceName)
	}

	effectiveOrdinal := resolveOrdinal(facet, processed.Ordinal)
	processed.Ordinal = &effectiveOrdinal

	strategy := processed.Strategy
	if strategy == "" {
		strategy = "merge"
	}

	sn := ""
	if len(sourceName) > 0 {
		sn = sourceName[0]
	}

	addEntry := func(entry blueprintv1alpha1.ConditionalKustomization) error {
		entryKey := traceEntryKey(sn, "kustomize", entry.Name)
		if strategy == "remove" {
			return p.addKustomizationRemoval(collected, entry, entryKey)
		}
		stripped, err := stripKustomization(entry.Kustomization, removalItems(collected.kustomizationRemovals[entry.Name], effectiveOrdinal))
		if err != nil {
			return werror.Wrap(err, "error removing fields from kustomization '%s'", entry.Name)
		}
		entry.Kustomization = stripped
		previous := kustomizationByName[entry.Name]
		if previous == nil {
			entry.Strategy = strategy
			stored := new(blueprintv1alpha1.ConditionalKustomization)
			*stored = entry
			kustomizationByName[entry.Name] = stored
		} else if err := p.updateKustomizationEntry(entry.Name, &entry, strategy, kustomizationByName, facetScope); err != nil {
			return err
		}
		settleEntryTrace(p, entryKey, previous, &entry, kustomizationByName[entry.Name])
		return nil
	}

	p.recordKustomizationTraces(processed.Name, k.Components, mergeSubstitute(k.Kustomization), facet.Path, sn, effectiveOrdinal, strategy)
	return addEntry(processed)
}

// resolveSourceName returns the explicit source for a facet-derived component: the loader's
// source name, unless it is empty or the reserved "primary" sentinel, in which case the
// component resolves against the default source (represented as an empty string).
func resolveSourceName(sourceName []string) string {
	if len(sourceName) > 0 && sourceName[0] != "" && sourceName[0] != "primary" {
		return sourceName[0]
	}
	return ""
}

// validateCrdRef returns an error if a CRD reference is not a single path segment.
// A reference with '/', '\', or '..' could point outside the kustomize/crds/ catalog. The error has a remediation hint.
func validateCrdRef(ref string) error {
	if strings.ContainsAny(ref, "/\\") || strings.Contains(ref, "..") {
		return werror.New(fmt.Sprintf("invalid crd reference %q", ref), "Use a single path segment without '/', '\\', or '..' for the CRD reference.", nil)
	}
	return nil
}

// mergeSubstitute returns k's Substitutions with its Substitute alias merged in (substitute wins on
// key conflict). Returns nil when both are empty.
func mergeSubstitute(k blueprintv1alpha1.Kustomization) map[string]string {
	if len(k.Substitute) == 0 {
		return k.Substitutions
	}
	out := maps.Clone(k.Substitutions)
	if out == nil {
		out = make(map[string]string, len(k.Substitute))
	}
	maps.Copy(out, k.Substitute)
	return out
}

// validateSystemSecrets checks the secrets: entries of a flux system. Each entry must have data keys
// in the form data: {<key>: <ref>}. Each value must contain a complete ${...} expression, a reference or a secret() call.
// It does not accept a plaintext literal, because git stores the blueprint file. A type must be Opaque or
// kubernetes.io/tls. ValidateComposedBlueprint checks the TLS data keys after facets merge.
// Each error has a remediation hint.
func (p *BaseBlueprintProcessor) validateSystemSecrets(systemName string, secrets map[string]blueprintv1alpha1.SecretEntry) error {
	for secretName, entry := range secrets {
		if len(entry.Data) == 0 {
			return werror.New(fmt.Sprintf("secret %q in flux system %q declares no data keys", secretName, systemName), "Put the keys under data: in the form data: {<key>: <ref>}. A namespaces: selector is optional.", nil)
		}
		switch entry.Type {
		case "", secretTypeOpaque, secretTypeTLS:
		default:
			return werror.New(fmt.Sprintf("secret %q in flux system %q has unsupported type %q", secretName, systemName, entry.Type), fmt.Sprintf("Set type: to %s or %s, or remove it.", secretTypeOpaque, secretTypeTLS), nil)
		}
		for key, ref := range entry.Data {
			open := strings.Index(ref, "${")
			if open < 0 || !strings.Contains(ref[open+2:], "}") {
				return werror.New(fmt.Sprintf("secret %q key %q in flux system %q must contain a complete ${...} expression, got %q", secretName, key, systemName, ref), "Set the value to a ${...} reference or a secret() call. Do not use a plaintext literal.", nil)
			}
		}
	}
	return nil
}

// isSensitivePath reports whether the config path is marked sensitive in the schema. It returns false
// when no config handler is available (e.g. isolated composer tests), so callers that reject sensitive
// references skip the check rather than failing spuriously.
func (p *BaseBlueprintProcessor) isSensitivePath(path string) bool {
	return p.runtime != nil && p.runtime.ConfigHandler != nil && p.runtime.ConfigHandler.IsSensitivePath(path)
}

// sensitivePathsInValue returns the sensitive config paths whose values can reach the result of the
// ${...} expressions in value. It finds a bare reference ("${cdn.key}"), an embedded one
// ("https://x/${cdn.key}"), and one inside a larger expression ("${'Bearer ' + cdn.key}"). It allows a
// path that only feeds a boolean, such as the condition in "${cdn.key != nil ? 'a' : 'b'}". It skips an
// expression that does not parse.
func (p *BaseBlueprintProcessor) sensitivePathsInValue(value string) []string {
	var found []string
	for _, ref := range extractValueRefs(value) {
		if p.isSensitivePath(ref) {
			found = append(found, ref)
		}
	}
	return found
}

// mergeSecretData unions two secret maps (Secret name -> SecretEntry), overlay winning per data key and
// on a non-empty type, and the two entries' namespaces unioned, and nil when both are empty. Neither input is mutated. Used to
// union a flux system's Secrets across facets.
func mergeSecretData(base, overlay map[string]blueprintv1alpha1.SecretEntry) map[string]blueprintv1alpha1.SecretEntry {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := make(map[string]blueprintv1alpha1.SecretEntry, len(base)+len(overlay))
	for name, entry := range base {
		out[name] = entry.DeepCopy()
	}
	for name, entry := range overlay {
		merged := out[name]
		for _, ns := range entry.Namespaces {
			if !slices.Contains(merged.Namespaces, ns) {
				merged.Namespaces = append(merged.Namespaces, ns)
			}
		}
		if len(entry.Data) > 0 {
			if merged.Data == nil {
				merged.Data = make(map[string]string, len(entry.Data))
			}
			maps.Copy(merged.Data, entry.Data)
		}
		if entry.Type != "" {
			merged.Type = entry.Type
		}
		out[name] = merged
	}
	return out
}

// kustomizeSubstitutionPrefix returns the composed-path prefix of the substitutions of a kustomization.
func kustomizeSubstitutionPrefix(name string) string {
	return "kustomize." + name + ".substitutions."
}

// fluxResourcesSubstitutionPrefix returns the deferred-path prefix for a flux system's resources
// tier substitutions, scoped per resources variant (empty variantName for the unnamed variant) so
// two variants sharing a substitution key name don't collide on the same deferred-path entry.
func fluxResourcesSubstitutionPrefix(systemName, variantName string) string {
	prefix := "flux." + systemName + ".resources"
	if variantName != "" {
		prefix += "-" + variantName
	}
	return prefix + ".substitutions."
}

// combineWhen ANDs two condition expressions, dropping empties. A system entry's own condition is
// combined with each resources variant's.
func combineWhen(a, b string) string {
	switch {
	case a != "" && b != "":
		return fmt.Sprintf("(%s) && (%s)", a, b)
	case a != "":
		return a
	default:
		return b
	}
}

// evalDropEmpty evaluates a slice of expressions against the facet scope and drops entries that
// resolved to empty.
func (p *BaseBlueprintProcessor) evalDropEmpty(raw []string, facetPath string, scope map[string]any) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	evaluated, err := p.evaluateStringSlice(raw, facetPath, scope)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(evaluated, func(s string) bool { return s == "" }), nil
}

// collectFluxSystem evaluates the flux system at systemIndex of a facet into collected. The
// index locates the system in the facet file for the trace. A remove system goes to
// addFluxSystemRemoval, and an equal or higher-ordinal removal strips its fields from a merge system.
// Only a system's own when controls its inclusion, and collect then clears the when. An empty
// component list never drops an install tier or a resources variant. A resources variant with no
// Timeout or Interval takes both from the install tier.
func (p *BaseBlueprintProcessor) collectFluxSystem(facet blueprintv1alpha1.Facet, systemIndex int, sourceName []string, collected collectedComponents, facetScope map[string]any) error {
	fluxSystemByName := collected.fluxSystemByName
	system := facet.FluxSystems[systemIndex]
	when := system.When
	if when == "" && facet.When != "" {
		when = facet.When
	}
	shouldInclude, err := p.shouldIncludeComponent(when, facet.Path, facetScope)
	if err != nil {
		return werror.Wrap(err, "error evaluating flux system condition")
	}
	if !shouldInclude {
		return nil
	}
	system.When = when
	if system.Source == "" {
		system.Source = resolveSourceName(sourceName)
	}

	if err := p.validateSystemSecrets(system.Name, system.Secrets); err != nil {
		return err
	}

	effectiveOrdinal := resolveOrdinal(facet, system.Ordinal)
	strategy := system.Strategy
	if strategy == "" {
		strategy = "merge"
	}

	deps, err := p.evalDropEmpty(system.DependsOn, facet.Path, facetScope)
	if err != nil {
		return werror.Wrap(err, "error evaluating dependsOn for system '%s'", system.Name)
	}
	system.DependsOn = deps

	if system.Install != nil {
		comps, err := p.evalDropEmpty(system.Install.Components, facet.Path, facetScope)
		if err != nil {
			return werror.Wrap(err, "error evaluating install components for system '%s'", system.Name)
		}
		installCopy := *system.Install.DeepCopy()
		installCopy.Components = comps
		if err := p.evalKustomizationSubstitutions(&installCopy, facet.Path, "flux."+system.Name+".install.substitutions.", facetScope); err != nil {
			return werror.Wrap(err, "error evaluating install substitutions for system '%s'", system.Name)
		}

		p.recordFluxTierTraces(system.Name, system.InstallTierName(), []any{"flux", seqIndex(systemIndex), "install"}, *system.Install, comps, facet.Path, sourceName, effectiveOrdinal, strategy)
		system.Install = &installCopy
	}

	seenVariants := make(map[string]bool)
	var evaluatedResources []blueprintv1alpha1.FluxVariant
	for variantIndex, v := range system.Resources {
		combined := combineWhen(system.When, v.When)
		include, err := p.shouldIncludeComponent(combined, facet.Path, facetScope)
		if err != nil {
			return werror.Wrap(err, "error evaluating resources condition for system '%s'", system.Name)
		}
		if !include {
			continue
		}
		comps, err := p.evalDropEmpty(v.Components, facet.Path, facetScope)
		if err != nil {
			return werror.Wrap(err, "error evaluating resources components for system '%s'", system.Name)
		}
		variantKey := system.ResourcesTierName(v.Name)
		if seenVariants[variantKey] {
			return werror.New(fmt.Sprintf("duplicate resources variant %q in system %q", variantKey, system.Name), "Add a unique name: field to each resources variant.", nil)
		}
		seenVariants[variantKey] = true

		evalV := blueprintv1alpha1.FluxVariant{Kustomization: *v.Kustomization.DeepCopy()}
		if system.Install != nil {
			if evalV.Timeout == nil {
				evalV.Timeout = system.Install.Timeout
			}
			if evalV.Interval == nil {
				evalV.Interval = system.Install.Interval
			}
		}
		evalV.Components = comps
		evalV.When = ""

		extraDeps, err := p.evalDropEmpty(v.DependsOn, facet.Path, facetScope)
		if err != nil {
			return werror.Wrap(err, "error evaluating resources dependsOn for system '%s'", system.Name)
		}
		evalV.DependsOn = extraDeps

		resourcesDeferredPrefix := fluxResourcesSubstitutionPrefix(system.Name, v.Name)
		if err := p.evalKustomizationSubstitutions(&evalV.Kustomization, facet.Path, resourcesDeferredPrefix, facetScope); err != nil {
			return werror.Wrap(err, "error evaluating resources substitutions for system '%s'", system.Name)
		}

		p.recordFluxTierTraces(system.Name, variantKey, []any{"flux", seqIndex(systemIndex), "resources", seqIndex(variantIndex)}, v.Kustomization, comps, facet.Path, sourceName, effectiveOrdinal, strategy)
		evaluatedResources = append(evaluatedResources, evalV)
	}
	system.Resources = evaluatedResources
	system.Ordinal = &effectiveOrdinal
	system.Strategy = strategy
	system.When = ""

	sn := ""
	if len(sourceName) > 0 {
		sn = sourceName[0]
	}
	entryKey := traceEntryKey(sn, "flux", system.Name)
	if strategy == "remove" {
		if err := p.addFluxSystemRemoval(collected, system, entryKey); err != nil {
			return err
		}
		return nil
	}
	stripped, err := stripFluxSystem(system, removalItems(collected.fluxSystemRemovals[system.Name], effectiveOrdinal))
	if err != nil {
		return werror.Wrap(err, "error removing fields from flux system '%s'", system.Name)
	}
	system = stripped

	previous := fluxSystemByName[system.Name]
	if previous == nil {
		stored := system
		fluxSystemByName[system.Name] = &stored
	} else if err := p.updateFluxSystemEntry(system.Name, &system, strategy, fluxSystemByName); err != nil {
		return err
	}
	settleEntryTrace(p, entryKey, previous, &system, fluxSystemByName[system.Name])
	return nil
}

// recordFluxTierTraces records the source of each substitution and component of one compiled flux tier
// under its kustomize.<tier> path. The tierPath segments locate the tier in the facet file. The raw value
// holds the unevaluated tier, and components holds its evaluated list. It does nothing without a collector.
func (p *BaseBlueprintProcessor) recordFluxTierTraces(system, tier string, tierPath []any, raw blueprintv1alpha1.Kustomization, components []string, facetPath string, sourceName []string, ordinal int, strategy string) {
	if p.traceCollector == nil {
		return
	}
	sn := ""
	if len(sourceName) > 0 {
		sn = sourceName[0]
	}
	entryKey := traceEntryKey(sn, "flux", system)
	for key, value := range mergeSubstitute(raw) {
		field := "substitutions"
		if _, ok := raw.Substitute[key]; ok {
			field = "substitute"
		}
		p.recordTrace("kustomize."+tier+".substitutions."+key, TraceContribution{
			FacetPath:  facetPath,
			SourceName: sn,
			Ordinal:    ordinal,
			Strategy:   strategy,
			Line:       yamlNodeLine(facetPath, slices.Concat(tierPath, []any{field, key})...),
			RawValue:   value,
			Entry:      entryKey,
		})
	}
	if len(raw.Components) == 0 {
		return
	}
	lines := make(map[string]int, len(components))
	for _, component := range components {
		lines[component] = yamlNodeLine(facetPath, slices.Concat(tierPath, []any{"components", seqEntryMatch{value: component}})...)
	}
	p.recordTrace("kustomize."+tier+".components", TraceContribution{
		FacetPath:      facetPath,
		SourceName:     sn,
		Ordinal:        ordinal,
		Strategy:       strategy,
		Line:           yamlNodeLine(facetPath, slices.Concat(tierPath, []any{"components"})...),
		RawComponents:  slices.Clone(raw.Components),
		ComponentLines: lines,
		Entry:          entryKey,
	})
}

// evalKustomizationSubstitutions merges substitute into substitutions, clears substitute, evaluates
// all substitution values, and marks any deferred keys under deferredPrefix.
func (p *BaseBlueprintProcessor) evalKustomizationSubstitutions(k *blueprintv1alpha1.Kustomization, facetPath, deferredPrefix string, scope map[string]any) error {
	subs := mergeSubstitute(*k)
	k.Substitute = nil
	k.Substitutions = nil
	if len(subs) == 0 {
		return nil
	}
	evaluated, deferredKeys, err := p.evaluateSubstitutions(subs, facetPath, scope)
	if err != nil {
		return err
	}
	k.Substitutions = evaluated
	for key, isDeferred := range deferredKeys {
		if isDeferred {
			p.markDeferredPath(deferredPrefix + key)
		}
	}
	return nil
}

// entriesByOrdinal returns a reference to each entry of one kind in facets, in ascending order of
// the effective ordinal of the entry. entries returns the entries of one facet, and ordinal returns
// the per-entry ordinal of one entry, or nil. Entries with equal ordinals keep the facet order.
func entriesByOrdinal[E any](facets []blueprintv1alpha1.Facet, entries func(blueprintv1alpha1.Facet) []E, ordinal func(E) *int) []entryRef {
	var refs []entryRef
	for fi, facet := range facets {
		for ei, entry := range entries(facet) {
			refs = append(refs, entryRef{facet: fi, entry: ei, ordinal: resolveOrdinal(facet, ordinal(entry))})
		}
	}
	slices.SortStableFunc(refs, func(a, b entryRef) int {
		return cmp.Compare(a.ordinal, b.ordinal)
	})
	return refs
}
