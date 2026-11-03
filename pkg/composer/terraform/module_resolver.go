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
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Interfaces
// =============================================================================

// ModuleResolver writes the shim modules and the tfvars files of the terraform components.
type ModuleResolver interface {
	ProcessModules() error
	GenerateTfvars(overwrite bool) error
}

// =============================================================================
// Types
// =============================================================================

// BaseModuleResolver holds the shared dependencies and the tfvars logic of the module resolvers.
type BaseModuleResolver struct {
	shims            *Shims
	runtime          *runtime.Runtime
	blueprintHandler blueprint.BlueprintHandler
	evaluator        evaluator.ExpressionEvaluator
}

// =============================================================================
// Constructor
// =============================================================================

// NewBaseModuleResolver creates a base module resolver from the runtime and the blueprint handler.
// It panics if rt, blueprintHandler, or rt.Evaluator is nil.
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

// GenerateTfvars writes a terraform.tfvars file for each terraform component in the blueprint.
// It evaluates the component inputs against the current configuration.
// It does not write an input whose value still contains a deferred expression.
// It always replaces the old tfvars files, so the overwrite argument has no effect.
// It stops at the first evaluation or file error.
func (h *BaseModuleResolver) GenerateTfvars(overwrite bool) error {
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
				return werror.Wrap(err, "failed to evaluate inputs for component %s", component.GetID())
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
			return werror.Wrap(err, "failed to generate tfvars for component %s", component.Path)
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// generateComponentTfvars writes the tfvars file for one component at
// .windsor/contexts/<context>/terraform/<componentID>/terraform.tfvars, for local and remote components.
// It removes the old .tfvars files in that directory first, and reads the variables from the module directory.
func (h *BaseModuleResolver) generateComponentTfvars(projectRoot string, component blueprintv1alpha1.TerraformComponent, componentValues map[string]any) error {
	modulePath, err := h.findModulePathForComponent(projectRoot, component)
	if err != nil {
		return werror.Wrap(err, "failed to find module path for component %s", component.GetID())
	}

	componentID := component.GetID()
	moduleTfvarsPath := filepath.Join(projectRoot, ".windsor", "contexts", h.runtime.ContextName, "terraform", componentID, "terraform.tfvars")
	if err := h.removeTfvarsFiles(filepath.Dir(moduleTfvarsPath)); err != nil {
		return werror.Wrap(err, "failed cleaning existing .tfvars in module dir %s", filepath.Dir(moduleTfvarsPath))
	}
	if err := h.generateTfvarsFile(moduleTfvarsPath, modulePath, componentValues, component.Source); err != nil {
		return werror.Wrap(err, "failed to generate module tfvars file")
	}

	return nil
}

// findModulePathForComponent returns the module directory for a terraform component.
// For a template source (an absolute path under contexts/_template or terraform/), it returns the source.
// For a component with a name or a source, it returns the shim at .windsor/contexts/<context>/terraform/<componentID>.
// For a local component without a name, it returns terraform/<path>.
// It returns an error if the directory does not exist. A missing local module error has a remediation hint.
func (h *BaseModuleResolver) findModulePathForComponent(projectRoot string, component blueprintv1alpha1.TerraformComponent) (string, error) {
	componentID := component.GetID()

	if filepath.IsAbs(component.Source) && (strings.Contains(component.Source, filepath.Join("contexts", "_template")) || strings.HasPrefix(component.Source, filepath.Join(projectRoot, "terraform"))) {
		modulePath := component.Source

		if _, err := h.shims.Stat(modulePath); err != nil {
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
		if !useScratchPath {
			return "", werror.New(fmt.Sprintf("module directory not found for component %s at %s", component.GetID(), modulePath), "Make sure the component path in the blueprint matches a directory under terraform/.", nil)
		}
		return "", fmt.Errorf("module directory not found for component %s at %s", component.GetID(), modulePath)
	}

	return modulePath, nil
}

// removeTfvarsFiles removes the .tfvars files directly under dir, so that no old tfvars stay before regeneration.
// It does nothing if dir does not exist.
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
