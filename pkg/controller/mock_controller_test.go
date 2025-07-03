package controller

import (
	"fmt"
	"reflect"
	"testing"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/generators"
	"github.com/windsorcli/cli/pkg/kubernetes"
	"github.com/windsorcli/cli/pkg/network"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/services"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/stack"
	"github.com/windsorcli/cli/pkg/tools"
	"github.com/windsorcli/cli/pkg/virt"
)

func TestMockController_SetRequirements(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		var capturedReq Requirements
		ctrl.SetRequirementsFunc = func(req Requirements) {
			capturedReq = req
		}

		expectedReq := Requirements{}
		ctrl.SetRequirements(expectedReq)

		if !reflect.DeepEqual(capturedReq, expectedReq) {
			t.Errorf("expected %v, got %v", expectedReq, capturedReq)
		}
	})
}

func TestMockController_InitializeComponents(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedErr := fmt.Errorf("mock error")
		ctrl.InitializeComponentsFunc = func() error {
			return expectedErr
		}

		err := ctrl.InitializeComponents()

		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})
}

func TestMockController_WriteConfigurationFiles(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedErr := fmt.Errorf("mock error")
		ctrl.WriteConfigurationFilesFunc = func() error {
			return expectedErr
		}

		err := ctrl.WriteConfigurationFiles()

		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})
}

func TestMockController_ResolveInjector(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedInjector := di.NewInjector()
		ctrl.ResolveInjectorFunc = func() di.Injector {
			return expectedInjector
		}

		injector := ctrl.ResolveInjector()

		if injector != expectedInjector {
			t.Errorf("expected injector %v, got %v", expectedInjector, injector)
		}
	})
}

func TestMockController_ResolveConfigHandler(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedHandler := config.NewMockConfigHandler()
		ctrl.ResolveConfigHandlerFunc = func() config.ConfigHandler {
			return expectedHandler
		}

		handler := ctrl.ResolveConfigHandler()

		if handler != expectedHandler {
			t.Errorf("expected handler %v, got %v", expectedHandler, handler)
		}
	})
}

func TestMockController_ResolveEnvPrinter(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedPrinter := env.NewMockEnvPrinter()
		ctrl.ResolveEnvPrinterFunc = func(name string) env.EnvPrinter {
			return expectedPrinter
		}

		printer := ctrl.ResolveEnvPrinter("test")

		if printer != expectedPrinter {
			t.Errorf("expected printer %v, got %v", expectedPrinter, printer)
		}
	})
}

func TestMockController_ResolveAllEnvPrinters(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedPrinters := []env.EnvPrinter{env.NewMockEnvPrinter()}
		ctrl.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return expectedPrinters
		}

		printers := ctrl.ResolveAllEnvPrinters()

		if len(printers) != len(expectedPrinters) {
			t.Errorf("expected %d printers, got %d", len(expectedPrinters), len(printers))
		}
	})
}

func TestMockController_ResolveShell(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedShell := shell.NewMockShell()
		ctrl.ResolveShellFunc = func() shell.Shell {
			return expectedShell
		}

		sh := ctrl.ResolveShell()

		if sh != expectedShell {
			t.Errorf("expected shell %v, got %v", expectedShell, sh)
		}
	})
}

func TestMockController_ResolveSecureShell(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedShell := shell.NewMockShell()
		ctrl.ResolveSecureShellFunc = func() shell.Shell {
			return expectedShell
		}

		sh := ctrl.ResolveSecureShell()

		if sh != expectedShell {
			t.Errorf("expected shell %v, got %v", expectedShell, sh)
		}
	})
}

func TestMockController_ResolveToolsManager(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedManager := tools.NewMockToolsManager()
		ctrl.ResolveToolsManagerFunc = func() tools.ToolsManager {
			return expectedManager
		}

		manager := ctrl.ResolveToolsManager()

		if manager != expectedManager {
			t.Errorf("expected manager %v, got %v", expectedManager, manager)
		}
	})
}

