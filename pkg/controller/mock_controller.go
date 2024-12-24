package controller

import (
	"fmt"

	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/generators"
	"github.com/windsorcli/cli/pkg/network"
	"github.com/windsorcli/cli/pkg/services"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/ssh"
	"github.com/windsorcli/cli/pkg/stack"
	"github.com/windsorcli/cli/pkg/virt"
)

// MockController is a mock implementation of the Controller interface
type MockController struct {
	BaseController
	InitializeFunc                     func() error
	InitializeComponentsFunc           func() error
	CreateCommonComponentsFunc         func() error
	CreateProjectComponentsFunc        func() error
	CreateEnvComponentsFunc            func() error
	CreateServiceComponentsFunc        func() error
	CreateVirtualizationComponentsFunc func() error
	CreateStackComponentsFunc          func() error
	ResolveInjectorFunc                func() di.Injector
	ResolveConfigHandlerFunc           func() config.ConfigHandler
	ResolveContextHandlerFunc          func() context.ContextHandler
	ResolveEnvPrinterFunc              func(name string) env.EnvPrinter
	ResolveAllEnvPrintersFunc          func() []env.EnvPrinter
	ResolveShellFunc                   func() shell.Shell
	ResolveSecureShellFunc             func() shell.Shell
	ResolveNetworkManagerFunc          func() network.NetworkManager
	ResolveServiceFunc                 func(name string) services.Service
	ResolveAllServicesFunc             func() []services.Service
	ResolveVirtualMachineFunc          func() virt.VirtualMachine
	ResolveContainerRuntimeFunc        func() virt.ContainerRuntime
	ResolveAllGeneratorsFunc           func() []generators.Generator
	ResolveStackFunc                   func() stack.Stack
	WriteConfigurationFilesFunc        func() error
}

func NewMockController(injector di.Injector) *MockController {
	return &MockController{
		BaseController: BaseController{
			injector: injector,
		},
	}
}

// Initialize calls the mock InitializeFunc if set, otherwise calls the parent function
func (m *MockController) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return m.BaseController.Initialize()
}

// InitializeComponents calls the mock InitializeComponentsFunc if set, otherwise calls the parent function
func (m *MockController) InitializeComponents() error {
	if m.InitializeComponentsFunc != nil {
		return m.InitializeComponentsFunc()
	}
	return m.BaseController.InitializeComponents()
}

// CreateCommonComponents calls the mock CreateCommonComponentsFunc if set, otherwise creates mock components
func (m *MockController) CreateCommonComponents() error {
	if m.CreateCommonComponentsFunc != nil {
		return m.CreateCommonComponentsFunc()
	}

	// Create a new mock configHandler
	configHandler := config.NewMockConfigHandler()
	m.injector.Register("configHandler", configHandler)

	// Set the configHandler
	m.configHandler = configHandler

	// Create a new mock contextHandler
	contextHandler := context.NewMockContext()
	m.injector.Register("contextHandler", contextHandler)

	// Create a new mock shell
	shellInstance := shell.NewMockShell()
	m.injector.Register("shell", shellInstance)

	// Testing Note: The following is hard to test as these are registered
	// above and can't be mocked externally. There may be a better way to
	// organize this in the future but this works for now, so we don't expect
	// these lines to be covered by tests.

	// Initialize the contextHandler
	resolvedContextHandler := m.injector.Resolve("contextHandler").(*context.MockContext)
	if err := resolvedContextHandler.Initialize(); err != nil {
		return fmt.Errorf("error initializing context handler: %w", err)
	}

	// Initialize the shell
	resolvedShell := m.injector.Resolve("shell").(*shell.MockShell)
	if err := resolvedShell.Initialize(); err != nil {
		return fmt.Errorf("error initializing shell: %w", err)
	}

	return nil
}

