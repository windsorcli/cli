package runtime

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/cluster"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/kubernetes"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Loader Methods
// =============================================================================

// LoadShell loads the shell dependency, creating a new default shell if none exists.
func (r *Runtime) LoadShell() *Runtime {
	if r.err != nil {
		return r
	}

	if r.Shell == nil {
		if existingShell := r.Injector.Resolve("shell"); existingShell != nil {
			if shellInstance, ok := existingShell.(shell.Shell); ok {
				r.Shell = shellInstance
			} else {
				r.Shell = shell.NewDefaultShell(r.Injector)
			}
		} else {
			r.Shell = shell.NewDefaultShell(r.Injector)
		}
	}
	r.Injector.Register("shell", r.Shell)
	return r
}

// LoadConfig initializes the configuration handler dependency and loads all configuration sources
// into memory for use by the runtime. This includes creating a new ConfigHandler if one does not exist,
// initializing it with required dependencies, and loading configuration from schema defaults, the root
// windsor.yaml context section, context-specific windsor.yaml/yml files, and values.yaml. The method
// registers the ConfigHandler with the injector, and returns the Runtime instance with any error set if
// initialization or configuration loading fails. This method supersedes the previous LoadConfigHandler by
// combining handler instantiation, initialization, and configuration loading in one step.
func (r *Runtime) LoadConfig() *Runtime {
	if r.err != nil {
		return r
	}
	if r.Shell == nil {
		r.err = fmt.Errorf("shell not loaded - call LoadShell() first")
		return r
	}

	if r.ConfigHandler == nil {
		if existingConfigHandler := r.Injector.Resolve("configHandler"); existingConfigHandler != nil {
			if configHandlerInstance, ok := existingConfigHandler.(config.ConfigHandler); ok {
				r.ConfigHandler = configHandlerInstance
			} else {
				r.ConfigHandler = config.NewConfigHandler(r.Injector)
			}
		} else {
			r.ConfigHandler = config.NewConfigHandler(r.Injector)
		}
	}
	r.Injector.Register("configHandler", r.ConfigHandler)
	if err := r.ConfigHandler.Initialize(); err != nil {
		r.err = fmt.Errorf("failed to initialize config handler: %w", err)
		return r
	}
	if err := r.ConfigHandler.LoadConfig(); err != nil {
		r.err = fmt.Errorf("failed to load configuration: %w", err)
		return r
	}
	return r
}

// LoadSecretsProviders loads and initializes the secrets providers using configuration and environment.
// It detects SOPS and 1Password vaults as in BasePipeline.withSecretsProviders.
func (r *Runtime) LoadSecretsProviders() *Runtime {
	if r.err != nil {
		return r
	}
	if r.ConfigHandler == nil {
		r.err = fmt.Errorf("config handler not loaded - call LoadConfig() first")
		return r
	}

	configRoot, err := r.ConfigHandler.GetConfigRoot()
	if err != nil {
		r.err = fmt.Errorf("error getting config root: %w", err)
		return r
	}

	secretsFilePaths := []string{"secrets.enc.yaml", "secrets.enc.yml"}
	for _, filePath := range secretsFilePaths {
		if _, err := r.Shims.Stat(filepath.Join(configRoot, filePath)); err == nil {
			if r.SecretsProviders.Sops == nil {
				r.SecretsProviders.Sops = secrets.NewSopsSecretsProvider(configRoot, r.Injector)
				r.Injector.Register("sopsSecretsProvider", r.SecretsProviders.Sops)
			}
			break
		}
	}

	vaults, ok := r.ConfigHandler.Get("secrets.onepassword.vaults").(map[string]secretsConfigType.OnePasswordVault)
	if ok && len(vaults) > 0 {
		useSDK := r.Shims.Getenv("OP_SERVICE_ACCOUNT_TOKEN") != ""

		for key, vault := range vaults {
			vaultCopy := vault
			vaultCopy.ID = key

			if r.SecretsProviders.Onepassword == nil {
				if useSDK {
					r.SecretsProviders.Onepassword = secrets.NewOnePasswordSDKSecretsProvider(vaultCopy, r.Injector)
				} else {
					r.SecretsProviders.Onepassword = secrets.NewOnePasswordCLISecretsProvider(vaultCopy, r.Injector)
				}
				r.Injector.Register("onePasswordSecretsProvider", r.SecretsProviders.Onepassword)
				break
			}
		}
	}

	return r
}

