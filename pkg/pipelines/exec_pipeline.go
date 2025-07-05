package pipelines

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"strings"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	envpkg "github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

// The ExecPipeline is a specialized component that manages command execution with environment variables.
// It provides comprehensive environment setup including environment variable collection,
// secrets loading, and command execution through the shell interface.
// The ExecPipeline handles dependency initialization, environment preparation, and coordinated execution.

// =============================================================================
// Types
// =============================================================================

// ExecConstructors defines constructor functions for ExecPipeline dependencies
type ExecConstructors struct {
	NewConfigHandler                 func(di.Injector) config.ConfigHandler
	NewShell                         func(di.Injector) shell.Shell
	NewAwsEnvPrinter                 func(di.Injector) envpkg.EnvPrinter
	NewAzureEnvPrinter               func(di.Injector) envpkg.EnvPrinter
	NewDockerEnvPrinter              func(di.Injector) envpkg.EnvPrinter
	NewKubeEnvPrinter                func(di.Injector) envpkg.EnvPrinter
	NewOmniEnvPrinter                func(di.Injector) envpkg.EnvPrinter
	NewTalosEnvPrinter               func(di.Injector) envpkg.EnvPrinter
	NewTerraformEnvPrinter           func(di.Injector) envpkg.EnvPrinter
	NewWindsorEnvPrinter             func(di.Injector) envpkg.EnvPrinter
	NewSopsSecretsProvider           func(string, di.Injector) secrets.SecretsProvider
	NewOnePasswordSDKSecretsProvider func(secretsConfigType.OnePasswordVault, di.Injector) secrets.SecretsProvider
	NewOnePasswordCLISecretsProvider func(secretsConfigType.OnePasswordVault, di.Injector) secrets.SecretsProvider
	NewShims                         func() *Shims
}

// ExecPipeline provides command execution with environment variables
type ExecPipeline struct {
	BasePipeline

	constructors ExecConstructors

	configHandler    config.ConfigHandler
	shell            shell.Shell
	shims            *Shims
	envPrinters      []envpkg.EnvPrinter
	secretsProviders []secrets.SecretsProvider
}

// =============================================================================
// Constructor
// =============================================================================

