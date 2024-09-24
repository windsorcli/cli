package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/shell"
)

// Define a variable for os.Getwd() for easier testing
var getwd = os.Getwd

// Define a variable for filepath.Glob for easier testing
var glob = filepath.Glob

// Wrapper function for os.WriteFile
var writeFile = os.WriteFile

// TerraformHelperInterface defines the methods for TerraformHelper
type TerraformHelperInterface interface {
	FindTerraformProjectPath() (string, error)
	GetCurrentBackend() (string, error)
	GenerateTerraformTfvarsFlags() (string, error)
	GenerateTerraformInitBackendFlags() (string, error)
	GenerateBackendOverrideTf() error
	GetEnvVars() (map[string]string, error)
}

// TerraformHelper is a struct that provides various utility functions for working with Terraform
type TerraformHelper struct {
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Context       context.ContextInterface
}

// NewTerraformHelper is a constructor for TerraformHelper
func NewTerraformHelper(configHandler config.ConfigHandler, shell shell.Shell, ctx context.ContextInterface) *TerraformHelper {
	return &TerraformHelper{
		ConfigHandler: configHandler,
		Shell:         shell,
		Context:       ctx,
	}
}

// FindTerraformProjectPath finds the path to the Terraform project
func (h *TerraformHelper) FindTerraformProjectPath() (string, error) {
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
		return "", fmt.Errorf("no Terraform files found in the current directory")
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

	return "", fmt.Errorf("no 'terraform' directory found in the current path")
}

// GetCurrentBackend retrieves the current backend configuration for Terraform
func (h *TerraformHelper) GetCurrentBackend() (string, error) {
	// Get the current context
	context, err := h.ConfigHandler.GetConfigValue("context")
	if err != nil {
		return "", fmt.Errorf("error retrieving context: %w", err)
	}

	// Get the configuration for the current context
	config, err := h.ConfigHandler.GetNestedMap(fmt.Sprintf("contexts.%s", context))
	if err != nil {
		return "", fmt.Errorf("error retrieving config for context: %w", err)
	}

	// Extract the backend configuration
	backend, ok := config["backend"].(string)
	if !ok {
		return "", nil
	}

	return backend, nil
}

// SanitizeForK8s sanitizes a string to be compatible with Kubernetes naming conventions
func (h *TerraformHelper) SanitizeForK8s(input string) string {
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

// GenerateTerraformTfvarsFlags generates the flags for Terraform tfvars files
func (h *TerraformHelper) GenerateTerraformTfvarsFlags() (string, error) {
	// Find the Terraform project path
	relativePath, err := h.FindTerraformProjectPath()
	if err != nil || relativePath == "" {
		return "", err
	}

	// Define patterns for tfvars files
	patterns := []string{
		fmt.Sprintf("%s.tfvars", relativePath),
		fmt.Sprintf("%s.json", relativePath),
		fmt.Sprintf("%s_generated.tfvars", relativePath),
		fmt.Sprintf("%s_generated.json", relativePath),
		fmt.Sprintf("%s_generated.tfvars.json", relativePath),
	}

	var varFileArgs []string
	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return "", err
	}

	// Debug print to check the config root and patterns
	fmt.Printf("Config Root: %s\n", configRoot)
	fmt.Printf("Patterns: %v\n", patterns)

	// Check for the existence of each tfvars file and add it to the arguments
	for _, pattern := range patterns {
		filePath := filepath.Join(configRoot, pattern)
		fmt.Printf("Checking file: %s\n", filePath)
		if _, err := os.Stat(filePath); err == nil {
			varFileArgs = append(varFileArgs, fmt.Sprintf("-var-file=%s", filePath))
		}
	}

	return strings.Join(varFileArgs, " "), nil
}

