package pipelines

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/generators"
	"github.com/windsorcli/cli/pkg/network"
	"github.com/windsorcli/cli/pkg/services"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/stack"
	"github.com/windsorcli/cli/pkg/template"
	"github.com/windsorcli/cli/pkg/terraform"
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

// InitPipeline handles the initialization of a Windsor project
type InitPipeline struct {
	BasePipeline
	templateRenderer     template.Template
	blueprintHandler     blueprint.BlueprintHandler
	toolsManager         tools.ToolsManager
	stack                stack.Stack
	generators           []generators.Generator
	bundlers             []artifact.Bundler
	artifactBuilder      artifact.Artifact
	services             []services.Service
	virtualMachine       virt.VirtualMachine
	containerRuntime     virt.ContainerRuntime
	networkManager       network.NetworkManager
	terraformResolvers   []terraform.ModuleResolver
	fallbackBlueprintURL string
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
// services, virtual machine components, terraform resolvers, and template renderer.
// It applies default configuration early so that service creation can access correct configuration values.
func (p *InitPipeline) Initialize(injector di.Injector, ctx context.Context) error {
	if err := p.BasePipeline.Initialize(injector, ctx); err != nil {
		return err
	}

	// Configuration phase

	contextName := p.determineContextName(ctx)
	if err := p.configHandler.SetContext(contextName); err != nil {
		return fmt.Errorf("Error setting context value: %w", err)
	}

	if !p.configHandler.IsContextConfigLoaded() {
		if err := p.setDefaultConfiguration(ctx, contextName); err != nil {
			return err
		}
	}

	if err := p.processPlatformConfiguration(ctx); err != nil {
		return err
	}

	if err := p.configHandler.GenerateContextID(); err != nil {
		return fmt.Errorf("failed to generate context ID: %w", err)
	}

	if err := p.configHandler.SaveConfig(); err != nil {
		return fmt.Errorf("Error saving config file: %w", err)
	}

	// Component Collection Phase

	kubernetesClient := p.withKubernetesClient()
	if kubernetesClient != nil {
		p.injector.Register("kubernetesClient", kubernetesClient)
	}

	kubernetesManager := p.withKubernetesManager()

	p.blueprintHandler = p.withBlueprintHandler()
	p.toolsManager = p.withToolsManager()

	if p.injector.Resolve("terraformEnv") == nil {
		terraformEnv := env.NewTerraformEnvPrinter(p.injector)
		p.injector.Register("terraformEnv", terraformEnv)
	}

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

	terraformResolvers, err := p.withTerraformResolvers()
	if err != nil {
		return fmt.Errorf("failed to create terraform resolvers: %w", err)
	}
	p.terraformResolvers = terraformResolvers

	p.templateRenderer = p.withTemplateRenderer()
	p.networkManager = p.withNetworking()
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

	for _, terraformResolver := range p.terraformResolvers {
		if err := terraformResolver.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize terraform resolver: %w", err)
		}
	}

	if p.templateRenderer != nil {
		if err := p.templateRenderer.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize template renderer: %w", err)
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

	if secureShell := p.injector.Resolve("secureShell"); secureShell != nil {
		if secureShellInterface, ok := secureShell.(shell.Shell); ok {
			if err := secureShellInterface.Initialize(); err != nil {
				return fmt.Errorf("failed to initialize secure shell: %w", err)
			}
		}
	}

	return nil
}

// Execute performs initialization by writing reset tokens, processing templates, handling blueprints separately,
// writing blueprint files, resolving Terraform modules, and generating final output files.
func (p *InitPipeline) Execute(ctx context.Context) error {

	// Phase 1: Setup
	if _, err := p.shell.WriteResetToken(); err != nil {
		return fmt.Errorf("Error writing reset token: %w", err)
	}

	// Phase 2: Template processing
	templateData, err := p.prepareTemplateData(ctx)
	if err != nil {
		return fmt.Errorf("failed to prepare template data: %w", err)
	}
	renderedData, err := p.processTemplateData(templateData)
	if err != nil {
		return err
	}

	// Phase 3: Blueprint handling
	reset := false
	if resetValue := ctx.Value("reset"); resetValue != nil {
		reset = resetValue.(bool)
	}
	if err := p.handleBlueprintLoading(ctx, renderedData, reset); err != nil {
		return err
	}
	if err := p.blueprintHandler.Write(reset); err != nil {
		return fmt.Errorf("failed to write blueprint file: %w", err)
	}

	// Phase 4: Terraform module resolution
	for _, resolver := range p.terraformResolvers {
		if err := resolver.ProcessModules(); err != nil {
			return fmt.Errorf("failed to process terraform modules: %w", err)
		}
	}

	// Phase 5: Final file generation
	if len(renderedData) > 0 {
		for _, generator := range p.generators {
			if err := generator.Generate(renderedData, reset); err != nil {
				return fmt.Errorf("failed to generate from template data: %w", err)
			}
		}
	}

	if err := p.writeConfigurationFiles(); err != nil {
		return err
	}

	// Save the configuration to windsor.yaml files
	if err := p.configHandler.SaveConfig(); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Fprintln(os.Stderr, "Initialization successful")

	return nil
}

// prepareTemplateData sets the fallbackBlueprintURL if the default blueprint URL is used.
// It calls the base pipeline's prepareTemplateData, checks for explicit blueprint context and local templates,
// and assigns the fallback URL for blueprint processing if necessary.
// Returns the prepared template data or an error.
func (p *InitPipeline) prepareTemplateData(ctx context.Context) (map[string][]byte, error) {
	templateData, err := p.BasePipeline.prepareTemplateData(ctx)
	if err != nil {
		return nil, err
	}
	if ctx.Value("blueprint") == nil && p.artifactBuilder != nil {
		blueprintHandler := p.withBlueprintHandler()
		hasLocalTemplates := false
		if blueprintHandler != nil {
			if localTemplateData, err := blueprintHandler.GetLocalTemplateData(); err == nil && len(localTemplateData) > 0 {
				hasLocalTemplates = true
			}
		}
		if !hasLocalTemplates {
			p.fallbackBlueprintURL = constants.GetEffectiveBlueprintURL()
		}
	}
	return templateData, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// setDefaultConfiguration sets default config values based on provider and VM driver detection.
// For local providers, uses config.DefaultConfig_Localhost if VM driver is "docker-desktop",
// else uses config.DefaultConfig_Full. For non-local, uses config.DefaultConfig.
// On darwin/windows, sets "vm.driver" to "docker-desktop"; otherwise to "docker".
// If provider is unset and context is local, sets provider to "local".
// Returns error if any config operation fails.
func (p *InitPipeline) setDefaultConfiguration(_ context.Context, contextName string) error {
	existingProvider := p.configHandler.GetString("provider")

	var isLocalContext bool
	if existingProvider != "" {
		// Treat "generic" provider with "local" context name as local context
		isLocalContext = existingProvider == "generic" && (contextName == "local" || strings.HasPrefix(contextName, "local-"))
	} else {
		isLocalContext = contextName == "local" || strings.HasPrefix(contextName, "local-")
	}

	vmDriver := p.configHandler.GetString("vm.driver")

	if isLocalContext && vmDriver == "" {
		switch runtime.GOOS {
		case "darwin", "windows":
			vmDriver = "docker-desktop"
		default:
			vmDriver = "docker"
		}
	}

	if vmDriver == "docker-desktop" {
		if err := p.configHandler.SetDefault(config.DefaultConfig_Localhost); err != nil {
			return fmt.Errorf("Error setting default config: %w", err)
		}
	} else if isLocalContext {
		if err := p.configHandler.SetDefault(config.DefaultConfig_Full); err != nil {
			return fmt.Errorf("Error setting default config: %w", err)
		}
	} else {
		if err := p.configHandler.SetDefault(config.DefaultConfig); err != nil {
			return fmt.Errorf("Error setting default config: %w", err)
		}
	}

	if isLocalContext && p.configHandler.GetString("vm.driver") == "" && vmDriver != "" {
		if err := p.configHandler.SetContextValue("vm.driver", vmDriver); err != nil {
			return fmt.Errorf("Error setting vm.driver: %w", err)
		}
	}

	if existingProvider == "" {
		if contextName == "local" || strings.HasPrefix(contextName, "local-") {
			if err := p.configHandler.SetContextValue("provider", "generic"); err != nil {
				return fmt.Errorf("Error setting provider from context name: %w", err)
			}
		}
	}

	return nil
}

// processPlatformConfiguration applies provider-specific configuration settings based on the "provider" value in the configuration handler.
// Since defaults are already applied in setDefaultConfiguration, this function only sets provider-specific overrides.
// For "aws", it enables AWS and sets the cluster driver to "eks".
// For "azure", it enables Azure and sets the cluster driver to "aks".
// For "generic", it sets the cluster driver to "talos".
// Returns an error if any configuration operation fails.
func (p *InitPipeline) processPlatformConfiguration(_ context.Context) error {
	provider := p.configHandler.GetString("provider")
	if provider == "" {
		return nil
	}

	switch provider {
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
	case "generic":
		if err := p.configHandler.SetContextValue("cluster.driver", "talos"); err != nil {
			return fmt.Errorf("Error setting cluster.driver: %w", err)
		}
	}

	return nil
}

// writeConfigurationFiles writes configuration files for all managed components in the InitPipeline.
// It sequentially invokes WriteManifest or WriteConfig on the tools manager, each registered service,
// the virtual machine, and the container runtime if present. Returns an error if any write operation fails.
func (p *InitPipeline) writeConfigurationFiles() error {
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

	return nil
}

// handleBlueprintLoading loads blueprint data for the InitPipeline based on the reset flag and blueprint file presence.
// If reset is true, loads blueprint from template data if available. If reset is false, prefers an existing blueprint.yaml file over template data.
// If no template blueprint data exists, loads from existing config. Returns an error if loading fails.
func (p *InitPipeline) handleBlueprintLoading(ctx context.Context, renderedData map[string]any, reset bool) error {
	shouldLoadFromTemplate := false
	usingLocalTemplates := p.hasLocalTemplates()

	if reset {
		shouldLoadFromTemplate = true
	} else {
		configRoot, err := p.configHandler.GetConfigRoot()
		if err != nil {
			return fmt.Errorf("error getting config root: %w", err)
		}
		blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
		if _, err := p.shims.Stat(blueprintPath); err != nil {
			shouldLoadFromTemplate = true
		}
	}

	if shouldLoadFromTemplate && len(renderedData) > 0 && renderedData["blueprint"] != nil {
		if p.fallbackBlueprintURL != "" {
			ctx = context.WithValue(ctx, "blueprint", p.fallbackBlueprintURL)
		}
		if err := p.loadBlueprintFromTemplate(ctx, renderedData); err != nil {
			return err
		}
		if usingLocalTemplates {
			if blueprintData, exists := renderedData["blueprint"]; exists {
				if blueprintMap, ok := blueprintData.(map[string]any); ok {
					if sources, ok := blueprintMap["sources"].([]any); ok && len(sources) > 0 {
						if err := p.loadExplicitSources(sources); err != nil {
							return fmt.Errorf("failed to load explicit sources: %w", err)
						}
					}
				}
			}
		}
	} else if !usingLocalTemplates {
		if err := p.blueprintHandler.LoadConfig(); err != nil {
			return fmt.Errorf("error loading blueprint config: %w", err)
		}
		sources := p.blueprintHandler.GetSources()
		if len(sources) > 0 && p.artifactBuilder != nil {
			var ociURLs []string
			for _, source := range sources {
				if strings.HasPrefix(source.Url, "oci://") {
					ociURLs = append(ociURLs, source.Url)
				}
			}
			if len(ociURLs) > 0 {
				_, err := p.artifactBuilder.Pull(ociURLs)
				if err != nil {
					return fmt.Errorf("failed to load OCI sources: %w", err)
				}
			}
		}
	}

	return nil
}

// hasLocalTemplates checks if the contexts/_template directory exists in the project.
func (p *InitPipeline) hasLocalTemplates() bool {
	if p.shell == nil || p.shims == nil {
		return false
	}

	projectRoot, err := p.shell.GetProjectRoot()
	if err != nil {
		return false
	}

	templateDir := filepath.Join(projectRoot, "contexts", "_template")
	_, err = p.shims.Stat(templateDir)
	return err == nil
}

// loadExplicitSources loads OCI sources that are explicitly defined in blueprint templates.
func (p *InitPipeline) loadExplicitSources(sources []any) error {
	if p.artifactBuilder == nil {
		return nil
	}

	var ociURLs []string
	for _, source := range sources {
		if sourceMap, ok := source.(map[string]any); ok {
			if url, ok := sourceMap["url"].(string); ok && strings.HasPrefix(url, "oci://") {
				ociURLs = append(ociURLs, url)
			}
		}
	}

	if len(ociURLs) > 0 {
		_, err := p.artifactBuilder.Pull(ociURLs)
		if err != nil {
			return fmt.Errorf("failed to load explicit OCI sources: %w", err)
		}
	}

	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ Pipeline = (*InitPipeline)(nil)
