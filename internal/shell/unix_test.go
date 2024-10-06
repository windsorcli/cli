//go:build darwin || linux
// +build darwin linux

package shell

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultShell(t *testing.T) {
	t.Run("PrintEnvVars", func(t *testing.T) {
		// Given a set of environment variables
		shell := NewDefaultShell()
		envVars := map[string]string{
			"VAR2": "value2",
			"VAR1": "value1",
			"VAR3": "",
		}
		expectedOutput := "export VAR1=\"value1\"\nexport VAR2=\"value2\"\nunset VAR3\n"

		// When capturing the output of PrintEnvVars
		output := captureStdout(t, func() {
			shell.PrintEnvVars(envVars)
		})

		// Then the output should match the expected output
		if output != expectedOutput {
			t.Errorf("PrintEnvVars() output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("GetProjectRoot", func(t *testing.T) {
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
				defer os.RemoveAll(rootDir)

				subDir := filepath.Join(rootDir, "subdir")
				if err := os.Mkdir(subDir, 0755); err != nil {
					t.Fatalf("Failed to create subdir: %v", err)
				}

				// When creating the specified file in the root directory
				createFile(t, rootDir, tc.fileName, "")

				// And changing the working directory to subDir
				changeDir(t, subDir)

				shell := NewDefaultShell()

				// Then GetProjectRoot should find the project root using the specified file
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
				expectedRootDir = normalizePath(expectedRootDir)
				projectRoot = normalizePath(projectRoot)

				if projectRoot != expectedRootDir {
					t.Errorf("Expected project root to be %s, got %s", expectedRootDir, projectRoot)
				}
			})
		}
	})
}
