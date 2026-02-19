package shell

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"text/template"
	"time"
)

// The ShellTest is a test suite for the Shell interface and its implementations.
// It provides comprehensive test coverage for shell operations, command execution,
// project root detection, and environment management.
// The ShellTest acts as a validation framework for shell functionality,
// ensuring reliable command execution, proper error handling, and environment isolation.

// =============================================================================
// Test Setup
// =============================================================================

type ShellTestMocks struct {
	Shims  *Shims
	TmpDir string
}

// setupDefaultShims creates a new Shims instance with default implementations
func setupDefaultShims(tmpDir string) *Shims {
	shims := NewShims()

	// Mock command execution with proper cleanup
	shims.Command = func(name string, args ...string) *exec.Cmd {
		cmd := exec.Command("echo", "test")
		cmd.Stdout = new(bytes.Buffer)
		cmd.Stderr = new(bytes.Buffer)
		return cmd
	}

	// Mock command execution methods with proper cleanup
	shims.CmdStart = func(cmd *exec.Cmd) error {
		if cmd.Stdout != nil {
			if _, err := cmd.Stdout.Write([]byte("test\n")); err != nil {
				return fmt.Errorf("failed to write to stdout: %v", err)
			}
		}
		return nil
	}

	shims.CmdWait = func(cmd *exec.Cmd) error {
		return nil
	}

	shims.CmdRun = func(cmd *exec.Cmd) error {
		if cmd.Stdout != nil {
			if _, err := cmd.Stdout.Write([]byte("test\n")); err != nil {
				return fmt.Errorf("failed to write to stdout: %v", err)
			}
		}
		return nil
	}

	// Mock pipes with proper cleanup
	shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
		r, w := io.Pipe()
		go func() {
			if _, err := w.Write([]byte("test output\n")); err != nil {
				// Ignore error in goroutine
			}
			w.Close()
		}()
		return r, nil
	}

	shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
		r, w := io.Pipe()
		go func() {
			if _, err := w.Write([]byte("error\n")); err != nil {
				// Ignore error in goroutine
			}
			w.Close()
		}()
		return r, nil
	}

	// Mock file operations
	shims.Getwd = func() (string, error) {
		return tmpDir, nil
	}

	shims.Stat = func(name string) (os.FileInfo, error) {
		if name == "trusted_dirs" {
			return nil, os.ErrNotExist
		}
		return nil, nil
	}

	// Mock file operations with proper cleanup
	shims.OpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		return os.NewFile(0, "test"), nil
	}

	shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
		return nil
	}

	shims.ReadFile = func(name string) ([]byte, error) {
		return []byte("test\n"), nil
	}

	shims.MkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}

	shims.Remove = func(name string) error {
		return nil
	}

	shims.RemoveAll = func(path string) error {
		return nil
	}

	shims.Chdir = func(dir string) error {
		return nil
	}

	shims.Setenv = func(key, value string) error {
		return nil
	}

	shims.Getenv = func(key string) string {
		return ""
	}

	shims.UserHomeDir = func() (string, error) {
		return tmpDir, nil
	}

	// Mock random operations with proper cleanup
	shims.RandRead = func(b []byte) (n int, err error) {
		for i := range b {
			b[i] = byte(i % 62)
		}
		return len(b), nil
	}

	// Mock template operations with proper cleanup
	shims.NewTemplate = func(name string) *template.Template {
		return template.New(name)
	}

	shims.TemplateParse = func(tmpl *template.Template, text string) (*template.Template, error) {
		return tmpl.Parse(text)
	}

	shims.TemplateExecute = func(tmpl *template.Template, wr io.Writer, data any) error {
		return nil
	}

	shims.ExecuteTemplate = func(tmpl *template.Template, data any) error {
		return nil
	}

	// Mock bufio operations with proper cleanup
	shims.ScannerErr = func(scanner *bufio.Scanner) error {
		return nil
	}

	shims.NewWriter = func(w io.Writer) *bufio.Writer {
		return bufio.NewWriter(w)
	}

	// Mock filepath operations
	shims.Glob = func(pattern string) ([]string, error) {
		return []string{filepath.Join(tmpDir, "test")}, nil
	}

	shims.Join = func(elem ...string) string {
		return filepath.Join(elem...)
	}

	shims.ScannerText = func(scanner *bufio.Scanner) string {
		return scanner.Text()
	}

	shims.IsTerminal = func(fd int) bool {
		return true
	}

	return shims
}

// setupShellMocks creates a new set of mocks for testing
func setupShellMocks(t *testing.T) *ShellTestMocks {
	t.Helper()

	// Create temp dir
	tmpDir := t.TempDir()

	// Create initial mocks with defaults
	mocks := &ShellTestMocks{
		Shims:  setupDefaultShims(tmpDir),
		TmpDir: tmpDir,
	}

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestShell_NewDefaultShell(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a shell
		shell := NewDefaultShell()

		// Then it should be created
		if shell == nil {
			t.Error("Expected shell to be created")
		}
	})
}

func TestShell_SetVerbosity(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a shell
		shell := NewDefaultShell()

		// When setting verbosity to true
		shell.SetVerbosity(true)

		// Then verbosity should be true
		if !shell.verbose {
			t.Fatalf("Expected verbosity to be true, got false")
		}
	})

	t.Run("DisableVerbosity", func(t *testing.T) {
		// Given a shell
		shell := NewDefaultShell()

		// When setting verbosity to false
		shell.SetVerbosity(false)

		// Then verbosity should be false
		if shell.verbose {
			t.Fatalf("Expected verbosity to be false, got true")
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestShell_GetProjectRoot(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a shell with project root set
		shell, _ := setup(t)
		shell.projectRoot = "/test/root"

		// When getting the project root
		root, err := shell.GetProjectRoot()

		// Then it should return the expected root
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if root != "/test/root" {
			t.Errorf("Expected /test/root, got %s", root)
		}
	})

	t.Run("FindsProjectRoot", func(t *testing.T) {
		// Given a shell in a directory with windsor.yaml
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/current", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "/test/current/windsor.yaml" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// When getting the project root
		root, err := shell.GetProjectRoot()

		// Then it should find the root directory
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if root != "/test/current" {
			t.Errorf("Expected /test/current, got %s", root)
		}
	})

	t.Run("FindsProjectRootWithYml", func(t *testing.T) {
		// Given a shell in a directory with windsor.yml
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/current", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "/test/current/windsor.yml" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// When getting the project root
		root, err := shell.GetProjectRoot()

		// Then it should find the root directory
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if root != "/test/current" {
			t.Errorf("Expected /test/current, got %s", root)
		}
	})

	t.Run("ErrorOnGetwdFailure", func(t *testing.T) {
		// Given a shell with failing Getwd
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("getwd failed")
		}

		// When getting project root
		_, err := shell.GetProjectRoot()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "getwd failed") {
			t.Errorf("Expected error to contain 'getwd failed', got %v", err)
		}
	})

	t.Run("MaxDepthExceeded", func(t *testing.T) {
		// Given a shell in a deep directory structure without windsor.yaml/yml
		shell, mocks := setup(t)
		originalDir := "/test/very/deep/directory/structure/without/config/file"
		mocks.Shims.Getwd = func() (string, error) {
			return originalDir, nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When getting the project root
		root, err := shell.GetProjectRoot()

		// Then it should return the original directory without error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if root != originalDir {
			t.Errorf("Expected %s, got %s", originalDir, root)
		}
	})
}