// CreateProjectComponents calls the mock CreateProjectComponentsFunc if set, otherwise creates mock components
func (m *MockController) CreateProjectComponents() error {
	if m.CreateProjectComponentsFunc != nil {
		return m.CreateProjectComponentsFunc()
	}

	// Create a new mock blueprint handler
	blueprintHandler := blueprint.NewMockBlueprintHandler(m.injector)
	m.injector.Register("blueprintHandler", blueprintHandler)

	// Create a new git generator
	gitGenerator := generators.NewMockGenerator()
	m.injector.Register("gitGenerator", gitGenerator)

	// Create a new mock terraform generator
	terraformGenerator := generators.NewMockGenerator()
	m.injector.Register("terraformGenerator", terraformGenerator)

	return nil
}

// CreateEnvComponents calls the mock CreateEnvComponentsFunc if set, otherwise creates mock components
func (m *MockController) CreateEnvComponents() error {
	if m.CreateEnvComponentsFunc != nil {
		return m.CreateEnvComponentsFunc()
	}

	// Create mock aws env printer
	awsEnv := env.NewMockEnvPrinter()
	m.injector.Register("awsEnv", awsEnv)

	// Create mock docker env printer
	dockerEnv := env.NewMockEnvPrinter()
	m.injector.Register("dockerEnv", dockerEnv)

	// Create mock kube env printer
	kubeEnv := env.NewMockEnvPrinter()
	m.injector.Register("kubeEnv", kubeEnv)

	// Create mock omni env printer
	omniEnv := env.NewMockEnvPrinter()
	m.injector.Register("omniEnv", omniEnv)

	// Create mock sops env printer
	sopsEnv := env.NewMockEnvPrinter()
	m.injector.Register("sopsEnv", sopsEnv)

	// Create mock talos env printer
	talosEnv := env.NewMockEnvPrinter()
	m.injector.Register("talosEnv", talosEnv)

	// Create mock terraform env printer
	terraformEnv := env.NewMockEnvPrinter()
	m.injector.Register("terraformEnv", terraformEnv)

	// Create mock windsor env printer
	windsorEnv := env.NewMockEnvPrinter()
	m.injector.Register("windsorEnv", windsorEnv)

	return nil
}

// CreateServiceComponents calls the mock CreateServiceComponentsFunc if set, otherwise creates mock components
func (m *MockController) CreateServiceComponents() error {
	if m.CreateServiceComponentsFunc != nil {
		return m.CreateServiceComponentsFunc()
	}

	contextConfig := m.configHandler.GetConfig()

	// Create mock dns service
	dnsEnabled := m.configHandler.GetBool("dns.enabled")
	if dnsEnabled {
		dnsService := services.NewMockService()
		m.injector.Register("dnsService", dnsService)
	}

	// Create mock git livereload service
	gitLivereloadEnabled := m.configHandler.GetBool("git.livereload.enabled")
	if gitLivereloadEnabled {
		gitLivereloadService := services.NewMockService()
		m.injector.Register("gitLivereloadService", gitLivereloadService)
	}

	// Create mock localstack service
	localstackEnabled := m.configHandler.GetBool("aws.localstack.enabled")
	if localstackEnabled {
		localstackService := services.NewMockService()
		m.injector.Register("localstackService", localstackService)
	}

	// Create mock registry services
	if contextConfig.Docker != nil && contextConfig.Docker.Registries != nil {
		registryServices := contextConfig.Docker.Registries
		for _, registry := range registryServices {
			service := services.NewMockService()
			service.SetName(registry.Name)
			serviceName := fmt.Sprintf("registryService.%s", registry.Name)
			m.injector.Register(serviceName, service)
		}
	}

	// Create mock cluster services
	clusterEnabled := m.configHandler.GetBool("cluster.enabled")
	if clusterEnabled {
		controlPlaneCount := m.configHandler.GetInt("cluster.controlplanes.count")
		workerCount := m.configHandler.GetInt("cluster.workers.count")

		clusterDriver := m.configHandler.GetString("cluster.driver")

		// Create mock talos cluster
		if clusterDriver == "talos" {
			for i := 1; i <= controlPlaneCount; i++ {
				controlPlaneService := services.NewMockService()
				controlPlaneService.SetName(fmt.Sprintf("controlplane-%d", i))
				serviceName := fmt.Sprintf("clusterNode.controlplane-%d", i)
				m.injector.Register(serviceName, controlPlaneService)
			}
			for i := 1; i <= workerCount; i++ {
				workerService := services.NewMockService()
				workerService.SetName(fmt.Sprintf("worker-%d", i))
				serviceName := fmt.Sprintf("clusterNode.worker-%d", i)
				m.injector.Register(serviceName, workerService)
			}
		}
	}

	return nil
}

