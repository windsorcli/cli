package config

import (
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestRenderValuesWithDescriptions(t *testing.T) {
	t.Run("ScalarWithDescription", func(t *testing.T) {
		values := map[string]any{"provider": "docker"}
		schema := map[string]any{
			"properties": map[string]any{
				"provider": map[string]any{
					"type":        "string",
					"description": "Cloud provider.",
				},
			},
		}
		out := RenderValuesWithDescriptions(values, schema)
		if !strings.Contains(out, "# Cloud provider.") {
			t.Errorf("expected description comment, got:\n%s", out)
		}
		if !strings.Contains(out, "provider: docker") {
			t.Errorf("expected value line, got:\n%s", out)
		}
	})

	t.Run("ScalarWithNoSchema", func(t *testing.T) {
		values := map[string]any{"provider": "docker"}
		out := RenderValuesWithDescriptions(values, nil)
		if strings.Contains(out, "#") {
			t.Errorf("expected no comments when schema is nil, got:\n%s", out)
		}
		if !strings.Contains(out, "provider: docker") {
			t.Errorf("expected value line, got:\n%s", out)
		}
	})

	t.Run("SchemaOnlyScalarRenderedCommentedOut", func(t *testing.T) {
		values := map[string]any{"enabled": true}
		schema := map[string]any{
			"properties": map[string]any{
				"enabled": map[string]any{"type": "boolean"},
				"driver": map[string]any{
					"type":        "string",
					"description": "Gateway driver.",
				},
			},
		}
		out := RenderValuesWithDescriptions(values, schema)
		if !strings.Contains(out, "enabled: true") {
			t.Errorf("expected set value, got:\n%s", out)
		}
		if !strings.Contains(out, "# Gateway driver.") {
			t.Errorf("expected description for unset field, got:\n%s", out)
		}
		if !strings.Contains(out, "# driver:") {
			t.Errorf("expected commented-out unset field, got:\n%s", out)
		}
		if strings.Contains(out, "\ndriver:") {
			t.Errorf("unset scalar should not appear as a live key, got:\n%s", out)
		}
	})

	t.Run("NestedObjectWithMixedSetAndUnset", func(t *testing.T) {
		values := map[string]any{
			"gateway": map[string]any{"enabled": true},
		}
		schema := map[string]any{
			"properties": map[string]any{
				"gateway": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"enabled": map[string]any{"type": "boolean", "description": "Enable gateway."},
						"driver":  map[string]any{"type": "string", "description": "Gateway driver."},
					},
				},
			},
		}
		out := RenderValuesWithDescriptions(values, schema)
		if !strings.Contains(out, "gateway:") {
			t.Errorf("expected gateway block, got:\n%s", out)
		}
		if !strings.Contains(out, "enabled: true") {
			t.Errorf("expected enabled value, got:\n%s", out)
		}
		if !strings.Contains(out, "# driver:") {
			t.Errorf("expected commented-out driver, got:\n%s", out)
		}
	})

	t.Run("SchemaOnlyObjectWithSubPropertiesShowsBlock", func(t *testing.T) {
		values := map[string]any{}
		schema := map[string]any{
			"properties": map[string]any{
				"cni": map[string]any{
					"type":        "object",
					"description": "CNI configuration.",
					"properties": map[string]any{
						"driver": map[string]any{"type": "string", "description": "CNI driver."},
					},
				},
			},
		}
		out := RenderValuesWithDescriptions(values, schema)
		if !strings.Contains(out, "cni:") {
			t.Errorf("expected cni block to appear, got:\n%s", out)
		}
		if !strings.Contains(out, "# CNI driver.") {
			t.Errorf("expected driver description, got:\n%s", out)
		}
		if !strings.Contains(out, "# driver:") {
			t.Errorf("expected commented-out driver, got:\n%s", out)
		}
	})

	t.Run("SchemaOnlyAdditionalPropertiesObjectRenderedAsCommentedKey", func(t *testing.T) {
		values := map[string]any{}
		schema := map[string]any{
			"properties": map[string]any{
				"registries": map[string]any{
					"type":                 "object",
					"description":          "Registry mirror config.",
					"additionalProperties": map[string]any{"type": "object"},
				},
			},
		}
		out := RenderValuesWithDescriptions(values, schema)
		if !strings.Contains(out, "# Registry mirror config.") {
			t.Errorf("expected description comment, got:\n%s", out)
		}
		if !strings.Contains(out, "# registries:") {
			t.Errorf("expected commented-out key, got:\n%s", out)
		}
	})

	t.Run("ArrayValue", func(t *testing.T) {
		values := map[string]any{
			"volumes": []any{"/var/mnt/local"},
		}
		schema := map[string]any{
			"properties": map[string]any{
				"volumes": map[string]any{"type": "array", "description": "Mount paths."},
			},
		}
		out := RenderValuesWithDescriptions(values, schema)
		if !strings.Contains(out, "# Mount paths.") {
			t.Errorf("expected array description, got:\n%s", out)
		}
		if !strings.Contains(out, "/var/mnt/local") {
			t.Errorf("expected array item, got:\n%s", out)
		}
	})

	t.Run("BooleanValue", func(t *testing.T) {
		values := map[string]any{"dev": true}
		out := RenderValuesWithDescriptions(values, nil)
		if !strings.Contains(out, "dev: true") {
			t.Errorf("expected boolean value, got:\n%s", out)
		}
	})

	t.Run("TopLevelKeysSeparatedByBlankLines", func(t *testing.T) {
		values := map[string]any{"a": "x", "b": "y"}
		out := RenderValuesWithDescriptions(values, nil)
		if !strings.Contains(out, "\n\n") {
			t.Errorf("expected blank line between top-level keys, got:\n%s", out)
		}
	})

	t.Run("EmptyValues", func(t *testing.T) {
		out := RenderValuesWithDescriptions(map[string]any{}, nil)
		if out != "" {
			t.Errorf("expected empty output for empty values, got:\n%s", out)
		}
	})

	t.Run("NilValues", func(t *testing.T) {
		out := RenderValuesWithDescriptions(nil, nil)
		if out != "" {
			t.Errorf("expected empty output for nil values and schema, got:\n%s", out)
		}
	})

	t.Run("MultiLineStringValue", func(t *testing.T) {
		values := map[string]any{"script": "line1\nline2\nline3"}
		out := RenderValuesWithDescriptions(values, nil)
		// The output must be valid YAML — a round-trip parse must recover the original value.
		var parsed map[string]any
		if err := yaml.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("multi-line value produced invalid YAML: %v\noutput:\n%s", err, out)
		}
		if parsed["script"] != "line1\nline2\nline3" {
			t.Errorf("expected multi-line string round-trip, got: %v", parsed["script"])
		}
	})
}

