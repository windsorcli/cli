package pipelines

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/cluster"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	envpkg "github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/generators"
	"github.com/windsorcli/cli/pkg/kubernetes"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/stack"
	"github.com/windsorcli/cli/pkg/terraform"
	"github.com/windsorcli/cli/pkg/tools"
	"github.com/windsorcli/cli/pkg/workstation/network"
	"github.com/windsorcli/cli/pkg/workstation/services"
	"github.com/windsorcli/cli/pkg/workstation/ssh"
	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// The BasePipeline is a foundational component that provides common pipeline functionality for command execution.
// It provides a unified interface for pipeline execution with dependency injection support,
// serving as the base implementation for all command-specific pipelines in the Windsor CLI system.
// The BasePipeline facilitates standardized command execution patterns with direct dependency injection.

// =============================================================================
// Types
// =============================================================================

// Pipeline defines the interface for all command pipelines
type Pipeline interface {
	Initialize(injector di.Injector, ctx context.Context) error
	Execute(ctx context.Context) error
}

// PipelineConstructor defines a function that creates a new pipeline instance
type PipelineConstructor func() Pipeline

// =============================================================================
// Pipeline Factory
// =============================================================================

// pipelineConstructors maps pipeline names to their constructor functions
var pipelineConstructors = map[string]PipelineConstructor{
	"initPipeline":    func() Pipeline { return NewInitPipeline() },
	"execPipeline":    func() Pipeline { return NewExecPipeline() },
	"checkPipeline":   func() Pipeline { return NewCheckPipeline() },
	"upPipeline":      func() Pipeline { return NewUpPipeline() },
	"downPipeline":    func() Pipeline { return NewDownPipeline() },
	"installPipeline": func() Pipeline { return NewInstallPipeline() },
	"buildIDPipeline": func() Pipeline { return NewBuildIDPipeline() },
	"basePipeline":    func() Pipeline { return NewBasePipeline() },
}

// WithPipeline resolves or creates a pipeline instance from the DI container by name.
// If the pipeline already exists in the injector, it is returned directly. Otherwise, a new instance is constructed,
// initialized with the provided injector and context, registered in the DI container, and then returned.
// Returns an error if the pipeline name is unknown or initialization fails.
func WithPipeline(injector di.Injector, ctx context.Context, pipelineName string) (Pipeline, error) {
	if existing := injector.Resolve(pipelineName); existing != nil {
		if pipeline, ok := existing.(Pipeline); ok {
			return pipeline, nil
		}
	}

	constructor, exists := pipelineConstructors[pipelineName]
	if !exists {
		return nil, fmt.Errorf("unknown pipeline: %s", pipelineName)
	}

	pipeline := constructor()

	if err := pipeline.Initialize(injector, ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize %s: %w", pipelineName, err)
	}

	injector.Register(pipelineName, pipeline)

	return pipeline, nil
}

// BasePipeline provides common pipeline functionality including config loading and template processing
// Specific pipelines should embed this and add their own dependencies
type BasePipeline struct {
	shell            shell.Shell
	configHandler    config.ConfigHandler
	shims            *Shims
	injector         di.Injector
	artifactBuilder  artifact.Artifact
	blueprintHandler blueprint.BlueprintHandler
}

// =============================================================================
// Constructor
// =============================================================================