// GenerateTerraformInitBackendFlags generates the flags for initializing the Terraform backend
func (h *TerraformHelper) GenerateTerraformInitBackendFlags() (string, error) {
	// Find the Terraform project path
	projectPath, err := h.FindTerraformProjectPath()
	if err != nil || projectPath == "" {
		fmt.Printf("Error finding project path: %v\n", err)
		return "", err
	}

	// Get the current backend configuration
	backend, err := h.GetCurrentBackend()
	if err != nil || backend == "" {
		fmt.Printf("Error getting backend: %v\n", err)
		return "", fmt.Errorf("backend not found")
	}

	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		fmt.Printf("Error getting config root: %v\n", err)
		return "", fmt.Errorf("error getting config root: %w", err)
	}

	// Define the backend configuration file
	backendConfigFile := filepath.Join(configRoot, "backend.tfvars")
	backendConfigs := []string{}

	// Check if the backend configuration file exists
	_, err = os.Stat(backendConfigFile)
	if err == nil {
		backendConfigs = append(backendConfigs, fmt.Sprintf("-backend-config=%s", backendConfigFile))
	}

	// Generate the backend configuration based on the backend type
	if backend == "local" {
		baseDir := filepath.Join(configRoot, ".tfstate")
		statePath := filepath.Join(baseDir, projectPath, "terraform.tfstate")
		backendConfigs = append(backendConfigs, fmt.Sprintf("-backend-config=path=%s", statePath))
	} else if backend == "s3" {
		backendConfigs = append(backendConfigs, fmt.Sprintf("-backend-config=key=%s/terraform.tfstate", projectPath))
		projectNameSanitized := h.SanitizeForK8s(projectPath)
		backendConfigs = append(backendConfigs, fmt.Sprintf("-backend-config=secret_suffix=%s", projectNameSanitized))
	}

	// Generate the backend flags
	if len(backendConfigs) > 0 {
		backendFlags := "-backend=true " + strings.Join(backendConfigs, " ")
		return backendFlags, nil
	}

	return "", nil
}

// GetAlias retrieves the alias for the Terraform command based on the current context
func (h *TerraformHelper) GetAlias() (map[string]string, error) {
	// Get the current context
	context, err := h.ConfigHandler.GetConfigValue("context")
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	// Return the alias based on the context
	if context == "local" {
		return map[string]string{"terraform": "tflocal"}, nil
	}

	return map[string]string{"terraform": ""}, nil
}

// GetEnvVars retrieves the environment variables for the Terraform project
func (h *TerraformHelper) GetEnvVars() (map[string]string, error) {
	envVars := map[string]string{
		"TF_DATA_DIR":         "",
		"TF_CLI_ARGS_init":    "",
		"TF_CLI_ARGS_plan":    "",
		"TF_CLI_ARGS_apply":   "",
		"TF_CLI_ARGS_import":  "",
		"TF_CLI_ARGS_destroy": "",
		"TF_VAR_context_path": "",
	}

	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		fmt.Printf("Error getting config root: %v\n", err)
		return envVars, fmt.Errorf("error getting config root: %w", err)
	}
	fmt.Printf("Config Root: %s\n", configRoot)

	// Get the current working directory
	currentPath, err := getwd()
	if err != nil {
		fmt.Printf("Error getting current directory: %v\n", err)
		return envVars, fmt.Errorf("error getting current directory: %w", err)
	}
	fmt.Printf("Current working directory: %s\n", currentPath)

	// Find the Terraform project path
	projectPath, err := h.FindTerraformProjectPath()
	if err != nil {
		fmt.Printf("Error finding project path: %v\n", err)
		return envVars, fmt.Errorf("error finding project path: %w", err)
	}

	// Set the TF_DATA_DIR environment variable
	tfDataDir := filepath.Join(configRoot, ".terraform", projectPath)
	envVars["TF_DATA_DIR"] = tfDataDir

	// Set the TF_CLI_ARGS_init environment variable
	tfStatePath := filepath.Join(configRoot, ".tfstate", projectPath, "terraform.tfstate")
	envVars["TF_CLI_ARGS_init"] = fmt.Sprintf("-backend=true -backend-config=path=%s", tfStatePath)

	// Set the TF_CLI_ARGS_plan environment variable
	tfPlanPath := filepath.Join(configRoot, ".terraform", projectPath, "terraform.tfplan")
	envVars["TF_CLI_ARGS_plan"] = fmt.Sprintf("-out=%s -var-file=%s.tfvars -var-file=%s.json -var-file=%s_generated.tfvars -var-file=%s_generated.json -var-file=%s_generated.tfvars.json",
		tfPlanPath, projectPath, projectPath, projectPath, projectPath, projectPath)

	// Set the TF_CLI_ARGS_apply environment variable
	envVars["TF_CLI_ARGS_apply"] = tfPlanPath

	// Set the TF_CLI_ARGS_import environment variable
	envVars["TF_CLI_ARGS_import"] = fmt.Sprintf("-var-file=%s.tfvars -var-file=%s.json -var-file=%s_generated.tfvars -var-file=%s_generated.json -var-file=%s_generated.tfvars.json",
		projectPath, projectPath, projectPath, projectPath, projectPath)

	// Set the TF_CLI_ARGS_destroy environment variable
	envVars["TF_CLI_ARGS_destroy"] = fmt.Sprintf("-var-file=%s.tfvars -var-file=%s.json -var-file=%s_generated.tfvars -var-file=%s_generated.json -var-file=%s_generated.tfvars.json",
		projectPath, projectPath, projectPath, projectPath, projectPath)

	// Set the TF_VAR_context_path environment variable
	envVars["TF_VAR_context_path"] = configRoot

	return envVars, nil
}

