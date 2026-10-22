// The match functions of the TestRunner compare a composed blueprint with the expected blueprint of a test case.
// They check only the fields that the test case sets, for terraform components, kustomizations, and flux systems.
// They also check the exclusions, which are entries that the composed blueprint must not contain.

package test

import (
	"fmt"
	"slices"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Private Methods
// =============================================================================

// matchBlueprint compares the actual composed blueprint against expected blueprint structure and returns
// a list of differences. It uses partial matching semantics: only fields explicitly specified in the expect
// blueprint are validated, allowing tests to focus on specific aspects without asserting the entire structure.
// The function validates Terraform components, Kustomizations, the crds: layer, and the config: scope,
// checking for presence and matching properties. For each expected component, it searches the actual
// blueprint, reports missing components, and compares specified properties (source, path, dependsOn, etc.);
// each expect.crds entry must appear in the blueprint's crds: list. Each expect.config entry is matched by
// top-level block name against bp.Config using subset semantics (see deepEqualInputsValue), so a nested
// path like config: cluster: controlplanes: cpu: 7 checks only the keys named. Returns an empty slice if
// all expectations are met, or a list of descriptive difference messages if mismatches are found.
func (r *TestRunner) matchBlueprint(bp *blueprintv1alpha1.Blueprint, expect *blueprintv1alpha1.Blueprint) []string {
	var diffs []string

	if expect == nil {
		return diffs
	}

	for _, expectTf := range expect.TerraformComponents {
		found := r.findTerraformComponent(bp, expectTf)
		if found == nil {
			identifier := expectTf.Name
			if identifier == "" {
				identifier = expectTf.Path
			}
			diffs = append(diffs, fmt.Sprintf("terraform component not found: %s", identifier))
			continue
		}

		componentDiffs := r.matchTerraformComponent(found, expectTf)
		diffs = append(diffs, componentDiffs...)
	}

	for _, expectK := range expect.Kustomizations {
		found := r.findKustomization(bp, expectK)
		if found == nil {
			diffs = append(diffs, fmt.Sprintf("kustomization not found: %s", expectK.Name))
			continue
		}

		kustomizeDiffs := r.matchKustomization(found, expectK, expectK.Name)
		diffs = append(diffs, kustomizeDiffs...)
	}

	for _, expectSys := range expect.FluxSystems {
		found := r.findFluxSystem(bp, expectSys)
		if found == nil {
			diffs = append(diffs, fmt.Sprintf("flux system not found: %s", expectSys.Name))
			continue
		}

		fluxDiffs := r.matchFluxSystem(found, expectSys)
		diffs = append(diffs, fluxDiffs...)
	}

	for _, ref := range expect.Crds {
		if !blueprintInstallsCrd(bp, ref) {
			diffs = append(diffs, fmt.Sprintf("crd reference not found: %s", ref))
		}
	}

	for name, expectedValue := range expect.Config {
		actualValue, exists := bp.Config[name]
		if !exists {
			diffs = append(diffs, fmt.Sprintf("config[%s]: key not found", name))
			continue
		}
		if !deepEqualInputsValue(expectedValue, actualValue) {
			diffs = append(diffs, fmt.Sprintf("config[%s]: expected %v, got %v", name, expectedValue, actualValue))
		}
	}

	return diffs
}

// matchExclusions verifies that components specified in the exclude blueprint are NOT present in the
// actual composed blueprint. This allows tests to assert that certain components should be excluded
// based on test conditions (e.g., a component should not exist when a feature flag is disabled).
// Uses partial matching: components are identified by name or path for Terraform components, and by
// name for Kustomizations. A Kustomization exclusion that also sets components: doesn't assert the
// whole entry is absent — the containing kustomization is expected to exist, so this asserts only
// that those specific components are absent from it (see matchComponentExclusion); a
// bare name: with no components: keeps whole-entry-absence semantics. Each exclude.config entry is
// matched by top-level block name against bp.Config using the same subset semantics matchBlueprint
// uses for expect.config; a match is a violation, a missing key or a non-matching value is not. If
// any excluded component or config value is found in the blueprint, a descriptive error message is
// added to the differences list. Returns an empty slice if all exclusions are satisfied.
func (r *TestRunner) matchExclusions(bp *blueprintv1alpha1.Blueprint, exclude *blueprintv1alpha1.Blueprint) []string {
	var diffs []string

	if exclude == nil {
		return diffs
	}

	for _, excludeTf := range exclude.TerraformComponents {
		found := r.findTerraformComponent(bp, excludeTf)
		if found != nil {
			identifier := excludeTf.Name
			if identifier == "" {
				identifier = excludeTf.Path
			}
			diffs = append(diffs, fmt.Sprintf("terraform component should not exist: %s", identifier))
		}
	}

	for _, excludeK := range exclude.Kustomizations {
		found := r.findKustomization(bp, excludeK)
		if found == nil {
			continue
		}
		if len(excludeK.Components) == 0 {
			diffs = append(diffs, fmt.Sprintf("kustomization should not exist: %s", excludeK.Name))
			continue
		}
		diffs = append(diffs, matchComponentExclusion(found.Components, excludeK.Components, fmt.Sprintf("kustomize[%s]", excludeK.Name))...)
	}

	for _, excludeSys := range exclude.FluxSystems {
		found := r.findFluxSystem(bp, excludeSys)
		if found == nil {
			continue
		}
		if excludeSys.Install == nil && len(excludeSys.Resources) == 0 {
			diffs = append(diffs, fmt.Sprintf("flux system should not exist: %s", excludeSys.Name))
			continue
		}
		diffs = append(diffs, r.matchFluxSystemExclusions(found, excludeSys)...)
	}

	for _, ref := range exclude.Crds {
		if blueprintInstallsCrd(bp, ref) {
			diffs = append(diffs, fmt.Sprintf("crd reference should not exist: %s", ref))
		}
	}

	for name, excludeValue := range exclude.Config {
		actualValue, exists := bp.Config[name]
		if !exists {
			continue
		}
		if deepEqualInputsValue(excludeValue, actualValue) {
			diffs = append(diffs, fmt.Sprintf("config[%s] should not match: %v", name, excludeValue))
		}
	}

	return diffs
}

// findTerraformComponent searches the blueprint's Terraform components for a component matching the
// expected component's name or path. It performs a linear search through all Terraform components,
// checking first for a name match (if expect.Name is non-empty), then for a path match (if expect.Path
// is non-empty). This allows tests to identify components by either identifier. Returns a pointer to
// the matching component if found, or nil if no match exists.
func (r *TestRunner) findTerraformComponent(bp *blueprintv1alpha1.Blueprint, expect blueprintv1alpha1.TerraformComponent) *blueprintv1alpha1.TerraformComponent {
	for i := range bp.TerraformComponents {
		tc := &bp.TerraformComponents[i]
		if expect.Name != "" && tc.Name == expect.Name {
			return tc
		}
		if expect.Path != "" && tc.Path == expect.Path {
			return tc
		}
	}
	return nil
}

// findKustomization searches the blueprint's Kustomizations for a kustomization matching the expected
// kustomization's name, across both plain kustomize: entries and the install/resources tiers compiled
// from flux: systems. It performs a linear search over AllKustomizations, comparing names exactly.
// Returns a pointer to a copy of the matching kustomization if found, or nil if no kustomization with
// the specified name exists in the blueprint.
func (r *TestRunner) findKustomization(bp *blueprintv1alpha1.Blueprint, expect blueprintv1alpha1.Kustomization) *blueprintv1alpha1.Kustomization {
	for _, k := range bp.AllKustomizations() {
		if k.Name == expect.Name {
			match := k
			return &match
		}
	}
	return nil
}

// findFluxSystem searches the blueprint's FluxSystems for a system matching the expected system's name.
// bp.FluxSystems holds each system after facet merging and when-expression evaluation but before tier
// compilation, so this is the author's own flux: shape rather than the compiled Kustomization output.
// Returns a pointer to the matching system if found, or nil if no system with that name exists.
func (r *TestRunner) findFluxSystem(bp *blueprintv1alpha1.Blueprint, expect blueprintv1alpha1.FluxSystem) *blueprintv1alpha1.FluxSystem {
	for i := range bp.FluxSystems {
		if bp.FluxSystems[i].Name == expect.Name {
			return &bp.FluxSystems[i]
		}
	}
	return nil
}

// findFluxVariant searches actual for a resources variant matching expect: by Name when the expectation
// sets one, otherwise by expect.Components being a subset of a variant's Components so an unnamed or
// facet-merged variant can still be identified. When expect gives neither a name nor components and
// actual holds exactly one variant, that variant is returned. Returns nil if no variant matches.
func (r *TestRunner) findFluxVariant(actual []blueprintv1alpha1.FluxVariant, expect blueprintv1alpha1.FluxVariant) *blueprintv1alpha1.FluxVariant {
	for i := range actual {
		v := &actual[i]
		if expect.Name != "" {
			if v.Name == expect.Name {
				return v
			}
			continue
		}
		if len(expect.Components) > 0 && containsAll(v.Components, expect.Components) {
			return v
		}
	}
	if expect.Name == "" && len(expect.Components) == 0 && len(actual) == 1 {
		return &actual[0]
	}
	return nil
}

// matchTerraformComponent compares an actual Terraform component against expected properties and returns
// a list of differences. It uses partial matching: only properties explicitly set in the expect component
// are validated. The function checks source, path, dependsOn, and inputs fields. For dependsOn, it verifies that
// all expected dependencies are present in the actual component's dependency list. For inputs, it performs
// strict value equality checking - each expected key must exist with the exact expected value. Returns an empty
// slice if all specified properties match, or a list of formatted difference messages describing mismatches.
func (r *TestRunner) matchTerraformComponent(actual *blueprintv1alpha1.TerraformComponent, expect blueprintv1alpha1.TerraformComponent) []string {
	var diffs []string
	identifier := expect.Name
	if identifier == "" {
		identifier = expect.Path
	}

	if expect.Source != "" && actual.Source != expect.Source {
		diffs = append(diffs, fmt.Sprintf("terraform[%s].source: expected %q, got %q", identifier, expect.Source, actual.Source))
	}

	if expect.Path != "" && actual.Path != expect.Path {
		diffs = append(diffs, fmt.Sprintf("terraform[%s].path: expected %q, got %q", identifier, expect.Path, actual.Path))
	}

	if len(expect.DependsOn) > 0 {
		for _, dep := range expect.DependsOn {
			if !contains(actual.DependsOn, dep) {
				diffs = append(diffs, fmt.Sprintf("terraform[%s].dependsOn: missing %q", identifier, dep))
			}
		}
	}

	if len(expect.Inputs) > 0 {
		for key, expectedValue := range expect.Inputs {
			actualValue, exists := actual.Inputs[key]
			if !exists {
				diffs = append(diffs, fmt.Sprintf("terraform[%s].inputs[%s]: key not found", identifier, key))
				continue
			}
			if !deepEqualInputsValue(expectedValue, actualValue) {
				diffs = append(diffs, fmt.Sprintf("terraform[%s].inputs[%s]: expected %v, got %v", identifier, key, expectedValue, actualValue))
			}
		}
	}

	return diffs
}

// matchKustomization compares an actual Kustomization against expected properties and returns a
// list of differences. It uses partial matching. Only properties explicitly set on expect are
// validated. Path, source, timeout, interval, retryInterval, deleteTimeout, and wait check exact
// value equality. DependsOn, components, healthChecks, and healthCheckExprs check that every
// expected item has a match in the actual list. Substitutions checks an exact value per key. label
// prefixes each diff so a caller nested under a flux system can identify the field's origin.
func (r *TestRunner) matchKustomization(actual *blueprintv1alpha1.Kustomization, expect blueprintv1alpha1.Kustomization, label string) []string {
	var diffs []string

	if expect.Path != "" && actual.Path != expect.Path {
		diffs = append(diffs, fmt.Sprintf("kustomize[%s].path: expected %q, got %q", label, expect.Path, actual.Path))
	}

	if expect.Source != "" && actual.Source != expect.Source {
		diffs = append(diffs, fmt.Sprintf("kustomize[%s].source: expected %q, got %q", label, expect.Source, actual.Source))
	}

	if len(expect.DependsOn) > 0 {
		for _, dep := range expect.DependsOn {
			if !contains(actual.DependsOn, dep) {
				diffs = append(diffs, fmt.Sprintf("kustomize[%s].dependsOn: missing %q", label, dep))
			}
		}
	}

	if len(expect.Components) > 0 {
		for _, comp := range expect.Components {
			if !contains(actual.Components, comp) {
				diffs = append(diffs, fmt.Sprintf("kustomize[%s].components: missing %q", label, comp))
			}
		}
	}

	if len(expect.Substitutions) > 0 {
		for key, expectedValue := range expect.Substitutions {
			actualValue, exists := actual.Substitutions[key]
			if !exists {
				diffs = append(diffs, fmt.Sprintf("kustomize[%s].substitutions[%s]: key not found", label, key))
				continue
			}
			if expectedValue != actualValue {
				diffs = append(diffs, fmt.Sprintf("kustomize[%s].substitutions[%s]: expected %q, got %q", label, key, expectedValue, actualValue))
			}
		}
	}

	diffs = matchDuration(diffs, label, "timeout", actual.Timeout, expect.Timeout)
	diffs = matchDuration(diffs, label, "interval", actual.Interval, expect.Interval)
	diffs = matchDuration(diffs, label, "retryInterval", actual.RetryInterval, expect.RetryInterval)
	diffs = matchDuration(diffs, label, "deleteTimeout", actual.DeleteTimeout, expect.DeleteTimeout)

	if expect.Wait != nil && (actual.Wait == nil || *actual.Wait != *expect.Wait) {
		got := "unset"
		if actual.Wait != nil {
			got = fmt.Sprintf("%t", *actual.Wait)
		}
		diffs = append(diffs, fmt.Sprintf("kustomize[%s].wait: expected %t, got %s", label, *expect.Wait, got))
	}

	if len(expect.HealthChecks) > 0 {
		for _, hc := range expect.HealthChecks {
			if !slices.ContainsFunc(actual.HealthChecks, func(a blueprintv1alpha1.HealthCheckRef) bool { return healthCheckMatches(a, hc) }) {
				diffs = append(diffs, fmt.Sprintf("kustomize[%s].healthChecks: missing %+v", label, hc))
			}
		}
	}

	if len(expect.HealthCheckExprs) > 0 {
		for _, hce := range expect.HealthCheckExprs {
			if !slices.ContainsFunc(actual.HealthCheckExprs, func(a blueprintv1alpha1.HealthCheckExpr) bool { return healthCheckExprMatches(a, hce) }) {
				diffs = append(diffs, fmt.Sprintf("kustomize[%s].healthCheckExprs: missing %+v", label, hce))
			}
		}
	}

	return diffs
}

// matchFluxSystem compares an actual FluxSystem against expected properties. The actual system comes
// from bp.FluxSystems, after facet merge and when-expression evaluation. The function returns a list
// of differences.
//
// The function uses partial matching. It checks only the properties set on the expect system.
//
// The function compares fields this way:
//   - Path: compares against actual.EffectivePath(), the same name fallback compileFluxSystemTiers
//     uses. A fixture that omits path: still matches.
//   - Source, When, Strategy, DependsOn: compares the raw values the author wrote. DependsOn is the
//     system's own cross-layer edges, not a value the composer computes.
//   - Ordinal, GlobalDependency: compares only when the expectation sets them. GlobalDependency
//     checks only the true direction, because false is indistinguishable from unset.
//   - Install, Resources: delegates to matchKustomization for each Kustomization field.
//   - Enabled, Destroy: not compared. Composition does not evaluate these fields, so they hold the
//     raw authored expression, not a rendering outcome.
//
// The function returns an empty slice when all specified properties match.
func (r *TestRunner) matchFluxSystem(actual *blueprintv1alpha1.FluxSystem, expect blueprintv1alpha1.FluxSystem) []string {
	var diffs []string
	name := expect.Name

	actualPath := actual.EffectivePath()
	if expect.Path != "" && actualPath != expect.Path {
		diffs = append(diffs, fmt.Sprintf("flux[%s].path: expected %q, got %q", name, expect.Path, actualPath))
	}

	if expect.Source != "" && actual.Source != expect.Source {
		diffs = append(diffs, fmt.Sprintf("flux[%s].source: expected %q, got %q", name, expect.Source, actual.Source))
	}

	if expect.When != "" && actual.When != expect.When {
		diffs = append(diffs, fmt.Sprintf("flux[%s].when: expected %q, got %q", name, expect.When, actual.When))
	}

	if expect.Strategy != "" && actual.Strategy != expect.Strategy {
		diffs = append(diffs, fmt.Sprintf("flux[%s].strategy: expected %q, got %q", name, expect.Strategy, actual.Strategy))
	}

	if len(expect.DependsOn) > 0 {
		for _, dep := range expect.DependsOn {
			if !contains(actual.DependsOn, dep) {
				diffs = append(diffs, fmt.Sprintf("flux[%s].dependsOn: missing %q", name, dep))
			}
		}
	}

	if expect.Ordinal != nil {
		switch {
		case actual.Ordinal == nil:
			diffs = append(diffs, fmt.Sprintf("flux[%s].ordinal: expected %d, got unset", name, *expect.Ordinal))
		case *actual.Ordinal != *expect.Ordinal:
			diffs = append(diffs, fmt.Sprintf("flux[%s].ordinal: expected %d, got %d", name, *expect.Ordinal, *actual.Ordinal))
		}
	}

	if expect.GlobalDependency && !actual.GlobalDependency {
		diffs = append(diffs, fmt.Sprintf("flux[%s].globalDependency: expected true, got false", name))
	}

	if expect.Install != nil {
		if actual.Install == nil {
			diffs = append(diffs, fmt.Sprintf("flux[%s].install: expected present, got absent", name))
		} else {
			diffs = append(diffs, r.matchKustomization(actual.Install, *expect.Install, name+".install")...)
		}
	}

	for _, expectVariant := range expect.Resources {
		identifier := fluxVariantIdentifier(expectVariant)
		actualVariant := r.findFluxVariant(actual.Resources, expectVariant)
		if actualVariant == nil {
			diffs = append(diffs, fmt.Sprintf("flux[%s].resources: variant not found: %s", name, identifier))
			continue
		}
		if expectVariant.When != "" && actualVariant.When != expectVariant.When {
			diffs = append(diffs, fmt.Sprintf("flux[%s].resources[%s].when: expected %q, got %q", name, identifier, expectVariant.When, actualVariant.When))
		}
		label := fmt.Sprintf("%s.resources[%s]", name, identifier)
		diffs = append(diffs, r.matchKustomization(&actualVariant.Kustomization, expectVariant.Kustomization, label)...)
	}

	return diffs
}

// matchFluxSystemExclusions verifies that specific parts of an actual FluxSystem (found by name) are
// absent: the install tier when exclude.Install is set, and each named/matched resources variant in
// exclude.Resources. Used when a fixture wants to assert a system is still partially present (e.g. its
// install tier remains while one resources variant is gated off) rather than absent entirely, which
// matchExclusions handles by name lookup alone before calling this. An exclude.Install or resources
// variant that also sets components: doesn't assert the tier/variant itself is absent — it's expected
// to exist — so this asserts only that those specific components are absent from it, the same
// name-plus-components treatment matchExclusions gives a Kustomization exclusion (see
// matchComponentExclusion). This narrowing applies whichever way findFluxVariant located the variant —
// by exact name, or by its components-subset match for an unnamed variant — since either way the found
// variant is expected to exist and only the listed components are asserted absent from it. A bare
// install: or resources variant with no components: keeps whole-tier/whole-variant-absence semantics.
// Returns an empty slice if every excluded part is absent.
func (r *TestRunner) matchFluxSystemExclusions(actual *blueprintv1alpha1.FluxSystem, exclude blueprintv1alpha1.FluxSystem) []string {
	var diffs []string
	name := exclude.Name

	if exclude.Install != nil && actual.Install != nil {
		if len(exclude.Install.Components) == 0 {
			diffs = append(diffs, fmt.Sprintf("flux[%s].install: should not exist", name))
		} else {
			diffs = append(diffs, matchComponentExclusion(actual.Install.Components, exclude.Install.Components, fmt.Sprintf("flux[%s].install", name))...)
		}
	}

	for _, excludeVariant := range exclude.Resources {
		found := r.findFluxVariant(actual.Resources, excludeVariant)
		if found == nil {
			continue
		}
		if len(excludeVariant.Components) > 0 {
			diffs = append(diffs, matchComponentExclusion(found.Components, excludeVariant.Components, fmt.Sprintf("flux[%s].resources[%s]", name, fluxVariantIdentifier(excludeVariant)))...)
			continue
		}
		diffs = append(diffs, fmt.Sprintf("flux[%s].resources: variant should not exist: %s", name, fluxVariantIdentifier(excludeVariant)))
	}

	return diffs
}

// =============================================================================
// Helpers
// =============================================================================

// fluxVariantIdentifier returns a human-readable label for a resources variant expectation, for use in
// diff messages: the variant's Name when set, otherwise its Components joined for display.
func fluxVariantIdentifier(v blueprintv1alpha1.FluxVariant) string {
	if v.Name != "" {
		return v.Name
	}
	return strings.Join(v.Components, ",")
}

// matchComponentExclusion asserts that none of excludeComponents are present in actual, returning
// one diff per component found in both. Used when an exclude entry (Kustomization, FluxSystem
// Install, or a Resources variant) sets components: alongside identifying the entry by name — the
// entry itself is expected to exist, so this checks the narrower "does it contain this component"
// claim instead of the entry's presence. label prefixes each diff message (e.g. "kustomize[name]",
// "flux[name].install", "flux[name].resources[variant]").
func matchComponentExclusion(actual []string, excludeComponents []string, label string) []string {
	var diffs []string
	for _, comp := range excludeComponents {
		if contains(actual, comp) {
			diffs = append(diffs, fmt.Sprintf("%s.components: should not contain %q", label, comp))
		}
	}
	return diffs
}

// contains checks whether a string slice contains a specific string value.
// It performs a linear search through the slice, returning true if the item is found
// and false otherwise. This is a simple utility function used throughout the test runner
// for checking membership in dependency lists, component lists, and other string collections.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// containsAll reports whether every item in needles is present in haystack, via contains.
func containsAll(haystack []string, needles []string) bool {
	for _, n := range needles {
		if !contains(haystack, n) {
			return false
		}
	}
	return true
}

// healthCheckMatches reports whether actual satisfies expect. It uses partial matching, the same
// as matchKustomization's other fields: a field left blank on expect is not compared.
func healthCheckMatches(actual, expect blueprintv1alpha1.HealthCheckRef) bool {
	if expect.APIVersion != "" && actual.APIVersion != expect.APIVersion {
		return false
	}
	if expect.Kind != "" && actual.Kind != expect.Kind {
		return false
	}
	if expect.Name != "" && actual.Name != expect.Name {
		return false
	}
	return expect.Namespace == "" || actual.Namespace == expect.Namespace
}

// healthCheckExprMatches reports whether actual satisfies expect. It uses partial matching, the
// same as matchKustomization's other fields: a field left blank on expect is not compared.
func healthCheckExprMatches(actual, expect blueprintv1alpha1.HealthCheckExpr) bool {
	if expect.APIVersion != "" && actual.APIVersion != expect.APIVersion {
		return false
	}
	if expect.Kind != "" && actual.Kind != expect.Kind {
		return false
	}
	if expect.Current != "" && actual.Current != expect.Current {
		return false
	}
	if expect.InProgress != "" && actual.InProgress != expect.InProgress {
		return false
	}
	return expect.Failed == "" || actual.Failed == expect.Failed
}

// matchDuration appends a diff when expect is set and actual does not equal it. It leaves diffs
// untouched when expect is nil, since that means the fixture left the field unset.
func matchDuration(diffs []string, label, field string, actual, expect *blueprintv1alpha1.DurationString) []string {
	if expect == nil {
		return diffs
	}
	got := "unset"
	if actual != nil {
		got = actual.Duration.String()
	}
	if actual == nil || actual.Duration != expect.Duration {
		diffs = append(diffs, fmt.Sprintf("kustomize[%s].%s: expected %s, got %s", label, field, expect.Duration, got))
	}
	return diffs
}

// blueprintInstallsCrd reports whether the composed blueprint installs ref — either from its own
// (default/project) crds list or from any source that vendors it. A test asserts a CRD's presence with
// the bare ref; the source it rides on is a composition detail the assertion need not name.
func blueprintInstallsCrd(bp *blueprintv1alpha1.Blueprint, ref string) bool {
	if contains(bp.Crds, ref) {
		return true
	}
	for _, source := range bp.Sources {
		if contains(source.Crds, ref) {
			return true
		}
	}
	return false
}

// deepEqualInputsValue compares expected and actual terraform input values with subset semantics
// so tests can assert minimal shape (e.g. instances with only name, role, disks). For slice-of-maps
// and map-of-maps, actual may contain extra keys; for slices, length and order must match and each
// expected element must be a subset of the corresponding actual element. Falls back to deepEqual
// for scalars and when subset semantics do not apply.
func deepEqualInputsValue(expected, actual any) bool {
	if expected == nil && actual == nil {
		return true
	}
	if expected == nil || actual == nil {
		return false
	}
	switch e := expected.(type) {
	case []any:
		a, ok := actual.([]any)
		if !ok || len(e) != len(a) {
			return false
		}
		for i := range e {
			if !deepEqualInputsValue(e[i], a[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		a, ok := actual.(map[string]any)
		if !ok {
			return false
		}
		for k, ev := range e {
			av, exists := a[k]
			if !exists || !deepEqualInputsValue(ev, av) {
				return false
			}
		}
		return true
	default:
		return deepEqual(expected, actual)
	}
}

// deepEqual performs deep equality comparison between two arbitrary values.
// It handles maps, slices, and scalar values recursively, returning true if the values
// are structurally and value-equal. For maps, both must have the same keys with equal values.
// For slices, both must have the same length with equal elements in the same order.
// For scalar values, it uses standard Go equality comparison with fmt.Sprintf fallback
// to handle type differences (e.g., int vs float64 from YAML parsing).
func deepEqual(expected, actual any) bool {
	if expected == nil && actual == nil {
		return true
	}
	if expected == nil || actual == nil {
		return false
	}

	switch e := expected.(type) {
	case map[string]any:
		a, ok := actual.(map[string]any)
		if !ok {
			return false
		}
		if len(e) != len(a) {
			return false
		}
		for k, ev := range e {
			av, exists := a[k]
			if !exists || !deepEqual(ev, av) {
				return false
			}
		}
		return true

	case []any:
		a, ok := actual.([]any)
		if !ok {
			return false
		}
		if len(e) != len(a) {
			return false
		}
		for i := range e {
			if !deepEqual(e[i], a[i]) {
				return false
			}
		}
		return true

	default:
		if expected == actual {
			return true
		}
		return fmt.Sprintf("%v", expected) == fmt.Sprintf("%v", actual)
	}
}
