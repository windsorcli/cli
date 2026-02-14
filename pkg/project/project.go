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
		prov = provisioner.NewProvisioner(rt, comp.BlueprintHandler, &provisioner.Provisioner{Workstation: ws})
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
				vmDriver := ""
				if flagOverrides != nil {
					if driver, ok := flagOverrides["vm.driver"].(string); ok {
						vmDriver = driver
					}
				}
				if vmDriver == "" {
					vmDriver = p.configHandler.GetString("vm.driver")
				}
				vmRuntime := ""
				if flagOverrides != nil {
					if r, ok := flagOverrides["vm.runtime"].(string); ok {
						vmRuntime = r
					}
				}
				if vmRuntime == "" {
					vmRuntime = p.configHandler.GetString("vm.runtime", "docker")
				}
				if vmDriver == "colima" && vmRuntime == "incus" {
					fmt.Fprintln(os.Stderr, "\033[33mWarning: vm.runtime is deprecated; use provider: incus in your context configuration instead. Support for vm.runtime will be removed in a future version.\033[0m")
					flagOverrides["provider"] = "incus"
				} else {
					flagOverrides["provider"] = "docker"
				}
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

	if p.configHandler.IsDevMode(p.contextName) {
		provider := p.configHandler.GetString("provider")
		vmDriver := p.configHandler.GetString("vm.driver")
		vmRuntime := p.configHandler.GetString("vm.runtime", "docker")
		if (provider == "" || provider == "docker") && vmDriver == "colima" && vmRuntime == "incus" {
			fmt.Fprintln(os.Stderr, "\033[33mWarning: vm.runtime is deprecated; use provider: incus in your context configuration instead. Support for vm.runtime will be removed in a future version.\033[0m")
			if err := p.configHandler.Set("provider", "incus"); err != nil {
				return fmt.Errorf("failed to set provider to incus: %w", err)
			}
		}
	}

	if p.Workstation == nil && p.configHandler.GetBool("workstation.enabled", false) {
		p.Workstation = workstation.NewWorkstation(p.Runtime)
	}
	if p.Workstation != nil && p.Provisioner != nil {
		p.Provisioner.Workstation = p.Workstation
	}

	if err := p.Runtime.LoadEnvironment(false); err != nil {
		return fmt.Errorf("failed to load environment: %w", err)
	}

	return nil
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