func TestShell_Exec(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a shell with mocked command execution
		shell, mocks := setup(t)
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			cmd := exec.Command("echo", "test")
			return cmd
		}
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			if cmd.Stdout != nil {
				if _, err := cmd.Stdout.Write([]byte("test\n")); err != nil {
					return fmt.Errorf("failed to write to stdout: %v", err)
				}
			}
			return nil
		}
		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			return nil
		}

		// Capture stdout during test
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// When executing a command
		out, err := shell.Exec("test", "arg")

		// Restore stdout
		w.Close()
		os.Stdout = oldStdout
		io.ReadAll(r)

		// Then it should succeed and return output
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if out != "test\n" {
			t.Errorf("Expected output 'test\n', got '%s'", out)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a shell with failing command
		shell, mocks := setup(t)
		mocks.Shims.Command = func(name string, arg ...string) *exec.Cmd {
			cmd := exec.Command("echo", "test")
			return cmd
		}
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			return nil
		}
		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			return fmt.Errorf("command failed")
		}

		// When executing a command
		out, err := shell.Exec("test", "arg")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "command failed") {
			t.Errorf("Expected error to contain 'command failed', got %v", err)
		}
		if out != "" {
			t.Errorf("Expected empty output, got '%s'", out)
		}
	})

	t.Run("ErrorOnStart", func(t *testing.T) {
		// Given a shell with failing command start
		shell, mocks := setup(t)
		mocks.Shims.Command = func(name string, arg ...string) *exec.Cmd {
			cmd := exec.Command("echo", "test")
			return cmd
		}
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			return fmt.Errorf("failed to start command")
		}

		// When executing a command
		out, err := shell.Exec("test", "arg")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to start command") {
			t.Errorf("Expected error to contain 'failed to start command', got %v", err)
		}
		if out != "" {
			t.Errorf("Expected empty output, got '%s'", out)
		}
	})
}

func TestShell_ExecSudo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ExecSudo Unix implementation tested here; Windows path covered in windows_shell_test.go")
	}
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		shell, mocks := setup(t)

		// Mock command to return test output
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			cmd := exec.Command("echo", "test")
			return cmd
		}

		// Mock successful command execution
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			if cmd.Stdout != nil {
				if _, err := cmd.Stdout.Write([]byte("test output")); err != nil {
					return fmt.Errorf("failed to write to stdout: %v", err)
				}
			}
			return nil
		}
		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			return nil
		}
		mocks.Shims.OpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
			return os.NewFile(0, "test"), nil
		}

		output, err := shell.ExecSudo("Running test", "test", "arg")
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if output != "test output" {
			t.Errorf("Expected output 'test output', got: %q", output)
		}
	})

	t.Run("SuccessWithVerbose", func(t *testing.T) {
		// Given a shell with verbose mode enabled
		shell, mocks := setup(t)
		shell.SetVerbosity(true)

		// Mock command execution
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			cmd := &exec.Cmd{
				Path:   name,
				Args:   append([]string{name}, args...),
				Stdout: new(bytes.Buffer),
				Stderr: new(bytes.Buffer),
			}
			return cmd
		}

		// Mock successful command execution
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			if cmd.Stdout != nil {
				if _, err := cmd.Stdout.Write([]byte("test\n")); err != nil {
					return fmt.Errorf("failed to write to stdout: %v", err)
				}
			}
			return nil
		}
		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			return nil
		}

		// Mock TTY handling
		mocks.Shims.OpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
			if name == "/dev/tty" {
				return os.NewFile(0, "/dev/tty"), nil
			}
			return nil, fmt.Errorf("unexpected file: %s", name)
		}

		// When executing a sudo command
		output, err := shell.ExecSudo("test", "command")

		// Then it should succeed and return the expected output
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if output != "test\n" {
			t.Errorf("Expected 'test\\n' output, got: %q", output)
		}
	})

	t.Run("SuccessWithNoTTY", func(t *testing.T) {
		shell, mocks := setup(t)

		mocks.Shims.IsTerminal = func(fd int) bool {
			return false
		}
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			cmd := &exec.Cmd{
				Path:   name,
				Args:   append([]string{name}, args...),
				Stdout: new(bytes.Buffer),
				Stderr: new(bytes.Buffer),
			}
			return cmd
		}
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			if cmd.Stdout != nil {
				if _, err := cmd.Stdout.Write([]byte("test output\n")); err != nil {
					return fmt.Errorf("failed to write to stdout: %v", err)
				}
			}
			return nil
		}
		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			return nil
		}

		output, err := shell.ExecSudo("Running test", "test", "arg")
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if output != "test output\n" {
			t.Errorf("Expected output 'test output\\n', got: %q", output)
		}
	})

	t.Run("ErrorOnOpenTTY", func(t *testing.T) {
		shell, mocks := setup(t)
		mocks.Shims.OpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
			return nil, fmt.Errorf("failed to open tty")
		}

		output, err := shell.ExecSudo("Running test", "test", "arg")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to open /dev/tty") {
			t.Errorf("Expected error about tty, got: %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got: %s", output)
		}
	})

	t.Run("ErrorOnStart", func(t *testing.T) {
		shell, mocks := setup(t)
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			return fmt.Errorf("failed to start command")
		}

		output, err := shell.ExecSudo("Running test", "test", "arg")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to start command") {
			t.Errorf("Expected error about command start, got: %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got: %s", output)
		}
	})

	t.Run("ErrorOnWait", func(t *testing.T) {
		shell, mocks := setup(t)

		expectedOutput := "test output"
		expectedErr := fmt.Errorf("wait error")

		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			cmd := exec.Command("echo", "test")
			return cmd
		}
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			if cmd.Stdout != nil {
				if w, ok := cmd.Stdout.(*bytes.Buffer); ok {
					w.WriteString(expectedOutput)
				}
			}
			return nil
		}
		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			return expectedErr
		}
		mocks.Shims.OpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
			return os.NewFile(0, "test"), nil
		}

		output, err := shell.ExecSudo("test", "test", "arg")

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), expectedErr.Error()) {
			t.Errorf("Expected error containing %v, got %v", expectedErr, err)
		}
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SudoCommand", func(t *testing.T) {
		// Given a shell with verbose mode disabled
		shell, mocks := setup(t)

		// Mock command execution
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			cmd := &exec.Cmd{
				Path:   name,
				Args:   append([]string{name}, args...),
				Stdout: new(bytes.Buffer),
				Stderr: new(bytes.Buffer),
			}
			return cmd
		}
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			return nil
		}
		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			return nil
		}
		mocks.Shims.OpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
			return os.NewFile(0, "test"), nil
		}

		// When executing a sudo command
		output, err := shell.ExecSudo("test", "sudo", "command")

		// Then it should succeed and return empty output
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got: %q", output)
		}
	})

	t.Run("CommandNil", func(t *testing.T) {
		// Given a shell with Command returning nil
		shell, mocks := setup(t)
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return nil
		}

		// When executing a sudo command
		output, err := shell.ExecSudo("test", "command")

		// Then it should fail with the expected error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create command") {
			t.Errorf("Expected error about failed command creation, got: %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got: %q", output)
		}
	})
}

func TestShell_ExecSilent(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a shell with mocked command execution
		shell, _ := setup(t)

		// When executing a command silently
		out, err := shell.ExecSilent("test", "arg")

		// Then it should succeed and return output
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if out != "test\n" {
			t.Errorf("Expected output 'test\n', got '%s'", out)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a shell with failing command
		shell, mocks := setup(t)
		mocks.Shims.Command = func(name string, arg ...string) *exec.Cmd {
			cmd := exec.Command("echo", "test")
			return cmd
		}
		mocks.Shims.CmdRun = func(cmd *exec.Cmd) error {
			return fmt.Errorf("command failed")
		}

		// When executing a command silently
		out, err := shell.ExecSilent("test", "arg")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "command failed") {
			t.Errorf("Expected error to contain 'command failed', got %v", err)
		}
		if out != "" {
			t.Errorf("Expected empty output, got '%s'", out)
		}
	})

	t.Run("CommandNil", func(t *testing.T) {
		// Given a shell with Command returning nil
		shell, mocks := setup(t)
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return nil
		}

		// When executing a command silently
		output, err := shell.ExecSilent("test", "arg")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create command") {
			t.Errorf("Expected error about command creation, got: %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
	})

	t.Run("StdoutPipeError", func(t *testing.T) {
		// Given a shell with failing StdoutPipe
		shell, mocks := setup(t)
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return exec.Command("echo", "test")
		}
		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			return nil, fmt.Errorf("stdout pipe error")
		}

		// When executing a command with progress
		output, err := shell.ExecProgress("test message", "test", "arg")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "stdout pipe error") {
			t.Errorf("Expected error about stdout pipe, got: %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
	})

	t.Run("StderrPipeError", func(t *testing.T) {
		// Given a shell with failing StderrPipe
		shell, mocks := setup(t)
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return exec.Command("echo", "test")
		}
		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				if _, err := w.Write([]byte("test output\n")); err != nil {
					t.Errorf("Failed to write to stdout pipe: %v", err)
				}
				w.Close()
			}()
			return r, nil
		}
		mocks.Shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			return nil, fmt.Errorf("stderr pipe error")
		}

		// When executing a command with progress
		output, err := shell.ExecProgress("test message", "test", "arg")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "stderr pipe error") {
			t.Errorf("Expected error about stderr pipe, got: %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
	})

	t.Run("CmdStartError", func(t *testing.T) {
		// Given a shell with failing CmdStart
		shell, mocks := setup(t)
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return exec.Command("echo", "test")
		}
		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				if _, err := w.Write([]byte("test output\n")); err != nil {
					t.Errorf("Failed to write to stdout pipe: %v", err)
				}
				w.Close()
			}()
			return r, nil
		}
		mocks.Shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				if _, err := w.Write([]byte("error\n")); err != nil {
					t.Errorf("Failed to write to stderr pipe: %v", err)
				}
				w.Close()
			}()
			return r, nil
		}
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			return fmt.Errorf("command start error")
		}

		// When executing a command with progress
		output, err := shell.ExecProgress("test message", "test", "arg")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "command start error") {
			t.Errorf("Expected error about command start, got: %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
	})
}

