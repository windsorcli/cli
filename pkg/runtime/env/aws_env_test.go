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
	// withProfile rewires the config-root mock to a real temp dir and writes
	// the supplied AWS config + credentials bodies into <root>/.aws/. Used by
	// project-mode tests that need contextHasAWSProfile to return true.
	withProfile := func(t *testing.T, mocks *EnvTestMocks, configBody, credentialsBody string) string {
		t.Helper()
		root := t.TempDir()
		awsDir := filepath.Join(root, ".aws")
		if err := os.MkdirAll(awsDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if configBody != "" {
			if err := os.WriteFile(filepath.Join(awsDir, "config"), []byte(configBody), 0644); err != nil {
				t.Fatalf("write config: %v", err)
			}
		}
		if credentialsBody != "" {
			if err := os.WriteFile(filepath.Join(awsDir, "credentials"), []byte(credentialsBody), 0644); err != nil {
				t.Fatalf("write credentials: %v", err)
			}
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return root, nil
		}
		return root
	}

	t.Run("EmitsAWSProfileWhenContextHasMatchingProfile", func(t *testing.T) {
		// Given the context's .aws/config contains the [default] profile section
		// that aws.profile resolves to
		mocks := setupAwsEnvMocks(t)
		root := withProfile(t, mocks, "[default]\nregion = us-west-2\n", "")
		env := NewAwsEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		env.shims = mocks.Shims

		// When GetEnvVars is called
		envVars, err := env.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then AWS_PROFILE flows alongside the file paths and destination vars
		expected := map[string]string{
			"AWS_PROFILE":                 "default",
			"AWS_REGION":                  "us-west-2",
			"AWS_ENDPOINT_URL":            "https://aws.endpoint",
			"S3_HOSTNAME":                 "s3.amazonaws.com",
			"MWAA_ENDPOINT":               "https://mwaa.endpoint",
			"AWS_CONFIG_FILE":             filepath.ToSlash(filepath.Join(root, ".aws", "config")),
			"AWS_SHARED_CREDENTIALS_FILE": filepath.ToSlash(filepath.Join(root, ".aws", "credentials")),
		}
		if !reflect.DeepEqual(envVars, expected) {
			t.Errorf("GetEnvVars returned %v, want %v", envVars, expected)
		}
	})

	t.Run("OmitsAWSProfileWhenContextHasNoProfile", func(t *testing.T) {
		// Given a project-mode context whose .aws/ is empty (operator has not
		// run `aws configure` yet, or is relying on env-key / IMDS / IRSA creds)
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
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then file paths still flow (so a future `aws configure` writes into the
		// context), but AWS_PROFILE is suppressed so the AWS SDK's credential
		// chain runs naturally instead of erroring on a missing profile section
		expected := map[string]string{
			"AWS_CONFIG_FILE":             "/mock/config/root/.aws/config",
			"AWS_SHARED_CREDENTIALS_FILE": "/mock/config/root/.aws/credentials",
		}
		if !reflect.DeepEqual(envVars, expected) {
			t.Errorf("GetEnvVars returned %v, want %v", envVars, expected)
		}
	})

	t.Run("ExplicitAWSProfileOverridesContextDefault", func(t *testing.T) {
		// Given an AWS block that pins a specific profile name, and a context
		// .aws/config that defines that profile
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
		withProfile(t, mocks, "[profile shared-ops]\nregion = us-east-1\n", "")
		env := NewAwsEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		env.shims = mocks.Shims

		// When GetEnvVars is called
		envVars, err := env.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then AWS_PROFILE reflects the override, not the context name
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

// =============================================================================
// Test Helpers
// =============================================================================

// TestContextHasAWSProfile covers the presence check that gates AWS_PROFILE
// emission. Each case writes a fresh .aws/ directory layout into a temp dir
// and asserts whether the helper finds the named profile.
func TestContextHasAWSProfile(t *testing.T) {
	writeAWSDir := func(t *testing.T, configBody, credentialsBody string) string {
		t.Helper()
		root := t.TempDir()
		awsDir := filepath.Join(root, ".aws")
		if err := os.MkdirAll(awsDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if configBody != "" {
			if err := os.WriteFile(filepath.Join(awsDir, "config"), []byte(configBody), 0644); err != nil {
				t.Fatalf("write config: %v", err)
			}
		}
		if credentialsBody != "" {
			if err := os.WriteFile(filepath.Join(awsDir, "credentials"), []byte(credentialsBody), 0644); err != nil {
				t.Fatalf("write credentials: %v", err)
			}
		}
		return root
	}

	t.Run("ReturnsFalseWhenAWSDirMissing", func(t *testing.T) {
		// Given a fresh context root with no .aws/ subdirectory
		root := t.TempDir()

		// When asking whether a named profile exists
		got := contextHasAWSProfile(root, "prod")

		// Then the helper returns false without erroring on the missing files
		if got {
			t.Errorf("expected false for missing .aws/, got true")
		}
	})

	t.Run("ReturnsTrueForNamedProfileInConfig", func(t *testing.T) {
		// Given a config file that declares [profile prod] alongside another section
		root := writeAWSDir(t, "[profile staging]\nregion = us-east-1\n\n[profile prod]\nregion = us-west-2\n", "")

		// When asking for prod
		got := contextHasAWSProfile(root, "prod")

		// Then the helper finds it
		if !got {
			t.Errorf("expected true for [profile prod] in config, got false")
		}
	})

	t.Run("ReturnsTrueForNamedProfileInCredentials", func(t *testing.T) {
		// Given a credentials file using the unprefixed [prod] section header
		root := writeAWSDir(t, "", "[prod]\naws_access_key_id = AKIA...\naws_secret_access_key = secret\n")

		// When asking for prod
		got := contextHasAWSProfile(root, "prod")

		// Then the helper finds it
		if !got {
			t.Errorf("expected true for [prod] in credentials, got false")
		}
	})

	t.Run("ReturnsTrueForDefaultProfileInConfig", func(t *testing.T) {
		// Given a config file containing the [default] section (no "profile " prefix)
		root := writeAWSDir(t, "[default]\nregion = us-west-2\n", "")

		// When asking for default
		got := contextHasAWSProfile(root, "default")

		// Then the helper finds it via the [default] header form
		if !got {
			t.Errorf("expected true for [default] in config, got false")
		}
	})

	t.Run("ReturnsFalseWhenOnlyOtherProfilesPresent", func(t *testing.T) {
		// Given a config that defines other profiles but not the one we ask about
		root := writeAWSDir(t, "[profile staging]\nregion = us-east-1\n", "[other]\naws_access_key_id = AKIA\n")

		// When asking for prod
		got := contextHasAWSProfile(root, "prod")

		// Then the helper returns false — neither file contains the named section
		if got {
			t.Errorf("expected false when prod absent from both files, got true")
		}
	})

	t.Run("MatchesSectionHeaderWithTrailingInlineComment", func(t *testing.T) {
		// Given a hand-edited config where the section header carries an inline
		// comment after the closing bracket — the aws CLI never writes these but
		// nothing stops an operator from adding one. The match should strip the
		// trailing #/; comment before comparing so the profile is still found.
		root := writeAWSDir(t, "[profile prod] # auto-generated by terraform\nregion = us-west-2\n", "")

		// When asking for prod
		got := contextHasAWSProfile(root, "prod")

		// Then the helper still finds the section despite the trailing comment
		if !got {
			t.Errorf("expected true for [profile prod] with trailing inline comment, got false")
		}
	})

	t.Run("IgnoresLookAlikeHeadersInsideConfigFile", func(t *testing.T) {
		// Given a config that names "prod" in unrelated positions but never as a section header
		root := writeAWSDir(t, "[profile staging]\n# comment about [profile prod]\nrole_arn = arn:aws:iam::1:role/prod\n", "")

		// When asking for prod
		got := contextHasAWSProfile(root, "prod")

		// Then the helper does not false-positive on the comment or the role arn
		if got {
			t.Errorf("expected false when prod appears only inside other sections, got true")
		}
	})
}
