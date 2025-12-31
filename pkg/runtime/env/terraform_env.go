// The TerraformEnvPrinter is a specialized component that manages Terraform environment configuration.
// It provides Terraform-specific environment variable management and configuration,
// The TerraformEnvPrinter handles backend configuration, variable files, and state management,
// ensuring proper Terraform CLI integration and environment setup for infrastructure operations.

package env

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/terraform"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Types
// =============================================================================

// TerraformArgs contains all the arguments needed for terraform operations
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
	TerraformVars   map[string]string
	BackendConfig   string
}

// TerraformEnvPrinter is a struct that implements Terraform environment configuration
type TerraformEnvPrinter struct {
	BaseEnvPrinter
	toolsManager      tools.ToolsManager
	terraformProvider terraform.TerraformProvider
}

// =============================================================================
// Constructor
// =============================================================================

// NewTerraformEnvPrinter creates a new TerraformEnvPrinter instance
func NewTerraformEnvPrinter(shell shell.Shell, configHandler config.ConfigHandler, toolsManager tools.ToolsManager) *TerraformEnvPrinter {
	return &TerraformEnvPrinter{
		BaseEnvPrinter:    *NewBaseEnvPrinter(shell, configHandler),
		toolsManager:      toolsManager,
		terraformProvider: terraform.NewTerraformProvider(configHandler, shell, toolsManager),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetEnvVars returns a map of environment variables for Terraform operations.
// If not in a Terraform project directory, it unsets managed TF_ variables present in the environment.
// Otherwise, it generates Terraform arguments for the current project.
// Returns the environment variable map or an error if resolution fails.
func (e *TerraformEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	projectPath, err := e.terraformProvider.FindRelativeProjectPath()
	if err != nil {
		return nil, fmt.Errorf("error finding project path: %w", err)
	}

	if projectPath == "" {
		managedVars := []string{
			"TF_DATA_DIR",
			"TF_CLI_ARGS_init",
			"TF_CLI_ARGS_plan",
			"TF_CLI_ARGS_apply",
			"TF_CLI_ARGS_import",
			"TF_CLI_ARGS_destroy",
			"TF_VAR_context_path",
			"TF_VAR_context_id",
			"TF_VAR_os_type",
		}

		for _, varName := range managedVars {
			if _, exists := e.shims.LookupEnv(varName); exists {
				envVars[varName] = ""
			}
		}

		return envVars, nil
	}

	terraformArgs, err := e.GenerateTerraformArgs(projectPath, projectPath, true)
	if err != nil {
		return nil, fmt.Errorf("error generating terraform args: %w", err)
	}

	return terraformArgs.TerraformVars, nil
}

// PostEnvHook executes operations after setting the environment variables.
func (e *TerraformEnvPrinter) PostEnvHook(directory ...string) error {
	var currentPath string
	if len(directory) > 0 {
		currentPath = filepath.Clean(directory[0])
	} else {
		var err error
		currentPath, err = e.shims.Getwd()
		if err != nil {
			return fmt.Errorf("error getting current directory: %w", err)
		}
	}
	projectPath, err := e.terraformProvider.FindRelativeProjectPath(directory...)
	if err != nil {
		return fmt.Errorf("error finding project path: %w", err)
	}
	if projectPath == "" {
		return nil
	}
	return e.terraformProvider.GenerateBackendOverride(currentPath)
}

// GenerateTerraformArgs constructs Terraform CLI arguments and environment variables for given project and module paths.
// Resolves config root, locates tfvars files, generates backend config args, and assembles all CLI/env values needed
// for Terraform operations. The interactive parameter controls whether -auto-approve is included in destroy args:
// when false, -auto-approve is included for non-interactive stack-based operations; when true, it is excluded for interactive regular injection.
// Returns a TerraformArgs struct or error.
func (e *TerraformEnvPrinter) GenerateTerraformArgs(projectPath, modulePath string, interactive bool) (*TerraformArgs, error) {
	configRoot, err := e.configHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error getting config root: %w", err)
	}

	windsorScratchPath, err := e.configHandler.GetWindsorScratchPath()
	if err != nil {
		return nil, fmt.Errorf("error getting windsor scratch path: %w", err)
	}

	component := e.terraformProvider.GetTerraformComponent(projectPath)
	componentID := projectPath
	if component != nil {
		componentID = component.GetID()
	}

	patterns := []string{
		filepath.Join(windsorScratchPath, "terraform", componentID, "terraform.tfvars"),
		filepath.Join(configRoot, "terraform", componentID+".tfvars"),
		filepath.Join(configRoot, "terraform", componentID+".tfvars.json"),
	}
	if componentID != projectPath {
		patterns = append(patterns,
			filepath.Join(configRoot, "terraform", projectPath+".tfvars"),
			filepath.Join(configRoot, "terraform", projectPath+".tfvars.json"),
		)
	}

	var varFileArgs []string
	var varFileArgsForEnv []string
	for _, pattern := range patterns {
		if _, err := e.shims.Stat(filepath.FromSlash(pattern)); err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("error checking file: %w", err)
			}
		} else {
			slashPath := filepath.ToSlash(pattern)
			varFileArgs = append(varFileArgs, fmt.Sprintf("-var-file=%s", slashPath))
			varFileArgsForEnv = append(varFileArgsForEnv, fmt.Sprintf("-var-file=\"%s\"", slashPath))
		}
	}

	tfDataDir := filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform", componentID))
	tfPlanPath := filepath.ToSlash(filepath.Join(tfDataDir, "terraform.tfplan"))

	backendConfigArgs, err := e.generateBackendConfigArgs(projectPath, configRoot, false)
	if err != nil {
		return nil, fmt.Errorf("error generating backend config args: %w", err)
	}

	backendConfigArgsForEnv, err := e.generateBackendConfigArgsForEnv(projectPath, configRoot)
	if err != nil {
		return nil, fmt.Errorf("error generating backend config args for env: %w", err)
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

	applyArgsForEnv := make([]string, len(applyArgs))
	for i, arg := range applyArgs {
		if arg == tfPlanPath {
			applyArgsForEnv[i] = fmt.Sprintf("\"%s\"", arg)
		} else {
			applyArgsForEnv[i] = arg
		}
	}

	destroyArgsForEnv := []string{}
	if !interactive {
		destroyArgsForEnv = append(destroyArgsForEnv, "-auto-approve")
	}
	destroyArgsForEnv = append(destroyArgsForEnv, varFileArgsForEnv...)
	if component != nil && component.Parallelism != nil {
		parallelismArg := fmt.Sprintf("-parallelism=%d", *component.Parallelism)
		destroyArgsForEnv = append(destroyArgsForEnv, parallelismArg)
	}

	terraformVars := make(map[string]string)
	terraformVars["TF_DATA_DIR"] = strings.TrimSpace(tfDataDir)
	terraformVars["TF_CLI_ARGS_init"] = strings.TrimSpace(fmt.Sprintf("-backend=true -force-copy -upgrade %s", strings.Join(backendConfigArgsForEnv, " ")))
	terraformVars["TF_CLI_ARGS_plan"] = strings.TrimSpace(fmt.Sprintf("-out=\"%s\" %s", tfPlanPath, strings.Join(varFileArgsForEnv, " ")))
	terraformVars["TF_CLI_ARGS_apply"] = strings.TrimSpace(strings.Join(applyArgsForEnv, " "))
	terraformVars["TF_CLI_ARGS_refresh"] = strings.TrimSpace(strings.Join(varFileArgsForEnv, " "))
	terraformVars["TF_CLI_ARGS_import"] = strings.TrimSpace(strings.Join(varFileArgsForEnv, " "))
	terraformVars["TF_CLI_ARGS_destroy"] = strings.TrimSpace(strings.Join(destroyArgsForEnv, " "))
	terraformVars["TF_VAR_context_path"] = strings.TrimSpace(filepath.ToSlash(configRoot))
	terraformVars["TF_VAR_context_id"] = strings.TrimSpace(e.configHandler.GetString("id", ""))

	if e.shims.Goos() == "windows" {
		terraformVars["TF_VAR_os_type"] = "windows"
	} else {
		terraformVars["TF_VAR_os_type"] = "unix"
	}

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
		TerraformVars:   terraformVars,
		BackendConfig:   strings.Join(backendConfigArgs, " "),
	}, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// restoreEnvVar restores an environment variable to its original value or unsets it if it was empty
