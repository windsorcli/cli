package context

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/context/config"
	envvars "github.com/windsorcli/cli/pkg/context/env"
	"github.com/windsorcli/cli/pkg/context/secrets"
	"github.com/windsorcli/cli/pkg/context/shell"
	"github.com/windsorcli/cli/pkg/context/tools"
	"github.com/windsorcli/cli/pkg/di"
)

// ExecutionContext holds common execution values and core dependencies used across the Windsor CLI.
// These fields are set during various initialization steps rather than computed on-demand.
// Includes secret providers for Sops and 1Password, enabling access to secrets across all contexts.
// Also includes environment printers, tools manager, and environment variable/alias storage.
type ExecutionContext struct {
	// ContextName is the current context name
	ContextName string

	// ProjectRoot is the project root directory path
	ProjectRoot string

	// ConfigRoot is the config root directory (<projectRoot>/contexts/<contextName>)
	ConfigRoot string

	// TemplateRoot is the template directory (<projectRoot>/contexts/_template)
	TemplateRoot string

	// Injector is the dependency injector
	Injector di.Injector

	// Core dependencies
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell

	// SecretsProviders contains providers for Sops and 1Password secrets management
	SecretsProviders struct {
		Sops        secrets.SecretsProvider
		Onepassword secrets.SecretsProvider
	}

	// EnvPrinters contains environment printers for various providers and tools
	EnvPrinters struct {
		AwsEnv       envvars.EnvPrinter
		AzureEnv     envvars.EnvPrinter
		DockerEnv    envvars.EnvPrinter
		KubeEnv      envvars.EnvPrinter
		TalosEnv     envvars.EnvPrinter
		TerraformEnv envvars.EnvPrinter
		WindsorEnv   envvars.EnvPrinter
	}

	// ToolsManager manages tool installation and configuration
	ToolsManager tools.ToolsManager

	// envVars stores collected environment variables
	envVars map[string]string

	// aliases stores collected shell aliases
	aliases map[string]string
}

// =============================================================================
// Constructor
// =============================================================================

// NewContext creates a new ExecutionContext with ConfigHandler and Shell initialized if not already present.
// This is the base constructor that ensures core dependencies are available.
// If ConfigHandler is nil, it creates one using the Injector and initializes it.
// If Shell is nil, it creates one using the Injector and initializes it.
// Both are registered in the Injector for use by other components.
// The context also initializes envVars and aliases maps, and automatically sets up
// ContextName, ProjectRoot, ConfigRoot, and TemplateRoot based on the current project state.
// Returns the ExecutionContext with initialized dependencies or an error if initialization fails.
func NewContext(ctx *ExecutionContext) (*ExecutionContext, error) {
	if ctx == nil {
		return nil, fmt.Errorf("execution context is required")
	}
	if ctx.Injector == nil {
		return nil, fmt.Errorf("injector is required")
	}
	injector := ctx.Injector

	if ctx.Shell == nil {
		if existing := injector.Resolve("shell"); existing != nil {
			if shellInstance, ok := existing.(shell.Shell); ok {
				ctx.Shell = shellInstance
			} else {
				shellInstance := shell.NewDefaultShell(injector)
				ctx.Shell = shellInstance
				injector.Register("shell", shellInstance)
			}
		} else {
			shellInstance := shell.NewDefaultShell(injector)
			ctx.Shell = shellInstance
			injector.Register("shell", shellInstance)
		}

		if err := ctx.Shell.Initialize(); err != nil {
			return nil, fmt.Errorf("failed to initialize shell: %w", err)
		}
	}

	if ctx.ConfigHandler == nil {
		if existing := injector.Resolve("configHandler"); existing != nil {
			if configHandler, ok := existing.(config.ConfigHandler); ok {
				ctx.ConfigHandler = configHandler
			} else {
				ctx.ConfigHandler = config.NewConfigHandler(injector)
				injector.Register("configHandler", ctx.ConfigHandler)
			}
		} else {
			ctx.ConfigHandler = config.NewConfigHandler(injector)
			injector.Register("configHandler", ctx.ConfigHandler)
		}

		if err := ctx.ConfigHandler.Initialize(); err != nil {
			return nil, fmt.Errorf("failed to initialize config handler: %w", err)
		}
	}

	if ctx.envVars == nil {
		ctx.envVars = make(map[string]string)
	}
	if ctx.aliases == nil {
		ctx.aliases = make(map[string]string)
	}

	projectRoot, err := ctx.Shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to get project root: %w", err)
	}

	contextName := ctx.ConfigHandler.GetContext()
	ctx.ContextName = contextName
	ctx.ProjectRoot = projectRoot
	ctx.ConfigRoot = filepath.Join(projectRoot, "contexts", contextName)
	ctx.TemplateRoot = filepath.Join(projectRoot, "contexts", "_template")

	return ctx, nil
}

