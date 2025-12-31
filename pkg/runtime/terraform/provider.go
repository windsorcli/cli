// The TerraformProvider provides all terraform-specific operations including output capture,
// component resolution, and terraform command execution. It handles session-based caching
// to avoid repeated operations.

package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Types
// =============================================================================

// TerraformProvider defines the interface for Terraform operations
type TerraformProvider interface {
	FindRelativeProjectPath(directory ...string) (string, error)
	GenerateBackendOverride(directory string) error
	GetTerraformComponent(componentID string) *blueprintv1alpha1.TerraformComponent
	GetTerraformComponents() []blueprintv1alpha1.TerraformComponent
	GetOutputs(componentID string) (map[string]any, error)
	ClearCache()
}

// terraformProvider provides all terraform-specific operations with session caching.
type terraformProvider struct {
	configHandler config.ConfigHandler
	shell         shell.Shell
	toolsManager  tools.ToolsManager
	Shims         *Shims // Exported for testing
	cache         map[string]map[string]any
	components    []blueprintv1alpha1.TerraformComponent
	mu            sync.RWMutex
}

// TerraformArgs contains terraform arguments needed for output capture.
type TerraformArgs struct {
	TFDataDir string
	InitArgs  []string
}

// =============================================================================
// Constructor
// =============================================================================

