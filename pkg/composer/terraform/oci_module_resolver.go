package terraform

import (
	"fmt"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
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
// Panics if rt, blueprintHandler, or artifactBuilder are nil.
func NewOCIModuleResolver(rt *runtime.Runtime, blueprintHandler blueprint.BlueprintHandler, artifactBuilder artifact.Artifact) *OCIModuleResolver {
	if rt == nil {
		panic("runtime is required")
	}
	if blueprintHandler == nil {
		panic("blueprint handler is required")
	}
	if artifactBuilder == nil {
		panic("artifact builder is required")
	}

	return &OCIModuleResolver{
		BaseModuleResolver: NewBaseModuleResolver(rt, blueprintHandler),
		artifactBuilder:    artifactBuilder,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// ProcessModules pulls the OCI artifacts that the terraform components use and writes a shim for each OCI component.
// It pulls each artifact once, and stops at the first pull or component error.
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
		return werror.Wrap(err, "failed to preload OCI artifacts")
	}

	for _, component := range components {
		if !h.shouldHandle(component.Source) {
			continue
		}

		if err := h.processComponent(component, ociArtifacts); err != nil {
			return werror.Wrap(err, "failed to process component %s", component.Path)
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// processComponent writes the shim for one OCI component at its FullPath.
// It creates the directory, removes the old .tf files, and extracts the module from the cached artifact.
// It then writes main.tf, variables.tf, and outputs.tf that point to the extracted module.
func (h *OCIModuleResolver) processComponent(component blueprintv1alpha1.TerraformComponent, ociArtifacts map[string]string) error {
	moduleDir := component.FullPath
	if err := h.shims.MkdirAll(moduleDir, 0755); err != nil {
		return werror.Wrap(err, "failed to create module directory")
	}
	if err := h.clearShimDirTfFiles(moduleDir); err != nil {
		return werror.Wrap(err, "failed to clear shim directory")
	}

	extractedPath, err := h.extractOCIModule(component.Source, ociArtifacts)
	if err != nil {
		return werror.Wrap(err, "failed to extract OCI module")
	}

	relPath, err := h.shims.FilepathRel(moduleDir, extractedPath)
	if err != nil {
		return werror.Wrap(err, "failed to calculate relative path")
	}

	if err := h.writeShimMainTf(moduleDir, relPath); err != nil {
		return werror.Wrap(err, "failed to write main.tf")
	}

	if err := h.writeShimVariablesTf(moduleDir, extractedPath, relPath); err != nil {
		return werror.Wrap(err, "failed to write variables.tf")
	}

	if err := h.writeShimOutputsTf(moduleDir, extractedPath); err != nil {
		return werror.Wrap(err, "failed to write outputs.tf")
	}

	return nil
}

// extractOCIModule extracts the module of a resolved OCI source and returns its path.
// The source format is oci://registry/repository:tag//path/to/module.
// It returns an error if the source format is not valid or the artifact is not in ociArtifacts.
// A source without a module path returns an error with a remediation hint.
func (h *OCIModuleResolver) extractOCIModule(resolvedSource string, ociArtifacts map[string]string) (string, error) {
	if !strings.HasPrefix(resolvedSource, "oci://") {
		return "", fmt.Errorf("invalid resolved OCI source format: %s", resolvedSource)
	}

	pathSeparatorIdx := strings.Index(resolvedSource[6:], "//")
	if pathSeparatorIdx == -1 {
		return "", werror.New(fmt.Sprintf("invalid resolved OCI source format, missing path separator: %s", resolvedSource), "Add a module path to the OCI source, for example oci://registry/repository:tag//terraform/module.", nil)
	}

	baseURL := resolvedSource[:6+pathSeparatorIdx]      // oci://registry/repo:tag
	modulePath := resolvedSource[6+pathSeparatorIdx+2:] // terraform/path/to/module

	registry, repository, tag, err := h.artifactBuilder.ParseOCIRef(baseURL)
	if err != nil {
		return "", werror.Wrap(err, "failed to parse OCI reference")
	}

	cacheKey := artifact.FormatOCIRef(registry, repository, tag)
	if _, exists := ociArtifacts[cacheKey]; !exists {
		return "", fmt.Errorf("OCI artifact %s not found in cache", cacheKey)
	}

	extractedPath, err := h.artifactBuilder.ExtractModulePath(registry, repository, tag, modulePath)
	if err != nil {
		return "", werror.Wrap(err, "failed to extract module path")
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
