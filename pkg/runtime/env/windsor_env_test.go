package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/secrets"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupWindsorEnvMocks(t *testing.T, opts ...func(*EnvTestMocks)) *EnvTestMocks {
	t.Helper()
	// Apply opts first to allow DI-style overrides (e.g., injecting a custom ConfigHandler)
	mocks := setupEnvMocks(t, opts...)

	// Only load default config if ConfigHandler wasn't overridden via opts
	// If ConfigHandler was injected via opts, assume test wants to control it
	if len(opts) == 0 {
		configStr := `
version: v1alpha1
contexts:
  mock-context:
    environment:
      TEST_VAR: test_value
      SECRET_VAR: "{{secret_name}}"
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		mocks.ConfigHandler.SetContext("mock-context")
	}

	// Get the temp dir that was set up in setupMocks
	projectRoot, err := mocks.Shell.GetProjectRoot()
	if err != nil {
		t.Fatalf("Failed to get project root: %v", err)
	}

	// Set up shims for Windsor operations
	mocks.Shims.LookupEnv = func(key string) (string, bool) {
		// Use os.LookupEnv to get the real environment variables
		val, ok := os.LookupEnv(key)
		return val, ok
	}

	// Mock GetSessionToken
	mocks.Shell.GetSessionTokenFunc = func() (string, error) {
		return "mock-token", nil
	}

	// Create and register mock secrets provider
	mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Shell)
	mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
		if strings.Contains(input, "${{secret_name}}") {
			return "parsed_secret_value", nil
		}
		return input, nil
	}
	_ = mockSecretsProvider

	t.Cleanup(func() {
		os.Unsetenv("NO_CACHE")
		os.Unsetenv("WINDSOR_MANAGED_ENV")
		mocks.Shell.ResetSessionToken()
	})

	// Set environment variables using the temp dir
	t.Setenv("WINDSOR_CONTEXT", "mock-context")
	t.Setenv("WINDSOR_PROJECT_ROOT", projectRoot)

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestWindsorEnv_GetEnvVars tests the GetEnvVars method of the WindsorEnvPrinter
func TestWindsorEnv_GetEnvVars(t *testing.T) {
	setup := func(t *testing.T) (*WindsorEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupWindsorEnvMocks(t)
		printer := NewWindsorEnvPrinter(mocks.Shell, mocks.ConfigHandler, []secrets.SecretsProvider{}, []EnvPrinter{})
		printer.shims = mocks.Shims
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly initialized WindsorEnvPrinter
		printer, _ := setup(t)

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And environment variables should contain expected values
		expectedContext := "mock-context"
		if envVars["WINDSOR_CONTEXT"] != expectedContext {
			t.Errorf("Expected WINDSOR_CONTEXT to be %q, got %q", expectedContext, envVars["WINDSOR_CONTEXT"])
		}

		// And project root should be set
		if envVars["WINDSOR_PROJECT_ROOT"] == "" {
			t.Error("Expected WINDSOR_PROJECT_ROOT to be set")
		}

		// And session token should be set
		expectedSessionToken := "mock-token"
		if envVars["WINDSOR_SESSION_TOKEN"] != expectedSessionToken {
			t.Errorf("Expected WINDSOR_SESSION_TOKEN to be %q, got %q", expectedSessionToken, envVars["WINDSOR_SESSION_TOKEN"])
		}

		// And context ID should be set but empty (ConfigHandler returns empty for non-existent keys)
		expectedContextID := ""
		if envVars["WINDSOR_CONTEXT_ID"] != expectedContextID {
			t.Errorf("Expected WINDSOR_CONTEXT_ID to be %q, got %q", expectedContextID, envVars["WINDSOR_CONTEXT_ID"])
		}
	})

	t.Run("ContextIDNotSet", func(t *testing.T) {
		// Given a WindsorEnvPrinter with no context ID
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "id" {
				return ""
			}
			return "mock-string"
		}

		mocks := setupWindsorEnvMocks(t, func(m *EnvTestMocks) {
			m.ConfigHandler = mockConfigHandler
		})
		printer := NewWindsorEnvPrinter(mocks.Shell, mocks.ConfigHandler, []secrets.SecretsProvider{}, []EnvPrinter{})
		printer.shims = mocks.Shims

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And WINDSOR_CONTEXT_ID should be set but empty
		if contextID, exists := envVars["WINDSOR_CONTEXT_ID"]; !exists {
			t.Error("Expected WINDSOR_CONTEXT_ID to be set even when empty")
		} else if contextID != "" {
			t.Errorf("Expected WINDSOR_CONTEXT_ID to be empty, got %q", contextID)
		}
	})

	t.Run("ProjectRootError", func(t *testing.T) {
		// Given a WindsorEnvPrinter with failing project root retrieval
		printer, mocks := setup(t)

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock project root error")
		}

		// When GetEnvVars is called
		_, err := printer.GetEnvVars()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error from project root retrieval, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving project root") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("SecretVarWithCacheEnabled", func(t *testing.T) {
		// Given a WindsorEnvPrinter with cache enabled
		printer, mocks := setup(t)

		t.Setenv("NO_CACHE", "0")
		t.Setenv("SECRET_VAR", "cached_value")
		t.Setenv("WINDSOR_MANAGED_ENV", "")

		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Shell)
		mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			if strings.Contains(input, "{{secret_name}}") {
				return "parsed_secret_value", nil
			}
			return input, nil
		}
		_ = mockSecretsProvider

		if err := mocks.ConfigHandler.LoadConfigString(`
version: v1alpha1
contexts:
  mock-context:
    environment:
      SECRET_VAR: "${{secret_name}}"
`); err != nil {
			t.Fatalf("LoadConfigString returned error: %v", err)
		}

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then the cached value should be used and the variable should not be tracked
		if _, exists := envVars["SECRET_VAR"]; exists {
			t.Error("Expected SECRET_VAR to not be in envVars when caching is enabled")
		}

		// And it should be tracked in managed env
		managedEnv := envVars["WINDSOR_MANAGED_ENV"]
		if !strings.Contains(managedEnv, "SECRET_VAR") {
			t.Error("Expected SECRET_VAR to be in managed env when caching is enabled")
		}
	})

	t.Run("SecretVarWithCacheDisabled", func(t *testing.T) {
		// Given a WindsorEnvPrinter with cache disabled
		printer, mocks := setup(t)

		t.Setenv("NO_CACHE", "1")
		t.Setenv("SECRET_VAR", "cached_value")

		if err := mocks.ConfigHandler.LoadConfigString(`
version: v1alpha1
contexts:
  mock-context:
    environment:
      SECRET_VAR: "${{secret_name}}"
`); err != nil {
			t.Fatalf("LoadConfigString returned error: %v", err)
		}

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then the cached value should not be used
		if envVars["SECRET_VAR"] == "cached_value" {
			t.Error("Expected SECRET_VAR to not use cached value")
		}
	})

	t.Run("SecretVarWithErrorInExistingValue", func(t *testing.T) {
		// Given a WindsorEnvPrinter with error in existing value
		printer, mocks := setup(t)

		t.Setenv("NO_CACHE", "0")
		t.Setenv("SECRET_VAR", "<e>secret error</e>")

		if err := mocks.ConfigHandler.LoadConfigString(`
version: v1alpha1
contexts:
  mock-context:
    environment:
      SECRET_VAR: "${{secret_name}}"
`); err != nil {
			t.Fatalf("LoadConfigString returned error: %v", err)
		}

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then the cached value should not be used
		if envVars["SECRET_VAR"] == "<e>secret error</e>" {
			t.Error("Expected SECRET_VAR to not use cached error value")
		}
	})

	t.Run("SecretVarWithManagedEnvExists", func(t *testing.T) {
		// Given a WindsorEnvPrinter with managed env exists
		printer, mocks := setup(t)

		t.Setenv("NO_CACHE", "0")
		t.Setenv("SECRET_VAR", "cached_value")
		t.Setenv("WINDSOR_MANAGED_ENV", "SECRET_VAR")

		if err := mocks.ConfigHandler.LoadConfigString(`
version: v1alpha1
contexts:
  mock-context:
    environment:
      SECRET_VAR: "${{secret_name}}"
`); err != nil {
			t.Fatalf("LoadConfigString returned error: %v", err)
		}

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then the variable should be in managed env
		managedEnv := envVars["WINDSOR_MANAGED_ENV"]
		if !strings.Contains(managedEnv, "SECRET_VAR") {
			t.Error("Expected SECRET_VAR to be in managed env")
		}
	})

	t.Run("ExistingSessionToken", func(t *testing.T) {
		// Given a WindsorEnvPrinter with token regeneration
		printer, mocks := setup(t)

		var callCount int
		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			callCount++
			if callCount == 1 {
				return "first-token", nil
			}
			return "regenerated-token", nil
		}

		// When GetEnvVars is called twice
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then the first token should be returned
		firstToken := envVars["WINDSOR_SESSION_TOKEN"]
		if firstToken != "first-token" {
			t.Errorf("Expected first token to be 'first-token', got %s", firstToken)
		}

		// And when called again
		envVars, err = printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then a new token should be generated
		secondToken := envVars["WINDSOR_SESSION_TOKEN"]
		if secondToken != "regenerated-token" {
			t.Errorf("Expected second token to be 'regenerated-token', got %s", secondToken)
		}
	})

	t.Run("SessionTokenError", func(t *testing.T) {
		// Given a WindsorEnvPrinter with failing session token generation
		printer, mocks := setup(t)

		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			return "", fmt.Errorf("mock session token error")
		}

		// When GetEnvVars is called
		_, err := printer.GetEnvVars()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error from session token generation, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving session token") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("NoEnvironmentVarsInConfig", func(t *testing.T) {
		// Given a WindsorEnvPrinter with empty environment configuration
		mocks := setupWindsorEnvMocks(t)
		configStr := `
version: v1alpha1
contexts:
  mock-context:
    environment: {}
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		mocks.ConfigHandler.SetContext("mock-context")
		printer := NewWindsorEnvPrinter(mocks.Shell, mocks.ConfigHandler, []secrets.SecretsProvider{}, []EnvPrinter{})
		printer.shims = mocks.Shims

		projectRoot, err := mocks.Shell.GetProjectRoot()
		if err != nil {
			t.Fatalf("Failed to get project root: %v", err)
		}

		printer.managedEnv = []string{}
		printer.managedAlias = []string{}

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars should not return an error: %v", err)
		}

		// Then base environment variables should be set
		if envVars["WINDSOR_CONTEXT"] != "mock-context" {
			t.Errorf("WINDSOR_CONTEXT should be set even when no environment vars are in config")
		}
		if envVars["WINDSOR_PROJECT_ROOT"] != projectRoot {
			t.Errorf("WINDSOR_PROJECT_ROOT = %q, want %q", envVars["WINDSOR_PROJECT_ROOT"], projectRoot)
		}
		if envVars["WINDSOR_SESSION_TOKEN"] == "" {
			t.Errorf("Session token should be generated")
		}

		// And no additional variables should be added
		t.Logf("Environment variables: %v", envVars)
		if len(envVars) != 7 {
			t.Errorf("Should have seven base environment variables (including BUILD_ID which is generated if missing)")
		}
		if envVars["BUILD_ID"] == "" {
			t.Errorf("BUILD_ID should be generated when file doesn't exist")
		}
	})

	t.Run("EnvironmentTokenWithoutSignalFile", func(t *testing.T) {
		// Given a WindsorEnvPrinter with environment token and no signal file
		printer, mocks := setup(t)

		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			if envToken, exists := mocks.Shims.LookupEnv("WINDSOR_SESSION_TOKEN"); exists {
				return envToken, nil
			}
			return "mock-token", nil
		}

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then the environment token should be used
		if envVars["WINDSOR_SESSION_TOKEN"] != "envtoken" {
			t.Errorf("Expected session token to be 'envtoken', got %s", envVars["WINDSOR_SESSION_TOKEN"])
		}
	})

	t.Run("EnvironmentTokenWithStatError", func(t *testing.T) {
		// Given a WindsorEnvPrinter with environment token and stat error
		printer, mocks := setup(t)

		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, fmt.Errorf("mock stat error")
		}

		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			if envToken, exists := mocks.Shims.LookupEnv("WINDSOR_SESSION_TOKEN"); exists {
				return envToken, nil
			}
			return "mock-token", nil
		}

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then the environment token should be used
		if envVars["WINDSOR_SESSION_TOKEN"] != "envtoken" {
			t.Errorf("Expected session token to be 'envtoken', got %s", envVars["WINDSOR_SESSION_TOKEN"])
		}
	})

	t.Run("EnvironmentTokenWithSignalFile", func(t *testing.T) {
		// Given a WindsorEnvPrinter with environment token and signal file
		printer, mocks := setup(t)

		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".session.envtoken") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Shims.RemoveAll = func(path string) error {
			return nil
		}

		mocks.Shims.CryptoRandRead = func(b []byte) (n int, err error) {
			for i := range b {
				b[i] = byte(i % 62)
			}
			return len(b), nil
		}

		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			if envToken, exists := mocks.Shims.LookupEnv("WINDSOR_SESSION_TOKEN"); exists {
				tokenFilePath := filepath.Join("/mock/project/root", ".windsor", ".session."+envToken)
				if _, err := mocks.Shims.Stat(tokenFilePath); err == nil {
					return "abcdefg", nil
				}
				return envToken, nil
			}
			return "mock-token", nil
		}

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then a new token should be generated
		if envVars["WINDSOR_SESSION_TOKEN"] == "envtoken" {
			t.Errorf("Expected a new token to be generated, but got the environment token")
		}
		if envVars["WINDSOR_SESSION_TOKEN"] != "abcdefg" {
			t.Errorf("Expected session token to be 'abcdefg', got %s", envVars["WINDSOR_SESSION_TOKEN"])
		}
	})

	t.Run("SignalFileRemovalError", func(t *testing.T) {
		// Given a WindsorEnvPrinter with environment token and signal file removal error
		printer, mocks := setup(t)

		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".session.envtoken") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Shims.RemoveAll = func(path string) error {
			return fmt.Errorf("mock error removing signal file")
		}

		mocks.Shims.CryptoRandRead = func(b []byte) (n int, err error) {
			for i := range b {
				b[i] = byte(i % 62)
			}
			return len(b), nil
		}

		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			if envToken, exists := mocks.Shims.LookupEnv("WINDSOR_SESSION_TOKEN"); exists {
				tokenFilePath := filepath.Join("/mock/project/root", ".windsor", ".session."+envToken)
				if _, err := mocks.Shims.Stat(tokenFilePath); err == nil {
					return "abcdefg", nil
				}
				return envToken, nil
			}
			return "mock-token", nil
		}

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then a new token should be generated despite removal error
		if envVars["WINDSOR_SESSION_TOKEN"] == "envtoken" {
			t.Errorf("Expected a new token to be generated, but got the environment token")
		}
		if envVars["WINDSOR_SESSION_TOKEN"] != "abcdefg" {
			t.Errorf("Expected session token to be 'abcdefg', got %s", envVars["WINDSOR_SESSION_TOKEN"])
		}
	})

	t.Run("ProjectRootErrorDuringEnvTokenSignalFileCheck", func(t *testing.T) {
		// Given a WindsorEnvPrinter with environment token and project root error
		printer, mocks := setup(t)

		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			if _, exists := mocks.Shims.LookupEnv("WINDSOR_SESSION_TOKEN"); exists {
				return "", fmt.Errorf("error getting project root: mock error getting project root during token check")
			}
			return "mock-token", nil
		}

		// When GetEnvVars is called
		_, err := printer.GetEnvVars()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		expectedErr := "error retrieving session token: error getting project root: mock error getting project root during token check"
		if err.Error() != expectedErr {
			t.Errorf("Expected error %q, got %q", expectedErr, err.Error())
		}
	})

	t.Run("RandomGenerationError", func(t *testing.T) {
		// Given a WindsorEnvPrinter with environment token and random generation error
		printer, mocks := setup(t)

		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".session.envtoken") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			if envToken, exists := mocks.Shims.LookupEnv("WINDSOR_SESSION_TOKEN"); exists {
				tokenFilePath := filepath.Join("/mock/project/root", ".windsor", ".session."+envToken)
				if _, err := mocks.Shims.Stat(tokenFilePath); err == nil {
					return "", fmt.Errorf("mock random generation error during token regeneration")
				}
				return envToken, nil
			}
			return "mock-token", nil
		}

		// When GetEnvVars is called
		_, err := printer.GetEnvVars()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error from random generation during token regeneration, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving session token") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Given a WindsorEnvPrinter with failing project root lookup
		printer, mocks := setup(t)

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock shell error")
		}

		// When GetEnvVars is called
		_, err := printer.GetEnvVars()

		// Then an error should be returned
		expectedErrorMessage := "error retrieving project root: mock shell error"
		if err == nil || err.Error() != expectedErrorMessage {
			t.Errorf("Expected error %q, got %v", expectedErrorMessage, err)
		}
	})

	t.Run("ProjectRootErrorDuringTokenCheck", func(t *testing.T) {
		// Given a WindsorEnvPrinter with environment token and project root error during token check
		printer, mocks := setup(t)

		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			if _, exists := mocks.Shims.LookupEnv("WINDSOR_SESSION_TOKEN"); exists {
				return "", fmt.Errorf("error getting project root: mock shell error during token check")
			}
			return "mock-token", nil
		}

		// When GetEnvVars is called
		_, err := printer.GetEnvVars()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		expectedErr := "error retrieving session token: error getting project root: mock shell error during token check"
		if err.Error() != expectedErr {
			t.Errorf("Expected error %q, got %q", expectedErr, err.Error())
		}
	})

	t.Run("ComprehensiveEnvironmentTokenTest", func(t *testing.T) {
		// Given a WindsorEnvPrinter with mock file system functions
		printer, mocks := setup(t)

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".session.testtoken") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// Phase 1: No environment token present
		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			return "", false
		}

		// When GetEnvVars is called
		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}
		firstToken := envVars["WINDSOR_SESSION_TOKEN"]

		// Phase 2: Set environment token
		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			if key == "WINDSOR_SESSION_TOKEN" {
				return "testtoken", true
			}
			return "", false
		}

		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			if envToken, exists := mocks.Shims.LookupEnv("WINDSOR_SESSION_TOKEN"); exists {
				tokenFilePath := filepath.Join("/mock/project/root", ".windsor", ".session."+envToken)
				if _, err := mocks.Shims.Stat(tokenFilePath); err == nil {
					return "newtoken", nil
				}
				return envToken, nil
			}
			return "mock-token", nil
		}

		// When GetEnvVars is called again
		envVars, err = printer.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error in phase 2: %v", err)
		}

		// Then a new token should be generated
		secondToken := envVars["WINDSOR_SESSION_TOKEN"]
		if secondToken != "newtoken" {
			t.Errorf("Expected token 'newtoken', got %q", secondToken)
		}

		if secondToken == firstToken {
			t.Errorf("Second token %q should be different from the first token %q", secondToken, firstToken)
		}
	})
}

