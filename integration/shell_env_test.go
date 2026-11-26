//go:build integration
// +build integration

// The shell env tests prove that env injection works in each shell the docs list.
// They run one shell session per shell and per injection path (hook or eval of windsor env).
// A shell driver says how to source the hook, run it, and print a variable. One case table feeds all drivers.

package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/windsorcli/cli/integration/helpers"
)

// =============================================================================
// Types
// =============================================================================

// shellDriver describes how to drive one shell. Each func returns script text in the syntax of that shell.
type shellDriver struct {
	name        string
	binary      string
	hookName    string
	ext         string
	windowsOnly bool
	args        func(script string) []string
	source      func(hookFile string) string
	runHook     func() string
	evalEnv     func(bin string) string // nil when the shell has no documented eval path
	run         func(bin string, args ...string) string
	print       func(name string) string
	marker      func(phase string) string
}

// envCase is one variable that the env-a context sets. A value in want replaces value for a shell that cannot carry it.
type envCase struct {
	name  string
	value string
	want  map[string]string
}

// =============================================================================
// Constants
// =============================================================================

const unsetMarker = "@@unset@@"

// envCases is the shared case table. Every driver must reproduce each value unchanged, except where want says otherwise.
var envCases = []envCase{
	{name: "CASE_PLAIN", value: "plain-value"},
	{name: "CASE_SPACES", value: "two  spaces and\ttab"},
	{name: "CASE_SQUOTE", value: "it's a 'quoted' value"},
	{name: "CASE_DQUOTE", value: `say "hello" now`},
	{name: "CASE_DOLLAR", value: "$HOME $(echo no) `echo no` $1 $$ $"},
	{name: "CASE_BACKSLASH", value: `a\b\\c\n\"`},
	{name: "CASE_BANG", value: "wow! !! !$ !echo"},
	{name: "CASE_NEWLINE", value: "line1\nline2", want: map[string]string{"tcsh": "line1 line2"}},
}

var shellDrivers = []shellDriver{
	{
		name: "bash", binary: "bash", hookName: "bash", ext: ".sh",
		args:    func(script string) []string { return []string{"--noprofile", "--norc", script} },
		source:  func(hookFile string) string { return ". " + sq(hookFile) + "\n" },
		runHook: func() string { return "_windsor_hook\n" },
		evalEnv: func(bin string) string { return `eval "$(` + sq(bin) + " env --decrypt)\"\n" },
		run:     posixRun,
		print:   posixPrint,
		marker:  func(phase string) string { return "printf '[[%s]]' " + phase + "\n" },
	},
	{
		name: "zsh", binary: "zsh", hookName: "zsh", ext: ".zsh",
		args:    func(script string) []string { return []string{"-f", script} },
		source:  func(hookFile string) string { return "source " + sq(hookFile) + "\n" },
		runHook: func() string { return "_windsor_hook\n" },
		evalEnv: func(bin string) string { return `eval "$(` + sq(bin) + " env --decrypt)\"\n" },
		run:     posixRun,
		print:   posixPrint,
		marker:  func(phase string) string { return "printf '[[%s]]' " + phase + "\n" },
	},
	{
		name: "fish", binary: "fish", hookName: "fish", ext: ".fish",
		args:    func(script string) []string { return []string{"--no-config", script} },
		source:  func(hookFile string) string { return "source " + sq(hookFile) + "\n" },
		runHook: func() string { return "_windsor_hook\n" },
		run:     posixRun,
		print: func(name string) string {
			return "if set -q " + name + "\n printf '<<%s|%s|>>' " + name + ` "$` + name + "\"\nelse\n printf '<<%s|" + unsetMarker + "|>>' " + name + "\nend\n"
		},
		marker: func(phase string) string { return "printf '[[%s]]' " + phase + "\n" },
	},
	{
		name: "tcsh", binary: "tcsh", hookName: "tcsh", ext: ".csh",
		args:    func(script string) []string { return []string{"-f", script} },
		source:  func(hookFile string) string { return "source " + sq(hookFile) + "\n" },
		runHook: func() string { return "_windsor_hook\n" },
		evalEnv: func(bin string) string { return "eval \"`" + sq(bin) + " env --decrypt --shell tcsh`\"\n" },
		run:     posixRun,
		print: func(name string) string {
			return "if ($?" + name + ") then\n printf '<<%s|%s|>>' " + name + ` "$` + name + ":q\"\nelse\n printf '<<%s|" + unsetMarker + "|>>' " + name + "\nendif\n"
		},
		marker: func(phase string) string { return "printf '[[%s]]' " + phase + "\n" },
	},
	{
		name: "powershell", binary: "pwsh", hookName: "powershell", ext: ".ps1", windowsOnly: true,
		args:    func(script string) []string { return []string{"-NoProfile", "-File", script} },
		source:  func(hookFile string) string { return "Invoke-Expression (Get-Content -Raw " + sq(hookFile) + ")\n" },
		runHook: func() string { return "prompt | Out-Null\n" },
		evalEnv: func(bin string) string { return "& " + sq(bin) + " env --decrypt | Out-String | Invoke-Expression\n" },
		run: func(bin string, args ...string) string {
			return "& " + sq(bin) + " " + strings.Join(args, " ") + " | Out-Null\n"
		},
		print: func(name string) string {
			return "if (Test-Path env:" + name + ") { [Console]::Out.Write('<<" + name + "|' + $env:" + name + " + '|>>') } else { [Console]::Out.Write('<<" + name + "|" + unsetMarker + "|>>') }\n"
		},
		marker: func(phase string) string { return "[Console]::Out.Write('[[" + phase + "]]')\n" },
	},
}