func TestMockController_ResolveNetworkManager(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedManager := network.NewMockNetworkManager()
		ctrl.ResolveNetworkManagerFunc = func() network.NetworkManager {
			return expectedManager
		}

		manager := ctrl.ResolveNetworkManager()

		if manager != expectedManager {
			t.Errorf("expected manager %v, got %v", expectedManager, manager)
		}
	})
}

func TestMockController_ResolveKubernetesManager(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedManager := kubernetes.NewMockKubernetesManager(nil)
		ctrl.ResolveKubernetesManagerFunc = func() kubernetes.KubernetesManager {
			return expectedManager
		}

		manager := ctrl.ResolveKubernetesManager()

		if manager != expectedManager {
			t.Errorf("expected manager %v, got %v", expectedManager, manager)
		}
	})
}

func TestMockController_ResolveService(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedService := services.NewMockService()
		ctrl.ResolveServiceFunc = func(name string) services.Service {
			return expectedService
		}

		service := ctrl.ResolveService("test")

		if service != expectedService {
			t.Errorf("expected service %v, got %v", expectedService, service)
		}
	})
}

func TestMockController_ResolveAllServices(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedServices := []services.Service{services.NewMockService()}
		ctrl.ResolveAllServicesFunc = func() []services.Service {
			return expectedServices
		}

		services := ctrl.ResolveAllServices()

		if len(services) != len(expectedServices) {
			t.Errorf("expected %d services, got %d", len(expectedServices), len(services))
		}
	})
}

func TestMockController_ResolveVirtualMachine(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedVM := virt.NewMockVirt()
		ctrl.ResolveVirtualMachineFunc = func() virt.VirtualMachine {
			return expectedVM
		}

		vm := ctrl.ResolveVirtualMachine()

		if vm != expectedVM {
			t.Errorf("expected vm %v, got %v", expectedVM, vm)
		}
	})
}

func TestMockController_ResolveContainerRuntime(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedRuntime := virt.NewMockVirt()
		ctrl.ResolveContainerRuntimeFunc = func() virt.ContainerRuntime {
			return expectedRuntime
		}

		runtime := ctrl.ResolveContainerRuntime()

		if runtime != expectedRuntime {
			t.Errorf("expected runtime %v, got %v", expectedRuntime, runtime)
		}
	})
}

func TestMockController_ResolveAllGenerators(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedGenerators := []generators.Generator{generators.NewMockGenerator()}
		ctrl.ResolveAllGeneratorsFunc = func() []generators.Generator {
			return expectedGenerators
		}

		generators := ctrl.ResolveAllGenerators()

		if len(generators) != len(expectedGenerators) {
			t.Errorf("expected %d generators, got %d", len(expectedGenerators), len(generators))
		}
	})
}

func TestMockController_ResolveStack(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedStack := stack.NewMockStack(ctrl.ResolveInjector())
		ctrl.ResolveStackFunc = func() stack.Stack {
			return expectedStack
		}

		stack := ctrl.ResolveStack()

		if stack != expectedStack {
			t.Errorf("expected stack %v, got %v", expectedStack, stack)
		}
	})
}

func TestMockController_ResolveBlueprintHandler(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedHandler := blueprint.NewMockBlueprintHandler(ctrl.ResolveInjector())
		ctrl.ResolveBlueprintHandlerFunc = func() blueprint.BlueprintHandler {
			return expectedHandler
		}

		handler := ctrl.ResolveBlueprintHandler()

		if handler != expectedHandler {
			t.Errorf("expected handler %v, got %v", expectedHandler, handler)
		}
	})
}

func TestMockController_ResolveAllSecretsProviders(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedProviders := []secrets.SecretsProvider{secrets.NewMockSecretsProvider(ctrl.ResolveInjector())}
		ctrl.ResolveAllSecretsProvidersFunc = func() []secrets.SecretsProvider {
			return expectedProviders
		}

		providers := ctrl.ResolveAllSecretsProviders()

		if len(providers) != len(expectedProviders) {
			t.Errorf("expected %d providers, got %d", len(expectedProviders), len(providers))
		}
	})
}

