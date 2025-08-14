package generators

import (
	"fmt"
	"strings"

	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Types
// =============================================================================

// KustomizeGenerator is a generator that processes and generates kustomize files
type KustomizeGenerator struct {
	BaseGenerator
	blueprintHandler blueprint.BlueprintHandler
}

// =============================================================================
// Constructor
// =============================================================================

// NewKustomizeGenerator creates a new KustomizeGenerator with the provided dependency injector.
// It initializes the base generator and prepares it for kustomize file generation.
func NewKustomizeGenerator(injector di.Injector) *KustomizeGenerator {
	return &KustomizeGenerator{
		BaseGenerator: *NewGenerator(injector),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize sets up the KustomizeGenerator dependencies including the blueprint handler.
// Calls the base generator's Initialize method and then resolves the blueprint handler
// for kustomize-specific operations.
func (g *KustomizeGenerator) Initialize() error {
	if err := g.BaseGenerator.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize base generator: %w", err)
	}

	blueprintHandler := g.injector.Resolve("blueprintHandler")
	if blueprintHandler == nil {
		return fmt.Errorf("blueprint handler not found in dependency injector")
	}

	handler, ok := blueprintHandler.(blueprint.BlueprintHandler)
	if !ok {
		return fmt.Errorf("resolved blueprint handler is not of expected type")
	}

	g.blueprintHandler = handler
	return nil
}

// Generate processes kustomize template data and stores it in-memory for use during install.
// Filters data for kustomize-related keys and stores them in the blueprint handler
// instead of writing files to disk. This allows values and patches to be composed
// with user-defined files at install time.
// Returns an error if data is nil or if storing the data fails.
func (g *KustomizeGenerator) Generate(data map[string]any, overwrite ...bool) error {
	if data == nil {
		return fmt.Errorf("data cannot be nil")
	}

	kustomizeData := make(map[string]any)
	for key, values := range data {
		if strings.HasPrefix(key, "kustomize/") {
			if err := g.validateKustomizeData(key, values); err != nil {
				return fmt.Errorf("invalid kustomize data for key %s: %w", key, err)
			}
			kustomizeData[key] = values
		}
	}

	if len(kustomizeData) > 0 {
		g.blueprintHandler.SetRenderedKustomizeData(kustomizeData)
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// validateKustomizeData validates kustomize template data based on the key type.
// Validates patches as Kubernetes manifests and values for post-build substitution compatibility.
func (g *KustomizeGenerator) validateKustomizeData(key string, values any) error {
	if strings.HasPrefix(key, "kustomize/patches/") {
		valuesMap, ok := values.(map[string]any)
		if !ok {
			return fmt.Errorf("patch values must be a map, got %T", values)
		}
		return g.validateKubernetesManifest(valuesMap)
	}

	if key == "kustomize/values" {
		valuesMap, ok := values.(map[string]any)
		if !ok {
			return fmt.Errorf("values must be a map, got %T", values)
		}
		return g.validatePostBuildValues(valuesMap, "", 0)
	}

	return nil
}

// validatePostBuildValues checks if the values map is valid for Flux post-build substitution.
// Permitted types: string, numeric, boolean. Allows one map nesting if all nested values are scalar.
// Slices and nested complex types are not allowed. parentKey is for error reporting (e.g. "ingress.ip").
// depth tracks nesting (0 = top, 1 = one level deep). Returns error if unsupported type or excess nesting.
func (g *KustomizeGenerator) validatePostBuildValues(values map[string]any, parentKey string, depth int) error {
	for key, value := range values {
		currentKey := key
		if parentKey != "" {
			currentKey = parentKey + "." + key
		}

		switch v := value.(type) {
		case string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
			continue
		case map[string]any:
			if depth >= 1 {
				return fmt.Errorf("values for post-build substitution cannot contain nested complex types, key '%s' has type %T", currentKey, v)
			}
			if err := g.validatePostBuildValues(v, currentKey, depth+1); err != nil {
				return err
			}
		case []any:
			return fmt.Errorf("values for post-build substitution cannot contain slices, key '%s' has type %T", currentKey, v)
		default:
			return fmt.Errorf("values for post-build substitution can only contain strings, numbers, booleans, or maps of scalar types, key '%s' has unsupported type %T", currentKey, v)
		}
	}
	return nil
}

// validateKubernetesManifest validates that the content represents a valid Kubernetes manifest.
// Checks for required fields like apiVersion, kind, and metadata.name.
// Returns an error if the manifest is invalid.
func (g *KustomizeGenerator) validateKubernetesManifest(content any) error {
	contentMap, ok := content.(map[string]any)
	if !ok {
		return fmt.Errorf("content must be a map, got %T", content)
	}

	apiVersion, ok := contentMap["apiVersion"].(string)
	if !ok || apiVersion == "" {
		return fmt.Errorf("manifest missing or invalid 'apiVersion' field")
	}

	kind, ok := contentMap["kind"].(string)
	if !ok || kind == "" {
		return fmt.Errorf("manifest missing or invalid 'kind' field")
	}

	metadata, ok := contentMap["metadata"].(map[string]any)
	if !ok {
		return fmt.Errorf("manifest missing 'metadata' field")
	}

	name, ok := metadata["name"].(string)
	if !ok || name == "" {
		return fmt.Errorf("manifest missing or invalid 'metadata.name' field")
	}

	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure KustomizeGenerator implements Generator
var _ Generator = (*KustomizeGenerator)(nil)