// NewBasePipeline creates a new BasePipeline instance
func NewBasePipeline() *BasePipeline {
	return &BasePipeline{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize sets up the base pipeline components including dependency injection container,
// shell interface, configuration handler, and shims. It resolves dependencies from the DI
// container and initializes core components required for pipeline execution.
func (p *BasePipeline) Initialize(injector di.Injector, ctx context.Context) error {
	p.injector = injector

	p.shell = p.withShell()
	p.configHandler = p.withConfigHandler()
	p.shims = p.withShims()
	p.artifactBuilder = p.withArtifactBuilder()
	p.blueprintHandler = p.withBlueprintHandler()

	if err := p.shell.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize shell: %w", err)
	}

	// Add current directory to trusted file if trust context is set
	if trust, ok := ctx.Value("trust").(bool); ok && trust {
		if err := p.shell.AddCurrentDirToTrustedFile(); err != nil {
			return fmt.Errorf("failed to add current directory to trusted file: %w", err)
		}
	}

	// Set shell verbosity based on context
	if verbose, ok := ctx.Value("verbose").(bool); ok && verbose {
		p.shell.SetVerbosity(true)
	}

	if err := p.configHandler.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize config handler: %w", err)
	}

	// Load base config first
	if err := p.loadBaseConfig(); err != nil {
		return fmt.Errorf("failed to load base config: %w", err)
	}

	// Set Windsor context if specified in execution context
	if contextName, ok := ctx.Value("contextName").(string); ok && contextName != "" {
		if err := p.configHandler.SetContext(contextName); err != nil {
			return fmt.Errorf("failed to set Windsor context: %w", err)
		}
	}

	// Load context config after context is set (only if not in init pipeline)
	// Init pipeline doesn't load config because files don't exist yet
	isInit, _ := ctx.Value("initPipeline").(bool)
	if !isInit {
		if err := p.configHandler.LoadConfig(); err != nil {
			return fmt.Errorf("failed to load context config: %w", err)
		}
	} else {
	}

	return nil
}

