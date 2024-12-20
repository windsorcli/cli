package controller

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/internal/blueprint"
	"github.com/windsorcli/cli/internal/config"
	"github.com/windsorcli/cli/internal/context"
	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/env"
	"github.com/windsorcli/cli/internal/generators"
	"github.com/windsorcli/cli/internal/network"
	"github.com/windsorcli/cli/internal/services"
	"github.com/windsorcli/cli/internal/shell"
	"github.com/windsorcli/cli/internal/stack"
	"github.com/windsorcli/cli/internal/virt"
)

// Controller interface defines the methods for the controller.
type Controller interface {
	Initialize() error
	InitializeComponents() error
	CreateCommonComponents() error
	CreateEnvComponents() error
	CreateServiceComponents() error
	CreateVirtualizationComponents() error
	CreateStackComponents() error
	ResolveInjector() di.Injector
	ResolveConfigHandler() config.ConfigHandler
	ResolveContextHandler() context.ContextHandler
	ResolveEnvPrinter(name string) env.EnvPrinter
	ResolveAllEnvPrinters() []env.EnvPrinter
	ResolveShell() shell.Shell
	ResolveSecureShell() shell.Shell
	ResolveNetworkManager() network.NetworkManager
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

// Initialize the controller.
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

	// Initialize the env printers
	envPrinters := c.ResolveAllEnvPrinters()
	if len(envPrinters) > 0 {
		for _, envPrinter := range envPrinters {
			if err := envPrinter.Initialize(); err != nil {
				return fmt.Errorf("error initializing env printer: %w", err)
			}
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

// ResolveContextHandler resolves the contextHandler instance.
func (c *BaseController) ResolveContextHandler() context.ContextHandler {
	instance := c.injector.Resolve("contextHandler")
	contextHandler, _ := instance.(context.ContextHandler)
	return contextHandler
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
	for _, instance := range instances {
		envPrinter, _ := instance.(env.EnvPrinter)
		envPrinters = append(envPrinters, envPrinter)
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

// getCLIConfigPath returns the path to the CLI configuration file
var getCLIConfigPath = func() (string, error) {
	cliConfigPath := os.Getenv("WINDSORCONFIG")
	if cliConfigPath == "" {
		home, err := osUserHomeDir()
		if err != nil {
			return "", fmt.Errorf("error retrieving user home directory: %w", err)
		}
		cliConfigPath = filepath.Join(home, ".config", "windsor", "config.yaml")
	}
	return cliConfigPath, nil
}
