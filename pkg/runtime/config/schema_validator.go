// The SchemaValidator is the JSON Schema (draft 2020-12) validator for context values.
// It provides schema fragment loading, validation, default extraction, and sensitive paths.
// Each fragment deep-merges into one schema map, which compiles once on the next Validate.
// A new fragment clears the compiled schema and the sensitive path cache.

package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/kaptinlin/jsonschema"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Types
// =============================================================================

// SchemaValidator handles Windsor blueprint schema validation.
type SchemaValidator struct {
	shell    shell.Shell
	Shims    *Shims
	Schema   map[string]any
	compiled *jsonschema.Schema

	// sensitivePaths caches the result of walking Schema for `sensitive: true` markers; it is
	// invalidated alongside compiled whenever a new schema fragment is loaded. sensitivePathsOK
	// distinguishes a computed-empty result from an uncomputed cache.
	sensitivePaths   []string
	sensitivePathsOK bool

	// lastReportedErrors caches the error list last surfaced to the operator (e.g. by
	// valuesSource.Load's warning), joined into one comparable string. A later caller validating
	// the same invalid data (e.g. ValidateContextValues, run after Load in the same invocation)
	// checks ErrorsAlreadyReported before printing, so the identical wall of text isn't dumped
	// twice for one root cause.
	lastReportedErrors string
}

// SchemaValidationResult carries the outcome of a Validate call. Defaults is populated by
// Validate for callers that want both validation and defaults in one pass; standalone
// defaults extraction uses GetSchemaDefaults instead.
type SchemaValidationResult struct {
	Valid    bool           `json:"valid"`
	Errors   []string       `json:"errors,omitempty"`
	Defaults map[string]any `json:"defaults,omitempty"`
}

// =============================================================================
// Constructor
// =============================================================================

