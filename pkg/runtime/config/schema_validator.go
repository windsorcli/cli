package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/kaptinlin/jsonschema"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// SchemaValidator wraps a kaptinlin JSON Schema (draft 2020-12) compiler. Schemas are
// authored as YAML alongside the resource types they describe and loaded one fragment at a
// time via LoadSchema / LoadSchemaFromBytes — each fragment deep-merges into the in-memory
// Schema map (see schema_merge.go). On the next call to Validate the merged map is
// marshaled to JSON and compiled once; the compiled schema is cached and invalidated
// whenever another fragment is loaded. Defaults extraction runs against the raw merged
// schema map via the standalone extractDefaults walker rather than the compiled form;
// see extractDefaults for the rationale (kaptinlin's Unmarshal applies defaults only into
// existing keys, which doesn't fit our "complete defaults shadow" semantic).

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
		return fmt.Errorf("failed to read schema file %s: %w", schemaPath, err)
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
		return fmt.Errorf("failed to parse schema YAML: %w", err)
	}

	if err := sv.validateSchemaStructure(newSchema); err != nil {
		return fmt.Errorf("invalid schema structure: %w", err)
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
// "<instance-path>: <message>" line per leaf, suitable for direct %v formatting by callers.
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
		return nil, fmt.Errorf("failed to marshal merged schema to JSON: %w", err)
	}

	compiled, err := jsonschema.NewCompiler().Compile(schemaJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to compile schema: %w", err)
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
		return fmt.Errorf("missing required '$schema' field")
	}

	if schemaStr, ok := schemaVersion.(string); ok {
		if schemaStr == legacyWindsorURI {
			return fmt.Errorf("the '%s' dialect was removed in v0.9.0; replace '$schema' with '%s'", legacyWindsorURI, canonicalSchemaURI)
		}
		if schemaStr != canonicalSchemaURI {
			return fmt.Errorf("unsupported schema version: %s", schemaStr)
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

// collectErrors flattens a kaptinlin EvaluationResult into one error string per leaf,
// formatted as "<instance-path>: <keyword>: <message>" so callers that %v-format the
// []string surface readable paths to operators.
func collectErrors(result *jsonschema.EvaluationResult) []string {
	if result.IsValid() {
		return nil
	}
	list := result.ToList(true)
	var errs []string
	walkList(list, "", &errs)
	if len(errs) == 0 {
		errs = []string{"validation failed"}
	}
	return errs
}

// walkList descends a flattened List, emitting one string per (instance-location, keyword)
// pair. Instance locations use JSON Pointer notation rooted at "/" (e.g. "/network/cidr_block").
// kaptinlin scopes each evaluator's leaf to a path local to that evaluator — a nested enum
// failure on storage.driver surfaces as "/driver" rather than "/storage/driver" — so this
// walker carries the parent location down the recursion and joins it onto each child to
// reconstruct the fully-qualified pointer operators expect when reading the message.
func walkList(list *jsonschema.List, parent string, errs *[]string) {
	loc := joinInstanceLocation(parent, list.InstanceLocation)
	display := loc
	if display == "" {
		display = "/"
	}
	for keyword, msg := range list.Errors {
		*errs = append(*errs, fmt.Sprintf("%s: %s: %s", display, keyword, msg))
	}
	for i := range list.Details {
		walkList(&list.Details[i], loc, errs)
	}
}

// joinInstanceLocation concatenates two JSON Pointer fragments, treating "" and "/" as the
// document root and guarding against double-prepending when a child evaluator already
// reports its absolute path.
func joinInstanceLocation(parent, child string) string {
	switch {
	case child == "" || child == "/":
		return parent
	case parent == "" || parent == "/":
		return child
	case strings.HasPrefix(child, parent+"/"):
		return child
	default:
		return parent + child
	}
}
