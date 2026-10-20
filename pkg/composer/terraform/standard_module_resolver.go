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

// The StandardModuleResolver is a terraform module resolver for standard source types.
// It provides functionality to process git repositories, local paths, and other standard terraform module sources.
// The StandardModuleResolver acts as a specialized resolver within the terraform module system,
// handling module initialization, shim generation, and configuration for non-OCI terraform sources.

// =============================================================================
// Types
// =============================================================================

// TerraformInitOutput represents the JSON output structure from terraform init
type TerraformInitOutput struct {
	Type    string `json:"@type"`
	Message string `json:"@message"`
}

// StandardModuleResolver handles standard terraform module sources (git, local paths, etc.)
type StandardModuleResolver struct {
	*BaseModuleResolver
	reset bool
}

// =============================================================================
// Constructor
// =============================================================================

// NewStandardModuleResolver creates a new standard module resolver with the provided dependencies.
// If overrides are provided, any non-nil component in the override StandardModuleResolver will be used instead of creating a default.
// Panics if rt or blueprintHandler are nil.
func NewStandardModuleResolver(rt *runtime.Runtime, blueprintHandler blueprint.BlueprintHandler) *StandardModuleResolver {
	if rt == nil {
		panic("runtime is required")
	}
	if blueprintHandler == nil {
		panic("blueprint handler is required")
	}

	resolver := &StandardModuleResolver{
		BaseModuleResolver: NewBaseModuleResolver(rt, blueprintHandler),
		reset:              false,
	}

	return resolver
}

// =============================================================================
// Public Methods
// =============================================================================

// ProcessModules writes a shim for each terraform component that this resolver handles.
// A local component with a name gets a shim through processLocalComponent. A template source gets a shim that points to it.
// For a remote source, it writes main.tf and runs terraform init in the shim directory with TF_DATA_DIR in the scratch path.
// It reads the module path from the init output, then writes variables.tf and outputs.tf from that module.
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

		sourcePath := component.Source
		isTemplatePath := filepath.IsAbs(sourcePath) && (strings.Contains(sourcePath, filepath.Join("contexts", "_template")) || strings.HasPrefix(sourcePath, filepath.Join(h.runtime.ProjectRoot, "terraform")))
		if isTemplatePath {
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

			continue
		}

		shouldHandle := h.shouldHandle(component.Source)
		if !shouldHandle {
			continue
		}

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

		if !terraformChecked {
			if err := h.runtime.ToolsManager.CheckTerraform(); err != nil {
				return werror.Wrap(err, "failed to initialize terraform for %s", componentID)
			}
			terraformChecked = true
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

		detectedPath := ""
		for line := range strings.SplitSeq(output, "\n") {
			if line == "" {
				continue
			}
			var initOutput TerraformInitOutput
			if err := h.shims.JsonUnmarshal([]byte(line), &initOutput); err != nil {
				continue
			}
			if initOutput.Type == "log" {
				msg := initOutput.Message
				startIdx := strings.Index(msg, "- main in")
				if startIdx == -1 {
					continue
				}

				pathStart := startIdx + len("- main in")
				if pathStart >= len(msg) {
					continue
				}

				path := strings.TrimSpace(msg[pathStart:])
				if path == "" {
					continue
				}

				if _, err := h.shims.Stat(path); err == nil {
					detectedPath = path
					break
				}
			}
		}

		modulePath := filepath.Join(h.runtime.WindsorScratchPath, ".terraform", componentID, "modules", "main", "terraform", component.Path)
		if detectedPath != "" {
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
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

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

// shouldHandle determines if this resolver should handle the given source by checking
// if the source matches valid terraform module source patterns. This includes local paths,
// terraform registry modules, git repositories, HTTP URLs, and cloud storage buckets.
// It does not handle OCI sources or perform any blueprint handler lookups.
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

// isTerraformRegistryModule checks if the source is a Terraform Registry module by validating
// the format namespace/name/provider and ensuring each component contains only valid characters.
// Registry modules must have exactly three slash-separated parts, with each part containing
// only alphanumeric characters, hyphens, and underscores.
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
