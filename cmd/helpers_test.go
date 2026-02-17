//go:build integration
// +build integration

package cmd

// Test helpers for the cmd package. Built with -tags=integration for integration test helpers
// (SetupIntegrationProject, runCmd, etc.); unit tests use captureOutput/setupMocks from root_test.go.

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

const minimalWindsorYAML = `version: v1alpha1
contexts:
  local: {}
`

// =============================================================================
// Test Helpers
// =============================================================================

// SetupIntegrationProject suppresses subprocess stdout and stderr for the test (so init/OCI/progress
// messages do not appear), creates a temp directory with the given windsor.yaml content, changes the
// process working directory to that directory, and registers a cleanup to restore the original working
// directory. The runtime's GetProjectRoot() will resolve to this directory when commands run without
// a runtime override. Returns the project root path.
func SetupIntegrationProject(t *testing.T, windsorYAML string) string {
	t.Helper()
	suppressProcessStdout(t)
	suppressProcessStderr(t)
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "windsor.yaml"), []byte(windsorYAML), 0600); err != nil {
		t.Fatalf("Failed to write windsor.yaml: %v", err)
	}
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to chdir to project: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(origWd); err != nil {
			t.Logf("Warning: failed to restore working directory: %v", err)
		}
	})
	return tmpDir
}

func captureOutputAndRestore(t *testing.T) (stdout, stderr *bytes.Buffer) {
	t.Helper()
	stdout = new(bytes.Buffer)
	stderr = new(bytes.Buffer)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})
	return stdout, stderr
}

// suppressProcessStdout redirects os.Stdout to a pipe drained to io.Discard for the
// duration of the test so that commands using fmt.Printf do not pollute the terminal. Restores on t.Cleanup.
func suppressProcessStdout(t *testing.T) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	t.Cleanup(func() {
		w.Close()
		io.Copy(io.Discard, r)
		os.Stdout = orig
	})
}

// suppressProcessStderr redirects os.Stderr to a pipe drained to io.Discard for the
// duration of the test so that init/OCI/progress messages do not pollute the terminal. Restores on t.Cleanup.
func suppressProcessStderr(t *testing.T) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	t.Cleanup(func() {
		w.Close()
		io.Copy(io.Discard, r)
		os.Stderr = orig
	})
}

func captureProcessStdout(t *testing.T) (buf *bytes.Buffer, restore func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	buf = new(bytes.Buffer)
	restore = func() {
		w.Close()
		io.Copy(buf, r)
		os.Stdout = orig
	}
	return buf, restore
}

// resetCommandFlagValues resets flag values to DefValue on cmd and all descendants so Cobra flag state does not leak between Execute() calls (see cobra issue #2079).
func resetCommandFlagValues(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
	})
	for _, child := range cmd.Commands() {
		resetCommandFlagValues(child)
	}
}

// runCmd runs the root command with the given context and args, capturing process stdout and cmd stderr.
// Pass context.Background() when no override is needed; pass a context with runtimeOverridesKey to inject a runtime.
// Caller must have set up the integration project first (e.g. SetupIntegrationProject).
// Returns trimmed stdout string, stderr string, and Execute() error.
func runCmd(t *testing.T, ctx context.Context, args []string) (stdout, stderr string, err error) {
	t.Helper()
	resetCommandFlagValues(rootCmd)
	_, stderrBuf := captureOutputAndRestore(t)
	stdoutBuf, restore := captureProcessStdout(t)
	rootCmd.SetContext(ctx)
	rootCmd.SetArgs(args)
	err = Execute()
	restore()
	return strings.TrimSpace(stdoutBuf.String()), stderrBuf.String(), err
}

func assertSuccessAndNoStderr(t *testing.T, err error, stderr string) {
	t.Helper()
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}
	if stderr != "" {
		t.Errorf("Expected empty stderr, got %q", stderr)
	}
}

// assertFailureAndErrorContains fails the test if err is nil or err.Error() does not contain the substring.
func assertFailureAndErrorContains(t *testing.T, err error, substring string) {
	t.Helper()
	if err == nil {
		t.Error("Expected command to fail")
		return
	}
	if substring != "" && !strings.Contains(err.Error(), substring) {
		t.Errorf("Expected error to contain %q, got %q", substring, err.Error())
	}
}

// =============================================================================
// Shell capture (integration tests)
// =============================================================================
//
// Integration tests can use the shell in three ways:
//
//  1. Real shell (default): Do not pass a project/runtime override. runCmd(t, context.Background(), args)
//     uses the real project and shell, so terraform, docker, init, etc. run for real.
//
//  2. Full mock: Use NewMockShellWithCapture(capture) and inject it via a project override. Every
//     Exec/ExecSudo/ExecSilent/ExecSilentWithTimeout is captured and no-opped. Use when the test
//     must not run any subprocesses (e.g. configure network tests that must not touch routes/DNS).
//
//  3. Partial mock (real + capture): Use NewMockShellWithPartialCapture(realShell, capture). Returns
//     a MockShell that delegates to realShell for all calls except ExecSudo and ExecSilentWithTimeout,
//     which are captured and no-opped. Use when you want real init/terraform/docker but no sudo or
//     colima-ssh (e.g. avoid real network config while still running real CLI flows).

