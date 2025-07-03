package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	bundler "github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/cluster"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/generators"
	"github.com/windsorcli/cli/pkg/kubernetes"
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
// infrastructure and application components. It provides a unified interface for resolving and managing
// dependencies, configurations, and resource orchestration across different environments. The Controller
// serves as the primary coordinator for all Windsor CLI operations, ensuring proper initialization,
// configuration, and lifecycle management of all system components.

// =============================================================================
// Types
// =============================================================================

// Controller defines the interface for managing Windsor CLI components and operations.
// It provides methods for component initialization, dependency resolution, and environment management.
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
	ResolveArtifactBuilder() bundler.Artifact
	ResolveAllBundlers() []bundler.Bundler
	WriteConfigurationFiles() error
	SetEnvironmentVariables() error
	ResolveKubernetesManager() kubernetes.KubernetesManager
	ResolveClusterClient() cluster.ClusterClient
}

// BaseController implements the Controller interface with default component management
// It provides concrete implementations of all Controller methods and manages component lifecycle
type BaseController struct {
	injector     di.Injector
	constructors ComponentConstructors
	requirements Requirements
}

// ComponentConstructors contains factory functions for creating all Windsor CLI components
// Each field represents a constructor function for a specific component type
type ComponentConstructors struct {
	NewConfigHandler func(di.Injector) config.ConfigHandler
	NewShell         func(di.Injector) sh.Shell
	NewSecureShell   func(di.Injector) sh.Shell

	NewGitGenerator       func(di.Injector) generators.Generator
	NewBlueprintHandler   func(di.Injector) blueprint.BlueprintHandler
	NewTerraformGenerator func(di.Injector) generators.Generator
	NewKustomizeGenerator func(di.Injector) generators.Generator
	NewToolsManager       func(di.Injector) tools.ToolsManager
	NewKubernetesManager  func(di.Injector) kubernetes.KubernetesManager
	NewKubernetesClient   func(di.Injector) kubernetes.KubernetesClient

	NewAwsEnvPrinter       func(di.Injector) env.EnvPrinter
	NewAzureEnvPrinter     func(di.Injector) env.EnvPrinter
	NewDockerEnvPrinter    func(di.Injector) env.EnvPrinter
	NewKubeEnvPrinter      func(di.Injector) env.EnvPrinter
	NewOmniEnvPrinter      func(di.Injector) env.EnvPrinter
	NewTalosEnvPrinter     func(di.Injector) env.EnvPrinter
	NewTerraformEnvPrinter func(di.Injector) env.EnvPrinter
	NewWindsorEnvPrinter   func(di.Injector) env.EnvPrinter

	NewDNSService           func(di.Injector) services.Service
	NewGitLivereloadService func(di.Injector) services.Service
	NewLocalstackService    func(di.Injector) services.Service
	NewRegistryService      func(di.Injector) services.Service
	NewTalosService         func(di.Injector, string) services.Service

	NewSSHClient                func() *ssh.SSHClient
	NewColimaVirt               func(di.Injector) virt.VirtualMachine
	NewColimaNetworkManager     func(di.Injector) network.NetworkManager
	NewBaseNetworkManager       func(di.Injector) network.NetworkManager
	NewDockerVirt               func(di.Injector) virt.ContainerRuntime
	NewNetworkInterfaceProvider func() network.NetworkInterfaceProvider

	NewSopsSecretsProvider           func(string, di.Injector) secrets.SecretsProvider
	NewOnePasswordSDKSecretsProvider func(secretsConfigType.OnePasswordVault, di.Injector) secrets.SecretsProvider
	NewOnePasswordCLISecretsProvider func(secretsConfigType.OnePasswordVault, di.Injector) secrets.SecretsProvider

	NewWindsorStack func(di.Injector) stack.Stack

	NewTalosClusterClient func(di.Injector) *cluster.TalosClusterClient

	NewArtifactBuilder  func(di.Injector) bundler.Artifact
	NewTemplateBundler  func(di.Injector) bundler.Bundler
	NewKustomizeBundler func(di.Injector) bundler.Bundler
	NewTerraformBundler func(di.Injector) bundler.Bundler
}

// Requirements defines the operational requirements for the controller
// It specifies which components and capabilities are needed for a given operation
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
	Cluster    bool // Needs Kubernetes manager

	// Service requirements
	Services bool // Needs service management

	// Project requirements
	Tools      bool // Needs git, terraform, etc.
	Blueprint  bool // Needs blueprint handling
	Generators bool // Needs code generation
	Stack      bool // Needs stack components
	Bundler    bool // Needs bundler and artifact functionality

	// Command info for context-specific decisions
	CommandName string          // Name of the command
	Flags       map[string]bool // Important flags that affect initialization
	Reset       bool            // Whether to reset/overwrite existing files
}

