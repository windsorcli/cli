package env

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupAzureEnvMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()
	if opts == nil {
		opts = []*SetupOptions{{}}
	}
	if opts[0].ConfigStr == "" {
		opts[0].ConfigStr = `
version: v1alpha1
contexts:
  mock-context:
    azure:
      subscription_id: "test-subscription"
      tenant_id: "test-tenant"
      environment: "test-environment"
`
	}
	mocks := setupMocks(t, opts[0])

	// Mock stat function to make Azure config directory exist
	mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
		if name == filepath.FromSlash("/mock/config/root/.azure") {
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
	setup := func(t *testing.T, opts ...*SetupOptions) (*AzureEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupAzureEnvMocks(t, opts...)
		printer := NewAzureEnvPrinter(mocks.Injector)
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize env: %v", err)
		}
		printer.shims = mocks.Shims
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		printer, mocks := setup(t)
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("Failed to get config root: %v", err)
		}
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		expectedEnvVars := map[string]string{
			"AZURE_CONFIG_DIR":               filepath.ToSlash(filepath.Join(configRoot, ".azure")),
			"AZURE_CORE_LOGIN_EXPERIENCE_V2": "false",
			"ARM_SUBSCRIPTION_ID":            "test-subscription",
			"ARM_TENANT_ID":                  "test-tenant",
			"ARM_ENVIRONMENT":                "test-environment",
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("GetEnvVars returned %v, want %v", envVars, expectedEnvVars)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		mockConfigHandler := &config.MockConfigHandler{}
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("error retrieving configuration root directory")
		}
		mocks := setupAzureEnvMocks(t, &SetupOptions{
			ConfigHandler: mockConfigHandler,
		})
		printer := NewAzureEnvPrinter(mocks.Injector)
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize env: %v", err)
		}
		printer.shims = mocks.Shims
		_, err := printer.GetEnvVars()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving configuration root directory") {
			t.Errorf("Expected error containing 'error retrieving configuration root directory', got %v", err)
		}
	})

	t.Run("MissingConfiguration", func(t *testing.T) {
		printer, mocks := setup(t)
		if err := mocks.ConfigHandler.LoadConfigString(`
version: v1alpha1
contexts:
  mock-context: {}
`); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(envVars) != 0 {
			t.Errorf("Expected empty environment variables, got %v", envVars)
		}
	})
}

func TestAzureEnv_Print(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*AzureEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupAzureEnvMocks(t, opts...)
		printer := NewAzureEnvPrinter(mocks.Injector)
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize env: %v", err)
		}
		printer.shims = mocks.Shims
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		printer, mocks := setup(t)
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("Failed to get config root: %v", err)
		}
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string, export bool) {
			capturedEnvVars = envVars
		}
		err = printer.Print()
		if err != nil {
			t.Errorf("Print returned an error: %v", err)
		}
		expectedEnvVars := map[string]string{
			"AZURE_CONFIG_DIR":               filepath.ToSlash(filepath.Join(configRoot, ".azure")),
			"AZURE_CORE_LOGIN_EXPERIENCE_V2": "false",
			"ARM_SUBSCRIPTION_ID":            "test-subscription",
			"ARM_TENANT_ID":                  "test-tenant",
			"ARM_ENVIRONMENT":                "test-environment",
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("Print set environment variables to %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("GetEnvVarsError", func(t *testing.T) {
		mockConfigHandler := &config.MockConfigHandler{}
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("error retrieving configuration root directory")
		}
		mocks := setupAzureEnvMocks(t, &SetupOptions{
			ConfigHandler: mockConfigHandler,
		})
		printer := NewAzureEnvPrinter(mocks.Injector)
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize env: %v", err)
		}
		printer.shims = mocks.Shims
		err := printer.Print()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting environment variables") {
			t.Errorf("Expected error containing 'error getting environment variables', got %v", err)
		}
	})
}
