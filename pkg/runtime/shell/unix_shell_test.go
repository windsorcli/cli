//go:build darwin || linux
// +build darwin linux

package shell

import (
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

// TestDefaultShell_GetProjectRoot tests the GetProjectRoot method on Unix systems
func TestDefaultShell_GetProjectRoot(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
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

// TestDefaultShell_UnsetEnvs tests the UnsetEnvs method on Unix systems
func TestDefaultShell_UnsetEnvs(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
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
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
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

// TestDefaultShell_RenderAliases tests the RenderAliases method on Unix systems
func TestDefaultShell_RenderAliases(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("RendersAliasesWithValues", func(t *testing.T) {
		// Given a shell with aliases
		shell, _ := setup(t)
		aliases := map[string]string{
			"ll":   "ls -la",
			"grep": "grep --color=auto",
			"cd":   "",
		}

		// When rendering aliases
		result := shell.RenderAliases(aliases)

		// Then the output should contain sorted alias commands
		if !strings.Contains(result, "alias cd=\"\"\n") && !strings.Contains(result, "unalias cd\n") {
			t.Errorf("Expected unalias or empty alias for cd, got: %s", result)
		}
		if !strings.Contains(result, "alias grep=\"grep --color=auto\"\n") {
			t.Errorf("Expected alias for grep, got: %s", result)
		}
		if !strings.Contains(result, "alias ll=\"ls -la\"\n") {
			t.Errorf("Expected alias for ll, got: %s", result)
		}
	})

	t.Run("RendersEmptyAliases", func(t *testing.T) {
		// Given a shell with empty aliases map
		shell, _ := setup(t)
		aliases := map[string]string{}

		// When rendering aliases
		result := shell.RenderAliases(aliases)

		// Then the output should be empty
		if result != "" {
			t.Errorf("Expected empty output for empty aliases, got: %s", result)
		}
	})

	t.Run("RendersAliasesWithEmptyValues", func(t *testing.T) {
		// Given a shell with aliases that have empty values
		shell, _ := setup(t)
		aliases := map[string]string{
			"alias1": "",
			"alias2": "",
		}

		// When rendering aliases
		result := shell.RenderAliases(aliases)

		// Then the output should contain unalias commands
		if !strings.Contains(result, "unalias") {
			t.Errorf("Expected unalias commands for empty aliases, got: %s", result)
		}
	})
}

// TestDefaultShell_renderEnvVarsWithExport tests the renderEnvVarsWithExport method on Unix systems
func TestDefaultShell_renderEnvVarsWithExport(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("RendersEnvVarsWithExport", func(t *testing.T) {
		// Given a shell with environment variables
		shell, _ := setup(t)
		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
			"VAR3": "",
		}

		// When rendering environment variables with export
		result := shell.renderEnvVarsWithExport(envVars)

		// Then the output should contain sorted export commands
		if !strings.Contains(result, "export VAR1=\"value1\"\n") {
			t.Errorf("Expected export VAR1, got: %s", result)
		}
		if !strings.Contains(result, "export VAR2=\"value2\"\n") {
			t.Errorf("Expected export VAR2, got: %s", result)
		}
		if !strings.Contains(result, "unset VAR3\n") {
			t.Errorf("Expected unset VAR3, got: %s", result)
		}
	})

	t.Run("RendersEmptyEnvVars", func(t *testing.T) {
		// Given a shell with empty environment variables map
		shell, _ := setup(t)
		envVars := map[string]string{}

		// When rendering environment variables with export
		result := shell.renderEnvVarsWithExport(envVars)

		// Then the output should be empty
		if result != "" {
			t.Errorf("Expected empty output for empty env vars, got: %s", result)
		}
	})

	t.Run("QuotesValuesWithSpecialCharacters", func(t *testing.T) {
		// Given a shell with environment variables containing special characters
		shell, _ := setup(t)
		envVars := map[string]string{
			"VAR1": `["item1","item2"]`,
			"VAR2": `{"key": "value"}`,
			"VAR3": "simple_value",
			"VAR4": "value with spaces",
		}

		// When rendering environment variables with export
		result := shell.renderEnvVarsWithExport(envVars)

		// Then values with special characters should be quoted with single quotes
		if !strings.Contains(result, "export VAR1='[\"item1\",\"item2\"]'\n") {
			t.Errorf("Expected VAR1 to be quoted, got: %s", result)
		}
		if !strings.Contains(result, "export VAR2='{\"key\": \"value\"}'\n") {
			t.Errorf("Expected VAR2 to be quoted, got: %s", result)
		}
		if !strings.Contains(result, "export VAR3=\"simple_value\"\n") {
			t.Errorf("Expected VAR3 to use double quotes, got: %s", result)
		}
		if !strings.Contains(result, "export VAR4='value with spaces'\n") {
			t.Errorf("Expected VAR4 to be quoted, got: %s", result)
		}
	})
}

// TestDefaultShell_RenderEnvVars tests the RenderEnvVars method on Unix systems
func TestDefaultShell_RenderEnvVars(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("RendersEnvVarsWithExport", func(t *testing.T) {
		// Given a shell with environment variables
		shell, _ := setup(t)
		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}

		// When rendering environment variables with export=true
		result := shell.RenderEnvVars(envVars, true)

		// Then the output should contain export commands
		if !strings.Contains(result, "export") {
			t.Errorf("Expected export commands, got: %s", result)
		}
	})

	t.Run("RendersEnvVarsWithoutExport", func(t *testing.T) {
		// Given a shell with environment variables
		shell, _ := setup(t)
		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}

		// When rendering environment variables with export=false
		result := shell.RenderEnvVars(envVars, false)

		// Then the output should contain plain KEY=value format
		if !strings.Contains(result, "VAR1=value1") {
			t.Errorf("Expected plain format, got: %s", result)
		}
		if strings.Contains(result, "export") {
			t.Errorf("Expected no export commands, got: %s", result)
		}
	})
}

// TestDefaultShell_PrintEnvVars tests the PrintEnvVars method on Unix systems
func TestDefaultShell_PrintEnvVars(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("PrintsEnvVarsWithExport", func(t *testing.T) {
		// Given a shell with environment variables
		shell, _ := setup(t)
		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}

		// When printing environment variables with export=true
		output := captureStdout(t, func() {
			shell.PrintEnvVars(envVars, true)
		})

		// Then the output should contain export commands
		if !strings.Contains(output, "export") {
			t.Errorf("Expected export commands, got: %s", output)
		}
	})

	t.Run("PrintsEnvVarsWithoutExport", func(t *testing.T) {
		// Given a shell with environment variables
		shell, _ := setup(t)
		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}

		// When printing environment variables with export=false
		output := captureStdout(t, func() {
			shell.PrintEnvVars(envVars, false)
		})

		// Then the output should contain plain KEY=value format
		if !strings.Contains(output, "VAR1=value1") {
			t.Errorf("Expected plain format, got: %s", output)
		}
		if strings.Contains(output, "export") {
			t.Errorf("Expected no export commands, got: %s", output)
		}
	})
}
