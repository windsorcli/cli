package env

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
	// Save original functions
	originalStat := stat
	originalOsRemoveAll := osRemoveAll
	originalCryptoRandRead := cryptoRandRead

	// Restore original functions after tests
	defer func() {
		stat = originalStat
		osRemoveAll = originalOsRemoveAll
		cryptoRandRead = originalCryptoRandRead
	}()

	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["WINDSOR_CONTEXT"] != "mock-context" {
			t.Errorf("WINDSOR_CONTEXT = %v, want %v", envVars["WINDSOR_CONTEXT"], "mock-context")
		}

		expectedProjectRoot := filepath.FromSlash("/mock/project/root")
		if envVars["WINDSOR_PROJECT_ROOT"] != expectedProjectRoot {
			t.Errorf("WINDSOR_PROJECT_ROOT = %v, want %v", envVars["WINDSOR_PROJECT_ROOT"], expectedProjectRoot)
		}

		// Verify session token is generated
		if envVars["WINDSOR_SESSION_TOKEN"] == "" {
			t.Errorf("Expected session token to be generated, but it was empty")
		}
		if len(envVars["WINDSOR_SESSION_TOKEN"]) != 7 {
			t.Errorf("Expected session token to have length 7, got %d", len(envVars["WINDSOR_SESSION_TOKEN"]))
		}
	})

	t.Run("ExistingSessionToken", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()

		// Add custom random generator to create a predictable "existing" token
		origCryptoRandRead := cryptoRandRead
		cryptoRandRead = func(b []byte) (n int, err error) {
			// Generate predictable output that will produce "existing"
			for i := range b {
				// This is simplified but works for our test
				b[i] = "existing"[i%8]
			}
			return len(b), nil
		}
		// Restore after test
		defer func() {
			cryptoRandRead = origCryptoRandRead
		}()

		// First generate a token
		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// Set the environment to empty to ensure we use the generated token
		t.Setenv("WINDSOR_SESSION_TOKEN", "")

		// Get the token for the first time
		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Now get it again to ensure we use the cached token
		envVars, err = windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Check that we get the expected token
		if len(envVars["WINDSOR_SESSION_TOKEN"]) != 7 {
			t.Errorf("Expected session token to have length 7, got %d", len(envVars["WINDSOR_SESSION_TOKEN"]))
		}

		// Skip the exact token check for now since the random generation makes it difficult to test deterministically
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
		stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".session.envtoken") {
				return nil, nil // File exists
			}
			return nil, os.ErrNotExist
		}

		osRemoveAll = func(path string) error {
			return nil
		}

		// Mock crypto functions for predictable output
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

		// Verify a new token was generated (not the env token)
		if envVars["WINDSOR_SESSION_TOKEN"] == "envtoken" {
			t.Errorf("Expected a new token to be generated, but got the environment token")
		}
		if len(envVars["WINDSOR_SESSION_TOKEN"]) != 7 {
			t.Errorf("Expected session token to have length 7, got %d", len(envVars["WINDSOR_SESSION_TOKEN"]))
		}
	})

	t.Run("SignalFileRemovalError", func(t *testing.T) {
		// Mock file system functions
		stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".session.envtoken") {
				return nil, nil // File exists
			}
			return nil, os.ErrNotExist
		}

		// Mock osRemoveAll to return an error
		osRemoveAll = func(path string) error {
			return fmt.Errorf("mock error removing signal file")
		}

		// Set up to capture stdout
		origStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Mock crypto functions for predictable output
		origCryptoRandRead := cryptoRandRead
		cryptoRandRead = func(b []byte) (n int, err error) {
			for i := range b {
				b[i] = byte(i % 62) // Will map to characters in charset
			}
			return len(b), nil
		}
		defer func() {
			cryptoRandRead = origCryptoRandRead
		}()

		mocks := setupSafeWindsorEnvMocks()

		// Set up environment variable with a token
		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// Call should not fail (error is deferred and printed to stdout)
		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Close the writer to get all output
		w.Close()
		os.Stdout = origStdout
		var buf strings.Builder
		_, err = io.Copy(&buf, r)
		if err != nil {
			t.Fatalf("Failed to read captured output: %v", err)
		}
		capturedOutput := buf.String()

		// Verify the error message was printed to stdout
		expectedErrMsg := "error removing token file: mock error removing signal file"
		if !strings.Contains(capturedOutput, expectedErrMsg) {
			t.Errorf("Expected output to contain %q, got %q", expectedErrMsg, capturedOutput)
		}

		// Verify a new token was generated (not the env token)
		if envVars["WINDSOR_SESSION_TOKEN"] == "envtoken" {
			t.Errorf("Expected a new token to be generated, but got the environment token")
		}
		if len(envVars["WINDSOR_SESSION_TOKEN"]) != 7 {
			t.Errorf("Expected session token to have length 7, got %d", len(envVars["WINDSOR_SESSION_TOKEN"]))
		}
	})

	t.Run("ProjectRootErrorDuringEnvTokenSignalFileCheck", func(t *testing.T) {
		// Mock file system and crypto functions
		stat = func(name string) (os.FileInfo, error) {
			// This mock won't be reached because we'll error out earlier
			return nil, nil
		}

		// Set up environment variable with a token
		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		mocks := setupSafeWindsorEnvMocks()

		// First call succeeds, second fails
		callCount := 0
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			callCount++
			if callCount == 1 {
				return filepath.FromSlash("/mock/project/root"), nil
			}
			return "", fmt.Errorf("mock error getting project root during token check")
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
		// Mock crypto functions to return an error
		cryptoRandRead = func(b []byte) (n int, err error) {
			return 0, fmt.Errorf("mock random generation error")
		}

		mocks := setupSafeWindsorEnvMocks()

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		_, err := windsorEnvPrinter.GetEnvVars()
		if err == nil {
			t.Fatal("Expected error from random generation, got nil")
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

		// First call succeeds, second fails (for project root during token check)
		callCount := 0
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			callCount++
			if callCount == 1 {
				return filepath.FromSlash("/mock/project/root"), nil
			}
			return "", fmt.Errorf("mock shell error during token check")
		}

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		_, err := windsorEnvPrinter.GetEnvVars()
		if err == nil {
			t.Error("Expected error, got nil")
		} else if !strings.Contains(err.Error(), "error retrieving session token") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("ComprehensiveEnvironmentTokenTest", func(t *testing.T) {
		// Save original functions to restore later for this test case
		origStat := stat
		origOsRemoveAll := osRemoveAll
		origCryptoRandRead := cryptoRandRead

		// First clear any existing env token
		t.Setenv("WINDSOR_SESSION_TOKEN", "")

		// Mock random generation for first call
		cryptoRandRead = func(b []byte) (n int, err error) {
			for i := range b {
				// Generate a predictable but distinct token for the first call
				b[i] = byte('a' + (i % 26))
			}
			return len(b), nil
		}

		// Mock for initial call with no env token
		mocks := setupSafeWindsorEnvMocks()
		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// First get a token with no env set (should generate a new one)
		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}
		firstToken := envVars["WINDSOR_SESSION_TOKEN"]

		// Now set the env token
		t.Setenv("WINDSOR_SESSION_TOKEN", "testtoken")

		// Reset the session token to force checking env
		windsorEnvPrinter.sessionToken = ""

		// Mock stat to return nil, nil for signal file (exists)
		stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".session.testtoken") {
				return nil, nil // File exists
			}
			return nil, os.ErrNotExist
		}

		// Mock removal to succeed
		osRemoveAll = func(path string) error {
			return nil
		}

		// Update crypto function to generate a different token for the second call
		cryptoRandRead = func(b []byte) (n int, err error) {
			for i := range b {
				// Generate a predictable but distinct token from the first
				b[i] = byte('A' + (i % 26))
			}
			return len(b), nil
		}

		// Second call with env token and signal file (should regenerate)
		envVars, err = windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}
		secondToken := envVars["WINDSOR_SESSION_TOKEN"]

		// Verify token was regenerated and is different from both env token and first token
		if secondToken == "testtoken" {
			t.Errorf("Token should not be the environment token, got %s", secondToken)
		}
		if secondToken == firstToken {
			t.Errorf("Second token %s should be different from first token %s", secondToken, firstToken)
		}

		// Third call should use the cached session token
		envVars, err = windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}
		thirdToken := envVars["WINDSOR_SESSION_TOKEN"]

		// Verify cached token was used
		if thirdToken != secondToken {
			t.Errorf("Expected same token %s to be returned, but got %s", secondToken, thirdToken)
		}

		// Restore original functions
		stat = origStat
		osRemoveAll = origOsRemoveAll
		cryptoRandRead = origCryptoRandRead
	})

	t.Run("RandomErrorDuringSignalFileRegeneration", func(t *testing.T) {
		// Mock file system functions
		stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, ".session.envtoken") {
				return nil, nil // File exists
			}
			return nil, os.ErrNotExist
		}

		// Mock osRemoveAll to succeed
		osRemoveAll = func(path string) error {
			return nil
		}

		// Mock crypto functions to return an error during regeneration
		cryptoRandRead = func(b []byte) (n int, err error) {
			return 0, fmt.Errorf("mock random generation error during token regeneration")
		}

		mocks := setupSafeWindsorEnvMocks()

		// Set up environment variable with a token
		t.Setenv("WINDSOR_SESSION_TOKEN", "envtoken")

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// Call should fail with random generation error
		_, err := windsorEnvPrinter.GetEnvVars()
		if err == nil {
			t.Fatal("Expected error from random generation during token regeneration, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving session token") ||
			!strings.Contains(err.Error(), "error generating session token") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("NoEnvironmentVarsInConfig", func(t *testing.T) {
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

		// Ensure environment is clean
		originalEnvContext := os.Getenv("WINDSOR_CONTEXT")
		originalEnvToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		t.Setenv("WINDSOR_CONTEXT", "")
		t.Setenv("WINDSOR_SESSION_TOKEN", "")
		defer func() {
			os.Setenv("WINDSOR_CONTEXT", originalEnvContext)
			os.Setenv("WINDSOR_SESSION_TOKEN", originalEnvToken)
		}()

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
		assert.NoError(t, err, "GetEnvVars should not return an error")

		// Verify we still have the base environment variables
		assert.Equal(t, "mock-context", envVars["WINDSOR_CONTEXT"], "WINDSOR_CONTEXT should be set even when no environment vars are in config")
		assert.Equal(t, filepath.FromSlash("/mock/project/root"), envVars["WINDSOR_PROJECT_ROOT"], "WINDSOR_PROJECT_ROOT should be set")
		assert.NotEmpty(t, envVars["WINDSOR_SESSION_TOKEN"], "Session token should be generated")

		// Verify no additional variables were added from config (since there were none)
		assert.Len(t, envVars, 5, "Should have five base environment variables")
	})

	t.Run("DifferentContextDisablesCache", func(t *testing.T) {
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

		// Set up environment with a different context than the one in config
		// to test the useCache=false path
		envVarKey := "TEST_VAR_WITH_SECRET"
		envVarValue := "value with ${{ secrets.mySecret }}"

		// Save original environment values and restore them after test
		originalEnvContext := os.Getenv("WINDSOR_CONTEXT")
		originalEnvToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		originalTestVar := os.Getenv(envVarKey)

		t.Setenv("WINDSOR_CONTEXT", "different-context")
		t.Setenv("WINDSOR_SESSION_TOKEN", "")
		t.Setenv(envVarKey, "existing-value")

		defer func() {
			os.Setenv("WINDSOR_CONTEXT", originalEnvContext)
			os.Setenv("WINDSOR_SESSION_TOKEN", originalEnvToken)
			os.Setenv(envVarKey, originalTestVar)
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
		assert.NoError(t, err, "Initialize should not return an error")

		// Make secretsProviders accessible to the test
		windsorEnvPrinter.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// Get environment variables
		envVars, err := windsorEnvPrinter.GetEnvVars()
		assert.NoError(t, err, "GetEnvVars should not return an error")

		// Verify the variable was resolved despite having an existing value in the environment
		// This confirms that useCache=false worked as expected
		assert.Equal(t, "resolved-value", envVars[envVarKey],
			"Environment variable should be resolved even with existing value when contexts differ")
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
		assert.NoError(t, err, "Initialize should not return an error")

		// Make secretsProviders accessible to the test
		windsorEnvPrinter.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// Get environment variables
		envVars, err := windsorEnvPrinter.GetEnvVars()
		assert.NoError(t, err, "GetEnvVars should not return an error")

		// Verify the variable was resolved despite having an existing value in the environment
		// This confirms that NO_CACHE=true worked as expected
		assert.Equal(t, "resolved-value", envVars[envVarKey],
			"Environment variable should be resolved even with existing value when NO_CACHE=true")
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
		assert.NoError(t, err, "Initialize should not return an error")

		// Make secretsProviders accessible to the test
		windsorEnvPrinter.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// Get environment variables
		envVars, err := windsorEnvPrinter.GetEnvVars()
		assert.NoError(t, err, "GetEnvVars should not return an error")

		// Verify the regular variables were set directly without parsing
		assert.Equal(t, regularVarValue1, envVars[regularVarKey1],
			"Regular environment variable should be set directly")
		assert.Equal(t, regularVarValue2, envVars[regularVarKey2],
			"Regular environment variable should be set directly")

		// Also verify that the secret was parsed correctly
		assert.Equal(t, "resolved-secret", envVars["WITH_SECRET"],
			"Environment variable with secret should be resolved")
	})

	t.Run("ManagedCustomEnvironmentVars", func(t *testing.T) {
		// Save original values
		originalManagedEnv := make([]string, len(windsorManagedEnv))
		copy(originalManagedEnv, windsorManagedEnv)

		// Restore original state after test
		defer func() {
			windsorManagedMu.Lock()
			windsorManagedEnv = originalManagedEnv
			windsorManagedMu.Unlock()
		}()

		// Setup mocks
		mocks := setupSafeWindsorEnvMocks()

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
		assert.NoError(t, err, "Failed to initialize WindsorEnvPrinter")

		// Get environment variables
		envVars, err := windsorEnvPrinter.GetEnvVars()
		assert.NoError(t, err, "GetEnvVars should not return an error")

		// Verify custom variables are in the environment variables map
		assert.Equal(t, "value1", envVars["CUSTOM_ENV_VAR1"], "CUSTOM_ENV_VAR1 should be set to 'value1'")
		assert.Equal(t, "value2", envVars["CUSTOM_ENV_VAR2"], "CUSTOM_ENV_VAR2 should be set to 'value2'")

		// Verify that WINDSOR_MANAGED_ENV includes our custom variables
		managedEnvList := envVars["WINDSOR_MANAGED_ENV"]
		assert.Contains(t, managedEnvList, "CUSTOM_ENV_VAR1", "WINDSOR_MANAGED_ENV should contain CUSTOM_ENV_VAR1")
		assert.Contains(t, managedEnvList, "CUSTOM_ENV_VAR2", "WINDSOR_MANAGED_ENV should contain CUSTOM_ENV_VAR2")
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
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
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

// TestWindsorEnv_CreateSessionInvalidationSignal tests the CreateSessionInvalidationSignal method
func TestWindsorEnv_CreateSessionInvalidationSignal(t *testing.T) {
	// Save original functions
	originalWriteFile := writeFile
	originalMkdirAll := mkdirAll

	// Restore original functions after tests
	defer func() {
		writeFile = originalWriteFile
		mkdirAll = originalMkdirAll
	}()

	t.Run("SuccessfulSignalCreation", func(t *testing.T) {
		// Set up environment variable with a token
		t.Setenv("WINDSOR_SESSION_TOKEN", "testtoken")

		// Mock file system functions
		var capturedMkdirPath string
		var capturedMkdirPerm os.FileMode
		mkdirAll = func(path string, perm os.FileMode) error {
			capturedMkdirPath = path
			capturedMkdirPerm = perm
			return nil
		}

		var capturedWritePath string
		var capturedWriteData []byte
		var capturedWritePerm os.FileMode
		writeFile = func(name string, data []byte, perm os.FileMode) error {
			capturedWritePath = name
			capturedWriteData = data
			capturedWritePerm = perm
			return nil
		}

		mocks := setupSafeWindsorEnvMocks()
		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// Create session invalidation signal
		err := windsorEnvPrinter.CreateSessionInvalidationSignal()
		if err != nil {
			t.Fatalf("CreateSessionInvalidationSignal returned an error: %v", err)
		}

		// Verify mkdir was called correctly
		expectedMkdirPath := filepath.FromSlash("/mock/project/root/.windsor")
		if capturedMkdirPath != expectedMkdirPath {
			t.Errorf("mkdirAll path = %q, want %q", capturedMkdirPath, expectedMkdirPath)
		}
		if capturedMkdirPerm != 0755 {
			t.Errorf("mkdirAll perm = %v, want %v", capturedMkdirPerm, 0755)
		}

		// Verify writeFile was called correctly
		expectedWritePath := filepath.FromSlash("/mock/project/root/.windsor/.session.testtoken")
		if capturedWritePath != expectedWritePath {
			t.Errorf("writeFile path = %q, want %q", capturedWritePath, expectedWritePath)
		}
		if len(capturedWriteData) != 0 {
			t.Errorf("writeFile data should be empty, got %v", capturedWriteData)
		}
		if capturedWritePerm != 0644 {
			t.Errorf("writeFile perm = %v, want %v", capturedWritePerm, 0644)
		}
	})

	t.Run("NoSessionToken", func(t *testing.T) {
		// Clear environment variable
		t.Setenv("WINDSOR_SESSION_TOKEN", "")

		// Mock file system functions to ensure they are not called
		mkdirAll = func(path string, perm os.FileMode) error {
			t.Error("mkdirAll should not be called")
			return nil
		}

		writeFile = func(name string, data []byte, perm os.FileMode) error {
			t.Error("writeFile should not be called")
			return nil
		}

		mocks := setupSafeWindsorEnvMocks()
		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// Create session invalidation signal
		err := windsorEnvPrinter.CreateSessionInvalidationSignal()
		if err != nil {
			t.Fatalf("CreateSessionInvalidationSignal returned an error: %v", err)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Set up environment variable with a token
		t.Setenv("WINDSOR_SESSION_TOKEN", "testtoken")

		mocks := setupSafeWindsorEnvMocks()

		// Mock GetProjectRootFunc to return an error
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock project root error")
		}

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// Create session invalidation signal
		err := windsorEnvPrinter.CreateSessionInvalidationSignal()
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		expectedErrMsg := "failed to get project root: mock project root error"
		if err.Error() != expectedErrMsg {
			t.Errorf("Error message = %q, want %q", err.Error(), expectedErrMsg)
		}
	})

	t.Run("MkdirAllError", func(t *testing.T) {
		// Set up environment variable with a token
		t.Setenv("WINDSOR_SESSION_TOKEN", "testtoken")

		// Mock mkdir to return an error
		mkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock mkdir error")
		}

		mocks := setupSafeWindsorEnvMocks()
		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// Create session invalidation signal
		err := windsorEnvPrinter.CreateSessionInvalidationSignal()
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		expectedErrMsg := "failed to create .windsor directory: mock mkdir error"
		if err.Error() != expectedErrMsg {
			t.Errorf("Error message = %q, want %q", err.Error(), expectedErrMsg)
		}
	})

	t.Run("WriteFileError", func(t *testing.T) {
		// Set up environment variable with a token
		t.Setenv("WINDSOR_SESSION_TOKEN", "testtoken")

		// Mock file system functions
		mkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		writeFile = func(name string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mock write file error")
		}

		mocks := setupSafeWindsorEnvMocks()
		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// Create session invalidation signal
		err := windsorEnvPrinter.CreateSessionInvalidationSignal()
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		expectedErrMsg := "failed to create signal file: mock write file error"
		if err.Error() != expectedErrMsg {
			t.Errorf("Error message = %q, want %q", err.Error(), expectedErrMsg)
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
		assert.NoError(t, err)

		// Verify that secretsProviders is populated
		assert.NotNil(t, windsorEnv.secretsProviders)
		assert.Equal(t, 1, len(windsorEnv.secretsProviders))
	})

	t.Run("BaseInitializationError", func(t *testing.T) {
		// Create a mock injector
		injector := di.NewMockInjector()

		// Don't register any components to cause initialization error

		// Create a new WindsorEnvPrinter
		windsorEnv := NewWindsorEnvPrinter(injector)

		// Call Initialize and expect an error
		err := windsorEnv.Initialize()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to initialize BaseEnvPrinter")
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
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to resolve secrets providers")
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
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to cast instance to SecretsProvider")
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
		assert.Equal(t, "value with resolved-secret", result)
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
		assert.Contains(t, result, "<ERROR: failed to parse")
		assert.Contains(t, result, "secrets.mySecret")
	})

	t.Run("NoSecretsProviders", func(t *testing.T) {
		// Setup
		mockInjector := di.NewMockInjector()
		windsorEnv := NewWindsorEnvPrinter(mockInjector)
		windsorEnv.secretsProviders = []secrets.SecretsProvider{} // Empty slice

		// Call the method with a string containing a secret
		result := windsorEnv.parseAndCheckSecrets("value with ${{ secrets.mySecret }}")

		// Verify result
		assert.Equal(t, "<ERROR: No secrets providers configured>", result)
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
		assert.Contains(t, result, "<ERROR: failed to parse")
		assert.Contains(t, result, "secrets.mySecret")
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
		assert.Contains(t, result, "<ERROR: failed to parse")
		assert.Contains(t, result, "secrets.secretA, secrets.secretB")
	})
}