// =============================================================================
// Integration Tests
// =============================================================================

// TestShellEnv_InjectsEnvInEachShell runs one real shell session per shell and per injection path.
// The session loads env-a, loads it again, switches to env-b, and loads again. After each load it prints
// every variable. The test checks that each value arrives unchanged, that a second load changes nothing,
// and that keys only env-a had are unset after the switch.
// With WINDSOR_TEST_SHELL set, only that shell runs and a missing shell fails the test. Without it,
// a missing shell skips the test.
func TestShellEnv_InjectsEnvInEachShell(t *testing.T) {
	required := os.Getenv("WINDSOR_TEST_SHELL")
	if required != "" && !hasDriver(required) {
		t.Fatalf("WINDSOR_TEST_SHELL=%q is not a known shell", required)
	}
	for _, d := range shellDrivers {
		if required != "" && d.name != required {
			continue
		}
		modes := []string{"hook"}
		if d.evalEnv != nil {
			modes = append(modes, "eval")
		}
		for _, mode := range modes {
			t.Run(d.name+"/"+mode, func(t *testing.T) {
				t.Parallel()
				if d.windowsOnly && runtime.GOOS != "windows" {
					t.Skipf("%s output is only rendered on Windows", d.name)
				}
				shellPath, err := exec.LookPath(d.binary)
				if err != nil {
					if required != "" {
						t.Fatalf("%s is required by WINDSOR_TEST_SHELL but is not installed", d.binary)
					}
					t.Skipf("%s is not installed", d.binary)
				}
				runShellSession(t, d, shellPath, mode)
			})
		}
	}
}

// =============================================================================
// Private Methods
// =============================================================================

