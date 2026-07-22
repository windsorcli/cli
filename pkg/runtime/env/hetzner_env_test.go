package env

import (
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupHetznerEnvMocks(t *testing.T, overrides ...*EnvTestMocks) *EnvTestMocks {
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
    hetzner:
      token: "test-token"
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

func TestHetznerEnv_GetEnvVars(t *testing.T) {
	setup := func(t *testing.T, eval evaluator.ExpressionEvaluator, overrides ...*EnvTestMocks) (*HetznerEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupHetznerEnvMocks(t, overrides...)
		printer := NewHetznerEnvPrinter(mocks.Shell, mocks.ConfigHandler, eval)
		printer.shims = mocks.Shims
		return printer, mocks
	}

	t.Run("EmitsHcloudTokenWhenTokenSet", func(t *testing.T) {
		// Given a printer with a literal Hetzner token configured
		printer, _ := setup(t, nil)

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Then HCLOUD_TOKEN reflects the configured token verbatim
		if envVars["HCLOUD_TOKEN"] != "test-token" {
			t.Errorf("GetEnvVars returned HCLOUD_TOKEN=%v, want test-token", envVars["HCLOUD_TOKEN"])
		}
		if len(envVars) != 1 {
			t.Errorf("Expected 1 environment variable, got %d: %v", len(envVars), envVars)
		}
	})

	t.Run("ResolvesSecretExpression", func(t *testing.T) {
		// Given a token expressed as a secret(...) reference and a mock evaluator
		mockEval := evaluator.NewMockExpressionEvaluator()
		mockEval.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			if strings.Contains(expression, "secret(") {
				return "resolved-token", nil
			}
			return expression, nil
		}
		mocks := setupHetznerEnvMocks(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    hetzner:
      token: ${secret("Developer", "hetzner_windsortest", "hetzner_key")}
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		printer := NewHetznerEnvPrinter(mocks.Shell, mocks.ConfigHandler, mockEval)
		printer.shims = mocks.Shims

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Then HCLOUD_TOKEN holds the decrypted value, not the literal expression
		if envVars["HCLOUD_TOKEN"] != "resolved-token" {
			t.Errorf("GetEnvVars returned HCLOUD_TOKEN=%q, want resolved-token", envVars["HCLOUD_TOKEN"])
		}
	})

	t.Run("RegistersResolvedTokenForScrubbing", func(t *testing.T) {
		// Given a resolved secret token
		mockEval := evaluator.NewMockExpressionEvaluator()
		mockEval.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return "resolved-token", nil
		}
		var registered []string
		mocks := setupHetznerEnvMocks(t)
		mocks.Shell.RegisterSecretFunc = func(value string) { registered = append(registered, value) }
		configStr := `
version: v1alpha1
contexts:
  test-context:
    hetzner:
      token: ${secret("Developer", "item", "field")}
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		printer := NewHetznerEnvPrinter(mocks.Shell, mocks.ConfigHandler, mockEval)
		printer.shims = mocks.Shims

		// When GetEnvVars is called
		if _, err := printer.GetEnvVars(); err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Then the resolved token is registered for output scrubbing
		found := false
		for _, v := range registered {
			if v == "resolved-token" {
				found = true
			}
		}
		if !found {
			t.Errorf("Expected resolved token to be registered for scrubbing, got %v", registered)
		}
	})

	t.Run("ReusesExportedTokenWithoutReResolving", func(t *testing.T) {
		// Given an already-exported HCLOUD_TOKEN and caching enabled
		mockEval := evaluator.NewMockExpressionEvaluator()
		evaluateCalled := false
		mockEval.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			evaluateCalled = true
			return "freshly-resolved", nil
		}
		mocks := setupHetznerEnvMocks(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    hetzner:
      token: ${secret("Developer", "item", "field")}
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		t.Setenv("NO_CACHE", "0")
		t.Setenv("HCLOUD_TOKEN", "cached-token")
		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			if key == "HCLOUD_TOKEN" {
				return "cached-token", true
			}
			return "", false
		}
		printer := NewHetznerEnvPrinter(mocks.Shell, mocks.ConfigHandler, mockEval)
		printer.shims = mocks.Shims

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Then the secret provider is not hit and the key is omitted (already exported)
		if evaluateCalled {
			t.Error("Expected evaluator not to be called when a cached token exists")
		}
		if _, exists := envVars["HCLOUD_TOKEN"]; exists {
			t.Errorf("Expected HCLOUD_TOKEN to be omitted when already exported, got %v", envVars)
		}
	})

	t.Run("EmitsHcloudTokenInGlobalMode", func(t *testing.T) {
		// Given a Hetzner context running in global mode
		printer, mocks := setup(t, nil)
		mocks.Shell.IsGlobalFunc = func() bool { return true }

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Then HCLOUD_TOKEN is still emitted since the token feeds terraform,
		// which runs from global shells too
		if envVars["HCLOUD_TOKEN"] != "test-token" {
			t.Errorf("GetEnvVars returned HCLOUD_TOKEN=%v, want test-token", envVars["HCLOUD_TOKEN"])
		}
	})

	t.Run("OmitsHcloudTokenWhenTokenUnset", func(t *testing.T) {
		// Given a Hetzner block present but no token set
		mocks := setupHetznerEnvMocks(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    hetzner: {}
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		printer := NewHetznerEnvPrinter(mocks.Shell, mocks.ConfigHandler, nil)
		printer.shims = mocks.Shims

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Then HCLOUD_TOKEN is omitted so the ambient credential applies
		if _, ok := envVars["HCLOUD_TOKEN"]; ok {
			t.Errorf("HCLOUD_TOKEN should not be set when token is unset, got %q", envVars["HCLOUD_TOKEN"])
		}
		if len(envVars) != 0 {
			t.Errorf("Expected empty environment variables, got %v", envVars)
		}
	})

	t.Run("OmitsHcloudTokenWhenNoHetznerConfig", func(t *testing.T) {
		// Given a context with no hetzner block at all
		mocks := setupHetznerEnvMocks(t)
		configStr := `
version: v1alpha1
contexts:
  test-context: {}
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		printer := NewHetznerEnvPrinter(mocks.Shell, mocks.ConfigHandler, nil)
		printer.shims = mocks.Shims

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Then no environment variables are emitted
		if len(envVars) != 0 {
			t.Errorf("Expected empty environment variables, got %v", envVars)
		}
	})
}
