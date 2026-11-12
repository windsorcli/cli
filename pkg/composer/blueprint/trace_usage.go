// The trace usage file answers whether the composed blueprint uses a value of a facet.
// It provides the set of scope references that the composed values reach through config blocks.
// It maps a line of a facet to the config key or the direct contribution on that line.
// It reports a value as unused only when the trace proves it, so a value it cannot resolve stays.

package blueprint

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Types
// =============================================================================

// facetValueUsage is the result of one walk of the composed values. refs holds each scope reference
// that a composed value reaches. opaque is the reason that the trace cannot resolve a reference, and
// it is empty when the walk read each value.
type facetValueUsage struct {
	refs   map[string]bool
	opaque string
}

// facetTarget is the value on one line of a facet. A config key sets configBlock. A direct
// contribution sets composedPath. rest is the path below the block or the contributed key.
type facetTarget struct {
	configBlock  string
	composedPath string
	rest         []string
	value        any
}

// =============================================================================
// Public Methods
// =============================================================================

// UnusedFacetValue reports whether the composed blueprint does not use the value on line of the facet
// at facetPath. It returns false for a file that composition did not load as a facet. A value is
// unused when no active config block or contribution holds it, or when no composed value reaches its
// config key. When the trace cannot resolve the value, it returns false and a note.
func (c *DefaultTraceCollector) UnusedFacetValue(facetPath string, line int) (bool, string) {
	if !c.isRecordedFacet(facetPath) {
		return false, ""
	}
	usage := c.valueUsage()
	if usage.opaque != "" {
		return false, usage.opaque
	}
	target, err := readFacetTarget(facetPath, line)
	if err != nil {
		return false, fmt.Sprintf("The trace cannot resolve %s:%d, so its artifact tag stays: %v.", facetPath, line, err)
	}
	return !c.targetUsed(facetPath, target, usage), ""
}

// =============================================================================
// Private Methods
// =============================================================================

// isRecordedFacet reports whether composition loaded the file at facetPath as a facet.
func (c *DefaultTraceCollector) isRecordedFacet(facetPath string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.facets[facetPath]
}

// valueUsage walks each value of the composed blueprint one time and returns the result.
func (c *DefaultTraceCollector) valueUsage() *facetValueUsage {
	c.usageOnce.Do(func() {
		c.usage = c.walkComposedValues()
	})
	return c.usage
}

// walkComposedValues collects the scope references of each contribution to an enabled entry of the
// composed blueprint and of each value in a blueprint file. It follows each reference through the
// config blocks of every active block with that name, so a block that sets only some keys of a value
// does not hide the keys of another block.
func (c *DefaultTraceCollector) walkComposedValues() *facetValueUsage {
	usage := &facetValueUsage{refs: make(map[string]bool)}
	if c.blueprint == nil {
		usage.opaque = "The trace has no composed blueprint, so the artifact tags of all facets stay."
		return usage
	}
	prefixes := composedValuePrefixes(c.blueprint)
	for composedPath, records := range c.contributions {
		if !hasAnyPrefix(composedPath, prefixes) {
			continue
		}
		for _, record := range records {
			c.addValueRefs(usage, composedPath, record.RawValue)
			c.addValueRefs(usage, composedPath, record.RawComponents)
		}
	}
	for _, value := range c.blueprintValues {
		c.addValueRefs(usage, "a blueprint file", value)
	}
	return usage
}

// addValueRefs adds the scope references of each expression in value, and of the config keys that
// those references name. An expression that the walk cannot resolve sets usage.opaque, which names
// origin and leaves out the value, because a value can hold a secret.
func (c *DefaultTraceCollector) addValueRefs(usage *facetValueUsage, origin string, value any) {
	switch v := value.(type) {
	case nil:
	case string:
		refs, ok := exprScopeRefs(v)
		if !ok && usage.opaque == "" {
			usage.opaque = fmt.Sprintf("An expression in %s does not parse or calls jsonnet(), so the trace cannot resolve it and the artifact tags of all facets stay.", origin)
		}
		for _, ref := range refs {
			c.addRef(usage, ref)
		}
	case []string:
		for _, item := range v {
			c.addValueRefs(usage, origin, item)
		}
	case []any:
		for _, item := range v {
			c.addValueRefs(usage, origin, item)
		}
	case map[string]string:
		for _, item := range v {
			c.addValueRefs(usage, origin, item)
		}
	default:
		if m, ok := asMapStringAny(value); ok {
			for _, item := range m {
				c.addValueRefs(usage, origin, item)
			}
		}
	}
}

// addRef adds ref and follows it into each active config block with the name of its first segment.
func (c *DefaultTraceCollector) addRef(usage *facetValueUsage, ref string) {
	if usage.refs[ref] {
		return
	}
	usage.refs[ref] = true
	segments := strings.Split(ref, ".")
	for _, record := range c.configBlocks["config."+segments[0]] {
		c.addValueRefs(usage, "config."+segments[0], descendMaps(record.RawValue, segments[1:]))
	}
}

