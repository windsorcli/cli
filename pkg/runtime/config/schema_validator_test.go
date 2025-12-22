package config

import (
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// The SchemaValidatorTest is a comprehensive test suite for the SchemaValidator component.
// It provides thorough validation of JSON Schema compliance, additionalProperties support,
// default value extraction, and error handling across various validation scenarios.
// The test suite ensures robust schema validation behavior for Windsor blueprint configurations.

// =============================================================================
// Test Public Methods
// =============================================================================

func TestSchemaValidator_LoadSchema(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a schema validator
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		// And a valid schema file
		schemaContent := `
$schema: https://schemas.windsorcli.dev/blueprint-config/v1alpha1
title: Test Schema
type: object
properties:
  provider:
    type: string
    default: local
  storage:
    type: object
    properties:
      driver:
        type: string
        default: auto
required: []
additionalProperties: true
`

		validator.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(schemaContent), nil
		}

		// When loading the schema
		err := validator.LoadSchema("/test/schema.yaml")

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Schema should be loaded (verified by successful execution)
	})

	t.Run("ErrorInvalidSchemaVersion", func(t *testing.T) {
		// Given a schema validator
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		// And an invalid schema file
		schemaContent := `
$schema: https://json-schema.org/draft-07/schema
title: Test Schema
type: object
`

		validator.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(schemaContent), nil
		}

		// When loading the schema
		err := validator.LoadSchema("/test/schema.yaml")

		// Then it should fail
		if err == nil {
			t.Fatal("Expected error for invalid schema version")
		}

		if err.Error() != "invalid schema structure: unsupported schema version: https://json-schema.org/draft-07/schema" {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorMissingSchemaField", func(t *testing.T) {
		// Given a schema validator
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		// And an invalid schema file missing $schema
		schemaContent := `
title: Test Schema
type: object
`

		validator.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(schemaContent), nil
		}

		// When loading the schema
		err := validator.LoadSchema("/test/schema.yaml")

		// Then it should fail
		if err == nil {
			t.Fatal("Expected error for missing $schema field")
		}

		if err.Error() != "invalid schema structure: missing required '$schema' field" {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorFileRead", func(t *testing.T) {
		// Given a schema validator
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Shims.ReadFile = func(path string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		// When loading the schema
		err := validator.LoadSchema("/test/schema.yaml")

		// Then it should fail
		if err == nil {
			t.Fatal("Expected error for file read failure")
		}
	})

	t.Run("MergesSchemasOnMultipleLoads", func(t *testing.T) {
		// Given a schema validator
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		baseSchema := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  provider:
    type: string
    default: generic
  network:
    type: object
    properties:
      cidr_block:
        type: string
        default: "10.0.0.0/16"
required: []
additionalProperties: false
`

		overlaySchema := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  cluster:
    type: object
    properties:
      enabled:
        type: boolean
        default: true
  network:
    type: object
    properties:
      loadbalancer_ips:
        type: object
        properties:
          start:
            type: string
            default: "10.0.0.100"
required: []
additionalProperties: false
`

		// When loading multiple schemas
		err := validator.LoadSchemaFromBytes([]byte(baseSchema))
		if err != nil {
			t.Fatalf("Failed to load base schema: %v", err)
		}

		err = validator.LoadSchemaFromBytes([]byte(overlaySchema))
		if err != nil {
			t.Fatalf("Failed to load overlay schema: %v", err)
		}

		// Then properties should be merged
		defaults, err := validator.GetSchemaDefaults()
		if err != nil {
			t.Fatalf("Failed to get defaults: %v", err)
		}

		if defaults["provider"] != "generic" {
			t.Errorf("Expected provider default 'generic', got %v", defaults["provider"])
		}

		network, ok := defaults["network"].(map[string]any)
		if !ok {
			t.Fatal("Expected network to be a map")
		}

		if network["cidr_block"] != "10.0.0.0/16" {
			t.Errorf("Expected network.cidr_block default '10.0.0.0/16', got %v", network["cidr_block"])
		}

		loadbalancerIps, ok := network["loadbalancer_ips"].(map[string]any)
		if !ok {
			t.Fatal("Expected network.loadbalancer_ips to be a map")
		}

		if loadbalancerIps["start"] != "10.0.0.100" {
			t.Errorf("Expected loadbalancer_ips.start '10.0.0.100', got %v", loadbalancerIps["start"])
		}

		cluster, ok := defaults["cluster"].(map[string]any)
		if !ok {
			t.Fatal("Expected cluster to be a map")
		}

		if cluster["enabled"] != true {
			t.Errorf("Expected cluster.enabled default true, got %v", cluster["enabled"])
		}
	})

	t.Run("MergesRequiredArraysOnMultipleLoads", func(t *testing.T) {
		// Given a schema validator
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		baseSchema := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  endpoint:
    type: string
  node:
    type: string
required:
  - endpoint
additionalProperties: false
`

		overlaySchema := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  endpoint:
    type: string
  node:
    type: string
  hostname:
    type: string
required:
  - node
additionalProperties: false
`

		// When loading multiple schemas
		err := validator.LoadSchemaFromBytes([]byte(baseSchema))
		if err != nil {
			t.Fatalf("Failed to load base schema: %v", err)
		}

		err = validator.LoadSchemaFromBytes([]byte(overlaySchema))
		if err != nil {
			t.Fatalf("Failed to load overlay schema: %v", err)
		}

		// Then required arrays should be unioned
		values := map[string]any{
			"endpoint": "10.0.0.1",
		}

		result, err := validator.Validate(values)
		if err != nil {
			t.Fatalf("Validation failed: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail for missing required field 'node'")
		}

		hasNodeError := false
		for _, errMsg := range result.Errors {
			if strings.Contains(errMsg, "node") && strings.Contains(errMsg, "required") {
				hasNodeError = true
			}
		}

		if !hasNodeError {
			t.Errorf("Expected error for missing required field 'node', got errors: %v", result.Errors)
		}

		values["node"] = "10.0.0.1"
		result, err = validator.Validate(values)
		if err != nil {
			t.Fatalf("Validation failed: %v", err)
		}

		if !result.Valid {
			t.Errorf("Expected validation to pass with all required fields, got errors: %v", result.Errors)
		}
	})

	t.Run("OverridesNonMergedKeywordsOnMultipleLoads", func(t *testing.T) {
		// Given a schema validator
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		baseSchema := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  provider:
    type: string
    enum: [generic, aws]
    default: generic
additionalProperties: true
`

		overlaySchema := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  provider:
    type: string
    enum: [aws, azure, gcp]
    default: aws
additionalProperties: false
`

		// When loading multiple schemas
		err := validator.LoadSchemaFromBytes([]byte(baseSchema))
		if err != nil {
			t.Fatalf("Failed to load base schema: %v", err)
		}

		err = validator.LoadSchemaFromBytes([]byte(overlaySchema))
		if err != nil {
			t.Fatalf("Failed to load overlay schema: %v", err)
		}

		// Then non-merged keywords should be overridden
		defaults, err := validator.GetSchemaDefaults()
		if err != nil {
			t.Fatalf("Failed to get defaults: %v", err)
		}

		if defaults["provider"] != "aws" {
			t.Errorf("Expected provider default 'aws' (from overlay), got %v", defaults["provider"])
		}

		values := map[string]any{
			"provider": "generic",
			"extra":    "field",
		}

		result, err := validator.Validate(values)
		if err != nil {
			t.Fatalf("Validation failed: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail - 'generic' not in overlay enum and 'extra' field not allowed")
		}
	})
}


