package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// The ValuesSource is a configuration source for context values.yaml files.
// It loads and saves dynamic context configuration values.
// Save patches an existing file at changed paths only; other writes are full.

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

// Save writes values.yaml for a context, patching an existing file at overrides/deletes only.
// wrote reports whether a write actually happened, so a caller tracking pending writes knows
// whether they were consumed or are still outstanding.
func (s *valuesSource) Save(
	projectRoot string,
	contextName string,
	data map[string]any,
	overrides map[string]any,
	deletes []string,
	overwrite bool,
	input persistencePolicyInput,
) (wrote bool, err error) {
	contextDir := filepath.Join(projectRoot, "contexts", contextName)
	if err := s.shims.MkdirAll(contextDir, 0755); err != nil {
		return false, fmt.Errorf("error creating context directory: %w", err)
	}

	if len(data) == 0 {
		return false, nil
	}

	valuesPath := filepath.Join(contextDir, "values.yaml")
	valuesExists := false
	if _, err := s.shims.Stat(valuesPath); err == nil {
		valuesExists = true
	}

	if valuesExists && !overwrite {
		return false, nil
	}

	partition := s.policy.Partition(data, input)

	if valuesExists {
		return s.patchValues(valuesPath, overrides, deletes, partition.Values, input)
	}

	marshaled, err := s.shims.YamlMarshal(partition.Values)
	if err != nil {
		return false, fmt.Errorf("error marshalling values.yaml: %w", err)
	}

	if err := s.shims.WriteFile(valuesPath, marshaled, 0644); err != nil {
		return false, fmt.Errorf("error writing values.yaml: %w", err)
	}

	return true, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// patchValues merges overrides and removes deletes at valuesPath, or writes fullValues whole
// when the existing file can't be parsed.
func (s *valuesSource) patchValues(valuesPath string, overrides map[string]any, deletes []string, fullValues map[string]any, input persistencePolicyInput) (bool, error) {
	overridesPartition := s.policy.Partition(overrides, input)
	if len(overridesPartition.Values) == 0 && len(deletes) == 0 {
		return false, nil
	}

	merged, patchable := s.mergedValuesYAML(valuesPath, overridesPartition.Values, deletes)
	if !patchable {
		if len(fullValues) == 0 {
			return false, nil
		}
		var err error
		merged, err = s.shims.YamlMarshal(fullValues)
		if err != nil {
			return false, fmt.Errorf("error marshalling values.yaml: %w", err)
		}
	}

	if err := s.shims.WriteFile(valuesPath, merged, 0644); err != nil {
		return false, fmt.Errorf("error writing values.yaml: %w", err)
	}

	return true, nil
}

// mergedValuesYAML merges overrideValues and deletes into valuesPath's content, or false if
// either side fails to parse as a YAML mapping.
func (s *valuesSource) mergedValuesYAML(valuesPath string, overrideValues map[string]any, deletes []string) ([]byte, bool) {
	current, err := s.shims.ReadFile(valuesPath)
	if err != nil {
		return nil, false
	}

	dstMap, ok := parseRootMapping(current)
	if !ok {
		return nil, false
	}

	if len(overrideValues) > 0 {
		srcBytes, err := s.shims.YamlMarshal(overrideValues)
		if err != nil {
			return nil, false
		}
		srcMap, ok := parseRootMapping(srcBytes)
		if !ok {
			return nil, false
		}
		mergeMappingNodes(dstMap, srcMap)
	}

	removeMappingKeys(dstMap, deletes)

	rendered := dstMap.String()
	if !strings.HasSuffix(rendered, "\n") {
		rendered += "\n"
	}
	return []byte(rendered), true
}

// parseRootMapping parses source and returns its root mapping node, or false on failure.
func parseRootMapping(source []byte) (*ast.MappingNode, bool) {
	file, err := parser.ParseBytes(source, parser.ParseComments)
	if err != nil || len(file.Docs) == 0 || file.Docs[0].Body == nil {
		return nil, false
	}
	m, ok := file.Docs[0].Body.(*ast.MappingNode)
	return m, ok
}

// mergeMappingNodes recursively merges src into dst, leaving dst-only keys untouched.
func mergeMappingNodes(dst, src *ast.MappingNode) {
	byKey := make(map[string]*ast.MappingValueNode, len(dst.Values))
	for _, v := range dst.Values {
		byKey[v.Key.String()] = v
	}

	for _, srcVal := range src.Values {
		dstVal, exists := byKey[srcVal.Key.String()]
		if !exists {
			dst.Values = append(dst.Values, srcVal)
			continue
		}

		dstChild, dstIsMap := dstVal.Value.(*ast.MappingNode)
		srcChild, srcIsMap := srcVal.Value.(*ast.MappingNode)
		if dstIsMap && srcIsMap {
			mergeMappingNodes(dstChild, srcChild)
			continue
		}
		dstVal.Value = srcVal.Value
	}
}

// removeMappingKeys drops each named top-level key from m, in place.
func removeMappingKeys(m *ast.MappingNode, keys []string) {
	if len(keys) == 0 {
		return
	}

	remove := make(map[string]bool, len(keys))
	for _, key := range keys {
		remove[key] = true
	}

	kept := m.Values[:0]
	for _, v := range m.Values {
		if !remove[v.Key.String()] {
			kept = append(kept, v)
		}
	}
	m.Values = kept
}
