// The schema generator writes a markdown reference page for each JSON Schema file in a directory.
// It provides the `gendocs schema` subcommand, which writes docs/reference/<name>.md for each <name>.yaml.
// A page has frontmatter, the schema description, the field tables, the examples and a See also section.
// An optional <name>.seealso.md file next to the schema holds the See also links, so the schema needs no vendor extensions.
//
// The renderer supports objects, arrays, maps, the primitive types, enum, default, description and local $ref values.
// It does not support oneOf or allOf. A property without a type shows as "any".

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

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constants
// =============================================================================

// schemaSourceURLPrefix is the GitHub URL prefix for the source schema link.
const schemaSourceURLPrefix = "https://github.com/windsorcli/cli/blob/main/"

// =============================================================================
// Types
// =============================================================================

// nestedSection is a nested schema that writeSchemaFields renders as a subsection, with its heading label.
type nestedSection struct {
	label  string
	schema map[string]any
}

// =============================================================================
// Constructor
// =============================================================================

// schemaCmd returns the `gendocs schema` subcommand. The --in flag sets the schema directory and --out sets the output directory.
// The default input is schemas/artifacts/, not schemas/, because the runtime validator embeds schemas/*.yaml.
// An artifact schema in schemas/ would add its fields to the windsor.yaml validation and reject valid configs.
func schemaCmd() *cobra.Command {
	var outDir, schemaDir string
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Generate markdown reference for Windsor YAML schemas",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return generateSchemaDocs(schemaDir, outDir)
		},
	}
	cmd.Flags().StringVar(&schemaDir, "in", "pkg/runtime/config/schemas/artifacts", "directory of *.yaml schemas to read")
	cmd.Flags().StringVar(&outDir, "out", "docs/reference", "directory to write *.md output into")
	return cmd
}

// =============================================================================
// Private Methods
// =============================================================================

// generateSchemaDocs writes one page into outDir for each *.yaml file in schemaDir.
// It reads the See also text from <name>.seealso.md next to each schema, when that file exists.
// It does not delete outDir, because other reference pages, such as commands/ and contexts.md, are also in it.
func generateSchemaDocs(schemaDir, outDir string) error {
	entries, err := os.ReadDir(schemaDir)
	if err != nil {
		return werror.Wrap(err, "read schema dir")
	}
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return werror.Wrap(err, "create out dir")
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}

		schemaPath := filepath.Join(schemaDir, name)
		base := strings.TrimSuffix(name, ".yaml")
		outPath := filepath.Join(outDir, base+".md")
		seealsoPath := filepath.Join(schemaDir, base+".seealso.md")
		if err := writeSchemaFile(schemaPath, outPath, seealsoPath); err != nil {
			return werror.Wrap(err, "emit %s", schemaPath)
		}
	}
	return nil
}

// writeSchemaFile reads the schema at schemaPath and writes its page to outPath.
// It parses the examples a second time as yaml.MapSlice, because a map loses the field order of the author.
// It reads the sidecar at seealsoPath when the file exists.
func writeSchemaFile(schemaPath, outPath, seealsoPath string) error {
	raw, err := os.ReadFile(schemaPath) // #nosec G304 - schemaPath is operator-supplied via --in; filename derives from the directory listing
	if err != nil {
		return werror.Wrap(err, "read %s", schemaPath)
	}
	var schema map[string]any
	if err := yaml.Unmarshal(raw, &schema); err != nil {
		return werror.Wrap(err, "parse %s", schemaPath)
	}
	var ordered struct {
		Examples []yaml.MapSlice `yaml:"examples"`
	}
	if err := yaml.Unmarshal(raw, &ordered); err != nil {
		return werror.Wrap(err, "parse %s examples", schemaPath)
	}

	var seealso string
	if seealsoPath != "" {
		if body, err := os.ReadFile(seealsoPath); err == nil { // #nosec G304 - seealsoPath is derived from --in plus the schema basename
			seealso = string(body)
		} else if !os.IsNotExist(err) {
			return werror.Wrap(err, "read sidecar %s", seealsoPath)
		}
	}

	f, err := os.Create(outPath) // #nosec G304 - outPath is constructed from --out plus the schema basename
	if err != nil {
		return werror.Wrap(err, "create %s", outPath)
	}
	defer f.Close()

	relSource, err := filepath.Rel(".", schemaPath)
	if err != nil {
		relSource = schemaPath
	}
	if err := renderSchema(f, schema, ordered.Examples, relSource, seealso); err != nil {
		return werror.Wrap(err, "render %s", outPath)
	}
	return nil
}

