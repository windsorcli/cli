package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestConfigHandler_LoadSchema(t *testing.T) {
	t.Run("LoadsSchemaSuccessfully", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  test_key:
    type: string
    default: test_value
`
		schemaPath := filepath.Join(schemaDir, "schema.yaml")
		os.WriteFile(schemaPath, []byte(schemaContent), 0644)

		err := handler.LoadSchema(schemaPath)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		value := handler.Get("test_key")
		if value != "test_value" {
			t.Error("Expected schema default to be accessible after LoadSchema")
		}
	})

	t.Run("ReturnsErrorForInvalidSchema", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		invalidSchema := `invalid: yaml: content: [[[`
		schemaPath := filepath.Join(schemaDir, "schema.yaml")
		os.WriteFile(schemaPath, []byte(invalidSchema), 0644)

		err := handler.LoadSchema(schemaPath)

		if err == nil {
			t.Error("Expected error for invalid schema")
		}
	})

	t.Run("HandlesReadFileError", func(t *testing.T) {
		// Given a handler and non-existent schema file
		handler, _ := setupPrivateTestHandler(t)

		// When loading non-existent schema
		err := handler.LoadSchema("/nonexistent/schema.yaml")

		// Then it should return read file error
		if err == nil {
			t.Error("Expected read file error")
		}
	})

	t.Run("HandlesInvalidSchemaContent", func(t *testing.T) {
		// Given a handler and invalid schema content
		handler, tmpDir := setupPrivateTestHandler(t)

		schemaPath := filepath.Join(tmpDir, "invalid_schema.yaml")
		os.WriteFile(schemaPath, []byte("invalid yaml [[["), 0644)

		// When loading invalid schema
		err := handler.LoadSchema(schemaPath)

		// Then it should return error for invalid schema content
		if err == nil {
			t.Error("Expected error for invalid schema content")
		}
	})

	t.Run("ReturnsErrorWhenSchemaValidatorIsNil", func(t *testing.T) {
		// Given a handler with nil schema validator
		handler, _ := setupPrivateTestHandler(t)
		handler.schemaValidator = nil

		// When loading schema
		err := handler.LoadSchema("/some/path/schema.yaml")

		// Then it should return error
		if err == nil {
			t.Error("Expected error when schema validator is nil")
		}
		if !strings.Contains(err.Error(), "schema validator missing") {
			t.Errorf("Expected error about schema validator missing, got: %v", err)
		}
	})
}

func TestConfigHandler_LoadSchemaFromBytes(t *testing.T) {
	t.Run("LoadsSchemaFromBytes", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		schemaContent := []byte(`$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  byte_schema_key:
    type: string
    default: from_bytes
`)

		err := handler.LoadSchemaFromBytes(schemaContent)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		value := handler.Get("byte_schema_key")
		if value != "from_bytes" {
			t.Error("Expected schema default from bytes to be accessible")
		}
	})

	t.Run("ReturnsErrorWhenSchemaValidatorIsNil", func(t *testing.T) {
		// Given a handler with nil schema validator
		handler, _ := setupPrivateTestHandler(t)
		handler.schemaValidator = nil

		// When loading schema from bytes
		err := handler.LoadSchemaFromBytes([]byte("test: content"))

		// Then it should return error
		if err == nil {
			t.Error("Expected error when schema validator is nil")
		}
		if !strings.Contains(err.Error(), "schema validator missing") {
			t.Errorf("Expected error about schema validator missing, got: %v", err)
		}
	})

	t.Run("HandlesInvalidSchemaBytes", func(t *testing.T) {
		// Given a handler and invalid schema bytes
		handler, _ := setupPrivateTestHandler(t)

		// When loading invalid schema bytes
		err := handler.LoadSchemaFromBytes([]byte("invalid yaml [[["))

		// Then it should return error for invalid schema bytes
		if err == nil {
			t.Error("Expected error for invalid schema bytes")
		}
	})

	t.Run("HandlesValidSchemaBytes", func(t *testing.T) {
		// Given a handler and valid schema bytes
		handler, _ := setupPrivateTestHandler(t)

		schemaBytes := []byte(`$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  test:
    type: string
`)

		// When loading valid schema bytes
		err := handler.LoadSchemaFromBytes(schemaBytes)

		// Then it should succeed without error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestConfigHandler_ValidateContextValues(t *testing.T) {
	t.Run("ReturnsNilWhenSchemaNotLoaded", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)
		handler.schemaValidator.Schema = nil
		handler.data = map[string]any{"dynamic_key": "value"}

		if err := handler.ValidateContextValues(); err != nil {
			t.Fatalf("Expected no error without schema, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenContextValuesFailValidation", func(t *testing.T) {
		handler, tmpDir := setupPrivateTestHandler(t)
		schemaPath := filepath.Join(tmpDir, "schema.yaml")
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  allowed:
    type: string
additionalProperties: false
`
		os.WriteFile(schemaPath, []byte(schemaContent), 0644)
		if err := handler.LoadSchema(schemaPath); err != nil {
			t.Fatalf("Expected no error loading schema, got %v", err)
		}
		handler.data = map[string]any{"dynamic_key": "value"}

		err := handler.ValidateContextValues()
		if err == nil {
			t.Fatal("Expected validation error for disallowed key")
		}
		if !strings.Contains(err.Error(), "Context values do not match the schema:") {
			t.Errorf("Expected validation failure error, got %v", err)
		}
	})

	t.Run("RefersBackToWarningWhenSameErrorsAlreadyReported", func(t *testing.T) {
		handler, tmpDir := setupPrivateTestHandler(t)
		schemaPath := filepath.Join(tmpDir, "schema.yaml")
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  allowed:
    type: string
additionalProperties: false
`
		os.WriteFile(schemaPath, []byte(schemaContent), 0644)
		if err := handler.LoadSchema(schemaPath); err != nil {
			t.Fatalf("Expected no error loading schema, got %v", err)
		}
		handler.data = map[string]any{"dynamic_key": "value"}

		// Given the same invalid data was already validated and warned about earlier (e.g. by
		// valuesSource.Load) in this invocation
		result, err := handler.schemaValidator.Validate(handler.data)
		if err != nil {
			t.Fatalf("Expected no error validating, got %v", err)
		}
		handler.schemaValidator.MarkErrorsReported(result.Errors)

		// When ValidateContextValues revalidates the identical data
		err = handler.ValidateContextValues()

		// Then it refers back to the earlier warning instead of re-dumping the same error list
		if err == nil {
			t.Fatal("Expected validation error")
		}
		if err.Error() != "Context values do not match the schema. The values.yaml warning above lists the errors." {
			t.Errorf("Expected a reference to the earlier warning, got %v", err)
		}
	})

	t.Run("ValidatesStaticFieldsAgainstSchema", func(t *testing.T) {
		handler, tmpDir := setupPrivateTestHandler(t)
		schemaPath := filepath.Join(tmpDir, "schema.yaml")
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  provider:
    type: string
    enum: [docker, incus]
`
		os.WriteFile(schemaPath, []byte(schemaContent), 0644)
		if err := handler.LoadSchema(schemaPath); err != nil {
			t.Fatalf("Expected no error loading schema, got %v", err)
		}
		handler.data = map[string]any{"provider": "not-allowed"}

		err := handler.ValidateContextValues()
		if err == nil {
			t.Fatal("Expected validation error for static field violating schema")
		}
		if !strings.Contains(err.Error(), "Context values do not match the schema:") {
			t.Errorf("Expected validation failure error, got %v", err)
		}
	})

	t.Run("RejectsExplicitIDNotMatchingRequiredShapeEvenWithNoSchemaLoaded", func(t *testing.T) {
		// Given a handler with no schema loaded (e.g. a fresh `windsor init` before any
		// blueprint schema exists) and an id set outside GenerateContextID's own safe-shape path
		handler, _ := setupPrivateTestHandler(t)
		handler.data = map[string]any{"id": "566b0c12a370"}

		// When validating context values
		err := handler.ValidateContextValues()

		// Then it should reject the digit-leading id instead of silently accepting it
		if err == nil {
			t.Fatal("Expected validation error for id not matching the required shape")
		}
		if !strings.Contains(err.Error(), "Context id ") {
			t.Errorf("Expected validation failure error, got %v", err)
		}
	})

	t.Run("AcceptsIDMatchingRequiredShapeWithNoSchemaLoaded", func(t *testing.T) {
		// Given a handler with no schema loaded and an id matching the required shape
		handler, _ := setupPrivateTestHandler(t)
		handler.data = map[string]any{"id": "wabc1234"}

		// When validating context values
		err := handler.ValidateContextValues()

		// Then it should pass
		if err != nil {
			t.Errorf("Expected no error for a valid id, got %v", err)
		}
	})

	t.Run("AcceptsIDContainingHyphens", func(t *testing.T) {
		// Given a handler with no schema loaded and a hyphenated, user-chosen id
		handler, _ := setupPrivateTestHandler(t)
		handler.data = map[string]any{"id": "my-plant"}

		// When validating context values
		err := handler.ValidateContextValues()

		// Then it should pass, since a hyphen is a valid DNS label character
		if err != nil {
			t.Errorf("Expected no error for a hyphenated id, got %v", err)
		}
	})

	t.Run("RejectsIDEndingInHyphen", func(t *testing.T) {
		// Given a handler with no schema loaded and an id ending in a hyphen
		handler, _ := setupPrivateTestHandler(t)
		handler.data = map[string]any{"id": "my-plant-"}

		// When validating context values
		err := handler.ValidateContextValues()

		// Then it should reject the trailing hyphen
		if err == nil {
			t.Fatal("Expected validation error for an id ending in a hyphen")
		}
		if !strings.Contains(err.Error(), "Context id ") {
			t.Errorf("Expected validation failure error, got %v", err)
		}
	})

	t.Run("RejectsIDWithRepeatedHyphen", func(t *testing.T) {
		// Given a handler with no schema loaded and an id containing a repeated hyphen
		handler, _ := setupPrivateTestHandler(t)
		handler.data = map[string]any{"id": "my--plant"}

		// When validating context values
		err := handler.ValidateContextValues()

		// Then it should reject the repeated hyphen
		if err == nil {
			t.Fatal("Expected validation error for an id with a repeated hyphen")
		}
		if !strings.Contains(err.Error(), "Context id ") {
			t.Errorf("Expected validation failure error, got %v", err)
		}
	})

	t.Run("FiresCrossFieldRuleSpanningStaticAndDynamic", func(t *testing.T) {
		handler, tmpDir := setupPrivateTestHandler(t)
		schemaPath := filepath.Join(tmpDir, "schema.yaml")
		// gateway (dynamic) requires dns (static) when access is private — exercises a rule that
		// the prior dynamicFields-only validation path filtered out and silently passed.
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  gateway:
    type: object
    properties:
      access:
        type: string
  dns:
    type: object
    properties:
      private_domain:
        type: string
allOf:
  - if:
      properties:
        gateway:
          type: object
          properties:
            access:
              const: private
          required: [access]
      required: [gateway]
    then:
      required: [dns]
      properties:
        dns:
          type: object
          required: [private_domain]
          properties:
            private_domain:
              type: string
              minLength: 1
`
		os.WriteFile(schemaPath, []byte(schemaContent), 0644)
		if err := handler.LoadSchema(schemaPath); err != nil {
			t.Fatalf("Expected no error loading schema, got %v", err)
		}

		// Violation: gateway.access=private without dns.private_domain.
		handler.data = map[string]any{
			"gateway": map[string]any{"access": "private"},
		}
		if err := handler.ValidateContextValues(); err == nil {
			t.Fatal("Expected cross-field rule to fire when gateway.access=private and dns.private_domain is missing")
		}

		// Satisfied: both fields present.
		handler.data = map[string]any{
			"gateway": map[string]any{"access": "private"},
			"dns":     map[string]any{"private_domain": "internal.example"},
		}
		if err := handler.ValidateContextValues(); err != nil {
			t.Fatalf("Expected cross-field rule to pass when both fields are set, got %v", err)
		}

		// Non-triggering: if-condition not met.
		handler.data = map[string]any{
			"gateway": map[string]any{"access": "public"},
		}
		if err := handler.ValidateContextValues(); err != nil {
			t.Fatalf("Expected validation to pass when if-condition is not met, got %v", err)
		}
	})
}

func TestConfigHandler_SensitivePaths(t *testing.T) {
	setup := func(t *testing.T) *configHandler {
		t.Helper()
		handler, _ := setupPrivateTestHandler(t)
		handler.schemaValidator.Schema = map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"cdn": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"cloudflare_api_key": map[string]any{"type": "string", "sensitive": true},
						"zone":               map[string]any{"type": "string"},
					},
				},
				"secrets": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"stores": map[string]any{
							"type": "object",
							"additionalProperties": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"token": map[string]any{"type": "string", "sensitive": true},
								},
							},
						},
					},
				},
			},
		}
		return handler
	}

	t.Run("GetSensitivePathsDelegatesSorted", func(t *testing.T) {
		// Given a handler with sensitive markers at a nested path and under a map region
		handler := setup(t)

		// When getting sensitive paths
		got := strings.Join(handler.GetSensitivePaths(), ",")

		// Then both are returned, sorted
		want := "cdn.cloudflare_api_key,secrets.stores.*.token"
		if got != want {
			t.Errorf("Expected %q, got %q", want, got)
		}
	})

	t.Run("IsSensitivePathExactMatch", func(t *testing.T) {
		// Given a handler with a sensitive nested property
		handler := setup(t)

		// Then the exact path is sensitive and a sibling is not
		if !handler.IsSensitivePath("cdn.cloudflare_api_key") {
			t.Error("Expected cdn.cloudflare_api_key to be sensitive")
		}
		if handler.IsSensitivePath("cdn.zone") {
			t.Error("Expected cdn.zone to not be sensitive")
		}
	})

	t.Run("IsSensitivePathWildcardSegment", func(t *testing.T) {
		// Given a sensitive leaf under a free-form map region
		handler := setup(t)

		// Then a concrete key matches the "*" segment
		if !handler.IsSensitivePath("secrets.stores.prod.token") {
			t.Error("Expected secrets.stores.prod.token to match wildcard sensitive path")
		}
		// And a path with the wrong segment count does not match
		if handler.IsSensitivePath("secrets.stores.token") {
			t.Error("Expected secrets.stores.token (wrong depth) to not match")
		}
	})

	t.Run("NilValidatorReturnsEmpty", func(t *testing.T) {
		// Given a handler with no schema validator
		handler, _ := setupPrivateTestHandler(t)
		handler.schemaValidator = nil

		// Then sensitive-path queries are safe no-ops
		if handler.GetSensitivePaths() != nil {
			t.Error("Expected nil sensitive paths")
		}
		if handler.IsSensitivePath("anything") {
			t.Error("Expected IsSensitivePath false with no validator")
		}
	})
}
