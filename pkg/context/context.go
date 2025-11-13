package context

import (
	"crypto/rand"
	"fmt"
	"maps"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/windsorcli/cli/pkg/context/config"
	"github.com/windsorcli/cli/pkg/context/env"
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
		AwsEnv       env.EnvPrinter
		AzureEnv     env.EnvPrinter
		DockerEnv    env.EnvPrinter
		KubeEnv      env.EnvPrinter
		TalosEnv     env.EnvPrinter
		TerraformEnv env.EnvPrinter
		WindsorEnv   env.EnvPrinter
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

	projectRoot, err := ctx.Shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to get project root: %w", err)
	}
	ctx.ProjectRoot = projectRoot
	ctx.ConfigRoot = filepath.Join(ctx.ProjectRoot, "contexts", ctx.ContextName)
	ctx.TemplateRoot = filepath.Join(ctx.ProjectRoot, "contexts", "_template")

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

	projectRoot, err = ctx.Shell.GetProjectRoot()
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

// LoadEnvironment loads environment variables and aliases from all configured environment printers,
// then executes post-environment hooks. It initializes all necessary components, optionally loads
// secrets if requested, and aggregates all environment variables and aliases into the ExecutionContext
// instance. Returns an error if any required dependency is missing or if any step fails. This method
// expects the ConfigHandler to be set before invocation.
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

	for key, value := range allEnvVars {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("error setting environment variable %s: %w", key, err)
		}
	}

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

// PrintEnvVars returns all collected environment variables in key=value format.
// If no environment variables are loaded, returns an empty string.
func (ctx *ExecutionContext) PrintEnvVars() string {
	if ctx.Shell == nil || len(ctx.envVars) == 0 {
		return ""
	}
	return ctx.Shell.RenderEnvVars(ctx.envVars, false)
}

// PrintEnvVarsExport returns all collected environment variables in export key=value format.
// If no environment variables are loaded, returns an empty string.
func (ctx *ExecutionContext) PrintEnvVarsExport() string {
	if ctx.Shell == nil || len(ctx.envVars) == 0 {
		return ""
	}
	return ctx.Shell.RenderEnvVars(ctx.envVars, true)
}

// PrintAliases returns all collected aliases using the shell's RenderAliases method.
// If no aliases are loaded, returns an empty string.
func (ctx *ExecutionContext) PrintAliases() string {
	if ctx.Shell == nil || len(ctx.aliases) == 0 {
		return ""
	}
	return ctx.Shell.RenderAliases(ctx.aliases)
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

// CheckTools performs tool version checking using the tools manager.
// It validates that all required tools are installed and meet minimum version requirements.
// The tools manager must be initialized before calling this method. Returns an error if
// the tools manager is not available or if tool checking fails.
func (ctx *ExecutionContext) CheckTools() error {
	if ctx.ToolsManager == nil {
		ctx.initializeToolsManager()
		if ctx.ToolsManager == nil {
			return fmt.Errorf("tools manager not available")
		}
		if err := ctx.ToolsManager.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize tools manager: %w", err)
		}
	}

	if err := ctx.ToolsManager.Check(); err != nil {
		return fmt.Errorf("error checking tools: %w", err)
	}

	return nil
}

// GetBuildID retrieves the current build ID from the .windsor/.build-id file.
// If no build ID exists, a new one is generated, persisted, and returned.
// Returns the build ID string or an error if retrieval or persistence fails.
func (ctx *ExecutionContext) GetBuildID() (string, error) {
	projectRoot := ctx.ProjectRoot

	if err := os.MkdirAll(projectRoot, 0750); err != nil {
		return "", fmt.Errorf("failed to create project root directory: %w", err)
	}

	root, err := os.OpenRoot(projectRoot)
	if err != nil {
		return "", fmt.Errorf("failed to open project root: %w", err)
	}
	defer root.Close()

	buildIDPath := ".windsor/.build-id"

	var buildID string

	if _, err := root.Stat(buildIDPath); os.IsNotExist(err) {
		buildID = ""
	} else {
		data, err := root.ReadFile(buildIDPath)
		if err != nil {
			return "", fmt.Errorf("failed to read build ID file: %w", err)
		}
		buildID = strings.TrimSpace(string(data))
	}

	if buildID == "" {
		newBuildID, err := ctx.generateBuildID()
		if err != nil {
			return "", fmt.Errorf("failed to generate build ID: %w", err)
		}
		if err := ctx.writeBuildIDToFile(newBuildID); err != nil {
			return "", fmt.Errorf("failed to set build ID: %w", err)
		}
		return newBuildID, nil
	}

	return buildID, nil
}

