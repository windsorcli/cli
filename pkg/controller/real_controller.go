package controller

import (
	"fmt"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/generators"
	"github.com/windsorcli/cli/pkg/network"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/services"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/ssh"
	"github.com/windsorcli/cli/pkg/stack"
	"github.com/windsorcli/cli/pkg/tools"
	"github.com/windsorcli/cli/pkg/virt"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
)

// RealController struct implements the RealController interface.
type RealController struct {
	BaseController
}

// NewRealController creates a new controller.
func NewRealController(injector di.Injector) *RealController {
	return &RealController{
		BaseController: BaseController{
			injector: injector,
		},
	}
}

// Ensure RealController implements the Controller interface
var _ Controller = (*RealController)(nil)

// CreateCommonComponents sets up config and shell for command execution.
// It registers and initializes these components.
func (c *RealController) CreateCommonComponents() error {
	configHandler := config.NewYamlConfigHandler(c.injector)
	c.injector.Register("configHandler", configHandler)
	c.configHandler = configHandler

	shell := shell.NewDefaultShell(c.injector)
	c.injector.Register("shell", shell)

	// Testing Note: The following is hard to test as these are registered
	// above and can't be mocked externally. There may be a better way to
	// organize this in the future but this works for now, so we don't expect
	// these lines to be covered by tests.

	// Initialize the config handler
	if err := configHandler.Initialize(); err != nil {
		return fmt.Errorf("error initializing config handler: %w", err)
	}

	// Initialize the shell
	if err := shell.Initialize(); err != nil {
		return fmt.Errorf("error initializing shell: %w", err)
	}

	return nil
}

// Initializes project components like generators and tools manager. Registers
// and initializes blueprint, terraform, and kustomize generators. Determines
// and sets the tools manager: aqua, asdf, or default, based on config or setup.
func (c *RealController) CreateProjectComponents() error {
	gitGenerator := generators.NewGitGenerator(c.injector)
	c.injector.Register("gitGenerator", gitGenerator)

	blueprintHandler := blueprint.NewBlueprintHandler(c.injector)
	c.injector.Register("blueprintHandler", blueprintHandler)

	terraformGenerator := generators.NewTerraformGenerator(c.injector)
	c.injector.Register("terraformGenerator", terraformGenerator)

	kustomizeGenerator := generators.NewKustomizeGenerator(c.injector)
	c.injector.Register("kustomizeGenerator", kustomizeGenerator)

	toolsManagerType := c.configHandler.GetString("toolsManager")
	var toolsManager tools.ToolsManager

	if toolsManagerType == "" {
		var err error
		toolsManagerType, err = tools.CheckExistingToolsManager(c.configHandler.GetString("projectRoot"))
		if err != nil {
			// Not tested as this is a static function and we can't mock it
			return fmt.Errorf("error checking existing tools manager: %w", err)
		}
	}

	switch toolsManagerType {
	// case "aqua":
	// TODO: Implement aqua tools manager
	// case "asdf":
	// TODO: Implement asdf tools manager
	default:
		toolsManager = tools.NewToolsManager(c.injector)
	}

	c.injector.Register("toolsManager", toolsManager)

	return nil
}

// CreateEnvComponents creates components required for env and exec commands
// Registers environment printers for AWS, Docker, Kube, Omni, Talos, Terraform, and Windsor.
// AWS and Docker printers are conditional on their respective configurations being enabled.
// Each printer is created and registered with the dependency injector.
// Returns nil on successful registration of all environment components.
func (c *RealController) CreateEnvComponents() error {
	envPrinters := map[string]func(di.Injector) env.EnvPrinter{
		"awsEnv":       func(injector di.Injector) env.EnvPrinter { return env.NewAwsEnvPrinter(injector) },
		"dockerEnv":    func(injector di.Injector) env.EnvPrinter { return env.NewDockerEnvPrinter(injector) },
		"kubeEnv":      func(injector di.Injector) env.EnvPrinter { return env.NewKubeEnvPrinter(injector) },
		"omniEnv":      func(injector di.Injector) env.EnvPrinter { return env.NewOmniEnvPrinter(injector) },
		"talosEnv":     func(injector di.Injector) env.EnvPrinter { return env.NewTalosEnvPrinter(injector) },
		"terraformEnv": func(injector di.Injector) env.EnvPrinter { return env.NewTerraformEnvPrinter(injector) },
		"windsorEnv":   func(injector di.Injector) env.EnvPrinter { return env.NewWindsorEnvPrinter(injector) },
		"customEnv":    func(injector di.Injector) env.EnvPrinter { return env.NewCustomEnvPrinter(injector) },
	}

	for key, constructor := range envPrinters {
		if key == "awsEnv" && !c.configHandler.GetBool("aws.enabled") {
			continue
		}
		if key == "dockerEnv" && !c.configHandler.GetBool("docker.enabled") {
			continue
		}
		envPrinter := constructor(c.injector)
		c.injector.Register(key, envPrinter)
	}

	return nil
}

