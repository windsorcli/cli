package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

// WindsorEnvMocks holds all mock objects used in Windsor environment tests
type WindsorEnvMocks struct {
	Injector      di.Injector
	ConfigHandler *config.MockConfigHandler
	Shell         *shell.MockShell
}

// setupSafeWindsorEnvMocks creates and configures mock objects for Windsor environment tests.
// It accepts an optional injector parameter and returns initialized WindsorEnvMocks.
func setupSafeWindsorEnvMocks(injector ...di.Injector) *WindsorEnvMocks {
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewMockInjector()
	}

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return filepath.FromSlash("/mock/config/root"), nil
	}
	mockConfigHandler.GetContextFunc = func() string {
		return "mock-context"
	}

	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return filepath.FromSlash("/mock/project/root"), nil
	}

	// Default behavior for GetSessionToken that matches test expectations
	mockShell.GetSessionTokenFunc = func() (string, error) {
		// If WINDSOR_SESSION_TOKEN is set in the environment, check it
		if envToken := os.Getenv("WINDSOR_SESSION_TOKEN"); envToken != "" {
			// Check for signal file if env token exists
			tokenFilePath := filepath.Join("/mock/project/root", ".windsor", ".session."+envToken)
			if _, err := stat(tokenFilePath); err == nil {
				// Signal file exists, generate new token
				return "abcdefg", nil
			}
			// Signal file doesn't exist, return environment token
			return envToken, nil
		}
		// No env token, return default mock token
		return "mock-token", nil
	}

	mockInjector.Register("configHandler", mockConfigHandler)
	mockInjector.Register("shell", mockShell)

	return &WindsorEnvMocks{
		Injector:      mockInjector,
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
	}
}

// customMockInjector is a custom injector for testing that returns non-castable objects
type customMockInjector struct {
	*di.MockInjector
}

