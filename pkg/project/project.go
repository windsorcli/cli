// The Project is the top level object of a Windsor command.
// It creates the runtime, the composer, the provisioner, and the workstation for a context.
// It runs the configure, initialize, up, bootstrap, and cleanup sequences with these objects.

package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/shims"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/provisioner"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/tools"
	"github.com/windsorcli/cli/pkg/workstation"
)

// =============================================================================
// Types
// =============================================================================

// Project holds the runtime, composer, provisioner and workstation of one context.
type Project struct {
	Runtime          *runtime.Runtime
	configHandler    config.ConfigHandler
	contextName      string
	projectRoot      string
	Provisioner      *provisioner.Provisioner
	Composer         *composer.Composer
	Workstation      *workstation.Workstation
	toolRequirements *tools.Requirements
	shims            *shims.Shims
}

// =============================================================================
// Constructor
// =============================================================================

// NewProject creates a Project for contextName and creates the runtime, composer and provisioner.
// It uses the current context, or "local", when contextName is empty. A non-empty contextName also sets the context of the config handler.
// It creates the Workstation only for a dev context. EnsureWorkstation creates it for other contexts after Configure.
// The fields of an optional Project in opts replace the default objects. Tests use this to inject mocks.
// It panics when the runtime has no config handler.
func NewProject(contextName string, opts ...*Project) *Project {
	var overrides *Project
	if len(opts) > 0 && opts[0] != nil {
		overrides = opts[0]
	}

	var rt *runtime.Runtime
	if overrides != nil && overrides.Runtime != nil {
		rt = overrides.Runtime
	} else {
		rt = runtime.NewRuntime()
	}

	if rt.ConfigHandler == nil {
		panic("config handler is required on runtime")
	}

	if contextName == "" {
		contextName = rt.ConfigHandler.GetContext()
		if contextName == "" {
			contextName = "local"
		}
	} else {
		rt.ConfigHandler = rt.ConfigHandler.WithContext(contextName)
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
		shims:         shims.NewShims(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// SetToolRequirements sets the tool families that Initialize checks.
// When it is not called, Initialize checks all tool families.
// A command that does not use a tool calls it, so the operator does not have to install that tool.
func (p *Project) SetToolRequirements(reqs tools.Requirements) {
	p.toolRequirements = &reqs
}

// Configure resolves the configuration of the project with flagOverrides and loads the environment variables.
// It returns an error when resolution or the environment load fails.
func (p *Project) Configure(flagOverrides map[string]any) error {
	if err := p.Runtime.ResolveConfig(flagOverrides); err != nil {
		return err
	}
	if err := p.Runtime.LoadEnvironment(false); err != nil {
		return werror.Wrap(err, "failed to load environment")
	}
	return nil
}

// EnsureWorkstation creates the Workstation when it is nil and workstation.runtime is set.
// Call it before code that uses p.Workstation.
func (p *Project) EnsureWorkstation() {
	if p.Workstation == nil && p.configHandler.GetString("workstation.runtime") != "" {
		p.Workstation = workstation.NewWorkstation(p.Runtime)
	}
}

// ComposeBlueprint generates the context ID and loads the blueprint from blueprintURL or the default sources.
// It does not write files or generate infrastructure. Use it when only the composed blueprint is necessary.
func (p *Project) ComposeBlueprint(blueprintURL ...string) error {
	if err := p.configHandler.GenerateContextID(); err != nil {
		return werror.Wrap(err, "failed to generate context ID")
	}
	if err := p.Composer.BlueprintHandler.LoadBlueprint(blueprintURL...); err != nil {
		return err
	}
	return nil
}

// Initialize prepares the workstation, loads the blueprint, saves the config and generates the infrastructure files.
// Then it prepares the tools and loads the environment. overwrite lets the generate step replace existing files.
// blueprintURL is an optional OCI URL or local .tar.gz path. It returns an error when a step fails.
func (p *Project) Initialize(overwrite bool, blueprintURL ...string) error {
	p.EnsureWorkstation()
	if p.Workstation != nil {
		if err := p.Workstation.Prepare(); err != nil {
			return werror.Wrap(err, "failed to prepare workstation")
		}
		if p.Workstation.ContainerRuntime != nil {
			if err := p.Workstation.ContainerRuntime.WriteConfig(); err != nil {
				return werror.Wrap(err, "failed to write container runtime config")
			}
		}
	}

	if err := p.configHandler.GenerateContextID(); err != nil {
		return werror.Wrap(err, "failed to generate context ID")
	}

	if err := p.Composer.BlueprintHandler.LoadBlueprint(blueprintURL...); err != nil {
		var reqErr *blueprint.RequirementsError
		if errors.As(err, &reqErr) {
			return err
		}
		return werror.Wrap(err, "failed to load blueprint data")
	}

	if err := p.Runtime.SaveConfig(overwrite); err != nil {
		return werror.Wrap(err, "failed to save config")
	}

	if err := p.Composer.Generate(overwrite); err != nil {
		return werror.Wrap(err, "failed to generate infrastructure")
	}

	reqs := tools.AllRequirements()
	if p.toolRequirements != nil {
		reqs = *p.toolRequirements
	}
	if err := p.Runtime.PrepareToolsFor(reqs); err != nil {
		return err
	}

	if err := p.Runtime.LoadEnvironment(true); err != nil {
		return werror.Wrap(err, "failed to load environment")
	}

	return nil
}

// Up generates the blueprint, starts the workstation when it exists and runs the provisioner.
// Before the provisioner runs, it restores the host route to a cluster that holds the Terraform state.
// It returns the blueprint, a halt flag and an error. The halt flag is true when an apply hook stops the run cleanly,
// or when the host route needs a privileged step. The caller then shows the deferred work and exits without an error.
func (p *Project) Up(ctx context.Context) (*blueprintv1alpha1.Blueprint, bool, error) {
	blueprint := p.Composer.BlueprintHandler.Generate()
	if blueprint == nil {
		return nil, false, fmt.Errorf("blueprint not loaded")
	}
	onApply, err := p.prepareWorkstationForApply(ctx, blueprint)
	if err != nil {
		return nil, false, err
	}
	if p.Workstation != nil {
		reachable, err := p.Workstation.RestoreClusterRoute(ctx)
		if err != nil {
			return nil, false, werror.Wrap(err, "error restoring host route")
		}
		if !reachable {
			return blueprint, true, nil
		}
	}
	var hooks []func(context.Context, string) (bool, error)
	if onApply != nil {
		hooks = []func(context.Context, string) (bool, error){onApply}
	}
	halted, err := p.Provisioner.Up(ctx, blueprint, hooks...)
	if err != nil {
		return nil, false, werror.Wrap(err, "error starting infrastructure")
	}
	return blueprint, halted, nil
}

// Bootstrap is the first run of Up. It calls confirm with a blueprint summary before it starts the workstation.
// When the operator declines, no privileged work or terraform work runs.
// The provisioner applies with a local backend and then migrates the state to the configured backend.
// It returns the blueprint, an applied flag, a halt flag and an error. applied is false when confirm declines.
func (p *Project) Bootstrap(ctx context.Context, confirm provisioner.BootstrapConfirmFn) (*blueprintv1alpha1.Blueprint, bool, bool, error) {
	blueprint := p.Composer.BlueprintHandler.Generate()
	if blueprint == nil {
		return nil, false, false, fmt.Errorf("blueprint not loaded")
	}

	if confirm != nil {
		backendType := p.configHandler.GetTerraformBackendType()
		summary := provisioner.BuildBootstrapSummary(blueprint, p.contextName, backendType)
		if !confirm(summary) {
			return blueprint, false, false, nil
		}
	}

	onApply, err := p.prepareWorkstationForApply(ctx, blueprint)
	if err != nil {
		return nil, false, false, err
	}
	var hooks []func(context.Context, string) (bool, error)
	if onApply != nil {
		hooks = []func(context.Context, string) (bool, error){onApply}
	}
	halted, err := p.Provisioner.Bootstrap(ctx, blueprint, hooks...)
	if err != nil {
		return nil, false, false, werror.Wrap(err, "error starting infrastructure")
	}
	return blueprint, true, halted, nil
}

// PerformCleanup removes the artifacts of the context. It cleans the config state and removes
// the .kube and .talos directories under contexts/<context>. It removes the contents of
// .windsor/contexts/<context>, but keeps workstation.yaml. It returns an error if a step fails.
// When .windsor/contexts/<context> is a file, the error has a remediation hint.
func (p *Project) PerformCleanup() error {
	if err := p.configHandler.Clean(); err != nil {
		return werror.Wrap(err, "error cleaning up context specific artifacts")
	}

	configRoot, err := p.configHandler.GetConfigRoot()
	if err != nil {
		return werror.Wrap(err, "error getting config root")
	}
	for _, dir := range []string{".kube", ".talos"} {
		if err := p.shims.RemoveAll(filepath.Join(configRoot, dir)); err != nil {
			return werror.Wrap(err, "error deleting %s", dir)
		}
	}

	contextDir := filepath.Join(p.projectRoot, ".windsor", "contexts", p.contextName)
	info, err := p.shims.Stat(contextDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return werror.Wrap(err, "error reading .windsor/contexts/%s", p.contextName)
	}
	if !info.IsDir() {
		return werror.New(fmt.Sprintf("error reading .windsor/contexts/%s: not a directory", p.contextName), fmt.Sprintf("Remove the file .windsor/contexts/%s, then run the command again.", p.contextName), nil)
	}
	entries, err := p.shims.ReadDir(contextDir)
	if err != nil {
		return werror.Wrap(err, "error reading .windsor/contexts/%s", p.contextName)
	}
	for _, entry := range entries {
		if entry.Name() == "workstation.yaml" {
			continue
		}
		if err := p.shims.RemoveAll(filepath.Join(contextDir, entry.Name())); err != nil {
			return werror.Wrap(err, "error deleting %s", entry.Name())
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// prepareWorkstationForApply starts the workstation and registers its post-apply hook on the provisioner.
// It returns the apply hook of the workstation, or nil when no workstation exists.
// Workstation start can write DNS resolver files and ask for sudo.
func (p *Project) prepareWorkstationForApply(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint) (func(context.Context, string) (bool, error), error) {
	p.EnsureWorkstation()
	if p.Workstation == nil {
		return nil, nil
	}
	p.Workstation.PrepareForUp(blueprint)
	if err := p.Workstation.Up(ctx); err != nil {
		return nil, werror.Wrap(err, "error starting workstation")
	}
	onApply := p.Workstation.MakeApplyHook()
	if postApply := p.Workstation.MakePostApplyHook(); postApply != nil {
		p.Provisioner.OnTerraformPostApply(postApply)
	}
	return onApply, nil
}
