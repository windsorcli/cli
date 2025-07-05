package pipelines

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	envpkg "github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

// The Pipeline is a specialized component that manages environment variable printing functionality.
// It provides environment-specific command execution including environment variable collection and printing,
// secrets decryption support, and shell integration for the Windsor CLI env command.
// The Pipeline handles trusted directory checks, environment printer management, and coordinated execution.

// =============================================================================
// Types
// =============================================================================

// Constructors defines constructor functions for Pipeline dependencies
type EnvConstructors struct {
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

// Pipeline provides environment variable printing functionality
type EnvPipeline struct {
	BasePipeline

	constructors EnvConstructors

	configHandler    config.ConfigHandler
	shell            shell.Shell
	shims            *Shims
	envPrinters      []envpkg.EnvPrinter
	secretsProviders []secrets.SecretsProvider
}

// =============================================================================
// Constructor
// =============================================================================

// NewPipeline creates a new Pipeline instance with optional constructors
func NewEnvPipeline(constructors ...EnvConstructors) *EnvPipeline {
	var ctors EnvConstructors
	if len(constructors) > 0 {
		ctors = constructors[0]
	} else {
		ctors = EnvConstructors{
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

	return &EnvPipeline{
		BasePipeline: *NewBasePipeline(),
		constructors: ctors,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize creates and registers all required components for the env pipeline.
// It sets up the config handler, shell, secrets providers, and environment printers
// in the correct order, registering each component with the dependency injector
// and initializing them sequentially to ensure proper dependency resolution.
func (p *EnvPipeline) Initialize(injector di.Injector) error {
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

// Execute runs the environment variable logic by checking directory trust status,
// handling session reset, loading secrets if requested, collecting and injecting
// environment variables into the process, and printing them unless in quiet mode.
func (p *EnvPipeline) Execute(ctx context.Context) error {
	isTrusted := p.shell.CheckTrustedDirectory() == nil
	hook, _ := ctx.Value("hook").(bool)
	quiet, _ := ctx.Value("quiet").(bool)

	if !isTrusted {
		p.shell.Reset(quiet)
		if !hook {
			fmt.Fprintf(os.Stderr, "\033[33mWarning: You are not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve.\033[0m\n")
		}
		return nil
	}

	if err := p.handleSessionReset(); err != nil {
		return fmt.Errorf("failed to handle session reset: %w", err)
	}

	if decrypt, ok := ctx.Value("decrypt").(bool); ok && decrypt && len(p.secretsProviders) > 0 {
		for _, secretsProvider := range p.secretsProviders {
			if err := secretsProvider.LoadSecrets(); err != nil {
				verbose, _ := ctx.Value("verbose").(bool)
				if verbose {
					return fmt.Errorf("failed to load secrets: %w", err)
				}
				return nil
			}
		}
	}

	if err := p.collectAndSetEnvVars(); err != nil {
		return fmt.Errorf("failed to collect and set environment variables: %w", err)
	}

	if !quiet {
		var firstError error
		for _, envPrinter := range p.envPrinters {
			if err := envPrinter.Print(); err != nil && firstError == nil {
				firstError = fmt.Errorf("failed to print env vars: %w", err)
			}

			if err := envPrinter.PostEnvHook(); err != nil && firstError == nil {
				firstError = fmt.Errorf("failed to execute post env hook: %w", err)
			}
		}

		verbose, _ := ctx.Value("verbose").(bool)
		if verbose {
			return firstError
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// createSecretsProviders creates and initializes secrets providers based on configuration.
// It checks for SOPS encrypted files and OnePassword vault configurations in the current context.
// The function supports both SOPS files and OnePassword providers with automatic SDK/CLI detection.
func (p *EnvPipeline) createSecretsProviders(injector di.Injector) error {
	contextName := p.configHandler.GetContext()
	configRoot, err := p.configHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
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

// createEnvPrinters creates and registers environment printers based on configuration
// It iterates through all available printer types and enables those that are configured
// The function maintains a registry of printer constructors with their enable conditions
// Some printers like terraform and windsor are always enabled while others depend on feature flags
func (p *EnvPipeline) createEnvPrinters(injector di.Injector) error {
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
		if config.enabled {
			envPrinter := config.constructor(injector)
			p.envPrinters = append(p.envPrinters, envPrinter)
			injector.Register(config.key, envPrinter)
		}
	}

	return nil
}

// collectAndSetEnvVars collects environment variables from all registered env printers
// and sets them in the current process environment. This ensures that environment
// variables are always available for both printing and command execution.
func (p *EnvPipeline) collectAndSetEnvVars() error {
	allEnvVars := make(map[string]string)
	for _, envPrinter := range p.envPrinters {
		envVars, err := envPrinter.GetEnvVars()
		if err != nil {
			continue
		}

		maps.Copy(allEnvVars, envVars)
	}

	for key, value := range allEnvVars {
		if err := p.shims.Setenv(key, value); err != nil {
			return fmt.Errorf("failed to set environment variable %s: %w", key, err)
		}
	}

	return nil
}