// ResolveAll overrides the ResolveAll method to return non-castable objects
func (c *customMockInjector) ResolveAll(targetType any) ([]any, error) {
	if _, ok := targetType.(*secrets.SecretsProvider); ok {
		// Return a non-castable int
		return []any{123}, nil
	}
	return c.MockInjector.ResolveAll(targetType)
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestWindsorEnv_GetEnvVars tests the GetEnvVars method of the WindsorEnvPrinter
func TestWindsorEnv_GetEnvVars(t *testing.T) {
	originalOsLookupEnv := osLookupEnv
	defer func() {
		osLookupEnv = originalOsLookupEnv
	}()

	// Reset session token before each test
	shell.ResetSessionToken()

	// Set up mock environment variables
	t.Setenv("NO_CACHE", "")

	t.Run("Success", func(t *testing.T) {
		// Given a WindsorEnvPrinter with mock dependencies and a consistent session token
		shell.ResetSessionToken()
		mocks := setupSafeWindsorEnvMocks()
		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			return "mock-token", nil
		}

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// When GetEnvVars is called
		envVars, err := windsorEnvPrinter.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And environment variables should contain expected values
		expectedContext := "mock-context"
		if envVars["WINDSOR_CONTEXT"] != expectedContext {
			t.Errorf("Expected WINDSOR_CONTEXT to be %q, got %q", expectedContext, envVars["WINDSOR_CONTEXT"])
		}

		expectedProjectRoot := "/mock/project/root"
		if filepath.ToSlash(envVars["WINDSOR_PROJECT_ROOT"]) != expectedProjectRoot {
			t.Errorf("Expected WINDSOR_PROJECT_ROOT to be %q, got %q", expectedProjectRoot, envVars["WINDSOR_PROJECT_ROOT"])
		}

		expectedSessionToken := "mock-token"
		if envVars["WINDSOR_SESSION_TOKEN"] != expectedSessionToken {
			t.Errorf("Expected WINDSOR_SESSION_TOKEN to be %q, got %q", expectedSessionToken, envVars["WINDSOR_SESSION_TOKEN"])
		}
	})

	t.Run("ExistingSessionToken", func(t *testing.T) {
		// Given a WindsorEnvPrinter with mock dependencies and token regeneration
		shell.ResetSessionToken()
		mocks := setupSafeWindsorEnvMocks()

		mockShell := shell.NewMockShell()
		var callCount int
		mockShell.GetSessionTokenFunc = func() (string, error) {
			callCount++
			if callCount == 1 {
				return "first-token", nil
			}
			return "regenerated-token", nil
		}
		mocks.Injector.Register("shell", mockShell)

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// When GetEnvVars is called twice
		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then the first token should be returned
		firstToken := envVars["WINDSOR_SESSION_TOKEN"]
		if firstToken != "first-token" {
			t.Errorf("Expected first token to be 'first-token', got %s", firstToken)
		}

		// And when called again
		envVars, err = windsorEnvPrinter.GetEnvVars()
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
		mocks := setupSafeWindsorEnvMocks()

		mockShell := shell.NewMockShell()
		mockShell.GetSessionTokenFunc = func() (string, error) {
			return "", fmt.Errorf("mock session token error")
		}
		mocks.Injector.Register("shell", mockShell)

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// When GetEnvVars is called
		_, err := windsorEnvPrinter.GetEnvVars()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error from session token generation, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving session token") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("NoEnvironmentVarsInConfig", func(t *testing.T) {
		// Reset managed environment variables and aliases
		windsorManagedMu.Lock()
		windsorManagedEnv = []string{}
		windsorManagedAlias = []string{}
		windsorManagedMu.Unlock()

		mocks := setupSafeWindsorEnvMocks()

		// Set GetStringMap to return an empty map to simulate no environment vars in config
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return map[string]string{}
			}
			return map[string]string{}
		}

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars should not return an error: %v", err)
		}

		// Verify we still have the base environment variables
		if envVars["WINDSOR_CONTEXT"] != "mock-context" {
			t.Errorf("WINDSOR_CONTEXT should be set even when no environment vars are in config")
		}
		if filepath.ToSlash(envVars["WINDSOR_PROJECT_ROOT"]) != "/mock/project/root" {
			t.Errorf("WINDSOR_PROJECT_ROOT should be set")
		}
		if envVars["WINDSOR_SESSION_TOKEN"] == "" {
			t.Errorf("Session token should be generated")
		}

		// Verify no additional variables were added from config (since there were none)
		t.Logf("Environment variables: %v", envVars)
		if len(envVars) != 5 {
			t.Errorf("Should have five base environment variables")
		}
	})

	t.Run("EnvironmentTokenWithoutSignalFile", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()

		// Set up environment variable with a token
		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		// Mock stat to simulate no signal file
		stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Verify the environment token is used
		if envVars["WINDSOR_SESSION_TOKEN"] != "envtoken" {
			t.Errorf("Expected session token to be 'envtoken', got %s", envVars["WINDSOR_SESSION_TOKEN"])
		}
	})

	t.Run("EnvironmentTokenWithStatError", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()

		// Set up environment variable with a token
		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		// Mock stat to return an error that is not os.ErrNotExist
		stat = func(name string) (os.FileInfo, error) {
			return nil, fmt.Errorf("mock stat error")
		}

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Verify the environment token is used, since the error is not specifically ErrNotExist
		if envVars["WINDSOR_SESSION_TOKEN"] != "envtoken" {
			t.Errorf("Expected session token to be 'envtoken', got %s", envVars["WINDSOR_SESSION_TOKEN"])
		}
	})

	t.Run("EnvironmentTokenWithSignalFile", func(t *testing.T) {
		// Mock file system functions
		originalStat := stat
		defer func() {
			stat = originalStat
		}()

		stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".session.envtoken") {
				return nil, nil // File exists
			}
			return nil, os.ErrNotExist
		}

		originalOsRemoveAll := osRemoveAll
		defer func() {
			osRemoveAll = originalOsRemoveAll
		}()

		osRemoveAll = func(path string) error {
			return nil
		}

		// Mock crypto functions for predictable output
		origCryptoRandRead := cryptoRandRead
		defer func() {
			cryptoRandRead = origCryptoRandRead
		}()

		cryptoRandRead = func(b []byte) (n int, err error) {
			for i := range b {
				b[i] = byte(i % 62) // Will map to characters in charset
			}
			return len(b), nil
		}

		mocks := setupSafeWindsorEnvMocks()

		// Set up environment variable with a token
		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Verify a new token was generated (should be "abcdefg" per our mock)
		if envVars["WINDSOR_SESSION_TOKEN"] == "envtoken" {
			t.Errorf("Expected a new token to be generated, but got the environment token")
		}
		if envVars["WINDSOR_SESSION_TOKEN"] != "abcdefg" {
			t.Errorf("Expected session token to be 'abcdefg', got %s", envVars["WINDSOR_SESSION_TOKEN"])
		}
	})

	t.Run("SignalFileRemovalError", func(t *testing.T) {
		// Mock file system functions
		originalStat := stat
		defer func() {
			stat = originalStat
		}()

		stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".session.envtoken") {
				return nil, nil // File exists
			}
			return nil, os.ErrNotExist
		}

		// Mock osRemoveAll to return an error
		originalOsRemoveAll := osRemoveAll
		defer func() {
			osRemoveAll = originalOsRemoveAll
		}()

		osRemoveAll = func(path string) error {
			return fmt.Errorf("mock error removing signal file")
		}

		// Mock crypto functions for predictable output
		origCryptoRandRead := cryptoRandRead
		defer func() {
			cryptoRandRead = origCryptoRandRead
		}()

		cryptoRandRead = func(b []byte) (n int, err error) {
			for i := range b {
				b[i] = byte(i % 62) // Will map to characters in charset
			}
			return len(b), nil
		}

		mocks := setupSafeWindsorEnvMocks()

		// Set up environment variable with a token
		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		// We'll redirect stdout to discard any error output
		origStdout := os.Stdout
		os.Stdout = os.NewFile(0, os.DevNull)
		defer func() {
			os.Stdout = origStdout
		}()

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// Call should not fail (error is deferred and printed to stdout)
		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Verify a new token was generated (should be "abcdefg" per our mock)
		if envVars["WINDSOR_SESSION_TOKEN"] == "envtoken" {
			t.Errorf("Expected a new token to be generated, but got the environment token")
		}
		if envVars["WINDSOR_SESSION_TOKEN"] != "abcdefg" {
			t.Errorf("Expected session token to be 'abcdefg', got %s", envVars["WINDSOR_SESSION_TOKEN"])
		}
	})

	t.Run("ProjectRootErrorDuringEnvTokenSignalFileCheck", func(t *testing.T) {
		// Set up environment variable with a token
		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		mocks := setupSafeWindsorEnvMocks()

		// First call succeeds, second fails (during token check)
		var callCount int
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			callCount++
			return filepath.FromSlash("/mock/project/root"), nil
		}

		// Make GetSessionToken return an error during the signal file check
		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			if envToken := os.Getenv("WINDSOR_SESSION_TOKEN"); envToken != "" {
				return "", fmt.Errorf("error getting project root: mock error getting project root during token check")
			}
			return "mock-token", nil
		}

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		_, err := windsorEnvPrinter.GetEnvVars()
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		expectedErr := "error retrieving session token: error getting project root: mock error getting project root during token check"
		if err.Error() != expectedErr {
			t.Errorf("Expected error %q, got %q", expectedErr, err.Error())
		}
	})

	t.Run("RandomGenerationError", func(t *testing.T) {
		// Mock file system functions
		originalStat := stat
		defer func() {
			stat = originalStat
		}()

		stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".session.envtoken") {
				return nil, nil // File exists
			}
			return nil, os.ErrNotExist
		}

		// Set up environment variable to trigger token regeneration
		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		mocks := setupSafeWindsorEnvMocks()

		// Make the shell's GetSessionToken return an error when regenerating token
		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			// Check if we are being called for the environment token check
			if envToken := os.Getenv("WINDSOR_SESSION_TOKEN"); envToken != "" {
				// We are doing the check for the environment token
				tokenFilePath := filepath.Join("/mock/project/root", ".windsor", ".session."+envToken)
				if _, err := stat(tokenFilePath); err == nil {
					// Signal file exists, mock error during regeneration
					return "", fmt.Errorf("mock random generation error during token regeneration")
				}
				return envToken, nil
			}
			return "mock-token", nil
		}

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// This should trigger an error in token regeneration
		_, err := windsorEnvPrinter.GetEnvVars()
		if err == nil {
			t.Fatal("Expected error from random generation during token regeneration, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving session token") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock shell error")
		}

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		_, err := windsorEnvPrinter.GetEnvVars()
		expectedErrorMessage := "error retrieving project root: mock shell error"
		if err == nil || err.Error() != expectedErrorMessage {
			t.Errorf("Expected error %q, got %v", expectedErrorMessage, err)
		}
	})

	t.Run("ProjectRootErrorDuringTokenCheck", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()

		// Set up environment variable with a token to trigger the token check code path
		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		// Make GetSessionToken return an error during the project root check
		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			if envToken := os.Getenv("WINDSOR_SESSION_TOKEN"); envToken != "" {
				return "", fmt.Errorf("error getting project root: mock shell error during token check")
			}
			return "mock-token", nil
		}

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		_, err := windsorEnvPrinter.GetEnvVars()
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		expectedErr := "error retrieving session token: error getting project root: mock shell error during token check"
		if err.Error() != expectedErr {
			t.Errorf("Expected error %q, got %q", expectedErr, err.Error())
		}
	})

	t.Run("ComprehensiveEnvironmentTokenTest", func(t *testing.T) {
		// Mock file system functions to handle various cases
		originalStat := stat
		defer func() {
			stat = originalStat
		}()

		stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".session.testtoken") {
				return nil, nil // Session file exists
			}
			return nil, os.ErrNotExist
		}

		mocks := setupSafeWindsorEnvMocks()

		// Phase 1: No environment token present
		// Should generate a new token
		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}
		firstToken := envVars["WINDSOR_SESSION_TOKEN"]

		// Phase 2: Set environment token
		// The mock should return this token since no signal file exists for it
		t.Setenv("WINDSOR_SESSION_TOKEN", "testtoken")

		// Update the mock to handle the testtoken case
		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			if envToken := os.Getenv("WINDSOR_SESSION_TOKEN"); envToken != "" {
				// Our testtoken has a signal file
				tokenFilePath := filepath.Join("/mock/project/root", ".windsor", ".session."+envToken)
				if _, err := stat(tokenFilePath); err == nil {
					return "newtoken", nil // Return a different token to show regeneration
				}
				return envToken, nil
			}
			return "mock-token", nil
		}

		// Now the test token should come back
		envVars, err = windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error in phase 2: %v", err)
		}

		secondToken := envVars["WINDSOR_SESSION_TOKEN"]
		if secondToken != "newtoken" {
			t.Errorf("Expected token 'newtoken', got %q", secondToken)
		}

		if secondToken == firstToken {
			t.Errorf("Second token %q should be different from the first token %q", secondToken, firstToken)
		}
	})

	t.Run("RandomErrorDuringSignalFileRegeneration", func(t *testing.T) {
		// Mock file system functions
		originalStat := stat
		defer func() {
			stat = originalStat
		}()

		stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".session.testtoken") {
				return nil, nil // File exists
			}
			return nil, os.ErrNotExist
		}

		// Set up environment variable to trigger token regeneration
		t.Setenv("WINDSOR_SESSION_TOKEN", "testtoken")

		mocks := setupSafeWindsorEnvMocks()

		// Make the shell's GetSessionToken return an error when regenerating token
		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			// Check if we are being called for the environment token check
			if envToken := os.Getenv("WINDSOR_SESSION_TOKEN"); envToken != "" {
				// We are doing the check for the environment token
				tokenFilePath := filepath.Join("/mock/project/root", ".windsor", ".session."+envToken)
				if _, err := stat(tokenFilePath); err == nil {
					// Signal file exists, mock error during regeneration
					return "", fmt.Errorf("mock random generation error during token regeneration")
				}
				return envToken, nil
			}
			return "mock-token", nil
		}

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// This should trigger an error in token regeneration
		_, err := windsorEnvPrinter.GetEnvVars()
		if err == nil {
			t.Fatal("Expected error from random generation during token regeneration, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving session token") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("DifferentContextDisablesCache", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()

		// Set up test environment
		envVarKey := "TEST_VAR_WITH_SECRET"
		envVarValue := "value with ${{ secrets.mySecret }}"

		// Save original environment values and restore them after test
		originalEnvContext := os.Getenv("WINDSOR_CONTEXT")
		originalEnvToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		originalTestVar := os.Getenv(envVarKey)
		originalNoCache := os.Getenv("NO_CACHE")

		// Setting NO_CACHE=true should disable the cache
		t.Setenv("NO_CACHE", "true")
		t.Setenv("WINDSOR_CONTEXT", "different-context")
		t.Setenv("WINDSOR_SESSION_TOKEN", "")
		t.Setenv(envVarKey, "existing-value")

		defer func() {
			os.Setenv("WINDSOR_CONTEXT", originalEnvContext)
			os.Setenv("WINDSOR_SESSION_TOKEN", originalEnvToken)
			os.Setenv(envVarKey, originalTestVar)
			os.Setenv("NO_CACHE", originalNoCache)
		}()

		// Set up mock config handler to return environment variables
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return map[string]string{
					envVarKey: envVarValue,
				}
			}
			return map[string]string{}
		}

		// Mock secrets provider that will be called regardless of cache
		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			if input == envVarValue {
				return "resolved-value", nil
			}
			return input, nil
		}

		// Create WindsorEnvPrinter with mock injector
		mockInjector := mocks.Injector
		mockInjector.Register("secretsProvider", mockSecretsProvider)
		windsorEnvPrinter := NewWindsorEnvPrinter(mockInjector)
		err := windsorEnvPrinter.Initialize()
		if err != nil {
			t.Fatalf("Initialize should not return an error: %v", err)
		}

		// Make secretsProviders accessible to the test
		windsorEnvPrinter.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// Get environment variables
		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars should not return an error: %v", err)
		}

		// Verify the variable was resolved despite having an existing value in the environment
		// This confirms that NO_CACHE=true worked as expected
		if envVars[envVarKey] != "resolved-value" {
			t.Errorf("Environment variable should be resolved even with existing value when NO_CACHE=true")
		}
	})

	t.Run("NoCacheEnvVarDisablesCache", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()

		// Override random generation to avoid token generation errors
		origCryptoRandRead := cryptoRandRead
		cryptoRandRead = func(b []byte) (n int, err error) {
			for i := range b {
				b[i] = byte(i%26) + 'a' // Generate predictable letters
			}
			return len(b), nil
		}
		defer func() {
			cryptoRandRead = origCryptoRandRead
		}()

		// Set up test environment variables
		envVarKey := "TEST_VAR_WITH_SECRET"
		envVarValue := "value with ${{ secrets.mySecret }}"

		// Save original environment values and restore them after test
		originalEnvContext := os.Getenv("WINDSOR_CONTEXT")
		originalEnvToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		originalTestVar := os.Getenv(envVarKey)
		originalNoCache := os.Getenv("NO_CACHE")

		// Setting NO_CACHE=true should disable the cache
		t.Setenv("NO_CACHE", "true")
		t.Setenv("WINDSOR_CONTEXT", "") // Use same context to test NO_CACHE specifically
		t.Setenv("WINDSOR_SESSION_TOKEN", "")
		t.Setenv(envVarKey, "existing-value-should-be-ignored")

		defer func() {
			os.Setenv("WINDSOR_CONTEXT", originalEnvContext)
			os.Setenv("WINDSOR_SESSION_TOKEN", originalEnvToken)
			os.Setenv(envVarKey, originalTestVar)
			os.Setenv("NO_CACHE", originalNoCache)
		}()

		// Configure mock config handler
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return map[string]string{
					envVarKey: envVarValue,
				}
			}
			return map[string]string{}
		}

		// Mock secrets provider that will resolve the secret
		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			if input == envVarValue {
				return "resolved-value", nil
			}
			return input, nil
		}

		// Create WindsorEnvPrinter with mock injector
		mockInjector := mocks.Injector
		mockInjector.Register("secretsProvider", mockSecretsProvider)
		windsorEnvPrinter := NewWindsorEnvPrinter(mockInjector)
		err := windsorEnvPrinter.Initialize()
		if err != nil {
			t.Fatalf("Initialize should not return an error: %v", err)
		}

		// Make secretsProviders accessible to the test
		windsorEnvPrinter.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// Get environment variables
		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars should not return an error: %v", err)
		}

		// Verify the variable was resolved despite having an existing value in the environment
		// This confirms that NO_CACHE=true worked as expected
		if envVars[envVarKey] != "resolved-value" {
			t.Errorf("Environment variable should be resolved even with existing value when NO_CACHE=true")
		}
	})

	t.Run("RegularEnvironmentVarsWithoutSecrets", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()

		// Override random generation to avoid token generation errors
		origCryptoRandRead := cryptoRandRead
		cryptoRandRead = func(b []byte) (n int, err error) {
			for i := range b {
				b[i] = byte(i%26) + 'a' // Generate predictable letters
			}
			return len(b), nil
		}
		defer func() {
			cryptoRandRead = origCryptoRandRead
		}()

		// Set up test environment variables with regular values (no secret placeholders)
		regularVarKey1 := "REGULAR_ENV_VAR1"
		regularVarValue1 := "regular value 1"
		regularVarKey2 := "REGULAR_ENV_VAR2"
		regularVarValue2 := "regular value 2"

		// Save original environment values
		originalEnvContext := os.Getenv("WINDSOR_CONTEXT")
		originalEnvToken := os.Getenv("WINDSOR_SESSION_TOKEN")

		// Clean environment for test
		t.Setenv("WINDSOR_CONTEXT", "")
		t.Setenv("WINDSOR_SESSION_TOKEN", "")

		defer func() {
			os.Setenv("WINDSOR_CONTEXT", originalEnvContext)
			os.Setenv("WINDSOR_SESSION_TOKEN", originalEnvToken)
		}()

		// Configure mock config handler with regular environment variables
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return map[string]string{
					regularVarKey1: regularVarValue1,
					regularVarKey2: regularVarValue2,
					"WITH_SECRET":  "${{ secrets.mySecret }}", // Include one with secret to test both branches
				}
			}
			return map[string]string{}
		}

		// Mock secrets provider
		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			if input == "${{ secrets.mySecret }}" {
				return "resolved-secret", nil
			}
			return input, nil
		}

		// Create WindsorEnvPrinter with mock injector
		mockInjector := mocks.Injector
		mockInjector.Register("secretsProvider", mockSecretsProvider)
		windsorEnvPrinter := NewWindsorEnvPrinter(mockInjector)
		err := windsorEnvPrinter.Initialize()
		if err != nil {
			t.Fatalf("Initialize should not return an error: %v", err)
		}

		// Make secretsProviders accessible to the test
		windsorEnvPrinter.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// Get environment variables
		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars should not return an error: %v", err)
		}

		// Verify the regular variables were set directly without parsing
		if envVars[regularVarKey1] != regularVarValue1 {
			t.Errorf("Regular environment variable should be set directly")
		}
		if envVars[regularVarKey2] != regularVarValue2 {
			t.Errorf("Regular environment variable should be set directly")
		}

		// Also verify that the secret was parsed correctly
		if envVars["WITH_SECRET"] != "resolved-secret" {
			t.Errorf("Environment variable with secret should be resolved")
		}
	})

	t.Run("ManagedCustomEnvironmentVars", func(t *testing.T) {
		// Save original values
		originalManagedEnv := make([]string, len(windsorManagedEnv))
		copy(originalManagedEnv, windsorManagedEnv)

		// Save original environment values
		originalEnvContext := os.Getenv("WINDSOR_CONTEXT")
		originalEnvToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		originalNoCache := os.Getenv("NO_CACHE")

		// Set environment variables for test - ensure NO_CACHE is unset
		t.Setenv("WINDSOR_CONTEXT", "")
		t.Setenv("WINDSOR_SESSION_TOKEN", "")
		t.Setenv("NO_CACHE", "")

		// Restore original environment variables after test
		defer func() {
			os.Setenv("WINDSOR_CONTEXT", originalEnvContext)
			os.Setenv("WINDSOR_SESSION_TOKEN", originalEnvToken)
			os.Setenv("NO_CACHE", originalNoCache)
		}()

		// Restore original state after test
		defer func() {
			windsorManagedMu.Lock()
			windsorManagedEnv = originalManagedEnv
			windsorManagedMu.Unlock()
		}()

		// Setup mocks
		mocks := setupSafeWindsorEnvMocks()

		// Override random generation to avoid token generation errors
		origCryptoRandRead := cryptoRandRead
		cryptoRandRead = func(b []byte) (n int, err error) {
			for i := range b {
				b[i] = byte(i%26) + 'a' // Generate predictable letters
			}
			return len(b), nil
		}
		defer func() {
			cryptoRandRead = origCryptoRandRead
		}()

		// Set up mock config handler to return custom environment variables
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return map[string]string{
					"CUSTOM_ENV_VAR1": "value1",
					"CUSTOM_ENV_VAR2": "value2",
				}
			}
			return map[string]string{}
		}

		// Track custom variables
		windsorManagedMu.Lock()
		windsorManagedEnv = []string{"CUSTOM_ENV_VAR1", "CUSTOM_ENV_VAR2"}
		windsorManagedMu.Unlock()

		// Create WindsorEnvPrinter and initialize it
		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		err := windsorEnvPrinter.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize WindsorEnvPrinter: %v", err)
		}

		// Get environment variables
		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars should not return an error: %v", err)
		}

		// Verify custom variables are in the environment variables map
		if envVars["CUSTOM_ENV_VAR1"] != "value1" {
			t.Errorf("CUSTOM_ENV_VAR1 should be set to 'value1'")
		}
		if envVars["CUSTOM_ENV_VAR2"] != "value2" {
			t.Errorf("CUSTOM_ENV_VAR2 should be set to 'value2'")
		}

		// Verify that WINDSOR_MANAGED_ENV includes our custom variables and Windsor prefixed vars
		managedEnvList := envVars["WINDSOR_MANAGED_ENV"]
		expectedVars := []string{
			"CUSTOM_ENV_VAR1",
			"CUSTOM_ENV_VAR2",
			"WINDSOR_CONTEXT",
			"WINDSOR_PROJECT_ROOT",
			"WINDSOR_SESSION_TOKEN",
			"WINDSOR_MANAGED_ENV",
			"WINDSOR_MANAGED_ALIAS",
		}
		for _, v := range expectedVars {
			if !strings.Contains(managedEnvList, v) {
				t.Errorf("WINDSOR_MANAGED_ENV should contain %s", v)
			}
		}
	})

	t.Run("ErrorValueBypassesCache", func(t *testing.T) {
		// Save original functions
		originalCryptoRandRead := cryptoRandRead

		// Restore original function after test
		defer func() {
			cryptoRandRead = originalCryptoRandRead
		}()

		// Set up test mocks
		mocks := setupSafeWindsorEnvMocks()

		// Override random generation to avoid token generation errors
		cryptoRandRead = func(b []byte) (n int, err error) {
			for i := range b {
				b[i] = byte(i%26) + 'a' // Generate predictable letters
			}
			return len(b), nil
		}

		// Set up test environment variables
		errorVarKey := "TEST_VAR_WITH_ERROR"
		normalVarKey := "TEST_VAR_NORMAL"
		errorVarValue := "value with ${{ secrets.errorSecret }}"
		normalVarValue := "value with ${{ secrets.normalSecret }}"

		// Save original environment values
		originalEnvContext := os.Getenv("WINDSOR_CONTEXT")
		originalEnvToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		originalErrorVar := os.Getenv(errorVarKey)
		originalNormalVar := os.Getenv(normalVarKey)
		originalNoCache := os.Getenv("NO_CACHE")

		// Explicitly set NO_CACHE=false to enable caching
		os.Setenv("NO_CACHE", "false")
		os.Setenv("WINDSOR_CONTEXT", "")
		os.Setenv("WINDSOR_SESSION_TOKEN", "")

		// Set the existing values - one with error and one normal but with a pattern that will be cached
		os.Setenv(errorVarKey, "<ERROR: failed to parse: secrets.errorSecret>")
		os.Setenv(normalVarKey, "cached-normal-value")

		// Restore original values after test
		defer func() {
			os.Setenv("WINDSOR_CONTEXT", originalEnvContext)
			os.Setenv("WINDSOR_SESSION_TOKEN", originalEnvToken)
			os.Setenv(errorVarKey, originalErrorVar)
			os.Setenv(normalVarKey, originalNormalVar)
			os.Setenv("NO_CACHE", originalNoCache)
		}()

		// Verify caching is enabled in this test
		if !shouldUseCache() {
			t.Fatalf("shouldUseCache() returned false, expected true with NO_CACHE=false")
		}

		// Configure mock config handler
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return map[string]string{
					errorVarKey:  errorVarValue,
					normalVarKey: normalVarValue,
				}
			}
			return map[string]string{}
		}

		// Mock secrets provider that will resolve the secrets
		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			if input == errorVarValue {
				return "resolved-error-value", nil
			}
			if input == normalVarValue {
				return "resolved-normal-value", nil
			}
			return input, nil
		}

		// Create WindsorEnvPrinter with mock injector
		mockInjector := mocks.Injector
		mockInjector.Register("secretsProvider", mockSecretsProvider)
		windsorEnvPrinter := NewWindsorEnvPrinter(mockInjector)

		// Initialize the WindsorEnvPrinter
		err := windsorEnvPrinter.Initialize()
		if err != nil {
			t.Fatalf("Initialize returned error: %v", err)
		}

		// Make secretsProviders accessible to the test
		windsorEnvPrinter.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// Get environment variables
		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned error: %v", err)
		}

		// Check that the variable with <ERROR> was re-resolved
		// despite caching being enabled
		if got, want := envVars[errorVarKey], "resolved-error-value"; got != want {
			t.Errorf("Environment variable with <ERROR> was not properly re-resolved: got %q, want %q", got, want)
		}

		// Check that we got the expected keys in the results
		expectedKeys := []string{
			"WINDSOR_CONTEXT",
			"WINDSOR_PROJECT_ROOT",
			"WINDSOR_SESSION_TOKEN",
			"WINDSOR_MANAGED_ENV",
			"WINDSOR_MANAGED_ALIAS",
			errorVarKey,
		}

		// Verify all expected keys are present
		for _, key := range expectedKeys {
			if _, exists := envVars[key]; !exists {
				t.Errorf("Expected key %q missing from results", key)
			}
		}

		// Normal variable should not be in results because it's cached
		if _, exists := envVars[normalVarKey]; exists {
			t.Errorf("Cached normal variable %q should not be in results", normalVarKey)
		}
	})

	t.Run("ManagedEnv", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeWindsorEnvMocks()

		// Create a test environment
		env := NewWindsorEnvPrinter(mocks.Injector)
		env.Initialize()

		// Set up managed environment
		env.SetManagedEnv("test-env")

		// Get environment variables
		vars, err := env.GetEnvVars()

		// Verify the result
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify managed environment contains the test-env and Windsor prefixed vars
		expectedVars := append([]string{"test-env"}, WindsorPrefixedVars...)
		managedEnvVars := strings.Split(vars["WINDSOR_MANAGED_ENV"], ",")

		for _, expected := range expectedVars {
			found := false
			for _, actual := range managedEnvVars {
				if actual == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected WINDSOR_MANAGED_ENV to contain %q", expected)
			}
		}
	})

	t.Run("CachedVariableAddedToManagedEnv", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeWindsorEnvMocks()

		// Set up test environment variables
		cachedVarKey := "CACHED_VAR"
		cachedVarValue := "value with ${{ secrets.cachedSecret }}"
		secretVarKey := "SECRET_VAR"
		secretVarValue := "value with ${{ secrets.mySecret }}"

		// Save original environment values
		originalEnvContext := os.Getenv("WINDSOR_CONTEXT")
		originalEnvToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		originalCachedVar := os.Getenv(cachedVarKey)
		originalSecretVar := os.Getenv(secretVarKey)
		originalNoCache := os.Getenv("NO_CACHE")

		// Set up environment with cached variable
		t.Setenv("NO_CACHE", "false")
		t.Setenv("WINDSOR_CONTEXT", "")
		t.Setenv("WINDSOR_SESSION_TOKEN", "")
		t.Setenv(cachedVarKey, "cached-value")

		defer func() {
			os.Setenv("WINDSOR_CONTEXT", originalEnvContext)
			os.Setenv("WINDSOR_SESSION_TOKEN", originalEnvToken)
			os.Setenv(cachedVarKey, originalCachedVar)
			os.Setenv(secretVarKey, originalSecretVar)
			os.Setenv("NO_CACHE", originalNoCache)
		}()

		// Configure mock config handler
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return map[string]string{
					cachedVarKey: cachedVarValue,
					secretVarKey: secretVarValue,
				}
			}
			return map[string]string{}
		}

		// Mock secrets provider
		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			if input == secretVarValue {
				return "resolved-secret", nil
			}
			if input == cachedVarValue {
				return "resolved-cached", nil
			}
			return input, nil
		}

		// Create WindsorEnvPrinter with mock injector
		mockInjector := mocks.Injector
		mockInjector.Register("secretsProvider", mockSecretsProvider)
		windsorEnvPrinter := NewWindsorEnvPrinter(mockInjector)
		err := windsorEnvPrinter.Initialize()
		if err != nil {
			t.Fatalf("Initialize returned error: %v", err)
		}

		// Make secretsProviders accessible to the test
		windsorEnvPrinter.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// Get environment variables
		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned error: %v", err)
		}

		// Verify cached variable is not in returned environment variables
		if _, exists := envVars[cachedVarKey]; exists {
			t.Errorf("Cached variable %q should not be in returned environment variables", cachedVarKey)
		}

		// Verify secret variable is in returned environment variables
		if envVars[secretVarKey] != "resolved-secret" {
			t.Errorf("Secret variable should be resolved and returned")
		}

		// Verify both variables are in managed env list
		managedEnvList := envVars["WINDSOR_MANAGED_ENV"]
		expectedVars := []string{cachedVarKey, secretVarKey}
		for _, v := range expectedVars {
			if !strings.Contains(managedEnvList, v) {
				t.Errorf("WINDSOR_MANAGED_ENV should contain %q", v)
			}
		}
	})

	t.Run("ExistingVariableNotInManagedEnv", func(t *testing.T) {
		// Reset managed environment variables
		windsorManagedMu.Lock()
		windsorManagedEnv = []string{}
		windsorManagedAlias = []string{}
		windsorManagedMu.Unlock()

		// Setup mocks
		mocks := setupSafeWindsorEnvMocks()

		// Set up test environment variables
		envVarKey := "EXISTING_VAR"
		envVarValue := "regular value"

		// Save original environment values
		originalEnvContext := os.Getenv("WINDSOR_CONTEXT")
		originalEnvToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		originalEnvVar := os.Getenv(envVarKey)
		originalManagedEnv := os.Getenv("WINDSOR_MANAGED_ENV")
		originalNoCache := os.Getenv("NO_CACHE")

		// Set up environment with variable but not in managed env
		t.Setenv("NO_CACHE", "false")
		t.Setenv("WINDSOR_CONTEXT", "")
		t.Setenv("WINDSOR_SESSION_TOKEN", "")
		t.Setenv(envVarKey, "existing-value")
		os.Unsetenv("WINDSOR_MANAGED_ENV")

		defer func() {
			os.Setenv("WINDSOR_CONTEXT", originalEnvContext)
			os.Setenv("WINDSOR_SESSION_TOKEN", originalEnvToken)
			os.Setenv(envVarKey, originalEnvVar)
			os.Setenv("WINDSOR_MANAGED_ENV", originalManagedEnv)
			os.Setenv("NO_CACHE", originalNoCache)
		}()

		// Configure mock config handler
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return map[string]string{
					envVarKey: envVarValue,
				}
			}
			return map[string]string{}
		}

		// Create WindsorEnvPrinter with mock injector
		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		err := windsorEnvPrinter.Initialize()
		if err != nil {
			t.Fatalf("Initialize returned error: %v", err)
		}

		// Get environment variables
		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned error: %v", err)
		}

		// Verify variable is in returned environment variables
		if envVars[envVarKey] != envVarValue {
			t.Errorf("Variable should be in returned environment variables with value %q, got %q", envVarValue, envVars[envVarKey])
		}

		// Verify variable is added to managed env list
		managedEnvList := envVars["WINDSOR_MANAGED_ENV"]
		if !strings.Contains(managedEnvList, envVarKey) {
			t.Errorf("WINDSOR_MANAGED_ENV should contain %q", envVarKey)
		}
	})

	t.Run("ExistingVariableInManagedEnv", func(t *testing.T) {
		// Reset managed environment variables
		windsorManagedMu.Lock()
		windsorManagedEnv = []string{}
		windsorManagedAlias = []string{}
		windsorManagedMu.Unlock()

		// Setup mocks
		mocks := setupSafeWindsorEnvMocks()

		// Set up test environment variables
		envVarKey := "EXISTING_VAR"
		envVarValue := "regular value"

		// Save original environment values
		originalEnvContext := os.Getenv("WINDSOR_CONTEXT")
		originalEnvToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		originalEnvVar := os.Getenv(envVarKey)
		originalManagedEnv := os.Getenv("WINDSOR_MANAGED_ENV")
		originalNoCache := os.Getenv("NO_CACHE")

		// Set up environment with variable already in managed env
		t.Setenv("NO_CACHE", "false")
		t.Setenv("WINDSOR_CONTEXT", "")
		t.Setenv("WINDSOR_SESSION_TOKEN", "")
		t.Setenv(envVarKey, "existing-value")
		t.Setenv("WINDSOR_MANAGED_ENV", envVarKey)

		defer func() {
			os.Setenv("WINDSOR_CONTEXT", originalEnvContext)
			os.Setenv("WINDSOR_SESSION_TOKEN", originalEnvToken)
			os.Setenv(envVarKey, originalEnvVar)
			os.Setenv("WINDSOR_MANAGED_ENV", originalManagedEnv)
			os.Setenv("NO_CACHE", originalNoCache)
		}()

		// Configure mock config handler
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return map[string]string{
					envVarKey: envVarValue,
				}
			}
			return map[string]string{}
		}

		// Create WindsorEnvPrinter with mock injector
		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		err := windsorEnvPrinter.Initialize()
		if err != nil {
			t.Fatalf("Initialize returned error: %v", err)
		}

		// Set managed environment variable
		windsorEnvPrinter.SetManagedEnv(envVarKey)

		// Get environment variables
		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned error: %v", err)
		}

		// Verify variable is in returned environment variables
		if envVars[envVarKey] != envVarValue {
			t.Errorf("Variable should be in returned environment variables with value %q, got %q", envVarValue, envVars[envVarKey])
		}

		// Verify variable is still in managed env list
		managedEnvList := envVars["WINDSOR_MANAGED_ENV"]
		if !strings.Contains(managedEnvList, envVarKey) {
			t.Errorf("WINDSOR_MANAGED_ENV should contain %q", envVarKey)
		}
	})

	t.Run("ManagedEnvExistsWithSecretPlaceholder", func(t *testing.T) {
		// Reset managed environment variables
		windsorManagedMu.Lock()
		windsorManagedEnv = []string{}
		windsorManagedAlias = []string{}
		windsorManagedMu.Unlock()

		// Setup mocks
		mocks := setupSafeWindsorEnvMocks()

		// Set up test environment variables
		envVarKey := "SECRET_VAR"
		envVarValue := "value with ${{ secrets.mySecret }}"

		// Save original environment values
		originalEnvContext := os.Getenv("WINDSOR_CONTEXT")
		originalEnvToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		originalManagedEnv := os.Getenv("WINDSOR_MANAGED_ENV")
		originalNoCache := os.Getenv("NO_CACHE")
		originalEnvVar := os.Getenv(envVarKey)

		// Set up environment with WINDSOR_MANAGED_ENV already set
		t.Setenv("NO_CACHE", "true") // Disable caching to force secret resolution
		t.Setenv("WINDSOR_CONTEXT", "")
		t.Setenv("WINDSOR_SESSION_TOKEN", "")
		t.Setenv("WINDSOR_MANAGED_ENV", envVarKey)
		t.Setenv(envVarKey, "existing-value")

		defer func() {
			os.Setenv("WINDSOR_CONTEXT", originalEnvContext)
			os.Setenv("WINDSOR_SESSION_TOKEN", originalEnvToken)
			os.Setenv("WINDSOR_MANAGED_ENV", originalManagedEnv)
			os.Setenv("NO_CACHE", originalNoCache)
			os.Setenv(envVarKey, originalEnvVar)
		}()

		// Configure mock config handler
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return map[string]string{
					envVarKey: envVarValue,
				}
			}
			return map[string]string{}
		}

		// Mock secrets provider
		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			if input == envVarValue {
				return "resolved-secret", nil
			}
			return input, nil
		}

		// Create WindsorEnvPrinter with mock injector
		mockInjector := mocks.Injector
		mockInjector.Register("secretsProvider", mockSecretsProvider)
		windsorEnvPrinter := NewWindsorEnvPrinter(mockInjector)
		err := windsorEnvPrinter.Initialize()
		if err != nil {
			t.Fatalf("Initialize returned error: %v", err)
		}

		// Make secretsProviders accessible to the test
		windsorEnvPrinter.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// Set managed environment variable
		windsorEnvPrinter.SetManagedEnv(envVarKey)

		// Get environment variables
		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned error: %v", err)
		}

		// Verify variable is in returned environment variables with resolved secret
		if envVars[envVarKey] != "resolved-secret" {
			t.Errorf("Variable should be in returned environment variables with resolved secret, got %q", envVars[envVarKey])
		}

		// Verify variable is added to managed env list
		managedEnvList := envVars["WINDSOR_MANAGED_ENV"]
		if !strings.Contains(managedEnvList, envVarKey) {
			t.Errorf("WINDSOR_MANAGED_ENV should contain %q", envVarKey)
		}
	})
}

