package runtime

import (
	"fmt"

	"github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/cluster"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/generators"
	"github.com/windsorcli/cli/pkg/kubernetes"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/terraform"
	"github.com/windsorcli/cli/pkg/tools"
	"github.com/windsorcli/cli/pkg/workstation/network"
	"github.com/windsorcli/cli/pkg/workstation/services"
	"github.com/windsorcli/cli/pkg/workstation/ssh"
	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// =============================================================================
// Types
// =============================================================================

// Dependencies contains all the dependencies that Runtime might need.
// This allows for explicit dependency injection without complex DI frameworks.
type Dependencies struct {
	Injector      di.Injector
	Shell         shell.Shell
	ConfigHandler config.ConfigHandler
	ToolsManager  tools.ToolsManager
	EnvPrinters   struct {
		AwsEnv       env.EnvPrinter
		AzureEnv     env.EnvPrinter
		DockerEnv    env.EnvPrinter
		KubeEnv      env.EnvPrinter
		TalosEnv     env.EnvPrinter
		TerraformEnv env.EnvPrinter
		WindsorEnv   env.EnvPrinter
	}
	SecretsProviders struct {
		Sops        secrets.SecretsProvider
		Onepassword secrets.SecretsProvider
	}
	BlueprintHandler blueprint.BlueprintHandler
	ArtifactBuilder  artifact.Artifact
	Generators       struct {
		GitGenerator       generators.Generator
		TerraformGenerator generators.Generator
	}
	TerraformResolver terraform.ModuleResolver
	ClusterClient     cluster.ClusterClient
	K8sManager        kubernetes.KubernetesManager
	Workstation       struct {
		Virt     virt.Virt
		Services struct {
			DnsService           services.Service
			GitLivereloadService services.Service
			LocalstackService    services.Service
			RegistryServices     map[string]services.Service
			TalosServices        map[string]services.Service
		}
		Network network.NetworkManager
		Ssh     ssh.SSHClient
	}
}

// Runtime encapsulates all core Windsor CLI runtime dependencies.
type Runtime struct {
	Dependencies
	Shims *Shims
	err   error
}

// =============================================================================
// Constructor
// =============================================================================

// NewRuntime creates a new Runtime instance with the provided dependencies.
func NewRuntime(deps ...*Dependencies) *Runtime {
	var depsVal *Dependencies
	if len(deps) > 0 && deps[0] != nil {
		depsVal = deps[0]
	} else {
		depsVal = &Dependencies{}
	}
	if depsVal.Injector == nil {
		depsVal.Injector = di.NewInjector()
	}
	return &Runtime{
		Dependencies: *depsVal,
		Shims:        NewShims(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Do returns the cumulative error state from all preceding runtime operations.
func (r *Runtime) Do() error {
	return r.err
}

// InstallHook installs a shell hook for the specified shell type.
func (r *Runtime) InstallHook(shellType string) *Runtime {
	if r.err != nil {
		return r
	}
	if r.Shell == nil {
		r.err = fmt.Errorf("shell not loaded - call LoadShell() first")
		return r
	}
	r.err = r.Shell.InstallHook(shellType)
	return r
}

// SetContext sets the context for the configuration handler.
func (r *Runtime) SetContext(context string) *Runtime {
	if r.err != nil {
		return r
	}
	if r.ConfigHandler == nil {
		r.err = fmt.Errorf("config handler not loaded - call LoadConfig() first")
		return r
	}
	r.err = r.ConfigHandler.SetContext(context)
	return r
}

// PrintContext outputs the current context using the provided output function.
func (r *Runtime) PrintContext(outputFunc func(string)) *Runtime {
	if r.err != nil {
		return r
	}
	if r.ConfigHandler == nil {
		r.err = fmt.Errorf("config handler not loaded - call LoadConfig() first")
		return r
	}
	context := r.ConfigHandler.GetContext()
	outputFunc(context)
	return r
}

// WriteResetToken writes a session/token reset file using the shell.
func (r *Runtime) WriteResetToken() *Runtime {
	if r.err != nil {
		return r
	}
	if r.Shell == nil {
		r.err = fmt.Errorf("shell not loaded - call LoadShell() first")
		return r
	}
	_, r.err = r.Shell.WriteResetToken()
	return r
}
