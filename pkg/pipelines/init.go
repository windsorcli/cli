package pipelines

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	bundler "github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/generators"
	"github.com/windsorcli/cli/pkg/network"
	"github.com/windsorcli/cli/pkg/services"
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

// InitPipeline provides application initialization functionality
type InitPipeline struct {
	BasePipeline
	blueprintHandler   blueprint.BlueprintHandler
	toolsManager       tools.ToolsManager
	stack              stack.Stack
	generators         []generators.Generator
	bundlers           []bundler.Bundler
	artifactBuilder    bundler.Artifact
	services           []services.Service
	virtualMachine     virt.VirtualMachine
	containerRuntime   virt.ContainerRuntime
	networkManager     network.NetworkManager
	terraformResolvers []terraform.ModuleResolver
	templateRenderer   template.Template
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

	for _, resolver := range p.terraformResolvers {
		if err := resolver.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize terraform resolver: %w", err)
		}
	}

	if p.templateRenderer != nil {
		if err := p.templateRenderer.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize template renderer: %w", err)
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

// Execute runs the init pipeline, performing trusted file setup, reset token writing, context ID generation,
// configuration saving, network manager initialization, template data preparation, template processing and
// blueprint generation, blueprint loading, terraform module resolution, and final file generation.
// All component initialization is performed in Initialize(). Phases are executed in strict order.
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

	reset := false
	if resetValue := ctx.Value("reset"); resetValue != nil {
		reset = resetValue.(bool)
	}

	// Set default configuration before saving
	contextName := p.determineContextName(ctx)
	if err := p.setDefaultConfiguration(ctx, contextName); err != nil {
		return err
	}

	if err := p.saveConfiguration(reset); err != nil {
		return err
	}

	// Network manager phase
	if p.networkManager != nil {
		if err := p.networkManager.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize network manager: %w", err)
		}
	}

	// Phase 1: template data preparation
	templateData, err := p.prepareTemplateData()
	if err != nil {
		return fmt.Errorf("failed to prepare template data: %w", err)
	}

	// Phase 2: template processing and blueprint generation
	var renderedData map[string]any
	if p.templateRenderer != nil && len(templateData) > 0 {
		renderedData = make(map[string]any)
		if err := p.templateRenderer.Process(templateData, renderedData); err != nil {
			return fmt.Errorf("failed to process template data: %w", err)
		}
		if blueprintData, exists := renderedData["blueprint"]; exists {
			if blueprintGenerator := p.injector.Resolve("blueprintGenerator"); blueprintGenerator != nil {
				if generator, ok := blueprintGenerator.(generators.Generator); ok {
					if err := generator.Generate(map[string]any{"blueprint": blueprintData}, reset); err != nil {
						return fmt.Errorf("failed to generate blueprint from template data: %w", err)
					}
				}
			}
		}
	}

	// Phase 3: blueprint loading
	if err := p.blueprintHandler.LoadConfig(false); err != nil {
		return fmt.Errorf("Error reloading blueprint config after generation: %w", err)
	}

	// Phase 4: terraform module resolution
	for _, resolver := range p.terraformResolvers {
		if err := resolver.ProcessModules(); err != nil {
			return fmt.Errorf("failed to process terraform modules: %w", err)
		}
	}

	// Phase 5: final generation
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

	// Print success message
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

	if vmDriver != "" {
		if err := p.configHandler.SetContextValue("vm.driver", vmDriver); err != nil {
			return fmt.Errorf("Error setting vm.driver: %w", err)
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
func (p *InitPipeline) saveConfiguration(overwrite bool) error {
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

	if err := p.configHandler.SaveConfig(cliConfigPath, overwrite); err != nil {
		return fmt.Errorf("Error saving config file: %w", err)
	}

	return nil
}

// prepareTemplateData handles the priority system for template input sources:
// 1. If --blueprint oci://xyz is passed, extract from OCI artifact
// 2. If contexts/_template folder exists and no --blueprint, load from local directory
// 3. Otherwise, return empty map to use default blueprint generation
func (p *InitPipeline) prepareTemplateData() (map[string][]byte, error) {
	blueprintValue := p.configHandler.GetString("blueprint")

	// Priority 1: OCI blueprint via --blueprint flag
	if blueprintValue != "" && strings.HasPrefix(blueprintValue, "oci://") {
		if p.artifactBuilder == nil {
			return nil, fmt.Errorf("artifact builder not available")
		}

		artifacts, err := p.artifactBuilder.Pull([]string{blueprintValue})
		if err != nil {
			return nil, fmt.Errorf("failed to pull OCI artifact %s: %w", blueprintValue, err)
		}

		if len(artifacts) == 0 {
			return nil, fmt.Errorf("no artifacts downloaded from %s", blueprintValue)
		}

		for _, artifactData := range artifacts {
			templateData := make(map[string][]byte)

			gzipReader, err := gzip.NewReader(bytes.NewReader(artifactData))
			if err != nil {
				return nil, fmt.Errorf("failed to create gzip reader: %w", err)
			}
			defer gzipReader.Close()

			tarReader := tar.NewReader(gzipReader)

			for {
				header, err := tarReader.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					return nil, fmt.Errorf("failed to read tar header: %w", err)
				}

				if header.Typeflag == tar.TypeReg {
					content, err := io.ReadAll(tarReader)
					if err != nil {
						return nil, fmt.Errorf("failed to read file %s: %w", header.Name, err)
					}

					// Store with forward slashes for consistent path handling
					templateData[filepath.ToSlash(header.Name)] = content
				}
			}

			return templateData, nil
		}

		return nil, fmt.Errorf("no valid artifacts found")
	}

	// Priority 2: Local _template directory (only if no --blueprint flag)
	if blueprintValue == "" {
		projectRoot, err := p.shell.GetProjectRoot()
		if err != nil {
			return nil, fmt.Errorf("failed to get project root: %w", err)
		}

		templateDir := filepath.Join(projectRoot, "contexts", "_template")
		if _, err := p.shims.Stat(templateDir); err == nil {
			templateData := make(map[string][]byte)
			if err := p.walkAndCollectTemplates(templateDir, templateData); err != nil {
				return nil, fmt.Errorf("failed to collect local templates: %w", err)
			}
			return templateData, nil
		}
	}

	// Priority 3: Default platform template data via blueprint handler
	if p.blueprintHandler != nil {
		contextName := p.determineContextName(context.Background())
		return p.blueprintHandler.GetDefaultTemplateData(contextName)
	}

	// Priority 4: Fallback (empty map - no template processing occurs)
	return make(map[string][]byte), nil
}

// walkAndCollectTemplates recursively walks through the template directory and collects all files.
// It maintains the relative path structure from the template directory root.
func (p *InitPipeline) walkAndCollectTemplates(templateDir string, templateData map[string][]byte) error {
	projectRoot, err := p.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}
	templateRoot := filepath.Join(projectRoot, "contexts", "_template")

	return p.walkAndCollectTemplatesHelper(templateDir, templateRoot, templateData)
}

// walkAndCollectTemplatesHelper is the helper function that walks directories.
func (p *InitPipeline) walkAndCollectTemplatesHelper(templateDir, templateRoot string, templateData map[string][]byte) error {
	entries, err := p.shims.ReadDir(templateDir)
	if err != nil {
		return fmt.Errorf("failed to read template directory: %w", err)
	}

	for _, entry := range entries {
		entryPath := filepath.Join(templateDir, entry.Name())

		if entry.IsDir() {
			if err := p.walkAndCollectTemplatesHelper(entryPath, templateRoot, templateData); err != nil {
				return err
			}
		} else {
			content, err := p.shims.ReadFile(filepath.Clean(entryPath))
			if err != nil {
				return fmt.Errorf("failed to read template file %s: %w", entryPath, err)
			}

			relPath, err := filepath.Rel(templateRoot, entryPath)
			if err != nil {
				return fmt.Errorf("failed to get relative path: %w", err)
			}

			templateData[filepath.ToSlash(relPath)] = content
		}
	}

	return nil
}

// writeConfigurationFiles writes configuration files for all components.
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

// =============================================================================
// Interface Compliance
// =============================================================================

var _ Pipeline = (*InitPipeline)(nil)