// runShellSession builds the fixture, runs the shell script, and checks the printed phases.
func runShellSession(t *testing.T, d shellDriver, shellPath, mode string) {
	t.Helper()
	dir, env := shellEnvFixture(t)
	bin := helpers.BinaryPath

	var script strings.Builder
	if mode == "hook" {
		hookOut, stderr, err := helpers.RunCLI(dir, []string{"hook", d.hookName}, env)
		if err != nil {
			t.Fatalf("hook %s: %v\nstderr: %s", d.hookName, err, stderr)
		}
		hookFile := filepath.Join(t.TempDir(), "hook"+d.ext)
		if err := os.WriteFile(hookFile, hookOut, 0600); err != nil {
			t.Fatalf("write hook: %v", err)
		}
		script.WriteString(d.source(hookFile))
	}
	load := d.runHook
	if mode == "eval" {
		load = func() string { return d.evalEnv(bin) }
	}
	names := []string{"WINDSOR_CONTEXT", "WINDSOR_PROJECT_ROOT", "SHARED_VAR", "ONLY_A_VAR"}
	for _, c := range envCases {
		names = append(names, c.name)
	}
	dump := func(phase string) {
		script.WriteString(d.marker(phase))
		for _, n := range names {
			script.WriteString(d.print(n))
		}
	}
	script.WriteString(load())
	dump("first")
	script.WriteString(load())
	dump("second")
	script.WriteString(d.run(bin, "set", "context", "env-b"))
	script.WriteString(load())
	dump("switched")

	scriptFile := filepath.Join(t.TempDir(), "session"+d.ext)
	if err := os.WriteFile(scriptFile, []byte(script.String()), 0600); err != nil {
		t.Fatalf("write script: %v", err)
	}
	cmd := exec.Command(shellPath, d.args(scriptFile)...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "WINDSOR_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, env...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s session failed: %v\nstdout: %s\nstderr: %s\nscript:\n%s", d.name, err, out, stderr.String(), script.String())
	}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("stdout: %q\nscript:\n%s", out, script.String())
			dbgOut, dbgErr, dbgRun := helpers.RunCLI(dir, []string{"env", "--decrypt"}, env)
			t.Logf("windsor env --decrypt in %s: err=%v\nstdout: %s\nstderr: %s", dir, dbgRun, dbgOut, dbgErr)
		}
	})
	phases := parsePhases(string(out))
	fail := func(format string, args ...any) {
		t.Errorf(format+"\nstderr: %s", append(args, stderr.String())...)
	}

	for _, phase := range []string{"first", "second"} {
		got := phases[phase]
		if got == nil {
			fail("phase %q printed nothing; stdout: %q", phase, out)
			continue
		}
		for _, c := range envCases {
			want := c.value
			if w, ok := c.want[d.name]; ok {
				want = w
			}
			ok := got[c.name] == want
			if knownBug(t, d.name, c.name, ok) {
				continue
			}
			if !ok {
				fail("%s: %s = %q, want %q", phase, c.name, got[c.name], want)
			}
		}
		expect := map[string]string{"WINDSOR_CONTEXT": "env-a", "SHARED_VAR": "from-a", "ONLY_A_VAR": "only-in-a"}
		for k, want := range expect {
			if got[k] != want {
				fail("%s: %s = %q, want %q", phase, k, got[k], want)
			}
		}
		if root := got["WINDSOR_PROJECT_ROOT"]; root == "" || root == unsetMarker {
			fail("%s: WINDSOR_PROJECT_ROOT is not set", phase)
		}
	}
	if phases["first"] != nil && phases["second"] != nil {
		for k, v := range phases["first"] {
			if phases["second"][k] != v {
				fail("second load changed %s from %q to %q", k, v, phases["second"][k])
			}
		}
	}

	if mode != "hook" {
		return
	}
	sw := phases["switched"]
	if sw == nil {
		fail("phase \"switched\" printed nothing; stdout: %q", out)
		return
	}
	if sw["WINDSOR_CONTEXT"] != "env-b" {
		fail("switched: WINDSOR_CONTEXT = %q, want env-b", sw["WINDSOR_CONTEXT"])
	}
	if sw["SHARED_VAR"] != "from-b" {
		fail("switched: SHARED_VAR = %q, want from-b", sw["SHARED_VAR"])
	}
	if root := sw["WINDSOR_PROJECT_ROOT"]; root == "" || root == unsetMarker {
		fail("switched: WINDSOR_PROJECT_ROOT is not set")
	}
	var leaked []string
	for _, name := range append([]string{"ONLY_A_VAR"}, caseNames()...) {
		if sw[name] != unsetMarker {
			leaked = append(leaked, name)
		}
	}
	if knownBug(t, d.name, "switch", len(leaked) == 0) {
		return
	}
	if len(leaked) > 0 {
		fail("switched: %v still set, want them unset", leaked)
	}
}

