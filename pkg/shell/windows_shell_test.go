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

func TestDefaultShell_GetProjectRoot(t *testing.T) {
	injector := di.NewInjector()

	testCases := []struct {
		name         string
		fileName     string
		expectedRoot string
	}{
		{"WindsorYaml", "windsor.yaml", "/mock/project/root"},
		{"WindsorYml", "windsor.yml", "/mock/project/root"},
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

			// Normalize paths for comparison
			expectedRootDir := normalizeWindowsPath(tc.expectedRoot)
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

// UnsetEnvVars takes an array of variables and outputs "Remove-Item Env:..." on one line.
func (s *DefaultShell) UnsetEnvVars(vars []string) {
	if len(vars) > 0 {
		fmt.Printf("Remove-Item Env:%s\n", strings.Join(vars, " Env:"))
	}
}

// UnsetAlias unsets the provided aliases
func (s *DefaultShell) UnsetAlias(aliases []string) {
	if len(aliases) > 0 {
		for _, alias := range aliases {
			fmt.Printf("Remove-Item Alias:%s\n", alias)
		}
	}
}

func TestDefaultShell_UnsetEnvVars(t *testing.T) {
	injector := di.NewInjector()

	t.Run("Success", func(t *testing.T) {
		// Given a default shell and a set of environment variables to unset
		shell := NewDefaultShell(injector)
		envVars := []string{"VAR1", "VAR2", "VAR3"}

		// Capture the output of UnsetEnvVars
		output := captureStdout(t, func() {
			shell.UnsetEnvVars(envVars)
		})

		// Then the output should contain the expected remove item command
		expectedOutput := "Remove-Item Env:VAR1 Env:VAR2 Env:VAR3\n"
		if output != expectedOutput {
			t.Errorf("UnsetEnvVars() output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("UnsetEnvVarsEmpty", func(t *testing.T) {
		// Given a default shell and an empty set of environment variables to unset
		shell := NewDefaultShell(injector)
		envVars := []string{}

		// Capture the output of UnsetEnvVars
		output := captureStdout(t, func() {
			shell.UnsetEnvVars(envVars)
		})

		// Then the output should be empty
		expectedOutput := ""
		if output != expectedOutput {
			t.Errorf("UnsetEnvVars() output = %v, want %v", output, expectedOutput)
		}
	})
}
