package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestShell_GetSessionToken(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a shell with session token and no reset flag
		shell, mocks := setup(t)
		expectedToken := "test-token"
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return expectedToken
			}
			return ""
		}

		// Mock Stat to return file not found (no reset flag exists)
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// Mock Glob to return empty slice (no session files)
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return []string{}, nil
		}

		// When getting session token
		token, err := shell.GetSessionToken()

		// Then it should return the expected token
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if token != expectedToken {
			t.Errorf("Expected token %q, got %q", expectedToken, token)
		}
	})

	t.Run("NoToken", func(t *testing.T) {
		// Given a shell with mocked operations
		shell, mocks := setup(t)

		// Mock environment to return no token
		mocks.Shims.Getenv = func(key string) string {
			return ""
		}

		// Mock random generation
		mocks.Shims.RandRead = func(b []byte) (n int, err error) {
			for i := range b {
				b[i] = byte('a')
			}
			return len(b), nil
		}

		// When getting session token
		token, err := shell.GetSessionToken()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Then it should return a token of length 7
		if len(token) != 7 {
			t.Errorf("Expected token length 7, got %d", len(token))
		}
	})

	t.Run("ReturnsCachedToken", func(t *testing.T) {
		// Given a shell with a cached session token
		shell, _ := setup(t)
		expectedToken := "cached-token"
		shell.sessionToken = expectedToken

		// When getting session token
		token, err := shell.GetSessionToken()

		// Then it should return the cached token without calling Getenv
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if token != expectedToken {
			t.Errorf("Expected token %q, got %q", expectedToken, token)
		}
	})
}

func TestShell_CheckResetFlags(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		shell, mocks := setup(t)
		expectedToken := "test-token"
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return expectedToken
			}
			return ""
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, expectedToken) {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return []string{"/test/project/.windsor/session-test-token"}, nil
		}
		mocks.Shims.RemoveAll = func(path string) error {
			return nil
		}

		shouldReset, err := shell.CheckResetFlags()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !shouldReset {
			t.Error("Expected shouldReset to be true")
		}
	})

	t.Run("NoResetToken", func(t *testing.T) {
		shell, mocks := setup(t)
		mocks.Shims.Getenv = func(key string) string {
			return ""
		}

		shouldReset, err := shell.CheckResetFlags()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if shouldReset {
			t.Error("Expected shouldReset to be false")
		}
	})

	t.Run("TokenMismatch", func(t *testing.T) {
		shell, mocks := setup(t)
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_RESET_TOKEN" {
				return "test-token"
			}
			return ""
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return []string{}, nil
		}

		shouldReset, err := shell.CheckResetFlags()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if shouldReset {
			t.Error("Expected shouldReset to be false")
		}
	})

	t.Run("ErrorOnGlob", func(t *testing.T) {
		// Given a shell with failing Glob
		shell, mocks := setup(t)
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return "test-token"
			}
			return ""
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return nil, fmt.Errorf("glob error")
		}

		// When checking reset flags
		_, err := shell.CheckResetFlags()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "glob error") {
			t.Errorf("Expected error to contain 'glob error', got %v", err)
		}
	})

	t.Run("ErrorOnRemoveAll", func(t *testing.T) {
		// Given a shell with failing RemoveAll
		shell, mocks := setup(t)
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return "test-token"
			}
			return ""
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return []string{"/test/project/.windsor/session-test-token"}, nil
		}
		mocks.Shims.RemoveAll = func(path string) error {
			return fmt.Errorf("remove error")
		}

		// When checking reset flags
		_, err := shell.CheckResetFlags()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "remove error") {
			t.Errorf("Expected error to contain 'remove error', got %v", err)
		}
	})
}

