// The TerraformProvider provides all terraform-specific operations including output capture,
// component resolution, and terraform command execution. It handles session-based caching
// to avoid repeated operations.

package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

// TerraformArgs contains all the CLI arguments needed for terraform operations.
// It does not include environment variable formatting - that is handled by the env printer.
type TerraformArgs struct {
	ModulePath      string
	TFDataDir       string
	InitArgs        []string
	PlanArgs        []string
	ApplyArgs       []string
	RefreshArgs     []string
	ImportArgs      []string
	DestroyArgs     []string
	PlanDestroyArgs []string
	BackendConfig   string
}

// =============================================================================
// Interfaces
// =============================================================================

// TerraformProvider defines the interface for Terraform operations
type TerraformProvider interface {
	FindRelativeProjectPath(directory ...string) (string, error)
	GenerateBackendOverride(directory string) error
	GenerateTerraformArgs(componentID, modulePath string, interactive bool) (*TerraformArgs, error)
	GetTerraformComponent(componentID string) *blueprintv1alpha1.TerraformComponent
	GetTerraformComponents() []blueprintv1alpha1.TerraformComponent
	GetOutputs(componentID string) (map[string]any, error)
	GetTFDataDir(componentID string) (string, error)
	ClearCache()
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

// GenerateTerraformArgs constructs Terraform CLI arguments used for the specified component ID, module path, and interaction mode.
// This function discovers applicable var files, configures backend arguments, and assembles all common Terraform command arguments for init, plan, apply, destroy, import, and refresh.
// No environment variable formatting is performed; formatting and printing of environment variables is delegated to the calling context.
// componentID is used for tfstate paths, var file lookups, and backend configuration. modulePath is the filesystem path to the terraform module.
// Returns a fully populated TerraformArgs structure or an error if processing or lookup fails.
func (p *terraformProvider) GenerateTerraformArgs(componentID, modulePath string, interactive bool) (*TerraformArgs, error) {
	configRoot, err := p.configHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error getting config root: %w", err)
	}

	windsorScratchPath, err := p.configHandler.GetWindsorScratchPath()
	if err != nil {
		return nil, fmt.Errorf("error getting windsor scratch path: %w", err)
	}

	component := p.GetTerraformComponent(componentID)
	actualComponentID := componentID
	if component != nil {
		actualComponentID = component.GetID()
	}

	patterns := []string{
		filepath.Join(windsorScratchPath, "terraform", actualComponentID, "terraform.tfvars"),
		filepath.Join(configRoot, "terraform", actualComponentID+".tfvars"),
		filepath.Join(configRoot, "terraform", actualComponentID+".tfvars.json"),
	}
	if component != nil && component.Path != actualComponentID {
		patterns = append(patterns,
			filepath.Join(configRoot, "terraform", component.Path+".tfvars"),
			filepath.Join(configRoot, "terraform", component.Path+".tfvars.json"),
		)
	}

	var varFileArgs []string
	for _, pattern := range patterns {
		if _, err := p.Shims.Stat(filepath.FromSlash(pattern)); err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("error checking file: %w", err)
			}
		} else {
			slashPath := filepath.ToSlash(pattern)
			varFileArgs = append(varFileArgs, fmt.Sprintf("-var-file=%s", slashPath))
		}
	}

	tfDataDir, err := p.GetTFDataDir(actualComponentID)
	if err != nil {
		return nil, fmt.Errorf("error getting TF_DATA_DIR: %w", err)
	}
	tfPlanPath := filepath.ToSlash(filepath.Join(tfDataDir, "terraform.tfplan"))

	backendConfigArgs, err := p.generateBackendConfigArgs(actualComponentID, configRoot)
	if err != nil {
		return nil, fmt.Errorf("error generating backend config args: %w", err)
	}

	initArgs := []string{"-backend=true", "-force-copy", "-upgrade"}
	initArgs = append(initArgs, backendConfigArgs...)

	planArgs := []string{fmt.Sprintf("-out=%s", tfPlanPath)}
	planArgs = append(planArgs, varFileArgs...)

	applyArgs := []string{}
	refreshArgs := []string{}
	refreshArgs = append(refreshArgs, varFileArgs...)

	planDestroyArgs := []string{"-destroy"}
	planDestroyArgs = append(planDestroyArgs, varFileArgs...)

	destroyArgs := []string{}
	if !interactive {
		destroyArgs = append(destroyArgs, "-auto-approve")
	}
	destroyArgs = append(destroyArgs, varFileArgs...)

	if component != nil && component.Parallelism != nil {
		parallelismArg := fmt.Sprintf("-parallelism=%d", *component.Parallelism)
		applyArgs = append(applyArgs, parallelismArg)
		destroyArgs = append(destroyArgs, parallelismArg)
	}

	applyArgs = append(applyArgs, tfPlanPath)

	return &TerraformArgs{
		ModulePath:      modulePath,
		TFDataDir:       strings.TrimSpace(tfDataDir),
		InitArgs:        initArgs,
		PlanArgs:        planArgs,
		ApplyArgs:       applyArgs,
		RefreshArgs:     refreshArgs,
		ImportArgs:      varFileArgs,
		DestroyArgs:     destroyArgs,
		PlanDestroyArgs: planDestroyArgs,
		BackendConfig:   strings.Join(backendConfigArgs, " "),
	}, nil
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