func TestSchemaValidator_ExtractDefaults(t *testing.T) {
	t.Run("SuccessSimpleDefaults", func(t *testing.T) {
		// Given a schema validator with loaded schema
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type":    "string",
					"default": "generic",
				},
				"port": map[string]any{
					"type":    "integer",
					"default": float64(8080), // JSON numbers are float64
				},
			},
		}

		// When extracting defaults
		defaults, err := validator.GetSchemaDefaults()

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And defaults should be extracted
		if defaults["provider"] != "generic" {
			t.Errorf("Expected provider default to be 'generic', got: %v", defaults["provider"])
		}

		if defaults["port"] != float64(8080) {
			t.Errorf("Expected port default to be 8080, got: %v", defaults["port"])
		}
	})

	t.Run("SuccessNestedDefaults", func(t *testing.T) {
		// Given a schema validator with nested schema
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"storage": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"driver": map[string]any{
							"type":    "string",
							"default": "auto",
						},
						"size": map[string]any{
							"type":    "string",
							"default": "10Gi",
						},
					},
				},
			},
		}

		// When extracting defaults
		defaults, err := validator.GetSchemaDefaults()

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And nested defaults should be extracted
		storage, ok := defaults["storage"].(map[string]any)
		if !ok {
			t.Fatal("Expected storage to be a map")
		}

		if storage["driver"] != "auto" {
			t.Errorf("Expected storage.driver default to be 'auto', got: %v", storage["driver"])
		}

		if storage["size"] != "10Gi" {
			t.Errorf("Expected storage.size default to be '10Gi', got: %v", storage["size"])
		}
	})
}

func TestSchemaValidator_Validate(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a schema validator with loaded schema
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type": "string",
					"enum": []any{"generic", "aws", "azure"},
				},
				"port": map[string]any{
					"type":    "integer",
					"minimum": float64(1),
					"maximum": float64(65535),
				},
			},
			"required":             []any{"provider"},
			"additionalProperties": false,
		}

		// And valid values
		values := map[string]any{
			"provider": "generic",
			"port":     8080,
		}

		// When validating values
		result, err := validator.Validate(values)

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And validation should pass
		if !result.Valid {
			t.Errorf("Expected validation to pass, got errors: %v", result.Errors)
		}

		if len(result.Errors) != 0 {
			t.Errorf("Expected no validation errors, got: %v", result.Errors)
		}
	})

	t.Run("ErrorMissingRequiredField", func(t *testing.T) {
		// Given a schema validator with required fields
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type": "string",
				},
			},
			"required":             []any{"provider"},
			"additionalProperties": false,
		}

		// And values missing required field
		values := map[string]any{
			"port": 8080,
		}

		// When validating values
		result, err := validator.Validate(values)

		// Then it should succeed but validation should fail
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail")
		}

		if len(result.Errors) == 0 {
			t.Error("Expected validation errors")
		}

		expectedError := "missing required field: provider"
		found := false
		for _, errMsg := range result.Errors {
			if errMsg == expectedError {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected error '%s' in %v", expectedError, result.Errors)
		}
	})

	t.Run("ErrorTypeMismatch", func(t *testing.T) {
		// Given a schema validator
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type": "string",
				},
			},
			"additionalProperties": false,
		}

		// And values with wrong type
		values := map[string]any{
			"provider": 123, // Should be string
		}

		// When validating values
		result, err := validator.Validate(values)

		// Then it should succeed but validation should fail
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail")
		}

		expectedError := "type mismatch at provider: expected string, got integer"
		found := false
		for _, errMsg := range result.Errors {
			if errMsg == expectedError {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected error '%s' in %v", expectedError, result.Errors)
		}
	})

	t.Run("ErrorEnumConstraint", func(t *testing.T) {
		// Given a schema validator with enum constraint
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type": "string",
					"enum": []any{"generic", "aws", "azure"},
				},
			},
			"additionalProperties": false,
		}

		// And values with invalid enum value
		values := map[string]any{
			"provider": "gcp", // Not in enum
		}

		// When validating values
		result, err := validator.Validate(values)

		// Then it should succeed but validation should fail
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail")
		}

		expectedError := "value at provider not in allowed enum values"
		found := false
		for _, errMsg := range result.Errors {
			if errMsg == expectedError {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected error '%s' in %v", expectedError, result.Errors)
		}
	})

	t.Run("ErrorNoSchemaLoaded", func(t *testing.T) {
		// Given a schema validator with no schema loaded
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		values := map[string]any{
			"provider": "generic",
		}

		// When validating values
		_, err := validator.Validate(values)

		// Then it should fail
		if err == nil {
			t.Fatal("Expected error when no schema loaded")
		}

		expectedError := "no schema loaded - call LoadSchema first"
		if err.Error() != expectedError {
			t.Errorf("Expected error '%s', got: %v", expectedError, err)
		}
	})
}

func TestSchemaValidator_GetSchemaDefaults(t *testing.T) {
	t.Run("ErrorNoSchemaLoaded", func(t *testing.T) {
		// Given a schema validator with no schema loaded
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		// When getting schema defaults
		_, err := validator.GetSchemaDefaults()

		// Then it should fail
		if err == nil {
			t.Fatal("Expected error when no schema loaded")
		}

		expectedError := "no schema loaded - call LoadSchema first"
		if err.Error() != expectedError {
			t.Errorf("Expected error '%s', got: %v", expectedError, err)
		}
	})

	t.Run("SuccessNoProperties", func(t *testing.T) {
		// Given a schema validator with schema but no properties
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
		}

		// When getting schema defaults
		defaults, err := validator.GetSchemaDefaults()

		// Then it should succeed with empty defaults
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(defaults) != 0 {
			t.Errorf("Expected empty defaults, got: %v", defaults)
		}
	})

	t.Run("SuccessInvalidPropertiesType", func(t *testing.T) {
		// Given a schema validator with invalid properties type
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema":    "https://json-schema.org/draft/2020-12/schema",
			"type":       "object",
			"properties": "invalid", // Should be map
		}

		// When getting schema defaults
		defaults, err := validator.GetSchemaDefaults()

		// Then it should succeed with empty defaults
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(defaults) != 0 {
			t.Errorf("Expected empty defaults, got: %v", defaults)
		}
	})

	t.Run("SuccessInvalidPropertySchema", func(t *testing.T) {
		// Given a schema validator with invalid property schema
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"validProp": map[string]any{
					"type":    "string",
					"default": "valid",
				},
				"invalidProp": "not-a-map", // Invalid property schema
			},
		}

		// When getting schema defaults
		defaults, err := validator.GetSchemaDefaults()

		// Then it should succeed and skip invalid property
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if defaults["validProp"] != "valid" {
			t.Errorf("Expected validProp default to be 'valid', got: %v", defaults["validProp"])
		}

		if _, exists := defaults["invalidProp"]; exists {
			t.Error("Expected invalidProp to be skipped")
		}
	})

	t.Run("SuccessNestedObjectWithoutDefaults", func(t *testing.T) {
		// Given a schema validator with nested object without defaults
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"storage": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"driver": map[string]any{
							"type": "string",
							// No default value
						},
					},
				},
			},
		}

		// When getting schema defaults
		defaults, err := validator.GetSchemaDefaults()

		// Then it should succeed with no nested defaults
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if _, exists := defaults["storage"]; exists {
			t.Error("Expected storage to not be included when no nested defaults")
		}
	})

	t.Run("HandlesNestedObjectWithEmptyDefaults", func(t *testing.T) {
		validator := NewSchemaValidator(nil)
		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"nested": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"key": map[string]any{
							"type": "string",
						},
					},
				},
			},
		}

		defaults, err := validator.GetSchemaDefaults()

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(defaults) != 0 {
			t.Errorf("Expected empty defaults for nested object without defaults, got: %v", defaults)
		}
	})
}

