package env

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// TerraformEnv is a struct that simulates a Terraform environment for testing purposes.
type TerraformEnv struct {
	Env
}

// TerraformDeps holds the resolved dependencies for TerraformEnv.
type TerraformDeps struct {
	ContextInterface context.ContextInterface
	Shell            shell.Shell
	ConfigHandler    config.ConfigHandler
}

// NewTerraformEnv initializes a new TerraformEnv instance using the provided dependency injection container.
func NewTerraformEnv(diContainer di.ContainerInterface) *TerraformEnv {
	return &TerraformEnv{
		Env: Env{
			diContainer: diContainer,
		},
	}
}

// GetEnvVars retrieves the environment variables for the Terraform environment.
func (e *TerraformEnv) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	// Resolve dependencies for context and shell operations
	deps, err := e.resolveDependencies()
	if err != nil {
		return nil, fmt.Errorf("error resolving dependencies: %w", err)
	}

	// Get the configuration root directory
	configRoot, err := deps.ContextInterface.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error getting config root: %w", err)
	}

	// Find the Terraform project path
	projectPath, err := findRelativeTerraformProjectPath()
	if err != nil {
		return nil, fmt.Errorf("error finding project path: %w", err)
	}

	// Return if we're not in a terraform project folder
	if projectPath == "" {
		return nil, nil
	}

	// Define patterns for tfvars files
	patterns := []string{
		filepath.Join(configRoot, "terraform", projectPath+".tfvars"),
		filepath.Join(configRoot, "terraform", projectPath+".tfvars.json"),
		filepath.Join(configRoot, "terraform", projectPath+"_generated.tfvars"),
		filepath.Join(configRoot, "terraform", projectPath+"_generated.tfvars.json"),
	}

	// Check for existing tfvars files
	var varFileArgs []string
	for _, pattern := range patterns {
		if _, err := stat(pattern); err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("error checking file: %w", err)
			}
		} else {
			varFileArgs = append(varFileArgs, fmt.Sprintf("-var-file=%s", pattern))
		}
	}

	// Set environment variables
	envVars["TF_DATA_DIR"] = strings.TrimSpace(filepath.Join(configRoot, ".terraform", projectPath))
	envVars["TF_CLI_ARGS_init"] = strings.TrimSpace(fmt.Sprintf("-backend=true -backend-config=path=%s", filepath.Join(configRoot, ".tfstate", projectPath, "terraform.tfstate")))
	envVars["TF_CLI_ARGS_plan"] = strings.TrimSpace(fmt.Sprintf("-out=%s %s", filepath.Join(configRoot, ".terraform", projectPath, "terraform.tfplan"), strings.Join(varFileArgs, " ")))
	envVars["TF_CLI_ARGS_apply"] = strings.TrimSpace(filepath.Join(configRoot, ".terraform", projectPath, "terraform.tfplan"))
	envVars["TF_CLI_ARGS_import"] = strings.TrimSpace(strings.Join(varFileArgs, " "))
	envVars["TF_CLI_ARGS_destroy"] = strings.TrimSpace(strings.Join(varFileArgs, " "))
	envVars["TF_VAR_context_path"] = strings.TrimSpace(configRoot)

	return envVars, nil
}

// PostEnvHook executes any required operations after setting the environment variables.
func (e *TerraformEnv) PostEnvHook() error {
	return e.generateBackendOverrideTf()
}

// Print prints the environment variables for the Terraform environment.
func (e *TerraformEnv) Print() error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		// Return the error if GetEnvVars fails
		return fmt.Errorf("error getting environment variables: %w", err)
	}
	// Call the Print method of the embedded Env struct with the retrieved environment variables
	return e.Env.Print(envVars)
}

// Ensure TerraformEnv implements the EnvPrinter interface
var _ EnvPrinter = (*TerraformEnv)(nil)

// resolveDependencies is a convenience function to resolve and cast multiple dependencies at once.
func (e *TerraformEnv) resolveDependencies() (*TerraformDeps, error) {
	contextHandler, err := e.diContainer.Resolve("contextHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving contextHandler: %w", err)
	}
	contextInterface, ok := contextHandler.(context.ContextInterface)
	if !ok {
		return nil, fmt.Errorf("contextHandler is not of type ContextInterface")
	}

	shellInstance, err := e.diContainer.Resolve("shell")
	if err != nil {
		return nil, fmt.Errorf("error resolving shell: %w", err)
	}
	shell, ok := shellInstance.(shell.Shell)
	if !ok {
		return nil, fmt.Errorf("shell is not of type Shell")
	}

	configHandler, err := e.diContainer.Resolve("cliConfigHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving cliConfigHandler: %w", err)
	}
	cliConfigHandler, ok := configHandler.(config.ConfigHandler)
	if !ok {
		return nil, fmt.Errorf("cliConfigHandler is not of type ConfigHandler")
	}

	return &TerraformDeps{
		ContextInterface: contextInterface,
		Shell:            shell,
		ConfigHandler:    cliConfigHandler,
	}, nil
}

