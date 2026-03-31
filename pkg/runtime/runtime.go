package runtime

import (
	"crypto/rand"
	"fmt"
	"maps"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/env"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	secretsRuntime "github.com/windsorcli/cli/pkg/runtime/secrets"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/terraform"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Types
// =============================================================================

// RuntimeSecretsProviders contains runtime-level secret provider dependencies.
type RuntimeSecretsProviders struct {
	Sops        secretsRuntime.SecretsProvider
	Onepassword []secretsRuntime.SecretsProvider
}

// RuntimeEnvPrinters contains runtime-level environment printer dependencies.
type RuntimeEnvPrinters struct {
	AwsEnv       env.EnvPrinter
	AzureEnv     env.EnvPrinter
	GcpEnv       env.EnvPrinter
	DockerEnv    env.EnvPrinter
	KubeEnv      env.EnvPrinter
	TalosEnv     env.EnvPrinter
	TerraformEnv env.EnvPrinter
	WindsorEnv   env.EnvPrinter
}

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

	// WindsorScratchPath is the windsor scratch directory (<projectRoot>/.windsor/contexts/<contextName>)
	WindsorScratchPath string

	// Core dependencies
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Evaluator     evaluator.ExpressionEvaluator

	// SecretsProviders contains providers for Sops and 1Password secrets management
	SecretsProviders RuntimeSecretsProviders

	// EnvPrinters contains environment printers for various providers and tools
	EnvPrinters RuntimeEnvPrinters

	// ToolsManager manages tool installation and configuration
	ToolsManager tools.ToolsManager

	// TerraformProvider provides Terraform-specific operations
	TerraformProvider terraform.TerraformProvider

	// envVars stores collected environment variables
	envVars map[string]string

	// aliases stores collected shell aliases
	aliases map[string]string

	// secretHelperRegistered tracks whether the secret helper has already been registered with the evaluator.
	secretHelperRegistered bool
	// secretsLoaded tracks whether configured secret providers have been loaded for this runtime lifecycle.
	secretsLoaded bool

	// secretCacheEnabled controls whether secret TF_VAR cache reuse is enabled for this runtime.
	secretCacheEnabled bool
}

// =============================================================================
// Constructor
// =============================================================================

