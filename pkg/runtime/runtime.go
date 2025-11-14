package runtime

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

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/env"
	"github.com/windsorcli/cli/pkg/runtime/secrets"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// Runtime holds common execution values and core dependencies used across the Windsor CLI.
// These fields are set during various initialization steps rather than computed on-demand.
// Includes secret providers for Sops and 1Password, enabling access to secrets across all contexts.
// Also includes environment printers, tools manager, and environment variable/alias storage.
type Runtime struct {
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

// NewRuntime creates a new Runtime with ConfigHandler and Shell initialized if not already present.
// This is the base constructor that ensures core dependencies are available.
// If ConfigHandler is nil, it creates one using the Injector and initializes it.
// If Shell is nil, it creates one using the Injector and initializes it.
// Both are registered in the Injector for use by other components.
// The runtime also initializes envVars and aliases maps, and automatically sets up
// ContextName, ProjectRoot, ConfigRoot, and TemplateRoot based on the current project state.
// Returns the Runtime with initialized dependencies or an error if initialization fails.
func NewRuntime(rt *Runtime) (*Runtime, error) {
	if rt == nil {
		return nil, fmt.Errorf("execution context is required")
	}
	if rt.Injector == nil {
		return nil, fmt.Errorf("injector is required")
	}
	injector := rt.Injector

	if rt.Shell == nil {
		if existing := injector.Resolve("shell"); existing != nil {
			if shellInstance, ok := existing.(shell.Shell); ok {
				rt.Shell = shellInstance
			} else {
				shellInstance := shell.NewDefaultShell()
				rt.Shell = shellInstance
				injector.Register("shell", shellInstance)
			}
		} else {
			shellInstance := shell.NewDefaultShell()
			rt.Shell = shellInstance
			injector.Register("shell", shellInstance)
		}
	}

	projectRoot, err := rt.Shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to get project root: %w", err)
	}
	rt.ProjectRoot = projectRoot
	rt.ConfigRoot = filepath.Join(rt.ProjectRoot, "contexts", rt.ContextName)
	rt.TemplateRoot = filepath.Join(rt.ProjectRoot, "contexts", "_template")

	if rt.ConfigHandler == nil {
		if rt.Shell == nil {
			return nil, fmt.Errorf("shell is required to create config handler")
		}
		if existing := injector.Resolve("configHandler"); existing != nil {
			if configHandler, ok := existing.(config.ConfigHandler); ok {
				rt.ConfigHandler = configHandler
			} else {
				rt.ConfigHandler = config.NewConfigHandler(rt.Shell)
				injector.Register("configHandler", rt.ConfigHandler)
			}
		} else {
			rt.ConfigHandler = config.NewConfigHandler(rt.Shell)
			injector.Register("configHandler", rt.ConfigHandler)
		}
	}

	if rt.envVars == nil {
		rt.envVars = make(map[string]string)
	}
	if rt.aliases == nil {
		rt.aliases = make(map[string]string)
	}

	projectRoot, err = rt.Shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to get project root: %w", err)
	}

	contextName := rt.ConfigHandler.GetContext()
	rt.ContextName = contextName
	rt.ProjectRoot = projectRoot
	rt.ConfigRoot = filepath.Join(projectRoot, "contexts", contextName)
	rt.TemplateRoot = filepath.Join(projectRoot, "contexts", "_template")

	return rt, nil
}

// =============================================================================
// Public Methods
// =============================================================================

// HandleSessionReset checks for reset flags and session tokens, then resets managed environment
// variables if needed. It checks for WINDSOR_SESSION_TOKEN and uses the shell's CheckResetFlags
// method to determine if a reset should occur. If reset is needed, it calls Shell.Reset() and
// sets NO_CACHE=true. Returns an error if Shell is not initialized or if reset flag checking fails.
func (rt *Runtime) HandleSessionReset() error {
	if rt.Shell == nil {
		return fmt.Errorf("shell not initialized")
	}

	hasSessionToken := os.Getenv("WINDSOR_SESSION_TOKEN") != ""
	shouldReset, err := rt.Shell.CheckResetFlags()
	if err != nil {
		return fmt.Errorf("failed to check reset flags: %w", err)
	}
	if !hasSessionToken {
		shouldReset = true
	}

	if shouldReset {
		rt.Shell.Reset()
		if err := os.Setenv("NO_CACHE", "true"); err != nil {
			return fmt.Errorf("failed to set NO_CACHE: %w", err)
		}
	}

	return nil
}

