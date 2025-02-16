package controller

import (
	"fmt"

	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/generators"
	"github.com/windsorcli/cli/pkg/network"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/services"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/stack"
	"github.com/windsorcli/cli/pkg/tools"
	"github.com/windsorcli/cli/pkg/virt"
)

// Controller interface defines the methods for the controller.
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
}

// BaseController struct implements the Controller interface.
type BaseController struct {
	injector      di.Injector
	configHandler config.ConfigHandler
}

// NewController creates a new controller.
func NewController(injector di.Injector) *BaseController {
	return &BaseController{injector: injector}
}

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
	// no-op
	return nil
}

// CreateSecretsProvider creates the secrets provider.
func (c *BaseController) CreateSecretsProviders() error {
	// no-op
	return nil
}

// CreateProjectComponents creates the project components.
func (c *BaseController) CreateProjectComponents() error {
	// no-op
	return nil
}

// CreateEnvComponents creates the env components.
func (c *BaseController) CreateEnvComponents() error {
	// no-op
	return nil
}

// CreateServiceComponents creates the service components.
func (c *BaseController) CreateServiceComponents() error {
	// no-op
	return nil
}

// CreateVirtualizationComponents creates the virtualization components.
func (c *BaseController) CreateVirtualizationComponents() error {
	// no-op
	return nil
}

// CreateStackComponents creates the stack components.
func (c *BaseController) CreateStackComponents() error {
	// no-op
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
	var customEnvPrinter env.EnvPrinter

	for _, instance := range instances {
		envPrinter, _ := instance.(env.EnvPrinter)
		if _, ok := envPrinter.(*env.CustomEnvPrinter); ok {
			customEnvPrinter = envPrinter
		} else {
			envPrinters = append(envPrinters, envPrinter)
		}
	}

	if customEnvPrinter != nil {
		envPrinters = append(envPrinters, customEnvPrinter)
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
	instance := c.injector.Resolve(name)
	serviceInstance, _ := instance.(services.Service)
	return serviceInstance
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

// Ensure BaseController implements the Controller interface
var _ Controller = (*BaseController)(nil)
