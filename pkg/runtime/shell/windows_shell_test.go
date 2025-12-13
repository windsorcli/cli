//go:build windows
// +build windows

package shell

import (
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

// TestDefaultShell_GetProjectRoot tests the GetProjectRoot method on Windows systems
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

// TestDefaultShell_UnsetEnvs tests the UnsetEnvs method on Windows systems
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

// TestDefaultShell_RenderEnvVars tests the RenderEnvVars method on Windows systems
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

		// Then the output should contain PowerShell syntax
		if !strings.Contains(result, "$env:VAR1='value1'") {
			t.Errorf("Expected PowerShell syntax, got: %s", result)
		}
		if !strings.Contains(result, "$env:VAR2='value2'") {
			t.Errorf("Expected PowerShell syntax, got: %s", result)
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
		if !strings.Contains(result, "VAR2=value2") {
			t.Errorf("Expected plain format, got: %s", result)
		}
	})
}

// TestDefaultShell_PrintEnvVars tests the PrintEnvVars method on Windows systems
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

		// Then the output should contain PowerShell syntax
		if !strings.Contains(output, "$env:VAR1='value1'") {
			t.Errorf("Expected PowerShell syntax, got: %s", output)
		}
		if !strings.Contains(output, "$env:VAR2='value2'") {
			t.Errorf("Expected PowerShell syntax, got: %s", output)
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
		if !strings.Contains(output, "VAR2=value2") {
			t.Errorf("Expected plain format, got: %s", output)
		}
	})
}

// TestDefaultShell_quoteValueForPowerShell tests the quoteValueForPowerShell method
func TestDefaultShell_quoteValueForPowerShell(t *testing.T) {
	setup := func(t *testing.T) *DefaultShell {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell
	}

	t.Run("QuotesValuesWithSpecialCharacters", func(t *testing.T) {
		shell := setup(t)

		testCases := []struct {
			name     string
			value    string
			expected string
		}{
			{"JSONArray", `["item1","item2"]`, `'["item1","item2"]'`},
			{"JSONObject", `{"key": "value"}`, `'{"key": "value"}'`},
			{"WithSpaces", "value with spaces", "'value with spaces'"},
			{"WithDollars", "value$test", "'value$test'"},
			{"WithBrackets", "[test]", "'[test]'"},
			{"SimpleValue", "simple", "'simple'"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := shell.quoteValueForPowerShell(tc.value)
				if result != tc.expected {
					t.Errorf("Expected %q, got %q", tc.expected, result)
				}
			})
		}
	})

	t.Run("EscapesSingleQuotesByDoubling", func(t *testing.T) {
		shell := setup(t)

		result := shell.quoteValueForPowerShell("value'with'quotes")
		expected := `'value''with''quotes'`
		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	})
}

// TestDefaultShell_renderEnvVarsWithExport_PowerShell tests renderEnvVarsWithExport with special characters
func TestDefaultShell_renderEnvVarsWithExport_PowerShell(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

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
		if !strings.Contains(result, "$env:VAR1='[\"item1\",\"item2\"]'\n") {
			t.Errorf("Expected VAR1 to be quoted, got: %s", result)
		}
		if !strings.Contains(result, "$env:VAR2='{\"key\": \"value\"}'\n") {
			t.Errorf("Expected VAR2 to be quoted, got: %s", result)
		}
		if !strings.Contains(result, "$env:VAR3='simple_value'\n") {
			t.Errorf("Expected VAR3 to be quoted, got: %s", result)
		}
		if !strings.Contains(result, "$env:VAR4='value with spaces'\n") {
			t.Errorf("Expected VAR4 to be quoted, got: %s", result)
		}
	})
}
