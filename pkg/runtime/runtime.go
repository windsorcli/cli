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
	err error
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
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Do returns the cumulative error state from all preceding runtime operations.
func (r *Runtime) Do() error {
	return r.err
}

// LoadShell loads the shell dependency, creating a new default shell if none exists.
func (r *Runtime) LoadShell() *Runtime {
	if r.err != nil {
		return r
	}

	if r.Shell == nil {
		r.Shell = shell.NewDefaultShell(r.Injector)
		r.Injector.Register("shell", r.Shell)
	}
	return r
}

// LoadConfigHandler loads and initializes the configuration handler dependency.
func (r *Runtime) LoadConfigHandler() *Runtime {
	if r.err != nil {
		return r
	}
	if r.Shell == nil {
		r.err = fmt.Errorf("shell not loaded - call LoadShell() first")
		return r
	}

	if r.ConfigHandler == nil {
		r.ConfigHandler = config.NewConfigHandler(r.Injector)
		if err := r.ConfigHandler.Initialize(); err != nil {
			r.err = fmt.Errorf("failed to initialize config handler: %w", err)
			return r
		}
	}
	return r
}

// LoadEnvPrinters loads and initializes the environment printers.
func (r *Runtime) LoadEnvPrinters() *Runtime {
	if r.err != nil {
		return r
	}
	if r.ConfigHandler == nil {
		r.err = fmt.Errorf("config handler not loaded - call LoadConfigHandler() first")
		return r
	}
	if r.EnvPrinters.AwsEnv == nil && r.ConfigHandler.GetBool("aws.enabled", false) {
		r.EnvPrinters.AwsEnv = env.NewAwsEnvPrinter(r.Injector)
		r.Injector.Register("awsEnv", r.EnvPrinters.AwsEnv)
	}
	if r.EnvPrinters.AzureEnv == nil && r.ConfigHandler.GetBool("azure.enabled", false) {
		r.EnvPrinters.AzureEnv = env.NewAzureEnvPrinter(r.Injector)
		r.Injector.Register("azureEnv", r.EnvPrinters.AzureEnv)
	}
	if r.EnvPrinters.DockerEnv == nil && r.ConfigHandler.GetBool("docker.enabled", false) {
		r.EnvPrinters.DockerEnv = env.NewDockerEnvPrinter(r.Injector)
		r.Injector.Register("dockerEnv", r.EnvPrinters.DockerEnv)
	}
	if r.EnvPrinters.KubeEnv == nil && r.ConfigHandler.GetBool("cluster.enabled", false) {
		r.EnvPrinters.KubeEnv = env.NewKubeEnvPrinter(r.Injector)
		r.Injector.Register("kubeEnv", r.EnvPrinters.KubeEnv)
	}
	if r.EnvPrinters.TalosEnv == nil &&
		(r.ConfigHandler.GetString("cluster.driver", "") == "talos" ||
			r.ConfigHandler.GetString("cluster.driver", "") == "omni") {
		r.EnvPrinters.TalosEnv = env.NewTalosEnvPrinter(r.Injector)
		r.Injector.Register("talosEnv", r.EnvPrinters.TalosEnv)
	}
	if r.EnvPrinters.TerraformEnv == nil && r.ConfigHandler.GetBool("terraform.enabled", false) {
		r.EnvPrinters.TerraformEnv = env.NewTerraformEnvPrinter(r.Injector)
		r.Injector.Register("terraformEnv", r.EnvPrinters.TerraformEnv)
	}
	if r.EnvPrinters.WindsorEnv == nil {
		r.EnvPrinters.WindsorEnv = env.NewWindsorEnvPrinter(r.Injector)
		r.Injector.Register("windsorEnv", r.EnvPrinters.WindsorEnv)
	}
	return r
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
		r.err = fmt.Errorf("config handler not loaded - call LoadConfigHandler() first")
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
		r.err = fmt.Errorf("config handler not loaded - call LoadConfigHandler() first")
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
