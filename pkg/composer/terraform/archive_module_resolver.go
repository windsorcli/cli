package terraform

import (
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/runtime"
)

// The ArchiveModuleResolver is a terraform module resolver for file:// archive sources.
// It provides functionality to extract terraform modules from local tar.gz archives and generate appropriate shim configurations.
// The ArchiveModuleResolver acts as a specialized resolver within the terraform module system,
// handling archive extraction, module extraction, and configuration for file://-based terraform sources.

// =============================================================================
// Types
// =============================================================================

// ArchiveModuleResolver handles terraform modules from local archive files
type ArchiveModuleResolver struct {
	*BaseModuleResolver
}

// =============================================================================
// Constructor
// =============================================================================

// NewArchiveModuleResolver creates a new archive module resolver with the provided dependencies.
func NewArchiveModuleResolver(rt *runtime.Runtime, blueprintHandler blueprint.BlueprintHandler) *ArchiveModuleResolver {
	return &ArchiveModuleResolver{
		BaseModuleResolver: NewBaseModuleResolver(rt, blueprintHandler),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// ProcessModules processes all terraform components that use file:// sources by extracting
// modules from local archives and generating appropriate module shims. It identifies
// components with resolved file:// source URLs, extracts the required modules, and creates
// the necessary terraform configuration files.
func (h *ArchiveModuleResolver) ProcessModules() error {
	components := h.blueprintHandler.GetTerraformComponents()

	archivePaths := make(map[string]bool)
	for _, component := range components {
		if h.shouldHandle(component.Source) {
			pathSeparatorIdx := strings.Index(component.Source[7:], "//")
			if pathSeparatorIdx != -1 {
				basePath := component.Source[7 : 7+pathSeparatorIdx]
				archivePaths[basePath] = true
			}
		}
	}

	if len(archivePaths) == 0 {
		return nil
	}

	for _, component := range components {
		if !h.shouldHandle(component.Source) {
			continue
		}

		if err := h.processComponent(component); err != nil {
			return fmt.Errorf("failed to process component %s: %w", component.Path, err)
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// shouldHandle determines if this resolver should handle the given source by checking
// if the source is a file:// archive URL. Returns true only for sources that begin with
// the "file://" protocol prefix, indicating they are local archive files.
func (h *ArchiveModuleResolver) shouldHandle(source string) bool {
	return strings.HasPrefix(source, "file://")
}

// processComponent processes a single terraform component with a file:// source.
// It creates the module directory, extracts the archive module, computes the relative path,
// and writes the required shim files (main.tf, variables.tf, outputs.tf) for the component.
// Returns an error if any step fails.
func (h *ArchiveModuleResolver) processComponent(component blueprintv1alpha1.TerraformComponent) error {
	moduleDir := component.FullPath
	if err := h.shims.MkdirAll(moduleDir, 0755); err != nil {
		return fmt.Errorf("failed to create module directory: %w", err)
	}

	extractedPath, err := h.extractArchiveModule(component.Source, component.Path)
	if err != nil {
		return fmt.Errorf("failed to extract archive module: %w", err)
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

// extractArchiveModule extracts a specific terraform module from a local archive file.
// It parses the resolved file:// source, determines the archive path and module path,
// checks for an existing extraction, and if not present, extracts the module from the archive.
// Returns the full path to the extracted module or an error if extraction fails.
func (h *ArchiveModuleResolver) extractArchiveModule(resolvedSource, componentPath string) (string, error) {
	message := fmt.Sprintf("📥 Loading component %s", componentPath)

	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " " + message
	spin.Start()

	defer func() {
		spin.Stop()
		fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)
	}()

	if !strings.HasPrefix(resolvedSource, "file://") {
		return "", fmt.Errorf("invalid resolved archive source format: %s", resolvedSource)
	}

	pathSeparatorIdx := strings.Index(resolvedSource[7:], "//")
	if pathSeparatorIdx == -1 {
		return "", fmt.Errorf("invalid resolved archive source format, missing path separator: %s", resolvedSource)
	}

	archivePath := resolvedSource[7 : 7+pathSeparatorIdx]
	modulePath := resolvedSource[7+pathSeparatorIdx+2:]

	if refIdx := strings.Index(modulePath, "?ref="); refIdx != -1 {
		modulePath = modulePath[:refIdx]
	}

	projectRoot := h.runtime.ProjectRoot
	if projectRoot == "" {
		return "", fmt.Errorf("failed to get project root: project root is empty")
	}

	configRoot := h.runtime.ConfigRoot
	if configRoot == "" {
		return "", fmt.Errorf("failed to get config root: config root is empty")
	}

	blueprintYamlPath := filepath.Join(configRoot, "blueprint.yaml")
	blueprintYamlDir := filepath.Dir(blueprintYamlPath)

	var absArchivePath string
	var err error
	if filepath.IsAbs(archivePath) {
		absArchivePath = archivePath
	} else {
		absArchivePath = filepath.Join(blueprintYamlDir, archivePath)
		absArchivePath, err = h.shims.FilepathAbs(absArchivePath)
		if err != nil {
			return "", fmt.Errorf("failed to get absolute path for archive: %w", err)
		}
	}

	extractionKey := h.shims.FilepathBase(absArchivePath)
	if strings.HasSuffix(extractionKey, ".tar.gz") {
		extractionKey = strings.TrimSuffix(extractionKey, ".tar.gz")
	} else if strings.HasSuffix(extractionKey, ".tar") {
		extractionKey = strings.TrimSuffix(extractionKey, ".tar")
	}

	fullModulePath := filepath.Join(projectRoot, ".windsor", ".archive_extracted", extractionKey, modulePath)
	if _, err := h.shims.Stat(fullModulePath); err == nil {
		spin.Stop()
		return fullModulePath, nil
	}

	archiveData, err := h.shims.ReadFile(absArchivePath)
	if err != nil {
		return "", fmt.Errorf("failed to read archive file: %w", err)
	}

	if err := h.extractModuleFromArchive(archiveData, modulePath, extractionKey); err != nil {
		return "", fmt.Errorf("failed to extract module from archive: %w", err)
	}

	return fullModulePath, nil
}

// extractModuleFromArchive extracts the specified terraform module from the provided archive data.
// It unpacks files and directories matching the modulePath from the tar archive into the extraction directory
// under the project root, preserving file permissions and handling executable scripts. Returns an error if
// extraction fails at any step, including directory creation, file writing, or permission setting.
func (h *ArchiveModuleResolver) extractModuleFromArchive(archiveData []byte, modulePath, extractionKey string) error {
	projectRoot := h.runtime.ProjectRoot
	if projectRoot == "" {
		return fmt.Errorf("failed to get project root: project root is empty")
	}

	reader := h.shims.NewBytesReader(archiveData)
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	tarReader := h.shims.NewTarReader(gzipReader)
	targetPrefix := modulePath

	extractionDir := filepath.Join(projectRoot, ".windsor", ".archive_extracted", extractionKey)

	if err := h.extractTarEntriesWithFilter(tarReader, extractionDir, targetPrefix); err != nil {
		return err
	}

	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure ArchiveModuleResolver implements ModuleResolver
var _ ModuleResolver = (*ArchiveModuleResolver)(nil)
