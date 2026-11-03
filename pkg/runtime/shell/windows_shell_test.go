//go:build windows
// +build windows

package shell_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Public Methods
// =============================================================================

// TestDefaultShell_ExecSudo tests the ExecSudo method on Windows: admin check and command execution.
func TestDefaultShell_ExecSudo(t *testing.T) {
	setup := func(t *testing.T) (*shell.DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		subject := shell.NewDefaultShell()
		subject.SetShims(mocks.Shims)
		return subject, mocks
	}

	t.Run("ErrorWhenNotAdmin", func(t *testing.T) {
		subject, _ := setup(t)

		_, err := subject.ExecSudo("msg", "cmd", "arg")
		if err == nil {
			t.Fatal("Expected error when not admin")
		}
		if !strings.Contains(err.Error(), "administrator privileges") {
			t.Errorf("Expected administrator privileges message, got: %v", err)
		}
	})

	t.Run("ErrorWhenAdminCheckFails", func(t *testing.T) {
		subject, mocks := setup(t)
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return nil
		}

		_, err := subject.ExecSudo("msg", "cmd", "arg")
		if err == nil {
			t.Fatal("Expected error when admin check fails")
		}
		if !strings.Contains(err.Error(), "could not check administrator privileges") {
			t.Errorf("Expected admin check error, got: %v", err)
		}
	})

	t.Run("SuccessWhenAdmin", func(t *testing.T) {
		subject, mocks := setup(t)
		mocks.Shims.CmdRun = func(cmd *exec.Cmd) error {
			if cmd.Stdout != nil {
				_, _ = cmd.Stdout.Write([]byte("true"))
			}
			return nil
		}
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			if cmd.Stdout != nil {
				_, _ = cmd.Stdout.Write([]byte("cmd-output"))
			}
			return nil
		}

		out, err := subject.ExecSudo("msg", "echo", "cmd-output")
		if err != nil {
			t.Fatalf("ExecSudo: %v", err)
		}
		if out != "cmd-output" {
			t.Errorf("Expected output 'cmd-output', got %q", out)
		}
	})

	t.Run("SuccessWhenCommandIsTrue", func(t *testing.T) {
		subject, mocks := setup(t)
		mocks.Shims.CmdRun = func(cmd *exec.Cmd) error {
			if cmd.Stdout != nil {
				_, _ = cmd.Stdout.Write([]byte("true"))
			}
			return nil
		}

		out, err := subject.ExecSudo("msg", "true")
		if err != nil {
			t.Fatalf("ExecSudo: %v", err)
		}
		if out != "" {
			t.Errorf("Expected empty output for 'true', got %q", out)
		}
	})

	t.Run("SudoCommandUnwraps", func(t *testing.T) {
		subject, mocks := setup(t)
		mocks.Shims.CmdRun = func(cmd *exec.Cmd) error {
			if cmd.Stdout != nil {
				_, _ = cmd.Stdout.Write([]byte("true"))
			}
			return nil
		}
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			if cmd.Stdout != nil {
				_, _ = cmd.Stdout.Write([]byte("unwrapped"))
			}
			return nil
		}

		out, err := subject.ExecSudo("msg", "sudo", "realcmd", "arg")
		if err != nil {
			t.Fatalf("ExecSudo: %v", err)
		}
		if out != "unwrapped" {
			t.Errorf("Expected output 'unwrapped', got %q", out)
		}
	})

	t.Run("VerbosePrintsMessage", func(t *testing.T) {
		subject, mocks := setup(t)
		subject.SetVerbosity(true)
		mocks.Shims.CmdRun = func(cmd *exec.Cmd) error {
			if cmd.Stdout != nil {
				_, _ = cmd.Stdout.Write([]byte("true"))
			}
			return nil
		}
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			if cmd.Stdout != nil {
				_, _ = cmd.Stdout.Write([]byte("true"))
			}
			return nil
		}

		out, err := subject.ExecSudo("verbose-msg", "true")
		if err != nil {
			t.Fatalf("ExecSudo: %v", err)
		}
		if out != "" {
			t.Errorf("Expected empty output, got %q", out)
		}
	})
}

// TestDefaultShell_GetProjectRoot tests the GetProjectRoot method on Windows systems
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

// TestDefaultShell_PrintAlias tests the PrintAlias method on Windows systems

// TestDefaultShell_UnsetEnvs tests the UnsetEnvs method on Windows systems
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
		expectedOutput := "Remove-Item Env:VAR1\nRemove-Item Env:VAR2\nRemove-Item Env:VAR3\n"

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

// TestDefaultShell_UnsetAlias tests the UnsetAlias method on Windows systems
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
		expectedOutput := "Remove-Item Alias:ALIAS1\nRemove-Item Alias:ALIAS2\nRemove-Item Alias:ALIAS3\n"

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

// TestDefaultShell_RenderEnvVars tests the RenderEnvVars method on Windows systems
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
		if !strings.Contains(result, "VAR2=value2") {
			t.Errorf("Expected plain format, got: %s", result)
		}
	})
}

// TestDefaultShell_PrintEnvVars tests the PrintEnvVars method on Windows systems
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
		if !strings.Contains(output, "VAR2=value2") {
			t.Errorf("Expected plain format, got: %s", output)
		}
	})
}