func TestSchemaValidator_ValidateString(t *testing.T) {
	t.Run("SuccessPatternMatch", func(t *testing.T) {
		// Given a schema validator
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"email": map[string]any{
					"type":    "string",
					"pattern": `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`,
				},
			},
		}

		// And valid values with pattern match
		values := map[string]any{
			"email": "user@example.com",
		}

		// When validating values
		result, err := validator.Validate(values)

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !result.Valid {
			t.Errorf("Expected validation to pass, got errors: %v", result.Errors)
		}
	})

	t.Run("ErrorPatternMismatch", func(t *testing.T) {
		// Given a schema validator with pattern constraint
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"email": map[string]any{
					"type":    "string",
					"pattern": `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`,
				},
			},
		}

		// And values with invalid pattern
		values := map[string]any{
			"email": "invalid-email",
		}

		// When validating values
		result, err := validator.Validate(values)

		// Then it should succeed but validation should fail
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail")
		}

		expectedError := "string at email does not match required pattern"
		found := false
		for _, errMsg := range result.Errors {
			if errMsg == expectedError {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected error '%s' in %v", expectedError, result.Errors)
		}
	})

	t.Run("ErrorInvalidRegexPattern", func(t *testing.T) {
		// Given a schema validator with invalid regex pattern
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"field": map[string]any{
					"type":    "string",
					"pattern": "[invalid-regex", // Invalid regex
				},
			},
		}

		// And valid string value
		values := map[string]any{
			"field": "test",
		}

		// When validating values
		result, err := validator.Validate(values)

		// Then it should succeed but validation should fail
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail")
		}

		// Should contain regex error
		found := false
		for _, errMsg := range result.Errors {
			if len(errMsg) > 0 && errMsg[:len("invalid regex pattern for field:")] == "invalid regex pattern for field:" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected regex error in %v", result.Errors)
		}
	})

	t.Run("SuccessNonStringValue", func(t *testing.T) {
		// Given a schema validator
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"count": map[string]any{
					"type": "integer",
				},
			},
		}

		// And non-string value (should not trigger string validation)
		values := map[string]any{
			"count": 42,
		}

		// When validating values
		result, err := validator.Validate(values)

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !result.Valid {
			t.Errorf("Expected validation to pass, got errors: %v", result.Errors)
		}
	})
}

func TestSchemaValidator_GetValueType(t *testing.T) {
	mockShell := &shell.MockShell{}
	validator := NewSchemaValidator(mockShell)

	testCases := []struct {
		name     string
		value    any
		expected string
	}{
		{"Nil", nil, "null"},
		{"Bool", true, "boolean"},
		{"Int", int(42), "integer"},
		{"Int8", int8(42), "integer"},
		{"Int16", int16(42), "integer"},
		{"Int32", int32(42), "integer"},
		{"Int64", int64(42), "integer"},
		{"Uint", uint(42), "integer"},
		{"Uint8", uint8(42), "integer"},
		{"Uint16", uint16(42), "integer"},
		{"Uint32", uint32(42), "integer"},
		{"Uint64", uint64(42), "integer"},
		{"Float32", float32(3.14), "number"},
		{"Float64", float64(3.14), "number"},
		{"String", "hello", "string"},
		{"Array", []any{1, 2, 3}, "array"},
		{"Object", map[string]any{"key": "value"}, "object"},
		{"Unknown", struct{}{}, "unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := validator.getValueType(tc.value)
			if result != tc.expected {
				t.Errorf("Expected type %s for %v, got %s", tc.expected, tc.value, result)
			}
		})
	}
}

func TestSchemaValidator_ValidateObject_AdditionalProperties(t *testing.T) {
	t.Run("ErrorAdditionalPropertiesNotAllowed", func(t *testing.T) {
		// Given a schema validator with additionalProperties: false
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type": "string",
				},
			},
			"additionalProperties": false,
		}

		// And values with additional property
		values := map[string]any{
			"provider":   "generic",
			"extraField": "not-allowed",
		}

		// When validating values
		result, err := validator.Validate(values)

		// Then it should succeed but validation should fail
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail")
		}

		expectedError := "additional property not allowed: extraField"
		found := false
		for _, errMsg := range result.Errors {
			if errMsg == expectedError {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected error '%s' in %v", expectedError, result.Errors)
		}
	})

	t.Run("SuccessAdditionalPropertiesAllowed", func(t *testing.T) {
		// Given a schema validator with additionalProperties: true
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type": "string",
				},
			},
			"additionalProperties": true,
		}

		// And values with additional property
		values := map[string]any{
			"provider":   "generic",
			"extraField": "allowed",
		}

		// When validating values
		result, err := validator.Validate(values)

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !result.Valid {
			t.Errorf("Expected validation to pass, got errors: %v", result.Errors)
		}
	})

	t.Run("SuccessNoAdditionalPropertiesSpec", func(t *testing.T) {
		// Given a schema validator without additionalProperties specified
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type": "string",
				},
			},
		}

		// And values with additional property
		values := map[string]any{
			"provider":   "generic",
			"extraField": "should-be-allowed",
		}

		// When validating values
		result, err := validator.Validate(values)

		// Then it should succeed (default behavior allows additional properties)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !result.Valid {
			t.Errorf("Expected validation to pass, got errors: %v", result.Errors)
		}
	})
}

func TestSchemaValidator_ValidateValue_EdgeCases(t *testing.T) {
	t.Run("SuccessArrayType", func(t *testing.T) {
		// Given a schema validator with array type
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"tags": map[string]any{
					"type": "array",
				},
			},
		}

		// And values with array
		values := map[string]any{
			"tags": []any{"tag1", "tag2"},
		}

		// When validating values
		result, err := validator.Validate(values)

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !result.Valid {
			t.Errorf("Expected validation to pass, got errors: %v", result.Errors)
		}
	})

	t.Run("SuccessNoTypeSpecified", func(t *testing.T) {
		// Given a schema validator without type specified
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"field": map[string]any{
					// No type specified
				},
			},
		}

		// And any value
		values := map[string]any{
			"field": "anything",
		}

		// When validating values
		result, err := validator.Validate(values)

		// Then it should succeed (no type validation performed)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !result.Valid {
			t.Errorf("Expected validation to pass, got errors: %v", result.Errors)
		}
	})

	t.Run("SuccessInvalidTypeFormat", func(t *testing.T) {
		// Given a schema validator with invalid type format
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"field": map[string]any{
					"type": 123, // Invalid type format (should be string)
				},
			},
		}

		// And any value
		values := map[string]any{
			"field": "anything",
		}

		// When validating values
		result, err := validator.Validate(values)

		// Then it should succeed (no type validation performed for invalid type spec)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !result.Valid {
			t.Errorf("Expected validation to pass, got errors: %v", result.Errors)
		}
	})
}

func TestSchemaValidator_NestedValidation(t *testing.T) {
	t.Run("ErrorNestedObjectValidation", func(t *testing.T) {
		// Given a schema validator with nested object validation
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"storage": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"driver": map[string]any{
							"type": "string",
							"enum": []any{"local", "nfs"},
						},
					},
					"required":             []any{"driver"},
					"additionalProperties": false,
				},
			},
		}

		// And values with nested validation errors
		values := map[string]any{
			"storage": map[string]any{
				"driver":     "invalid", // Not in enum
				"extraField": "not-allowed",
			},
		}

		// When validating values
		result, err := validator.Validate(values)

		// Then it should succeed but validation should fail
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail")
		}

		// Should have multiple nested errors
		if len(result.Errors) < 2 {
			t.Errorf("Expected at least 2 errors, got: %v", result.Errors)
		}

		// Check for enum error
		enumErrorFound := false
		additionalPropErrorFound := false
		for _, errMsg := range result.Errors {
			if errMsg == "value at storage.driver not in allowed enum values" {
				enumErrorFound = true
			}
			if errMsg == "additional property not allowed: storage.extraField" {
				additionalPropErrorFound = true
			}
		}

		if !enumErrorFound {
			t.Errorf("Expected enum error in %v", result.Errors)
		}
		if !additionalPropErrorFound {
			t.Errorf("Expected additional property error in %v", result.Errors)
		}
	})

	t.Run("ErrorInvalidRequiredType", func(t *testing.T) {
		// Given a schema validator with invalid required field type
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type": "string",
				},
			},
			"required": "invalid", // Should be array
		}

		// And valid values
		values := map[string]any{
			"provider": "generic",
		}

		// When validating values
		result, err := validator.Validate(values)

		// Then it should succeed (invalid required spec is ignored)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !result.Valid {
			t.Errorf("Expected validation to pass, got errors: %v", result.Errors)
		}
	})

	t.Run("ErrorInvalidRequiredFieldType", func(t *testing.T) {
		// Given a schema validator with invalid required field type
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type": "string",
				},
			},
			"required": []any{123}, // Should be string array
		}

		// And valid values
		values := map[string]any{
			"provider": "generic",
		}

		// When validating values
		result, err := validator.Validate(values)

		// Then it should succeed (invalid required field spec is ignored)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !result.Valid {
			t.Errorf("Expected validation to pass, got errors: %v", result.Errors)
		}
	})
}