func TestShell_GetSessionToken(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a shell with session token and no reset flag
		shell, mocks := setup(t)
		expectedToken := "test-token"
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return expectedToken
			}
			return ""
		}

		// Mock Stat to return file not found (no reset flag exists)
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// Mock Glob to return empty slice (no session files)
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return []string{}, nil
		}

		// When getting session token
		token, err := shell.GetSessionToken()

		// Then it should return the expected token
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if token != expectedToken {
			t.Errorf("Expected token %q, got %q", expectedToken, token)
		}
	})

	t.Run("NoToken", func(t *testing.T) {
		// Given a shell with mocked operations
		shell, mocks := setup(t)

		// Mock environment to return no token
		mocks.Shims.Getenv = func(key string) string {
			return ""
		}

		// Mock random generation
		mocks.Shims.RandRead = func(b []byte) (n int, err error) {
			for i := range b {
				b[i] = byte('a')
			}
			return len(b), nil
		}

		// When getting session token
		token, err := shell.GetSessionToken()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Then it should return a token of length 7
		if len(token) != 7 {
			t.Errorf("Expected token length 7, got %d", len(token))
		}
	})

	t.Run("ReturnsCachedToken", func(t *testing.T) {
		// Given a shell with a cached session token
		shell, _ := setup(t)
		expectedToken := "cached-token"
		shell.sessionToken = expectedToken

		// When getting session token
		token, err := shell.GetSessionToken()

		// Then it should return the cached token without calling Getenv
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if token != expectedToken {
			t.Errorf("Expected token %q, got %q", expectedToken, token)
		}
	})
}

func TestShell_CheckResetFlags(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		shell, mocks := setup(t)
		expectedToken := "test-token"
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return expectedToken
			}
			return ""
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, expectedToken) {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return []string{"/test/project/.windsor/session-test-token"}, nil
		}
		mocks.Shims.RemoveAll = func(path string) error {
			return nil
		}

		shouldReset, err := shell.CheckResetFlags()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !shouldReset {
			t.Error("Expected shouldReset to be true")
		}
	})

	t.Run("NoResetToken", func(t *testing.T) {
		shell, mocks := setup(t)
		mocks.Shims.Getenv = func(key string) string {
			return ""
		}

		shouldReset, err := shell.CheckResetFlags()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if shouldReset {
			t.Error("Expected shouldReset to be false")
		}
	})

	t.Run("TokenMismatch", func(t *testing.T) {
		shell, mocks := setup(t)
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_RESET_TOKEN" {
				return "test-token"
			}
			return ""
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return []string{}, nil
		}

		shouldReset, err := shell.CheckResetFlags()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if shouldReset {
			t.Error("Expected shouldReset to be false")
		}
	})

	t.Run("ErrorOnGlob", func(t *testing.T) {
		// Given a shell with failing Glob
		shell, mocks := setup(t)
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return "test-token"
			}
			return ""
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return nil, fmt.Errorf("glob error")
		}

		// When checking reset flags
		_, err := shell.CheckResetFlags()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "glob error") {
			t.Errorf("Expected error to contain 'glob error', got %v", err)
		}
	})

	t.Run("ErrorOnRemoveAll", func(t *testing.T) {
		// Given a shell with failing RemoveAll
		shell, mocks := setup(t)
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return "test-token"
			}
			return ""
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return []string{"/test/project/.windsor/session-test-token"}, nil
		}
		mocks.Shims.RemoveAll = func(path string) error {
			return fmt.Errorf("remove error")
		}

		// When checking reset flags
		_, err := shell.CheckResetFlags()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "remove error") {
			t.Errorf("Expected error to contain 'remove error', got %v", err)
		}
	})
}

func TestShell_GenerateRandomString(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		shell, mocks := setup(t)
		mocks.Shims.RandRead = func(b []byte) (n int, err error) {
			for i := range b {
				b[i] = byte(i % 62) // This will map to characters in the charset
			}
			return len(b), nil
		}
		result, err := shell.generateRandomString(10)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(result) != 10 {
			t.Errorf("Expected length 10, got %d", len(result))
		}
	})

	t.Run("ErrorWhenRandReadFails", func(t *testing.T) {
		shell, mocks := setup(t)
		mocks.Shims.RandRead = func(b []byte) (n int, err error) {
			return 0, fmt.Errorf("random read failed")
		}
		result, err := shell.generateRandomString(10)
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if result != "" {
			t.Errorf("Expected empty result on error, got %q", result)
		}
	})
}

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
		if !strings.Contains(err.Error(), "Unsupported shell: unsupported") {
			t.Errorf("Expected error about unsupported shell, got: %v", err)
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

// mockReadCloser implements io.ReadCloser for testing
type mockReadCloser struct {
	io.Reader
	closeFunc func() error
}

func (m *mockReadCloser) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

// =============================================================================
// Test Helpers
// =============================================================================

// Helper function to capture stdout
func captureStdout(t *testing.T, f func()) string {
	var output bytes.Buffer
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
		w.Close()
	}()

	_, err := output.ReadFrom(r)
	if err != nil {
		t.Fatalf("Failed to read from pipe: %v", err)
	}
	<-done
	os.Stdout = originalStdout
	return output.String()
}