// targetUsed reports whether the trace holds target and, for a config key, whether a composed value
// reaches that key.
func (c *DefaultTraceCollector) targetUsed(facetPath string, target facetTarget, usage *facetValueUsage) bool {
	if target.configBlock != "" {
		for _, record := range c.configBlocks["config."+target.configBlock] {
			if record.FacetPath == facetPath && sameValue(descendMaps(record.RawValue, target.rest), target.value) {
				return usage.reaches(target.configBlock, target.rest) && !c.overriddenByBlock(target)
			}
		}
		return false
	}
	if !hasAnyPrefix(target.composedPath, composedValuePrefixes(c.blueprint)) {
		return false
	}
	for _, record := range c.contributions[target.composedPath] {
		if record.FacetPath == facetPath && sameValue(descendMaps(record.RawValue, target.rest), target.value) {
			return true
		}
	}
	return false
}

// overriddenByBlock reports whether another active block with the same name sets the final value of
// a literal config key. A value that the operator sets has no block, so the tag of that key stays.
func (c *DefaultTraceCollector) overriddenByBlock(target facetTarget) bool {
	literal, ok := target.value.(string)
	if !ok || strings.Contains(literal, "${") {
		return false
	}
	final, inScope := resolveScopePath(c.scope, strings.Join(append([]string{target.configBlock}, target.rest...), "."))
	if !inScope || sameValue(final, target.value) {
		return false
	}
	for _, record := range c.configBlocks["config."+target.configBlock] {
		if sameValue(descendMaps(record.RawValue, target.rest), final) {
			return true
		}
	}
	return false
}

// reaches reports whether a reference names the key, a parent of the key, or a child of the key.
func (u *facetValueUsage) reaches(block string, rest []string) bool {
	path := strings.Join(append([]string{block}, rest...), ".")
	for candidate := path; ; {
		if u.refs[candidate] {
			return true
		}
		i := strings.LastIndex(candidate, ".")
		if i < 0 {
			break
		}
		candidate = candidate[:i]
	}
	for ref := range u.refs {
		if strings.HasPrefix(ref, path+".") {
			return true
		}
	}
	return false
}

// =============================================================================
// Helpers
// =============================================================================

// composedValuePrefixes returns the contribution path prefixes of each enabled entry of bp.
func composedValuePrefixes(bp *blueprintv1alpha1.Blueprint) []string {
	prefixes := []string{"substitutions."}
	for _, t := range bp.TerraformComponents {
		if t.Enabled.IsEnabled() {
			prefixes = append(prefixes, "terraform."+t.GetID()+".inputs.")
		}
	}
	for _, k := range bp.Kustomizations {
		if k.Enabled.IsEnabled() {
			prefixes = append(prefixes, "kustomize."+k.Name+".")
		}
	}
	for _, sys := range bp.FluxSystems {
		if sys.Enabled.IsEnabled() {
			prefixes = append(prefixes, "kustomize."+sys.InstallTierName()+".", "kustomize."+sys.ResourcesTierName(""))
		}
	}
	return prefixes
}

// hasAnyPrefix reports whether s starts with one of prefixes.
func hasAnyPrefix(s string, prefixes []string) bool {
	return slices.ContainsFunc(prefixes, func(prefix string) bool { return strings.HasPrefix(s, prefix) })
}

// descendMaps follows segments through nested maps. It stops at a value that is not a map, because
// an expression or a list holds all of the keys below it. It returns nil for a key that is missing.
func descendMaps(value any, segments []string) any {
	current := value
	for _, segment := range segments {
		m, ok := asMapStringAny(current)
		if !ok {
			return current
		}
		current = m[segment]
	}
	return current
}

// sameValue reports whether a recorded value and a value from a facet file have the same text.
func sameValue(recorded, fromFile any) bool {
	return recorded != nil && fmt.Sprint(recorded) == fmt.Sprint(fromFile)
}

// readFacetTarget returns the value on line of the facet at facetPath. It returns an error for a line
// that is not a key or a list scalar, and for a path that is not a config key or a contribution.
func readFacetTarget(facetPath string, line int) (facetTarget, error) {
	content, err := os.ReadFile(facetPath) // #nosec G304 -- path of a facet that composition loaded
	if err != nil {
		return facetTarget{}, err
	}
	file, err := parser.ParseBytes(content, 0)
	if err != nil {
		return facetTarget{}, errors.New("the file is not valid YAML")
	}
	if len(file.Docs) == 0 {
		return facetTarget{}, fmt.Errorf("the file has no YAML document")
	}
	path, ok := locateLine(file.Docs[0].Body, line, nil)
	if !ok {
		return facetTarget{}, fmt.Errorf("the line holds no YAML key")
	}
	var doc map[string]any
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return facetTarget{}, errors.New("the file is not valid YAML")
	}
	target, ok := targetOfPath(doc, path)
	if !ok {
		return facetTarget{}, fmt.Errorf("the line is not a config key or a contributed value")
	}
	target.value = valueAt(doc, path)
	return target, nil
}