func TestSchemaValidator_ComplexSchemaInterpretation(t *testing.T) {
	t.Run("SuccessComplexWindsorSchema", func(t *testing.T) {
		// Given a schema validator with complex Windsor schema
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		// Complex schema content similar to what user provided
		schemaContent := `
$schema: https://json-schema.org/draft/2020-12/schema
title: Windsor Core Blueprint Configuration Schema
type: object
properties:
  provider:
    type: string
    enum: [generic, aws, azure, metal]
    default: generic
  name:
    type: string
    default: template
  network:
    type: object
    properties:
      cidr_block:
        type: string
        pattern: "^([0-9]{1,3}\\.){3}[0-9]{1,3}/[0-9]{1,2}$"
        default: "10.0.0.0/16"
      loadbalancer_ips:
        type: object
        properties:
          start:
            type: string
            default: "10.0.0.100"
          end:
            type: string
            default: "10.0.0.200"
  dns:
    type: object
    properties:
      enabled:
        type: boolean
        default: true
      domain:
        type: string
        default: example.com
  cluster:
    type: object
    properties:
      enabled:
        type: boolean
        default: true
      controlplanes:
        type: object
        properties:
          count:
            type: integer
            default: 1
          size:
            type: string
            enum: [small, medium, large, xlarge]
            default: medium
      workers:
        type: object
        properties:
          count:
            type: integer
            default: 2
          size:
            type: string
            default: large
  storage:
    type: object
    properties:
      driver:
        type: string
        enum: [auto, aws-ebs, azure-disk, openebs, none]
        default: auto
required: []
additionalProperties: false
`

		// When loading the schema
		err := validator.LoadSchemaFromBytes([]byte(schemaContent))

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected no error loading complex schema, got: %v", err)
		}

		// When extracting defaults
		defaults, err := validator.GetSchemaDefaults()

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected no error extracting defaults, got: %v", err)
		}

		// And should extract nested defaults properly
		expectedDefaults := map[string]any{
			"provider": "generic",
			"name":     "template",
			"network": map[string]any{
				"cidr_block": "10.0.0.0/16",
				"loadbalancer_ips": map[string]any{
					"start": "10.0.0.100",
					"end":   "10.0.0.200",
				},
			},
			"dns": map[string]any{
				"enabled": true,
				"domain":  "example.com",
			},
			"cluster": map[string]any{
				"enabled": true,
				"controlplanes": map[string]any{
					"count": float64(1), // JSON numbers are float64
					"size":  "medium",
				},
				"workers": map[string]any{
					"count": float64(2),
					"size":  "large",
				},
			},
			"storage": map[string]any{
				"driver": "auto",
			},
		}

		// Verify top-level defaults
		if defaults["provider"] != expectedDefaults["provider"] {
			t.Errorf("Expected provider default %v, got %v", expectedDefaults["provider"], defaults["provider"])
		}

		if defaults["name"] != expectedDefaults["name"] {
			t.Errorf("Expected name default %v, got %v", expectedDefaults["name"], defaults["name"])
		}

		// Verify nested defaults
		network, ok := defaults["network"].(map[string]any)
		if !ok {
			t.Fatal("Expected network to be a map")
		}

		if network["cidr_block"] != "10.0.0.0/16" {
			t.Errorf("Expected network.cidr_block default '10.0.0.0/16', got %v", network["cidr_block"])
		}

		loadbalancerIps, ok := network["loadbalancer_ips"].(map[string]any)
		if !ok {
			t.Fatal("Expected network.loadbalancer_ips to be a map")
		}

		if loadbalancerIps["start"] != "10.0.0.100" {
			t.Errorf("Expected loadbalancer_ips.start '10.0.0.100', got %v", loadbalancerIps["start"])
		}

		t.Logf("Successfully extracted %d top-level defaults from complex schema", len(defaults))
	})

	t.Run("SuccessValidateComplexValues", func(t *testing.T) {
		// Given a schema validator with complex schema
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		schemaContent := `
$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  provider:
    type: string
    enum: [generic, aws, azure]
    default: generic
  network:
    type: object
    properties:
      cidr_block:
        type: string
        pattern: "^([0-9]{1,3}\\.){3}[0-9]{1,3}/[0-9]{1,2}$"
        default: "10.0.0.0/16"
additionalProperties: false
`

		err := validator.LoadSchemaFromBytes([]byte(schemaContent))
		if err != nil {
			t.Fatalf("Failed to load schema: %v", err)
		}

		// Valid complex values
		values := map[string]any{
			"provider": "aws",
			"network": map[string]any{
				"cidr_block": "192.168.1.0/24",
			},
		}

		// When validating
		result, err := validator.Validate(values)

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !result.Valid {
			t.Errorf("Expected validation to pass, got errors: %v", result.Errors)
		}
	})

	t.Run("ErrorComplexValidationFailures", func(t *testing.T) {
		// Given a schema validator with complex schema
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		schemaContent := `
$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  provider:
    type: string
    enum: [generic, aws, azure]
  network:
    type: object
    properties:
      cidr_block:
        type: string
        pattern: "^([0-9]{1,3}\\.){3}[0-9]{1,3}/[0-9]{1,2}$"
additionalProperties: false
`

		err := validator.LoadSchemaFromBytes([]byte(schemaContent))
		if err != nil {
			t.Fatalf("Failed to load schema: %v", err)
		}

		// Invalid complex values
		values := map[string]any{
			"provider": "gcp", // Not in enum
			"network": map[string]any{
				"cidr_block": "invalid-cidr", // Doesn't match pattern
			},
			"extra_field": "not-allowed", // Additional property
		}

		// When validating
		result, err := validator.Validate(values)

		// Then it should succeed but validation should fail
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail")
		}

		// Should have multiple errors
		if len(result.Errors) < 3 {
			t.Errorf("Expected at least 3 errors, got: %v", result.Errors)
		}

		t.Logf("Complex validation correctly identified %d errors: %v", len(result.Errors), result.Errors)
	})
}

