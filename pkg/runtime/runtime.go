// The Runtime is the shared execution state of a Windsor command.
// It provides the context paths, the config handler, the shell, and the expression evaluator.
// It creates the tools manager, the secrets resolver, and the Terraform provider when a command needs them.
// NewRuntime sets each value that the caller does not supply.

package runtime

import (
	"fmt"
	"path/filepath"

	"github.com/windsorcli/cli/internal/werror"
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

	// Global indicates the runtime is operating in global mode, where no windsor.yaml
	// project was found and $HOME/.config/windsor is used as the effective project root.
	Global bool

	// Core dependencies
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Evaluator     evaluator.ExpressionEvaluator

	// Resolver dispatches secret references across configured providers
	Resolver *secretsRuntime.Resolver

	// EnvPrinters contains environment printers for various providers and tools
	EnvPrinters struct {
		DotEnvEnv    env.EnvPrinter
		AwsEnv       env.EnvPrinter
		AzureEnv     env.EnvPrinter
		GcpEnv       env.EnvPrinter
		VsphereEnv   env.EnvPrinter
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

	// Shims holds the system calls that tests replace
	Shims *Shims

	// envVars stores collected environment variables
	envVars map[string]string

	// aliases stores collected shell aliases
	aliases map[string]string

	// secretHelperRegistered tracks whether the evaluator secret helper was registered.
	secretHelperRegistered bool
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
		if overrides.Global {
			rt.Global = true
		}
		if overrides.ToolsManager != nil {
			rt.ToolsManager = overrides.ToolsManager
		}
		if overrides.Resolver != nil {
			rt.Resolver = overrides.Resolver
		}
		if overrides.Shims != nil {
			rt.Shims = overrides.Shims
		}
		if overrides.EnvPrinters.DotEnvEnv != nil {
			rt.EnvPrinters.DotEnvEnv = overrides.EnvPrinters.DotEnvEnv
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
		if overrides.EnvPrinters.VsphereEnv != nil {
			rt.EnvPrinters.VsphereEnv = overrides.EnvPrinters.VsphereEnv
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

	if rt.Shims == nil {
		rt.Shims = NewShims()
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
		if !rt.Global {
			rt.Global = rt.Shell.IsGlobal()
		}
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

// InitializeComponents initializes all environment-related components required after setup.
// Initializes TerraformProvider if terraform is enabled. Returns an error if initialization fails.
func (rt *Runtime) InitializeComponents() error {
	rt.initializeSecretsProviders()

	if rt.ConfigHandler != nil && rt.ConfigHandler.GetBool("terraform.enabled", true) {
		if rt.TerraformProvider == nil {
			rt.initializeToolsManager()
			rt.TerraformProvider = terraform.NewTerraformProvider(rt.ConfigHandler, rt.Shell, rt.ToolsManager, rt.Evaluator)
		}
	}
	return nil
}

// CheckTools performs tool version checking using the tools manager against the full
// AllRequirements set. Equivalent to CheckToolsFor(tools.AllRequirements()) and exists for
// backward compatibility plus the explicit "verify everything" command (`windsor check`).
// New callers should declare per-command requirements via CheckToolsFor.
func (rt *Runtime) CheckTools() error {
	return rt.CheckToolsFor(tools.AllRequirements())
}

// CheckToolsFor checks the versions of the tool families in reqs.
// Each check skips a tool that the config does not use, such as Docker in a context without docker.
// It creates the tools manager if necessary, and returns an error if it cannot.
func (rt *Runtime) CheckToolsFor(reqs tools.Requirements) error {
	if rt.ToolsManager == nil {
		rt.initializeToolsManager()
		if rt.ToolsManager == nil {
			return fmt.Errorf("tools manager not available")
		}
	}

	if err := rt.ToolsManager.CheckRequirements(reqs); err != nil {
		return werror.Wrap(err, "error checking tools")
	}

	return nil
}

// PrepareTools checks and installs every tool family. Equivalent to PrepareToolsFor with
// AllRequirements; preserved for backward compatibility. New callers should narrow their
// requirements via PrepareToolsFor so commands like `windsor init` don't trip checks they
// won't actually exercise.
func (rt *Runtime) PrepareTools() error {
	return rt.PrepareToolsFor(tools.AllRequirements())
}

// PrepareToolsFor checks the tool families in reqs, then installs the tools that are missing or out of date.
// Each check skips a tool that the config does not use.
// It creates the tools manager if necessary, and returns an error if it cannot.
func (rt *Runtime) PrepareToolsFor(reqs tools.Requirements) error {
	if rt.ToolsManager == nil {
		rt.initializeToolsManager()
		if rt.ToolsManager == nil {
			return fmt.Errorf("tools manager not available")
		}
	}

	if err := rt.ToolsManager.CheckRequirements(reqs); err != nil {
		return werror.Wrap(err, "error checking tools")
	}
	if err := rt.ToolsManager.Install(); err != nil {
		return werror.Wrap(err, "error installing tools")
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

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
// Creates a Resolver with SOPS and 1Password providers, and registers the secret() helper on the evaluator.
func (rt *Runtime) initializeSecretsProviders() {
	if rt.Resolver != nil {
		rt.registerSecretHelper()
		return
	}

	var providers []secretsRuntime.Provider

	if rt.ConfigHandler.GetBool("secrets.sops.enabled", false) {
		providers = append(providers, secretsRuntime.NewSopsProvider(rt.ConfigRoot))
	}

	vaultsValue := rt.ConfigHandler.Get("secrets.onepassword.vaults")
	if vaultsValue != nil {
		if vaultsMap, ok := vaultsValue.(map[string]any); ok {
			for vaultID, vaultData := range vaultsMap {
				if vaultMap, ok := vaultData.(map[string]any); ok {
					vault := secretsRuntime.OnePasswordVault{
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

					var provider secretsRuntime.Provider
					if rt.Shims.Getenv("OP_SERVICE_ACCOUNT_TOKEN") != "" {
						provider = secretsRuntime.NewOnePasswordSDKProvider(vault)
					} else {
						provider = secretsRuntime.NewOnePasswordCLIProvider(vault)
					}
					providers = append(providers, provider)
				}
			}
		}
	}

	rt.Resolver = secretsRuntime.NewResolver(providers, rt.Shell)
	rt.registerSecretHelper()
}

// registerSecretHelper registers the secret() helper on the evaluator once.
// The field argument is optional, so both two- and three-argument calls compile.
func (rt *Runtime) registerSecretHelper() {
	if rt.secretHelperRegistered || rt.Evaluator == nil || rt.Resolver == nil {
		return
	}

	rt.Evaluator.Register("secret", rt.Resolver.EvaluateHelper,
		new(func(string, string) any),
		new(func(string, string, string) any))
	rt.secretHelperRegistered = true
}

// loadSecrets loads secrets from configured secrets providers via the Resolver.
func (rt *Runtime) loadSecrets() error {
	if rt.Resolver == nil {
		return nil
	}
	return rt.Resolver.LoadAll()
}
