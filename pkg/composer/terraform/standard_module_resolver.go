package terraform

import (
	"fmt"
	"path/filepath"
	"strings"

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
func NewStandardModuleResolver(rt *runtime.Runtime, blueprintHandler blueprint.BlueprintHandler) *StandardModuleResolver {
	resolver := &StandardModuleResolver{
		BaseModuleResolver: NewBaseModuleResolver(rt, blueprintHandler),
		reset:              false,
	}

	return resolver
}

// =============================================================================
// Public Methods
// =============================================================================

// ProcessModules processes all standard terraform modules from the blueprint.
// It iterates over each terraform component, determines if the resolver should handle the source,
// creates the module directory, writes shim files (main.tf, variables.tf, outputs.tf), initializes
// the module with terraform, and determines the correct module path for shimming. Errors are returned
// if any step fails, ensuring that only valid and initialized modules are processed.
func (h *StandardModuleResolver) ProcessModules() error {
	if h.blueprintHandler == nil {
		return fmt.Errorf("blueprint handler not initialized")
	}

	components := h.blueprintHandler.GetTerraformComponents()

	for _, component := range components {
		if component.Source == "" {
			continue
		}

		if !h.shouldHandle(component.Source) {
			continue
		}

		moduleDir := component.FullPath
		if err := h.shims.MkdirAll(moduleDir, 0755); err != nil {
			return fmt.Errorf("failed to create module directory for %s: %w", component.Path, err)
		}

		if err := h.writeShimMainTf(moduleDir, component.Source); err != nil {
			return fmt.Errorf("failed to write main.tf for %s: %w", component.Path, err)
		}

		if err := h.shims.Chdir(moduleDir); err != nil {
			return fmt.Errorf("failed to change to module directory for %s: %w", component.Path, err)
		}

		contextPath := h.runtime.ConfigRoot
		if contextPath == "" {
			return fmt.Errorf("failed to get config root: config root is empty")
		}

		tfDataDir := filepath.Join(contextPath, ".terraform", component.Path)
		if err := h.shims.Setenv("TF_DATA_DIR", tfDataDir); err != nil {
			return fmt.Errorf("failed to set TF_DATA_DIR for %s: %w", component.Path, err)
		}

		output, err := h.runtime.Shell.ExecProgress(
			fmt.Sprintf("📥 Loading component %s", component.Path),
			"terraform",
			"init",
			"--backend=false",
			"-input=false",
			"-upgrade",
			"-json",
		)
		if err != nil {
			return fmt.Errorf("failed to initialize terraform for %s: %w", component.Path, err)
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

		modulePath := filepath.Join(contextPath, ".terraform", component.Path, "modules", "main", "terraform", component.Path)
		if detectedPath != "" {
			if detectedPath != modulePath {
				fmt.Printf("\033[33mWarning: Using detected module path %s instead of standard path %s\033[0m\n", detectedPath, modulePath)
			}
			modulePath = detectedPath
		}

		if err := h.writeShimVariablesTf(moduleDir, modulePath, component.Source); err != nil {
			return fmt.Errorf("failed to write variables.tf for %s: %w", component.Path, err)
		}

		if err := h.writeShimOutputsTf(moduleDir, modulePath); err != nil {
			return fmt.Errorf("failed to write outputs.tf for %s: %w", component.Path, err)
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

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
