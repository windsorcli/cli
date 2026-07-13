// The TerraformEnvPrinter is a specialized component that manages Terraform environment configuration.
// It provides Terraform-specific environment variable management and configuration,
// The TerraformEnvPrinter handles backend configuration, variable files, and state management,
// ensuring proper Terraform CLI integration and environment setup for infrastructure operations.

package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/terraform"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Types
// =============================================================================

// TerraformArgs is an alias for terraform.TerraformArgs to maintain backward compatibility
type TerraformArgs = terraform.TerraformArgs

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
func NewTerraformEnvPrinter(shell shell.Shell, configHandler config.ConfigHandler, toolsManager tools.ToolsManager, terraformProvider terraform.TerraformProvider) *TerraformEnvPrinter {
	if shell == nil {
		panic("shell is required")
	}
	if configHandler == nil {
		panic("config handler is required")
	}
	if toolsManager == nil {
		panic("tools manager is required")
	}
	if terraformProvider == nil {
		panic("terraform provider is required")
	}

	return &TerraformEnvPrinter{
		BaseEnvPrinter:    *NewBaseEnvPrinter(shell, configHandler),
		toolsManager:      toolsManager,
		terraformProvider: terraformProvider,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// terraformScopedEnvKeysVar is the managed env var recording which keys the current
// contexts/<context>/terraform/.env last exported, so getEmptyEnvVars can unset exactly
// those keys on a later invocation even if the file has since changed or been removed.
const terraformScopedEnvKeysVar = "WINDSOR_MANAGED_TERRAFORM_ENV"

// GetEnvVars returns a map of environment variables for Terraform operations.
// If not in a Terraform project directory, it unsets managed variables present in the environment.
// Otherwise, it generates Terraform arguments for the current project, merged with any
// contexts/<context>/terraform/.env content. Every returned key is tracked as managed so it
// is unset cleanly when the operator leaves the directory; the terraform/.env key names are
// additionally recorded in WINDSOR_MANAGED_TERRAFORM_ENV so getEmptyEnvVars can still unset
// them later even if the file's contents change or the file is removed in the meantime.
// Returns the environment variable map or an error if resolution fails.
func (e *TerraformEnvPrinter) GetEnvVars() (map[string]string, error) {
	projectPath, err := e.terraformProvider.FindRelativeProjectPath()
	if err != nil {
		return nil, fmt.Errorf("error finding project path: %w", err)
	}

	if projectPath == "" {
		return e.getEmptyEnvVars(), nil
	}

	terraformVars, _, err := e.terraformProvider.GetEnvVars(projectPath, true)
	for key := range terraformVars {
		e.SetManagedEnv(key)
	}

	if scopedKeys, keysErr := e.terraformProvider.TerraformScopedEnvKeys(); keysErr == nil && len(scopedKeys) > 0 {
		if terraformVars == nil {
			terraformVars = make(map[string]string)
		}
		terraformVars[terraformScopedEnvKeysVar] = strings.Join(scopedKeys, ",")
		e.SetManagedEnv(terraformScopedEnvKeysVar)
	}

	return terraformVars, err
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

// getEmptyEnvVars returns env vars for unsetting managed variables when not in a terraform
// project, including any keys the last-visited contexts/<context>/terraform/.env exported.
// Those key names come from WINDSOR_MANAGED_TERRAFORM_ENV (recorded by GetEnvVars when the
// keys were actually exported) rather than re-reading the current terraform/.env: the file
// may have changed or been removed since, and re-reading it would miss keys it no longer
// declares, leaking them in the shell indefinitely.
func (e *TerraformEnvPrinter) getEmptyEnvVars() map[string]string {
	envVars := make(map[string]string)
	managedVars := []string{
		"TF_DATA_DIR",
		"TF_CLI_ARGS_init",
		"TF_CLI_ARGS_plan",
		"TF_CLI_ARGS_apply",
		"TF_CLI_ARGS_import",
		"TF_CLI_ARGS_destroy",
		"TF_VAR_context",
		"TF_VAR_project_root",
		"TF_VAR_context_path",
		"TF_VAR_context_id",
		"TF_VAR_os_type",
		"TF_VAR_operation",
		terraformScopedEnvKeysVar,
	}
	if managedEnv := e.shims.Getenv("WINDSOR_MANAGED_ENV"); managedEnv != "" {
		for _, key := range strings.Split(managedEnv, ",") {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if strings.HasPrefix(key, "TF_VAR_") || strings.HasPrefix(key, "TF_CLI_ARGS_") || key == "TF_DATA_DIR" {
				managedVars = append(managedVars, key)
			}
		}
	}
	if scoped := e.shims.Getenv(terraformScopedEnvKeysVar); scoped != "" {
		for _, key := range strings.Split(scoped, ",") {
			if key = strings.TrimSpace(key); key != "" {
				managedVars = append(managedVars, key)
			}
		}
	}

	for _, varName := range managedVars {
		if _, exists := e.shims.LookupEnv(varName); exists {
			envVars[varName] = ""
		}
	}

	return envVars
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure TerraformEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*TerraformEnvPrinter)(nil)
