// The TerraformProvider provides all terraform-specific operations including output capture,
// component resolution, and terraform command execution. It handles session-based caching
// to avoid repeated operations.

package terraform

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
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
	mu            sync.RWMutex
}

// terraformContext provides a scoped environment for Terraform operations with automatic cleanup.
type terraformContext struct {
	ComponentID         string
	AbsModulePath       string
	TerraformArgs       *TerraformArgs
	BackendOverridePath string
	originalEnvVars     map[string]string
	provider            *terraformProvider
}

// TerraformArgs contains all the CLI arguments needed for terraform operations.
// It does not include environment variable formatting - that is handled by the env printer.
type TerraformArgs struct {
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
	IsInTerraformProject() bool
	GenerateBackendOverride(directory string) error
	GenerateTerraformArgs(componentID, modulePath string, interactive bool) (*TerraformArgs, error)
	GetTerraformComponent(componentID string) *blueprintv1alpha1.TerraformComponent
	GetTerraformComponents() []blueprintv1alpha1.TerraformComponent
	SetTerraformComponents(components []blueprintv1alpha1.TerraformComponent)
	GetTerraformOutputs(componentID string) (map[string]any, error)
	GetTFDataDir(componentID string) (string, error)
	GetEnvVars(componentID string, interactive bool) (map[string]string, *TerraformArgs, error)
	FormatArgsForEnv(args []string) string
	ClearCache()
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

// GenerateBackendOverride creates or removes the backend_override.tf file for the specified directory
// based on the configured backend type. This file is used to override Terraform backend configuration
// at runtime without modifying the original Terraform files. If the backend type is 'none', it removes
// the override file if it exists. Otherwise, it writes a backend_override.tf file with the appropriate
// backend stanza for local, s3, kubernetes, or azurerm backends. Returns an error for unsupported backend types.
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

// GenerateTerraformArgs constructs Terraform CLI arguments for the specified component ID, module path, and interaction mode.
// This function discovers applicable var files, configures backend arguments, and assembles all common Terraform command
// arguments for init, plan, apply, destroy, import, and refresh operations. The componentID is used for tfstate paths,
// var file lookups, and backend configuration. The modulePath parameter is unused and maintained for backward compatibility.
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

// GetTerraformComponents loads and parses Terraform components from blueprint.yaml.
// It retrieves the blueprint configuration root, reads the blueprint.yaml file, and computes the FullPath
// for each Terraform component based on the presence of a Source field and the resolved project root.
// Returns all TerraformComponent structs from blueprint.yaml with FullPath fields set.
// Components are cached after first load to avoid repeated file I/O operations.
// If components have been set via SetTerraformComponents, those are returned instead.
// If components loaded from file have no inputs, it attempts to lazy-load the blueprint to get components with inputs.
func (p *terraformProvider) GetTerraformComponents() []blueprintv1alpha1.TerraformComponent {
	p.mu.RLock()
	if p.components != nil {
		componentsWithInputs := 0
		for _, comp := range p.components {
			if len(comp.Inputs) > 0 {
				componentsWithInputs++
			}
		}
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

// GetTFDataDir calculates the TF_DATA_DIR path for a given component ID.
// It resolves the component ID to the actual component ID (using GetID if component exists),
// then constructs the path as ${windsorScratchPath}/.terraform/${componentID}.
// This path is used by Terraform to store its working directory data and state.
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

// GetEnvVars constructs the environment variables required for Terraform execution for the specified component ID.
// It generates TerraformArgs internally and sets up base environment variables including TF_DATA_DIR,
// TF_CLI_ARGS_*, and TF_VAR_context_* variables. Component inputs are evaluated to populate the cache via
// terraform_output() calls, and resulting TF_VAR_* environment variables are populated from the evaluated
// inputs of the current component only. Outputs from other components are used solely to evaluate inputs and
// are not included as separate TF_VAR_* variables. Complex output values are JSON-encoded.
// Returns the generated environment variables map, the TerraformArgs struct, or an error if processing fails.
func (p *terraformProvider) GetEnvVars(componentID string, interactive bool) (map[string]string, *TerraformArgs, error) {
	component := p.GetTerraformComponent(componentID)
	var modulePath string
	if component != nil {
		var err error
		modulePath, err = p.resolveModulePath(component)
		if err != nil {
			return nil, nil, fmt.Errorf("error resolving module path for component %s: %w", componentID, err)
		}
	} else {
		components := p.GetTerraformComponents()
		for i := range components {
			if components[i].GetID() == componentID {
				var err error
				modulePath, err = p.resolveModulePath(&components[i])
				if err != nil {
					return nil, nil, fmt.Errorf("error resolving module path for component %s: %w", componentID, err)
				}
				component = &components[i]
				break
			}
		}
		if modulePath == "" {
			componentID := componentID
			projectRoot, err := p.shell.GetProjectRoot()
			if err == nil {
				modulePath = filepath.Join(projectRoot, "terraform", componentID)
			} else {
				return nil, nil, fmt.Errorf("component %s not found and module path could not be resolved: %w", componentID, err)
			}
		}
	}
	terraformArgs, err := p.GenerateTerraformArgs(componentID, modulePath, interactive)
	if err != nil {
		return nil, nil, fmt.Errorf("error generating terraform args: %w", err)
	}

	envVars, err := p.getBaseEnvVarsForComponent(terraformArgs)
	if err != nil {
		return nil, nil, err
	}

	if componentID != "" && p.evaluator != nil {
		component := p.GetTerraformComponent(componentID)
		if component != nil && component.Inputs != nil && len(component.Inputs) > 0 {
			for key, value := range component.Inputs {
				if evaluator.ContainsExpression(value) {
					evaluated, err := p.evaluator.EvaluateMap(map[string]any{key: value}, "", true)
					if err != nil {
						return nil, nil, fmt.Errorf("error evaluating input '%s' for component %s: %w", key, componentID, err)
					}
					evaluatedValue, exists := evaluated[key]
					if !exists {
						continue
					}
					envKey := fmt.Sprintf("TF_VAR_%s", key)
					var envValue string
					if valueStr, ok := evaluatedValue.(string); ok {
						envValue = valueStr
					} else {
						valueBytes, err := p.Shims.JsonMarshal(evaluatedValue)
						if err != nil {
							continue
						}
						envValue = string(valueBytes)
					}
					envVars[envKey] = envValue
				}
			}
		}
	}

	return envVars, terraformArgs, nil
}

// FormatArgsForEnv formats CLI arguments for use in environment variables.
// It adds quotes around file paths and values that need them for shell compatibility, ensuring that
// paths with spaces or special characters are properly escaped. Handles Unix absolute paths (/path),
// relative paths (./path), and Windows drive letter paths (both D:\path and D:/path formats).
// This is necessary because Terraform CLI arguments passed via TF_CLI_ARGS_* environment variables
// must be properly quoted to work correctly across different shell environments.
func (p *terraformProvider) FormatArgsForEnv(args []string) string {
	formatted := make([]string, len(args))
	for i, arg := range args {
		if strings.HasPrefix(arg, "-var-file=") {
			value := strings.TrimPrefix(arg, "-var-file=")
			formatted[i] = fmt.Sprintf("-var-file=\"%s\"", value)
		} else if strings.HasPrefix(arg, "-out=") {
			value := strings.TrimPrefix(arg, "-out=")
			formatted[i] = fmt.Sprintf("-out=\"%s\"", value)
		} else if strings.HasPrefix(arg, "-backend-config=") {
			if strings.Contains(arg, "=") && !strings.HasPrefix(strings.SplitN(arg, "=", 2)[1], "\"") {
				parts := strings.SplitN(arg, "=", 2)
				formatted[i] = fmt.Sprintf("%s=\"%s\"", parts[0], parts[1])
			} else {
				formatted[i] = arg
			}
		} else if !strings.HasPrefix(arg, "-") && (strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, ".") || (len(arg) >= 2 && arg[1] == ':' && ((arg[0] >= 'A' && arg[0] <= 'Z') || (arg[0] >= 'a' && arg[0] <= 'z')))) {
			formatted[i] = fmt.Sprintf("\"%s\"", arg)
		} else {
			formatted[i] = arg
		}
	}
	return strings.Join(formatted, " ")
}

// GetTerraformOutputs retrieves Terraform outputs for the specified component by running 'terraform output -json'.
// It sets up the appropriate environment (TF_DATA_DIR, backend override file) before executing the command.
// If output fails initially, it attempts to initialize the module and retry, allowing outputs to be retrieved
// even if the module hasn't been initialized yet. The backend override and environment variables are cleaned up
// after execution. Returns a map of output values or an empty map on error. Only output 'value' fields are returned.
func (p *terraformProvider) GetTerraformOutputs(componentID string) (map[string]any, error) {
	return p.withTerraformContext(componentID, func(ctx *terraformContext) (map[string]any, error) {
		backendOverrideExists := false
		if _, err := ctx.provider.Shims.Stat(ctx.BackendOverridePath); err == nil {
			backendOverrideExists = true
		}

		terraformCommand := ctx.provider.toolsManager.GetTerraformCommand()
		if terraformCommand == "" {
			terraformCommand = "terraform"
		}
		outputArgs := []string{fmt.Sprintf("-chdir=%s", ctx.AbsModulePath), "output", "-json"}
		output, err := ctx.provider.shell.ExecSilent(terraformCommand, outputArgs...)
		if err != nil {
			chdirInitArgs := []string{fmt.Sprintf("-chdir=%s", ctx.AbsModulePath), "init"}
			if backendOverrideExists {
				chdirInitArgs = append(chdirInitArgs, "-reconfigure")
			}
			chdirInitArgs = append(chdirInitArgs, ctx.TerraformArgs.InitArgs...)
			_, initErr := ctx.provider.shell.ExecSilent(terraformCommand, chdirInitArgs...)
			if initErr != nil {
				return make(map[string]any), nil
			}
			output, err = ctx.provider.shell.ExecSilent(terraformCommand, outputArgs...)
			if err != nil {
				return make(map[string]any), nil
			}
		}

		if strings.TrimSpace(output) == "" || strings.TrimSpace(output) == "{}" {
			return make(map[string]any), nil
		}

		var outputs map[string]any
		if err := ctx.provider.Shims.JsonUnmarshal([]byte(output), &outputs); err != nil {
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
	})
}

// ClearCache clears the session cache for all components.
// This invalidates cached Terraform components and output values, forcing them to be reloaded
// from the blueprint file and re-fetched from Terraform state on the next access.
// This is useful when the blueprint has been modified or when outputs need to be refreshed.
func (p *terraformProvider) ClearCache() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = make(map[string]map[string]any)
	p.components = nil
}

// =============================================================================
// Private Methods
// =============================================================================

// registerTerraformOutputHelper registers the terraform_output helper function with the evaluator.
// This allows blueprint expressions to reference Terraform outputs from other components using
// the terraform_output(component, key) syntax. The helper validates that exactly two string arguments
// are provided (component ID and output key), then delegates to getOutput to retrieve the value.
func (p *terraformProvider) registerTerraformOutputHelper(evaluator evaluator.ExpressionEvaluator) {
	evaluator.Register("terraform_output", func(params []any, deferred bool) (any, error) {
		if len(params) != 2 {
			return nil, fmt.Errorf("terraform_output() requires exactly 2 arguments (component, key), got %d", len(params))
		}
		component, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("terraform_output() component must be a string, got %T", params[0])
		}
		key, ok := params[1].(string)
		if !ok {
			return nil, fmt.Errorf("terraform_output() key must be a string, got %T", params[1])
		}
		expr := fmt.Sprintf(`terraform_output("%s", "%s")`, component, key)
		value, err := p.getOutput(component, key, expr, deferred)
		if err != nil {
			return nil, err
		}
		return value, nil
	}, new(func(string, string) any))
}

// getOutput retrieves a single output value for a Terraform component by key.
// If outputs for the component are requested for the first time, all outputs are fetched from Terraform
// and cached for subsequent requests. Cached values are used for later accesses to avoid redundant retrievals.
// When deferred is false, this function returns a DeferredError to signal that the expression should be preserved.
// When deferred is true, it returns the actual output value if available, or the expression string if not found.
func (p *terraformProvider) getOutput(componentID, key string, expression string, deferred bool) (any, error) {
	if !deferred {
		return nil, &evaluator.DeferredError{
			Expression: expression,
			Message:    fmt.Sprintf("terraform output '%s' for component %s is deferred", key, componentID),
		}
	}

	p.mu.RLock()
	if cached, exists := p.cache[componentID]; exists {
		if value, exists := cached[key]; exists {
			p.mu.RUnlock()
			return value, nil
		}
	}
	p.mu.RUnlock()

	outputs, _ := p.GetTerraformOutputs(componentID)
	if len(outputs) == 0 {
		return expression, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if cached, exists := p.cache[componentID]; exists {
		if value, exists := cached[key]; exists {
			return value, nil
		}
	}

	if p.cache[componentID] == nil {
		p.cache[componentID] = make(map[string]any)
	}

	maps.Copy(p.cache[componentID], outputs)

	if value, exists := outputs[key]; exists {
		return value, nil
	}

	return expression, nil
}

// getBaseEnvVarsForComponent returns the base environment variables for a Terraform component
// without TF_VAR outputs from other components. This is the core set of env vars needed
// for any Terraform operation on the component, including TF_DATA_DIR, TF_CLI_ARGS_* for all
// Terraform commands, and TF_VAR_context_* variables that provide context information to Terraform.
// These variables ensure Terraform can locate its state, use the correct backend configuration,
// and have access to context-specific information during execution.
func (p *terraformProvider) getBaseEnvVarsForComponent(terraformArgs *TerraformArgs) (map[string]string, error) {
	configRoot, err := p.configHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error getting config root: %w", err)
	}

	envVars := make(map[string]string)
	envVars["TF_DATA_DIR"] = terraformArgs.TFDataDir
	envVars["TF_CLI_ARGS_init"] = p.FormatArgsForEnv(terraformArgs.InitArgs)
	envVars["TF_CLI_ARGS_plan"] = p.FormatArgsForEnv(terraformArgs.PlanArgs)
	envVars["TF_CLI_ARGS_apply"] = p.FormatArgsForEnv(terraformArgs.ApplyArgs)
	envVars["TF_CLI_ARGS_refresh"] = p.FormatArgsForEnv(terraformArgs.RefreshArgs)
	envVars["TF_CLI_ARGS_import"] = p.FormatArgsForEnv(terraformArgs.ImportArgs)
	envVars["TF_CLI_ARGS_destroy"] = p.FormatArgsForEnv(terraformArgs.DestroyArgs)
	envVars["TF_VAR_context_path"] = filepath.ToSlash(configRoot)
	envVars["TF_VAR_context_id"] = p.configHandler.GetString("id", "")

	if p.Shims.Goos() == "windows" {
		envVars["TF_VAR_os_type"] = "windows"
	} else {
		envVars["TF_VAR_os_type"] = "unix"
	}

	return envVars, nil
}

// getStatePath returns the path to the Terraform state file for the specified component ID.
// The state file is stored in .tfstate/<componentID>/terraform.tfstate within the Windsor scratch path.
// If a backend prefix is configured, it is included in the path to support multi-tenant or
// multi-environment deployments where state files need to be namespaced.
func (p *terraformProvider) getStatePath(componentID string) (string, error) {
	windsorScratchPath, err := p.configHandler.GetWindsorScratchPath()
	if err != nil {
		return "", fmt.Errorf("error getting windsor scratch path: %w", err)
	}

	component := p.GetTerraformComponent(componentID)
	actualComponentID := componentID
	if component != nil {
		actualComponentID = component.GetID()
	}

	prefix := p.configHandler.GetString("terraform.backend.prefix", "")
	path := filepath.Join(windsorScratchPath, ".tfstate")
	if prefix != "" {
		path = filepath.Join(path, prefix)
	}
	statePath := filepath.Join(path, actualComponentID, "terraform.tfstate")
	return statePath, nil
}

// prepareTerraformContext prepares a terraformContext for a component with all necessary setup.
// It resolves the component's module path, generates Terraform arguments, sets up environment variables,
// and creates the backend override file. This context is used to ensure consistent Terraform execution
// environment across different operations. Returns the context and original env vars for cleanup,
// allowing the caller to restore the environment after operations complete.
func (p *terraformProvider) prepareTerraformContext(componentID string) (*terraformContext, map[string]string, error) {
	component := p.GetTerraformComponent(componentID)
	if component == nil {
		return nil, nil, fmt.Errorf("component not found: %s", componentID)
	}

	absModulePath, err := p.resolveModulePath(component)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve module path: %w", err)
	}

	terraformArgs, err := p.GenerateTerraformArgs(componentID, absModulePath, false)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate terraform args: %w", err)
	}

	envVars, err := p.getBaseEnvVarsForComponent(terraformArgs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get base env vars: %w", err)
	}

	originalEnvVars, err := p.applyEnvVars(envVars)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply env vars: %w", err)
	}

	if err := p.GenerateBackendOverride(absModulePath); err != nil {
		p.restoreEnvVars(originalEnvVars)
		return nil, nil, fmt.Errorf("failed to generate backend override: %w", err)
	}

	backendOverridePath := filepath.Join(absModulePath, "backend_override.tf")

	ctx := &terraformContext{
		ComponentID:         componentID,
		AbsModulePath:       absModulePath,
		TerraformArgs:       terraformArgs,
		BackendOverridePath: backendOverridePath,
		originalEnvVars:     originalEnvVars,
		provider:            p,
	}

	return ctx, originalEnvVars, nil
}

