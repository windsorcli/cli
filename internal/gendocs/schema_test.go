package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestCollapseWhitespace(t *testing.T) {
	t.Run("LeavesSingleSpacedTextAlone", func(t *testing.T) {
		// Given normal prose
		// When collapseWhitespace is called
		got := collapseWhitespace("hello world")
		// Then it returns the input unchanged
		if got != "hello world" {
			t.Errorf("got %q, want %q", got, "hello world")
		}
	})

	t.Run("FlattensBlockScalarNewlines", func(t *testing.T) {
		// Given a YAML block-scalar style multi-line string
		// When collapseWhitespace is called
		got := collapseWhitespace("Semver constraint for the required CLI version.\nWhen set, the CLI validates\nthat its current version satisfies this constraint.")
		// Then internal newlines and runs of spaces become single spaces
		want := "Semver constraint for the required CLI version. When set, the CLI validates that its current version satisfies this constraint."
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("CollapsesRunsOfSpaces", func(t *testing.T) {
		// Given text with runs of internal whitespace
		// When collapseWhitespace is called
		got := collapseWhitespace("  many    spaces   between\t\twords  ")
		// Then runs collapse and outer whitespace is trimmed
		if got != "many spaces between words" {
			t.Errorf("got %q, want %q", got, "many spaces between words")
		}
	})

	t.Run("EmptyAndWhitespaceOnlyReturnEmpty", func(t *testing.T) {
		// Given empty and whitespace-only inputs
		for _, in := range []string{"", "  ", "\n\n\t"} {
			if got := collapseWhitespace(in); got != "" {
				t.Errorf("collapseWhitespace(%q) = %q, want empty", in, got)
			}
		}
	})
}

func TestSummarize(t *testing.T) {
	t.Run("TakesFirstSentenceWhenLongDescription", func(t *testing.T) {
		// Given a multi-sentence description with block-scalar newlines (the
		// real-world metadata.yaml case that motivated this helper)
		intro := "Optional metadata.yaml file that ships alongside a blueprint at\ncontexts/_template/metadata.yaml. Used by 'windsor bundle' and 'windsor push'\nto derive the artifact name and tag."

		// When summarize is called
		got := summarize(intro)

		// Then it returns the first sentence as a single line
		want := "Optional metadata.yaml file that ships alongside a blueprint at contexts/_template/metadata.yaml."
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("ReturnsWholeStringWhenSingleSentence", func(t *testing.T) {
		// Given a short, single-sentence description
		// When summarize is called
		got := summarize("Blueprint name.")
		// Then it returns the whole string
		if got != "Blueprint name." {
			t.Errorf("got %q, want %q", got, "Blueprint name.")
		}
	})

	t.Run("EmptyInputReturnsEmpty", func(t *testing.T) {
		if got := summarize(""); got != "" {
			t.Errorf("summarize empty input = %q, want empty", got)
		}
	})
}

func TestPropertyNames(t *testing.T) {
	t.Run("RequiredFirstInOrderThenAlphabetical", func(t *testing.T) {
		// Given properties with explicit required ordering and other keys mixed in
		props := map[string]any{
			"author":      nil,
			"cliVersion":  nil,
			"description": nil,
			"name":        nil,
			"version":     nil,
		}
		requiredOrder := []string{"name", "version"}

		// When propertyNames is called
		got := propertyNames(props, requiredOrder)

		// Then required-listed names appear first in their declared order, then
		// the remaining keys alphabetically
		want := []string{"name", "version", "author", "cliVersion", "description"}
		if !equalSlices(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("AllOptionalReturnsAlphabetical", func(t *testing.T) {
		// Given no required fields
		props := map[string]any{"zeta": nil, "alpha": nil, "beta": nil}

		// When propertyNames is called
		got := propertyNames(props, nil)

		// Then output is alphabetical
		want := []string{"alpha", "beta", "zeta"}
		if !equalSlices(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("RequiredNotInPropsIsSkipped", func(t *testing.T) {
		// Given a required entry that does not appear in properties (a schema
		// authoring bug we should not mask)
		props := map[string]any{"name": nil, "tags": nil}
		requiredOrder := []string{"name", "nonexistent"}

		// When propertyNames is called
		got := propertyNames(props, requiredOrder)

		// Then only the present-in-props required name is emitted; the missing
		// one is silently dropped (it has no row to render anyway)
		want := []string{"name", "tags"}
		if !equalSlices(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

func TestSchemaFieldRow(t *testing.T) {
	t.Run("BasicStringFieldRendersDescription", func(t *testing.T) {
		// Given a simple string property
		propSchema := map[string]any{"type": "string", "description": "Blueprint name."}

		// When schemaFieldRow is called for a non-required field
		got := schemaFieldRow("name", propSchema, false)

		// Then the row contains the name, type, and description
		want := "| `name` | `string` | Blueprint name. |"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("RequiredFieldAppendsBoldMarker", func(t *testing.T) {
		// Given the same field marked required
		propSchema := map[string]any{"type": "string", "description": "Blueprint name."}

		// When schemaFieldRow is called with required=true
		got := schemaFieldRow("name", propSchema, true)

		// Then the description ends with the bold (required) marker
		if !strings.Contains(got, "**(required)**") {
			t.Errorf("expected required marker in row, got %q", got)
		}
	})

	t.Run("EnumAppendsOneOfClause", func(t *testing.T) {
		// Given a string field with an enum constraint
		propSchema := map[string]any{
			"type":        "string",
			"description": "Target platform.",
			"enum":        []any{"aws", "azure", "gcp"},
		}

		// When schemaFieldRow is called
		got := schemaFieldRow("platform", propSchema, false)

		// Then the description includes the enum values in backticks
		if !strings.Contains(got, "One of: `aws`, `azure`, `gcp`.") {
			t.Errorf("expected enum clause in row, got %q", got)
		}
	})

	t.Run("DefaultAppendsBacktickedValue", func(t *testing.T) {
		// Given a boolean field with a default
		propSchema := map[string]any{
			"type":        "boolean",
			"description": "Enable DNS.",
			"default":     true,
		}

		// When schemaFieldRow is called
		got := schemaFieldRow("dns", propSchema, false)

		// Then the description includes the default in backticks
		if !strings.Contains(got, "Default: `true`.") {
			t.Errorf("expected default clause in row, got %q", got)
		}
	})

	t.Run("ArrayOfStringRendersTypedType", func(t *testing.T) {
		// Given an array property with string items
		propSchema := map[string]any{
			"type":        "array",
			"description": "Tags.",
			"items":       map[string]any{"type": "string"},
		}

		// When schemaFieldRow is called
		got := schemaFieldRow("tags", propSchema, false)

		// Then the type column shows the element type
		if !strings.Contains(got, "`array<string>`") {
			t.Errorf("expected typed array in row, got %q", got)
		}
	})

	t.Run("EscapesPipesInDescription", func(t *testing.T) {
		// Given a description containing a pipe character (enum-style)
		propSchema := map[string]any{"type": "string", "description": "Format: a|b|c."}

		// When schemaFieldRow is called
		got := schemaFieldRow("fmt", propSchema, false)

		// Then pipes are escaped so the markdown table layout survives
		if strings.Contains(got, "a|b|c") {
			t.Errorf("expected pipes to be escaped, got %q", got)
		}
		if !strings.Contains(got, `a\|b\|c`) {
			t.Errorf("expected escaped pipes, got %q", got)
		}
	})
}

func TestRenderSchema(t *testing.T) {
	t.Run("FullEndToEnd", func(t *testing.T) {
		// Given a minimal but representative schema with title, intro, fields,
		// required, examples, and a See Also sidecar
		schema := map[string]any{
			"title":       "Metadata",
			"description": "Optional metadata.yaml file. Used by bundle and push.",
			"type":        "object",
			"required":    []any{"name"},
			"properties": map[string]any{
				"name":    map[string]any{"type": "string", "description": "Blueprint name."},
				"version": map[string]any{"type": "string", "description": "Blueprint version."},
			},
		}
		examples := []yaml.MapSlice{
			{
				yaml.MapItem{Key: "name", Value: "my-blueprint"},
				yaml.MapItem{Key: "version", Value: "1.0.0"},
			},
		}
		sourcePath := "pkg/runtime/config/schemas/metadata.yaml"
		seealso := "- [`bundle`](commands/bundle.md)\n"

		// When renderSchema runs
		var buf bytes.Buffer
		if err := renderSchema(&buf, schema, examples, sourcePath, seealso); err != nil {
			t.Fatalf("renderSchema: %v", err)
		}
		out := buf.String()

		// Then the output contains every expected section in order
		for _, want := range []string{
			`title: "Metadata"`,
			"# Metadata\n",
			"## Fields\n",
			"| `name` | `string` | Blueprint name. **(required)** |",
			"| `version` | `string` | Blueprint version. |",
			"## Examples\n",
			"```yaml",
			"name: my-blueprint",
			"version: 1.0.0",
			"## See also\n",
			"- [`bundle`](commands/bundle.md)",
			"- Source schema: [pkg/runtime/config/schemas/metadata.yaml]",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("expected output to contain %q", want)
			}
		}
	})

	t.Run("ReturnsWriteErrorFromBrokenWriter", func(t *testing.T) {
		// Given a writer that fails on first write
		schema := map[string]any{"title": "X", "type": "object"}

		// When renderSchema is given the broken writer
		err := renderSchema(brokenWriter{err: errSentinel}, schema, nil, "", "")

		// Then the captured error surfaces (via errWriter in render.go)
		if err == nil {
			t.Error("expected non-nil error from broken writer")
		}
	})
}

func TestRenderSchemaHeadingDepthLadder(t *testing.T) {
	t.Run("HeadingLevelTracksNestingDepthUpToH4", func(t *testing.T) {
		// Given a schema with three levels of nested object properties:
		//   root -> outer -> inner -> deepest
		schema := map[string]any{
			"title": "Nested",
			"type":  "object",
			"properties": map[string]any{
				"outer": map[string]any{
					"type":        "object",
					"description": "outer object",
					"properties": map[string]any{
						"flat": map[string]any{"type": "string", "description": "flat field"},
						"inner": map[string]any{
							"type":        "object",
							"description": "inner object",
							"properties": map[string]any{
								"deepest": map[string]any{
									"type":        "object",
									"description": "deepest object",
									"properties": map[string]any{
										"k": map[string]any{"type": "string", "description": "k"},
									},
								},
							},
						},
					},
				},
			},
		}

		// When renderSchema runs
		var buf bytes.Buffer
		if err := renderSchema(&buf, schema, nil, "", ""); err != nil {
			t.Fatalf("renderSchema: %v", err)
		}
		out := buf.String()

		// Then each nesting depth gets a distinct heading level:
		//   ## Fields           (top-level)
		//   ## outer            (depth 1, same as top per design)
		//   ### outer.inner     (depth 2)
		//   #### outer.inner.deepest  (depth 3, capped at h4)
		for _, want := range []string{
			"## Fields\n",
			"## outer\n",
			"### outer.inner\n",
			"#### outer.inner.deepest\n",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("expected heading %q in output", want)
			}
		}
	})
}

func TestGenerateSchemaDocs(t *testing.T) {
	t.Run("EmitsMarkdownForEachSchemaPlusSidecar", func(t *testing.T) {
		// Given a temp schema directory with a schema and its sidecar
		schemaDir := t.TempDir()
		outDir := t.TempDir()
		writeFile(t, filepath.Join(schemaDir, "metadata.yaml"),
			"title: Metadata\ntype: object\nproperties:\n  name:\n    type: string\n    description: A name.\n")
		writeFile(t, filepath.Join(schemaDir, "metadata.seealso.md"),
			"- [`bundle`](commands/bundle.md)\n")

		// When generateSchemaDocs runs
		if err := generateSchemaDocs(schemaDir, outDir); err != nil {
			t.Fatalf("generateSchemaDocs: %v", err)
		}

		// Then metadata.md exists with the sidecar's See Also content
		body := readFile(t, filepath.Join(outDir, "metadata.md"))
		if !strings.Contains(body, "## See also") {
			t.Error("expected See also section sourced from sidecar")
		}
		if !strings.Contains(body, "[`bundle`](commands/bundle.md)") {
			t.Error("expected sidecar content in See also")
		}
	})

	t.Run("MissingSidecarRendersWithoutSeeAlsoCustomLines", func(t *testing.T) {
		// Given a schema with no companion .seealso.md
		schemaDir := t.TempDir()
		outDir := t.TempDir()
		writeFile(t, filepath.Join(schemaDir, "alone.yaml"),
			"title: Alone\ntype: object\nproperties:\n  k:\n    type: string\n")

		// When generateSchemaDocs runs
		if err := generateSchemaDocs(schemaDir, outDir); err != nil {
			t.Fatalf("generateSchemaDocs: %v", err)
		}

		// Then See also still appears (with just the Source schema link),
		// but no sidecar bullets
		body := readFile(t, filepath.Join(outDir, "alone.md"))
		if !strings.Contains(body, "## See also") {
			t.Error("expected See also section (for Source schema link)")
		}
		if !strings.Contains(body, "Source schema:") {
			t.Error("expected Source schema bullet")
		}
	})
}

// =============================================================================
// helpers
// =============================================================================

// errSentinel is the canonical error used to assert that renderSchema surfaces
// write failures rather than swallowing them.
var errSentinel = sentinelErr{}

type sentinelErr struct{}

func (sentinelErr) Error() string { return "sentinel write error" }

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