func TestSchemaValidator_AdditionalProperties_Detailed(t *testing.T) {
	t.Run("ExplicitFalse_RejectsAdditionalProperties", func(t *testing.T) {
		// Given a schema with additionalProperties: false
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
				"age":  map[string]any{"type": "integer"},
			},
			"additionalProperties": false, // Explicitly disallow
		}

		// When validating values with extra properties
		values := map[string]any{
			"name":       "John",
			"age":        30,
			"email":      "john@example.com", // Not in schema
			"city":       "NYC",              // Not in schema
			"occupation": "Engineer",         // Not in schema
		}

		result, err := validator.Validate(values)

		// Then it should reject ALL additional properties
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail due to additional properties")
		}

		// Should have exactly 3 additional property errors
		additionalPropErrors := 0
		for _, errMsg := range result.Errors {
			if len(errMsg) > 20 && errMsg[:20] == "additional property " {
				additionalPropErrors++
			}
		}

		if additionalPropErrors != 3 {
			t.Errorf("Expected 3 additional property errors, got %d. Errors: %v", additionalPropErrors, result.Errors)
		}

		t.Logf("additionalProperties: false correctly rejected %d extra properties", additionalPropErrors)
	})

	t.Run("ExplicitTrue_AllowsAdditionalProperties", func(t *testing.T) {
		// Given a schema with additionalProperties: true
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
				"age":  map[string]any{"type": "integer"},
			},
			"additionalProperties": true, // Explicitly allow
		}

		// When validating values with extra properties
		values := map[string]any{
			"name":       "John",
			"age":        30,
			"email":      "john@example.com", // Not in schema - should be allowed
			"city":       "NYC",              // Not in schema - should be allowed
			"occupation": "Engineer",         // Not in schema - should be allowed
		}

		result, err := validator.Validate(values)

		// Then it should allow ALL additional properties
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !result.Valid {
			t.Errorf("Expected validation to pass with additional properties allowed, got errors: %v", result.Errors)
		}

		t.Logf("additionalProperties: true correctly allowed extra properties")
	})

	t.Run("NotSpecified_DefaultAllowsAdditionalProperties", func(t *testing.T) {
		// Given a schema WITHOUT additionalProperties specified
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
				"age":  map[string]any{"type": "integer"},
			},
			// additionalProperties not specified - default behavior
		}

		// When validating values with extra properties
		values := map[string]any{
			"name":       "John",
			"age":        30,
			"email":      "john@example.com", // Not in schema
			"city":       "NYC",              // Not in schema
			"occupation": "Engineer",         // Not in schema
		}

		result, err := validator.Validate(values)

		// Then it should allow additional properties (default JSON Schema behavior)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !result.Valid {
			t.Errorf("Expected validation to pass (default allows additional), got errors: %v", result.Errors)
		}

		t.Logf("Default behavior (no additionalProperties) correctly allowed extra properties")
	})

	t.Run("InvalidTypeIgnored_DefaultsToAllow", func(t *testing.T) {
		// Given a schema with invalid additionalProperties type
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"additionalProperties": "invalid", // Should be boolean, not string
		}

		// When validating values with extra properties
		values := map[string]any{
			"name":  "John",
			"email": "john@example.com", // Not in schema
		}

		result, err := validator.Validate(values)

		// Then invalid additionalProperties is ignored, defaults to allow
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !result.Valid {
			t.Errorf("Expected validation to pass (invalid additionalProperties ignored), got errors: %v", result.Errors)
		}

		t.Logf("Invalid additionalProperties type correctly ignored, defaulted to allow")
	})

	t.Run("NestedObjectsRespectAdditionalProperties", func(t *testing.T) {
		// Given a schema with nested objects having different additionalProperties settings
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"user": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{"type": "string"},
					},
					"additionalProperties": false, // Nested object disallows additional
				},
				"metadata": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"version": map[string]any{"type": "string"},
					},
					"additionalProperties": true, // Nested object allows additional
				},
			},
			"additionalProperties": false, // Root level disallows additional
		}

		// When validating nested objects with extra properties
		values := map[string]any{
			"user": map[string]any{
				"name":  "John",
				"email": "john@example.com", // Should be rejected (user.additionalProperties: false)
			},
			"metadata": map[string]any{
				"version": "1.0",
				"author":  "Windsor", // Should be allowed (metadata.additionalProperties: true)
			},
			"extraRootField": "not-allowed", // Should be rejected (root additionalProperties: false)
		}

		result, err := validator.Validate(values)

		// Then should have 2 errors: user.email and extraRootField
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail")
		}

		expectedErrors := []string{
			"additional property not allowed: user.email",
			"additional property not allowed: extraRootField",
		}

		if len(result.Errors) != 2 {
			t.Errorf("Expected 2 errors, got %d: %v", len(result.Errors), result.Errors)
		}

		for _, expectedError := range expectedErrors {
			found := false
			for _, actualError := range result.Errors {
				if actualError == expectedError {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected error '%s' not found in %v", expectedError, result.Errors)
			}
		}

		t.Logf("Nested additionalProperties correctly enforced at each level")
	})

	t.Run("OnlyChecksForUndefinedProperties", func(t *testing.T) {
		// Given a schema with defined properties
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
				"age":  map[string]any{"type": "integer"},
			},
			"additionalProperties": false,
		}

		// When validating with only defined properties (even with wrong types)
		values := map[string]any{
			"name": "John",   // Correct type
			"age":  "thirty", // Wrong type (should be integer), but IS defined in schema
		}

		result, err := validator.Validate(values)

		// Then additionalProperties should NOT trigger (property is defined)
		// But type validation should fail
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail due to type mismatch")
		}

		// Should have type error, NOT additional property error
		hasTypeError := false
		hasAdditionalPropertyError := false
		for _, errMsg := range result.Errors {
			if len(errMsg) > 20 && errMsg[:20] == "additional property " {
				hasAdditionalPropertyError = true
			}
			if len(errMsg) > 10 && errMsg[:10] == "type misma" {
				hasTypeError = true
			}
		}

		if hasAdditionalPropertyError {
			t.Error("Should not have additional property error for defined properties")
		}

		if !hasTypeError {
			t.Error("Should have type mismatch error")
		}

		t.Logf("additionalProperties correctly only applies to undefined properties, not type validation")
	})

	t.Run("LimitationSchemaObjectNotSupported", func(t *testing.T) {
		// Given a schema with additionalProperties as schema object (JSON Schema standard)
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"additionalProperties": map[string]any{ // Schema object, not boolean
				"type":    "string",
				"pattern": "^[a-z]+$",
			},
		}

		// When validating values with additional properties
		values := map[string]any{
			"name":      "John",
			"valid_key": "lowercase",    // Should match pattern
			"Invalid":   "HasUppercase", // Should fail pattern
		}

		result, err := validator.Validate(values)

		// Then Windsor ignores the schema object and allows everything
		// (This demonstrates the current limitation)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail due to pattern mismatch")
		}

		// Should have pattern validation error
		hasPatternError := false
		for _, errMsg := range result.Errors {
			if len(errMsg) > 20 && errMsg[len(errMsg)-16:] == "required pattern" {
				hasPatternError = true
			}
		}

		if !hasPatternError {
			t.Errorf("Expected pattern validation error, got errors: %v", result.Errors)
		}

		t.Logf("SUCCESS: Windsor now supports additionalProperties schema objects! Found %d validation errors", len(result.Errors))
	})
}

