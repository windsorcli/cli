// schema.go emits docs/reference/<name>.md from JSON Schema files under
// pkg/runtime/config/schemas/. The schema is the single source of truth for
// the artifact's shape (windsor.yaml, metadata.yaml, blueprint.yaml, etc.);
// the walker renders frontmatter, an h1 + intro from the top-level
// title/description, a field table per object schema, nested-object subsections,
// an optional Example block from the schema's examples array, and a See also
// section sourced from an optional '<name>.seealso.md' sidecar file alongside
// the schema. The sidecar pattern keeps schemas pure JSON Schema (no vendor
// extensions) while still letting authors curate cross-links per page.
//
// Supported schema features: object (with properties + required), array (with
// items.type), string/integer/boolean/number primitives, enum, default,
// description. Anything fancier (oneOf, allOf, refs) is rendered as a best-
// effort "object" row; extend the walker when a real schema needs it.

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
)

const schemaSourceURLPrefix = "https://github.com/windsorcli/cli/blob/main/"

func schemaCmd() *cobra.Command {
	var outDir, schemaDir string
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Generate markdown reference for Windsor YAML schemas",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return generateSchemaDocs(schemaDir, outDir)
		},
	}
	cmd.Flags().StringVar(&schemaDir, "in", "pkg/runtime/config/schemas", "directory of *.yaml schemas to read")
	cmd.Flags().StringVar(&outDir, "out", "docs/reference", "directory to write *.md output into")
	return cmd
}

