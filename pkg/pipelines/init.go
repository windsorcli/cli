package pipelines

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"os"

	bundler "github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/generators"
	"github.com/windsorcli/cli/pkg/network"
	"github.com/windsorcli/cli/pkg/services"
	"github.com/windsorcli/cli/pkg/stack"
	"github.com/windsorcli/cli/pkg/tools"
	"github.com/windsorcli/cli/pkg/virt"
)

// The InitPipeline is a specialized component that manages application initialization functionality.
// It provides init-specific command execution including configuration setup, context management,
// flag processing, and component initialization for the Windsor CLI init command.
// The InitPipeline handles the complete initialization workflow including default configuration
// setting, blueprint processing, and infrastructure component setup.

// =============================================================================
// Types
// =============================================================================

// InitPipeline provides application initialization functionality
type InitPipeline struct {
	BasePipeline
	blueprintHandler blueprint.BlueprintHandler
	toolsManager     tools.ToolsManager
	stack            stack.Stack
	generators       []generators.Generator
	bundlers         []bundler.Bundler
	artifactBuilder  bundler.Artifact
	services         []services.Service
	virtualMachine   virt.VirtualMachine
	containerRuntime virt.ContainerRuntime
	networkManager   network.NetworkManager
}

// =============================================================================
// Constructor
// =============================================================================

// NewInitPipeline creates a new InitPipeline instance
func NewInitPipeline() *InitPipeline {
	return &InitPipeline{
		BasePipeline: *NewBasePipeline(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize sets up the init pipeline components including dependency injection container,
// configuration handler, blueprint handler, tools manager, stack, generators, bundlers,
// services, and virtual machine components. It applies default configuration early so that
// service creation can access correct configuration values.
func (p *InitPipeline) Initialize(injector di.Injector, ctx context.Context) error {
	if err := p.BasePipeline.Initialize(injector, ctx); err != nil {
		return err
	}

	// Configuration phase

	contextName := p.determineContextName(ctx)
	if err := p.configHandler.SetContext(contextName); err != nil {
		return fmt.Errorf("Error setting context value: %w", err)
	}

	if err := p.setDefaultConfiguration(ctx, contextName); err != nil {
		return err
	}

	if err := p.configureVMDriver(ctx, contextName); err != nil {
		return err
	}

	if err := p.processPlatformConfiguration(ctx); err != nil {
		return err
	}

	// Component Collection Phase

	kubernetesClient := p.withKubernetesClient()
	if kubernetesClient != nil {
		p.injector.Register("kubernetesClient", kubernetesClient)
	}

	kubernetesManager := p.withKubernetesManager()

	p.blueprintHandler = p.withBlueprintHandler()
	p.toolsManager = p.withToolsManager()
	p.stack = p.withStack()
	p.artifactBuilder = p.withArtifactBuilder()

	generators, err := p.withGenerators()
	if err != nil {
		return fmt.Errorf("failed to create generators: %w", err)
	}
	p.generators = generators

	bundlers, err := p.withBundlers()
	if err != nil {
		return fmt.Errorf("failed to create bundlers: %w", err)
	}
	p.bundlers = bundlers

	services, err := p.withServices()
	if err != nil {
		return fmt.Errorf("failed to create services: %w", err)
	}
	p.services = services

	p.networkManager = p.withNetworkManager()

	p.virtualMachine = p.withVirtualMachine()
	p.containerRuntime = p.withContainerRuntime()

	// Component Initialization Phase

	if kubernetesManager != nil {
		if err := kubernetesManager.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize kubernetes manager: %w", err)
		}
	}

	if p.blueprintHandler != nil {
		if err := p.blueprintHandler.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize blueprint handler: %w", err)
		}
	}

	if p.toolsManager != nil {
		if err := p.toolsManager.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize tools manager: %w", err)
		}
	}

	if p.stack != nil {
		if err := p.stack.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize stack: %w", err)
		}
	}

	if p.artifactBuilder != nil {
		if err := p.artifactBuilder.Initialize(p.injector); err != nil {
			return fmt.Errorf("failed to initialize artifact builder: %w", err)
		}
	}

	for _, generator := range p.generators {
		if err := generator.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize generator: %w", err)
		}
	}

	for _, bundler := range p.bundlers {
		if err := bundler.Initialize(p.injector); err != nil {
			return fmt.Errorf("failed to initialize bundler: %w", err)
		}
	}

	for _, service := range p.services {
		if err := service.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize service: %w", err)
		}
	}

	if p.networkManager != nil {
		if err := p.networkManager.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize network manager: %w", err)
		}
	}

	if p.virtualMachine != nil {
		if err := p.virtualMachine.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize virtual machine: %w", err)
		}
	}

	if p.containerRuntime != nil {
		if err := p.containerRuntime.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize container runtime: %w", err)
		}
	}

	return nil
}

