// The processor_collect file holds the component collection steps of the BlueprintProcessor.
// It provides collection of terraform components, kustomizations, and flux systems from a facet.
// It evaluates the conditions and values of each entry against the facet scope.
// It validates CRD references and flux system secrets, and rejects sensitive substitutions.

package blueprint

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Private Methods
// =============================================================================

// collectTerraformComponents collects the terraform components of a facet into terraformByID. It
// evaluates the when condition, inputs, and source of each component, and merges entries by strategy.
// A component with no when takes the facet condition. When facetScope is not nil, expressions can read
// config block values. Evaluation errors return.
func (p *BaseBlueprintProcessor) collectTerraformComponents(
	facet blueprintv1alpha1.Facet,
	sourceName []string,
	terraformByID map[string]*blueprintv1alpha1.ConditionalTerraformComponent,
	facetScope map[string]any,
) error {
	for _, tc := range facet.TerraformComponents {
		componentWhen := tc.When
		if componentWhen == "" && facet.When != "" {
			componentWhen = facet.When
		}

		shouldInclude, err := p.shouldIncludeComponent(componentWhen, facet.Path, facetScope)
		if err != nil {
			return werror.Wrap(err, "error evaluating terraform component condition")
		}
		if !shouldInclude {
			continue
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
		for k, v := range tc.Inputs {
			composedPath := "terraform." + componentID + ".inputs." + k
			p.recordTrace(composedPath, TraceContribution{
				FacetPath:  facet.Path,
				SourceName: sn,
				Ordinal:    effectiveOrdinal,
				Strategy:   strategy,
				Line:       yamlNodeLine(facet.Path, "terraform", namedItem(componentID), "inputs", k),
				RawValue:   deepCopyValue(v),
			})
		}

		if _, exists := terraformByID[componentID]; !exists {
			processed.Strategy = strategy
			terraformByID[componentID] = &processed
		} else {
			if err := p.updateTerraformComponentEntry(componentID, &processed, strategy, terraformByID, facetScope); err != nil {
				return err
			}
		}
	}
	return nil
}

// recordKustomizationTraces records the source of each substitution and component of a kustomization,
// so `windsor explain` can name the facet, source, and line of each value.
func (p *BaseBlueprintProcessor) recordKustomizationTraces(name string, rawComponents []string, rawSubstitutions map[string]string, facetPath, sourceName string, ordinal int, strategy string) {
	for sk, sv := range rawSubstitutions {
		p.recordTrace("kustomize."+name+".substitutions."+sk, TraceContribution{
			FacetPath:  facetPath,
			SourceName: sourceName,
			Ordinal:    ordinal,
			Strategy:   strategy,
			Line:       yamlNodeLine(facetPath, "kustomize", namedItem(name), "substitutions", sk),
			RawValue:   deepCopyValue(sv),
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
		})
	}
}

// collectKustomizations collects the kustomizations of a facet into kustomizationByName. It evaluates
// the when condition, substitutions, and source of each kustomization. Entries merge by ordinal, then
// remove > replace > merge. A kustomization with no when takes the facet condition. When facetScope is
// not nil, expressions can read config block values. Evaluation errors return.
func (p *BaseBlueprintProcessor) collectKustomizations(facet blueprintv1alpha1.Facet, sourceName []string, kustomizationByName map[string]*blueprintv1alpha1.ConditionalKustomization, facetScope map[string]any) error {
	for _, k := range facet.Kustomizations {
		componentWhen := k.When
		if componentWhen == "" && facet.When != "" {
			componentWhen = facet.When
		}

		shouldInclude, err := p.shouldIncludeComponent(componentWhen, facet.Path, facetScope)
		if err != nil {
			return werror.Wrap(err, "error evaluating kustomization condition")
		}
		if !shouldInclude {
			continue
		}

		processed := k
		processed.When = componentWhen
		if err := p.evalKustomizationSubstitutions(&processed.Kustomization, facet.Path, "kustomize."+processed.Name+".substitutions.", facetScope); err != nil {
			return werror.Wrap(err, "error evaluating substitutions for kustomization '%s'", processed.Name)
		}
		if evaluator.ContainsExpression(processed.Path) {
			evaluatedPath, err := p.evaluator.Evaluate(processed.Path, facet.Path, facetScope, false)
			if err != nil {
				return werror.Wrap(err, "error evaluating path for kustomization '%s'", processed.Name)
			}
			if containsDeferredInValue(evaluatedPath) {
				expr, _ := evaluator.DeferredExpression(evaluatedPath)
				return fmt.Errorf("kustomization '%s' path references a value that is not yet available: %s; a path must resolve at compose time", processed.Name, expr)
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
			if _, exists := kustomizationByName[entry.Name]; !exists {
				entry.Strategy = strategy
				stored := new(blueprintv1alpha1.ConditionalKustomization)
				*stored = entry
				kustomizationByName[entry.Name] = stored
				return nil
			}
			return p.updateKustomizationEntry(entry.Name, &entry, strategy, kustomizationByName, facetScope)
		}

		p.recordKustomizationTraces(processed.Name, k.Components, k.Substitutions, facet.Path, sn, effectiveOrdinal, strategy)
		if err := addEntry(processed); err != nil {
			return err
		}
	}
	return nil
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

// validateCrdRef rejects a CRD reference that would escape the kustomize/crds/ catalog when
// concatenated into a path. A reference is a single directory segment, so any path separator or
// parent-directory token is a malformed (or malicious) value.
func validateCrdRef(ref string) error {
	if strings.ContainsAny(ref, "/\\") || strings.Contains(ref, "..") {
		return fmt.Errorf("invalid crd reference %q: must be a single path segment without '/', '\\', or '..'", ref)
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

// validateSystemSecrets rejects a malformed secrets: entry of a flux system. An entry must have data
// keys, in the form `data: {<key>: <ref>}` with an optional `namespaces:` selector; an empty entry would
// never create a secret. Each value must be a complete ${...} expression, a reference or a secret()
// call. A plaintext literal is rejected, because the blueprint file holds it and git would store it.
func (p *BaseBlueprintProcessor) validateSystemSecrets(systemName string, secrets map[string]blueprintv1alpha1.SecretEntry) error {
	for secretName, entry := range secrets {
		if len(entry.Data) == 0 {
			return fmt.Errorf("secret %q in flux system %q declares no data keys; nest them under `data:` (an entry is `data: {<key>: <ref>}` plus an optional `namespaces:` selector)", secretName, systemName)
		}
		for key, ref := range entry.Data {
			open := strings.Index(ref, "${")
			if open < 0 || !strings.Contains(ref[open+2:], "}") {
				return fmt.Errorf("secret %q key %q in flux system %q must contain a complete ${...} expression (a reference or a secret() call), not a plaintext literal or unterminated expression, got %q", secretName, key, systemName, ref)
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

// sensitivePathsInValue returns the sensitive config paths referenced as bare "${path}" interpolation
// segments anywhere in value — whole-string ("${cdn.key}") or embedded ("https://x/${cdn.key}",
// "${cdn.key}-suffix"). A segment whose inner text is not a plain config path (it contains operators,
// a function call, or quotes) can't be resolved to a single sensitive property and is skipped, so a
// sensitive path used inside a larger expression remains the one uncaught form.
func (p *BaseBlueprintProcessor) sensitivePathsInValue(value string) []string {
	var found []string
	rest := value
	for {
		start := strings.Index(rest, "${")
		if start < 0 {
			break
		}
		rest = rest[start+2:]
		end := strings.Index(rest, "}")
		if end < 0 {
			break
		}
		inner := strings.TrimSpace(rest[:end])
		if inner != "" && !strings.ContainsAny(inner, "(){}\"' \t\n\r") && p.isSensitivePath(inner) {
			found = append(found, inner)
		}
		rest = rest[end+1:]
	}
	return found
}

// mergeSecretData unions two secret maps (Secret name -> SecretEntry), overlay winning per data key and
// the two entries' namespaces unioned, and nil when both are empty. Neither input is mutated. Used to
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
		out[name] = merged
	}
	return out
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

// collectFluxSystems evaluates a facet's flux: system descriptors and stores the result in
// fluxSystemByName. It clears each system's when once inclusion is decided. Neither an install
// tier nor a resources variant is dropped for an empty component list. Only a system's own
// when controls whether it is included at all. A resources variant with no Timeout/Interval of
// its own inherits both from the system's install tier; an explicit value on the variant always
// wins.
func (p *BaseBlueprintProcessor) collectFluxSystems(facet blueprintv1alpha1.Facet, sourceName []string, fluxSystemByName map[string]*blueprintv1alpha1.FluxSystem, facetScope map[string]any) error {
	for _, system := range facet.FluxSystems {
		when := system.When
		if when == "" && facet.When != "" {
			when = facet.When
		}
		shouldInclude, err := p.shouldIncludeComponent(when, facet.Path, facetScope)
		if err != nil {
			return werror.Wrap(err, "error evaluating flux system condition")
		}
		if !shouldInclude {
			continue
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
			system.Install = &installCopy
		}

		seenVariants := make(map[string]bool)
		var evaluatedResources []blueprintv1alpha1.FluxVariant
		for _, v := range system.Resources {
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
			variantKey := system.Name + "-resources"
			if v.Name != "" {
				variantKey += "-" + v.Name
			}
			if seenVariants[variantKey] {
				return fmt.Errorf("duplicate resources variant %q in system %q; add a unique name: field to each variant", variantKey, system.Name)
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
			evaluatedResources = append(evaluatedResources, evalV)
		}
		system.Resources = evaluatedResources
		system.Ordinal = &effectiveOrdinal
		system.Strategy = strategy
		system.When = ""

		if _, exists := fluxSystemByName[system.Name]; !exists {
			stored := system
			fluxSystemByName[system.Name] = &stored
			continue
		}
		if err := p.updateFluxSystemEntry(system.Name, &system, strategy, fluxSystemByName); err != nil {
			return err
		}
	}
	return nil
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