func (e *TerraformEnv) getAlias() (map[string]string, error) {
	// Resolve necessary dependencies for context operations.
	deps, err := e.resolveDependencies()
	if err != nil {
		return nil, err
	}

	// Get the current context
	currentContext, err := deps.ContextInterface.GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	contextConfig, err := deps.ConfigHandler.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context config: %w", err)
	}

	// Check if Localstack is enabled
	if currentContext == "local" &&
		contextConfig.AWS != nil &&
		contextConfig.AWS.Localstack != nil &&
		contextConfig.AWS.Localstack.Create != nil &&
		*contextConfig.AWS.Localstack.Create {
		return map[string]string{"terraform": "tflocal"}, nil
	}

	return map[string]string{"terraform": ""}, nil
}

// findRelativeTerraformProjectPath finds the path to the Terraform project from the terraform directory
func findRelativeTerraformProjectPath() (string, error) {
	// Get the current working directory
	currentPath, err := getwd()
	if err != nil {
		return "", fmt.Errorf("error getting current directory: %w", err)
	}

	// Check if the current directory contains any Terraform files
	globPattern := filepath.Join(currentPath, "*.tf")
	matches, err := glob(globPattern)
	if err != nil {
		return "", fmt.Errorf("error finding project path: %w", err)
	}
	if len(matches) == 0 {
		// No Terraform files found, return an empty string without an error
		return "", nil
	}

	// Split the current path into its components
	pathParts := strings.Split(currentPath, string(os.PathSeparator))

	// Iterate through the path components to find the "terraform" directory
	for i := len(pathParts) - 1; i >= 0; i-- {
		if pathParts[i] == "terraform" {
			// Join the path components after the "terraform" directory
			relativePath := filepath.Join(pathParts[i+1:]...)
			return relativePath, nil
		}
	}

	// No "terraform" directory found, return an empty string without an error
	return "", nil
}

// sanitizeForK8s sanitizes a string to be compatible with Kubernetes naming conventions
func sanitizeForK8s(input string) string {
	// Convert the input string to lowercase
	sanitized := strings.ToLower(input)
	// Replace underscores with hyphens
	sanitized = regexp.MustCompile(`[_]+`).ReplaceAllString(sanitized, "-")
	// Remove any character that is not a lowercase letter, digit, or hyphen
	sanitized = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(sanitized, "-")
	// Replace multiple consecutive hyphens with a single hyphen
	sanitized = regexp.MustCompile(`-+`).ReplaceAllString(sanitized, "-")
	// Trim leading and trailing hyphens
	sanitized = strings.Trim(sanitized, "-")
	// Ensure the sanitized string does not exceed 63 characters
	if len(sanitized) > 63 {
		sanitized = sanitized[:63]
	}
	return sanitized
}

// generateBackendOverrideTf generates the backend_override.tf file for the Terraform project
func (h *TerraformEnv) generateBackendOverrideTf() error {
	deps, err := h.resolveDependencies()
	if err != nil {
		return err
	}

	// Get the current working directory
	currentPath, err := getwd()
	if err != nil {
		return fmt.Errorf("error getting current directory: %w", err)
	}

	// Find the Terraform project path
	projectPath, err := findRelativeTerraformProjectPath()
	if err != nil {
		return fmt.Errorf("error finding project path: %w", err)
	}

	// If projectPath is empty, do nothing
	if projectPath == "" {
		return nil
	}

	// Get the configuration root directory
	configRoot, err := deps.ContextInterface.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	// Get the current backend
	contextConfig, err := deps.ConfigHandler.GetConfig()
	if err != nil {
		return fmt.Errorf("error retrieving context: %w", err)
	}

	backend := contextConfig.Terraform.Backend

	// Create the backend_override.tf file
	backendOverridePath := filepath.Join(currentPath, "backend_override.tf")
	var backendConfig string

	switch *backend {
	case "local":
		backendConfig = fmt.Sprintf(`
terraform {
  backend "local" {
    path = "%s"
  }
}`, filepath.Join(configRoot, ".tfstate", projectPath, "terraform.tfstate"))
	case "s3":
		// Normalize the key to use Unix-style path separators
		key := filepath.ToSlash(filepath.Join(projectPath, "terraform.tfstate"))
		backendConfig = fmt.Sprintf(`
terraform {
  backend "s3" {
    key = "%s"
  }
}`, key)
	case "kubernetes":
		projectNameSanitized := sanitizeForK8s(projectPath)
		backendConfig = fmt.Sprintf(`
terraform {
  backend "kubernetes" {
    secret_suffix = "%s"
  }
}`, projectNameSanitized)
	default:
		return fmt.Errorf("unsupported backend: %s", *backend)
	}

	err = writeFile(backendOverridePath, []byte(backendConfig), os.ModePerm)
	if err != nil {
		return fmt.Errorf("error writing backend_override.tf: %w", err)
	}

	return nil
}
