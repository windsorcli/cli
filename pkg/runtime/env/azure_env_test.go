package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/azure"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupAzureEnvMocks(t *testing.T, overrides ...*EnvTestMocks) *EnvTestMocks {
	t.Helper()
	mocks := setupEnvMocks(t, overrides...)

	// Only load default config if ConfigHandler wasn't overridden
	// If ConfigHandler was injected via overrides, assume test wants to control it
	if len(overrides) == 0 || overrides[0] == nil || overrides[0].ConfigHandler == nil {
		// Set the context environment variable first, before loading config
		os.Setenv("WINDSOR_CONTEXT", "test-context")

		configStr := `
version: v1alpha1
contexts:
  test-context:
    azure:
      subscription_id: "test-subscription"
      tenant_id: "test-tenant"
      environment: "test-environment"
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		mocks.ConfigHandler.SetContext("test-context")
	}

	configRoot, _ := mocks.ConfigHandler.GetConfigRoot()
	azureConfigDir := filepath.Join(configRoot, ".azure")
	mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
		if name == azureConfigDir {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestAzureEnv_GetEnvVars(t *testing.T) {
	setup := func(t *testing.T, overrides ...*EnvTestMocks) (*AzureEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupAzureEnvMocks(t, overrides...)
		printer := NewAzureEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.shims = mocks.Shims
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a printer with Azure configuration
		printer, mocks := setup(t)
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("Failed to get config root: %v", err)
		}

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Then the environment variables should match expected values
		expectedEnvVars := map[string]string{
			"AZURE_CONFIG_DIR":               filepath.ToSlash(filepath.Join(configRoot, ".azure")),
			"AZURE_CORE_LOGIN_EXPERIENCE_V2": "false",
			"ARM_SUBSCRIPTION_ID":            "test-subscription",
			"ARM_TENANT_ID":                  "test-tenant",
			"ARM_ENVIRONMENT":                "test-environment",
		}
		if envVars["AZURE_CORE_LOGIN_EXPERIENCE_V2"] != expectedEnvVars["AZURE_CORE_LOGIN_EXPERIENCE_V2"] {
			t.Errorf("GetEnvVars returned AZURE_CORE_LOGIN_EXPERIENCE_V2=%v, want %v", envVars["AZURE_CORE_LOGIN_EXPERIENCE_V2"], expectedEnvVars["AZURE_CORE_LOGIN_EXPERIENCE_V2"])
		}
		if envVars["ARM_SUBSCRIPTION_ID"] != expectedEnvVars["ARM_SUBSCRIPTION_ID"] {
			t.Errorf("GetEnvVars returned ARM_SUBSCRIPTION_ID=%v, want %v", envVars["ARM_SUBSCRIPTION_ID"], expectedEnvVars["ARM_SUBSCRIPTION_ID"])
		}
		if envVars["ARM_TENANT_ID"] != expectedEnvVars["ARM_TENANT_ID"] {
			t.Errorf("GetEnvVars returned ARM_TENANT_ID=%v, want %v", envVars["ARM_TENANT_ID"], expectedEnvVars["ARM_TENANT_ID"])
		}
		if envVars["ARM_ENVIRONMENT"] != expectedEnvVars["ARM_ENVIRONMENT"] {
			t.Errorf("GetEnvVars returned ARM_ENVIRONMENT=%v, want %v", envVars["ARM_ENVIRONMENT"], expectedEnvVars["ARM_ENVIRONMENT"])
		}
		if !strings.HasSuffix(envVars["AZURE_CONFIG_DIR"], filepath.ToSlash("/.azure")) {
			t.Errorf("GetEnvVars returned AZURE_CONFIG_DIR=%v, want path ending with /.azure", envVars["AZURE_CONFIG_DIR"])
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		// Given a printer with a config handler that fails to get config root
		// AND an azure block in context config (so the config-root resolution path
		// is exercised — without azure config, GetEnvVars short-circuits before
		// touching the config root).
		subID := "test-subscription"
		mockConfigHandler := &config.MockConfigHandler{}
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("error retrieving configuration root directory")
		}
		mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{Azure: &azure.AzureConfig{SubscriptionID: &subID}}
		}
		mocks := setupAzureEnvMocks(t, &EnvTestMocks{
			ConfigHandler: mockConfigHandler,
		})
		printer := NewAzureEnvPrinter(mocks.Shell, mocks.ConfigHandler)

		// When GetEnvVars is called
		_, err := printer.GetEnvVars()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving configuration root directory") {
			t.Errorf("Expected error containing 'error retrieving configuration root directory', got %v", err)
		}
	})

	t.Run("GlobalModeDefersToAmbientAzureConfig", func(t *testing.T) {
		// Given an Azure-platform context running in global mode
		printer, mocks := setup(t)
		mocks.Shell.IsGlobalFunc = func() bool { return true }

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then AZURE_CONFIG_DIR and AZURE_CORE_LOGIN_EXPERIENCE_V2 are NOT emitted —
		// the az CLI defers to the operator's ambient ~/.azure config. The project-
		// level identifiers (subscription, tenant, environment) still flow through
		// because they describe which Azure account the context targets, not whose
		// credentials are used.
		if _, ok := envVars["AZURE_CONFIG_DIR"]; ok {
			t.Errorf("AZURE_CONFIG_DIR should not be set in global mode, got %q", envVars["AZURE_CONFIG_DIR"])
		}
		if _, ok := envVars["AZURE_CORE_LOGIN_EXPERIENCE_V2"]; ok {
			t.Errorf("AZURE_CORE_LOGIN_EXPERIENCE_V2 should not be set in global mode")
		}
		if got := envVars["ARM_SUBSCRIPTION_ID"]; got != "test-subscription" {
			t.Errorf("ARM_SUBSCRIPTION_ID = %q, want %q", got, "test-subscription")
		}
		if got := envVars["ARM_TENANT_ID"]; got != "test-tenant" {
			t.Errorf("ARM_TENANT_ID = %q, want %q", got, "test-tenant")
		}
	})

	t.Run("MissingAzureConfigBlockStillScopesConfigDirInProjectMode", func(t *testing.T) {
		// Given a project-mode context with platform: azure and no azure: block
		baseMocks := setupEnvMocks(t)
		mocks := setupAzureEnvMocks(t, &EnvTestMocks{
			ConfigHandler: config.NewConfigHandler(baseMocks.Shell),
		})
		configStr := `
version: v1alpha1
contexts:
  test-context: {}
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		mocks.ConfigHandler.SetContext("test-context")
		printer := NewAzureEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.shims = mocks.Shims

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()

		// Then AZURE_CONFIG_DIR is scoped to the context anyway, and no ARM_* vars
		// are emitted since the azure: block isn't there to source them.
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		configRoot, _ := mocks.ConfigHandler.GetConfigRoot()
		want := filepath.ToSlash(filepath.Join(configRoot, ".azure"))
		if got := envVars["AZURE_CONFIG_DIR"]; got != want {
			t.Errorf("AZURE_CONFIG_DIR = %q, want %q (must scope to context even without azure: block)", got, want)
		}
		if got := envVars["AZURE_CORE_LOGIN_EXPERIENCE_V2"]; got != "false" {
			t.Errorf("AZURE_CORE_LOGIN_EXPERIENCE_V2 = %q, want %q", got, "false")
		}
		for _, k := range []string{"ARM_SUBSCRIPTION_ID", "ARM_TENANT_ID", "ARM_ENVIRONMENT"} {
			if _, ok := envVars[k]; ok {
				t.Errorf("%s should not be set when the azure: block is absent, got %q", k, envVars[k])
			}
		}
	})
}

