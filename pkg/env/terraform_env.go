package env

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/windsorcli/cli/pkg/di"
)

// TerraformEnvPrinter simulates a Terraform environment for testing purposes.
type TerraformEnvPrinter struct {
	BaseEnvPrinter
}

// NewTerraformEnvPrinter initializes a new TerraformEnvPrinter instance.
func NewTerraformEnvPrinter(injector di.Injector) *TerraformEnvPrinter {
	return &TerraformEnvPrinter{
		BaseEnvPrinter: BaseEnvPrinter{
			injector: injector,
		},
	}
}

// GetEnvVars retrieves environment variables for Terraform by determining the config root and
// project path, checking for tfvars files, and setting variables based on the OS. It returns
// a map of environment variables or an error if any step fails.
func (e *TerraformEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	configRoot, err := e.configHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error getting config root: %w", err)
	}

	projectPath, err := findRelativeTerraformProjectPath()
	if err != nil {
		return nil, fmt.Errorf("error finding project path: %w", err)
	}

	if projectPath == "" {
		return nil, nil
	}

	patterns := []string{
		filepath.Join(configRoot, "terraform", projectPath+".tfvars"),
		filepath.Join(configRoot, "terraform", projectPath+".tfvars.json"),
		filepath.Join(configRoot, "terraform", projectPath+"_generated.tfvars"),
		filepath.Join(configRoot, "terraform", projectPath+"_generated.tfvars.json"),
	}

	var varFileArgs []string
	for _, pattern := range patterns {
		if _, err := stat(pattern); err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("error checking file: %w", err)
			}
		} else {
			varFileArgs = append(varFileArgs, fmt.Sprintf("-var-file=\"%s\"", filepath.ToSlash(pattern)))
		}
	}

	tfDataDir := filepath.ToSlash(filepath.Join(configRoot, ".terraform", projectPath))
	tfPlanPath := filepath.ToSlash(filepath.Join(tfDataDir, "terraform.tfplan"))

	backendConfigArgs, err := e.generateBackendConfigArgs(projectPath, configRoot)
	if err != nil {
		return nil, fmt.Errorf("error generating backend config args: %w", err)
	}

	initArgs := []string{
		"-backend=true",
		strings.Join(backendConfigArgs, " "),
	}

	envVars["TF_DATA_DIR"] = strings.TrimSpace(tfDataDir)
	envVars["TF_CLI_ARGS_init"] = strings.TrimSpace(strings.Join(initArgs, " "))
	envVars["TF_CLI_ARGS_plan"] = strings.TrimSpace(fmt.Sprintf("-out=\"%s\" %s", tfPlanPath, strings.Join(varFileArgs, " ")))
	envVars["TF_CLI_ARGS_apply"] = strings.TrimSpace(fmt.Sprintf("\"%s\"", tfPlanPath))
	envVars["TF_CLI_ARGS_import"] = strings.TrimSpace(strings.Join(varFileArgs, " "))
	envVars["TF_CLI_ARGS_destroy"] = strings.TrimSpace(strings.Join(varFileArgs, " "))
	envVars["TF_VAR_context_path"] = strings.TrimSpace(filepath.ToSlash(configRoot))

	if goos() == "windows" {
		envVars["TF_VAR_os_type"] = "windows"
	} else {
		envVars["TF_VAR_os_type"] = "unix"
	}

	return envVars, nil
}

// PostEnvHook executes operations after setting the environment variables.
func (e *TerraformEnvPrinter) PostEnvHook() error {
	return e.generateBackendOverrideTf()
}

// Print retrieves and prints the environment variables for the Docker environment.
func (e *TerraformEnvPrinter) Print(customVars ...map[string]string) error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		return fmt.Errorf("error getting environment variables: %w", err)
	}

	// If customVars are provided, merge them with envVars
	if len(customVars) > 0 {
		for key, value := range customVars[0] {
			envVars[key] = strings.TrimSpace(value)
		}
	}

	return e.BaseEnvPrinter.Print(envVars)
}

