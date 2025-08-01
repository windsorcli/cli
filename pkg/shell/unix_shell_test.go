//go:build darwin || linux
// +build darwin linux

package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The UnixShellTest is a test suite for Unix-specific shell operations.
// It provides comprehensive test coverage for environment variable management,
// project root detection, and alias handling on Unix-like systems.
// The UnixShellTest ensures reliable shell operations on macOS and Linux platforms.

// =============================================================================
// Test Public Methods
// =============================================================================

// TestDefaultShell_PrintEnvVars tests the PrintEnvVars method on Unix systems
func TestDefaultShell_PrintEnvVars(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		shell := NewDefaultShell(mocks.Injector)
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("PrintEnvVars", func(t *testing.T) {
		// Given a shell with environment variables
		shell, _ := setup(t)
		envVars := map[string]string{
			"VAR2": "value2",
			"VAR1": "value1",
			"VAR3": "",
		}
		expectedOutput := "export VAR1=\"value1\"\nexport VAR2=\"value2\"\nunset VAR3\n"

		// When capturing the output of PrintEnvVars
		output := captureStdout(t, func() {
			shell.PrintEnvVars(envVars, true)
		})

		// Then the output should match the expected output
		if output != expectedOutput {
			t.Errorf("PrintEnvVars() output = %v, want %v", output, expectedOutput)
		}
	})
}

// TestDefaultShell_GetProjectRoot tests the GetProjectRoot method on Unix systems
func TestDefaultShell_GetProjectRoot(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		shell := NewDefaultShell(mocks.Injector)
		shell.shims = mocks.Shims
		return shell, mocks
	}

	testCases := []struct {
		name     string
		fileName string
	}{
		{"WindsorYaml", "windsor.yaml"},
		{"WindsorYml", "windsor.yml"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Given a shell with mocked file system
			shell, mocks := setup(t)

			// Mock the file system behavior
			rootDir := mocks.TmpDir
			subDir := filepath.Join(rootDir, "subdir")

			// Override Getwd to return the subdirectory
			shell.shims.Getwd = func() (string, error) {
				return subDir, nil
			}

			// Override Stat to return nil for the windsor file
			shell.shims.Stat = func(name string) (os.FileInfo, error) {
				if name == filepath.Join(rootDir, tc.fileName) {
					return nil, nil
				}
				return nil, os.ErrNotExist
			}

			// When finding the project root using the specified file
			projectRoot, err := shell.GetProjectRoot()
			if err != nil {
				t.Fatalf("GetProjectRoot returned an error: %v", err)
			}

			// Then the project root should match the expected root directory
			if projectRoot != rootDir {
				t.Errorf("Expected project root to be %s, got %s", rootDir, projectRoot)
			}
		})
	}
}

// TestDefaultShell_PrintAlias tests the PrintAlias method on Unix systems
func TestDefaultShell_PrintAlias(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		shell := NewDefaultShell(mocks.Injector)
		shell.shims = mocks.Shims
		return shell, mocks
	}

	aliasVars := map[string]string{
		"ALIAS1": "command1",
		"ALIAS2": "command2",
	}

	t.Run("PrintAlias", func(t *testing.T) {
		// Given a default shell
		shell, _ := setup(t)

		// When capturing the output of PrintAlias
		output := captureStdout(t, func() {
			shell.PrintAlias(aliasVars)
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
		// Given a default shell with an alias having an empty value
		shell, _ := setup(t)
		aliasVarsWithEmpty := map[string]string{
			"ALIAS1": "command1",
			"ALIAS2": "",
		}

		// When capturing the output of PrintAlias
		output := captureStdout(t, func() {
			shell.PrintAlias(aliasVarsWithEmpty)
		})

		// Then the output should contain the expected alias and unalias commands
		expectedAliasLine := "alias ALIAS1=\"command1\"\n"
		expectedUnaliasLine := "unalias ALIAS2\n"

		if !strings.Contains(output, expectedAliasLine) {
			t.Errorf("PrintAlias() output missing expected line: %v", expectedAliasLine)
		}
		if !strings.Contains(output, expectedUnaliasLine) {
			t.Errorf("PrintAlias() output missing expected line: %v", expectedUnaliasLine)
		}
	})
}

// TestDefaultShell_UnsetEnvs tests the UnsetEnvs method on Unix systems
func TestDefaultShell_UnsetEnvs(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		shell := NewDefaultShell(mocks.Injector)
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("UnsetEnvs", func(t *testing.T) {
		// Given a set of environment variables to unset
		shell, _ := setup(t)
		envVars := []string{"VAR1", "VAR2", "VAR3"}
		expectedOutput := "unset VAR1 VAR2 VAR3\n"

		// When capturing the output of UnsetEnvs
		output := captureStdout(t, func() {
			shell.UnsetEnvs(envVars)
		})

		// Then the output should match the expected output
		if output != expectedOutput {
			t.Errorf("UnsetEnvs() output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("UnsetEnvsWithEmptyList", func(t *testing.T) {
		// Given an empty list of environment variables
		shell, _ := setup(t)

		// When capturing the output of UnsetEnvs
		output := captureStdout(t, func() {
			shell.UnsetEnvs([]string{})
		})

		// Then the output should be empty
		if output != "" {
			t.Errorf("UnsetEnvs() with empty list should produce no output, got %v", output)
		}
	})
}

// TestDefaultShell_UnsetAlias tests the UnsetAlias method on Unix systems
func TestDefaultShell_UnsetAlias(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		shell := NewDefaultShell(mocks.Injector)
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("UnsetAlias", func(t *testing.T) {
		// Given a set of aliases to unset
		shell, _ := setup(t)
		aliases := []string{"ALIAS1", "ALIAS2", "ALIAS3"}
		expectedOutput := "unalias ALIAS1\nunalias ALIAS2\nunalias ALIAS3\n"

		// When capturing the output of UnsetAlias
		output := captureStdout(t, func() {
			shell.UnsetAlias(aliases)
		})

		// Then the output should match the expected output
		if output != expectedOutput {
			t.Errorf("UnsetAlias() output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("UnsetAliasWithEmptyList", func(t *testing.T) {
		// Given an empty list of aliases
		shell, _ := setup(t)

		// When capturing the output of UnsetAlias
		output := captureStdout(t, func() {
			shell.UnsetAlias([]string{})
		})

		// Then the output should be empty
		if output != "" {
			t.Errorf("UnsetAlias() with empty list should produce no output, got %v", output)
		}
	})
}
