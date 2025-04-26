package controller

import (
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

// MockController is a mock implementation of the Controller interface
type MockController struct {
	BaseController
	InitializeFunc                     func() error
	InitializeComponentsFunc           func() error
	CreateCommonComponentsFunc         func() error
	CreateSecretsProvidersFunc         func() error
	CreateProjectComponentsFunc        func() error
	CreateEnvComponentsFunc            func() error
	CreateServiceComponentsFunc        func() error
	CreateVirtualizationComponentsFunc func() error
	CreateStackComponentsFunc          func() error
	ResolveInjectorFunc                func() di.Injector
	ResolveConfigHandlerFunc           func() config.ConfigHandler
	ResolveEnvPrinterFunc              func(name string) env.EnvPrinter
	ResolveAllEnvPrintersFunc          func() []env.EnvPrinter
	ResolveShellFunc                   func() shell.Shell
	ResolveSecureShellFunc             func() shell.Shell
	ResolveToolsManagerFunc            func() tools.ToolsManager
	ResolveNetworkManagerFunc          func() network.NetworkManager
	ResolveServiceFunc                 func(name string) services.Service
	ResolveAllServicesFunc             func() []services.Service
	ResolveVirtualMachineFunc          func() virt.VirtualMachine
	ResolveContainerRuntimeFunc        func() virt.ContainerRuntime
	ResolveAllGeneratorsFunc           func() []generators.Generator
	ResolveStackFunc                   func() stack.Stack
	ResolveBlueprintHandlerFunc        func() blueprint.BlueprintHandler
	ResolveAllSecretsProvidersFunc     func() []secrets.SecretsProvider
	WriteConfigurationFilesFunc        func() error
	SetEnvironmentVariablesFunc        func() error
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockController creates a new MockController with the given injector
func NewMockController(injector di.Injector) *MockController {
	// Create with mock constructors from controller.go
	return &MockController{
		BaseController: BaseController{
			injector:     injector,
			constructors: MockConstructors(),
		},
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize implements the Controller interface
func (m *MockController) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
}

// InitializeComponents implements the Controller interface
func (m *MockController) InitializeComponents() error {
	if m.InitializeComponentsFunc != nil {
		return m.InitializeComponentsFunc()
	}
	return nil
}

// CreateCommonComponents implements the Controller interface
func (m *MockController) CreateCommonComponents() error {
	if m.CreateCommonComponentsFunc != nil {
		return m.CreateCommonComponentsFunc()
	}
	return nil
}

// CreateSecretsProviders implements the Controller interface
func (m *MockController) CreateSecretsProviders() error {
	if m.CreateSecretsProvidersFunc != nil {
		return m.CreateSecretsProvidersFunc()
	}
	return nil
}

// CreateProjectComponents implements the Controller interface
func (m *MockController) CreateProjectComponents() error {
	if m.CreateProjectComponentsFunc != nil {
		return m.CreateProjectComponentsFunc()
	}
	return nil
}

// CreateEnvComponents implements the Controller interface
func (m *MockController) CreateEnvComponents() error {
	if m.CreateEnvComponentsFunc != nil {
		return m.CreateEnvComponentsFunc()
	}
	return nil
}

// CreateServiceComponents implements the Controller interface
func (m *MockController) CreateServiceComponents() error {
	if m.CreateServiceComponentsFunc != nil {
		return m.CreateServiceComponentsFunc()
	}
	return nil
}

// CreateVirtualizationComponents implements the Controller interface
func (m *MockController) CreateVirtualizationComponents() error {
	if m.CreateVirtualizationComponentsFunc != nil {
		return m.CreateVirtualizationComponentsFunc()
	}
	return nil
}

// CreateStackComponents implements the Controller interface
func (m *MockController) CreateStackComponents() error {
	if m.CreateStackComponentsFunc != nil {
		return m.CreateStackComponentsFunc()
	}
	return nil
}

// WriteConfigurationFiles implements the Controller interface
func (m *MockController) WriteConfigurationFiles() error {
	if m.WriteConfigurationFilesFunc != nil {
		return m.WriteConfigurationFilesFunc()
	}
	return nil
}

// ResolveInjector implements the Controller interface
func (m *MockController) ResolveInjector() di.Injector {
	if m.ResolveInjectorFunc != nil {
		return m.ResolveInjectorFunc()
	}
	return nil
}

// ResolveConfigHandler implements the Controller interface
func (m *MockController) ResolveConfigHandler() config.ConfigHandler {
	if m.ResolveConfigHandlerFunc != nil {
		return m.ResolveConfigHandlerFunc()
	}
	return nil
}

// ResolveEnvPrinter implements the Controller interface
func (m *MockController) ResolveEnvPrinter(name string) env.EnvPrinter {
	if m.ResolveEnvPrinterFunc != nil {
		return m.ResolveEnvPrinterFunc(name)
	}
	return nil
}

// ResolveAllEnvPrinters implements the Controller interface
func (m *MockController) ResolveAllEnvPrinters() []env.EnvPrinter {
	if m.ResolveAllEnvPrintersFunc != nil {
		return m.ResolveAllEnvPrintersFunc()
	}
	return nil
}

// ResolveShell implements the Controller interface
func (m *MockController) ResolveShell() shell.Shell {
	if m.ResolveShellFunc != nil {
		return m.ResolveShellFunc()
	}
	return nil
}

// ResolveSecureShell implements the Controller interface
func (m *MockController) ResolveSecureShell() shell.Shell {
	if m.ResolveSecureShellFunc != nil {
		return m.ResolveSecureShellFunc()
	}
	return nil
}

// ResolveToolsManager implements the Controller interface
func (m *MockController) ResolveToolsManager() tools.ToolsManager {
	if m.ResolveToolsManagerFunc != nil {
		return m.ResolveToolsManagerFunc()
	}
	return nil
}

// ResolveNetworkManager implements the Controller interface
func (m *MockController) ResolveNetworkManager() network.NetworkManager {
	if m.ResolveNetworkManagerFunc != nil {
		return m.ResolveNetworkManagerFunc()
	}
	return nil
}

// ResolveService implements the Controller interface
func (m *MockController) ResolveService(name string) services.Service {
	if m.ResolveServiceFunc != nil {
		return m.ResolveServiceFunc(name)
	}
	return nil
}

// ResolveAllServices implements the Controller interface
func (m *MockController) ResolveAllServices() []services.Service {
	if m.ResolveAllServicesFunc != nil {
		return m.ResolveAllServicesFunc()
	}
	return nil
}

// ResolveVirtualMachine implements the Controller interface
func (m *MockController) ResolveVirtualMachine() virt.VirtualMachine {
	if m.ResolveVirtualMachineFunc != nil {
		return m.ResolveVirtualMachineFunc()
	}
	return nil
}

// ResolveContainerRuntime implements the Controller interface
func (m *MockController) ResolveContainerRuntime() virt.ContainerRuntime {
	if m.ResolveContainerRuntimeFunc != nil {
		return m.ResolveContainerRuntimeFunc()
	}
	return nil
}

// ResolveAllGenerators implements the Controller interface
func (m *MockController) ResolveAllGenerators() []generators.Generator {
	if m.ResolveAllGeneratorsFunc != nil {
		return m.ResolveAllGeneratorsFunc()
	}
	return nil
}

// ResolveStack implements the Controller interface
func (m *MockController) ResolveStack() stack.Stack {
	if m.ResolveStackFunc != nil {
		return m.ResolveStackFunc()
	}
	return nil
}

// ResolveBlueprintHandler implements the Controller interface
func (m *MockController) ResolveBlueprintHandler() blueprint.BlueprintHandler {
	if m.ResolveBlueprintHandlerFunc != nil {
		return m.ResolveBlueprintHandlerFunc()
	}
	return nil
}

// ResolveAllSecretsProviders implements the Controller interface
func (m *MockController) ResolveAllSecretsProviders() []secrets.SecretsProvider {
	if m.ResolveAllSecretsProvidersFunc != nil {
		return m.ResolveAllSecretsProvidersFunc()
	}
	return nil
}

// SetEnvironmentVariables implements the Controller interface
func (m *MockController) SetEnvironmentVariables() error {
	if m.SetEnvironmentVariablesFunc != nil {
		return m.SetEnvironmentVariablesFunc()
	}
	return nil
}

// Ensure MockController implements Controller
var _ Controller = (*MockController)(nil)