// NewTerraformProvider creates a new TerraformProvider instance.
func NewTerraformProvider(
	configHandler config.ConfigHandler,
	shell shell.Shell,
	toolsManager tools.ToolsManager,
) TerraformProvider {
	return &terraformProvider{
		configHandler: configHandler,
		shell:         shell,
		toolsManager:  toolsManager,
		Shims:         NewShims(),
		cache:         make(map[string]map[string]any),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetOutputs fetches all outputs for a terraform component, using cache if available.
// Returns a map of output keys to values, or an error if outputs cannot be fetched.
func (p *terraformProvider) GetOutputs(componentID string) (map[string]any, error) {
	p.mu.RLock()
	if cached, exists := p.cache[componentID]; exists {
		p.mu.RUnlock()
		return cached, nil
	}
	p.mu.RUnlock()

	outputs, err := p.captureTerraformOutputs(componentID)
	if err != nil {
		return nil, fmt.Errorf("failed to capture outputs for component '%s': %w", componentID, err)
	}

	p.mu.Lock()
	if cached, exists := p.cache[componentID]; exists {
		p.mu.Unlock()
		return cached, nil
	}
	p.cache[componentID] = outputs
	p.mu.Unlock()

	return outputs, nil
}

// GetTerraformComponents loads and parses Terraform components from blueprint.yaml.
// Returns all TerraformComponent structs from blueprint.yaml with FullPath fields set.
// Components are cached after first load.
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

// GetTerraformComponent finds a terraform component by its path or name.
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

// GenerateBackendOverride creates the backend_override.tf file for the specified directory.
func (p *terraformProvider) GenerateBackendOverride(directory string) error {
	backend := p.configHandler.GetString("terraform.backend.type", "local")

	var backendConfig string
	switch backend {
	case "none":
		backendOverridePath := filepath.Join(directory, "backend_override.tf")
		if _, err := p.Shims.Stat(backendOverridePath); err == nil {
			if err := p.Shims.Remove(backendOverridePath); err != nil {
				return fmt.Errorf("error removing backend_override.tf: %w", err)
			}
		}
		return nil
	case "local":
		backendConfig = `terraform {
  backend "local" {}
}`
	case "s3":
		backendConfig = `terraform {
  backend "s3" {}
}`
	case "kubernetes":
		backendConfig = `terraform {
  backend "kubernetes" {}
}`
	case "azurerm":
		backendConfig = `terraform {
  backend "azurerm" {}
}`
	default:
		return fmt.Errorf("unsupported backend: %s", backend)
	}

	backendOverridePath := filepath.Join(directory, "backend_override.tf")
	err := p.Shims.WriteFile(backendOverridePath, []byte(backendConfig), os.ModePerm)
	if err != nil {
		return fmt.Errorf("error writing backend_override.tf: %w", err)
	}

	return nil
}

// ClearCache clears the session cache for all components.
func (p *terraformProvider) ClearCache() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = make(map[string]map[string]any)
	p.components = nil
}

// FindRelativeProjectPath locates the Terraform project path by checking the current
// directory (or provided directory) and its ancestors for Terraform files, returning the relative path if found.
func (p *terraformProvider) FindRelativeProjectPath(directory ...string) (string, error) {
	var currentPath string
	if len(directory) > 0 {
		currentPath = filepath.Clean(directory[0])
	} else {
		var err error
		currentPath, err = p.Shims.Getwd()
		if err != nil {
			return "", fmt.Errorf("error getting current directory: %w", err)
		}
	}

	globPattern := filepath.Join(currentPath, "*.tf")
	matches, err := p.Shims.Glob(globPattern)
	if err != nil {
		return "", fmt.Errorf("error finding project path: %w", err)
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

// =============================================================================
// Private Methods
// =============================================================================

// captureTerraformOutputs executes terraform output with proper environment setup for the specified component.
func (p *terraformProvider) captureTerraformOutputs(componentID string) (map[string]any, error) {
	components := p.GetTerraformComponents()
	var component *blueprintv1alpha1.TerraformComponent
	for i := range components {
		if components[i].Path == componentID || (components[i].Name != "" && components[i].Name == componentID) {
			component = &components[i]
			break
		}
	}
	if component == nil {
		return make(map[string]any), nil
	}

	absModulePath, err := p.resolveModulePath(component)
	if err != nil {
		return make(map[string]any), nil
	}

	terraformArgs, err := p.generateTerraformArgs(componentID)
	if err != nil {
		return make(map[string]any), nil
	}

	originalTFDataDir := p.Shims.Getenv("TF_DATA_DIR")

	if err := p.Shims.Setenv("TF_DATA_DIR", terraformArgs.TFDataDir); err != nil {
		return make(map[string]any), nil
	}

	if err := p.GenerateBackendOverride(absModulePath); err != nil {
		p.restoreEnvVar("TF_DATA_DIR", originalTFDataDir)
		return make(map[string]any), nil
	}

	cleanup := func() {
		backendOverridePath := filepath.Join(absModulePath, "backend_override.tf")
		if _, err := p.Shims.Stat(backendOverridePath); err == nil {
			_ = p.Shims.Remove(backendOverridePath)
		}
		p.restoreEnvVar("TF_DATA_DIR", originalTFDataDir)
	}
	defer cleanup()

	terraformCommand := p.toolsManager.GetTerraformCommand()
	if terraformCommand == "" {
		terraformCommand = "terraform"
	}
	outputArgs := []string{fmt.Sprintf("-chdir=%s", absModulePath), "output", "-json"}
	output, err := p.shell.ExecSilent(terraformCommand, outputArgs...)
	if err != nil {
		initArgs := []string{fmt.Sprintf("-chdir=%s", absModulePath), "init"}
		initArgs = append(initArgs, terraformArgs.InitArgs...)
		if _, initErr := p.shell.ExecSilent(terraformCommand, initArgs...); initErr != nil {
			return make(map[string]any), nil
		}
		output, err = p.shell.ExecSilent(terraformCommand, outputArgs...)
		if err != nil {
			return make(map[string]any), nil
		}
	}

	if strings.TrimSpace(output) == "" || strings.TrimSpace(output) == "{}" {
		return make(map[string]any), nil
	}

	var outputs map[string]any
	if err := p.Shims.JsonUnmarshal([]byte(output), &outputs); err != nil {
		return make(map[string]any), nil
	}

	result := make(map[string]any)
	for key, value := range outputs {
		if valueMap, ok := value.(map[string]any); ok {
			if outputValue, exists := valueMap["value"]; exists {
				result[key] = outputValue
			}
		}
	}

	return result, nil
}

// generateTerraformArgs generates terraform arguments needed for output capture.
func (p *terraformProvider) generateTerraformArgs(componentID string) (*TerraformArgs, error) {
	windsorScratchPath, err := p.configHandler.GetWindsorScratchPath()
	if err != nil {
		return nil, fmt.Errorf("error getting windsor scratch path: %w", err)
	}

	components := p.GetTerraformComponents()
	var component *blueprintv1alpha1.TerraformComponent
	for i := range components {
		if components[i].Path == componentID || (components[i].Name != "" && components[i].Name == componentID) {
			component = &components[i]
			break
		}
	}
	actualComponentID := componentID
	if component != nil {
		actualComponentID = component.GetID()
	}

	tfDataDir := filepath.Join(windsorScratchPath, "terraform", actualComponentID, ".terraform")

	initArgs := []string{}
	backend := p.configHandler.GetString("terraform.backend.type", "local")
	if backend != "none" {
		initArgs = append(initArgs, "-backend-config=backend_override.tf")
	}

	return &TerraformArgs{
		TFDataDir: tfDataDir,
		InitArgs:  initArgs,
	}, nil
}

// loadTerraformComponents loads and parses Terraform components from a blueprint.yaml file.
func (p *terraformProvider) loadTerraformComponents() []blueprintv1alpha1.TerraformComponent {
	configRoot, err := p.configHandler.GetConfigRoot()
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
		context := p.configHandler.GetContext()
		windsorScratchPath := filepath.Join(projectRoot, ".windsor", "contexts", context)
		for i := range blueprint.TerraformComponents {
			component := &blueprint.TerraformComponents[i]
			if component.Source != "" {
				component.FullPath = filepath.Join(windsorScratchPath, "terraform", component.Path)
			} else {
				component.FullPath = filepath.Join(projectRoot, "terraform", component.Path)
			}
		}
	}

	return blueprint.TerraformComponents
}

// resolveModulePath resolves the absolute module path for a terraform component.
func (p *terraformProvider) resolveModulePath(component *blueprintv1alpha1.TerraformComponent) (string, error) {
	if component.Name != "" {
		windsorScratchPath, err := p.configHandler.GetWindsorScratchPath()
		if err != nil {
			return "", err
		}
		return filepath.Join(windsorScratchPath, "terraform", component.Name), nil
	}

	if component.Source != "" {
		windsorScratchPath, err := p.configHandler.GetWindsorScratchPath()
		if err != nil {
			return "", err
		}
		return filepath.Join(windsorScratchPath, "terraform", component.Path), nil
	}

	projectRoot, err := p.shell.GetProjectRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(projectRoot, "terraform", component.Path), nil
}

// restoreEnvVar restores an environment variable to its original value or unsets it if it was empty.
func (p *terraformProvider) restoreEnvVar(key, originalValue string) {
	if originalValue != "" {
		_ = p.Shims.Setenv(key, originalValue)
	} else {
		_ = p.Shims.Unsetenv(key)
	}
}
