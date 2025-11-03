package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/context/config"
	"github.com/windsorcli/cli/pkg/context/secrets"
	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupWindsorEnvMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()
	if opts == nil {
		opts = []*SetupOptions{{}}
	}
	if opts[0].ConfigStr == "" {
		opts[0].ConfigStr = `
version: v1alpha1
contexts:
  mock-context:
    environment:
      TEST_VAR: test_value
      SECRET_VAR: "{{secret_name}}"
`
	}
	if opts[0].Context == "" {
		opts[0].Context = "mock-context"
	}
	mocks := setupMocks(t, opts[0])

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
	mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
	mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
		if strings.Contains(input, "${{secret_name}}") {
			return "parsed_secret_value", nil
		}
		return input, nil
	}
	mocks.Injector.Register("secretsProvider", mockSecretsProvider)

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
	setup := func(t *testing.T) (*WindsorEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupWindsorEnvMocks(t)
		printer := NewWindsorEnvPrinter(mocks.Injector)
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize env: %v", err)
		}
		printer.shims = mocks.Shims
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		printer, _ := setup(t)

		// Given a properly initialized WindsorEnvPrinter
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

		mocks := setupWindsorEnvMocks(t, &SetupOptions{
			ConfigHandler: mockConfigHandler,
		})
		printer := NewWindsorEnvPrinter(mocks.Injector)
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize env: %v", err)
		}
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
		printer, mocks := setup(t)

		// Given a WindsorEnvPrinter with failing project root retrieval
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
		printer, mocks := setup(t)

		// Given a WindsorEnvPrinter with cache enabled
		t.Setenv("NO_CACHE", "0")
		t.Setenv("SECRET_VAR", "cached_value")
		t.Setenv("WINDSOR_MANAGED_ENV", "")

		// And mock secrets provider
		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			if strings.Contains(input, "{{secret_name}}") {
				return "parsed_secret_value", nil
			}
			return input, nil
		}
		mocks.Injector.Register("secretsProvider", mockSecretsProvider)

		// And re-initialize printer to pick up new mock
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to re-initialize env: %v", err)
		}

		// And config with secret variable
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
		printer, mocks := setup(t)

		// Given a WindsorEnvPrinter with cache disabled
		t.Setenv("NO_CACHE", "1")
		t.Setenv("SECRET_VAR", "cached_value")

		// And config with secret variable
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
		printer, mocks := setup(t)

		// Given a WindsorEnvPrinter with error in existing value
		t.Setenv("NO_CACHE", "0")
		t.Setenv("SECRET_VAR", "<e>secret error</e>")

		// And config with secret variable
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
		printer, mocks := setup(t)

		// Given a WindsorEnvPrinter with managed env exists
		t.Setenv("NO_CACHE", "0")
		t.Setenv("SECRET_VAR", "cached_value")
		t.Setenv("WINDSOR_MANAGED_ENV", "SECRET_VAR")

		// And config with secret variable
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
		printer, mocks := setup(t)

		// Given a WindsorEnvPrinter with token regeneration
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
		printer, mocks := setup(t)

		// Given a WindsorEnvPrinter with failing session token generation
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
		// Setup with empty environment
		mocks := setupWindsorEnvMocks(t, &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  mock-context:
    environment: {}
`,
			Context: "mock-context",
		})
		printer := NewWindsorEnvPrinter(mocks.Injector)
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize env: %v", err)
		}
		printer.shims = mocks.Shims

		// Given a WindsorEnvPrinter with empty environment configuration
		projectRoot, err := mocks.Shell.GetProjectRoot()
		if err != nil {
			t.Fatalf("Failed to get project root: %v", err)
		}

		// And no managed environment variables or aliases
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
		if len(envVars) != 6 {
			t.Errorf("Should have six base environment variables (BUILD_ID excluded when file doesn't exist)")
		}
	})

	t.Run("EnvironmentTokenWithoutSignalFile", func(t *testing.T) {
		printer, mocks := setup(t)

		// Given a WindsorEnvPrinter with environment token
		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		// And no signal file exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// And GetSessionToken configured to handle environment token
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
		printer, mocks := setup(t)

		// Given a WindsorEnvPrinter with environment token
		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		// And stat returns a non-ErrNotExist error
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, fmt.Errorf("mock stat error")
		}

		// And GetSessionToken configured to handle environment token
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
		printer, mocks := setup(t)

		// Given a WindsorEnvPrinter with environment token
		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		// And signal file exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".session.envtoken") {
				return nil, nil // File exists
			}
			return nil, os.ErrNotExist
		}

		// And RemoveAll succeeds
		mocks.Shims.RemoveAll = func(path string) error {
			return nil
		}

		// And CryptoRandRead returns predictable output
		mocks.Shims.CryptoRandRead = func(b []byte) (n int, err error) {
			for i := range b {
				b[i] = byte(i % 62) // Will map to characters in charset
			}
			return len(b), nil
		}

		// And GetSessionToken configured to handle environment token and signal file
		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			if envToken, exists := mocks.Shims.LookupEnv("WINDSOR_SESSION_TOKEN"); exists {
				tokenFilePath := filepath.Join("/mock/project/root", ".windsor", ".session."+envToken)
				if _, err := mocks.Shims.Stat(tokenFilePath); err == nil {
					// Signal file exists, generate new token
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
		printer, mocks := setup(t)

		// Given a WindsorEnvPrinter with environment token
		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		// And signal file exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".session.envtoken") {
				return nil, nil // File exists
			}
			return nil, os.ErrNotExist
		}

		// And RemoveAll fails
		mocks.Shims.RemoveAll = func(path string) error {
			return fmt.Errorf("mock error removing signal file")
		}

		// And CryptoRandRead returns predictable output
		mocks.Shims.CryptoRandRead = func(b []byte) (n int, err error) {
			for i := range b {
				b[i] = byte(i % 62) // Will map to characters in charset
			}
			return len(b), nil
		}

		// And GetSessionToken configured to handle environment token and signal file
		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			if envToken, exists := mocks.Shims.LookupEnv("WINDSOR_SESSION_TOKEN"); exists {
				tokenFilePath := filepath.Join("/mock/project/root", ".windsor", ".session."+envToken)
				if _, err := mocks.Shims.Stat(tokenFilePath); err == nil {
					// Signal file exists, generate new token
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
		printer, mocks := setup(t)

		// Given a WindsorEnvPrinter with environment token
		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		// And GetSessionToken returns an error during signal file check
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
		printer, mocks := setup(t)

		// Given a WindsorEnvPrinter with environment token
		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		// And signal file exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".session.envtoken") {
				return nil, nil // File exists
			}
			return nil, os.ErrNotExist
		}

		// And GetSessionToken returns an error during token regeneration
		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			if envToken, exists := mocks.Shims.LookupEnv("WINDSOR_SESSION_TOKEN"); exists {
				tokenFilePath := filepath.Join("/mock/project/root", ".windsor", ".session."+envToken)
				if _, err := mocks.Shims.Stat(tokenFilePath); err == nil {
					// Signal file exists, mock error during regeneration
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
		printer, mocks := setup(t)

		// Given a WindsorEnvPrinter with failing project root lookup
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
		printer, mocks := setup(t)

		// Given a WindsorEnvPrinter with environment token
		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		// And GetSessionToken returns an error during project root check
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
		printer, mocks := setup(t)

		// Given a WindsorEnvPrinter with mock file system functions
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".session.testtoken") {
				return nil, nil // Session file exists
			}
			return nil, os.ErrNotExist
		}

		// Phase 1: No environment token present
		// Given no environment token
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
		// Given environment token is set
		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			if key == "WINDSOR_SESSION_TOKEN" {
				return "testtoken", true
			}
			return "", false
		}

		// And GetSessionToken configured to handle testtoken
		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			if envToken, exists := mocks.Shims.LookupEnv("WINDSOR_SESSION_TOKEN"); exists {
				// Our testtoken has a signal file
				tokenFilePath := filepath.Join("/mock/project/root", ".windsor", ".session."+envToken)
				if _, err := mocks.Shims.Stat(tokenFilePath); err == nil {
					return "newtoken", nil // Return a different token to show regeneration
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
		injector := di.NewMockInjector()
		windsorEnvPrinter := NewWindsorEnvPrinter(injector)

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
	setup := func(t *testing.T) (*WindsorEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupWindsorEnvMocks(t)
		printer := NewWindsorEnvPrinter(mocks.Injector)
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		printer, _ := setup(t)

		// When Initialize is called
		err := printer.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Initialize returned error: %v", err)
		}

		// And secretsProviders should be populated
		if len(printer.secretsProviders) != 1 {
			t.Errorf("Expected 1 secrets provider, got %d", len(printer.secretsProviders))
		}
	})

	t.Run("ResolveAllError", func(t *testing.T) {
		// Given a WindsorEnvPrinter with failing ResolveAll
		injector := di.NewMockInjector()
		setupWindsorEnvMocks(t, &SetupOptions{
			Injector: injector,
		})

		// And error set for resolving secrets providers
		injector.SetResolveAllError((*secrets.SecretsProvider)(nil), fmt.Errorf("mock error"))

		// When Initialize is called
		printer := NewWindsorEnvPrinter(injector)
		err := printer.Initialize()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to resolve secrets providers") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})
}

// TestWindsorEnv_ParseAndCheckSecrets tests the parseAndCheckSecrets method
func TestWindsorEnv_ParseAndCheckSecrets(t *testing.T) {
	setup := func(t *testing.T) (*WindsorEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupWindsorEnvMocks(t)
		printer := NewWindsorEnvPrinter(mocks.Injector)
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize env: %v", err)
		}
		printer.shims = mocks.Shims
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		printer, mocks := setup(t)

		// Given a mock secrets provider that successfully parses secrets
		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
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
		printer, mocks := setup(t)

		// Given a mock secrets provider that fails to parse secrets
		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
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
		printer, _ := setup(t)

		// Given a WindsorEnvPrinter with no secrets providers
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
		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
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
		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
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
	setup := func(t *testing.T) (*WindsorEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupWindsorEnvMocks(t)
		printer := NewWindsorEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize printer: %v", err)
		}
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