// =============================================================================
// Constructor
// =============================================================================

// NewController creates a new BaseController instance with the provided dependency injector
// It initializes the controller with default component constructors
func NewController(injector di.Injector) *BaseController {
	return &BaseController{
		injector:     injector,
		constructors: NewDefaultConstructors(),
	}
}

// NewDefaultConstructors creates a ComponentConstructors instance with default implementations
// It provides factory functions for all Windsor CLI components
func NewDefaultConstructors() ComponentConstructors {
	return ComponentConstructors{
		NewConfigHandler: func(injector di.Injector) config.ConfigHandler {
			return config.NewYamlConfigHandler(injector)
		},
		NewShell: func(injector di.Injector) sh.Shell {
			return sh.NewDefaultShell(injector)
		},
		NewSecureShell: func(injector di.Injector) sh.Shell {
			return sh.NewSecureShell(injector)
		},

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
		NewKubernetesManager: func(injector di.Injector) kubernetes.KubernetesManager {
			return kubernetes.NewKubernetesManager(injector)
		},
		NewKubernetesClient: func(injector di.Injector) kubernetes.KubernetesClient {
			return kubernetes.NewDynamicKubernetesClient()
		},
		NewAwsEnvPrinter: func(injector di.Injector) env.EnvPrinter {
			return env.NewAwsEnvPrinter(injector)
		},
		NewAzureEnvPrinter: func(injector di.Injector) env.EnvPrinter {
			return env.NewAzureEnvPrinter(injector)
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

		NewSopsSecretsProvider: func(secretsFile string, injector di.Injector) secrets.SecretsProvider {
			return secrets.NewSopsSecretsProvider(secretsFile, injector)
		},
		NewOnePasswordSDKSecretsProvider: func(vault secretsConfigType.OnePasswordVault, injector di.Injector) secrets.SecretsProvider {
			return secrets.NewOnePasswordSDKSecretsProvider(vault, injector)
		},
		NewOnePasswordCLISecretsProvider: func(vault secretsConfigType.OnePasswordVault, injector di.Injector) secrets.SecretsProvider {
			return secrets.NewOnePasswordCLISecretsProvider(vault, injector)
		},

		NewWindsorStack: func(injector di.Injector) stack.Stack {
			return stack.NewWindsorStack(injector)
		},

		NewTalosClusterClient: func(injector di.Injector) *cluster.TalosClusterClient {
			return cluster.NewTalosClusterClient(injector)
		},

		NewArtifactBuilder: func(injector di.Injector) bundler.Artifact {
			return bundler.NewArtifactBuilder()
		},
		NewTemplateBundler: func(injector di.Injector) bundler.Bundler {
			return bundler.NewTemplateBundler()
		},
		NewKustomizeBundler: func(injector di.Injector) bundler.Bundler {
			return bundler.NewKustomizeBundler()
		},
		NewTerraformBundler: func(injector di.Injector) bundler.Bundler {
			return bundler.NewTerraformBundler()
		},
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// SetRequirements configures the controller with specific operational requirements
// It stores the requirements for use during component creation and initialization
func (c *BaseController) SetRequirements(req Requirements) {
	c.requirements = req
}

// CreateComponents creates all required components based on the current requirements
// It creates components in a specific order to ensure proper dependency resolution
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
		{"kubernetes", c.createClusterComponents},
		{"virtualization", c.createVirtualizationComponents},
		{"service", c.createServiceComponents},
		{"network", c.createNetworkComponents},
		{"stack", c.createStackComponent},
		{"blueprint", c.createBlueprintComponent},
		{"bundler", c.createBundlerComponents},
	}

	for _, cc := range componentCreators {
		if err := cc.fn(req); err != nil {
			return fmt.Errorf("failed to create %s components: %w", cc.name, err)
		}
	}

	return nil
}

// InitializeComponents performs initialization of all created components
// It initializes each component in the correct order to maintain dependencies
func (c *BaseController) InitializeComponents() error {
	shell := c.ResolveShell()
	if shell != nil {
		if err := shell.Initialize(); err != nil {
			return fmt.Errorf("error initializing shell: %w", err)
		}
	}

	secureShell := c.ResolveSecureShell()
	if secureShell != nil {
		if err := secureShell.Initialize(); err != nil {
			return fmt.Errorf("error initializing secure shell: %w", err)
		}
	}

	secretsProviders := c.ResolveAllSecretsProviders()
	if len(secretsProviders) > 0 {
		for _, secretsProvider := range secretsProviders {
			if err := secretsProvider.Initialize(); err != nil {
				return fmt.Errorf("error initializing secrets provider: %w", err)
			}
		}
	}

	envPrinters := c.ResolveAllEnvPrinters()
	if len(envPrinters) > 0 {
		for _, envPrinter := range envPrinters {
			if err := envPrinter.Initialize(); err != nil {
				return fmt.Errorf("error initializing env printer: %w", err)
			}
		}
	}

	toolsManager := c.ResolveToolsManager()
	if toolsManager != nil {
		if err := toolsManager.Initialize(); err != nil {
			return fmt.Errorf("error initializing tools manager: %w", err)
		}
	}

	services := c.ResolveAllServices()
	if len(services) > 0 {
		for _, service := range services {
			if err := service.Initialize(); err != nil {
				return fmt.Errorf("error initializing service: %w", err)
			}
		}
	}

	virtualMachine := c.ResolveVirtualMachine()
	if virtualMachine != nil {
		if err := virtualMachine.Initialize(); err != nil {
			return fmt.Errorf("error initializing virtual machine: %w", err)
		}
	}

	containerRuntime := c.ResolveContainerRuntime()
	if containerRuntime != nil {
		if err := containerRuntime.Initialize(); err != nil {
			return fmt.Errorf("error initializing container runtime: %w", err)
		}
	}

	networkManager := c.ResolveNetworkManager()
	if networkManager != nil {
		if err := networkManager.Initialize(); err != nil {
			return fmt.Errorf("error initializing network manager: %w", err)
		}
	}

	blueprintHandler := c.ResolveBlueprintHandler()
	if blueprintHandler != nil {
		if err := blueprintHandler.Initialize(); err != nil {
			return fmt.Errorf("error initializing blueprint handler: %w", err)
		}
		if err := blueprintHandler.LoadConfig(c.requirements.Reset); err != nil {
			return fmt.Errorf("error loading blueprint config: %w", err)
		}
	}

	generators := c.ResolveAllGenerators()
	if len(generators) > 0 {
		for _, generator := range generators {
			if err := generator.Initialize(); err != nil {
				return fmt.Errorf("error initializing generator: %w", err)
			}
		}
	}

	artifactBuilder := c.ResolveArtifactBuilder()
	if artifactBuilder != nil {
		if err := artifactBuilder.Initialize(c.injector); err != nil {
			return fmt.Errorf("error initializing artifact builder: %w", err)
		}
	}

	bundlers := c.ResolveAllBundlers()
	if len(bundlers) > 0 {
		for _, bundler := range bundlers {
			if err := bundler.Initialize(c.injector); err != nil {
				return fmt.Errorf("error initializing bundler: %w", err)
			}
		}
	}

	stack := c.ResolveStack()
	if stack != nil {
		if err := stack.Initialize(); err != nil {
			return fmt.Errorf("error initializing stack: %w", err)
		}
	}

	return nil
}

// InitializeWithRequirements sets requirements and initializes components in one step
// It provides a standard initialization sequence used by commands
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

// WriteConfigurationFiles writes all component configurations to disk
// It handles configuration for tools, blueprints, services, and infrastructure components
func (c *BaseController) WriteConfigurationFiles() error {
	req := c.requirements

	if req.Tools {
		toolsManager := c.ResolveToolsManager()
		if toolsManager != nil {
			if err := toolsManager.WriteManifest(); err != nil {
				return fmt.Errorf("error writing tools manifest: %w", err)
			}
		}
	}

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

	if req.Generators {
		generators := c.ResolveAllGenerators()
		for _, generator := range generators {
			if generator != nil {
				if err := generator.Write(req.Reset); err != nil {
					return fmt.Errorf("error writing generator config: %w", err)
				}
			}
		}
	}

	return nil
}

// ResolveInjector returns the dependency injection container
// It provides access to the injector for component resolution
func (c *BaseController) ResolveInjector() di.Injector {
	return c.injector
}

// ResolveConfigHandler returns the configuration management component
// It retrieves the config handler from the dependency injection container
func (c *BaseController) ResolveConfigHandler() config.ConfigHandler {
	instance := c.injector.Resolve("configHandler")
	configHandler, _ := instance.(config.ConfigHandler)
	return configHandler
}

// ResolveAllSecretsProviders returns all configured secrets providers
// It retrieves all secrets providers from the dependency injection container
func (c *BaseController) ResolveAllSecretsProviders() []secrets.SecretsProvider {
	instances, _ := c.injector.ResolveAll((*secrets.SecretsProvider)(nil))
	secretsProviders := make([]secrets.SecretsProvider, 0, len(instances))

	for _, instance := range instances {
		secretsProvider, _ := instance.(secrets.SecretsProvider)
		secretsProviders = append(secretsProviders, secretsProvider)
	}

	return secretsProviders
}

// ResolveEnvPrinter returns a specific environment printer by name
// It retrieves the requested environment printer from the dependency injection container
func (c *BaseController) ResolveEnvPrinter(name string) env.EnvPrinter {
	instance := c.injector.Resolve(name)
	envPrinter, _ := instance.(env.EnvPrinter)
	return envPrinter
}

// ResolveAllEnvPrinters returns all configured environment printers
// It retrieves all environment printers from the dependency injection container
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

// ResolveShell returns the default shell component
// It retrieves the shell from the dependency injection container
func (c *BaseController) ResolveShell() sh.Shell {
	instance := c.injector.Resolve("shell")
	shellInstance, _ := instance.(sh.Shell)
	return shellInstance
}

// ResolveSecureShell returns the secure shell component
// It retrieves the secure shell from the dependency injection container
func (c *BaseController) ResolveSecureShell() sh.Shell {
	instance := c.injector.Resolve("secureShell")
	shellInstance, _ := instance.(sh.Shell)
	return shellInstance
}

// ResolveNetworkManager returns the network management component
// It retrieves the network manager from the dependency injection container
func (c *BaseController) ResolveNetworkManager() network.NetworkManager {
	instance := c.injector.Resolve("networkManager")
	networkManager, _ := instance.(network.NetworkManager)
	return networkManager
}

// ResolveToolsManager returns the tools management component
// It retrieves the tools manager from the dependency injection container
func (c *BaseController) ResolveToolsManager() tools.ToolsManager {
	instance := c.injector.Resolve("toolsManager")
	toolsManager, _ := instance.(tools.ToolsManager)
	return toolsManager
}

// ResolveBlueprintHandler returns the blueprint management component
// It retrieves the blueprint handler from the dependency injection container
func (c *BaseController) ResolveBlueprintHandler() blueprint.BlueprintHandler {
	instance := c.injector.Resolve("blueprintHandler")
	blueprintHandler, _ := instance.(blueprint.BlueprintHandler)
	return blueprintHandler
}

// ResolveService returns a specific service by name
// It retrieves the requested service from the dependency injection container
func (c *BaseController) ResolveService(name string) services.Service {
	instance := c.injector.Resolve(fmt.Sprintf("%s", name))
	service, _ := instance.(services.Service)
	return service
}

// ResolveAllServices returns all configured services
// It retrieves all services from the dependency injection container
func (c *BaseController) ResolveAllServices() []services.Service {
	instances, _ := c.injector.ResolveAll((*services.Service)(nil))
	servicesInstances := make([]services.Service, 0, len(instances))
	for _, instance := range instances {
		serviceInstance, _ := instance.(services.Service)
		servicesInstances = append(servicesInstances, serviceInstance)
	}
	return servicesInstances
}

// ResolveVirtualMachine returns the virtual machine component
// It retrieves the virtual machine from the dependency injection container
func (c *BaseController) ResolveVirtualMachine() virt.VirtualMachine {
	instance := c.injector.Resolve("virtualMachine")
	virtualMachine, _ := instance.(virt.VirtualMachine)
	return virtualMachine
}

// ResolveContainerRuntime returns the container runtime component
// It retrieves the container runtime from the dependency injection container
func (c *BaseController) ResolveContainerRuntime() virt.ContainerRuntime {
	instance := c.injector.Resolve("containerRuntime")
	containerRuntime, _ := instance.(virt.ContainerRuntime)
	return containerRuntime
}

// ResolveStack returns the stack management component
// It retrieves the stack from the dependency injection container
func (c *BaseController) ResolveStack() stack.Stack {
	instance := c.injector.Resolve("stack")
	stackInstance, _ := instance.(stack.Stack)
	return stackInstance
}

// ResolveAllGenerators returns all configured code generators
// It retrieves all generators from the dependency injection container
func (c *BaseController) ResolveAllGenerators() []generators.Generator {
	instances, _ := c.injector.ResolveAll((*generators.Generator)(nil))
	generatorsInstances := make([]generators.Generator, 0, len(instances))
	for _, instance := range instances {
		generatorInstance, _ := instance.(generators.Generator)
		generatorsInstances = append(generatorsInstances, generatorInstance)
	}
	return generatorsInstances
}

// ResolveArtifactBuilder returns the artifact builder component
// It retrieves the artifact builder from the dependency injection container
func (c *BaseController) ResolveArtifactBuilder() bundler.Artifact {
	instance := c.injector.Resolve("artifactBuilder")
	artifactBuilder, _ := instance.(bundler.Artifact)
	return artifactBuilder
}

// ResolveAllBundlers returns all configured bundlers
// It retrieves all bundlers from the dependency injection container
func (c *BaseController) ResolveAllBundlers() []bundler.Bundler {
	instances, _ := c.injector.ResolveAll((*bundler.Bundler)(nil))
	bundlerInstances := make([]bundler.Bundler, 0, len(instances))
	for _, instance := range instances {
		bundlerInstance, _ := instance.(bundler.Bundler)
		bundlerInstances = append(bundlerInstances, bundlerInstance)
	}
	return bundlerInstances
}

// SetEnvironmentVariables configures the environment for all components
// It sets environment variables from all configured environment printers
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

// ResolveKubernetesManager returns the Kubernetes manager component
// It retrieves the Kubernetes manager from the dependency injection container
func (c *BaseController) ResolveKubernetesManager() kubernetes.KubernetesManager {
	instance := c.injector.Resolve("kubernetesManager")
	manager, _ := instance.(kubernetes.KubernetesManager)
	return manager
}

// ResolveClusterClient returns the cluster client component
// It retrieves the cluster client from the dependency injection container
func (c *BaseController) ResolveClusterClient() cluster.ClusterClient {
	instance := c.injector.Resolve("clusterClient")
	client, _ := instance.(cluster.ClusterClient)
	return client
}

// =============================================================================
// Private Methods
// =============================================================================

// createShellComponent creates and initializes the shell component if required
// It handles shell creation, verbosity settings, and trusted directory checks
func (c *BaseController) createShellComponent(req Requirements) error {
	if c.constructors.NewShell == nil {
		return fmt.Errorf("required constructor NewShell is nil")
	}

	if existingShell := c.ResolveShell(); existingShell != nil {
		if verbose, ok := req.Flags["verbose"]; ok && verbose {
			existingShell.SetVerbosity(true)
		}
		return nil
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
// It handles config loading, initialization, and context management
func (c *BaseController) createConfigComponent(req Requirements) error {
	if c.constructors.NewConfigHandler == nil {
		return fmt.Errorf("required constructor NewConfigHandler is nil")
	}

	if existingConfigHandler := c.ResolveConfigHandler(); existingConfigHandler != nil {
		if req.ConfigLoaded && !existingConfigHandler.IsLoaded() {
			fmt.Fprintln(os.Stderr, "Cannot execute commands. Please run 'windsor init' to set up your project first.")
		}
		return nil
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
// It sets up SOPS and OnePassword secrets providers based on configuration
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
			if existingProvider := c.injector.Resolve("sopsSecretsProvider"); existingProvider == nil {
				sopsSecretsProvider := c.constructors.NewSopsSecretsProvider(configRoot, c.injector)
				c.injector.Register("sopsSecretsProvider", sopsSecretsProvider)
				configHandler.SetSecretsProvider(sopsSecretsProvider)
			}
			break
		}
	}

	vaults, ok := configHandler.Get(fmt.Sprintf("contexts.%s.secrets.onepassword.vaults", contextName)).(map[string]secretsConfigType.OnePasswordVault)
	if ok && len(vaults) > 0 {
		useSDK := os.Getenv("OP_SERVICE_ACCOUNT_TOKEN") != ""

		for key, vault := range vaults {
			vault.ID = key
			providerName := fmt.Sprintf("op%sSecretsProvider", strings.ToUpper(key[:1])+key[1:])

			if existingProvider := c.injector.Resolve(providerName); existingProvider == nil {
				var opSecretsProvider secrets.SecretsProvider

				if useSDK {
					opSecretsProvider = c.constructors.NewOnePasswordSDKSecretsProvider(vault, c.injector)
				} else {
					opSecretsProvider = c.constructors.NewOnePasswordCLISecretsProvider(vault, c.injector)
				}

				c.injector.Register(providerName, opSecretsProvider)
				configHandler.SetSecretsProvider(opSecretsProvider)
			}
		}
	}

	return nil
}

// createToolsComponents creates and initializes project tools if required
// It sets up the tools manager based on configuration and existing tools
func (c *BaseController) createToolsComponents(req Requirements) error {
	if !req.Tools {
		return nil
	}

	if existingToolsManager := c.ResolveToolsManager(); existingToolsManager != nil {
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
// It sets up Git, Terraform, and Kustomize generators based on requirements
func (c *BaseController) createGeneratorsComponents(req Requirements) error {
	if !req.Generators {
		return nil
	}

	existingGenerators := c.ResolveAllGenerators()
	existingGeneratorNames := make(map[string]bool)

	for _, generator := range existingGenerators {
		if c.injector.Resolve("gitGenerator") == generator {
			existingGeneratorNames["gitGenerator"] = true
		} else if c.injector.Resolve("terraformGenerator") == generator {
			existingGeneratorNames["terraformGenerator"] = true
		} else if c.injector.Resolve("kustomizeGenerator") == generator {
			existingGeneratorNames["kustomizeGenerator"] = true
		}
	}

	if !existingGeneratorNames["gitGenerator"] {
		gitGenerator := c.constructors.NewGitGenerator(c.injector)
		c.injector.Register("gitGenerator", gitGenerator)
	}

	if req.Blueprint {
		if !existingGeneratorNames["terraformGenerator"] {
			terraformGenerator := c.constructors.NewTerraformGenerator(c.injector)
			c.injector.Register("terraformGenerator", terraformGenerator)
		}

		if !existingGeneratorNames["kustomizeGenerator"] {
			kustomizeGenerator := c.constructors.NewKustomizeGenerator(c.injector)
			c.injector.Register("kustomizeGenerator", kustomizeGenerator)
		}
	}

	return nil
}

// createBlueprintComponent creates and initializes the blueprint handler if required
// It sets up the blueprint handler for managing project blueprints
func (c *BaseController) createBlueprintComponent(req Requirements) error {
	if !req.Blueprint {
		return nil
	}

	if existingBlueprintHandler := c.ResolveBlueprintHandler(); existingBlueprintHandler != nil {
		return nil
	}

	blueprintHandler := c.constructors.NewBlueprintHandler(c.injector)
	c.injector.Register("blueprintHandler", blueprintHandler)

	return nil
}

// createEnvComponents creates and initializes environment components if required
// It sets up environment printers for different platforms and services
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
		"azureEnv":     c.constructors.NewAzureEnvPrinter,
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
		if key == "azureEnv" && !configHandler.GetBool("azure.enabled") {
			continue
		}
		if key == "dockerEnv" && !configHandler.GetBool("docker.enabled") {
			continue
		}

		if existingEnvPrinter := c.ResolveEnvPrinter(key); existingEnvPrinter == nil {
			envPrinter := constructor(c.injector)
			c.injector.Register(key, envPrinter)
		}
	}

	return nil
}

// createServiceComponents creates and initializes service components if required
// It sets up DNS, Git livereload, Localstack, and registry services
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
		if existingService := c.ResolveService("dnsService"); existingService == nil {
			dnsService := c.constructors.NewDNSService(c.injector)
			dnsService.SetName("dns")
			c.injector.Register("dnsService", dnsService)
		}
	}

	gitLivereloadEnabled := configHandler.GetBool("git.livereload.enabled")
	if gitLivereloadEnabled {
		if existingService := c.ResolveService("gitLivereloadService"); existingService == nil {
			gitLivereloadService := c.constructors.NewGitLivereloadService(c.injector)
			gitLivereloadService.SetName("git")
			c.injector.Register("gitLivereloadService", gitLivereloadService)
		}
	}

	localstackEnabled := configHandler.GetBool("aws.localstack.enabled")
	if localstackEnabled {
		if existingService := c.ResolveService("localstackService"); existingService == nil {
			localstackService := c.constructors.NewLocalstackService(c.injector)
			localstackService.SetName("aws")
			c.injector.Register("localstackService", localstackService)
		}
	}

	contextConfig := configHandler.GetConfig()
	if contextConfig.Docker != nil && contextConfig.Docker.Registries != nil {
		for key := range contextConfig.Docker.Registries {
			serviceName := fmt.Sprintf("registryService.%s", key)
			if existingService := c.ResolveService(serviceName); existingService == nil {
				service := c.constructors.NewRegistryService(c.injector)
				service.SetName(key)
				c.injector.Register(serviceName, service)
			}
		}
	}

	if configHandler.GetBool("cluster.enabled") {
		clusterDriver := configHandler.GetString("cluster.driver")
		if clusterDriver == "talos" {
			controlPlaneCount := configHandler.GetInt("cluster.controlplanes.count")
			workerCount := configHandler.GetInt("cluster.workers.count")

			for i := 1; i <= controlPlaneCount; i++ {
				serviceName := fmt.Sprintf("clusterNode.controlplane-%d", i)
				if existingService := c.ResolveService(serviceName); existingService == nil {
					controlPlaneService := c.constructors.NewTalosService(c.injector, "controlplane")
					controlPlaneService.SetName(fmt.Sprintf("controlplane-%d", i))
					c.injector.Register(serviceName, controlPlaneService)
				}
			}

			for i := 1; i <= workerCount; i++ {
				serviceName := fmt.Sprintf("clusterNode.worker-%d", i)
				if existingService := c.ResolveService(serviceName); existingService == nil {
					workerService := c.constructors.NewTalosService(c.injector, "worker")
					workerService.SetName(fmt.Sprintf("worker-%d", i))
					c.injector.Register(serviceName, workerService)
				}
			}
		}
	}

	return nil
}

