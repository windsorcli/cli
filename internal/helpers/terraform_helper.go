package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/shell"
)

type TerraformHelper struct {
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
}

func NewTerraformHelper(configHandler config.ConfigHandler, shell shell.Shell) *TerraformHelper {
	return &TerraformHelper{
		ConfigHandler: configHandler,
		Shell:         shell,
	}
}

func (h *TerraformHelper) GetCurrentBackend() (string, error) {
	context, err := h.ConfigHandler.GetConfigValue("context")
	if err != nil {
		return "", fmt.Errorf("error retrieving context: %w", err)
	}

	config, err := h.ConfigHandler.GetNestedMap(fmt.Sprintf("contexts.%s", context))
	if err != nil {
		return "", fmt.Errorf("error retrieving config for context: %w", err)
	}

	backend, ok := config["backend"].(string)
	if !ok {
		return "", nil
	}

	return backend, nil
}

func (h *TerraformHelper) SanitizeForK8s(input string) string {
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

func (h *TerraformHelper) FindTerraformProjectPath() (string, error) {
	currentPath, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("error getting current directory: %w", err)
	}

	pathParts := strings.Split(currentPath, string(os.PathSeparator))
	for i, part := range pathParts {
		if part == "terraform" {
			terraformPath := filepath.Join(pathParts[i:]...)
			if matches, _ := filepath.Glob(filepath.Join(terraformPath, "*.tf")); len(matches) > 0 {
				return terraformPath, nil
			}
			break
		}
	}

	return "", nil
}

func (h *TerraformHelper) GenerateTerraformTfvarsFlags() (string, error) {
	projectPath, err := h.FindTerraformProjectPath()
	if err != nil || projectPath == "" {
		return "", err
	}

	patterns := []string{
		fmt.Sprintf("%s.tfvars", projectPath),
		fmt.Sprintf("%s.json", projectPath),
		fmt.Sprintf("%s_generated.tfvars", projectPath),
		fmt.Sprintf("%s_generated.json", projectPath),
		fmt.Sprintf("%s_generated.tfvars.json", projectPath),
	}

	var varFileArgs []string
	for _, pattern := range patterns {
		filePath := filepath.Join(h.ConfigHandler.GetConfigDir(), pattern)
		if _, err := os.Stat(filePath); err == nil {
			varFileArgs = append(varFileArgs, fmt.Sprintf("-var-file=%s", filePath))
		}
	}

	return strings.Join(varFileArgs, " "), nil
}

func (h *TerraformHelper) GenerateTerraformInitBackendFlags() (string, error) {
	projectPath, err := h.FindTerraformProjectPath()
	if err != nil || projectPath == "" {
		return "", err
	}

	backend, err := h.GetCurrentBackend()
	if err != nil {
		return "", err
	}

	var backendConfigs []string
	backendConfigFile := filepath.Join(h.ConfigHandler.GetConfigDir(), "backend.tfvars")
	if _, err := os.Stat(backendConfigFile); err == nil {
		backendConfigs = append(backendConfigs, backendConfigFile)
	}

	switch backend {
	case "local":
		baseDir := filepath.Join(h.ConfigHandler.GetConfigDir(), ".tfstate")
		statePath := filepath.Join(baseDir, projectPath, "terraform.tfstate")
		backendConfigs = append(backendConfigs, fmt.Sprintf("path=%s", statePath))
	case "s3":
		backendConfigs = append(backendConfigs, fmt.Sprintf("key=%s/terraform.tfstate", projectPath))
	case "kubernetes":
		projectNameSanitized := h.SanitizeForK8s(projectPath)
		backendConfigs = append(backendConfigs, fmt.Sprintf("secret_suffix=%s", projectNameSanitized))
	}

	if len(backendConfigs) > 0 {
		return fmt.Sprintf("-backend=true %s", strings.Join(backendConfigs, " -backend-config=")), nil
	}

	return "", nil
}

func (h *TerraformHelper) GetAlias() (map[string]string, error) {
	context, err := h.ConfigHandler.GetConfigValue("context")
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	if context == "local" {
		return map[string]string{"terraform": "tflocal"}, nil
	}

	return map[string]string{"terraform": ""}, nil
}

func (h *TerraformHelper) GetEnvVars() (map[string]string, error) {
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

	dataDir := filepath.Join(h.ConfigHandler.GetConfigDir(), ".terraform", projectPath)
	varFlagString, err := h.GenerateTerraformTfvarsFlags()
	if err != nil {
		return nil, err
	}
	backendFlagString, err := h.GenerateTerraformInitBackendFlags()
	if err != nil {
		return nil, err
	}

	// Generate backend_override.tf
	if err := h.GenerateBackendOverrideTf(); err != nil {
		return nil, err
	}

	return map[string]string{
		"TF_DATA_DIR":         dataDir,
		"TF_CLI_ARGS_init":    backendFlagString,
		"TF_CLI_ARGS_plan":    fmt.Sprintf("-out=%s %s", filepath.Join(dataDir, "terraform.tfplan"), varFlagString),
		"TF_CLI_ARGS_apply":   filepath.Join(dataDir, "terraform.tfplan"),
		"TF_CLI_ARGS_import":  varFlagString,
		"TF_CLI_ARGS_destroy": varFlagString,
		"TF_VAR_context_path": h.ConfigHandler.GetConfigDir(),
	}, nil
}

func (h *TerraformHelper) WriteFile(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

func (h *TerraformHelper) GenerateBackendOverrideTf() error {
	projectPath, err := h.FindTerraformProjectPath()
	if err != nil || projectPath == "" {
		return nil
	}

	backend, err := h.GetCurrentBackend()
	if err != nil || backend == "" {
		return nil
	}

	backendConfigPath := filepath.Join(os.Getwd(), "backend_override.tf")
	var backendConfigContent string

	if backend == "local" {
		backendConfigContent = fmt.Sprintf(
			`terraform {
  backend "local" {
    path = "%s/terraform.tfstate"
  }
}`, h.ConfigHandler.GetConfigDir())
	} else {
		backendConfigContent = fmt.Sprintf(
			`terraform {
  backend "%s" {}
}`, backend)
	}

	return h.WriteFile(backendConfigPath, backendConfigContent)
}
