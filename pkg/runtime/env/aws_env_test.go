package env

import (
	"fmt"
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
      profile: default
      region: us-west-2
      endpoint_url: https://aws.endpoint
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
			"AWS_REGION":                  "us-west-2",
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

	t.Run("EmitsConfigPathsEvenWhenFilesAbsent", func(t *testing.T) {
		// Given a fresh AWS-platform context where .aws/config and .aws/credentials
		// have not been created yet (operator hasn't run `aws configure`)
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
		envVars, err := env.GetEnvVars()

		// Then AWS_CONFIG_FILE and AWS_SHARED_CREDENTIALS_FILE still point at the
		// context-scoped paths so a subsequent `aws configure` writes into the context
		// folder rather than ~/.aws. AWS_PROFILE defaults to the context name so the
		// freshly-created profile section matches.
		if err != nil {
			t.Errorf("GetEnvVars returned an error: %v", err)
		}

		expected := map[string]string{
			"AWS_CONFIG_FILE":             "/mock/config/root/.aws/config",
			"AWS_SHARED_CREDENTIALS_FILE": "/mock/config/root/.aws/credentials",
			"AWS_PROFILE":                 "test-context",
		}

		if !reflect.DeepEqual(envVars, expected) {
			t.Errorf("GetEnvVars returned %v, want %v", envVars, expected)
		}
	})

	t.Run("ExplicitAWSProfileOverridesContextDefault", func(t *testing.T) {
		// Given an AWS block that pins a specific profile name
		mocks := setupAwsEnvMocks(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    aws:
      profile: shared-ops
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		env := NewAwsEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		env.shims = mocks.Shims

		// When GetEnvVars is called
		envVars, err := env.GetEnvVars()

		// Then AWS_PROFILE reflects the override, not the context name
		if err != nil {
			t.Errorf("GetEnvVars returned an error: %v", err)
		}
		if got := envVars["AWS_PROFILE"]; got != "shared-ops" {
			t.Errorf("AWS_PROFILE = %q, want %q", got, "shared-ops")
		}
	})

	t.Run("GlobalModeDefersToAmbientAWSConfig", func(t *testing.T) {
		// Given an AWS-platform context running in global mode (no project root —
		// operator is invoking windsor outside of a windsor.yaml-anchored tree)
		mocks := setupAwsEnvMocks(t)
		mocks.Shell.IsGlobalFunc = func() bool { return true }
		env := NewAwsEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		env.shims = mocks.Shims

		// When GetEnvVars is called
		envVars, err := env.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then AWS_CONFIG_FILE and AWS_SHARED_CREDENTIALS_FILE are NOT emitted, so
		// the AWS CLI/SDK fall through to the operator's ambient ~/.aws/. AWS_PROFILE
		// is still emitted because aws.profile is set explicitly in the context — the
		// user asked for that profile, even in global mode. Region/endpoint and the
		// other project-level identifiers continue to flow through.
		if _, ok := envVars["AWS_CONFIG_FILE"]; ok {
			t.Errorf("AWS_CONFIG_FILE should not be set in global mode, got %q", envVars["AWS_CONFIG_FILE"])
		}
		if _, ok := envVars["AWS_SHARED_CREDENTIALS_FILE"]; ok {
			t.Errorf("AWS_SHARED_CREDENTIALS_FILE should not be set in global mode, got %q", envVars["AWS_SHARED_CREDENTIALS_FILE"])
		}
		if got := envVars["AWS_PROFILE"]; got != "default" {
			t.Errorf("AWS_PROFILE = %q, want %q (explicit aws.profile override)", got, "default")
		}
		if got := envVars["AWS_REGION"]; got != "us-west-2" {
			t.Errorf("AWS_REGION = %q, want %q", got, "us-west-2")
		}
	})

	t.Run("GlobalModeFallsBackToContextNameForAWSProfile", func(t *testing.T) {
		// Given a context with no aws.profile override, running in global mode
		mocks := setupAwsEnvMocks(t)
		configStr := `
version: v1alpha1
contexts:
  test-context: {}
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		mocks.Shell.IsGlobalFunc = func() bool { return true }
		env := NewAwsEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		env.shims = mocks.Shims

		// When GetEnvVars is called
		envVars, err := env.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then AWS_PROFILE defaults to the context name so the AWS SDK resolves
		// the right profile in the operator's ambient ~/.aws/config — without it,
		// calls fall through to [default] even when the matching profile is logged
		// in. AWS_CONFIG_FILE / AWS_SHARED_CREDENTIALS_FILE remain unset so the
		// SDK uses the user-managed config locations.
		if got := envVars["AWS_PROFILE"]; got != "test-context" {
			t.Errorf("AWS_PROFILE = %q, want %q", got, "test-context")
		}
		if _, ok := envVars["AWS_CONFIG_FILE"]; ok {
			t.Errorf("AWS_CONFIG_FILE should not be set in global mode, got %q", envVars["AWS_CONFIG_FILE"])
		}
		if _, ok := envVars["AWS_SHARED_CREDENTIALS_FILE"]; ok {
			t.Errorf("AWS_SHARED_CREDENTIALS_FILE should not be set in global mode, got %q", envVars["AWS_SHARED_CREDENTIALS_FILE"])
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
      profile: default
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
