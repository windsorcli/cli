package runtime

import (
	"fmt"
	"path/filepath"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/env"
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

// LoadSecretsProviders loads and initializes the secrets providers using configuration and environment.
// It detects SOPS and 1Password vaults as in BasePipeline.withSecretsProviders.
func (r *Runtime) LoadSecretsProviders() *Runtime {
	if r.err != nil {
		return r
	}
	if r.ConfigHandler == nil {
		r.err = fmt.Errorf("config handler not loaded - call LoadConfigHandler() first")
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
