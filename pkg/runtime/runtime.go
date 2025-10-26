package runtime

import (
	"fmt"
	"maps"
	"os"

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
	"github.com/windsorcli/cli/pkg/shell/ssh"
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

// EnvVarsOptions contains options for environment variable operations.
type EnvVarsOptions struct {
	Decrypt    bool         // Whether to decrypt secrets
	Verbose    bool         // Whether to show verbose error output
	Export     bool         // Whether to use export format (export KEY=value vs KEY=value)
	OutputFunc func(string) // Callback function for handling output
}

// Runtime encapsulates all core Windsor CLI runtime dependencies.
type Runtime struct {
	Dependencies
	Shims      *Shims
	EnvVars    map[string]string
	EnvAliases map[string]string
	err        error
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

// HandleSessionReset resets managed environment variables if needed before loading new ones.
// It checks for reset flags, session tokens, and context changes. Errors are recorded in r.err.
func (r *Runtime) HandleSessionReset() *Runtime {
	if r.err != nil {
		return r
	}
	if r.Shell == nil {
		r.err = fmt.Errorf("shell not loaded - call LoadShell() first")
		return r
	}

	hasSessionToken := os.Getenv("WINDSOR_SESSION_TOKEN") != ""
	shouldReset, err := r.Shell.CheckResetFlags()
	if err != nil {
		r.err = fmt.Errorf("failed to check reset flags: %w", err)
		return r
	}
	if !hasSessionToken {
		shouldReset = true
	}

	if shouldReset {
		r.Shell.Reset()
		if err := os.Setenv("NO_CACHE", "true"); err != nil {
			r.err = fmt.Errorf("failed to set NO_CACHE: %w", err)
			return r
		}
	}

	return r
}

// PrintEnvVars renders and prints the environment variables that were previously collected
// and stored in r.EnvVars using the shell's RenderEnvVars method. The EnvVarsOptions parameter
// controls export formatting and provides an output callback. This method should be called
// after LoadEnvVars to print the collected environment variables.
func (r *Runtime) PrintEnvVars(opts EnvVarsOptions) *Runtime {
	if r.err != nil {
		return r
	}

	if len(r.EnvVars) > 0 {
		output := r.Shell.RenderEnvVars(r.EnvVars, opts.Export)
		opts.OutputFunc(output)
	}

	return r
}

// PrintAliases prints all collected aliases using the shell's RenderAliases method.
// The outputFunc callback is invoked with the rendered aliases string output.
// If any error occurs during alias retrieval, the Runtime error state is updated
// and the original instance is returned unmodified.
func (r *Runtime) PrintAliases(outputFunc func(string)) *Runtime {
	if r.err != nil {
		return r
	}

	allAliases := make(map[string]string)
	for _, envPrinter := range r.getAllEnvPrinters() {
		aliases, err := envPrinter.GetAlias()
		if err != nil {
			r.err = fmt.Errorf("error getting aliases: %w", err)
			return r
		}
		maps.Copy(allAliases, aliases)
	}

	if len(allAliases) > 0 {
		output := r.Shell.RenderAliases(allAliases)
		outputFunc(output)
	}

	return r
}

// ExecutePostEnvHook executes post-environment hooks for all environment printers.
// The Verbose flag controls whether errors are reported. Returns the Runtime instance
// with error state updated if any step fails.
func (r *Runtime) ExecutePostEnvHook(verbose bool) *Runtime {
	if r.err != nil {
		return r
	}

	var firstError error

	printers := r.getAllEnvPrinters()

	for _, printer := range printers {
		if printer != nil {
			if err := printer.PostEnvHook(); err != nil && firstError == nil {
				firstError = err
			}
		}
	}

	if firstError != nil && verbose {
		r.err = fmt.Errorf("failed to execute post env hooks: %w", firstError)
		return r
	}

	return r
}

// CheckTrustedDirectory checks if the current directory is trusted using the shell's
// CheckTrustedDirectory method. Returns the Runtime instance with updated error state.
func (r *Runtime) CheckTrustedDirectory() *Runtime {
	if r.err != nil {
		return r
	}
	if r.Shell == nil {
		r.err = fmt.Errorf("shell not loaded - call LoadShell() first")
		return r
	}

	if err := r.Shell.CheckTrustedDirectory(); err != nil {
		r.err = fmt.Errorf("not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve")
		return r
	}

	return r
}

// ArtifactOptions contains options for artifact operations (bundle or push).
type ArtifactOptions struct {
	// Bundle options
	OutputPath string // Output path for bundle (file or directory)

	// Push options
	RegistryBase string // Registry base URL (e.g., "ghcr.io")
	RepoName     string // Repository name

	// Common options
	Tag        string       // Tag/version (overrides metadata.yaml)
	OutputFunc func(string) // Callback for success output
}

// ProcessArtifacts builds and processes artifacts (bundle or push) from the project's templates,
// kustomize, and terraform files. It loads blueprint and artifact handlers, bundles all files,
// and either archives to a file or pushes to a registry based on ArtifactOptions. Supports both
// bundle and push operations. If any step fails, the returned Runtime has an updated error state;
// otherwise, returns the current instance.
func (r *Runtime) ProcessArtifacts(opts ArtifactOptions) *Runtime {
	if r.err != nil {
		return r
	}
	if r.Shell == nil {
		r.err = fmt.Errorf("shell not loaded - call LoadShell() first")
		return r
	}

	if r.ArtifactBuilder == nil {
		if existingArtifactBuilder := r.Injector.Resolve("artifactBuilder"); existingArtifactBuilder != nil {
			if artifactBuilderInstance, ok := existingArtifactBuilder.(artifact.Artifact); ok {
				r.ArtifactBuilder = artifactBuilderInstance
			} else {
				r.ArtifactBuilder = artifact.NewArtifactBuilder()
				r.Injector.Register("artifactBuilder", r.ArtifactBuilder)
			}
		} else {
			r.ArtifactBuilder = artifact.NewArtifactBuilder()
			r.Injector.Register("artifactBuilder", r.ArtifactBuilder)
		}
		if err := r.ArtifactBuilder.Initialize(r.Injector); err != nil {
			r.err = fmt.Errorf("failed to initialize artifact builder: %w", err)
			return r
		}
	}

	if opts.RegistryBase != "" && opts.RepoName != "" {
		if err := r.ArtifactBuilder.Bundle(); err != nil {
			r.err = fmt.Errorf("failed to bundle artifacts: %w", err)
			return r
		}

		if err := r.ArtifactBuilder.Push(opts.RegistryBase, opts.RepoName, opts.Tag); err != nil {
			r.err = fmt.Errorf("failed to push artifact: %w", err)
			return r
		}
		registryURL := fmt.Sprintf("%s/%s", opts.RegistryBase, opts.RepoName)
		if opts.Tag != "" {
			registryURL = fmt.Sprintf("%s:%s", registryURL, opts.Tag)
		}
		if opts.OutputFunc != nil {
			opts.OutputFunc(registryURL)
		}
	} else {
		actualOutputPath, err := r.ArtifactBuilder.Write(opts.OutputPath, opts.Tag)
		if err != nil {
			r.err = fmt.Errorf("failed to bundle and create artifact: %w", err)
			return r
		}
		if opts.OutputFunc != nil {
			opts.OutputFunc(actualOutputPath)
		}
	}

	return r
}

// =============================================================================
// Private Methods
// =============================================================================

// getAllEnvPrinters returns all environment printers in field order, ensuring WindsorEnv is last.
// This method provides compile-time structure assertions by mirroring the struct layout definition.
// Panics at runtime if WindsorEnv is not last to guarantee environment variable precedence.
func (r *Runtime) getAllEnvPrinters() []env.EnvPrinter {
	const expectedPrinterCount = 7
	_ = [expectedPrinterCount]struct{}{}

	allPrinters := []env.EnvPrinter{
		r.EnvPrinters.AwsEnv,
		r.EnvPrinters.AzureEnv,
		r.EnvPrinters.DockerEnv,
		r.EnvPrinters.KubeEnv,
		r.EnvPrinters.TalosEnv,
		r.EnvPrinters.TerraformEnv,
		r.EnvPrinters.WindsorEnv,
	}

	var printers []env.EnvPrinter
	for _, printer := range allPrinters {
		if printer != nil {
			printers = append(printers, printer)
		}
	}

	if len(printers) > 0 && printers[len(printers)-1] != r.EnvPrinters.WindsorEnv {
		panic("WindsorEnv must be the last printer in the list")
	}

	return printers
}
