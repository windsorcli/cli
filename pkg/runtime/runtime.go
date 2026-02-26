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

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/env"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	secretsRuntime "github.com/windsorcli/cli/pkg/runtime/secrets"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/terraform"
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

	// WindsorScratchPath is the windsor scratch directory (<projectRoot>/.windsor/contexts/<contextName>)
	WindsorScratchPath string

	// Core dependencies
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Evaluator     evaluator.ExpressionEvaluator

	// SecretsProviders contains providers for Sops and 1Password secrets management
	SecretsProviders struct {
		Sops        secretsRuntime.SecretsProvider
		Onepassword []secretsRuntime.SecretsProvider
	}

	// EnvPrinters contains environment printers for various providers and tools
	EnvPrinters struct {
		AwsEnv       env.EnvPrinter
		AzureEnv     env.EnvPrinter
		GcpEnv       env.EnvPrinter
		DockerEnv    env.EnvPrinter
		KubeEnv      env.EnvPrinter
		TalosEnv     env.EnvPrinter
		TerraformEnv env.EnvPrinter
		WindsorEnv   env.EnvPrinter
	}

	// ToolsManager manages tool installation and configuration
	ToolsManager tools.ToolsManager

	// TerraformProvider provides Terraform-specific operations
	TerraformProvider terraform.TerraformProvider

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
	if rt.ConfigHandler != nil && rt.ConfigHandler.GetBool("terraform.enabled", false) {
		if rt.TerraformProvider == nil {
			rt.initializeToolsManager()
			rt.TerraformProvider = terraform.NewTerraformProvider(rt.ConfigHandler, rt.Shell, rt.ToolsManager, rt.Evaluator)
		}
	}
	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// initializeEnvPrinters initializes environment printers based on configuration settings.
// It creates and registers the appropriate environment printers with the dependency injector
// based on the current configuration state.
func (rt *Runtime) initializeEnvPrinters() {
	if rt.EnvPrinters.AwsEnv == nil && rt.ConfigHandler.GetBool("aws.enabled", false) {
		rt.EnvPrinters.AwsEnv = env.NewAwsEnvPrinter(rt.Shell, rt.ConfigHandler)
	}
	if rt.EnvPrinters.AzureEnv == nil && rt.ConfigHandler.GetBool("azure.enabled", false) {
		rt.EnvPrinters.AzureEnv = env.NewAzureEnvPrinter(rt.Shell, rt.ConfigHandler)
	}
	if rt.EnvPrinters.GcpEnv == nil && rt.ConfigHandler.GetBool("gcp.enabled", false) {
		rt.EnvPrinters.GcpEnv = env.NewGcpEnvPrinter(rt.Shell, rt.ConfigHandler)
	}
	if rt.EnvPrinters.DockerEnv == nil && rt.needsDockerEnv() {
		rt.EnvPrinters.DockerEnv = env.NewVirtEnvPrinter(rt.Shell, rt.ConfigHandler)
	}
	if rt.EnvPrinters.KubeEnv == nil && rt.ConfigHandler.GetBool("cluster.enabled", false) {
		rt.EnvPrinters.KubeEnv = env.NewKubeEnvPrinter(rt.Shell, rt.ConfigHandler)
	}
	if rt.EnvPrinters.TalosEnv == nil &&
		(rt.ConfigHandler.GetString("cluster.driver", "") == "talos" ||
			rt.ConfigHandler.GetString("cluster.driver", "") == "omni") {
		rt.EnvPrinters.TalosEnv = env.NewTalosEnvPrinter(rt.Shell, rt.ConfigHandler)
	}
	if rt.EnvPrinters.TerraformEnv == nil && rt.ConfigHandler.GetBool("terraform.enabled", false) {
		rt.initializeToolsManager()
		if rt.TerraformProvider == nil {
			rt.TerraformProvider = terraform.NewTerraformProvider(rt.ConfigHandler, rt.Shell, rt.ToolsManager, rt.Evaluator)
		}
		rt.EnvPrinters.TerraformEnv = env.NewTerraformEnvPrinter(rt.Shell, rt.ConfigHandler, rt.ToolsManager, rt.TerraformProvider)
	}
	if rt.EnvPrinters.WindsorEnv == nil {
		secretsProviders := []secretsRuntime.SecretsProvider{}
		if rt.SecretsProviders.Sops != nil {
			secretsProviders = append(secretsProviders, rt.SecretsProviders.Sops)
		}
		if rt.SecretsProviders.Onepassword != nil {
			secretsProviders = append(secretsProviders, rt.SecretsProviders.Onepassword...)
		}
		allEnvPrinters := rt.getAllEnvPrinters()
		rt.EnvPrinters.WindsorEnv = env.NewWindsorEnvPrinter(rt.Shell, rt.ConfigHandler, secretsProviders, allEnvPrinters)
	}
}