// Execute provides a default implementation that can be overridden by specific pipelines
func (p *BasePipeline) Execute(ctx context.Context) error {
	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// withShell resolves or creates shell from DI container
func (p *BasePipeline) withShell() shell.Shell {
	if existing := p.injector.Resolve("shell"); existing != nil {
		if shell, ok := existing.(shell.Shell); ok {
			p.shell = shell
			return shell
		}
	}

	p.shell = shell.NewDefaultShell(p.injector)
	p.injector.Register("shell", p.shell)
	return p.shell
}

// withConfigHandler resolves or creates config handler from DI container
func (p *BasePipeline) withConfigHandler() config.ConfigHandler {
	if existing := p.injector.Resolve("configHandler"); existing != nil {
		if configHandler, ok := existing.(config.ConfigHandler); ok {
			p.configHandler = configHandler
			return configHandler
		}
	}

	p.configHandler = config.NewConfigHandler(p.injector)
	p.injector.Register("configHandler", p.configHandler)
	return p.configHandler
}

// withShims resolves or creates shims from DI container
func (p *BasePipeline) withShims() *Shims {
	if existing := p.injector.Resolve("shims"); existing != nil {
		if shims, ok := existing.(*Shims); ok {
			p.shims = shims
			return shims
		}
	}

	p.shims = NewShims()
	p.injector.Register("shims", p.shims)
	return p.shims
}

// withToolsManager resolves or creates tools manager from DI container
func (p *BasePipeline) withToolsManager() tools.ToolsManager {
	if existing := p.injector.Resolve("toolsManager"); existing != nil {
		if toolsManager, ok := existing.(tools.ToolsManager); ok {
			return toolsManager
		}
	}

	toolsManager := tools.NewToolsManager(p.injector)
	p.injector.Register("toolsManager", toolsManager)
	return toolsManager
}

// withClusterClient resolves or creates cluster client from DI container
func (p *BasePipeline) withClusterClient() cluster.ClusterClient {
	if existing := p.injector.Resolve("clusterClient"); existing != nil {
		if clusterClient, ok := existing.(cluster.ClusterClient); ok {
			return clusterClient
		}
	}

	clusterClient := cluster.NewTalosClusterClient(p.injector)
	p.injector.Register("clusterClient", clusterClient)
	return clusterClient
}

// withBlueprintHandler resolves or creates blueprint handler from DI container
func (p *BasePipeline) withBlueprintHandler() blueprint.BlueprintHandler {
	if existing := p.injector.Resolve("blueprintHandler"); existing != nil {
		if handler, ok := existing.(blueprint.BlueprintHandler); ok {
			return handler
		}
	}

	handler := blueprint.NewBlueprintHandler(p.injector)
	p.injector.Register("blueprintHandler", handler)
	return handler
}

// withStack resolves or creates stack from DI container
func (p *BasePipeline) withStack() stack.Stack {
	if existing := p.injector.Resolve("stack"); existing != nil {
		if stack, ok := existing.(stack.Stack); ok {
			return stack
		}
	}

	stack := stack.NewWindsorStack(p.injector)
	p.injector.Register("stack", stack)
	return stack
}

// withGenerators creates and registers generator instances for git and terraform based on configuration.
// It always registers the git generator. The terraform generator is registered if "terraform.enabled" is true.
// Returns a slice of initialized generators or an error.
func (p *BasePipeline) withGenerators() ([]generators.Generator, error) {
	var generatorList []generators.Generator

	gitGenerator := generators.NewGitGenerator(p.injector)
	p.injector.Register("gitGenerator", gitGenerator)
	generatorList = append(generatorList, gitGenerator)

	if p.configHandler.GetBool("terraform.enabled", false) {
		terraformGenerator := generators.NewTerraformGenerator(p.injector)
		p.injector.Register("terraformGenerator", terraformGenerator)
		generatorList = append(generatorList, terraformGenerator)
	}

	return generatorList, nil
}

// withArtifactBuilder resolves or creates artifact builder from DI container
func (p *BasePipeline) withArtifactBuilder() artifact.Artifact {
	if existing := p.injector.Resolve("artifactBuilder"); existing != nil {
		if builder, ok := existing.(artifact.Artifact); ok {
			return builder
		}
	}

	builder := artifact.NewArtifactBuilder()
	p.injector.Register("artifactBuilder", builder)
	return builder
}

// withVirtualMachine resolves or creates virtual machine from DI container
func (p *BasePipeline) withVirtualMachine() virt.VirtualMachine {
	vmDriver := p.configHandler.GetString("vm.driver")
	if vmDriver == "" {
		return nil
	}

	if existing := p.injector.Resolve("virtualMachine"); existing != nil {
		if vm, ok := existing.(virt.VirtualMachine); ok {
			return vm
		}
	}

	if vmDriver == "colima" {
		vm := virt.NewColimaVirt(p.injector)
		p.injector.Register("virtualMachine", vm)
		return vm
	}

	return nil
}

// withContainerRuntime resolves or creates container runtime from DI container
func (p *BasePipeline) withContainerRuntime() virt.ContainerRuntime {
	if existing := p.injector.Resolve("containerRuntime"); existing != nil {
		if containerRuntime, ok := existing.(virt.ContainerRuntime); ok {
			return containerRuntime
		}
	}

	// Only create docker runtime if docker is enabled
	if !p.configHandler.GetBool("docker.enabled", false) {
		return nil
	}

	containerRuntime := virt.NewDockerVirt(p.injector)
	p.injector.Register("containerRuntime", containerRuntime)
	return containerRuntime
}

// withKubernetesClient resolves or creates kubernetes client from DI container
func (p *BasePipeline) withKubernetesClient() kubernetes.KubernetesClient {
	if existing := p.injector.Resolve("kubernetesClient"); existing != nil {
		if kubernetesClient, ok := existing.(kubernetes.KubernetesClient); ok {
			return kubernetesClient
		}
	}

	kubernetesClient := kubernetes.NewDynamicKubernetesClient()
	p.injector.Register("kubernetesClient", kubernetesClient)
	return kubernetesClient
}

// withKubernetesManager resolves or creates kubernetes manager from DI container
func (p *BasePipeline) withKubernetesManager() kubernetes.KubernetesManager {
	if existing := p.injector.Resolve("kubernetesManager"); existing != nil {
		if kubernetesManager, ok := existing.(kubernetes.KubernetesManager); ok {
			return kubernetesManager
		}
	}

	kubernetesManager := kubernetes.NewKubernetesManager(p.injector)
	p.injector.Register("kubernetesManager", kubernetesManager)
	return kubernetesManager
}

// withNetworking resolves or creates all networking components from DI container
func (p *BasePipeline) withNetworking() network.NetworkManager {
	// Check if network manager already exists
	if existing := p.injector.Resolve("networkManager"); existing != nil {
		if networkManager, ok := existing.(network.NetworkManager); ok {
			return networkManager
		}
	}

	// Ensure SSH client is registered
	if existing := p.injector.Resolve("sshClient"); existing == nil {
		sshClient := ssh.NewSSHClient()
		p.injector.Register("sshClient", sshClient)
	}

	// Ensure secure shell is registered
	if existing := p.injector.Resolve("secureShell"); existing == nil {
		secureShell := shell.NewSecureShell(p.injector)
		p.injector.Register("secureShell", secureShell)
	}

	// Ensure network interface provider is registered
	if existing := p.injector.Resolve("networkInterfaceProvider"); existing == nil {
		networkInterfaceProvider := network.NewNetworkInterfaceProvider()
		p.injector.Register("networkInterfaceProvider", networkInterfaceProvider)
	}

	// Create and register network manager
	vmDriver := p.configHandler.GetString("vm.driver")
	var networkManager network.NetworkManager

	if vmDriver == "colima" {
		networkManager = network.NewColimaNetworkManager(p.injector)
	} else {
		networkManager = network.NewBaseNetworkManager(p.injector)
	}

	p.injector.Register("networkManager", networkManager)
	return networkManager
}

// handleSessionReset checks session state and performs reset if needed.
// This is a common pattern used by multiple commands (env, exec, context, init).
func (p *BasePipeline) handleSessionReset() error {
	if p.shell == nil {
		return nil
	}

	hasSessionToken := os.Getenv("WINDSOR_SESSION_TOKEN") != ""

	shouldReset, err := p.shell.CheckResetFlags()
	if err != nil {
		return err
	}

	if !hasSessionToken {
		shouldReset = true
	}

	if shouldReset {
		p.shell.Reset()

		if err := os.Setenv("NO_CACHE", "true"); err != nil {
			return err
		}
	}

	return nil
}

// loadBaseConfig loads only the base configuration file (windsor.yaml) without loading context config
func (p *BasePipeline) loadBaseConfig() error {
	if p.shell == nil {
		return fmt.Errorf("shell not initialized")
	}
	if p.configHandler == nil {
		return fmt.Errorf("config handler not initialized")
	}
	if p.shims == nil {
		return fmt.Errorf("shims not initialized")
	}

	// Config is now loaded via LoadConfig() which loads from standard paths
	// Root windsor.yaml loading is handled by LoadConfig()

	return nil
}

// withEnvPrinters creates environment printers based on configuration
func (p *BasePipeline) withEnvPrinters() ([]envpkg.EnvPrinter, error) {
	if p.configHandler == nil {
		return nil, fmt.Errorf("config handler not initialized")
	}

	var envPrinters []envpkg.EnvPrinter

	if p.configHandler.GetBool("aws.enabled", false) {
		awsEnv := envpkg.NewAwsEnvPrinter(p.injector)
		envPrinters = append(envPrinters, awsEnv)
		p.injector.Register("awsEnv", awsEnv)
	}

	if p.configHandler.GetBool("azure.enabled", false) {
		azureEnv := envpkg.NewAzureEnvPrinter(p.injector)
		envPrinters = append(envPrinters, azureEnv)
		p.injector.Register("azureEnv", azureEnv)
	}

	if p.configHandler.GetBool("docker.enabled", false) {
		dockerEnv := envpkg.NewDockerEnvPrinter(p.injector)
		envPrinters = append(envPrinters, dockerEnv)
		p.injector.Register("dockerEnv", dockerEnv)
	}

	if p.configHandler.GetBool("cluster.enabled", false) {
		kubeEnv := envpkg.NewKubeEnvPrinter(p.injector)
		envPrinters = append(envPrinters, kubeEnv)
		p.injector.Register("kubeEnv", kubeEnv)
	}

	clusterDriver := p.configHandler.GetString("cluster.driver", "")
	if clusterDriver == "talos" || clusterDriver == "omni" {
		talosEnv := envpkg.NewTalosEnvPrinter(p.injector)
		envPrinters = append(envPrinters, talosEnv)
		p.injector.Register("talosEnv", talosEnv)
	}

	// Always register terraformEnv in the injector since the stack needs it
	terraformEnv := envpkg.NewTerraformEnvPrinter(p.injector)
	p.injector.Register("terraformEnv", terraformEnv)

	// Only include it in the returned array when terraform is enabled
	if p.configHandler.GetBool("terraform.enabled", false) {
		envPrinters = append(envPrinters, terraformEnv)
	}

	windsorEnv := envpkg.NewWindsorEnvPrinter(p.injector)
	envPrinters = append(envPrinters, windsorEnv)
	p.injector.Register("windsorEnv", windsorEnv)

	return envPrinters, nil
}

// withSecretsProviders creates secrets providers based on configuration
func (p *BasePipeline) withSecretsProviders() ([]secrets.SecretsProvider, error) {
	if p.configHandler == nil {
		return nil, fmt.Errorf("config handler not initialized")
	}

	var secretsProviders []secrets.SecretsProvider

	configRoot, err := p.configHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error getting config root: %w", err)
	}

	secretsFilePaths := []string{"secrets.enc.yaml", "secrets.enc.yml"}
	for _, filePath := range secretsFilePaths {
		if _, err := p.shims.Stat(filepath.Join(configRoot, filePath)); err == nil {
			secretsProviders = append(secretsProviders, secrets.NewSopsSecretsProvider(configRoot, p.injector))
			break
		}
	}

	vaults, ok := p.configHandler.Get("secrets.onepassword.vaults").(map[string]secretsConfigType.OnePasswordVault)
	if ok && len(vaults) > 0 {
		useSDK := p.shims.Getenv("OP_SERVICE_ACCOUNT_TOKEN") != ""

		for key, vault := range vaults {
			vaultCopy := vault
			vaultCopy.ID = key

			if useSDK {
				secretsProviders = append(secretsProviders, secrets.NewOnePasswordSDKSecretsProvider(vaultCopy, p.injector))
			} else {
				secretsProviders = append(secretsProviders, secrets.NewOnePasswordCLISecretsProvider(vaultCopy, p.injector))
			}
		}
	}

	return secretsProviders, nil
}

