package pipelines

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/cluster"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/generators"
	"github.com/windsorcli/cli/pkg/kubernetes"
	"github.com/windsorcli/cli/pkg/network"
	"github.com/windsorcli/cli/pkg/services"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/ssh"
	"github.com/windsorcli/cli/pkg/stack"
	"github.com/windsorcli/cli/pkg/tools"
	"github.com/windsorcli/cli/pkg/virt"
)

// The InitPipeline is a specialized component that manages application initialization functionality.
// It provides initialization-specific command execution including configuration setup, context management,
// blueprint template processing, and environment preparation for the Windsor CLI init command.
// The InitPipeline handles the complete initialization workflow with proper dependency injection and validation.

// =============================================================================
// Types
// =============================================================================

// InitConstructors defines all the constructor functions required for the init pipeline.
// These constructors create the components needed for complete application initialization
// including environment setup, virtualization, services, networking, and project components.
type InitConstructors struct {
	// Core components
	NewConfigHandler    func(di.Injector) config.ConfigHandler
	NewShell            func(di.Injector) shell.Shell
	NewSecureShell      func(di.Injector) shell.Shell
	NewBlueprintHandler func(di.Injector) blueprint.BlueprintHandler
	NewShims            func() *Shims

	// Tools and project components
	NewToolsManager       func(di.Injector) tools.ToolsManager
	NewGitGenerator       func(di.Injector) generators.Generator
	NewTerraformGenerator func(di.Injector) generators.Generator

	// Service components
	NewDNSService           func(di.Injector) services.Service
	NewGitLivereloadService func(di.Injector) services.Service
	NewLocalstackService    func(di.Injector) services.Service
	NewRegistryService      func(di.Injector) services.Service
	NewTalosService         func(di.Injector, string) services.Service

	// Infrastructure components
	NewSSHClient            func() ssh.Client
	NewColimaVirt           func(di.Injector) virt.VirtualMachine
	NewDockerVirt           func(di.Injector) virt.ContainerRuntime
	NewColimaNetworkManager func(di.Injector) network.NetworkManager
	NewBaseNetworkManager   func(di.Injector) network.NetworkManager

	// Kubernetes and cluster components
	NewKubernetesManager  func(di.Injector) kubernetes.KubernetesManager
	NewKubernetesClient   func(di.Injector) kubernetes.KubernetesClient
	NewTalosClusterClient func(di.Injector) *cluster.TalosClusterClient

	// Stack and bundler components
	NewWindsorStack     func(di.Injector) stack.Stack
	NewArtifactBuilder  func() artifact.Artifact
	NewTemplateBundler  func() artifact.Bundler
	NewKustomizeBundler func() artifact.Bundler
	NewTerraformBundler func() artifact.Bundler
}

// InitPipeline provides application initialization functionality
type InitPipeline struct {
	BasePipeline

	constructors InitConstructors

	configHandler    config.ConfigHandler
	shell            shell.Shell
	blueprintHandler blueprint.BlueprintHandler
	shims            *Shims
	injector         di.Injector
}

// =============================================================================
// Constructor
// =============================================================================

