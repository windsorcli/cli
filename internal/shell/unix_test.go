//go:build darwin || linux
// +build darwin linux

package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/di"
)

func TestDefaultShell_PrintEnvVars(t *testing.T) {
	injector := di.NewInjector()

	// Given a set of environment variables
	shell := NewDefaultShell(injector)
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
}

func TestDefaultShell_GetProjectRoot(t *testing.T) {
	testCases := []struct {
		name     string
		fileName string
	}{
		{"WindsorYaml", "windsor.yaml"},
		{"WindsorYml", "windsor.yml"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			injector := di.NewInjector()

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

			shell := NewDefaultShell(injector)

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
}

func TestDefaultShell_PrintAlias(t *testing.T) {
	aliasVars := map[string]string{
		"ALIAS1": "command1",
		"ALIAS2": "command2",
	}

	t.Run("PrintAlias", func(t *testing.T) {
		injector := di.NewInjector()

		// Given a default shell
		shell := NewDefaultShell(injector)

		// Capture the output of PrintAlias
		output := captureStdout(t, func() {
			err := shell.PrintAlias(aliasVars)
			if err != nil {
				t.Fatalf("PrintAlias returned an error: %v", err)
			}
		})

		// Then the output should contain all expected alias variables
		for key, value := range aliasVars {
			expectedLine := fmt.Sprintf("alias %s=\"%s\"\n", key, value)
			if !strings.Contains(output, expectedLine) {
				t.Errorf("PrintAlias() output missing expected line: %v", expectedLine)
			}
		}
	})

	t.Run("PrintAliasWithEmptyValue", func(t *testing.T) {
		injector := di.NewInjector()

		// Given a default shell with an alias having an empty value
		shell := NewDefaultShell(injector)
		aliasVarsWithEmpty := map[string]string{
			"ALIAS1": "command1",
			"ALIAS2": "",
		}

		// Capture the output of PrintAlias
		output := captureStdout(t, func() {
			err := shell.PrintAlias(aliasVarsWithEmpty)
			if err != nil {
				t.Fatalf("PrintAlias returned an error: %v", err)
			}
		})

		// Then the output should contain the expected alias and unalias commands
		expectedAliasLine := fmt.Sprintf("alias ALIAS1=\"command1\"\n")
		expectedUnaliasLine := fmt.Sprintf("unalias ALIAS2\n")

		if !strings.Contains(output, expectedAliasLine) {
			t.Errorf("PrintAlias() output missing expected line: %v", expectedAliasLine)
		}
		if !strings.Contains(output, expectedUnaliasLine) {
			t.Errorf("PrintAlias() output missing expected line: %v", expectedUnaliasLine)
		}
	})
}