// NewExecPipeline creates a new ExecPipeline instance with optional constructors
func NewExecPipeline(constructors ...ExecConstructors) *ExecPipeline {
	var ctors ExecConstructors
	if len(constructors) > 0 {
		ctors = constructors[0]
	} else {
		ctors = ExecConstructors{
			NewConfigHandler: func(injector di.Injector) config.ConfigHandler {
				return config.NewYamlConfigHandler(injector)
			},
			NewShell: func(injector di.Injector) shell.Shell {
				return shell.NewDefaultShell(injector)
			},
			NewAwsEnvPrinter: func(injector di.Injector) envpkg.EnvPrinter {
				return envpkg.NewAwsEnvPrinter(injector)
			},
			NewAzureEnvPrinter: func(injector di.Injector) envpkg.EnvPrinter {
				return envpkg.NewAzureEnvPrinter(injector)
			},
			NewDockerEnvPrinter: func(injector di.Injector) envpkg.EnvPrinter {
				return envpkg.NewDockerEnvPrinter(injector)
			},
			NewKubeEnvPrinter: func(injector di.Injector) envpkg.EnvPrinter {
				return envpkg.NewKubeEnvPrinter(injector)
			},
			NewOmniEnvPrinter: func(injector di.Injector) envpkg.EnvPrinter {
				return envpkg.NewOmniEnvPrinter(injector)
			},
			NewTalosEnvPrinter: func(injector di.Injector) envpkg.EnvPrinter {
				return envpkg.NewTalosEnvPrinter(injector)
			},
			NewTerraformEnvPrinter: func(injector di.Injector) envpkg.EnvPrinter {
				return envpkg.NewTerraformEnvPrinter(injector)
			},
			NewWindsorEnvPrinter: func(injector di.Injector) envpkg.EnvPrinter {
				return envpkg.NewWindsorEnvPrinter(injector)
			},
			NewSopsSecretsProvider: func(secretsFile string, injector di.Injector) secrets.SecretsProvider {
				return secrets.NewSopsSecretsProvider(secretsFile, injector)
			},
			NewOnePasswordSDKSecretsProvider: func(vault secretsConfigType.OnePasswordVault, injector di.Injector) secrets.SecretsProvider {
				return secrets.NewOnePasswordSDKSecretsProvider(vault, injector)
			},
			NewOnePasswordCLISecretsProvider: func(vault secretsConfigType.OnePasswordVault, injector di.Injector) secrets.SecretsProvider {
				return secrets.NewOnePasswordCLISecretsProvider(vault, injector)
			},
			NewShims: func() *Shims {
				return NewShims()
			},
		}
	}

	return &ExecPipeline{
		BasePipeline: *NewBasePipeline(),
		constructors: ctors,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize creates and registers all required components for the exec pipeline.
// It sets up the config handler, shell, secrets providers, and environment printers
// in the correct order, registering each component with the dependency injector
// and initializing them sequentially to ensure proper dependency resolution.
func (p *ExecPipeline) Initialize(injector di.Injector) error {
	p.shims = p.constructors.NewShims()

	if existing := injector.Resolve("shell"); existing != nil {
		p.shell = existing.(shell.Shell)
	} else {
		p.shell = p.constructors.NewShell(injector)
		injector.Register("shell", p.shell)
	}
	p.BasePipeline.shell = p.shell

	if existing := injector.Resolve("configHandler"); existing != nil {
		p.configHandler = existing.(config.ConfigHandler)
	} else {
		p.configHandler = p.constructors.NewConfigHandler(injector)
		injector.Register("configHandler", p.configHandler)
	}
	p.BasePipeline.configHandler = p.configHandler

	if err := p.shell.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize shell: %w", err)
	}

	if err := p.configHandler.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize config handler: %w", err)
	}

	if err := p.loadConfig(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := p.createSecretsProviders(injector); err != nil {
		return fmt.Errorf("failed to create secrets providers: %w", err)
	}

	for _, secretsProvider := range p.secretsProviders {
		if err := secretsProvider.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize secrets provider: %w", err)
		}
	}

	if err := p.createEnvPrinters(injector); err != nil {
		return fmt.Errorf("failed to create env printers: %w", err)
	}

	for _, envPrinter := range p.envPrinters {
		if err := envPrinter.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize env printer: %w", err)
		}
	}

	return nil
}

