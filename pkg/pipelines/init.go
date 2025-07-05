package pipelines

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// The InitPipeline is a specialized component that manages application initialization functionality.
// It provides initialization-specific command execution including configuration setup, context management,
// blueprint template processing, and environment preparation for the Windsor CLI init command.
// The InitPipeline handles the complete initialization workflow with proper dependency injection and validation.

// =============================================================================
// Types
// =============================================================================

// InitConstructors defines constructor functions for InitPipeline dependencies
type InitConstructors struct {
	NewConfigHandler    func(di.Injector) config.ConfigHandler
	NewShell            func(di.Injector) shell.Shell
	NewBlueprintHandler func(di.Injector) blueprint.BlueprintHandler
	NewShims            func() *Shims
}

// InitPipeline provides application initialization functionality
type InitPipeline struct {
	BasePipeline

	constructors InitConstructors

	configHandler    config.ConfigHandler
	shell            shell.Shell
	blueprintHandler blueprint.BlueprintHandler
	shims            *Shims
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
			NewConfigHandler: func(injector di.Injector) config.ConfigHandler {
				return config.NewYamlConfigHandler(injector)
			},
			NewShell: func(injector di.Injector) shell.Shell {
				return shell.NewDefaultShell(injector)
			},
			NewBlueprintHandler: func(injector di.Injector) blueprint.BlueprintHandler {
				return blueprint.NewBlueprintHandler(injector)
			},
			NewShims: func() *Shims {
				return NewShims()
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
// It sets up the config handler, shell, and blueprint handler in the correct order,
// registering each component with the dependency injector and initializing them sequentially
// to ensure proper dependency resolution.
func (p *InitPipeline) Initialize(injector di.Injector) error {
	p.shims = p.constructors.NewShims()

	if existing := injector.Resolve("shell"); existing != nil {
		p.shell = existing.(shell.Shell)
	} else {
		p.shell = p.constructors.NewShell(injector)
		injector.Register("shell", p.shell)
	}
	p.BasePipeline.shell = p.shell

	if existing := injector.Resolve("configHandler"); existing != nil {
		p.configHandler = existing.(config.ConfigHandler)
	} else {
		p.configHandler = p.constructors.NewConfigHandler(injector)
		injector.Register("configHandler", p.configHandler)
	}
	p.BasePipeline.configHandler = p.configHandler

	if existing := injector.Resolve("blueprintHandler"); existing != nil {
		p.blueprintHandler = existing.(blueprint.BlueprintHandler)
	} else {
		p.blueprintHandler = p.constructors.NewBlueprintHandler(injector)
		injector.Register("blueprintHandler", p.blueprintHandler)
	}

	if err := p.shell.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize shell: %w", err)
	}

	if err := p.configHandler.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize config handler: %w", err)
	}

	// Try to initialize blueprint handler - if it fails, continue without it
	// This allows init to work without full kubernetes setup
	_ = p.blueprintHandler.Initialize()

	return nil
}