// renderSchema writes the page for schema to w, in the same section order as a command page.
// examples keeps the field order of the author, and seealso is the text of the sidecar file or empty.
// It passes the root schema to each recursion, so local $ref values resolve without a second read of the file.
func renderSchema(w io.Writer, schema map[string]any, examples []yaml.MapSlice, sourcePath, seealso string) error {
	ew := &errWriter{w: w}

	title, _ := schema["title"].(string)
	if title == "" {
		title = "Schema"
	}
	intro, _ := schema["description"].(string)

	writeSchemaFrontmatter(ew, title, summarize(intro))
	fmt.Fprintln(ew)
	if intro != "" {
		fmt.Fprintln(ew, strings.TrimSpace(intro))
		fmt.Fprintln(ew)
	}

	writeSchemaFields(ew, schema, "", schema)
	writeSchemaExamples(ew, examples)
	writeSchemaSeeAlso(ew, seealso, sourcePath)
	return ew.err
}

// writeSchemaFrontmatter writes the frontmatter of a schema page with title and description.
func writeSchemaFrontmatter(w io.Writer, title, description string) {
	fmt.Fprintln(w, "---")
	fmt.Fprintf(w, "title: %q\n", title)
	if description != "" {
		fmt.Fprintf(w, "description: %q\n", description)
	}
	fmt.Fprintln(w, "---")
}

// writeSchemaFields writes the field table of an object schema, then a subsection for each nested object.
// headingPath is the path of the subsection, such as "repository". An array of objects adds "[]" and a map of objects adds "{}".
// The top-level table and the first nested level use ##. Each "." in headingPath adds one #, up to ####.
// root is the document root, so local $ref values resolve at each depth.
func writeSchemaFields(w io.Writer, schema map[string]any, headingPath string, root map[string]any) {
	schema = resolveRef(schema, root)
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
	depth := strings.Count(headingPath, ".") + 2
	if depth > 4 {
		depth = 4
	}
	level := strings.Repeat("#", depth)
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
		propSchema = resolveRef(propSchema, root)
		fmt.Fprintln(w, schemaFieldRow(name, propSchema, required[name], root))
		if sub, label := nestedSchemaFor(propSchema, name, headingPath, root); sub != nil {
			nested = append(nested, nestedSection{label: label, schema: sub})
		}
	}
	fmt.Fprintln(w)

	for _, n := range nested {
		writeSchemaFields(w, n.schema, n.label, root)
	}
}

// nestedSchemaFor returns the schema and heading label of the subsection for a property.
// An object with properties gives its own schema. An array of objects gives its items, with "[]" after the label.
// A map of objects gives its value schema, with "{}" after the label.
// It returns nil for a primitive, an array of primitives or an empty object.
func nestedSchemaFor(propSchema map[string]any, name, headingPath string, root map[string]any) (map[string]any, string) {
	base := name
	if headingPath != "" {
		base = headingPath + "." + name
	}
	if typeOf(propSchema) == "object" {
		if hasProperties(propSchema) {
			return propSchema, base
		}
		if addProps, ok := propSchema["additionalProperties"].(map[string]any); ok {
			addProps = resolveRef(addProps, root)
			if typeOf(addProps) == "object" && hasProperties(addProps) {
				return addProps, base + "{}"
			}
		}
	}
	if typeOf(propSchema) == "array" {
		items, _ := propSchema["items"].(map[string]any)
		if items == nil {
			return nil, ""
		}
		items = resolveRef(items, root)
		if typeOf(items) == "object" && hasProperties(items) {
			return items, base + "[]"
		}
	}
	return nil, ""
}