// TestWindsorEnv_PostEnvHook tests the PostEnvHook method of the WindsorEnvPrinter
func TestWindsorEnv_PostEnvHook(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a WindsorEnvPrinter
		mocks := setupWindsorEnvMocks(t)
		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Shell, mocks.ConfigHandler, []secrets.SecretsProvider{}, []EnvPrinter{})

		// When PostEnvHook is called
		err := windsorEnvPrinter.PostEnvHook()

		// Then no error should be returned
		if err != nil {
			t.Errorf("PostEnvHook() returned an error: %v", err)
		}
	})
}

// TestWindsorEnv_Initialize tests the Initialize method of the WindsorEnvPrinter
func TestWindsorEnv_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*WindsorEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupWindsorEnvMocks(t)
		printer := NewWindsorEnvPrinter(mocks.Shell, mocks.ConfigHandler, []secrets.SecretsProvider{}, []EnvPrinter{})
		printer.shims = mocks.Shims
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a WindsorEnvPrinter
		printer, _ := setup(t)

		// Then printer should be created
		if printer == nil {
			t.Fatal("Expected printer to be created")
		}

		if len(printer.secretsProviders) != 0 {
			t.Errorf("Expected 0 secrets providers, got %d", len(printer.secretsProviders))
		}
	})

	t.Run("ResolveAllError", func(t *testing.T) {
		// Given a WindsorEnvPrinter
		mocks := setupWindsorEnvMocks(t)
		printer := NewWindsorEnvPrinter(mocks.Shell, mocks.ConfigHandler, []secrets.SecretsProvider{}, []EnvPrinter{})

		// Then printer should be created
		if printer == nil {
			t.Fatal("Expected printer to be created")
		}
	})
}

