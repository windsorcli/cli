// The command execution methods are the DefaultShell methods that run external commands.
// It provides the plain, silent, capture, and progress forms of command execution.
// Each form runs the command in the inherited environment and scrubs registered secrets.
// The progress forms relay one terminal interrupt to the child and block the interrupts after it.

package shell

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Types
// =============================================================================

// interruptGuard relays one interrupt to a child process. It blocks every interrupt
// after that for the rest of the run. A second interrupt can force-kill the child
// before it exits cleanly. forward sends the interrupt to the child's own process
// group. This stops a second terminal interrupt from reaching the child directly.
type interruptGuard struct {
	forward   func() error
	warn      io.Writer
	component string
}

// =============================================================================
// Public Methods
// =============================================================================

// Exec runs a command with args, capturing stdout and stderr. It prints output and returns stdout as a string.
// If the command is "sudo", it connects stdin to the terminal for password input.
// All output is scrubbed to remove registered secrets before being displayed or returned.
func (s *DefaultShell) Exec(command string, args ...string) (string, error) {
	cmd := s.shims.Command(command, args...)
	var stdoutBuf, stderrBuf bytes.Buffer

	scrubbingStdoutWriter := s.newScrubbingWriter(os.Stdout)
	scrubbingStderrWriter := s.newScrubbingWriter(os.Stderr)
	defer func() { _ = scrubbingStdoutWriter.Flush() }()
	defer func() { _ = scrubbingStderrWriter.Flush() }()

	cmd.Stdout = io.MultiWriter(scrubbingStdoutWriter, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(scrubbingStderrWriter, &stderrBuf)
	if command == "sudo" {
		cmd.Stdin = os.Stdin
	}
	if cmd.Env == nil {
		cmd.Env = s.shims.Environ()
	}
	if err := s.shims.CmdStart(cmd); err != nil {
		return stdoutBuf.String(), werror.Wrap(err, "command start failed")
	}
	if err := s.shims.CmdWait(cmd); err != nil {
		return s.scrubString(stdoutBuf.String()), werror.Wrap(err, "command execution failed")
	}
	return s.scrubString(stdoutBuf.String()), nil
}

// ExecSilent is a method that runs a command quietly, capturing its output.
// It returns the command's stdout as a string and any error encountered.
func (s *DefaultShell) ExecSilent(command string, args ...string) (string, error) {
	if s.verbose {
		return s.Exec(command, args...)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := s.shims.Command(command, args...)
	if cmd == nil {
		return "", fmt.Errorf("failed to create command")
	}

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if cmd.Env == nil {
		cmd.Env = s.shims.Environ()
	}

	if err := s.shims.CmdRun(cmd); err != nil {
		return s.scrubString(stdoutBuf.String()), execError(err, s.scrubString(stderrBuf.String()))
	}

	return s.scrubString(stdoutBuf.String()), nil
}

// ExecSilentWithEnv runs a command with merged environment variables, capturing output silently.
// In verbose mode, output is streamed directly to stdout and stderr.
func (s *DefaultShell) ExecSilentWithEnv(command string, env map[string]string, args ...string) (string, error) {
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := s.shims.Command(command, args...)
	if cmd == nil {
		return "", fmt.Errorf("failed to create command")
	}
	cmd.Env = mergeEnvVars(s.shims.Environ(), env)
	if s.verbose {
		scrubbingStdoutWriter := s.newScrubbingWriter(os.Stdout)
		scrubbingStderrWriter := s.newScrubbingWriter(os.Stderr)
		defer func() { _ = scrubbingStdoutWriter.Flush() }()
		defer func() { _ = scrubbingStderrWriter.Flush() }()
		cmd.Stdout = io.MultiWriter(scrubbingStdoutWriter, &stdoutBuf)
		cmd.Stderr = io.MultiWriter(scrubbingStderrWriter, &stderrBuf)
	} else {
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
	}
	if err := s.shims.CmdRun(cmd); err != nil {
		return s.scrubString(stdoutBuf.String()), execError(err, s.scrubString(stderrBuf.String()))
	}
	return s.scrubString(stdoutBuf.String()), nil
}

// ExecCaptureWithEnv runs a command with merged environment variables and captures its output,
// never streaming to stdout or stderr even in verbose mode. Use it for internal, machine-only output
// (e.g. `terraform show -json` state inspection) that is parsed rather than shown and may contain
// sensitive material such as cluster PKI — unlike ExecSilentWithEnv, which echoes captured output
// under verbose.
func (s *DefaultShell) ExecCaptureWithEnv(command string, env map[string]string, args ...string) (string, error) {
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := s.shims.Command(command, args...)
	if cmd == nil {
		return "", fmt.Errorf("failed to create command")
	}
	cmd.Env = mergeEnvVars(s.shims.Environ(), env)
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if err := s.shims.CmdRun(cmd); err != nil {
		return s.scrubString(stdoutBuf.String()), execError(err, s.scrubString(stderrBuf.String()))
	}
	return s.scrubString(stdoutBuf.String()), nil
}

// ExecProgress is a method of the DefaultShell struct that executes a command for a progress step.
// It takes a message, a command, and arguments, using the Exec method if verbose mode is enabled.
// Otherwise, it captures stdout and stderr with pipes; the caller's presenter step shows progress.
// The method returns the command's stdout as a string and any error encountered.
func (s *DefaultShell) ExecProgress(message string, command string, args ...string) (string, error) {
	return s.ExecProgressWithEnv(message, command, nil, args...)
}

// ExecProgressWithEnv executes a command with command-scoped env overrides. In verbose mode it prints
// message and streams output; otherwise it captures output and prints it only on failure.
func (s *DefaultShell) ExecProgressWithEnv(message string, command string, env map[string]string, args ...string) (string, error) {
	if s.verbose {
		return s.execProgressVerbose(message, command, env, args)
	}

	cmd := s.shims.Command(command, args...)
	if cmd == nil {
		return "", fmt.Errorf("failed to create command")
	}
	cmd.Env = mergeEnvVars(s.shims.Environ(), env)
	if command == "sudo" {
		cmd.Stdin = os.Stdin
	}

	stdoutPipe, err := s.shims.StdoutPipe(cmd)
	if err != nil {
		return "", err
	}

	stderrPipe, err := s.shims.StderrPipe(cmd)
	if err != nil {
		return "", err
	}

	defer s.startInterruptGuard(cmd, command, message)()
	if err := s.shims.CmdStart(cmd); err != nil {
		return "", err
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	errChan := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		errChan <- s.scanProgressStream(stdoutPipe, &stdoutBuf, "stdout")
	}()

	go func() {
		defer wg.Done()
		errChan <- s.scanProgressStream(stderrPipe, &stderrBuf, "stderr")
	}()

	wg.Wait()

	var firstErr error
	for range [2]struct{}{} {
		err := <-errChan
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	cmdErr := s.shims.CmdWait(cmd)

	if firstErr != nil || cmdErr != nil {
		fmt.Fprintf(os.Stderr, "\n[ExecProgress ERROR]\nCommand: %s %v\nStdout:\n%s\nStderr:\n%s\nError: %v\n", command, s.scrubString(fmt.Sprintf("%v", args)), s.scrubString(stdoutBuf.String()), s.scrubString(stderrBuf.String()), firstErr)
		if cmdErr != nil {
			return s.scrubString(stdoutBuf.String()), werror.Wrap(cmdErr, "command execution failed")
		}
		return s.scrubString(stdoutBuf.String()), firstErr
	}

	return s.scrubString(stdoutBuf.String()), nil
}

// =============================================================================
// Private Methods
// =============================================================================

// execProgressVerbose runs a progress command in verbose mode. It prints message, then streams
// the scrubbed command output to stdout and stderr while it captures stdout for the return value.
func (s *DefaultShell) execProgressVerbose(message string, command string, env map[string]string, args []string) (string, error) {
	fmt.Fprintln(os.Stderr, message)
	cmd := s.shims.Command(command, args...)
	if cmd == nil {
		return "", fmt.Errorf("failed to create command")
	}
	var stdoutBuf, stderrBuf bytes.Buffer
	scrubbingStdoutWriter := s.newScrubbingWriter(os.Stdout)
	scrubbingStderrWriter := s.newScrubbingWriter(os.Stderr)
	defer func() { _ = scrubbingStdoutWriter.Flush() }()
	defer func() { _ = scrubbingStderrWriter.Flush() }()
	cmd.Stdout = io.MultiWriter(scrubbingStdoutWriter, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(scrubbingStderrWriter, &stderrBuf)
	cmd.Env = mergeEnvVars(s.shims.Environ(), env)
	if command == "sudo" {
		cmd.Stdin = os.Stdin
	}
	defer s.startInterruptGuard(cmd, command, message)()
	if err := s.shims.CmdStart(cmd); err != nil {
		return stdoutBuf.String(), werror.Wrap(err, "command start failed")
	}
	if err := s.shims.CmdWait(cmd); err != nil {
		return s.scrubString(stdoutBuf.String()), werror.Wrap(err, "command execution failed")
	}
	return s.scrubString(stdoutBuf.String()), nil
}

// scanProgressStream reads the lines of one progress command stream into buf and skips empty lines.
// stream names the stream in the error text. A closed pipe at the end of the command is not an error.
func (s *DefaultShell) scanProgressStream(pipe io.Reader, buf *bytes.Buffer, stream string) error {
	scanner := s.shims.NewScanner(pipe)
	if scanner == nil {
		return fmt.Errorf("failed to create %s scanner", stream)
	}
	for s.shims.ScannerScan(scanner) {
		line := s.shims.ScannerText(scanner)
		if line != "" {
			buf.WriteString(line + "\n")
		}
	}
	if err := s.shims.ScannerErr(scanner); err != nil && err != io.EOF && !isClosedPipe(err) {
		return werror.Wrap(err, "error reading %s", stream)
	}
	return nil
}

// startInterruptGuard places cmd in its own process group. It relays the first
// terminal interrupt to cmd, then blocks every interrupt after that. See
// interruptGuard. It skips sudo. Setpgid would strand sudo's password prompt on
// SIGTTIN. Call the returned stop func once cmd exits. It no-ops when skipped.
// While the guard runs, presenter.InterruptsGuarded reports true.
func (s *DefaultShell) startInterruptGuard(cmd *exec.Cmd, command string, message string) (stop func()) {
	if command == "sudo" {
		return func() {}
	}

	setProcessGroup(cmd)

	sigCh := make(chan os.Signal, 4)
	s.shims.SignalNotify(sigCh, os.Interrupt)
	done := make(chan struct{})
	release := presenter.GuardInterrupts()

	guard := &interruptGuard{
		forward:   func() error { return interruptProcessGroup(cmd) },
		warn:      os.Stderr,
		component: message,
	}
	go guard.watch(sigCh, done)

	return func() {
		close(done)
		s.shims.SignalStop(sigCh)
		release()
	}
}

// watch forwards the first signal on sigCh to the child via forward. Every signal
// after that prints a warning instead of forwarding. Each warning hands the terminal off
// from the default presenter first, so its live region cannot draw over the warning.
// watch returns once done closes. The caller closes done when the child exits.
func (g *interruptGuard) watch(sigCh <-chan os.Signal, done <-chan struct{}) {
	forwarded := false
	for {
		select {
		case <-done:
			return
		case <-sigCh:
			resume := presenter.HandOff()
			if !forwarded {
				forwarded = true
				fmt.Fprintf(g.warn, "warning: interrupt received during apply of %s. The resource may now be tainted in Terraform state. Windsor waits for terraform to stop gracefully. A further interrupt will not force it. If terraform hangs, stop it from another terminal.\n", g.component)
				_ = g.forward()
			} else {
				fmt.Fprintf(g.warn, "warning: still waiting for terraform to stop gracefully for %s. Forcing now can corrupt state. If it hangs, stop it from another terminal.\n", g.component)
			}
			resume()
		}
	}
}

// =============================================================================
// Helpers
// =============================================================================

// execError wraps the error of a failed command. It adds the scrubbed stderr of the command
// after the cause, so that the error text and the verbose trace both show it.
func execError(err error, stderr string) error {
	if stderr == "" {
		return werror.Wrap(err, "command execution failed")
	}
	return werror.Wrap(errors.Join(err, errors.New(stderr)), "command execution failed")
}

// mergeEnvVars merges env overrides onto a base env slice in KEY=VALUE form.
func mergeEnvVars(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}
	merged := make(map[string]string, len(base)+len(overrides))
	for _, entry := range base {
		parts := strings.SplitN(entry, "=", 2)
		key := parts[0]
		val := ""
		if len(parts) > 1 {
			val = parts[1]
		}
		merged[key] = val
	}
	maps.Copy(merged, overrides)
	result := make([]string, 0, len(merged))
	for key, value := range merged {
		result = append(result, key+"="+value)
	}
	sort.Strings(result)
	return result
}

// isClosedPipe returns true if the error is an io.ErrClosedPipe or equivalent
func isClosedPipe(err error) bool {
	return err != nil && (err == io.ErrClosedPipe || strings.Contains(err.Error(), "file already closed") || strings.Contains(err.Error(), "use of closed file"))
}