// TestAzureEnv_KubeloginMode pins the auto-detect chain so the same blueprint
// resolves to the right kubelogin mode across laptop, OIDC runner, SPN runner,
// and AKS pod without per-context input.
func TestAzureEnv_KubeloginMode(t *testing.T) {
	clearAmbient := func(t *testing.T) {
		t.Helper()
		t.Setenv("AZURE_FEDERATED_TOKEN_FILE", "")
		t.Setenv("AZURE_CLIENT_SECRET", "")
		t.Setenv("AZURE_CLIENT_CERTIFICATE_PATH", "")
	}

	setupPrinter := func(t *testing.T, configStr string) (*AzureEnvPrinter, config.ConfigHandler) {
		t.Helper()
		baseMocks := setupEnvMocks(t)
		mocks := setupAzureEnvMocks(t, &EnvTestMocks{
			ConfigHandler: config.NewConfigHandler(baseMocks.Shell),
		})
		// SetContext before LoadConfigString — the loader slices on the current context name.
		mocks.ConfigHandler.SetContext("test-context")
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		printer := NewAzureEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.shims = mocks.Shims
		return printer, mocks.ConfigHandler
	}

	t.Run("DefaultsToAzureCliWhenNoSignals", func(t *testing.T) {
		// Given an azure context with no SPN/WI/MI env signal — the laptop developer case
		clearAmbient(t)
		printer, _ := setupPrinter(t, `
version: v1alpha1
contexts:
  test-context:
    azure:
      tenant_id: 11111111-2222-3333-4444-555555555555
`)

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars: %v", err)
		}

		// Then TF_VAR_kubelogin_mode = azurecli (kubelogin reuses the active `az` session)
		if got := envVars["TF_VAR_kubelogin_mode"]; got != "azurecli" {
			t.Errorf("TF_VAR_kubelogin_mode = %q, want %q (default for laptop dev)", got, "azurecli")
		}
	})

	t.Run("DetectsWorkloadIdentityFromFederatedTokenFile", func(t *testing.T) {
		// Given a runner with AKS Workload Identity / OIDC env injected
		t.Setenv("AZURE_FEDERATED_TOKEN_FILE", "/var/run/secrets/azure/tokens/azure-identity-token")
		t.Setenv("AZURE_CLIENT_SECRET", "")
		t.Setenv("AZURE_CLIENT_CERTIFICATE_PATH", "")
		printer, _ := setupPrinter(t, `
version: v1alpha1
contexts:
  test-context:
    azure:
      tenant_id: 11111111-2222-3333-4444-555555555555
`)

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars: %v", err)
		}

		// Then TF_VAR_kubelogin_mode = workloadidentity (matches azurerm provider's own choice)
		if got := envVars["TF_VAR_kubelogin_mode"]; got != "workloadidentity" {
			t.Errorf("TF_VAR_kubelogin_mode = %q, want %q under AZURE_FEDERATED_TOKEN_FILE", got, "workloadidentity")
		}
	})

	t.Run("DetectsSpnFromClientSecret", func(t *testing.T) {
		// Given a CI runner with SPN secret env
		t.Setenv("AZURE_FEDERATED_TOKEN_FILE", "")
		t.Setenv("AZURE_CLIENT_SECRET", "spn-secret-value")
		t.Setenv("AZURE_CLIENT_CERTIFICATE_PATH", "")
		printer, _ := setupPrinter(t, `
version: v1alpha1
contexts:
  test-context:
    azure:
      tenant_id: 11111111-2222-3333-4444-555555555555
`)

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars: %v", err)
		}

		if got := envVars["TF_VAR_kubelogin_mode"]; got != "spn" {
			t.Errorf("TF_VAR_kubelogin_mode = %q, want %q under AZURE_CLIENT_SECRET", got, "spn")
		}
	})

	t.Run("DetectsSpnFromCertificatePath", func(t *testing.T) {
		// Given a runner with SPN cert auth — same kubelogin mode as secret-based SPN
		t.Setenv("AZURE_FEDERATED_TOKEN_FILE", "")
		t.Setenv("AZURE_CLIENT_SECRET", "")
		t.Setenv("AZURE_CLIENT_CERTIFICATE_PATH", "/secrets/spn.pem")
		printer, _ := setupPrinter(t, `
version: v1alpha1
contexts:
  test-context:
    azure:
      tenant_id: 11111111-2222-3333-4444-555555555555
`)

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars: %v", err)
		}

		if got := envVars["TF_VAR_kubelogin_mode"]; got != "spn" {
			t.Errorf("TF_VAR_kubelogin_mode = %q, want %q under AZURE_CLIENT_CERTIFICATE_PATH", got, "spn")
		}
	})

	t.Run("OperatorOverrideWinsOverAutoDetect", func(t *testing.T) {
		// Given azure.kubelogin_mode=msi in values.yaml on a laptop with no env signals
		// — the only path that handles managed-identity, which has no env to detect.
		clearAmbient(t)
		printer, _ := setupPrinter(t, `
version: v1alpha1
contexts:
  test-context:
    azure:
      tenant_id: 11111111-2222-3333-4444-555555555555
      kubelogin_mode: msi
`)

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars: %v", err)
		}

		// Then the override wins (auto-detect would have returned azurecli)
		if got := envVars["TF_VAR_kubelogin_mode"]; got != "msi" {
			t.Errorf("TF_VAR_kubelogin_mode = %q, want %q (operator override)", got, "msi")
		}
	})

	t.Run("OperatorOverrideWinsEvenWithSpnEnvSet", func(t *testing.T) {
		// Given SPN env set (auto-detect would pick spn) and operator-pinned override
		t.Setenv("AZURE_CLIENT_SECRET", "spn-secret-value")
		printer, _ := setupPrinter(t, `
version: v1alpha1
contexts:
  test-context:
    azure:
      tenant_id: 11111111-2222-3333-4444-555555555555
      kubelogin_mode: workloadidentity
`)

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars: %v", err)
		}

		if got := envVars["TF_VAR_kubelogin_mode"]; got != "workloadidentity" {
			t.Errorf("TF_VAR_kubelogin_mode = %q, want %q (operator override beats env auto-detect)", got, "workloadidentity")
		}
	})

	t.Run("EmptyOverrideFallsThroughToAutoDetect", func(t *testing.T) {
		// Given azure.kubelogin_mode is set but empty — fall through to auto-detect
		clearAmbient(t)
		printer, _ := setupPrinter(t, `
version: v1alpha1
contexts:
  test-context:
    azure:
      tenant_id: 11111111-2222-3333-4444-555555555555
      kubelogin_mode: ""
`)

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars: %v", err)
		}

		if got := envVars["TF_VAR_kubelogin_mode"]; got != "azurecli" {
			t.Errorf("TF_VAR_kubelogin_mode = %q, want %q (empty override → auto-detect)", got, "azurecli")
		}
	})

	t.Run("ExportedWhenAzureBlockIsAbsent", func(t *testing.T) {
		// Given platform: azure with no azure: block — TF_VAR_kubelogin_mode must still export
		clearAmbient(t)
		printer, _ := setupPrinter(t, `
version: v1alpha1
contexts:
  test-context: {}
`)

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars: %v", err)
		}

		if got := envVars["TF_VAR_kubelogin_mode"]; got != "azurecli" {
			t.Errorf("TF_VAR_kubelogin_mode = %q, want %q (must be set even with no azure: block)", got, "azurecli")
		}
	})
}