// Execute runs the init command pipeline including file system operations, configuration saving,
// and template processing. The component initialization has already been completed in Initialize().
func (p *InitPipeline) Execute(ctx context.Context) error {
	if err := p.shell.AddCurrentDirToTrustedFile(); err != nil {
		return fmt.Errorf("Error adding current directory to trusted file: %w", err)
	}

	if _, err := p.shell.WriteResetToken(); err != nil {
		return fmt.Errorf("Error writing reset token: %w", err)
	}

	if err := p.configHandler.GenerateContextID(); err != nil {
		return fmt.Errorf("failed to generate context ID: %w", err)
	}

	if err := p.saveConfiguration(ctx); err != nil {
		return err
	}

	contextName := p.determineContextName(ctx)
	if err := p.processContextTemplates(ctx, contextName); err != nil {
		return err
	}

	if err := p.blueprintHandler.LoadConfig(false); err != nil {
		return fmt.Errorf("Error reloading blueprint config after IP assignment: %w", err)
	}

	if err := p.writeConfigurationFiles(ctx); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "Initialization successful")
	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// determineContextName determines the context name from arguments or defaults to "local".
func (p *InitPipeline) determineContextName(ctx context.Context) string {
	if contextName := ctx.Value("contextName"); contextName != nil {
		if name, ok := contextName.(string); ok {
			return name
		}
	}
	return "local"
}

// configureVMDriver applies VM driver configuration from command flags to override defaults.
func (p *InitPipeline) configureVMDriver(_ context.Context, contextName string) error {
	vmDriver := p.configHandler.GetString("vm.driver")

	if vmDriver != "" {
		if err := p.configHandler.SetContextValue("vm.driver", vmDriver); err != nil {
			return fmt.Errorf("error setting vm.driver: %w", err)
		}
	}

	return nil
}

// setDefaultConfiguration sets the appropriate default configuration based on context and VM driver detection.
func (p *InitPipeline) setDefaultConfiguration(_ context.Context, contextName string) error {
	vmDriver := p.configHandler.GetString("vm.driver")
	if vmDriver == "" && (contextName == "local" || strings.HasPrefix(contextName, "local-")) {
		switch runtime.GOOS {
		case "darwin", "windows":
			vmDriver = "docker-desktop"
		default:
			vmDriver = "docker"
		}
	}

	switch vmDriver {
	case "docker-desktop":
		if err := p.configHandler.SetDefault(config.DefaultConfig_Localhost); err != nil {
			return fmt.Errorf("Error setting default config: %w", err)
		}
	case "colima", "docker":
		if err := p.configHandler.SetDefault(config.DefaultConfig_Full); err != nil {
			return fmt.Errorf("Error setting default config: %w", err)
		}
	default:
		if err := p.configHandler.SetDefault(config.DefaultConfig); err != nil {
			return fmt.Errorf("Error setting default config: %w", err)
		}
	}

	return nil
}

