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

func TestShell_AddCurrentDirToTrustedFile(t *testing.T) {
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
		homeDir := "/home/test"
		trustedFilePath := "/home/test/.config/windsor/.trusted"

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

		// Mock UserHomeDir
		mocks.Shims.UserHomeDir = func() (string, error) {
			return homeDir, nil
		}

		// Mock file operations
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			if path != "/home/test/.config/windsor" {
				t.Errorf("Expected path /home/test/.config/windsor, got %s", path)
			}
			return nil
		}

		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			if name != trustedFilePath {
				t.Errorf("Expected path %s, got %s", trustedFilePath, name)
			}
			return []byte("/other/project\n"), nil
		}

		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			if name != trustedFilePath {
				t.Errorf("Expected path %s, got %s", trustedFilePath, name)
			}
			expectedData := []byte("/other/project\n" + projectRoot + "\n")
			if string(data) != string(expectedData) {
				t.Errorf("Expected data %s, got %s", string(expectedData), string(data))
			}
			if perm != 0600 {
				t.Errorf("Expected perm 0600, got %o", perm)
			}
			return nil
		}

		// When adding current dir to trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorOnGetProjectRoot", func(t *testing.T) {
		// Given a shell with failing GetProjectRoot
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("getwd failed")
		}

		// When adding current dir to trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error getting project root directory") {
			t.Errorf("Expected error about project root, got: %v", err)
		}
	})

	t.Run("ErrorOnUserHomeDir", func(t *testing.T) {
		// Given a shell with failing UserHomeDir
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
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "", fmt.Errorf("user home dir failed")
		}

		// When adding current dir to trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error getting user home directory") {
			t.Errorf("Expected error about user home dir, got: %v", err)
		}
	})

	t.Run("ErrorOnMkdirAll", func(t *testing.T) {
		// Given a shell with failing MkdirAll
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
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "/home/test", nil
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mkdir failed")
		}

		// When adding current dir to trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error creating directories for trusted file") {
			t.Errorf("Expected error about mkdir, got: %v", err)
		}
	})

	t.Run("ErrorOnReadFile", func(t *testing.T) {
		// Given a shell with failing ReadFile
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
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "/home/test", nil
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return nil, fmt.Errorf("read failed")
		}

		// When adding current dir to trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error reading trusted file") {
			t.Errorf("Expected error about read file, got: %v", err)
		}
	})

	t.Run("ErrorOnWriteFile", func(t *testing.T) {
		// Given a shell with failing WriteFile
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
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "/home/test", nil
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("/other/project\n"), nil
		}
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("write failed")
		}

		// When adding current dir to trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error writing to trusted file") {
			t.Errorf("Expected error about write file, got: %v", err)
		}
	})

	t.Run("AlreadyTrusted", func(t *testing.T) {
		// Given a shell with current dir already trusted
		shell, mocks := setup(t)
		projectRoot := "/test/project"
		mocks.Shims.Getwd = func() (string, error) {
			return projectRoot, nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join(projectRoot, "windsor.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "/home/test", nil
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte(projectRoot + "\n"), nil
		}

		// When adding current dir to trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then it should succeed without writing
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("NoOpInGlobalMode", func(t *testing.T) {
		// Given a shell that has resolved to global mode
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/some/random/dir", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "/home/test", nil
		}
		writeCalled := false
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			writeCalled = true
			return nil
		}

		// When adding current dir to trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then it should succeed without writing the trust file
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !shell.IsGlobal() {
			t.Error("Expected shell to be in global mode")
		}
		if writeCalled {
			t.Error("Expected WriteFile to not be called in global mode")
		}
	})
}

