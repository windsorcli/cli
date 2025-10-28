// The Environment package provides a top-level interface for environment management functionality.
// It consolidates environment variable and alias management from the runtime and environment packages,
// providing a unified API for loading, printing, and managing environment state across the Windsor CLI.
// The Environment acts as the primary interface for all environment-related operations,
// coordinating between different environment printers and providing consistent behavior.

package environment

import (
	"fmt"
	"maps"

	"github.com/windsorcli/cli/pkg/environment/envvars"
	"github.com/windsorcli/cli/pkg/environment/tools"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/types"
)

// =============================================================================
// Types
// =============================================================================

// EnvironmentExecutionContext holds the execution context for environment operations.
// It embeds the base ExecutionContext and includes all environment-specific dependencies.
type EnvironmentExecutionContext struct {
	types.ExecutionContext

	EnvPrinters struct {
		AwsEnv       envvars.EnvPrinter
		AzureEnv     envvars.EnvPrinter
		DockerEnv    envvars.EnvPrinter
		KubeEnv      envvars.EnvPrinter
		TalosEnv     envvars.EnvPrinter
		TerraformEnv envvars.EnvPrinter
		WindsorEnv   envvars.EnvPrinter
	}
	ToolsManager tools.ToolsManager
}

// Environment manages environment variables and aliases across the Windsor CLI.
// It provides a unified interface for loading, printing, and managing environment state
// by coordinating between different environment printers and handling secrets decryption.
type Environment struct {
	*EnvironmentExecutionContext
	envVars map[string]string
	aliases map[string]string
}

// =============================================================================
// Constructor
// =============================================================================

// NewEnvironment creates a new Environment instance with the given execution context.
// The constructor sets up all environment printers, the tools manager, and secrets providers based on current configuration.
// It returns an Environment object that will initialize components when LoadEnvironment is called.
func NewEnvironment(ctx *EnvironmentExecutionContext) *Environment {
	env := &Environment{
		EnvironmentExecutionContext: ctx,
		envVars:                     make(map[string]string),
		aliases:                     make(map[string]string),
	}

	env.initializeEnvPrinters()
	env.initializeToolsManager()
	env.initializeSecretsProviders()

	return env
}

// =============================================================================
// Public Methods
// =============================================================================

// LoadEnvironment loads environment variables and aliases from all configured environment printers.
// It initializes all necessary components, optionally loads secrets if requested, and aggregates
// all environment variables and aliases into the Environment instance. Returns an error if any required
// dependency is missing or if any step fails. This method expects the ConfigHandler to be set before invocation.
func (e *Environment) LoadEnvironment(decrypt bool) error {
	if e.ConfigHandler == nil {
		return fmt.Errorf("config handler not loaded")
	}

	if err := e.initializeComponents(); err != nil {
		return fmt.Errorf("failed to initialize environment components: %w", err)
	}

	if decrypt {
		if err := e.loadSecrets(); err != nil {
			return fmt.Errorf("failed to load secrets: %w", err)
		}
	}

	allEnvVars := make(map[string]string)
	allAliases := make(map[string]string)

	for _, printer := range e.getAllEnvPrinters() {
		if printer != nil {
			envVars, err := printer.GetEnvVars()
			if err != nil {
				return fmt.Errorf("error getting environment variables: %w", err)
			}
			maps.Copy(allEnvVars, envVars)

			aliases, err := printer.GetAlias()
			if err != nil {
				return fmt.Errorf("error getting aliases: %w", err)
			}
			maps.Copy(allAliases, aliases)
		}
	}

	e.envVars = allEnvVars
	e.aliases = allAliases

	return nil
}

// PrintEnvVars returns all collected environment variables in key=value format.
// If no environment variables are loaded, returns an empty string.
func (e *Environment) PrintEnvVars() string {
	if len(e.envVars) > 0 {
		return e.Shell.RenderEnvVars(e.envVars, false)
	}
	return ""
}

// PrintEnvVarsExport returns all collected environment variables in export key=value format.
// If no environment variables are loaded, returns an empty string.
func (e *Environment) PrintEnvVarsExport() string {
	if len(e.envVars) > 0 {
		return e.Shell.RenderEnvVars(e.envVars, true)
	}
	return ""
}

// PrintAliases returns all collected aliases using the shell's RenderAliases method.
// If no aliases are loaded, returns an empty string.
func (e *Environment) PrintAliases() string {
	if len(e.aliases) > 0 {
		return e.Shell.RenderAliases(e.aliases)
	}
	return ""
}

// ExecutePostEnvHooks executes post-environment hooks for all environment printers.
func (e *Environment) ExecutePostEnvHooks() error {
	var firstError error

	for _, printer := range e.getAllEnvPrinters() {
		if printer != nil {
			if err := printer.PostEnvHook(); err != nil && firstError == nil {
				firstError = err
			}
		}
	}

	if firstError != nil {
		return fmt.Errorf("failed to execute post env hooks: %w", firstError)
	}

	return nil
}

// GetEnvVars returns a copy of the collected environment variables.
func (e *Environment) GetEnvVars() map[string]string {
	result := make(map[string]string)
	maps.Copy(result, e.envVars)
	return result
}

// GetAliases returns a copy of the collected aliases.
func (e *Environment) GetAliases() map[string]string {
	result := make(map[string]string)
	maps.Copy(result, e.aliases)
	return result
}

// =============================================================================
// Private Methods
// =============================================================================

