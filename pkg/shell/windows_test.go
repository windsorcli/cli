//go:build windows
// +build windows

package shell

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
	"golang.org/x/sys/windows"
)

// The WindowsShellTest is a test suite for Windows-specific shell operations.
// It provides comprehensive test coverage for PowerShell environment management,
// project root detection, and alias handling on Windows systems.
// The WindowsShellTest ensures reliable shell operations on Windows platforms.

// =============================================================================
// Test Setup
// =============================================================================

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

// =============================================================================
// Test Public Methods
// =============================================================================

// TestDefaultShell_PrintEnvVars tests the PrintEnvVars method on Windows systems
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

	// Capture the output
	var output bytes.Buffer
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Run PrintEnvVars in a goroutine and capture its output
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
}

// TestDefaultShell_GetProjectRoot tests the GetProjectRoot method on Windows systems
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

// TestDefaultShell_PrintAlias tests the PrintAlias method on Windows systems
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
		injector := di.NewInjector()
		shell := NewDefaultShell(injector)
		aliasVarsWithEmpty := map[string]string{
			"ALIAS1": "command1",
			"ALIAS2": "",
		}

		// Capture the output of PrintAlias
		output := captureStdout(t, func() {
			shell.PrintAlias(aliasVarsWithEmpty)
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

// TestDefaultShell_UnsetEnvs tests the UnsetEnvs method on Windows systems
func TestDefaultShell_UnsetEnvs(t *testing.T) {
	injector := di.NewInjector()

	// Given a set of environment variables to unset
	shell := NewDefaultShell(injector)
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

	// Test with empty list
	emptyOutput := captureStdout(t, func() {
		shell.UnsetEnvs([]string{})
	})

	// Then the output should be empty
	if emptyOutput != "" {
		t.Errorf("UnsetEnvs() with empty list should produce no output, got %v", emptyOutput)
	}
}

// TestDefaultShell_UnsetAlias tests the UnsetAlias method on Windows systems
func TestDefaultShell_UnsetAlias(t *testing.T) {
	injector := di.NewInjector()

	// Given a set of aliases to unset
	shell := NewDefaultShell(injector)
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

	// Test with empty list
	emptyOutput := captureStdout(t, func() {
		shell.UnsetAlias([]string{})
	})

	// Then the output should be empty
	if emptyOutput != "" {
		t.Errorf("UnsetAlias() with empty list should produce no output, got %v", emptyOutput)
	}
}