// =============================================================================
// Public Methods
// =============================================================================

// CheckTrustedDirectory verifies that the current directory is in the trusted file list.
// It delegates to the Shell's CheckTrustedDirectory method. Returns an error if the
// directory is not trusted or if Shell is not initialized.
func (ctx *ExecutionContext) CheckTrustedDirectory() error {
	if ctx.Shell == nil {
		return fmt.Errorf("shell not initialized")
	}
	return ctx.Shell.CheckTrustedDirectory()
}

// LoadConfig loads configuration from all sources.
// The context paths (ContextName, ProjectRoot, ConfigRoot, TemplateRoot) are already
// set up in the constructor, so this method only needs to load the configuration data.
// Returns an error if configuration loading fails or if required dependencies are missing.
func (ctx *ExecutionContext) LoadConfig() error {
	if ctx.ConfigHandler == nil {
		return fmt.Errorf("config handler not initialized")
	}

	return ctx.ConfigHandler.LoadConfig()
}

// HandleSessionReset checks for reset flags and session tokens, then resets managed environment
// variables if needed. It checks for WINDSOR_SESSION_TOKEN and uses the shell's CheckResetFlags
// method to determine if a reset should occur. If reset is needed, it calls Shell.Reset() and
// sets NO_CACHE=true. Returns an error if Shell is not initialized or if reset flag checking fails.
func (ctx *ExecutionContext) HandleSessionReset() error {
	if ctx.Shell == nil {
		return fmt.Errorf("shell not initialized")
	}

	hasSessionToken := os.Getenv("WINDSOR_SESSION_TOKEN") != ""
	shouldReset, err := ctx.Shell.CheckResetFlags()
	if err != nil {
		return fmt.Errorf("failed to check reset flags: %w", err)
	}
	if !hasSessionToken {
		shouldReset = true
	}

	if shouldReset {
		ctx.Shell.Reset()
		if err := os.Setenv("NO_CACHE", "true"); err != nil {
			return fmt.Errorf("failed to set NO_CACHE: %w", err)
		}
	}

	return nil
}

