//go:build windows
// +build windows

package shell

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// Helper function to get the long path name on Windows
func getLongPathName(shortPath string) (string, error) {
	p, err := windows.UTF16PtrFromString(shortPath)
	if err != nil {
		return "", err
	}
	b := make([]uint16, windows.MAX_LONG_PATH)
	r, err := windows.GetLongPathName(p, &b[0], uint32(len(b)))
	if r == 0 {
		return "", err
	}
	return windows.UTF16ToString(b), nil
}

// Helper function to normalize a Windows path
func normalizeWindowsPath(path string) string {
	longPath, err := getLongPathName(path)
	if err != nil {
		return normalizePath(path)
	}
	return normalizePath(longPath)
}

func TestDefaultShell_PrintEnvVars_Windows(t *testing.T) {
	t.Run("PrintEnvVars", func(t *testing.T) {
		// Given a default shell and environment variables
		shell := NewDefaultShell()
		envVars := map[string]string{
			"VAR2": "value2",
			"VAR1": "value1",
			"VAR3": "",
		}

		// Expected output for PowerShell
		expectedOutputPowerShell := "$env:VAR1=\"value1\"\n$env:VAR2=\"value2\"\nRemove-Item Env:VAR3\n"

		// Capture the output
		var output bytes.Buffer
		originalStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		go func() {
			shell.PrintEnvVars(envVars)
			w.Close()
		}()

		output.ReadFrom(r)
		os.Stdout = originalStdout

		// Then the output should match the expected PowerShell format
		if output.String() != expectedOutputPowerShell {
			t.Errorf("PrintEnvVars() output = %v, want %v", output.String(), expectedOutputPowerShell)
		}
	})
}

func TestGetProjectRoot_Windows(t *testing.T) {
	testCases := []struct {
		name     string
		fileName string
	}{
		{"WindsorYaml", "windsor.yaml"},
		{"WindsorYml", "windsor.yml"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Given a temporary directory structure with the specified file
			rootDir := createTempDir(t, "project-root")
			subDir := filepath.Join(rootDir, "subdir")
			if err := os.Mkdir(subDir, 0755); err != nil {
				t.Fatalf("Failed to create subdir: %v", err)
			}
			createFile(t, rootDir, tc.fileName)
			changeDir(t, subDir)

			shell := NewDefaultShell()

			// When finding the project root using the specified file
			projectRoot, err := shell.GetProjectRoot()
			if err != nil {
				t.Fatalf("GetProjectRoot returned an error: %v", err)
			}

			// Resolve symlinks to handle macOS /private prefix
			expectedRootDir, err := filepath.EvalSymlinks(rootDir)
			if err != nil {
				t.Fatalf("Failed to evaluate symlinks for rootDir: %v", err)
			}

			// Normalize paths for comparison
			expectedRootDir = normalizeWindowsPath(expectedRootDir)
			projectRoot = normalizeWindowsPath(projectRoot)

			// Then the project root should match the expected root directory
			if projectRoot != expectedRootDir {
				t.Errorf("Expected project root to be %s, got %s", expectedRootDir, projectRoot)
			}
		})
	}
}
