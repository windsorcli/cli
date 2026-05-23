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
	"github.com/windsorcli/cli/pkg/runtime/tools"
	"github.com/windsorcli/cli/pkg/workstation"
)

// =============================================================================
// Types
// =============================================================================

// Project orchestrates the setup and initialization of a Windsor project.
// It coordinates context, provisioner, composer, and workstation managers
// to provide a unified interface for project initialization and management.
type Project struct {
	Runtime           *runtime.Runtime
	configHandler     config.ConfigHandler
	contextName       string
	projectRoot       string
	Provisioner       *provisioner.Provisioner
	Composer          *composer.Composer
	Workstation       *workstation.Workstation
	toolRequirements  *tools.Requirements
}

// =============================================================================
// Constructor
// =============================================================================

// NewProject creates and initializes a new Project instance with all required managers.
// It sets up the execution context, creates provisioner and composer, and creates the
// workstation when in dev mode or when is true (config is loaded if needed for the latter).
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
		if rt.ConfigHandler.GetString("workstation.runtime") != "" {
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

// =============================================================================
// Public Methods
// =============================================================================

// SetToolRequirements narrows which tool families Initialize will check on this project.
// When unset, Initialize defaults to AllRequirements — the historical "check everything"
// behavior. Commands whose codepath is statically known (e.g. `windsor down` only stops
// the workstation, never invokes terraform) call this with a narrower set so the operator
// is not blocked on installing tools they will not actually use.
func (p *Project) SetToolRequirements(reqs tools.Requirements) {
	p.toolRequirements = &reqs
}

// Configure resolves project configuration including defaults, file loading, migration, and override processing.
// Loads project environment variables and returns an error if resolution or environment loading fails.
func (p *Project) Configure(flagOverrides map[string]any) error {
	if err := p.Runtime.ResolveConfig(flagOverrides); err != nil {
		return err
	}
	if err := p.Runtime.LoadEnvironment(false); err != nil {
		return fmt.Errorf("failed to load environment: %w", err)
	}
	return nil
}

// EnsureWorkstation creates Workstation if it is nil and workstation.runtime is set.
// Call before using p.Workstation in code paths that need it (e.g. Initialize, Up, Down).
func (p *Project) EnsureWorkstation() {
	if p.Workstation == nil && p.configHandler.GetString("workstation.runtime") != "" {
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

	if err := p.Runtime.SaveConfig(overwrite); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if err := p.Composer.Generate(overwrite); err != nil {
		return fmt.Errorf("failed to generate infrastructure: %w", err)
	}

	reqs := tools.AllRequirements()
	if p.toolRequirements != nil {
		reqs = *p.toolRequirements
	}
	if err := p.Runtime.PrepareToolsFor(reqs); err != nil {
		return err
	}

	if err := p.Runtime.LoadEnvironment(true); err != nil {
		return fmt.Errorf("failed to load environment: %w", err)
	}

	return nil
}

// Up generates the blueprint, starts the workstation if present (using PrepareForUp to defer
// host/guest setup when a workstation Terraform component exists), runs the provisioner, and
// returns the blueprint for use by Install/Wait.
//
// Returns (blueprint, halted, err). halted=true means a hook signaled a clean stop after a
// component apply (e.g. cluster reachability needs host configuration the operator hasn't
// done yet); err remains the path for real failures. Callers render the deferred-work
// summary based on the halt and exit cleanly.
func (p *Project) Up() (*blueprintv1alpha1.Blueprint, bool, error) {
	return p.runApply(p.Provisioner.Up)
}

// Bootstrap is Up's first-run sibling. Operator confirmation runs first against a cheap
// blueprint summary; only after acceptance does workstation prep run (so declining never
// triggers privileged work like DNS resolver writes or sudo prompts). Then the provisioner
// pins backend.type=local for one Up pass and migrates state to the configured remote
// backend at the end.
//
// Returns (blueprint, applied, halted, err). applied=false when confirm declined; in that
// case no workstation startup or terraform work has happened. halted=true means one of the
// inner Up calls signaled a clean halt-after-component.
func (p *Project) Bootstrap(confirm provisioner.BootstrapConfirmFn) (*blueprintv1alpha1.Blueprint, bool, bool, error) {
	blueprint := p.Composer.BlueprintHandler.Generate()
	if blueprint == nil {
		return nil, false, false, fmt.Errorf("blueprint not loaded")
	}

	backendType := p.configHandler.GetString("terraform.backend.type", "local")
	if err := provisioner.ValidateBootstrap(blueprint, backendType); err != nil {
		return blueprint, false, false, err
	}

	if confirm != nil {
		summary := provisioner.BuildBootstrapSummary(blueprint, p.contextName, backendType)
		if !confirm(summary) {
			return blueprint, false, false, nil
		}
	}

	onApply, err := p.prepareWorkstationForApply(blueprint)
	if err != nil {
		return nil, false, false, err
	}
	var hooks []func(string) (bool, error)
	if onApply != nil {
		hooks = []func(string) (bool, error){onApply}
	}
	halted, err := p.Provisioner.Bootstrap(blueprint, hooks...)
	if err != nil {
		return nil, false, false, fmt.Errorf("error starting infrastructure: %w", err)
	}
	return blueprint, true, halted, nil
}

// PerformCleanup removes context-specific artifacts: config state and
// contents of .windsor/contexts/<context> (preserving workstation.yaml).
// Returns an error if any step fails.
func (p *Project) PerformCleanup() error {
	if err := p.configHandler.Clean(); err != nil {
		return fmt.Errorf("error cleaning up context specific artifacts: %w", err)
	}

	contextDir := filepath.Join(p.projectRoot, ".windsor", "contexts", p.contextName)
	info, err := os.Stat(contextDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("error reading .windsor/contexts/%s: %w", p.contextName, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("error reading .windsor/contexts/%s: not a directory", p.contextName)
	}
	entries, err := os.ReadDir(contextDir)
	if err != nil {
		return fmt.Errorf("error reading .windsor/contexts/%s: %w", p.contextName, err)
	}
	for _, entry := range entries {
		if entry.Name() == "workstation.yaml" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(contextDir, entry.Name())); err != nil {
			return fmt.Errorf("error deleting %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// runApply runs workstation prep then dispatches to a Provisioner Up-style method (Up or
// Bootstrap), wrapping any error as "error starting infrastructure". Propagates the halt
// signal from the dispatched method so the caller can render the deferred-work summary.
func (p *Project) runApply(fn func(*blueprintv1alpha1.Blueprint, ...func(string) (bool, error)) (bool, error)) (*blueprintv1alpha1.Blueprint, bool, error) {
	blueprint, onApply, err := p.prepareForApply()
	if err != nil {
		return nil, false, err
	}
	var hooks []func(string) (bool, error)
	if onApply != nil {
		hooks = []func(string) (bool, error){onApply}
	}
	halted, err := fn(blueprint, hooks...)
	if err != nil {
		return nil, false, fmt.Errorf("error starting infrastructure: %w", err)
	}
	return blueprint, halted, nil
}

// prepareForApply generates the blueprint and runs workstation lifecycle hooks before any
// terraform applies. Used by Up; Bootstrap drives the two halves separately so confirmation
// gates workstation prep.
func (p *Project) prepareForApply() (*blueprintv1alpha1.Blueprint, func(string) (bool, error), error) {
	blueprint := p.Composer.BlueprintHandler.Generate()
	onApply, err := p.prepareWorkstationForApply(blueprint)
	if err != nil {
		return nil, nil, err
	}
	return blueprint, onApply, nil
}

// prepareWorkstationForApply runs the privileged half of pre-apply setup: workstation Up
// (DNS resolver writes, host routes, container runtime). Split out so Bootstrap can gate
// it on operator confirmation — declining the plan must not trigger sudo prompts or
// /etc/resolver writes.
func (p *Project) prepareWorkstationForApply(blueprint *blueprintv1alpha1.Blueprint) (func(string) (bool, error), error) {
	p.EnsureWorkstation()
	if p.Workstation == nil {
		return nil, nil
	}
	p.Workstation.PrepareForUp(blueprint)
	if err := p.Workstation.Up(); err != nil {
		return nil, fmt.Errorf("error starting workstation: %w", err)
	}
	onApply := p.Workstation.MakeApplyHook()
	if postApply := p.Workstation.MakePostApplyHook(); postApply != nil {
		p.Provisioner.OnTerraformPostApply(postApply)
	}
	return onApply, nil
}
