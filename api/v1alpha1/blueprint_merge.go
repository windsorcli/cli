// The merge functions of Blueprint apply overlay blueprints to a base blueprint.
// They also replace and remove terraform components, kustomizations, and flux systems, and merge maps of values.

package v1alpha1

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
)

// =============================================================================
// Public Methods
// =============================================================================

// StrategicMerge performs a strategic merge of the provided overlay Blueprints into the receiver Blueprint.
// This method appends to array fields, deep merges map fields, and updates scalar fields if present in the overlays.
// It is designed for feature composition, enabling the combination of multiple features into a single blueprint.
func (b *Blueprint) StrategicMerge(overlays ...*Blueprint) error {
	for _, overlay := range overlays {
		if overlay == nil {
			continue
		}

		if overlay.Kind != "" {
			b.Kind = overlay.Kind
		}
		if overlay.ApiVersion != "" {
			b.ApiVersion = overlay.ApiVersion
		}

		if overlay.Metadata.Name != "" {
			b.Metadata.Name = overlay.Metadata.Name
		}
		if overlay.Metadata.Description != "" {
			b.Metadata.Description = overlay.Metadata.Description
		}

		if overlay.Backend != "" {
			b.Backend = overlay.Backend
		}

		if overlay.Repository.Url != "" {
			b.Repository = overlay.Repository
		}

		sourceMap := make(map[string]Source)
		sourceOrder := make([]string, 0)
		for _, source := range b.Sources {
			if source.Name != "" {
				sourceMap[source.Name] = source
				sourceOrder = append(sourceOrder, source.Name)
			}
		}
		for _, overlaySource := range overlay.Sources {
			if overlaySource.Name == "" {
				continue
			}
			existing, exists := sourceMap[overlaySource.Name]
			if !exists {
				sourceOrder = append(sourceOrder, overlaySource.Name)
				sourceMap[overlaySource.Name] = overlaySource
				continue
			}
			merged := overlaySource
			for _, ref := range existing.Crds {
				if !slices.Contains(merged.Crds, ref) {
					merged.Crds = append(merged.Crds, ref)
				}
			}
			sourceMap[overlaySource.Name] = merged
		}
		b.Sources = make([]Source, 0, len(sourceMap))
		for _, name := range sourceOrder {
			if source, exists := sourceMap[name]; exists {
				b.Sources = append(b.Sources, source)
			}
		}

		for _, overlayComponent := range overlay.TerraformComponents {
			if err := b.strategicMergeTerraformComponent(overlayComponent); err != nil {
				return err
			}
		}

		for _, ref := range overlay.Crds {
			if !slices.Contains(b.Crds, ref) {
				b.Crds = append(b.Crds, ref)
			}
		}

		for _, overlayK := range overlay.Kustomizations {
			if err := b.strategicMergeKustomization(overlayK); err != nil {
				return err
			}
		}

		for _, sys := range overlay.FluxSystems {
			if err := b.UpsertFluxSystem(sys); err != nil {
				return err
			}
		}

		if overlay.Substitutions != nil {
			if b.Substitutions == nil {
				b.Substitutions = make(map[string]string)
			}
			maps.Copy(b.Substitutions, overlay.Substitutions)
		}

		if overlay.ConfigMaps != nil {
			if b.ConfigMaps == nil {
				b.ConfigMaps = make(map[string]map[string]string)
			}
			for name, data := range overlay.ConfigMaps {
				if b.ConfigMaps[name] == nil {
					b.ConfigMaps[name] = make(map[string]string)
				}
				maps.Copy(b.ConfigMaps[name], data)
			}
		}

		// Post-run messages accumulate across overlays, preserving order; identical (when, text)
		// entries are dropped so the same note contributed by multiple sources surfaces once.
		for _, overlayMsg := range overlay.Messages {
			if !slices.ContainsFunc(b.Messages, func(m Message) bool { return m == overlayMsg }) {
				b.Messages = append(b.Messages, overlayMsg)
			}
		}
	}

	return b.validateTerraformComponents()
}

// ReplaceTerraformComponent replaces an existing TerraformComponent with the provided component.
// Components are matched by component ID (name if provided, otherwise Path).
// If a matching component exists, it is completely replaced. Otherwise, the component is appended.
// Returns an error if a dependency cycle is detected during sorting or if component IDs are not unique.
func (b *Blueprint) ReplaceTerraformComponent(component TerraformComponent) error {
	componentID := component.GetID()

	for i, existing := range b.TerraformComponents {
		existingID := existing.GetID()

		if existingID == componentID {
			b.TerraformComponents[i] = component
			if err := b.sortTerraform(); err != nil {
				return err
			}
			return b.validateTerraformComponents()
		}
	}
	b.TerraformComponents = append(b.TerraformComponents, component)
	if err := b.sortTerraform(); err != nil {
		return err
	}
	return b.validateTerraformComponents()
}