// ShellCall records a single Exec, ExecSudo, or ExecSilent invocation (command + args).
type ShellCall struct {
	Command string
	Args    []string
}

// ShellCallWithTimeout records a single ExecSilentWithTimeout invocation.
type ShellCallWithTimeout struct {
	Command string
	Args    []string
	Timeout time.Duration
}

// ShellCapture holds slices of captured shell invocations for assertion in tests.
// Create with NewShellCapture and pass to NewMockShellWithCapture to wire a MockShell to it.
type ShellCapture struct {
	ExecCalls              []ShellCall
	SudoCalls              []ShellCall
	SilentCalls            []ShellCall
	SilentWithTimeoutCalls []ShellCallWithTimeout
}

// NewShellCapture returns a new ShellCapture with empty slices, ready for use with NewMockShellWithCapture.
func NewShellCapture() *ShellCapture {
	return &ShellCapture{
		ExecCalls:              nil,
		SudoCalls:              nil,
		SilentCalls:            nil,
		SilentWithTimeoutCalls: nil,
	}
}

// TotalCalls returns the number of captured Exec, ExecSudo, ExecSilent, and ExecSilentWithTimeout calls.
func (c *ShellCapture) TotalCalls() int {
	return len(c.ExecCalls) + len(c.SudoCalls) + len(c.SilentCalls) + len(c.SilentWithTimeoutCalls)
}

// NewMockShellWithCapture returns a MockShell whose Exec, ExecSudo, ExecSilent, and ExecSilentWithTimeout
// append to capture and return "", nil. Callers can set other funcs (GetProjectRootFunc, etc.) as needed.
func NewMockShellWithCapture(capture *ShellCapture) *shell.MockShell {
	m := shell.NewMockShell()
	m.ExecFunc = func(command string, args ...string) (string, error) {
		capture.ExecCalls = append(capture.ExecCalls, ShellCall{Command: command, Args: args})
		return "", nil
	}
	m.ExecSudoFunc = func(message, command string, args ...string) (string, error) {
		capture.SudoCalls = append(capture.SudoCalls, ShellCall{Command: command, Args: args})
		return "", nil
	}
	m.ExecSilentFunc = func(command string, args ...string) (string, error) {
		capture.SilentCalls = append(capture.SilentCalls, ShellCall{Command: command, Args: args})
		return "", nil
	}
	m.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
		capture.SilentWithTimeoutCalls = append(capture.SilentWithTimeoutCalls, ShellCallWithTimeout{Command: command, Args: args, Timeout: timeout})
		return "", nil
	}
	return m
}

// NewMockShellWithPartialCapture returns a MockShell that delegates to realShell for all calls
// except ExecSudo and ExecSilentWithTimeout, which are captured and no-opped. Use when you want
// real terraform/docker/init but must not run network config or other privileged commands.
func NewMockShellWithPartialCapture(realShell shell.Shell, capture *ShellCapture) *shell.MockShell {
	m := shell.NewMockShell()
	m.SetVerbosityFunc = func(v bool) { realShell.SetVerbosity(v) }
	m.IsVerboseFunc = func() bool { return realShell.IsVerbose() }
	m.RenderEnvVarsFunc = realShell.RenderEnvVars
	m.RenderAliasesFunc = realShell.RenderAliases
	m.GetProjectRootFunc = realShell.GetProjectRoot
	m.ExecFunc = realShell.Exec
	m.ExecSilentFunc = realShell.ExecSilent
	m.ExecSilentWithTimeoutFunc = func(cmd string, args []string, timeout time.Duration) (string, error) {
		capture.SilentWithTimeoutCalls = append(capture.SilentWithTimeoutCalls, ShellCallWithTimeout{Command: cmd, Args: args, Timeout: timeout})
		return "", nil
	}
	m.ExecSudoFunc = func(_, cmd string, args ...string) (string, error) {
		capture.SudoCalls = append(capture.SudoCalls, ShellCall{Command: cmd, Args: args})
		return "", nil
	}
	m.ExecProgressFunc = realShell.ExecProgress
	m.InstallHookFunc = realShell.InstallHook
	m.AddCurrentDirToTrustedFileFunc = realShell.AddCurrentDirToTrustedFile
	m.CheckTrustedDirectoryFunc = realShell.CheckTrustedDirectory
	m.UnsetEnvsFunc = realShell.UnsetEnvs
	m.UnsetAliasFunc = realShell.UnsetAlias
	m.WriteResetTokenFunc = realShell.WriteResetToken
	m.GetSessionTokenFunc = realShell.GetSessionToken
	m.CheckResetFlagsFunc = realShell.CheckResetFlags
	m.ResetFunc = realShell.Reset
	m.RegisterSecretFunc = realShell.RegisterSecret
	return m
}
