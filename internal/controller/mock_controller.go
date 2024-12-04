package controller

import (
	"fmt"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/env"
	"github.com/windsor-hotel/cli/internal/network"
	"github.com/windsor-hotel/cli/internal/services"
	"github.com/windsor-hotel/cli/internal/shell"
	"github.com/windsor-hotel/cli/internal/virt"
)

// MockController is a mock implementation of the Controller interface
type MockController struct {
	BaseController
	InitializeFunc                     func() error
	InitializeComponentsFunc           func() error
	CreateCommonComponentsFunc         func() error
	CreateEnvComponentsFunc            func() error
	CreateServiceComponentsFunc        func() error
	CreateVirtualizationComponentsFunc func() error
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
	shell := shell.NewMockShell()
	m.injector.Register("shell", shell)

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
func (m *MockController) CreateVirtualizationComponents() error {
	if m.CreateVirtualizationComponentsFunc != nil {
		return m.CreateVirtualizationComponentsFunc()
	}

	vmDriver := m.configHandler.GetString("vm.driver")
	dockerEnabled := m.configHandler.GetBool("docker.enabled")

	// Create mock colima components
	if vmDriver == "colima" {
		// Create mock colima virtual machine
		colimaVirtualMachine := virt.NewMockVirt()
		m.injector.Register("virtualMachine", colimaVirtualMachine)

		// Create mock colima network manager
		networkManager := network.NewMockNetworkManager()
		m.injector.Register("networkManager", networkManager)
	}

	// Create mock docker container runtime
	if dockerEnabled {
		containerRuntime := virt.NewMockVirt()
		m.injector.Register("containerRuntime", containerRuntime)
	}

	return nil
}

// WriteConfigurationFiles calls the mock WriteConfigurationFilesFunc if set, otherwise calls the parent function
func (m *MockController) WriteConfigurationFiles() error {
	if m.WriteConfigurationFilesFunc != nil {
		return m.WriteConfigurationFilesFunc()
	}
	return nil
}

// ResolveInjector calls the mock ResolveInjectorFunc if set, otherwise returns a mock injector
func (m *MockController) ResolveInjector() di.Injector {
	if m.ResolveInjectorFunc != nil {
		return m.ResolveInjectorFunc()
	}
	return m.BaseController.ResolveInjector()
}

// ResolveConfigHandler calls the mock ResolveConfigHandlerFunc if set, otherwise calls the parent function
func (m *MockController) ResolveConfigHandler() config.ConfigHandler {
	if m.ResolveConfigHandlerFunc != nil {
		return m.ResolveConfigHandlerFunc()
	}
	return m.BaseController.ResolveConfigHandler()
}

// ResolveContextHandler calls the mock ResolveContextHandlerFunc if set, otherwise calls the parent function
func (m *MockController) ResolveContextHandler() context.ContextHandler {
	if m.ResolveContextHandlerFunc != nil {
		return m.ResolveContextHandlerFunc()
	}
	return m.BaseController.ResolveContextHandler()
}

// ResolveEnvPrinter calls the mock ResolveEnvPrinterFunc if set, otherwise calls the parent function
func (m *MockController) ResolveEnvPrinter(name string) env.EnvPrinter {
	if m.ResolveEnvPrinterFunc != nil {
		return m.ResolveEnvPrinterFunc(name)
	}
	return m.BaseController.ResolveEnvPrinter(name)
}

// ResolveAllEnvPrinters calls the mock ResolveAllEnvPrintersFunc if set, otherwise calls the parent function
func (m *MockController) ResolveAllEnvPrinters() []env.EnvPrinter {
	if m.ResolveAllEnvPrintersFunc != nil {
		return m.ResolveAllEnvPrintersFunc()
	}
	return m.BaseController.ResolveAllEnvPrinters()
}

// ResolveShell calls the mock ResolveShellFunc if set, otherwise calls the parent function
func (m *MockController) ResolveShell() shell.Shell {
	if m.ResolveShellFunc != nil {
		return m.ResolveShellFunc()
	}
	return m.BaseController.ResolveShell()
}

// ResolveSecureShell calls the mock ResolveSecureShellFunc if set, otherwise calls the parent function
func (m *MockController) ResolveSecureShell() shell.Shell {
	if m.ResolveSecureShellFunc != nil {
		return m.ResolveSecureShellFunc()
	}
	return m.BaseController.ResolveSecureShell()
}

// ResolveNetworkManager calls the mock ResolveNetworkManagerFunc if set, otherwise calls the parent function
func (m *MockController) ResolveNetworkManager() network.NetworkManager {
	if m.ResolveNetworkManagerFunc != nil {
		return m.ResolveNetworkManagerFunc()
	}
	return m.BaseController.ResolveNetworkManager()
}

// ResolveService calls the mock ResolveServiceFunc if set, otherwise calls the parent function
func (m *MockController) ResolveService(name string) services.Service {
	if m.ResolveServiceFunc != nil {
		return m.ResolveServiceFunc(name)
	}
	return m.BaseController.ResolveService(name)
}

// ResolveAllServices calls the mock ResolveAllServicesFunc if set, otherwise calls the parent function
func (m *MockController) ResolveAllServices() []services.Service {
	if m.ResolveAllServicesFunc != nil {
		return m.ResolveAllServicesFunc()
	}
	return m.BaseController.ResolveAllServices()
}

// ResolveVirtualMachine calls the mock ResolveVirtualMachineFunc if set, otherwise calls the parent function
func (m *MockController) ResolveVirtualMachine() virt.VirtualMachine {
	if m.ResolveVirtualMachineFunc != nil {
		return m.ResolveVirtualMachineFunc()
	}
	return m.BaseController.ResolveVirtualMachine()
}

// ResolveContainerRuntime calls the mock ResolveContainerRuntimeFunc if set, otherwise calls the parent function
func (m *MockController) ResolveContainerRuntime() virt.ContainerRuntime {
	if m.ResolveContainerRuntimeFunc != nil {
		return m.ResolveContainerRuntimeFunc()
	}
	return m.BaseController.ResolveContainerRuntime()
}

// Ensure MockController implements Controller
var _ Controller = (*MockController)(nil)
