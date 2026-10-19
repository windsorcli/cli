// The Azure auth check is the BaseToolsManager check for the Azure CLI and the Azure credentials.
// It provides the az binary check, the credential check, and the login hint.
// It runs the az CLI with the context AZURE_CONFIG_DIR when the env does not already point at it.
// With ambient service principal or workload identity credentials, it does not need the az CLI.

package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Private Methods
// =============================================================================

// azureEnabled reports whether the current context exercises Azure.
func (t *BaseToolsManager) azureEnabled() bool {
	platform := t.configHandler.GetString("platform")
	if platform == "azure" {
		return true
	}
	cfg := t.configHandler.GetHeader()
	return cfg != nil && cfg.Azure != nil
}

// checkAzureAuth verifies the az CLI is present and its cached credentials can reach ARM.
// Probes `az rest .../tenants` (a real ARM call) rather than `az account get-access-token`
// (a cache-only read that passes on revoked/expired refresh tokens). When ambient SDK
// credentials are set and az is absent, skips — terraform's own SDK will handle auth.
func (t *BaseToolsManager) checkAzureAuth() error {
	if hasAmbientAzureCredentials() {
		if _, err := t.shims.LookPath("az"); err != nil {
			return nil
		}
	}
	if err := t.checkAzureBinary(); err != nil {
		return err
	}
	env, err := t.azureContextEnv()
	if err != nil {
		return fmt.Errorf("cannot resolve context-scoped Azure env for credential check: %w", err)
	}
	cfg := t.configHandler.GetHeader()
	azureEnv := ""
	if cfg != nil && cfg.Azure != nil && cfg.Azure.Environment != nil {
		azureEnv = *cfg.Azure.Environment
	}
	uri := fmt.Sprintf("https://%s/tenants?api-version=2020-01-01", armTenantEndpoint(azureEnv))
	args := []string{"rest", "--method", "get", "--uri", uri, "--output", "none"}
	if _, err := t.shell.ExecSilentWithEnvAndTimeout("az", env, args, 30*time.Second); err != nil {
		return fmt.Errorf("%s", t.azureAuthHint())
	}
	return nil
}

// azureContextEnv returns env vars scoping the az CLI to the context's .azure/ dir.
// Returns nil when ambient SDK credentials are present (don't mask the native chain) or
// in global mode (defer to ~/.azure/).
func (t *BaseToolsManager) azureContextEnv() (map[string]string, error) {
	if hasAmbientAzureCredentials() {
		return nil, nil
	}
	if t.shell.IsGlobal() {
		return nil, nil
	}
	configRoot, err := t.configHandler.GetConfigRoot()
	if err != nil {
		return nil, err
	}
	azureConfigDir := filepath.Join(configRoot, ".azure")
	return map[string]string{
		"AZURE_CONFIG_DIR":               filepath.ToSlash(azureConfigDir),
		"AZURE_CORE_LOGIN_EXPERIENCE_V2": "false",
	}, nil
}

// checkAzureBinary verifies the Azure CLI is available in PATH and meets the minimum version.
func (t *BaseToolsManager) checkAzureBinary() error {
	if _, err := t.shims.LookPath("az"); err != nil {
		return missingToolError("az")
	}

	out, err := t.shell.ExecSilentWithTimeout("az", []string{"--version"}, 10*time.Second)
	if err != nil {
		return fmt.Errorf("az --version failed: %v", err)
	}
	version := extractVersion(out)
	if version == "" {
		return fmt.Errorf("failed to extract az CLI version")
	}
	if compareVersion(version, constants.MinimumVersionAzure) < 0 {
		return outdatedToolError("az", version)
	}

	return nil
}

// azureAuthHint returns the recovery command for an Azure auth failure: `az login`
// (pinned to azure.tenant_id when set), prefixed with AZURE_CONFIG_DIR in project mode
// when the current env doesn't already point at the context dir.
func (t *BaseToolsManager) azureAuthHint() string {
	tenantID := ""
	cfg := t.configHandler.GetHeader()
	if cfg != nil && cfg.Azure != nil && cfg.Azure.TenantID != nil {
		tenantID = *cfg.Azure.TenantID
	}

	tenantArg := ""
	if tenantID != "" {
		tenantArg = fmt.Sprintf(" --tenant %s", tenantID)
	}

	if t.shell.IsGlobal() {
		return fmt.Sprintf("Azure CLI session has likely expired or is not initialized. Run:\n  az login%s", tenantArg)
	}

	configRoot, err := t.configHandler.GetConfigRoot()
	if err != nil || configRoot == "" {
		return fmt.Sprintf("Azure CLI session has likely expired or is not initialized. Run:\n  az login%s", tenantArg)
	}
	azureConfigDir := filepath.Join(configRoot, ".azure")
	prefix := ""
	if !azureEnvPointsAtContext(azureConfigDir) {
		prefix = fmt.Sprintf("AZURE_CONFIG_DIR=%q ", filepath.ToSlash(azureConfigDir))
	}
	return fmt.Sprintf("Azure CLI session for context %q has likely expired or is not initialized. Run:\n  %saz login%s",
		t.configHandler.GetContext(), prefix, tenantArg)
}

// =============================================================================
// Helpers
// =============================================================================

// armTenantEndpoint maps an azure.environment value to its ARM host. Sovereign clouds
// (usgovernment, china, german) have separate endpoints; az CLI does not rewrite explicit
// --uri values, so the host must be selected up-front. Unknown/empty values default to public.
func armTenantEndpoint(env string) string {
	switch env {
	case "usgovernment":
		return "management.usgovcloudapi.net"
	case "china":
		return "management.chinacloudapi.cn"
	case "german":
		return "management.microsoftazure.de"
	default:
		return "management.azure.com"
	}
}

// hasAmbientAzureCredentials reports whether the parent env already carries Azure
// credentials (Workload Identity / SPN secret / SPN cert). MI has no env signal and
// is intentionally not detected here — those runners fall through to the az CLI check.
func hasAmbientAzureCredentials() bool {
	if os.Getenv("AZURE_FEDERATED_TOKEN_FILE") != "" {
		return true
	}
	if os.Getenv("AZURE_CLIENT_SECRET") != "" {
		return true
	}
	if os.Getenv("AZURE_CLIENT_CERTIFICATE_PATH") != "" {
		return true
	}
	return false
}

// azureEnvPointsAtContext reports whether AZURE_CONFIG_DIR already resolves to configDir.
func azureEnvPointsAtContext(configDir string) bool {
	cd := os.Getenv("AZURE_CONFIG_DIR")
	if cd == "" {
		return false
	}
	return filepath.ToSlash(cd) == filepath.ToSlash(configDir)
}
