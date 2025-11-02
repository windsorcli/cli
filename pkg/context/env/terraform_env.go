// The TerraformEnvPrinter is a specialized component that manages Terraform environment configuration.
// It provides Terraform-specific environment variable management and configuration,
// The TerraformEnvPrinter handles backend configuration, variable files, and state management,
// ensuring proper Terraform CLI integration and environment setup for infrastructure operations.

package envvars

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
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
}

// =============================================================================
// Constructor
// =============================================================================

// NewTerraformEnvPrinter creates a new TerraformEnvPrinter instance
func NewTerraformEnvPrinter(injector di.Injector) *TerraformEnvPrinter {
	return &TerraformEnvPrinter{
		BaseEnvPrinter: *NewBaseEnvPrinter(injector),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetEnvVars returns a map of environment variables for Terraform operations.
// If not in a Terraform project directory, it unsets managed TF_ variables present in the environment.
// Otherwise, it generates Terraform arguments and augments them with dependency variables.
// Returns the environment variable map or an error if resolution fails.
func (e *TerraformEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	projectPath, err := e.findRelativeTerraformProjectPath()
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

	terraformArgs, err := e.GenerateTerraformArgs(projectPath, projectPath)
	if err != nil {
		return nil, fmt.Errorf("error generating terraform args: %w", err)
	}

	if err := e.addDependencyVariables(projectPath, terraformArgs); err != nil {
		return nil, fmt.Errorf("error adding dependency variables: %w", err)
	}

	return terraformArgs.TerraformVars, nil
}

// PostEnvHook executes operations after setting the environment variables.
func (e *TerraformEnvPrinter) PostEnvHook(directory ...string) error {
	return e.generateBackendOverrideTf(directory...)
}

// GenerateTerraformArgs constructs Terraform CLI arguments and environment variables for given project and module paths.
// Resolves config root, locates tfvars files, generates backend config args, and assembles all CLI/env values needed
// for Terraform operations. Returns a TerraformArgs struct or error.
func (e *TerraformEnvPrinter) GenerateTerraformArgs(projectPath, modulePath string) (*TerraformArgs, error) {
	configRoot, err := e.configHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error getting config root: %w", err)
	}

	patterns := []string{
		filepath.Join(configRoot, "terraform", projectPath+".tfvars"),
		filepath.Join(configRoot, "terraform", projectPath+".tfvars.json"),
		filepath.Join(configRoot, "terraform", projectPath+"_generated.tfvars"),
		filepath.Join(configRoot, "terraform", projectPath+"_generated.tfvars.json"),
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

	tfDataDir := filepath.ToSlash(filepath.Join(configRoot, ".terraform", projectPath))
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

	destroyArgs := []string{"-auto-approve"}
	destroyArgs = append(destroyArgs, varFileArgs...)

	var parallelismArg string
	component := e.getTerraformComponent(projectPath)
	if component != nil && component.Parallelism != nil {
		parallelismArg = fmt.Sprintf(" -parallelism=%d", *component.Parallelism)
		applyArgs = append(applyArgs, fmt.Sprintf("-parallelism=%d", *component.Parallelism))
		destroyArgs = append(destroyArgs, fmt.Sprintf("-parallelism=%d", *component.Parallelism))
	}

	applyArgs = append(applyArgs, tfPlanPath)

	terraformVars := make(map[string]string)
	terraformVars["TF_DATA_DIR"] = strings.TrimSpace(tfDataDir)
	terraformVars["TF_CLI_ARGS_init"] = strings.TrimSpace(fmt.Sprintf("-backend=true -force-copy -upgrade %s", strings.Join(backendConfigArgsForEnv, " ")))
	terraformVars["TF_CLI_ARGS_plan"] = strings.TrimSpace(fmt.Sprintf("-out=\"%s\" %s", tfPlanPath, strings.Join(varFileArgsForEnv, " ")))
	terraformVars["TF_CLI_ARGS_apply"] = strings.TrimSpace(fmt.Sprintf("\"%s\"%s", tfPlanPath, parallelismArg))
	terraformVars["TF_CLI_ARGS_refresh"] = strings.TrimSpace(strings.Join(varFileArgsForEnv, " "))
	terraformVars["TF_CLI_ARGS_import"] = strings.TrimSpace(strings.Join(varFileArgsForEnv, " "))
	terraformVars["TF_CLI_ARGS_destroy"] = strings.TrimSpace(fmt.Sprintf("%s%s", strings.Join(varFileArgsForEnv, " "), parallelismArg))
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

// addDependencyVariables sets dependency outputs as TF_VAR_* environment variables for the specified projectPath.
// It locates the current component, resolves dependency order, captures outputs from dependencies, and injects them
// into terraformArgs.TerraformVars using the format TF_VAR_<outputKey>. Non-string outputs are stringified.
// If the component has no dependencies, the function is a no-op. Errors are returned for
// dependency resolution failures; missing outputs are tolerated.
func (e *TerraformEnvPrinter) addDependencyVariables(projectPath string, terraformArgs *TerraformArgs) error {
	currentComponent := e.getTerraformComponent(projectPath)
	if currentComponent == nil || len(currentComponent.DependsOn) == 0 {
		return nil
	}

	componentsInterface := e.getTerraformComponents()
	components, ok := componentsInterface.([]blueprintv1alpha1.TerraformComponent)
	if !ok || len(components) == 0 {
		return nil
	}

	sortedComponents, err := e.resolveTerraformComponentDependencies(components)
	if err != nil {
		return fmt.Errorf("error resolving terraform component dependencies: %w", err)
	}

	componentOutputs := make(map[string]map[string]any)

	for _, component := range sortedComponents {
		if component.Path == projectPath {
			break
		}
		outputs, err := e.captureTerraformOutputs(component.FullPath)
		if err != nil {
			continue
		}
		componentOutputs[component.Path] = outputs
	}

	for _, depPath := range currentComponent.DependsOn {
		if outputs, exists := componentOutputs[depPath]; exists {
			for outputKey, outputValue := range outputs {
				var valueStr string
				switch v := outputValue.(type) {
				case string:
					valueStr = v
				case float64:
					valueStr = fmt.Sprintf("%.0f", v)
				case bool:
					valueStr = fmt.Sprintf("%t", v)
				default:
					valueStr = fmt.Sprintf("%v", v)
				}
				varName := fmt.Sprintf("TF_VAR_%s", outputKey)
				terraformArgs.TerraformVars[varName] = valueStr
			}
		}
	}

	return nil
}

// resolveTerraformComponentDependencies returns a topologically sorted slice of Terraform components,
// ensuring that all dependencies are ordered before their dependents. It detects and reports missing
// or circular dependencies. The function uses component.Path as the key.
// Returns an error if a dependency is missing or a cycle is detected.
func (e *TerraformEnvPrinter) resolveTerraformComponentDependencies(components []blueprintv1alpha1.TerraformComponent) ([]blueprintv1alpha1.TerraformComponent, error) {
	pathToComponent := make(map[string]blueprintv1alpha1.TerraformComponent)
	pathToIndex := make(map[string]int)

	for i, component := range components {
		pathToComponent[component.Path] = component
		pathToIndex[component.Path] = i
	}

	graph := make(map[string][]string)
	inDegree := make(map[string]int)

	for path := range pathToComponent {
		graph[path] = []string{}
		inDegree[path] = 0
	}

	for path, component := range pathToComponent {
		for _, depPath := range component.DependsOn {
			if _, exists := pathToComponent[depPath]; !exists {
				return nil, fmt.Errorf("terraform component %q depends on %q which does not exist", path, depPath)
			}
			graph[depPath] = append(graph[depPath], path)
			inDegree[path]++
		}
	}

	var queue []string
	var sorted []string

	for path, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, path)
		}
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		sorted = append(sorted, current)

		for _, neighbor := range graph[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(sorted) != len(components) {
		return nil, fmt.Errorf("circular dependency detected in terraform components")
	}

	var sortedComponents []blueprintv1alpha1.TerraformComponent
	for _, path := range sorted {
		sortedComponents = append(sortedComponents, pathToComponent[path])
	}

	return sortedComponents, nil
}

// captureTerraformOutputs executes terraform output with proper environment setup for the specified component.
// It locates the component path, generates terraform arguments with backend configuration, sets environment variables,
// creates backend_override.tf, runs terraform output, and performs cleanup. Returns an empty map for any error to avoid blocking the env pipeline.
func (e *TerraformEnvPrinter) captureTerraformOutputs(modulePath string) (map[string]any, error) {
	var componentPath string
	componentsInterface := e.getTerraformComponents()
	components, ok := componentsInterface.([]blueprintv1alpha1.TerraformComponent)
	if !ok {
		return make(map[string]any), nil
	}
	for _, component := range components {
		if component.FullPath == modulePath {
			componentPath = component.Path
			break
		}
	}

	if componentPath == "" {
		return make(map[string]any), nil
	}

	terraformArgs, err := e.GenerateTerraformArgs(componentPath, modulePath)
	if err != nil {
		return make(map[string]any), nil
	}

	originalTFDataDir := e.shims.Getenv("TF_DATA_DIR")

	if err := e.setEnvVar("TF_DATA_DIR", terraformArgs.TFDataDir); err != nil {
		return make(map[string]any), nil
	}

	if err := e.generateBackendOverrideTf(modulePath); err != nil {
		e.restoreEnvVar("TF_DATA_DIR", originalTFDataDir)
		return make(map[string]any), nil
	}

	cleanup := func() {
		backendOverridePath := filepath.Join(modulePath, "backend_override.tf")
		if _, err := e.shims.Stat(backendOverridePath); err == nil {
			_ = e.shims.Remove(backendOverridePath)
		}
		e.restoreEnvVar("TF_DATA_DIR", originalTFDataDir)
	}
	defer cleanup()

	outputArgs := []string{fmt.Sprintf("-chdir=%s", modulePath), "output", "-json"}
	output, err := e.shell.ExecSilent("terraform", outputArgs...)
	if err != nil {
		return make(map[string]any), nil
	}

	if strings.TrimSpace(output) == "" || strings.TrimSpace(output) == "{}" {
		return make(map[string]any), nil
	}

	var outputs map[string]any
	if err := e.shims.JsonUnmarshal([]byte(output), &outputs); err != nil {
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

// setEnvVar sets an environment variable using os.Setenv as a fallback since shims doesn't have Setenv
func (e *TerraformEnvPrinter) setEnvVar(key, value string) error {
	return os.Setenv(key, value)
}

// restoreEnvVar restores an environment variable to its original value or unsets it if it was empty
func (e *TerraformEnvPrinter) restoreEnvVar(key, originalValue string) {
	if originalValue != "" {
		_ = os.Setenv(key, originalValue)
	} else {
		_ = os.Unsetenv(key)
	}
}

// generateBackendOverrideTf creates the backend_override.tf file for the project by determining
// the backend type and writing the appropriate configuration to the file.
func (e *TerraformEnvPrinter) generateBackendOverrideTf(directory ...string) error {
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

	projectPath, err := e.findRelativeTerraformProjectPath(directory...)
	if err != nil {
		return fmt.Errorf("error finding project path: %w", err)
	}

	if projectPath == "" {
		return nil
	}

	backend := e.configHandler.GetString("terraform.backend.type", "local")

	var backendConfig string
	switch backend {
	case "none":
		backendOverridePath := filepath.Join(currentPath, "backend_override.tf")
		if _, err := e.shims.Stat(backendOverridePath); err == nil {
			if err := e.shims.Remove(backendOverridePath); err != nil {
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

	backendOverridePath := filepath.Join(currentPath, "backend_override.tf")
	err = e.shims.WriteFile(backendOverridePath, []byte(backendConfig), os.ModePerm)
	if err != nil {
		return fmt.Errorf("error writing backend_override.tf: %w", err)
	}

	return nil
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
		path := filepath.Join(configRoot, ".tfstate")
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

// findRelativeTerraformProjectPath locates the Terraform project path by checking the current
// directory (or provided directory) and its ancestors for Terraform files, returning the relative path if found.
func (e *TerraformEnvPrinter) findRelativeTerraformProjectPath(directory ...string) (string, error) {
	var currentPath string
	if len(directory) > 0 {
		currentPath = filepath.Clean(directory[0])
	} else {
		var err error
		currentPath, err = e.shims.Getwd()
		if err != nil {
			return "", fmt.Errorf("error getting current directory: %w", err)
		}
	}

	globPattern := filepath.Join(currentPath, "*.tf")
	matches, err := e.shims.Glob(globPattern)
	if err != nil {
		return "", fmt.Errorf("error finding project path: %w", err)
	}
	if len(matches) == 0 {
		return "", nil
	}

	pathParts := strings.Split(currentPath, string(os.PathSeparator))
	for i := len(pathParts) - 1; i >= 0; i-- {
		if strings.EqualFold(pathParts[i], "terraform") || strings.EqualFold(pathParts[i], ".tf_modules") {
			relativePath := filepath.Join(pathParts[i+1:]...)
			return filepath.ToSlash(relativePath), nil
		}
	}

	return "", nil
}

// getTerraformComponent finds a Terraform component by path.
// Returns nil if not found.
func (e *TerraformEnvPrinter) getTerraformComponent(projectPath string) *blueprintv1alpha1.TerraformComponent {
	componentsInterface := e.getTerraformComponents()
	components, ok := componentsInterface.([]blueprintv1alpha1.TerraformComponent)
	if !ok {
		return nil
	}
	for _, component := range components {
		if component.Path == projectPath {
			return &component
		}
	}
	return nil
}

// getTerraformComponents loads and parses Terraform components from a blueprint.yaml file.
// If projectPath is provided and not empty, it returns a pointer to the matching TerraformComponent or nil if not found.
// If projectPath is not provided, it returns a slice of all TerraformComponent structs from blueprint.yaml.
// For each component, the FullPath field is set to the resolved absolute path for sourced components, or the relative path for local components.
func (e *TerraformEnvPrinter) getTerraformComponents(projectPath ...string) interface{} {
	configRoot, err := e.configHandler.GetConfigRoot()
	if err != nil {
		if len(projectPath) > 0 {
			return nil
		}
		return []blueprintv1alpha1.TerraformComponent{}
	}

	blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
	data, err := e.shims.ReadFile(blueprintPath)
	if err != nil {
		if len(projectPath) > 0 {
			return nil
		}
		return []blueprintv1alpha1.TerraformComponent{}
	}

	var blueprint blueprintv1alpha1.Blueprint
	if err := yaml.Unmarshal(data, &blueprint); err != nil {
		if len(projectPath) > 0 {
			return nil
		}
		return []blueprintv1alpha1.TerraformComponent{}
	}

	for i := range blueprint.TerraformComponents {
		component := &blueprint.TerraformComponents[i]
		if component.Source != "" {
			component.FullPath = filepath.Join(configRoot, "terraform", component.Path)
		} else {
			component.FullPath = component.Path
		}
	}

	if len(projectPath) > 0 {
		for i := range blueprint.TerraformComponents {
			if blueprint.TerraformComponents[i].Path == projectPath[0] {
				return &blueprint.TerraformComponents[i]
			}
		}
		return nil
	}

	return blueprint.TerraformComponents
}

// Ensure TerraformEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*TerraformEnvPrinter)(nil)