// TestWindsorEnv_ParseAndCheckSecrets tests the parseAndCheckSecrets method
func TestWindsorEnv_ParseAndCheckSecrets(t *testing.T) {
	setup := func(t *testing.T) (*WindsorEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupWindsorEnvMocks(t)
		printer := NewWindsorEnvPrinter(mocks.Shell, mocks.ConfigHandler, []secrets.SecretsProvider{}, []EnvPrinter{})
		printer.shims = mocks.Shims
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a WindsorEnvPrinter with a secrets provider that successfully parses secrets
		printer, mocks := setup(t)

		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Shell)
		mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			if input == "value with ${{ secrets.mySecret }}" {
				return "value with resolved-secret", nil
			}
			return input, nil
		}
		printer.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// When parseAndCheckSecrets is called
		result := printer.parseAndCheckSecrets("value with ${{ secrets.mySecret }}")

		// Then the secret should be resolved
		if result != "value with resolved-secret" {
			t.Errorf("Expected 'value with resolved-secret', got %q", result)
		}
	})

	t.Run("SecretsProviderError", func(t *testing.T) {
		// Given a WindsorEnvPrinter with a secrets provider that fails to parse secrets
		printer, mocks := setup(t)

		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Shell)
		mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			return "", fmt.Errorf("error parsing secrets")
		}
		printer.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// When parseAndCheckSecrets is called
		result := printer.parseAndCheckSecrets("value with ${{ secrets.mySecret }}")

		// Then an error message should be returned
		if !strings.Contains(result, "<ERROR: failed to parse") {
			t.Errorf("Expected error message containing 'failed to parse', got %q", result)
		}
		if !strings.Contains(result, "secrets.mySecret") {
			t.Errorf("Expected error message to contain 'secrets.mySecret', got %q", result)
		}
	})

	t.Run("NoSecretsProviders", func(t *testing.T) {
		// Given a WindsorEnvPrinter with no secrets providers
		printer, _ := setup(t)

		printer.secretsProviders = []secrets.SecretsProvider{}

		// When parseAndCheckSecrets is called
		result := printer.parseAndCheckSecrets("value with ${{ secrets.mySecret }}")

		// Then an error message should be returned
		if result != "<ERROR: No secrets providers configured>" {
			t.Errorf("Expected '<ERROR: No secrets providers configured>', got %q", result)
		}
	})

	t.Run("UnparsedSecrets", func(t *testing.T) {
		printer, mocks := setup(t)

		// Given a mock secrets provider that doesn't recognize secrets
		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Shell)
		mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			return input, nil
		}
		printer.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// When parseAndCheckSecrets is called
		result := printer.parseAndCheckSecrets("value with ${{ secrets.mySecret }}")

		// Then an error message should be returned
		if !strings.Contains(result, "<ERROR: failed to parse") {
			t.Errorf("Expected error message containing 'failed to parse', got %q", result)
		}
		if !strings.Contains(result, "secrets.mySecret") {
			t.Errorf("Expected error message to contain 'secrets.mySecret', got %q", result)
		}
	})

	t.Run("MultipleUnparsedSecrets", func(t *testing.T) {
		printer, mocks := setup(t)

		// Given a mock secrets provider that doesn't recognize secrets
		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Shell)
		mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			return input, nil
		}
		printer.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// When parseAndCheckSecrets is called with multiple secrets
		result := printer.parseAndCheckSecrets("value with ${{ secrets.secretA }} and ${{ secrets.secretB }}")

		// Then an error message should be returned
		if !strings.Contains(result, "<ERROR: failed to parse") {
			t.Errorf("Expected error message containing 'failed to parse', got %q", result)
		}
		if !strings.Contains(result, "secrets.secretA, secrets.secretB") {
			t.Errorf("Expected error message to contain 'secrets.secretA, secrets.secretB', got %q", result)
		}
	})
}

