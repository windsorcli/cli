package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/runtime"
)

// The OCIModuleResolver is a terraform module resolver for OCI artifact sources.
// It provides functionality to extract terraform modules from OCI artifacts and generate appropriate shim configurations.
// The OCIModuleResolver acts as a specialized resolver within the terraform module system,
// handling OCI artifact downloading, module extraction, and configuration for OCI-based terraform sources.

// =============================================================================
// Types
// =============================================================================

// OCIModuleResolver handles terraform modules from OCI artifacts
type OCIModuleResolver struct {
	*BaseModuleResolver
	artifactBuilder artifact.Artifact
}

// =============================================================================
// Constructor
// =============================================================================

// NewOCIModuleResolver creates a new OCI module resolver with the provided dependencies.
func NewOCIModuleResolver(rt *runtime.Runtime, blueprintHandler blueprint.BlueprintHandler, artifactBuilder artifact.Artifact) *OCIModuleResolver {
	return &OCIModuleResolver{
		BaseModuleResolver: NewBaseModuleResolver(rt, blueprintHandler),
		artifactBuilder:    artifactBuilder,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// ProcessModules processes all terraform components that use OCI sources by extracting
// modules from OCI artifacts and generating appropriate module shims. It identifies
// components with resolved OCI source URLs, extracts the required modules, and creates
// the necessary terraform configuration files.
func (h *OCIModuleResolver) ProcessModules() error {
	components := h.blueprintHandler.GetTerraformComponents()

	ociURLs := make(map[string]bool)
	for _, component := range components {
		if h.shouldHandle(component.Source) {
			pathSeparatorIdx := strings.Index(component.Source[6:], "//")
			if pathSeparatorIdx != -1 {
				baseURL := component.Source[:6+pathSeparatorIdx] // oci://registry/repo:tag
				ociURLs[baseURL] = true
			}
		}
	}

	if len(ociURLs) == 0 {
		return nil
	}

	var ociURLList []string
	for url := range ociURLs {
		ociURLList = append(ociURLList, url)
	}

	ociArtifacts, err := h.artifactBuilder.Pull(ociURLList)
	if err != nil {
		return fmt.Errorf("failed to preload OCI artifacts: %w", err)
	}

	for _, component := range components {
		if !h.shouldHandle(component.Source) {
			continue
		}

		if err := h.processComponent(component, ociArtifacts); err != nil {
			return fmt.Errorf("failed to process component %s: %w", component.Path, err)
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// shouldHandle determines if this resolver should handle the given source by checking
// if the source is an OCI artifact URL. Returns true only for sources that begin with
// the "oci://" protocol prefix, indicating they are OCI registry artifacts.
func (h *OCIModuleResolver) shouldHandle(source string) bool {
	return strings.HasPrefix(source, "oci://")
}

// processComponent processes a single terraform component with an OCI source.
// It creates the module directory, extracts the OCI module, computes the relative path,
// and writes the required shim files (main.tf, variables.tf, outputs.tf) for the component.
// Returns an error if any step fails.
func (h *OCIModuleResolver) processComponent(component blueprintv1alpha1.TerraformComponent, ociArtifacts map[string][]byte) error {
	moduleDir := component.FullPath
	if err := h.shims.MkdirAll(moduleDir, 0755); err != nil {
		return fmt.Errorf("failed to create module directory: %w", err)
	}

	extractedPath, err := h.extractOCIModule(component.Source, component.Path, ociArtifacts)
	if err != nil {
		return fmt.Errorf("failed to extract OCI module: %w", err)
	}

	relPath, err := h.shims.FilepathRel(moduleDir, extractedPath)
	if err != nil {
		return fmt.Errorf("failed to calculate relative path: %w", err)
	}

	if err := h.writeShimMainTf(moduleDir, relPath); err != nil {
		return fmt.Errorf("failed to write main.tf: %w", err)
	}

	if err := h.writeShimVariablesTf(moduleDir, extractedPath, relPath); err != nil {
		return fmt.Errorf("failed to write variables.tf: %w", err)
	}

	if err := h.writeShimOutputsTf(moduleDir, extractedPath); err != nil {
		return fmt.Errorf("failed to write outputs.tf: %w", err)
	}

	return nil
}

// extractOCIModule extracts a specific terraform module from an OCI artifact.
// It parses the resolved OCI source, ensures the entire artifact is cached on disk,
// and returns the full path to the extracted module. The artifact is extracted by
// registry/repository:tag and cached for reuse across multiple module extractions and feature processing.
// Returns the full path to the extracted module or an error if extraction fails.
func (h *OCIModuleResolver) extractOCIModule(resolvedSource, componentPath string, ociArtifacts map[string][]byte) (string, error) {
	message := fmt.Sprintf("📥 Loading component %s", componentPath)

	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " " + message
	spin.Start()

	defer func() {
		spin.Stop()
		fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)
	}()

	if !strings.HasPrefix(resolvedSource, "oci://") {
		return "", fmt.Errorf("invalid resolved OCI source format: %s", resolvedSource)
	}

	pathSeparatorIdx := strings.Index(resolvedSource[6:], "//")
	if pathSeparatorIdx == -1 {
		return "", fmt.Errorf("invalid resolved OCI source format, missing path separator: %s", resolvedSource)
	}

	baseURL := resolvedSource[:6+pathSeparatorIdx]      // oci://registry/repo:tag
	modulePath := resolvedSource[6+pathSeparatorIdx+2:] // terraform/path/to/module

	registry, repository, tag, err := h.parseOCIRef(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse OCI reference: %w", err)
	}

	cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)

	projectRoot := h.runtime.ProjectRoot
	if projectRoot == "" {
		return "", fmt.Errorf("failed to get project root: project root is empty")
	}

	extractionKey := strings.ReplaceAll(strings.ReplaceAll(cacheKey, "/", "_"), ":", "_")
	extractionDir := filepath.Join(projectRoot, ".windsor", ".oci_extracted", extractionKey)
	fullModulePath := filepath.Join(extractionDir, modulePath)

	if _, err := h.shims.Stat(fullModulePath); err == nil {
		return fullModulePath, nil
	}

	artifactData, exists := ociArtifacts[cacheKey]
	if !exists {
		return "", fmt.Errorf("OCI artifact %s not found in cache", cacheKey)
	}

	if err := h.extractArtifactToCache(artifactData, registry, repository, tag); err != nil {
		return "", fmt.Errorf("failed to extract artifact to cache: %w", err)
	}

	if _, err := h.shims.Stat(fullModulePath); err != nil {
		return "", fmt.Errorf("module path %s not found in cached artifact", modulePath)
	}

	return fullModulePath, nil
}

// extractArtifactToCache extracts the full OCI artifact tar archive to the disk cache keyed by registry, repository, and tag.
// It unpacks the contents into a dedicated extraction directory under the project root, preserving permissions and handling executables.
// Extraction is atomic: all files are first unpacked to a temporary directory, then renamed into place on success, which avoids leaving partial state.
// This approach ensures all artifact files are available for subsequent module and feature resolutions and enables cache reuse.
// Returns an error if any extraction phase fails or if path validation checks do not pass.
func (h *OCIModuleResolver) extractArtifactToCache(artifactData []byte, registry, repository, tag string) (err error) {
	projectRoot := h.runtime.ProjectRoot
	if projectRoot == "" {
		return fmt.Errorf("failed to get project root: project root is empty")
	}

	reader := h.shims.NewBytesReader(artifactData)
	tarReader := h.shims.NewTarReader(reader)

	cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)
	extractionKey := strings.ReplaceAll(strings.ReplaceAll(cacheKey, "/", "_"), ":", "_")
	extractionDir := filepath.Join(projectRoot, ".windsor", ".oci_extracted", extractionKey)
	tmpExtractionDir := extractionDir + ".tmp"

	defer func() {
		if err != nil {
			if _, statErr := h.shims.Stat(tmpExtractionDir); statErr == nil {
				if removeErr := h.shims.RemoveAll(tmpExtractionDir); removeErr != nil {
					fmt.Fprintf(os.Stderr, "warning: failed to clean up temporary extraction directory: %v\n", removeErr)
				}
			}
		}
	}()

	if _, statErr := h.shims.Stat(tmpExtractionDir); statErr == nil {
		if removeErr := h.shims.RemoveAll(tmpExtractionDir); removeErr != nil {
			return fmt.Errorf("failed to clean up existing temporary extraction directory: %w", removeErr)
		}
	}

	if mkdirErr := h.shims.MkdirAll(tmpExtractionDir, 0755); mkdirErr != nil {
		return fmt.Errorf("failed to create temporary extraction directory: %w", mkdirErr)
	}

	if err := h.extractTarEntries(tarReader, tmpExtractionDir); err != nil {
		return err
	}

	parentDir := filepath.Dir(extractionDir)
	if mkdirErr := h.shims.MkdirAll(parentDir, 0755); mkdirErr != nil {
		return fmt.Errorf("failed to create parent directory: %w", mkdirErr)
	}

	if _, statErr := h.shims.Stat(extractionDir); statErr == nil {
		if removeErr := h.shims.RemoveAll(extractionDir); removeErr != nil {
			return fmt.Errorf("failed to remove existing extraction directory: %w", removeErr)
		}
	}

	if renameErr := h.shims.Rename(tmpExtractionDir, extractionDir); renameErr != nil {
		return fmt.Errorf("failed to rename temporary extraction directory to final location: %w", renameErr)
	}

	return nil
}

// parseOCIRef extracts the registry, repository, and tag components from an OCI reference string.
// The OCI reference must be in the format "oci://registry/repository:tag".
// Returns the registry, repository, and tag if parsing is successful, or an error if the format is invalid.
func (h *OCIModuleResolver) parseOCIRef(ociRef string) (registry, repository, tag string, err error) {
	if !strings.HasPrefix(ociRef, "oci://") {
		return "", "", "", fmt.Errorf("invalid OCI reference format: %s", ociRef)
	}

	ref := strings.TrimPrefix(ociRef, "oci://")

	parts := strings.Split(ref, ":")
	if len(parts) != 2 {
		return "", "", "", fmt.Errorf("invalid OCI reference format, expected registry/repository:tag: %s", ociRef)
	}

	repoWithRegistry := parts[0]
	tag = parts[1]

	repoParts := strings.SplitN(repoWithRegistry, "/", 2)
	if len(repoParts) != 2 {
		return "", "", "", fmt.Errorf("invalid OCI reference format, expected registry/repository:tag: %s", ociRef)
	}

	registry = repoParts[0]
	repository = repoParts[1]

	return registry, repository, tag, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure OCIModuleResolver implements ModuleResolver
var _ ModuleResolver = (*OCIModuleResolver)(nil)
