//go:build darwin || linux
// +build darwin linux

package shell_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Public Methods
// =============================================================================

// TestDefaultShell_GetProjectRoot tests the GetProjectRoot method on Unix systems
func TestDefaultShell_GetProjectRoot(t *testing.T) {
	setup := func(t *testing.T) (*shell.DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		subject := shell.NewDefaultShell()
		subject.SetShims(mocks.Shims)
		return subject, mocks
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
			subject, mocks := setup(t)

			// Mock the file system behavior
			rootDir := mocks.TmpDir
			subDir := filepath.Join(rootDir, "subdir")

			// Override Getwd to return the subdirectory
			subject.Shims().Getwd = func() (string, error) {
				return subDir, nil
			}

			// Override Stat to return nil for the windsor file
			subject.Shims().Stat = func(name string) (os.FileInfo, error) {
				if name == filepath.Join(rootDir, tc.fileName) {
					return nil, nil
				}
				return nil, os.ErrNotExist
			}

			// When finding the project root using the specified file
			projectRoot, err := subject.GetProjectRoot()
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
	setup := func(t *testing.T) (*shell.DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		subject := shell.NewDefaultShell()
		subject.SetShims(mocks.Shims)
		return subject, mocks
	}

	t.Run("UnsetEnvs", func(t *testing.T) {
		// Given a set of environment variables to unset
		subject, _ := setup(t)
		envVars := []string{"VAR1", "VAR2", "VAR3"}
		expectedOutput := "unset VAR1 VAR2 VAR3\n"

		// When capturing the output of UnsetEnvs
		output := captureStdout(t, func() {
			subject.UnsetEnvs(envVars)
		})

		// Then the output should match the expected output
		if output != expectedOutput {
			t.Errorf("UnsetEnvs() output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("UnsetEnvsWithEmptyList", func(t *testing.T) {
		// Given an empty list of environment variables
		subject, _ := setup(t)

		// When capturing the output of UnsetEnvs
		output := captureStdout(t, func() {
			subject.UnsetEnvs([]string{})
		})

		// Then the output should be empty
		if output != "" {
			t.Errorf("UnsetEnvs() with empty list should produce no output, got %v", output)
		}
	})
}

// TestDefaultShell_UnsetAlias tests the UnsetAlias method on Unix systems
func TestDefaultShell_UnsetAlias(t *testing.T) {
	setup := func(t *testing.T) (*shell.DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		subject := shell.NewDefaultShell()
		subject.SetShims(mocks.Shims)
		return subject, mocks
	}

	t.Run("UnsetAlias", func(t *testing.T) {
		// Given a set of aliases to unset
		subject, _ := setup(t)
		aliases := []string{"ALIAS1", "ALIAS2", "ALIAS3"}
		expectedOutput := "unalias ALIAS1\nunalias ALIAS2\nunalias ALIAS3\n"

		// When capturing the output of UnsetAlias
		output := captureStdout(t, func() {
			subject.UnsetAlias(aliases)
		})

		// Then the output should match the expected output
		if output != expectedOutput {
			t.Errorf("UnsetAlias() output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("UnsetAliasWithEmptyList", func(t *testing.T) {
		// Given an empty list of aliases
		subject, _ := setup(t)

		// When capturing the output of UnsetAlias
		output := captureStdout(t, func() {
			subject.UnsetAlias([]string{})
		})

		// Then the output should be empty
		if output != "" {
			t.Errorf("UnsetAlias() with empty list should produce no output, got %v", output)
		}
	})
}

// TestDefaultShell_RenderAliases tests the RenderAliases method on Unix systems
func TestDefaultShell_RenderAliases(t *testing.T) {
	setup := func(t *testing.T) (*shell.DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		subject := shell.NewDefaultShell()
		subject.SetShims(mocks.Shims)
		return subject, mocks
	}

	t.Run("RendersAliasesWithValues", func(t *testing.T) {
		// Given a shell with aliases
		subject, _ := setup(t)
		aliases := map[string]string{
			"ll":   "ls -la",
			"grep": "grep --color=auto",
			"cd":   "",
		}

		// When rendering aliases
		result := subject.RenderAliases(aliases)

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
		subject, _ := setup(t)
		aliases := map[string]string{}

		// When rendering aliases
		result := subject.RenderAliases(aliases)

		// Then the output should be empty
		if result != "" {
			t.Errorf("Expected empty output for empty aliases, got: %s", result)
		}
	})

	t.Run("RendersAliasesWithEmptyValues", func(t *testing.T) {
		// Given a shell with aliases that have empty values
		subject, _ := setup(t)
		aliases := map[string]string{
			"alias1": "",
			"alias2": "",
		}

		// When rendering aliases
		result := subject.RenderAliases(aliases)

		// Then the output should contain unalias commands
		if !strings.Contains(result, "unalias") {
			t.Errorf("Expected unalias commands for empty aliases, got: %s", result)
		}
	})
}

// TestDefaultShell_RenderEnvVars tests the RenderEnvVars method on Unix systems
func TestDefaultShell_RenderEnvVars(t *testing.T) {
	setup := func(t *testing.T) (*shell.DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		subject := shell.NewDefaultShell()
		subject.SetShims(mocks.Shims)
		return subject, mocks
	}

	t.Run("RendersEnvVarsWithExport", func(t *testing.T) {
		// Given a shell with environment variables
		subject, _ := setup(t)
		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}

		// When rendering environment variables with export=true
		result := subject.RenderEnvVars(envVars, true)

		// Then the output should contain export commands
		if !strings.Contains(result, "export") {
			t.Errorf("Expected export commands, got: %s", result)
		}
	})

	t.Run("RendersEnvVarsWithoutExport", func(t *testing.T) {
		// Given a shell with environment variables
		subject, _ := setup(t)
		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}

		// When rendering environment variables with export=false
		result := subject.RenderEnvVars(envVars, false)

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
	setup := func(t *testing.T) (*shell.DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		subject := shell.NewDefaultShell()
		subject.SetShims(mocks.Shims)
		return subject, mocks
	}

	t.Run("PrintsEnvVarsWithExport", func(t *testing.T) {
		// Given a shell with environment variables
		subject, _ := setup(t)
		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}

		// When printing environment variables with export=true
		output := captureStdout(t, func() {
			subject.PrintEnvVars(envVars, true)
		})

		// Then the output should contain export commands
		if !strings.Contains(output, "export") {
			t.Errorf("Expected export commands, got: %s", output)
		}
	})

	t.Run("PrintsEnvVarsWithoutExport", func(t *testing.T) {
		// Given a shell with environment variables
		subject, _ := setup(t)
		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}

		// When printing environment variables with export=false
		output := captureStdout(t, func() {
			subject.PrintEnvVars(envVars, false)
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