func TestShell_CheckTrustedDirectory(t *testing.T) {
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
		homeDir := "/home/test"
		trustedFilePath := "/home/test/.config/windsor/.trusted"

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

		// Mock UserHomeDir
		mocks.Shims.UserHomeDir = func() (string, error) {
			return homeDir, nil
		}

		// Mock ReadFile
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			if name != trustedFilePath {
				t.Errorf("Expected path %s, got %s", trustedFilePath, name)
			}
			return []byte("/test\n"), nil
		}

		// When checking trusted directory
		err := shell.CheckTrustedDirectory()

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorOnGetProjectRoot", func(t *testing.T) {
		// Given a shell with failing GetProjectRoot
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("getwd failed")
		}

		// When checking trusted directory
		err := shell.CheckTrustedDirectory()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error getting project root directory") {
			t.Errorf("Expected error about project root, got: %v", err)
		}
	})

	t.Run("ErrorOnUserHomeDir", func(t *testing.T) {
		// Given a shell with failing UserHomeDir
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
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "", fmt.Errorf("user home dir failed")
		}

		// When checking trusted directory
		err := shell.CheckTrustedDirectory()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error getting user home directory") {
			t.Errorf("Expected error about user home dir, got: %v", err)
		}
	})

	t.Run("ErrorOnReadFile", func(t *testing.T) {
		// Given a shell with failing ReadFile
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
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "/home/test", nil
		}
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return nil, fmt.Errorf("read failed")
		}

		// When checking trusted directory
		err := shell.CheckTrustedDirectory()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error reading trusted file") {
			t.Errorf("Expected error about read file, got: %v", err)
		}
	})

	t.Run("TrustedFileNotExist", func(t *testing.T) {
		// Given a shell with non-existent trusted file
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
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "/home/test", nil
		}
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		// When checking trusted directory
		err := shell.CheckTrustedDirectory()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Trusted file does not exist") {
			t.Errorf("Expected error about file not existing, got: %v", err)
		}
	})

	t.Run("NotTrusted", func(t *testing.T) {
		// Given a shell with directory not in trusted list
		shell, mocks := setup(t)
		projectRoot := "/test/project"
		mocks.Shims.Getwd = func() (string, error) {
			return projectRoot, nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join(projectRoot, "windsor.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "/home/test", nil
		}
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("/other/project\n"), nil
		}

		// When checking trusted directory
		err := shell.CheckTrustedDirectory()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Current directory not in the trusted list") {
			t.Errorf("Expected error about directory not trusted, got: %v", err)
		}
	})

	t.Run("PathMatching", func(t *testing.T) {
		cases := []struct {
			name        string
			projectRoot string
			trusted     string
			wantTrusted bool
		}{
			{"ExactMatch", "/test/project", "/test/project", true},
			{"ChildOfTrusted", "/test/project/sub", "/test/project", true},
			{"TrailingSlashEntry", "/test/project", "/test/project/", true},
			{"SiblingSharingPrefix", "/test/project-evil", "/test/project", false},
			{"SiblingWithSuffixDigit", "/test/other2", "/test/other", false},
			{"ParentOfTrusted", "/test", "/test/project", false},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				// Given a project root and a single trusted entry
				shell, mocks := setup(t)
				mocks.Shims.Getwd = func() (string, error) { return tc.projectRoot, nil }
				mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
					if name == filepath.Join(tc.projectRoot, "windsor.yaml") {
						return nil, nil
					}
					return nil, os.ErrNotExist
				}
				mocks.Shims.UserHomeDir = func() (string, error) { return "/home/test", nil }
				mocks.Shims.ReadFile = func(name string) ([]byte, error) {
					return []byte(tc.trusted + "\n"), nil
				}

				// When checking the trusted directory
				err := shell.CheckTrustedDirectory()

				// Then trust follows path components, not string prefixes
				if tc.wantTrusted && err != nil {
					t.Errorf("Expected trusted, got %v", err)
				}
				if !tc.wantTrusted && err == nil {
					t.Error("Expected not trusted, got nil")
				}
			})
		}
	})

	t.Run("NoOpInGlobalMode", func(t *testing.T) {
		// Given a shell that has resolved to global mode
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/some/random/dir", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "/home/test", nil
		}
		readCalled := false
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			readCalled = true
			return nil, os.ErrNotExist
		}

		// When checking trusted directory
		err := shell.CheckTrustedDirectory()

		// Then it should succeed without reading the trust file
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !shell.IsGlobal() {
			t.Error("Expected shell to be in global mode")
		}
		if readCalled {
			t.Error("Expected ReadFile to not be called in global mode")
		}
	})
}