// ReplaceKustomization replaces an existing Kustomization with the provided kustomization.
// If a kustomization with the same Name exists, it is completely replaced. Otherwise, the kustomization is appended.
// Returns an error if a dependency cycle is detected during sorting.
func (b *Blueprint) ReplaceKustomization(kustomization Kustomization) error {
	for i, existing := range b.Kustomizations {
		if existing.Name == kustomization.Name {
			b.Kustomizations[i] = kustomization
			return b.sortKustomize()
		}
	}
	b.Kustomizations = append(b.Kustomizations, kustomization)
	return b.sortKustomize()
}

// RemoveTerraformComponent removes specified non-index fields from an existing TerraformComponent.
// Components are matched by component ID (name if provided, otherwise Path).
// It removes inputs, dependencies, and other fields that are specified in the removal component.
// Index fields (Name, Path, Source) are not affected. If no matching component exists, no action is taken.
func (b *Blueprint) RemoveTerraformComponent(removal TerraformComponent) error {
	removalID := removal.GetID()
	for i, existing := range b.TerraformComponents {
		existingID := existing.GetID()

		if existingID == removalID {
			if len(removal.Inputs) > 0 && existing.Inputs != nil {
				for key := range removal.Inputs {
					delete(existing.Inputs, key)
				}
			}

			if len(removal.DependsOn) > 0 {
				existing.DependsOn = slices.DeleteFunc(existing.DependsOn, func(dep string) bool {
					return slices.Contains(removal.DependsOn, dep)
				})
			}

			b.TerraformComponents[i] = existing
			return b.sortTerraform()
		}
	}
	return nil
}

// RemoveKustomization removes specified non-index fields from an existing Kustomization.
// It finds a kustomization matching the same Name, then removes patches, components, dependencies,
// cleanup items, and substitutions that are specified in the removal kustomization.
// The index field (Name) is not affected. If no matching kustomization exists, no action is taken.
func (b *Blueprint) RemoveKustomization(removal Kustomization) error {
	for i, existing := range b.Kustomizations {
		if existing.Name == removal.Name {
			b.Kustomizations[i] = subtractKustomizationFields(existing, removal)
			return b.sortKustomize()
		}
	}
	return nil
}

// RemoveFluxSystem removes specified non-index fields from an existing FluxSystem. It finds a
// system matching the same Name, then subtracts dependsOn entries and any install fields named in
// removal.Install (reusing subtractKustomizationFields, since Install is itself a Kustomization
// carrying no Name until tier compilation, so it cannot go through RemoveKustomization's by-Name
// lookup), and drops any resources variant whose Name is named in removal.Resources (empty Name
// matches the unnamed variant). A resources variant has no kustomize: analog — it compiles to its
// own distinct Kustomization — so naming it is itself the field-level operation, not a whole-system
// deletion. The index field (Name) is not affected. If no matching system exists, no action is taken.
func (b *Blueprint) RemoveFluxSystem(removal FluxSystem) error {
	for i, existing := range b.FluxSystems {
		if existing.Name != removal.Name {
			continue
		}

		existing.DependsOn = subtractStringSlice(existing.DependsOn, removal.DependsOn)

		if removal.Install != nil && existing.Install != nil {
			subtracted := subtractKustomizationFields(*existing.Install, *removal.Install)
			existing.Install = &subtracted
		}

		if len(removal.Resources) > 0 {
			existing.Resources = slices.DeleteFunc(existing.Resources, func(v FluxVariant) bool {
				return slices.ContainsFunc(removal.Resources, func(rv FluxVariant) bool {
					return rv.Name == v.Name
				})
			})
		}

		b.FluxSystems[i] = existing
		return nil
	}
	return nil
}

