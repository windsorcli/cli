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

	if err := p.setDefaultConfiguration(ctx, contextName); err != nil {
		return err
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

// =============================================================================
// Private Methods
// =============================================================================

// determineContextName selects the context name from ctx, config, or defaults to "local" if unset or "local".
func (p *InitPipeline) determineContextName(ctx context.Context) string {
	if contextName := ctx.Value("contextName"); contextName != nil {
		if name, ok := contextName.(string); ok {
			return name
		}
	}
	currentContext := p.configHandler.GetContext()
	if currentContext != "" && currentContext != "local" {
		return currentContext
	}
	return "local"
}

// setDefaultConfiguration sets the appropriate default configuration based on provider and VM driver detection.
// For local provider, it applies VM driver-specific defaults (DefaultConfig_Localhost for docker-desktop,
// DefaultConfig_Full for others). For cloud providers, it applies minimal DefaultConfig.
// It also auto-detects VM drivers on macOS/Windows and sets the vm.driver configuration value.
func (p *InitPipeline) setDefaultConfiguration(_ context.Context, contextName string) error {
	existingProvider := p.configHandler.GetString("provider")
	shouldApplyDefaults := existingProvider == ""

	if shouldApplyDefaults {
		vmDriver := p.configHandler.GetString("vm.driver")
		isLocalContext := existingProvider == "local" || contextName == "local" || strings.HasPrefix(contextName, "local-")

		if isLocalContext && vmDriver == "" {
			switch runtime.GOOS {
			case "darwin", "windows":
				vmDriver = "docker-desktop"
			default:
				vmDriver = "docker"
			}
		}

		// Apply VM driver-specific defaults regardless of context if VM driver is set
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
	}

	existingProvider = p.configHandler.GetString("provider")
	if existingProvider == "" {
		switch contextName {
		case "aws", "azure", "local", "metal":
			if err := p.configHandler.SetContextValue("provider", contextName); err != nil {
				return fmt.Errorf("Error setting provider from context name: %w", err)
			}
		}
	}

	return nil
}

// processPlatformConfiguration applies provider-specific configuration settings based on the "provider" value in the configuration handler.
// For "aws", it enables AWS and sets the cluster driver to "eks".
// For "azure", it enables Azure and sets the cluster driver to "aks".
// For "metal" and "local", it sets the cluster driver to "talos".
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
	case "metal":
		if err := p.configHandler.SetContextValue("cluster.driver", "talos"); err != nil {
			return fmt.Errorf("Error setting cluster.driver: %w", err)
		}
	case "local":
		if err := p.configHandler.SetContextValue("cluster.driver", "talos"); err != nil {
			return fmt.Errorf("Error setting cluster.driver: %w", err)
		}
		if err := p.configHandler.SetContextValue("terraform.enabled", true); err != nil {
			return fmt.Errorf("Error setting terraform.enabled: %w", err)
		}
	}

	return nil
}

// prepareTemplateData selects and returns template input sources for template rendering in InitPipeline.
// Selection order:
// 1. If the context "blueprint" value is an OCI reference and artifactBuilder is set, extract template data from the OCI artifact.
// 2. If blueprintHandler is set, attempt to load template data from the local _template directory.
// 3. If local template data is unavailable, generate default template data for the current context using blueprintHandler.
// 4. If none of the above yield data, return an empty map.
// Returns a map of template file names to contents, or an error if extraction fails at any step.
func (p *InitPipeline) prepareTemplateData(ctx context.Context) (map[string][]byte, error) {
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
		localTemplateData, err := p.blueprintHandler.GetLocalTemplateData()
		if err != nil {
			return nil, fmt.Errorf("failed to get local template data: %w", err)
		}
		if len(localTemplateData) > 0 {
			return localTemplateData, nil
		}
	}

	if p.artifactBuilder != nil {
		ociInfo, err := artifact.ParseOCIReference(constants.DEFAULT_OCI_BLUEPRINT_URL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse default blueprint reference: %w", err)
		}
		templateData, err := p.artifactBuilder.GetTemplateData(ociInfo.URL)
		if err != nil {
			return nil, fmt.Errorf("failed to get template data from default blueprint: %w", err)
		}
		p.fallbackBlueprintURL = constants.DEFAULT_OCI_BLUEPRINT_URL
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

// processTemplateData processes the template data to load blueprint data and render it.
func (p *InitPipeline) processTemplateData(templateData map[string][]byte) (map[string]any, error) {
	var renderedData map[string]any
	if p.templateRenderer != nil && len(templateData) > 0 {
		renderedData = make(map[string]any)
		if err := p.templateRenderer.Process(templateData, renderedData); err != nil {
			return nil, fmt.Errorf("failed to process template data: %w", err)
		}
	}
	return renderedData, nil
}

// loadBlueprintFromTemplate loads blueprint data from rendered template data. If the "blueprint" key exists
// in renderedData and is a map, attempts to parse OCI artifact info from the context's "blueprint" value.
// Delegates loading to blueprintHandler.LoadData with the parsed blueprint map and optional OCI info.
func (p *InitPipeline) loadBlueprintFromTemplate(ctx context.Context, renderedData map[string]any) error {
	if blueprintData, exists := renderedData["blueprint"]; exists {
		if blueprintMap, ok := blueprintData.(map[string]any); ok {
			var ociInfo *artifact.OCIArtifactInfo
			if blueprintCtx := ctx.Value("blueprint"); blueprintCtx != nil {
				if blueprintValue, ok := blueprintCtx.(string); ok {
					var err error
					ociInfo, err = artifact.ParseOCIReference(blueprintValue)
					if err != nil {
						return err
					}
				}
			}

			if err := p.blueprintHandler.LoadData(blueprintMap, ociInfo); err != nil {
				return fmt.Errorf("failed to load blueprint data: %w", err)
			}
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

// handleBlueprintLoading manages blueprint loading logic based on reset flag and existing blueprint files.
// If reset is true, loads blueprint from template data if available.
// If reset is false, prefers existing blueprint.yaml over template data.
// Falls back to loading from existing config if no template blueprint data exists.
func (p *InitPipeline) handleBlueprintLoading(ctx context.Context, renderedData map[string]any, reset bool) error {
	shouldLoadFromTemplate := false

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
		// If we have a fallback blueprint URL, set it in the context
		if p.fallbackBlueprintURL != "" {
			ctx = context.WithValue(ctx, "blueprint", p.fallbackBlueprintURL)
		}
		if err := p.loadBlueprintFromTemplate(ctx, renderedData); err != nil {
			return err
		}
	} else {
		if err := p.blueprintHandler.LoadConfig(); err != nil {
			return fmt.Errorf("Error loading blueprint config: %w", err)
		}
	}

	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ Pipeline = (*InitPipeline)(nil)