// LoadEnvironment loads environment variables and aliases from all configured environment printers,
// then executes post-environment hooks. It initializes all necessary components, optionally loads
// secrets if requested, and aggregates all environment variables and aliases into the Runtime
// instance. Returns an error if any required dependency is missing or if any step fails. This method
// expects the ConfigHandler to be set before invocation.
func (rt *Runtime) LoadEnvironment(decrypt bool) error {
	if rt.ConfigHandler == nil {
		return fmt.Errorf("config handler not loaded")
	}

	rt.initializeEnvPrinters()
	rt.initializeToolsManager()
	rt.initializeSecretsProviders()

	if err := rt.initializeComponents(); err != nil {
		return fmt.Errorf("failed to initialize environment components: %w", err)
	}

	if decrypt {
		if err := rt.loadSecrets(); err != nil {
			return fmt.Errorf("failed to load secrets: %w", err)
		}
	}

	allEnvVars := make(map[string]string)
	allAliases := make(map[string]string)

	for _, printer := range rt.getAllEnvPrinters() {
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

	rt.envVars = allEnvVars
	rt.aliases = allAliases

	for key, value := range allEnvVars {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("error setting environment variable %s: %w", key, err)
		}
	}

	var firstError error
	for _, printer := range rt.getAllEnvPrinters() {
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
func (rt *Runtime) GetEnvVars() map[string]string {
	result := make(map[string]string)
	maps.Copy(result, rt.envVars)
	return result
}

// GetAliases returns a copy of the collected aliases.
func (rt *Runtime) GetAliases() map[string]string {
	result := make(map[string]string)
	maps.Copy(result, rt.aliases)
	return result
}

// CheckTools performs tool version checking using the tools manager.
// It validates that all required tools are installed and meet minimum version requirements.
// The tools manager must be initialized before calling this method. Returns an error if
// the tools manager is not available or if tool checking fails.
func (rt *Runtime) CheckTools() error {
	if rt.ToolsManager == nil {
		rt.initializeToolsManager()
		if rt.ToolsManager == nil {
			return fmt.Errorf("tools manager not available")
		}
	}

	if err := rt.ToolsManager.Check(); err != nil {
		return fmt.Errorf("error checking tools: %w", err)
	}

	return nil
}

// GetBuildID retrieves the current build ID from the .windsor/.build-id file.
// If no build ID exists, a new one is generated, persisted, and returned.
// Returns the build ID string or an error if retrieval or persistence fails.
func (rt *Runtime) GetBuildID() (string, error) {
	projectRoot := rt.ProjectRoot

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
		newBuildID, err := rt.generateBuildID()
		if err != nil {
			return "", fmt.Errorf("failed to generate build ID: %w", err)
		}
		if err := rt.writeBuildIDToFile(newBuildID); err != nil {
			return "", fmt.Errorf("failed to set build ID: %w", err)
		}
		return newBuildID, nil
	}

	return buildID, nil
}

// GenerateBuildID generates a new build ID and persists it to the .windsor/.build-id file,
// overwriting any existing value. Returns the new build ID or an error if generation or persistence fails.
func (rt *Runtime) GenerateBuildID() (string, error) {
	newBuildID, err := rt.generateBuildID()
	if err != nil {
		return "", fmt.Errorf("failed to generate build ID: %w", err)
	}

	if err := rt.writeBuildIDToFile(newBuildID); err != nil {
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
func (rt *Runtime) initializeEnvPrinters() {
	if rt.Shell == nil || rt.ConfigHandler == nil {
		return
	}

	if rt.EnvPrinters.AwsEnv == nil && rt.ConfigHandler.GetBool("aws.enabled", false) {
		rt.EnvPrinters.AwsEnv = env.NewAwsEnvPrinter(rt.Shell, rt.ConfigHandler)
		rt.Injector.Register("awsEnv", rt.EnvPrinters.AwsEnv)
	}
	if rt.EnvPrinters.AzureEnv == nil && rt.ConfigHandler.GetBool("azure.enabled", false) {
		rt.EnvPrinters.AzureEnv = env.NewAzureEnvPrinter(rt.Shell, rt.ConfigHandler)
		rt.Injector.Register("azureEnv", rt.EnvPrinters.AzureEnv)
	}
	if rt.EnvPrinters.DockerEnv == nil && rt.ConfigHandler.GetBool("docker.enabled", false) {
		if existingPrinter := rt.Injector.Resolve("dockerEnv"); existingPrinter != nil {
			if printer, ok := existingPrinter.(env.EnvPrinter); ok {
				rt.EnvPrinters.DockerEnv = printer
			}
		}
		if rt.EnvPrinters.DockerEnv == nil {
			rt.EnvPrinters.DockerEnv = env.NewDockerEnvPrinter(rt.Shell, rt.ConfigHandler)
			rt.Injector.Register("dockerEnv", rt.EnvPrinters.DockerEnv)
		}
	}
	if rt.EnvPrinters.KubeEnv == nil && rt.ConfigHandler.GetBool("cluster.enabled", false) {
		if existingPrinter := rt.Injector.Resolve("kubeEnv"); existingPrinter != nil {
			if printer, ok := existingPrinter.(env.EnvPrinter); ok {
				rt.EnvPrinters.KubeEnv = printer
			}
		}
		if rt.EnvPrinters.KubeEnv == nil {
			rt.EnvPrinters.KubeEnv = env.NewKubeEnvPrinter(rt.Shell, rt.ConfigHandler)
			rt.Injector.Register("kubeEnv", rt.EnvPrinters.KubeEnv)
		}
	}
	if rt.EnvPrinters.TalosEnv == nil &&
		(rt.ConfigHandler.GetString("cluster.driver", "") == "talos" ||
			rt.ConfigHandler.GetString("cluster.driver", "") == "omni") {
		if existingPrinter := rt.Injector.Resolve("talosEnv"); existingPrinter != nil {
			if printer, ok := existingPrinter.(env.EnvPrinter); ok {
				rt.EnvPrinters.TalosEnv = printer
			}
		}
		if rt.EnvPrinters.TalosEnv == nil {
			rt.EnvPrinters.TalosEnv = env.NewTalosEnvPrinter(rt.Shell, rt.ConfigHandler)
			rt.Injector.Register("talosEnv", rt.EnvPrinters.TalosEnv)
		}
	}
	if rt.EnvPrinters.TerraformEnv == nil && rt.ConfigHandler.GetBool("terraform.enabled", false) {
		if existingPrinter := rt.Injector.Resolve("terraformEnv"); existingPrinter != nil {
			if printer, ok := existingPrinter.(env.EnvPrinter); ok {
				rt.EnvPrinters.TerraformEnv = printer
			}
		}
		if rt.EnvPrinters.TerraformEnv == nil {
			rt.EnvPrinters.TerraformEnv = env.NewTerraformEnvPrinter(rt.Shell, rt.ConfigHandler)
			rt.Injector.Register("terraformEnv", rt.EnvPrinters.TerraformEnv)
		}
	}
	if rt.EnvPrinters.WindsorEnv == nil {
		if existingPrinter := rt.Injector.Resolve("windsorEnv"); existingPrinter != nil {
			if printer, ok := existingPrinter.(env.EnvPrinter); ok {
				rt.EnvPrinters.WindsorEnv = printer
			}
		}
		if rt.EnvPrinters.WindsorEnv == nil {
			secretsProviders := []secrets.SecretsProvider{}
			if rt.SecretsProviders.Sops != nil {
				secretsProviders = append(secretsProviders, rt.SecretsProviders.Sops)
			}
			if rt.SecretsProviders.Onepassword != nil {
				secretsProviders = append(secretsProviders, rt.SecretsProviders.Onepassword)
			}
			allEnvPrinters := rt.getAllEnvPrinters()
			rt.EnvPrinters.WindsorEnv = env.NewWindsorEnvPrinter(rt.Shell, rt.ConfigHandler, secretsProviders, allEnvPrinters)
			rt.Injector.Register("windsorEnv", rt.EnvPrinters.WindsorEnv)
		}
	}
}

// initializeToolsManager initializes the tools manager if not already set.
// It checks the injector for an existing tools manager first, and only creates a new one if not found.
// It creates a new ToolsManager instance and registers it with the dependency injector.
func (rt *Runtime) initializeToolsManager() {
	if rt.ToolsManager == nil {
		if existingManager := rt.Injector.Resolve("toolsManager"); existingManager != nil {
			if toolsManager, ok := existingManager.(tools.ToolsManager); ok {
				rt.ToolsManager = toolsManager
				return
			}
		}
		if rt.ConfigHandler != nil && rt.Shell != nil {
			rt.ToolsManager = tools.NewToolsManager(rt.ConfigHandler, rt.Shell)
			rt.Injector.Register("toolsManager", rt.ToolsManager)
		}
	}
}

// initializeSecretsProviders initializes and registers secrets providers with the dependency injector
// based on current configuration settings. The method sets up the SOPS provider if enabled with the
// context's config root path, and sets up the 1Password provider if enabled, using a mock in test
// scenarios. Providers are only initialized if not already present on the context.
func (rt *Runtime) initializeSecretsProviders() {
	if rt.SecretsProviders.Sops == nil && rt.ConfigHandler.GetBool("secrets.sops.enabled", false) {
		if existingProvider := rt.Injector.Resolve("sopsSecretsProvider"); existingProvider != nil {
			if provider, ok := existingProvider.(secrets.SecretsProvider); ok {
				rt.SecretsProviders.Sops = provider
			}
		}
		if rt.SecretsProviders.Sops == nil {
			configPath := rt.ConfigRoot
			rt.SecretsProviders.Sops = secrets.NewSopsSecretsProvider(configPath, rt.Injector)
			rt.Injector.Register("sopsSecretsProvider", rt.SecretsProviders.Sops)
		}
	}

	if rt.SecretsProviders.Onepassword == nil && rt.ConfigHandler.GetBool("secrets.onepassword.enabled", false) {
		if existingProvider := rt.Injector.Resolve("onepasswordSecretsProvider"); existingProvider != nil {
			if provider, ok := existingProvider.(secrets.SecretsProvider); ok {
				rt.SecretsProviders.Onepassword = provider
			}
		}
		if rt.SecretsProviders.Onepassword == nil {
			rt.SecretsProviders.Onepassword = secrets.NewMockSecretsProvider(rt.Injector)
			rt.Injector.Register("onepasswordSecretsProvider", rt.SecretsProviders.Onepassword)
		}
	}
}

// getAllEnvPrinters returns all environment printers in a consistent order.
// This ensures that environment variables are processed in a predictable sequence
// with WindsorEnv being processed last to take precedence.
func (rt *Runtime) getAllEnvPrinters() []env.EnvPrinter {
	return []env.EnvPrinter{
		rt.EnvPrinters.AwsEnv,
		rt.EnvPrinters.AzureEnv,
		rt.EnvPrinters.DockerEnv,
		rt.EnvPrinters.KubeEnv,
		rt.EnvPrinters.TalosEnv,
		rt.EnvPrinters.TerraformEnv,
		rt.EnvPrinters.WindsorEnv,
	}
}

// initializeComponents initializes all environment-related components required after setup.
// This method is a placeholder for any future initialization logic that may be needed.
// Returns nil as components are now fully initialized in their constructors.
func (rt *Runtime) initializeComponents() error {
	return nil
}

// loadSecrets loads secrets from configured secrets providers.
// It attempts to load secrets from both SOPS and 1Password providers if they are available.
func (rt *Runtime) loadSecrets() error {
	providers := []secrets.SecretsProvider{
		rt.SecretsProviders.Sops,
		rt.SecretsProviders.Onepassword,
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
func (rt *Runtime) writeBuildIDToFile(buildID string) error {
	projectRoot := rt.ProjectRoot

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
func (rt *Runtime) generateBuildID() (string, error) {
	now := time.Now()
	yy := now.Year() % 100
	mm := int(now.Month())
	dd := now.Day()
	datePart := fmt.Sprintf("%02d%02d%02d", yy, mm, dd)

	projectRoot := rt.ProjectRoot

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
		return rt.incrementBuildID(existingBuildID, datePart)
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
func (rt *Runtime) incrementBuildID(existingBuildID, currentDate string) (string, error) {
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
// The context name is read from rt.ContextName. Returns an error if any configuration operation fails.
func (rt *Runtime) ApplyConfigDefaults() error {
	contextName := rt.ContextName
	if contextName == "" {
		contextName = "local"
	}

	if rt.ConfigHandler == nil {
		return fmt.Errorf("config handler not available")
	}

	if !rt.ConfigHandler.IsLoaded() {
		existingProvider := rt.ConfigHandler.GetString("provider")
		contextName := rt.ContextName
		if contextName == "" {
			contextName = "local"
		}
		isDevMode := rt.ConfigHandler.IsDevMode(contextName)

		if isDevMode {
			if err := rt.ConfigHandler.Set("dev", true); err != nil {
				return fmt.Errorf("failed to set dev mode: %w", err)
			}
		}

		vmDriver := rt.ConfigHandler.GetString("vm.driver")
		if isDevMode && vmDriver == "" {
			switch runtime.GOOS {
			case "darwin", "windows":
				vmDriver = "docker-desktop"
			default:
				vmDriver = "docker"
			}
		}

		if vmDriver == "docker-desktop" {
			if err := rt.ConfigHandler.SetDefault(config.DefaultConfig_Localhost); err != nil {
				return fmt.Errorf("failed to set default config: %w", err)
			}
		} else if isDevMode {
			if err := rt.ConfigHandler.SetDefault(config.DefaultConfig_Full); err != nil {
				return fmt.Errorf("failed to set default config: %w", err)
			}
		} else {
			if err := rt.ConfigHandler.SetDefault(config.DefaultConfig); err != nil {
				return fmt.Errorf("failed to set default config: %w", err)
			}
		}

		if isDevMode && rt.ConfigHandler.GetString("vm.driver") == "" && vmDriver != "" {
			if err := rt.ConfigHandler.Set("vm.driver", vmDriver); err != nil {
				return fmt.Errorf("failed to set vm.driver: %w", err)
			}
		}

		if existingProvider == "" && isDevMode {
			if err := rt.ConfigHandler.Set("provider", "generic"); err != nil {
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
// The context name is read from rt.ContextName. Returns an error if any configuration operation fails.
func (rt *Runtime) ApplyProviderDefaults(providerOverride string) error {
	if rt.ConfigHandler == nil {
		return fmt.Errorf("config handler not available")
	}

	contextName := rt.ContextName
	if contextName == "" {
		contextName = "local"
	}

	provider := providerOverride
	if provider == "" {
		provider = rt.ConfigHandler.GetString("provider")
	}

	if provider != "" {
		switch provider {
		case "aws":
			if err := rt.ConfigHandler.Set("aws.enabled", true); err != nil {
				return fmt.Errorf("failed to set aws.enabled: %w", err)
			}
			if err := rt.ConfigHandler.Set("cluster.driver", "eks"); err != nil {
				return fmt.Errorf("failed to set cluster.driver: %w", err)
			}
		case "azure":
			if err := rt.ConfigHandler.Set("azure.enabled", true); err != nil {
				return fmt.Errorf("failed to set azure.enabled: %w", err)
			}
			if err := rt.ConfigHandler.Set("cluster.driver", "aks"); err != nil {
				return fmt.Errorf("failed to set cluster.driver: %w", err)
			}
		case "generic":
			if err := rt.ConfigHandler.Set("cluster.driver", "talos"); err != nil {
				return fmt.Errorf("failed to set cluster.driver: %w", err)
			}
		}
	} else if rt.ConfigHandler.IsDevMode(contextName) {
		if rt.ConfigHandler.GetString("cluster.driver") == "" {
			if err := rt.ConfigHandler.Set("cluster.driver", "talos"); err != nil {
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
func (rt *Runtime) PrepareTools() error {
	if rt.ToolsManager == nil {
		rt.initializeToolsManager()
		if rt.ToolsManager == nil {
			return fmt.Errorf("tools manager not available")
		}
	}

	if err := rt.ToolsManager.Check(); err != nil {
		return fmt.Errorf("error checking tools: %w", err)
	}
	if err := rt.ToolsManager.Install(); err != nil {
		return fmt.Errorf("error installing tools: %w", err)
	}

	return nil
}
