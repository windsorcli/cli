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

// Implement the interface methods...
func (h *TerraformHelper) FindTerraformProjectPath() (string, error) {
	// Get the current working directory
	currentPath, err := getwd()
	if err != nil {
		return "", fmt.Errorf("error getting current directory: %w", err)
	}

	// Split the current path into its components
	pathParts := strings.Split(currentPath, string(os.PathSeparator))
	// Iterate through the path components to find the "terraform" directory
	for i, part := range pathParts {
		if part == "terraform" {
			// Join the path components from the "terraform" directory onwards
			terraformPath := filepath.Join(pathParts[i:]...)
			// Check if the directory contains any Terraform files
			if matches, _ := filepath.Glob(filepath.Join(terraformPath, "*.tf")); len(matches) > 0 {
				return terraformPath, nil
			}
			break
		}
	}

	return "", nil
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
	projectPath, err := h.FindTerraformProjectPath()
	if err != nil || projectPath == "" {
		return "", err
	}

	// Define patterns for tfvars files
	patterns := []string{
		fmt.Sprintf("%s.tfvars", projectPath),
		fmt.Sprintf("%s.json", projectPath),
		fmt.Sprintf("%s_generated.tfvars", projectPath),
		fmt.Sprintf("%s_generated.json", projectPath),
		fmt.Sprintf("%s_generated.tfvars.json", projectPath),
	}

	var varFileArgs []string
	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return "", err
	}

	// Check for the existence of each tfvars file and add it to the arguments
	for _, pattern := range patterns {
		filePath := filepath.Join(configRoot, pattern)
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
		return "", err
	}

	// Get the current backend configuration
	backend, err := h.GetCurrentBackend()
	if err != nil {
		return "", err
	}

	var backendConfigs []string
	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return "", err
	}

	// Check for the existence of the backend configuration file
	backendConfigFile := filepath.Join(configRoot, "backend.tfvars")
	if _, err := os.Stat(backendConfigFile); err == nil {
		backendConfigs = append(backendConfigs, backendConfigFile)
	}

	// Generate backend configuration based on the backend type
	switch backend {
	case "local":
		baseDir := filepath.Join(configRoot, ".tfstate")
		statePath := filepath.Join(baseDir, projectPath, "terraform.tfstate")
		backendConfigs = append(backendConfigs, fmt.Sprintf("path=%s", statePath))
	case "s3":
		backendConfigs = append(backendConfigs, fmt.Sprintf("key=%s/terraform.tfstate", projectPath))
	case "kubernetes":
		projectNameSanitized := h.SanitizeForK8s(projectPath)
		backendConfigs = append(backendConfigs, fmt.Sprintf("secret_suffix=%s", projectNameSanitized))
	}

	// Return the backend configuration flags
	if len(backendConfigs) > 0 {
		return fmt.Sprintf("-backend=true %s", strings.Join(backendConfigs, " -backend-config=")), nil
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
	// Find the Terraform project path
	projectPath, err := h.FindTerraformProjectPath()
	if err != nil || projectPath == "" {
		return map[string]string{
			"TF_DATA_DIR":         "",
			"TF_CLI_ARGS_init":    "",
			"TF_CLI_ARGS_plan":    "",
			"TF_CLI_ARGS_apply":   "",
			"TF_CLI_ARGS_import":  "",
			"TF_CLI_ARGS_destroy": "",
			"TF_VAR_context_path": "",
		}, nil
	}

	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return nil, err
	}

	// Define the data directory for Terraform
	dataDir := filepath.Join(configRoot, ".terraform", projectPath)
	// Generate the tfvars flags
	varFlagString, err := h.GenerateTerraformTfvarsFlags()
	if err != nil {
		return nil, err
	}
	// Generate the backend initialization flags
	backendFlagString, err := h.GenerateTerraformInitBackendFlags()
	if err != nil {
		return nil, err
	}

	// Generate backend_override.tf
	if err := h.GenerateBackendOverrideTf(); err != nil {
		return nil, err
	}

	// Return the environment variables for Terraform
	return map[string]string{
		"TF_DATA_DIR":         dataDir,
		"TF_CLI_ARGS_init":    backendFlagString,
		"TF_CLI_ARGS_plan":    fmt.Sprintf("-out=%s %s", filepath.Join(dataDir, "terraform.tfplan"), varFlagString),
		"TF_CLI_ARGS_apply":   filepath.Join(dataDir, "terraform.tfplan"),
		"TF_CLI_ARGS_import":  varFlagString,
		"TF_CLI_ARGS_destroy": varFlagString,
		"TF_VAR_context_path": configRoot,
	}, nil
}

// WriteFile writes content to a file at the specified path
func (h *TerraformHelper) WriteFile(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// GenerateBackendOverrideTf generates the backend_override.tf file for the Terraform project
func (h *TerraformHelper) GenerateBackendOverrideTf() error {
	// Find the Terraform project path
	projectPath, err := h.FindTerraformProjectPath()
	if err != nil || projectPath == "" {
		return nil
	}

	// Get the current backend configuration
	backend, err := h.GetCurrentBackend()
	if err != nil || backend == "" {
		return nil
	}

	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return err
	}

	// Get the current working directory
	workingDir, err := getwd()
	if err != nil {
		return err
	}

	// Define the path for the backend override file
	backendConfigPath := filepath.Join(workingDir, "backend_override.tf")
	var backendConfigContent string

	// Generate the content for the backend override file based on the backend type
	if backend == "local" {
		backendConfigContent = fmt.Sprintf(
			`terraform {
  backend "local" {
    path = "%s/terraform.tfstate"
  }
}`, configRoot)
	} else {
		backendConfigContent = fmt.Sprintf(
			`terraform {
  backend "%s" {}
}`, backend)
	}

	// Write the backend override file
	return h.WriteFile(backendConfigPath, backendConfigContent)
}
