package terraform

import (
	"fmt"
	"os"
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

// processComponent processes a single terraform component with an OCI source.
// It creates the module directory, extracts the OCI module, computes the relative path,
// and writes the required shim files (main.tf, variables.tf, outputs.tf) for the component.
// Returns an error if any step fails.
func (h *OCIModuleResolver) processComponent(component blueprintv1alpha1.TerraformComponent, ociArtifacts map[string]string) error {
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
// It parses the resolved OCI source, determines the cache key, and uses the artifact package
// to extract the module path from the cached artifact. Returns the full path to the extracted module
// or an error if extraction fails.
func (h *OCIModuleResolver) extractOCIModule(resolvedSource, componentPath string, ociArtifacts map[string]string) (string, error) {
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

	registry, repository, tag, err := h.artifactBuilder.ParseOCIRef(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse OCI reference: %w", err)
	}

	cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)
	_, exists := ociArtifacts[cacheKey]
	if !exists {
		return "", fmt.Errorf("OCI artifact %s not found in cache", cacheKey)
	}

	extractedPath, err := h.artifactBuilder.ExtractModulePath(registry, repository, tag, modulePath)
	if err != nil {
		return "", fmt.Errorf("failed to extract module path: %w", err)
	}

	return extractedPath, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// shouldHandle determines if this resolver should handle the given source by checking
// if the source is an OCI artifact URL. Returns true only for sources that begin with
// the "oci://" protocol prefix, indicating they are OCI registry artifacts.
func (h *OCIModuleResolver) shouldHandle(source string) bool {
	if !strings.HasPrefix(source, "oci://") {
		return false
	}

	return true
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure OCIModuleResolver implements ModuleResolver
var _ ModuleResolver = (*OCIModuleResolver)(nil)
