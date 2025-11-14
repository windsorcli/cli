package config

import (
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// The SchemaValidator is a JSON Schema validation component for Windsor blueprints.
// It provides comprehensive validation capabilities including type checking, pattern matching,
// and support for both boolean and schema object additionalProperties validation.
// The SchemaValidator enables robust configuration validation with detailed error reporting.

// =============================================================================
// Types
// =============================================================================

// SchemaValidator handles Windsor blueprint schema validation
type SchemaValidator struct {
	shell  shell.Shell
	Shims  *Shims
	Schema map[string]any
}

// SchemaValidationResult contains validation results and extracted defaults
type SchemaValidationResult struct {
	Valid    bool           `json:"valid"`
	Errors   []string       `json:"errors,omitempty"`
	Defaults map[string]any `json:"defaults,omitempty"`
}

// =============================================================================
// Constructor
// =============================================================================

// NewSchemaValidator creates a new schema validator instance
func NewSchemaValidator(shell shell.Shell) *SchemaValidator {
	return &SchemaValidator{
		shell: shell,
		Shims: NewShims(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// LoadSchema loads the schema.yaml file from the specified directory
// Returns error if schema file doesn't exist or is invalid
func (sv *SchemaValidator) LoadSchema(schemaPath string) error {
	schemaContent, err := sv.Shims.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("failed to read schema file %s: %w", schemaPath, err)
	}

	return sv.LoadSchemaFromBytes(schemaContent)
}

// LoadSchemaFromBytes loads schema directly from byte content
// Returns error if schema content is invalid
func (sv *SchemaValidator) LoadSchemaFromBytes(schemaContent []byte) error {
	var schema map[string]any
	if err := yaml.Unmarshal(schemaContent, &schema); err != nil {
		return fmt.Errorf("failed to parse schema YAML: %w", err)
	}

	if err := sv.validateSchemaStructure(schema); err != nil {
		return fmt.Errorf("invalid schema structure: %w", err)
	}

	sv.injectSubstitutionSchema(&schema)

	sv.Schema = schema
	return nil
}

// Validate validates user values against the loaded schema
// Returns validation result with errors and defaults
func (sv *SchemaValidator) Validate(values map[string]any) (*SchemaValidationResult, error) {
	if sv.Schema == nil {
		return nil, fmt.Errorf("no schema loaded - call LoadSchema first")
	}

	result := &SchemaValidationResult{
		Valid:  true,
		Errors: []string{},
	}

	defaults, err := sv.extractDefaults(sv.Schema)
	if err != nil {
		return nil, fmt.Errorf("failed to extract defaults from schema: %w", err)
	}
	result.Defaults = defaults

	errors := sv.validateObject(values, sv.Schema, "")
	if len(errors) > 0 {
		result.Valid = false
		result.Errors = errors
	}

	return result, nil
}

// GetSchemaDefaults extracts default values from the loaded schema
// Returns defaults as a map suitable for merging with user values
func (sv *SchemaValidator) GetSchemaDefaults() (map[string]any, error) {
	if sv.Schema == nil {
		return nil, fmt.Errorf("no schema loaded - call LoadSchema first")
	}

	return sv.extractDefaults(sv.Schema)
}

// =============================================================================
// Private Methods
// =============================================================================

// extractDefaults recursively extracts default values from a schema
func (sv *SchemaValidator) extractDefaults(schema map[string]any) (map[string]any, error) {
	defaults := make(map[string]any)

	properties, ok := schema["properties"]
	if !ok {
		return defaults, nil
	}

	propertiesMap, ok := properties.(map[string]any)
	if !ok {
		return defaults, nil
	}

	for propName, propSchema := range propertiesMap {
		propSchemaMap, ok := propSchema.(map[string]any)
		if !ok {
			continue
		}

		if defaultValue, hasDefault := propSchemaMap["default"]; hasDefault {
			defaults[propName] = defaultValue
		}

		if propType, ok := propSchemaMap["type"]; ok {
			if typeStr, ok := propType.(string); ok && typeStr == "object" {
				nestedDefaults, err := sv.extractDefaults(propSchemaMap)
				if err != nil {
					return nil, fmt.Errorf("failed to extract defaults for property %s: %w", propName, err)
				}
				if len(nestedDefaults) > 0 {
					defaults[propName] = nestedDefaults
				}
			}
		}
	}

	return defaults, nil
}

// validateObject validates a value against an object schema.
// It checks required fields, validates each property, and enforces additionalProperties constraints.
// Returns a slice of error messages for any validation failures encountered.
func (sv *SchemaValidator) validateObject(value map[string]any, schema map[string]any, path string) []string {
	var errors []string

	properties, ok := schema["properties"]
	if !ok {
		return errors
	}

	propertiesMap, ok := properties.(map[string]any)
	if !ok {
		return errors
	}

	if required, ok := schema["required"]; ok {
		if requiredSlice, ok := required.([]any); ok {
			for _, reqField := range requiredSlice {
				if reqFieldStr, ok := reqField.(string); ok {
					if _, exists := value[reqFieldStr]; !exists {
						fieldPath := sv.buildPath(path, reqFieldStr)
						errors = append(errors, fmt.Sprintf("missing required field: %s", fieldPath))
					}
				}
			}
		}
	}

	for propName, propValue := range value {
		propPath := sv.buildPath(path, propName)

		propSchema, exists := propertiesMap[propName]
		if !exists {
			if additionalProps, ok := schema["additionalProperties"]; ok {
				if allow, ok := additionalProps.(bool); ok && !allow {
					errors = append(errors, fmt.Sprintf("additional property not allowed: %s", propPath))
					continue
				}
				if additionalSchema, ok := additionalProps.(map[string]any); ok {
					propErrors := sv.validateValue(propValue, additionalSchema, propPath)
					errors = append(errors, propErrors...)
					continue
				}
			}
			continue
		}

		propSchemaMap, ok := propSchema.(map[string]any)
		if !ok {
			continue
		}

		propErrors := sv.validateValue(propValue, propSchemaMap, propPath)
		errors = append(errors, propErrors...)
	}

	return errors
}

// validateValue validates a single value against its schema.
// It checks type conformity, delegates to object or string validation as appropriate,
// and enforces enum constraints if present. Returns a slice of error messages for any violations.
func (sv *SchemaValidator) validateValue(value any, schema map[string]any, path string) []string {
	var errors []string

	expectedType, ok := schema["type"]
	if !ok {
		return errors
	}

	expectedTypeStr, ok := expectedType.(string)
	if !ok {
		return errors
	}

	actualType := sv.getValueType(value)
	if actualType != expectedTypeStr {
		errors = append(errors, fmt.Sprintf("type mismatch at %s: expected %s, got %s", path, expectedTypeStr, actualType))
		return errors
	}

	switch expectedTypeStr {
	case "object":
		if valueMap, ok := value.(map[string]any); ok {
			objErrors := sv.validateObject(valueMap, schema, path)
			errors = append(errors, objErrors...)
		}
	case "string":
		stringErrors := sv.validateString(value, schema, path)
		errors = append(errors, stringErrors...)
	case "array":
		arrayErrors := sv.validateArray(value, schema, path)
		errors = append(errors, arrayErrors...)
	case "integer":
		integerErrors := sv.validateInteger(value, schema, path)
		errors = append(errors, integerErrors...)
	case "boolean":
		booleanErrors := sv.validateBoolean(value, schema, path)
		errors = append(errors, booleanErrors...)
	}

	if enum, ok := schema["enum"]; ok {
		if enumSlice, ok := enum.([]any); ok {
			found := false
			for _, enumValue := range enumSlice {
				if sv.valuesEqual(value, enumValue) {
					found = true
					break
				}
			}
			if !found {
				errors = append(errors, fmt.Sprintf("value at %s not in allowed enum values", path))
			}
		}
	}

	return errors
}

// validateString checks if a value is a string and validates it against the pattern constraint in the schema.
// Returns a slice of error messages if the value does not match the pattern or if the pattern is invalid.
func (sv *SchemaValidator) validateString(value any, schema map[string]any, path string) []string {
	var errors []string

	str, ok := value.(string)
	if !ok {
		return errors
	}

	if pattern, ok := schema["pattern"]; ok {
		if patternStr, ok := pattern.(string); ok {
			matched, err := sv.Shims.RegexpMatchString(patternStr, str)
			if err != nil {
				errors = append(errors, fmt.Sprintf("invalid regex pattern for %s: %v", path, err))
			} else if !matched {
				errors = append(errors, fmt.Sprintf("string at %s does not match required pattern", path))
			}
		}
	}

	return errors
}

// validateArray validates an array value against its schema.
// It checks array items against the items schema if present and validates each element.
// Returns a slice of error messages for any validation failures encountered.
func (sv *SchemaValidator) validateArray(value any, schema map[string]any, path string) []string {
	var errors []string

	arrayValue, ok := value.([]any)
	if !ok {
		return errors
	}

	items, ok := schema["items"]
	if !ok {
		return errors
	}

	itemSchema, ok := items.(map[string]any)
	if !ok {
		return errors
	}

	for i, item := range arrayValue {
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		itemErrors := sv.validateValue(item, itemSchema, itemPath)
		errors = append(errors, itemErrors...)
	}

	return errors
}

// validateInteger checks if the provided value is an integer type according to JSON schema requirements.
// This method currently performs type validation only and is structured for future extension to support
// numeric constraints such as minimum, maximum, and multipleOf. It returns a slice of error messages
// for any validation failures encountered. The schema and path parameters are reserved for future use.
func (sv *SchemaValidator) validateInteger(value any, schema map[string]any, path string) []string {
	_ = schema
	_ = path
	var errors []string

	switch value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
	default:
	}

	return errors
}

// validateBoolean checks if the provided value is a boolean type according to JSON schema requirements.
// This method performs type validation only and is structured for future extension if needed.
// It returns a slice of error messages for any validation failures encountered.
func (sv *SchemaValidator) validateBoolean(value any, schema map[string]any, path string) []string {
	_ = schema
	_ = path
	var errors []string

	switch value.(type) {
	case bool:
	default:
	}

	return errors
}

// buildPath constructs a dot-notation path for error reporting
func (sv *SchemaValidator) buildPath(basePath, field string) string {
	if basePath == "" {
		return field
	}
	return basePath + "." + field
}

// getValueType returns the JSON schema type corresponding to the provided Go value.
// It maps Go types to JSON schema types: null, boolean, integer, number, string, array, object, or unknown.
func (sv *SchemaValidator) getValueType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "integer"
	case float32, float64:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "unknown"
	}
}

