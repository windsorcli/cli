// The TerraformProvider runs the Terraform operations of Windsor.
// It resolves components, generates arguments and environment variables, and reads outputs.
// It keeps a session cache of outputs to prevent repeated Terraform runs.
// The provider registers the terraform_output helper with the expression evaluator.

package terraform

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Types
// =============================================================================

// terraformProvider provides all terraform-specific operations with session caching.
type terraformProvider struct {
	configHandler config.ConfigHandler
	shell         shell.Shell
	toolsManager  tools.ToolsManager
	evaluator     evaluator.ExpressionEvaluator
	Shims         *Shims // Exported for testing
	cache         map[string]map[string]any
	components    []blueprintv1alpha1.TerraformComponent
	configScope   map[string]any
	warningWriter io.Writer
	mu            sync.RWMutex
}

// TerraformArgs contains all the CLI arguments needed for terraform operations.
// It does not include environment variable formatting - that is handled by the env printer.
// KubernetesNamespace is the namespace the config sets for the kubernetes backend, and empty when none is set.
type TerraformArgs struct {
	TFDataDir           string
	InitArgs            []string
	PlanArgs            []string
	ApplyArgs           []string
	RefreshArgs         []string
	ImportArgs          []string
	DestroyArgs         []string
	PlanDestroyArgs     []string
	BackendConfig       string
	KubernetesNamespace string
}

// =============================================================================
// Interfaces
// =============================================================================

