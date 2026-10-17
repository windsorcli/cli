// The processor_eval file holds the expression evaluation steps of the BlueprintProcessor.
// It provides the evaluation of when conditions, substitutions, and typed expressions.
// It checks the requires blocks of each facet and of its config blocks.
// It builds the RequirementsError for the required values that are not set.

package blueprint

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Types
// =============================================================================

// requirementBlockMiss records the unsatisfied paths and the effective condition under which a
// single requirement block applied during a ProcessFacets pass. Condition is the AND of the parent
// facet's When and the block's When; an empty Condition means the paths were unconditionally
// required. Message is the optional author-supplied context for this block.
type requirementBlockMiss struct {
	Condition string
	Message   string
	Paths     []string
}

// facetRequirementMisses aggregates a single facet's unsatisfied requirement blocks for the
// final-round error report. The facet name is carried by the outer map key in pendingRequirements.
type facetRequirementMisses struct {
	Misses []requirementBlockMiss
}

// =============================================================================
// Private Methods
// =============================================================================

// shouldIncludeFacet evaluates whether a facet should be included based on its 'when' condition.
// Returns true if the facet has no condition or if the condition evaluates to true. Returns
// false if the condition evaluates to false. Returns an error if condition evaluation fails.
func (p *BaseBlueprintProcessor) shouldIncludeFacet(facet blueprintv1alpha1.Facet, scope map[string]any) (bool, error) {
	if facet.When == "" {
		return true, nil
	}
	matches, err := p.evaluateCondition(facet.When, facet.Path, scope)
	if err != nil {
		return false, werror.Wrap(err, "error evaluating facet '%s' condition", facet.Metadata.Name)
	}
	return matches, nil
}

// shouldIncludeComponent evaluates whether a component or kustomization should be included based
// on its 'when' condition. Returns true if there is no condition or if the condition evaluates
// to true. Returns false if the condition evaluates to false. Returns an error if condition
// evaluation fails.
func (p *BaseBlueprintProcessor) shouldIncludeComponent(when string, facetPath string, scope map[string]any) (bool, error) {
	if when == "" {
		return true, nil
	}
	matches, err := p.evaluateCondition(when, facetPath, scope)
	if err != nil {
		return false, err
	}
	return matches, nil
}

// evaluateCondition uses the expression evaluator to evaluate a 'when' condition string against
// the provided configuration data. The path parameter provides context for error messages and
// helper function resolution. Returns true if the expression evaluates to boolean true or the
// string "true". Returns false for nil (undefined variables) or boolean false. Returns an error
// if the expression is invalid or evaluates to an unexpected type. When scope is non-nil (e.g.
// context merged with facet scope), expressions can reference values like workstation.runtime.
func (p *BaseBlueprintProcessor) evaluateCondition(expr string, path string, scope map[string]any) (bool, error) {
	if !evaluator.ContainsExpression(expr) {
		expr = "${" + expr + "}"
	}
	evaluated, err := p.evaluator.Evaluate(expr, path, scope, false)
	if err != nil {
		return false, err
	}
	if evaluator.IsDeferredValue(evaluated) {
		if deferredExpr, ok := evaluator.DeferredExpression(evaluated); ok {
			return false, fmt.Errorf("condition references a value that is not yet resolved (still evaluating config, or awaiting a terraform apply): %s", deferredExpr)
		}
		return false, fmt.Errorf("condition references a value that is not yet resolved (still evaluating config, or awaiting a terraform apply)")
	}
	var result bool
	switch v := evaluated.(type) {
	case nil:
		result = false
	case bool:
		result = v
	case string:
		result = v == "true"
	default:
		return false, fmt.Errorf("condition must evaluate to boolean, got %T", evaluated)
	}
	return result, nil
}