// valuesEqual compares two values for equality
func (sv *SchemaValidator) valuesEqual(a, b any) bool {
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// injectSubstitutionSchema injects the substitutions property schema into the provided schema.
// The substitutions property is always allowed regardless of the schema's additionalProperties setting.
// This enables users to define kustomization substitutions in values.yaml without schema conflicts.
func (sv *SchemaValidator) injectSubstitutionSchema(schema *map[string]any) {
	if schema == nil {
		return
	}

	properties, ok := (*schema)["properties"]
	if !ok {
		properties = make(map[string]any)
		(*schema)["properties"] = properties
	}

	propertiesMap, ok := properties.(map[string]any)
	if !ok {
		return
	}

	propertiesMap["substitutions"] = map[string]any{
		"type": "object",
		"additionalProperties": map[string]any{
			"type": "object",
			"additionalProperties": map[string]any{
				"type": "string",
			},
		},
	}
}

// validateSchemaStructure checks that the provided schema map conforms to Windsor or JSON Schema requirements.
// It verifies the presence of the '$schema' field and ensures the schema version is supported.
// Returns an error if the schema is missing required fields or uses an unsupported version.
func (sv *SchemaValidator) validateSchemaStructure(schema map[string]any) error {
	schemaVersion, ok := schema["$schema"]
	if !ok {
		return fmt.Errorf("missing required '$schema' field")
	}

	if schemaStr, ok := schemaVersion.(string); ok {
		if schemaStr != "https://schemas.windsorcli.dev/blueprint-config/v1alpha1" &&
			schemaStr != "https://json-schema.org/draft/2020-12/schema" {
			return fmt.Errorf("unsupported schema version: %s", schemaStr)
		}
	}

	return nil
}
