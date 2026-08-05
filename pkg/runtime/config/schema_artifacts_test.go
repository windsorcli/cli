package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// loadArtifactValidator loads a production schema artifact from schemas/artifacts/ into a fresh
// SchemaValidator, so tests exercise the exact JSON schema the CLI validates user config against.
func loadArtifactValidator(t *testing.T, artifact string) *SchemaValidator {
	t.Helper()
	v := NewSchemaValidator(shell.NewMockShell())
	content, err := os.ReadFile(filepath.Join("schemas", "artifacts", artifact))
	if err != nil {
		t.Fatalf("read %s: %v", artifact, err)
	}
	if err := v.LoadSchemaFromBytes(content); err != nil {
		t.Fatalf("load %s: %v", artifact, err)
	}
	return v
}

// TestSchemaArtifacts_ConfigurationFieldsPresent guards against a Go struct field existing
// without a corresponding property in the strict (additionalProperties: false) configuration
// schema artifact, which silently rejects valid user config at validation time.
func TestSchemaArtifacts_ConfigurationFieldsPresent(t *testing.T) {
	t.Run("azure.region validates", func(t *testing.T) {
		// Given the production configuration schema artifact
		validator := loadArtifactValidator(t, "configuration.yaml")

		// When validating a context that sets azure.region
		result, err := validator.Validate(map[string]any{
			"version": "v1alpha1",
			"contexts": map[string]any{
				"test": map[string]any{
					"azure": map[string]any{"region": "eastus"},
				},
			},
		})

		// Then it validates without error
		if err != nil {
			t.Fatalf("Validate returned error: %v", err)
		}
		if !result.Valid {
			t.Errorf("expected valid, got errors: %v", result.Errors)
		}
	})

	t.Run("azure rejects unknown fields", func(t *testing.T) {
		// Given the production configuration schema artifact
		validator := loadArtifactValidator(t, "configuration.yaml")

		// When validating an azure block with an unrecognized field
		result, _ := validator.Validate(map[string]any{
			"version": "v1alpha1",
			"contexts": map[string]any{
				"test": map[string]any{
					"azure": map[string]any{"bogus": "nope"},
				},
			},
		})

		// Then it is rejected
		if result.Valid {
			t.Error("expected invalid for unknown azure field")
		}
	})
}

// TestSchemaArtifacts_BlueprintFieldsPresent guards against the same drift on the strict
// blueprint schema artifact for fields on FluxSystem that are real on the Go struct.
func TestSchemaArtifacts_BlueprintFieldsPresent(t *testing.T) {
	baseBlueprint := func(fluxEntry map[string]any) map[string]any {
		return map[string]any{
			"apiVersion": "blueprints.windsorcli.dev/v1alpha1",
			"kind":       "Blueprint",
			"metadata":   map[string]any{"name": "test"},
			"flux":       []any{fluxEntry},
		}
	}

	t.Run("flux.globalDependency validates", func(t *testing.T) {
		// Given the production blueprint schema artifact
		validator := loadArtifactValidator(t, "blueprint.yaml")

		// When validating a flux system with globalDependency set
		result, err := validator.Validate(baseBlueprint(map[string]any{
			"name":             "policy",
			"globalDependency": true,
		}))

		// Then it validates without error
		if err != nil {
			t.Fatalf("Validate returned error: %v", err)
		}
		if !result.Valid {
			t.Errorf("expected valid, got errors: %v", result.Errors)
		}
	})

	t.Run("flux.secrets validates with namespaces and data", func(t *testing.T) {
		// Given the production blueprint schema artifact
		validator := loadArtifactValidator(t, "blueprint.yaml")

		// When validating a flux system declaring a Secret with namespaces and data
		result, err := validator.Validate(baseBlueprint(map[string]any{
			"name": "policy",
			"secrets": map[string]any{
				"creds": map[string]any{
					"namespaces": []any{"policy-system"},
					"data":       map[string]any{"token": `${secret("foo")}`},
				},
			},
		}))

		// Then it validates without error
		if err != nil {
			t.Fatalf("Validate returned error: %v", err)
		}
		if !result.Valid {
			t.Errorf("expected valid, got errors: %v", result.Errors)
		}
	})

	t.Run("flux.secrets entry rejects unknown fields", func(t *testing.T) {
		// Given the production blueprint schema artifact
		validator := loadArtifactValidator(t, "blueprint.yaml")

		// When validating a secrets entry with an unrecognized field
		result, _ := validator.Validate(baseBlueprint(map[string]any{
			"name": "policy",
			"secrets": map[string]any{
				"creds": map[string]any{"bogus": "nope"},
			},
		}))

		// Then it is rejected
		if result.Valid {
			t.Error("expected invalid for unknown secrets entry field")
		}
	})

	t.Run("flux rejects unknown fields", func(t *testing.T) {
		// Given the production blueprint schema artifact
		validator := loadArtifactValidator(t, "blueprint.yaml")

		// When validating a flux system with an unrecognized field
		result, _ := validator.Validate(baseBlueprint(map[string]any{
			"name":  "policy",
			"bogus": "nope",
		}))

		// Then it is rejected
		if result.Valid {
			t.Error("expected invalid for unknown flux field")
		}
	})
}