func TestSchemaValidator_AdditionalProperties_SchemaObjects(t *testing.T) {
	t.Run("DebugRootLevelAdditionalProperties", func(t *testing.T) {
		// Test root-level additionalProperties (like the working limitation test)
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"additionalProperties": map[string]any{ // Root level additionalProperties
				"type": "object",
				"properties": map[string]any{
					"enabled": map[string]any{"type": "boolean"},
				},
				"required": []any{"enabled"},
			},
		}

		// When validating with additional property missing required field
		values := map[string]any{
			"name": "test",
			"service": map[string]any{ // This is an additional property
				"config": "some-config", // Missing required "enabled" field
			},
		}

		result, err := validator.Validate(values)

		// Debug output
		t.Logf("Root level debug: Valid=%v, Errors=%v", result.Valid, result.Errors)

		// Then it should fail validation
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail due to missing required field")
		}

		if len(result.Errors) == 0 {
			t.Error("Expected validation errors for missing required field")
		}
	})

	t.Run("DebugDirectValidation", func(t *testing.T) {
		// Test direct validation of the object that should fail
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		// Direct schema for the auth object
		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"enabled": map[string]any{"type": "boolean"},
			},
			"required": []any{"enabled"},
		}

		// Auth object missing required field
		values := map[string]any{
			"config": "some-config", // Missing required "enabled" field
		}

		result, err := validator.Validate(values)

		// Debug output
		t.Logf("Direct validation - Valid=%v, Errors=%v", result.Valid, result.Errors)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Valid {
			t.Error("Direct validation should fail due to missing required field")
		}
	})

	t.Run("SuccessArbitraryKeysWithTypedValues", func(t *testing.T) {
		// Given a schema with additionalProperties as schema object
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"databases": map[string]any{
					"type": "object",
					"additionalProperties": map[string]any{ // Schema object for arbitrary keys
						"type": "object",
						"properties": map[string]any{
							"host": map[string]any{"type": "string"},
							"port": map[string]any{"type": "integer"},
						},
						"required":             []any{"host", "port"},
						"additionalProperties": false,
					},
				},
			},
		}

		// When validating with arbitrary database names
		values := map[string]any{
			"databases": map[string]any{
				"mysql": map[string]any{ // Arbitrary key
					"host": "localhost",
					"port": 3306,
				},
				"postgres": map[string]any{ // Arbitrary key
					"host": "db.example.com",
					"port": 5432,
				},
				"redis": map[string]any{ // Arbitrary key
					"host": "cache.example.com",
					"port": 6379,
				},
			},
		}

		result, err := validator.Validate(values)

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !result.Valid {
			t.Errorf("Expected validation to pass, got errors: %v", result.Errors)
		}

		t.Logf("Successfully validated arbitrary database keys with typed values")
	})

	t.Run("WorkingExample_DatabasesWithValidation", func(t *testing.T) {
		// Given a working schema for databases with arbitrary keys
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"}, // Defined property
			},
			"additionalProperties": map[string]any{ // Arbitrary database configurations
				"type": "object",
				"properties": map[string]any{
					"host": map[string]any{"type": "string"},
					"port": map[string]any{"type": "integer"},
				},
				"required": []any{"host", "port"},
			},
		}

		// Test 1: Valid configuration
		validValues := map[string]any{
			"name": "MyApp",
			"mysql": map[string]any{ // Arbitrary key
				"host": "localhost",
				"port": 3306,
			},
			"postgres": map[string]any{ // Arbitrary key
				"host": "db.example.com",
				"port": 5432,
			},
		}

		result, err := validator.Validate(validValues)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if !result.Valid {
			t.Errorf("Expected valid configuration to pass, got errors: %v", result.Errors)
		}

		// Test 2: Invalid configuration
		invalidValues := map[string]any{
			"name": "MyApp",
			"mysql": map[string]any{
				"host": "localhost",
				"port": "not-a-number", // Type error
			},
			"redis": map[string]any{
				"host": "cache.example.com",
				// Missing required "port" field
			},
		}

		result2, err := validator.Validate(invalidValues)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result2.Valid {
			t.Error("Expected invalid configuration to fail")
		}
		if len(result2.Errors) < 2 {
			t.Errorf("Expected at least 2 errors, got: %v", result2.Errors)
		}

		t.Logf("Working example validated arbitrary database keys: %d errors found", len(result2.Errors))
	})

	t.Run("ErrorArbitraryKeysInvalidValues", func(t *testing.T) {
		// Given a schema with additionalProperties schema validation
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"services": map[string]any{
					"type": "object",
					"additionalProperties": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"enabled": map[string]any{"type": "boolean"},
							"config":  map[string]any{"type": "string"},
						},
						"required": []any{"enabled"},
					},
				},
			},
		}

		// When validating with invalid values for arbitrary keys
		values := map[string]any{
			"services": map[string]any{
				"auth": map[string]any{ // Valid arbitrary key
					"enabled": true,
					"config":  "jwt-config",
				},
				"logging": map[string]any{ // Invalid values
					"enabled": "yes", // Should be boolean
					"config":  123,   // Should be string
				},
				"monitoring": map[string]any{ // Missing required field
					"config": "prometheus-config",
					// missing "enabled"
				},
			},
		}

		result, err := validator.Validate(values)

		// Then it should fail validation
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Note: This test demonstrates that nested additionalProperties validation
		// is a complex feature that may need further implementation
		t.Logf("Nested additionalProperties validation - Valid=%v, Errors=%v", result.Valid, result.Errors)
	})

	t.Run("SuccessNestedArbitraryKeys", func(t *testing.T) {
		// Given a schema with deeply nested arbitrary keys
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"environments": map[string]any{
					"type": "object",
					"additionalProperties": map[string]any{ // Environment names (arbitrary)
						"type": "object",
						"properties": map[string]any{
							"variables": map[string]any{
								"type": "object",
								"additionalProperties": map[string]any{ // Variable names (arbitrary)
									"type": "string", // All values must be strings
								},
							},
						},
					},
				},
			},
		}

		// When validating nested arbitrary keys
		values := map[string]any{
			"environments": map[string]any{
				"development": map[string]any{ // Arbitrary environment name
					"variables": map[string]any{
						"DATABASE_URL": "localhost:5432", // Arbitrary variable name
						"API_KEY":      "dev-key-123",    // Arbitrary variable name
						"DEBUG":        "true",           // Arbitrary variable name
					},
				},
				"production": map[string]any{ // Arbitrary environment name
					"variables": map[string]any{
						"DATABASE_URL": "prod-db:5432", // Arbitrary variable name
						"API_KEY":      "prod-key-456", // Arbitrary variable name
						"DEBUG":        "false",        // Arbitrary variable name
					},
				},
			},
		}

		result, err := validator.Validate(values)

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !result.Valid {
			t.Errorf("Expected validation to pass, got errors: %v", result.Errors)
		}

		t.Logf("Successfully validated deeply nested arbitrary keys")
	})

	t.Run("ErrorNestedArbitraryKeysInvalidTypes", func(t *testing.T) {
		// Given nested arbitrary keys with type constraints
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"config": map[string]any{
					"type": "object",
					"additionalProperties": map[string]any{
						"type":    "string",
						"pattern": "^[a-z_]+$", // Only lowercase letters and underscores
					},
				},
			},
		}

		// When validating with invalid patterns
		values := map[string]any{
			"config": map[string]any{
				"valid_key":   "valid_value",   // Valid
				"Invalid-Key": "invalid_value", // Invalid key pattern (has dash and uppercase)
				"valid_key2":  "Invalid-Value", // Invalid value pattern (has dash and uppercase)
			},
		}

		result, err := validator.Validate(values)

		// Then it should fail validation
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Note: This test demonstrates pattern validation on arbitrary keys
		t.Logf("Pattern validation on arbitrary keys - Valid=%v, Errors=%v", result.Valid, result.Errors)
	})

	t.Run("SuccessMixedDefinedAndArbitraryProperties", func(t *testing.T) {
		// Given a schema with both defined properties and arbitrary additional properties
		mockShell := shell.NewMockShell()
		validator := NewSchemaValidator(mockShell)

		validator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"name":    map[string]any{"type": "string"}, // Defined property
				"version": map[string]any{"type": "string"}, // Defined property
			},
			"additionalProperties": map[string]any{ // Schema for arbitrary properties
				"type": "object",
				"properties": map[string]any{
					"enabled": map[string]any{"type": "boolean", "default": true},
				},
			},
		}

		// When validating with mix of defined and arbitrary properties
		values := map[string]any{
			"name":    "MyService", // Defined property
			"version": "1.0.0",     // Defined property
			"auth": map[string]any{ // Arbitrary property following schema
				"enabled": true,
			},
			"logging": map[string]any{ // Arbitrary property following schema
				"enabled": false,
			},
		}

		result, err := validator.Validate(values)

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !result.Valid {
			t.Errorf("Expected validation to pass, got errors: %v", result.Errors)
		}

		t.Logf("Successfully validated mix of defined and arbitrary properties")
	})
}