func TestWindsorEnv_shouldUseCache(t *testing.T) {
	setup := func(t *testing.T) (*WindsorEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupWindsorEnvMocks(t)
		printer := NewWindsorEnvPrinter(mocks.Shell, mocks.ConfigHandler, []secrets.SecretsProvider{}, []EnvPrinter{})
		printer.shims = mocks.Shims
		return printer, mocks
	}

	t.Run("EmptyNoCache", func(t *testing.T) {
		// Given NO_CACHE environment variable is not set
		printer, mocks := setup(t)
		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			if key == "NO_CACHE" {
				return "", false
			}
			return "", false
		}

		// When shouldUseCache is called
		shouldCache := printer.shouldUseCache()

		// Then it should return true
		if !shouldCache {
			t.Error("Expected shouldUseCache to return true for empty NO_CACHE")
		}
	})

	t.Run("NoCacheZero", func(t *testing.T) {
		// Given NO_CACHE environment variable is set to "0"
		printer, mocks := setup(t)
		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			if key == "NO_CACHE" {
				return "0", true
			}
			return "", false
		}

		// When shouldUseCache is called
		shouldCache := printer.shouldUseCache()

		// Then it should return true
		if !shouldCache {
			t.Error("Expected shouldUseCache to return true for NO_CACHE=0")
		}
	})

	t.Run("NoCacheFalse", func(t *testing.T) {
		// Given NO_CACHE environment variable is set to "false"
		printer, mocks := setup(t)
		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			if key == "NO_CACHE" {
				return "false", true
			}
			return "", false
		}

		// When shouldUseCache is called
		shouldCache := printer.shouldUseCache()

		// Then it should return true
		if !shouldCache {
			t.Error("Expected shouldUseCache to return true for NO_CACHE=false")
		}
	})

	t.Run("NoCacheOne", func(t *testing.T) {
		// Given NO_CACHE environment variable is set to "1"
		printer, mocks := setup(t)
		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			if key == "NO_CACHE" {
				return "1", true
			}
			return "", false
		}

		// When shouldUseCache is called
		shouldCache := printer.shouldUseCache()

		// Then it should return false
		if shouldCache {
			t.Error("Expected shouldUseCache to return false for NO_CACHE=1")
		}
	})
}

