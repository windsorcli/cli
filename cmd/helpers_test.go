//go:build integration
// +build integration

package cmd_test

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

	cliCmd "github.com/windsorcli/cli/cmd"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
	cliCmd.RootCmd().SetOut(stdout)
	cliCmd.RootCmd().SetErr(stderr)
	t.Cleanup(func() {
		cliCmd.RootCmd().SetOut(nil)
		cliCmd.RootCmd().SetErr(nil)
	})
	return stdout, stderr
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
// Slice-typed flags (StringSliceVar, etc.) require Replace rather than Set: pflag's
// Set for slice values appends instead of resetting, so calling Set("[]") would
// accumulate the literal "[]" element across resets and corrupt later parses.
func resetCommandFlagValues(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if sliceValue, ok := f.Value.(pflag.SliceValue); ok {
			_ = sliceValue.Replace(nil)
			return
		}
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
	resetCommandFlagValues(cliCmd.RootCmd())
	_, stderrBuf := captureOutputAndRestore(t)
	stdoutBuf, restore := captureProcessStdout(t)
	cliCmd.RootCmd().SetContext(ctx)
	cliCmd.RootCmd().SetArgs(args)
	err = cliCmd.Execute()
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
// Integration tests can use the shell in two ways:
//
//  1. Real shell (default): Do not pass a project/runtime override. runCmd(t, context.Background(), args)
//     uses the real project and shell, so terraform, docker, init, etc. run for real.
//
//  2. Full mock: Use NewMockShellWithCapture(capture) and inject it via a project override. Every
//     Exec/ExecSudo/ExecSilent/ExecSilentWithTimeout is captured and no-opped. Use when the test
//     must not run any subprocesses (e.g. configure network tests that must not touch routes/DNS).
