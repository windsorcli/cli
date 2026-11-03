//go:build darwin || linux
// +build darwin linux

package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// =============================================================================
// Test Public Methods
// =============================================================================

// TestDefaultShell_renderEnvVarsWithExport tests the renderEnvVarsWithExport method on Unix systems
func TestDefaultShell_renderEnvVarsWithExport(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("RendersEnvVarsWithExport", func(t *testing.T) {
		// Given a shell with environment variables
		shell, _ := setup(t)
		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
			"VAR3": "",
		}

		// When rendering environment variables with export
		result := shell.renderEnvVarsWithExport(envVars)

		// Then the output should contain sorted export commands
		if !strings.Contains(result, "export VAR1=\"value1\"\n") {
			t.Errorf("Expected export VAR1, got: %s", result)
		}
		if !strings.Contains(result, "export VAR2=\"value2\"\n") {
			t.Errorf("Expected export VAR2, got: %s", result)
		}
		if !strings.Contains(result, "unset VAR3\n") {
			t.Errorf("Expected unset VAR3, got: %s", result)
		}
	})

	t.Run("RendersEmptyEnvVars", func(t *testing.T) {
		// Given a shell with empty environment variables map
		shell, _ := setup(t)
		envVars := map[string]string{}

		// When rendering environment variables with export
		result := shell.renderEnvVarsWithExport(envVars)

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
		}

		// When rendering environment variables with export
		result := shell.renderEnvVarsWithExport(envVars)

		// Then values with special characters should be quoted with single quotes
		if !strings.Contains(result, "export VAR1='[\"item1\",\"item2\"]'\n") {
			t.Errorf("Expected VAR1 to be quoted, got: %s", result)
		}
		if !strings.Contains(result, "export VAR2='{\"key\": \"value\"}'\n") {
			t.Errorf("Expected VAR2 to be quoted, got: %s", result)
		}
		if !strings.Contains(result, "export VAR3=\"simple_value\"\n") {
			t.Errorf("Expected VAR3 to use double quotes, got: %s", result)
		}
		if !strings.Contains(result, "export VAR4='value with spaces'\n") {
			t.Errorf("Expected VAR4 to be quoted, got: %s", result)
		}
	})
}

// TestSetProcessGroup_InterruptProcessGroup verifies the real syscalls behind the
// interrupt guard. A command in its own process group sits outside the test
// process's signal delivery. interruptProcessGroup can still reach and terminate it.
func TestSetProcessGroup_InterruptProcessGroup(t *testing.T) {
	t.Run("PlacesTheChildInADifferentProcessGroupFromTheTestProcess", func(t *testing.T) {
		cmd := exec.Command("sleep", "5")
		setProcessGroup(cmd)

		if err := cmd.Start(); err != nil {
			t.Fatalf("failed to start test process: %v", err)
		}
		defer func() { _ = cmd.Process.Kill() }()

		ownPgid, err := syscall.Getpgid(os.Getpid())
		if err != nil {
			t.Fatalf("failed to get own pgid: %v", err)
		}
		childPgid, err := syscall.Getpgid(cmd.Process.Pid)
		if err != nil {
			t.Fatalf("failed to get child pgid: %v", err)
		}
		if childPgid == ownPgid {
			t.Errorf("expected child to be in its own process group, got the test process's group %d", ownPgid)
		}
	})

	t.Run("TerminatesTheChildEvenThoughItIsInADifferentProcessGroup", func(t *testing.T) {
		cmd := exec.Command("sleep", "5")
		setProcessGroup(cmd)

		if err := cmd.Start(); err != nil {
			t.Fatalf("failed to start test process: %v", err)
		}

		if err := interruptProcessGroup(cmd); err != nil {
			_ = cmd.Process.Kill()
			t.Fatalf("interruptProcessGroup returned an error: %v", err)
		}

		waitErr := make(chan error, 1)
		go func() { waitErr <- cmd.Wait() }()

		select {
		case err := <-waitErr:
			if err == nil {
				t.Error("expected the child to exit with an error from the delivered signal, got nil")
			}
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill()
			t.Fatal("child did not exit after interruptProcessGroup")
		}
	})

	t.Run("NoOpWhenTheProcessHasNotStarted", func(t *testing.T) {
		cmd := exec.Command("sleep", "5")
		if err := interruptProcessGroup(cmd); err != nil {
			t.Errorf("expected no error for an unstarted process, got %v", err)
		}
	})
}

func TestKillProcessGroup(t *testing.T) {
	t.Run("KillsAGrandchildTheChildSpawnedIntoTheSameGroup", func(t *testing.T) {
		// A single-process kill only reaches cmd's own pid. That leaves a subprocess it forked
		// (terraform's own provider plugins, in production) orphaned and still running. The
		// shell here forks a grandchild sleep, in the same process group by inheritance, and
		// writes its pid so the test can confirm killProcessGroup reaches it too.
		pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
		cmd := exec.Command("sh", "-c", "sleep 30 & echo $! > "+pidFile+"; wait")
		setProcessGroup(cmd)

		if err := cmd.Start(); err != nil {
			t.Fatalf("failed to start test process: %v", err)
		}
		defer func() { _ = cmd.Wait() }()

		var grandchildPid int
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			data, err := os.ReadFile(pidFile)
			if err == nil {
				if pid, convErr := strconv.Atoi(strings.TrimSpace(string(data))); convErr == nil && pid > 0 {
					grandchildPid = pid
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
		if grandchildPid == 0 {
			_ = cmd.Process.Kill()
			t.Fatal("grandchild pid was never written")
		}

		if err := killProcessGroup(cmd); err != nil {
			_ = cmd.Process.Kill()
			_ = syscall.Kill(grandchildPid, syscall.SIGKILL)
			t.Fatalf("killProcessGroup returned an error: %v", err)
		}

		deadline = time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if err := syscall.Kill(grandchildPid, 0); err != nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		_ = syscall.Kill(grandchildPid, syscall.SIGKILL)
		t.Error("expected the grandchild to be killed along with the group, but it is still alive")
	})

	t.Run("NoOpWhenTheProcessHasNotStarted", func(t *testing.T) {
		cmd := exec.Command("sleep", "5")
		if err := killProcessGroup(cmd); err != nil {
			t.Errorf("expected no error for an unstarted process, got %v", err)
		}
	})
}