// createNetworkComponents creates and initializes network components based on configuration
// It sets up network interface providers and managers for different VM drivers
func (c *BaseController) createNetworkComponents(req Requirements) error {
	if !req.Network {
		return nil
	}

	vmDriver := c.ResolveConfigHandler().GetString("vm.driver")

	if existingProvider := c.injector.Resolve("networkInterfaceProvider"); existingProvider == nil {
		networkInterfaceProvider := c.constructors.NewNetworkInterfaceProvider()
		c.injector.Register("networkInterfaceProvider", networkInterfaceProvider)
	}

	if req.VM {
		if existingSecureShell := c.ResolveSecureShell(); existingSecureShell == nil {
			secureShell := c.constructors.NewSecureShell(c.injector)
			c.injector.Register("secureShell", secureShell)
		}

		if existingSSHClient := c.injector.Resolve("sshClient"); existingSSHClient == nil {
			sshClient := c.constructors.NewSSHClient()
			c.injector.Register("sshClient", sshClient)
		}

		if existingNetworkManager := c.ResolveNetworkManager(); existingNetworkManager == nil {
			if vmDriver == "colima" {
				networkManager := c.constructors.NewColimaNetworkManager(c.injector)
				c.injector.Register("networkManager", networkManager)
			} else {
				networkManager := c.constructors.NewBaseNetworkManager(c.injector)
				c.injector.Register("networkManager", networkManager)
			}
		}
	} else {
		if existingNetworkManager := c.ResolveNetworkManager(); existingNetworkManager == nil {
			networkManager := c.constructors.NewBaseNetworkManager(c.injector)
			c.injector.Register("networkManager", networkManager)
		}
	}

	return nil
}