// UpsertFluxSystem merges sys into the FluxSystem with the same Name if one exists in
// b.FluxSystems, or appends it when none does. On a match, DependsOn accumulates (union),
// Install/Resources deep-merge via MergeFluxInstall/MergeFluxVariants, and Secrets union per data key
// — the same field-level semantics strategicMergeKustomization already gives same-name plain
// kustomize: entries — rather than sys wholesale-replacing what is already there. This is the path
// StrategicMerge uses to combine FluxSystems across sources (each source's facets already merged among
// themselves before reaching here), so a same-name collision here must not silently drop one side's
// install/resources tiers or Secrets the way a blind replace would.
func (b *Blueprint) UpsertFluxSystem(sys FluxSystem) error {
	for i, existing := range b.FluxSystems {
		if existing.Name == sys.Name {
			merged := existing
			for _, dep := range sys.DependsOn {
				if dep != "" && !slices.Contains(merged.DependsOn, dep) {
					merged.DependsOn = append(merged.DependsOn, dep)
				}
			}
			merged.Install = MergeFluxInstall(existing.Install, sys.Install)
			merged.Resources = MergeFluxVariants(existing.Resources, sys.Resources)
			merged.Secrets = mergeSecretData(existing.Secrets, sys.Secrets)
			b.FluxSystems[i] = merged
			return nil
		}
	}
	b.FluxSystems = append(b.FluxSystems, sys)
	return nil
}

// MergeFluxInstall deep-merges overlay's Install tier into existing's via MergeKustomizationFields
// (Components/DependsOn accumulate, other fields are overridden by overlay when set) rather than
// via by-name Blueprint.Kustomizations matching, since Install carries no Name until tier
// compilation and that by-name path is a no-op on an unnamed Kustomization. Either side being nil
// just takes the other, matching how a facet or source with no Install opinion shouldn't erase one
// contributed elsewhere.
func MergeFluxInstall(existing, overlay *Kustomization) *Kustomization {
	if overlay == nil {
		return existing
	}
	if existing == nil {
		return overlay
	}
	merged := MergeKustomizationFields(*existing, *overlay)
	return &merged
}

// MergeFluxVariants combines two resources-variant lists keyed by variant name. A variant from
// overlay deep-merges into the same-named variant in existing via MergeKustomizationFields (see
// MergeFluxInstall for why by-name Blueprint.Kustomizations matching isn't used instead), and the
// two When conditions combine with AND. A variant with a new name is appended in order. This keeps
// a cross-facet or cross-source merge from emitting two Kustomizations with the same
// "<system>-resources[-<name>]" identity.
func MergeFluxVariants(existing, overlay []FluxVariant) []FluxVariant {
	merged := slices.Clone(existing)
	for _, nv := range overlay {
		idx := slices.IndexFunc(merged, func(v FluxVariant) bool { return v.Name == nv.Name })
		if idx == -1 {
			merged = append(merged, nv)
			continue
		}

		mergedK := MergeKustomizationFields(merged[idx].Kustomization, nv.Kustomization)

		combinedWhen := merged[idx].When
		switch {
		case merged[idx].When != "" && nv.When != "":
			combinedWhen = fmt.Sprintf("(%s) && (%s)", merged[idx].When, nv.When)
		case nv.When != "":
			combinedWhen = nv.When
		}
		merged[idx] = FluxVariant{
			Kustomization: mergedK,
			When:          combinedWhen,
		}
	}
	return merged
}