// evaluateSubstitutions evaluates a map of string substitutions using the BaseBlueprintProcessor's expression evaluator.
// When scope is non-nil, it is merged into the evaluation environment (e.g. facet config so expressions
// can reference talos.controlplanes). If the result is nil (undefined path without ?? fallback), the key
// is included with an empty string value. If the result contains unresolved deferred expressions, the
// original expression is preserved for later evaluation. Returns the evaluated substitutions map or an
// error if evaluation fails for any substitution.
func (p *BaseBlueprintProcessor) evaluateSubstitutions(subs map[string]string, facetPath string, scope map[string]any) (map[string]string, map[string]bool, error) {
	result := make(map[string]string)
	deferredKeys := make(map[string]bool)
	for key, value := range subs {
		if paths := p.sensitivePathsInValue(value); len(paths) > 0 {
			return nil, nil, werror.New(fmt.Sprintf("Substitution %q references sensitive property %q.", key, paths[0]),
				"Move the value to the secrets: key of the flux system. Windsor writes a plaintext substitution to a ConfigMap.", nil)
		}
		evaluated, err := p.evaluator.Evaluate(value, facetPath, scope, false)
		if err != nil {
			return nil, nil, werror.Wrap(err, "failed to evaluate '%s'", key)
		}
		if evaluator.IsDeferredValue(evaluated) {
			if deferredExpr, ok := evaluator.DeferredExpression(evaluated); ok {
				result[key] = deferredExpr
				deferredKeys[key] = true
				continue
			}
			result[key] = value
			deferredKeys[key] = true
			continue
		}
		if evaluator.ContainsExpression(evaluated) {
			if str, ok := evaluated.(string); ok {
				value = str
			}
			result[key] = value
			deferredKeys[key] = true
			continue
		}
		if evaluated == nil {
			result[key] = ""
		} else if str, ok := evaluated.(string); ok {
			result[key] = str
		} else {
			result[key] = fmt.Sprintf("%v", evaluated)
		}
	}
	return result, deferredKeys, nil
}

// evaluateStringSlice evaluates each string of slice as an expression, with scope merged into the
// environment when it is not nil. A deferred expression is an error. An array result is flattened
// into the slice. Empty strings stay, so placeholder entries keep their position. Nil results are
// dropped.
func (p *BaseBlueprintProcessor) evaluateStringSlice(slice []string, facetPath string, scope map[string]any) ([]string, error) {
	if len(slice) == 0 {
		return nil, nil
	}

	result := make([]string, 0, len(slice))
	for _, s := range slice {
		evaluated, err := p.evaluator.Evaluate(s, facetPath, scope, true)
		if err != nil {
			return nil, err
		}
		if evaluated == nil {
			continue
		}
		switch v := evaluated.(type) {
		case string:
			result = append(result, v)
		case []any:
			for _, item := range v {
				if item == nil {
					continue
				}
				var str string
				switch itemVal := item.(type) {
				case string:
					str = itemVal
				default:
					str = fmt.Sprintf("%v", itemVal)
				}
				result = append(result, str)
			}
		default:
			result = append(result, fmt.Sprintf("%v", v))
		}
	}

	return result, nil
}

// evaluateBooleanExpression evaluates a boolean expression string. When scope is non-nil, it is
// merged into the evaluation environment (e.g. facet config). Uses evaluateDeferred=true to
// disallow deferred expressions (they will error). Returns the evaluated boolean value, or an error
// if evaluation fails or the result is not a boolean.
func (p *BaseBlueprintProcessor) evaluateBooleanExpression(expr string, facetPath string, scope map[string]any) (*bool, error) {
	if expr == "" {
		return nil, nil
	}
	evaluated, err := p.evaluator.Evaluate(expr, facetPath, scope, true)
	if err != nil {
		return nil, err
	}
	var result bool
	switch v := evaluated.(type) {
	case bool:
		result = v
	case string:
		switch v {
		case "true":
			result = true
		case "false":
			result = false
		default:
			return nil, fmt.Errorf("expected boolean, got string %q", v)
		}
	default:
		return nil, fmt.Errorf("expected boolean, got %T", evaluated)
	}
	return &result, nil
}

// evaluateIntegerExpression evaluates an integer expression string. When scope is non-nil, it is
// merged into the evaluation environment (e.g. facet config). Uses evaluateDeferred=true to
// disallow deferred expressions (they will error). Returns the evaluated integer value, or an error
// if evaluation fails or the result is not an integer.
func (p *BaseBlueprintProcessor) evaluateIntegerExpression(expr string, facetPath string, scope map[string]any) (*int, error) {
	if expr == "" {
		return nil, nil
	}
	evaluated, err := p.evaluator.Evaluate(expr, facetPath, scope, true)
	if err != nil {
		return nil, err
	}
	var result int
	switch v := evaluated.(type) {
	case int:
		result = v
	case int64:
		result = int(v)
	case float64:
		result = int(v)
	case string:
		parsed, err := fmt.Sscanf(v, "%d", &result)
		if err != nil || parsed != 1 {
			return nil, fmt.Errorf("expected integer, got string %q", v)
		}
	default:
		return nil, fmt.Errorf("expected integer, got %T", evaluated)
	}
	return &result, nil
}

