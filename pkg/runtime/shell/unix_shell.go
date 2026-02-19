//go:build !windows
// +build !windows

package shell

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
)

// The UnixShell is a platform-specific implementation of shell operations for Unix-like systems.
// It provides Unix-specific implementations of environment variable and alias management.
// It handles the differences between Unix shells and Windows PowerShell.
// Key features include Unix-specific command generation for environment variables and aliases.

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

// ExecSudo runs a command with 'sudo', ensuring elevated privileges. It handles password prompts by
// connecting to the terminal and captures the command's output. If verbose mode is enabled or no TTY
// is available (CI/CD environments), it uses direct execution. Otherwise, it connects to /dev/tty for
// interactive password prompts. The function returns the command's stdout or an error if execution fails.
func (s *DefaultShell) ExecSudo(message string, command string, args ...string) (string, error) {
	if command != "sudo" {
		args = append([]string{command}, args...)
		command = "sudo"
	}

	if s.verbose || !s.shims.IsTerminal(int(os.Stdin.Fd())) {
		if s.verbose {
			fmt.Fprintln(os.Stderr, message)
		}
		return s.Exec(command, args...)
	}

	cmd := s.shims.Command(command, args...)
	if cmd == nil {
		return "", fmt.Errorf("failed to create command")
	}

	if cmd.Env == nil {
		cmd.Env = s.shims.Environ()
	}

	tty, err := s.shims.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("failed to open /dev/tty: %w", err)
	}
	defer tty.Close()

	cmd.Stdin = tty
	cmd.Stderr = tty

	var stdoutBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf

	if err := s.shims.CmdStart(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
		return stdoutBuf.String(), err
	}

	err = s.shims.CmdWait(cmd)

	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
		return stdoutBuf.String(), fmt.Errorf("command execution failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)

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
