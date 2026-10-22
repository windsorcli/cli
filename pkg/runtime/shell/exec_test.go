package shell

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/windsorcli/cli/internal/werror"

	"github.com/windsorcli/cli/internal/presenter"
)

// =============================================================================
// Test Public Methods
// =============================================================================

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

	t.Run("ScrubsSecretsFromInteractiveStderr", func(t *testing.T) {
		// Given a shell with a registered secret and a real pipe standing in for /dev/tty
		shell, mocks := setup(t)
		shell.RegisterSecret("supersecretvalue")

		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("failed to create pipe: %v", err)
		}
		defer r.Close()

		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return &exec.Cmd{Path: name, Args: append([]string{name}, args...)}
		}
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			// Simulate the sudo'd child echoing a secret-bearing line to its interactive stderr.
			_, writeErr := cmd.Stderr.Write([]byte("password for supersecretvalue accepted\n"))
			return writeErr
		}
		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error { return nil }
		mocks.Shims.OpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
			if name == "/dev/tty" {
				return w, nil
			}
			return nil, fmt.Errorf("unexpected file: %s", name)
		}

		// When executing a sudo command
		if _, err := shell.ExecSudo("Running test", "test", "arg"); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the registered secret never reaches what was written to the tty
		buf := make([]byte, 4096)
		n, _ := r.Read(buf)
		got := string(buf[:n])
		if strings.Contains(got, "supersecretvalue") {
			t.Errorf("expected secret scrubbed from tty stderr, got %q", got)
		}
		if !strings.Contains(got, "********") {
			t.Errorf("expected masked placeholder in tty stderr, got %q", got)
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

func TestShell_ExecSilentWithEnv(t *testing.T) {
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

		// When executing a command with env vars silently
		out, err := shell.ExecSilentWithEnv("test", map[string]string{"FOO": "bar"}, "arg")

		// Then it should succeed and return output
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if out != "test\n" {
			t.Errorf("Expected output 'test\n', got '%s'", out)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a shell whose CmdRun returns an error
		shell, mocks := setup(t)
		mocks.Shims.CmdRun = func(cmd *exec.Cmd) error {
			return fmt.Errorf("command failed")
		}

		// When executing a command with env vars silently
		out, err := shell.ExecSilentWithEnv("test", nil, "arg")

		// Then an error is returned and output is empty
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
		// Given a shell whose Command shim returns nil
		shell, mocks := setup(t)
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return nil
		}

		// When executing a command with env vars silently
		output, err := shell.ExecSilentWithEnv("test", nil, "arg")

		// Then an error about command creation is returned
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

	t.Run("MergesEnvVars", func(t *testing.T) {
		// Given a shell with a custom env var
		shell, mocks := setup(t)
		var capturedEnv []string
		mocks.Shims.CmdRun = func(cmd *exec.Cmd) error {
			capturedEnv = cmd.Env
			return nil
		}

		// When executing with an extra env var
		_, _ = shell.ExecSilentWithEnv("test", map[string]string{"CUSTOM_VAR": "custom_value"}, "arg")

		// Then the env var is present in the command environment
		found := false
		for _, e := range capturedEnv {
			if e == "CUSTOM_VAR=custom_value" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected CUSTOM_VAR=custom_value to be in cmd.Env")
		}
	})

	t.Run("VerboseMode", func(t *testing.T) {
		// Given a shell with verbose mode enabled
		shell, mocks := setup(t)
		shell.SetVerbosity(true)

		// Mock CmdRun to write output to cmd.Stdout
		mocks.Shims.CmdRun = func(cmd *exec.Cmd) error {
			if cmd.Stdout != nil {
				if _, err := cmd.Stdout.Write([]byte("test\n")); err != nil {
					return fmt.Errorf("failed to write: %v", err)
				}
			}
			return nil
		}

		// Capture os.Stdout to avoid test noise
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// When executing with env vars in verbose mode
		out, err := shell.ExecSilentWithEnv("test", map[string]string{"FOO": "bar"}, "arg")

		w.Close()
		os.Stdout = oldStdout
		io.ReadAll(r)

		// Then it succeeds and returns the captured output
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if out != "test\n" {
			t.Errorf("Expected output 'test\n', got '%s'", out)
		}
	})

	t.Run("VerboseModeScrubbingSecrets", func(t *testing.T) {
		// Given a shell with verbose mode and a registered secret
		shell, mocks := setup(t)
		shell.SetVerbosity(true)
		shell.RegisterSecret("s3cr3t123")

		// Mock CmdRun to write a line containing the secret
		mocks.Shims.CmdRun = func(cmd *exec.Cmd) error {
			if cmd.Stdout != nil {
				if _, err := cmd.Stdout.Write([]byte("output with s3cr3t123 inside\n")); err != nil {
					return fmt.Errorf("failed to write: %v", err)
				}
			}
			return nil
		}

		// Capture what gets written to os.Stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// When executing in verbose mode
		out, err := shell.ExecSilentWithEnv("test", nil, "arg")

		w.Close()
		os.Stdout = oldStdout
		terminalOutput, _ := io.ReadAll(r)

		// Then no error occurs
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// And the secret is redacted in the return value
		if strings.Contains(out, "s3cr3t123") {
			t.Errorf("Expected secret to be redacted in return value, got %q", out)
		}
		// And the secret is redacted in terminal output
		if strings.Contains(string(terminalOutput), "s3cr3t123") {
			t.Errorf("Expected secret to be redacted in terminal output, got %q", string(terminalOutput))
		}
	})
}

func TestShell_ExecCaptureWithEnv(t *testing.T) {
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

		// When capturing a command's output
		out, err := shell.ExecCaptureWithEnv("test", map[string]string{"FOO": "bar"}, "arg")

		// Then it succeeds and returns the captured output
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if out != "test\n" {
			t.Errorf("Expected output 'test\n', got '%s'", out)
		}
	})

	t.Run("NeverStreamsEvenInVerboseMode", func(t *testing.T) {
		// Given a shell in verbose mode whose command emits sensitive machine output
		shell, mocks := setup(t)
		shell.SetVerbosity(true)
		mocks.Shims.CmdRun = func(cmd *exec.Cmd) error {
			if cmd.Stdout != nil {
				if _, err := cmd.Stdout.Write([]byte("cluster-pki-secret\n")); err != nil {
					return fmt.Errorf("failed to write: %v", err)
				}
			}
			return nil
		}

		// Capture what reaches os.Stdout
		oldStdout := os.Stdout
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("failed to create pipe: %v", err)
		}
		os.Stdout = w

		// When capturing the output in verbose mode
		out, err := shell.ExecCaptureWithEnv("test", nil, "arg")

		w.Close()
		os.Stdout = oldStdout
		streamed, _ := io.ReadAll(r)

		// Then the output is returned to the caller but never echoed to the terminal
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if out != "cluster-pki-secret\n" {
			t.Errorf("Expected captured output, got %q", out)
		}
		if strings.Contains(string(streamed), "cluster-pki-secret") {
			t.Errorf("Expected nothing streamed to stdout in verbose mode, got %q", string(streamed))
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a shell whose CmdRun returns an error
		shell, mocks := setup(t)
		mocks.Shims.CmdRun = func(cmd *exec.Cmd) error {
			return fmt.Errorf("command failed")
		}

		// When capturing output
		out, err := shell.ExecCaptureWithEnv("test", nil, "arg")

		// Then an error is returned and output is empty
		if err == nil || !strings.Contains(err.Error(), "command failed") {
			t.Errorf("Expected 'command failed' error, got %v", err)
		}
		if out != "" {
			t.Errorf("Expected empty output, got %q", out)
		}
	})

	t.Run("CommandNil", func(t *testing.T) {
		// Given a shell whose Command shim returns nil
		shell, mocks := setup(t)
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return nil
		}

		// When capturing output
		output, err := shell.ExecCaptureWithEnv("test", nil, "arg")

		// Then an error about command creation is returned
		if err == nil || !strings.Contains(err.Error(), "failed to create command") {
			t.Errorf("Expected error about command creation, got: %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
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
		var output string
		var err error
		stderr := captureStderr(t, func() {
			output, err = shell.ExecProgress(message, "test")
		})

		// Then it should print the message, succeed, and return expected output
		if !strings.Contains(stderr, message) {
			t.Errorf("Expected stderr to contain %q, got %q", message, stderr)
		}
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

func TestShell_ExecProgressWithEnv_InterruptGuard(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("RegistersAndStopsAnInterruptGuardAroundTheCommand", func(t *testing.T) {
		// Given a shell whose SignalNotify/SignalStop calls are observed
		shell, mocks := setup(t)

		var notifiedSignals []os.Signal
		var notifyCh chan<- os.Signal
		stopped := false
		mocks.Shims.SignalNotify = func(c chan<- os.Signal, sig ...os.Signal) {
			notifyCh = c
			notifiedSignals = sig
		}
		mocks.Shims.SignalStop = func(c chan<- os.Signal) {
			if c == notifyCh {
				stopped = true
			}
		}

		// When executing a command with progress
		if _, err := shell.ExecProgressWithEnv("Applying test", "test", nil); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the guard registered for os.Interrupt and stopped once the command finished
		if len(notifiedSignals) != 1 || notifiedSignals[0] != os.Interrupt {
			t.Errorf("expected SignalNotify to be called with os.Interrupt, got %v", notifiedSignals)
		}
		if !stopped {
			t.Error("expected SignalStop to be called with the same channel passed to SignalNotify")
		}
	})

	t.Run("PlacesTheCommandInItsOwnProcessGroup", func(t *testing.T) {
		// Given a shell that captures the command built for execution
		shell, mocks := setup(t)

		var capturedCmd *exec.Cmd
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			capturedCmd = exec.Command("test")
			return capturedCmd
		}

		// When executing a command with progress
		if _, err := shell.ExecProgressWithEnv("Applying test", "test", nil); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then setProcessGroup should have set SysProcAttr, isolating it from a repeat
		// terminal interrupt so only the guard's forwarded signal reaches it
		if capturedCmd.SysProcAttr == nil {
			t.Error("expected SysProcAttr to be set so the command runs in its own process group")
		}
	})

	t.Run("VerboseModeAlsoRegistersAndStopsTheGuard", func(t *testing.T) {
		// Given a verbose shell whose SignalNotify/SignalStop calls are observed
		shell, mocks := setup(t)
		shell.SetVerbosity(true)

		notifyCalls := 0
		stopCalls := 0
		mocks.Shims.SignalNotify = func(c chan<- os.Signal, sig ...os.Signal) { notifyCalls++ }
		mocks.Shims.SignalStop = func(c chan<- os.Signal) { stopCalls++ }
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			cmd := exec.Command("test")
			cmd.Stdout = new(bytes.Buffer)
			cmd.Stderr = new(bytes.Buffer)
			return cmd
		}

		// When executing a command with progress in verbose mode
		if _, err := shell.ExecProgressWithEnv("Applying test", "test", nil); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the guard should still be registered and stopped exactly once
		if notifyCalls != 1 || stopCalls != 1 {
			t.Errorf("expected exactly one SignalNotify and one SignalStop call, got %d and %d", notifyCalls, stopCalls)
		}
	})

	t.Run("SkipsTheGuardForSudo", func(t *testing.T) {
		// Setpgid would move a sudo child out of the terminal's foreground group.
		// Its password prompt would then stall on SIGTTIN. The guard must not run.
		shell, mocks := setup(t)

		notifyCalls := 0
		mocks.Shims.SignalNotify = func(c chan<- os.Signal, sig ...os.Signal) { notifyCalls++ }
		var capturedCmd *exec.Cmd
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			capturedCmd = exec.Command("sudo")
			return capturedCmd
		}

		if _, err := shell.ExecProgressWithEnv("Applying test", "sudo", nil); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if notifyCalls != 0 {
			t.Errorf("expected the interrupt guard not to register for sudo, got %d SignalNotify calls", notifyCalls)
		}
		if capturedCmd.SysProcAttr != nil {
			t.Error("expected sudo's process group to be left untouched")
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestInterruptGuard_Watch(t *testing.T) {
	setup := func() (*interruptGuard, *syncWriter, func() int) {
		warn := newSyncWriter()
		var mu sync.Mutex
		forwardCount := 0
		g := &interruptGuard{
			forward: func() error {
				mu.Lock()
				forwardCount++
				mu.Unlock()
				return nil
			},
			warn:      warn,
			component: "cluster/aws-eks",
		}
		count := func() int {
			mu.Lock()
			defer mu.Unlock()
			return forwardCount
		}
		return g, warn, count
	}

	runWatch := func(t *testing.T, g *interruptGuard, sigCh chan os.Signal, done chan struct{}) <-chan struct{} {
		t.Helper()
		finished := make(chan struct{})
		go func() {
			g.watch(sigCh, done)
			close(finished)
		}()
		return finished
	}

	waitFinished := func(t *testing.T, finished <-chan struct{}) {
		t.Helper()
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatal("watch did not return after done was closed")
		}
	}

	t.Run("ReturnsWithoutForwardingWhenNoSignalArrives", func(t *testing.T) {
		g, warn, count := setup()
		sigCh := make(chan os.Signal, 1)
		done := make(chan struct{})

		finished := runWatch(t, g, sigCh, done)
		close(done)
		waitFinished(t, finished)

		if count() != 0 {
			t.Errorf("expected forward not to be called, got %d calls", count())
		}
		if warn.String() != "" {
			t.Errorf("expected no warning output, got %q", warn.String())
		}
	})

	t.Run("ForwardsOnlyTheFirstOfSeveralSignals", func(t *testing.T) {
		g, warn, count := setup()
		sigCh := make(chan os.Signal, 4)
		done := make(chan struct{})

		finished := runWatch(t, g, sigCh, done)

		sigCh <- os.Interrupt
		<-warn.written
		sigCh <- os.Interrupt
		<-warn.written
		sigCh <- os.Interrupt
		<-warn.written

		close(done)
		waitFinished(t, finished)

		if count() != 1 {
			t.Errorf("expected forward to be called exactly once, got %d", count())
		}
		lines := strings.Count(warn.String(), "\n")
		if lines != 3 {
			t.Errorf("expected one warning line per signal (3), got %d in %q", lines, warn.String())
		}
		if !strings.Contains(warn.String(), "cluster/aws-eks") {
			t.Errorf("expected warning to mention the component, got %q", warn.String())
		}
	})

	t.Run("SecondWarningIsDistinctFromTheFirst", func(t *testing.T) {
		g, warn, _ := setup()
		sigCh := make(chan os.Signal, 2)
		done := make(chan struct{})

		finished := runWatch(t, g, sigCh, done)

		sigCh <- os.Interrupt
		<-warn.written
		sigCh <- os.Interrupt
		<-warn.written

		close(done)
		waitFinished(t, finished)

		lines := strings.Split(strings.TrimRight(warn.String(), "\n"), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected exactly two warning lines, got %d: %q", len(lines), warn.String())
		}
		if lines[0] == lines[1] {
			t.Errorf("expected a distinct follow-up warning, got the same line twice: %q", lines[0])
		}
		if !strings.Contains(lines[1], "corrupt") {
			t.Errorf("expected the second warning to warn that forcing can corrupt state, got %q", lines[1])
		}
	})

	t.Run("ReturnsPromptlyOnceDoneIsClosed", func(t *testing.T) {
		g, _, _ := setup()
		sigCh := make(chan os.Signal, 1)
		done := make(chan struct{})

		finished := runWatch(t, g, sigCh, done)
		close(done)
		waitFinished(t, finished)
	})

	t.Run("HandsTheTerminalOffAroundEachWarning", func(t *testing.T) {
		// The console redraws its live region on a timer. An unpaused write here would
		// get drawn over by the next frame. Each warning must hand off, then take back.
		var mu sync.Mutex
		var events []string
		presenter.SetDefault(&handoffRecorder{record: func(e string) {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		}})
		t.Cleanup(func() { presenter.SetDefault(nil) })

		warn := newSyncWriter()
		warn.onWrite = func() {
			mu.Lock()
			events = append(events, "write")
			mu.Unlock()
		}
		g := &interruptGuard{
			forward:   func() error { return nil },
			warn:      warn,
			component: "cluster/aws-eks",
		}
		sigCh := make(chan os.Signal, 1)
		done := make(chan struct{})

		finished := runWatch(t, g, sigCh, done)
		sigCh <- os.Interrupt
		<-warn.written
		close(done)
		waitFinished(t, finished)

		mu.Lock()
		got := append([]string(nil), events...)
		mu.Unlock()
		want := []string{"pause", "write", "resume"}
		if !slices.Equal(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})
}

func Test_execError(t *testing.T) {
	cause := errors.New("exit status 1")

	t.Run("KeepsStderrAfterTheCause", func(t *testing.T) {
		// Given a failed command that wrote to stderr
		// When execError wraps the failure
		err := execError(cause, "permission denied")

		// Then the text and the trace keep the stderr, and the cause stays in the chain
		if err.Error() != "command execution failed: exit status 1\npermission denied" {
			t.Errorf("Expected the cause then the stderr, got %q", err.Error())
		}
		if !errors.Is(err, cause) {
			t.Error("Expected the cause to stay in the error chain")
		}
		crumbs := werror.Breadcrumbs(err)
		if len(crumbs) == 0 || !strings.Contains(crumbs[len(crumbs)-1], "permission denied") {
			t.Errorf("Expected the trace to show the stderr, got %v", crumbs)
		}
	})

	t.Run("OmitsEmptyStderr", func(t *testing.T) {
		// Given a failed command that wrote nothing to stderr
		// When execError wraps the failure
		err := execError(cause, "")

		// Then the text ends with the cause
		if err.Error() != "command execution failed: exit status 1" {
			t.Errorf("Expected no trailing stderr, got %q", err.Error())
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

// handoffRecorder is a presenter that records Pause and Resume calls.
type handoffRecorder struct {
	record func(string)
}

func (h *handoffRecorder) Emit(context.Context, presenter.Event) {}

func (h *handoffRecorder) Pause() { h.record("pause") }

func (h *handoffRecorder) Resume() { h.record("resume") }
