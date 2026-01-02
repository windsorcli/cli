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
	return &TerraformEnvPrinter{
		BaseEnvPrinter:    *NewBaseEnvPrinter(shell, configHandler),
		toolsManager:      toolsManager,
		terraformProvider: terraformProvider,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetEnvVars returns a map of environment variables for Terraform operations.
// If not in a Terraform project directory, it unsets managed TF_ variables present in the environment.
// Otherwise, it generates Terraform arguments for the current project.
// Returns the environment variable map or an error if resolution fails.
func (e *TerraformEnvPrinter) GetEnvVars() (map[string]string, error) {
	projectPath, err := e.terraformProvider.FindRelativeProjectPath()
	if err != nil {
		return nil, fmt.Errorf("error finding project path: %w", err)
	}

	if projectPath == "" {
		return e.getEmptyEnvVars(), nil
	}

	terraformVars, _, err := e.GetEnvVarsForPath(projectPath, projectPath, true)
	return terraformVars, err
}

// GetEnvVarsForPath generates Terraform environment variables for the given component ID and module path.
// Returns both the environment variables map and the TerraformArgs struct, or an error.
func (e *TerraformEnvPrinter) GetEnvVarsForPath(componentID, modulePath string, interactive bool) (map[string]string, *TerraformArgs, error) {
	terraformArgs, err := e.terraformProvider.GenerateTerraformArgs(componentID, modulePath, interactive)
	if err != nil {
		return nil, nil, fmt.Errorf("error generating terraform args: %w", err)
	}

	terraformVars, err := e.formatTerraformArgsAsEnvVars(terraformArgs)
	if err != nil {
		return nil, nil, err
	}

	return terraformVars, terraformArgs, nil
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

// getEmptyEnvVars returns env vars for unsetting managed variables when not in a terraform project.
func (e *TerraformEnvPrinter) getEmptyEnvVars() map[string]string {
	envVars := make(map[string]string)
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

	return envVars
}

// formatTerraformArgsAsEnvVars formats TerraformArgs into environment variables.
// Returns an error if GetConfigRoot fails, which would result in an empty TF_VAR_context_path.
func (e *TerraformEnvPrinter) formatTerraformArgsAsEnvVars(terraformArgs *TerraformArgs) (map[string]string, error) {
	configRoot, err := e.configHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error getting config root: %w", err)
	}

	terraformVars := make(map[string]string)
	terraformVars["TF_DATA_DIR"] = terraformArgs.TFDataDir
	terraformVars["TF_CLI_ARGS_init"] = e.formatArgsForEnv(terraformArgs.InitArgs)
	terraformVars["TF_CLI_ARGS_plan"] = e.formatArgsForEnv(terraformArgs.PlanArgs)
	terraformVars["TF_CLI_ARGS_apply"] = e.formatArgsForEnv(terraformArgs.ApplyArgs)
	terraformVars["TF_CLI_ARGS_refresh"] = e.formatArgsForEnv(terraformArgs.RefreshArgs)
	terraformVars["TF_CLI_ARGS_import"] = e.formatArgsForEnv(terraformArgs.ImportArgs)
	terraformVars["TF_CLI_ARGS_destroy"] = e.formatArgsForEnv(terraformArgs.DestroyArgs)
	terraformVars["TF_VAR_context_path"] = filepath.ToSlash(configRoot)
	terraformVars["TF_VAR_context_id"] = e.configHandler.GetString("id", "")

	if e.shims.Goos() == "windows" {
		terraformVars["TF_VAR_os_type"] = "windows"
	} else {
		terraformVars["TF_VAR_os_type"] = "unix"
	}

	return terraformVars, nil
}

// formatArgsForEnv formats CLI arguments for use in environment variables.
// It adds quotes around file paths and values that need them for shell compatibility.
// Handles Unix absolute paths (/path), relative paths (./path), and Windows drive letter paths
// (both D:\path and D:/path formats) by detecting drive letters followed by a colon.
func (e *TerraformEnvPrinter) formatArgsForEnv(args []string) string {
	formatted := make([]string, len(args))
	for i, arg := range args {
		if strings.HasPrefix(arg, "-var-file=") {
			value := strings.TrimPrefix(arg, "-var-file=")
			formatted[i] = fmt.Sprintf("-var-file=\"%s\"", value)
		} else if strings.HasPrefix(arg, "-out=") {
			value := strings.TrimPrefix(arg, "-out=")
			formatted[i] = fmt.Sprintf("-out=\"%s\"", value)
		} else if strings.HasPrefix(arg, "-backend-config=") {
			if strings.Contains(arg, "=") && !strings.HasPrefix(strings.SplitN(arg, "=", 2)[1], "\"") {
				parts := strings.SplitN(arg, "=", 2)
				formatted[i] = fmt.Sprintf("%s=\"%s\"", parts[0], parts[1])
			} else {
				formatted[i] = arg
			}
		} else if !strings.HasPrefix(arg, "-") && (strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, ".") || (len(arg) >= 2 && arg[1] == ':' && ((arg[0] >= 'A' && arg[0] <= 'Z') || (arg[0] >= 'a' && arg[0] <= 'z')))) {
			formatted[i] = fmt.Sprintf("\"%s\"", arg)
		} else {
			formatted[i] = arg
		}
	}
	return strings.TrimSpace(strings.Join(formatted, " "))
}

// Ensure TerraformEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*TerraformEnvPrinter)(nil)
