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

type WindsorEnvMocks struct {
	Injector      di.Injector
	ConfigHandler *config.MockConfigHandler
	Shell         *shell.MockShell
}

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
func (c *customMockInjector) ResolveAll(targetType interface{}) ([]interface{}, error) {
	if _, ok := targetType.(*secrets.SecretsProvider); ok {
		// Return a non-castable int
		return []interface{}{123}, nil
	}
	return c.MockInjector.ResolveAll(targetType)
}

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
		// Reset session token to ensure consistent behavior
		shell.ResetSessionToken()

		// Given a WindsorEnvPrinter with mock dependencies
		mocks := setupSafeWindsorEnvMocks()

		// Make the shell return a consistent mock token
		mocks.Shell.GetSessionTokenFunc = func() (string, error) {
			return "mock-token", nil
		}

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// When GetEnvVars is called
		envVars, err := windsorEnvPrinter.GetEnvVars()

		// Then the result should not contain an error
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And the environment variables should contain the expected values
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
		// Reset session token to ensure consistent behavior
		shell.ResetSessionToken()

		mocks := setupSafeWindsorEnvMocks()

		// Setup mock shell to simulate token regeneration
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

		// First call
		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}
		firstToken := envVars["WINDSOR_SESSION_TOKEN"]
		if firstToken != "first-token" {
			t.Errorf("Expected first token to be 'first-token', got %s", firstToken)
		}

		// Second call
		envVars, err = windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}
		secondToken := envVars["WINDSOR_SESSION_TOKEN"]
		if secondToken != "regenerated-token" {
			t.Errorf("Expected second token to be 'regenerated-token', got %s", secondToken)
		}
	})

	t.Run("SessionTokenError", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()

		// Setup mock shell to simulate token error
		mockShell := shell.NewMockShell()
		mockShell.GetSessionTokenFunc = func() (string, error) {
			return "", fmt.Errorf("mock session token error")
		}
		mocks.Injector.Register("shell", mockShell)

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// Call should fail with session token error
		_, err := windsorEnvPrinter.GetEnvVars()
		if err == nil {
			t.Fatal("Expected error from session token generation, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving session token") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("NoEnvironmentVarsInConfig", func(t *testing.T) {
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
		mocks.ConfigHandler.GetContextFunc = func() string {
			return "mock-context" // Different from "different-context" in env
		}

		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return map[string]string{
					envVarKey: envVarValue,
				}
			}
			return map[string]string{}
		}

		// Mock secrets provider that will be called regardless of cache
		mockSecretsProvider := secrets.NewMockSecretsProvider()
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
		mockSecretsProvider := secrets.NewMockSecretsProvider()
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
		mockSecretsProvider := secrets.NewMockSecretsProvider()
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

		// Verify that WINDSOR_MANAGED_ENV includes our custom variables
		managedEnvList := envVars["WINDSOR_MANAGED_ENV"]
		if !strings.Contains(managedEnvList, "CUSTOM_ENV_VAR1") {
			t.Errorf("WINDSOR_MANAGED_ENV should contain CUSTOM_ENV_VAR1")
		}
		if !strings.Contains(managedEnvList, "CUSTOM_ENV_VAR2") {
			t.Errorf("WINDSOR_MANAGED_ENV should contain CUSTOM_ENV_VAR2")
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
		mockSecretsProvider := secrets.NewMockSecretsProvider()
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
}

func TestWindsorEnv_PostEnvHook(t *testing.T) {
	t.Run("TestPostEnvHookNoError", func(t *testing.T) {
		windsorEnvPrinter := &WindsorEnvPrinter{}

		err := windsorEnvPrinter.PostEnvHook()
		if err != nil {
			t.Errorf("PostEnvHook() returned an error: %v", err)
		}
	})
}

func TestWindsorEnv_Print(t *testing.T) {
	// Save original stat function
	originalStat := stat
	defer func() {
		stat = originalStat
	}()

	t.Run("Success", func(t *testing.T) {
		// Use setupSafeWindsorEnvMocks to create mocks
		mocks := setupSafeWindsorEnvMocks()
		mockInjector := mocks.Injector
		windsorEnvPrinter := NewWindsorEnvPrinter(mockInjector)
		windsorEnvPrinter.Initialize()

		// Mock the stat function to simulate the existence of the Windsor config file
		stat = func(name string) (os.FileInfo, error) {
			if filepath.Clean(name) == filepath.FromSlash("/mock/config/root/.windsor/config") {
				return nil, nil // Simulate that the file exists
			}
			return nil, os.ErrNotExist
		}

		// Mock the PrintEnvVarsFunc to verify it is called with the correct envVars
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) {
			capturedEnvVars = envVars
		}

		// Call Print and check for errors
		err := windsorEnvPrinter.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify the core Windsor environment variables
		if capturedEnvVars["WINDSOR_CONTEXT"] != "mock-context" {
			t.Errorf("WINDSOR_CONTEXT = %q, want %q", capturedEnvVars["WINDSOR_CONTEXT"], "mock-context")
		}

		if capturedEnvVars["WINDSOR_PROJECT_ROOT"] != filepath.FromSlash("/mock/project/root") {
			t.Errorf("WINDSOR_PROJECT_ROOT = %q, want %q", capturedEnvVars["WINDSOR_PROJECT_ROOT"], filepath.FromSlash("/mock/project/root"))
		}

		if capturedEnvVars["WINDSOR_SESSION_TOKEN"] == "" {
			t.Errorf("WINDSOR_SESSION_TOKEN is empty")
		}

		// Verify that WINDSOR_MANAGED_ENV includes the core Windsor variables
		managedEnv := capturedEnvVars["WINDSOR_MANAGED_ENV"]
		coreVars := []string{"WINDSOR_CONTEXT", "WINDSOR_PROJECT_ROOT", "WINDSOR_SESSION_TOKEN", "WINDSOR_MANAGED_ENV", "WINDSOR_MANAGED_ALIAS"}
		for _, v := range coreVars {
			if !strings.Contains(managedEnv, v) {
				t.Errorf("WINDSOR_MANAGED_ENV should contain %q, got %q", v, managedEnv)
			}
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Use setupSafeWindsorEnvMocks to create mocks
		mocks := setupSafeWindsorEnvMocks()

		// Override the GetProjectRootFunc to simulate an error
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock project root error")
		}

		mockInjector := mocks.Injector

		windsorEnvPrinter := NewWindsorEnvPrinter(mockInjector)
		windsorEnvPrinter.Initialize()

		// Call Print and check for errors
		err := windsorEnvPrinter.Print()
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "mock project root error") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

// TestWindsorEnv_Initialize tests the Initialize method for WindsorEnvPrinter
func TestWindsorEnv_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create a mock injector
		injector := di.NewMockInjector()
		mockShell := shell.NewMockShell()
		mockConfigHandler := config.NewMockConfigHandler()
		mockSecretsProvider := secrets.NewMockSecretsProvider()

		// Register mocks in the injector
		injector.Register("shell", mockShell)
		injector.Register("configHandler", mockConfigHandler)
		injector.Register("secretsProvider", mockSecretsProvider)

		// Create a new WindsorEnvPrinter
		windsorEnv := NewWindsorEnvPrinter(injector)

		// Call Initialize and check for errors
		err := windsorEnv.Initialize()
		if err != nil {
			t.Fatalf("Initialize returned error: %v", err)
		}

		// Verify that secretsProviders is populated
		if len(windsorEnv.secretsProviders) != 1 {
			t.Errorf("Expected 1 secrets provider, got %d", len(windsorEnv.secretsProviders))
		}
	})

	t.Run("BaseInitializationError", func(t *testing.T) {
		// Create a mock injector
		injector := di.NewMockInjector()

		// Don't register any components to cause initialization error

		// Create a new WindsorEnvPrinter
		windsorEnv := NewWindsorEnvPrinter(injector)

		// Call Initialize and expect an error
		err := windsorEnv.Initialize()
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to initialize BaseEnvPrinter") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("ResolveAllError", func(t *testing.T) {
		// Create a mock injector that returns an error for ResolveAll
		mockInjector := di.NewMockInjector()
		mockShell := shell.NewMockShell()
		mockConfigHandler := config.NewMockConfigHandler()

		// Register mocks in the injector
		mockInjector.Register("shell", mockShell)
		mockInjector.Register("configHandler", mockConfigHandler)

		// Make ResolveAll return an error
		mockInjector.SetResolveAllError((*secrets.SecretsProvider)(nil), fmt.Errorf("error resolving secrets providers"))

		// Create a new WindsorEnvPrinter
		windsorEnv := NewWindsorEnvPrinter(mockInjector)

		// Call Initialize and expect an error
		err := windsorEnv.Initialize()
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to resolve secrets providers") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("CastError", func(t *testing.T) {
		// Create a custom injector that returns something that can't be cast to SecretsProvider
		customInjector := &customMockInjector{
			MockInjector: di.NewMockInjector(),
		}

		mockShell := shell.NewMockShell()
		mockConfigHandler := config.NewMockConfigHandler()

		// Register mocks in the injector
		customInjector.Register("shell", mockShell)
		customInjector.Register("configHandler", mockConfigHandler)

		// Create a new WindsorEnvPrinter
		windsorEnv := NewWindsorEnvPrinter(customInjector)

		// Call Initialize and expect an error
		err := windsorEnv.Initialize()
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
		// Setup
		mockInjector := di.NewMockInjector()
		mockSecretsProvider := secrets.NewMockSecretsProvider()
		mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			if input == "value with ${{ secrets.mySecret }}" {
				return "value with resolved-secret", nil
			}
			return input, nil
		}

		windsorEnv := NewWindsorEnvPrinter(mockInjector)
		windsorEnv.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// Call the method
		result := windsorEnv.parseAndCheckSecrets("value with ${{ secrets.mySecret }}")

		// Verify result
		if result != "value with resolved-secret" {
			t.Errorf("Expected 'value with resolved-secret', got %q", result)
		}
	})

	t.Run("SecretsProviderError", func(t *testing.T) {
		// Setup
		mockInjector := di.NewMockInjector()
		mockSecretsProvider := secrets.NewMockSecretsProvider()
		mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			return "", fmt.Errorf("error parsing secrets")
		}

		windsorEnv := NewWindsorEnvPrinter(mockInjector)
		windsorEnv.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// Call the method with a string containing a secret
		result := windsorEnv.parseAndCheckSecrets("value with ${{ secrets.mySecret }}")

		// Verify result
		if !strings.Contains(result, "<ERROR: failed to parse") {
			t.Errorf("Expected error message containing 'failed to parse', got %q", result)
		}
		if !strings.Contains(result, "secrets.mySecret") {
			t.Errorf("Expected error message to contain 'secrets.mySecret', got %q", result)
		}
	})

	t.Run("NoSecretsProviders", func(t *testing.T) {
		// Setup
		mockInjector := di.NewMockInjector()
		windsorEnv := NewWindsorEnvPrinter(mockInjector)
		windsorEnv.secretsProviders = []secrets.SecretsProvider{} // Empty slice

		// Call the method with a string containing a secret
		result := windsorEnv.parseAndCheckSecrets("value with ${{ secrets.mySecret }}")

		// Verify result
		if result != "<ERROR: No secrets providers configured>" {
			t.Errorf("Expected '<ERROR: No secrets providers configured>', got %q", result)
		}
	})

	t.Run("UnparsedSecrets", func(t *testing.T) {
		// Setup
		mockInjector := di.NewMockInjector()
		mockSecretsProvider := secrets.NewMockSecretsProvider()
		// This provider doesn't recognize the secret pattern
		mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			return input, nil
		}

		windsorEnv := NewWindsorEnvPrinter(mockInjector)
		windsorEnv.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// Call the method with a string containing a secret
		result := windsorEnv.parseAndCheckSecrets("value with ${{ secrets.mySecret }}")

		// Verify result
		if !strings.Contains(result, "<ERROR: failed to parse") {
			t.Errorf("Expected error message containing 'failed to parse', got %q", result)
		}
		if !strings.Contains(result, "secrets.mySecret") {
			t.Errorf("Expected error message to contain 'secrets.mySecret', got %q", result)
		}
	})

	t.Run("MultipleUnparsedSecrets", func(t *testing.T) {
		// Setup
		mockInjector := di.NewMockInjector()
		mockSecretsProvider := secrets.NewMockSecretsProvider()
		// This provider doesn't recognize any secrets
		mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			return input, nil
		}

		windsorEnv := NewWindsorEnvPrinter(mockInjector)
		windsorEnv.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// Call the method with multiple secrets
		result := windsorEnv.parseAndCheckSecrets("value with ${{ secrets.secretA }} and ${{ secrets.secretB }}")

		// Verify result
		if !strings.Contains(result, "<ERROR: failed to parse") {
			t.Errorf("Expected error message containing 'failed to parse', got %q", result)
		}
		if !strings.Contains(result, "secrets.secretA, secrets.secretB") {
			t.Errorf("Expected error message to contain 'secrets.secretA, secrets.secretB', got %q", result)
		}
	})
}
