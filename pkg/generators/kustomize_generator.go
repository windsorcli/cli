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
		if strings.HasPrefix(key, "kustomize/") {
			if err := g.generatePatchFile(key, values, configRoot, shouldOverwrite); err != nil {
				return err
			}
		} else if strings.HasPrefix(key, "values/") {
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
// It validates the kustomization name and patch directory, ensures values are a map, constructs the full patch file path,
// validates the path, and delegates file generation to generatePatchFiles. Returns an error if any step fails.
func (g *KustomizeGenerator) generatePatchFile(key string, values any, configRoot string, overwrite bool) error {
	patchPath := strings.TrimPrefix(key, "kustomize/")

	if err := g.validateKustomizationName(patchPath); err != nil {
		return fmt.Errorf("invalid kustomization name %s: %w", patchPath, err)
	}

	patchesDir := filepath.Join(configRoot, "kustomize")
	if err := g.validatePath(patchesDir, configRoot); err != nil {
		return fmt.Errorf("invalid patches directory path %s: %w", patchesDir, err)
	}

	valuesMap, ok := values.(map[string]any)
	if !ok {
		return fmt.Errorf("values for kustomization %s must be a map, got %T", patchPath, values)
	}

	fullPatchPath := filepath.Join(patchesDir, patchPath)
	if !strings.HasSuffix(fullPatchPath, ".yaml") && !strings.HasSuffix(fullPatchPath, ".yml") {
		fullPatchPath = fullPatchPath + ".yaml"
	}
	if err := g.validatePath(fullPatchPath, configRoot); err != nil {
		return fmt.Errorf("invalid patch file path %s: %w", fullPatchPath, err)
	}

	if err := g.generatePatchFiles(fullPatchPath, valuesMap, overwrite); err != nil {
		return fmt.Errorf("failed to generate patch files for %s: %w", patchPath, err)
	}

	return nil
}

// generateValuesFile creates values.yaml files for post-build variable substitution in kustomize workflows.
// Accepts a key, values map, configuration root, and overwrite flag. Validates the values file name and directory,
// ensures the values are a map with only scalar types, determines the correct file path for global or component-specific values,
// validates the final path, and writes the values file. Returns an error if any validation or file operation fails.
func (g *KustomizeGenerator) generateValuesFile(key string, values any, configRoot string, overwrite bool) error {
	valuesPath := strings.TrimPrefix(key, "values/")

	if err := g.validateKustomizationName(valuesPath); err != nil {
		return fmt.Errorf("invalid values name %s: %w", valuesPath, err)
	}

	valuesDir := filepath.Join(configRoot, "kustomize")
	if err := g.validatePath(valuesDir, configRoot); err != nil {
		return fmt.Errorf("invalid values directory path %s: %w", valuesDir, err)
	}

	valuesMap, ok := values.(map[string]any)
	if !ok {
		return fmt.Errorf("values for kustomization %s must be a map, got %T", valuesPath, values)
	}

	if err := g.validateValuesForSubstitution(valuesMap); err != nil {
		return fmt.Errorf("invalid values for post-build substitution %s: %w", valuesPath, err)
	}

	var fullValuesPath string
	if valuesPath == "global" {
		fullValuesPath = filepath.Join(valuesDir, "values.yaml")
	} else {
		fullValuesPath = filepath.Join(valuesDir, valuesPath, "values.yaml")
	}

	if err := g.validatePath(fullValuesPath, configRoot); err != nil {
		return fmt.Errorf("invalid values file path %s: %w", fullValuesPath, err)
	}

	if err := g.generateValuesFiles(fullValuesPath, valuesMap, overwrite); err != nil {
		return fmt.Errorf("failed to generate values files for %s: %w", valuesPath, err)
	}

	return nil
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

	components := strings.Split(name, "/")
	for _, component := range components {
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

// validateValuesForSubstitution checks that all values are valid for Flux post-build variable substitution.
// Permitted types are string, numeric, and boolean. Complex types (maps, slices) are rejected.
// Returns an error if any value is not a supported type.
func (g *KustomizeGenerator) validateValuesForSubstitution(values map[string]any) error {
	for key, value := range values {
		switch v := value.(type) {
		case string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
			continue
		case map[string]any, []any:
			return fmt.Errorf("values for post-build substitution cannot contain complex types (maps or slices), key '%s' has type %T", key, v)
		default:
			return fmt.Errorf("values for post-build substitution can only contain strings, numbers, and booleans, key '%s' has unsupported type %T", key, v)
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

// generatePatchFiles writes a YAML patch file to patchPath using the provided values map.
// patchPath must be a file path. If it doesn't have a .yaml or .yml extension, .yaml will be automatically appended.
// For Jsonnet format, values is a direct object. If overwrite is false, existing files are not replaced.
// The content must be a valid Kubernetes manifest map with non-empty "apiVersion", "kind", and "metadata.name" fields.
// Returns an error on validation, marshalling, or file operation failure.
func (g *KustomizeGenerator) generatePatchFiles(patchPath string, values map[string]any, overwrite bool) error {
	if !strings.HasSuffix(patchPath, ".yaml") && !strings.HasSuffix(patchPath, ".yml") {
		patchPath = patchPath + ".yaml"
	}

	dir := filepath.Dir(patchPath)
	if err := g.shims.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	if !overwrite {
		if _, err := g.shims.Stat(patchPath); err == nil {
			return nil
		}
	}

	if err := g.validateKubernetesManifest(values); err != nil {
		return fmt.Errorf("invalid Kubernetes manifest for %s: %w", patchPath, err)
	}

	yamlData, err := g.shims.MarshalYAML(values)
	if err != nil {
		return fmt.Errorf("failed to marshal content to YAML for %s: %w", patchPath, err)
	}

	if err := g.shims.WriteFile(patchPath, yamlData, 0644); err != nil {
		return fmt.Errorf("failed to write patch file %s: %w", patchPath, err)
	}

	return nil
}

// generateValuesFiles writes a YAML values file to valuesPath using the provided values map.
// valuesPath must be a file path. If it doesn't have a .yaml or .yml extension, .yaml will be automatically appended.
// For Jsonnet format, values is a direct object. If overwrite is false, existing files are not replaced.
// The content must be a valid YAML map structure suitable for post-build variable substitution.
// Returns an error on validation, marshalling, or file operation failure.
func (g *KustomizeGenerator) generateValuesFiles(valuesPath string, values map[string]any, overwrite bool) error {
	if !strings.HasSuffix(valuesPath, ".yaml") && !strings.HasSuffix(valuesPath, ".yml") {
		valuesPath = valuesPath + ".yaml"
	}

	dir := filepath.Dir(valuesPath)
	if err := g.shims.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	if !overwrite {
		if _, err := g.shims.Stat(valuesPath); err == nil {
			return nil
		}
	}

	yamlData, err := g.shims.MarshalYAML(values)
	if err != nil {
		return fmt.Errorf("failed to marshal content to YAML for %s: %w", valuesPath, err)
	}

	if err := g.shims.WriteFile(valuesPath, yamlData, 0644); err != nil {
		return fmt.Errorf("failed to write values file %s: %w", valuesPath, err)
	}

	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure KustomizeGenerator implements Generator
var _ Generator = (*KustomizeGenerator)(nil)
