package config

import (
	"fmt"
	"os"
	"path/filepath"
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
			fmt.Fprintf(os.Stderr, "Warning: values.yaml validation failed (config still loaded): %v\n", result.Errors)
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

	if !valuesExists || overwrite {
		partition := s.policy.Partition(data, input)
		marshaled, err := s.shims.YamlMarshal(partition.Values)
		if err != nil {
			return fmt.Errorf("error marshalling values.yaml: %w", err)
		}

		if err := s.shims.WriteFile(valuesPath, marshaled, 0644); err != nil {
			return fmt.Errorf("error writing values.yaml: %w", err)
		}

		return nil
	}

	fileData, err := s.shims.ReadFile(valuesPath)
	if err != nil {
		return nil
	}

	var existing map[string]any
	if err := s.shims.YamlUnmarshal(fileData, &existing); err != nil {
		return nil
	}

	cleaned := s.policy.Partition(existing, input).Values
	if len(cleaned) != len(existing) {
		marshaled, err := s.shims.YamlMarshal(cleaned)
		if err != nil {
			return nil
		}
		if err := s.shims.WriteFile(valuesPath, marshaled, 0644); err != nil {
			return fmt.Errorf("error writing cleaned values.yaml: %w", err)
		}
	}

	return nil
}
