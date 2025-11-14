package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/provisioner"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/workstation"
)

// Project orchestrates the setup and initialization of a Windsor project.
// It coordinates context, provisioner, composer, and workstation managers
// to provide a unified interface for project initialization and management.
type Project struct {
	Runtime     *runtime.Runtime
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
func NewProject(injector di.Injector, contextName string, existingRuntime ...*runtime.Runtime) (*Project, error) {
	var rt *runtime.Runtime
	var err error

	if len(existingRuntime) > 0 && existingRuntime[0] != nil {
		rt = existingRuntime[0]
	} else {
		rt = &runtime.Runtime{
			Injector: injector,
		}
		rt, err = runtime.NewRuntime(rt)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize context: %w", err)
		}
	}

	if contextName == "" {
		contextName = rt.ConfigHandler.GetContext()
		if contextName == "" {
			contextName = "local"
		}
	}

	rt.ContextName = contextName
	rt.ConfigRoot = filepath.Join(rt.ProjectRoot, "contexts", contextName)

	if err := rt.ApplyConfigDefaults(); err != nil {
		return nil, err
	}

	provCtx := &provisioner.ProvisionerRuntime{
		Runtime: *rt,
	}
	prov := provisioner.NewProvisioner(provCtx)

	comp := composer.NewComposer(rt)

	var ws *workstation.Workstation
	if rt.ConfigHandler.IsDevMode(rt.ContextName) {
		workstationCtx := &workstation.WorkstationRuntime{
			Runtime: *rt,
		}
		ws, err = workstation.NewWorkstation(workstationCtx, rt.Injector)
		if err != nil {
			return nil, fmt.Errorf("failed to create workstation: %w", err)
		}
	}

	return &Project{
		Runtime:     rt,
		Provisioner: prov,
		Composer:    comp,
		Workstation: ws,
	}, nil
}

// Configure loads configuration from disk and applies flag-based overrides.
// This should be called after NewProject if command flags need to override
// configuration values. Returns an error if loading or applying overrides fails.
func (p *Project) Configure(flagOverrides map[string]any) error {
	contextName := p.Runtime.ContextName
	if contextName == "" {
		contextName = "local"
	}

	if p.Runtime.ConfigHandler.IsDevMode(contextName) {
		if flagOverrides == nil {
			flagOverrides = make(map[string]any)
		}
		if _, exists := flagOverrides["provider"]; !exists {
			if p.Runtime.ConfigHandler.GetString("provider") == "" {
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

	if err := p.Runtime.ApplyProviderDefaults(providerOverride); err != nil {
		return err
	}

	if err := p.Runtime.ConfigHandler.LoadConfig(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	for key, value := range flagOverrides {
		if err := p.Runtime.ConfigHandler.Set(key, value); err != nil {
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

	if err := p.Runtime.ConfigHandler.GenerateContextID(); err != nil {
		return fmt.Errorf("failed to generate context ID: %w", err)
	}
	if err := p.Runtime.ConfigHandler.SaveConfig(overwrite); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if err := p.Composer.Generate(overwrite); err != nil {
		return fmt.Errorf("failed to generate infrastructure: %w", err)
	}

	if err := p.Runtime.PrepareTools(); err != nil {
		return err
	}

	if err := p.Runtime.LoadEnvironment(true); err != nil {
		return fmt.Errorf("failed to load environment: %w", err)
	}

	return nil
}

// PerformCleanup removes context-specific artifacts including volumes, terraform modules,
// and generated configuration files. It calls the config handler's Clean method to remove
// saved state, then deletes the .volumes directory, .windsor/.tf_modules directory,
// .windsor/Corefile, and .windsor/docker-compose.yaml. Returns an error if any cleanup step fails.
func (p *Project) PerformCleanup() error {
	if err := p.Runtime.ConfigHandler.Clean(); err != nil {
		return fmt.Errorf("error cleaning up context specific artifacts: %w", err)
	}

	volumesPath := filepath.Join(p.Runtime.ProjectRoot, ".volumes")
	if err := os.RemoveAll(volumesPath); err != nil {
		return fmt.Errorf("error deleting .volumes folder: %w", err)
	}

	tfModulesPath := filepath.Join(p.Runtime.ProjectRoot, ".windsor", ".tf_modules")
	if err := os.RemoveAll(tfModulesPath); err != nil {
		return fmt.Errorf("error deleting .windsor/.tf_modules folder: %w", err)
	}

	corefilePath := filepath.Join(p.Runtime.ProjectRoot, ".windsor", "Corefile")
	if err := os.RemoveAll(corefilePath); err != nil {
		return fmt.Errorf("error deleting .windsor/Corefile: %w", err)
	}

	dockerComposePath := filepath.Join(p.Runtime.ProjectRoot, ".windsor", "docker-compose.yaml")
	if err := os.RemoveAll(dockerComposePath); err != nil {
		return fmt.Errorf("error deleting .windsor/docker-compose.yaml: %w", err)
	}

	return nil
}