// GenerateBuildID generates a new build ID and persists it to the .windsor/.build-id file,
// overwriting any existing value. Returns the new build ID or an error if generation or persistence fails.
func (ctx *ExecutionContext) GenerateBuildID() (string, error) {
	newBuildID, err := ctx.generateBuildID()
	if err != nil {
		return "", fmt.Errorf("failed to generate build ID: %w", err)
	}

	if err := ctx.writeBuildIDToFile(newBuildID); err != nil {
		return "", fmt.Errorf("failed to set build ID: %w", err)
	}

	return newBuildID, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// initializeEnvPrinters initializes environment printers based on configuration settings.
// It creates and registers the appropriate environment printers with the dependency injector
// based on the current configuration state.
func (ctx *ExecutionContext) initializeEnvPrinters() {
	if ctx.EnvPrinters.AwsEnv == nil && ctx.ConfigHandler.GetBool("aws.enabled", false) {
		ctx.EnvPrinters.AwsEnv = env.NewAwsEnvPrinter(ctx.Injector)
		ctx.Injector.Register("awsEnv", ctx.EnvPrinters.AwsEnv)
	}
	if ctx.EnvPrinters.AzureEnv == nil && ctx.ConfigHandler.GetBool("azure.enabled", false) {
		ctx.EnvPrinters.AzureEnv = env.NewAzureEnvPrinter(ctx.Injector)
		ctx.Injector.Register("azureEnv", ctx.EnvPrinters.AzureEnv)
	}
	if ctx.EnvPrinters.DockerEnv == nil && ctx.ConfigHandler.GetBool("docker.enabled", false) {
		if existingPrinter := ctx.Injector.Resolve("dockerEnv"); existingPrinter != nil {
			if printer, ok := existingPrinter.(env.EnvPrinter); ok {
				ctx.EnvPrinters.DockerEnv = printer
			}
		}
		if ctx.EnvPrinters.DockerEnv == nil {
			ctx.EnvPrinters.DockerEnv = env.NewDockerEnvPrinter(ctx.Injector)
			ctx.Injector.Register("dockerEnv", ctx.EnvPrinters.DockerEnv)
		}
	}
	if ctx.EnvPrinters.KubeEnv == nil && ctx.ConfigHandler.GetBool("cluster.enabled", false) {
		if existingPrinter := ctx.Injector.Resolve("kubeEnv"); existingPrinter != nil {
			if printer, ok := existingPrinter.(env.EnvPrinter); ok {
				ctx.EnvPrinters.KubeEnv = printer
			}
		}
		if ctx.EnvPrinters.KubeEnv == nil {
			ctx.EnvPrinters.KubeEnv = env.NewKubeEnvPrinter(ctx.Injector)
			ctx.Injector.Register("kubeEnv", ctx.EnvPrinters.KubeEnv)
		}
	}
	if ctx.EnvPrinters.TalosEnv == nil &&
		(ctx.ConfigHandler.GetString("cluster.driver", "") == "talos" ||
			ctx.ConfigHandler.GetString("cluster.driver", "") == "omni") {
		if existingPrinter := ctx.Injector.Resolve("talosEnv"); existingPrinter != nil {
			if printer, ok := existingPrinter.(env.EnvPrinter); ok {
				ctx.EnvPrinters.TalosEnv = printer
			}
		}
		if ctx.EnvPrinters.TalosEnv == nil {
			ctx.EnvPrinters.TalosEnv = env.NewTalosEnvPrinter(ctx.Injector)
			ctx.Injector.Register("talosEnv", ctx.EnvPrinters.TalosEnv)
		}
	}
	if ctx.EnvPrinters.TerraformEnv == nil && ctx.ConfigHandler.GetBool("terraform.enabled", false) {
		if existingPrinter := ctx.Injector.Resolve("terraformEnv"); existingPrinter != nil {
			if printer, ok := existingPrinter.(env.EnvPrinter); ok {
				ctx.EnvPrinters.TerraformEnv = printer
			}
		}
		if ctx.EnvPrinters.TerraformEnv == nil {
			ctx.EnvPrinters.TerraformEnv = env.NewTerraformEnvPrinter(ctx.Injector)
			ctx.Injector.Register("terraformEnv", ctx.EnvPrinters.TerraformEnv)
		}
	}
	if ctx.EnvPrinters.WindsorEnv == nil {
		if existingPrinter := ctx.Injector.Resolve("windsorEnv"); existingPrinter != nil {
			if printer, ok := existingPrinter.(env.EnvPrinter); ok {
				ctx.EnvPrinters.WindsorEnv = printer
			}
		}
		if ctx.EnvPrinters.WindsorEnv == nil {
			ctx.EnvPrinters.WindsorEnv = env.NewWindsorEnvPrinter(ctx.Injector)
			ctx.Injector.Register("windsorEnv", ctx.EnvPrinters.WindsorEnv)
		}
	}
}

// initializeToolsManager initializes the tools manager if not already set.
// It checks the injector for an existing tools manager first, and only creates a new one if not found.
// It creates a new ToolsManager instance and registers it with the dependency injector.
func (ctx *ExecutionContext) initializeToolsManager() {
	if ctx.ToolsManager == nil {
		if existingManager := ctx.Injector.Resolve("toolsManager"); existingManager != nil {
			if toolsManager, ok := existingManager.(tools.ToolsManager); ok {
				ctx.ToolsManager = toolsManager
				return
			}
		}
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
		if existingProvider := ctx.Injector.Resolve("sopsSecretsProvider"); existingProvider != nil {
			if provider, ok := existingProvider.(secrets.SecretsProvider); ok {
				ctx.SecretsProviders.Sops = provider
			}
		}
		if ctx.SecretsProviders.Sops == nil {
			configPath := ctx.ConfigRoot
			ctx.SecretsProviders.Sops = secrets.NewSopsSecretsProvider(configPath, ctx.Injector)
			ctx.Injector.Register("sopsSecretsProvider", ctx.SecretsProviders.Sops)
		}
	}

	if ctx.SecretsProviders.Onepassword == nil && ctx.ConfigHandler.GetBool("secrets.onepassword.enabled", false) {
		if existingProvider := ctx.Injector.Resolve("onepasswordSecretsProvider"); existingProvider != nil {
			if provider, ok := existingProvider.(secrets.SecretsProvider); ok {
				ctx.SecretsProviders.Onepassword = provider
			}
		}
		if ctx.SecretsProviders.Onepassword == nil {
			ctx.SecretsProviders.Onepassword = secrets.NewMockSecretsProvider(ctx.Injector)
			ctx.Injector.Register("onepasswordSecretsProvider", ctx.SecretsProviders.Onepassword)
		}
	}
}

// getAllEnvPrinters returns all environment printers in a consistent order.
// This ensures that environment variables are processed in a predictable sequence
// with WindsorEnv being processed last to take precedence.
func (ctx *ExecutionContext) getAllEnvPrinters() []env.EnvPrinter {
	return []env.EnvPrinter{
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

// writeBuildIDToFile writes the provided build ID string to the .windsor/.build-id file in the project root.
// Ensures the .windsor directory exists before writing. Returns an error if directory creation or file write fails.
func (ctx *ExecutionContext) writeBuildIDToFile(buildID string) error {
	projectRoot := ctx.ProjectRoot

	if err := os.MkdirAll(projectRoot, 0750); err != nil {
		return fmt.Errorf("failed to create project root directory: %w", err)
	}

	root, err := os.OpenRoot(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to open project root: %w", err)
	}
	defer root.Close()

	buildIDPath := ".windsor/.build-id"
	buildIDDir := ".windsor"

	if err := root.MkdirAll(buildIDDir, 0750); err != nil {
		return fmt.Errorf("failed to create build ID directory: %w", err)
	}

	return root.WriteFile(buildIDPath, []byte(buildID), 0600)
}

// generateBuildID generates and returns a build ID string in the format YYMMDD.RANDOM.#.
// YYMMDD is the current date (year, month, day), RANDOM is a random three-digit number for collision prevention,
// and # is a sequential counter incremented for each build on the same day. If a build ID already exists for the current day,
// the counter is incremented; otherwise, a new build ID is generated with counter set to 1. Ensures global ordering and uniqueness.
// Returns the build ID string or an error if generation or retrieval fails.
func (ctx *ExecutionContext) generateBuildID() (string, error) {
	now := time.Now()
	yy := now.Year() % 100
	mm := int(now.Month())
	dd := now.Day()
	datePart := fmt.Sprintf("%02d%02d%02d", yy, mm, dd)

	projectRoot := ctx.ProjectRoot

	if err := os.MkdirAll(projectRoot, 0750); err != nil {
		return "", fmt.Errorf("failed to create project root directory: %w", err)
	}

	root, err := os.OpenRoot(projectRoot)
	if err != nil {
		return "", fmt.Errorf("failed to open project root: %w", err)
	}
	defer root.Close()

	buildIDPath := ".windsor/.build-id"

	var existingBuildID string

	if _, err := root.Stat(buildIDPath); os.IsNotExist(err) {
		existingBuildID = ""
	} else {
		data, err := root.ReadFile(buildIDPath)
		if err != nil {
			return "", fmt.Errorf("failed to read build ID file: %w", err)
		}
		existingBuildID = strings.TrimSpace(string(data))
	}

	if existingBuildID != "" {
		return ctx.incrementBuildID(existingBuildID, datePart)
	}

	random, err := rand.Int(rand.Reader, big.NewInt(1000))
	if err != nil {
		return "", fmt.Errorf("failed to generate random number: %w", err)
	}
	counter := 1
	randomPart := fmt.Sprintf("%03d", random.Int64())
	counterPart := fmt.Sprintf("%d", counter)

	return fmt.Sprintf("%s.%s.%s", datePart, randomPart, counterPart), nil
}

// incrementBuildID parses an existing build ID and increments its counter component.
// If the date component differs from the current date, generates a new random number and resets the counter to 1.
// Returns the incremented or reset build ID string, or an error if the input format is invalid.
func (ctx *ExecutionContext) incrementBuildID(existingBuildID, currentDate string) (string, error) {
	parts := strings.Split(existingBuildID, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid build ID format: %s", existingBuildID)
	}

	existingDate := parts[0]
	existingRandom := parts[1]
	existingCounter, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", fmt.Errorf("invalid counter component: %s", parts[2])
	}

	if existingDate != currentDate {
		random, err := rand.Int(rand.Reader, big.NewInt(1000))
		if err != nil {
			return "", fmt.Errorf("failed to generate random number: %w", err)
		}
		return fmt.Sprintf("%s.%03d.1", currentDate, random.Int64()), nil
	}

	existingCounter++
	return fmt.Sprintf("%s.%s.%d", existingDate, existingRandom, existingCounter), nil
}

// ApplyConfigDefaults sets default configuration values based on context name, dev mode, and VM driver.
// It determines the appropriate default configuration (localhost, full, or standard) based on the VM driver
// and dev mode settings. For dev mode, it also sets the provider to "generic" if not already set.
// This method should be called before loading configuration from disk to ensure defaults are applied first.
// The context name is read from ctx.ContextName. Returns an error if any configuration operation fails.
func (ctx *ExecutionContext) ApplyConfigDefaults() error {
	contextName := ctx.ContextName
	if contextName == "" {
		contextName = "local"
	}

	if ctx.ConfigHandler == nil {
		return fmt.Errorf("config handler not available")
	}

	if !ctx.ConfigHandler.IsLoaded() {
		existingProvider := ctx.ConfigHandler.GetString("provider")
		contextName := ctx.ContextName
		if contextName == "" {
			contextName = "local"
		}
		isDevMode := ctx.ConfigHandler.IsDevMode(contextName)

		if isDevMode {
			if err := ctx.ConfigHandler.Set("dev", true); err != nil {
				return fmt.Errorf("failed to set dev mode: %w", err)
			}
		}

		vmDriver := ctx.ConfigHandler.GetString("vm.driver")
		if isDevMode && vmDriver == "" {
			switch runtime.GOOS {
			case "darwin", "windows":
				vmDriver = "docker-desktop"
			default:
				vmDriver = "docker"
			}
		}

		if vmDriver == "docker-desktop" {
			if err := ctx.ConfigHandler.SetDefault(config.DefaultConfig_Localhost); err != nil {
				return fmt.Errorf("failed to set default config: %w", err)
			}
		} else if isDevMode {
			if err := ctx.ConfigHandler.SetDefault(config.DefaultConfig_Full); err != nil {
				return fmt.Errorf("failed to set default config: %w", err)
			}
		} else {
			if err := ctx.ConfigHandler.SetDefault(config.DefaultConfig); err != nil {
				return fmt.Errorf("failed to set default config: %w", err)
			}
		}

		if isDevMode && ctx.ConfigHandler.GetString("vm.driver") == "" && vmDriver != "" {
			if err := ctx.ConfigHandler.Set("vm.driver", vmDriver); err != nil {
				return fmt.Errorf("failed to set vm.driver: %w", err)
			}
		}

		if existingProvider == "" && isDevMode {
			if err := ctx.ConfigHandler.Set("provider", "generic"); err != nil {
				return fmt.Errorf("failed to set provider from context name: %w", err)
			}
		}
	}

	return nil
}

// ApplyProviderDefaults sets provider-specific configuration values based on the provider type.
// For "aws", it enables AWS and sets the cluster driver to "eks".
// For "azure", it enables Azure and sets the cluster driver to "aks".
// For "generic", it sets the cluster driver to "talos".
// If no provider is set but dev mode is enabled, it defaults the cluster driver to "talos".
// The context name is read from ctx.ContextName. Returns an error if any configuration operation fails.
func (ctx *ExecutionContext) ApplyProviderDefaults(providerOverride string) error {
	if ctx.ConfigHandler == nil {
		return fmt.Errorf("config handler not available")
	}

	contextName := ctx.ContextName
	if contextName == "" {
		contextName = "local"
	}

	provider := providerOverride
	if provider == "" {
		provider = ctx.ConfigHandler.GetString("provider")
	}

	if provider != "" {
		switch provider {
		case "aws":
			if err := ctx.ConfigHandler.Set("aws.enabled", true); err != nil {
				return fmt.Errorf("failed to set aws.enabled: %w", err)
			}
			if err := ctx.ConfigHandler.Set("cluster.driver", "eks"); err != nil {
				return fmt.Errorf("failed to set cluster.driver: %w", err)
			}
		case "azure":
			if err := ctx.ConfigHandler.Set("azure.enabled", true); err != nil {
				return fmt.Errorf("failed to set azure.enabled: %w", err)
			}
			if err := ctx.ConfigHandler.Set("cluster.driver", "aks"); err != nil {
				return fmt.Errorf("failed to set cluster.driver: %w", err)
			}
		case "generic":
			if err := ctx.ConfigHandler.Set("cluster.driver", "talos"); err != nil {
				return fmt.Errorf("failed to set cluster.driver: %w", err)
			}
		}
	} else if ctx.ConfigHandler.IsDevMode(contextName) {
		if ctx.ConfigHandler.GetString("cluster.driver") == "" {
			if err := ctx.ConfigHandler.Set("cluster.driver", "talos"); err != nil {
				return fmt.Errorf("failed to set cluster.driver: %w", err)
			}
		}
	}

	return nil
}

// PrepareTools checks and installs required tools using the tools manager.
// It first checks that all required tools are installed and meet version requirements,
// then installs any missing or outdated tools. The tools manager must be available.
// Returns an error if the tools manager is not available or if checking or installation fails.
func (ctx *ExecutionContext) PrepareTools() error {
	if ctx.ToolsManager == nil {
		ctx.initializeToolsManager()
		if ctx.ToolsManager == nil {
			return fmt.Errorf("tools manager not available")
		}
		if err := ctx.ToolsManager.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize tools manager: %w", err)
		}
	}

	if err := ctx.ToolsManager.Check(); err != nil {
		return fmt.Errorf("error checking tools: %w", err)
	}
	if err := ctx.ToolsManager.Install(); err != nil {
		return fmt.Errorf("error installing tools: %w", err)
	}

	return nil
}