// generateBackendOverrideTf creates the backend_override.tf file for the project by determining
// the backend type and writing the appropriate configuration to the file.
func (e *TerraformEnvPrinter) generateBackendOverrideTf() error {
	currentPath, err := getwd()
	if err != nil {
		return fmt.Errorf("error getting current directory: %w", err)
	}

	projectPath, err := findRelativeTerraformProjectPath()
	if err != nil {
		return fmt.Errorf("error finding project path: %w", err)
	}

	if projectPath == "" {
		return nil
	}

	backend := e.configHandler.GetString("terraform.backend.type", "local")

	backendOverridePath := filepath.Join(currentPath, "backend_override.tf")
	var backendConfig string

	switch backend {
	case "local":
		backendConfig = fmt.Sprintf(`terraform {
  backend "local" {}
}`)
	case "s3":
		backendConfig = fmt.Sprintf(`terraform {
  backend "s3" {}
}`)
	case "kubernetes":
		backendConfig = fmt.Sprintf(`terraform {
  backend "kubernetes" {}
}`)
	default:
		return fmt.Errorf("unsupported backend: %s", backend)
	}

	err = writeFile(backendOverridePath, []byte(backendConfig), os.ModePerm)
	if err != nil {
		return fmt.Errorf("error writing backend_override.tf: %w", err)
	}

	return nil
}

// generateBackendConfigArgs constructs backend config args for terraform init.
// It reads the backend type from the config and adds relevant key-value pairs.
// The function supports local, s3, and kubernetes backends.
// It also includes backend.tfvars if present in the context directory.
func (e *TerraformEnvPrinter) generateBackendConfigArgs(projectPath, configRoot string) ([]string, error) {
	var backendConfigArgs []string
	backend := e.configHandler.GetString("terraform.backend.type", "local")

	addBackendConfigArg := func(key, value string) {
		if value != "" {
			backendConfigArgs = append(backendConfigArgs, fmt.Sprintf("-backend-config=\"%s=%s\"", key, filepath.ToSlash(value)))
		}
	}

	if context := e.configHandler.GetContext(); context != "" {
		backendTfvarsPath := filepath.Join(configRoot, "terraform", "backend.tfvars")
		if _, err := stat(backendTfvarsPath); err == nil {
			backendConfigArgs = append(backendConfigArgs, fmt.Sprintf("-backend-config=\"%s\"", filepath.ToSlash(backendTfvarsPath)))
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
			if err := processBackendConfig(backend, addBackendConfigArg); err != nil {
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
			if err := processBackendConfig(backend, addBackendConfigArg); err != nil {
				return nil, fmt.Errorf("error processing Kubernetes backend config: %w", err)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported backend: %s", backend)
	}

	return backendConfigArgs, nil
}

// Ensure TerraformEnvPrinter implements the EnvPrinter interface.
var _ EnvPrinter = (*TerraformEnvPrinter)(nil)

// processBackendConfig processes the backend config and adds the key-value pairs to the backend config args.
var processBackendConfig = func(backendConfig any, addArg func(key, value string)) error {
	yamlData, err := yamlMarshal(backendConfig)
	if err != nil {
		return fmt.Errorf("error marshalling backend to YAML: %w", err)
	}

	var configMap map[string]any
	if err := yamlUnmarshal(yamlData, &configMap); err != nil {
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
func processMap(prefix string, configMap map[string]interface{}, addArg func(key, value string)) {
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
		case []interface{}:
			for _, item := range v {
				if strItem, ok := item.(string); ok {
					addArg(fullKey, strItem)
				}
			}
		case map[string]interface{}:
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
// directory and its ancestors for Terraform files, returning the relative path if found.
var findRelativeTerraformProjectPath = func() (string, error) {
	currentPath, err := getwd()
	if err != nil {
		return "", fmt.Errorf("error getting current directory: %w", err)
	}

	currentPath = filepath.Clean(currentPath)

	globPattern := filepath.Join(currentPath, "*.tf")
	matches, err := glob(globPattern)
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
			return relativePath, nil
		}
	}

	return "", nil
}