// createVirtualizationComponents creates virtualization components based on configuration
// It sets up virtual machines and container runtimes for different platforms
func (c *BaseController) createVirtualizationComponents(req Requirements) error {
	if !req.VM && !req.Containers {
		return nil
	}

	vmDriver := c.ResolveConfigHandler().GetString("vm.driver")
	dockerEnabled := c.ResolveConfigHandler().GetBool("docker.enabled")

	if req.VM && vmDriver == "colima" {
		if existingVM := c.ResolveVirtualMachine(); existingVM == nil {
			if c.constructors.NewColimaVirt == nil {
				return fmt.Errorf("failed to create virtualization components: NewColimaVirt constructor is nil")
			}
			colimaVirtualMachine := c.constructors.NewColimaVirt(c.injector)
			if colimaVirtualMachine == nil {
				return fmt.Errorf("failed to create virtualization components: NewColimaVirt returned nil")
			}
			c.injector.Register("virtualMachine", colimaVirtualMachine)
		}
	}

	if req.Containers && dockerEnabled {
		if existingContainerRuntime := c.ResolveContainerRuntime(); existingContainerRuntime == nil {
			if c.constructors.NewDockerVirt == nil {
				return fmt.Errorf("failed to create Docker container runtime: NewDockerVirt constructor is nil")
			}
			containerRuntime := c.constructors.NewDockerVirt(c.injector)
			if containerRuntime == nil {
				return fmt.Errorf("failed to create Docker container runtime: NewDockerVirt returned nil")
			}
			c.injector.Register("containerRuntime", containerRuntime)
		}
	}

	return nil
}