func TestShell_AddCurrentDirToTrustedFile(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a shell with mocked operations
		shell, mocks := setup(t)
		projectRoot := "/test/project"
		homeDir := "/home/test"
		trustedFilePath := "/home/test/.config/windsor/.trusted"

		// Mock GetProjectRoot
		mocks.Shims.Getwd = func() (string, error) {
			return projectRoot, nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join(projectRoot, "windsor.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock UserHomeDir
		mocks.Shims.UserHomeDir = func() (string, error) {
			return homeDir, nil
		}

		// Mock file operations
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			if path != "/home/test/.config/windsor" {
				t.Errorf("Expected path /home/test/.config/windsor, got %s", path)
			}
			return nil
		}

		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			if name != trustedFilePath {
				t.Errorf("Expected path %s, got %s", trustedFilePath, name)
			}
			return []byte("/other/project\n"), nil
		}

		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			if name != trustedFilePath {
				t.Errorf("Expected path %s, got %s", trustedFilePath, name)
			}
			expectedData := []byte("/other/project\n" + projectRoot + "\n")
			if string(data) != string(expectedData) {
				t.Errorf("Expected data %s, got %s", string(expectedData), string(data))
			}
			if perm != 0600 {
				t.Errorf("Expected perm 0600, got %o", perm)
			}
			return nil
		}

		// When adding current dir to trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorOnGetProjectRoot", func(t *testing.T) {
		// Given a shell with failing GetProjectRoot
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("getwd failed")
		}

		// When adding current dir to trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error getting project root directory") {
			t.Errorf("Expected error about project root, got: %v", err)
		}
	})

	t.Run("ErrorOnUserHomeDir", func(t *testing.T) {
		// Given a shell with failing UserHomeDir
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/project", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "/test/project/windsor.yaml" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "", fmt.Errorf("user home dir failed")
		}

		// When adding current dir to trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error getting user home directory") {
			t.Errorf("Expected error about user home dir, got: %v", err)
		}
	})

	t.Run("ErrorOnMkdirAll", func(t *testing.T) {
		// Given a shell with failing MkdirAll
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/project", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "/test/project/windsor.yaml" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "/home/test", nil
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mkdir failed")
		}

		// When adding current dir to trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error creating directories for trusted file") {
			t.Errorf("Expected error about mkdir, got: %v", err)
		}
	})

	t.Run("ErrorOnReadFile", func(t *testing.T) {
		// Given a shell with failing ReadFile
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/project", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "/test/project/windsor.yaml" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "/home/test", nil
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return nil, fmt.Errorf("read failed")
		}

		// When adding current dir to trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error reading trusted file") {
			t.Errorf("Expected error about read file, got: %v", err)
		}
	})

	t.Run("ErrorOnWriteFile", func(t *testing.T) {
		// Given a shell with failing WriteFile
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/project", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "/test/project/windsor.yaml" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "/home/test", nil
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("/other/project\n"), nil
		}
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("write failed")
		}

		// When adding current dir to trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error writing to trusted file") {
			t.Errorf("Expected error about write file, got: %v", err)
		}
	})

	t.Run("AlreadyTrusted", func(t *testing.T) {
		// Given a shell with current dir already trusted
		shell, mocks := setup(t)
		projectRoot := "/test/project"
		mocks.Shims.Getwd = func() (string, error) {
			return projectRoot, nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join(projectRoot, "windsor.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "/home/test", nil
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte(projectRoot + "\n"), nil
		}

		// When adding current dir to trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then it should succeed without writing
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestShell_CheckTrustedDirectory(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a shell with mocked operations
		shell, mocks := setup(t)
		projectRoot := "/test/project"
		homeDir := "/home/test"
		trustedFilePath := "/home/test/.config/windsor/.trusted"

		// Mock GetProjectRoot
		mocks.Shims.Getwd = func() (string, error) {
			return projectRoot, nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join(projectRoot, "windsor.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock UserHomeDir
		mocks.Shims.UserHomeDir = func() (string, error) {
			return homeDir, nil
		}

		// Mock ReadFile
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			if name != trustedFilePath {
				t.Errorf("Expected path %s, got %s", trustedFilePath, name)
			}
			return []byte("/test\n"), nil
		}

		// When checking trusted directory
		err := shell.CheckTrustedDirectory()

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorOnGetProjectRoot", func(t *testing.T) {
		// Given a shell with failing GetProjectRoot
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("getwd failed")
		}

		// When checking trusted directory
		err := shell.CheckTrustedDirectory()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error getting project root directory") {
			t.Errorf("Expected error about project root, got: %v", err)
		}
	})

	t.Run("ErrorOnUserHomeDir", func(t *testing.T) {
		// Given a shell with failing UserHomeDir
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/project", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "/test/project/windsor.yaml" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "", fmt.Errorf("user home dir failed")
		}

		// When checking trusted directory
		err := shell.CheckTrustedDirectory()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error getting user home directory") {
			t.Errorf("Expected error about user home dir, got: %v", err)
		}
	})

	t.Run("ErrorOnReadFile", func(t *testing.T) {
		// Given a shell with failing ReadFile
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/project", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "/test/project/windsor.yaml" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "/home/test", nil
		}
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return nil, fmt.Errorf("read failed")
		}

		// When checking trusted directory
		err := shell.CheckTrustedDirectory()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error reading trusted file") {
			t.Errorf("Expected error about read file, got: %v", err)
		}
	})

	t.Run("TrustedFileNotExist", func(t *testing.T) {
		// Given a shell with non-existent trusted file
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/project", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "/test/project/windsor.yaml" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "/home/test", nil
		}
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		// When checking trusted directory
		err := shell.CheckTrustedDirectory()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Trusted file does not exist") {
			t.Errorf("Expected error about file not existing, got: %v", err)
		}
	})

	t.Run("NotTrusted", func(t *testing.T) {
		// Given a shell with directory not in trusted list
		shell, mocks := setup(t)
		projectRoot := "/test/project"
		mocks.Shims.Getwd = func() (string, error) {
			return projectRoot, nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join(projectRoot, "windsor.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "/home/test", nil
		}
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("/other/project\n"), nil
		}

		// When checking trusted directory
		err := shell.CheckTrustedDirectory()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Current directory not in the trusted list") {
			t.Errorf("Expected error about directory not trusted, got: %v", err)
		}
	})
}

func TestShell_WriteResetToken(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a shell with mocked operations
		shell, mocks := setup(t)
		projectRoot := "/test/project"
		sessionToken := "test-token"
		sessionFilePath := filepath.Join(projectRoot, ".windsor", ".session."+sessionToken)

		// Mock GetProjectRoot
		mocks.Shims.Getwd = func() (string, error) {
			return projectRoot, nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join(projectRoot, "windsor.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock environment
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return sessionToken
			}
			return ""
		}

		// Mock file operations
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			expectedPath := filepath.Join(projectRoot, ".windsor")
			if path != expectedPath {
				t.Errorf("Expected path %s, got %s", expectedPath, path)
			}
			if perm != 0750 {
				t.Errorf("Expected perm 0750, got %o", perm)
			}
			return nil
		}

		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			if name != sessionFilePath {
				t.Errorf("Expected path %s, got %s", sessionFilePath, name)
			}
			if len(data) != 0 {
				t.Errorf("Expected empty data, got %v", data)
			}
			if perm != 0600 {
				t.Errorf("Expected perm 0600, got %o", perm)
			}
			return nil
		}

		// When writing reset token
		path, err := shell.WriteResetToken()

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if path != sessionFilePath {
			t.Errorf("Expected path %s, got %s", sessionFilePath, path)
		}
	})

	t.Run("NoSessionToken", func(t *testing.T) {
		// Given a shell with no session token
		shell, mocks := setup(t)
		mocks.Shims.Getenv = func(key string) string {
			return ""
		}

		// When writing reset token
		path, err := shell.WriteResetToken()

		// Then it should return empty path without error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if path != "" {
			t.Errorf("Expected empty path, got %s", path)
		}
	})

	t.Run("ErrorOnGetProjectRoot", func(t *testing.T) {
		// Given a shell with failing GetProjectRoot
		shell, mocks := setup(t)
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return "test-token"
			}
			return ""
		}
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("getwd failed")
		}

		// When writing reset token
		path, err := shell.WriteResetToken()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting project root") {
			t.Errorf("Expected error about project root, got: %v", err)
		}
		if path != "" {
			t.Errorf("Expected empty path, got %s", path)
		}
	})

	t.Run("ErrorOnMkdirAll", func(t *testing.T) {
		// Given a shell with failing MkdirAll
		shell, mocks := setup(t)
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return "test-token"
			}
			return ""
		}
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/project", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "/test/project/windsor.yaml" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mkdir failed")
		}

		// When writing reset token
		path, err := shell.WriteResetToken()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error creating .windsor directory") {
			t.Errorf("Expected error about mkdir, got: %v", err)
		}
		if path != "" {
			t.Errorf("Expected empty path, got %s", path)
		}
	})

	t.Run("ErrorOnWriteFile", func(t *testing.T) {
		// Given a shell with failing WriteFile
		shell, mocks := setup(t)
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return "test-token"
			}
			return ""
		}
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/project", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "/test/project/windsor.yaml" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("write failed")
		}

		// When writing reset token
		path, err := shell.WriteResetToken()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error writing reset token file") {
			t.Errorf("Expected error about write file, got: %v", err)
		}
		if path != "" {
			t.Errorf("Expected empty path, got %s", path)
		}
	})
}

