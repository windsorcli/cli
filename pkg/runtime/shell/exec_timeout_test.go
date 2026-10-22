package shell

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// Test Public Methods
// =============================================================================

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

func TestShell_ExecSilentWithEnvAndTimeout(t *testing.T) {
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

		// When executing a command with env vars and a timeout
		out, err := shell.ExecSilentWithEnvAndTimeout("test", map[string]string{"FOO": "bar"}, []string{"arg"}, 5*time.Second)

		// Then it succeeds and returns the captured output
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if out != "test\n" {
			t.Errorf("Expected output 'test\\n', got %q", out)
		}
	})

	t.Run("MergesEnvVars", func(t *testing.T) {
		// Given a shell that captures cmd.Env on run
		shell, mocks := setup(t)
		var capturedEnv []string
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			capturedEnv = cmd.Env
			return nil
		}
		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			return nil
		}

		// When executing with extra env vars
		_, _ = shell.ExecSilentWithEnvAndTimeout("test", map[string]string{"AWS_PROFILE": "ctx"}, []string{"arg"}, 5*time.Second)

		// Then the extra env var is present in the merged environment — confirming the env
		// override actually reaches the child process rather than being silently dropped
		found := false
		for _, e := range capturedEnv {
			if e == "AWS_PROFILE=ctx" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected AWS_PROFILE=ctx in cmd.Env, got %v", capturedEnv)
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		// Given a CmdWait that sleeps past the timeout
		shell, mocks := setup(t)
		mocks.Shims.CmdStart = func(cmd *exec.Cmd) error {
			return nil
		}
		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			time.Sleep(200 * time.Millisecond)
			return nil
		}

		// When executing with a short timeout
		out, err := shell.ExecSilentWithEnvAndTimeout("test", nil, []string{"arg"}, 50*time.Millisecond)

		// Then a timeout error is surfaced — the env-aware variant must inherit the same
		// process-killing safety net as ExecSilentWithTimeout, otherwise hung commands would
		// block bootstrap indefinitely
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Errorf("Expected timeout error, got: %v", err)
		}
		if out != "" {
			t.Errorf("Expected empty output on timeout, got %q", out)
		}
	})

	t.Run("CommandNil", func(t *testing.T) {
		// Given a Command shim that returns nil
		shell, mocks := setup(t)
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return nil
		}

		// When executing
		output, err := shell.ExecSilentWithEnvAndTimeout("test", nil, []string{"arg"}, 5*time.Second)

		// Then a command-creation error is returned
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
}