// withTerraformContext sets up a Terraform environment context for a component, executes the provided function,
// and ensures cleanup. This helper ensures that environment variables are properly set, backend overrides
// are created, and all resources are cleaned up after the function completes, even if it returns an error.
// This pattern provides a safe way to execute Terraform operations in an isolated environment context.
// Returns the result of the function and any error encountered during setup or execution.
func (p *terraformProvider) withTerraformContext(componentID string, fn func(*terraformContext) (map[string]any, error)) (map[string]any, error) {
	ctx, originalEnvVars, err := p.prepareTerraformContext(componentID)
	if err != nil {
		return make(map[string]any), nil
	}

	cleanup := func() {
		if _, err := p.Shims.Stat(ctx.BackendOverridePath); err == nil {
			_ = p.Shims.Remove(ctx.BackendOverridePath)
		}
		p.restoreEnvVars(originalEnvVars)
	}
	defer cleanup()

	return fn(ctx)
}

// loadTerraformComponents loads and parses Terraform components from a blueprint.yaml file.
// It retrieves the blueprint configuration root, reads the blueprint.yaml file, unmarshals it into
// a Blueprint struct, and computes the FullPath for each Terraform component based on the presence
// of a Source field and the resolved project root. Components with a Source are located in Windsor scratch
// space, while local components are in the project root. Returns the list of TerraformComponents found with FullPath fields set.
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
		windsorScratchPath, err := p.configHandler.GetWindsorScratchPath()
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

