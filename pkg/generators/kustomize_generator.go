package generators

import (
	"fmt"
	"path/filepath"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
)

// The KustomizeGenerator is a specialized component that manages Kustomize files.
// It provides functionality to process patch templates and values templates, generating
// patch files and values.yaml files for kustomizations defined in the blueprint.
// The KustomizeGenerator ensures proper file generation with templating support.

// =============================================================================
// Types
// =============================================================================

// KustomizeBlueprintHandler defines the interface for accessing blueprint data
type KustomizeBlueprintHandler interface {
	GetKustomizations() []blueprintv1alpha1.Kustomization
}

// KustomizeGenerator is a generator that processes and generates kustomize files
type KustomizeGenerator struct {
	BaseGenerator
	blueprintHandler KustomizeBlueprintHandler
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

	handler, ok := blueprintHandler.(KustomizeBlueprintHandler)
	if !ok {
		return fmt.Errorf("resolved blueprint handler is not of expected type")
	}

	g.blueprintHandler = handler
	return nil
}

// Generate creates kustomize files using the provided template data.
// Processes data keyed by "kustomize/<kustomization_name>" for patches and
// "values/<kustomization_name>" for values.yaml files.
// The template engine has already filtered to only include referenced files, so this
// processes all provided data without additional filtering.
// Returns an error if data is nil, if generation fails, or if validation fails.
func (g *KustomizeGenerator) Generate(data map[string]any, overwrite ...bool) error {
	if data == nil {
		return fmt.Errorf("data cannot be nil")
	}

	shouldOverwrite := false
	if len(overwrite) > 0 {
		shouldOverwrite = overwrite[0]
	}

	configRoot, err := g.configHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("failed to get config root: %w", err)
	}

	for key, values := range data {
		if strings.HasPrefix(key, "kustomize/patches/") {
			if err := g.generatePatchFile(key, values, configRoot, shouldOverwrite); err != nil {
				return err
			}
		} else if key == "kustomize/values" {
			if err := g.generateValuesFile(key, values, configRoot, shouldOverwrite); err != nil {
				return err
			}
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// generatePatchFile generates patch files for kustomizations based on the provided key and values.
// It validates the patch path, ensures values are a map, constructs the full patch file path,
// validates the path, validates the Kubernetes manifest, and writes the patch file. Returns an error if any step fails.
func (g *KustomizeGenerator) generatePatchFile(key string, values any, configRoot string, overwrite bool) error {
	patchPath := strings.TrimPrefix(key, "kustomize/patches/")

	if err := g.validateKustomizationName(patchPath); err != nil {
		return fmt.Errorf("invalid patch path %s: %w", patchPath, err)
	}

	patchesDir := filepath.Join(configRoot, "kustomize", "patches")
	if err := g.validatePath(patchesDir, configRoot); err != nil {
		return fmt.Errorf("invalid patches directory path %s: %w", patchesDir, err)
	}

	valuesMap, ok := values.(map[string]any)
	if !ok {
		return fmt.Errorf("values for patch %s must be a map, got %T", patchPath, values)
	}

	fullPatchPath := filepath.Join(patchesDir, patchPath)
	if !strings.HasSuffix(fullPatchPath, ".yaml") && !strings.HasSuffix(fullPatchPath, ".yml") {
		fullPatchPath = fullPatchPath + ".yaml"
	}
	if err := g.validatePath(fullPatchPath, configRoot); err != nil {
		return fmt.Errorf("invalid patch file path %s: %w", fullPatchPath, err)
	}

	if err := g.validateKubernetesManifest(valuesMap); err != nil {
		return fmt.Errorf("invalid Kubernetes manifest for %s: %w", patchPath, err)
	}

	return g.writeYamlFile(fullPatchPath, valuesMap, overwrite)
}

// generateValuesFile writes a centralized values.yaml for kustomize post-build substitution.
// Accepts only "kustomize/values" as key. Validates that values is a map with only scalar types or one-level nested maps.
// Merges with any existing values.yaml, overwriting keys with new values. Writes the result to values.yaml in the kustomize directory.
// Returns error on invalid key, type, path, or file operation.
func (g *KustomizeGenerator) generateValuesFile(key string, values any, configRoot string, overwrite bool) error {
	if key != "kustomize/values" {
		return fmt.Errorf("invalid values key %s, expected 'kustomize/values'", key)
	}

	valuesDir := filepath.Join(configRoot, "kustomize")
	if err := g.validatePath(valuesDir, configRoot); err != nil {
		return fmt.Errorf("invalid values directory path %s: %w", valuesDir, err)
	}

	valuesMap, ok := values.(map[string]any)
	if !ok {
		return fmt.Errorf("values must be a map, got %T", values)
	}

	if err := g.validatePostBuildValues(valuesMap, "", 0); err != nil {
		return fmt.Errorf("invalid values for post-build substitution: %w", err)
	}

	fullValuesPath := filepath.Join(valuesDir, "values.yaml")
	if err := g.validatePath(fullValuesPath, configRoot); err != nil {
		return fmt.Errorf("invalid values file path %s: %w", fullValuesPath, err)
	}

	existingValues := make(map[string]any)
	if _, err := g.shims.Stat(fullValuesPath); err == nil {
		if data, err := g.shims.ReadFile(fullValuesPath); err == nil {
			if err := g.shims.YamlUnmarshal(data, &existingValues); err != nil {
				return fmt.Errorf("failed to unmarshal existing values file %s: %w", fullValuesPath, err)
			}
		}
	}

	for k, v := range valuesMap {
		existingValues[k] = v
	}

	return g.writeYamlFile(fullValuesPath, existingValues, overwrite)
}

// validateKustomizationName validates that a kustomization name is safe and valid.
// Prevents path traversal attacks and ensures names contain only valid characters.
// Now handles full paths including subdirectories (e.g., "ingress/nginx").
func (g *KustomizeGenerator) validateKustomizationName(name string) error {
	if name == "" {
		return fmt.Errorf("kustomization name cannot be empty")
	}

	if strings.Contains(name, "..") || strings.Contains(name, "\\") {
		return fmt.Errorf("kustomization name cannot contain path traversal characters")
	}

	for _, component := range strings.Split(name, "/") {
		if component == "" {
			return fmt.Errorf("kustomization name cannot contain empty path components")
		}
		for _, char := range component {
			if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_') {
				return fmt.Errorf("kustomization name component '%s' contains invalid character '%c'", component, char)
			}
		}
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

// validatePath ensures the target path is within the base path to prevent path traversal attacks.
// Returns an error if the target path is outside the base path or contains invalid characters.
func (g *KustomizeGenerator) validatePath(targetPath, basePath string) error {
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for %s: %w", targetPath, err)
	}

	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for %s: %w", basePath, err)
	}

	if !strings.HasPrefix(absTarget, absBase) {
		return fmt.Errorf("target path %s is outside base path %s", absTarget, absBase)
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

// writeYamlFile writes a YAML file to filePath using the provided values map.
// filePath must be a file path. If it doesn't have a .yaml or .yml extension, .yaml will be automatically appended.
// If overwrite is false, existing files are not replaced.
// Returns an error on marshalling or file operation failure.
func (g *KustomizeGenerator) writeYamlFile(filePath string, values map[string]any, overwrite bool) error {
	if !strings.HasSuffix(filePath, ".yaml") && !strings.HasSuffix(filePath, ".yml") {
		filePath = filePath + ".yaml"
	}

	dir := filepath.Dir(filePath)
	if err := g.shims.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	if !overwrite {
		if _, err := g.shims.Stat(filePath); err == nil {
			return nil
		}
	}

	yamlData, err := g.shims.MarshalYAML(values)
	if err != nil {
		return fmt.Errorf("failed to marshal content to YAML for %s: %w", filePath, err)
	}

	if err := g.shims.WriteFile(filePath, yamlData, 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure KustomizeGenerator implements Generator
var _ Generator = (*KustomizeGenerator)(nil)
