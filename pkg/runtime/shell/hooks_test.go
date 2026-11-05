package shell_test

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"text/template"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestShell_InstallHook(t *testing.T) {
	setup := func(t *testing.T) (*shell.DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		subject := shell.NewDefaultShell()
		subject.SetShims(mocks.Shims)
		return subject, mocks
	}

	t.Run("UnsupportedShell", func(t *testing.T) {
		// Given a shell with an unsupported shell type
		subject, _ := setup(t)

		// When installing a hook for an unsupported shell
		err := subject.InstallHook("unsupported")

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
		subject, mocks := setup(t)
		mocks.Shims.Executable = func() (string, error) {
			return "", fmt.Errorf("executable error")
		}

		// When installing a hook
		err := subject.InstallHook("zsh")

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
		subject, mocks := setup(t)
		mocks.Shims.Executable = func() (string, error) {
			return "/usr/bin/windsor", nil
		}
		mocks.Shims.NewTemplate = func(name string) *template.Template {
			return nil
		}

		// When installing a hook
		err := subject.InstallHook("zsh")

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
		subject, mocks := setup(t)
		mocks.Shims.Executable = func() (string, error) {
			return "/usr/bin/windsor", nil
		}
		mocks.Shims.TemplateParse = func(tmpl *template.Template, text string) (*template.Template, error) {
			return nil, fmt.Errorf("parse error")
		}

		// When installing a hook
		err := subject.InstallHook("zsh")

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
		subject, mocks := setup(t)
		mocks.Shims.Executable = func() (string, error) {
			return "/usr/bin/windsor", nil
		}
		mocks.Shims.TemplateExecute = func(tmpl *template.Template, wr io.Writer, data any) error {
			return fmt.Errorf("execute error")
		}

		// When installing a hook
		err := subject.InstallHook("zsh")

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
		subject, mocks := setup(t)
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
		err := subject.InstallHook("zsh")

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

	t.Run("TcshRequestsCshEnvOutput", func(t *testing.T) {
		// Given a shell with the real template operations
		subject, mocks := setup(t)
		real := shell.NewShims()
		mocks.Shims.NewTemplate = real.NewTemplate
		mocks.Shims.TemplateParse = real.TemplateParse
		mocks.Shims.TemplateExecute = real.TemplateExecute
		mocks.Shims.Executable = func() (string, error) {
			return "/usr/local/bin/windsor", nil
		}
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// When installing a hook for tcsh
		err := subject.InstallHook("tcsh")
		w.Close()
		os.Stdout = oldStdout
		output, _ := io.ReadAll(r)

		// Then the hook evaluates tcsh output through backtick substitution
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		want := "alias _windsor_hook 'eval \"`'\"'\"'/usr/local/bin/windsor'\"'\"' env --decrypt --hook --shell tcsh`\"';"
		if !strings.Contains(string(output), want) {
			t.Errorf("Expected output to contain %q, got %q", want, output)
		}
		if strings.Contains(string(output), "$(") {
			t.Errorf("Expected no POSIX command substitution, got %q", output)
		}
	})

	t.Run("ElvishIsUnsupported", func(t *testing.T) {
		// Given a shell
		subject, _ := setup(t)

		// When installing a hook for elvish
		err := subject.InstallHook("elvish")

		// Then it fails as an unsupported shell
		if err == nil || !strings.Contains(err.Error(), `Windsor has no hook for the shell "elvish".`) {
			t.Errorf("Expected unsupported shell error, got: %v", err)
		}
	})

	t.Run("PowerShellSuccess", func(t *testing.T) {
		// Given a shell with working template operations
		subject, mocks := setup(t)
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
		err := subject.InstallHook("powershell")

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
