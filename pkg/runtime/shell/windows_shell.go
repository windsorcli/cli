//go:build windows
// +build windows

// The Windows shell methods are the DefaultShell methods for PowerShell.
// It provides the Remove-Item commands, the alias commands, and the env rendering with PowerShell quotes.
// ExecSudo runs the command without sudo, because Windows has no sudo command.
// It puts a progress command in its own console process group, so that Windsor can interrupt the group.

package shell

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// =============================================================================
// Public Methods
// =============================================================================

// UnsetEnvs generates commands to unset multiple environment variables.
// For Windows PowerShell, this produces a Remove-Item command for each environment variable.
func (s *DefaultShell) UnsetEnvs(envVars []string) {
	if len(envVars) == 0 {
		return
	}

	for _, env := range envVars {
		fmt.Printf("Remove-Item Env:%s\n", env)
	}
}

// UnsetAlias generates commands to unset multiple aliases.
// For Windows PowerShell, this produces a Remove-Item command for each alias.
func (s *DefaultShell) UnsetAlias(aliases []string) {
	if len(aliases) == 0 {
		return
	}

	for _, alias := range aliases {
		fmt.Printf("Remove-Item Alias:%s\n", alias)
	}
}

// RenderAliases returns the rendered aliases as a string using PowerShell syntax
func (s *DefaultShell) RenderAliases(aliases map[string]string) string {
	keys := make([]string, 0, len(aliases))
	for k := range aliases {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var result strings.Builder
	for _, k := range keys {
		if aliases[k] == "" {
			result.WriteString(fmt.Sprintf("Remove-Item Alias:%s\n", k))
		} else {
			result.WriteString(fmt.Sprintf("Set-Alias %s '%s'\n", k, aliases[k]))
		}
	}
	return result.String()
}

// ExecSudo verifies the process has administrator privileges. On Windows there is no sudo;
// the user must run PowerShell (or this executable) as Administrator. If already elevated,
// runs the given command; otherwise returns an error instructing them to run as Administrator.
func (s *DefaultShell) ExecSudo(message string, command string, args ...string) (string, error) {
	out, err := s.ExecSilent("powershell", "-NoProfile", "-NonInteractive", "-Command",
		"([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)")
	if err != nil {
		return "", fmt.Errorf("could not check administrator privileges: %w", err)
	}
	if !strings.Contains(strings.TrimSpace(strings.ToLower(out)), "true") {
		return "", fmt.Errorf("network configuration requires administrator privileges: open a new PowerShell as Administrator, then run the command again")
	}
	if s.verbose && message != "" {
		fmt.Fprintln(os.Stderr, message)
	}
	if command == "sudo" && len(args) > 0 {
		command, args = args[0], args[1:]
	}
	if command == "" || (command == "true" && len(args) == 0) {
		return "", nil
	}
	return s.Exec(command, args...)
}

// =============================================================================
// Private Methods
// =============================================================================

// renderEnvVarsWithExport returns environment variables with PowerShell commands as a string. Empty values trigger a removal command.
// Values containing special characters are properly quoted for PowerShell.
func (s *DefaultShell) renderEnvVarsWithExport(envVars map[string]string) string {
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var result strings.Builder
	for _, k := range keys {
		if envVars[k] == "" {
			result.WriteString(fmt.Sprintf("Remove-Item Env:%s\n", k))
		} else {
			value := s.quoteValueForPowerShell(envVars[k])
			result.WriteString(fmt.Sprintf("$env:%s=%s\n", k, value))
		}
	}
	return result.String()
}

// quoteValueForPowerShell quotes a value appropriately for PowerShell evaluation.
// PowerShell uses single quotes for literal strings, escaping single quotes by doubling them.
func (s *DefaultShell) quoteValueForPowerShell(value string) string {
	if !strings.ContainsAny(value, "[]{}()\"'$`\\ \t\n&|;<>*?~#") {
		return "'" + value + "'"
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// =============================================================================
// Helpers
// =============================================================================

// setProcessGroup places cmd in its own console process group. A second terminal
// interrupt then reaches cmd only through interruptGuard's call to interruptProcessGroup.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

// interruptProcessGroup sends CTRL_BREAK_EVENT to cmd's console process group.
// Windows has no SIGINT. A process in its own group (set by setProcessGroup)
// receives CTRL_BREAK_EVENT the way a foreground process receives Ctrl+C.
func interruptProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid)) // #nosec G115 -- process IDs are small, safe to cast to uint32
}

// killProcessGroup force-kills cmd's whole process tree via `taskkill /T /F`. Windows has
// no direct equivalent of a negative-pid group kill; taskkill's /T flag walks the tree, so
// a subprocess cmd spawned (e.g. terraform's own provider plugins) dies too, not just cmd.
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
}