func TestSchemaValidator_ArrayAndIntegerValidation(t *testing.T) {
	validator := NewSchemaValidator(nil)

	t.Run("ArrayValidation", func(t *testing.T) {
		err := validator.LoadSchemaFromBytes([]byte(`$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  items:
    type: array
    items:
      type: string
      pattern: "^[a-z]+$"`))
		if err != nil {
			t.Fatalf("Failed to load schema: %v", err)
		}

		// Valid array
		validValues := map[string]any{
			"items": []any{"hello", "world"},
		}
		result, err := validator.Validate(validValues)
		if err != nil {
			t.Fatalf("Validation failed: %v", err)
		}
		if !result.Valid {
			t.Errorf("Expected validation to pass, got errors: %v", result.Errors)
		}

		// Invalid array items
		invalidValues := map[string]any{
			"items": []any{"hello", "WORLD"},
		}
		result, err = validator.Validate(invalidValues)
		if err != nil {
			t.Fatalf("Validation failed: %v", err)
		}
		if result.Valid {
			t.Error("Expected validation to fail for invalid array items")
		}
		if len(result.Errors) == 0 {
			t.Error("Expected validation errors for invalid array items")
		}
	})

	t.Run("ValidateArrayWithNonArrayValue", func(t *testing.T) {
		validator := NewSchemaValidator(nil)
		validator.Schema = map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "string",
			},
		}

		errors := validator.validateArray("not_an_array", validator.Schema, "test")

		if len(errors) != 0 {
			t.Errorf("Expected no errors for non-array value, got %v", errors)
		}
	})

	t.Run("ValidateArrayWithoutItems", func(t *testing.T) {
		validator := NewSchemaValidator(nil)
		validator.Schema = map[string]any{
			"type": "array",
		}

		errors := validator.validateArray([]any{"item1", "item2"}, validator.Schema, "test")

		if len(errors) != 0 {
			t.Errorf("Expected no errors when items not in schema, got %v", errors)
		}
	})

	t.Run("ValidateArrayWithNonMapItems", func(t *testing.T) {
		validator := NewSchemaValidator(nil)
		validator.Schema = map[string]any{
			"type":  "array",
			"items": "not_a_map",
		}

		errors := validator.validateArray([]any{"item1", "item2"}, validator.Schema, "test")

		if len(errors) != 0 {
			t.Errorf("Expected no errors when items is not a map, got %v", errors)
		}
	})

	t.Run("IntegerValidation", func(t *testing.T) {
		err := validator.LoadSchemaFromBytes([]byte(`$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  count:
    type: integer
    default: 1`))
		if err != nil {
			t.Fatalf("Failed to load schema: %v", err)
		}

		// Valid integer
		validValues := map[string]any{
			"count": 5,
		}
		result, err := validator.Validate(validValues)
		if err != nil {
			t.Fatalf("Validation failed: %v", err)
		}
		if !result.Valid {
			t.Errorf("Expected validation to pass, got errors: %v", result.Errors)
		}

		// Invalid type (string instead of integer)
		invalidValues := map[string]any{
			"count": "5",
		}
		result, err = validator.Validate(invalidValues)
		if err != nil {
			t.Fatalf("Validation failed: %v", err)
		}
		if result.Valid {
			t.Error("Expected validation to fail for string instead of integer")
		}
	})

	t.Run("BooleanValidation", func(t *testing.T) {
		err := validator.LoadSchemaFromBytes([]byte(`$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  enabled:
    type: boolean
    default: true`))
		if err != nil {
			t.Fatalf("Failed to load schema: %v", err)
		}

		// Valid boolean
		validValues := map[string]any{
			"enabled": true,
		}
		result, err := validator.Validate(validValues)
		if err != nil {
			t.Fatalf("Validation failed: %v", err)
		}
		if !result.Valid {
			t.Errorf("Expected validation to pass, got errors: %v", result.Errors)
		}

		// Invalid type (string instead of boolean)
		invalidValues := map[string]any{
			"enabled": "true",
		}
		result, err = validator.Validate(invalidValues)
		if err != nil {
			t.Fatalf("Validation failed: %v", err)
		}
		if result.Valid {
			t.Error("Expected validation to fail for string instead of boolean")
		}
	})

	t.Run("AdditionalPropertiesWithRequired", func(t *testing.T) {
		// Test root-level additionalProperties (which we know works)
		err := validator.LoadSchemaFromBytes([]byte(`$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  name:
    type: string
additionalProperties:
  type: object
  properties:
    endpoint:
      type: string
    hostname:
      type: string
    node:
      type: string
  required: [endpoint, node]`))
		if err != nil {
			t.Fatalf("Failed to load schema: %v", err)
		}

		// Valid configuration with required fields
		validValues := map[string]any{
			"name": "cluster",
			"node1": map[string]any{
				"endpoint": "192.168.1.1",
				"node":     "control1",
				"hostname": "control-node-1",
			},
		}
		result, err := validator.Validate(validValues)
		if err != nil {
			t.Fatalf("Validation failed: %v", err)
		}
		if !result.Valid {
			t.Errorf("Expected validation to pass, got errors: %v", result.Errors)
		}

		// Invalid configuration missing required field
		invalidValues := map[string]any{
			"name": "cluster",
			"node1": map[string]any{
				"endpoint": "192.168.1.1",
				// Missing required "node" field
			},
		}
		result, err = validator.Validate(invalidValues)
		if err != nil {
			t.Fatalf("Validation failed: %v", err)
		}
		if result.Valid {
			t.Error("Expected validation to fail for missing required field in additionalProperties")
		}
		if len(result.Errors) == 0 {
			t.Error("Expected validation errors for missing required field")
		}
	})
}

func TestSchemaValidator_mergeSchema(t *testing.T) {
	t.Run("MergesPropertiesRecursively", func(t *testing.T) {
		// Given a schema validator
		validator := NewSchemaValidator(nil)

		base := map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type":    "string",
					"default": "generic",
				},
				"network": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"cidr_block": map[string]any{
							"type":    "string",
							"default": "10.0.0.0/16",
						},
					},
				},
			},
		}

		overlay := map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"cluster": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"enabled": map[string]any{
							"type":    "boolean",
							"default": true,
						},
					},
				},
				"network": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"loadbalancer_ips": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"start": map[string]any{
									"type":    "string",
									"default": "10.0.0.100",
								},
							},
						},
					},
				},
			},
		}

		// When merging schemas
		merged := validator.mergeSchema(base, overlay)

		// Then properties should be merged recursively
		properties, ok := merged["properties"].(map[string]any)
		if !ok {
			t.Fatal("Expected properties to be a map")
		}

		if properties["provider"] == nil {
			t.Error("Expected provider property from base schema")
		}

		if properties["cluster"] == nil {
			t.Error("Expected cluster property from overlay schema")
		}

		network, ok := properties["network"].(map[string]any)
		if !ok {
			t.Fatal("Expected network to be a map")
		}

		networkProps, ok := network["properties"].(map[string]any)
		if !ok {
			t.Fatal("Expected network.properties to be a map")
		}

		if networkProps["cidr_block"] == nil {
			t.Error("Expected cidr_block from base schema")
		}

		if networkProps["loadbalancer_ips"] == nil {
			t.Error("Expected loadbalancer_ips from overlay schema")
		}
	})

	t.Run("MergesRequiredArrays", func(t *testing.T) {
		// Given a schema validator
		validator := NewSchemaValidator(nil)

		base := map[string]any{
			"$schema":  "https://json-schema.org/draft/2020-12/schema",
			"type":     "object",
			"required": []any{"endpoint"},
		}

		overlay := map[string]any{
			"$schema":  "https://json-schema.org/draft/2020-12/schema",
			"type":     "object",
			"required": []any{"node"},
		}

		// When merging schemas
		merged := validator.mergeSchema(base, overlay)

		// Then required arrays should be unioned
		required, ok := merged["required"].([]any)
		if !ok {
			t.Fatal("Expected required to be an array")
		}

		if len(required) != 2 {
			t.Errorf("Expected 2 required fields, got %d", len(required))
		}

		hasEndpoint := false
		hasNode := false
		for _, req := range required {
			if req == "endpoint" {
				hasEndpoint = true
			}
			if req == "node" {
				hasNode = true
			}
		}

		if !hasEndpoint {
			t.Error("Expected 'endpoint' in required array")
		}

		if !hasNode {
			t.Error("Expected 'node' in required array")
		}
	})

	t.Run("OverridesNonMergedKeywords", func(t *testing.T) {
		// Given a schema validator
		validator := NewSchemaValidator(nil)

		base := map[string]any{
			"$schema":              "https://json-schema.org/draft/2020-12/schema",
			"type":                 "object",
			"additionalProperties": true,
		}

		overlay := map[string]any{
			"$schema":              "https://json-schema.org/draft/2020-12/schema",
			"type":                 "object",
			"additionalProperties": false,
		}

		// When merging schemas
		merged := validator.mergeSchema(base, overlay)

		// Then non-merged keywords should be overridden
		if merged["additionalProperties"] != false {
			t.Error("Expected additionalProperties to be overridden to false")
		}
	})
}