// MergeKustomizationFields deep-merges overlay onto base and returns the result without mutating
// either input. Components and DependsOn accumulate, deduplicated, with Components sorted.
// Patches accumulate. Substitutions copy in. Every other field is overridden by overlay when
// overlay sets it.
//
// HealthCheckExprs accumulate, with an overlay entry replacing a base entry of the same
// APIVersion and Kind. HealthChecks accumulate the same way, matching on APIVersion, Kind, and
// Name. Namespace is excluded from that match, since it defaults to the kustomization's own
// namespace and so cannot reliably distinguish one entry from another.
//
// Callers that need by-name matching within a Blueprint's Kustomizations list use
// strategicMergeKustomization, which calls this once a match is found; callers merging two
// already-matched Kustomization values directly (e.g. FluxSystem tiers, which carry no Name
// until tier compilation) call this directly.
func MergeKustomizationFields(base, overlay Kustomization) Kustomization {
	existing := base
	existing.Components = slices.Clone(base.Components)
	existing.DependsOn = slices.Clone(base.DependsOn)
	existing.Patches = slices.Clone(base.Patches)
	existing.HealthCheckExprs = slices.Clone(base.HealthCheckExprs)
	existing.HealthChecks = slices.Clone(base.HealthChecks)
	existing.Substitutions = maps.Clone(base.Substitutions)
	for _, component := range overlay.Components {
		if component == "" || !slices.Contains(existing.Components, component) {
			existing.Components = append(existing.Components, component)
		}
	}
	slices.Sort(existing.Components)
	for _, dep := range overlay.DependsOn {
		if dep != "" && !slices.Contains(existing.DependsOn, dep) {
			existing.DependsOn = append(existing.DependsOn, dep)
		}
	}
	if overlay.Path != "" {
		existing.Path = overlay.Path
	}
	if overlay.Source != "" {
		existing.Source = overlay.Source
	}
	if overlay.Namespace != "" {
		existing.Namespace = overlay.Namespace
	}
	if overlay.TargetNamespace != "" {
		existing.TargetNamespace = overlay.TargetNamespace
	}
	if overlay.Destroy != nil {
		existing.Destroy = overlay.Destroy
	}
	if overlay.Enabled != nil {
		existing.Enabled = overlay.Enabled
	}
	if overlay.DestroyOnly != nil {
		existing.DestroyOnly = overlay.DestroyOnly
	}
	if overlay.Interval != nil {
		existing.Interval = overlay.Interval
	}
	if overlay.RetryInterval != nil {
		existing.RetryInterval = overlay.RetryInterval
	}
	if overlay.Timeout != nil {
		existing.Timeout = overlay.Timeout
	}
	if overlay.DeleteTimeout != nil {
		existing.DeleteTimeout = overlay.DeleteTimeout
	}
	if overlay.Wait != nil {
		existing.Wait = overlay.Wait
	}
	if overlay.Force != nil {
		existing.Force = overlay.Force
	}
	if overlay.Prune != nil {
		existing.Prune = overlay.Prune
	}
	if len(overlay.Patches) > 0 {
		existing.Patches = append(existing.Patches, overlay.Patches...)
	}
	for _, expr := range overlay.HealthCheckExprs {
		index := slices.IndexFunc(existing.HealthCheckExprs, func(candidate HealthCheckExpr) bool {
			return candidate.APIVersion == expr.APIVersion && candidate.Kind == expr.Kind
		})
		if index >= 0 {
			existing.HealthCheckExprs[index] = expr
			continue
		}
		existing.HealthCheckExprs = append(existing.HealthCheckExprs, expr)
	}
	for _, ref := range overlay.HealthChecks {
		index := slices.IndexFunc(existing.HealthChecks, func(candidate HealthCheckRef) bool {
			return candidate.APIVersion == ref.APIVersion && candidate.Kind == ref.Kind &&
				candidate.Name == ref.Name
		})
		if index >= 0 {
			existing.HealthChecks[index] = ref
			continue
		}
		existing.HealthChecks = append(existing.HealthChecks, ref)
	}
	if len(overlay.Substitutions) > 0 {
		if existing.Substitutions == nil {
			existing.Substitutions = make(map[string]string)
		}
		maps.Copy(existing.Substitutions, overlay.Substitutions)
	}
	if overlay.Decryption != nil {
		existing.Decryption = overlay.Decryption.DeepCopy()
	}
	return existing
}

// DeepMergeMaps returns a new map from a deep merge of base and overlay maps.
// Overlay values take precedence; nested maps merge recursively. Non-map overlay values replace base values.
// Empty overlay values—including empty string "", empty slice, and empty map—never overwrite an existing
// non-empty base value, so populated Inputs (or other merged maps) are preserved when overlay has empty values.
// Map-like overlay values are normalized to map[string]any. Package-level so callers without a Blueprint can merge
// scope maps (e.g. context over facet scope) without losing nested facet-derived keys.
func DeepMergeMaps(base, overlay map[string]any) map[string]any {
	result := maps.Clone(base)
	for k, overlayValue := range overlay {
		overlayMap := ToMapStringAny(overlayValue)
		if baseValue, exists := result[k]; exists {
			if baseMap, baseIsMap := baseValue.(map[string]any); baseIsMap && overlayMap != nil {
				result[k] = DeepMergeMaps(baseMap, overlayMap)
				continue
			}
		}
		var effectiveOverlay any
		if overlayMap != nil {
			effectiveOverlay = overlayMap
		} else {
			effectiveOverlay = overlayValue
		}
		baseVal, existsInBase := result[k]
		if isEmptyMergeValue(effectiveOverlay) && existsInBase && baseVal != nil && !isEmptyMergeValue(baseVal) {
			continue
		}
		if overlayMap != nil {
			result[k] = overlayMap
		} else {
			result[k] = overlayValue
		}
	}
	return result
}