// TestWindsorEnv_PostEnvHook tests the PostEnvHook method of the WindsorEnvPrinter
func TestWindsorEnv_PostEnvHook(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a WindsorEnvPrinter
		windsorEnvPrinter := &WindsorEnvPrinter{}

		// When PostEnvHook is called
		err := windsorEnvPrinter.PostEnvHook()

		// Then no error should be returned
		if err != nil {
			t.Errorf("PostEnvHook() returned an error: %v", err)
		}
	})
}

// TestWindsorEnv_Print tests the Print method of the WindsorEnvPrinter
func TestWindsorEnv_Print(t *testing.T) {
	originalStat := stat
	defer func() {
		stat = originalStat
	}()

	t.Run("Success", func(t *testing.T) {
		// Given a WindsorEnvPrinter with mock dependencies
		mocks := setupSafeWindsorEnvMocks()
		mockInjector := mocks.Injector
		windsorEnvPrinter := NewWindsorEnvPrinter(mockInjector)
		windsorEnvPrinter.Initialize()

		// And a mock file system with existing Windsor config
		stat = func(name string) (os.FileInfo, error) {
			if filepath.Clean(name) == filepath.FromSlash("/mock/config/root/.windsor/config") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// And a mock PrintEnvVars function
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) {
			capturedEnvVars = envVars
		}

		// When Print is called
		err := windsorEnvPrinter.Print()

		// Then no error should be returned
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// And core Windsor environment variables should be set correctly
		if capturedEnvVars["WINDSOR_CONTEXT"] != "mock-context" {
			t.Errorf("WINDSOR_CONTEXT = %q, want %q", capturedEnvVars["WINDSOR_CONTEXT"], "mock-context")
		}

		if capturedEnvVars["WINDSOR_PROJECT_ROOT"] != filepath.FromSlash("/mock/project/root") {
			t.Errorf("WINDSOR_PROJECT_ROOT = %q, want %q", capturedEnvVars["WINDSOR_PROJECT_ROOT"], filepath.FromSlash("/mock/project/root"))
		}

		if capturedEnvVars["WINDSOR_SESSION_TOKEN"] == "" {
			t.Errorf("WINDSOR_SESSION_TOKEN is empty")
		}

		// And WINDSOR_MANAGED_ENV should include core Windsor variables
		managedEnv := capturedEnvVars["WINDSOR_MANAGED_ENV"]
		coreVars := []string{"WINDSOR_CONTEXT", "WINDSOR_PROJECT_ROOT", "WINDSOR_SESSION_TOKEN", "WINDSOR_MANAGED_ENV", "WINDSOR_MANAGED_ALIAS"}
		for _, v := range coreVars {
			if !strings.Contains(managedEnv, v) {
				t.Errorf("WINDSOR_MANAGED_ENV should contain %q, got %q", v, managedEnv)
			}
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Given a WindsorEnvPrinter with failing project root lookup
		mocks := setupSafeWindsorEnvMocks()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock project root error")
		}

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// When Print is called
		err := windsorEnvPrinter.Print()

		// Then an error should be returned
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "mock project root error") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

// TestWindsorEnv_Initialize tests the Initialize method of the WindsorEnvPrinter
func TestWindsorEnv_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a WindsorEnvPrinter with mock dependencies
		injector := di.NewMockInjector()
		mockShell := shell.NewMockShell()
		mockConfigHandler := config.NewMockConfigHandler()
		mockSecretsProvider := secrets.NewMockSecretsProvider(injector)

		injector.Register("shell", mockShell)
		injector.Register("configHandler", mockConfigHandler)
		injector.Register("secretsProvider", mockSecretsProvider)

		windsorEnv := NewWindsorEnvPrinter(injector)

		// When Initialize is called
		err := windsorEnv.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Initialize returned error: %v", err)
		}

		// And secretsProviders should be populated
		if len(windsorEnv.secretsProviders) != 1 {
			t.Errorf("Expected 1 secrets provider, got %d", len(windsorEnv.secretsProviders))
		}
	})

	t.Run("BaseInitializationError", func(t *testing.T) {
		// Given a WindsorEnvPrinter with no registered components
		injector := di.NewMockInjector()

		// When Initialize is called
		windsorEnv := NewWindsorEnvPrinter(injector)
		err := windsorEnv.Initialize()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to initialize BaseEnvPrinter") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("ResolveAllError", func(t *testing.T) {
		// Given a WindsorEnvPrinter with failing ResolveAll
		mockInjector := di.NewMockInjector()
		mockShell := shell.NewMockShell()
		mockConfigHandler := config.NewMockConfigHandler()

		mockInjector.Register("shell", mockShell)
		mockInjector.Register("configHandler", mockConfigHandler)
		mockInjector.SetResolveAllError((*secrets.SecretsProvider)(nil), fmt.Errorf("error resolving secrets providers"))

		// When Initialize is called
		windsorEnv := NewWindsorEnvPrinter(mockInjector)
		err := windsorEnv.Initialize()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to resolve secrets providers") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("CastError", func(t *testing.T) {
		// Given a WindsorEnvPrinter with non-castable secrets provider
		customInjector := &customMockInjector{
			MockInjector: di.NewMockInjector(),
		}

		mockShell := shell.NewMockShell()
		mockConfigHandler := config.NewMockConfigHandler()

		customInjector.Register("shell", mockShell)
		customInjector.Register("configHandler", mockConfigHandler)

		// When Initialize is called
		windsorEnv := NewWindsorEnvPrinter(customInjector)
		err := windsorEnv.Initialize()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to cast instance to SecretsProvider") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})
}

// TestWindsorEnv_ParseAndCheckSecrets tests the parseAndCheckSecrets method
func TestWindsorEnv_ParseAndCheckSecrets(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a WindsorEnvPrinter with a mock secrets provider
		mockInjector := di.NewMockInjector()
		mockSecretsProvider := secrets.NewMockSecretsProvider(mockInjector)
		mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			if input == "value with ${{ secrets.mySecret }}" {
				return "value with resolved-secret", nil
			}
			return input, nil
		}

		windsorEnv := NewWindsorEnvPrinter(mockInjector)
		windsorEnv.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// When parseAndCheckSecrets is called
		result := windsorEnv.parseAndCheckSecrets("value with ${{ secrets.mySecret }}")

		// Then the secret should be resolved
		if result != "value with resolved-secret" {
			t.Errorf("Expected 'value with resolved-secret', got %q", result)
		}
	})

	t.Run("SecretsProviderError", func(t *testing.T) {
		// Given a WindsorEnvPrinter with a failing secrets provider
		mockInjector := di.NewMockInjector()
		mockSecretsProvider := secrets.NewMockSecretsProvider(mockInjector)
		mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			return "", fmt.Errorf("error parsing secrets")
		}

		windsorEnv := NewWindsorEnvPrinter(mockInjector)
		windsorEnv.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// When parseAndCheckSecrets is called
		result := windsorEnv.parseAndCheckSecrets("value with ${{ secrets.mySecret }}")

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
		mockInjector := di.NewMockInjector()
		windsorEnv := NewWindsorEnvPrinter(mockInjector)
		windsorEnv.secretsProviders = []secrets.SecretsProvider{}

		// When parseAndCheckSecrets is called
		result := windsorEnv.parseAndCheckSecrets("value with ${{ secrets.mySecret }}")

		// Then an error message should be returned
		if result != "<ERROR: No secrets providers configured>" {
			t.Errorf("Expected '<ERROR: No secrets providers configured>', got %q", result)
		}
	})

	t.Run("UnparsedSecrets", func(t *testing.T) {
		// Given a WindsorEnvPrinter with a non-recognizing secrets provider
		mockInjector := di.NewMockInjector()
		mockSecretsProvider := secrets.NewMockSecretsProvider(mockInjector)
		mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			return input, nil
		}

		windsorEnv := NewWindsorEnvPrinter(mockInjector)
		windsorEnv.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// When parseAndCheckSecrets is called
		result := windsorEnv.parseAndCheckSecrets("value with ${{ secrets.mySecret }}")

		// Then an error message should be returned
		if !strings.Contains(result, "<ERROR: failed to parse") {
			t.Errorf("Expected error message containing 'failed to parse', got %q", result)
		}
		if !strings.Contains(result, "secrets.mySecret") {
			t.Errorf("Expected error message to contain 'secrets.mySecret', got %q", result)
		}
	})

	t.Run("MultipleUnparsedSecrets", func(t *testing.T) {
		// Given a WindsorEnvPrinter with a non-recognizing secrets provider
		mockInjector := di.NewMockInjector()
		mockSecretsProvider := secrets.NewMockSecretsProvider(mockInjector)
		mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			return input, nil
		}

		windsorEnv := NewWindsorEnvPrinter(mockInjector)
		windsorEnv.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// When parseAndCheckSecrets is called with multiple secrets
		result := windsorEnv.parseAndCheckSecrets("value with ${{ secrets.secretA }} and ${{ secrets.secretB }}")

		// Then an error message should be returned
		if !strings.Contains(result, "<ERROR: failed to parse") {
			t.Errorf("Expected error message containing 'failed to parse', got %q", result)
		}
		if !strings.Contains(result, "secrets.secretA, secrets.secretB") {
			t.Errorf("Expected error message to contain 'secrets.secretA, secrets.secretB', got %q", result)
		}
	})
}
