// The AzureEnvPrinter is a specialized component that manages Azure environment configuration.
// It provides Azure-specific environment variable management and configuration,
// The AzureEnvPrinter handles Azure configuration settings and environment setup,
// ensuring proper Azure CLI integration and environment setup for operations.

package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Types
// =============================================================================

// AzureEnvPrinter is a struct that implements Azure environment configuration
type AzureEnvPrinter struct {
	BaseEnvPrinter
}

// =============================================================================
// Constructor
// =============================================================================

// NewAzureEnvPrinter creates a new AzureEnvPrinter instance
func NewAzureEnvPrinter(shell shell.Shell, configHandler config.ConfigHandler) *AzureEnvPrinter {
	if shell == nil {
		panic("shell is required")
	}
	if configHandler == nil {
		panic("config handler is required")
	}

	return &AzureEnvPrinter{
		BaseEnvPrinter: *NewBaseEnvPrinter(shell, configHandler),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetEnvVars retrieves the environment variables for the Azure environment.
// In project mode AZURE_CONFIG_DIR always points at the context's .azure dir,
// matching how the AWS env printer scopes AWS_CONFIG_FILE — keeps `az login`
// from contaminating the operator's global ~/.azure. In global mode it is not
// emitted; ARM_SUBSCRIPTION_ID / ARM_TENANT_ID / ARM_ENVIRONMENT are emitted in
// both modes because they describe which account/tenant the context targets.
func (e *AzureEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)
	global := e.shell.IsGlobal()

	if !global {
		configRoot, err := e.configHandler.GetConfigRoot()
		if err != nil {
			return nil, fmt.Errorf("error retrieving configuration root directory: %w", err)
		}
		azureConfigDir := filepath.Join(configRoot, ".azure")
		envVars["AZURE_CONFIG_DIR"] = filepath.ToSlash(azureConfigDir)
		envVars["AZURE_CORE_LOGIN_EXPERIENCE_V2"] = "false"
	}

	config := e.configHandler.GetConfig()
	if config != nil && config.Azure != nil {
		if config.Azure.SubscriptionID != nil {
			envVars["ARM_SUBSCRIPTION_ID"] = *config.Azure.SubscriptionID
		}
		if config.Azure.TenantID != nil {
			envVars["ARM_TENANT_ID"] = *config.Azure.TenantID
		}
		if config.Azure.Environment != nil {
			envVars["ARM_ENVIRONMENT"] = *config.Azure.Environment
		}
	}

	envVars["TF_VAR_kubelogin_mode"] = e.resolveKubeloginMode(config)

	return envVars, nil
}

// resolveKubeloginMode returns the kubelogin login mode for the AKS
// kubeconfig, in precedence order: azure.kubelogin_mode (operator override,
// the only path that handles managed-identity since MI has no env signal) →
// AZURE_FEDERATED_TOKEN_FILE → workloadidentity → AZURE_CLIENT_SECRET /
// AZURE_CLIENT_CERTIFICATE_PATH → spn → otherwise → azurecli.
func (e *AzureEnvPrinter) resolveKubeloginMode(config *v1alpha1.Context) string {
	if config != nil && config.Azure != nil && config.Azure.KubeloginMode != nil {
		if v := *config.Azure.KubeloginMode; v != "" {
			return v
		}
	}
	if os.Getenv("AZURE_FEDERATED_TOKEN_FILE") != "" {
		return "workloadidentity"
	}
	if os.Getenv("AZURE_CLIENT_SECRET") != "" || os.Getenv("AZURE_CLIENT_CERTIFICATE_PATH") != "" {
		return "spn"
	}
	return "azurecli"
}
