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
		if dropped := s.droppedTopLevelKeys(valuesPath, partition.Values); len(dropped) > 0 {
			return fmt.Errorf("refusing to write values.yaml for context %q: this write would drop top-level section(s) %v present in the current file. This looks like a bug, not an intended change. No changes were written", contextName, dropped)
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

// droppedTopLevelKeys compares the top-level keys already on disk at valuesPath against next,
// the map about to be written, and returns any key present on disk but absent from next. Save
// persists the full merged config, a superset of disk by construction, so a legitimate write
// should never produce a strict subset of the prior top-level keys. isVolatile keys are skipped,
// since the policy may relocate or discard those on its own. An unreadable or unparseable
// existing file returns no dropped keys rather than blocking the write, since there is nothing
// trustworthy left to compare against.
func (s *valuesSource) droppedTopLevelKeys(valuesPath string, next map[string]any) []string {
	current, err := s.shims.ReadFile(valuesPath)
	if err != nil {
		return nil
	}

	var existing map[string]any
	if err := s.shims.YamlUnmarshal(current, &existing); err != nil {
		return nil
	}

	var dropped []string
	for key := range existing {
		if s.policy.isVolatile(key) {
			continue
		}
		if _, ok := next[key]; !ok {
			dropped = append(dropped, key)
		}
	}
	sort.Strings(dropped)
	return dropped
}