// resolveRef returns the schema that a local '$ref: #/$defs/<name>' points to, merged with the other fields of schema.
// JSON Schema 2020-12 allows fields such as description next to $ref.
// It returns schema unchanged when there is no local $ref or the lookup fails. It does not support remote refs.
func resolveRef(schema, root map[string]any) map[string]any {
	if schema == nil {
		return nil
	}
	ref, _ := schema["$ref"].(string)
	if ref == "" || !strings.HasPrefix(ref, "#/$defs/") {
		return schema
	}
	defs, _ := root["$defs"].(map[string]any)
	if defs == nil {
		return schema
	}
	target, _ := defs[strings.TrimPrefix(ref, "#/$defs/")].(map[string]any)
	if target == nil {
		return schema
	}
	merged := make(map[string]any, len(target)+len(schema))
	for k, v := range target {
		merged[k] = v
	}
	for k, v := range schema {
		if k == "$ref" {
			continue
		}
		merged[k] = v
	}
	return merged
}

// schemaFieldRow returns one row of the field table.
// The Type column shows "array<type>" for an array and "map<type>" for an object with an additionalProperties schema.
// It shows "any" when the schema has no type, because JSON Schema then accepts any value.
// The Description column adds the enum values, the default and "**(required)**".
// It leaves out an enum with one value, because the description already names that value.
// root is the document root, so a local $ref on the items or the values resolves.
func schemaFieldRow(name string, propSchema map[string]any, required bool, root map[string]any) string {
	typeName := typeOf(propSchema)
	if typeName == "" {
		typeName = "any"
	}
	if typeName == "array" {
		if items, ok := propSchema["items"].(map[string]any); ok {
			items = resolveRef(items, root)
			if itemType := typeOf(items); itemType != "" {
				typeName = "array<" + itemType + ">"
			}
		}
	}
	if typeName == "object" {
		if addProps, ok := propSchema["additionalProperties"].(map[string]any); ok {
			addProps = resolveRef(addProps, root)
			if valType := typeOf(addProps); valType != "" {
				typeName = "map<" + valType + ">"
			}
		}
	}
	desc := collapseWhitespace(stringField(propSchema, "description"))
	if enum, ok := propSchema["enum"].([]any); ok && len(enum) > 1 {
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

// writeSchemaExamples writes each example of the schema in a yaml code block, in the field order of the author.
// It indents sequence items under their parent key, because a strict YAML 1.2 parser can reject them without the indent.
func writeSchemaExamples(w io.Writer, examples []yaml.MapSlice) {
	if len(examples) == 0 {
		return
	}
	fmt.Fprintln(w, "## Examples")
	fmt.Fprintln(w)
	for _, ex := range examples {
		body, err := yaml.MarshalWithOptions(ex, yaml.IndentSequence(true))
		if err != nil {
			continue
		}
		fmt.Fprintln(w, "```yaml")
		fmt.Fprint(w, string(body))
		fmt.Fprintln(w, "```")
		fmt.Fprintln(w)
	}
}

// writeSchemaSeeAlso writes the See also section from the sidecar text and a link to the source schema.
// It writes nothing when both are empty.
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

// =============================================================================
// Helpers
// =============================================================================

// summarize returns the first sentence of s on one line, for the frontmatter description.
// It changes each run of whitespace to one space before it finds the end of the sentence.
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

// collapseWhitespace changes each run of whitespace in s, including newlines, to one space.
// A multi-line description can then go in one table cell or on one frontmatter line.
func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// typeOf returns the JSON Schema type as display text.
// It accepts one type, or a list of types such as [boolean, string], which it joins with " / ".
// It does not use a pipe, because a pipe breaks the markdown table.
func typeOf(schema map[string]any) string {
	switch t := schema["type"].(type) {
	case string:
		return t
	case []any:
		parts := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " / ")
	}
	return ""
}

// hasProperties reports whether schema has at least one property.
func hasProperties(schema map[string]any) bool {
	props, ok := schema["properties"].(map[string]any)
	return ok && len(props) > 0
}

// requiredSet returns the names in the required field of schema as a set.
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

// propertyNames returns the property names with the required names first, in the order of the required field.
// It sorts the other names, because yaml.Unmarshal does not keep the order of the properties.
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

// requiredList returns the names in the required field of schema, in their order.
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

// stringField returns the string value of key in m, or an empty string.
func stringField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// formatEnum returns the enum values as a list of code spans, separated by commas.
func formatEnum(values []any) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, fmt.Sprintf("`%v`", v))
	}
	return strings.Join(parts, ", ")
}

// appendSentence adds extra to desc as a new sentence, and adds a period to desc when it has none.
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
