package project

import (
	"fmt"
	"os"
	"path/filepath"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
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
// It sets up the execution context, creates provisioner and composer, and creates the
// workstation when in dev mode or when workstation.enabled is true (config is loaded if needed for the latter).
// Panics if required dependencies are nil. After creation, call Configure() to apply flag overrides.
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

	var ws *workstation.Workstation
	if overrides != nil && overrides.Workstation != nil {
		ws = overrides.Workstation
	} else if rt.ConfigHandler.IsDevMode(contextName) {
		ws = workstation.NewWorkstation(rt)
	} else {
		if !rt.ConfigHandler.IsLoaded() {
			_ = rt.ConfigHandler.LoadConfig()
		}
		if rt.ConfigHandler.GetBool("workstation.enabled", false) {
			ws = workstation.NewWorkstation(rt)
		}
	}

	var prov *provisioner.Provisioner
	if overrides != nil && overrides.Provisioner != nil {
		prov = overrides.Provisioner
	} else {
		prov = provisioner.NewProvisioner(rt, comp.BlueprintHandler)
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

// Configure resolves configuration (defaults, load, migration, overrides), applies
// workstation.enabled override to the project (nils Workstation when override is false),
// and loads environment. Does not create Workstation; use EnsureWorkstation() when
// a code path needs the workstation. Returns error on resolve or load failure.
func (p *Project) Configure(flagOverrides map[string]any) error {
	if err := p.Runtime.ResolveConfig(flagOverrides); err != nil {
		return err
	}
	if flagOverrides != nil {
		if v, ok := flagOverrides["workstation.enabled"].(bool); ok && !v {
			p.Workstation = nil
		}
	}
	if err := p.Runtime.LoadEnvironment(false); err != nil {
		return fmt.Errorf("failed to load environment: %w", err)
	}
	return nil
}

// EnsureWorkstation creates Workstation if it is nil and config has workstation.enabled true.
// Call before using p.Workstation in code paths that need it (e.g. Initialize, Up, Down).
func (p *Project) EnsureWorkstation() {
	if p.Workstation == nil && p.configHandler.GetBool("workstation.enabled", false) {
		p.Workstation = workstation.NewWorkstation(p.Runtime)
	}
}

// ComposeBlueprint loads and composes the blueprint without writing files or generating infrastructure.
// It generates context ID if needed and loads all blueprint sources. Use this when the goal is only
// to obtain the composed blueprint (e.g. for windsor show blueprint). It does not run Composer.Generate()
// and thus does not write blueprint files, process terraform modules, or generate tfvars.
func (p *Project) ComposeBlueprint(blueprintURL ...string) error {
	if err := p.configHandler.GenerateContextID(); err != nil {
		return fmt.Errorf("failed to generate context ID: %w", err)
	}
	if err := p.Composer.BlueprintHandler.LoadBlueprint(blueprintURL...); err != nil {
		return err
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
	p.EnsureWorkstation()
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

// Up generates the blueprint, starts the workstation when present (using PrepareForUp so host/guest setup is deferred when a workstation Terraform component exists), runs provisioner with the workstation post-apply hook when present, and returns the blueprint for use by Install/Wait. Returns an error if any step fails.
func (p *Project) Up() (*blueprintv1alpha1.Blueprint, error) {
	p.EnsureWorkstation()
	blueprint := p.Composer.BlueprintHandler.Generate()
	var onApply []func(string) error
	if p.Workstation != nil {
		p.Workstation.PrepareForUp(blueprint)
		if err := p.Workstation.Up(); err != nil {
			return nil, fmt.Errorf("error starting workstation: %w", err)
		}
		onApply = append(onApply, p.Workstation.AfterWorkstationComponent())
	}
	if err := p.Provisioner.Up(blueprint, onApply...); err != nil {
		return nil, fmt.Errorf("error starting infrastructure: %w", err)
	}
	return blueprint, nil
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