func TestMockController_SetEnvironmentVariables(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedErr := fmt.Errorf("mock error")
		ctrl.SetEnvironmentVariablesFunc = func() error {
			return expectedErr
		}

		err := ctrl.SetEnvironmentVariables()

		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})
}

func TestMockController_CreateComponents(t *testing.T) {
	t.Run("WithMockFunc", func(t *testing.T) {
		ctrl := NewMockController()
		expectedErr := fmt.Errorf("mock error")
		ctrl.CreateComponentsFunc = func() error {
			return expectedErr
		}

		err := ctrl.CreateComponents()

		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})
}

func TestNewMockController(t *testing.T) {
	t.Run("WithoutInjector", func(t *testing.T) {
		ctrl := NewMockController()

		if ctrl == nil {
			t.Fatal("expected controller to not be nil")
		}
		if ctrl.injector == nil {
			t.Error("expected injector to not be nil")
		}
	})

	t.Run("WithInjector", func(t *testing.T) {
		injector := di.NewInjector()
		ctrl := NewMockController(injector)

		if ctrl == nil {
			t.Fatal("expected controller to not be nil")
		}
		if ctrl.injector != injector {
			t.Error("expected provided injector to be used")
		}
	})
}

func TestNewMockConstructors(t *testing.T) {
	t.Run("CreatesAllMockConstructors", func(t *testing.T) {
		constructors := NewMockConstructors()
		injector := di.NewInjector()

		// Test shell constructors
		shell := constructors.NewShell(injector)
		if shell == nil {
			t.Error("expected NewShell to return non-nil")
		}
		secureShell := constructors.NewSecureShell(injector)
		if secureShell == nil {
			t.Error("expected NewSecureShell to return non-nil")
		}

		// Test config handler constructor
		configHandler := constructors.NewConfigHandler(injector)
		if configHandler == nil {
			t.Error("expected NewConfigHandler to return non-nil")
		}

		// Test tools manager constructor
		toolsManager := constructors.NewToolsManager(injector)
		if toolsManager == nil {
			t.Error("expected NewToolsManager to return non-nil")
		}

		// Test blueprint handler constructor
		blueprintHandler := constructors.NewBlueprintHandler(injector)
		if blueprintHandler == nil {
			t.Error("expected NewBlueprintHandler to return non-nil")
		}

		// Test generator constructors
		gitGenerator := constructors.NewGitGenerator(injector)
		if gitGenerator == nil {
			t.Error("expected NewGitGenerator to return non-nil")
		}
		terraformGenerator := constructors.NewTerraformGenerator(injector)
		if terraformGenerator == nil {
			t.Error("expected NewTerraformGenerator to return non-nil")
		}
		kustomizeGenerator := constructors.NewKustomizeGenerator(injector)
		if kustomizeGenerator == nil {
			t.Error("expected NewKustomizeGenerator to return non-nil")
		}

		// Test env printer constructors
		awsEnvPrinter := constructors.NewAwsEnvPrinter(injector)
		if awsEnvPrinter == nil {
			t.Error("expected NewAwsEnvPrinter to return non-nil")
		}
		dockerEnvPrinter := constructors.NewDockerEnvPrinter(injector)
		if dockerEnvPrinter == nil {
			t.Error("expected NewDockerEnvPrinter to return non-nil")
		}
		kubeEnvPrinter := constructors.NewKubeEnvPrinter(injector)
		if kubeEnvPrinter == nil {
			t.Error("expected NewKubeEnvPrinter to return non-nil")
		}
		omniEnvPrinter := constructors.NewOmniEnvPrinter(injector)
		if omniEnvPrinter == nil {
			t.Error("expected NewOmniEnvPrinter to return non-nil")
		}
		talosEnvPrinter := constructors.NewTalosEnvPrinter(injector)
		if talosEnvPrinter == nil {
			t.Error("expected NewTalosEnvPrinter to return non-nil")
		}
		terraformEnvPrinter := constructors.NewTerraformEnvPrinter(injector)
		if terraformEnvPrinter == nil {
			t.Error("expected NewTerraformEnvPrinter to return non-nil")
		}
		windsorEnvPrinter := constructors.NewWindsorEnvPrinter(injector)
		if windsorEnvPrinter == nil {
			t.Error("expected NewWindsorEnvPrinter to return non-nil")
		}

		// Test service constructors
		dnsService := constructors.NewDNSService(injector)
		if dnsService == nil {
			t.Error("expected NewDNSService to return non-nil")
		}
		gitLivereloadService := constructors.NewGitLivereloadService(injector)
		if gitLivereloadService == nil {
			t.Error("expected NewGitLivereloadService to return non-nil")
		}
		localstackService := constructors.NewLocalstackService(injector)
		if localstackService == nil {
			t.Error("expected NewLocalstackService to return non-nil")
		}
		registryService := constructors.NewRegistryService(injector)
		if registryService == nil {
			t.Error("expected NewRegistryService to return non-nil")
		}
		talosService := constructors.NewTalosService(injector, "test")
		if talosService == nil {
			t.Error("expected NewTalosService to return non-nil")
		}

		// Test virtualization constructors
		sshClient := constructors.NewSSHClient()
		if sshClient == nil {
			t.Error("expected NewSSHClient to return non-nil")
		}
		colimaVirt := constructors.NewColimaVirt(injector)
		if colimaVirt == nil {
			t.Error("expected NewColimaVirt to return non-nil")
		}
		colimaNetworkManager := constructors.NewColimaNetworkManager(injector)
		if colimaNetworkManager == nil {
			t.Error("expected NewColimaNetworkManager to return non-nil")
		}
		baseNetworkManager := constructors.NewBaseNetworkManager(injector)
		if baseNetworkManager == nil {
			t.Error("expected NewBaseNetworkManager to return non-nil")
		}
		dockerVirt := constructors.NewDockerVirt(injector)
		if dockerVirt == nil {
			t.Error("expected NewDockerVirt to return non-nil")
		}
		networkInterfaceProvider := constructors.NewNetworkInterfaceProvider()
		if networkInterfaceProvider == nil {
			t.Error("expected NewNetworkInterfaceProvider to return non-nil")
		}

		// Test secrets provider constructors
		sopsSecretsProvider := constructors.NewSopsSecretsProvider("test", injector)
		if sopsSecretsProvider == nil {
			t.Error("expected NewSopsSecretsProvider to return non-nil")
		}
		onePasswordSDKSecretsProvider := constructors.NewOnePasswordSDKSecretsProvider(secretsConfigType.OnePasswordVault{}, injector)
		if onePasswordSDKSecretsProvider == nil {
			t.Error("expected NewOnePasswordSDKSecretsProvider to return non-nil")
		}
		onePasswordCLISecretsProvider := constructors.NewOnePasswordCLISecretsProvider(secretsConfigType.OnePasswordVault{}, injector)
		if onePasswordCLISecretsProvider == nil {
			t.Error("expected NewOnePasswordCLISecretsProvider to return non-nil")
		}

		// Test stack constructor
		windsorStack := constructors.NewWindsorStack(injector)
		if windsorStack == nil {
			t.Error("expected NewWindsorStack to return non-nil")
		}

		// Test bundler constructors
		artifactBuilder := constructors.NewArtifactBuilder(injector)
		if artifactBuilder == nil {
			t.Error("expected NewArtifactBuilder to return non-nil")
		}
		templateBundler := constructors.NewTemplateBundler(injector)
		if templateBundler == nil {
			t.Error("expected NewTemplateBundler to return non-nil")
		}
		kustomizeBundler := constructors.NewKustomizeBundler(injector)
		if kustomizeBundler == nil {
			t.Error("expected NewKustomizeBundler to return non-nil")
		}
		terraformBundler := constructors.NewTerraformBundler(injector)
		if terraformBundler == nil {
			t.Error("expected NewTerraformBundler to return non-nil")
		}
	})
}