func TestShell_WriteResetToken(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a shell with mocked operations
		shell, mocks := setup(t)
		projectRoot := "/test/project"
		sessionToken := "test-token"
		sessionFilePath := filepath.Join(projectRoot, ".windsor", ".session."+sessionToken)

		// Mock GetProjectRoot
		mocks.Shims.Getwd = func() (string, error) {
			return projectRoot, nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join(projectRoot, "windsor.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock environment
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return sessionToken
			}
			return ""
		}

		// Mock file operations
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			expectedPath := filepath.Join(projectRoot, ".windsor")
			if path != expectedPath {
				t.Errorf("Expected path %s, got %s", expectedPath, path)
			}
			if perm != 0750 {
				t.Errorf("Expected perm 0750, got %o", perm)
			}
			return nil
		}

		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			if name != sessionFilePath {
				t.Errorf("Expected path %s, got %s", sessionFilePath, name)
			}
			if len(data) != 0 {
				t.Errorf("Expected empty data, got %v", data)
			}
			if perm != 0600 {
				t.Errorf("Expected perm 0600, got %o", perm)
			}
			return nil
		}

		// When writing reset token
		path, err := shell.WriteResetToken()

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if path != sessionFilePath {
			t.Errorf("Expected path %s, got %s", sessionFilePath, path)
		}
	})

	t.Run("NoSessionToken", func(t *testing.T) {
		// Given a shell with no session token
		shell, mocks := setup(t)
		mocks.Shims.Getenv = func(key string) string {
			return ""
		}

		// When writing reset token
		path, err := shell.WriteResetToken()

		// Then it should return empty path without error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if path != "" {
			t.Errorf("Expected empty path, got %s", path)
		}
	})

	t.Run("ErrorOnGetProjectRoot", func(t *testing.T) {
		// Given a shell with failing GetProjectRoot
		shell, mocks := setup(t)
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return "test-token"
			}
			return ""
		}
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("getwd failed")
		}

		// When writing reset token
		path, err := shell.WriteResetToken()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting project root") {
			t.Errorf("Expected error about project root, got: %v", err)
		}
		if path != "" {
			t.Errorf("Expected empty path, got %s", path)
		}
	})

	t.Run("ErrorOnMkdirAll", func(t *testing.T) {
		// Given a shell with failing MkdirAll
		shell, mocks := setup(t)
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return "test-token"
			}
			return ""
		}
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/project", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/project", "windsor.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mkdir failed")
		}

		// When writing reset token
		path, err := shell.WriteResetToken()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error creating .windsor directory") {
			t.Errorf("Expected error about mkdir, got: %v", err)
		}
		if path != "" {
			t.Errorf("Expected empty path, got %s", path)
		}
	})

	t.Run("ErrorOnWriteFile", func(t *testing.T) {
		// Given a shell with failing WriteFile
		shell, mocks := setup(t)
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return "test-token"
			}
			return ""
		}
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/project", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/project", "windsor.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("write failed")
		}

		// When writing reset token
		path, err := shell.WriteResetToken()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error writing reset token file") {
			t.Errorf("Expected error about write file, got: %v", err)
		}
		if path != "" {
			t.Errorf("Expected empty path, got %s", path)
		}
	})
}