// TestWindsorEnv_getBuildID tests the getBuildID method of the WindsorEnvPrinter
func TestWindsorEnv_getBuildID(t *testing.T) {
	setup := func(t *testing.T) (*WindsorEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupWindsorEnvMocks(t)
		printer := NewWindsorEnvPrinter(mocks.Shell, mocks.ConfigHandler, []secrets.SecretsProvider{}, []EnvPrinter{})
		printer.shims = mocks.Shims
		return printer, mocks
	}

	t.Run("ErrorWhenGetProjectRootFails", func(t *testing.T) {
		// Given a WindsorEnvPrinter with failing GetProjectRoot
		printer, mocks := setup(t)

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting project root")
		}

		// When getBuildID is called
		_, err := printer.getBuildID()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get project root") {
			t.Errorf("Expected error about getting project root, got %v", err)
		}
	})

	t.Run("ErrorWhenReadFileFails", func(t *testing.T) {
		// Given a WindsorEnvPrinter with existing build ID file that fails to read
		printer, mocks := setup(t)

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".build-id") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Shims.ReadFile = func(filename string) ([]byte, error) {
			if strings.Contains(filename, ".build-id") {
				return nil, fmt.Errorf("mock error reading build ID file")
			}
			return os.ReadFile(filename)
		}

		// When getBuildID is called
		_, err := printer.getBuildID()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to read build ID file") {
			t.Errorf("Expected error about reading build ID file, got %v", err)
		}
	})

	t.Run("ErrorWhenWriteBuildIDToFileFails", func(t *testing.T) {
		// Given a WindsorEnvPrinter with no existing build ID file
		printer, mocks := setup(t)

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock error creating directory")
		}

		// When getBuildID is called
		_, err := printer.getBuildID()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to set build ID") {
			t.Errorf("Expected error about setting build ID, got %v", err)
		}
	})
}

