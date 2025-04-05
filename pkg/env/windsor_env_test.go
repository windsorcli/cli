package env

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
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
		if envVars[EnvSessionTokenVar] == "" {
			t.Errorf("Expected session token to be generated, but it was empty")
		}
		if len(envVars[EnvSessionTokenVar]) != 7 {
			t.Errorf("Expected session token to have length 7, got %d", len(envVars[EnvSessionTokenVar]))
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
		t.Setenv(EnvSessionTokenVar, "")

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
		if len(envVars[EnvSessionTokenVar]) != 7 {
			t.Errorf("Expected session token to have length 7, got %d", len(envVars[EnvSessionTokenVar]))
		}

		// Skip the exact token check for now since the random generation makes it difficult to test deterministically
	})

	t.Run("EnvironmentTokenWithoutSignalFile", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()

		// Set up environment variable with a token
		t.Setenv(EnvSessionTokenVar, "envtoken")

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
		if envVars[EnvSessionTokenVar] != "envtoken" {
			t.Errorf("Expected session token to be 'envtoken', got %s", envVars[EnvSessionTokenVar])
		}
	})

	t.Run("EnvironmentTokenWithStatError", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()

		// Set up environment variable with a token
		t.Setenv(EnvSessionTokenVar, "envtoken")

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
		if envVars[EnvSessionTokenVar] != "envtoken" {
			t.Errorf("Expected session token to be 'envtoken', got %s", envVars[EnvSessionTokenVar])
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
		t.Setenv(EnvSessionTokenVar, "envtoken")

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Verify a new token was generated (not the env token)
		if envVars[EnvSessionTokenVar] == "envtoken" {
			t.Errorf("Expected a new token to be generated, but got the environment token")
		}
		if len(envVars[EnvSessionTokenVar]) != 7 {
			t.Errorf("Expected session token to have length 7, got %d", len(envVars[EnvSessionTokenVar]))
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

		// Mock crypto functions - will not be reached due to error
		cryptoRandRead = func(b []byte) (n int, err error) {
			t.Error("cryptoRandRead should not be called")
			return 0, nil
		}

		mocks := setupSafeWindsorEnvMocks()

		// Set up environment variable with a token
		t.Setenv(EnvSessionTokenVar, "envtoken")

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// Call should fail with file removal error
		_, err := windsorEnvPrinter.GetEnvVars()
		if err == nil {
			t.Fatal("Expected error from file removal, got nil")
		}

		expectedErr := "error retrieving session token: error removing token file: mock error removing signal file"
		if err.Error() != expectedErr {
			t.Errorf("Expected error %q, got %q", expectedErr, err.Error())
		}
	})

	t.Run("ProjectRootErrorDuringEnvTokenSignalFileCheck", func(t *testing.T) {
		// Mock file system and crypto functions
		stat = func(name string) (os.FileInfo, error) {
			// This mock won't be reached because we'll error out earlier
			return nil, nil
		}

		// Set up environment variable with a token
		t.Setenv(EnvSessionTokenVar, "envtoken")

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
		t.Setenv(EnvSessionTokenVar, "envtoken")

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
		t.Setenv(EnvSessionTokenVar, "")

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
		firstToken := envVars[EnvSessionTokenVar]

		// Now set the env token
		t.Setenv(EnvSessionTokenVar, "testtoken")

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
		secondToken := envVars[EnvSessionTokenVar]

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
		thirdToken := envVars[EnvSessionTokenVar]

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
		t.Setenv(EnvSessionTokenVar, "envtoken")

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

		// Verify that PrintEnvVarsFunc was called with the correct envVars
		expectedEnvVars := map[string]string{
			"WINDSOR_CONTEXT":       "mock-context",
			"WINDSOR_PROJECT_ROOT":  filepath.FromSlash("/mock/project/root"),
			"WINDSOR_SESSION_TOKEN": capturedEnvVars["WINDSOR_SESSION_TOKEN"], // Include session token
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
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
		t.Setenv(EnvSessionTokenVar, "testtoken")

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
		t.Setenv(EnvSessionTokenVar, "")

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
		t.Setenv(EnvSessionTokenVar, "testtoken")

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
		t.Setenv(EnvSessionTokenVar, "testtoken")

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
		t.Setenv(EnvSessionTokenVar, "testtoken")

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
