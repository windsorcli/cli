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
	InitializeFunc              func() error
	ResolveInjectorFunc         func() di.Injector
	ResolveConfigHandlerFunc    func() (config.ConfigHandler, error)
	ResolveContextHandlerFunc   func() (context.ContextHandler, error)
	ResolveEnvPrinterFunc       func(name string) (env.EnvPrinter, error)
	ResolveAllEnvPrintersFunc   func() ([]env.EnvPrinter, error)
	ResolveShellFunc            func() (shell.Shell, error)
	ResolveSecureShellFunc      func() (shell.Shell, error)
	ResolveNetworkManagerFunc   func() (network.NetworkManager, error)
	ResolveServiceFunc          func(name string) (services.Service, error)
	ResolveAllServicesFunc      func() ([]services.Service, error)
	ResolveVirtualMachineFunc   func() (virt.VirtualMachine, error)
	ResolveContainerRuntimeFunc func() (virt.ContainerRuntime, error)
}

// Initialize calls the mock InitializeFunc if set, otherwise returns nil
func (m *MockController) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
}

// ResolveInjector calls the mock ResolveInjectorFunc if set, otherwise returns nil
func (m *MockController) ResolveInjector() di.Injector {
	if m.ResolveInjectorFunc != nil {
		return m.ResolveInjectorFunc()
	}
	return nil
}

// ResolveConfigHandler calls the mock ResolveConfigHandlerFunc if set, otherwise returns nil and an error
func (m *MockController) ResolveConfigHandler() (config.ConfigHandler, error) {
	if m.ResolveConfigHandlerFunc != nil {
		return m.ResolveConfigHandlerFunc()
	}
	return nil, fmt.Errorf("mock error resolving config handler")
}

// ResolveContextHandler calls the mock ResolveContextHandlerFunc if set, otherwise returns nil and an error
func (m *MockController) ResolveContextHandler() (context.ContextHandler, error) {
	if m.ResolveContextHandlerFunc != nil {
		return m.ResolveContextHandlerFunc()
	}
	return nil, fmt.Errorf("mock error resolving context handler")
}

// ResolveEnvPrinter calls the mock ResolveEnvPrinterFunc if set, otherwise returns nil and an error
func (m *MockController) ResolveEnvPrinter(name string) (env.EnvPrinter, error) {
	if m.ResolveEnvPrinterFunc != nil {
		return m.ResolveEnvPrinterFunc(name)
	}
	return nil, fmt.Errorf("mock error resolving env printer")
}

// ResolveAllEnvPrinters calls the mock ResolveAllEnvPrintersFunc if set, otherwise returns nil and an error
func (m *MockController) ResolveAllEnvPrinters() ([]env.EnvPrinter, error) {
	if m.ResolveAllEnvPrintersFunc != nil {
		return m.ResolveAllEnvPrintersFunc()
	}
	return nil, fmt.Errorf("mock error resolving all env printers")
}

// ResolveShell calls the mock ResolveShellFunc if set, otherwise returns nil and an error
func (m *MockController) ResolveShell() (shell.Shell, error) {
	if m.ResolveShellFunc != nil {
		return m.ResolveShellFunc()
	}
	return nil, fmt.Errorf("mock error resolving shell")
}

// ResolveSecureShell calls the mock ResolveSecureShellFunc if set, otherwise returns nil and an error
func (m *MockController) ResolveSecureShell() (shell.Shell, error) {
	if m.ResolveSecureShellFunc != nil {
		return m.ResolveSecureShellFunc()
	}
	return nil, fmt.Errorf("mock error resolving secure shell")
}

// ResolveNetworkManager calls the mock ResolveNetworkManagerFunc if set, otherwise returns nil and an error
func (m *MockController) ResolveNetworkManager() (network.NetworkManager, error) {
	if m.ResolveNetworkManagerFunc != nil {
		return m.ResolveNetworkManagerFunc()
	}
	return nil, fmt.Errorf("mock error resolving network manager")
}

// ResolveService calls the mock ResolveServiceFunc if set, otherwise returns nil and an error
func (m *MockController) ResolveService(name string) (services.Service, error) {
	if m.ResolveServiceFunc != nil {
		return m.ResolveServiceFunc(name)
	}
	return nil, fmt.Errorf("mock error resolving service")
}

// ResolveAllServices calls the mock ResolveAllServicesFunc if set, otherwise returns nil and an error
func (m *MockController) ResolveAllServices() ([]services.Service, error) {
	if m.ResolveAllServicesFunc != nil {
		return m.ResolveAllServicesFunc()
	}
	return nil, fmt.Errorf("mock error resolving all services")
}

// ResolveVirtualMachine calls the mock ResolveVirtualMachineFunc if set, otherwise returns nil and an error
func (m *MockController) ResolveVirtualMachine() (virt.VirtualMachine, error) {
	if m.ResolveVirtualMachineFunc != nil {
		return m.ResolveVirtualMachineFunc()
	}
	return nil, fmt.Errorf("mock error resolving virtual machine")
}

// ResolveContainerRuntime calls the mock ResolveContainerRuntimeFunc if set, otherwise returns nil and an error
func (m *MockController) ResolveContainerRuntime() (virt.ContainerRuntime, error) {
	if m.ResolveContainerRuntimeFunc != nil {
		return m.ResolveContainerRuntimeFunc()
	}
	return nil, fmt.Errorf("mock error resolving container runtime")
}

// Ensure MockController implements Controller
var _ Controller = (*MockController)(nil)
