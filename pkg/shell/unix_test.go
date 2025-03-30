//go:build darwin || linux
// +build darwin linux

package shell

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/di"
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

			// Mock osStat to simulate the presence of the specified file
			originalOsStat := osStat
			defer func() { osStat = originalOsStat }()
			osStat = func(name string) (os.FileInfo, error) {
				if strings.HasSuffix(name, tc.fileName) {
					return &mockFileInfo{name: tc.fileName}, nil
				}
				return nil, fmt.Errorf("file not found")
			}

			// Mock getwd to simulate a specific working directory
			originalGetwd := getwd
			defer func() { getwd = originalGetwd }()
			getwd = func() (string, error) {
				return "/mock/project/root", nil
			}

			shell := NewDefaultShell(injector)

			// Then GetProjectRoot should find the project root using the specified file
			projectRoot, err := shell.GetProjectRoot()
			if err != nil {
				t.Fatalf("GetProjectRoot returned an error: %v", err)
			}

			// Validate that the project root is the mocked directory
			expectedRootDir := "/mock/project/root"

			// Normalize paths for comparison
			expectedRootDir = normalizePath(expectedRootDir)
			projectRoot = normalizePath(projectRoot)

			if projectRoot != expectedRootDir {
				t.Errorf("Expected project root to be %s, got %s", expectedRootDir, projectRoot)
			}
		})
	}
}

// mockFileInfo is a mock implementation of os.FileInfo
type mockFileInfo struct {
	name string
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return 0 }
func (m *mockFileInfo) Mode() os.FileMode  { return 0 }
func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfo) IsDir() bool        { return false }
func (m *mockFileInfo) Sys() interface{}   { return nil }

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

		// Mock osExecCommand to simulate checking for an existing alias
		originalExecCommand := execCommand
		defer func() {
			execCommand = originalExecCommand
		}()
		execCommand = func(name string, arg ...string) *exec.Cmd {
			cmd := exec.Command("echo", append([]string{name}, arg...)...)
			if name == "alias" && len(arg) > 0 && arg[0] == "ALIAS2" {
				cmd.Stdout = bytes.NewBufferString("alias ALIAS2='somecommand'\n")
			} else {
				cmd.Stdout = &bytes.Buffer{}
			}
			return cmd
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