// evaluateRequirements returns the unmet requirements of a facet: its own requires blocks, and those
// of each config block, component, and kustomization whose when is true. A block whose own when is
// false is skipped. Each path must be present and not empty (false and 0 are present). The Condition
// of a miss joins the facet, entry, and block conditions, for grouping. contextScope hides a config
// block's own prior-round value from its when. Only a when that fails to evaluate is an error.
func (p *BaseBlueprintProcessor) evaluateRequirements(facet blueprintv1alpha1.Facet, scope, contextScope map[string]any) ([]requirementBlockMiss, error) {
	var misses []requirementBlockMiss

	facetMisses, err := p.evaluateRequirementBlocks(facet.Requires, scope, facet.Path, facet.Metadata.Name, facet.When)
	if err != nil {
		return nil, err
	}
	misses = append(misses, facetMisses...)

	for _, cb := range facet.Config {
		if len(cb.Requires) == 0 {
			continue
		}
		guardScope := scopeBlindToBlockKeys(scope, contextScope, cb.Name, cb.Body)
		include, err := p.shouldIncludeComponent(cb.When, facet.Path, guardScope)
		if err != nil {
			return nil, werror.Wrap(err, "error evaluating config block when '%s' in facet '%s'", cb.When, facet.Metadata.Name)
		}
		if !include {
			continue
		}
		m, err := p.evaluateRequirementBlocks(cb.Requires, scope, facet.Path, facet.Metadata.Name, facet.When, cb.When)
		if err != nil {
			return nil, err
		}
		misses = append(misses, m...)
	}

	for _, tc := range facet.TerraformComponents {
		if len(tc.Requires) == 0 {
			continue
		}
		include, err := p.shouldIncludeComponent(tc.When, facet.Path, scope)
		if err != nil {
			return nil, werror.Wrap(err, "error evaluating terraform component when '%s' in facet '%s'", tc.When, facet.Metadata.Name)
		}
		if !include {
			continue
		}
		m, err := p.evaluateRequirementBlocks(tc.Requires, scope, facet.Path, facet.Metadata.Name, facet.When, tc.When)
		if err != nil {
			return nil, err
		}
		misses = append(misses, m...)
	}

	for _, k := range facet.Kustomizations {
		if len(k.Requires) == 0 {
			continue
		}
		include, err := p.shouldIncludeComponent(k.When, facet.Path, scope)
		if err != nil {
			return nil, werror.Wrap(err, "error evaluating kustomization when '%s' in facet '%s'", k.When, facet.Metadata.Name)
		}
		if !include {
			continue
		}
		m, err := p.evaluateRequirementBlocks(k.Requires, scope, facet.Path, facet.Metadata.Name, facet.When, k.When)
		if err != nil {
			return nil, err
		}
		misses = append(misses, m...)
	}

	return misses, nil
}

// evaluateRequirementBlocks walks blocks against scope and returns the unsatisfied entries.
// parentWhens are the chain of parent-level when expressions (facet.When, optionally
// component.When) that are composed with each block's own When into the miss's effective condition.
// A block is skipped when its own When evaluates to false. Empty parts in parentWhens are dropped
// by combineConditions, so callers may pass an unset When (e.g. an empty facet.When) without
// special-casing.
func (p *BaseBlueprintProcessor) evaluateRequirementBlocks(blocks []blueprintv1alpha1.RequirementBlock, scope map[string]any, facetPath, facetName string, parentWhens ...string) ([]requirementBlockMiss, error) {
	if len(blocks) == 0 {
		return nil, nil
	}
	var misses []requirementBlockMiss
	for _, block := range blocks {
		if len(block.Paths) == 0 {
			continue
		}
		applies := true
		if block.When != "" {
			ok, err := p.evaluateCondition(block.When, facetPath, scope)
			if err != nil {
				return nil, werror.Wrap(err, "error evaluating requirement when '%s' in facet '%s'", block.When, facetName)
			}
			applies = ok
		}
		if !applies {
			continue
		}
		var missing []string
		for _, path := range block.Paths {
			if !isPresentAtPath(scope, path) {
				missing = append(missing, path)
			}
		}
		if len(missing) == 0 {
			continue
		}
		misses = append(misses, requirementBlockMiss{
			Condition: combineConditions(append(parentWhens, block.When)...),
			Message:   block.Message,
			Paths:     missing,
		})
	}
	return misses, nil
}