// ToMapStringAny converts a map-like value to map[string]any recursively (e.g. map[interface{}]interface{}
// from YAML or expr). Returns nil if v is not a map. Ensures blueprint Inputs are always map[string]any
// so YAML marshaling produces nested YAML rather than Go's %v string format.
func ToMapStringAny(v any) map[string]any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Map {
		return nil
	}
	out := make(map[string]any, rv.Len())
	iter := rv.MapRange()
	for iter.Next() {
		k := iter.Key()
		var key string
		switch k.Kind() {
		case reflect.Interface:
			if k.IsNil() {
				continue
			}
			key = fmt.Sprint(k.Elem().Interface())
		default:
			key = fmt.Sprint(k.Interface())
		}
		val := iter.Value().Interface()
		if nested := ToMapStringAny(val); nested != nil {
			out[key] = nested
		} else if reflect.ValueOf(val).Kind() == reflect.Slice {
			out[key] = ToSliceAny(val)
		} else {
			out[key] = val
		}
	}
	return out
}

// ToSliceAny converts a slice-like value to []any recursively (e.g. []interface{} from YAML or expr).
// Returns nil if v is not a slice.
func ToSliceAny(v any) []any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil
	}
	out := make([]any, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		elem := rv.Index(i)
		if elem.Kind() == reflect.Interface && !elem.IsNil() {
			elem = elem.Elem()
		}
		val := elem.Interface()
		if nested := ToMapStringAny(val); nested != nil {
			out = append(out, nested)
		} else if reflect.ValueOf(val).Kind() == reflect.Slice {
			out = append(out, ToSliceAny(val))
		} else {
			out = append(out, val)
		}
	}
	return out
}

// =============================================================================
// Private Methods
// =============================================================================

// validateTerraformComponents validates that all terraform component IDs are unique.
// Component IDs are either the Name (if provided) or Path (if no name).
// Returns an error if duplicate IDs are found.
func (b *Blueprint) validateTerraformComponents() error {
	idToComponent := make(map[string]TerraformComponent)

	for _, component := range b.TerraformComponents {
		componentID := component.GetID()

		if existing, exists := idToComponent[componentID]; exists {
			return fmt.Errorf("duplicate terraform component ID %q: component with name %q and path %q conflicts with existing component with name %q and path %q", componentID, component.Name, component.Path, existing.Name, existing.Path)
		}

		idToComponent[componentID] = component
	}

	return nil
}

// strategicMergeTerraformComponent performs a strategic merge of the provided TerraformComponent into the Blueprint.
// It merges values, appends unique dependencies, updates fields if provided, and maintains dependency order.
// Components are matched by component ID (name if provided, otherwise Path).
// Returns an error if a dependency cycle is detected during sorting or if component IDs are not unique.
func (b *Blueprint) strategicMergeTerraformComponent(component TerraformComponent) error {
	componentID := component.GetID()

	for i, existing := range b.TerraformComponents {
		existingID := existing.GetID()

		if existingID == componentID {
			if len(component.Inputs) > 0 {
				if existing.Inputs == nil {
					existing.Inputs = make(map[string]any)
				}
				preMergeExisting := maps.Clone(existing.Inputs)
				existing.Inputs = b.deepMergeMaps(existing.Inputs, component.Inputs)
				if component.InputOrigins != nil {
					if existing.InputOrigins == nil {
						existing.InputOrigins = make(map[string]string)
					}
					for key, path := range component.InputOrigins {
						existingVal, existingIsMap := preMergeExisting[key].(map[string]any)
						componentVal, componentIsMap := component.Inputs[key].(map[string]any)
						if existingIsMap && componentIsMap {
							if existingOrigin, ok := existing.InputOrigins[key]; ok {
								expandInputOrigins(existing.InputOrigins, key, existingVal, existingOrigin)
							}
							expandInputOrigins(existing.InputOrigins, key, componentVal, path)
							delete(existing.InputOrigins, key)
						} else {
							existing.InputOrigins[key] = path
						}
					}
				}
			}
			for _, dep := range component.DependsOn {
				if dep != "" && !slices.Contains(existing.DependsOn, dep) {
					existing.DependsOn = append(existing.DependsOn, dep)
				}
			}
			if component.Destroy != nil {
				existing.Destroy = component.Destroy
			}
			if component.Parallelism != nil {
				existing.Parallelism = component.Parallelism
			}
			if component.Enabled != nil {
				existing.Enabled = component.Enabled
			}
			if component.Name != "" {
				existing.Name = component.Name
			}
			b.TerraformComponents[i] = existing
			if err := b.sortTerraform(); err != nil {
				return err
			}
			return b.validateTerraformComponents()
		}
	}

	b.TerraformComponents = append(b.TerraformComponents, component)
	if err := b.sortTerraform(); err != nil {
		return err
	}
	return b.validateTerraformComponents()
}

