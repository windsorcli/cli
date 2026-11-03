package config

import (
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestSchemaValidator_mergeSchema(t *testing.T) {
	t.Run("MergesPropertiesRecursively", func(t *testing.T) {
		// Given a schema validator
		validator := NewSchemaValidator(shell.NewMockShell())

		base := map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type":    "string",
					"default": "docker",
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
		validator := NewSchemaValidator(shell.NewMockShell())

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
		validator := NewSchemaValidator(shell.NewMockShell())

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
		validator := NewSchemaValidator(shell.NewMockShell())

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
		validator := NewSchemaValidator(shell.NewMockShell())

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
		validator := NewSchemaValidator(shell.NewMockShell())

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
		validator := NewSchemaValidator(shell.NewMockShell())

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
		validator := NewSchemaValidator(shell.NewMockShell())

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
		validator := NewSchemaValidator(shell.NewMockShell())

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
		validator := NewSchemaValidator(shell.NewMockShell())

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
		validator := NewSchemaValidator(shell.NewMockShell())

		// When merging with non-map inputs
		merged := validator.mergeItemsSchema("not-a-map", map[string]any{"type": "string"})

		// Then should return overlay
		if merged["type"] != "string" {
			t.Error("Expected overlay to be returned")
		}
	})
}