// NewSchemaValidator creates a new schema validator instance.
func NewSchemaValidator(shell shell.Shell) *SchemaValidator {
	if shell == nil {
		panic("shell is required")
	}

	return &SchemaValidator{
		shell: shell,
		Shims: NewShims(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// LoadSchema loads a schema fragment from a file on disk and merges it into the current
// in-memory schema. Subsequent Validate / GetSchemaDefaults calls operate on the union.
func (sv *SchemaValidator) LoadSchema(schemaPath string) error {
	schemaContent, err := sv.Shims.ReadFile(schemaPath)
	if err != nil {
		return werror.Wrap(err, "failed to read schema file %s", schemaPath)
	}

	return sv.LoadSchemaFromBytes(schemaContent)
}

// LoadSchemaFromBytes parses a YAML schema fragment and merges it into the current
// in-memory schema. The first fragment becomes the base; later fragments deep-merge over
// it with overlay properties winning on conflict (see schema_merge.go). Loading any new
// fragment invalidates the compiled schema cache; recompilation runs on the next Validate
// or GetSchemaDefaults call.
func (sv *SchemaValidator) LoadSchemaFromBytes(schemaContent []byte) error {
	var newSchema map[string]any
	if err := yaml.Unmarshal(schemaContent, &newSchema); err != nil {
		return werror.Wrap(err, "failed to parse schema YAML")
	}

	if err := sv.validateSchemaStructure(newSchema); err != nil {
		return werror.Wrap(err, "invalid schema structure")
	}

	if sv.Schema == nil {
		sv.Schema = newSchema
	} else {
		sv.Schema = sv.mergeSchema(sv.Schema, newSchema)
	}
	sv.compiled = nil
	sv.sensitivePaths = nil
	sv.sensitivePathsOK = false
	return nil
}

// Validate checks values against the loaded schema and returns the outcome plus extracted
// defaults. Errors are flattened from kaptinlin's hierarchical EvaluationResult into one
// deduplicated, prioritized "<instance-path>: <keyword>: <message>" line per leaf — see
// collectErrors — pass them to FormatValidationErrors for display, or ErrorsAlreadyReported to
// check whether a caller earlier in this invocation already surfaced the identical set.
func (sv *SchemaValidator) Validate(values map[string]any) (*SchemaValidationResult, error) {
	schema, err := sv.ensureCompiled()
	if err != nil {
		return nil, err
	}

	evaluation := schema.ValidateMap(values)
	result := &SchemaValidationResult{
		Valid:    evaluation.IsValid(),
		Errors:   collectErrors(evaluation),
		Defaults: extractDefaults(sv.Schema),
	}
	return result, nil
}

// MarkErrorsReported records errs (a Validate result's Errors) as already surfaced to the
// operator, so a later ErrorsAlreadyReported call for the identical error set — e.g.
// ValidateContextValues revalidating the same invalid data already warned about by
// valuesSource.Load earlier in the same invocation — can recognize the duplicate and skip
// re-printing it.
func (sv *SchemaValidator) MarkErrorsReported(errs []string) {
	sv.lastReportedErrors = errorSetFingerprint(errs)
}

// ErrorsAlreadyReported reports whether errs is the same set of errors last recorded by
// MarkErrorsReported, regardless of order.
func (sv *SchemaValidator) ErrorsAlreadyReported(errs []string) bool {
	return sv.lastReportedErrors != "" && sv.lastReportedErrors == errorSetFingerprint(errs)
}

// errorSetFingerprint builds an order-independent fingerprint for an error slice. Two separate
// Validate calls against the identical invalid data can each produce their Errors in a different
// order — kaptinlin's per-node Errors map (multiple keywords failing at the same instance
// location) iterates in Go's randomized map order, and collectErrors' stable sort only
// reorders by structural/specific priority, not by content — so comparing raw join order would
// spuriously treat the same error set as different across calls.
func errorSetFingerprint(errs []string) string {
	sorted := make([]string, len(errs))
	copy(sorted, errs)
	sort.Strings(sorted)
	return strings.Join(sorted, "\x00")
}

// FormatValidationErrors renders a Validate result's Errors as an indented list, one violation
// per line, instead of Go's flat space-joined %v slice format, so a multi-error result reads as
// a scannable list rather than a run-on string.
func FormatValidationErrors(errs []string) string {
	var b strings.Builder
	for _, e := range errs {
		b.WriteString("\n  - ")
		b.WriteString(e)
	}
	return b.String()
}

// GetSchemaDefaults returns the default values declared in the loaded schema as a single
// nested map. Callers (accessors.go, resolve.go) merge this under user values to fill in
// fields the operator omitted. Defaults extraction walks the raw merged schema without
// touching kaptinlin's compiled form — see extractDefaults for why.
func (sv *SchemaValidator) GetSchemaDefaults() (map[string]any, error) {
	if sv.Schema == nil {
		return nil, fmt.Errorf("config schema has not been loaded")
	}
	return extractDefaults(sv.Schema), nil
}

// GetSensitivePaths returns the dotted config paths of every property marked `sensitive: true`
// in the loaded schema, sorted for stable output. A free-form map region (additionalProperties)
// contributes a "*" wildcard segment for its dynamic key. Returns nil when no schema is loaded.
// The result is computed once per loaded schema and cached (invalidated on LoadSchemaFromBytes),
// so repeated calls — e.g. IsSensitivePath over every key of a config map — do not re-walk the
// schema. The returned slice is shared; callers must not mutate it.
func (sv *SchemaValidator) GetSensitivePaths() []string {
	if sv.Schema == nil {
		return nil
	}
	if sv.sensitivePathsOK {
		return sv.sensitivePaths
	}

	var paths []string
	collectSensitivePaths(sv.Schema, "", &paths)
	sort.Strings(paths)

	deduped := paths[:0]
	for i, path := range paths {
		if i == 0 || path != paths[i-1] {
			deduped = append(deduped, path)
		}
	}

	sv.sensitivePaths = deduped
	sv.sensitivePathsOK = true
	return sv.sensitivePaths
}

// =============================================================================
// Private Methods
// =============================================================================

// ensureCompiled lazily marshals the merged in-memory schema to JSON and compiles it with
// kaptinlin. Subsequent calls return the cached compiled schema until LoadSchemaFromBytes
// invalidates it.
func (sv *SchemaValidator) ensureCompiled() (*jsonschema.Schema, error) {
	if sv.Schema == nil {
		return nil, fmt.Errorf("config schema has not been loaded")
	}
	if sv.compiled != nil {
		return sv.compiled, nil
	}

	schemaJSON, err := json.Marshal(sv.Schema)
	if err != nil {
		return nil, werror.Wrap(err, "failed to marshal merged schema to JSON")
	}

	compiled, err := jsonschema.NewCompiler().Compile(schemaJSON)
	if err != nil {
		return nil, werror.Wrap(err, "failed to compile schema")
	}
	sv.compiled = compiled
	return compiled, nil
}

// validateSchemaStructure verifies the loaded fragment carries the canonical draft 2020-12
// $schema URI. The retired windsorcli dialect URI returns a migration hint pointing at
// the canonical replacement so operators upgrading from v0.8 get an actionable error
// instead of a stock "unsupported schema version".
func (sv *SchemaValidator) validateSchemaStructure(schema map[string]any) error {
	const (
		canonicalSchemaURI = "https://json-schema.org/draft/2020-12/schema"
		legacyWindsorURI   = "https://windsorcli.dev/draft/2026-02/schema"
	)

	schemaVersion, ok := schema["$schema"]
	if !ok {
		return werror.New("The schema has no '$schema' field.", fmt.Sprintf("Add '$schema: %s' to the schema.", canonicalSchemaURI), nil)
	}

	if schemaStr, ok := schemaVersion.(string); ok {
		if schemaStr == legacyWindsorURI {
			return werror.New(fmt.Sprintf("The schema uses the '%s' dialect, which windsor v0.9.0 removed.", legacyWindsorURI),
				fmt.Sprintf("Replace '$schema' with '%s'.", canonicalSchemaURI), nil)
		}
		if schemaStr != canonicalSchemaURI {
			return werror.New(fmt.Sprintf("The schema version %s is not supported.", schemaStr),
				fmt.Sprintf("Set '$schema' to '%s'.", canonicalSchemaURI), nil)
		}
	}

	return nil
}

// =============================================================================
// Helpers
// =============================================================================

// extractDefaults walks the merged in-memory schema and returns a nested map of every
// "default:" value declared under "properties". Kaptinlin's Unmarshal also applies defaults
// but only into keys the input already carries — it won't fabricate intermediate objects
// for nested-only defaults. Our callers (accessors.go, resolve.go) and operators expect a
// complete defaults shadow regardless of what the user has set, so we keep the walk on the
// raw schema map rather than routing through the compiled schema.
func extractDefaults(schema map[string]any) map[string]any {
	defaults := map[string]any{}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return defaults
	}

	for propName, propSchema := range properties {
		propSchemaMap, ok := propSchema.(map[string]any)
		if !ok {
			continue
		}
		if defaultValue, hasDefault := propSchemaMap["default"]; hasDefault {
			defaults[propName] = defaultValue
		}
		if propType, _ := propSchemaMap["type"].(string); propType == "object" {
			nested := extractDefaults(propSchemaMap)
			if len(nested) > 0 {
				defaults[propName] = nested
			}
		}
	}
	return defaults
}

// collectSensitivePaths walks a schema node, appending the dotted path of every property that
// carries a truthy `sensitive` keyword. It mirrors extractDefaults' recursion over `properties`,
// descends `additionalProperties`/`patternProperties` map regions with a "*" segment (their keys
// are dynamic), and follows `allOf`/`anyOf`/`oneOf` composition branches at the same path prefix
// (they constrain the same instance location). It does not resolve `$ref`; a sensitive leaf reached
// only through a `$ref` is not enumerated. Duplicate paths from multiple branches are removed by
// the caller.
func collectSensitivePaths(schema map[string]any, prefix string, out *[]string) {
	joinPath := func(segment string) string {
		if prefix == "" {
			return segment
		}
		return prefix + "." + segment
	}

	if properties, ok := schema["properties"].(map[string]any); ok {
		for propName, propSchema := range properties {
			propSchemaMap, ok := propSchema.(map[string]any)
			if !ok {
				continue
			}
			path := joinPath(propName)
			if propSchemaMap["sensitive"] == true {
				*out = append(*out, path)
			}
			collectSensitivePaths(propSchemaMap, path, out)
		}
	}

	for _, mapKeyword := range []string{"additionalProperties", "patternProperties"} {
		region, ok := schema[mapKeyword].(map[string]any)
		if !ok {
			continue
		}
		path := joinPath("*")
		if mapKeyword == "additionalProperties" {
			if region["sensitive"] == true {
				*out = append(*out, path)
			}
			collectSensitivePaths(region, path, out)
			continue
		}
		// patternProperties maps each regex pattern to its own subschema; all dynamic keys
		// collapse onto the same "*" segment.
		for _, patternSchema := range region {
			patternSchemaMap, ok := patternSchema.(map[string]any)
			if !ok {
				continue
			}
			if patternSchemaMap["sensitive"] == true {
				*out = append(*out, path)
			}
			collectSensitivePaths(patternSchemaMap, path, out)
		}
	}

	for _, composition := range []string{"allOf", "anyOf", "oneOf"} {
		branches, ok := schema[composition].([]any)
		if !ok {
			continue
		}
		for _, branch := range branches {
			if branchMap, ok := branch.(map[string]any); ok {
				collectSensitivePaths(branchMap, prefix, out)
			}
		}
	}
}
