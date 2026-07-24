package env

import (
	"fmt"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupVsphereEnvMocks(t *testing.T, overrides ...*EnvTestMocks) *EnvTestMocks {
	t.Helper()

	mocks := setupEnvMocks(t, overrides...)

	if _, ok := mocks.ConfigHandler.(*config.MockConfigHandler); !ok {
		mocks.ConfigHandler = config.NewMockConfigHandler()
	}

	mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)

	loadedConfigs := make(map[string]*v1alpha1.Context)
	currentContext := "test-context"

	mockConfigHandler.GetContextFunc = func() string {
		return currentContext
	}

	mockConfigHandler.SetContextFunc = func(context string) error {
		currentContext = context
		return nil
	}

	mockConfigHandler.LoadConfigStringFunc = func(content string) error {
		var cfg v1alpha1.Config
		if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
			return err
		}
		for name, ctx := range cfg.Contexts {
			if ctx != nil {
				ctxCopy := *ctx
				loadedConfigs[name] = &ctxCopy
			} else {
				loadedConfigs[name] = &v1alpha1.Context{}
			}
		}
		return nil
	}

	mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
		if ctx, ok := loadedConfigs[currentContext]; ok {
			return ctx
		}
		return &v1alpha1.Context{}
	}

	if len(overrides) == 0 || overrides[0] == nil || overrides[0].ConfigHandler == nil {
		defaultConfigStr := `
version: v1alpha1
contexts:
  test-context:
    vsphere:
      server: "vcenter.example.com"
      user: "administrator@vsphere.local"
      datacenter: "DC0"
      cluster: "cluster-01"
      datastore: "datastore1"
      network: "VM Network"
      resource_pool: "Resources"
      folder: "/DC0/vm"
      insecure: true
`
		if err := mocks.ConfigHandler.LoadConfigString(defaultConfigStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
	}

	if err := mocks.ConfigHandler.SetContext("test-context"); err != nil {
		t.Fatalf("Failed to set context: %v", err)
	}

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestVsphereEnv_GetEnvVars(t *testing.T) {
	setup := func(t *testing.T, overrides ...*EnvTestMocks) (*VsphereEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupVsphereEnvMocks(t, overrides...)
		printer := NewVsphereEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.shims = mocks.Shims
		return printer, mocks
	}

	t.Run("SuccessWithAllConfig", func(t *testing.T) {
		// Given a full vSphere configuration
		printer, _ := setup(t)

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()

		// Then only the three Terraform vSphere provider env vars are emitted.
		// Inventory pointers (datacenter, cluster, datastore, etc.) are Terraform
		// variable inputs wired by the facet — they are never emitted as env vars.
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if envVars["VSPHERE_SERVER"] != "vcenter.example.com" {
			t.Errorf("VSPHERE_SERVER = %q, want %q", envVars["VSPHERE_SERVER"], "vcenter.example.com")
		}
		if envVars["VSPHERE_USER"] != "administrator@vsphere.local" {
			t.Errorf("VSPHERE_USER = %q, want %q", envVars["VSPHERE_USER"], "administrator@vsphere.local")
		}
		if envVars["VSPHERE_ALLOW_UNVERIFIED_SSL"] != "true" {
			t.Errorf("VSPHERE_ALLOW_UNVERIFIED_SSL = %q, want %q", envVars["VSPHERE_ALLOW_UNVERIFIED_SSL"], "true")
		}
		if envVars["VSPHERE_PERSIST_SESSION"] != "true" {
			t.Errorf("VSPHERE_PERSIST_SESSION = %q, want %q", envVars["VSPHERE_PERSIST_SESSION"], "true")
		}
		if !strings.HasSuffix(envVars["VSPHERE_VIM_SESSION_PATH"], "/.vsphere/sessions") {
			t.Errorf("VSPHERE_VIM_SESSION_PATH = %q, want suffix %q", envVars["VSPHERE_VIM_SESSION_PATH"], "/.vsphere/sessions")
		}
		if !strings.HasSuffix(envVars["VSPHERE_REST_SESSION_PATH"], "/.vsphere/rest_sessions") {
			t.Errorf("VSPHERE_REST_SESSION_PATH = %q, want suffix %q", envVars["VSPHERE_REST_SESSION_PATH"], "/.vsphere/rest_sessions")
		}
		if len(envVars) != 6 {
			t.Errorf("Expected exactly 6 env vars (3 provider credentials + 3 session vars), got %d: %v", len(envVars), envVars)
		}
	})

	t.Run("SuccessWithMinimalConfig", func(t *testing.T) {
		// Given a minimal vSphere configuration (server only)
		mocks := setupVsphereEnvMocks(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    vsphere:
      server: "vcenter.example.com"
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		printer := NewVsphereEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.shims = mocks.Shims

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()

		// Then only VSPHERE_SERVER is set
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if envVars["VSPHERE_SERVER"] != "vcenter.example.com" {
			t.Errorf("VSPHERE_SERVER = %q, want vcenter.example.com", envVars["VSPHERE_SERVER"])
		}
		if _, ok := envVars["VSPHERE_USER"]; ok {
			t.Error("VSPHERE_USER should not be set when user is not configured")
		}
		if _, ok := envVars["VSPHERE_ALLOW_UNVERIFIED_SSL"]; ok {
			t.Error("VSPHERE_ALLOW_UNVERIFIED_SSL should not be set when insecure is not configured")
		}
		if len(envVars) != 4 {
			t.Errorf("Expected 4 environment variables (server + 3 session vars), got %d: %v", len(envVars), envVars)
		}
	})

	t.Run("MissingVsphereConfigBlockStillScopesSessionPathsInProjectMode", func(t *testing.T) {
		// Given a context with no vsphere block
		mocks := setupVsphereEnvMocks(t)
		configStr := `
version: v1alpha1
contexts:
  test-context: {}
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		printer := NewVsphereEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.shims = mocks.Shims

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()

		// Then the three session vars are still emitted in project mode, and
		// no provider-credential vars are emitted since there is no config
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if envVars["VSPHERE_PERSIST_SESSION"] != "true" {
			t.Errorf("VSPHERE_PERSIST_SESSION = %q, want %q", envVars["VSPHERE_PERSIST_SESSION"], "true")
		}
		if !strings.HasSuffix(envVars["VSPHERE_VIM_SESSION_PATH"], "/.vsphere/sessions") {
			t.Errorf("VSPHERE_VIM_SESSION_PATH = %q, want suffix %q", envVars["VSPHERE_VIM_SESSION_PATH"], "/.vsphere/sessions")
		}
		if !strings.HasSuffix(envVars["VSPHERE_REST_SESSION_PATH"], "/.vsphere/rest_sessions") {
			t.Errorf("VSPHERE_REST_SESSION_PATH = %q, want suffix %q", envVars["VSPHERE_REST_SESSION_PATH"], "/.vsphere/rest_sessions")
		}
		if len(envVars) != 3 {
			t.Errorf("Expected exactly 3 env vars (session vars only), got %d: %v", len(envVars), envVars)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		// Given a config handler that fails to resolve the config root
		mocks := setupVsphereEnvMocks(t)
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("boom")
		}
		printer := NewVsphereEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.shims = mocks.Shims

		// When GetEnvVars is called
		_, err := printer.GetEnvVars()

		// Then an error is returned naming the config root failure
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving configuration root directory") {
			t.Errorf("Expected error to mention configuration root directory, got %v", err)
		}
	})

	t.Run("GlobalModeOmitsSessionVars", func(t *testing.T) {
		// Given a full vSphere configuration in global mode
		printer, mocks := setup(t)
		mocks.Shell.IsGlobalFunc = func() bool { return true }

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()

		// Then session vars are omitted but provider-credential vars remain
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		for _, key := range []string{"VSPHERE_PERSIST_SESSION", "VSPHERE_VIM_SESSION_PATH", "VSPHERE_REST_SESSION_PATH"} {
			if _, ok := envVars[key]; ok {
				t.Errorf("%s should not be set in global mode", key)
			}
		}
		if envVars["VSPHERE_SERVER"] != "vcenter.example.com" {
			t.Errorf("VSPHERE_SERVER = %q, want %q", envVars["VSPHERE_SERVER"], "vcenter.example.com")
		}
	})

	t.Run("InsecureFalseEmitted", func(t *testing.T) {
		// Given a config with insecure explicitly set to false
		mocks := setupVsphereEnvMocks(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    vsphere:
      server: "vcenter.example.com"
      insecure: false
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		printer := NewVsphereEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.shims = mocks.Shims

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()

		// Then VSPHERE_ALLOW_UNVERIFIED_SSL is "false" (explicit, not omitted)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if envVars["VSPHERE_ALLOW_UNVERIFIED_SSL"] != "false" {
			t.Errorf("VSPHERE_ALLOW_UNVERIFIED_SSL = %q, want %q", envVars["VSPHERE_ALLOW_UNVERIFIED_SSL"], "false")
		}
	})
}