// needsDockerEnv returns true when VirtEnvPrinter (DOCKER_HOST, DOCKER_CONFIG, etc.) should be used:
// either the internal compose workstation or any Docker-based workstation runtime (colima, docker-desktop, docker).
func (rt *Runtime) needsDockerEnv() bool {
	if rt.ConfigHandler.GetString("provider") != "docker" {
		return false
	}
	runtime := rt.ConfigHandler.GetString("workstation.runtime")
	if runtime == "" {
		runtime = rt.ConfigHandler.GetString("vm.driver")
	}
	switch runtime {
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
				for vaultID, vaultData := range vaultsMap {
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
	providers := []secretsRuntime.SecretsProvider{}
	if rt.SecretsProviders.Sops != nil {
		providers = append(providers, rt.SecretsProviders.Sops)
	}
	if rt.SecretsProviders.Onepassword != nil {
		providers = append(providers, rt.SecretsProviders.Onepassword...)
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

// ApplyConfigDefaults applies base configuration defaults if no config is currently loaded.
// It sets "dev" mode in config if the context is a dev context, chooses a default workstation runtime
// (optionally honoring flagOverrides["workstation.runtime"] or deprecated flagOverrides["vm.driver"]), and sets
// provider to "docker" in dev mode if not already set, or "incus" when overrides or config specify provider incus.
// After those, it loads a default configuration set, choosing among standard, full, localhost, or none
// defaults depending on provider, dev mode, and workstation runtime.
// This must be called before loading from disk to ensure proper defaulting. Returns error on config operation failure.
func (rt *Runtime) ApplyConfigDefaults(flagOverrides ...map[string]any) error {
	if !rt.ConfigHandler.IsLoaded() {
		existingProvider := rt.ConfigHandler.GetString("provider")
		isDevMode := rt.ConfigHandler.IsDevMode(rt.ContextName)

		if isDevMode {
			if err := rt.ConfigHandler.Set("dev", true); err != nil {
				return fmt.Errorf("failed to set dev mode: %w", err)
			}
		}

		workstationRuntime := rt.ConfigHandler.GetString("workstation.runtime")
		if workstationRuntime == "" {
			workstationRuntime = rt.ConfigHandler.GetString("vm.driver")
		}
		hadRuntime := workstationRuntime != ""
		if workstationRuntime == "" && len(flagOverrides) > 0 && flagOverrides[0] != nil {
			if driver, ok := flagOverrides[0]["workstation.runtime"].(string); ok && driver != "" {
				workstationRuntime = driver
			}
			if workstationRuntime == "" {
				if driver, ok := flagOverrides[0]["vm.driver"].(string); ok && driver != "" {
					workstationRuntime = driver
				}
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
			if err := rt.ConfigHandler.Set("vm.driver", workstationRuntime); err != nil {
				return fmt.Errorf("failed to set vm.driver: %w", err)
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

		if existingProvider == "" && isDevMode {
			overrideProvider := ""
			if len(flagOverrides) > 0 && flagOverrides[0] != nil {
				if p, ok := flagOverrides[0]["provider"].(string); ok && p != "" {
					overrideProvider = p
				}
			}
			if overrideProvider != "" {
				if err := rt.ConfigHandler.Set("provider", overrideProvider); err != nil {
					return fmt.Errorf("failed to set provider from overrides: %w", err)
				}
			} else if workstationRuntime == "colima" && vmRuntime == "incus" {
				fmt.Fprintln(os.Stderr, "\033[33mWarning: vm.runtime is deprecated; use platform: incus in your context configuration instead. Support for vm.runtime will be removed in a future version.\033[0m")
				if err := rt.ConfigHandler.Set("provider", "incus"); err != nil {
					return fmt.Errorf("failed to set provider to incus: %w", err)
				}
			} else {
				if err := rt.ConfigHandler.Set("provider", "docker"); err != nil {
					return fmt.Errorf("failed to set provider from context name: %w", err)
				}
			}
		}

		if isDevMode {
			if err := rt.ConfigHandler.Set("workstation.enabled", true); err != nil {
				return fmt.Errorf("failed to set workstation.enabled for dev context: %w", err)
			}
		}

		provider := rt.ConfigHandler.GetString("provider")
		if provider == "none" {
			defaultConfig := config.DefaultConfig
			noneProvider := "none"
			defaultConfig.Provider = &noneProvider
			if err := rt.ConfigHandler.SetDefault(defaultConfig); err != nil {
				return fmt.Errorf("failed to set default config: %w", err)
			}
		} else if workstationRuntime == "docker-desktop" {
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
	}

	return nil
}

// ResolveConfig runs the full config pipeline: infer provider for dev when missing, apply pre-load
// defaults, load from disk, migrate vm.driver to workstation.runtime, apply flag overrides, then
// apply dev-mode provider normalization (colima+incus). Call this once to produce final config.
// Mutates flagOverrides when inferring provider. Returns error on config load or set failure.
func (rt *Runtime) ResolveConfig(flagOverrides map[string]any) error {
	if flagOverrides == nil {
		flagOverrides = make(map[string]any)
	}
	if p, ok := flagOverrides["platform"]; ok {
		flagOverrides["provider"] = p
		delete(flagOverrides, "platform")
	}
	isDevMode := rt.ConfigHandler.IsDevMode(rt.ContextName)
	if isDevMode {
		if _, exists := flagOverrides["provider"]; !exists && rt.ConfigHandler.GetString("provider") == "" {
			workstationRuntime := ""
			if driver, ok := flagOverrides["workstation.runtime"].(string); ok && driver != "" {
				workstationRuntime = driver
			}
			if workstationRuntime == "" {
				if driver, ok := flagOverrides["vm.driver"].(string); ok {
					workstationRuntime = driver
				}
			}
			if workstationRuntime == "" {
				workstationRuntime = rt.ConfigHandler.GetString("workstation.runtime")
			}
			if workstationRuntime == "" {
				workstationRuntime = rt.ConfigHandler.GetString("vm.driver")
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
				flagOverrides["provider"] = "incus"
			} else {
				flagOverrides["provider"] = "docker"
			}
		}
	}
	if err := rt.ApplyConfigDefaults(flagOverrides); err != nil {
		return fmt.Errorf("failed to apply config defaults: %w", err)
	}
	providerOverride := ""
	if prov, ok := flagOverrides["provider"].(string); ok {
		providerOverride = prov
	}
	if err := rt.ApplyProviderDefaults(providerOverride); err != nil {
		return err
	}
	if err := rt.ConfigHandler.LoadConfig(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if p := rt.ConfigHandler.GetString("platform"); p != "" {
		_ = rt.ConfigHandler.Set("provider", p)
	}
	if rt.ConfigHandler.GetString("workstation.runtime") == "" && rt.ConfigHandler.GetString("vm.driver") != "" {
		_ = rt.ConfigHandler.Set("workstation.runtime", rt.ConfigHandler.GetString("vm.driver"))
	}
	if rt.ConfigHandler.GetString("workstation.address") == "" && rt.ConfigHandler.GetString("vm.address") != "" {
		_ = rt.ConfigHandler.Set("workstation.address", rt.ConfigHandler.GetString("vm.address"))
	}
	if rt.ConfigHandler.GetBool("dns.enabled") && rt.ConfigHandler.GetString("dns.domain") != "" && rt.ConfigHandler.GetString("workstation.runtime") == "docker-desktop" && rt.ConfigHandler.GetString("dns.address") == "" {
		_ = rt.ConfigHandler.Set("dns.address", "127.0.0.1")
	}
	for key, value := range flagOverrides {
		if err := rt.ConfigHandler.Set(key, value); err != nil {
			return fmt.Errorf("failed to set %s: %w", key, err)
		}
	}
	if isDevMode {
		provider := rt.ConfigHandler.GetString("provider")
		workstationRuntime := rt.ConfigHandler.GetString("workstation.runtime")
		if workstationRuntime == "" {
			workstationRuntime = rt.ConfigHandler.GetString("vm.driver")
		}
		vmRuntime := rt.ConfigHandler.GetString("vm.runtime", "docker")
		if (provider == "" || provider == "docker") && workstationRuntime == "colima" && vmRuntime == "incus" {
			fmt.Fprintln(os.Stderr, "\033[33mWarning: vm.runtime is deprecated; use platform: incus in your context configuration instead. Support for vm.runtime will be removed in a future version.\033[0m")
			if err := rt.ConfigHandler.Set("provider", "incus"); err != nil {
				return fmt.Errorf("failed to set provider to incus: %w", err)
			}
		}
	}
	return nil
}

// ApplyProviderDefaults sets provider-specific configuration values based on the provider type.
// For "aws", it enables AWS and sets the cluster driver to "eks".
// For "azure", it enables Azure and sets the cluster driver to "aks".
// For "gcp", it enables GCP and sets the cluster driver to "gke".
// For "docker", it sets the cluster driver to "talos".
// For "metal", it sets the cluster driver to "talos".
// For "incus", it sets the cluster driver to "talos".
// If no provider is set but dev mode is enabled, it defaults the cluster driver to "talos".
// The context name is read from rt.ContextName. Returns an error if any configuration operation fails.
func (rt *Runtime) ApplyProviderDefaults(providerOverride string) error {
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
		case "gcp":
			if err := rt.ConfigHandler.Set("gcp.enabled", true); err != nil {
				return fmt.Errorf("failed to set gcp.enabled: %w", err)
			}
			if err := rt.ConfigHandler.Set("cluster.driver", "gke"); err != nil {
				return fmt.Errorf("failed to set cluster.driver: %w", err)
			}
		case "docker":
			if err := rt.ConfigHandler.Set("cluster.driver", "talos"); err != nil {
				return fmt.Errorf("failed to set cluster.driver: %w", err)
			}
		case "metal":
			if err := rt.ConfigHandler.Set("cluster.driver", "talos"); err != nil {
				return fmt.Errorf("failed to set cluster.driver: %w", err)
			}
		case "incus":
			if err := rt.ConfigHandler.Set("cluster.driver", "talos"); err != nil {
				return fmt.Errorf("failed to set cluster.driver: %w", err)
			}
		}
	} else if rt.ConfigHandler.IsDevMode(rt.ContextName) {
		if rt.ConfigHandler.GetString("cluster.driver") == "" {
			if err := rt.ConfigHandler.Set("cluster.driver", "talos"); err != nil {
				return fmt.Errorf("failed to set cluster.driver: %w", err)
			}
		}
	}

	return nil
}

// SaveConfig normalizes deprecated keys before persisting: provider→platform, workstation.runtime→vm.driver.
// Copies provider to platform only when platform is not already set, so a second SaveConfig does not
// overwrite platform with the schema default (e.g. "none") when provider has been cleared from data.
func (rt *Runtime) SaveConfig(overwrite ...bool) error {
	if rt.ConfigHandler == nil {
		return fmt.Errorf("config handler not initialized")
	}
	if v := rt.ConfigHandler.GetString("provider"); v != "" {
		if rt.ConfigHandler.GetString("platform") == "" {
			_ = rt.ConfigHandler.Set("platform", v)
			_ = rt.ConfigHandler.Set("provider", v)
		}
	}
	if v := rt.ConfigHandler.GetString("workstation.runtime"); v != "" {
		_ = rt.ConfigHandler.Set("vm.driver", v)
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