// TestWindsorEnv_writeBuildIDToFile tests the writeBuildIDToFile method of the WindsorEnvPrinter
func TestWindsorEnv_writeBuildIDToFile(t *testing.T) {
	setup := func(t *testing.T) (*WindsorEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupWindsorEnvMocks(t)
		printer := NewWindsorEnvPrinter(mocks.Shell, mocks.ConfigHandler, []secrets.SecretsProvider{}, []EnvPrinter{})
		printer.shims = mocks.Shims
		return printer, mocks
	}

	t.Run("ErrorWhenGetProjectRootFails", func(t *testing.T) {
		// Given a WindsorEnvPrinter with failing GetProjectRoot
		printer, mocks := setup(t)

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting project root")
		}

		// When writeBuildIDToFile is called
		err := printer.writeBuildIDToFile("240101.123.1")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get project root") {
			t.Errorf("Expected error about getting project root, got %v", err)
		}
	})

	t.Run("ErrorWhenMkdirAllFails", func(t *testing.T) {
		// Given a WindsorEnvPrinter with failing MkdirAll
		printer, mocks := setup(t)

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock error creating directory")
		}

		// When writeBuildIDToFile is called
		err := printer.writeBuildIDToFile("240101.123.1")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create build ID directory") {
			t.Errorf("Expected error about creating directory, got %v", err)
		}
	})
}