// withServices creates and configures service instances based on the current configuration.
// Services are created conditionally based on feature flags and configuration settings.
// Each service is registered in the DI container with appropriate naming conventions.
func (p *BasePipeline) withServices() ([]services.Service, error) {
	if p.configHandler == nil {
		return nil, fmt.Errorf("config handler not initialized")
	}

	var serviceList []services.Service

	dockerEnabled := p.configHandler.GetBool("docker.enabled", false)
	if !dockerEnabled {
		return serviceList, nil
	}

	dnsEnabled := p.configHandler.GetBool("dns.enabled", false)
	if dnsEnabled {
		service := services.NewDNSService(p.injector)
		service.SetName("dns")
		p.injector.Register("dnsService", service)
		serviceList = append(serviceList, service)
	}

	gitEnabled := p.configHandler.GetBool("git.livereload.enabled", false)
	if gitEnabled {
		service := services.NewGitLivereloadService(p.injector)
		service.SetName("git")
		p.injector.Register("gitLivereloadService", service)
		serviceList = append(serviceList, service)
	}

	awsEnabled := p.configHandler.GetBool("aws.localstack.enabled", false)
	if awsEnabled {
		service := services.NewLocalstackService(p.injector)
		service.SetName("aws")
		p.injector.Register("localstackService", service)
		serviceList = append(serviceList, service)
	}

	contextConfig := p.configHandler.GetConfig()
	if contextConfig.Docker != nil && contextConfig.Docker.Registries != nil {
		for key := range contextConfig.Docker.Registries {
			service := services.NewRegistryService(p.injector)
			service.SetName(key)
			p.injector.Register(fmt.Sprintf("registryService.%s", key), service)
			serviceList = append(serviceList, service)
		}
	}

	// Add cluster services based on cluster driver
	clusterDriver := p.configHandler.GetString("cluster.driver", "")
	switch clusterDriver {
	case "talos", "omni":
		controlPlaneCount := p.configHandler.GetInt("cluster.controlplanes.count")
		workerCount := p.configHandler.GetInt("cluster.workers.count")

		for i := 1; i <= controlPlaneCount; i++ {
			controlPlaneService := services.NewTalosService(p.injector, "controlplane")
			serviceName := fmt.Sprintf("controlplane-%d", i)
			controlPlaneService.SetName(serviceName)
			p.injector.Register(fmt.Sprintf("clusterNode.%s", serviceName), controlPlaneService)
			serviceList = append(serviceList, controlPlaneService)
		}

		for i := 1; i <= workerCount; i++ {
			workerService := services.NewTalosService(p.injector, "worker")
			serviceName := fmt.Sprintf("worker-%d", i)
			workerService.SetName(serviceName)
			p.injector.Register(fmt.Sprintf("clusterNode.%s", serviceName), workerService)
			serviceList = append(serviceList, workerService)
		}
	case "eks", "aks":
		// For managed cloud clusters (EKS, AKS), no local cluster services are needed
		// The cluster is managed by the cloud provider
		break
	}

	return serviceList, nil
}

