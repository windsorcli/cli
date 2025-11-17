package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
		mockConfigHandler := &config.MockConfigHandler{}
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("error retrieving configuration root directory")
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

	t.Run("MissingConfiguration", func(t *testing.T) {
		// Given a printer with no Azure configuration
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

		// Then no error should be returned and environment variables should be empty
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(envVars) != 0 {
			t.Errorf("Expected empty environment variables, got %v", envVars)
		}
	})
}