// GenerateBackendOverrideTf generates the backend_override.tf file for the Terraform project
func (h *TerraformHelper) GenerateBackendOverrideTf() error {
	// Get the current backend
	backend, err := h.GetCurrentBackend()
	if err != nil {
		return fmt.Errorf("error getting backend: %w", err)
	}

	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	// Get the current working directory
	currentPath, err := getwd()
	if err != nil {
		return fmt.Errorf("error getting current directory: %w", err)
	}

	// Find the Terraform project path
	projectPath, err := h.FindTerraformProjectPath()
	if err != nil {
		return fmt.Errorf("error finding project path: %w", err)
	}

	// Create the backend_override.tf file
	backendOverridePath := filepath.Join(currentPath, "backend_override.tf")
	backendConfig := fmt.Sprintf(`
terraform {
  backend "%s" {
    path = "%s"
  }
}`, backend, filepath.Join(configRoot, ".tfstate", projectPath, "terraform.tfstate"))

	err = writeFile(backendOverridePath, []byte(backendConfig), os.ModePerm)
	if err != nil {
		return fmt.Errorf("error writing backend_override.tf: %w", err)
	}

	return nil
}

// FindTerraformProjectRoot finds the root directory containing the "terraform" directory
func (h *TerraformHelper) FindTerraformProjectRoot() (string, error) {
	// Get the current working directory
	currentPath, err := getwd()
	if err != nil {
		return "", fmt.Errorf("error getting current directory: %w", err)
	}

	// Debug print to check the current working directory
	fmt.Printf("Current working directory: %s\n", currentPath)

	// Split the current path into its components
	pathParts := strings.Split(currentPath, string(os.PathSeparator))
	// Debug print to check the path components
	fmt.Printf("Path components: %v\n", pathParts)

	// Iterate through the path components to find the "terraform" directory
	for i := len(pathParts) - 1; i >= 0; i-- {
		if pathParts[i] == "terraform" {
			// Join the path components up to the "terraform" directory
			projectRoot := filepath.Join(pathParts[:i+1]...)
			fmt.Printf("Found 'terraform' directory, project root: %s\n", projectRoot)
			return projectRoot, nil
		}
	}

	fmt.Printf("No 'terraform' directory found in the current path: %s\n", currentPath)
	return "", fmt.Errorf("no 'terraform' directory found in the current path")
}
