// The ModuleResolver is a resolver that prepares the Terraform modules of the blueprint components.
// It provides the ModuleResolver interface and the BaseModuleResolver that the standard and OCI resolvers embed.
// GenerateTfvars writes a tfvars file for each component from its inputs and the variables of its module.
// The tfvars writers are in tfvars.go, and the shim module writers are in module_shim.go.

package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Interfaces
// =============================================================================

// ModuleResolver processes terraform module sources and generates appropriate module configurations
type ModuleResolver interface {
	ProcessModules() error
	GenerateTfvars(overwrite bool) error
}

// =============================================================================
// Types
// =============================================================================

// BaseModuleResolver provides common functionality for all module resolvers
type BaseModuleResolver struct {
	shims            *Shims
	runtime          *runtime.Runtime
	blueprintHandler blueprint.BlueprintHandler
	evaluator        evaluator.ExpressionEvaluator
	reset            bool
}

// =============================================================================
// Constructor
// =============================================================================

// NewBaseModuleResolver creates a new base module resolver with the provided dependencies.
// If overrides are provided, any non-nil component in the override BaseModuleResolver will be used instead of creating a default.
// Panics if rt, blueprintHandler, or rt.Evaluator are nil.
func NewBaseModuleResolver(rt *runtime.Runtime, blueprintHandler blueprint.BlueprintHandler) *BaseModuleResolver {
	if rt == nil {
		panic("runtime is required")
	}
	if blueprintHandler == nil {
		panic("blueprint handler is required")
	}
	if rt.Evaluator == nil {
		panic("evaluator is required on runtime")
	}

	return &BaseModuleResolver{
		shims:            NewShims(),
		runtime:          rt,
		blueprintHandler: blueprintHandler,
		evaluator:        rt.Evaluator,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GenerateTfvars creates Terraform configuration files, including tfvars files, for all blueprint components.
// It processes template data keyed by "terraform/<module_path>", generating tfvars files at
// contexts/<context>/terraform/<module_path>.tfvars. For each entry in the input data, it skips keys
// not prefixed with "terraform/" and skips components not present in the blueprint. For all components
// in the blueprint, it ensures a tfvars file is generated if not already handled by the input data.
// The method uses the blueprint handler to retrieve TerraformComponents and parses variables from all
// .tf files in the module directory based on component source (remote or local). Module resolution is handled by pkg/terraform.
// Input expressions are evaluated against the current configuration before being written to tfvars.
func (h *BaseModuleResolver) GenerateTfvars(overwrite bool) error {
	h.reset = overwrite

	projectRoot := h.runtime.ProjectRoot

	components := h.blueprintHandler.GetTerraformComponents()

	for _, component := range components {
		componentValues := component.Inputs
		if componentValues == nil {
			componentValues = make(map[string]any)
		}

		nonDeferredValues := make(map[string]any)
		for key, value := range componentValues {
			evaluated, err := blueprint.EvaluateWithOrigins(
				h.evaluator, key, value, component.InputOrigins, nil, false,
			)
			if err != nil {
				return fmt.Errorf("failed to evaluate inputs for component %s: %w", component.GetID(), err)
			}
			if evaluator.ContainsExpression(evaluated) {
				continue
			}
			if s, ok := evaluated.(string); ok && strings.Contains(s, "${") {
				continue
			}
			nonDeferredValues[key] = evaluated
		}

		if err := h.generateComponentTfvars(projectRoot, component, nonDeferredValues); err != nil {
			return fmt.Errorf("failed to generate tfvars for component %s: %w", component.Path, err)
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// validateAndSanitizePath sanitizes a file path for safe extraction by removing path traversal sequences
// and rejecting absolute paths. Returns the cleaned path if valid, or an error if the path is unsafe.
func (h *BaseModuleResolver) validateAndSanitizePath(path string) (string, error) {
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return "", fmt.Errorf("path contains directory traversal sequence: %s", path)
	}
	if strings.HasPrefix(cleanPath, string(filepath.Separator)) || (len(cleanPath) >= 2 && cleanPath[1] == ':' && (cleanPath[0] >= 'A' && cleanPath[0] <= 'Z' || cleanPath[0] >= 'a' && cleanPath[0] <= 'z')) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", path)
	}
	return cleanPath, nil
}

// generateComponentTfvars generates tfvars files for a single Terraform component.
// All components write tfvars files to .windsor/contexts/<context>/terraform/<componentID>/terraform.tfvars,
// regardless of whether they have a Source (remote) or not (local). This unifies the behavior
// between local templates and OCI artifacts, preventing writes to the contexts folder.
// Returns an error if tfvars file generation fails.
func (h *BaseModuleResolver) generateComponentTfvars(projectRoot string, component blueprintv1alpha1.TerraformComponent, componentValues map[string]any) error {
	modulePath, err := h.findModulePathForComponent(projectRoot, component)
	if err != nil {
		return fmt.Errorf("failed to find module path for component %s: %w", component.GetID(), err)
	}

	componentID := component.GetID()
	moduleTfvarsPath := filepath.Join(projectRoot, ".windsor", "contexts", h.runtime.ContextName, "terraform", componentID, "terraform.tfvars")
	if err := h.removeTfvarsFiles(filepath.Dir(moduleTfvarsPath)); err != nil {
		return fmt.Errorf("failed cleaning existing .tfvars in module dir %s: %w", filepath.Dir(moduleTfvarsPath), err)
	}
	if err := h.generateTfvarsFile(moduleTfvarsPath, modulePath, componentValues, component.Source); err != nil {
		return fmt.Errorf("failed to generate module tfvars file: %w", err)
	}

	return nil
}

// findModulePathForComponent returns the path to the module directory for the specified Terraform component.
// For components with a name, the path is .windsor/contexts/<context>/terraform/<name> (where the shim is located).
// For components without a name but with a Source, the path is .windsor/contexts/<context>/terraform/<component.Path>.
// For local components without a name, the path is terraform/<component.Path> (the actual module location).
// For template sources (absolute paths containing contexts/_template), returns the actual template module path.
// Returns the module directory path if it exists, or an error if not found.
func (h *BaseModuleResolver) findModulePathForComponent(projectRoot string, component blueprintv1alpha1.TerraformComponent) (string, error) {
	componentID := component.GetID()

	if filepath.IsAbs(component.Source) && (strings.Contains(component.Source, filepath.Join("contexts", "_template")) || strings.HasPrefix(component.Source, filepath.Join(projectRoot, "terraform"))) {
		modulePath := component.Source

		if _, err := h.shims.Stat(modulePath); err != nil {
			if os.IsNotExist(err) {
				_, _ = h.shims.ReadDir(filepath.Dir(modulePath))
			}
			return "", fmt.Errorf("module directory not found for component %s at %s", component.GetID(), modulePath)
		}
		return modulePath, nil
	}

	useScratchPath := component.Name != "" || component.Source != ""
	var modulePath string
	if useScratchPath {
		modulePath = filepath.Join(projectRoot, ".windsor", "contexts", h.runtime.ContextName, "terraform", componentID)
	} else {
		modulePath = filepath.Join(projectRoot, "terraform", componentID)
	}

	if _, err := h.shims.Stat(modulePath); err != nil {
		return "", fmt.Errorf("module directory not found for component %s at %s", component.GetID(), modulePath)
	}

	return modulePath, nil
}

// removeTfvarsFiles removes any .tfvars files directly under the specified directory.
// This is used to ensure module directories do not retain stale tfvars prior to regeneration.
func (h *BaseModuleResolver) removeTfvarsFiles(dir string) error {
	if _, err := h.shims.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	entries, err := h.shims.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(strings.ToLower(name), ".tfvars") {
			fullPath := filepath.Join(dir, name)
			if err := h.shims.RemoveAll(fullPath); err != nil {
				return err
			}
		}
	}

	return nil
}
