package project

import (
	"fmt"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/provisioner"
	"github.com/windsorcli/cli/pkg/workstation"
)

// Project orchestrates the setup and initialization of a Windsor project.
// It coordinates context, provisioner, composer, and workstation managers
// to provide a unified interface for project initialization and management.
type Project struct {
	Context     *context.ExecutionContext
	Provisioner *provisioner.Provisioner
	Composer    *composer.Composer
	Workstation *workstation.Workstation
}

// NewProject creates and initializes a new Project instance with all required managers.
// It sets up the execution context, applies config defaults, and creates provisioner,
// composer, and workstation managers. The workstation is only created if the project
// is in dev mode. If an existing context is provided, it will be reused; otherwise,
// a new context will be created. Returns the initialized Project or an error if any step fails.
// After creation, call Configure() to apply flag overrides if needed.
func NewProject(injector di.Injector, contextName string, existingCtx ...*context.ExecutionContext) (*Project, error) {
	var baseCtx *context.ExecutionContext
	var err error

	if len(existingCtx) > 0 && existingCtx[0] != nil {
		baseCtx = existingCtx[0]
	} else {
		baseCtx = &context.ExecutionContext{
			Injector: injector,
		}
		baseCtx, err = context.NewContext(baseCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize context: %w", err)
		}
	}

	if contextName == "" {
		contextName = baseCtx.ConfigHandler.GetContext()
		if contextName == "" {
			contextName = "local"
		}
	}

	baseCtx.ContextName = contextName
	baseCtx.ConfigRoot = filepath.Join(baseCtx.ProjectRoot, "contexts", contextName)

	if err := baseCtx.ApplyConfigDefaults(); err != nil {
		return nil, err
	}

	provCtx := &provisioner.ProvisionerExecutionContext{
		ExecutionContext: *baseCtx,
	}
	prov := provisioner.NewProvisioner(provCtx)

	composerCtx := &composer.ComposerExecutionContext{
		ExecutionContext: *baseCtx,
	}
	comp := composer.NewComposer(composerCtx)

	var ws *workstation.Workstation
	if baseCtx.ConfigHandler.IsDevMode(baseCtx.ContextName) {
		workstationCtx := &workstation.WorkstationExecutionContext{
			ExecutionContext: *baseCtx,
		}
		ws, err = workstation.NewWorkstation(workstationCtx, baseCtx.Injector)
		if err != nil {
			return nil, fmt.Errorf("failed to create workstation: %w", err)
		}
	}

	return &Project{
		Context:     baseCtx,
		Provisioner: prov,
		Composer:    comp,
		Workstation: ws,
	}, nil
}

// Configure loads configuration from disk and applies flag-based overrides.
// This should be called after NewProject if command flags need to override
// configuration values. Returns an error if loading or applying overrides fails.
func (p *Project) Configure(flagOverrides map[string]any) error {
	contextName := p.Context.ContextName
	if contextName == "" {
		contextName = "local"
	}

	if p.Context.ConfigHandler.IsDevMode(contextName) {
		if flagOverrides == nil {
			flagOverrides = make(map[string]any)
		}
		if _, exists := flagOverrides["provider"]; !exists {
			if p.Context.ConfigHandler.GetString("provider") == "" {
				flagOverrides["provider"] = "generic"
			}
		}
	}

	providerOverride := ""
	if flagOverrides != nil {
		if prov, ok := flagOverrides["provider"].(string); ok {
			providerOverride = prov
		}
	}

	if err := p.Context.ApplyProviderDefaults(providerOverride); err != nil {
		return err
	}

	if err := p.Context.ConfigHandler.LoadConfig(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	for key, value := range flagOverrides {
		if err := p.Context.ConfigHandler.Set(key, value); err != nil {
			return fmt.Errorf("failed to set %s: %w", key, err)
		}
	}

	return nil
}

// Initialize runs the complete initialization sequence for the project.
// It initializes network, prepares context, generates infrastructure, prepares tools,
// and bootstraps the environment. The overwrite parameter controls whether
// infrastructure generation should overwrite existing files. Returns an error
// if any step fails.
func (p *Project) Initialize(overwrite bool) error {
	if p.Workstation != nil && p.Workstation.NetworkManager != nil {
		if err := p.Workstation.NetworkManager.Initialize(p.Workstation.Services); err != nil {
			return fmt.Errorf("failed to initialize network manager: %w", err)
		}
	}

	if err := p.Context.ConfigHandler.GenerateContextID(); err != nil {
		return fmt.Errorf("failed to generate context ID: %w", err)
	}
	if err := p.Context.ConfigHandler.SaveConfig(overwrite); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if err := p.Composer.Generate(overwrite); err != nil {
		return fmt.Errorf("failed to generate infrastructure: %w", err)
	}

	if err := p.Context.PrepareTools(); err != nil {
		return err
	}

	if err := p.Context.LoadEnvironment(true); err != nil {
		return fmt.Errorf("failed to load environment: %w", err)
	}

	return nil
}