// applyEnvVars applies environment variables and returns a map of original values for restoration.
// It stores the current value of each environment variable before setting the new value, allowing
// the caller to restore the original environment state. If setting any variable fails, it attempts
// to restore all previously set variables before returning the error.
func (p *terraformProvider) applyEnvVars(envVars map[string]string) (map[string]string, error) {
	originalEnvVars := make(map[string]string)
	for key, value := range envVars {
		originalValue := p.Shims.Getenv(key)
		originalEnvVars[key] = originalValue
		if err := p.Shims.Setenv(key, value); err != nil {
			p.restoreEnvVars(originalEnvVars)
			return nil, err
		}
	}
	return originalEnvVars, nil
}

// restoreEnvVar restores an environment variable to its original value or unsets it if it was empty.
// This is used to clean up environment variables that were temporarily set for Terraform operations,
// ensuring that the process environment is restored to its original state after operations complete.
func (p *terraformProvider) restoreEnvVar(key, originalValue string) {
	if originalValue != "" {
		_ = p.Shims.Setenv(key, originalValue)
	} else {
		_ = p.Shims.Unsetenv(key)
	}
}

// restoreEnvVars restores environment variables to their original values or unsets them if they were empty.
// This iterates through the provided map of original values and restores each environment variable,
// ensuring that all temporarily set environment variables are cleaned up after Terraform operations complete.
func (p *terraformProvider) restoreEnvVars(originalEnvVars map[string]string) {
	for key, originalValue := range originalEnvVars {
		p.restoreEnvVar(key, originalValue)
	}
}

// generateBackendConfigArgs constructs the -backend-config CLI arguments for Terraform based on project configuration.
// This method determines the backend type from the configuration and assembles key-value argument pairs for supported
// backend types (local, s3, kubernetes, azurerm). It also detects the presence of backend.tfvars in the context root
// or a terraform/ fallback subdirectory, and includes a -backend-config pointing to that file if found.
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
		statePath, err := p.getStatePath(projectPath)
		if err != nil {
			return nil, fmt.Errorf("error getting state path: %w", err)
		}
		addBackendConfigArg("path", filepath.ToSlash(statePath))
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

// processBackendConfig processes backend configuration and applies each key-value pair to the provided addArg function.
// It marshals the provided backendConfig to YAML, then unmarshals it into a map structure to normalize the format.
// It traverses the resulting map, applying each key-value pair to the addArg function. Nested configuration
// objects result in compound keys using dot notation (e.g., "s3.bucket" for a nested bucket field).
// Returns an error if marshalling, unmarshalling, or processing fails.
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
