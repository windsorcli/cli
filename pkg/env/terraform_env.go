package env

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/windsorcli/cli/pkg/di"
)

// TerraformEnvPrinter is a struct that simulates a Terraform environment for testing purposes.
type TerraformEnvPrinter struct {
	BaseEnvPrinter
}

// NewTerraformEnvPrinter initializes a new TerraformEnvPrinter instance using the provided dependency injector.
func NewTerraformEnvPrinter(injector di.Injector) *TerraformEnvPrinter {
	return &TerraformEnvPrinter{
		BaseEnvPrinter: BaseEnvPrinter{
			injector: injector,
		},
	}
}

// GetEnvVars retrieves the environment variables for the Terraform environment.
func (e *TerraformEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	// Get the configuration root directory
	configRoot, err := e.contextHandler.GetConfigRoot()
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

	// Set TF_VAR_os_type based on the operating system
	if goos() == "windows" {
		envVars["TF_VAR_os_type"] = "windows"
	} else {
		envVars["TF_VAR_os_type"] = "unix"
	}

	return envVars, nil
}

// PostEnvHook executes any required operations after setting the environment variables.
func (e *TerraformEnvPrinter) PostEnvHook() error {
	return e.generateBackendOverrideTf()
}

// Print prints the environment variables for the Terraform environment.
func (e *TerraformEnvPrinter) Print() error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		// Return the error if GetEnvVars fails
		return fmt.Errorf("error getting environment variables: %w", err)
	}
	// Call the Print method of the embedded BaseEnvPrinter struct with the retrieved environment variables
	return e.BaseEnvPrinter.Print(envVars)
}

// Ensure TerraformEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*TerraformEnvPrinter)(nil)

func (e *TerraformEnvPrinter) getAlias() (map[string]string, error) {
	enableLocalstack := e.configHandler.GetBool("aws.localstack.create", false)

	// Check if Localstack is enabled
	if enableLocalstack {
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

	// Normalize the path for consistent behavior across different OS
	currentPath = filepath.Clean(currentPath)

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
	// Iterate through the path components to find the "terraform" or ".tf_modules" directory
	for i := len(pathParts) - 1; i >= 0; i-- {
		if strings.EqualFold(pathParts[i], "terraform") || strings.EqualFold(pathParts[i], ".tf_modules") { // Use case-insensitive comparison for Windows
			// Join the path components after the "terraform" or ".tf_modules" directory
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
func (e *TerraformEnvPrinter) generateBackendOverrideTf() error {
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
	configRoot, err := e.contextHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	// Get the current backend
	contextConfig := e.configHandler.GetConfig()

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
