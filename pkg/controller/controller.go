package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/generators"
	"github.com/windsorcli/cli/pkg/network"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/services"
	sh "github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/ssh"
	"github.com/windsorcli/cli/pkg/stack"
	"github.com/windsorcli/cli/pkg/tools"
	"github.com/windsorcli/cli/pkg/virt"
)

// The Controller is a central orchestrator that manages the lifecycle and interactions of various
// infrastructure and application components. It serves as the primary coordinator for resolving
// dependencies, managing configurations, and orchestrating the creation and deployment of resources
// across different environments. The controller handles:
//
// - Component initialization and lifecycle management
// - Configuration resolution and environment variable management
// - Secrets and credentials management
// - Service deployment and orchestration
// - Virtualization and container runtime management
// - Network configuration and management
// - Stack deployment and management
// - Code generation and templating
//
// It integrates with multiple subsystems including blueprint management, environment configuration,
// secrets management, service orchestration, and infrastructure provisioning. The controller
// provides a unified interface for resolving and managing these components while ensuring
// proper dependency injection and configuration management across the system.

// =============================================================================
// Types
// =============================================================================

type Controller interface {
	SetRequirements(req Requirements)
	CreateComponents() error
	InitializeComponents() error
	InitializeWithRequirements(req Requirements) error
	ResolveInjector() di.Injector
	ResolveConfigHandler() config.ConfigHandler
	ResolveAllSecretsProviders() []secrets.SecretsProvider
	ResolveEnvPrinter(name string) env.EnvPrinter
	ResolveAllEnvPrinters() []env.EnvPrinter
	ResolveShell() sh.Shell
	ResolveSecureShell() sh.Shell
	ResolveNetworkManager() network.NetworkManager
	ResolveToolsManager() tools.ToolsManager
	ResolveBlueprintHandler() blueprint.BlueprintHandler
	ResolveService(name string) services.Service
	ResolveAllServices() []services.Service
	ResolveVirtualMachine() virt.VirtualMachine
	ResolveContainerRuntime() virt.ContainerRuntime
	ResolveStack() stack.Stack
	ResolveAllGenerators() []generators.Generator
	WriteConfigurationFiles() error
	SetEnvironmentVariables() error
}

// BaseController struct implements the Controller interface.
type BaseController struct {
	injector     di.Injector
	constructors ComponentConstructors
	requirements Requirements
}

// ComponentConstructors contains factory functions for all components used in the controller
type ComponentConstructors struct {
	// Common components
	NewConfigHandler func(di.Injector) config.ConfigHandler
	NewShell         func(di.Injector) sh.Shell
	NewSecureShell   func(di.Injector) sh.Shell

	// Project components
	NewGitGenerator       func(di.Injector) generators.Generator
	NewBlueprintHandler   func(di.Injector) blueprint.BlueprintHandler
	NewTerraformGenerator func(di.Injector) generators.Generator
	NewKustomizeGenerator func(di.Injector) generators.Generator
	NewToolsManager       func(di.Injector) tools.ToolsManager

	// Environment printers
	NewAwsEnvPrinter       func(di.Injector) env.EnvPrinter
	NewDockerEnvPrinter    func(di.Injector) env.EnvPrinter
	NewKubeEnvPrinter      func(di.Injector) env.EnvPrinter
	NewOmniEnvPrinter      func(di.Injector) env.EnvPrinter
	NewTalosEnvPrinter     func(di.Injector) env.EnvPrinter
	NewTerraformEnvPrinter func(di.Injector) env.EnvPrinter
	NewWindsorEnvPrinter   func(di.Injector) env.EnvPrinter

	// Service components
	NewDNSService           func(di.Injector) services.Service
	NewGitLivereloadService func(di.Injector) services.Service
	NewLocalstackService    func(di.Injector) services.Service
	NewRegistryService      func(di.Injector) services.Service
	NewTalosService         func(di.Injector, string) services.Service

	// Virtualization components
	NewSSHClient                func() *ssh.SSHClient
	NewColimaVirt               func(di.Injector) virt.VirtualMachine
	NewColimaNetworkManager     func(di.Injector) network.NetworkManager
	NewBaseNetworkManager       func(di.Injector) network.NetworkManager
	NewDockerVirt               func(di.Injector) virt.ContainerRuntime
	NewNetworkInterfaceProvider func() network.NetworkInterfaceProvider

	// Secrets providers
	NewSopsSecretsProvider           func(string, di.Injector) secrets.SecretsProvider
	NewOnePasswordSDKSecretsProvider func(secretsConfigType.OnePasswordVault, di.Injector) secrets.SecretsProvider
	NewOnePasswordCLISecretsProvider func(secretsConfigType.OnePasswordVault, di.Injector) secrets.SecretsProvider

	// Stack components
	NewWindsorStack func(di.Injector) stack.Stack
}

