//go:build windows
// +build windows

package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
	"golang.org/x/sys/windows"
)

// Helper function to get the long path name on Windows
// This function converts a short path to its long form
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
// This function ensures the path is in its long form and normalized
func normalizeWindowsPath(path string) string {
	longPath, err := getLongPathName(path)
	if err != nil {
		return normalizePath(path)
	}
	return normalizePath(longPath)
}

func TestDefaultShell_PrintEnvVars(t *testing.T) {
	injector := di.NewInjector()

	// Given a default shell and a set of environment variables
	shell := NewDefaultShell(injector)
	envVars := map[string]string{
		"VAR2": "value2",
		"VAR1": "value1",
		"VAR3": "",
	}

	// Expected output for PowerShell
	expectedOutputPowerShell := "$env:VAR1='value1'\n$env:VAR2='value2'\nRemove-Item Env:VAR3\n"

	// Capture the output using the captureStdout helper
	output := captureStdout(t, func() {
		err := shell.PrintEnvVars(envVars)
		if err != nil {
			t.Fatalf("PrintEnvVars returned an error: %v", err)
		}
	})

	// Then the output should match the expected PowerShell format
	if output != expectedOutputPowerShell {
		t.Errorf("PrintEnvVars() output = %v, want %v", output, expectedOutputPowerShell)
	}
}

func TestDefaultShell_PrintEnvVarsErrors(t *testing.T) {
	injector := di.NewInjector()

	t.Run("SetenvError", func(t *testing.T) {
		// Save the original functions to restore them later
		originalOsSetenv := osSetenv
		defer func() { osSetenv = originalOsSetenv }()

		// Mock osSetenv to return an error
		osSetenv = func(key, value string) error {
			if key == "ERROR_VAR" {
				return fmt.Errorf("simulated error setting %s", key)
			}
			return nil
		}

		// Given a shell and environment variables with one that will cause an error
		shell := NewDefaultShell(injector)
		envVars := map[string]string{
			"NORMAL_VAR": "value",
			"ERROR_VAR":  "error_value",
		}

		// Capture stdout to prevent output in test
		_ = captureStdout(t, func() {
			// When calling PrintEnvVars
			err := shell.PrintEnvVars(envVars)

			// Then an error should be returned
			if err == nil {
				t.Fatal("PrintEnvVars did not return the expected error")
			}

			// And the error message should include the variable name
			expectedErrorMsg := "failed to set environment variable ERROR_VAR"
			if !strings.Contains(err.Error(), expectedErrorMsg) {
				t.Errorf("PrintEnvVars() error = %v, expected to contain %v", err, expectedErrorMsg)
			}
		})
	})

	t.Run("UnsetenvError", func(t *testing.T) {
		// Save the original function to restore it later
		originalOsUnsetenv := osUnsetenv
		defer func() { osUnsetenv = originalOsUnsetenv }()

		// Mock osUnsetenv to return an error
		osUnsetenv = func(key string) error {
			if key == "EMPTY_ERROR_VAR" {
				return fmt.Errorf("simulated error unsetting %s", key)
			}
			return nil
		}

		// Given a shell and environment variables with an empty one that will cause an error
		shell := NewDefaultShell(injector)
		envVars := map[string]string{
			"NORMAL_VAR":      "value",
			"EMPTY_ERROR_VAR": "",
		}

		// Capture stdout to prevent output in test
		_ = captureStdout(t, func() {
			// When calling PrintEnvVars
			err := shell.PrintEnvVars(envVars)

			// Then an error should be returned
			if err == nil {
				t.Fatal("PrintEnvVars did not return the expected error")
			}

			// And the error message should include the variable name
			expectedErrorMsg := "failed to unset environment variable EMPTY_ERROR_VAR"
			if !strings.Contains(err.Error(), expectedErrorMsg) {
				t.Errorf("PrintEnvVars() error = %v, expected to contain %v", err, expectedErrorMsg)
			}
		})
	})
}

func TestDefaultShell_GetProjectRoot(t *testing.T) {
	injector := di.NewInjector()

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

			// When creating the specified file in the root directory
			createFile(t, rootDir, tc.fileName, "")

			// And changing the working directory to subDir
			changeDir(t, subDir)

			shell := NewDefaultShell(injector)

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

func TestDefaultShell_PrintAlias(t *testing.T) {
	aliasVars := map[string]string{
		"ALIAS1": "command1",
		"ALIAS2": "command2",
	}

	t.Run("PrintAlias", func(t *testing.T) {
		// Given a default shell
		injector := di.NewInjector()
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
			expectedLine := fmt.Sprintf("Set-Alias -Name %s -Value \"%s\"\n", key, value)
			if !strings.Contains(output, expectedLine) {
				t.Errorf("PrintAlias() output missing expected line: %v", expectedLine)
			}
		}
	})

	t.Run("PrintAliasWithEmptyValue", func(t *testing.T) {
		// Given a default shell with an alias having an empty value
		injector := di.NewInjector()
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

		// Then the output should contain the expected alias and remove alias commands
		expectedAliasLine := fmt.Sprintf("Set-Alias -Name ALIAS1 -Value \"command1\"\n")
		expectedRemoveAliasLine := fmt.Sprintf("Remove-Item Alias:ALIAS2\n")

		if !strings.Contains(output, expectedAliasLine) {
			t.Errorf("PrintAlias() output missing expected line: %v", expectedAliasLine)
		}
		if !strings.Contains(output, expectedRemoveAliasLine) {
			t.Errorf("PrintAlias() output missing expected line: %v", expectedRemoveAliasLine)
		}
	})
}

