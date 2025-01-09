package controller

import (
	"fmt"

	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
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

// RealController struct implements the RealController interface.
type RealController struct {
	BaseController
}

// NewRealController creates a new controller.
func NewRealController(injector di.Injector) *RealController {
	return &RealController{BaseController: BaseController{injector: injector}}
}

// Ensure RealController implements the Controller interface
var _ Controller = (*RealController)(nil)

// CreateCommonComponents creates components commonly used by all commands.
func (c *RealController) CreateCommonComponents() error {
	// Create a new configHandler
	configHandler := config.NewYamlConfigHandler(c.injector)
	c.injector.Register("configHandler", configHandler)

	// Set the configHandler
	c.configHandler = configHandler

	// Create a new shell
	shell := shell.NewDefaultShell(c.injector)
	c.injector.Register("shell", shell)

	// Testing Note: The following is hard to test as these are registered
	// above and can't be mocked externally. There may be a better way to
	// organize this in the future but this works for now, so we don't expect
	// these lines to be covered by tests.

	// Initialize the shell
	if err := shell.Initialize(); err != nil {
		return fmt.Errorf("error initializing shell: %w", err)
	}

	return nil
}

// CreateProjectComponents creates components required for project initialization
func (c *RealController) CreateProjectComponents() error {
	// Create a new git generator
	gitGenerator := generators.NewGitGenerator(c.injector)
	c.injector.Register("gitGenerator", gitGenerator)

	// Create a new blueprint handler
	blueprintHandler := blueprint.NewBlueprintHandler(c.injector)
	c.injector.Register("blueprintHandler", blueprintHandler)

	// Create a new terraform generator
	terraformGenerator := generators.NewTerraformGenerator(c.injector)
	c.injector.Register("terraformGenerator", terraformGenerator)

	// Create a new kustomize generator
	kustomizeGenerator := generators.NewKustomizeGenerator(c.injector)
	c.injector.Register("kustomizeGenerator", kustomizeGenerator)

	return nil
}

// CreateEnvComponents creates components required for env and exec commands
func (c *RealController) CreateEnvComponents() error {
	// Create aws env printer only if aws.enabled is true
	if c.configHandler.GetBool("aws.enabled") {
		awsEnv := env.NewAwsEnvPrinter(c.injector)
		c.injector.Register("awsEnv", awsEnv)
	}

	// Create docker env printer only if docker is enabled
	if c.configHandler.GetBool("docker.enabled") {
		dockerEnv := env.NewDockerEnvPrinter(c.injector)
		c.injector.Register("dockerEnv", dockerEnv)
	}

	// Create kube env printer
	kubeEnv := env.NewKubeEnvPrinter(c.injector)
	c.injector.Register("kubeEnv", kubeEnv)

	// Create omni env printer
	omniEnv := env.NewOmniEnvPrinter(c.injector)
	c.injector.Register("omniEnv", omniEnv)

	// Create sops env printer
	sopsEnv := env.NewSopsEnvPrinter(c.injector)
	c.injector.Register("sopsEnv", sopsEnv)

	// Create talos env printer
	talosEnv := env.NewTalosEnvPrinter(c.injector)
	c.injector.Register("talosEnv", talosEnv)

	// Create terraform env printer
	terraformEnv := env.NewTerraformEnvPrinter(c.injector)
	c.injector.Register("terraformEnv", terraformEnv)

	// Create windsor env printer
	windsorEnv := env.NewWindsorEnvPrinter(c.injector)
	c.injector.Register("windsorEnv", windsorEnv)

	return nil
}

// CreateServiceComponents initializes and registers various services based on configuration settings.
// It checks if Docker is enabled before proceeding to create DNS, Git livereload, and Localstack services
// if their respective configurations are enabled. It also sets up registry services for each configured
// Docker registry, appending the DNS TLD to their names. Additionally, if the cluster is enabled and
// uses the Talos driver, it creates and registers control plane and worker services based on the
// specified counts.
func (c *RealController) CreateServiceComponents() error {
	configHandler := c.configHandler
	contextConfig := configHandler.GetConfig()

	if !configHandler.GetBool("docker.enabled") {
		return nil
	}

	dnsEnabled := configHandler.GetBool("dns.enabled")
	if dnsEnabled {
		dnsService := services.NewDNSService(c.injector)
		c.injector.Register("dnsService", dnsService)
	}

	gitLivereloadEnabled := configHandler.GetBool("git.livereload.enabled")
	if gitLivereloadEnabled {
		gitLivereloadService := services.NewGitLivereloadService(c.injector)
		c.injector.Register("gitLivereloadService", gitLivereloadService)
	}

	localstackEnabled := configHandler.GetBool("aws.localstack.enabled")
	if localstackEnabled {
		localstackService := services.NewLocalstackService(c.injector)
		c.injector.Register("localstackService", localstackService)
	}

	if contextConfig.Docker != nil && contextConfig.Docker.Registries != nil {
		for key := range contextConfig.Docker.Registries {
			service := services.NewRegistryService(c.injector)
			service.SetName(key)
			serviceName := fmt.Sprintf("registryService.%s", key)
			c.injector.Register(serviceName, service)
		}
	}

	clusterEnabled := configHandler.GetBool("cluster.enabled")
	if clusterEnabled {
		controlPlaneCount := configHandler.GetInt("cluster.controlplanes.count")
		workerCount := configHandler.GetInt("cluster.workers.count")

		clusterDriver := configHandler.GetString("cluster.driver")

		if clusterDriver == "talos" {
			for i := 1; i <= controlPlaneCount; i++ {
				controlPlaneService := services.NewTalosControlPlaneService(c.injector)
				controlPlaneService.SetName(fmt.Sprintf("controlplane-%d", i))
				serviceName := fmt.Sprintf("clusterNode.controlplane-%d", i)
				c.injector.Register(serviceName, controlPlaneService)
			}
			for i := 1; i <= workerCount; i++ {
				workerService := services.NewTalosWorkerService(c.injector)
				workerService.SetName(fmt.Sprintf("worker-%d", i))
				serviceName := fmt.Sprintf("clusterNode.worker-%d", i)
				c.injector.Register(serviceName, workerService)
			}
		}
	}

	return nil
}

// CreateVirtualizationComponents creates virtualization components
func (c *RealController) CreateVirtualizationComponents() error {
	configHandler := c.configHandler

	vmDriver := configHandler.GetString("vm.driver")
	dockerEnabled := configHandler.GetBool("docker.enabled")

	if vmDriver == "colima" {
		// Create and register the RealNetworkInterfaceProvider instance
		networkInterfaceProvider := &network.RealNetworkInterfaceProvider{}
		c.injector.Register("networkInterfaceProvider", networkInterfaceProvider)

		// Create and register the ssh client
		sshClient := ssh.NewSSHClient()
		c.injector.Register("sshClient", sshClient)

		// Create and register the secure shell
		secureShell := shell.NewSecureShell(c.injector)
		c.injector.Register("secureShell", secureShell)

		// Create a colima virtual machine
		colimaVirtualMachine := virt.NewColimaVirt(c.injector)
		c.injector.Register("virtualMachine", colimaVirtualMachine)

		// Create a colima network manager
		networkManager := network.NewColimaNetworkManager(c.injector)
		c.injector.Register("networkManager", networkManager)
	} else {
		// Create a base network manager
		networkManager, err := network.NewBaseNetworkManager(c.injector)
		if err != nil {
			return fmt.Errorf("error creating base network manager: %w", err)
		}
		c.injector.Register("networkManager", networkManager)
	}

	// Create docker container runtime
	if dockerEnabled {
		containerRuntime := virt.NewDockerVirt(c.injector)
		c.injector.Register("containerRuntime", containerRuntime)
	}

	return nil
}

// CreateStackComponents creates stack components
func (c *RealController) CreateStackComponents() error {
	// Create a new stack
	stackInstance := stack.NewWindsorStack(c.injector)
	c.injector.Register("stack", stackInstance)

	return nil
}

// Ensure RealController implements the Controller interface
var _ Controller = (*RealController)(nil)
