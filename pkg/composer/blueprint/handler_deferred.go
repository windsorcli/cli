package blueprint

import (
	"fmt"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Private Methods
// =============================================================================

// resolveDeferredSubstitutions re-evaluates deferred substitutions with evaluateDeferred=true
// so that helpers like terraform_output() resolve using outputs available at generate time
// (after terraform apply). Only substitutions marked as deferred during composition are
// re-evaluated; already-resolved substitutions are left untouched. Covers global substitutions,
// blueprint-level ConfigMaps, per-kustomization substitutions, and flux: systems' install/
// resources tier substitutions.
//
// Returns the first evaluation error encountered, with the substitution path named (e.g.
// "kustomize.dns.substitutions.external_dns_tenant_id"), surfacing the problem at
// blueprint-resolution time, before anything is written to the cluster.
func (h *BaseBlueprintHandler) resolveDeferredSubstitutions() error {
	if h.runtime == nil || h.runtime.Evaluator == nil || h.composedBlueprint == nil {
		return nil
	}

	deferred := h.GetDeferredPaths()
	if len(deferred) == 0 {
		return nil
	}

	bp := h.composedBlueprint

	for key, value := range bp.Substitutions {
		if !deferred["substitutions."+key] {
			continue
		}
		str, err := h.resolveDeferred(value)
		if err != nil {
			return fmt.Errorf("substitutions.%s: %w", key, err)
		}
		bp.Substitutions[key] = str
	}

	for name, configMap := range bp.ConfigMaps {
		for key, value := range configMap {
			if !deferred["configmaps."+name+"."+key] {
				continue
			}
			str, err := h.resolveDeferred(value)
			if err != nil {
				return fmt.Errorf("configmaps.%s.%s: %w", name, key, err)
			}
			bp.ConfigMaps[name][key] = str
		}
	}

	for i := range bp.Kustomizations {
		k := &bp.Kustomizations[i]
		for key, value := range k.Substitutions {
			if !deferred["kustomize."+k.Name+".substitutions."+key] {
				continue
			}
			str, err := h.resolveDeferred(value)
			if err != nil {
				return fmt.Errorf("kustomize.%s.substitutions.%s: %w", k.Name, key, err)
			}
			k.Substitutions[key] = str
		}
	}

	for i := range bp.FluxSystems {
		sys := &bp.FluxSystems[i]
		if sys.Install != nil {
			for key, value := range sys.Install.Substitutions {
				if !deferred["flux."+sys.Name+".install.substitutions."+key] {
					continue
				}
				str, err := h.resolveDeferred(value)
				if err != nil {
					return fmt.Errorf("flux.%s.install.substitutions.%s: %w", sys.Name, key, err)
				}
				sys.Install.Substitutions[key] = str
			}
		}
		for j := range sys.Resources {
			v := &sys.Resources[j]
			resourcesPrefix := fluxResourcesSubstitutionPrefix(sys.Name, v.Name)
			for key, value := range v.Substitutions {
				if !deferred[resourcesPrefix+key] {
					continue
				}
				str, err := h.resolveDeferred(value)
				if err != nil {
					return fmt.Errorf("%s%s: %w", resourcesPrefix, key, err)
				}
				v.Substitutions[key] = str
			}
		}
	}

	return nil
}

// resolveDeferred evaluates a single expression with evaluateDeferred=true using the composed
// scope. Returns the resolved string on success, or an error when the expression cannot be
// evaluated (e.g. terraform_output() references a missing module output, or terraform itself
// errors). Operators with a legitimately-optional reference should write `?? <fallback>` in the
// expression itself; nil from the evaluator (with no coalesce) renders as the empty string here,
// which is the operator's explicit choice rather than a silent failure.
func (h *BaseBlueprintHandler) resolveDeferred(expr string) (string, error) {
	resolved, err := h.runtime.Evaluator.Evaluate(expr, "", h.composedScope, true)
	if err != nil {
		return "", err
	}
	if resolved == nil {
		return "", nil
	}
	if str, ok := resolved.(string); ok {
		return str, nil
	}
	return fmt.Sprintf("%v", resolved), nil
}

// resolveMessages renders the composed blueprint's post-run messages against the resolved scope,
// where run values (e.g. terraform_output) are available. Each message's optional When is evaluated
// as a boolean gate — a message whose When is false or non-boolean is dropped — and its Text is
// interpolated. Only kept messages carry through, with When cleared (it has served its purpose) and
// Text replaced by the rendered result. Rendered text is deduplicated so multiple active facets
// emitting the same note surface it once, preserving included-facet order. Evaluation errors are
// surfaced at generate time, before anything reaches the operator or the cluster.
func (h *BaseBlueprintHandler) resolveMessages() error {
	if h.runtime == nil || h.runtime.Evaluator == nil || h.composedBlueprint == nil {
		return nil
	}
	bp := h.composedBlueprint
	if len(bp.Messages) == 0 {
		return nil
	}

	rendered := make([]blueprintv1alpha1.Message, 0, len(bp.Messages))
	seen := make(map[string]bool)
	for _, msg := range bp.Messages {
		if msg.When != "" {
			cond, err := h.runtime.Evaluator.Evaluate("${"+msg.When+"}", "", h.composedScope, true)
			if err != nil {
				return fmt.Errorf("message when %q: %w", msg.When, err)
			}
			if b, ok := cond.(bool); !ok || !b {
				continue
			}
		}
		text, err := h.resolveDeferred(msg.Text)
		if err != nil {
			return fmt.Errorf("message text %q: %w", msg.Text, err)
		}
		if text == "" || seen[text] {
			continue
		}
		seen[text] = true
		rendered = append(rendered, blueprintv1alpha1.Message{Text: text})
	}
	bp.Messages = rendered
	return nil
}

// deriveConfigMapDeferredPaths propagates deferred status to ConfigMap entries that inherited
// values from deferred substitutions. The composer copies blueprint.Substitutions into
// values-common and kustomization substitutions into per-kustomization ConfigMaps; this
// method marks the corresponding "configmaps.<name>.<key>" paths as deferred so that
// resolveDeferredSubstitutions and applyDeferredPathsToBlueprint handle them correctly.
func (h *BaseBlueprintHandler) deriveConfigMapDeferredPaths() {
	bp := h.composedBlueprint
	if bp == nil || len(bp.ConfigMaps) == 0 {
		return
	}

	h.deferredPathsMu.Lock()
	defer h.deferredPathsMu.Unlock()

	for name, configMap := range bp.ConfigMaps {
		for key, value := range configMap {
			if value != deferredPlaceholder {
				continue
			}
			if h.deferredPaths == nil {
				h.deferredPaths = make(map[string]bool)
			}
			h.deferredPaths["configmaps."+name+"."+key] = true
		}
	}
}

// =============================================================================
// Helpers
// =============================================================================

// fluxTierScopeName converts a "flux."-prefixed deferred path's remainder (for example
// "database.install.substitutions.cert" or "database.resources-primary.substitutions.cert") to
// the compiled Kustomization name compileFluxSystemTiers produces for that tier ("database-install"
// or "database-resources-primary"). Returns false when rest is not a recognized tier-substitution
// path.
func fluxTierScopeName(rest string) (string, bool) {
	withoutSubs, _, ok := strings.Cut(rest, ".substitutions.")
	if !ok {
		return "", false
	}
	system, tier, ok := strings.Cut(withoutSubs, ".")
	if !ok {
		return "", false
	}
	return system + "-" + tier, true
}