// processPlatformConfiguration handles platform-specific configuration settings.
func (p *InitPipeline) processPlatformConfiguration(_ context.Context) error {
	platform := p.configHandler.GetString("platform")
	if platform == "" {
		return nil
	}

	switch platform {
	case "aws":
		if err := p.configHandler.SetContextValue("aws.enabled", true); err != nil {
			return fmt.Errorf("Error setting aws.enabled: %w", err)
		}
		if err := p.configHandler.SetContextValue("cluster.driver", "eks"); err != nil {
			return fmt.Errorf("Error setting cluster.driver: %w", err)
		}
	case "azure":
		if err := p.configHandler.SetContextValue("azure.enabled", true); err != nil {
			return fmt.Errorf("Error setting azure.enabled: %w", err)
		}
		if err := p.configHandler.SetContextValue("cluster.driver", "aks"); err != nil {
			return fmt.Errorf("Error setting cluster.driver: %w", err)
		}
	case "metal":
		if err := p.configHandler.SetContextValue("cluster.driver", "talos"); err != nil {
			return fmt.Errorf("Error setting cluster.driver: %w", err)
		}
	case "local":
		if err := p.configHandler.SetContextValue("cluster.driver", "talos"); err != nil {
			return fmt.Errorf("Error setting cluster.driver: %w", err)
		}
	}

	return nil
}

// saveConfiguration saves the configuration to the appropriate file.
func (p *InitPipeline) saveConfiguration(ctx context.Context) error {
	projectRoot, err := p.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("Error retrieving project root: %w", err)
	}

	yamlPath := filepath.Join(projectRoot, "windsor.yaml")
	ymlPath := filepath.Join(projectRoot, "windsor.yml")

	var cliConfigPath string
	if _, err := p.shims.Stat(yamlPath); err == nil {
		cliConfigPath = yamlPath
	} else if _, err := p.shims.Stat(ymlPath); err == nil {
		cliConfigPath = ymlPath
	} else {
		cliConfigPath = yamlPath
	}

	reset := false
	if resetValue := ctx.Value("reset"); resetValue != nil {
		reset = resetValue.(bool)
	}

	if err := p.configHandler.SaveConfig(cliConfigPath, reset); err != nil {
		return fmt.Errorf("Error saving config file: %w", err)
	}

	return nil
}

// processContextTemplates processes context templates if they exist.
func (p *InitPipeline) processContextTemplates(ctx context.Context, contextName string) error {
	if p.blueprintHandler == nil {
		return nil
	}

	reset := false
	if resetValue := ctx.Value("reset"); resetValue != nil {
		reset = resetValue.(bool)
	}

	if err := p.blueprintHandler.ProcessContextTemplates(contextName, reset); err != nil {
		return fmt.Errorf("Error processing context templates: %w", err)
	}

	if err := p.blueprintHandler.LoadConfig(reset); err != nil {
		return fmt.Errorf("Error reloading blueprint config: %w", err)
	}

	return nil
}

// writeConfigurationFiles writes configuration files for all components.
func (p *InitPipeline) writeConfigurationFiles(ctx context.Context) error {
	reset := false
	if resetValue := ctx.Value("reset"); resetValue != nil {
		reset = resetValue.(bool)
	}

	if p.toolsManager != nil {
		if err := p.toolsManager.WriteManifest(); err != nil {
			return fmt.Errorf("error writing tools manifest: %w", err)
		}
	}

	for _, service := range p.services {
		if err := service.WriteConfig(); err != nil {
			return fmt.Errorf("error writing service config: %w", err)
		}
	}

	if p.virtualMachine != nil {
		if err := p.virtualMachine.WriteConfig(); err != nil {
			return fmt.Errorf("error writing virtual machine config: %w", err)
		}
	}

	if p.containerRuntime != nil {
		if err := p.containerRuntime.WriteConfig(); err != nil {
			return fmt.Errorf("error writing container runtime config: %w", err)
		}
	}

	for _, generator := range p.generators {
		if err := generator.Write(reset); err != nil {
			return fmt.Errorf("error writing generator config: %w", err)
		}
	}

	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ Pipeline = (*InitPipeline)(nil)