func TestDefaultShell_UnsetEnv(t *testing.T) {
	injector := di.NewInjector()

	t.Run("UnsetEnvWithVariables", func(t *testing.T) {
		// Given a default shell and variables to unset
		shell := NewDefaultShell(injector)
		vars := []string{"VAR1", "VAR2", "VAR3"}

		// Capture the output of UnsetEnv
		output := captureStdout(t, func() {
			err := shell.UnsetEnv(vars)
			if err != nil {
				t.Fatalf("UnsetEnv returned an error: %v", err)
			}
		})

		// Then the output should contain the Remove-Item commands for each variable
		expectedLines := []string{
			"Remove-Item Env:VAR1 -ErrorAction SilentlyContinue\n",
			"Remove-Item Env:VAR2 -ErrorAction SilentlyContinue\n",
			"Remove-Item Env:VAR3 -ErrorAction SilentlyContinue\n",
		}

		for _, line := range expectedLines {
			if !strings.Contains(output, line) {
				t.Errorf("UnsetEnv() output missing expected line: %q", line)
			}
		}
	})

	t.Run("UnsetEnvWithNoVariables", func(t *testing.T) {
		// Given a default shell and no variables to unset
		shell := NewDefaultShell(injector)
		var vars []string

		// Capture the output of UnsetEnv
		output := captureStdout(t, func() {
			err := shell.UnsetEnv(vars)
			if err != nil {
				t.Fatalf("UnsetEnv returned an error: %v", err)
			}
		})

		// Then the output should be empty
		if output != "" {
			t.Errorf("UnsetEnv() output = %q, want empty string", output)
		}
	})

	t.Run("UnsetEnvWithOsUnsetenvError", func(t *testing.T) {
		// Save the original function to restore it later
		originalOsUnsetenv := osUnsetenv
		defer func() { osUnsetenv = originalOsUnsetenv }()

		// Mock osUnsetenv to return an error
		osUnsetenv = func(key string) error {
			return fmt.Errorf("simulated error unsetting %s", key)
		}

		// Given a default shell and a variable to unset
		shell := NewDefaultShell(injector)
		vars := []string{"ERROR_VAR"}

		// Capture the output of UnsetEnv to ensure it still prints the command
		output := captureStdout(t, func() {
			// When calling UnsetEnv with the mocked osUnsetenv
			err := shell.UnsetEnv(vars)

			// Then an error should be returned
			if err == nil {
				t.Fatal("UnsetEnv did not return the expected error")
			}

			// And the error message should include the variable name
			expectedErrorMsg := "failed to unset environment variable ERROR_VAR: simulated error unsetting ERROR_VAR"
			if err.Error() != expectedErrorMsg {
				t.Errorf("UnsetEnv() error = %v, want %v", err.Error(), expectedErrorMsg)
			}
		})

		// Verify the command was still printed
		expectedCommand := "Remove-Item Env:ERROR_VAR -ErrorAction SilentlyContinue\n"
		if !strings.Contains(output, expectedCommand) {
			t.Errorf("UnsetEnv() output missing expected command: %q", expectedCommand)
		}
	})
}

func TestDefaultShell_UnsetAlias(t *testing.T) {
	injector := di.NewInjector()

	t.Run("UnsetAliasWithAliases", func(t *testing.T) {
		// Given a default shell and aliases to unset
		shell := NewDefaultShell(injector)
		aliases := []string{"ALIAS1", "ALIAS2", "ALIAS3"}

		// Capture the output of UnsetAlias
		output := captureStdout(t, func() {
			err := shell.UnsetAlias(aliases)
			if err != nil {
				t.Fatalf("UnsetAlias returned an error: %v", err)
			}
		})

		// Then the output should contain Test-Path and Remove-Item commands for each alias
		expectedLines := []string{
			"if (Test-Path Alias:ALIAS1) { Remove-Item Alias:ALIAS1 }\n",
			"if (Test-Path Alias:ALIAS2) { Remove-Item Alias:ALIAS2 }\n",
			"if (Test-Path Alias:ALIAS3) { Remove-Item Alias:ALIAS3 }\n",
		}

		for _, line := range expectedLines {
			if !strings.Contains(output, line) {
				t.Errorf("UnsetAlias() output missing expected line: %q", line)
			}
		}
	})

	t.Run("UnsetAliasWithNoAliases", func(t *testing.T) {
		// Given a default shell and no aliases to unset
		shell := NewDefaultShell(injector)
		var aliases []string

		// Capture the output of UnsetAlias
		output := captureStdout(t, func() {
			err := shell.UnsetAlias(aliases)
			if err != nil {
				t.Fatalf("UnsetAlias returned an error: %v", err)
			}
		})

		// Then the output should be empty
		if output != "" {
			t.Errorf("UnsetAlias() output = %q, want empty string", output)
		}
	})
}
