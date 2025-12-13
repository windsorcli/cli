package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupGcpEnvMocks(t *testing.T, overrides ...*EnvTestMocks) *EnvTestMocks {
	t.Helper()

	mocks := setupEnvMocks(t, overrides...)

	if _, ok := mocks.ConfigHandler.(*config.MockConfigHandler); !ok {
		mocks.ConfigHandler = config.NewMockConfigHandler()
	}

	mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)

	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return "/mock/config/root", nil
	}

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
    gcp:
      enabled: true
      project_id: "test-project"
      quota_project: "billing-project"
`

		if err := mocks.ConfigHandler.LoadConfigString(defaultConfigStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
	}

	if err := mocks.ConfigHandler.SetContext("test-context"); err != nil {
		t.Fatalf("Failed to set context: %v", err)
	}

	mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}

	mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestGcpEnv_GetEnvVars(t *testing.T) {
	setup := func(t *testing.T, overrides ...*EnvTestMocks) (*GcpEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupGcpEnvMocks(t, overrides...)
		printer := NewGcpEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.shims = mocks.Shims
		return printer, mocks
	}

	t.Run("SuccessWithAllConfig", func(t *testing.T) {
		printer, mocks := setup(t)
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("Failed to get config root: %v", err)
		}

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		expectedGcloudConfigDir := filepath.ToSlash(filepath.Join(configRoot, ".gcp", "gcloud"))
		if envVars["CLOUDSDK_CONFIG"] != expectedGcloudConfigDir {
			t.Errorf("GetEnvVars returned CLOUDSDK_CONFIG=%v, want %v", envVars["CLOUDSDK_CONFIG"], expectedGcloudConfigDir)
		}

		if envVars["GOOGLE_CLOUD_PROJECT"] != "test-project" {
			t.Errorf("GetEnvVars returned GOOGLE_CLOUD_PROJECT=%v, want test-project", envVars["GOOGLE_CLOUD_PROJECT"])
		}

		if envVars["GCLOUD_PROJECT"] != "test-project" {
			t.Errorf("GetEnvVars returned GCLOUD_PROJECT=%v, want test-project", envVars["GCLOUD_PROJECT"])
		}

		if envVars["GOOGLE_CLOUD_QUOTA_PROJECT"] != "billing-project" {
			t.Errorf("GetEnvVars returned GOOGLE_CLOUD_QUOTA_PROJECT=%v, want billing-project", envVars["GOOGLE_CLOUD_QUOTA_PROJECT"])
		}
	})

	t.Run("SuccessWithMinimalConfig", func(t *testing.T) {
		mocks := setupGcpEnvMocks(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    gcp:
      enabled: true
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		printer := NewGcpEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.shims = mocks.Shims

		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("Failed to get config root: %v", err)
		}

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		expectedGcloudConfigDir := filepath.ToSlash(filepath.Join(configRoot, ".gcp", "gcloud"))
		if envVars["CLOUDSDK_CONFIG"] != expectedGcloudConfigDir {
			t.Errorf("GetEnvVars returned CLOUDSDK_CONFIG=%v, want %v", envVars["CLOUDSDK_CONFIG"], expectedGcloudConfigDir)
		}

		if len(envVars) != 1 {
			t.Errorf("Expected 1 environment variable, got %d: %v", len(envVars), envVars)
		}
	})

	t.Run("CreatesDirectoryAutomatically", func(t *testing.T) {
		printer, mocks := setup(t)

		mkdirAllCalled := false
		mkdirAllPath := ""
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			mkdirAllCalled = true
			mkdirAllPath = path
			return nil
		}

		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("Failed to get config root: %v", err)
		}

		_, err = printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if !mkdirAllCalled {
			t.Error("MkdirAll was not called")
		}

		expectedPath := filepath.Join(configRoot, ".gcp", "gcloud")
		if mkdirAllPath != expectedPath {
			t.Errorf("MkdirAll called with path=%v, want %v", mkdirAllPath, expectedPath)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		mocks := setupGcpEnvMocks(t)
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("error retrieving configuration root directory")
		}
		printer := NewGcpEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.shims = mocks.Shims

		_, err := printer.GetEnvVars()

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving configuration root directory") {
			t.Errorf("Expected error containing 'error retrieving configuration root directory', got %v", err)
		}
	})

	t.Run("MkdirAllError", func(t *testing.T) {
		printer, mocks := setup(t)

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock mkdirAll error")
		}

		_, err := printer.GetEnvVars()

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error creating GCP config directory") {
			t.Errorf("Expected error containing 'error creating GCP config directory', got %v", err)
		}
	})

	t.Run("MissingConfiguration", func(t *testing.T) {
		baseMocks := setupEnvMocks(t)
		mocks := setupGcpEnvMocks(t, &EnvTestMocks{
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
		printer := NewGcpEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.shims = mocks.Shims

		envVars, err := printer.GetEnvVars()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(envVars) != 0 {
			t.Errorf("Expected empty environment variables, got %v", envVars)
		}
	})

	t.Run("CredentialsPathSet", func(t *testing.T) {
		mocks := setupGcpEnvMocks(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    gcp:
      enabled: true
      credentials_path: "/path/to/credentials.json"
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		printer := NewGcpEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.shims = mocks.Shims

		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			return "", false
		}

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if envVars["GOOGLE_APPLICATION_CREDENTIALS"] != "/path/to/credentials.json" {
			t.Errorf("GetEnvVars returned GOOGLE_APPLICATION_CREDENTIALS=%v, want /path/to/credentials.json", envVars["GOOGLE_APPLICATION_CREDENTIALS"])
		}
	})

	t.Run("ServiceAccountFileExists", func(t *testing.T) {
		mocks := setupGcpEnvMocks(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    gcp:
      enabled: true
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		printer := NewGcpEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.shims = mocks.Shims

		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("Failed to get config root: %v", err)
		}

		serviceAccountPath := filepath.Join(configRoot, ".gcp", "service-accounts", "default.json")
		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			return "", false
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == serviceAccountPath {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		expectedPath := filepath.ToSlash(serviceAccountPath)
		if envVars["GOOGLE_APPLICATION_CREDENTIALS"] != expectedPath {
			t.Errorf("GetEnvVars returned GOOGLE_APPLICATION_CREDENTIALS=%v, want %v", envVars["GOOGLE_APPLICATION_CREDENTIALS"], expectedPath)
		}
	})

	t.Run("GOOGLE_APPLICATION_CREDENTIALSAlreadySet", func(t *testing.T) {
		mocks := setupGcpEnvMocks(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    gcp:
      enabled: true
      credentials_path: "/path/to/credentials.json"
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		printer := NewGcpEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.shims = mocks.Shims

		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			if key == "GOOGLE_APPLICATION_CREDENTIALS" {
				return "/existing/credentials.json", true
			}
			return "", false
		}

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if _, exists := envVars["GOOGLE_APPLICATION_CREDENTIALS"]; exists {
			t.Error("GOOGLE_APPLICATION_CREDENTIALS should not be set when already in environment")
		}
	})

	t.Run("OnlyProjectIDSet", func(t *testing.T) {
		mocks := setupGcpEnvMocks(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    gcp:
      enabled: true
      project_id: "my-project"
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		printer := NewGcpEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.shims = mocks.Shims

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if envVars["GOOGLE_CLOUD_PROJECT"] != "my-project" {
			t.Errorf("GetEnvVars returned GOOGLE_CLOUD_PROJECT=%v, want my-project", envVars["GOOGLE_CLOUD_PROJECT"])
		}

		if envVars["GCLOUD_PROJECT"] != "my-project" {
			t.Errorf("GetEnvVars returned GCLOUD_PROJECT=%v, want my-project", envVars["GCLOUD_PROJECT"])
		}
	})
}