// GetTFDataDir calculates the TF_DATA_DIR path for a given component ID.
// It resolves the component ID to the actual component ID (using GetID if component exists),
// then constructs the path as ${windsorScratchPath}/.terraform/${componentID}.
// Returns the path with forward slashes or an error if scratch path lookup fails.
func (p *terraformProvider) GetTFDataDir(componentID string) (string, error) {
	windsorScratchPath, err := p.configHandler.GetWindsorScratchPath()
	if err != nil {
		return "", fmt.Errorf("error getting windsor scratch path: %w", err)
	}

	component := p.GetTerraformComponent(componentID)
	actualComponentID := componentID
	if component != nil {
		actualComponentID = component.GetID()
	}

	tfDataDir := filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform", actualComponentID))
	return tfDataDir, nil
}

// ClearCache clears the session cache for all components.
func (p *terraformProvider) ClearCache() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = make(map[string]map[string]any)
	p.components = nil
}

// =============================================================================
// Private Methods
// =============================================================================

// captureTerraformOutputs executes terraform output with proper environment setup for the specified component.
func (p *terraformProvider) captureTerraformOutputs(componentID string) (map[string]any, error) {
	component := p.GetTerraformComponent(componentID)
	if component == nil {
		return make(map[string]any), nil
	}

	absModulePath, err := p.resolveModulePath(component)
	if err != nil {
		return make(map[string]any), nil
	}

	terraformArgs, err := p.GenerateTerraformArgs(componentID, absModulePath, false)
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
		chdirInitArgs := []string{fmt.Sprintf("-chdir=%s", absModulePath), "init"}
		chdirInitArgs = append(chdirInitArgs, terraformArgs.InitArgs...)
		if _, initErr := p.shell.ExecSilent(terraformCommand, chdirInitArgs...); initErr != nil {
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

// generateBackendConfigArgs constructs the -backend-config CLI arguments for Terraform based on project configuration.
// This method determines the backend type from the configuration, assembles key-value argument pairs for supported
// backend types (local, s3, kubernetes, azurerm), and detects the presence of backend.tfvars in the context root or
// a terraform/ fallback subdirectory. If found, it includes a -backend-config pointing to that file.
// Returns raw CLI arguments without shell quoting; formatting for environment variables is handled by the calling context.
// Returns a slice of backend configuration arguments or an error if required configuration or paths are unavailable.
func (p *terraformProvider) generateBackendConfigArgs(projectPath, configRoot string) ([]string, error) {
	var backendConfigArgs []string
	backend := p.configHandler.GetString("terraform.backend.type", "local")

	addBackendConfigArg := func(key, value string) {
		if value != "" {
			backendConfigArgs = append(backendConfigArgs, fmt.Sprintf("-backend-config=%s=%s", key, filepath.ToSlash(value)))
		}
	}

	if context := p.configHandler.GetContext(); context != "" {
		backendTfvarsPath := filepath.Join(configRoot, "backend.tfvars")
		if _, err := p.Shims.Stat(backendTfvarsPath); err != nil {
			backendTfvarsPath = filepath.Join(configRoot, "terraform", "backend.tfvars")
			if _, err := p.Shims.Stat(backendTfvarsPath); err != nil {
				backendTfvarsPath = ""
			}
		}
		if backendTfvarsPath != "" {
			absBackendTfvarsPath, err := filepath.Abs(backendTfvarsPath)
			if err == nil {
				backendConfigArgs = append(backendConfigArgs, fmt.Sprintf("-backend-config=%s", filepath.ToSlash(absBackendTfvarsPath)))
			}
		}
	}

	prefix := p.configHandler.GetString("terraform.backend.prefix", "")

	switch backend {
	case "none":
		return []string{}, nil
	case "local":
		windsorScratchPath, err := p.configHandler.GetWindsorScratchPath()
		if err != nil {
			return nil, fmt.Errorf("error getting windsor scratch path: %w", err)
		}
		path := filepath.Join(windsorScratchPath, ".tfstate")
		if prefix != "" {
			path = filepath.Join(path, prefix)
		}
		path = filepath.Join(path, projectPath, "terraform.tfstate")
		addBackendConfigArg("path", filepath.ToSlash(path))
	case "s3":
		keyPath := fmt.Sprintf("%s%s", prefix, filepath.ToSlash(filepath.Join(projectPath, "terraform.tfstate")))
		addBackendConfigArg("key", keyPath)
		if backend := p.configHandler.GetConfig().Terraform.Backend.S3; backend != nil {
			if err := p.processBackendConfig(backend, addBackendConfigArg); err != nil {
				return nil, fmt.Errorf("error processing S3 backend config: %w", err)
			}
		}
	case "kubernetes":
		secretSuffix := projectPath
		if prefix != "" {
			secretSuffix = fmt.Sprintf("%s-%s", strings.ReplaceAll(prefix, "/", "-"), secretSuffix)
		}
		secretSuffix = sanitizeForK8s(secretSuffix)
		addBackendConfigArg("secret_suffix", secretSuffix)
		if backend := p.configHandler.GetConfig().Terraform.Backend.Kubernetes; backend != nil {
			if err := p.processBackendConfig(backend, addBackendConfigArg); err != nil {
				return nil, fmt.Errorf("error processing Kubernetes backend config: %w", err)
			}
		}
	case "azurerm":
		keyPath := fmt.Sprintf("%s%s", prefix, filepath.ToSlash(filepath.Join(projectPath, "terraform.tfstate")))
		addBackendConfigArg("key", keyPath)
		if backend := p.configHandler.GetConfig().Terraform.Backend.AzureRM; backend != nil {
			if err := p.processBackendConfig(backend, addBackendConfigArg); err != nil {
				return nil, fmt.Errorf("error processing AzureRM backend config: %w", err)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported backend: %s", backend)
	}

	return backendConfigArgs, nil
}

// processBackendConfig marshals the provided backendConfig to YAML, then unmarshals it into a map structure.
// It traverses the resulting map, applying each key-value pair to the provided addArg function. Nested configuration
// objects result in compound keys using dot notation. Returns an error if marshalling or unmarshalling fails.
func (p *terraformProvider) processBackendConfig(backendConfig any, addArg func(key, value string)) error {
	yamlData, err := p.Shims.YamlMarshal(backendConfig)
	if err != nil {
		return fmt.Errorf("error marshalling backend to YAML: %w", err)
	}

	var configMap map[string]any
	if err := p.Shims.YamlUnmarshal(yamlData, &configMap); err != nil {
		return fmt.Errorf("error unmarshalling backend YAML: %w", err)
	}

	processMap("", configMap, addArg)
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

// processMap traverses the provided configMap, applying each key-value pair to the addArg function.
// Nested maps are handled recursively, forming compound keys with dot notation.
// Supports string, bool, int, uint64, []any (string slice only), and nested map[string]any types.
func processMap(prefix string, configMap map[string]any, addArg func(key, value string)) {
	keys := make([]string, 0, len(configMap))
	for key := range configMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		fullKey := key
		if prefix != "" {
			fullKey = fmt.Sprintf("%s.%s", prefix, key)
		}

		switch v := configMap[key].(type) {
		case string:
			addArg(fullKey, v)
		case bool:
			addArg(fullKey, fmt.Sprintf("%t", v))
		case int, uint64:
			addArg(fullKey, fmt.Sprintf("%d", v))
		case []any:
			for _, item := range v {
				if strItem, ok := item.(string); ok {
					addArg(fullKey, strItem)
				}
			}
		case map[string]any:
			processMap(fullKey, v, addArg)
		}
	}
}

// sanitizeForK8s ensures a string is compatible with Kubernetes naming conventions by converting
// to lowercase, replacing invalid characters, and trimming to a maximum length of 63 characters.
func sanitizeForK8s(input string) string {
	sanitized := strings.ToLower(input)
	sanitized = regexp.MustCompile(`[_]+`).ReplaceAllString(sanitized, "-")
	sanitized = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(sanitized, "-")
	sanitized = regexp.MustCompile(`-+`).ReplaceAllString(sanitized, "-")
	sanitized = strings.Trim(sanitized, "-")
	if len(sanitized) > 63 {
		sanitized = sanitized[:63]
	}
	return sanitized
}
