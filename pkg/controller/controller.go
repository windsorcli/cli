package controller

import (
	"fmt"
	"net"
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
	"github.com/windsorcli/cli/pkg/shell"
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

type Controller interface {
	Initialize() error
	InitializeComponents() error
	CreateCommonComponents() error
	CreateSecretsProviders() error
	CreateProjectComponents() error
	CreateEnvComponents() error
	CreateServiceComponents() error
	CreateVirtualizationComponents() error
	CreateStackComponents() error
	ResolveInjector() di.Injector
	ResolveConfigHandler() config.ConfigHandler
	ResolveAllSecretsProviders() []secrets.SecretsProvider
	ResolveEnvPrinter(name string) env.EnvPrinter
	ResolveAllEnvPrinters() []env.EnvPrinter
	ResolveShell() shell.Shell
	ResolveSecureShell() shell.Shell
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
	injector      di.Injector
	configHandler config.ConfigHandler
	constructors  ComponentConstructors
}

// ComponentConstructors contains factory functions for all components used in the controller
type ComponentConstructors struct {
	// Common components
	NewYamlConfigHandler func(di.Injector) config.ConfigHandler
	NewDefaultShell      func(di.Injector) shell.Shell
	NewSecureShell       func(di.Injector) shell.Shell

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

// DefaultConstructors returns a ComponentConstructors with the default implementation
// of all factory functions
func DefaultConstructors() ComponentConstructors {
	return ComponentConstructors{
		// Common components
		NewYamlConfigHandler: func(injector di.Injector) config.ConfigHandler {
			return config.NewYamlConfigHandler(injector)
		},
		NewDefaultShell: func(injector di.Injector) shell.Shell {
			return shell.NewDefaultShell(injector)
		},
		NewSecureShell: func(injector di.Injector) shell.Shell {
			return shell.NewSecureShell(injector)
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

// MockConstructors returns a ComponentConstructors with all factory functions set to return mocks
// useful for testing
func MockConstructors() ComponentConstructors {
	return ComponentConstructors{
		// Common components
		NewYamlConfigHandler: func(injector di.Injector) config.ConfigHandler {
			return config.NewMockConfigHandler()
		},
		NewDefaultShell: func(injector di.Injector) shell.Shell {
			return shell.NewMockShell()
		},
		NewSecureShell: func(injector di.Injector) shell.Shell {
			return shell.NewMockShell()
		},

		// Project components
		NewGitGenerator: func(injector di.Injector) generators.Generator {
			return generators.NewMockGenerator()
		},
		NewBlueprintHandler: func(injector di.Injector) blueprint.BlueprintHandler {
			return blueprint.NewMockBlueprintHandler(injector)
		},
		NewTerraformGenerator: func(injector di.Injector) generators.Generator {
			return generators.NewMockGenerator()
		},
		NewKustomizeGenerator: func(injector di.Injector) generators.Generator {
			return generators.NewMockGenerator()
		},
		NewToolsManager: func(injector di.Injector) tools.ToolsManager {
			return tools.NewMockToolsManager()
		},

		// Environment printers
		NewAwsEnvPrinter: func(injector di.Injector) env.EnvPrinter {
			return env.NewMockEnvPrinter()
		},
		NewDockerEnvPrinter: func(injector di.Injector) env.EnvPrinter {
			return env.NewMockEnvPrinter()
		},
		NewKubeEnvPrinter: func(injector di.Injector) env.EnvPrinter {
			return env.NewMockEnvPrinter()
		},
		NewOmniEnvPrinter: func(injector di.Injector) env.EnvPrinter {
			return env.NewMockEnvPrinter()
		},
		NewTalosEnvPrinter: func(injector di.Injector) env.EnvPrinter {
			return env.NewMockEnvPrinter()
		},
		NewTerraformEnvPrinter: func(injector di.Injector) env.EnvPrinter {
			return env.NewMockEnvPrinter()
		},
		NewWindsorEnvPrinter: func(injector di.Injector) env.EnvPrinter {
			return env.NewMockEnvPrinter()
		},

		// Service components
		NewDNSService: func(injector di.Injector) services.Service {
			return services.NewMockService()
		},
		NewGitLivereloadService: func(injector di.Injector) services.Service {
			return services.NewMockService()
		},
		NewLocalstackService: func(injector di.Injector) services.Service {
			return services.NewMockService()
		},
		NewRegistryService: func(injector di.Injector) services.Service {
			return services.NewMockService()
		},
		NewTalosService: func(injector di.Injector, nodeType string) services.Service {
			return services.NewMockService()
		},

		// Virtualization components
		NewSSHClient: func() *ssh.SSHClient {
			return ssh.NewSSHClient()
		},
		NewColimaVirt: func(injector di.Injector) virt.VirtualMachine {
			return virt.NewMockVirt()
		},
		NewColimaNetworkManager: func(injector di.Injector) network.NetworkManager {
			return network.NewMockNetworkManager()
		},
		NewBaseNetworkManager: func(injector di.Injector) network.NetworkManager {
			return network.NewMockNetworkManager()
		},
		NewDockerVirt: func(_ di.Injector) virt.ContainerRuntime {
			return virt.NewMockVirt()
		},
		NewNetworkInterfaceProvider: func() network.NetworkInterfaceProvider {
			return &network.MockNetworkInterfaceProvider{
				InterfacesFunc: func() ([]net.Interface, error) {
					return nil, nil
				},
				InterfaceAddrsFunc: func(iface net.Interface) ([]net.Addr, error) {
					return nil, nil
				},
			}
		},

		// Secrets providers
		NewSopsSecretsProvider: func(configRoot string, injector di.Injector) secrets.SecretsProvider {
			return secrets.NewMockSecretsProvider(injector)
		},
		NewOnePasswordSDKSecretsProvider: func(vault secretsConfigType.OnePasswordVault, injector di.Injector) secrets.SecretsProvider {
			return secrets.NewMockSecretsProvider(injector)
		},
		NewOnePasswordCLISecretsProvider: func(vault secretsConfigType.OnePasswordVault, injector di.Injector) secrets.SecretsProvider {
			return secrets.NewMockSecretsProvider(injector)
		},

		// Stack components
		NewWindsorStack: func(injector di.Injector) stack.Stack {
			return stack.NewMockStack(injector)
		},
	}
}

// =============================================================================
// Constructor
// =============================================================================

// NewController creates a new controller.
func NewController(injector di.Injector, constructors ...ComponentConstructors) *BaseController {
	var c ComponentConstructors
	if len(constructors) > 0 {
		c = constructors[0]
	} else {
		c = DefaultConstructors()
	}

	return &BaseController{
		injector:     injector,
		constructors: c,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize the controller. Initializes the config handler
// as well.
func (c *BaseController) Initialize() error {
	configHandler := c.ResolveConfigHandler()
	c.configHandler = configHandler

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

// CreateCommonComponents creates the common components.
func (c *BaseController) CreateCommonComponents() error {
	if c.injector == nil {
		return fmt.Errorf("injector is nil")
	}
	if c.constructors.NewYamlConfigHandler == nil || c.constructors.NewDefaultShell == nil {
		return fmt.Errorf("required constructors are nil")
	}
	configHandler := c.constructors.NewYamlConfigHandler(c.injector)
	c.injector.Register("configHandler", configHandler)
	c.configHandler = configHandler

	shell := c.constructors.NewDefaultShell(c.injector)
	c.injector.Register("shell", shell)

	// Initialize the config handler
	if err := configHandler.Initialize(); err != nil {
		return fmt.Errorf("error initializing config handler: %w", err)
	}

	// Initialize the shell
	if err := shell.Initialize(); err != nil {
		return fmt.Errorf("error initializing shell: %w", err)
	}

	return nil
}

// CreateSecretsProviders sets up the secrets provider based on config settings.
// It supports SOPS and 1Password CLI for decryption.
// Registers the appropriate secrets provider with the injector and config handler.
func (c *BaseController) CreateSecretsProviders() error {
	contextName := c.configHandler.GetContext()
	configRoot, err := c.configHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	secretsFilePaths := []string{"secrets.enc.yaml", "secrets.enc.yml"}
	for _, filePath := range secretsFilePaths {
		if _, err := osStat(filepath.Join(configRoot, filePath)); err == nil {
			sopsSecretsProvider := c.constructors.NewSopsSecretsProvider(configRoot, c.injector)
			c.injector.Register("sopsSecretsProvider", sopsSecretsProvider)
			c.configHandler.SetSecretsProvider(sopsSecretsProvider)
		}
	}

	vaults, ok := c.configHandler.Get(fmt.Sprintf("contexts.%s.secrets.onepassword.vaults", contextName)).(map[string]secretsConfigType.OnePasswordVault)
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
			c.configHandler.SetSecretsProvider(opSecretsProvider)
		}
	}

	return nil
}

// CreateProjectComponents creates the project components.
func (c *BaseController) CreateProjectComponents() error {
	if c.injector == nil {
		return fmt.Errorf("injector is nil")
	}
	if c.configHandler == nil {
		return fmt.Errorf("config handler is nil")
	}

	gitGenerator := c.constructors.NewGitGenerator(c.injector)
	c.injector.Register("gitGenerator", gitGenerator)

	blueprintHandler := c.constructors.NewBlueprintHandler(c.injector)
	c.injector.Register("blueprintHandler", blueprintHandler)

	terraformGenerator := c.constructors.NewTerraformGenerator(c.injector)
	c.injector.Register("terraformGenerator", terraformGenerator)

	kustomizeGenerator := c.constructors.NewKustomizeGenerator(c.injector)
	c.injector.Register("kustomizeGenerator", kustomizeGenerator)

	toolsManagerType := c.configHandler.GetString("toolsManager")
	var toolsManager tools.ToolsManager

	if toolsManagerType == "" {
		var err error
		toolsManagerType, err = tools.CheckExistingToolsManager(c.configHandler.GetString("projectRoot"))
		if err != nil {
			return fmt.Errorf("error checking existing tools manager: %w", err)
		}
	}

	switch toolsManagerType {
	// Future implementations for different tools managers can go here
	default:
		toolsManager = c.constructors.NewToolsManager(c.injector)
	}

	c.injector.Register("toolsManager", toolsManager)

	return nil
}

// CreateEnvComponents creates the env components.
func (c *BaseController) CreateEnvComponents() error {
	if c.injector == nil {
		return fmt.Errorf("injector is nil")
	}
	if c.configHandler == nil {
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
		if key == "awsEnv" && !c.configHandler.GetBool("aws.enabled") {
			continue
		}
		if key == "dockerEnv" && !c.configHandler.GetBool("docker.enabled") {
			continue
		}
		envPrinter := constructor(c.injector)
		c.injector.Register(key, envPrinter)
	}

	return nil
}

// CreateServiceComponents creates the service components.
func (c *BaseController) CreateServiceComponents() error {
	configHandler := c.configHandler
	contextConfig := configHandler.GetConfig()

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

	if contextConfig.Docker != nil && contextConfig.Docker.Registries != nil {
		for key := range contextConfig.Docker.Registries {
			service := c.constructors.NewRegistryService(c.injector)
			service.SetName(key)
			serviceName := fmt.Sprintf("registryService.%s", key)
			c.injector.Register(serviceName, service)
		}
	}

	clusterEnabled := configHandler.GetBool("cluster.enabled")
	if clusterEnabled {
		controlPlaneCount := configHandler.GetInt("cluster.controlplanes.count")
		workerCount := configHandler.GetInt("cluster.workers.count")

		clusterDriver := configHandler.GetString("cluster.driver")

		if clusterDriver == "talos" {
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

// CreateVirtualizationComponents creates virtualization components based on configuration.
func (c *BaseController) CreateVirtualizationComponents() error {
	configHandler := c.ResolveConfigHandler()

	vmDriver := configHandler.GetString("vm.driver")
	dockerEnabled := configHandler.GetBool("docker.enabled")

	if vmDriver == "colima" {
		// Create and register NetworkInterfaceProvider
		networkInterfaceProvider := c.constructors.NewNetworkInterfaceProvider()
		c.injector.Register("networkInterfaceProvider", networkInterfaceProvider)

		// Create secure shell
		secureShell := c.constructors.NewSecureShell(c.injector)
		c.injector.Register("secureShell", secureShell)

		// Create SSH client
		sshClient := c.constructors.NewSSHClient()
		c.injector.Register("sshClient", sshClient)

		// Create Colima virtual machine
		colimaVirtualMachine := c.constructors.NewColimaVirt(c.injector)
		c.injector.Register("virtualMachine", colimaVirtualMachine)

		// Create Colima network manager
		networkManager := c.constructors.NewColimaNetworkManager(c.injector)
		c.injector.Register("networkManager", networkManager)
	} else {
		// Create base network manager
		networkManager := c.constructors.NewBaseNetworkManager(c.injector)
		c.injector.Register("networkManager", networkManager)
	}

	if dockerEnabled {
		// Create Docker virtualization
		containerRuntime := c.constructors.NewDockerVirt(c.injector)
		c.injector.Register("containerRuntime", containerRuntime)
	}

	return nil
}

// CreateStackComponents creates the stack components.
func (c *BaseController) CreateStackComponents() error {
	// Create Windsor stack
	stackInstance := c.constructors.NewWindsorStack(c.injector)
	c.injector.Register("stack", stackInstance)

	return nil
}

// WriteConfigurationFiles writes the configuration files.
func (c *BaseController) WriteConfigurationFiles() error {
	// Resolve all services
	resolvedServices := c.ResolveAllServices()

	// Write tools manifest
	toolsManager := c.ResolveToolsManager()
	if toolsManager != nil {
		if err := toolsManager.WriteManifest(); err != nil {
			return fmt.Errorf("error writing tools manifest: %w", err)
		}
	}

	// Write blueprint
	blueprintHandler := c.ResolveBlueprintHandler()
	if blueprintHandler != nil {
		if err := blueprintHandler.WriteConfig(); err != nil {
			return fmt.Errorf("error writing blueprint config: %w", err)
		}
	}

	// Write configuration for all services
	for _, service := range resolvedServices {
		if service != nil {
			if err := service.WriteConfig(); err != nil {
				return fmt.Errorf("error writing service config: %w", err)
			}
		}
	}

	// Resolve and write configuration for virtual machine if vm.driver is defined
	if vmDriver := c.configHandler.GetString("vm.driver"); vmDriver != "" {
		resolvedVirt := c.ResolveVirtualMachine()
		if resolvedVirt != nil {
			if err := resolvedVirt.WriteConfig(); err != nil {
				return fmt.Errorf("error writing virtual machine config: %w", err)
			}
		}
	}

	// Resolve and write configuration for container runtime if docker.enabled is true
	if dockerEnabled := c.configHandler.GetBool("docker.enabled"); dockerEnabled {
		resolvedContainerRuntime := c.ResolveContainerRuntime()
		if resolvedContainerRuntime != nil {
			if err := resolvedContainerRuntime.WriteConfig(); err != nil {
				return fmt.Errorf("error writing container runtime config: %w", err)
			}
		}
	}

	// Resolve and write configuration for all generators
	generators := c.ResolveAllGenerators()
	for _, generator := range generators {
		if generator != nil {
			if err := generator.Write(); err != nil {
				return fmt.Errorf("error writing generator config: %w", err)
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
func (c *BaseController) ResolveShell() shell.Shell {
	instance := c.injector.Resolve("shell")
	shellInstance, _ := instance.(shell.Shell)
	return shellInstance
}

// ResolveSecureShell resolves the secureShell instance.
func (c *BaseController) ResolveSecureShell() shell.Shell {
	instance := c.injector.Resolve("secureShell")
	shellInstance, _ := instance.(shell.Shell)
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

// Ensure BaseController implements the Controller interface
var _ Controller = (*BaseController)(nil)
