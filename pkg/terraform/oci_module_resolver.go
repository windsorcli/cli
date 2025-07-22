package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/di"
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

// NewOCIModuleResolver creates a new OCI module resolver
func NewOCIModuleResolver(injector di.Injector) *OCIModuleResolver {
	return &OCIModuleResolver{
		BaseModuleResolver: NewBaseModuleResolver(injector),
	}
}

// Initialize sets up the OCI module resolver with required dependencies
func (h *OCIModuleResolver) Initialize() error {
	if err := h.BaseModuleResolver.Initialize(); err != nil {
		return err
	}

	artifactBuilderInterface := h.injector.Resolve("artifactBuilder")
	var ok bool
	h.artifactBuilder, ok = artifactBuilderInterface.(artifact.Artifact)
	if !ok {
		return fmt.Errorf("failed to resolve artifact builder")
	}

	blueprintHandlerInterface := h.injector.Resolve("blueprintHandler")
	h.blueprintHandler, ok = blueprintHandlerInterface.(blueprint.BlueprintHandler)
	if !ok {
		return fmt.Errorf("failed to resolve blueprint handler")
	}

	return nil
}

// =============================================================================
// Public Methods
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
// It parses the resolved OCI source, determines the cache key, checks for an existing extraction,
// and if not present, extracts the module from the cached artifact data. Returns the full path
// to the extracted module or an error if extraction fails.
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

	projectRoot, err := h.shell.GetProjectRoot()
	if err != nil {
		return "", fmt.Errorf("failed to get project root: %w", err)
	}

	extractionKey := fmt.Sprintf("%s-%s-%s", registry, repository, tag)
	fullModulePath := filepath.Join(projectRoot, ".windsor", ".oci_extracted", extractionKey, modulePath)
	if _, err := h.shims.Stat(fullModulePath); err == nil {
		return fullModulePath, nil
	}

	artifactData, exists := ociArtifacts[cacheKey]
	if !exists {
		return "", fmt.Errorf("OCI artifact %s not found in cache", cacheKey)
	}

	if err := h.extractModuleFromArtifact(artifactData, modulePath, extractionKey); err != nil {
		return "", fmt.Errorf("failed to extract module from artifact: %w", err)
	}

	return fullModulePath, nil
}

// extractModuleFromArtifact extracts the specified terraform module from the provided artifact data.
// It unpacks files and directories matching the modulePath from the tar archive into the extraction directory
// under the project root, preserving file permissions and handling executable scripts. Returns an error if
// extraction fails at any step, including directory creation, file writing, or permission setting.
func (h *OCIModuleResolver) extractModuleFromArtifact(artifactData []byte, modulePath, extractionKey string) error {
	projectRoot, err := h.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	reader := h.shims.NewBytesReader(artifactData)
	tarReader := h.shims.NewTarReader(reader)
	targetPrefix := modulePath

	extractionDir := filepath.Join(projectRoot, ".windsor", ".oci_extracted", extractionKey)

	for {
		header, err := tarReader.Next()
		if err == h.shims.EOFError() {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		if !strings.HasPrefix(header.Name, targetPrefix) {
			continue
		}

		// Validate and sanitize the path to prevent directory traversal
		sanitizedPath, err := h.validateAndSanitizePath(header.Name)
		if err != nil {
			return fmt.Errorf("invalid path in tar archive: %w", err)
		}

		destPath := filepath.Join(extractionDir, sanitizedPath)

		// Ensure the destination path is still within the extraction directory
		if !strings.HasPrefix(destPath, extractionDir) {
			return fmt.Errorf("path traversal attempt detected: %s", header.Name)
		}

		if header.Typeflag == h.shims.TypeDir() {
			if err := h.shims.MkdirAll(destPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", destPath, err)
			}
			continue
		}

		if err := h.shims.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create parent directory for %s: %w", destPath, err)
		}

		file, err := h.shims.Create(destPath)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", destPath, err)
		}

		_, err = h.shims.Copy(file, tarReader)
		if closeErr := file.Close(); closeErr != nil {
			return fmt.Errorf("failed to close file %s: %w", destPath, closeErr)
		}
		if err != nil {
			return fmt.Errorf("failed to write file %s: %w", destPath, err)
		}

		modeValue := header.Mode & 0777
		if modeValue < 0 || modeValue > 0777 {
			return fmt.Errorf("invalid file mode %o for %s", header.Mode, destPath)
		}
		fileMode := os.FileMode(uint32(modeValue))

		if strings.HasSuffix(destPath, ".sh") {
			fileMode |= 0111
		}

		if err := h.shims.Chmod(destPath, fileMode); err != nil {
			return fmt.Errorf("failed to set file permissions for %s: %w", destPath, err)
		}
	}

	return nil
}

// validateAndSanitizePath sanitizes a file path for safe extraction by removing path traversal sequences
// and rejecting absolute paths. Returns the cleaned path if valid, or an error if the path is unsafe.
func (h *OCIModuleResolver) validateAndSanitizePath(path string) (string, error) {
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return "", fmt.Errorf("path contains directory traversal sequence: %s", path)
	}
	if filepath.IsAbs(cleanPath) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", path)
	}
	return cleanPath, nil
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
