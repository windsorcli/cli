//go:build !windows
// +build !windows

// The Unix shell methods are the DefaultShell methods for bash, zsh, fish, and other Unix shells.
// It provides the unset and unalias commands, the export rendering, and sudo through /dev/tty.
// It puts a progress command in its own process group, so that Windsor can interrupt or kill the group.
// The methods in windows_shell.go provide the same methods for PowerShell.

package shell

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"

	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Public Methods
// =============================================================================

// UnsetEnvs generates a single unset command for multiple environment variables in Unix shells.
// It prints a single 'unset' command with all provided variable names separated by spaces.
// If the input slice is empty, no output is produced.
func (s *DefaultShell) UnsetEnvs(envVars []string) {
	if len(envVars) == 0 {
		return
	}
	fmt.Printf("unset %s\n", strings.Join(envVars, " "))
}

// UnsetAlias generates individual unalias commands for each alias in Unix shells.
// It prints a separate 'unalias' command for each alias name provided.
// If the input slice is empty, no output is produced.
func (s *DefaultShell) UnsetAlias(aliases []string) {
	if len(aliases) == 0 {
		return
	}
	for _, alias := range aliases {
		fmt.Printf("unalias %s\n", alias)
	}
}

// RenderAliases returns the rendered aliases as a string using Unix shell syntax
func (s *DefaultShell) RenderAliases(aliases map[string]string) string {
	keys := make([]string, 0, len(aliases))
	for k := range aliases {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var result strings.Builder
	for _, k := range keys {
		if aliases[k] == "" {
			result.WriteString(fmt.Sprintf("unalias %s\n", k))
		} else {
			result.WriteString(fmt.Sprintf("alias %s=%q\n", k, aliases[k]))
		}
	}
	return result.String()
}

// ExecSudo runs command with sudo and returns its output.
// In verbose mode or without a TTY, it runs the command directly.
// Otherwise it connects to /dev/tty for the password prompt and takes the terminal from the presenter.
// A scrubbingWriter buffers stderr by line, so registered secrets never reach the terminal.
// Because of the buffer, progress that uses only \r shows when the child writes \n or exits.
func (s *DefaultShell) ExecSudo(message string, command string, args ...string) (string, error) {
	if command != "sudo" {
		args = append([]string{command}, args...)
		command = "sudo"
	}

	if s.verbose || !s.shims.IsTerminal(int(os.Stdin.Fd())) { // #nosec G115 -- file descriptors are small, safe to cast to int
		if s.verbose && message != "" {
			fmt.Fprintln(os.Stderr, message)
		}
		return s.Exec(command, args...)
	}

	defer presenter.HandOff()()
	cmd := s.shims.Command(command, args...)
	if cmd == nil {
		return "", fmt.Errorf("failed to create command")
	}

	if cmd.Env == nil {
		cmd.Env = s.shims.Environ()
	}

	tty, err := s.shims.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", werror.Wrap(err, "failed to open /dev/tty")
	}
	defer tty.Close()

	cmd.Stdin = tty
	scrubbingTTYWriter := s.newScrubbingWriter(tty)
	defer func() { _ = scrubbingTTYWriter.Flush() }()
	cmd.Stderr = scrubbingTTYWriter

	var stdoutBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf

	if err := s.shims.CmdStart(cmd); err != nil {
		return stdoutBuf.String(), err
	}

	if err := s.shims.CmdWait(cmd); err != nil {
		return stdoutBuf.String(), werror.Wrap(err, "command execution failed")
	}

	return s.scrubString(stdoutBuf.String()), nil
}

// =============================================================================
// Private Methods
// =============================================================================

// renderEnvVarsWithExport returns environment variables in sorted order using export commands as a string.
// If a variable's value is empty, it returns an unset command instead.
// Values containing special characters are quoted with single quotes to ensure safe shell evaluation.
func (s *DefaultShell) renderEnvVarsWithExport(envVars map[string]string) string {
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var result strings.Builder
	for _, k := range keys {
		if envVars[k] == "" {
			result.WriteString(fmt.Sprintf("unset %s\n", k))
		} else {
			value := s.quoteValueForShell(envVars[k], true)
			result.WriteString(fmt.Sprintf("export %s=%s\n", k, value))
		}
	}
	return result.String()
}

// =============================================================================
// Helpers
// =============================================================================

// setProcessGroup places cmd in its own process group. A second terminal interrupt
// then reaches cmd only through interruptGuard's call to interruptProcessGroup.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// interruptProcessGroup sends SIGINT to cmd's process group. The negative pid
// targets the whole group, the same as a terminal-delivered interrupt would.
func interruptProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
}

// killProcessGroup sends SIGKILL to cmd's process group. The negative pid targets the
// whole group, so a subprocess cmd spawned (e.g. terraform's own provider plugins) dies
// too, not just cmd itself.
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
