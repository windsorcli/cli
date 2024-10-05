package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

// TerraformHelper is a struct that provides various utility functions for working with Terraform
type TerraformHelper struct {
	ConfigHandler config.ConfigHandler
	Context       context.ContextInterface
}

// NewTerraformHelper is a constructor for TerraformHelper
func NewTerraformHelper(container *di.DIContainer) (*TerraformHelper, error) {
	resolvedConfigHandler, err := container.Resolve("cliConfigHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving cliConfigHandler: %w", err)
	}

	cliConfigHandler, ok := resolvedConfigHandler.(config.ConfigHandler)
	if !ok {
		return nil, fmt.Errorf("resolved cliConfigHandler is not of type ConfigHandler")
	}

	resolvedContext, err := container.Resolve("context")
	if err != nil {
		return nil, fmt.Errorf("error resolving context: %w", err)
	}

	contextInterface, ok := resolvedContext.(context.ContextInterface)
	if !ok {
		return nil, fmt.Errorf("resolved context is not of type ContextInterface")
	}

	return &TerraformHelper{
		ConfigHandler: cliConfigHandler,
		Context:       contextInterface,
	}, nil
}

// getAlias retrieves the alias for the Terraform command based on the current context
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

// GetEnvVars retrieves the environment variables for the Terraform command
func (h *TerraformHelper) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	// Find the Terraform project path
	projectPath, err := findRelativeTerraformProjectPath()
	if err != nil {
		// Return empty environment variables if there's a legitimate error
		return envVars, err
	}

	// If projectPath is empty, return empty environment variables
	if projectPath == "" {
		return envVars, nil
	}

	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error getting config root: %w", err)
	}

	// Define patterns for tfvars files based on the relative path
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

// PostEnvExec runs any necessary commands after the environment variables have been set.
func (h *TerraformHelper) PostEnvExec() error {
	return generateBackendOverrideTf(h)
}

// SetConfig sets the configuration value for the given key
func (h *TerraformHelper) SetConfig(key, value string) error {
	if key == "backend" {
		if value == "" {
			return nil
		}
		context, err := h.Context.GetContext()
		if err != nil {
			return fmt.Errorf("error retrieving context: %w", err)
		}
		if err = h.ConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.terraform.backend", context), value); err != nil {
			return fmt.Errorf("error setting backend: %w", err)
		}
		return nil
	}
	return fmt.Errorf("unsupported config key: %s", key)
}

// GetContainerConfig returns a list of container data for docker-compose.
func (h *TerraformHelper) GetContainerConfig() ([]map[string]interface{}, error) {
	// Stub implementation
	return nil, nil
}

// Ensure TerraformHelper implements Helper interface
var _ Helper = (*TerraformHelper)(nil)

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

	// No "terraform" directory found, return an error
	return "", fmt.Errorf("no 'terraform' directory found in the current path")
}

// getCurrentBackend retrieves the current backend configuration for Terraform
func getCurrentBackend(h *TerraformHelper) (string, error) {
	// Get the current context
	context, err := h.ConfigHandler.GetConfigValue("context")
	if err != nil {
		return "local", fmt.Errorf("error retrieving context, defaulting to 'local': %w", err)
	}

	// Get the configuration for the current context
	backend, err := h.ConfigHandler.GetConfigValue(fmt.Sprintf("contexts.%s.terraform.backend", context))
	if err != nil {
		return "local", fmt.Errorf("error retrieving config for context, defaulting to 'local': %w", err)
	}

	return backend, nil
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
func generateBackendOverrideTf(h *TerraformHelper) error {
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

	// Get the current backend
	backend, err := getCurrentBackend(h)
	if err != nil {
		return fmt.Errorf("error getting backend: %w", err)
	}

	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	// Create the backend_override.tf file
	backendOverridePath := filepath.Join(currentPath, "backend_override.tf")
	var backendConfig string

	switch backend {
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
		return fmt.Errorf("unsupported backend: %s", backend)
	}

	err = writeFile(backendOverridePath, []byte(backendConfig), os.ModePerm)
	if err != nil {
		return fmt.Errorf("error writing backend_override.tf: %w", err)
	}

	return nil
}