// createStackComponent creates and initializes the stack component if required
// It sets up the stack manager for handling infrastructure stacks
func (c *BaseController) createStackComponent(req Requirements) error {
	if !req.Stack {
		return nil
	}

	if existingStack := c.ResolveStack(); existingStack == nil {
		stackInstance := c.constructors.NewWindsorStack(c.injector)
		c.injector.Register("stack", stackInstance)
	}

	return nil
}

// createClusterComponents creates and initializes the cluster components if required
// It sets up the cluster manager for managing cluster operations
func (c *BaseController) createClusterComponents(req Requirements) error {
	if !req.Cluster {
		return nil
	}

	configHandler := c.ResolveConfigHandler()
	if configHandler == nil {
		return fmt.Errorf("config handler is nil")
	}

	// Create Kubernetes components
	if existingManager := c.ResolveKubernetesManager(); existingManager != nil {
		return nil
	}

	if c.constructors.NewKubernetesManager == nil {
		return fmt.Errorf("failed to create kubernetes components: NewKubernetesManager constructor is nil")
	}

	if c.constructors.NewKubernetesClient == nil {
		return fmt.Errorf("failed to create kubernetes components: NewKubernetesClient constructor is nil")
	}

	client := c.constructors.NewKubernetesClient(c.injector)
	if client == nil {
		return fmt.Errorf("failed to create kubernetes components: NewKubernetesClient returned nil")
	}

	c.injector.Register("kubernetesClient", client)

	kubernetesManager := c.constructors.NewKubernetesManager(c.injector)
	if kubernetesManager == nil {
		return fmt.Errorf("failed to create kubernetes components: NewKubernetesManager returned nil")
	}

	// Skip initialization during init command
	if c.requirements.CommandName != "init" {
		if err := kubernetesManager.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize kubernetes manager: %w", err)
		}
	}

	c.injector.Register("kubernetesManager", kubernetesManager)

	// Create Talos components if configured
	if configHandler.GetString("cluster.driver") == "talos" {
		if c.constructors.NewTalosClusterClient == nil {
			return fmt.Errorf("failed to create talos components: NewTalosClusterClient constructor is nil")
		}

		talosClient := c.constructors.NewTalosClusterClient(c.injector)
		if talosClient == nil {
			return fmt.Errorf("failed to create talos components: NewTalosClusterClient returned nil")
		}

		c.injector.Register("clusterClient", talosClient)
	}

	return nil
}