func TestShell_Reset(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a shell with mocked operations
		shell, mocks := setup(t)
		projectRoot := "/test/project"
		sessionFiles := []string{
			filepath.Join(projectRoot, ".windsor", ".session.1"),
			filepath.Join(projectRoot, ".windsor", ".session.2"),
		}

		// Mock GetProjectRoot
		mocks.Shims.Getwd = func() (string, error) {
			return projectRoot, nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join(projectRoot, "windsor.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock file operations
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			expectedPattern := filepath.Join(projectRoot, ".windsor", ".session.*")
			if pattern != expectedPattern {
				t.Errorf("Expected pattern %s, got %s", expectedPattern, pattern)
			}
			return sessionFiles, nil
		}

		mocks.Shims.RemoveAll = func(path string) error {
			if path != filepath.Join(projectRoot, ".windsor") {
				t.Errorf("Expected path %s, got %s", filepath.Join(projectRoot, ".windsor"), path)
			}
			return nil
		}

		// When resetting
		shell.Reset()
	})

	t.Run("ErrorOnGetProjectRoot", func(t *testing.T) {
		// Given a shell with failing GetProjectRoot
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("getwd failed")
		}

		// When resetting
		shell.Reset()
		// No error expected since Reset() doesn't return error
	})

	t.Run("ErrorOnGlob", func(t *testing.T) {
		// Given a shell with failing Glob
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/project", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "/test/project/windsor.yaml" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return nil, fmt.Errorf("glob failed")
		}

		// When resetting
		shell.Reset()
		// No error expected since Reset() doesn't return error
	})

	t.Run("ErrorOnRemoveAll", func(t *testing.T) {
		// Given a shell with failing RemoveAll
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/project", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "/test/project/windsor.yaml" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return []string{"/test/project/.windsor/.session.1"}, nil
		}
		mocks.Shims.RemoveAll = func(path string) error {
			return fmt.Errorf("remove failed")
		}

		// When resetting
		shell.Reset()
		// No error expected since Reset() doesn't return error
	})

	t.Run("NoSessionFiles", func(t *testing.T) {
		// Given a shell with no session files
		shell, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/project", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "/test/project/windsor.yaml" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return []string{}, nil
		}

		// When resetting
		shell.Reset()
		// No error expected since Reset() doesn't return error
	})

	t.Run("EnvironmentAndAliasReset", func(t *testing.T) {
		// Given a shell with managed environment variables and aliases
		shell, mocks := setup(t)

		// Mock environment variables
		mocks.Shims.Getenv = func(key string) string {
			switch key {
			case "WINDSOR_MANAGED_ENV":
				return "ENV1, ENV2, ENV3"
			case "WINDSOR_MANAGED_ALIAS":
				return "ALIAS1, ALIAS2, ALIAS3"
			default:
				return ""
			}
		}

		// When resetting
		shell.Reset()

		// Then environment variables and aliases should be unset
		// Note: We can't directly verify the unset operations since they're system calls
		// The test coverage will show that these code paths were executed
	})

	t.Run("EmptyEnvironmentAndAlias", func(t *testing.T) {
		// Given a shell with empty managed environment variables and aliases
		shell, mocks := setup(t)

		// Mock empty environment variables
		mocks.Shims.Getenv = func(key string) string {
			return ""
		}

		// When resetting
		shell.Reset()

		// Then no environment variables or aliases should be unset
		// Note: We can't directly verify the unset operations since they're system calls
		// The test coverage will show that these code paths were executed
	})

	t.Run("QuietModeDoesNotPrintCommands", func(t *testing.T) {
		// Given a shell with managed environment variables and aliases
		shell, mocks := setup(t)

		// Mock environment variables
		mocks.Shims.Getenv = func(key string) string {
			switch key {
			case "WINDSOR_MANAGED_ENV":
				return "ENV1, ENV2, ENV3"
			case "WINDSOR_MANAGED_ALIAS":
				return "ALIAS1, ALIAS2, ALIAS3"
			default:
				return ""
			}
		}

		// When resetting with quiet=true
		output := captureStdout(t, func() {
			shell.Reset(true)
		})

		// Then no shell commands should be printed
		if output != "" {
			t.Errorf("Expected no output in quiet mode, got: %v", output)
		}
	})

	t.Run("NonQuietModePrintsCommands", func(t *testing.T) {
		// Given a shell with managed environment variables and aliases
		shell, mocks := setup(t)

		// Mock environment variables
		mocks.Shims.Getenv = func(key string) string {
			switch key {
			case "WINDSOR_MANAGED_ENV":
				return "ENV1, ENV2, ENV3"
			case "WINDSOR_MANAGED_ALIAS":
				return "ALIAS1, ALIAS2, ALIAS3"
			default:
				return ""
			}
		}

		// When resetting with quiet=false
		output := captureStdout(t, func() {
			shell.Reset(false)
		})

		// Then shell commands should be printed (platform-specific)
		// Unix: "unset ENV1 ENV2 ENV3" / Windows: "Remove-Item Env:ENV1"
		if !strings.Contains(output, "unset ENV1 ENV2 ENV3") && !strings.Contains(output, "Remove-Item Env:ENV1") {
			t.Errorf("Expected environment unset command to be printed, got: %v", output)
		}
		// Unix: "unalias ALIAS1" / Windows: "Remove-Item Alias:ALIAS1"
		if !strings.Contains(output, "unalias ALIAS1") && !strings.Contains(output, "Remove-Item Alias:ALIAS1") {
			t.Errorf("Expected alias unset command to be printed, got: %v", output)
		}
	})
}

func TestShell_ResetSessionToken(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a shell with mocked operations
		shell, mocks := setup(t)
		expectedToken := "test-token"

		// Mock environment variable
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return expectedToken
			}
			return ""
		}

		// Mock random generation to return predictable bytes
		mocks.Shims.RandRead = func(b []byte) (n int, err error) {
			// Fill with bytes that will map to "new-test-token"
			for i := range b {
				b[i] = byte(i % 62)
			}
			return len(b), nil
		}

		// Mock Stat to return file not found (no reset flag exists)
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// Mock Glob to return empty slice (no session files)
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return []string{}, nil
		}

		// When getting session token
		token, err := shell.GetSessionToken()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if token != expectedToken {
			t.Errorf("Expected token %q, got %q", expectedToken, token)
		}

		// When resetting session token
		shell.ResetSessionToken()
		mocks.Shims.Getenv = func(key string) string {
			return "" // Simulate environment variable being unset
		}

		// Then getting session token should return a new token
		newToken, err := shell.GetSessionToken()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(newToken) != 7 {
			t.Errorf("Expected new token length to be 7, got %d", len(newToken))
		}
	})
}

func TestShell_ExecSilentWithTimeout(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		shell, _ := setup(t)

		out, err := shell.ExecSilentWithTimeout("test", []string{"arg"}, 5*time.Second)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if out != "test\n" {
			t.Errorf("Expected output 'test\n', got '%s'", out)
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		shell, mocks := setup(t)
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			return nil
		}
		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			time.Sleep(200 * time.Millisecond)
			return nil
		}

		out, err := shell.ExecSilentWithTimeout("test", []string{"arg"}, 50*time.Millisecond)

		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Errorf("Expected timeout error, got: %v", err)
		}
		if out != "" {
			t.Errorf("Expected empty output on timeout, got '%s'", out)
		}
	})

	t.Run("Error", func(t *testing.T) {
		shell, mocks := setup(t)
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			return nil
		}
		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			return fmt.Errorf("command failed")
		}

		out, err := shell.ExecSilentWithTimeout("test", []string{"arg"}, 5*time.Second)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "command failed") {
			t.Errorf("Expected error to contain 'command failed', got %v", err)
		}
		if out != "" {
			t.Errorf("Expected empty output, got '%s'", out)
		}
	})

	t.Run("CommandNil", func(t *testing.T) {
		shell, mocks := setup(t)
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return nil
		}

		output, err := shell.ExecSilentWithTimeout("test", []string{"arg"}, 5*time.Second)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create command") {
			t.Errorf("Expected error about command creation, got: %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
	})

	t.Run("VerboseMode", func(t *testing.T) {
		shell, mocks := setup(t)
		shell.SetVerbosity(true)

		execCalled := false
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			execCalled = true
			return exec.Command("echo", "test")
		}
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			if cmd.Stdout != nil {
				if _, err := cmd.Stdout.Write([]byte("test\n")); err != nil {
					return fmt.Errorf("failed to write to stdout: %v", err)
				}
			}
			return nil
		}
		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			return nil
		}

		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		out, err := shell.ExecSilentWithTimeout("test", []string{"arg"}, 5*time.Second)

		w.Close()
		os.Stdout = oldStdout
		io.ReadAll(r)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !execCalled {
			t.Error("Expected Exec to be called in verbose mode, but it was not")
		}
		if out != "test\n" {
			t.Errorf("Expected output 'test\n', got '%s'", out)
		}
	})

	t.Run("ErrorOnStart", func(t *testing.T) {
		shell, mocks := setup(t)
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return exec.Command("echo", "test")
		}
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			return fmt.Errorf("command start failed")
		}

		out, err := shell.ExecSilentWithTimeout("test", []string{"arg"}, 5*time.Second)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "command start failed") {
			t.Errorf("Expected error to contain 'command start failed', got %v", err)
		}
		if out != "" {
			t.Errorf("Expected empty output, got '%s'", out)
		}
	})

	t.Run("TimeoutNoDoubleWait", func(t *testing.T) {
		shell, mocks := setup(t)
		waitCallCount := 0
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			return nil
		}
		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			waitCallCount++
			time.Sleep(200 * time.Millisecond)
			return nil
		}

		out, err := shell.ExecSilentWithTimeout("test", []string{"arg"}, 50*time.Millisecond)

		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Errorf("Expected timeout error, got: %v", err)
		}
		if out != "" {
			t.Errorf("Expected empty output on timeout, got '%s'", out)
		}
		if waitCallCount != 1 {
			t.Errorf("Expected Wait to be called exactly once, got %d calls", waitCallCount)
		}
	})
}

