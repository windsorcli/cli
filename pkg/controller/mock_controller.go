package controller

import (
	"net"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/bundler"
	"github.com/windsorcli/cli/pkg/cluster"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/generators"
	"github.com/windsorcli/cli/pkg/kubernetes"
	"github.com/windsorcli/cli/pkg/network"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/services"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/ssh"
	"github.com/windsorcli/cli/pkg/stack"
	"github.com/windsorcli/cli/pkg/tools"
	"github.com/windsorcli/cli/pkg/virt"
)

// MockController is a mock implementation of the Controller interface
type MockController struct {
	BaseController
	SetRequirementsFunc            func(Requirements)
	InitializeComponentsFunc       func() error
	CreateComponentsFunc           func() error
	InitializeWithRequirementsFunc func(Requirements) error
	ResolveInjectorFunc            func() di.Injector
	ResolveConfigHandlerFunc       func() config.ConfigHandler
	ResolveEnvPrinterFunc          func(name string) env.EnvPrinter
	ResolveAllEnvPrintersFunc      func() []env.EnvPrinter
	ResolveShellFunc               func() shell.Shell
	ResolveSecureShellFunc         func() shell.Shell
	ResolveToolsManagerFunc        func() tools.ToolsManager
	ResolveNetworkManagerFunc      func() network.NetworkManager
	ResolveServiceFunc             func(name string) services.Service
	ResolveAllServicesFunc         func() []services.Service
	ResolveVirtualMachineFunc      func() virt.VirtualMachine
	ResolveContainerRuntimeFunc    func() virt.ContainerRuntime
	ResolveAllGeneratorsFunc       func() []generators.Generator
	ResolveStackFunc               func() stack.Stack
	ResolveBlueprintHandlerFunc    func() blueprint.BlueprintHandler
	ResolveAllSecretsProvidersFunc func() []secrets.SecretsProvider
	ResolveKubernetesManagerFunc   func() kubernetes.KubernetesManager
	ResolveKubernetesClientFunc    func() kubernetes.KubernetesClient
	ResolveClusterClientFunc       func() cluster.ClusterClient
	ResolveArtifactBuilderFunc     func() bundler.Artifact
	ResolveAllBundlersFunc         func() []bundler.Bundler
	WriteConfigurationFilesFunc    func() error
	SetEnvironmentVariablesFunc    func() error
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockController creates a new MockController with an optional injector
func NewMockController(injector ...di.Injector) *MockController {
	var inj di.Injector
	if len(injector) > 0 {
		inj = injector[0]
	} else {
		inj = di.NewInjector()
	}

	return &MockController{
		BaseController: BaseController{
			injector:     inj,
			constructors: NewMockConstructors(),
		},
	}
}

// NewMockConstructors returns a ComponentConstructors with all factory functions set to return mocks
// useful for testing
func NewMockConstructors() ComponentConstructors {
	return ComponentConstructors{
		// Common components
		NewConfigHandler: func(injector di.Injector) config.ConfigHandler {
			return config.NewYamlConfigHandler(injector) // Use a real config handler
		},
		NewShell: func(injector di.Injector) shell.Shell {
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
		NewAzureEnvPrinter: func(injector di.Injector) env.EnvPrinter {
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

		// Kubernetes components
		NewKubernetesManager: func(injector di.Injector) kubernetes.KubernetesManager {
			return kubernetes.NewMockKubernetesManager(injector)
		},
		NewKubernetesClient: func(injector di.Injector) kubernetes.KubernetesClient {
			return kubernetes.NewMockKubernetesClient()
		},

		// Cluster components
		NewTalosClusterClient: func(injector di.Injector) *cluster.TalosClusterClient {
			return cluster.NewTalosClusterClient(injector)
		},

		// Bundler components
		NewArtifactBuilder: func(injector di.Injector) bundler.Artifact {
			return bundler.NewMockArtifact()
		},
		NewTemplateBundler: func(injector di.Injector) bundler.Bundler {
			return bundler.NewMockBundler()
		},
		NewKustomizeBundler: func(injector di.Injector) bundler.Bundler {
			return bundler.NewMockBundler()
		},
	}
}

// =============================================================================
// Public Methods
// =============================================================================

func (m *MockController) SetRequirements(req Requirements) {
	if m.SetRequirementsFunc != nil {
		m.SetRequirementsFunc(req)
	}
	m.BaseController.SetRequirements(req)
}

// InitializeComponents implements the Controller interface
func (m *MockController) InitializeComponents() error {
	if m.InitializeComponentsFunc != nil {
		return m.InitializeComponentsFunc()
	}
	return m.BaseController.InitializeComponents()
}

// CreateComponents implements the Controller interface
func (m *MockController) CreateComponents() error {
	if m.CreateComponentsFunc != nil {
		return m.CreateComponentsFunc()
	}
	return m.BaseController.CreateComponents()
}

// InitializeWithRequirements implements the Controller interface
func (m *MockController) InitializeWithRequirements(req Requirements) error {
	if m.InitializeWithRequirementsFunc != nil {
		return m.InitializeWithRequirementsFunc(req)
	}
	return m.BaseController.InitializeWithRequirements(req)
}

// WriteConfigurationFiles implements the Controller interface
func (m *MockController) WriteConfigurationFiles() error {
	if m.WriteConfigurationFilesFunc != nil {
		return m.WriteConfigurationFilesFunc()
	}
	return m.BaseController.WriteConfigurationFiles()
}

// ResolveInjector implements the Controller interface
func (m *MockController) ResolveInjector() di.Injector {
	if m.ResolveInjectorFunc != nil {
		return m.ResolveInjectorFunc()
	}
	return m.BaseController.ResolveInjector()
}

// ResolveConfigHandler implements the Controller interface
func (m *MockController) ResolveConfigHandler() config.ConfigHandler {
	if m.ResolveConfigHandlerFunc != nil {
		return m.ResolveConfigHandlerFunc()
	}
	return m.BaseController.ResolveConfigHandler()
}

// ResolveEnvPrinter implements the Controller interface
func (m *MockController) ResolveEnvPrinter(name string) env.EnvPrinter {
	if m.ResolveEnvPrinterFunc != nil {
		return m.ResolveEnvPrinterFunc(name)
	}
	return m.BaseController.ResolveEnvPrinter(name)
}

// ResolveAllEnvPrinters implements the Controller interface
func (m *MockController) ResolveAllEnvPrinters() []env.EnvPrinter {
	if m.ResolveAllEnvPrintersFunc != nil {
		return m.ResolveAllEnvPrintersFunc()
	}
	return m.BaseController.ResolveAllEnvPrinters()
}

// ResolveShell implements the Controller interface
func (m *MockController) ResolveShell() shell.Shell {
	if m.ResolveShellFunc != nil {
		return m.ResolveShellFunc()
	}
	return m.BaseController.ResolveShell()
}

// ResolveSecureShell implements the Controller interface
func (m *MockController) ResolveSecureShell() shell.Shell {
	if m.ResolveSecureShellFunc != nil {
		return m.ResolveSecureShellFunc()
	}
	return m.BaseController.ResolveSecureShell()
}

// ResolveToolsManager implements the Controller interface
func (m *MockController) ResolveToolsManager() tools.ToolsManager {
	if m.ResolveToolsManagerFunc != nil {
		return m.ResolveToolsManagerFunc()
	}
	return m.BaseController.ResolveToolsManager()
}

// ResolveNetworkManager implements the Controller interface
func (m *MockController) ResolveNetworkManager() network.NetworkManager {
	if m.ResolveNetworkManagerFunc != nil {
		return m.ResolveNetworkManagerFunc()
	}
	return m.BaseController.ResolveNetworkManager()
}

// ResolveService implements the Controller interface
func (m *MockController) ResolveService(name string) services.Service {
	if m.ResolveServiceFunc != nil {
		return m.ResolveServiceFunc(name)
	}
	return m.BaseController.ResolveService(name)
}

// ResolveAllServices implements the Controller interface
func (m *MockController) ResolveAllServices() []services.Service {
	if m.ResolveAllServicesFunc != nil {
		return m.ResolveAllServicesFunc()
	}
	return m.BaseController.ResolveAllServices()
}

// ResolveVirtualMachine implements the Controller interface
func (m *MockController) ResolveVirtualMachine() virt.VirtualMachine {
	if m.ResolveVirtualMachineFunc != nil {
		return m.ResolveVirtualMachineFunc()
	}
	return m.BaseController.ResolveVirtualMachine()
}

// ResolveContainerRuntime implements the Controller interface
func (m *MockController) ResolveContainerRuntime() virt.ContainerRuntime {
	if m.ResolveContainerRuntimeFunc != nil {
		return m.ResolveContainerRuntimeFunc()
	}
	return m.BaseController.ResolveContainerRuntime()
}

// ResolveAllGenerators implements the Controller interface
func (m *MockController) ResolveAllGenerators() []generators.Generator {
	if m.ResolveAllGeneratorsFunc != nil {
		return m.ResolveAllGeneratorsFunc()
	}
	return m.BaseController.ResolveAllGenerators()
}

// ResolveStack implements the Controller interface
func (m *MockController) ResolveStack() stack.Stack {
	if m.ResolveStackFunc != nil {
		return m.ResolveStackFunc()
	}
	return m.BaseController.ResolveStack()
}

// ResolveBlueprintHandler implements the Controller interface
func (m *MockController) ResolveBlueprintHandler() blueprint.BlueprintHandler {
	if m.ResolveBlueprintHandlerFunc != nil {
		return m.ResolveBlueprintHandlerFunc()
	}
	return m.BaseController.ResolveBlueprintHandler()
}

// ResolveAllSecretsProviders implements the Controller interface
func (m *MockController) ResolveAllSecretsProviders() []secrets.SecretsProvider {
	if m.ResolveAllSecretsProvidersFunc != nil {
		return m.ResolveAllSecretsProvidersFunc()
	}
	return m.BaseController.ResolveAllSecretsProviders()
}

// SetEnvironmentVariables implements the Controller interface
func (m *MockController) SetEnvironmentVariables() error {
	if m.SetEnvironmentVariablesFunc != nil {
		return m.SetEnvironmentVariablesFunc()
	}
	return m.BaseController.SetEnvironmentVariables()
}

// ResolveKubernetesManager implements the Controller interface
func (m *MockController) ResolveKubernetesManager() kubernetes.KubernetesManager {
	if m.ResolveKubernetesManagerFunc != nil {
		return m.ResolveKubernetesManagerFunc()
	}
	return m.BaseController.ResolveKubernetesManager()
}

// ResolveClusterClient implements the Controller interface
func (m *MockController) ResolveClusterClient() cluster.ClusterClient {
	if m.ResolveClusterClientFunc != nil {
		return m.ResolveClusterClientFunc()
	}
	return m.BaseController.ResolveClusterClient()
}

// ResolveArtifactBuilder implements the Controller interface
func (m *MockController) ResolveArtifactBuilder() bundler.Artifact {
	if m.ResolveArtifactBuilderFunc != nil {
		return m.ResolveArtifactBuilderFunc()
	}
	return m.BaseController.ResolveArtifactBuilder()
}

// ResolveAllBundlers implements the Controller interface
func (m *MockController) ResolveAllBundlers() []bundler.Bundler {
	if m.ResolveAllBundlersFunc != nil {
		return m.ResolveAllBundlersFunc()
	}
	return m.BaseController.ResolveAllBundlers()
}

// Ensure MockController implements Controller
var _ Controller = (*MockController)(nil)
