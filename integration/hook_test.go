//go:build integration
// +build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/windsorcli/cli/integration/helpers"
)

// =============================================================================
// Integration Tests
// =============================================================================

// TestHook_SetsContextInEachShell sources the output of `windsor hook <shell>` in each shell that
// the runner has, runs the hook once, and checks that the shell then has WINDSOR_CONTEXT. It skips
// a shell that is not installed.
func TestHook_SetsContextInEachShell(t *testing.T) {
	type shellCase struct {
		hook   string
		binary string
		script string
		args   func(hookFile, scriptFile string) []string
	}
	cases := []shellCase{
		{hook: "bash", binary: "bash", args: func(hookFile, _ string) []string {
			return []string{"--noprofile", "--norc", "-c", `. "$1"; _windsor_hook; printf %s "$WINDSOR_CONTEXT"`, "bash", hookFile}
		}},
		{hook: "zsh", binary: "zsh", args: func(hookFile, _ string) []string {
			return []string{"-f", "-c", `source "$1"; _windsor_hook; printf %s "$WINDSOR_CONTEXT"`, "zsh", hookFile}
		}},
		{hook: "fish", binary: "fish", args: func(hookFile, _ string) []string {
			return []string{"--no-config", "-c", `source $argv[1]; _windsor_hook; printf %s "$WINDSOR_CONTEXT"`, hookFile}
		}},
		{hook: "tcsh", binary: "tcsh", script: "source \"%HOOK%\"\n_windsor_hook\nprintenv WINDSOR_CONTEXT\n", args: func(_, scriptFile string) []string {
			return []string{"-f", scriptFile}
		}},
	}
	if runtime.GOOS == "windows" {
		cases = []shellCase{
			{hook: "powershell", binary: "pwsh", args: func(hookFile, _ string) []string {
				return []string{"-NoProfile", "-Command", "Invoke-Expression (Get-Content -Raw '" + hookFile + "'); prompt | Out-Null; Write-Output $env:WINDSOR_CONTEXT"}
			}},
		}
	}

	for _, tc := range cases {
		t.Run(tc.hook, func(t *testing.T) {
			t.Parallel()
			shellPath, err := exec.LookPath(tc.binary)
			if err != nil {
				t.Skipf("%s is not installed", tc.binary)
			}
			dir, env := helpers.PrepareFixture(t, "default")
			hookOut, stderr, err := helpers.RunCLI(dir, []string{"hook", tc.hook}, env)
			if err != nil {
				t.Fatalf("hook %s: %v\nstderr: %s", tc.hook, err, stderr)
			}
			hookFile := filepath.Join(t.TempDir(), "hook")
			if err := os.WriteFile(hookFile, hookOut, 0600); err != nil {
				t.Fatalf("write hook: %v", err)
			}
			scriptFile := ""
			if tc.script != "" {
				scriptFile = filepath.Join(t.TempDir(), "run")
				if err := os.WriteFile(scriptFile, []byte(strings.ReplaceAll(tc.script, "%HOOK%", hookFile)), 0600); err != nil {
					t.Fatalf("write script: %v", err)
				}
			}

			cmd := exec.Command(shellPath, tc.args(hookFile, scriptFile)...)
			cmd.Dir = dir
			for _, kv := range os.Environ() {
				if !strings.HasPrefix(kv, "WINDSOR_") {
					cmd.Env = append(cmd.Env, kv)
				}
			}
			cmd.Env = append(cmd.Env, env...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s failed to run the hook: %v\noutput: %s", tc.binary, err, out)
			}
			if got := strings.TrimSpace(string(out)); !strings.HasSuffix(got, "local") {
				t.Errorf("expected WINDSOR_CONTEXT=local after the hook ran, got output: %q", got)
			}
		})
	}
}