// initializeEnvPrinters initializes environment printers based on configuration settings.
// It creates and registers the appropriate environment printers with the dependency injector
// based on the current configuration state.
func (e *Environment) initializeEnvPrinters() {
	if e.EnvPrinters.AwsEnv == nil && e.ConfigHandler.GetBool("aws.enabled", false) {
		e.EnvPrinters.AwsEnv = envvars.NewAwsEnvPrinter(e.Injector)
		e.Injector.Register("awsEnv", e.EnvPrinters.AwsEnv)
	}
	if e.EnvPrinters.AzureEnv == nil && e.ConfigHandler.GetBool("azure.enabled", false) {
		e.EnvPrinters.AzureEnv = envvars.NewAzureEnvPrinter(e.Injector)
		e.Injector.Register("azureEnv", e.EnvPrinters.AzureEnv)
	}
	if e.EnvPrinters.DockerEnv == nil && e.ConfigHandler.GetBool("docker.enabled", false) {
		e.EnvPrinters.DockerEnv = envvars.NewDockerEnvPrinter(e.Injector)
		e.Injector.Register("dockerEnv", e.EnvPrinters.DockerEnv)
	}
	if e.EnvPrinters.KubeEnv == nil && e.ConfigHandler.GetBool("cluster.enabled", false) {
		e.EnvPrinters.KubeEnv = envvars.NewKubeEnvPrinter(e.Injector)
		e.Injector.Register("kubeEnv", e.EnvPrinters.KubeEnv)
	}
	if e.EnvPrinters.TalosEnv == nil &&
		(e.ConfigHandler.GetString("cluster.driver", "") == "talos" ||
			e.ConfigHandler.GetString("cluster.driver", "") == "omni") {
		e.EnvPrinters.TalosEnv = envvars.NewTalosEnvPrinter(e.Injector)
		e.Injector.Register("talosEnv", e.EnvPrinters.TalosEnv)
	}
	if e.EnvPrinters.TerraformEnv == nil && e.ConfigHandler.GetBool("terraform.enabled", false) {
		e.EnvPrinters.TerraformEnv = envvars.NewTerraformEnvPrinter(e.Injector)
		e.Injector.Register("terraformEnv", e.EnvPrinters.TerraformEnv)
	}
	if e.EnvPrinters.WindsorEnv == nil {
		e.EnvPrinters.WindsorEnv = envvars.NewWindsorEnvPrinter(e.Injector)
		e.Injector.Register("windsorEnv", e.EnvPrinters.WindsorEnv)
	}
}

// initializeToolsManager initializes the tools manager if not already set.
// It creates a new ToolsManager instance and registers it with the dependency injector.
func (e *Environment) initializeToolsManager() {
	if e.ToolsManager == nil {
		e.ToolsManager = tools.NewToolsManager(e.Injector)
		e.Injector.Register("toolsManager", e.ToolsManager)
	}
}

// initializeSecretsProviders initializes and registers secrets providers with the dependency injector
// based on current configuration settings. The method sets up the SOPS provider if enabled with the
// environment's config root path, and sets up the 1Password provider if enabled, using a mock in test
// scenarios. Providers are only initialized if not already present on the environment.
func (e *Environment) initializeSecretsProviders() {
	if e.SecretsProviders.Sops == nil && e.ConfigHandler.GetBool("secrets.sops.enabled", false) {
		configPath := e.ConfigRoot
		e.SecretsProviders.Sops = secrets.NewSopsSecretsProvider(configPath, e.Injector)
		e.Injector.Register("sopsSecretsProvider", e.SecretsProviders.Sops)
	}

	if e.SecretsProviders.Onepassword == nil && e.ConfigHandler.GetBool("secrets.onepassword.enabled", false) {
		e.SecretsProviders.Onepassword = secrets.NewMockSecretsProvider(e.Injector)
		e.Injector.Register("onepasswordSecretsProvider", e.SecretsProviders.Onepassword)
	}
}

// getAllEnvPrinters returns all environment printers in a consistent order.
// This ensures that environment variables are processed in a predictable sequence
// with WindsorEnv being processed last to take precedence.
func (e *Environment) getAllEnvPrinters() []envvars.EnvPrinter {
	return []envvars.EnvPrinter{
		e.EnvPrinters.AwsEnv,
		e.EnvPrinters.AzureEnv,
		e.EnvPrinters.DockerEnv,
		e.EnvPrinters.KubeEnv,
		e.EnvPrinters.TalosEnv,
		e.EnvPrinters.TerraformEnv,
		e.EnvPrinters.WindsorEnv,
	}
}

// initializeComponents initializes all environment-related components required after setup.
// This includes initializing the tools manager (if present) and all configured environment printers.
// Each component's Initialize method is called if the component is non-nil.
// Returns an error if any initialization fails, otherwise returns nil.
func (e *Environment) initializeComponents() error {
	if e.ToolsManager != nil {
		if err := e.ToolsManager.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize tools manager: %w", err)
		}
	}
	for _, printer := range e.getAllEnvPrinters() {
		if printer != nil {
			if err := printer.Initialize(); err != nil {
				return fmt.Errorf("failed to initialize environment printer: %w", err)
			}
		}
	}
	return nil
}

// loadSecrets loads secrets from configured secrets providers.
// It attempts to load secrets from both SOPS and 1Password providers if they are available.
func (e *Environment) loadSecrets() error {
	providers := []secrets.SecretsProvider{
		e.SecretsProviders.Sops,
		e.SecretsProviders.Onepassword,
	}

	for _, provider := range providers {
		if provider != nil {
			if err := provider.LoadSecrets(); err != nil {
				return fmt.Errorf("failed to load secrets: %w", err)
			}
		}
	}

	return nil
}