// =============================================================================
// Helpers
// =============================================================================

// lookupPath resolves a dotted-path key (e.g. "cluster.workers.count") in a nested scope. Returns
// (value, true) when every segment is found; returns (nil, false) when any segment is missing or
// when an intermediate segment is not a map.
func lookupPath(scope map[string]any, path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	segments := strings.Split(path, ".")
	var current any = scope
	for _, seg := range segments {
		m, ok := asMapStringAny(current)
		if !ok {
			return nil, false
		}
		v, exists := m[seg]
		if !exists {
			return nil, false
		}
		current = v
	}
	return current, true
}

// isPresentAtPath reports whether the dotted path resolves to a non-nil, non-empty value in scope.
// Empty string, empty slice, and empty map are treated as missing; false and 0 are present.
func isPresentAtPath(scope map[string]any, path string) bool {
	val, ok := lookupPath(scope, path)
	if !ok || val == nil {
		return false
	}
	if s, ok := val.(string); ok {
		return s != ""
	}
	rv := reflect.ValueOf(val)
	switch rv.Kind() {
	case reflect.Map, reflect.Slice, reflect.Array:
		return rv.Len() > 0
	}
	return true
}

// combineConditions joins one or more when expressions into a single condition string used as a
// grouping key in the aggregated requirements error. Empty parts are trimmed and dropped; all
// empty (or no parts) yields "", meaning the requirement was unconditional.
func combineConditions(parts ...string) string {
	var nonEmpty []string
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			nonEmpty = append(nonEmpty, trimmed)
		}
	}
	return strings.Join(nonEmpty, " && ")
}

// formatRequirementsError renders the user-facing aggregated error for unsatisfied facet
// requirements. Paths are listed alphabetically, deduplicated across facets, with any
// author-supplied messages attached underneath. Conditions are deliberately not surfaced —
// the message field is the right place for an author to explain why a value is needed; raw
// when-expressions leak engine detail and confuse operators. The word "facet" is intentionally
// absent from the rendered output.
func formatRequirementsError(pending map[string]facetRequirementMisses) error {
	pathMessages := make(map[string][]string)
	var pathOrder []string
	addMessage := func(path, msg string) {
		if _, exists := pathMessages[path]; !exists {
			pathOrder = append(pathOrder, path)
			pathMessages[path] = nil
		}
		if msg == "" {
			return
		}
		if slices.Contains(pathMessages[path], msg) {
			return
		}
		pathMessages[path] = append(pathMessages[path], msg)
	}

	names := make([]string, 0, len(pending))
	for name := range pending {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, miss := range pending[name].Misses {
			for _, p := range miss.Paths {
				addMessage(p, miss.Message)
			}
		}
	}

	sort.Strings(pathOrder)

	var b strings.Builder
	b.WriteString("the following required values are not set in values.yaml:\n\n")
	for _, path := range pathOrder {
		fmt.Fprintf(&b, "  - %s\n", path)
	}

	hasNotes := false
	for _, msgs := range pathMessages {
		if len(msgs) > 0 {
			hasNotes = true
			break
		}
	}
	if hasNotes {
		b.WriteString("\nNotes:\n\n")
		for _, path := range pathOrder {
			msgs := pathMessages[path]
			if len(msgs) == 0 {
				continue
			}
			fmt.Fprintf(&b, "  %s:\n", path)
			for _, msg := range msgs {
				for _, line := range strings.Split(strings.TrimRight(msg, "\n"), "\n") {
					b.WriteString("    ")
					b.WriteString(line)
					b.WriteByte('\n')
				}
			}
			b.WriteByte('\n')
		}
	}
	return &RequirementsError{Message: strings.TrimRight(b.String(), "\n")}
}