// LoadEnvironment loads environment variables and aliases from all configured environment printers.
// It initializes all necessary components, optionally loads secrets if requested, and aggregates
// all environment variables and aliases into the ExecutionContext instance. Returns an error if any required
// dependency is missing or if any step fails. This method expects the ConfigHandler to be set before invocation.
func (ctx *ExecutionContext) LoadEnvironment(decrypt bool) error {
	if ctx.ConfigHandler == nil {
		return fmt.Errorf("config handler not loaded")
	}

	ctx.initializeEnvPrinters()
	ctx.initializeToolsManager()
	ctx.initializeSecretsProviders()

	if err := ctx.initializeComponents(); err != nil {
		return fmt.Errorf("failed to initialize environment components: %w", err)
	}

	if decrypt {
		if err := ctx.loadSecrets(); err != nil {
			return fmt.Errorf("failed to load secrets: %w", err)
		}
	}

	allEnvVars := make(map[string]string)
	allAliases := make(map[string]string)

	for _, printer := range ctx.getAllEnvPrinters() {
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

	ctx.envVars = allEnvVars
	ctx.aliases = allAliases

	return nil
}

// PrintEnvVars returns all collected environment variables in key=value format.
// If no environment variables are loaded, returns an empty string.
func (ctx *ExecutionContext) PrintEnvVars() string {
	if len(ctx.envVars) > 0 {
		return ctx.Shell.RenderEnvVars(ctx.envVars, false)
	}
	return ""
}

// PrintEnvVarsExport returns all collected environment variables in export key=value format.
// If no environment variables are loaded, returns an empty string.
func (ctx *ExecutionContext) PrintEnvVarsExport() string {
	if len(ctx.envVars) > 0 {
		return ctx.Shell.RenderEnvVars(ctx.envVars, true)
	}
	return ""
}

// PrintAliases returns all collected aliases using the shell's RenderAliases method.
// If no aliases are loaded, returns an empty string.
func (ctx *ExecutionContext) PrintAliases() string {
	if len(ctx.aliases) > 0 {
		return ctx.Shell.RenderAliases(ctx.aliases)
	}
	return ""
}

// ExecutePostEnvHooks executes post-environment hooks for all environment printers.
// Returns an error if any hook fails, wrapping the first error encountered with context.
// Returns nil if all hooks execute successfully.
func (ctx *ExecutionContext) ExecutePostEnvHooks() error {
	var firstError error

	for _, printer := range ctx.getAllEnvPrinters() {
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
func (ctx *ExecutionContext) GetEnvVars() map[string]string {
	result := make(map[string]string)
	maps.Copy(result, ctx.envVars)
	return result
}

// GetAliases returns a copy of the collected aliases.
func (ctx *ExecutionContext) GetAliases() map[string]string {
	result := make(map[string]string)
	maps.Copy(result, ctx.aliases)
	return result
}

// =============================================================================
// Environment Private Methods
// =============================================================================

// initializeEnvPrinters initializes environment printers based on configuration settings.
// It creates and registers the appropriate environment printers with the dependency injector
// based on the current configuration state.
func (ctx *ExecutionContext) initializeEnvPrinters() {
	if ctx.EnvPrinters.AwsEnv == nil && ctx.ConfigHandler.GetBool("aws.enabled", false) {
		ctx.EnvPrinters.AwsEnv = envvars.NewAwsEnvPrinter(ctx.Injector)
		ctx.Injector.Register("awsEnv", ctx.EnvPrinters.AwsEnv)
	}
	if ctx.EnvPrinters.AzureEnv == nil && ctx.ConfigHandler.GetBool("azure.enabled", false) {
		ctx.EnvPrinters.AzureEnv = envvars.NewAzureEnvPrinter(ctx.Injector)
		ctx.Injector.Register("azureEnv", ctx.EnvPrinters.AzureEnv)
	}
	if ctx.EnvPrinters.DockerEnv == nil && ctx.ConfigHandler.GetBool("docker.enabled", false) {
		ctx.EnvPrinters.DockerEnv = envvars.NewDockerEnvPrinter(ctx.Injector)
		ctx.Injector.Register("dockerEnv", ctx.EnvPrinters.DockerEnv)
	}
	if ctx.EnvPrinters.KubeEnv == nil && ctx.ConfigHandler.GetBool("cluster.enabled", false) {
		ctx.EnvPrinters.KubeEnv = envvars.NewKubeEnvPrinter(ctx.Injector)
		ctx.Injector.Register("kubeEnv", ctx.EnvPrinters.KubeEnv)
	}
	if ctx.EnvPrinters.TalosEnv == nil &&
		(ctx.ConfigHandler.GetString("cluster.driver", "") == "talos" ||
			ctx.ConfigHandler.GetString("cluster.driver", "") == "omni") {
		ctx.EnvPrinters.TalosEnv = envvars.NewTalosEnvPrinter(ctx.Injector)
		ctx.Injector.Register("talosEnv", ctx.EnvPrinters.TalosEnv)
	}
	if ctx.EnvPrinters.TerraformEnv == nil && ctx.ConfigHandler.GetBool("terraform.enabled", false) {
		ctx.EnvPrinters.TerraformEnv = envvars.NewTerraformEnvPrinter(ctx.Injector)
		ctx.Injector.Register("terraformEnv", ctx.EnvPrinters.TerraformEnv)
	}
	if ctx.EnvPrinters.WindsorEnv == nil {
		ctx.EnvPrinters.WindsorEnv = envvars.NewWindsorEnvPrinter(ctx.Injector)
		ctx.Injector.Register("windsorEnv", ctx.EnvPrinters.WindsorEnv)
	}
}

// initializeToolsManager initializes the tools manager if not already set.
// It creates a new ToolsManager instance and registers it with the dependency injector.
func (ctx *ExecutionContext) initializeToolsManager() {
	if ctx.ToolsManager == nil {
		ctx.ToolsManager = tools.NewToolsManager(ctx.Injector)
		ctx.Injector.Register("toolsManager", ctx.ToolsManager)
	}
}

// initializeSecretsProviders initializes and registers secrets providers with the dependency injector
// based on current configuration settings. The method sets up the SOPS provider if enabled with the
// context's config root path, and sets up the 1Password provider if enabled, using a mock in test
// scenarios. Providers are only initialized if not already present on the context.
func (ctx *ExecutionContext) initializeSecretsProviders() {
	if ctx.SecretsProviders.Sops == nil && ctx.ConfigHandler.GetBool("secrets.sops.enabled", false) {
		configPath := ctx.ConfigRoot
		ctx.SecretsProviders.Sops = secrets.NewSopsSecretsProvider(configPath, ctx.Injector)
		ctx.Injector.Register("sopsSecretsProvider", ctx.SecretsProviders.Sops)
	}

	if ctx.SecretsProviders.Onepassword == nil && ctx.ConfigHandler.GetBool("secrets.onepassword.enabled", false) {
		ctx.SecretsProviders.Onepassword = secrets.NewMockSecretsProvider(ctx.Injector)
		ctx.Injector.Register("onepasswordSecretsProvider", ctx.SecretsProviders.Onepassword)
	}
}

// getAllEnvPrinters returns all environment printers in a consistent order.
// This ensures that environment variables are processed in a predictable sequence
// with WindsorEnv being processed last to take precedence.
func (ctx *ExecutionContext) getAllEnvPrinters() []envvars.EnvPrinter {
	return []envvars.EnvPrinter{
		ctx.EnvPrinters.AwsEnv,
		ctx.EnvPrinters.AzureEnv,
		ctx.EnvPrinters.DockerEnv,
		ctx.EnvPrinters.KubeEnv,
		ctx.EnvPrinters.TalosEnv,
		ctx.EnvPrinters.TerraformEnv,
		ctx.EnvPrinters.WindsorEnv,
	}
}

// initializeComponents initializes all environment-related components required after setup.
// This includes initializing the tools manager (if present) and all configured environment printers.
// Each component's Initialize method is called if the component is non-nil.
// Returns an error if any initialization fails, otherwise returns nil.
func (ctx *ExecutionContext) initializeComponents() error {
	if ctx.ToolsManager != nil {
		if err := ctx.ToolsManager.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize tools manager: %w", err)
		}
	}
	for _, printer := range ctx.getAllEnvPrinters() {
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
func (ctx *ExecutionContext) loadSecrets() error {
	providers := []secrets.SecretsProvider{
		ctx.SecretsProviders.Sops,
		ctx.SecretsProviders.Onepassword,
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
