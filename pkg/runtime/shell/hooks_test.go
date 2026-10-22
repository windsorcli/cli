package shell

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"text/template"

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestShell_InstallHook(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("UnsupportedShell", func(t *testing.T) {
		// Given a shell with an unsupported shell type
		shell, _ := setup(t)

		// When installing a hook for an unsupported shell
		err := shell.InstallHook("unsupported")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), `Windsor has no hook for the shell "unsupported".`) {
			t.Errorf("Expected error about unsupported shell, got: %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "bash, zsh") {
			t.Errorf("Expected a remediation that lists the supported shells, got: %v", we)
		}
	})

	t.Run("ExecutableError", func(t *testing.T) {
		// Given a shell with failing Executable
		shell, mocks := setup(t)
		mocks.Shims.Executable = func() (string, error) {
			return "", fmt.Errorf("executable error")
		}

		// When installing a hook
		err := shell.InstallHook("zsh")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "executable error") {
			t.Errorf("Expected error about executable, got: %v", err)
		}
	})

	t.Run("TemplateCreateError", func(t *testing.T) {
		// Given a shell with failing template creation
		shell, mocks := setup(t)
		mocks.Shims.Executable = func() (string, error) {
			return "/usr/bin/windsor", nil
		}
		mocks.Shims.NewTemplate = func(name string) *template.Template {
			return nil
		}

		// When installing a hook
		err := shell.InstallHook("zsh")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create new template") {
			t.Errorf("Expected error about template creation, got: %v", err)
		}
	})

	t.Run("TemplateParseError", func(t *testing.T) {
		// Given a shell with failing template parse
		shell, mocks := setup(t)
		mocks.Shims.Executable = func() (string, error) {
			return "/usr/bin/windsor", nil
		}
		mocks.Shims.TemplateParse = func(tmpl *template.Template, text string) (*template.Template, error) {
			return nil, fmt.Errorf("parse error")
		}

		// When installing a hook
		err := shell.InstallHook("zsh")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse hook template") {
			t.Errorf("Expected error about template parsing, got: %v", err)
		}
	})

	t.Run("TemplateExecuteError", func(t *testing.T) {
		// Given a shell with failing template execution
		shell, mocks := setup(t)
		mocks.Shims.Executable = func() (string, error) {
			return "/usr/bin/windsor", nil
		}
		mocks.Shims.TemplateExecute = func(tmpl *template.Template, wr io.Writer, data any) error {
			return fmt.Errorf("execute error")
		}

		// When installing a hook
		err := shell.InstallHook("zsh")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to execute hook template") {
			t.Errorf("Expected error about template execution, got: %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		// Given a shell with working template operations
		shell, mocks := setup(t)
		mocks.Shims.Executable = func() (string, error) {
			return "/usr/bin/windsor", nil
		}
		mocks.Shims.TemplateExecute = func(tmpl *template.Template, wr io.Writer, data any) error {
			_, err := wr.Write([]byte(`
				_windsor_hook() {
					trap -- '' SIGINT;
					eval "$(/usr/bin/windsor env --decrypt)";
					trap - SIGINT;
				};
			`))
			return err
		}

		// Capture stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// When installing a hook
		err := shell.InstallHook("zsh")

		// Close writer and restore stdout
		w.Close()
		os.Stdout = oldStdout

		// Read captured output
		output, _ := io.ReadAll(r)

		// Then it should succeed and output the hook script
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !strings.Contains(string(output), "_windsor_hook") {
			t.Error("Expected output to contain _windsor_hook")
		}
	})

	t.Run("PowerShellSuccess", func(t *testing.T) {
		// Given a shell with working template operations
		shell, mocks := setup(t)
		mocks.Shims.Executable = func() (string, error) {
			return "/usr/bin/windsor", nil
		}
		mocks.Shims.TemplateExecute = func(tmpl *template.Template, wr io.Writer, data any) error {
			_, err := wr.Write([]byte(`
				function prompt {
					$windsorEnvScript = & "/usr/bin/windsor" env --decrypt | Out-String
					if ($windsorEnvScript) {
						Invoke-Expression $windsorEnvScript
					}
				}
			`))
			return err
		}

		// Capture stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// When installing a hook for PowerShell
		err := shell.InstallHook("powershell")

		// Close writer and restore stdout
		w.Close()
		os.Stdout = oldStdout

		// Read captured output
		output, _ := io.ReadAll(r)

		// Then it should succeed and output the hook script without empty lines
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if strings.Contains(string(output), "\n\n") {
			t.Error("Expected no empty lines in PowerShell output")
		}
		if !strings.Contains(string(output), "function prompt") {
			t.Error("Expected output to contain function prompt")
		}
	})
}
