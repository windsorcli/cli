package controller

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/env"
	"github.com/windsor-hotel/cli/internal/network"
	"github.com/windsor-hotel/cli/internal/services"
	"github.com/windsor-hotel/cli/internal/shell"
	"github.com/windsor-hotel/cli/internal/virt"
)

// Controller interface defines the methods for the controller.
type Controller interface {
	Initialize() error
	InitializeComponents() error
	CreateCommonComponents() error
	CreateEnvComponents() error
	CreateServiceComponents() error
	CreateVirtualizationComponents() error
	ResolveInjector() di.Injector
	ResolveConfigHandler() (config.ConfigHandler, error)
	ResolveContextHandler() (context.ContextHandler, error)
	ResolveEnvPrinter(name string) (env.EnvPrinter, error)
	ResolveAllEnvPrinters() ([]env.EnvPrinter, error)
	ResolveShell() (shell.Shell, error)
	ResolveSecureShell() (shell.Shell, error)
	ResolveNetworkManager() (network.NetworkManager, error)
	ResolveService(name string) (services.Service, error)
	ResolveAllServices() ([]services.Service, error)
	ResolveVirtualMachine() (virt.VirtualMachine, error)
	ResolveContainerRuntime() (virt.ContainerRuntime, error)
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
	configHandler, err := c.ResolveConfigHandler()
	if err != nil {
		return fmt.Errorf("error initializing controller: %w", err)
	}
	c.configHandler = configHandler

	// Load the configuration
	cliConfigPath, err := getCLIConfigPath()
	if err != nil {
		return fmt.Errorf("error getting CLI config path: %w", err)
	}
	if err := configHandler.LoadConfig(cliConfigPath); err != nil {
		return fmt.Errorf("error loading CLI config: %w", err)
	}

	return nil
}