// LoadKubernetes loads and initializes Kubernetes and cluster client dependencies.
func (r *Runtime) LoadKubernetes() *Runtime {
	if r.err != nil {
		return r
	}
	if r.ConfigHandler == nil {
		r.err = fmt.Errorf("config handler not loaded - call LoadConfig() first")
		return r
	}

	driver := r.ConfigHandler.GetString("cluster.driver")
	if driver != "" && driver != "talos" {
		r.err = fmt.Errorf("unsupported cluster driver: %s", driver)
		return r
	}

	if r.Injector.Resolve("kubernetesClient") == nil {
		kubernetesClient := kubernetes.NewDynamicKubernetesClient()
		r.Injector.Register("kubernetesClient", kubernetesClient)
	}

	if r.K8sManager == nil {
		r.K8sManager = kubernetes.NewKubernetesManager(r.Injector)
	}
	r.Injector.Register("kubernetesManager", r.K8sManager)
	if err := r.K8sManager.Initialize(); err != nil {
		r.err = fmt.Errorf("failed to initialize kubernetes manager: %w", err)
		return r
	}

	if driver == "talos" {
		if r.ClusterClient == nil {
			r.ClusterClient = cluster.NewTalosClusterClient(r.Injector)
			r.Injector.Register("clusterClient", r.ClusterClient)
		}
	}

	return r
}

// LoadBlueprint initializes and configures all runtime dependencies necessary for blueprint processing.
// It creates and registers the blueprint handler and artifact builder if they do not already exist,
// then initializes each component to provide template processing, OCI artifact loading, and blueprint
// data management. All dependencies are injected and registered as needed. If any error occurs during
// initialization, the error is set in the runtime and the method returns. Returns the Runtime instance
// with updated dependencies and error state.
func (r *Runtime) LoadBlueprint() *Runtime {
	if r.err != nil {
		return r
	}
	if r.ConfigHandler == nil {
		r.err = fmt.Errorf("config handler not loaded - call LoadConfig() first")
		return r
	}
	if r.BlueprintHandler == nil {
		r.BlueprintHandler = blueprint.NewBlueprintHandler(r.Injector)
		r.Injector.Register("blueprintHandler", r.BlueprintHandler)
	}
	if r.ArtifactBuilder == nil {
		r.ArtifactBuilder = artifact.NewArtifactBuilder()
		r.Injector.Register("artifactBuilder", r.ArtifactBuilder)
	}
	if err := r.BlueprintHandler.Initialize(); err != nil {
		r.err = fmt.Errorf("failed to initialize blueprint handler: %w", err)
		return r
	}
	if err := r.ArtifactBuilder.Initialize(r.Injector); err != nil {
		r.err = fmt.Errorf("failed to initialize artifact builder: %w", err)
		return r
	}
	if err := r.BlueprintHandler.LoadBlueprint(); err != nil {
		r.err = fmt.Errorf("failed to load blueprint data: %w", err)
		return r
	}
	return r
}

// LoadEnvVars loads environment variables and injects them into the process environment.
// It initializes secret providers if Decrypt is true, and aggregates environment variables
// and aliases from all enabled environment printers. Environment variables are injected
// into the current process, and aliases are collected for later use. The Verbose flag controls
// whether secret loading errors are reported. Returns the Runtime instance with the error
// state updated if any failure occurs during processing.
func (r *Runtime) LoadEnvVars(opts EnvVarsOptions) *Runtime {
	if r.err != nil {
		return r
	}
	if r.ConfigHandler == nil {
		r.err = fmt.Errorf("config handler not loaded - call LoadConfig() first")
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

	for _, printer := range r.getAllEnvPrinters() {
		if printer != nil {
			if err := printer.Initialize(); err != nil {
				r.err = fmt.Errorf("failed to initialize env printer: %w", err)
				return r
			}
		}
	}

	if opts.Decrypt && (r.SecretsProviders.Sops != nil || r.SecretsProviders.Onepassword != nil) {
		providers := []secrets.SecretsProvider{
			r.SecretsProviders.Sops,
			r.SecretsProviders.Onepassword,
		}
		for _, provider := range providers {
			if provider != nil {
				if err := provider.LoadSecrets(); err != nil {
					if opts.Verbose {
						r.err = fmt.Errorf("failed to load secrets: %w", err)
						return r
					}
					return r
				}
			}
		}
	}

	allEnvVars := make(map[string]string)
	allAliases := make(map[string]string)

	printers := r.getAllEnvPrinters()

	for _, printer := range printers {
		if printer != nil {
			envVars, err := printer.GetEnvVars()
			if err != nil {
				r.err = fmt.Errorf("error getting environment variables: %w", err)
				return r
			}
			maps.Copy(allEnvVars, envVars)

			aliases, err := printer.GetAlias()
			if err != nil {
				r.err = fmt.Errorf("error getting aliases: %w", err)
				return r
			}
			maps.Copy(allAliases, aliases)
		}
	}

	r.EnvVars = allEnvVars

	for key, value := range allEnvVars {
		if err := os.Setenv(key, value); err != nil {
			r.err = fmt.Errorf("error setting environment variable %s: %w", key, err)
			return r
		}
	}

	r.EnvAliases = allAliases

	return r
}