// generateSchemaDocs walks schemaDir for *.yaml files and emits one .md file
// per schema into outDir (filename derived by stripping the .yaml extension).
// Unlike the commands generator this does NOT wipe outDir — schema pages
// coexist with other reference content (commands/, contexts.md, etc.) that
// the commands generator and human authors maintain.
func generateSchemaDocs(schemaDir, outDir string) error {
	entries, err := os.ReadDir(schemaDir)
	if err != nil {
		return fmt.Errorf("read schema dir: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}
		// 'common.yaml' merges into windsor.yaml at validation time and is not
		// a standalone artifact; skipping it keeps the docs/reference tree
		// scoped to user-facing artifacts. Re-enable explicitly if we ever
		// want a published fragment page.
		if name == "common.yaml" {
			continue
		}

		schemaPath := filepath.Join(schemaDir, name)
		base := strings.TrimSuffix(name, ".yaml")
		outPath := filepath.Join(outDir, base+".md")
		// Sidecar prose lives alongside the schema as '<name>.seealso.md'.
		// Empty string when absent — renderSchema treats that as "no See also".
		seealsoPath := filepath.Join(schemaDir, base+".seealso.md")
		if err := writeSchemaFile(schemaPath, outPath, seealsoPath); err != nil {
			return fmt.Errorf("emit %s: %w", schemaPath, err)
		}
	}
	return nil
}

func writeSchemaFile(schemaPath, outPath, seealsoPath string) error {
	raw, err := os.ReadFile(schemaPath) // #nosec G304 - schemaPath is operator-supplied via --in; filename derives from the directory listing
	if err != nil {
		return fmt.Errorf("read %s: %w", schemaPath, err)
	}
	var schema map[string]any
	if err := yaml.Unmarshal(raw, &schema); err != nil {
		return fmt.Errorf("parse %s: %w", schemaPath, err)
	}
	// Re-parse the examples block as yaml.MapSlice so each example renders in
	// author-defined field order. The plain map[string]any unmarshal above
	// loses insertion order, which would alphabetize fields in the rendered
	// YAML block — confusing for readers comparing the example to a real
	// metadata.yaml they've authored.
	var ordered struct {
		Examples []yaml.MapSlice `yaml:"examples"`
	}
	if err := yaml.Unmarshal(raw, &ordered); err != nil {
		return fmt.Errorf("parse %s examples: %w", schemaPath, err)
	}

	var seealso string
	if seealsoPath != "" {
		if body, err := os.ReadFile(seealsoPath); err == nil { // #nosec G304 - seealsoPath is derived from --in plus the schema basename
			seealso = string(body)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("read sidecar %s: %w", seealsoPath, err)
		}
	}

	f, err := os.Create(outPath) // #nosec G304 - outPath is constructed from --out plus the schema basename
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	defer f.Close()

	relSource, err := filepath.Rel(".", schemaPath)
	if err != nil {
		relSource = schemaPath
	}
	if err := renderSchema(f, schema, ordered.Examples, relSource, seealso); err != nil {
		return fmt.Errorf("render %s: %w", outPath, err)
	}
	return nil
}

// renderSchema produces the per-page markdown. Mirrors the structure used by
// the cobra renderer (frontmatter → h1 → prose → tables → example → see also)
// so the docs site presents schema and command pages consistently. examples
// is the ordered (MapSlice) re-parse of the schema's examples array so each
// example renders in author-defined field order; seealso is the raw markdown
// body of the per-schema sidecar file (empty when absent).
func renderSchema(w io.Writer, schema map[string]any, examples []yaml.MapSlice, sourcePath, seealso string) error {
	ew := &errWriter{w: w}

	title, _ := schema["title"].(string)
	if title == "" {
		title = "Schema"
	}
	intro, _ := schema["description"].(string)

	writeSchemaFrontmatter(ew, title, summarize(intro))
	fmt.Fprintf(ew, "# %s\n\n", title)
	if intro != "" {
		fmt.Fprintln(ew, strings.TrimSpace(intro))
		fmt.Fprintln(ew)
	}

	writeSchemaFields(ew, schema, "")
	writeSchemaExamples(ew, examples)
	writeSchemaSeeAlso(ew, seealso, sourcePath)
	return ew.err
}

func writeSchemaFrontmatter(w io.Writer, title, description string) {
	fmt.Fprintln(w, "---")
	fmt.Fprintf(w, "title: %q\n", title)
	if description != "" {
		fmt.Fprintf(w, "description: %q\n", description)
	}
	fmt.Fprintln(w, "---")
}

// writeSchemaFields renders the top-level field table for an object schema,
// then recurses into nested object properties as h2/h3 subsections. headingPath
// tracks the breadcrumb so nested headings read like "## Workstation" rather
// than just "## (object)".
func writeSchemaFields(w io.Writer, schema map[string]any, headingPath string) {
	if typeOf(schema) != "object" {
		return
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok || len(props) == 0 {
		return
	}

	required := requiredSet(schema)
	heading := "Fields"
	if headingPath != "" {
		heading = headingPath
	}
	level := "##"
	if strings.Count(headingPath, ".") > 0 {
		level = "###"
	}
	fmt.Fprintf(w, "%s %s\n\n", level, heading)
	fmt.Fprintln(w, "| Field | Type | Description |")
	fmt.Fprintln(w, "|------|------|-------------|")

	names := propertyNames(props, requiredList(schema))
	var nested []nestedSection
	for _, name := range names {
		propSchema, _ := props[name].(map[string]any)
		if propSchema == nil {
			continue
		}
		fmt.Fprintln(w, schemaFieldRow(name, propSchema, required[name]))
		if typeOf(propSchema) == "object" && hasProperties(propSchema) {
			label := name
			if headingPath != "" {
				label = headingPath + "." + name
			}
			nested = append(nested, nestedSection{label: label, schema: propSchema})
		}
	}
	fmt.Fprintln(w)

	for _, n := range nested {
		writeSchemaFields(w, n.schema, n.label)
	}
}

type nestedSection struct {
	label  string
	schema map[string]any
}

// schemaFieldRow renders one row of the field table. The Type column uses
// 'array<string>' for typed arrays, the bare type name otherwise. Required
// fields get a bold "**(required)**" suffix in Description; defaults get
// "Default: `<value>`"; enums get "One of: `a`, `b`, `c`".
func schemaFieldRow(name string, propSchema map[string]any, required bool) string {
	typeName := typeOf(propSchema)
	if typeName == "array" {
		if items, ok := propSchema["items"].(map[string]any); ok {
			if itemType := typeOf(items); itemType != "" {
				typeName = "array<" + itemType + ">"
			}
		}
	}
	desc := collapseWhitespace(stringField(propSchema, "description"))
	if enum, ok := propSchema["enum"].([]any); ok && len(enum) > 0 {
		desc = appendSentence(desc, "One of: "+formatEnum(enum)+".")
	}
	if def, ok := propSchema["default"]; ok {
		desc = appendSentence(desc, fmt.Sprintf("Default: `%v`.", def))
	}
	if required {
		desc = appendSentence(desc, "**(required)**")
	}
	return fmt.Sprintf("| `%s` | `%s` | %s |", name, typeName, escapePipes(desc))
}

// writeSchemaExamples renders the schema's top-level 'examples' array as a
// fenced yaml code block. Each example is a yaml.MapSlice so author-defined
// field order survives the round-trip and the rendered YAML matches the
// source-of-truth format operators write.
func writeSchemaExamples(w io.Writer, examples []yaml.MapSlice) {
	if len(examples) == 0 {
		return
	}
	fmt.Fprintln(w, "## Examples")
	fmt.Fprintln(w)
	for _, ex := range examples {
		body, err := yaml.Marshal(ex)
		if err != nil {
			continue
		}
		fmt.Fprintln(w, "```yaml")
		fmt.Fprint(w, string(body))
		fmt.Fprintln(w, "```")
		fmt.Fprintln(w)
	}
}

// writeSchemaSeeAlso emits the See also section. seealso is the raw markdown
// body of the sidecar (already formatted as bullets); we append the Source
// schema link as an additional bullet so every schema page links back to its
// authoritative source file. Skipped entirely when both inputs are empty.
func writeSchemaSeeAlso(w io.Writer, seealso, sourcePath string) {
	seealso = strings.TrimRight(seealso, "\n")
	if seealso == "" && sourcePath == "" {
		return
	}
	fmt.Fprintln(w, "## See also")
	fmt.Fprintln(w)
	if seealso != "" {
		fmt.Fprintln(w, seealso)
	}
	if sourcePath != "" {
		fmt.Fprintf(w, "- Source schema: [%s](%s%s)\n", sourcePath, schemaSourceURLPrefix, sourcePath)
	}
}

// summarize returns a single-line description for frontmatter use. It collapses
// internal whitespace (block-scalar newlines and runs of spaces) into single
// spaces, then truncates at the first sentence boundary (period followed by
// space or end-of-string) so the SEO summary stays compact without losing
// mid-sentence content the way firstLine() would.
func summarize(s string) string {
	s = collapseWhitespace(s)
	if s == "" {
		return ""
	}
	if i := strings.Index(s, ". "); i >= 0 {
		return s[:i+1]
	}
	return s
}

// collapseWhitespace turns every run of whitespace (including newlines from
// YAML block scalars) into a single space. Used so multi-line description
// strings can render in a single-cell markdown table row without breaking the
// table layout, and so frontmatter strings stay on one line.
func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// =============================================================================
// helpers
// =============================================================================

func typeOf(schema map[string]any) string {
	t, _ := schema["type"].(string)
	return t
}

func hasProperties(schema map[string]any) bool {
	props, ok := schema["properties"].(map[string]any)
	return ok && len(props) > 0
}

func requiredSet(schema map[string]any) map[string]bool {
	out := map[string]bool{}
	req, ok := schema["required"].([]any)
	if !ok {
		return out
	}
	for _, r := range req {
		if s, ok := r.(string); ok {
			out[s] = true
		}
	}
	return out
}

// propertyNames returns property keys in the order operators expect: required
// fields first (in the order they appear in the schema's 'required' array),
// then the rest alphabetically. JSON Schema does not preserve property
// insertion order after yaml.Unmarshal, so sorting is the only stable choice
// for optional fields; required-first matches the convention from the hand-
// written reference set and puts the must-set fields at the top of every table.
func propertyNames(props map[string]any, requiredOrder []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(props))
	for _, name := range requiredOrder {
		if _, ok := props[name]; ok && !seen[name] {
			out = append(out, name)
			seen[name] = true
		}
	}
	rest := make([]string, 0, len(props))
	for name := range props {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// requiredList returns the 'required' field as a string slice in the order it
// appears in the schema. Used to seed propertyNames so required-first ordering
// matches author intent.
func requiredList(schema map[string]any) []string {
	req, ok := schema["required"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(req))
	for _, r := range req {
		if s, ok := r.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func stringField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func formatEnum(values []any) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, fmt.Sprintf("`%v`", v))
	}
	return strings.Join(parts, ", ")
}

func appendSentence(desc, extra string) string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return extra
	}
	if !strings.HasSuffix(desc, ".") && !strings.HasSuffix(desc, "!") && !strings.HasSuffix(desc, "?") {
		desc += "."
	}
	return desc + " " + extra
}