func TestShell_ExecProgress(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a shell with mocked operations
		shell, mocks := setup(t)

		// Set expected output
		expectedOutput := "test output\n"
		message := "Test Progress"

		// Mock command execution
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return exec.Command("test")
		}

		// Mock stdout pipe to write expected output
		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				w.Write([]byte(expectedOutput))
				w.Close()
			}()
			return r, nil
		}

		// Mock stderr pipe
		mocks.Shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			w.Close()
			return r, nil
		}

		// Mock command start and wait
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			return nil
		}
		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			return nil
		}

		// When executing command with progress
		output, err := shell.ExecProgress(message, "test")

		// Then it should succeed and return expected output
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("VerboseMode", func(t *testing.T) {
		// Given a shell with verbose mode enabled
		shell, mocks := setup(t)
		shell.SetVerbosity(true)

		// Set expected output and message
		expectedOutput := "test output\n"
		message := "Test Progress"

		// Mock command execution
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			cmd := exec.Command("test")
			cmd.Stdout = new(bytes.Buffer)
			cmd.Stderr = new(bytes.Buffer)
			return cmd
		}

		// Mock command start to write output
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			if cmd.Stdout != nil {
				cmd.Stdout.Write([]byte(expectedOutput))
			}
			return nil
		}

		// Mock command wait
		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			return nil
		}

		// When executing command with progress
		output, err := shell.ExecProgress(message, "test")

		// Then it should succeed and return expected output
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("CommandCreationFailure", func(t *testing.T) {
		// Given a shell with failing command creation
		shell, mocks := setup(t)

		// Mock command execution to fail
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return nil
		}

		// When executing command with progress
		output, err := shell.ExecProgress("Test Progress", "test")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create command") {
			t.Errorf("Expected error to contain 'failed to create command', got %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
	})

	t.Run("StdoutPipeFailure", func(t *testing.T) {
		// Given a shell with failing stdout pipe
		shell, mocks := setup(t)

		// Mock command execution
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return exec.Command("test")
		}

		// Mock stdout pipe to fail
		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			return nil, fmt.Errorf("stdout pipe error")
		}

		// When executing command with progress
		output, err := shell.ExecProgress("Test Progress", "test")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "stdout pipe error") {
			t.Errorf("Expected error to contain 'stdout pipe error', got %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
	})

	t.Run("StderrPipeFailure", func(t *testing.T) {
		// Given a shell with failing stderr pipe
		shell, mocks := setup(t)

		// Mock command execution
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return exec.Command("test")
		}

		// Mock stdout pipe to succeed
		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				w.Write([]byte("test output\n"))
				w.Close()
			}()
			return r, nil
		}

		// Mock stderr pipe to fail
		mocks.Shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			return nil, fmt.Errorf("stderr pipe error")
		}

		// When executing command with progress
		output, err := shell.ExecProgress("Test Progress", "test")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "stderr pipe error") {
			t.Errorf("Expected error to contain 'stderr pipe error', got %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
	})

	t.Run("CommandStartFailure", func(t *testing.T) {
		// Given a shell with failing command start
		shell, mocks := setup(t)

		// Mock command execution
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return exec.Command("test")
		}

		// Mock stdout pipe
		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				w.Write([]byte("test output\n"))
				w.Close()
			}()
			return r, nil
		}

		// Mock stderr pipe
		mocks.Shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			w.Close()
			return r, nil
		}

		// Mock command start to fail
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			return fmt.Errorf("command start error")
		}

		// When executing command with progress
		output, err := shell.ExecProgress("Test Progress", "test")

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "command start error") {
			t.Errorf("Expected error to contain 'command start error', got %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
	})

	t.Run("CommandExecutionFailure", func(t *testing.T) {
		shell, mocks := setup(t)

		expectedOutput := ""
		message := "Test Progress"

		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return exec.Command("test")
		}

		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				w.Close()
			}()
			return r, nil
		}

		mocks.Shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				w.Close()
			}()
			return r, nil
		}

		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			return nil
		}

		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			return fmt.Errorf("command execution error")
		}

		output, err := shell.ExecProgress(message, "test")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "command execution error") {
			t.Errorf("Expected error to contain 'command execution error', got %v", err)
		}
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("StdoutScannerError", func(t *testing.T) {
		shell, mocks := setup(t)
		message := "Test Progress"

		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return exec.Command("test")
		}

		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				w.Close()
			}()
			return r, nil
		}

		mocks.Shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				w.Close()
			}()
			return r, nil
		}

		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			return nil
		}

		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			return nil
		}

		mocks.Shims.ScannerErr = func(scanner *bufio.Scanner) error {
			return fmt.Errorf("stdout scanner error")
		}

		output, err := shell.ExecProgress(message, "test")
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "stdout scanner error") {
			t.Errorf("Expected error to contain 'stdout scanner error', got %v", err)
		}
	})

	t.Run("StderrScannerError", func(t *testing.T) {
		shell, mocks := setup(t)
		message := "Test Progress"

		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return exec.Command("test")
		}

		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				w.Close()
			}()
			return r, nil
		}

		mocks.Shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				w.Close()
			}()
			return r, nil
		}

		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			return nil
		}

		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			return nil
		}

		mocks.Shims.ScannerErr = func(scanner *bufio.Scanner) error {
			return fmt.Errorf("stderr scanner error")
		}

		output, err := shell.ExecProgress(message, "test")
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "stderr scanner error") {
			t.Errorf("Expected error to contain 'stderr scanner error', got %v", err)
		}
	})

	t.Run("EmptyOutput", func(t *testing.T) {
		shell, mocks := setup(t)

		expectedOutput := ""
		message := "Test Progress"

		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return exec.Command("test")
		}

		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				w.Close()
			}()
			return r, nil
		}

		mocks.Shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				w.Close()
			}()
			return r, nil
		}

		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			return nil
		}

		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			return nil
		}

		output, err := shell.ExecProgress(message, "test")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("MultiLineOutput", func(t *testing.T) {
		shell, mocks := setup(t)

		expectedOutput := "line 1\nline 2\nline 3\n"
		message := "Test Progress"

		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return exec.Command("test")
		}

		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				w.Write([]byte(expectedOutput))
				w.Close()
			}()
			return r, nil
		}

		mocks.Shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				w.Close()
			}()
			return r, nil
		}

		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			return nil
		}

		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			return nil
		}

		output, err := shell.ExecProgress(message, "test")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ScannerBehavior", func(t *testing.T) {
		shell, mocks := setup(t)
		message := "Test Progress"
		expectedOutput := "test output\n"

		// Mock command execution
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			cmd := exec.Command("test")
			cmd.Stdout = new(bytes.Buffer)
			cmd.Stderr = new(bytes.Buffer)
			return cmd
		}

		// Mock command start to write output
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			if cmd.Stdout != nil {
				cmd.Stdout.Write([]byte(expectedOutput))
			}
			return nil
		}

		// Mock command wait
		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			return nil
		}

		// When executing command with progress
		output, err := shell.ExecProgress(message, "test")

		// Then it should succeed and return expected output
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("EmptyStderrOnFailure", func(t *testing.T) {
		// Given a shell with mocked operations
		shell, mocks := setup(t)

		// Mock command execution
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return exec.Command("test-command", "arg1", "arg2")
		}

		// Mock stdout pipe to return test output
		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("test output\n")), nil
		}

		// Mock stderr pipe to return empty string
		mocks.Shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("")), nil
		}

		// Mock command start and wait
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			return nil
		}
		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			return fmt.Errorf("command failed")
		}

		// When executing command with progress
		output, err := shell.ExecProgress("test progress", "test-command", "arg1", "arg2")

		// Then it should return error and output
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if output != "test output\n" {
			t.Errorf("Expected output 'test output', got '%s'", output)
		}
	})
}