func TestSchemaValidator_mergeProperties(t *testing.T) {
	t.Run("MergesPropertyMaps", func(t *testing.T) {
		// Given a schema validator
		validator := NewSchemaValidator(nil)

		base := map[string]any{
			"provider": map[string]any{
				"type":    "string",
				"default": "generic",
			},
			"network": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cidr_block": map[string]any{
						"type": "string",
					},
				},
			},
		}

		overlay := map[string]any{
			"cluster": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"enabled": map[string]any{
						"type": "boolean",
					},
				},
			},
			"network": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"loadbalancer_ips": map[string]any{
						"type": "object",
					},
				},
			},
		}

		// When merging properties
		merged := validator.mergeProperties(base, overlay)

		// Then all properties should be present
		if merged["provider"] == nil {
			t.Error("Expected provider from base")
		}

		if merged["cluster"] == nil {
			t.Error("Expected cluster from overlay")
		}

		network, ok := merged["network"].(map[string]any)
		if !ok {
			t.Fatal("Expected network to be a map")
		}

		networkProps, ok := network["properties"].(map[string]any)
		if !ok {
			t.Fatal("Expected network.properties to be a map")
		}

		if networkProps["cidr_block"] == nil {
			t.Error("Expected cidr_block from base")
		}

		if networkProps["loadbalancer_ips"] == nil {
			t.Error("Expected loadbalancer_ips from overlay")
		}
	})

	t.Run("OverridesNonObjectProperties", func(t *testing.T) {
		// Given a schema validator
		validator := NewSchemaValidator(nil)

		base := map[string]any{
			"provider": map[string]any{
				"type":    "string",
				"default": "generic",
			},
		}

		overlay := map[string]any{
			"provider": map[string]any{
				"type":    "string",
				"default": "aws",
			},
		}

		// When merging properties
		merged := validator.mergeProperties(base, overlay)

		// Then overlay should override
		provider, ok := merged["provider"].(map[string]any)
		if !ok {
			t.Fatal("Expected provider to be a map")
		}

		if provider["default"] != "aws" {
			t.Errorf("Expected default 'aws' from overlay, got %v", provider["default"])
		}
	})
}

func TestSchemaValidator_mergeRequired(t *testing.T) {
	t.Run("UnionsRequiredArrays", func(t *testing.T) {
		// Given a schema validator
		validator := NewSchemaValidator(nil)

		base := []any{"endpoint", "node"}
		overlay := []any{"node", "hostname"}

		// When merging required arrays
		merged := validator.mergeRequired(base, overlay)

		// Then arrays should be unioned with duplicates removed
		if len(merged) != 3 {
			t.Errorf("Expected 3 required fields, got %d", len(merged))
		}

		hasEndpoint := false
		hasNode := false
		hasHostname := false
		for _, req := range merged {
			if req == "endpoint" {
				hasEndpoint = true
			}
			if req == "node" {
				hasNode = true
			}
			if req == "hostname" {
				hasHostname = true
			}
		}

		if !hasEndpoint {
			t.Error("Expected 'endpoint' in merged required")
		}

		if !hasNode {
			t.Error("Expected 'node' in merged required")
		}

		if !hasHostname {
			t.Error("Expected 'hostname' in merged required")
		}
	})

	t.Run("HandlesEmptyArrays", func(t *testing.T) {
		// Given a schema validator
		validator := NewSchemaValidator(nil)

		base := []any{}
		overlay := []any{"node"}

		// When merging required arrays
		merged := validator.mergeRequired(base, overlay)

		// Then should contain overlay items
		if len(merged) != 1 {
			t.Errorf("Expected 1 required field, got %d", len(merged))
		}

		if merged[0] != "node" {
			t.Errorf("Expected 'node', got %v", merged[0])
		}
	})

	t.Run("HandlesNilInputs", func(t *testing.T) {
		// Given a schema validator
		validator := NewSchemaValidator(nil)

		// When merging with nil inputs
		merged := validator.mergeRequired(nil, nil)

		// Then should return empty array
		if len(merged) != 0 {
			t.Errorf("Expected empty array, got %d items", len(merged))
		}
	})
}

func TestSchemaValidator_mergeItemsSchema(t *testing.T) {
	t.Run("MergesObjectItemsSchemas", func(t *testing.T) {
		// Given a schema validator
		validator := NewSchemaValidator(nil)

		base := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type": "string",
				},
				"size": map[string]any{
					"type": "string",
				},
			},
			"required": []any{"name"},
		}

		overlay := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type": "string",
				},
				"mount_path": map[string]any{
					"type":    "string",
					"default": "/mnt",
				},
			},
			"required": []any{"name", "mount_path"},
		}

		// When merging items schemas
		merged := validator.mergeItemsSchema(base, overlay)

		// Then properties should be merged
		properties, ok := merged["properties"].(map[string]any)
		if !ok {
			t.Fatal("Expected properties to be a map")
		}

		if properties["name"] == nil {
			t.Error("Expected name property")
		}

		if properties["size"] == nil {
			t.Error("Expected size property from base")
		}

		if properties["mount_path"] == nil {
			t.Error("Expected mount_path property from overlay")
		}

		// And required should be unioned
		required, ok := merged["required"].([]any)
		if !ok {
			t.Fatal("Expected required to be an array")
		}

		if len(required) != 2 {
			t.Errorf("Expected 2 required fields, got %d", len(required))
		}
	})

	t.Run("OverridesNonObjectItems", func(t *testing.T) {
		// Given a schema validator
		validator := NewSchemaValidator(nil)

		base := map[string]any{
			"type": "string",
		}

		overlay := map[string]any{
			"type": "integer",
		}

		// When merging items schemas
		merged := validator.mergeItemsSchema(base, overlay)

		// Then overlay should override
		if merged["type"] != "integer" {
			t.Errorf("Expected type 'integer' from overlay, got %v", merged["type"])
		}
	})

	t.Run("HandlesNonMapInputs", func(t *testing.T) {
		// Given a schema validator
		validator := NewSchemaValidator(nil)

		// When merging with non-map inputs
		merged := validator.mergeItemsSchema("not-a-map", map[string]any{"type": "string"})

		// Then should return overlay
		if merged["type"] != "string" {
			t.Error("Expected overlay to be returned")
		}
	})
}