// NewRuntime creates a new Runtime with ConfigHandler and Shell initialized if not already present.
// This is the base constructor that ensures core dependencies are available.
// If Shell is nil, it creates a new DefaultShell.
// If ConfigHandler is nil, it creates one using the Shell.
// The runtime also initializes envVars and aliases maps, and automatically sets up
// ContextName, ProjectRoot, ConfigRoot, and TemplateRoot based on the current project state.
// Optional overrides can be provided via opts to inject mocks for testing.
// Panics if Shell or ConfigHandler cannot be initialized.
func NewRuntime(opts ...*Runtime) *Runtime {
	rt := &Runtime{}

	if len(opts) > 0 && opts[0] != nil {
		overrides := opts[0]
		if overrides.Shell != nil {
			rt.Shell = overrides.Shell
		}
		if overrides.ConfigHandler != nil {
			rt.ConfigHandler = overrides.ConfigHandler
		}
		if overrides.Evaluator != nil {
			rt.Evaluator = overrides.Evaluator
		}
		if overrides.ContextName != "" {
			rt.ContextName = overrides.ContextName
		}
		if overrides.ProjectRoot != "" {
			rt.ProjectRoot = overrides.ProjectRoot
		}
		if overrides.ConfigRoot != "" {
			rt.ConfigRoot = overrides.ConfigRoot
		}
		if overrides.TemplateRoot != "" {
			rt.TemplateRoot = overrides.TemplateRoot
		}
		if overrides.WindsorScratchPath != "" {
			rt.WindsorScratchPath = overrides.WindsorScratchPath
		}
		rt.secretCacheEnabled = overrides.secretCacheEnabled
		if overrides.ToolsManager != nil {
			rt.ToolsManager = overrides.ToolsManager
		}
		if overrides.SecretsProviders.Sops != nil {
			rt.SecretsProviders.Sops = overrides.SecretsProviders.Sops
		}
		if overrides.SecretsProviders.Onepassword != nil {
			rt.SecretsProviders.Onepassword = overrides.SecretsProviders.Onepassword
		}
		if overrides.EnvPrinters.AwsEnv != nil {
			rt.EnvPrinters.AwsEnv = overrides.EnvPrinters.AwsEnv
		}
		if overrides.EnvPrinters.AzureEnv != nil {
			rt.EnvPrinters.AzureEnv = overrides.EnvPrinters.AzureEnv
		}
		if overrides.EnvPrinters.GcpEnv != nil {
			rt.EnvPrinters.GcpEnv = overrides.EnvPrinters.GcpEnv
		}
		if overrides.EnvPrinters.DockerEnv != nil {
			rt.EnvPrinters.DockerEnv = overrides.EnvPrinters.DockerEnv
		}
		if overrides.EnvPrinters.KubeEnv != nil {
			rt.EnvPrinters.KubeEnv = overrides.EnvPrinters.KubeEnv
		}
		if overrides.EnvPrinters.TalosEnv != nil {
			rt.EnvPrinters.TalosEnv = overrides.EnvPrinters.TalosEnv
		}
		if overrides.EnvPrinters.TerraformEnv != nil {
			rt.EnvPrinters.TerraformEnv = overrides.EnvPrinters.TerraformEnv
		}
		if overrides.EnvPrinters.WindsorEnv != nil {
			rt.EnvPrinters.WindsorEnv = overrides.EnvPrinters.WindsorEnv
		}
	}

	if rt.Shell == nil {
		rt.Shell = shell.NewDefaultShell()
	}

	if rt.ProjectRoot == "" {
		projectRoot, err := rt.Shell.GetProjectRoot()
		if err != nil {
			panic(fmt.Sprintf("failed to get project root: %v", err))
		}
		rt.ProjectRoot = projectRoot
	}

	if rt.ConfigHandler == nil {
		rt.ConfigHandler = config.NewConfigHandler(rt.Shell)
	}

	if rt.Shell == nil {
		panic("shell is required")
	}
	if rt.ConfigHandler == nil {
		panic("config handler is required")
	}

	if rt.envVars == nil {
		rt.envVars = make(map[string]string)
	}
	if rt.aliases == nil {
		rt.aliases = make(map[string]string)
	}

	contextName := rt.ConfigHandler.GetContext()
	if rt.ContextName == "" {
		rt.ContextName = contextName
	}
	if rt.ContextName == "" {
		rt.ContextName = "local"
	}

	if rt.ConfigRoot == "" {
		rt.ConfigRoot = filepath.Join(rt.ProjectRoot, "contexts", rt.ContextName)
	}
	if rt.TemplateRoot == "" {
		rt.TemplateRoot = filepath.Join(rt.ProjectRoot, "contexts", "_template")
	}

	if rt.Evaluator == nil {
		rt.Evaluator = evaluator.NewExpressionEvaluator(rt.ConfigHandler, rt.ProjectRoot, rt.TemplateRoot)
	}
	rt.registerSecretHelper()
	if rt.WindsorScratchPath == "" {
		rt.WindsorScratchPath = filepath.Join(rt.ProjectRoot, ".windsor", "contexts", rt.ContextName)
	}

	return rt
}

// =============================================================================
// Public Methods
// =============================================================================

// HandleSessionReset checks for reset flags and session tokens, then resets managed environment
// variables if needed. It checks for WINDSOR_SESSION_TOKEN and uses the shell's CheckResetFlags
// method to determine if a reset should occur. If reset is needed, it calls Shell.Reset() and
// sets NO_CACHE=true. Returns an error if reset flag checking fails.
func (rt *Runtime) HandleSessionReset() error {
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
		rt.secretsLoaded = false
		if err := os.Setenv("NO_CACHE", "true"); err != nil {
			return fmt.Errorf("failed to set NO_CACHE: %w", err)
		}
		if rt.TerraformProvider != nil {
			rt.TerraformProvider.ClearCache()
		}
	}

	return nil
}

