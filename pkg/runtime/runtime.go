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

// Runtime encapsulates all core Windsor CLI runtime dependencies for injection.
type Runtime struct {

	// Core dependencies
	shell         shell.Shell
	configHandler config.ConfigHandler
	toolsManager  tools.ToolsManager
	envPrinters   struct {
		awsEnv       env.EnvPrinter
		azureEnv     env.EnvPrinter
		dockerEnv    env.EnvPrinter
		kubeEnv      env.EnvPrinter
		talosEnv     env.EnvPrinter
		terraformEnv env.EnvPrinter
		windsorEnv   env.EnvPrinter
	}
	secretsProviders struct {
		sops        secrets.SecretsProvider
		onepassword secrets.SecretsProvider
	}

	// Blueprint dependencies
	blueprintHandler blueprint.BlueprintHandler
	artifactBuilder  artifact.Artifact
	generators       struct {
		gitGenerator       generators.Generator
		terraformGenerator generators.Generator
	}
	terraformResolver terraform.ModuleResolver

	// Cluster dependencies
	clusterClient cluster.ClusterClient
	k8sManager    kubernetes.KubernetesManager

	// Workstation dependencies
	workstation struct {
		virt     virt.Virt
		services struct {
			dnsService           services.Service
			gitLivereloadService services.Service
			localstackService    services.Service
			registryServices     map[string]services.Service
			talosServices        map[string]services.Service
		}
		network network.NetworkManager
		ssh     ssh.SSHClient
	}

	// Error
	err error

	// Injector (to be deprecated)
	injector di.Injector
}

// =============================================================================
// Constructor
// =============================================================================

// NewRuntime creates a new Runtime instance
func NewRuntime(injector di.Injector) *Runtime {
	return &Runtime{
		injector: injector,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Do serves as the final execution point in the Windsor application lifecycle.
// It returns the cumulative error state from all preceding runtime operations, ensuring that the
// top-level caller receives any error reported by the Windsor runtime subsystems.
//
// Do does not perform any additional processing; it solely propagates the stored error value,
// which must be set by lower-level runtime methods. If no error has occurred, Do returns nil.
func (r *Runtime) Do() error {
	return r.err
}

// LoadShell loads the shell dependency from the injector.
func (r *Runtime) LoadShell() *Runtime {
	if r.err != nil {
		return r
	}
	r.shell = shell.NewDefaultShell(r.injector)
	r.injector.Register("shell", r.shell)
	return r
}

// InstallHook installs a shell hook for the specified shell type.
// It requires the shell to be loaded first via LoadShell().
func (r *Runtime) InstallHook(shellType string) *Runtime {
	if r.err != nil {
		return r
	}
	if r.shell == nil {
		r.err = fmt.Errorf("shell not loaded - call LoadShell() first")
		return r
	}
	r.err = r.shell.InstallHook(shellType)
	return r
}