// TestWindsorEnv_generateBuildID tests the generateBuildID method of the WindsorEnvPrinter
func TestWindsorEnv_generateBuildID(t *testing.T) {
	setup := func(t *testing.T) (*WindsorEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupWindsorEnvMocks(t)
		printer := NewWindsorEnvPrinter(mocks.Shell, mocks.ConfigHandler, []secrets.SecretsProvider{}, []EnvPrinter{})
		printer.shims = mocks.Shims
		return printer, mocks
	}

	t.Run("ErrorWhenGetProjectRootFails", func(t *testing.T) {
		// Given a WindsorEnvPrinter with failing GetProjectRoot
		printer, mocks := setup(t)

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting project root")
		}

		// When generateBuildID is called
		_, err := printer.generateBuildID()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get project root") {
			t.Errorf("Expected error about getting project root, got %v", err)
		}
	})

	t.Run("ErrorWhenReadFileFails", func(t *testing.T) {
		// Given a WindsorEnvPrinter with existing build ID file that fails to read
		printer, mocks := setup(t)

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".build-id") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Shims.ReadFile = func(filename string) ([]byte, error) {
			if strings.Contains(filename, ".build-id") {
				return nil, fmt.Errorf("mock error reading build ID file")
			}
			return os.ReadFile(filename)
		}

		// When generateBuildID is called
		_, err := printer.generateBuildID()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to read build ID file") {
			t.Errorf("Expected error about reading build ID file, got %v", err)
		}
	})

}