// =============================================================================
// Secret Management Tests
// =============================================================================

func TestShell_RegisterSecret(t *testing.T) {
	// setup creates a new shell with mocked dependencies for testing
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		return shell, mocks
	}

	t.Run("RegisterSingleSecret", func(t *testing.T) {
		// Given a shell instance with no registered secrets
		shell, _ := setup(t)

		// When registering a single secret value
		shell.RegisterSecret("mysecret123")

		// Then the secret should be stored in the secrets list
		if len(shell.secrets) != 1 {
			t.Errorf("Expected 1 secret, got %d", len(shell.secrets))
		}
		if shell.secrets[0] != "mysecret123" {
			t.Errorf("Expected secret 'mysecret123', got '%s'", shell.secrets[0])
		}
	})

	t.Run("RegisterMultipleSecrets", func(t *testing.T) {
		// Given a shell instance with no registered secrets
		shell, _ := setup(t)

		// When registering multiple different secret values
		shell.RegisterSecret("secret1")
		shell.RegisterSecret("secret2")
		shell.RegisterSecret("secret3")

		// Then all secrets should be stored in the secrets list
		if len(shell.secrets) != 3 {
			t.Errorf("Expected 3 secrets, got %d", len(shell.secrets))
		}
		expectedSecrets := []string{"secret1", "secret2", "secret3"}
		for i, expected := range expectedSecrets {
			if shell.secrets[i] != expected {
				t.Errorf("Expected secret[%d] to be '%s', got '%s'", i, expected, shell.secrets[i])
			}
		}
	})

	t.Run("RegisterEmptySecret", func(t *testing.T) {
		// Given a shell instance with no registered secrets
		shell, _ := setup(t)

		// When attempting to register an empty string as a secret
		shell.RegisterSecret("")

		// Then the empty secret should be ignored and not stored
		if len(shell.secrets) != 0 {
			t.Errorf("Expected 0 secrets, got %d", len(shell.secrets))
		}
	})

	t.Run("RegisterDuplicateSecrets", func(t *testing.T) {
		// Given a shell instance with no registered secrets
		shell, _ := setup(t)

		// When registering the same secret value multiple times
		shell.RegisterSecret("duplicate")
		shell.RegisterSecret("duplicate")
		shell.RegisterSecret("duplicate")

		// Then only one instance should be stored to prevent duplicates
		if len(shell.secrets) != 1 {
			t.Errorf("Expected 1 secret, got %d", len(shell.secrets))
		}
		if shell.secrets[0] != "duplicate" {
			t.Errorf("Expected secret to be 'duplicate', got '%s'", shell.secrets[0])
		}
	})
}

func TestShell_scrubString(t *testing.T) {
	// setup creates a new shell with mocked dependencies for testing
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		return shell, mocks
	}

	t.Run("ScrubSingleSecret", func(t *testing.T) {
		// Given a shell with one registered secret value
		shell, _ := setup(t)
		shell.RegisterSecret("mysecret123")

		// When scrubbing a string that contains the registered secret
		input := "The password is mysecret123 and it's confidential"
		result := shell.scrubString(input)

		// Then the secret should be replaced with asterisks while preserving the rest of the text
		expected := "The password is ******** and it's confidential"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubMultipleSecrets", func(t *testing.T) {
		// Given a shell with multiple registered secret values
		shell, _ := setup(t)
		shell.RegisterSecret("secret1")
		shell.RegisterSecret("secret2")
		shell.RegisterSecret("secret3")

		// When scrubbing a string that contains multiple registered secrets
		input := "First secret1, then secret2, finally secret3"
		result := shell.scrubString(input)

		// Then all secrets should be replaced with asterisks
		expected := "First ********, then ********, finally ********"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubNoSecrets", func(t *testing.T) {
		// Given a shell with no registered secrets
		shell, _ := setup(t)

		// When scrubbing a string that contains no sensitive information
		input := "This string contains no secrets"
		result := shell.scrubString(input)

		// Then the string should remain completely unchanged
		if result != input {
			t.Errorf("Expected unchanged string '%s', got '%s'", input, result)
		}
	})

	t.Run("ScrubEmptyString", func(t *testing.T) {
		// Given a shell with registered secrets
		shell, _ := setup(t)
		shell.RegisterSecret("secret")

		// When scrubbing an empty input string
		input := ""
		result := shell.scrubString(input)

		// Then the result should remain empty
		if result != "" {
			t.Errorf("Expected empty string, got '%s'", result)
		}
	})

	t.Run("ScrubEmptySecret", func(t *testing.T) {
		// Given a shell with an empty string registered as a secret
		shell, _ := setup(t)
		shell.RegisterSecret("")

		// When scrubbing a normal text string
		input := "This is a test string"
		result := shell.scrubString(input)

		// Then the string should remain unchanged since empty secrets don't match anything
		if result != input {
			t.Errorf("Expected unchanged string '%s', got '%s'", input, result)
		}
	})

	t.Run("ScrubSecretAtBeginning", func(t *testing.T) {
		// Given a shell with a registered secret value
		shell, _ := setup(t)
		shell.RegisterSecret("secret123")

		// When scrubbing a string where the secret appears at the beginning
		input := "secret123 is at the start"
		result := shell.scrubString(input)

		// Then the secret should be replaced with asterisks
		expected := "******** is at the start"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubSecretAtEnd", func(t *testing.T) {
		// Given a shell with a registered secret value
		shell, _ := setup(t)
		shell.RegisterSecret("secret123")

		// When scrubbing a string where the secret appears at the end
		input := "The secret is secret123"
		result := shell.scrubString(input)

		// Then the secret should be replaced with asterisks
		expected := "The secret is ********"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubSecretMultipleOccurrences", func(t *testing.T) {
		// Given a shell with a registered secret value
		shell, _ := setup(t)
		shell.RegisterSecret("secret")

		// When scrubbing a string where the same secret appears multiple times
		input := "secret appears here and secret appears there, secret everywhere"
		result := shell.scrubString(input)

		// Then all occurrences of the secret should be replaced with asterisks
		expected := "******** appears here and ******** appears there, ******** everywhere"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubSecretPartialMatch", func(t *testing.T) {
		// Given a shell with a registered secret value
		shell, _ := setup(t)
		shell.RegisterSecret("secret")

		// When scrubbing a string containing words that partially match the secret
		input := "secretive and secrets contain secret"
		result := shell.scrubString(input)

		// Then all occurrences should be replaced using simple string replacement
		expected := "********ive and ********s contain ********"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubLongSecret", func(t *testing.T) {
		// Given a shell with a very long secret value registered
		shell, _ := setup(t)
		longSecret := "this-is-a-very-long-secret-key-with-many-characters-1234567890"
		shell.RegisterSecret(longSecret)

		// When scrubbing a string containing the long secret
		input := fmt.Sprintf("The key is %s and should be hidden", longSecret)
		result := shell.scrubString(input)

		// Then the long secret should be replaced with a fixed-length asterisk string
		expected := "The key is ******** and should be hidden"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubSpecialCharacters", func(t *testing.T) {
		// Given a shell with a secret containing special characters
		shell, _ := setup(t)
		specialSecret := "p@ssw0rd!#$%^&*()"
		shell.RegisterSecret(specialSecret)

		// When scrubbing a string containing the special character secret
		input := fmt.Sprintf("Password: %s", specialSecret)
		result := shell.scrubString(input)

		// Then the special character secret should be replaced with asterisks
		expected := "Password: ********"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubComplexMultipleSecrets", func(t *testing.T) {
		// Given a shell with multiple different registered secrets
		shell, _ := setup(t)
		shell.RegisterSecret("secret123")
		shell.RegisterSecret("password456")
		shell.RegisterSecret("token789")

		// When scrubbing a complex configuration-like string with multiple secrets
		input := "Config: secret123, Auth: password456, API: token789, Normal: text"
		result := shell.scrubString(input)

		// Then all secrets should be scrubbed while preserving non-secret text
		expected := "Config: ********, Auth: ********, API: ********, Normal: text"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubSecretsInJSON", func(t *testing.T) {
		// Given a shell with registered password and token secrets
		shell, _ := setup(t)
		shell.RegisterSecret("mypassword")
		shell.RegisterSecret("mytoken")

		// When scrubbing a JSON-formatted string containing secrets
		input := `{"password": "mypassword", "token": "mytoken", "user": "admin"}`
		result := shell.scrubString(input)

		// Then the secrets should be scrubbed while preserving JSON structure
		expected := `{"password": "********", "token": "********", "user": "admin"}`
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubSecretsInCommandLine", func(t *testing.T) {
		// Given a shell with a registered secret value
		shell, _ := setup(t)
		shell.RegisterSecret("supersecret")

		// When scrubbing a command line string containing the secret
		input := "terraform apply -var password=supersecret -var user=admin"
		result := shell.scrubString(input)

		// Then the secret should be scrubbed while preserving the command structure
		expected := "terraform apply -var password=******** -var user=admin"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubSecretsInErrorMessages", func(t *testing.T) {
		// Given a shell with a registered password secret
		shell, _ := setup(t)
		shell.RegisterSecret("badpassword")

		// When scrubbing an error message that accidentally contains the secret
		input := "Error: authentication failed with password 'badpassword'"
		result := shell.scrubString(input)

		// Then the secret should be scrubbed to prevent leakage in error logs
		expected := "Error: authentication failed with password '********'"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})
}

