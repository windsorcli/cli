package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// The ValuesSource is a configuration source for context values.yaml files.
// It provides load and save operations for dynamic context configuration values,
// The ValuesSource applies schema validation warnings during load and workstation filtering on save,
// and keeps values.yaml persistence behavior isolated from handler orchestration.

// =============================================================================
// Types
// =============================================================================

// valuesSource handles values.yaml loading and persistence behavior.
type valuesSource struct {
	shims           *Shims
	schemaValidator *SchemaValidator
	policy          *persistencePolicy
}

// =============================================================================
// Constructor
// =============================================================================

// newValuesSource creates a valuesSource with schema validator dependency.
func newValuesSource(shims *Shims, schemaValidator *SchemaValidator, policy *persistencePolicy) *valuesSource {
	if shims == nil {
		shims = NewShims()
	}
	if policy == nil {
		policy = newPersistencePolicy()
	}

	return &valuesSource{
		shims:           shims,
		schemaValidator: schemaValidator,
		policy:          policy,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Load loads values.yaml for a context and emits schema-validation warnings when invalid.
func (s *valuesSource) Load(projectRoot, contextName string) (map[string]any, bool, error) {
	valuesPath := filepath.Join(projectRoot, "contexts", contextName, "values.yaml")
	if _, err := s.shims.Stat(valuesPath); err != nil {
		return nil, false, nil
	}

	fileData, err := s.shims.ReadFile(valuesPath)
	if err != nil {
		return nil, false, fmt.Errorf("error reading values.yaml: %w", err)
	}

	var values map[string]any
	if err := s.shims.YamlUnmarshal(fileData, &values); err != nil {
		return nil, false, fmt.Errorf("error unmarshalling values.yaml: %w", err)
	}

	if s.schemaValidator != nil && s.schemaValidator.Schema != nil {
		if result, err := s.schemaValidator.Validate(values); err == nil && !result.Valid {
			fmt.Fprintf(os.Stderr, "Warning: values.yaml validation failed (config still loaded):%s\n", FormatValidationErrors(result.Errors))
			s.schemaValidator.MarkErrorsReported(result.Errors)
		}
	}

	return values, true, nil
}

// Save writes values.yaml for a context, with optional overwrite and workstation-key cleaning.
func (s *valuesSource) Save(
	projectRoot string,
	contextName string,
	data map[string]any,
	overwrite bool,
	input persistencePolicyInput,
) error {
	contextDir := filepath.Join(projectRoot, "contexts", contextName)
	if err := s.shims.MkdirAll(contextDir, 0755); err != nil {
		return fmt.Errorf("error creating context directory: %w", err)
	}

	if len(data) == 0 {
		return nil
	}

	valuesPath := filepath.Join(contextDir, "values.yaml")
	valuesExists := false
	if _, err := s.shims.Stat(valuesPath); err == nil {
		valuesExists = true
	}

	if valuesExists && !overwrite {
		return nil
	}

	partition := s.policy.Partition(data, input)

	if valuesExists {
		if dropped := s.droppedKeys(valuesPath, partition.Values); len(dropped) > 0 {
			return fmt.Errorf("refusing to write values.yaml for context %q: this write would drop %v present in the current file. This looks like a bug, not an intended change. No changes were written", contextName, dropped)
		}
	}

	marshaled, err := s.shims.YamlMarshal(partition.Values)
	if err != nil {
		return fmt.Errorf("error marshalling values.yaml: %w", err)
	}

	if err := s.shims.WriteFile(valuesPath, marshaled, 0644); err != nil {
		return fmt.Errorf("error writing values.yaml: %w", err)
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// droppedKeys compares the file already on disk at valuesPath against next, the map about to be
// written, and returns each dotted path (e.g. "terraform.backend.type") present on disk but
// missing from next. It walks into nested maps rather than stopping at the top level, since a
// field buried inside a section that itself survives the write is just as much a loss as the
// whole section disappearing. Save persists the full merged config, a superset of disk by
// construction, so a legitimate write should never lose a path that was there before. A
// top-level isVolatile key is skipped, since the policy may relocate or discard those on its
// own; the same name reappearing nested is an unrelated field and is not exempted. An
// unreadable or unparseable existing file returns no dropped keys rather than blocking the
// write, since there is nothing trustworthy left to compare against.
func (s *valuesSource) droppedKeys(valuesPath string, next map[string]any) []string {
	current, err := s.shims.ReadFile(valuesPath)
	if err != nil {
		return nil
	}

	var existing map[string]any
	if err := s.shims.YamlUnmarshal(current, &existing); err != nil {
		return nil
	}

	dropped := s.droppedKeysIn("", existing, next)
	sort.Strings(dropped)
	return dropped
}

// droppedKeysIn walks one level of the comparison droppedKeys performs, prefixing every reported
// path with prefix so a nested miss reads as "terraform.backend.type" rather than bare "type".
// A key present in existing but absent from next is dropped outright. A key present in both as a
// map recurses; a key present in both as anything else is left alone, since droppedKeys only
// tracks disappearance, not value changes. isVolatile only exempts a top-level key (prefix ""),
// matching its own top-level-only contract; a nested key that happens to share a volatile name
// (e.g. "secrets.provider") is an unrelated field and is tracked like any other.
func (s *valuesSource) droppedKeysIn(prefix string, existing, next map[string]any) []string {
	var dropped []string
	for key, existingValue := range existing {
		if prefix == "" && s.policy.isVolatile(key) {
			continue
		}

		path := key
		if prefix != "" {
			path = prefix + "." + key
		}

		nextValue, ok := next[key]
		if !ok {
			dropped = append(dropped, path)
			continue
		}

		if existingMap, isMap := existingValue.(map[string]any); isMap {
			if nextMap, isMap := nextValue.(map[string]any); isMap {
				dropped = append(dropped, s.droppedKeysIn(path, existingMap, nextMap)...)
			}
		}
	}
	return dropped
}
