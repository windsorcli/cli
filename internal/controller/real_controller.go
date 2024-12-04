package controller

import (
	"fmt"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/env"
	"github.com/windsor-hotel/cli/internal/network"
	"github.com/windsor-hotel/cli/internal/services"
	sh "github.com/windsor-hotel/cli/internal/shell"
	"github.com/windsor-hotel/cli/internal/virt"
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
	configHandler := config.NewYamlConfigHandler()
	c.injector.Register("configHandler", configHandler)

	// Set the configHandler
	c.configHandler = configHandler

	// Create a new contextHandler
	contextHandler := context.NewContextHandler(c.injector)
	c.injector.Register("contextHandler", contextHandler)

	// Create a new shell
	shell := sh.NewDefaultShell(c.injector)
	c.injector.Register("shell", shell)

	// Create a new secure shell
	secureShell := sh.NewSecureShell(c.injector)
	c.injector.Register("secureShell", secureShell)

	return nil
}

// CreateEnvComponents creates components required for env and exec commands
func (c *RealController) CreateEnvComponents() error {
	// Create aws env printer
	awsEnv := env.NewAwsEnvPrinter(c.injector)
	c.injector.Register("awsEnv", awsEnv)

	// Create docker env printer
	dockerEnv := env.NewDockerEnvPrinter(c.injector)
	c.injector.Register("dockerEnv", dockerEnv)

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

// CreateServiceComponents creates components required for services
func (c *RealController) CreateServiceComponents() error {
	configHandler := c.configHandler
	contextConfig := configHandler.GetConfig()

	// Don't create services if docker is not enabled
	if !configHandler.GetBool("docker.enabled") {
		return nil
	}

	// Create dns service
	dnsEnabled := configHandler.GetBool("dns.enabled")
	if dnsEnabled {
		dnsService := services.NewDNSService(c.injector)
		c.injector.Register("dnsService", dnsService)
	}

	// Create git livereload service
	gitLivereloadEnabled := configHandler.GetBool("git.livereload.enabled")
	if gitLivereloadEnabled {
		gitLivereloadService := services.NewGitLivereloadService(c.injector)
		c.injector.Register("gitLivereloadService", gitLivereloadService)
	}

	// Create localstack service
	localstackEnabled := configHandler.GetBool("aws.localstack.enabled")
	if localstackEnabled {
		localstackService := services.NewLocalstackService(c.injector)
		c.injector.Register("localstackService", localstackService)
	}

	// Create registry services
	registryServices := contextConfig.Docker.Registries
	for _, registry := range registryServices {
		service := services.NewRegistryService(c.injector)
		service.SetName(registry.Name)
		serviceName := fmt.Sprintf("registryService.%s", registry.Name)
		c.injector.Register(serviceName, service)
	}

	// Create cluster services
	clusterEnabled := configHandler.GetBool("cluster.enabled")
	if clusterEnabled {
		controlPlaneCount := configHandler.GetInt("cluster.controlplanes.count")
		workerCount := configHandler.GetInt("cluster.workers.count")

		clusterDriver := configHandler.GetString("cluster.driver")

		// Create a talos cluster
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

	// Create colima components
	if vmDriver == "colima" {
		// Create a colima virtual machine
		colimaVirtualMachine := virt.NewColimaVirt(c.injector)
		c.injector.Register("virtualMachine", colimaVirtualMachine)

		// Create a colima network manager
		networkManager := network.NewColimaNetworkManager(c.injector)
		c.injector.Register("networkManager", networkManager)
	}

	// Create docker container runtime
	if dockerEnabled {
		containerRuntime := virt.NewDockerVirt(c.injector)
		c.injector.Register("containerRuntime", containerRuntime)
	}

	return nil
}
