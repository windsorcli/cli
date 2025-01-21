package env

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

	envVars["TF_DATA_DIR"] = strings.TrimSpace(tfDataDir)
	envVars["TF_CLI_ARGS_init"] = strings.TrimSpace("-backend=true")
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

// Print outputs the environment variables for the Terraform environment.
func (e *TerraformEnvPrinter) Print() error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		return fmt.Errorf("error getting environment variables: %w", err)
	}
	return e.BaseEnvPrinter.Print(envVars)
}

// Ensure TerraformEnvPrinter implements the EnvPrinter interface.
var _ EnvPrinter = (*TerraformEnvPrinter)(nil)

// getAlias returns command aliases based on Localstack configuration.
func (e *TerraformEnvPrinter) getAlias() (map[string]string, error) {
	enableLocalstack := e.configHandler.GetBool("aws.localstack.create", false)

	if enableLocalstack {
		return map[string]string{"terraform": "tflocal"}, nil
	}

	return map[string]string{"terraform": ""}, nil
}

// findRelativeTerraformProjectPath locates the Terraform project path by checking the current
// directory and its ancestors for Terraform files, returning the relative path if found.
func findRelativeTerraformProjectPath() (string, error) {
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

	configRoot, err := e.configHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	contextConfig := e.configHandler.GetConfig()
	backend := contextConfig.Terraform.Backend

	backendOverridePath := filepath.Join(currentPath, "backend_override.tf")
	var backendConfig string

	switch *backend {
	case "local":
		backendConfig = fmt.Sprintf(`
terraform {
  backend "local" {
    path = "%s"
  }
}`, filepath.ToSlash(filepath.Join(configRoot, ".tfstate", projectPath, "terraform.tfstate")))
	case "s3":
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