// CreateVirtualizationComponents calls the mock CreateVirtualizationComponentsFunc if set, otherwise creates mock components
func (c *MockController) CreateVirtualizationComponents() error {
	if c.CreateVirtualizationComponentsFunc != nil {
		return c.CreateVirtualizationComponentsFunc()
	}

	vmDriver := c.configHandler.GetString("vm.driver")
	dockerEnabled := c.configHandler.GetBool("docker.enabled")

	if vmDriver != "" {
		// Create and register the RealNetworkInterfaceProvider instance
		networkInterfaceProvider := &network.MockNetworkInterfaceProvider{}
		c.injector.Register("networkInterfaceProvider", networkInterfaceProvider)

		// Create and register the ssh client
		sshClient := ssh.NewMockSSHClient()
		c.injector.Register("sshClient", sshClient)

		// Create and register the secure shell
		secureShell := shell.NewSecureShell(c.injector)
		c.injector.Register("secureShell", secureShell)
	}

	// Create mock colima components
	if vmDriver == "colima" {
		// Create mock colima virtual machine
		colimaVirtualMachine := virt.NewMockVirt()
		c.injector.Register("virtualMachine", colimaVirtualMachine)

		// Create mock colima network manager
		networkManager := network.NewMockNetworkManager()
		c.injector.Register("networkManager", networkManager)
	}

	// Create mock docker container runtime
	if dockerEnabled {
		containerRuntime := virt.NewMockVirt()
		c.injector.Register("containerRuntime", containerRuntime)
	}

	return nil
}

// CreateStackComponents calls the mock CreateStackComponentsFunc if set, otherwise creates mock components
func (c *MockController) CreateStackComponents() error {
	if c.CreateStackComponentsFunc != nil {
		return c.CreateStackComponentsFunc()
	}

	// Create a new stack
	stackInstance := stack.NewMockStack(c.injector)
	c.injector.Register("stack", stackInstance)

	return nil
}

// WriteConfigurationFiles calls the mock WriteConfigurationFilesFunc if set, otherwise calls the parent function
func (c *MockController) WriteConfigurationFiles() error {
	if c.WriteConfigurationFilesFunc != nil {
		return c.WriteConfigurationFilesFunc()
	}
	return nil
}

// ResolveInjector calls the mock ResolveInjectorFunc if set, otherwise returns a mock injector
func (c *MockController) ResolveInjector() di.Injector {
	if c.ResolveInjectorFunc != nil {
		return c.ResolveInjectorFunc()
	}
	return c.BaseController.ResolveInjector()
}

// ResolveConfigHandler calls the mock ResolveConfigHandlerFunc if set, otherwise calls the parent function
func (c *MockController) ResolveConfigHandler() config.ConfigHandler {
	if c.ResolveConfigHandlerFunc != nil {
		return c.ResolveConfigHandlerFunc()
	}
	return c.BaseController.ResolveConfigHandler()
}

// ResolveContextHandler calls the mock ResolveContextHandlerFunc if set, otherwise calls the parent function
func (c *MockController) ResolveContextHandler() context.ContextHandler {
	if c.ResolveContextHandlerFunc != nil {
		return c.ResolveContextHandlerFunc()
	}
	return c.BaseController.ResolveContextHandler()
}