func TestShell_Reset(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a shell with mocked operations
		shell, mocks := setup(t)
		projectRoot := "/test/project"
		sessionFiles := []string{
			filepath.Join(projectRoot, ".windsor", ".session.1"),
			filepath.Join(projectRoot, ".windsor", ".session.2"),
		}

		// Mock GetProjectRoot
		mocks.Shims.Getwd = func() (string, error) {
			return projectRoot, nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join(projectRoot, "windsor.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock file operations
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			expectedPattern := filepath.Join(projectRoot, ".windsor", ".session.*")
			if pattern != expectedPattern {
				t.Errorf("Expected pattern %s, got %s", expectedPattern, pattern)
			}
			return sessionFiles, nil
		}

		mocks.Shims.RemoveAll = func(path string) error {
			if path != filepath.Join(projectRoot, ".windsor") {
				t.Errorf("Expected path %s, got %s", filepath.Join(projectRoot, ".windsor"), path)
			}
			return nil
		}

		// When resetting
		shell.Reset()
	})

	t.Run("ErrorOnGetProjectRoot", func(t *testing.T) {
		// Given a shell with failing GetProjectRoot
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("getwd failed")
		}

		// When resetting
		shell.Reset()
		// No error expected since Reset() doesn't return error
	})

	t.Run("ErrorOnGlob", func(t *testing.T) {
		// Given a shell with failing Glob
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/project", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/project", "windsor.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return nil, fmt.Errorf("glob failed")
		}

		// When resetting
		shell.Reset()
		// No error expected since Reset() doesn't return error
	})

	t.Run("ErrorOnRemoveAll", func(t *testing.T) {
		// Given a shell with failing RemoveAll
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/project", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/project", "windsor.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return []string{"/test/project/.windsor/.session.1"}, nil
		}
		mocks.Shims.RemoveAll = func(path string) error {
			return fmt.Errorf("remove failed")
		}

		// When resetting
		shell.Reset()
		// No error expected since Reset() doesn't return error
	})

	t.Run("NoSessionFiles", func(t *testing.T) {
		// Given a shell with no session files
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/project", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/project", "windsor.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return []string{}, nil
		}

		// When resetting
		shell.Reset()
		// No error expected since Reset() doesn't return error
	})

	t.Run("EnvironmentAndAliasReset", func(t *testing.T) {
		// Given a shell with managed environment variables and aliases
		shell, mocks := setup(t)

		// Mock environment variables
		mocks.Shims.Getenv = func(key string) string {
			switch key {
			case "WINDSOR_MANAGED_ENV":
				return "ENV1, ENV2, ENV3"
			case "WINDSOR_MANAGED_ALIAS":
				return "ALIAS1, ALIAS2, ALIAS3"
			default:
				return ""
			}
		}

		// When resetting
		shell.Reset()

		// Then environment variables and aliases should be unset
		// Note: We can't directly verify the unset operations since they're system calls
		// The test coverage will show that these code paths were executed
	})

	t.Run("EmptyEnvironmentAndAlias", func(t *testing.T) {
		// Given a shell with empty managed environment variables and aliases
		shell, mocks := setup(t)

		// Mock empty environment variables
		mocks.Shims.Getenv = func(key string) string {
			return ""
		}

		// When resetting
		shell.Reset()

		// Then no environment variables or aliases should be unset
		// Note: We can't directly verify the unset operations since they're system calls
		// The test coverage will show that these code paths were executed
	})

	t.Run("QuietModeDoesNotPrintCommands", func(t *testing.T) {
		// Given a shell with managed environment variables and aliases
		shell, mocks := setup(t)

		// Mock environment variables
		mocks.Shims.Getenv = func(key string) string {
			switch key {
			case "WINDSOR_MANAGED_ENV":
				return "ENV1, ENV2, ENV3"
			case "WINDSOR_MANAGED_ALIAS":
				return "ALIAS1, ALIAS2, ALIAS3"
			default:
				return ""
			}
		}

		// When resetting with quiet=true
		output := captureStdout(t, func() {
			shell.Reset(true)
		})

		// Then no shell commands should be printed
		if output != "" {
			t.Errorf("Expected no output in quiet mode, got: %v", output)
		}
	})

	t.Run("NonQuietModePrintsCommands", func(t *testing.T) {
		// Given a shell with managed environment variables and aliases
		shell, mocks := setup(t)

		// Mock environment variables
		mocks.Shims.Getenv = func(key string) string {
			switch key {
			case "WINDSOR_MANAGED_ENV":
				return "ENV1, ENV2, ENV3"
			case "WINDSOR_MANAGED_ALIAS":
				return "ALIAS1, ALIAS2, ALIAS3"
			default:
				return ""
			}
		}

		// When resetting with quiet=false
		output := captureStdout(t, func() {
			shell.Reset(false)
		})

		// Then shell commands should be printed (platform-specific)
		// Unix: "unset ENV1 ENV2 ENV3" / Windows: "Remove-Item Env:ENV1"
		if !strings.Contains(output, "unset ENV1 ENV2 ENV3") && !strings.Contains(output, "Remove-Item Env:ENV1") {
			t.Errorf("Expected environment unset command to be printed, got: %v", output)
		}
		// Unix: "unalias ALIAS1" / Windows: "Remove-Item Alias:ALIAS1"
		if !strings.Contains(output, "unalias ALIAS1") && !strings.Contains(output, "Remove-Item Alias:ALIAS1") {
			t.Errorf("Expected alias unset command to be printed, got: %v", output)
		}
	})
}

func TestShell_ResetSessionToken(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a shell with mocked operations
		shell, mocks := setup(t)
		expectedToken := "test-token"

		// Mock environment variable
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return expectedToken
			}
			return ""
		}

		// Mock random generation to return predictable bytes
		mocks.Shims.RandRead = func(b []byte) (n int, err error) {
			// Fill with bytes that will map to "new-test-token"
			for i := range b {
				b[i] = byte(i % 62)
			}
			return len(b), nil
		}

		// Mock Stat to return file not found (no reset flag exists)
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// Mock Glob to return empty slice (no session files)
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return []string{}, nil
		}

		// When getting session token
		token, err := shell.GetSessionToken()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if token != expectedToken {
			t.Errorf("Expected token %q, got %q", expectedToken, token)
		}

		// When resetting session token
		shell.ResetSessionToken()
		mocks.Shims.Getenv = func(key string) string {
			return "" // Simulate environment variable being unset
		}

		// Then getting session token should return a new token
		newToken, err := shell.GetSessionToken()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(newToken) != 7 {
			t.Errorf("Expected new token length to be 7, got %d", len(newToken))
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestShell_GenerateRandomString(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		shell, mocks := setup(t)
		mocks.Shims.RandRead = func(b []byte) (n int, err error) {
			for i := range b {
				b[i] = byte(i % 62) // This will map to characters in the charset
			}
			return len(b), nil
		}
		result, err := shell.generateRandomString(10)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(result) != 10 {
			t.Errorf("Expected length 10, got %d", len(result))
		}
	})

	t.Run("ErrorWhenRandReadFails", func(t *testing.T) {
		shell, mocks := setup(t)
		mocks.Shims.RandRead = func(b []byte) (n int, err error) {
			return 0, fmt.Errorf("random read failed")
		}
		result, err := shell.generateRandomString(10)
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if result != "" {
			t.Errorf("Expected empty result on error, got %q", result)
		}
	})
}