// LoadEnvironment loads environment variables and aliases from all configured environment printers,
// then executes post-environment hooks. It initializes all necessary components, optionally loads
// secrets if requested, and aggregates all environment variables and aliases into the Runtime
// instance. Returns an error if any step fails.
func (rt *Runtime) LoadEnvironment(decrypt bool) error {
	rt.initializeSecretsProviders()
	rt.initializeEnvPrinters()
	rt.initializeToolsManager()

	if err := rt.InitializeComponents(); err != nil {
		return fmt.Errorf("failed to initialize environment components: %w", err)
	}

	if decrypt && len(rt.configuredSecretsProviders()) > 0 {
		if err := rt.loadSecrets(); err != nil {
			return fmt.Errorf("failed to load secrets: %w", err)
		}
	}

	allEnvVars := make(map[string]string)
	allAliases := make(map[string]string)
	managedEnv := make([]string, 0)
	managedAlias := make([]string, 0)

	for _, printer := range rt.getAllEnvPrinters() {
		if printer != nil {
			envVars, err := printer.GetEnvVars()
			if err != nil {
				return fmt.Errorf("error getting environment variables: %w", err)
			}
			maps.Copy(allEnvVars, envVars)
			managedEnv = appendManagedValues(managedEnv, envVars["WINDSOR_MANAGED_ENV"])
			managedAlias = appendManagedValues(managedAlias, envVars["WINDSOR_MANAGED_ALIAS"])

			aliases, err := printer.GetAlias()
			if err != nil {
				return fmt.Errorf("error getting aliases: %w", err)
			}
			maps.Copy(allAliases, aliases)
			managedEnv = appendUniqueStrings(managedEnv, printer.GetManagedEnv()...)
			managedAlias = appendUniqueStrings(managedAlias, printer.GetManagedAlias()...)
		}
	}
	allEnvVars["WINDSOR_MANAGED_ENV"] = strings.Join(managedEnv, ",")
	allEnvVars["WINDSOR_MANAGED_ALIAS"] = strings.Join(managedAlias, ",")

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

// SetSecretCacheEnabled enables or disables runtime secret cache reuse behavior.
func (rt *Runtime) SetSecretCacheEnabled(enabled bool) {
	rt.secretCacheEnabled = enabled
}

// IsSecretCacheEnabled reports whether runtime secret cache reuse behavior is enabled.
func (rt *Runtime) IsSecretCacheEnabled() bool {
	return rt.secretCacheEnabled
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
// This is a read-only operation - it never creates a build ID.
// Returns the build ID string if it exists, or empty string if it doesn't exist.
// Returns an error only if file read fails (not if file doesn't exist).
func (rt *Runtime) GetBuildID() (string, error) {
	projectRoot := rt.ProjectRoot

	if err := os.MkdirAll(projectRoot, 0750); err != nil {
		return "", fmt.Errorf("failed to create project root directory: %w", err)
	}

	root, err := os.OpenRoot(projectRoot)
	if err != nil {
		return "", fmt.Errorf("failed to open project root: %w", err)
	}
	if root == nil {
		return "", fmt.Errorf("failed to open project root: root is nil")
	}
	defer root.Close()

	buildIDPath := ".windsor/.build-id"

	if _, err := root.Stat(buildIDPath); os.IsNotExist(err) {
		return "", nil
	}

	data, err := root.ReadFile(buildIDPath)
	if err != nil {
		return "", fmt.Errorf("failed to read build ID file: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
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

// InitializeComponents initializes all environment-related components required after setup.
// Initializes TerraformProvider if terraform is enabled. Returns an error if initialization fails.
func (rt *Runtime) InitializeComponents() error {
	if rt.ConfigHandler != nil && rt.ConfigHandler.GetBool("terraform.enabled", true) {
		if rt.TerraformProvider == nil {
			rt.initializeToolsManager()
			rt.TerraformProvider = terraform.NewTerraformProvider(rt.ConfigHandler, rt.Shell, rt.ToolsManager, rt.Evaluator)
		}
	}
	return nil
}

// ApplyConfigDefaults applies base configuration defaults if no config is currently loaded.
// It sets "dev" mode in config if the context is a dev context, chooses a default workstation runtime
// (optionally honoring flagOverrides["workstation.runtime"]), and sets
// platform to "docker" in dev mode if not already set, or "incus" when overrides or config specify incus.
// After those, it loads a default configuration set, choosing among standard, full, localhost, or none
// defaults depending on platform, dev mode, and workstation runtime.
// This must be called before loading from disk to ensure proper defaulting. Returns error on config operation failure.
func (rt *Runtime) ApplyConfigDefaults(flagOverrides ...map[string]any) error {
	if !rt.ConfigHandler.IsLoaded() {
		existingPlatform := rt.ConfigHandler.GetString("platform")
		if existingPlatform == "" {
			existingPlatform = rt.ConfigHandler.GetString("provider")
		}
		isDevMode := rt.ConfigHandler.IsDevMode(rt.ContextName)

		if isDevMode {
			if err := rt.ConfigHandler.Set("dev", true); err != nil {
				return fmt.Errorf("failed to set dev mode: %w", err)
			}
		}

		workstationRuntime := rt.ConfigHandler.GetString("workstation.runtime")
		hadRuntime := workstationRuntime != ""
		if workstationRuntime == "" && len(flagOverrides) > 0 && flagOverrides[0] != nil {
			if driver, ok := flagOverrides[0]["workstation.runtime"].(string); ok && driver != "" {
				workstationRuntime = driver
			}
		}
		if isDevMode && workstationRuntime == "" {
			switch runtime.GOOS {
			case "darwin", "windows":
				workstationRuntime = "docker-desktop"
			default:
				workstationRuntime = "docker"
			}
		}

		if isDevMode && !hadRuntime && workstationRuntime != "" {
			if err := rt.ConfigHandler.Set("workstation.runtime", workstationRuntime); err != nil {
				return fmt.Errorf("failed to set workstation.runtime: %w", err)
			}
		}
		vmRuntime := ""
		if len(flagOverrides) > 0 && flagOverrides[0] != nil {
			if r, ok := flagOverrides[0]["vm.runtime"].(string); ok && r != "" {
				vmRuntime = r
			}
		}
		if vmRuntime == "" {
			vmRuntime = rt.ConfigHandler.GetString("vm.runtime", "docker")
		}

		if existingPlatform == "" && isDevMode {
			overridePlatform := ""
			if len(flagOverrides) > 0 && flagOverrides[0] != nil {
				if p, ok := flagOverrides[0]["platform"].(string); ok && p != "" {
					overridePlatform = p
				}
			}
			if overridePlatform != "" {
				if err := rt.ConfigHandler.Set("platform", overridePlatform); err != nil {
					return fmt.Errorf("failed to set platform from overrides: %w", err)
				}
			} else if workstationRuntime == "colima" && vmRuntime == "incus" {
				fmt.Fprintln(os.Stderr, "\033[33mWarning: vm.runtime is deprecated; use platform: incus in your context configuration instead. Support for vm.runtime will be removed in a future version.\033[0m")
				if err := rt.ConfigHandler.Set("platform", "incus"); err != nil {
					return fmt.Errorf("failed to set platform to incus: %w", err)
				}
			} else {
				if err := rt.ConfigHandler.Set("platform", "docker"); err != nil {
					return fmt.Errorf("failed to set platform: %w", err)
				}
			}
		}

		platform := rt.ConfigHandler.GetString("platform")
		if platform == "" {
			platform = rt.ConfigHandler.GetString("provider")
		}
		if platform == "none" {
			defaultConfig := config.DefaultConfig
			nonePlatform := "none"
			defaultConfig.Platform = &nonePlatform
			if err := rt.ConfigHandler.SetDefault(defaultConfig); err != nil {
				return fmt.Errorf("failed to set default config: %w", err)
			}
		} else if isDevMode {
			if err := rt.ConfigHandler.SetDefault(config.DefaultConfig_Dev); err != nil {
				return fmt.Errorf("failed to set default config: %w", err)
			}
		} else {
			if err := rt.ConfigHandler.SetDefault(config.DefaultConfig); err != nil {
				return fmt.Errorf("failed to set default config: %w", err)
			}
		}
	}

	return nil
}

// ResolveConfig runs the full config pipeline: infer platform for dev when missing, apply pre-load
// defaults, load from disk, apply flag overrides, then
// apply dev-mode platform normalization (colima+incus). Call this once to produce final config.
// Mutates flagOverrides when inferring platform. Returns error on config load or set failure.
func (rt *Runtime) ResolveConfig(flagOverrides map[string]any) error {
	if flagOverrides == nil {
		flagOverrides = make(map[string]any)
	}
	explicitPlatform, hasExplicitPlatform := explicitPlatformFromOverrides(flagOverrides)
	preLoadOverrides := maps.Clone(flagOverrides)
	if preLoadOverrides == nil {
		preLoadOverrides = make(map[string]any)
	}
	delete(preLoadOverrides, "provider")
	if hasExplicitPlatform {
		preLoadOverrides["platform"] = explicitPlatform
	} else {
		delete(preLoadOverrides, "platform")
	}
	inferredPlatform := rt.inferDevPlatformOverride(preLoadOverrides)
	if !hasExplicitPlatform && inferredPlatform != "" {
		preLoadOverrides["platform"] = inferredPlatform
	}
	if err := rt.ApplyConfigDefaults(preLoadOverrides); err != nil {
		return fmt.Errorf("failed to apply config defaults: %w", err)
	}
	if err := rt.ConfigHandler.LoadConfig(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	for key, value := range flagOverrides {
		if key == "provider" {
			continue
		}
		if key == "platform" && !hasExplicitPlatform {
			continue
		}
		if err := rt.ConfigHandler.Set(key, value); err != nil {
			return fmt.Errorf("failed to set %s: %w", key, err)
		}
	}
	if hasExplicitPlatform && explicitPlatform != "" {
		if err := rt.ConfigHandler.Set("platform", explicitPlatform); err != nil {
			return fmt.Errorf("failed to set platform: %w", err)
		}
	}
	rt.migrateLoadedConfig()
	dnsEnabled := rt.ConfigHandler.Get("dns.enabled")
	if (dnsEnabled == nil || dnsEnabled == true) && rt.ConfigHandler.GetString("workstation.runtime") != "" {
		if rt.ConfigHandler.GetString("dns.domain") == "" {
			_ = rt.ConfigHandler.Set("dns.domain", "test")
		}
		if rt.ConfigHandler.GetString("workstation.runtime") == "docker-desktop" && rt.ConfigHandler.GetString("workstation.dns.address") == "" {
			_ = rt.ConfigHandler.Set("workstation.dns.address", "127.0.0.1")
		}
	}
	if rt.ConfigHandler.IsDevMode(rt.ContextName) {
		platform := rt.ConfigHandler.GetString("platform")
		workstationRuntime := rt.ConfigHandler.GetString("workstation.runtime")
		vmRuntime := rt.ConfigHandler.GetString("vm.runtime", "docker")
		if (platform == "" || platform == "docker") && workstationRuntime == "colima" && vmRuntime == "incus" {
			fmt.Fprintln(os.Stderr, "\033[33mWarning: vm.runtime is deprecated; use platform: incus in your context configuration instead. Support for vm.runtime will be removed in a future version.\033[0m")
			if err := rt.ConfigHandler.Set("platform", "incus"); err != nil {
				return fmt.Errorf("failed to set platform to incus: %w", err)
			}
		}
	}
	return nil
}

// SaveConfig migrates provider→platform and clears the deprecated provider key before persistence.
// Delegates persistence to the config handler. Workstation-managed keys are written to
// .windsor/contexts/<context>/workstation.yaml and excluded from values.yaml.
func (rt *Runtime) SaveConfig(overwrite ...bool) error {
	if rt.ConfigHandler == nil {
		return fmt.Errorf("config handler not initialized")
	}
	if v := rt.ConfigHandler.GetString("provider"); v != "" {
		if rt.ConfigHandler.GetString("platform") == "" {
			_ = rt.ConfigHandler.Set("platform", v)
		}
		_ = rt.ConfigHandler.Set("provider", nil)
	}
	if err := rt.ConfigHandler.SaveWorkstationState(); err != nil {
		return fmt.Errorf("failed to save workstation state: %w", err)
	}
	return rt.ConfigHandler.SaveConfig(overwrite...)
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

// =============================================================================
// Private Methods
// =============================================================================

// explicitPlatformFromOverrides extracts an explicitly provided platform selection from CLI overrides.
// It treats both "platform" and legacy "provider" override keys as explicit platform intent.
func explicitPlatformFromOverrides(flagOverrides map[string]any) (string, bool) {
	if flagOverrides == nil {
		return "", false
	}
	if p, ok := flagOverrides["platform"]; ok {
		if s, ok := p.(string); ok && s != "" {
			return s, true
		}
		return "", true
	}
	if p, ok := flagOverrides["provider"]; ok {
		if s, ok := p.(string); ok && s != "" {
			return s, true
		}
		return "", true
	}
	return "", false
}

// canonicalPlatform returns the effective platform from config, preferring "platform" and
// falling back to legacy "provider" for backward compatibility.
func (rt *Runtime) canonicalPlatform() string {
	platform := rt.ConfigHandler.GetString("platform")
	if platform == "" {
		platform = rt.ConfigHandler.GetString("provider")
	}
	return platform
}

// inferDevPlatformOverride infers a dev-mode platform only when no explicit or configured platform exists.
// It preserves legacy colima+incus inference behavior and returns empty string when no inference is needed.
func (rt *Runtime) inferDevPlatformOverride(flagOverrides map[string]any) string {
	if !rt.ConfigHandler.IsDevMode(rt.ContextName) {
		return ""
	}
	if p, ok := explicitPlatformFromOverrides(flagOverrides); ok && p != "" {
		return ""
	}
	existingPlatform := rt.canonicalPlatform()
	if existingPlatform != "" {
		return ""
	}
	workstationRuntime := ""
	if driver, ok := flagOverrides["workstation.runtime"].(string); ok && driver != "" {
		workstationRuntime = driver
	}
	if workstationRuntime == "" {
		workstationRuntime = rt.ConfigHandler.GetString("workstation.runtime")
	}
	vmRuntime := ""
	if r, ok := flagOverrides["vm.runtime"].(string); ok {
		vmRuntime = r
	}
	if vmRuntime == "" {
		vmRuntime = rt.ConfigHandler.GetString("vm.runtime", "docker")
	}
	if workstationRuntime == "colima" && vmRuntime == "incus" {
		fmt.Fprintln(os.Stderr, "\033[33mWarning: vm.runtime is deprecated; use platform: incus in your context configuration instead. Support for vm.runtime will be removed in a future version.\033[0m")
		return "incus"
	}
	return "docker"
}

// initializeEnvPrinters initializes environment printers based on configuration settings.
// It creates and registers the appropriate environment printers with the dependency injector
// based on the current configuration state.
func (rt *Runtime) initializeEnvPrinters() {
	contextValues := map[string]any{}
	if values, err := rt.ConfigHandler.GetContextValues(); err == nil && values != nil {
		contextValues = values
	}

	clusterDriver := getNestedString(contextValues, "cluster", "driver")
	if clusterDriver == "" {
		clusterDriver = rt.ConfigHandler.GetString("cluster.driver", "")
	}
	awsEnabled := rt.ConfigHandler.GetBool("aws.enabled", false)
	azureEnabled := rt.ConfigHandler.GetBool("azure.enabled", false)
	gcpEnabled := rt.ConfigHandler.GetBool("gcp.enabled", false)
	clusterEnabled := clusterDriver != ""
	needsDocker := rt.needsDockerEnv()
	configData := rt.ConfigHandler.GetConfig()
	hasAWSConfig := configData != nil && configData.AWS != nil
	hasAzureConfig := configData != nil && configData.Azure != nil
	hasGCPConfig := configData != nil && configData.GCP != nil

	if rt.EnvPrinters.AwsEnv == nil && awsEnabled && hasAWSConfig {
		rt.EnvPrinters.AwsEnv = env.NewAwsEnvPrinter(rt.Shell, rt.ConfigHandler)
	}
	if rt.EnvPrinters.AzureEnv == nil && azureEnabled && hasAzureConfig {
		rt.EnvPrinters.AzureEnv = env.NewAzureEnvPrinter(rt.Shell, rt.ConfigHandler)
	}
	if rt.EnvPrinters.GcpEnv == nil && gcpEnabled && hasGCPConfig {
		rt.EnvPrinters.GcpEnv = env.NewGcpEnvPrinter(rt.Shell, rt.ConfigHandler)
	}
	if rt.EnvPrinters.DockerEnv == nil && needsDocker {
		rt.EnvPrinters.DockerEnv = env.NewVirtEnvPrinter(rt.Shell, rt.ConfigHandler)
	}
	if rt.EnvPrinters.KubeEnv == nil && clusterEnabled {
		rt.EnvPrinters.KubeEnv = env.NewKubeEnvPrinter(rt.Shell, rt.ConfigHandler)
	}
	if rt.EnvPrinters.TalosEnv == nil && clusterDriver == "talos" {
		rt.EnvPrinters.TalosEnv = env.NewTalosEnvPrinter(rt.Shell, rt.ConfigHandler)
	}
	if rt.EnvPrinters.TerraformEnv == nil && rt.ConfigHandler.GetBool("terraform.enabled", true) {
		rt.initializeToolsManager()
		if rt.TerraformProvider == nil {
			rt.TerraformProvider = terraform.NewTerraformProvider(rt.ConfigHandler, rt.Shell, rt.ToolsManager, rt.Evaluator)
		}
		rt.TerraformProvider.SetSecretCacheEnabled(rt.secretCacheEnabled)
		rt.EnvPrinters.TerraformEnv = env.NewTerraformEnvPrinter(rt.Shell, rt.ConfigHandler, rt.ToolsManager, rt.TerraformProvider)
	} else if rt.TerraformProvider != nil {
		rt.TerraformProvider.SetSecretCacheEnabled(rt.secretCacheEnabled)
	}
	if rt.EnvPrinters.WindsorEnv == nil {
		secretsProviders := []secretsRuntime.SecretsProvider{}
		if rt.SecretsProviders.Sops != nil {
			secretsProviders = append(secretsProviders, rt.SecretsProviders.Sops)
		}
		if rt.SecretsProviders.Onepassword != nil {
			secretsProviders = append(secretsProviders, rt.SecretsProviders.Onepassword...)
		}
		rt.EnvPrinters.WindsorEnv = env.NewWindsorEnvPrinter(
			rt.Shell,
			rt.ConfigHandler,
			secretsProviders,
			&env.WindsorEnvOptions{SecretCacheEnabled: rt.secretCacheEnabled},
		)
	}
}

// migrateLoadedConfig normalizes in-memory config after load by copying provider→platform when platform is empty.
// Call after LoadConfig only.
func (rt *Runtime) migrateLoadedConfig() {
	if rt.ConfigHandler.GetString("platform") == "" {
		if p := rt.ConfigHandler.GetString("provider"); p != "" {
			_ = rt.ConfigHandler.Set("platform", p)
		}
	}
}

// needsDockerEnv returns true when VirtEnvPrinter (DOCKER_HOST, DOCKER_CONFIG, etc.) should be used:
// platform must be "docker" (falls back to provider for legacy configs) and workstation.runtime must be colima, docker-desktop, or docker.
func (rt *Runtime) needsDockerEnv() bool {
	platform := rt.ConfigHandler.GetString("platform")
	if platform == "" {
		platform = rt.ConfigHandler.GetString("provider")
	}
	if platform != "docker" {
		return false
	}
	wsRuntime := rt.ConfigHandler.GetString("workstation.runtime")
	switch wsRuntime {
	case "colima", "docker-desktop", "docker":
		return true
	default:
		return false
	}
}

// initializeToolsManager initializes the tools manager if not already set.
// It creates a new ToolsManager instance if ConfigHandler and Shell are available.
func (rt *Runtime) initializeToolsManager() {
	if rt.ToolsManager == nil {
		if rt.ConfigHandler != nil && rt.Shell != nil {
			rt.ToolsManager = tools.NewToolsManager(rt.ConfigHandler, rt.Shell)
		}
	}
}

func getNestedString(values map[string]any, path ...string) string {
	current := any(values)
	for _, key := range path {
		next, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		val, exists := next[key]
		if !exists {
			return ""
		}
		current = val
	}
	str, _ := current.(string)
	return str
}

// initializeSecretsProviders initializes secrets providers based on current configuration settings.
// The method sets up the SOPS provider if enabled with the context's config root path, and sets up
// 1Password providers for each configured vault. Providers are only initialized if not already present.
func (rt *Runtime) initializeSecretsProviders() {
	if rt.SecretsProviders.Sops == nil && rt.ConfigHandler.GetBool("secrets.sops.enabled", false) {
		configPath := rt.ConfigRoot
		rt.SecretsProviders.Sops = secretsRuntime.NewSopsSecretsProvider(configPath, rt.Shell)
	}

	if rt.SecretsProviders.Onepassword == nil {
		vaultsValue := rt.ConfigHandler.Get("secrets.onepassword.vaults")
		if vaultsValue != nil {
			if vaultsMap, ok := vaultsValue.(map[string]any); ok {
				rt.SecretsProviders.Onepassword = []secretsRuntime.SecretsProvider{}
				vaultIDs := make([]string, 0, len(vaultsMap))
				for vaultID := range vaultsMap {
					vaultIDs = append(vaultIDs, vaultID)
				}
				sort.Strings(vaultIDs)
				for _, vaultID := range vaultIDs {
					vaultData := vaultsMap[vaultID]
					if vaultMap, ok := vaultData.(map[string]any); ok {
						vault := secretsConfigType.OnePasswordVault{
							ID: vaultID,
						}
						if url, ok := vaultMap["url"].(string); ok {
							vault.URL = url
						}
						if name, ok := vaultMap["name"].(string); ok {
							vault.Name = name
						}
						if id, ok := vaultMap["id"].(string); ok && id != "" {
							vault.ID = id
						}

						var provider secretsRuntime.SecretsProvider
						if os.Getenv("OP_SERVICE_ACCOUNT_TOKEN") != "" {
							provider = secretsRuntime.NewOnePasswordSDKSecretsProvider(vault, rt.Shell)
						} else {
							provider = secretsRuntime.NewOnePasswordCLISecretsProvider(vault, rt.Shell)
						}
						rt.SecretsProviders.Onepassword = append(rt.SecretsProviders.Onepassword, provider)
					}
				}
			}
		}
	}
	rt.registerSecretHelper()
}

// registerSecretHelper registers the secret() expression helper once for this runtime.
func (rt *Runtime) registerSecretHelper() {
	if rt.secretHelperRegistered || rt.Evaluator == nil {
		return
	}
	secretsRuntime.RegisterSecretHelper(rt.Evaluator, rt.resolveSecretReference)
	rt.secretHelperRegistered = true
}

// resolveSecretReference resolves a single secret reference using configured secret providers.
func (rt *Runtime) resolveSecretReference(ref string) (string, error) {
	rt.initializeSecretsProviders()
	if err := rt.loadSecrets(); err != nil {
		return "", err
	}
	return secretsRuntime.ResolveReference(ref, rt.configuredSecretsProviders())
}

// getAllEnvPrinters returns all environment printers in a consistent order.
// This ensures that environment variables are processed in a predictable sequence
// with WindsorEnv being processed last to take precedence.
func (rt *Runtime) getAllEnvPrinters() []env.EnvPrinter {
	return []env.EnvPrinter{
		rt.EnvPrinters.AwsEnv,
		rt.EnvPrinters.AzureEnv,
		rt.EnvPrinters.GcpEnv,
		rt.EnvPrinters.DockerEnv,
		rt.EnvPrinters.KubeEnv,
		rt.EnvPrinters.TalosEnv,
		rt.EnvPrinters.TerraformEnv,
		rt.EnvPrinters.WindsorEnv,
	}
}

// loadSecrets loads secrets from configured secrets providers.
// It attempts to load secrets from both SOPS and 1Password providers if they are available.
// If a provider fails to load (e.g., file not found), it continues with other providers.
// Returns an error only if a provider encounters a non-recoverable error (e.g., decryption failure).
func (rt *Runtime) loadSecrets() error {
	if rt.secretsLoaded {
		return nil
	}
	for _, provider := range rt.configuredSecretsProviders() {
		if provider != nil {
			if err := provider.LoadSecrets(); err != nil {
				return fmt.Errorf("failed to load secrets: %w", err)
			}
		}
	}
	rt.secretsLoaded = true
	return nil
}

// configuredSecretsProviders returns all active secret providers in deterministic order.
func (rt *Runtime) configuredSecretsProviders() []secretsRuntime.SecretsProvider {
	providers := make([]secretsRuntime.SecretsProvider, 0)
	if rt.SecretsProviders.Sops != nil {
		providers = append(providers, rt.SecretsProviders.Sops)
	}
	if len(rt.SecretsProviders.Onepassword) > 0 {
		providers = append(providers, rt.SecretsProviders.Onepassword...)
	}
	return providers
}

// appendUniqueStrings appends non-empty values to base while preserving first-seen order and avoiding duplicates.
func appendUniqueStrings(base []string, values ...string) []string {
	seen := make(map[string]struct{}, len(base))
	for _, value := range base {
		seen[value] = struct{}{}
	}
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		base = append(base, value)
	}
	return base
}

// appendManagedValues appends comma-separated managed names while preserving first-seen order.
func appendManagedValues(base []string, csvValues string) []string {
	if strings.TrimSpace(csvValues) == "" {
		return base
	}
	parts := strings.Split(csvValues, ",")
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized = append(normalized, strings.TrimSpace(part))
	}
	return appendUniqueStrings(base, normalized...)
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
	if root == nil {
		return fmt.Errorf("failed to open project root: root is nil")
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
	if root == nil {
		return "", fmt.Errorf("failed to open project root: root is nil")
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