// locateLine returns the path of the mapping key or the list scalar on line. A path holds a string
// for a mapping key and an int for a list index.
func locateLine(node ast.Node, line int, path []any) ([]any, bool) {
	switch n := node.(type) {
	case *ast.MappingNode:
		for _, value := range n.Values {
			if found, ok := locateLine(value, line, path); ok {
				return found, true
			}
		}
	case *ast.MappingValueNode:
		child := append(slices.Clip(path), any(n.Key.GetToken().Value))
		if n.Key.GetToken().Position.Line == line {
			return child, true
		}
		return locateLine(n.Value, line, child)
	case *ast.SequenceNode:
		for i, item := range n.Values {
			child := append(slices.Clip(path), any(i))
			if _, isScalar := item.(ast.ScalarNode); isScalar && item.GetToken().Position.Line == line {
				return child, true
			}
			if found, ok := locateLine(item, line, child); ok {
				return found, true
			}
		}
	}
	return nil, false
}

// targetOfPath maps a path in a facet document to its config key or its contribution. It returns
// false for any other path, and for a path through a list below the key.
func targetOfPath(doc map[string]any, path []any) (facetTarget, bool) {
	section, _ := path[0].(string)
	switch {
	case section == "config" && len(path) >= 4 && path[2] == "value":
		rest, ok := stringSegments(path[3:])
		return facetTarget{configBlock: itemName(doc, path[:2]), rest: rest}, ok
	case section == "substitutions" && len(path) == 2:
		return facetTarget{composedPath: fmt.Sprintf("substitutions.%v", path[1])}, true
	case section == "terraform" && len(path) >= 4 && path[2] == "inputs":
		id := itemName(doc, path[:2])
		if id == "" {
			id, _ = valueAt(doc, append(slices.Clip(path[:2]), any("path"))).(string)
		}
		return contributedTarget(fmt.Sprintf("terraform.%s.inputs.", id), path[3:])
	case section == "kustomize" && len(path) >= 4 && isSubstitutionKey(path[2]):
		return contributedTarget(kustomizeSubstitutionPrefix(itemName(doc, path[:2])), path[3:])
	case section == "flux" && len(path) >= 5 && path[2] == "install" && isSubstitutionKey(path[3]):
		system := blueprintv1alpha1.FluxSystem{Name: itemName(doc, path[:2])}
		return contributedTarget(kustomizeSubstitutionPrefix(system.InstallTierName()), path[4:])
	case section == "flux" && len(path) >= 6 && path[2] == "resources" && isSubstitutionKey(path[4]):
		system := blueprintv1alpha1.FluxSystem{Name: itemName(doc, path[:2])}
		return contributedTarget(kustomizeSubstitutionPrefix(system.ResourcesTierName(itemName(doc, path[:4]))), path[5:])
	}
	return facetTarget{}, false
}

// contributedTarget returns the target of the contributed key at the start of keyPath.
func contributedTarget(prefix string, keyPath []any) (facetTarget, bool) {
	segments, ok := stringSegments(keyPath)
	if !ok {
		return facetTarget{}, false
	}
	return facetTarget{composedPath: prefix + segments[0], rest: segments[1:]}, true
}

// isSubstitutionKey reports whether segment is the substitutions field or its substitute alias.
func isSubstitutionKey(segment any) bool {
	return segment == "substitutions" || segment == "substitute"
}

// stringSegments converts a path to strings. It returns false for a path with a list index.
func stringSegments(path []any) ([]string, bool) {
	segments := make([]string, 0, len(path))
	for _, segment := range path {
		s, ok := segment.(string)
		if !ok {
			return nil, false
		}
		segments = append(segments, s)
	}
	return segments, true
}

// itemName returns the name field of the list item at path.
func itemName(doc map[string]any, path []any) string {
	name, _ := valueAt(doc, append(slices.Clip(path), any("name"))).(string)
	return name
}

// valueAt returns the value at path in a decoded facet document, or nil when the path is missing.
func valueAt(doc map[string]any, path []any) any {
	var current any = doc
	for _, segment := range path {
		switch s := segment.(type) {
		case string:
			m, ok := asMapStringAny(current)
			if !ok {
				return nil
			}
			current = m[s]
		case int:
			list, ok := current.([]any)
			if !ok || s >= len(list) {
				return nil
			}
			current = list[s]
		}
	}
	return current
}
