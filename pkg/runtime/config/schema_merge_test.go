package config_test

import (
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestSchemaValidator_ConservativeKeywordMerge(t *testing.T) {
	// loadMerged loads each fragment in turn and returns the resulting validator, so a test
	// can assert the merged constraints without pinning to fragment load order.
	loadMerged := func(t *testing.T, fragments ...string) *config.SchemaValidator {
		t.Helper()
		validator := config.NewSchemaValidator(shell.NewMockShell())
		for i, fragment := range fragments {
			if err := validator.LoadSchemaFromBytes([]byte(fragment)); err != nil {
				t.Fatalf("Failed to load fragment %d: %v", i, err)
			}
		}
		return validator
	}

	closed := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  a:
    type: string
additionalProperties: false
`
	open := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  b:
    type: string
additionalProperties: true
`

	t.Run("AdditionalPropertiesFalseSurvivesRegardlessOfLoadOrder", func(t *testing.T) {
		// Given fragments that disagree on additionalProperties, loaded in each order
		for _, order := range [][]string{{closed, open}, {open, closed}} {
			validator := loadMerged(t, order...)

			// And a value carrying a key neither fragment declares
			values := map[string]any{"a": "x", "b": "y", "c": "z"}

			// When validating
			result, err := validator.Validate(values)
			if err != nil {
				t.Fatalf("Validation failed: %v", err)
			}

			// Then the closed constraint wins and the undeclared key is rejected
			if result.Valid {
				t.Errorf("Expected 'c' to be rejected once any fragment closes the object, order %v", order)
			}
			if !hasErrorMatching(result.Errors, "additionalProperties") {
				t.Errorf("Expected an additionalProperties error, got %v", result.Errors)
			}
		}
	})

	t.Run("NumericBoundsKeepTighterValue", func(t *testing.T) {
		// Given fragments that widen and narrow the same numeric bounds
		loose := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  count:
    type: integer
    minimum: 1
    maximum: 100
additionalProperties: true
`
		strict := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  count:
    type: integer
    minimum: 5
    maximum: 10
additionalProperties: true
`
		// When loaded in either order, the tighter bounds hold both ways
		for _, order := range [][]string{{loose, strict}, {strict, loose}} {
			validator := loadMerged(t, order...)

			// Then a value below the tighter minimum (5) is rejected
			result, err := validator.Validate(map[string]any{"count": 3})
			if err != nil {
				t.Fatalf("Validation failed: %v", err)
			}
			if result.Valid {
				t.Errorf("Expected count=3 to fail the tighter minimum of 5, order %v", order)
			}

			// And a value above the tighter maximum (10) is rejected
			result, err = validator.Validate(map[string]any{"count": 50})
			if err != nil {
				t.Fatalf("Validation failed: %v", err)
			}
			if result.Valid {
				t.Errorf("Expected count=50 to fail the tighter maximum of 10, order %v", order)
			}

			// And a value inside the intersection passes
			result, err = validator.Validate(map[string]any{"count": 7})
			if err != nil {
				t.Fatalf("Validation failed: %v", err)
			}
			if !result.Valid {
				t.Errorf("Expected count=7 to pass, order %v, got errors: %v", order, result.Errors)
			}
		}
	})

	t.Run("EnumIntersectsAcrossFragments", func(t *testing.T) {
		// Given fragments whose enums overlap but are not identical
		first := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  platform:
    type: string
    enum: [aws, azure, gcp]
additionalProperties: true
`
		second := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  platform:
    type: string
    enum: [azure, gcp, metal]
additionalProperties: true
`
		// When loaded in either order, only the intersection {azure, gcp} stays valid
		for _, order := range [][]string{{first, second}, {second, first}} {
			validator := loadMerged(t, order...)

			// Then a value in only the first fragment's enum is rejected
			result, err := validator.Validate(map[string]any{"platform": "aws"})
			if err != nil {
				t.Fatalf("Validation failed: %v", err)
			}
			if result.Valid {
				t.Errorf("Expected platform=aws to fail (outside intersection), order %v", order)
			}

			// And a value in only the second fragment's enum is rejected
			result, err = validator.Validate(map[string]any{"platform": "metal"})
			if err != nil {
				t.Fatalf("Validation failed: %v", err)
			}
			if result.Valid {
				t.Errorf("Expected platform=metal to fail (outside intersection), order %v", order)
			}

			// And a value in both fragments' enums passes
			result, err = validator.Validate(map[string]any{"platform": "azure"})
			if err != nil {
				t.Fatalf("Validation failed: %v", err)
			}
			if !result.Valid {
				t.Errorf("Expected platform=azure to pass, order %v, got errors: %v", order, result.Errors)
			}
		}
	})

	t.Run("SchemaObjectAdditionalPropertiesMergeConservatively", func(t *testing.T) {
		// Given fragments whose additionalProperties are schema objects, not booleans:
		// one constrains extra keys to strings, the other requires a minimum length
		strings := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
additionalProperties:
  type: string
`
		minLen := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
additionalProperties:
  minLength: 3
`
		for _, order := range [][]string{{strings, minLen}, {minLen, strings}} {
			validator := loadMerged(t, order...)

			// Then an extra key satisfying both constraints passes
			result, err := validator.Validate(map[string]any{"extra": "abcd"})
			if err != nil {
				t.Fatalf("Validation failed: %v", err)
			}
			if !result.Valid {
				t.Errorf("Expected extra='abcd' to pass, order %v, got errors: %v", order, result.Errors)
			}

			// And one violating the base fragment's constraint (too short) is still rejected,
			// proving the base constraint was not dropped when the overlay was also a schema
			result, err = validator.Validate(map[string]any{"extra": "ab"})
			if err != nil {
				t.Fatalf("Validation failed: %v", err)
			}
			if result.Valid {
				t.Errorf("Expected extra='ab' to fail the merged minLength constraint, order %v", order)
			}
		}
	})

	t.Run("DisjointEnumsRejectEveryValue", func(t *testing.T) {
		// Given fragments whose enums share no members
		first := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  mode:
    type: string
    enum: [a, b]
additionalProperties: true
`
		second := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  mode:
    type: string
    enum: [c, d]
additionalProperties: true
`
		validator := loadMerged(t, first, second)

		// When validating a value from either fragment's enum
		for _, val := range []string{"a", "c"} {
			result, err := validator.Validate(map[string]any{"mode": val})
			if err != nil {
				t.Fatalf("Validation failed: %v", err)
			}

			// Then it is rejected: an empty intersection permits no value
			if result.Valid {
				t.Errorf("Expected mode=%q to fail against an empty enum intersection", val)
			}
		}
	})
}