// Requirements represents what functionality is required of the controller
type Requirements struct {
	// Core requirements (most commands need these)
	Trust        bool // Requires being in a trusted directory
	ConfigLoaded bool // Requires config to be loaded

	// Environment requirements
	Env bool // Needs access to environment variables

	// Security requirements
	Secrets bool // Needs to decrypt/access secrets

	// Infrastructure requirements
	VM         bool // Needs virtual machine capabilities
	Containers bool // Needs container runtime capabilities
	Network    bool // Needs network management

	// Service requirements
	Services bool // Needs service management

	// Project requirements
	Tools      bool // Needs git, terraform, etc.
	Blueprint  bool // Needs blueprint handling
	Generators bool // Needs code generation
	Stack      bool // Needs stack components

	// Command info for context-specific decisions
	CommandName string          // Name of the command
	Flags       map[string]bool // Important flags that affect initialization
}

// =============================================================================
// Constructor
// =============================================================================

// NewController creates a new controller.
func NewController(injector di.Injector) *BaseController {
	return &BaseController{
		injector:     injector,
		constructors: NewDefaultConstructors(),
	}
}

// DefaultConstructors returns a ComponentConstructors with the default implementation
// of all factory functions
func NewDefaultConstructors() ComponentConstructors {
	return ComponentConstructors{
		// Common components
		NewConfigHandler: func(injector di.Injector) config.ConfigHandler {
			return config.NewYamlConfigHandler(injector)
		},
		NewShell: func(injector di.Injector) sh.Shell {
			return sh.NewDefaultShell(injector)
		},
		NewSecureShell: func(injector di.Injector) sh.Shell {
			return sh.NewSecureShell(injector)
		},

		// Project components
		NewGitGenerator: func(injector di.Injector) generators.Generator {
			return generators.NewGitGenerator(injector)
		},
		NewBlueprintHandler: func(injector di.Injector) blueprint.BlueprintHandler {
			return blueprint.NewBlueprintHandler(injector)
		},
		NewTerraformGenerator: func(injector di.Injector) generators.Generator {
			return generators.NewTerraformGenerator(injector)
		},
		NewKustomizeGenerator: func(injector di.Injector) generators.Generator {
			return generators.NewKustomizeGenerator(injector)
		},
		NewToolsManager: func(injector di.Injector) tools.ToolsManager {
			return tools.NewToolsManager(injector)
		},

		// Environment printers
		NewAwsEnvPrinter: func(injector di.Injector) env.EnvPrinter {
			return env.NewAwsEnvPrinter(injector)
		},
		NewDockerEnvPrinter: func(injector di.Injector) env.EnvPrinter {
			return env.NewDockerEnvPrinter(injector)
		},
		NewKubeEnvPrinter: func(injector di.Injector) env.EnvPrinter {
			return env.NewKubeEnvPrinter(injector)
		},
		NewOmniEnvPrinter: func(injector di.Injector) env.EnvPrinter {
			return env.NewOmniEnvPrinter(injector)
		},
		NewTalosEnvPrinter: func(injector di.Injector) env.EnvPrinter {
			return env.NewTalosEnvPrinter(injector)
		},
		NewTerraformEnvPrinter: func(injector di.Injector) env.EnvPrinter {
			return env.NewTerraformEnvPrinter(injector)
		},
		NewWindsorEnvPrinter: func(injector di.Injector) env.EnvPrinter {
			return env.NewWindsorEnvPrinter(injector)
		},

		// Service components
		NewDNSService: func(injector di.Injector) services.Service {
			return services.NewDNSService(injector)
		},
		NewGitLivereloadService: func(injector di.Injector) services.Service {
			return services.NewGitLivereloadService(injector)
		},
		NewLocalstackService: func(injector di.Injector) services.Service {
			return services.NewLocalstackService(injector)
		},
		NewRegistryService: func(injector di.Injector) services.Service {
			return services.NewRegistryService(injector)
		},
		NewTalosService: func(injector di.Injector, nodeType string) services.Service {
			return services.NewTalosService(injector, nodeType)
		},

		// Virtualization components
		NewSSHClient: func() *ssh.SSHClient {
			return ssh.NewSSHClient()
		},
		NewColimaVirt: func(injector di.Injector) virt.VirtualMachine {
			return virt.NewColimaVirt(injector)
		},
		NewColimaNetworkManager: func(injector di.Injector) network.NetworkManager {
			return network.NewColimaNetworkManager(injector)
		},
		NewBaseNetworkManager: func(injector di.Injector) network.NetworkManager {
			return network.NewBaseNetworkManager(injector)
		},
		NewDockerVirt: func(injector di.Injector) virt.ContainerRuntime {
			return virt.NewDockerVirt(injector)
		},
		NewNetworkInterfaceProvider: func() network.NetworkInterfaceProvider {
			return network.NewNetworkInterfaceProvider()
		},

		// Secrets providers
		NewSopsSecretsProvider: func(secretsFile string, injector di.Injector) secrets.SecretsProvider {
			return secrets.NewSopsSecretsProvider(secretsFile, injector)
		},
		NewOnePasswordSDKSecretsProvider: func(vault secretsConfigType.OnePasswordVault, injector di.Injector) secrets.SecretsProvider {
			return secrets.NewOnePasswordSDKSecretsProvider(vault, injector)
		},
		NewOnePasswordCLISecretsProvider: func(vault secretsConfigType.OnePasswordVault, injector di.Injector) secrets.SecretsProvider {
			return secrets.NewOnePasswordCLISecretsProvider(vault, injector)
		},

		// Stack components
		NewWindsorStack: func(injector di.Injector) stack.Stack {
			return stack.NewWindsorStack(injector)
		},
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// SetRequirements sets the requirements for the controller
func (c *BaseController) SetRequirements(req Requirements) {
	c.requirements = req
}

// CreateComponents creates components based on the specified requirements
func (c *BaseController) CreateComponents() error {
	if c.injector == nil {
		return fmt.Errorf("injector is nil")
	}
	if c.requirements.CommandName == "" {
		return fmt.Errorf("requirements not set")
	}
	req := c.requirements

	componentCreators := []struct {
		name string
		fn   func(Requirements) error
	}{
		{"shell", c.createShellComponent},
		{"config", c.createConfigComponent},
		{"tools", c.createToolsComponents},
		{"env", c.createEnvComponents},
		{"secrets", c.createSecretsComponents},
		{"generators", c.createGeneratorsComponents},
		{"blueprint", c.createBlueprintComponent},
		{"virtualization", c.createVirtualizationComponents},
		{"service", c.createServiceComponents},
		{"network", c.createNetworkComponents},
		{"stack", c.createStackComponent},
	}

	for _, cc := range componentCreators {
		if err := cc.fn(req); err != nil {
			return fmt.Errorf("failed to create %s components: %w", cc.name, err)
		}
	}

	return nil
}

// InitializeComponents initializes all components.
func (c *BaseController) InitializeComponents() error {

	// Initialize the shell
	shell := c.ResolveShell()
	if shell != nil {
		if err := shell.Initialize(); err != nil {
			return fmt.Errorf("error initializing shell: %w", err)
		}
	}

	// Initialize the secure shell
	secureShell := c.ResolveSecureShell()
	if secureShell != nil {
		if err := secureShell.Initialize(); err != nil {
			return fmt.Errorf("error initializing secure shell: %w", err)
		}
	}

	// Initialize the secrets providers
	secretsProviders := c.ResolveAllSecretsProviders()
	if len(secretsProviders) > 0 {
		for _, secretsProvider := range secretsProviders {
			if err := secretsProvider.Initialize(); err != nil {
				return fmt.Errorf("error initializing secrets provider: %w", err)
			}
		}
	}

	// Initialize the env printers
	envPrinters := c.ResolveAllEnvPrinters()
	if len(envPrinters) > 0 {
		for _, envPrinter := range envPrinters {
			if err := envPrinter.Initialize(); err != nil {
				return fmt.Errorf("error initializing env printer: %w", err)
			}
		}
	}

	// Initialize the tools manager
	toolsManager := c.ResolveToolsManager()
	if toolsManager != nil {
		if err := toolsManager.Initialize(); err != nil {
			return fmt.Errorf("error initializing tools manager: %w", err)
		}
	}

	// Initialize the services
	services := c.ResolveAllServices()
	if len(services) > 0 {
		for _, service := range services {
			if err := service.Initialize(); err != nil {
				return fmt.Errorf("error initializing service: %w", err)
			}
		}
	}

	// Initialize the virtual machine
	virtualMachine := c.ResolveVirtualMachine()
	if virtualMachine != nil {
		if err := virtualMachine.Initialize(); err != nil {
			return fmt.Errorf("error initializing virtual machine: %w", err)
		}
	}

	// Initialize the container runtime
	containerRuntime := c.ResolveContainerRuntime()
	if containerRuntime != nil {
		if err := containerRuntime.Initialize(); err != nil {
			return fmt.Errorf("error initializing container runtime: %w", err)
		}
	}

	// Initialize the network manager
	networkManager := c.ResolveNetworkManager()
	if networkManager != nil {
		if err := networkManager.Initialize(); err != nil {
			return fmt.Errorf("error initializing network manager: %w", err)
		}
	}

	// Initialize the blueprint handler
	blueprintHandler := c.ResolveBlueprintHandler()
	if blueprintHandler != nil {
		if err := blueprintHandler.Initialize(); err != nil {
			return fmt.Errorf("error initializing blueprint handler: %w", err)
		}
		if err := blueprintHandler.LoadConfig(); err != nil {
			return fmt.Errorf("error loading blueprint config: %w", err)
		}
	}

	// Initialize the generators
	generators := c.ResolveAllGenerators()
	if len(generators) > 0 {
		for _, generator := range generators {
			if err := generator.Initialize(); err != nil {
				return fmt.Errorf("error initializing generator: %w", err)
			}
		}
	}

	// Initialize the stack
	stack := c.ResolveStack()
	if stack != nil {
		if err := stack.Initialize(); err != nil {
			return fmt.Errorf("error initializing stack: %w", err)
		}
	}

	return nil
}

// InitializeWithRequirements sets the requirements, creates components, and initializes them.
// This is the standard initialization sequence used by commands.
func (c *BaseController) InitializeWithRequirements(req Requirements) error {
	c.SetRequirements(req)
	if err := c.CreateComponents(); err != nil {
		return fmt.Errorf("failed to create components: %w", err)
	}
	if err := c.InitializeComponents(); err != nil {
		return fmt.Errorf("failed to initialize components: %w", err)
	}
	return nil
}

// WriteConfigurationFiles writes the configuration files.
func (c *BaseController) WriteConfigurationFiles() error {
	req := c.requirements

	// Write tools manifest if tools are required
	if req.Tools {
		toolsManager := c.ResolveToolsManager()
		if toolsManager != nil {
			if err := toolsManager.WriteManifest(); err != nil {
				return fmt.Errorf("error writing tools manifest: %w", err)
			}
		}
	}

	// Write blueprint if blueprint is required
	if req.Blueprint {
		blueprintHandler := c.ResolveBlueprintHandler()
		if blueprintHandler != nil {
			if err := blueprintHandler.WriteConfig(); err != nil {
				return fmt.Errorf("error writing blueprint config: %w", err)
			}
		}
	}

	// Write configuration for all services if services are required
	if req.Services {
		resolvedServices := c.ResolveAllServices()
		for _, service := range resolvedServices {
			if service != nil {
				if err := service.WriteConfig(); err != nil {
					return fmt.Errorf("error writing service config: %w", err)
				}
			}
		}
	}

	// Write configuration for virtual machine if vm is required
	if req.VM {
		if vmDriver := c.ResolveConfigHandler().GetString("vm.driver"); vmDriver != "" {
			resolvedVirt := c.ResolveVirtualMachine()
			if resolvedVirt != nil {
				if err := resolvedVirt.WriteConfig(); err != nil {
					return fmt.Errorf("error writing virtual machine config: %w", err)
				}
			}
		}
	}

	// Write configuration for container runtime if containers are required
	if req.Containers {
		if dockerEnabled := c.ResolveConfigHandler().GetBool("docker.enabled"); dockerEnabled {
			resolvedContainerRuntime := c.ResolveContainerRuntime()
			if resolvedContainerRuntime != nil {
				if err := resolvedContainerRuntime.WriteConfig(); err != nil {
					return fmt.Errorf("error writing container runtime config: %w", err)
				}
			}
		}
	}

	// Write configuration for all generators if generators are required
	if req.Generators {
		generators := c.ResolveAllGenerators()
		for _, generator := range generators {
			if generator != nil {
				if err := generator.Write(); err != nil {
					return fmt.Errorf("error writing generator config: %w", err)
				}
			}
		}
	}

	return nil
}

// ResolveInjector resolves the injector instance.
func (c *BaseController) ResolveInjector() di.Injector {
	return c.injector
}

// ResolveConfigHandler resolves the configHandler instance.
func (c *BaseController) ResolveConfigHandler() config.ConfigHandler {
	instance := c.injector.Resolve("configHandler")
	configHandler, _ := instance.(config.ConfigHandler)
	return configHandler
}

// ResolveAllSecretsProviders resolves all secretsProvider instances.
func (c *BaseController) ResolveAllSecretsProviders() []secrets.SecretsProvider {
	instances, _ := c.injector.ResolveAll((*secrets.SecretsProvider)(nil))
	secretsProviders := make([]secrets.SecretsProvider, 0, len(instances))

	for _, instance := range instances {
		secretsProvider, _ := instance.(secrets.SecretsProvider)
		secretsProviders = append(secretsProviders, secretsProvider)
	}

	return secretsProviders
}

// ResolveEnvPrinter resolves the envPrinter instance.
func (c *BaseController) ResolveEnvPrinter(name string) env.EnvPrinter {
	instance := c.injector.Resolve(name)
	envPrinter, _ := instance.(env.EnvPrinter)
	return envPrinter
}

// ResolveAllEnvPrinters resolves all envPrinter instances.
func (c *BaseController) ResolveAllEnvPrinters() []env.EnvPrinter {
	instances, _ := c.injector.ResolveAll((*env.EnvPrinter)(nil))
	envPrinters := make([]env.EnvPrinter, 0, len(instances))
	var windsorEnv env.EnvPrinter

	for _, instance := range instances {
		envPrinter, _ := instance.(env.EnvPrinter)
		if _, ok := envPrinter.(*env.WindsorEnvPrinter); ok {
			windsorEnv = envPrinter
		} else {
			envPrinters = append(envPrinters, envPrinter)
		}
	}

	if windsorEnv != nil {
		envPrinters = append(envPrinters, windsorEnv)
	}

	return envPrinters
}

// ResolveShell resolves the shell instance.
func (c *BaseController) ResolveShell() sh.Shell {
	instance := c.injector.Resolve("shell")
	shellInstance, _ := instance.(sh.Shell)
	return shellInstance
}

// ResolveSecureShell resolves the secureShell instance.
func (c *BaseController) ResolveSecureShell() sh.Shell {
	instance := c.injector.Resolve("secureShell")
	shellInstance, _ := instance.(sh.Shell)
	return shellInstance
}

// ResolveNetworkManager resolves the networkManager instance.
func (c *BaseController) ResolveNetworkManager() network.NetworkManager {
	instance := c.injector.Resolve("networkManager")
	networkManager, _ := instance.(network.NetworkManager)
	return networkManager
}

// ResolveToolsManager resolves the toolsManager instance.
func (c *BaseController) ResolveToolsManager() tools.ToolsManager {
	instance := c.injector.Resolve("toolsManager")
	toolsManager, _ := instance.(tools.ToolsManager)
	return toolsManager
}

// ResolveBlueprintHandler resolves the blueprintHandler instance.
func (c *BaseController) ResolveBlueprintHandler() blueprint.BlueprintHandler {
	instance := c.injector.Resolve("blueprintHandler")
	blueprintHandler, _ := instance.(blueprint.BlueprintHandler)
	return blueprintHandler
}

// ResolveService resolves the requested service instance.
func (c *BaseController) ResolveService(name string) services.Service {
	instance := c.injector.Resolve(fmt.Sprintf("%s", name))
	service, _ := instance.(services.Service)
	return service
}

// ResolveAllServices resolves all service instances.
func (c *BaseController) ResolveAllServices() []services.Service {
	instances, _ := c.injector.ResolveAll((*services.Service)(nil))
	servicesInstances := make([]services.Service, 0, len(instances))
	for _, instance := range instances {
		serviceInstance, _ := instance.(services.Service)
		servicesInstances = append(servicesInstances, serviceInstance)
	}
	return servicesInstances
}

// ResolveVirtualMachine resolves the requested virtualMachine instance.
func (c *BaseController) ResolveVirtualMachine() virt.VirtualMachine {
	instance := c.injector.Resolve("virtualMachine")
	virtualMachine, _ := instance.(virt.VirtualMachine)
	return virtualMachine
}

// ResolveContainerRuntime resolves the requested containerRuntime instance.
func (c *BaseController) ResolveContainerRuntime() virt.ContainerRuntime {
	instance := c.injector.Resolve("containerRuntime")
	containerRuntime, _ := instance.(virt.ContainerRuntime)
	return containerRuntime
}

// ResolveStack resolves the requested stack instance.
func (c *BaseController) ResolveStack() stack.Stack {
	instance := c.injector.Resolve("stack")
	stackInstance, _ := instance.(stack.Stack)
	return stackInstance
}

// ResolveAllGenerators resolves all generator instances.
func (c *BaseController) ResolveAllGenerators() []generators.Generator {
	instances, _ := c.injector.ResolveAll((*generators.Generator)(nil))
	generatorsInstances := make([]generators.Generator, 0, len(instances))
	for _, instance := range instances {
		generatorInstance, _ := instance.(generators.Generator)
		generatorsInstances = append(generatorsInstances, generatorInstance)
	}
	return generatorsInstances
}

// SetEnvironmentVariables sets the environment variables in the session
func (c *BaseController) SetEnvironmentVariables() error {
	envPrinters := c.ResolveAllEnvPrinters()
	for _, envPrinter := range envPrinters {
		envVars, err := envPrinter.GetEnvVars()
		if err != nil {
			return fmt.Errorf("error getting environment variables: %w", err)
		}
		for key, value := range envVars {
			if err := osSetenv(key, value); err != nil {
				return fmt.Errorf("error setting environment variable %s: %w", key, err)
			}
		}
	}
	return nil
}

// =============================================================================
// Private functions
// =============================================================================

// createShellComponent creates and initializes the shell component if required
func (c *BaseController) createShellComponent(req Requirements) error {
	if c.constructors.NewShell == nil {
		return fmt.Errorf("required constructor NewShell is nil")
	}

	shell := c.constructors.NewShell(c.injector)
	c.injector.Register("shell", shell)

	if verbose, ok := req.Flags["verbose"]; ok && verbose {
		shell.SetVerbosity(true)
	}

	if req.Trust {
		if shell.CheckTrustedDirectory() != nil {
			fmt.Fprintf(os.Stderr, "\033[33mWarning: You are not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve.\033[0m\n")
		}
	}

	return nil
}

// createConfigComponent creates and initializes the config component if required
func (c *BaseController) createConfigComponent(req Requirements) error {
	if c.constructors.NewConfigHandler == nil {
		return fmt.Errorf("required constructor NewConfigHandler is nil")
	}

	configHandler := c.constructors.NewConfigHandler(c.injector)
	c.injector.Register("configHandler", configHandler)

	if err := configHandler.Initialize(); err != nil {
		return fmt.Errorf("error initializing config handler: %w", err)
	}

	cliConfigPath := os.Getenv("WINDSORCONFIG")
	if cliConfigPath == "" {
		shell := c.ResolveShell()
		projectRoot, err := shell.GetProjectRoot()
		if err != nil {
			return fmt.Errorf("error retrieving project root: %w", err)
		}

		yamlPath := filepath.Join(projectRoot, "windsor.yaml")
		ymlPath := filepath.Join(projectRoot, "windsor.yml")

		if _, err := osStat(yamlPath); os.IsNotExist(err) {
			if _, err := osStat(ymlPath); err == nil {
				cliConfigPath = ymlPath
			}
		} else {
			cliConfigPath = yamlPath
		}
	}

	configHandler.GetContext()

	if cliConfigPath != "" {
		if err := configHandler.LoadConfig(cliConfigPath); err != nil {
			return fmt.Errorf("error loading config file: %w", err)
		}
	}

	if req.ConfigLoaded && !configHandler.IsLoaded() {
		fmt.Fprintln(os.Stderr, "Cannot execute commands. Please run 'windsor init' to set up your project first.")
		return nil
	}

	return nil
}

// createSecretsComponents creates and initializes secrets providers if required
func (c *BaseController) createSecretsComponents(req Requirements) error {
	if !req.Secrets {
		return nil
	}

	configHandler := c.ResolveConfigHandler()
	if configHandler == nil {
		return fmt.Errorf("config handler is nil")
	}

	contextName := configHandler.GetContext()
	configRoot, err := configHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	secretsFilePaths := []string{"secrets.enc.yaml", "secrets.enc.yml"}
	for _, filePath := range secretsFilePaths {
		if _, err := osStat(filepath.Join(configRoot, filePath)); err == nil {
			sopsSecretsProvider := c.constructors.NewSopsSecretsProvider(configRoot, c.injector)
			c.injector.Register("sopsSecretsProvider", sopsSecretsProvider)
			configHandler.SetSecretsProvider(sopsSecretsProvider)
		}
	}

	vaults, ok := configHandler.Get(fmt.Sprintf("contexts.%s.secrets.onepassword.vaults", contextName)).(map[string]secretsConfigType.OnePasswordVault)
	if ok && len(vaults) > 0 {
		useSDK := os.Getenv("OP_SERVICE_ACCOUNT_TOKEN") != ""

		for key, vault := range vaults {
			vault.ID = key
			var opSecretsProvider secrets.SecretsProvider

			if useSDK {
				opSecretsProvider = c.constructors.NewOnePasswordSDKSecretsProvider(vault, c.injector)
			} else {
				opSecretsProvider = c.constructors.NewOnePasswordCLISecretsProvider(vault, c.injector)
			}

			c.injector.Register(fmt.Sprintf("op%sSecretsProvider", strings.ToUpper(key[:1])+key[1:]), opSecretsProvider)
			configHandler.SetSecretsProvider(opSecretsProvider)
		}
	}

	return nil
}

// createToolsComponents creates and initializes project tools if required
func (c *BaseController) createToolsComponents(req Requirements) error {
	if !req.Tools {
		return nil
	}

	configHandler := c.ResolveConfigHandler()
	if configHandler == nil {
		return fmt.Errorf("config handler is nil")
	}

	toolsManagerType := configHandler.GetString("toolsManager")
	if toolsManagerType == "" {
		toolsManagerType, _ = tools.CheckExistingToolsManager(configHandler.GetString("projectRoot"))
	}

	toolsManager := c.constructors.NewToolsManager(c.injector)
	c.injector.Register("toolsManager", toolsManager)

	return nil
}

// createGeneratorsComponents creates and initializes code generators if required
func (c *BaseController) createGeneratorsComponents(req Requirements) error {
	if !req.Generators {
		return nil
	}

	gitGenerator := c.constructors.NewGitGenerator(c.injector)
	c.injector.Register("gitGenerator", gitGenerator)

	if req.Blueprint {
		terraformGenerator := c.constructors.NewTerraformGenerator(c.injector)
		c.injector.Register("terraformGenerator", terraformGenerator)

		kustomizeGenerator := c.constructors.NewKustomizeGenerator(c.injector)
		c.injector.Register("kustomizeGenerator", kustomizeGenerator)
	}

	return nil
}

// createBlueprintComponent creates and initializes the blueprint handler if required
func (c *BaseController) createBlueprintComponent(req Requirements) error {
	if !req.Blueprint {
		return nil
	}

	blueprintHandler := c.constructors.NewBlueprintHandler(c.injector)
	c.injector.Register("blueprintHandler", blueprintHandler)

	return nil
}

// createEnvComponents creates and initializes environment components if required
func (c *BaseController) createEnvComponents(req Requirements) error {
	if !req.Env {
		return nil
	}

	configHandler := c.ResolveConfigHandler()
	if configHandler == nil {
		return fmt.Errorf("config handler is nil")
	}

	envPrinters := map[string]func(di.Injector) env.EnvPrinter{
		"awsEnv":       c.constructors.NewAwsEnvPrinter,
		"dockerEnv":    c.constructors.NewDockerEnvPrinter,
		"kubeEnv":      c.constructors.NewKubeEnvPrinter,
		"omniEnv":      c.constructors.NewOmniEnvPrinter,
		"talosEnv":     c.constructors.NewTalosEnvPrinter,
		"terraformEnv": c.constructors.NewTerraformEnvPrinter,
		"windsorEnv":   c.constructors.NewWindsorEnvPrinter,
	}

	for key, constructor := range envPrinters {
		if key == "awsEnv" && !configHandler.GetBool("aws.enabled") {
			continue
		}
		if key == "dockerEnv" && !configHandler.GetBool("docker.enabled") {
			continue
		}
		envPrinter := constructor(c.injector)
		c.injector.Register(key, envPrinter)
	}

	return nil
}

// createServiceComponents creates and initializes service components if required
func (c *BaseController) createServiceComponents(req Requirements) error {
	if !req.Services {
		return nil
	}

	configHandler := c.ResolveConfigHandler()
	if configHandler == nil {
		return fmt.Errorf("config handler is nil")
	}

	if !configHandler.GetBool("docker.enabled") {
		return nil
	}

	dnsEnabled := configHandler.GetBool("dns.enabled")
	if dnsEnabled {
		dnsService := c.constructors.NewDNSService(c.injector)
		dnsService.SetName("dns")
		c.injector.Register("dnsService", dnsService)
	}

	gitLivereloadEnabled := configHandler.GetBool("git.livereload.enabled")
	if gitLivereloadEnabled {
		gitLivereloadService := c.constructors.NewGitLivereloadService(c.injector)
		gitLivereloadService.SetName("git")
		c.injector.Register("gitLivereloadService", gitLivereloadService)
	}

	localstackEnabled := configHandler.GetBool("aws.localstack.enabled")
	if localstackEnabled {
		localstackService := c.constructors.NewLocalstackService(c.injector)
		localstackService.SetName("aws")
		c.injector.Register("localstackService", localstackService)
	}

	contextConfig := configHandler.GetConfig()
	if contextConfig.Docker != nil && contextConfig.Docker.Registries != nil {
		for key := range contextConfig.Docker.Registries {
			service := c.constructors.NewRegistryService(c.injector)
			service.SetName(key)
			serviceName := fmt.Sprintf("registryService.%s", key)
			c.injector.Register(serviceName, service)
		}
	}

	if configHandler.GetBool("cluster.enabled") {
		clusterDriver := configHandler.GetString("cluster.driver")
		if clusterDriver == "talos" {
			controlPlaneCount := configHandler.GetInt("cluster.controlplanes.count")
			workerCount := configHandler.GetInt("cluster.workers.count")

			for i := 1; i <= controlPlaneCount; i++ {
				controlPlaneService := c.constructors.NewTalosService(c.injector, "controlplane")
				controlPlaneService.SetName(fmt.Sprintf("controlplane-%d", i))
				serviceName := fmt.Sprintf("clusterNode.controlplane-%d", i)
				c.injector.Register(serviceName, controlPlaneService)
			}

			for i := 1; i <= workerCount; i++ {
				workerService := c.constructors.NewTalosService(c.injector, "worker")
				workerService.SetName(fmt.Sprintf("worker-%d", i))
				serviceName := fmt.Sprintf("clusterNode.worker-%d", i)
				c.injector.Register(serviceName, workerService)
			}
		}
	}

	return nil
}

// createNetworkComponents creates and initializes network components based on configuration.
func (c *BaseController) createNetworkComponents(req Requirements) error {
	if !req.Network {
		return nil
	}

	vmDriver := c.ResolveConfigHandler().GetString("vm.driver")

	networkInterfaceProvider := c.constructors.NewNetworkInterfaceProvider()
	c.injector.Register("networkInterfaceProvider", networkInterfaceProvider)

	if req.VM {
		secureShell := c.constructors.NewSecureShell(c.injector)
		c.injector.Register("secureShell", secureShell)

		sshClient := c.constructors.NewSSHClient()
		c.injector.Register("sshClient", sshClient)

		if vmDriver == "colima" {
			networkManager := c.constructors.NewColimaNetworkManager(c.injector)
			c.injector.Register("networkManager", networkManager)
		} else {
			networkManager := c.constructors.NewBaseNetworkManager(c.injector)
			c.injector.Register("networkManager", networkManager)
		}
	} else {
		networkManager := c.constructors.NewBaseNetworkManager(c.injector)
		c.injector.Register("networkManager", networkManager)
	}

	return nil
}

// createVirtualizationComponents creates virtualization components based on configuration.
func (c *BaseController) createVirtualizationComponents(req Requirements) error {
	if !req.VM && !req.Containers {
		return nil
	}

	vmDriver := c.ResolveConfigHandler().GetString("vm.driver")
	dockerEnabled := c.ResolveConfigHandler().GetBool("docker.enabled")

	// Create virtualization components based on configuration
	if req.VM && vmDriver == "colima" {
		if c.constructors.NewColimaVirt == nil {
			return fmt.Errorf("failed to create virtualization components: NewColimaVirt constructor is nil")
		}
		colimaVirtualMachine := c.constructors.NewColimaVirt(c.injector)
		if colimaVirtualMachine == nil {
			return fmt.Errorf("failed to create virtualization components: NewColimaVirt returned nil")
		}
		c.injector.Register("virtualMachine", colimaVirtualMachine)
	}

	if req.Containers && dockerEnabled {
		if c.constructors.NewDockerVirt == nil {
			return fmt.Errorf("failed to create Docker container runtime: NewDockerVirt constructor is nil")
		}
		containerRuntime := c.constructors.NewDockerVirt(c.injector)
		if containerRuntime == nil {
			return fmt.Errorf("failed to create Docker container runtime: NewDockerVirt returned nil")
		}
		c.injector.Register("containerRuntime", containerRuntime)
	}

	return nil
}

// createStackComponent creates and initializes the stack component if required
func (c *BaseController) createStackComponent(req Requirements) error {
	if !req.Stack {
		return nil
	}

	stackInstance := c.constructors.NewWindsorStack(c.injector)
	c.injector.Register("stack", stackInstance)

	return nil
}

// =============================================================================
// Interface compliance
// =============================================================================

var _ Controller = (*BaseController)(nil)