// Execute runs the command execution logic by loading secrets, collecting environment variables,
// setting them in the process environment, and executing the specified command through the shell.
func (p *ExecPipeline) Execute(ctx context.Context) error {
	command, _ := ctx.Value("command").(string)
	args, _ := ctx.Value("args").([]string)

	if command == "" {
		return fmt.Errorf("no command provided")
	}

	for _, secretsProvider := range p.secretsProviders {
		if err := secretsProvider.LoadSecrets(); err != nil {
			return fmt.Errorf("error loading secrets: %w", err)
		}
	}

	envVars := make(map[string]string)
	for _, envPrinter := range p.envPrinters {
		vars, err := envPrinter.GetEnvVars()
		if err != nil {
			return fmt.Errorf("error getting environment variables: %w", err)
		}
		maps.Copy(envVars, vars)
		if err := envPrinter.PostEnvHook(); err != nil {
			return fmt.Errorf("error executing PostEnvHook: %w", err)
		}
	}

	for k, v := range envVars {
		if p.shims == nil {
			if p.constructors.NewShims != nil {
				p.shims = p.constructors.NewShims()
			} else {
				p.shims = NewShims()
			}
		}
		if err := p.shims.Setenv(k, v); err != nil {
			return fmt.Errorf("error setting environment variable %q: %w", k, err)
		}
	}

	if p.shell == nil {
		return fmt.Errorf("no shell found")
	}

	_, err := p.shell.Exec(command, args...)
	if err != nil {
		return fmt.Errorf("command execution failed: %w", err)
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// createSecretsProviders creates and initializes secrets providers based on configuration.
// It checks for SOPS encrypted files and OnePassword vault configurations in the current context.
// The function supports both SOPS files and OnePassword providers with automatic SDK/CLI detection.
func (p *ExecPipeline) createSecretsProviders(injector di.Injector) error {
	contextName := p.configHandler.GetContext()
	configRoot, err := p.configHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	if p.shims == nil {
		if p.constructors.NewShims != nil {
			p.shims = p.constructors.NewShims()
		} else {
			p.shims = NewShims()
		}
	}

	// SOPS
	secretsFilePaths := []string{"secrets.enc.yaml", "secrets.enc.yml"}
	for _, filePath := range secretsFilePaths {
		if _, err := p.shims.Stat(filepath.Join(configRoot, filePath)); err == nil {
			sopsSecretsProvider := p.constructors.NewSopsSecretsProvider(configRoot, injector)
			p.secretsProviders = append(p.secretsProviders, sopsSecretsProvider)
			injector.Register("sopsSecretsProvider", sopsSecretsProvider)
			p.configHandler.SetSecretsProvider(sopsSecretsProvider)
			break
		}
	}

	// 1Password
	vaults, ok := p.configHandler.Get(fmt.Sprintf("contexts.%s.secrets.onepassword.vaults", contextName)).(map[string]secretsConfigType.OnePasswordVault)
	if ok && len(vaults) > 0 {
		useSDK := p.shims.Getenv("OP_SERVICE_ACCOUNT_TOKEN") != ""

		for key, vault := range vaults {
			vault.ID = key
			providerName := fmt.Sprintf("op%sSecretsProvider", strings.ToUpper(key[:1])+key[1:])

			var opSecretsProvider secrets.SecretsProvider

			if useSDK {
				opSecretsProvider = p.constructors.NewOnePasswordSDKSecretsProvider(vault, injector)
			} else {
				opSecretsProvider = p.constructors.NewOnePasswordCLISecretsProvider(vault, injector)
			}

			p.secretsProviders = append(p.secretsProviders, opSecretsProvider)
			injector.Register(providerName, opSecretsProvider)
			p.configHandler.SetSecretsProvider(opSecretsProvider)
		}
	}

	return nil
}

// createEnvPrinters creates and registers environment printers based on configuration.
// It iterates through all available printer types and enables those that are configured.
// The function maintains a registry of printer constructors with their enable conditions.
// Some printers like terraform and windsor are always enabled while others depend on feature flags.
func (p *ExecPipeline) createEnvPrinters(injector di.Injector) error {
	envPrinterConfigs := map[string]struct {
		enabled     bool
		constructor func(di.Injector) envpkg.EnvPrinter
		key         string
	}{
		"aws": {
			enabled:     p.configHandler.GetBool("aws.enabled"),
			constructor: p.constructors.NewAwsEnvPrinter,
			key:         "awsEnv",
		},
		"azure": {
			enabled:     p.configHandler.GetBool("azure.enabled"),
			constructor: p.constructors.NewAzureEnvPrinter,
			key:         "azureEnv",
		},
		"docker": {
			enabled:     p.configHandler.GetBool("docker.enabled"),
			constructor: p.constructors.NewDockerEnvPrinter,
			key:         "dockerEnv",
		},
		"kube": {
			enabled:     p.configHandler.GetBool("cluster.enabled"),
			constructor: p.constructors.NewKubeEnvPrinter,
			key:         "kubeEnv",
		},
		"omni": {
			enabled:     p.configHandler.GetBool("omni.enabled"),
			constructor: p.constructors.NewOmniEnvPrinter,
			key:         "omniEnv",
		},
		"talos": {
			enabled:     p.configHandler.GetBool("cluster.enabled") && p.configHandler.GetString("cluster.driver") == "talos",
			constructor: p.constructors.NewTalosEnvPrinter,
			key:         "talosEnv",
		},
		"terraform": {
			enabled:     p.configHandler.GetBool("terraform.enabled"),
			constructor: p.constructors.NewTerraformEnvPrinter,
			key:         "terraformEnv",
		},
		"windsor": {
			enabled:     true,
			constructor: p.constructors.NewWindsorEnvPrinter,
			key:         "windsorEnv",
		},
	}

	for _, config := range envPrinterConfigs {
		if config.enabled && config.constructor != nil {
			envPrinter := config.constructor(injector)
			p.envPrinters = append(p.envPrinters, envPrinter)
			injector.Register(config.key, envPrinter)
		}
	}

	return nil
}