func (e *TerraformEnvPrinter) restoreEnvVar(key, originalValue string) {
	if originalValue != "" {
		_ = os.Setenv(key, originalValue)
	} else {
		_ = os.Unsetenv(key)
	}
}

// generateBackendConfigArgs constructs backend config args for terraform commands.
// It reads the backend type from the config and adds relevant key-value pairs.
// The function supports local, s3, kubernetes, and azurerm backends.
// It also includes backend.tfvars if present in the context directory.
// The forEnvVar parameter controls whether the arguments are quoted for environment variables.
func (e *TerraformEnvPrinter) generateBackendConfigArgs(projectPath, configRoot string, forEnvVar bool) ([]string, error) {
	var backendConfigArgs []string
	backend := e.configHandler.GetString("terraform.backend.type", "local")

	addBackendConfigArg := func(key, value string) {
		if value != "" {
			if forEnvVar {
				backendConfigArgs = append(backendConfigArgs, fmt.Sprintf("-backend-config=\"%s=%s\"", key, filepath.ToSlash(value)))
			} else {
				backendConfigArgs = append(backendConfigArgs, fmt.Sprintf("-backend-config=%s=%s", key, filepath.ToSlash(value)))
			}
		}
	}

	if context := e.configHandler.GetContext(); context != "" {
		backendTfvarsPath := filepath.Join(configRoot, "terraform", "backend.tfvars")
		if _, err := e.shims.Stat(backendTfvarsPath); err == nil {
			if forEnvVar {
				backendConfigArgs = append(backendConfigArgs, fmt.Sprintf("-backend-config=\"%s\"", filepath.ToSlash(backendTfvarsPath)))
			} else {
				backendConfigArgs = append(backendConfigArgs, fmt.Sprintf("-backend-config=%s", filepath.ToSlash(backendTfvarsPath)))
			}
		}
	}

	prefix := e.configHandler.GetString("terraform.backend.prefix", "")

	switch backend {
	case "local":
		windsorScratchPath, err := e.configHandler.GetWindsorScratchPath()
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
		if backend := e.configHandler.GetConfig().Terraform.Backend.S3; backend != nil {
			if err := e.processBackendConfig(backend, addBackendConfigArg); err != nil {
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
		if backend := e.configHandler.GetConfig().Terraform.Backend.Kubernetes; backend != nil {
			if err := e.processBackendConfig(backend, addBackendConfigArg); err != nil {
				return nil, fmt.Errorf("error processing Kubernetes backend config: %w", err)
			}
		}
	case "azurerm":
		keyPath := fmt.Sprintf("%s%s", prefix, filepath.ToSlash(filepath.Join(projectPath, "terraform.tfstate")))
		addBackendConfigArg("key", keyPath)
		if backend := e.configHandler.GetConfig().Terraform.Backend.AzureRM; backend != nil {
			if err := e.processBackendConfig(backend, addBackendConfigArg); err != nil {
				return nil, fmt.Errorf("error processing AzureRM backend config: %w", err)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported backend: %s", backend)
	}

	return backendConfigArgs, nil
}

// generateBackendConfigArgsForEnv constructs backend config args for terraform environment variables.
// This is a convenience wrapper around generateBackendConfigArgs with forEnvVar=true.
func (e *TerraformEnvPrinter) generateBackendConfigArgsForEnv(projectPath, configRoot string) ([]string, error) {
	return e.generateBackendConfigArgs(projectPath, configRoot, true)
}

// processBackendConfig processes the backend config and adds the key-value pairs to the backend config args.
func (e *TerraformEnvPrinter) processBackendConfig(backendConfig any, addArg func(key, value string)) error {
	yamlData, err := e.shims.YamlMarshal(backendConfig)
	if err != nil {
		return fmt.Errorf("error marshalling backend to YAML: %w", err)
	}

	var configMap map[string]any
	if err := e.shims.YamlUnmarshal(yamlData, &configMap); err != nil {
		return fmt.Errorf("error unmarshalling backend YAML: %w", err)
	}

	var args []string
	processMap("", configMap, func(key, value string) {
		args = append(args, fmt.Sprintf("%s=%s", key, value))
	})

	sort.Strings(args)
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) == 2 {
			addArg(parts[0], parts[1])
		}
	}

	return nil
}

// processMap processes a map and adds the key-value pairs to the backend config args.
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
var sanitizeForK8s = func(input string) string {
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

// Ensure TerraformEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*TerraformEnvPrinter)(nil)