// ResolveEnvPrinter calls the mock ResolveEnvPrinterFunc if set, otherwise calls the parent function
func (c *MockController) ResolveEnvPrinter(name string) env.EnvPrinter {
	if c.ResolveEnvPrinterFunc != nil {
		return c.ResolveEnvPrinterFunc(name)
	}
	return c.BaseController.ResolveEnvPrinter(name)
}

// ResolveAllEnvPrinters calls the mock ResolveAllEnvPrintersFunc if set, otherwise calls the parent function
func (c *MockController) ResolveAllEnvPrinters() []env.EnvPrinter {
	if c.ResolveAllEnvPrintersFunc != nil {
		return c.ResolveAllEnvPrintersFunc()
	}
	return c.BaseController.ResolveAllEnvPrinters()
}

// ResolveShell calls the mock ResolveShellFunc if set, otherwise calls the parent function
func (c *MockController) ResolveShell() shell.Shell {
	if c.ResolveShellFunc != nil {
		return c.ResolveShellFunc()
	}
	return c.BaseController.ResolveShell()
}

// ResolveSecureShell calls the mock ResolveSecureShellFunc if set, otherwise calls the parent function
func (c *MockController) ResolveSecureShell() shell.Shell {
	if c.ResolveSecureShellFunc != nil {
		return c.ResolveSecureShellFunc()
	}
	return c.BaseController.ResolveSecureShell()
}

// ResolveNetworkManager calls the mock ResolveNetworkManagerFunc if set, otherwise calls the parent function
func (c *MockController) ResolveNetworkManager() network.NetworkManager {
	if c.ResolveNetworkManagerFunc != nil {
		return c.ResolveNetworkManagerFunc()
	}
	return c.BaseController.ResolveNetworkManager()
}

// ResolveService calls the mock ResolveServiceFunc if set, otherwise calls the parent function
func (c *MockController) ResolveService(name string) services.Service {
	if c.ResolveServiceFunc != nil {
		return c.ResolveServiceFunc(name)
	}
	return c.BaseController.ResolveService(name)
}

// ResolveAllServices calls the mock ResolveAllServicesFunc if set, otherwise calls the parent function
func (c *MockController) ResolveAllServices() []services.Service {
	if c.ResolveAllServicesFunc != nil {
		return c.ResolveAllServicesFunc()
	}
	return c.BaseController.ResolveAllServices()
}

// ResolveVirtualMachine calls the mock ResolveVirtualMachineFunc if set, otherwise calls the parent function
func (c *MockController) ResolveVirtualMachine() virt.VirtualMachine {
	if c.ResolveVirtualMachineFunc != nil {
		return c.ResolveVirtualMachineFunc()
	}
	return c.BaseController.ResolveVirtualMachine()
}

// ResolveContainerRuntime calls the mock ResolveContainerRuntimeFunc if set, otherwise calls the parent function
func (c *MockController) ResolveContainerRuntime() virt.ContainerRuntime {
	if c.ResolveContainerRuntimeFunc != nil {
		return c.ResolveContainerRuntimeFunc()
	}
	return c.BaseController.ResolveContainerRuntime()
}

// ResolveAllGenerators calls the mock ResolveAllGeneratorsFunc if set, otherwise calls the parent function
func (c *MockController) ResolveAllGenerators() []generators.Generator {
	if c.ResolveAllGeneratorsFunc != nil {
		return c.ResolveAllGeneratorsFunc()
	}
	return c.BaseController.ResolveAllGenerators()
}

// ResolveStack calls the mock ResolveStackFunc if set, otherwise calls the parent function
func (c *MockController) ResolveStack() stack.Stack {
	if c.ResolveStackFunc != nil {
		return c.ResolveStackFunc()
	}
	return c.BaseController.ResolveStack()
}

// Ensure MockController implements Controller
var _ Controller = (*MockController)(nil)