func TestScrubbingWriter(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *bytes.Buffer) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims

		// Register a test secret
		shell.RegisterSecret("secret123")

		// Create a buffer to capture output
		var buf bytes.Buffer
		return shell, &buf
	}

	t.Run("ScrubsSecretsFromOutput", func(t *testing.T) {
		// Given a shell with registered secrets and a scrubbing writer configured
		shell, buf := setup(t)
		writer := &scrubbingWriter{writer: buf, scrubFunc: shell.scrubString}

		// When writing content that contains registered secrets to the scrubbing writer
		testContent := "This contains secret123 and other text"
		n, err := writer.Write([]byte(testContent))

		// Then the secrets should be scrubbed from the output while maintaining byte count consistency
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		output := buf.String()
		if strings.Contains(output, "secret123") {
			t.Errorf("Expected secret to be scrubbed, but found in output: %s", output)
		}
		// Should replace with fixed ******** and pad to maintain length
		if !strings.Contains(output, "********") {
			t.Errorf("Expected scrubbed output to contain ********, got: %s", output)
		}
		// Byte counts should match due to padding
		if n != len(testContent) {
			t.Errorf("Expected bytes written to match content length %d, got %d", len(testContent), n)
		}
		// Output length should match input length due to padding
		if len(output) != len(testContent) {
			t.Errorf("Expected output length %d to match input length %d", len(output), len(testContent))
		}
	})

	t.Run("HandlesMultipleSecrets", func(t *testing.T) {
		// Given a shell with multiple registered secrets and a scrubbing writer
		shell, buf := setup(t)
		shell.RegisterSecret("password456")
		writer := &scrubbingWriter{writer: buf, scrubFunc: shell.scrubString}

		// When writing content that contains multiple different secrets
		testContent := "User secret123 has password456 for access"
		_, err := writer.Write([]byte(testContent))

		// Then all secrets should be scrubbed while maintaining output length consistency
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		output := buf.String()
		if strings.Contains(output, "secret123") || strings.Contains(output, "password456") {
			t.Errorf("Expected all secrets to be scrubbed, got: %s", output)
		}
		// Should contain fixed ******** replacements
		if !strings.Contains(output, "********") {
			t.Errorf("Expected scrubbed output to contain ********, got: %s", output)
		}
		// Output length should match input due to padding
		if len(output) != len(testContent) {
			t.Errorf("Expected output length %d to match input length %d", len(output), len(testContent))
		}
	})
}

// TestDefaultShell_renderEnvVarsPlain tests the renderEnvVarsPlain method
func TestDefaultShell_renderEnvVarsPlain(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("RendersPlainEnvVars", func(t *testing.T) {
		// Given a shell with environment variables
		shell, _ := setup(t)
		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
			"VAR3": "",
		}

		// When rendering plain environment variables
		result := shell.renderEnvVarsPlain(envVars)

		// Then the output should contain sorted KEY=value format
		if !strings.Contains(result, "VAR1=value1\n") {
			t.Errorf("Expected VAR1=value1, got: %s", result)
		}
		if !strings.Contains(result, "VAR2=value2\n") {
			t.Errorf("Expected VAR2=value2, got: %s", result)
		}
		if !strings.Contains(result, "VAR3=\n") {
			t.Errorf("Expected VAR3=, got: %s", result)
		}
	})

	t.Run("RendersEmptyEnvVars", func(t *testing.T) {
		// Given a shell with empty environment variables map
		shell, _ := setup(t)
		envVars := map[string]string{}

		// When rendering plain environment variables
		result := shell.renderEnvVarsPlain(envVars)

		// Then the output should be empty
		if result != "" {
			t.Errorf("Expected empty output for empty env vars, got: %s", result)
		}
	})

	t.Run("QuotesValuesWithSpecialCharacters", func(t *testing.T) {
		// Given a shell with environment variables containing special characters
		shell, _ := setup(t)
		envVars := map[string]string{
			"VAR1": `["item1","item2"]`,
			"VAR2": `{"key": "value"}`,
			"VAR3": "simple_value",
			"VAR4": "value with spaces",
			"VAR5": "value$with$dollars",
		}

		// When rendering plain environment variables
		result := shell.renderEnvVarsPlain(envVars)

		// Then values with special characters should be quoted
		if !strings.Contains(result, "VAR1='[\"item1\",\"item2\"]'\n") {
			t.Errorf("Expected VAR1 to be quoted, got: %s", result)
		}
		if !strings.Contains(result, "VAR2='{\"key\": \"value\"}'\n") {
			t.Errorf("Expected VAR2 to be quoted, got: %s", result)
		}
		if !strings.Contains(result, "VAR3=simple_value\n") {
			t.Errorf("Expected VAR3 to not be quoted, got: %s", result)
		}
		if !strings.Contains(result, "VAR4='value with spaces'\n") {
			t.Errorf("Expected VAR4 to be quoted, got: %s", result)
		}
		if !strings.Contains(result, "VAR5='value$with$dollars'\n") {
			t.Errorf("Expected VAR5 to be quoted, got: %s", result)
		}
	})
}

// TestDefaultShell_quoteValueForShell tests the quoteValueForShell method
func TestDefaultShell_quoteValueForShell(t *testing.T) {
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
			{"SimpleValue", "simple", "simple"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := shell.quoteValueForShell(tc.value, false)
				if result != tc.expected {
					t.Errorf("Expected %q, got %q", tc.expected, result)
				}
			})
		}
	})

	t.Run("UsesDoubleQuotesWhenRequested", func(t *testing.T) {
		shell := setup(t)

		result := shell.quoteValueForShell("simple", true)
		if result != `"simple"` {
			t.Errorf("Expected \"simple\", got %q", result)
		}
	})

	t.Run("EscapesSingleQuotesInValue", func(t *testing.T) {
		shell := setup(t)

		result := shell.quoteValueForShell("value'with'quotes", false)
		expected := `'value'"'"'with'"'"'quotes'`
		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	})
}