// InitializeComponents initializes all components.
func (c *BaseController) InitializeComponents() error {
	// Initialize the context handler
	contextHandler, err := c.ResolveContextHandler()
	if err != nil {
		return fmt.Errorf("error initializing context handler: %w", err)
	}
	if err := contextHandler.Initialize(); err != nil {
		return fmt.Errorf("error initializing context handler: %w", err)
	}

	// Initialize the env printers
	envPrinters, err := c.ResolveAllEnvPrinters()
	if err != nil {
		return fmt.Errorf("error initializing env printers: %w", err)
	}
	for _, envPrinter := range envPrinters {
		if err := envPrinter.Initialize(); err != nil {
			return fmt.Errorf("error initializing env printer: %w", err)
		}
	}

	// Initialize the shell
	shell, err := c.ResolveShell()
	if err != nil {
		return fmt.Errorf("error initializing shell: %w", err)
	}
	if err := shell.Initialize(); err != nil {
		return fmt.Errorf("error initializing shell: %w", err)
	}

	// Initialize the secure shell
	secureShell, err := c.ResolveSecureShell()
	if err != nil {
		return fmt.Errorf("error initializing secure shell: %w", err)
	}
	if err := secureShell.Initialize(); err != nil {
		return fmt.Errorf("error initializing secure shell: %w", err)
	}

	// Initialize the network manager
	networkManager, err := c.ResolveNetworkManager()
	if err != nil {
		return fmt.Errorf("error initializing network manager: %w", err)
	}
	if err := networkManager.Initialize(); err != nil {
		return fmt.Errorf("error initializing network manager: %w", err)
	}

	// Initialize the services
	services, err := c.ResolveAllServices()
	if err != nil {
		return fmt.Errorf("error initializing services: %w", err)
	}
	for _, service := range services {
		if err := service.Initialize(); err != nil {
			return fmt.Errorf("error initializing service: %w", err)
		}
	}

	// Initialize the virtual machine
	virtualMachine, err := c.ResolveVirtualMachine()
	if err != nil {
		return fmt.Errorf("error initializing virtual machine: %w", err)
	}
	if err := virtualMachine.Initialize(); err != nil {
		return fmt.Errorf("error initializing virtual machine: %w", err)
	}

	// Initialize the container runtime
	containerRuntime, err := c.ResolveContainerRuntime()
	if err != nil {
		return fmt.Errorf("error initializing container runtime: %w", err)
	}
	if err := containerRuntime.Initialize(); err != nil {
		return fmt.Errorf("error initializing container runtime: %w", err)
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

// ResolveInjector resolves the injector instance.
func (c *BaseController) ResolveInjector() di.Injector {
	return c.injector
}

// ResolveConfigHandler resolves the configHandler instance.
func (c *BaseController) ResolveConfigHandler() (config.ConfigHandler, error) {
	instance, err := c.injector.Resolve("configHandler")
	if err != nil {
		return nil, err
	}
	configHandler, ok := instance.(config.ConfigHandler)
	if !ok {
		return nil, fmt.Errorf("resolved instance is not a ConfigHandler")
	}
	return configHandler, nil
}

// ResolveContextHandler resolves the contextHandler instance.
func (c *BaseController) ResolveContextHandler() (context.ContextHandler, error) {
	instance, err := c.injector.Resolve("contextHandler")
	if err != nil {
		return nil, err
	}
	contextHandler, ok := instance.(context.ContextHandler)
	if !ok {
		return nil, fmt.Errorf("resolved instance is not a ContextHandler")
	}
	return contextHandler, nil
}

// ResolveEnvPrinter resolves the envPrinter instance.
func (c *BaseController) ResolveEnvPrinter(name string) (env.EnvPrinter, error) {
	instance, err := c.injector.Resolve(name)
	if err != nil {
		return nil, err
	}
	envPrinter, ok := instance.(env.EnvPrinter)
	if !ok {
		return nil, fmt.Errorf("resolved instance is not an EnvPrinter")
	}
	return envPrinter, nil
}

// ResolveAllEnvPrinters resolves all envPrinter instances.
func (c *BaseController) ResolveAllEnvPrinters() ([]env.EnvPrinter, error) {
	instances, err := c.injector.ResolveAll((*env.EnvPrinter)(nil))
	if err != nil {
		return nil, err
	}
	envPrinters := make([]env.EnvPrinter, 0, len(instances))
	for _, instance := range instances {
		envPrinter, _ := instance.(env.EnvPrinter)
		envPrinters = append(envPrinters, envPrinter)
	}
	return envPrinters, nil
}

// ResolveShell resolves the shell instance.
func (c *BaseController) ResolveShell() (shell.Shell, error) {
	instance, err := c.injector.Resolve("shell")
	if err != nil {
		return nil, err
	}
	shellInstance, ok := instance.(shell.Shell)
	if !ok {
		return nil, fmt.Errorf("resolved instance is not a Shell")
	}
	return shellInstance, nil
}

// ResolveSecureShell resolves the secureShell instance.
func (c *BaseController) ResolveSecureShell() (shell.Shell, error) {
	instance, err := c.injector.Resolve("secureShell")
	if err != nil {
		return nil, err
	}
	shellInstance, ok := instance.(shell.Shell)
	if !ok {
		return nil, fmt.Errorf("resolved instance is not a Shell")
	}
	return shellInstance, nil
}

// ResolveNetworkManager resolves the networkManager instance.
func (c *BaseController) ResolveNetworkManager() (network.NetworkManager, error) {
	instance, err := c.injector.Resolve("networkManager")
	if err != nil {
		return nil, err
	}
	networkManager, ok := instance.(network.NetworkManager)
	if !ok {
		return nil, fmt.Errorf("resolved instance is not a NetworkManager")
	}
	return networkManager, nil
}

// ResolveService resolves the requested service instance.
func (c *BaseController) ResolveService(name string) (services.Service, error) {
	instance, err := c.injector.Resolve(name)
	if err != nil {
		return nil, err
	}
	serviceInstance, ok := instance.(services.Service)
	if !ok {
		return nil, fmt.Errorf("resolved instance is not a Service")
	}
	return serviceInstance, nil
}

// ResolveAllServices resolves all service instances.
func (c *BaseController) ResolveAllServices() ([]services.Service, error) {
	instances, err := c.injector.ResolveAll((*services.Service)(nil))
	if err != nil {
		return nil, err
	}
	servicesInstances := make([]services.Service, 0, len(instances))
	for _, instance := range instances {
		serviceInstance, _ := instance.(services.Service)
		servicesInstances = append(servicesInstances, serviceInstance)
	}
	return servicesInstances, nil
}

// ResolveVirtualMachine resolves the requested virtualMachine instance.
func (c *BaseController) ResolveVirtualMachine() (virt.VirtualMachine, error) {
	instance, err := c.injector.Resolve("virtualMachine")
	if err != nil {
		return nil, fmt.Errorf("error resolving virtual machine: %w", err)
	}
	virtualMachine, ok := instance.(virt.VirtualMachine)
	if !ok {
		return nil, fmt.Errorf("resolved instance is not a VirtualMachine")
	}
	return virtualMachine, nil
}

// ResolveContainerRuntime resolves the requested containerRuntime instance.
func (c *BaseController) ResolveContainerRuntime() (virt.ContainerRuntime, error) {
	instance, err := c.injector.Resolve("containerRuntime")
	if err != nil {
		return nil, fmt.Errorf("error resolving container runtime: %w", err)
	}
	containerRuntime, ok := instance.(virt.ContainerRuntime)
	if !ok {
		return nil, fmt.Errorf("resolved instance is not a ContainerRuntime")
	}
	return containerRuntime, nil
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