func TestRedactSensitiveValues(t *testing.T) {
	t.Run("RedactsExactPathPreservesSibling", func(t *testing.T) {
		// Given values with a sensitive leaf and a sibling
		values := map[string]any{
			"cdn": map[string]any{
				"cloudflare_api_key": "SUPERSECRET",
				"zone":               "example.com",
			},
		}

		// When redacting the sensitive path
		RedactSensitiveValues(values, []string{"cdn.cloudflare_api_key"})

		// Then the value is replaced with the marker and the sibling is untouched
		cdn := values["cdn"].(map[string]any)
		if cdn["cloudflare_api_key"] != SensitiveRedactionMarker {
			t.Errorf("Expected redaction marker, got %v", cdn["cloudflare_api_key"])
		}
		if cdn["zone"] != "example.com" {
			t.Errorf("Expected sibling preserved, got %v", cdn["zone"])
		}
	})

	t.Run("RedactsWildcardLeavesAcrossKeys", func(t *testing.T) {
		// Given a dynamic-key map with a sensitive leaf under each entry
		values := map[string]any{
			"stores": map[string]any{
				"a": map[string]any{"token": "atok", "url": "https://a"},
				"b": map[string]any{"token": "btok"},
			},
		}

		// When redacting the wildcard path
		RedactSensitiveValues(values, []string{"stores.*.token"})

		// Then every store's token is redacted and non-sensitive siblings remain
		stores := values["stores"].(map[string]any)
		if stores["a"].(map[string]any)["token"] != SensitiveRedactionMarker {
			t.Error("Expected stores.a.token redacted")
		}
		if stores["b"].(map[string]any)["token"] != SensitiveRedactionMarker {
			t.Error("Expected stores.b.token redacted")
		}
		if stores["a"].(map[string]any)["url"] != "https://a" {
			t.Error("Expected stores.a.url preserved")
		}
	})

	t.Run("AbsentPathIsNotFabricated", func(t *testing.T) {
		// Given values lacking the sensitive path
		values := map[string]any{"cdn": map[string]any{"zone": "example.com"}}

		// When redacting a path that is not present
		RedactSensitiveValues(values, []string{"cdn.cloudflare_api_key"})

		// Then no key is created
		if _, exists := values["cdn"].(map[string]any)["cloudflare_api_key"]; exists {
			t.Error("Expected absent sensitive key to not be fabricated")
		}
	})
}
