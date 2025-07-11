package generators

import (
	"fmt"
	"path/filepath"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
)

// The BlueprintGenerator is a specialized component that manages blueprint configuration files.
// It processes the "blueprint" key from template data and generates blueprint.yaml files
// according to the blueprint schema. The BlueprintGenerator ensures proper blueprint structure
// and validates the generated content against the v1alpha1 Blueprint schema.

// =============================================================================
// Types
// =============================================================================

// BlueprintGenerator is a generator that writes blueprint.yaml files
type BlueprintGenerator struct {
	BaseGenerator
}

// =============================================================================
// Constructor
// =============================================================================

// NewBlueprintGenerator creates a new BlueprintGenerator with the provided dependency injector.
// It initializes the base generator and prepares it for blueprint file generation.
func NewBlueprintGenerator(injector di.Injector) *BlueprintGenerator {
	return &BlueprintGenerator{
		BaseGenerator: *NewGenerator(injector),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Write generates blueprint configuration files by delegating to the Generate method.
// It maintains backward compatibility while Generate handles the actual file generation.
// The overwrite parameter controls whether existing blueprint.yaml files should be overwritten.
func (g *BlueprintGenerator) Write(overwrite ...bool) error {
	return g.Generate(nil, overwrite...)
}

// Generate writes blueprint.yaml files from template data containing the "blueprint" key.
// Processes the blueprint data according to the v1alpha1 Blueprint schema and writes
// the resulting YAML file to the current context directory. The data parameter must
// contain a "blueprint" key with the blueprint configuration. The overwrite parameter
// controls whether existing blueprint.yaml files are overwritten. Returns an error if
// blueprint data is invalid, marshaling fails, or file operations fail.
func (g *BlueprintGenerator) Generate(data map[string]any, overwrite ...bool) error {
	shouldOverwrite := false
	if len(overwrite) > 0 {
		shouldOverwrite = overwrite[0]
	}

	if data == nil {
		return nil
	}

	blueprintData, exists := data["blueprint"]
	if !exists {
		return nil
	}

	blueprintMap, ok := blueprintData.(map[string]any)
	if !ok {
		return fmt.Errorf("blueprint data must be a map[string]any, got %T", blueprintData)
	}

	contextPath, err := g.configHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("failed to get config root: %w", err)
	}

	blueprintFilePath := filepath.Join(contextPath, "blueprint.yaml")

	if !shouldOverwrite {
		if _, err := g.shims.Stat(blueprintFilePath); err == nil {
			return nil
		}
	}

	blueprint, err := g.createBlueprintFromData(blueprintMap)
	if err != nil {
		return fmt.Errorf("failed to create blueprint from data: %w", err)
	}

	yamlData, err := g.shims.MarshalYAML(blueprint)
	if err != nil {
		return fmt.Errorf("failed to marshal blueprint to YAML: %w", err)
	}

	if err := g.shims.MkdirAll(filepath.Dir(blueprintFilePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := g.shims.WriteFile(blueprintFilePath, yamlData, 0644); err != nil {
		return fmt.Errorf("failed to write blueprint.yaml: %w", err)
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// createBlueprintFromData converts a map[string]any template data into a Blueprint struct using YAML marshaling and unmarshaling.
// It ensures the resulting Blueprint has the correct Kind and ApiVersion fields set. Returns the constructed Blueprint or an error if marshaling or unmarshaling fails.
func (g *BlueprintGenerator) createBlueprintFromData(data map[string]any) (*blueprintv1alpha1.Blueprint, error) {
	yamlData, err := g.shims.MarshalYAML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data to YAML: %w", err)
	}

	blueprint := &blueprintv1alpha1.Blueprint{
		Kind:       "Blueprint",
		ApiVersion: "v1alpha1",
	}

	if err := g.shims.YamlUnmarshal(yamlData, blueprint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML to Blueprint: %w", err)
	}

	return blueprint, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure BlueprintGenerator implements Generator
var _ Generator = (*BlueprintGenerator)(nil)