// TerraformProvider defines the interface for Terraform operations
type TerraformProvider interface {
	FindRelativeProjectPath(directory ...string) (string, error)
	IsInTerraformProject() bool
	GenerateBackendOverride(directory string) error
	GenerateTerraformArgs(componentID string, interactive bool) (*TerraformArgs, error)
	GetTerraformComponent(componentID string) *blueprintv1alpha1.TerraformComponent
	GetTerraformComponents() []blueprintv1alpha1.TerraformComponent
	GetTerraformComponentForPath(directory string) *blueprintv1alpha1.TerraformComponent
	SetTerraformComponents(components []blueprintv1alpha1.TerraformComponent)
	SetConfigScope(scope map[string]any)
	GetTerraformOutputs(componentID string) (map[string]any, error)
	CacheOutputs(componentID string) error
	GetTFDataDir(componentID string) (string, error)
	GetStatePath(componentID string) (string, error)
	ListLocalStateComponentIDs() ([]string, error)
	BackendConfigComplete() bool
	GetEnvVars(componentID string, interactive bool) (map[string]string, []string, *TerraformArgs, error)
	FormatArgsForEnv(args []string) string
	ClearCache()
	TerraformScopedEnvKeys() ([]string, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewTerraformProvider creates a new TerraformProvider instance and registers its helper functions with the evaluator.
// Panics if configHandler, shell, toolsManager, or evaluator are nil.
func NewTerraformProvider(
	configHandler config.ConfigHandler,
	shell shell.Shell,
	toolsManager tools.ToolsManager,
	evaluator evaluator.ExpressionEvaluator,
) TerraformProvider {
	if configHandler == nil {
		panic("config handler is required")
	}
	if shell == nil {
		panic("shell is required")
	}
	if toolsManager == nil {
		panic("tools manager is required")
	}
	if evaluator == nil {
		panic("evaluator is required")
	}

	provider := &terraformProvider{
		configHandler: configHandler,
		shell:         shell,
		toolsManager:  toolsManager,
		evaluator:     evaluator,
		Shims:         NewShims(),
		cache:         make(map[string]map[string]any),
		warningWriter: os.Stderr,
	}

	provider.registerTerraformOutputHelper(evaluator)

	return provider
}

// =============================================================================
// Public Methods
// =============================================================================

// FindRelativeProjectPath returns the relative path to a Terraform project from the current
// working directory or the specified directory. It locates Terraform files (*.tf) and resolves
// the path relative to "terraform" or "contexts" directory markers in the path structure.
// Returns an empty string if no Terraform files are found, or an error if file system inspection fails.
func (p *terraformProvider) FindRelativeProjectPath(directory ...string) (string, error) {
	var currentPath string
	if len(directory) > 0 {
		currentPath = filepath.Clean(directory[0])
	} else {
		var err error
		currentPath, err = p.Shims.Getwd()
		if err != nil {
			return "", werror.Wrap(err, "error getting current directory")
		}
	}

	globPattern := filepath.Join(currentPath, "*.tf")
	matches, err := p.Shims.Glob(globPattern)
	if err != nil {
		return "", werror.Wrap(err, "error finding project path")
	}
	if len(matches) == 0 {
		return "", nil
	}

	pathParts := strings.Split(currentPath, string(os.PathSeparator))
	terraformIdx := -1
	contextsIdx := -1
	for i := len(pathParts) - 1; i >= 0; i-- {
		if strings.EqualFold(pathParts[i], "terraform") {
			terraformIdx = i
		}
		if strings.EqualFold(pathParts[i], "contexts") {
			contextsIdx = i
		}
	}

	if terraformIdx >= 0 {
		relativePath := filepath.Join(pathParts[terraformIdx+1:]...)
		return filepath.ToSlash(relativePath), nil
	}

	if contextsIdx >= 0 {
		relativePath := filepath.Join(pathParts[contextsIdx+1:]...)
		return filepath.ToSlash(relativePath), nil
	}

	return "", nil
}

// IsInTerraformProject checks if the current working directory is within a Terraform project.
// A Terraform project is identified by the presence of .tf files in the current directory or parent directories.
// Returns true if .tf files are found and a valid project path can be resolved, false otherwise.
func (p *terraformProvider) IsInTerraformProject() bool {
	projectPath, err := p.FindRelativeProjectPath()
	if err != nil {
		return false
	}
	return projectPath != ""
}

// GetTerraformComponent finds a terraform component by its path or name from the loaded blueprint components.
// It searches through all Terraform components and matches against both the Path and Name fields.
// Returns the component if found, or nil if not found.
func (p *terraformProvider) GetTerraformComponent(componentID string) *blueprintv1alpha1.TerraformComponent {
	components := p.GetTerraformComponents()
	for i := range components {
		if components[i].Path == componentID || (components[i].Name != "" && components[i].Name == componentID) {
			return &components[i]
		}
	}
	return nil
}

// GetTerraformComponentForPath finds the loaded blueprint component whose resolved FullPath
// matches directory. Terraform module directories are shared on disk across contexts, so this
// confirms directory actually belongs to the active context's blueprint before a caller treats
// it as one of its components. Returns nil if no component resolves to directory.
func (p *terraformProvider) GetTerraformComponentForPath(directory string) *blueprintv1alpha1.TerraformComponent {
	directory = filepath.Clean(directory)
	components := p.GetTerraformComponents()
	for i := range components {
		if filepath.Clean(components[i].FullPath) == directory {
			return &components[i]
		}
	}
	return nil
}

// GetTerraformComponents loads and parses Terraform components from blueprint.yaml.
// It retrieves the blueprint configuration root, reads the blueprint.yaml file, and computes the FullPath
// for each Terraform component based on the presence of a Source field and the resolved project root.
// Returns all TerraformComponent structs from blueprint.yaml with FullPath fields set.
// Components are cached after first load to avoid repeated file I/O operations.
// If components have been set via SetTerraformComponents, those are returned instead.
func (p *terraformProvider) GetTerraformComponents() []blueprintv1alpha1.TerraformComponent {
	p.mu.RLock()
	if p.components != nil {
		p.mu.RUnlock()
		return p.components
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.components != nil {
		return p.components
	}

	p.components = p.loadTerraformComponents()
	return p.components
}

// SetTerraformComponents sets the terraform components directly, bypassing file loading.
// This allows the provider to use in-memory components with inputs preserved from blueprint composition.
// Components set via this method take precedence over components loaded from blueprint.yaml.
func (p *terraformProvider) SetTerraformComponents(components []blueprintv1alpha1.TerraformComponent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.components = components
}

// SetConfigScope sets the config scope (facet config blocks) used when evaluating component inputs
// in GetEnvVars, so expressions like docker_desktop_common_patches.patchVars.common_config_patches resolve.
func (p *terraformProvider) SetConfigScope(scope map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.configScope = scope
}

// =============================================================================
// Private Methods
// =============================================================================

// loadTerraformComponents loads and parses Terraform components from a blueprint.yaml file.
// It retrieves the blueprint configuration root, reads the blueprint.yaml file, unmarshals it into
// a Blueprint struct, and computes the FullPath for each Terraform component based on the presence
// of a Source field and the resolved project root. Components with a Source are located in Windsor scratch
// space, while local components are in the project root. Returns the list of TerraformComponents found with FullPath fields set.
func (p *terraformProvider) loadTerraformComponents() []blueprintv1alpha1.TerraformComponent {
	contextName, configRoot, _, err := p.providerScope()
	if err != nil {
		return []blueprintv1alpha1.TerraformComponent{}
	}

	blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
	data, err := p.Shims.ReadFile(blueprintPath)
	if err != nil {
		return []blueprintv1alpha1.TerraformComponent{}
	}

	var blueprint blueprintv1alpha1.Blueprint
	if err := p.Shims.YamlUnmarshal(data, &blueprint); err != nil {
		return []blueprintv1alpha1.TerraformComponent{}
	}

	projectRoot, err := p.shell.GetProjectRoot()
	if err == nil {
		windsorScratchPath := filepath.Join(projectRoot, ".windsor", "contexts", contextName)
		for i := range blueprint.TerraformComponents {
			component := &blueprint.TerraformComponents[i]
			componentID := component.GetID()
			useScratchPath := component.Name != "" || component.Source != ""
			if useScratchPath {
				component.FullPath = filepath.Join(windsorScratchPath, "terraform", componentID)
			} else {
				component.FullPath = filepath.Join(projectRoot, "terraform", componentID)
			}
		}
	}

	return blueprint.TerraformComponents
}

// resolveModulePath returns the absolute path to the Terraform module for the specified component.
// If FullPath is already set on the component, it is used directly. Otherwise, the path is computed
// as a fallback using the component ID (Name if present, otherwise Path) as the directory name.
// For components with a Name or Source, the path is in Windsor scratch space (.windsor/contexts/<context>/terraform/<ID>).
// For local components without a Name or Source, the path is in the project root (terraform/<Path>).
// In production code, FullPath should always be set, making the fallback logic primarily for edge cases.
// Returns the computed absolute path or an error if the required project or context root cannot be determined.
func (p *terraformProvider) resolveModulePath(component *blueprintv1alpha1.TerraformComponent) (string, error) {
	if component.FullPath != "" {
		return component.FullPath, nil
	}

	componentID := component.GetID()

	useScratchPath := component.Name != "" || component.Source != ""
	if useScratchPath {
		_, _, windsorScratchPath, err := p.providerScope()
		if err != nil {
			return "", err
		}
		return filepath.Join(windsorScratchPath, "terraform", componentID), nil
	}

	projectRoot, err := p.shell.GetProjectRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(projectRoot, "terraform", componentID), nil
}