func TestShell_ExecSilentWithEnvAndIdleTimeout(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a command that writes output and exits cleanly well within both timeouts
		shell, mocks := setup(t)
		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				w.Write([]byte("test output\n"))
				w.Close()
			}()
			return r, nil
		}
		mocks.Shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			w.Close()
			return r, nil
		}

		// When executing with generous idle and absolute timeouts
		out, err := shell.ExecSilentWithEnvAndIdleTimeout("test", map[string]string{"FOO": "bar"}, []string{"arg"}, 5*time.Second, time.Hour, time.Second)

		// Then it succeeds and returns the captured stdout
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if out != "test output\n" {
			t.Errorf("Expected output 'test output\\n', got %q", out)
		}
	})

	t.Run("IdleTimeoutFiresOnSustainedSilenceAndHardKillsAnUnresponsiveProcess", func(t *testing.T) {
		// Given a process that never writes anything and ignores the interrupt, bounded so
		// the test itself does not hang
		shell, mocks := setup(t)
		stdoutR, stdoutW := io.Pipe()
		stderrR, stderrW := io.Pipe()
		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) { return stdoutR, nil }
		mocks.Shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) { return stderrR, nil }
		var interrupted, killed bool
		mocks.Shims.InterruptProcessGroup = func(cmd *exec.Cmd) error {
			interrupted = true
			return nil
		}
		mocks.Shims.KillProcessGroup = func(cmd *exec.Cmd) error {
			killed = true
			_ = stdoutW.Close()
			_ = stderrW.Close()
			return nil
		}

		// When executing with a short idle timeout, a long absolute timeout, and a short grace period
		out, err := shell.ExecSilentWithEnvAndIdleTimeout("test", nil, []string{"arg"}, 50*time.Millisecond, time.Hour, 50*time.Millisecond)

		// Then the idle timeout fires, naming the silence, only after the process group was
		// interrupted first; the eventual hard kill also targets the whole group, not just
		// the terraform process itself, so a still-running provider-plugin subprocess does
		// not survive orphaned
		if err == nil || !errors.Is(err, ErrCommandTimedOut) || !strings.Contains(err.Error(), "no output for") {
			t.Errorf("Expected an idle-timeout error, got %v", err)
		}
		if out != "" {
			t.Errorf("Expected empty output on timeout, got %q", out)
		}
		if !interrupted {
			t.Error("Expected the process group to be interrupted before the timeout was reported")
		}
		if !killed {
			t.Error("Expected the process group to be hard-killed after the grace period elapsed")
		}
	})

	t.Run("ActivityResetsIdleTimeoutSoASlowButProgressingCommandSucceeds", func(t *testing.T) {
		// Given a command that writes a line every 150ms for 750ms total — longer than the
		// idle timeout, but never silent for that long at a stretch. The gap between writes
		// stays a small fraction of the idle timeout so scheduler jitter on a loaded CI
		// runner cannot make a single gap look like a stall.
		shell, mocks := setup(t)
		stdoutR, stdoutW := io.Pipe()
		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) { return stdoutR, nil }
		mocks.Shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			w.Close()
			return r, nil
		}
		go func() {
			for i := range 5 {
				time.Sleep(150 * time.Millisecond)
				fmt.Fprintf(stdoutW, "line %d\n", i)
			}
			stdoutW.Close()
		}()

		// When executing with an idle timeout shorter than the command's total runtime
		out, err := shell.ExecSilentWithEnvAndIdleTimeout("test", nil, []string{"arg"}, 500*time.Millisecond, time.Hour, time.Second)

		// Then it succeeds: each line resets the idle window before it can expire
		if err != nil {
			t.Errorf("Expected the periodic output to keep resetting the idle timeout, got: %v", err)
		}
		if !strings.Contains(out, "line 4") {
			t.Errorf("Expected every line to be captured, got: %q", out)
		}
	})

	t.Run("AbsoluteTimeoutFiresDespiteOngoingActivity", func(t *testing.T) {
		// Given a command that keeps writing often enough to never trip the idle timeout, but
		// runs longer than the absolute backstop
		shell, mocks := setup(t)
		stdoutR, stdoutW := io.Pipe()
		stderrR, stderrW := io.Pipe()
		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) { return stdoutR, nil }
		mocks.Shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) { return stderrR, nil }
		var interrupted, killed bool
		mocks.Shims.InterruptProcessGroup = func(cmd *exec.Cmd) error {
			interrupted = true
			return nil
		}
		mocks.Shims.KillProcessGroup = func(cmd *exec.Cmd) error {
			killed = true
			_ = stdoutW.Close()
			_ = stderrW.Close()
			return nil
		}
		stop := make(chan struct{})
		defer close(stop)
		go func() {
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					fmt.Fprintln(stdoutW, "still going")
				}
			}
		}()

		// When executing with an idle timeout the activity never lets expire, and a short
		// absolute timeout
		out, err := shell.ExecSilentWithEnvAndIdleTimeout("test", nil, []string{"arg"}, time.Hour, 100*time.Millisecond, 50*time.Millisecond)

		// Then the absolute backstop fires anyway, and still interrupts before it kills,
		// surfacing whatever output was captured before the cutoff
		if err == nil || !errors.Is(err, ErrCommandTimedOut) || !strings.Contains(err.Error(), "ran for longer than 100ms") {
			t.Errorf("Expected an absolute-timeout error, got %v", err)
		}
		if !strings.Contains(out, "still going") {
			t.Errorf("Expected the captured output up to the cutoff, got %q", out)
		}
		if !interrupted {
			t.Error("Expected the process group to be interrupted before the timeout was reported")
		}
		if !killed {
			t.Error("Expected the process group to be hard-killed after the grace period elapsed")
		}
	})

	t.Run("ReturnsPromptlyWhenProcessExitsWithinGracePeriod", func(t *testing.T) {
		// Given a process that exits shortly after being interrupted — honoring it cleanly,
		// well inside a generous grace period
		shell, mocks := setup(t)
		stdoutR, stdoutW := io.Pipe()
		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) { return stdoutR, nil }
		mocks.Shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			w.Close()
			return r, nil
		}
		mocks.Shims.InterruptProcessGroup = func(cmd *exec.Cmd) error {
			go func() {
				time.Sleep(20 * time.Millisecond)
				_ = stdoutW.Close()
			}()
			return nil
		}

		// When executing with a short idle timeout and a very long grace period
		start := time.Now()
		_, err := shell.ExecSilentWithEnvAndIdleTimeout("test", nil, []string{"arg"}, 50*time.Millisecond, time.Hour, time.Hour)
		elapsed := time.Since(start)

		// Then the call returns once the process exits, not after the full (very long) grace period
		if err == nil || !errors.Is(err, ErrCommandTimedOut) || !strings.Contains(err.Error(), "no output for") {
			t.Errorf("Expected an idle-timeout error, got %v", err)
		}
		if elapsed > 500*time.Millisecond {
			t.Errorf("Expected cleanup to return once the process exited, not wait out the grace period, took %v", elapsed)
		}
	})

	t.Run("CommandNil", func(t *testing.T) {
		// Given a Command shim that returns nil
		shell, mocks := setup(t)
		mocks.Shims.Command = func(name string, args ...string) *exec.Cmd {
			return nil
		}

		// When executing
		output, err := shell.ExecSilentWithEnvAndIdleTimeout("test", nil, []string{"arg"}, 5*time.Second, time.Hour, time.Second)

		// Then a command-creation error is returned
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
		// Given a StdoutPipe shim that fails
		shell, mocks := setup(t)
		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			return nil, fmt.Errorf("boom")
		}

		// When executing
		output, err := shell.ExecSilentWithEnvAndIdleTimeout("test", nil, []string{"arg"}, 5*time.Second, time.Hour, time.Second)

		// Then the pipe error is returned directly
		if err == nil || !strings.Contains(err.Error(), "boom") {
			t.Errorf("Expected the stdout pipe error, got %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
	})

	t.Run("StderrPipeError", func(t *testing.T) {
		// Given a StderrPipe shim that fails
		shell, mocks := setup(t)
		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			w.Close()
			return r, nil
		}
		mocks.Shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			return nil, fmt.Errorf("boom")
		}

		// When executing
		output, err := shell.ExecSilentWithEnvAndIdleTimeout("test", nil, []string{"arg"}, 5*time.Second, time.Hour, time.Second)

		// Then the pipe error is returned directly
		if err == nil || !strings.Contains(err.Error(), "boom") {
			t.Errorf("Expected the stderr pipe error, got %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
	})

	t.Run("StdoutScannerErrorIsNotSwallowed", func(t *testing.T) {
		// Given a stdout stream that closes cleanly but whose scanner reports an error
		// (e.g. a line past the buffer's cap) — a still-running process must not be
		// mistaken for a clean, silent exit
		shell, mocks := setup(t)
		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() { w.Close() }()
			return r, nil
		}
		mocks.Shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() { w.Close() }()
			return r, nil
		}
		mocks.Shims.ScannerErr = func(scanner *bufio.Scanner) error {
			return fmt.Errorf("token too long")
		}

		// When executing
		output, err := shell.ExecSilentWithEnvAndIdleTimeout("test", nil, []string{"arg"}, 5*time.Second, time.Hour, time.Second)

		// Then the scan error surfaces rather than being reported as a clean delete
		if err == nil || !strings.Contains(err.Error(), "token too long") {
			t.Errorf("Expected the scanner error to surface, got %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
	})

	t.Run("CommandFailureSurfacesStderr", func(t *testing.T) {
		// Given a command that exits non-zero after writing to both streams
		shell, mocks := setup(t)
		stdoutR, stdoutW := io.Pipe()
		stderrR, stderrW := io.Pipe()
		mocks.Shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) { return stdoutR, nil }
		mocks.Shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) { return stderrR, nil }
		go func() {
			stdoutW.Write([]byte("out line\n"))
			stdoutW.Close()
		}()
		go func() {
			stderrW.Write([]byte("boom detail\n"))
			stderrW.Close()
		}()
		mocks.Shims.CmdWait = func(cmd *exec.Cmd) error {
			return fmt.Errorf("exit status 1")
		}

		// When executing
		out, err := shell.ExecSilentWithEnvAndIdleTimeout("test", nil, []string{"arg"}, time.Second, time.Hour, time.Second)

		// Then the failure wraps CmdWait's error and surfaces captured stderr
		if err == nil || !strings.Contains(err.Error(), "exit status 1") || !strings.Contains(err.Error(), "boom detail") {
			t.Errorf("Expected a wrapped failure naming the exit error and stderr, got %v", err)
		}
		if out != "out line\n" {
			t.Errorf("Expected the captured stdout, got %q", out)
		}
	})
}
