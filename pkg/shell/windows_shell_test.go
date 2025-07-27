//go:build windows
// +build windows

package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The WindowsShellTest is a test suite for Windows-specific shell operations.
// It provides comprehensive test coverage for PowerShell environment management,
// project root detection, and alias handling on Windows systems.
// The WindowsShellTest ensures reliable shell operations on Windows platforms.

// =============================================================================
// Test Public Methods
// =============================================================================

// TestDefaultShell_PrintEnvVars tests the PrintEnvVars method on Windows systems
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
		expectedOutput := "$env:VAR1='value1'\n$env:VAR2='value2'\nRemove-Item Env:VAR3\n"

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

// TestDefaultShell_GetProjectRoot tests the GetProjectRoot method on Windows systems
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
			rootDir := "C:\\test\\project"
			subDir := filepath.Join(rootDir, "subdir")

			// Mock Getwd to return the subdirectory
			mocks.Shims.Getwd = func() (string, error) {
				return subDir, nil
			}

			// Mock Stat to return nil for the windsor file
			mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
				if name == filepath.Join(rootDir, tc.fileName) {
					return nil, nil
				}
				return nil, os.ErrNotExist
			}

			// When finding the project root
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

// TestDefaultShell_PrintAlias tests the PrintAlias method on Windows systems
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
			expectedLine := fmt.Sprintf("Set-Alias -Name %s -Value \"%s\"\n", key, value)
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

		// Then the output should contain the expected alias and remove alias commands
		expectedAliasLine := "Set-Alias -Name ALIAS1 -Value \"command1\"\n"
		expectedRemoveAliasLine := "Remove-Item Alias:ALIAS2\n"

		if !strings.Contains(output, expectedAliasLine) {
			t.Errorf("PrintAlias() output missing expected line: %v", expectedAliasLine)
		}
		if !strings.Contains(output, expectedRemoveAliasLine) {
			t.Errorf("PrintAlias() output missing expected line: %v", expectedRemoveAliasLine)
		}
	})
}

// TestDefaultShell_UnsetEnvs tests the UnsetEnvs method on Windows systems
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
		expectedOutput := "Remove-Item Env:VAR1\nRemove-Item Env:VAR2\nRemove-Item Env:VAR3\n"

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

// TestDefaultShell_UnsetAlias tests the UnsetAlias method on Windows systems
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
		expectedOutput := "Remove-Item Alias:ALIAS1\nRemove-Item Alias:ALIAS2\nRemove-Item Alias:ALIAS3\n"

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
