// The StandardModuleResolver is a terraform module resolver for standard source types.
// It writes shims for local paths, template paths, git repositories, registry modules, and other non-OCI sources.
// It runs terraform init to fetch a remote module, then copies its variables and outputs into the shim.
// The CompositeModuleResolver runs it after the OCIModuleResolver.

package terraform

import (
	"fmt"
	"path/filepath"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/runtime"
)

// =============================================================================
// Types
// =============================================================================

// TerraformInitOutput is one JSON line of the terraform init output.
type TerraformInitOutput struct {
	Type    string `json:"@type"`
	Message string `json:"@message"`
}

// StandardModuleResolver handles standard terraform module sources, such as git and local paths.
type StandardModuleResolver struct {
	*BaseModuleResolver
}

// =============================================================================
// Constructor
// =============================================================================

// NewStandardModuleResolver creates a standard module resolver from the runtime and the blueprint handler.
// It panics if rt or blueprintHandler is nil.
func NewStandardModuleResolver(rt *runtime.Runtime, blueprintHandler blueprint.BlueprintHandler) *StandardModuleResolver {
	if rt == nil {
		panic("runtime is required")
	}
	if blueprintHandler == nil {
		panic("blueprint handler is required")
	}

	return &StandardModuleResolver{
		BaseModuleResolver: NewBaseModuleResolver(rt, blueprintHandler),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// ProcessModules writes a shim for each terraform component that this resolver handles.
// A local component with a name gets a shim that points to terraform/<path>.
// A template source gets a shim that points to the template module.
// A remote source gets a shim that terraform init resolves. It runs the terraform check once.
// It stops at the first error.
func (h *StandardModuleResolver) ProcessModules() error {
	components := h.blueprintHandler.GetTerraformComponents()

	terraformChecked := false

	for _, component := range components {
		if component.Source == "" {
			if component.Name != "" {
				if err := h.processLocalComponent(component); err != nil {
					return werror.Wrap(err, "failed to process local component %s", component.GetID())
				}
			}
			continue
		}

		if h.isTemplateSource(component.Source) {
			if err := h.processTemplateComponent(component); err != nil {
				return err
			}
			continue
		}

		if !h.shouldHandle(component.Source) {
			continue
		}

		if err := h.processRemoteComponent(component, &terraformChecked); err != nil {
			return err
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// isTemplateSource reports whether source is an absolute path under contexts/_template or <project root>/terraform.
func (h *StandardModuleResolver) isTemplateSource(source string) bool {
	return filepath.IsAbs(source) && (strings.Contains(source, filepath.Join("contexts", "_template")) || strings.HasPrefix(source, filepath.Join(h.runtime.ProjectRoot, "terraform")))
}

// processTemplateComponent writes a shim at FullPath that points to the template module at the component source.
// It clears the old .tf files, then writes main.tf, variables.tf, and outputs.tf from the template module.
func (h *StandardModuleResolver) processTemplateComponent(component blueprintv1alpha1.TerraformComponent) error {
	sourcePath := component.Source
	moduleDir := component.FullPath
	componentID := component.GetID()
	if err := h.shims.MkdirAll(moduleDir, 0755); err != nil {
		return werror.Wrap(err, "failed to create module directory for %s", componentID)
	}
	if err := h.clearShimDirTfFiles(moduleDir); err != nil {
		return werror.Wrap(err, "failed to clear shim directory for %s", componentID)
	}

	relPath, err := h.shims.FilepathRel(moduleDir, sourcePath)
	if err != nil {
		return werror.Wrap(err, "failed to calculate relative path from %s to %s", moduleDir, sourcePath)
	}

	if err := h.writeShimMainTf(moduleDir, relPath); err != nil {
		return werror.Wrap(err, "failed to write main.tf for %s", componentID)
	}

	if err := h.writeShimVariablesTf(moduleDir, sourcePath, relPath); err != nil {
		return werror.Wrap(err, "failed to write variables.tf for %s", componentID)
	}

	if err := h.writeShimOutputsTf(moduleDir, sourcePath); err != nil {
		return werror.Wrap(err, "failed to write outputs.tf for %s", componentID)
	}

	return nil
}

// processRemoteComponent writes main.tf at FullPath and runs terraform init in that directory.
// It sets TF_DATA_DIR to .terraform/<componentID> in the scratch path, and runs the terraform check once.
// It reads the module path from the init output, then writes variables.tf and outputs.tf from that module.
func (h *StandardModuleResolver) processRemoteComponent(component blueprintv1alpha1.TerraformComponent, terraformChecked *bool) error {
	sourcePath := component.Source
	moduleDir := component.FullPath
	componentID := component.GetID()
	if err := h.shims.MkdirAll(moduleDir, 0755); err != nil {
		return werror.Wrap(err, "failed to create module directory for %s", componentID)
	}
	if err := h.clearShimDirTfFiles(moduleDir); err != nil {
		return werror.Wrap(err, "failed to clear shim directory for %s", componentID)
	}

	if filepath.IsAbs(sourcePath) {
		relPath, err := h.shims.FilepathRel(moduleDir, sourcePath)
		if err != nil {
			return werror.Wrap(err, "failed to calculate relative path from %s to %s", moduleDir, sourcePath)
		}
		sourcePath = relPath
	}

	if err := h.writeShimMainTf(moduleDir, sourcePath); err != nil {
		return werror.Wrap(err, "failed to write main.tf for %s", componentID)
	}

	if err := h.shims.Chdir(moduleDir); err != nil {
		return werror.Wrap(err, "failed to change to module directory for %s", componentID)
	}

	if h.runtime.WindsorScratchPath == "" {
		return fmt.Errorf("failed to get windsor scratch path: windsor scratch path is empty")
	}

	tfDataDir := filepath.Join(h.runtime.WindsorScratchPath, ".terraform", componentID)
	if err := h.shims.Setenv("TF_DATA_DIR", tfDataDir); err != nil {
		return werror.Wrap(err, "failed to set TF_DATA_DIR for %s", componentID)
	}

	if !*terraformChecked {
		if err := h.runtime.ToolsManager.CheckTerraform(); err != nil {
			return werror.Wrap(err, "failed to initialize terraform for %s", componentID)
		}
		*terraformChecked = true
	}

	terraformCommand := h.runtime.ToolsManager.GetTerraformCommand()
	output, err := h.runtime.Shell.ExecSilent(
		terraformCommand,
		"init",
		"--backend=false",
		"-input=false",
		"-upgrade",
		"-json",
	)
	if err != nil {
		return werror.Wrap(err, "failed to initialize terraform for %s", componentID)
	}

	modulePath := filepath.Join(h.runtime.WindsorScratchPath, ".terraform", componentID, "modules", "main", "terraform", component.Path)
	if detectedPath := h.detectModulePath(output); detectedPath != "" {
		if detectedPath != modulePath {
			fmt.Printf("\033[33mWarning: Using detected module path %s instead of standard path %s\033[0m\n", detectedPath, modulePath)
		}
		modulePath = detectedPath
	}

	if err := h.writeShimVariablesTf(moduleDir, modulePath, component.Source); err != nil {
		return werror.Wrap(err, "failed to write variables.tf for %s", componentID)
	}

	if err := h.writeShimOutputsTf(moduleDir, modulePath); err != nil {
		return werror.Wrap(err, "failed to write outputs.tf for %s", componentID)
	}

	return nil
}

// detectModulePath returns the first existing path from a "- main in <path>" log line of the terraform init JSON output.
// It skips a line that is not valid JSON. It returns an empty string if no line names an existing path.
func (h *StandardModuleResolver) detectModulePath(output string) string {
	for line := range strings.SplitSeq(output, "\n") {
		if line == "" {
			continue
		}
		var initOutput TerraformInitOutput
		if err := h.shims.JsonUnmarshal([]byte(line), &initOutput); err != nil {
			continue
		}
		if initOutput.Type != "log" {
			continue
		}
		startIdx := strings.Index(initOutput.Message, "- main in")
		if startIdx == -1 {
			continue
		}
		path := strings.TrimSpace(initOutput.Message[startIdx+len("- main in"):])
		if path == "" {
			continue
		}
		if _, err := h.shims.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// processLocalComponent writes a shim at FullPath for a local terraform component that has a name.
// The shim points to the module at terraform/<path> with a relative path. The module stays at its path.
// It does nothing for a component without a name.
func (h *StandardModuleResolver) processLocalComponent(component blueprintv1alpha1.TerraformComponent) error {
	if component.Name == "" {
		return nil
	}

	moduleDir := component.FullPath
	actualModulePath := filepath.Join(h.runtime.ProjectRoot, "terraform", component.Path)

	relPath, err := h.shims.FilepathRel(moduleDir, actualModulePath)
	if err != nil {
		return werror.Wrap(err, "failed to calculate relative path from %s to %s", moduleDir, actualModulePath)
	}

	if err := h.shims.MkdirAll(moduleDir, 0755); err != nil {
		return werror.Wrap(err, "failed to create module directory for %s", component.GetID())
	}
	if err := h.clearShimDirTfFiles(moduleDir); err != nil {
		return werror.Wrap(err, "failed to clear shim directory for %s", component.GetID())
	}

	if err := h.writeShimMainTf(moduleDir, relPath); err != nil {
		return werror.Wrap(err, "failed to write main.tf for %s", component.GetID())
	}

	if err := h.writeShimVariablesTf(moduleDir, actualModulePath, relPath); err != nil {
		return werror.Wrap(err, "failed to write variables.tf for %s", component.GetID())
	}

	if err := h.writeShimOutputsTf(moduleDir, actualModulePath); err != nil {
		return werror.Wrap(err, "failed to write outputs.tf for %s", component.GetID())
	}

	return nil
}

// shouldHandle reports whether source is a standard terraform module source.
// Standard sources are local paths, registry modules, git and hg repositories, HTTP URLs, and S3 or GCS buckets.
// It returns false for an OCI source.
func (h *StandardModuleResolver) shouldHandle(source string) bool {
	if source == "" {
		return false
	}

	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") {
		return true
	}

	if filepath.IsAbs(source) {
		return true
	}

	if h.isTerraformRegistryModule(source) {
		return true
	}

	if strings.HasPrefix(source, "github.com/") {
		return true
	}

	if strings.HasPrefix(source, "git@github.com:") {
		return true
	}

	if strings.HasPrefix(source, "bitbucket.org/") {
		return true
	}

	if strings.HasPrefix(source, "git::") {
		return true
	}

	if strings.HasPrefix(source, "hg::") {
		return true
	}

	if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "http://") {
		return true
	}

	if strings.HasPrefix(source, "s3::") {
		return true
	}

	if strings.HasPrefix(source, "gcs::") {
		return true
	}

	if strings.HasPrefix(source, "git@") && strings.Contains(source, ":") {
		return true
	}

	return false
}

// isTerraformRegistryModule reports whether source has the registry format namespace/name/provider.
// Each of the three parts must be non-empty and contain only letters, digits, hyphens, and underscores.
func (h *StandardModuleResolver) isTerraformRegistryModule(source string) bool {
	parts := strings.Split(source, "/")
	if len(parts) != 3 {
		return false
	}

	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, char := range part {
			if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
				(char >= '0' && char <= '9') || char == '-' || char == '_') {
				return false
			}
		}
	}

	return true
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure StandardModuleResolver implements ModuleResolver
var _ ModuleResolver = (*StandardModuleResolver)(nil)
