package config

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/kaptinlin/jsonschema"
	"github.com/windsorcli/cli/pkg/debug"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestFlattenErrorList(t *testing.T) {
	t.Run("DropsExactDuplicateLines", func(t *testing.T) {
		// Given two branches that both fail with the identical (path, keyword, message) triple,
		// plus one other specific failure
		list := &jsonschema.List{
			Details: []jsonschema.List{
				{InstanceLocation: "/cluster", Errors: map[string]string{"additionalProperties": "Additional property 'oidc' does not match the schema"}},
				{InstanceLocation: "/cluster", Errors: map[string]string{"additionalProperties": "Additional property 'oidc' does not match the schema"}},
				{InstanceLocation: "/network", Errors: map[string]string{"type": "Value is integer but should be object"}},
			},
		}

		// When flattening
		errs := flattenErrorList(list)

		// Then the duplicate line appears exactly once
		count := 0
		for _, e := range errs {
			if e == "/cluster: additionalProperties: Additional property 'oidc' does not match the schema" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("Expected the duplicate line to appear once, got %d occurrences in %v", count, errs)
		}
		if len(errs) != 2 {
			t.Errorf("Expected 2 distinct errors, got %d: %v", len(errs), errs)
		}
	})

	t.Run("HidesStructuralRestatementsOfASpecificViolation", func(t *testing.T) {
		// Given a "properties" (structural) failure at the root and an "additionalProperties"
		// (specific) failure nested under it
		list := &jsonschema.List{
			InstanceLocation: "/",
			Errors:           map[string]string{"properties": "Property 'cluster' does not match the schema"},
			Details: []jsonschema.List{
				{InstanceLocation: "/cluster", Errors: map[string]string{"additionalProperties": "Additional property 'oidc' does not match the schema"}},
			},
		}

		// When flattening
		errs := flattenErrorList(list)

		// Then only the specific error is reported, with no hidden-count note
		if len(errs) != 1 || !strings.HasPrefix(errs[0], "/cluster: additionalProperties:") {
			t.Errorf("Expected only the additionalProperties error, got %v", errs)
		}
	})

	t.Run("FiltersThenAsStructuralNoise", func(t *testing.T) {
		// Given a root "then" failure (the if condition matched but the then branch didn't),
		// an unrelated required/type cascade, and one real const violation
		list := &jsonschema.List{
			Details: []jsonschema.List{
				{InstanceLocation: "/", Errors: map[string]string{"then": "Value meets the 'if' condition but does not match the 'then' schema"}},
				{InstanceLocation: "/", Errors: map[string]string{"required": "Required property 'identity' is missing"}},
				{InstanceLocation: "/identity", Errors: map[string]string{"type": "Value is null but should be object"}},
				{InstanceLocation: "/database/postgres/driver", Errors: map[string]string{"const": "Value does not match the constant value"}},
			},
		}

		// When flattening
		errs := flattenErrorList(list)

		// Then only the const violation and a summary note survive, and the note does not
		// count the "then" restatement
		if len(errs) != 2 {
			t.Fatalf("Expected 2 errors, got %d: %v", len(errs), errs)
		}
		if !strings.Contains(errs[0], "/database/postgres/driver: const:") {
			t.Errorf("Expected the const error first, got %v", errs)
		}
		if !strings.Contains(errs[1], "2 other error(s) are hidden") {
			t.Errorf("Expected a summary note about hidden errors, got %v", errs)
		}
		if !strings.Contains(errs[1], "--debug") {
			t.Errorf("Expected the summary note to point at --debug, got %v", errs)
		}
	})

	t.Run("LogsHiddenErrorsToDebugWhenEnabled", func(t *testing.T) {
		// Given the same shape as above, with debug logging enabled
		debug.Init(true)
		t.Cleanup(func() { debug.Init(false) })

		list := &jsonschema.List{
			Details: []jsonschema.List{
				{InstanceLocation: "/", Errors: map[string]string{"then": "Value meets the 'if' condition but does not match the 'then' schema"}},
				{InstanceLocation: "/", Errors: map[string]string{"required": "Required property 'identity' is missing"}},
				{InstanceLocation: "/identity", Errors: map[string]string{"type": "Value is null but should be object"}},
				{InstanceLocation: "/database/postgres/driver", Errors: map[string]string{"const": "Value does not match the constant value"}},
			},
		}

		// When flattening
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("failed to create pipe: %v", err)
		}
		old := os.Stderr
		os.Stderr = w
		flattenErrorList(list)
		w.Close()
		os.Stderr = old
		var buf bytes.Buffer
		io.Copy(&buf, r)
		r.Close()
		output := buf.String()

		// Then every hidden line reaches the debug log
		for _, want := range []string{"/: then:", "/: required:", "/identity: type:"} {
			if !strings.Contains(output, want) {
				t.Errorf("Expected debug output to contain %q, got %q", want, output)
			}
		}
		// And the one visible violation is not logged as hidden
		if strings.Contains(output, "/database/postgres/driver: const:") {
			t.Errorf("Did not expect the visible violation to be logged as hidden, got %q", output)
		}
	})

	t.Run("ShowsStructuralErrorsWhenNoneAreSpecific", func(t *testing.T) {
		// Given only structural failures, with no specific violation to point to instead
		list := &jsonschema.List{
			Details: []jsonschema.List{
				{InstanceLocation: "/", Errors: map[string]string{"properties": "Property 'x' does not match the schema"}},
				{InstanceLocation: "/", Errors: map[string]string{"allOf": "Value does not match the allOf schema at index 0"}},
			},
		}

		// When flattening
		errs := flattenErrorList(list)

		// Then both structural errors are reported in their original order
		if len(errs) != 2 {
			t.Fatalf("Expected 2 errors, got %d: %v", len(errs), errs)
		}
		if !strings.Contains(errs[0], "properties") || !strings.Contains(errs[1], "allOf") {
			t.Errorf("Expected properties then allOf, got %v", errs)
		}
	})

	t.Run("SuppressesCascadingErrorsWhenPlatformEnumFails", func(t *testing.T) {
		// Given an invalid platform value plus the required/type/properties failures that
		// cascade from no platform-conditional branch matching it
		list := &jsonschema.List{
			Details: []jsonschema.List{
				{InstanceLocation: "/", Errors: map[string]string{"required": "Required property 'identity' is missing"}},
				{InstanceLocation: "/identity", Errors: map[string]string{"type": "Value is null but should be object"}},
				{InstanceLocation: "/", Errors: map[string]string{"required": "Required property 'cluster' is missing"}},
				{InstanceLocation: "/cluster", Errors: map[string]string{"type": "Value is null but should be object"}},
				{InstanceLocation: "/platform", Errors: map[string]string{"enum": "Value gcp should be one of the allowed values: none, aws, azure"}},
				{InstanceLocation: "/", Errors: map[string]string{"properties": "Property 'platform' does not match the schema"}},
			},
		}

		// When flattening
		errs := flattenErrorList(list)

		// Then only the platform error and a summary note survive
		if len(errs) != 2 {
			t.Fatalf("Expected 2 errors, got %d: %v", len(errs), errs)
		}
		if !strings.Contains(errs[0], "/platform: enum:") {
			t.Errorf("Expected the platform enum error first, got %v", errs)
		}
		if !strings.Contains(errs[1], "4 other error(s) are hidden") {
			t.Errorf("Expected a summary note about hidden errors, got %v", errs)
		}
	})

	t.Run("SuppressesCascadeFromAnyOtherSpecificViolation", func(t *testing.T) {
		// Given an invalid /gcp block plus the same identity required/type/properties fallout
		list := &jsonschema.List{
			Details: []jsonschema.List{
				{InstanceLocation: "/", Errors: map[string]string{"required": "Required property 'identity' is missing"}},
				{InstanceLocation: "/identity", Errors: map[string]string{"type": "Value is null but should be object"}},
				{InstanceLocation: "/gcp", Errors: map[string]string{"additionalProperties": "Additional properties 'project', 'zone' do not match the schema"}},
				{InstanceLocation: "/gcp/project", Errors: map[string]string{"schema": "No values are allowed because the schema is set to 'false'"}},
				{InstanceLocation: "/", Errors: map[string]string{"properties": "Property 'gcp' does not match the schema"}},
			},
		}

		// When flattening
		errs := flattenErrorList(list)

		// Then the two /gcp violations survive and the identity fallout collapses into a note
		if len(errs) != 3 {
			t.Fatalf("Expected 3 errors, got %d: %v", len(errs), errs)
		}
		joined := strings.Join(errs, "\n")
		if !strings.Contains(joined, "/gcp: additionalProperties:") || !strings.Contains(joined, "/gcp/project: schema:") {
			t.Errorf("Expected both /gcp violations to survive, got %v", errs)
		}
		if !strings.Contains(joined, "2 other error(s) are hidden") {
			t.Errorf("Expected a summary note about hidden errors, got %v", errs)
		}
	})

	t.Run("ShowsAllErrorsWhenNoneAreSpecific", func(t *testing.T) {
		// Given only cascade-prone failures — no concrete violation to point to instead
		list := &jsonschema.List{
			Details: []jsonschema.List{
				{InstanceLocation: "/", Errors: map[string]string{"required": "Required property 'identity' is missing"}},
				{InstanceLocation: "/identity", Errors: map[string]string{"type": "Value is null but should be object"}},
			},
		}

		// When flattening
		errs := flattenErrorList(list)

		// Then nothing is suppressed — required/type are the only information available
		if len(errs) != 2 {
			t.Errorf("Expected 2 errors unchanged, got %d: %v", len(errs), errs)
		}
	})

	t.Run("KeepsIndependentErrorsAlongsideAnInvalidPlatform", func(t *testing.T) {
		// Given an invalid platform value, a cascade-paired identity error, and an
		// independent pattern violation
		list := &jsonschema.List{
			Details: []jsonschema.List{
				{InstanceLocation: "/platform", Errors: map[string]string{"enum": "Value gcp should be one of the allowed values: none, aws, azure"}},
				{InstanceLocation: "/", Errors: map[string]string{"required": "Required property 'identity' is missing"}},
				{InstanceLocation: "/identity", Errors: map[string]string{"type": "Value is null but should be object"}},
				{InstanceLocation: "/network/cidr_block", Errors: map[string]string{"pattern": "Value does not match the required pattern"}},
			},
		}

		// When flattening
		errs := flattenErrorList(list)

		// Then the platform error and the independent pattern error both survive, and only the
		// cascade-paired identity required/type errors are hidden
		if len(errs) != 3 {
			t.Fatalf("Expected 3 errors, got %d: %v", len(errs), errs)
		}
		if !strings.Contains(errs[0], "/platform: enum:") {
			t.Errorf("Expected the platform enum error first, got %v", errs)
		}
		joined := strings.Join(errs, "\n")
		if !strings.Contains(joined, "/network/cidr_block: pattern:") {
			t.Errorf("Expected the independent pattern error to survive, got %v", errs)
		}
		if !strings.Contains(joined, "2 other error(s) are hidden") {
			t.Errorf("Expected two hidden errors noted, got %v", errs)
		}
	})

	t.Run("LeavesAnUnpairedRequiredErrorVisible", func(t *testing.T) {
		// Given a plain required failure with no null-type companion, alongside an
		// unrelated specific violation
		list := &jsonschema.List{
			Details: []jsonschema.List{
				{InstanceLocation: "/", Errors: map[string]string{"required": "Required property 'provider' is missing"}},
				{InstanceLocation: "/", Errors: map[string]string{"additionalProperties": "Additional property 'oidc' does not match the schema"}},
			},
		}

		// When flattening
		errs := flattenErrorList(list)

		// Then both violations stay visible — nothing is hidden
		if len(errs) != 2 {
			t.Fatalf("Expected 2 errors, got %d: %v", len(errs), errs)
		}
	})

	t.Run("MergesDuplicateRequiredMessagesAtTheSameNestedLocation", func(t *testing.T) {
		// Given three separate allOf branches each restating a required error for /cluster —
		// the shape a cluster-driver coherence check, an oidc coherence check, and a storage
		// coherence check produce independently
		list := &jsonschema.List{
			Details: []jsonschema.List{
				{InstanceLocation: "/cluster", Errors: map[string]string{"required": "Required property 'oidc' is missing"}},
				{InstanceLocation: "/cluster", Errors: map[string]string{"required": "Required property 'driver' is missing"}},
				{InstanceLocation: "/cluster", Errors: map[string]string{"required": "Required properties 'storage', 'driver' are missing"}},
			},
		}

		// When flattening
		errs := flattenErrorList(list)

		// Then the three lines collapse into one, naming every missing field exactly once,
		// sorted for a deterministic message
		if len(errs) != 1 {
			t.Fatalf("Expected 1 merged error, got %d: %v", len(errs), errs)
		}
		want := "/cluster: required: Required properties 'driver', 'oidc', 'storage' are missing"
		if errs[0] != want {
			t.Errorf("Expected merged message %q, got %q", want, errs[0])
		}
	})

	t.Run("DropsScalarTypeAndConstCompanionsOfANestedRequiredError", func(t *testing.T) {
		// Given a nested required error (cluster.driver is missing) whose null value also
		// trips a scalar type check and an unrelated const check from other allOf branches —
		// the exact shape a string-typed field like cluster.driver produces, which the
		// object-only root-only pairing in focusOnSpecificViolations never catches
		list := &jsonschema.List{
			Details: []jsonschema.List{
				{InstanceLocation: "/cluster", Errors: map[string]string{"required": "Required property 'driver' is missing"}},
				{InstanceLocation: "/cluster/driver", Errors: map[string]string{"type": "Value is null but should be string"}},
				{InstanceLocation: "/cluster/driver", Errors: map[string]string{"const": "Value does not match the constant value"}},
			},
		}

		// When flattening
		errs := flattenErrorList(list)

		// Then the required line survives — unconditionally, with nothing else specific
		// present — and both restating companions collapse into a hidden-count note
		if len(errs) != 2 {
			t.Fatalf("Expected 2 errors, got %d: %v", len(errs), errs)
		}
		if !strings.Contains(errs[0], "/cluster: required: Required property 'driver' is missing") {
			t.Errorf("Expected the required error to survive, got %v", errs)
		}
		if !strings.Contains(errs[1], "2 other error(s) are hidden") {
			t.Errorf("Expected both companions folded into one hidden-count note, got %v", errs)
		}
	})

	t.Run("HidesStructuralRestatementsOfANestedRequiredError", func(t *testing.T) {
		// Given a nested required error, its null type companion, and the two structural
		// errors that restate it at the root and at cluster
		list := &jsonschema.List{
			Details: []jsonschema.List{
				{InstanceLocation: "/cluster", Errors: map[string]string{"required": "Required property 'driver' is missing"}},
				{InstanceLocation: "/cluster/driver", Errors: map[string]string{"type": "Value is null but should be string"}},
				{InstanceLocation: "/", Errors: map[string]string{"properties": "Property 'cluster' does not match the schema"}},
				{InstanceLocation: "/cluster", Errors: map[string]string{"properties": "Property 'driver' does not match the schema"}},
			},
		}

		// When flattening
		errs := flattenErrorList(list)

		// Then only the required error and the hidden-count note remain, and the note counts
		// only the type companion
		if len(errs) != 2 {
			t.Fatalf("Expected 2 errors, got %d: %v", len(errs), errs)
		}
		if errs[0] != "/cluster: required: Required property 'driver' is missing" {
			t.Errorf("Expected the required error first, got %v", errs)
		}
		if !strings.HasPrefix(errs[1], "1 other error(s) are hidden") {
			t.Errorf("Expected the hidden-count note last with 1 hidden, got %v", errs)
		}
	})

	t.Run("CountsCascadeAndCompanionErrorsInOneNote", func(t *testing.T) {
		// Given a required/type cascade at the root that focusOnSpecificViolations hides, a
		// specific const violation, and a required companion that a later step drops
		list := &jsonschema.List{
			Details: []jsonschema.List{
				{InstanceLocation: "/", Errors: map[string]string{"required": "Required property 'identity' is missing"}},
				{InstanceLocation: "/identity", Errors: map[string]string{"type": "Value is null but should be object"}},
				{InstanceLocation: "/cluster", Errors: map[string]string{"required": "Required property 'driver' is missing"}},
				{InstanceLocation: "/cluster/driver", Errors: map[string]string{"const": "Value does not match the constant value"}},
				{InstanceLocation: "/", Errors: map[string]string{"allOf": "Value does not match the allOf schema"}},
			},
		}

		// When flattening
		errs := flattenErrorList(list)

		// Then the note is the last line and counts every hidden error once, except the
		// structural restatement
		if len(errs) != 2 {
			t.Fatalf("Expected 2 errors, got %d: %v", len(errs), errs)
		}
		if !strings.HasPrefix(errs[1], "3 other error(s) are hidden") {
			t.Errorf("Expected the hidden-count note last with 3 hidden, got %v", errs)
		}
	})

	t.Run("KeepsAScalarTypeErrorWithNoRequiredCompanion", func(t *testing.T) {
		// Given a type mismatch on a field that is present (not missing) — no required error
		// names it, so the value carries real information a required error would not
		list := &jsonschema.List{
			Details: []jsonschema.List{
				{InstanceLocation: "/cluster/driver", Errors: map[string]string{"type": "Value is null but should be string"}},
			},
		}

		// When flattening
		errs := flattenErrorList(list)

		// Then the type error is not dropped — it is only a companion once required also
		// lists the same field at the same parent
		if len(errs) != 1 || !strings.Contains(errs[0], "/cluster/driver: type:") {
			t.Errorf("Expected the unpaired type error to survive, got %v", errs)
		}
	})

	t.Run("SkipsValidSubtrees", func(t *testing.T) {
		// Given an allOf item that passes because its "not" passes, which leaves the inner
		// enum error in a valid subtree, and one real violation
		list := &jsonschema.List{
			Details: []jsonschema.List{
				{
					Valid:          true,
					EvaluationPath: "/allOf/1",
					Details: []jsonschema.List{
						{Valid: false, EvaluationPath: "/not", InstanceLocation: "/platform", Errors: map[string]string{"enum": "Value docker should be one of the allowed values: aws, azure, gcp"}},
					},
				},
				{InstanceLocation: "/secrets", Errors: map[string]string{"additionalProperties": "Additional property 'copia' does not match the schema"}},
			},
		}

		// When flattening
		errs := flattenErrorList(list)

		// Then only the real violation is reported and nothing is counted as hidden
		if len(errs) != 1 || !strings.HasPrefix(errs[0], "/secrets: additionalProperties:") {
			t.Errorf("Expected only the /secrets violation, got %v", errs)
		}
	})

	t.Run("SkipsIfConditionSubtreeUnderAFailingItem", func(t *testing.T) {
		// Given an allOf item that fails in its "else" branch, so its failed "if" condition
		// sits inside an invalid parent
		list := &jsonschema.List{
			Details: []jsonschema.List{
				{
					EvaluationPath: "/allOf/0",
					Details: []jsonschema.List{
						{
							EvaluationPath: "/if",
							Errors:         map[string]string{"required": "Required property 'platform' is missing"},
							Details: []jsonschema.List{
								{EvaluationPath: "/properties/platform", InstanceLocation: "/platform", Errors: map[string]string{"const": "Value does not match the constant value"}},
							},
						},
						{EvaluationPath: "/else", InstanceLocation: "/region", Errors: map[string]string{"type": "Value is integer but should be string"}},
					},
				},
			},
		}

		// When flattening
		errs := flattenErrorList(list)

		// Then only the "else" violation is reported
		if len(errs) != 1 || !strings.HasPrefix(errs[0], "/region: type:") {
			t.Errorf("Expected only the /region violation, got %v", errs)
		}
	})

	t.Run("ReportsErrorsUnderAPropertyNamedIf", func(t *testing.T) {
		// Given a violation under a property named "if", which is not the "if" keyword
		list := &jsonschema.List{
			Details: []jsonschema.List{
				{EvaluationPath: "/properties/if", InstanceLocation: "/if", Errors: map[string]string{"type": "Value is integer but should be string"}},
			},
		}

		// When flattening
		errs := flattenErrorList(list)

		// Then the violation is reported
		if len(errs) != 1 || !strings.HasPrefix(errs[0], "/if: type:") {
			t.Errorf("Expected the /if type violation, got %v", errs)
		}
	})

	t.Run("ReturnsPlaceholderWhenListHasNoErrors", func(t *testing.T) {
		// Given a list with no error entries anywhere
		list := &jsonschema.List{}

		// When flattening
		errs := flattenErrorList(list)

		// Then a placeholder is returned rather than an empty slice
		if len(errs) != 1 || errs[0] != "validation failed" {
			t.Errorf("Expected placeholder 'validation failed', got %v", errs)
		}
	})
}
