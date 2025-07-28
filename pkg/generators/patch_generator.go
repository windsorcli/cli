package generators

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/pkg/di"
)

// The PatchGenerator is a specialized component that manages Kustomize patch files.
// It provides functionality to process patch templates and generate patch files for
// kustomizations defined in the blueprint. The PatchGenerator ensures proper
// patch file generation with templating support.

// =============================================================================
// Types
// =============================================================================

// PatchGenerator is a generator that processes and generates patch files
type PatchGenerator struct {
	BaseGenerator
}

// =============================================================================
// Constructor
// =============================================================================

// NewPatchGenerator creates a new PatchGenerator with the provided dependency injector.
// It initializes the base generator and prepares it for patch file generation.
func NewPatchGenerator(injector di.Injector) *PatchGenerator {
	return &PatchGenerator{
		BaseGenerator: *NewGenerator(injector),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Generate creates patch files for kustomizations using provided template data.
// Processes template data from the init pipeline (renderedData) or patch-specific data from the install pipeline.
// For init pipeline, extracts patch-relevant information from renderedData.
// For install pipeline, processes data keyed by "patches/<kustomization_name>" to generate patch files.
func (g *PatchGenerator) Generate(data map[string]any, overwrite ...bool) error {
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
		if !strings.HasPrefix(key, "patches/") {
			continue
		}

		kustomizationName := strings.TrimPrefix(key, "patches/")
		if err := g.validateKustomizationName(kustomizationName); err != nil {
			return fmt.Errorf("invalid kustomization name %s: %w", kustomizationName, err)
		}

		patchesDir := filepath.Join(configRoot, "patches", kustomizationName)
		if err := g.validatePath(patchesDir, configRoot); err != nil {
			return fmt.Errorf("invalid patches directory path %s: %w", patchesDir, err)
		}

		if err := g.shims.MkdirAll(patchesDir, 0755); err != nil {
			return fmt.Errorf("failed to create patches directory %s: %w", patchesDir, err)
		}

		valuesMap, ok := values.(map[string]any)
		if !ok {
			return fmt.Errorf("values for kustomization %s must be a map, got %T", kustomizationName, values)
		}

		if err := g.generatePatchFiles(patchesDir, valuesMap, shouldOverwrite); err != nil {
			return fmt.Errorf("failed to generate patch files for %s: %w", kustomizationName, err)
		}
	}

	return nil
}

// generatePatchFiles writes YAML patch files to the specified directory using provided template values.
// For each entry in values, creates a YAML file named after the key (appending .yaml if necessary).
// If overwrite is false, existing files are not replaced. Content is marshaled to YAML before writing.
// Returns an error if marshalling or file operations fail.
func (g *PatchGenerator) generatePatchFiles(patchesDir string, values map[string]any, overwrite bool) error {
	for filename, content := range values {
		if !strings.HasSuffix(filename, ".yaml") && !strings.HasSuffix(filename, ".yml") {
			filename = filename + ".yaml"
		}
		patchFilePath := filepath.Join(patchesDir, filename)
		if !overwrite {
			if _, err := g.shims.Stat(patchFilePath); err == nil {
				continue
			}
		}

		if err := g.validateKubernetesManifest(content); err != nil {
			return fmt.Errorf("invalid Kubernetes manifest in %s: %w", filename, err)
		}

		yamlData, err := g.shims.MarshalYAML(content)
		if err != nil {
			return fmt.Errorf("failed to marshal patch content to YAML: %w", err)
		}
		if err := g.shims.WriteFile(patchFilePath, yamlData, 0644); err != nil {
			return fmt.Errorf("failed to write patch file %s: %w", patchFilePath, err)
		}
	}
	return nil
}

// validateKustomizationName validates that a kustomization name is safe and valid.
// Prevents path traversal attacks and ensures names contain only valid characters.
func (g *PatchGenerator) validateKustomizationName(name string) error {
	if name == "" {
		return fmt.Errorf("kustomization name cannot be empty")
	}

	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("kustomization name cannot contain path traversal characters")
	}

	if strings.ContainsAny(name, "<>:\"|?*") {
		return fmt.Errorf("kustomization name contains invalid characters")
	}

	return nil
}

// validatePath ensures the target path is within the expected directory structure.
// Prevents directory traversal attacks by ensuring the path is a subdirectory of the base path.
func (g *PatchGenerator) validatePath(targetPath, basePath string) error {
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return fmt.Errorf("failed to resolve base path: %w", err)
	}

	if !strings.HasPrefix(absTarget, absBase) {
		return fmt.Errorf("target path %s is outside base path %s", targetPath, basePath)
	}

	return nil
}

// validateKubernetesManifest validates that the content represents a valid Kubernetes manifest.
// Ensures the content has the required apiVersion, kind, and metadata fields.
func (g *PatchGenerator) validateKubernetesManifest(content any) error {
	contentMap, ok := content.(map[string]any)
	if !ok {
		return fmt.Errorf("content must be a map")
	}

	apiVersion, ok := contentMap["apiVersion"].(string)
	if !ok || apiVersion == "" {
		return fmt.Errorf("missing or invalid apiVersion field")
	}

	kind, ok := contentMap["kind"].(string)
	if !ok || kind == "" {
		return fmt.Errorf("missing or invalid kind field")
	}

	metadata, ok := contentMap["metadata"].(map[string]any)
	if !ok {
		return fmt.Errorf("missing metadata field")
	}

	name, ok := metadata["name"].(string)
	if !ok || name == "" {
		return fmt.Errorf("missing or invalid name in metadata")
	}

	return nil
}
