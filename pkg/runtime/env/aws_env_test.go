package env

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupAwsEnvMocks(t *testing.T, overrides ...*EnvTestMocks) *EnvTestMocks {
	t.Helper()

	mocks := setupEnvMocks(t, overrides...)

	// If ConfigHandler wasn't overridden, use MockConfigHandler
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

	// Only load default config if ConfigHandler wasn't overridden
	// If ConfigHandler was injected via overrides, assume test wants to control it
	if len(overrides) == 0 || overrides[0] == nil || overrides[0].ConfigHandler == nil {
		defaultConfigStr := `
version: v1alpha1
contexts:
  test-context:
    aws:
      aws_profile: default
      aws_endpoint_url: https://aws.endpoint
      s3_hostname: s3.amazonaws.com
      mwaa_endpoint: https://mwaa.endpoint
`

		if err := mocks.ConfigHandler.LoadConfigString(defaultConfigStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
	}

	if err := mocks.ConfigHandler.SetContext("test-context"); err != nil {
		t.Fatalf("Failed to set context: %v", err)
	}

	mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
		if name == filepath.FromSlash("/mock/config/root/.aws/config") {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestAwsEnv_GetEnvVars tests the GetEnvVars method of the AwsEnvPrinter
func TestAwsEnv_GetEnvVars(t *testing.T) {
	setup := func() (*AwsEnvPrinter, *EnvTestMocks) {
		mocks := setupAwsEnvMocks(t)
		env := NewAwsEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		env.shims = mocks.Shims
		return env, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given an AWS env printer with configuration
		env, _ := setup()

		// When GetEnvVars is called
		envVars, err := env.GetEnvVars()

		// Then environment variables should match expected values
		if err != nil {
			t.Errorf("GetEnvVars returned an error: %v", err)
		}

		expected := map[string]string{
			"AWS_PROFILE":                 "default",
			"AWS_ENDPOINT_URL":            "https://aws.endpoint",
			"S3_HOSTNAME":                 "s3.amazonaws.com",
			"MWAA_ENDPOINT":               "https://mwaa.endpoint",
			"AWS_CONFIG_FILE":             "/mock/config/root/.aws/config",
			"AWS_SHARED_CREDENTIALS_FILE": "/mock/config/root/.aws/credentials",
		}

		if !reflect.DeepEqual(envVars, expected) {
			t.Errorf("GetEnvVars returned %v, want %v", envVars, expected)
		}
	})

	t.Run("NonExistentConfigFile", func(t *testing.T) {
		// Given an AWS env printer with non-existent config file
		env, _ := setup()

		env.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When GetEnvVars is called
		envVars, err := env.GetEnvVars()

		// Then environment variables should be returned with AWS_CONFIG_FILE and AWS_SHARED_CREDENTIALS_FILE
		// set even when files don't exist, to allow CLIs to generate auth files in the right location
		if err != nil {
			t.Errorf("GetEnvVars returned an error: %v", err)
		}

		expected := map[string]string{
			"AWS_PROFILE":                 "default",
			"AWS_ENDPOINT_URL":            "https://aws.endpoint",
			"S3_HOSTNAME":                 "s3.amazonaws.com",
			"MWAA_ENDPOINT":               "https://mwaa.endpoint",
			"AWS_CONFIG_FILE":             "/mock/config/root/.aws/config",
			"AWS_SHARED_CREDENTIALS_FILE": "/mock/config/root/.aws/credentials",
		}

		if !reflect.DeepEqual(envVars, expected) {
			t.Errorf("GetEnvVars returned %v, want %v", envVars, expected)
		}
	})

	t.Run("MissingConfiguration", func(t *testing.T) {
		// Given an AWS env printer with missing AWS configuration
		mocks := setupAwsEnvMocks(t)
		configStr := `
version: v1alpha1
contexts:
  test-context: {}
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		env := NewAwsEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		env.shims = mocks.Shims

		// When GetEnvVars is called
		_, err := env.GetEnvVars()

		// Then an error should be returned
		if err == nil {
			t.Error("GetEnvVars did not return an error")
		}
		if err != nil && !strings.Contains(err.Error(), "context configuration or AWS configuration is missing") {
			t.Errorf("GetEnvVars returned error %v, want error containing 'context configuration or AWS configuration is missing'", err)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		// Given a printer with a config handler that fails to get config root
		mocks := setupAwsEnvMocks(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    aws:
      aws_profile: default
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		if err := mocks.ConfigHandler.SetContext("test-context"); err != nil {
			t.Fatalf("Failed to set context: %v", err)
		}

		env := NewAwsEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		env.shims = mocks.Shims

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("error retrieving configuration root directory")
		}

		// When GetEnvVars is called
		_, err := env.GetEnvVars()

		// Then an error should be returned
		if err == nil {
			t.Error("GetEnvVars did not return an error")
		}
		if !strings.Contains(err.Error(), "error retrieving configuration root directory") {
			t.Errorf("GetEnvVars returned error %v, want error containing 'error retrieving configuration root directory'", err)
		}
	})
}