// Execute runs the initialization logic including configuration setup, context management,
// blueprint processing, and environment preparation. It handles all the initialization steps
// that were previously in the init command's RunE function.
func (p *InitPipeline) Execute(ctx context.Context) error {
	if err := p.setupTrustedEnvironment(); err != nil {
		return err
	}

	contextName, err := p.determineAndSetContext(ctx)
	if err != nil {
		return err
	}

	if err := p.configureSettings(ctx); err != nil {
		return err
	}

	if err := p.saveConfigAndProcessTemplates(contextName); err != nil {
		return err
	}

	p.outputSuccess(ctx)
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

// configureSettings handles all configuration setup including defaults, flags, and platform-specific settings
func (p *InitPipeline) configureSettings(ctx context.Context) error {
	// Extract flag values from context
	flagValues := make(map[string]any)
	if fv := ctx.Value("flagValues"); fv != nil {
		if flagMap, ok := fv.(map[string]any); ok {
			flagValues = flagMap
		}
	}

	changedFlags := make(map[string]bool)
	if cf := ctx.Value("changedFlags"); cf != nil {
		if changedMap, ok := cf.(map[string]bool); ok {
			changedFlags = changedMap
		}
	}

	setFlags := []string{}
	if sf := ctx.Value("setFlags"); sf != nil {
		if setSlice, ok := sf.([]string); ok {
			setFlags = setSlice
		}
	}

	// Determine platform and VM driver configuration
	platform := "local"
	if changedFlags["aws"] && flagValues["aws"] == true {
		platform = "aws"
	} else if changedFlags["azure"] && flagValues["azure"] == true {
		platform = "azure"
	} else if changedFlags["talos"] && flagValues["talos"] == true {
		platform = "talos"
	}

	vmDriverConfig := ""
	if changedFlags["colima"] && flagValues["colima"] == true {
		vmDriverConfig = "colima"
	} else if platform == "local" || strings.HasPrefix(platform, "local-") {
		switch p.shims.Getenv("GOOS") {
		case "darwin", "windows":
			vmDriverConfig = "docker-desktop"
		default:
			vmDriverConfig = "docker"
		}
	}

	// Set configuration defaults
	switch vmDriverConfig {
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

	if err := p.configHandler.SetContextValue("cluster.platform", platform); err != nil {
		return fmt.Errorf("Error setting platform: %w", err)
	}

	// Apply flag-based configurations
	if changedFlags["blueprint"] {
		if value, exists := flagValues["blueprint"]; exists && value != nil {
			if strValue, ok := value.(string); ok {
				if err := p.configHandler.SetContextValue("blueprint", strValue); err != nil {
					return fmt.Errorf("Error setting blueprint: %w", err)
				}
			}
		}
	}

	if changedFlags["terraform"] {
		if value, exists := flagValues["terraform"]; exists && value != nil {
			if boolValue, ok := value.(bool); ok {
				if err := p.configHandler.SetContextValue("terraform.enabled", boolValue); err != nil {
					return fmt.Errorf("Error setting terraform: %w", err)
				}
			}
		}
	}

	if changedFlags["k8s"] {
		if value, exists := flagValues["k8s"]; exists && value != nil {
			if boolValue, ok := value.(bool); ok {
				if err := p.configHandler.SetContextValue("cluster.enabled", boolValue); err != nil {
					return fmt.Errorf("Error setting k8s: %w", err)
				}
			}
		}
	}

	dockerComposeEnabled := true
	if vmDriverConfig == "colima" {
		dockerComposeEnabled = false
	}
	if changedFlags["docker-compose"] {
		if value, exists := flagValues["docker-compose"]; exists && value != nil {
			if boolValue, ok := value.(bool); ok {
				dockerComposeEnabled = boolValue
			}
		}
	}
	if err := p.configHandler.SetContextValue("docker.enabled", dockerComposeEnabled); err != nil {
		return fmt.Errorf("Error setting docker-compose: %w", err)
	}

	// Apply platform-specific configurations
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
	case "talos":
		if err := p.configHandler.SetContextValue("cluster.driver", "talos"); err != nil {
			return fmt.Errorf("Error setting cluster.driver: %w", err)
		}
	case "local":
		if err := p.configHandler.SetContextValue("cluster.driver", "talos"); err != nil {
			return fmt.Errorf("Error setting cluster.driver: %w", err)
		}
	}

	if changedFlags["talos"] && flagValues["talos"] == true {
		if err := p.configHandler.SetContextValue("cluster.driver", "talos"); err != nil {
			return fmt.Errorf("Error setting cluster.driver to talos: %w", err)
		}
	}

	// Apply --set flag configurations
	for _, setFlag := range setFlags {
		parts := strings.SplitN(setFlag, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("Invalid format for --set flag. Expected key=value")
		}
		key, value := parts[0], parts[1]
		if err := p.configHandler.SetContextValue(key, value); err != nil {
			return fmt.Errorf("Error setting config override %s: %w", key, err)
		}
	}

	if vmDriverConfig != "" && p.configHandler.GetString("vm.driver") == "" {
		if err := p.configHandler.SetContextValue("vm.driver", vmDriverConfig); err != nil {
			return fmt.Errorf("Error setting vm driver: %w", err)
		}
	}

	return nil
}

// saveConfigAndProcessTemplates saves the configuration file and processes blueprint templates.
// It determines the appropriate config file path (windsor.yaml or windsor.yml), generates a context ID,
// saves the configuration, and processes blueprint templates for the specified context.
func (p *InitPipeline) saveConfigAndProcessTemplates(contextName string) error {
	projectRoot, err := p.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("Error retrieving project root: %w", err)
	}

	yamlPath := filepath.Join(projectRoot, "windsor.yaml")
	ymlPath := filepath.Join(projectRoot, "windsor.yml")
	cliConfigPath := yamlPath
	if _, err := p.shims.Stat(yamlPath); err == nil {
		cliConfigPath = yamlPath
	} else if _, err := p.shims.Stat(ymlPath); err == nil {
		cliConfigPath = ymlPath
	}

	if err := p.configHandler.GenerateContextID(); err != nil {
		return fmt.Errorf("failed to generate context ID: %w", err)
	}

	if err := p.configHandler.SaveConfig(cliConfigPath); err != nil {
		return fmt.Errorf("Error saving config file: %w", err)
	}

	if err := p.blueprintHandler.ProcessContextTemplates(contextName, false); err != nil {
		return fmt.Errorf("Error processing context templates: %w", err)
	}

	if err := p.blueprintHandler.LoadConfig(false); err != nil {
		return fmt.Errorf("Error reloading blueprint config: %w", err)
	}

	return nil
}

// outputSuccess outputs the initialization success message to the appropriate output stream.
// It checks for a custom output function in the context and uses it if available,
// otherwise defaults to writing to stderr.
func (p *InitPipeline) outputSuccess(ctx context.Context) {
	output := ctx.Value("output")
	if output != nil {
		if outputFunc, ok := output.(func(string)); ok {
			outputFunc("Initialization successful")
		}
	} else {
		fmt.Fprintln(os.Stderr, "Initialization successful")
	}
}