// createBundlerComponents creates bundler components if required
// It sets up the artifact builder and all bundlers for blueprint packaging
func (c *BaseController) createBundlerComponents(req Requirements) error {
	if !req.Bundler {
		return nil
	}

	// Create artifact builder if not already exists
	if existingArtifactBuilder := c.ResolveArtifactBuilder(); existingArtifactBuilder == nil {
		if c.constructors.NewArtifactBuilder == nil {
			return fmt.Errorf("NewArtifactBuilder constructor is nil")
		}
		artifactBuilder := c.constructors.NewArtifactBuilder(c.injector)
		if artifactBuilder == nil {
			return fmt.Errorf("NewArtifactBuilder returned nil")
		}
		c.injector.Register("artifactBuilder", artifactBuilder)
	}

	// Create bundlers if not already exist
	existingBundlers := c.ResolveAllBundlers()
	existingBundlerNames := make(map[string]bool)

	// Track existing bundlers
	for _, bundler := range existingBundlers {
		if c.injector.Resolve("templateBundler") == bundler {
			existingBundlerNames["templateBundler"] = true
		} else if c.injector.Resolve("kustomizeBundler") == bundler {
			existingBundlerNames["kustomizeBundler"] = true
		} else if c.injector.Resolve("terraformBundler") == bundler {
			existingBundlerNames["terraformBundler"] = true
		}
	}

	// Create template bundler if not exists
	if !existingBundlerNames["templateBundler"] {
		if c.constructors.NewTemplateBundler == nil {
			return fmt.Errorf("NewTemplateBundler constructor is nil")
		}
		templateBundler := c.constructors.NewTemplateBundler(c.injector)
		if templateBundler == nil {
			return fmt.Errorf("NewTemplateBundler returned nil")
		}
		c.injector.Register("templateBundler", templateBundler)
	}

	// Create kustomize bundler if not exists
	if !existingBundlerNames["kustomizeBundler"] {
		if c.constructors.NewKustomizeBundler == nil {
			return fmt.Errorf("NewKustomizeBundler constructor is nil")
		}
		kustomizeBundler := c.constructors.NewKustomizeBundler(c.injector)
		if kustomizeBundler == nil {
			return fmt.Errorf("NewKustomizeBundler returned nil")
		}
		c.injector.Register("kustomizeBundler", kustomizeBundler)
	}

	// Create terraform bundler if not exists
	if !existingBundlerNames["terraformBundler"] {
		if c.constructors.NewTerraformBundler == nil {
			return fmt.Errorf("NewTerraformBundler constructor is nil")
		}
		terraformBundler := c.constructors.NewTerraformBundler(c.injector)
		if terraformBundler == nil {
			return fmt.Errorf("NewTerraformBundler returned nil")
		}
		c.injector.Register("terraformBundler", terraformBundler)
	}

	return nil
}

// =============================================================================
// Interface compliance
// =============================================================================

var _ Controller = (*BaseController)(nil)