// strategicMergeKustomization performs a strategic merge of the provided Kustomization into the Blueprint.
// It merges components and dependencies, updates fields if provided, and maintains dependency order.
// Empty-string components are always appended so facet-defined placeholder slots are preserved;
// other components are appended only when not already present. Patches from the provided
// kustomization are appended to existing patches. Returns an error if a dependency cycle is detected during sorting.
func (b *Blueprint) strategicMergeKustomization(kustomization Kustomization) error {
	if kustomization.Name == "" {
		return nil
	}

	for i, existing := range b.Kustomizations {
		if existing.Name == kustomization.Name {
			b.Kustomizations[i] = MergeKustomizationFields(existing, kustomization)
			return b.sortKustomize()
		}
	}
	b.Kustomizations = append(b.Kustomizations, kustomization)
	return b.sortKustomize()
}

// deepMergeMaps delegates to DeepMergeMaps so Blueprint merge logic stays in one place.
func (b *Blueprint) deepMergeMaps(base, overlay map[string]any) map[string]any {
	return DeepMergeMaps(base, overlay)
}

// =============================================================================
// Helpers
// =============================================================================

// mergeSecretData unions two secret maps (Secret name -> SecretEntry) with overlay winning per data key
// and the two entries' namespaces unioned, and nil when both are empty. Neither input is mutated. Used
// to combine a flux system's Secrets when the same-named system is upserted across sources.
func mergeSecretData(base, overlay map[string]SecretEntry) map[string]SecretEntry {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := cloneSecretData(base)
	if out == nil {
		out = make(map[string]SecretEntry, len(overlay))
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

// subtractStringSlice returns existing with every element also present in removal deleted.
// Shared by subtractKustomizationFields (DependsOn, Components) and RemoveFluxSystem
// (DependsOn), the two field-level removal entry points.
func subtractStringSlice(existing, removal []string) []string {
	if len(removal) == 0 {
		return existing
	}
	return slices.DeleteFunc(existing, func(item string) bool {
		return slices.Contains(removal, item)
	})
}

// subtractKustomizationFields returns existing with the patches, components, dependencies, and
// substitutions named in removal stripped out. Patches match by Path or Patch content equality;
// everything else matches by value/key. Fields removal leaves empty are left untouched on
// existing. The Name field is never touched, so this is safe to use on both indexed
// (Blueprint.Kustomizations) and unindexed (a FluxSystem's Install tier) Kustomizations.
func subtractKustomizationFields(existing, removal Kustomization) Kustomization {
	existing.DependsOn = subtractStringSlice(existing.DependsOn, removal.DependsOn)
	existing.Components = subtractStringSlice(existing.Components, removal.Components)

	if len(removal.Patches) > 0 {
		existing.Patches = slices.DeleteFunc(existing.Patches, func(patch BlueprintPatch) bool {
			for _, removalPatch := range removal.Patches {
				if removalPatch.Path != "" && patch.Path == removalPatch.Path {
					return true
				}
				if removalPatch.Patch != "" && patch.Patch == removalPatch.Patch {
					return true
				}
			}
			return false
		})
	}

	if len(removal.Substitutions) > 0 && existing.Substitutions != nil {
		for key := range removal.Substitutions {
			delete(existing.Substitutions, key)
		}
	}

	return existing
}

// expandInputOrigins recursively populates origins with dot-separated sub-key paths for every
// leaf in m, allowing per-sub-key origin tracking after a deep merge of nested input maps.
func expandInputOrigins(origins map[string]string, prefix string, m map[string]any, path string) {
	for k, v := range m {
		fullKey := prefix + "." + k
		if subMap, ok := v.(map[string]any); ok {
			expandInputOrigins(origins, fullKey, subMap, path)
		} else {
			origins[fullKey] = path
		}
	}
}

// isEmptyMergeValue returns true for empty string "", empty slice, or empty map so that
// DeepMergeMaps never lets an empty overlay overwrite a non-empty base. Returns false for nil
// so that explicit nil in the overlay can still remove or clear a key (e.g. removal merge).
func isEmptyMergeValue(v any) bool {
	if v == nil {
		return false
	}
	if s, ok := v.(string); ok {
		return s == ""
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map, reflect.Slice:
		return rv.Len() == 0
	}
	return false
}
