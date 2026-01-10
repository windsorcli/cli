package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/provisioner"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/workstation"
)

// Project orchestrates the setup and initialization of a Windsor project.
// It coordinates context, provisioner, composer, and workstation managers
// to provide a unified interface for project initialization and management.
type Project struct {
	Runtime       *runtime.Runtime
	configHandler config.ConfigHandler
	contextName   string
	projectRoot   string
	Provisioner   *provisioner.Provisioner
	Composer      *composer.Composer
	Workstation   *workstation.Workstation
}

// NewProject creates and initializes a new Project instance with all required managers.
// It sets up the execution context, applies config defaults, and creates provisioner,
// composer, and workstation managers. The workstation is only created if the project
// is in dev mode. Panics if required dependencies are nil.
// After creation, call Configure() to apply flag overrides if needed.
// Optional overrides can be provided via opts to inject mocks for testing.
// If opts contains a Project with Runtime set, that runtime will be reused.
func NewProject(contextName string, opts ...*Project) *Project {
	var rt *runtime.Runtime

	var overrides *Project
	if len(opts) > 0 && opts[0] != nil {
		overrides = opts[0]
		if overrides.Runtime != nil {
			rt = overrides.Runtime
		}
	}

	if rt == nil {
		var rtOpts []*runtime.Runtime
		if overrides != nil && overrides.Runtime != nil {
			rtOpts = []*runtime.Runtime{overrides.Runtime}
		}
		rt = runtime.NewRuntime(rtOpts...)
	}

	if rt == nil {
		panic("runtime is required")
	}
	if rt.ConfigHandler == nil {
		panic("config handler is required on runtime")
	}

	if contextName == "" {
		contextName = rt.ConfigHandler.GetContext()
		if contextName == "" {
			contextName = "local"
		}
	}

	rt.ContextName = contextName
	rt.ConfigRoot = filepath.Join(rt.ProjectRoot, "contexts", contextName)

	var comp *composer.Composer
	if overrides != nil && overrides.Composer != nil {
		comp = overrides.Composer
	} else {
		comp = composer.NewComposer(rt)
	}

	var prov *provisioner.Provisioner
	if overrides != nil && overrides.Provisioner != nil {
		prov = overrides.Provisioner
	} else {
		prov = provisioner.NewProvisioner(rt, comp.BlueprintHandler)
	}

	var ws *workstation.Workstation
	if rt.ConfigHandler.IsDevMode(contextName) {
		if overrides != nil && overrides.Workstation != nil {
			ws = overrides.Workstation
		} else {
			ws = workstation.NewWorkstation(rt)
		}
	}

	return &Project{
		Runtime:       rt,
		configHandler: rt.ConfigHandler,
		contextName:   rt.ContextName,
		projectRoot:   rt.ProjectRoot,
		Provisioner:   prov,
		Composer:      comp,
		Workstation:   ws,
	}
}

// Configure loads configuration from disk and applies flag-based overrides.
// This should be called after NewProject if command flags need to override
// configuration values. Returns an error if loading or applying overrides fails.
func (p *Project) Configure(flagOverrides map[string]any) error {
	if p.configHandler.IsDevMode(p.contextName) {
		if flagOverrides == nil {
			flagOverrides = make(map[string]any)
		}
		if _, exists := flagOverrides["provider"]; !exists {
			if p.configHandler.GetString("provider") == "" {
				flagOverrides["provider"] = "generic"
			}
		}
	}

	if err := p.Runtime.ApplyConfigDefaults(flagOverrides); err != nil {
		return fmt.Errorf("failed to apply config defaults: %w", err)
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

	if err := p.configHandler.LoadConfig(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	for key, value := range flagOverrides {
		if err := p.configHandler.Set(key, value); err != nil {
			return fmt.Errorf("failed to set %s: %w", key, err)
		}
	}

	if err := p.Runtime.LoadEnvironment(false); err != nil {
		return fmt.Errorf("failed to load environment: %w", err)
	}

	return nil
}

// Initialize runs the complete initialization sequence for the project.
// It prepares the workstation (creates services and assigns IPs), prepares context,
// generates infrastructure, prepares tools, and bootstraps the environment.
// The overwrite parameter controls whether infrastructure generation should overwrite
// existing files. The optional blueprintURL parameter specifies the blueprint artifact
// to load (OCI URL or local .tar.gz path). Returns an error if any step fails.
func (p *Project) Initialize(overwrite bool, blueprintURL ...string) error {
	if p.Workstation != nil {
		if err := p.Workstation.Prepare(); err != nil {
			return fmt.Errorf("failed to prepare workstation: %w", err)
		}
		if p.Workstation.ContainerRuntime != nil {
			if err := p.Workstation.ContainerRuntime.WriteConfig(); err != nil {
				return fmt.Errorf("failed to write container runtime config: %w", err)
			}
		}
	}

	if err := p.configHandler.GenerateContextID(); err != nil {
		return fmt.Errorf("failed to generate context ID: %w", err)
	}

	if err := p.Composer.BlueprintHandler.LoadBlueprint(blueprintURL...); err != nil {
		return fmt.Errorf("failed to load blueprint data: %w", err)
	}

	if err := p.configHandler.SaveConfig(overwrite); err != nil {
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
// saved state, then deletes the .volumes directory, .windsor/contexts/<context> directory,
// .windsor/Corefile, and .windsor/docker-compose.yaml. Returns an error if any cleanup step fails.
func (p *Project) PerformCleanup() error {
	if err := p.configHandler.Clean(); err != nil {
		return fmt.Errorf("error cleaning up context specific artifacts: %w", err)
	}

	volumesPath := filepath.Join(p.projectRoot, ".volumes")
	if err := os.RemoveAll(volumesPath); err != nil {
		return fmt.Errorf("error deleting .volumes folder: %w", err)
	}

	tfModulesPath := filepath.Join(p.projectRoot, ".windsor", "contexts", p.contextName)
	if err := os.RemoveAll(tfModulesPath); err != nil {
		return fmt.Errorf("error deleting .windsor/contexts/%s folder: %w", p.contextName, err)
	}

	corefilePath := filepath.Join(p.projectRoot, ".windsor", "Corefile")
	if err := os.RemoveAll(corefilePath); err != nil {
		return fmt.Errorf("error deleting .windsor/Corefile: %w", err)
	}

	dockerComposePath := filepath.Join(p.projectRoot, ".windsor", "docker-compose.yaml")
	if err := os.RemoveAll(dockerComposePath); err != nil {
		return fmt.Errorf("error deleting .windsor/docker-compose.yaml: %w", err)
	}

	return nil
}