// NewInitPipeline creates a new InitPipeline instance with optional constructors.
// It accepts variadic InitConstructors parameters to allow dependency injection customization.
// If no constructors are provided, it uses default implementations for all dependencies.
// Returns a fully configured InitPipeline ready for initialization.
func NewInitPipeline(constructors ...InitConstructors) *InitPipeline {
	var ctors InitConstructors
	if len(constructors) > 0 {
		ctors = constructors[0]
	} else {
		ctors = InitConstructors{
			// Core components
			NewConfigHandler: func(injector di.Injector) config.ConfigHandler {
				return config.NewYamlConfigHandler(injector)
			},
			NewShell: func(injector di.Injector) shell.Shell {
				return shell.NewDefaultShell(injector)
			},
			NewSecureShell: func(injector di.Injector) shell.Shell {
				return shell.NewSecureShell(injector)
			},
			NewBlueprintHandler: func(injector di.Injector) blueprint.BlueprintHandler {
				return blueprint.NewBlueprintHandler(injector)
			},
			NewShims: func() *Shims {
				return NewShims()
			},

			// Tools and project components
			NewToolsManager: func(injector di.Injector) tools.ToolsManager {
				return tools.NewToolsManager(injector)
			},
			NewGitGenerator: func(injector di.Injector) generators.Generator {
				return generators.NewGitGenerator(injector)
			},
			NewTerraformGenerator: func(injector di.Injector) generators.Generator {
				return generators.NewTerraformGenerator(injector)
			},

			// Service components
			NewDNSService: func(injector di.Injector) services.Service {
				return services.NewDNSService(injector)
			},
			NewGitLivereloadService: func(injector di.Injector) services.Service {
				return services.NewGitLivereloadService(injector)
			},
			NewLocalstackService: func(injector di.Injector) services.Service {
				return services.NewLocalstackService(injector)
			},
			NewRegistryService: func(injector di.Injector) services.Service {
				return services.NewRegistryService(injector)
			},
			NewTalosService: func(injector di.Injector, contextName string) services.Service {
				return services.NewTalosService(injector, contextName)
			},

			// Infrastructure components
			NewSSHClient: func() ssh.Client {
				return ssh.NewSSHClient()
			},
			NewColimaVirt: func(injector di.Injector) virt.VirtualMachine {
				return virt.NewColimaVirt(injector)
			},
			NewDockerVirt: func(injector di.Injector) virt.ContainerRuntime {
				return virt.NewDockerVirt(injector)
			},
			NewColimaNetworkManager: func(injector di.Injector) network.NetworkManager {
				return network.NewColimaNetworkManager(injector)
			},
			NewBaseNetworkManager: func(injector di.Injector) network.NetworkManager {
				return network.NewBaseNetworkManager(injector)
			},

			// Kubernetes and cluster components
			NewKubernetesManager: func(injector di.Injector) kubernetes.KubernetesManager {
				return kubernetes.NewKubernetesManager(injector)
			},
			NewKubernetesClient: func(injector di.Injector) kubernetes.KubernetesClient {
				return kubernetes.NewDynamicKubernetesClient()
			},
			NewTalosClusterClient: func(injector di.Injector) *cluster.TalosClusterClient {
				return cluster.NewTalosClusterClient(injector)
			},

			// Stack and bundler components
			NewWindsorStack: func(injector di.Injector) stack.Stack {
				return stack.NewWindsorStack(injector)
			},
			NewArtifactBuilder: func() artifact.Artifact {
				return artifact.NewArtifactBuilder()
			},
			NewTemplateBundler: func() artifact.Bundler {
				return artifact.NewTemplateBundler()
			},
			NewKustomizeBundler: func() artifact.Bundler {
				return artifact.NewKustomizeBundler()
			},
			NewTerraformBundler: func() artifact.Bundler {
				return artifact.NewTerraformBundler()
			},
		}
	}

	return &InitPipeline{
		BasePipeline: *NewBasePipeline(),
		constructors: ctors,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize creates and registers all required components for the init pipeline.
// It sets up all components needed for complete initialization including config handler,
// shell, blueprint handler, environment printers, services, virtualization, networking,
// and project components in the correct order to ensure proper dependency resolution.
func (p *InitPipeline) Initialize(injector di.Injector, ctx context.Context) error {
	p.shims = p.constructors.NewShims()

	// Create and register core components
	if existing := injector.Resolve("shell"); existing != nil {
		p.shell = existing.(shell.Shell)
	} else {
		p.shell = p.constructors.NewShell(injector)
		injector.Register("shell", p.shell)
	}
	p.BasePipeline.shell = p.shell

	if existing := injector.Resolve("secureShell"); existing != nil {
		secureShell := existing.(shell.Shell)
		injector.Register("secureShell", secureShell)
	} else if p.constructors.NewSecureShell != nil {
		secureShell := p.constructors.NewSecureShell(injector)
		injector.Register("secureShell", secureShell)
	}

	// Create SSH client if needed for secure shell or VM components
	if injector.Resolve("sshClient") == nil && p.constructors.NewSSHClient != nil {
		sshClient := p.constructors.NewSSHClient()
		injector.Register("sshClient", sshClient)
	}

	if existing := injector.Resolve("configHandler"); existing != nil {
		p.configHandler = existing.(config.ConfigHandler)
	} else {
		p.configHandler = p.constructors.NewConfigHandler(injector)
		injector.Register("configHandler", p.configHandler)
	}
	p.BasePipeline.configHandler = p.configHandler

	// Initialize config handler early since other components depend on it
	if err := p.configHandler.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize config handler: %w", err)
	}

	// Create and register blueprint handler
	if existing := injector.Resolve("blueprintHandler"); existing != nil {
		p.blueprintHandler = existing.(blueprint.BlueprintHandler)
	} else {
		p.blueprintHandler = p.constructors.NewBlueprintHandler(injector)
		injector.Register("blueprintHandler", p.blueprintHandler)
	}

	// Create tools manager
	if p.constructors.NewToolsManager != nil && injector.Resolve("toolsManager") == nil {
		toolsManager := p.constructors.NewToolsManager(injector)
		injector.Register("toolsManager", toolsManager)
	}

	// Create generators
	generatorConstructors := []struct {
		name string
		fn   func(di.Injector) generators.Generator
	}{
		{"gitGenerator", p.constructors.NewGitGenerator},
		{"terraformGenerator", p.constructors.NewTerraformGenerator},
	}

	for _, gc := range generatorConstructors {
		if gc.fn != nil && injector.Resolve(gc.name) == nil {
			generator := gc.fn(injector)
			injector.Register(gc.name, generator)
		}
	}

	// Create services only if docker is enabled
	if p.configHandler.GetBool("docker.enabled") {
		serviceConstructors := []struct {
			name string
			fn   func(di.Injector) services.Service
		}{
			{"dnsService", p.constructors.NewDNSService},
			{"gitLivereloadService", p.constructors.NewGitLivereloadService},
			{"localstackService", p.constructors.NewLocalstackService},
		}

		for _, sc := range serviceConstructors {
			if sc.fn != nil && injector.Resolve(sc.name) == nil {
				service := sc.fn(injector)
				injector.Register(sc.name, service)
			}
		}

		// Create registry services for each registry in configuration
		if p.constructors.NewRegistryService != nil {
			contextConfig := p.configHandler.GetConfig()
			if contextConfig.Docker != nil && contextConfig.Docker.Registries != nil {
				for key := range contextConfig.Docker.Registries {
					serviceName := fmt.Sprintf("registryService.%s", key)
					if injector.Resolve(serviceName) == nil {
						service := p.constructors.NewRegistryService(injector)
						service.SetName(key)
						injector.Register(serviceName, service)
					}
				}
			}
		}
	}

	// Create Talos services for cluster nodes if cluster is enabled and vm.driver is set
	vmDriver := p.configHandler.GetString("vm.driver")
	if vmDriver != "" && p.configHandler.GetBool("cluster.enabled") && p.configHandler.GetString("cluster.driver") == "talos" && p.constructors.NewTalosService != nil {
		controlPlaneCount := p.configHandler.GetInt("cluster.controlplanes.count", 1)
		workerCount := p.configHandler.GetInt("cluster.workers.count", 1)

		for i := 1; i <= controlPlaneCount; i++ {
			serviceName := fmt.Sprintf("clusterNode.controlplane-%d", i)
			if injector.Resolve(serviceName) == nil {
				controlPlaneService := p.constructors.NewTalosService(injector, "controlplane")
				controlPlaneService.SetName(fmt.Sprintf("controlplane-%d", i))
				injector.Register(serviceName, controlPlaneService)
			}
		}

		for i := 1; i <= workerCount; i++ {
			serviceName := fmt.Sprintf("clusterNode.worker-%d", i)
			if injector.Resolve(serviceName) == nil {
				workerService := p.constructors.NewTalosService(injector, "worker")
				workerService.SetName(fmt.Sprintf("worker-%d", i))
				injector.Register(serviceName, workerService)
			}
		}
	}

	// Create network manager only if vm.driver is set
	if vmDriver != "" {
		if injector.Resolve("networkManager") == nil {
			var networkManager network.NetworkManager
			if vmDriver == "colima" && p.constructors.NewColimaNetworkManager != nil {
				networkManager = p.constructors.NewColimaNetworkManager(injector)
			} else if p.constructors.NewBaseNetworkManager != nil {
				networkManager = p.constructors.NewBaseNetworkManager(injector)
			}
			if networkManager != nil {
				injector.Register("networkManager", networkManager)
			}
		}

		// Create SSH client only if vm.driver is set
		if injector.Resolve("sshClient") == nil && p.constructors.NewSSHClient != nil {
			sshClient := p.constructors.NewSSHClient()
			injector.Register("sshClient", sshClient)
		}

		// Create virtualization components only if vm.driver is set
		if injector.Resolve("virtualMachine") == nil {
			var virtualMachine virt.VirtualMachine
			if vmDriver == "colima" && p.constructors.NewColimaVirt != nil {
				virtualMachine = p.constructors.NewColimaVirt(injector)
			}
			if virtualMachine != nil {
				injector.Register("virtualMachine", virtualMachine)
			}
		}
	}

	// Create container runtime only if docker is enabled
	if p.configHandler.GetBool("docker.enabled") {
		if injector.Resolve("containerRuntime") == nil && p.constructors.NewDockerVirt != nil {
			containerRuntime := p.constructors.NewDockerVirt(injector)
			injector.Register("containerRuntime", containerRuntime)
		}
	}

	// Create cluster components
	if p.constructors.NewKubernetesManager != nil && injector.Resolve("kubernetesManager") == nil {
		kubernetesManager := p.constructors.NewKubernetesManager(injector)
		injector.Register("kubernetesManager", kubernetesManager)
	}

	if p.constructors.NewKubernetesClient != nil && injector.Resolve("kubernetesClient") == nil {
		kubernetesClient := p.constructors.NewKubernetesClient(injector)
		injector.Register("kubernetesClient", kubernetesClient)
	}

	if p.constructors.NewTalosClusterClient != nil && injector.Resolve("talosClusterClient") == nil {
		talosClusterClient := p.constructors.NewTalosClusterClient(injector)
		injector.Register("talosClusterClient", talosClusterClient)
	}

	// Create stack
	if p.constructors.NewWindsorStack != nil && injector.Resolve("stack") == nil {
		stack := p.constructors.NewWindsorStack(injector)
		injector.Register("stack", stack)
	}

	// Create artifact components
	if p.constructors.NewArtifactBuilder != nil && injector.Resolve("artifactBuilder") == nil {
		artifactBuilder := p.constructors.NewArtifactBuilder()
		injector.Register("artifactBuilder", artifactBuilder)
	}

	if p.constructors.NewTemplateBundler != nil && injector.Resolve("templateBundler") == nil {
		templateBundler := p.constructors.NewTemplateBundler()
		injector.Register("templateBundler", templateBundler)
	}

	if p.constructors.NewKustomizeBundler != nil && injector.Resolve("kustomizeBundler") == nil {
		kustomizeBundler := p.constructors.NewKustomizeBundler()
		injector.Register("kustomizeBundler", kustomizeBundler)
	}

	if p.constructors.NewTerraformBundler != nil && injector.Resolve("terraformBundler") == nil {
		terraformBundler := p.constructors.NewTerraformBundler()
		injector.Register("terraformBundler", terraformBundler)
	}

	// Initialize all components
	if sh := injector.Resolve("shell"); sh != nil {
		if err := sh.(shell.Shell).Initialize(); err != nil {
			return fmt.Errorf("error initializing shell: %w", err)
		}
	}

	if secSh := injector.Resolve("secureShell"); secSh != nil {
		if err := secSh.(shell.Shell).Initialize(); err != nil {
			return fmt.Errorf("error initializing secure shell: %w", err)
		}
	}

	// Initialize tools manager
	if toolsManager := injector.Resolve("toolsManager"); toolsManager != nil {
		if err := toolsManager.(tools.ToolsManager).Initialize(); err != nil {
			return fmt.Errorf("error initializing tools manager: %w", err)
		}
	}

	// Initialize services
	servicesList, _ := injector.ResolveAll((*services.Service)(nil))
	for _, service := range servicesList {
		if err := service.(services.Service).Initialize(); err != nil {
			return fmt.Errorf("error initializing service: %w", err)
		}
	}

	// Initialize virtualization components
	if virtualMachine := injector.Resolve("virtualMachine"); virtualMachine != nil {
		if err := virtualMachine.(virt.VirtualMachine).Initialize(); err != nil {
			return fmt.Errorf("error initializing virtual machine: %w", err)
		}
	}

	if containerRuntime := injector.Resolve("containerRuntime"); containerRuntime != nil {
		if err := containerRuntime.(virt.ContainerRuntime).Initialize(); err != nil {
			return fmt.Errorf("error initializing container runtime: %w", err)
		}
	}

	// Initialize network manager
	if networkManager := injector.Resolve("networkManager"); networkManager != nil {
		if err := networkManager.(network.NetworkManager).Initialize(); err != nil {
			return fmt.Errorf("error initializing network manager: %w", err)
		}
	}

	// Initialize blueprint handler
	if p.blueprintHandler != nil {
		if err := p.blueprintHandler.Initialize(); err != nil {
			return fmt.Errorf("error initializing blueprint handler: %w", err)
		}
		if err := p.blueprintHandler.LoadConfig(false); err != nil {
			return fmt.Errorf("error loading blueprint config: %w", err)
		}
	}

	// Initialize generators
	generatorsList, _ := injector.ResolveAll((*generators.Generator)(nil))
	for _, generator := range generatorsList {
		if err := generator.(generators.Generator).Initialize(); err != nil {
			return fmt.Errorf("error initializing generator: %w", err)
		}
	}

	// Initialize artifact builder
	if artifactBuilder := injector.Resolve("artifactBuilder"); artifactBuilder != nil {
		if err := artifactBuilder.(artifact.Artifact).Initialize(injector); err != nil {
			return fmt.Errorf("error initializing artifact builder: %w", err)
		}
	}

	// Initialize bundlers
	bundlersList, _ := injector.ResolveAll((*artifact.Bundler)(nil))
	for _, bundler := range bundlersList {
		if err := bundler.(artifact.Bundler).Initialize(injector); err != nil {
			return fmt.Errorf("error initializing bundler: %w", err)
		}
	}

	// Initialize stack
	if stackObj := injector.Resolve("stack"); stackObj != nil {
		if err := stackObj.(stack.Stack).Initialize(); err != nil {
			return fmt.Errorf("error initializing stack: %w", err)
		}
	}

	p.injector = injector
	return nil
}

// Execute runs the init pipeline with the provided context.
// It performs the complete initialization workflow including environment setup,
// configuration management, template processing, and component initialization.
func (p *InitPipeline) Execute(ctx context.Context) error {

	if err := p.setupTrustedEnvironment(); err != nil {
		return err
	}

	contextName, err := p.determineAndSetContext(ctx)
	if err != nil {
		return err
	}

	// Set default configuration immediately after setting context (matching main branch order)
	if err := p.setDefaultConfiguration(ctx); err != nil {
		return err
	}

	if err := p.configureSettings(ctx); err != nil {
		return err
	}

	if err := p.saveConfigAndProcessTemplates(contextName, ctx); err != nil {
		return err
	}

	fmt.Println("✅ Windsor initialized successfully!")
	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// setupTrustedEnvironment sets up the trusted directory and reset token.
// It adds the current directory to the trusted file and writes a reset token
// to enable secure initialization operations.
func (p *InitPipeline) setupTrustedEnvironment() error {
	if err := p.shell.AddCurrentDirToTrustedFile(); err != nil {
		return fmt.Errorf("Error adding current directory to trusted file: %w", err)
	}

	if _, err := p.shell.WriteResetToken(); err != nil {
		return fmt.Errorf("Error writing reset token: %w", err)
	}

	return nil
}

// determineAndSetContext determines the context name from command arguments or uses the current
// context if no arguments are provided. It defaults to "local" if no context is available and
// sets the determined context in the configuration handler.
func (p *InitPipeline) determineAndSetContext(ctx context.Context) (string, error) {
	var contextName string
	args := ctx.Value("args")
	if args != nil {
		if argSlice, ok := args.([]string); ok && len(argSlice) > 0 {
			contextName = argSlice[0]
		}
	}
	if contextName == "" {
		if currentContext := p.configHandler.GetContext(); currentContext != "" {
			contextName = currentContext
		} else {
			contextName = "local"
		}
	}

	if err := p.configHandler.SetContext(contextName); err != nil {
		return "", fmt.Errorf("Error setting context value: %w", err)
	}

	return contextName, nil
}

// setDefaultConfiguration determines the VM driver and sets the appropriate default configuration
// immediately after setting the context, matching the main branch order of operations.
func (p *InitPipeline) setDefaultConfiguration(ctx context.Context) error {
	// Get the current context name to determine appropriate defaults
	contextName := p.configHandler.GetContext()

	// Only set vm.driver for local contexts
	isLocalContext := contextName == "local" || (len(contextName) > 6 && contextName[:6] == "local-")

	if isLocalContext {
		// Determine the default VM driver for local configuration selection
		vmDriverConfig := p.configHandler.GetString("vm.driver")
		if vmDriverConfig == "" {
			// Default to docker-desktop for darwin/windows, docker for others
			switch runtime.GOOS {
			case "darwin", "windows":
				vmDriverConfig = "docker-desktop"
			default:
				vmDriverConfig = "docker"
			}
		}
		if err := p.configHandler.SetDefault(config.DefaultConfig_Localhost); err != nil {
			return fmt.Errorf("Error setting default localhost configuration: %w", err)
		}
		if p.configHandler.GetString("vm.driver") == "" {
			if err := p.configHandler.SetContextValue("vm.driver", vmDriverConfig); err != nil {
				return fmt.Errorf("Error setting default VM driver: %w", err)
			}
		}
	} else {
		// For non-local contexts, use basic configuration and do NOT set vm.driver at all
		// Check if talos flag is set to determine which default to use
		talos, _ := ctx.Value("talos").(bool)
		changedFlags := make(map[string]bool)
		if cf := ctx.Value("changedFlags"); cf != nil {
			if changedMap, ok := cf.(map[string]bool); ok {
				changedFlags = changedMap
			}
		}

		if changedFlags["talos"] && talos {
			// Use full configuration when --talos flag is set
			if err := p.configHandler.SetDefault(config.DefaultConfig_Full); err != nil {
				return fmt.Errorf("Error setting default full configuration: %w", err)
			}
		} else {
			// Use basic configuration for non-local contexts without --talos
			if err := p.configHandler.SetDefault(config.DefaultConfig); err != nil {
				return fmt.Errorf("Error setting default configuration: %w", err)
			}
		}
	}

	return nil
}

// configureSettings handles all configuration setup including defaults, flags, and platform-specific settings
func (p *InitPipeline) configureSettings(ctx context.Context) error {
	// Extract flag values from context
	aws, _ := ctx.Value("aws").(bool)
	azure, _ := ctx.Value("azure").(bool)
	talos, _ := ctx.Value("talos").(bool)
	colima, _ := ctx.Value("colima").(bool)
	dockerCompose, _ := ctx.Value("docker-compose").(bool)
	blueprint, _ := ctx.Value("blueprint").(string)

	changedFlags := make(map[string]bool)
	if cf := ctx.Value("changedFlags"); cf != nil {
		if changedMap, ok := cf.(map[string]bool); ok {
			changedFlags = changedMap
		}
	}

	// Determine platform and VM driver configuration
	platform := "local"
	if changedFlags["aws"] && aws {
		platform = "aws"
	} else if changedFlags["azure"] && azure {
		platform = "azure"
	} else if changedFlags["talos"] && talos {
		platform = "talos"
	}

	// Set the platform in configuration
	if err := p.configHandler.SetContextValue("cluster.platform", platform); err != nil {
		return fmt.Errorf("Error setting platform: %w", err)
	}

	// Handle blueprint flag if provided
	if changedFlags["blueprint"] && blueprint != "" {
		if err := p.configHandler.SetContextValue("blueprint", blueprint); err != nil {
			return fmt.Errorf("Error setting blueprint: %w", err)
		}
	}

	// VM driver configuration (only if explicitly provided via flags)
	vmDriver := ctx.Value("vmDriver")
	if vmDriver != nil {
		if vmDriverStr, ok := vmDriver.(string); ok && vmDriverStr != "" {
			if err := p.configHandler.Set("vm.driver", vmDriverStr); err != nil {
				return fmt.Errorf("Error setting VM driver: %w", err)
			}
		}
	}

	// Handle colima flag conversion to vm.driver (only for local contexts)
	if changedFlags["colima"] && colima {
		contextName := p.configHandler.GetContext()
		isLocalContext := contextName == "local" || (len(contextName) > 6 && contextName[:6] == "local-")

		if isLocalContext {
			if err := p.configHandler.SetContextValue("vm.driver", "colima"); err != nil {
				return fmt.Errorf("Error setting VM driver: %w", err)
			}
		}
	}

	// Docker configuration - only set if flag is explicitly provided
	if changedFlags["docker-compose"] {
		if err := p.configHandler.SetContextValue("docker.enabled", dockerCompose); err != nil {
			return fmt.Errorf("Error setting docker enabled: %w", err)
		}
	}

	return nil
}

// saveConfigAndProcessTemplates saves the configuration, processes blueprint templates, and writes all component configurations.
// It generates a unique context ID, persists the configuration to windsor.yaml, processes blueprint templates for the context,
// and writes configurations for tools, services, virtual machines, container runtimes, and generators. The reset parameter
// controls whether existing files should be overwritten during the save and write operations.
func (p *InitPipeline) saveConfigAndProcessTemplates(contextName string, ctx context.Context) error {
	reset, _ := ctx.Value("reset").(bool)

	if err := p.configHandler.GenerateContextID(); err != nil {
		return fmt.Errorf("failed to generate context ID: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Error getting current working directory: %w", err)
	}
	cliConfigPath := filepath.Join(cwd, "windsor.yaml")

	if err := p.configHandler.SaveConfig(cliConfigPath, reset); err != nil {
		return fmt.Errorf("Error saving config file: %w", err)
	}

	if p.blueprintHandler != nil {
		if err := p.blueprintHandler.ProcessContextTemplates(contextName); err != nil {
			return fmt.Errorf("error processing blueprint templates: %w", err)
		}
	}

	injector := p.injector

	if toolsManager := injector.Resolve("toolsManager"); toolsManager != nil {
		if tm, ok := toolsManager.(tools.ToolsManager); ok {
			if err := tm.WriteManifest(); err != nil {
				return fmt.Errorf("error writing tools manifest: %w", err)
			}
		}
	}

	serviceInstances, _ := injector.ResolveAll((*services.Service)(nil))
	for _, service := range serviceInstances {
		if s, ok := service.(services.Service); ok && s != nil {
			if err := s.WriteConfig(); err != nil {
				return fmt.Errorf("error writing service config: %w", err)
			}
		}
	}

	if vmDriver := p.configHandler.GetString("vm.driver"); vmDriver != "" {
		if virtualMachine := injector.Resolve("virtualMachine"); virtualMachine != nil {
			if vm, ok := virtualMachine.(virt.VirtualMachine); ok {
				if err := vm.WriteConfig(); err != nil {
					return fmt.Errorf("error writing virtual machine config: %w", err)
				}
			}
		}
	}

	if dockerEnabled := p.configHandler.GetBool("docker.enabled"); dockerEnabled {
		if containerRuntime := injector.Resolve("containerRuntime"); containerRuntime != nil {
			if cr, ok := containerRuntime.(virt.ContainerRuntime); ok {
				if err := cr.WriteConfig(); err != nil {
					return fmt.Errorf("error writing container runtime config: %w", err)
				}
			}
		}
	}

	generatorInstances, _ := injector.ResolveAll((*generators.Generator)(nil))
	for _, generator := range generatorInstances {
		if g, ok := generator.(generators.Generator); ok && g != nil {
			if err := g.Write(reset); err != nil {
				return fmt.Errorf("error writing generator config: %w", err)
			}
		}
	}

	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ Pipeline = (*EnvPipeline)(nil)