// TestWindsorEnv_incrementBuildID tests the incrementBuildID method of the WindsorEnvPrinter
func TestWindsorEnv_incrementBuildID(t *testing.T) {
	setup := func(t *testing.T) (*WindsorEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupWindsorEnvMocks(t)
		printer := NewWindsorEnvPrinter(mocks.Shell, mocks.ConfigHandler, []secrets.SecretsProvider{}, []EnvPrinter{})
		printer.shims = mocks.Shims
		return printer, mocks
	}

	t.Run("ErrorOnInvalidFormat", func(t *testing.T) {
		// Given a WindsorEnvPrinter with invalid build ID format
		printer, _ := setup(t)

		// When incrementBuildID is called with invalid format
		_, err := printer.incrementBuildID("invalid", "240101")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid build ID format") {
			t.Errorf("Expected error about invalid format, got %v", err)
		}
	})

	t.Run("ErrorOnInvalidCounter", func(t *testing.T) {
		// Given a WindsorEnvPrinter with invalid counter component
		printer, _ := setup(t)

		// When incrementBuildID is called with invalid counter
		_, err := printer.incrementBuildID("240101.123.invalid", "240101")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid counter component") {
			t.Errorf("Expected error about invalid counter, got %v", err)
		}
	})

	t.Run("IncrementsCounterForSameDate", func(t *testing.T) {
		// Given a WindsorEnvPrinter with same date
		printer, _ := setup(t)

		// When incrementBuildID is called with same date
		result, err := printer.incrementBuildID("240101.123.5", "240101")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And counter should be incremented
		expected := "240101.123.6"
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("ResetsCounterForDifferentDate", func(t *testing.T) {
		// Given a WindsorEnvPrinter with different date
		printer, _ := setup(t)

		// When incrementBuildID is called with different date
		result, err := printer.incrementBuildID("240101.123.5", "240102")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And counter should be reset to 1
		if !strings.HasSuffix(result, ".1") {
			t.Errorf("Expected counter to be reset to 1, got %s", result)
		}
		if !strings.HasPrefix(result, "240102.") {
			t.Errorf("Expected date to be 240102, got %s", result)
		}
	})
}