// withTerraformResolvers constructs and registers terraform module resolvers based on configuration state.
// If terraform.enabled is true in the configuration, the method instantiates both StandardModuleResolver and OCIModuleResolver,
// registers them in the dependency injection container, and returns them as a slice. If terraform is not enabled, returns an empty slice.
// Returns an error if the config handler is uninitialized.
func (p *BasePipeline) withTerraformResolvers() ([]terraform.ModuleResolver, error) {
	if p.configHandler == nil {
		return nil, fmt.Errorf("config handler not initialized")
	}

	var resolvers []terraform.ModuleResolver

	if !p.configHandler.GetBool("terraform.enabled", false) {
		return resolvers, nil
	}

	standardResolver := terraform.NewStandardModuleResolver(p.injector)
	p.injector.Register("standardModuleResolver", standardResolver)
	resolvers = append(resolvers, standardResolver)

	ociResolver := terraform.NewOCIModuleResolver(p.injector)
	p.injector.Register("ociModuleResolver", ociResolver)
	resolvers = append(resolvers, ociResolver)

	return resolvers, nil
}

// prepareTemplateData loads template data for pipeline execution.
// Source priority: blueprint context, local handler data, default artifact,
// then default template for current context. Returns a map of template file
// names to byte content, or error if loading fails.
func (p *BasePipeline) prepareTemplateData(ctx context.Context) (map[string][]byte, error) {
	var blueprintValue string
	if blueprintCtx := ctx.Value("blueprint"); blueprintCtx != nil {
		if blueprint, ok := blueprintCtx.(string); ok {
			blueprintValue = blueprint
		}
	}

	if blueprintValue != "" {
		if p.artifactBuilder != nil {
			ociInfo, err := artifact.ParseOCIReference(blueprintValue)
			if err != nil {
				return nil, fmt.Errorf("failed to parse blueprint reference: %w", err)
			}
			if ociInfo == nil {
				return nil, fmt.Errorf("invalid blueprint reference: %s", blueprintValue)
			}
			templateData, err := p.artifactBuilder.GetTemplateData(ociInfo.URL)
			if err != nil {
				return nil, fmt.Errorf("failed to get template data from blueprint: %w", err)
			}
			return templateData, nil
		}
	}

	if p.blueprintHandler != nil {
		blueprintTemplateData, err := p.blueprintHandler.GetLocalTemplateData()
		if err != nil {
			return nil, fmt.Errorf("failed to get local template data: %w", err)
		}

		if len(blueprintTemplateData) > 0 {
			return blueprintTemplateData, nil
		}
	}

	if p.artifactBuilder != nil {
		effectiveBlueprintURL := constants.GetEffectiveBlueprintURL()
		ociInfo, err := artifact.ParseOCIReference(effectiveBlueprintURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse default blueprint reference: %w", err)
		}
		templateData, err := p.artifactBuilder.GetTemplateData(ociInfo.URL)
		if err != nil {
			return nil, fmt.Errorf("failed to get template data from default blueprint: %w", err)
		}
		return templateData, nil
	}

	if p.blueprintHandler != nil {
		contextName := p.determineContextName(ctx)
		defaultTemplateData, err := p.blueprintHandler.GetDefaultTemplateData(contextName)
		if err != nil {
			return nil, fmt.Errorf("failed to get default template data: %w", err)
		}
		return defaultTemplateData, nil
	}

	return make(map[string][]byte), nil
}

// determineContextName selects the context name from ctx, config, or defaults to "local" if unset or "local".
func (p *BasePipeline) determineContextName(ctx context.Context) string {
	if contextName := ctx.Value("contextName"); contextName != nil {
		if name, ok := contextName.(string); ok {
			return name
		}
	}
	if p.configHandler != nil {
		currentContext := p.configHandler.GetContext()
		if currentContext != "" && currentContext != "local" {
			return currentContext
		}
	}
	return "local"
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ Pipeline = (*BasePipeline)(nil)