// CreateServiceComponents sets up services based on config, including DNS,
// Git livereload, Localstack, and Docker registries. If Talos is used, it
// registers control plane and worker services for the cluster.
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
		// Not unit tested currently as we can't easily create registry entries in tests
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
				controlPlaneService := services.NewTalosService(c.injector, "controlplane")
				controlPlaneService.SetName(fmt.Sprintf("controlplane-%d", i))
				serviceName := fmt.Sprintf("clusterNode.controlplane-%d", i)
				c.injector.Register(serviceName, controlPlaneService)
			}
			for i := 1; i <= workerCount; i++ {
				workerService := services.NewTalosService(c.injector, "worker")
				workerService.SetName(fmt.Sprintf("worker-%d", i))
				serviceName := fmt.Sprintf("clusterNode.worker-%d", i)
				c.injector.Register(serviceName, workerService)
			}
		}
	}

	return nil
}

// CreateVirtualizationComponents sets up virtualization based on config.
// Registers network, SSH, and VM components for Colima. Adds Docker runtime if enabled.
func (c *RealController) CreateVirtualizationComponents() error {
	configHandler := c.configHandler

	vmDriver := configHandler.GetString("vm.driver")
	dockerEnabled := configHandler.GetBool("docker.enabled")

	if vmDriver == "colima" {
		networkInterfaceProvider := &network.RealNetworkInterfaceProvider{}
		c.injector.Register("networkInterfaceProvider", networkInterfaceProvider)

		sshClient := ssh.NewSSHClient()
		c.injector.Register("sshClient", sshClient)

		secureShell := shell.NewSecureShell(c.injector)
		c.injector.Register("secureShell", secureShell)

		colimaVirtualMachine := virt.NewColimaVirt(c.injector)
		c.injector.Register("virtualMachine", colimaVirtualMachine)

		networkManager := network.NewColimaNetworkManager(c.injector)
		c.injector.Register("networkManager", networkManager)
	} else {
		networkManager := network.NewBaseNetworkManager(c.injector)
		c.injector.Register("networkManager", networkManager)
	}

	if dockerEnabled {
		containerRuntime := virt.NewDockerVirt(c.injector)
		c.injector.Register("containerRuntime", containerRuntime)
	}

	return nil
}

// CreateStackComponents creates stack components
func (c *RealController) CreateStackComponents() error {
	stackInstance := stack.NewWindsorStack(c.injector)
	c.injector.Register("stack", stackInstance)

	return nil
}

// CreateSecretsProviders sets up the secrets provider based on config settings.
// It supports SOPS and 1Password CLI for decryption.
// Registers the appropriate secrets provider with the injector.
func (c *RealController) CreateSecretsProviders() error {
	contextName := c.configHandler.GetContext()
	configRoot, err := c.configHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	var secretsProvider secrets.SecretsProvider

	secretsFilePaths := []string{"secrets.enc.yaml", "secrets.enc.yml"}
	for _, filePath := range secretsFilePaths {
		if _, err := osStat(filepath.Join(configRoot, filePath)); err == nil {
			secretsProvider = secrets.NewSopsSecretsProvider(configRoot)
			c.injector.Register("sopsSecretsProvider", secretsProvider)
			break
		}
	}

	vaults, ok := c.configHandler.Get(fmt.Sprintf("contexts.%s.secrets.onepassword.vaults", contextName)).(map[string]secretsConfigType.OnePasswordVault)
	if ok && len(vaults) > 0 {
		for _, vault := range vaults {
			secretsProvider = secrets.NewOnePasswordCLISecretsProvider(vault)
			c.injector.Register("onePasswordSecretsProvider", secretsProvider)
			break
		}
	}

	return nil
}

// Ensure RealController implements the Controller interface
var _ Controller = (*RealController)(nil)