// shellEnvFixture builds a trusted project with two contexts. env-a sets every case variable.
// env-b sets only SHARED_VAR. The project is left on env-a.
func shellEnvFixture(t *testing.T) (dir string, env []string) {
	t.Helper()
	dir, env = helpers.CopyFixtureOnly(t, "default")
	envA := map[string]string{"SHARED_VAR": "from-a", "ONLY_A_VAR": "only-in-a"}
	for _, c := range envCases {
		envA[c.name] = c.value
	}
	contexts := map[string]string{
		"env-a": yamlEnvironment(t, envA),
		"env-b": yamlEnvironment(t, map[string]string{"SHARED_VAR": "from-b"}),
	}
	for name, values := range contexts {
		if err := os.MkdirAll(filepath.Join(dir, "contexts", name), 0755); err != nil {
			t.Fatalf("mkdir context: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "contexts", name, "values.yaml"), []byte(values), 0644); err != nil {
			t.Fatalf("write values.yaml: %v", err)
		}
	}
	helpers.MarkAsGitRepo(t, dir)
	for _, args := range [][]string{{"init"}, {"init", "env-a"}, {"init", "env-b"}, {"set", "context", "env-a"}} {
		if _, stderr, err := helpers.RunCLI(dir, args, env); err != nil {
			t.Fatalf("windsor %v: %v\nstderr: %s", args, err, stderr)
		}
	}
	return dir, env
}

// =============================================================================
// Helpers
// =============================================================================

// knownBugs maps "shell:key" to the issue that tracks a quoting or unset bug in that shell.
// A key is a case variable name, or "switch" for the unset checks after a context switch.
// The test still runs each tracked check and fails when the check passes, so a fix cannot go unnoticed.
var knownBugs = map[string]string{
	"fish:CASE_BACKSLASH": "https://github.com/windsorcli/cli/issues/3925",
	"fish:switch":         "https://github.com/windsorcli/cli/issues/3926",
}

// knownBug handles a check that has a tracked bug for the shell. ok says whether the check passed.
// It returns false when the shell has no tracked bug for key, and the caller reports a failure as usual.
// Otherwise it returns true. A failing check logs the issue. A passing check fails the test,
// because the bug is fixed and the entry MUST leave knownBugs.
func knownBug(t *testing.T, shell, key string, ok bool) bool {
	t.Helper()
	issue, tracked := knownBugs[shell+":"+key]
	if !tracked {
		return false
	}
	if ok {
		t.Errorf("%s for %s now passes: remove %q from knownBugs (%s)", key, shell, shell+":"+key, issue)
	} else {
		t.Logf("%s for %s still fails: known bug %s", key, shell, issue)
	}
	return true
}

// caseNames returns the variable name of each case.
func caseNames() []string {
	names := make([]string, 0, len(envCases))
	for _, c := range envCases {
		names = append(names, c.name)
	}
	return names
}

var phasePattern = regexp.MustCompile(`(?s)\[\[(\w+)\]\]|<<(\w+)\|(.*?)\|>>`)

// parsePhases turns the session output into a map of phase name to variable values.
func parsePhases(out string) map[string]map[string]string {
	phases := map[string]map[string]string{}
	var current map[string]string
	for _, m := range phasePattern.FindAllStringSubmatch(out, -1) {
		if m[1] != "" {
			current = map[string]string{}
			phases[m[1]] = current
		} else if current != nil {
			current[m[2]] = m[3]
		}
	}
	return phases
}

// yamlEnvironment renders vars as a values.yaml with an environment map. JSON strings are valid YAML.
func yamlEnvironment(t *testing.T, vars map[string]string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("environment:\n")
	for k, v := range vars {
		enc, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("encode %s: %v", k, err)
		}
		fmt.Fprintf(&b, "  %s: %s\n", k, enc)
	}
	return b.String()
}

// hasDriver reports whether a driver has the given name.
func hasDriver(name string) bool {
	for _, d := range shellDrivers {
		if d.name == name {
			return true
		}
	}
	return false
}

// sq wraps s in single quotes for bash, zsh, fish, tcsh, and PowerShell.
func sq(s string) string {
	if runtime.GOOS == "windows" {
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// posixRun returns a command line that runs bin with args in a POSIX-like shell, fish, or tcsh.
func posixRun(bin string, args ...string) string {
	return sq(bin) + " " + strings.Join(args, " ") + " >/dev/null\n"
}

// posixPrint prints name in bash or zsh with a marker for an unset variable.
func posixPrint(name string) string {
	return `if [ -n "${` + name + `+x}" ]; then printf '<<%s|%s|>>' ` + name + ` "$` + name + `"; else printf '<<%s|` + unsetMarker + `|>>' ` + name + "; fi\n"
}
