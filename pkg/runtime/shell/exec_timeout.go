// The timeout execution methods are the DefaultShell methods that run a command with a deadline.
// It provides a flat timeout that kills the command, and an idle timeout that resets on output.
// The idle timeout interrupts the process group and waits a grace period before it kills the group.
// Each method returns ErrCommandTimedOut in its error when a deadline stops the command.

package shell

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constants
// =============================================================================

// ErrCommandTimedOut marks a command killed for exceeding its timeout.
var ErrCommandTimedOut = errors.New("command timed out")

// =============================================================================
// Public Methods
// =============================================================================

// ExecSilentWithTimeout executes a command with a timeout and returns the output.
// If the command takes longer than the timeout, it kills the process and returns an error.
// Uses ExecSilent internally but wraps it with a timeout mechanism.
func (s *DefaultShell) ExecSilentWithTimeout(command string, args []string, timeout time.Duration) (string, error) {
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

	return s.runWithTimeout(cmd, &stdoutBuf, &stderrBuf, timeout)
}

// ExecSilentWithEnvAndTimeout combines ExecSilentWithEnv and ExecSilentWithTimeout: it merges
// command-scoped env overrides into the inherited environment AND enforces a timeout, killing
// the child process if it exceeds the deadline. Used by callers that need both behaviors at
// once — e.g. CheckAuth() running `aws sts get-caller-identity` with the context-scoped
// AWS_CONFIG_FILE/AWS_PROFILE set, where the env is required for credential resolution and
// the timeout guards against a hung network call.
func (s *DefaultShell) ExecSilentWithEnvAndTimeout(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := s.shims.Command(command, args...)
	if cmd == nil {
		return "", fmt.Errorf("failed to create command")
	}

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	cmd.Env = mergeEnvVars(s.shims.Environ(), env)

	return s.runWithTimeout(cmd, &stdoutBuf, &stderrBuf, timeout)
}

// ExecSilentWithEnvAndIdleTimeout runs command, streaming its output so a stall can be detected
// without bounding how long a legitimately slow but still-progressing run may take. idleTimeout
// resets on every line the command emits; only silence for that long counts as stuck.
// absoluteTimeout is a generous backstop against a command that keeps emitting output without
// ever finishing. Either firing interrupts the process group first, waits up to gracePeriod for
// the process to exit, and only then force-kills.
//
// Use this for a long-running command whose own duration varies with what it is acting on, and
// whose abrupt termination can corrupt external state. `terraform destroy` is the motivating
// case: a wall-clock timeout kills a slow-but-healthy cloud teardown as readily as a genuinely
// hung one, and a hard kill mid-run loses its chance to write a state checkpoint and release its
// backend lock.
//
// Ordinary timeout-bound commands hold no external state and have no progress output to key off
// of. They should keep using ExecSilentWithEnvAndTimeout, which kills immediately on one flat
// deadline.
func (s *DefaultShell) ExecSilentWithEnvAndIdleTimeout(command string, env map[string]string, args []string, idleTimeout, absoluteTimeout, gracePeriod time.Duration) (string, error) {
	cmd := s.shims.Command(command, args...)
	if cmd == nil {
		return "", fmt.Errorf("failed to create command")
	}
	cmd.Env = mergeEnvVars(s.shims.Environ(), env)
	setProcessGroup(cmd)

	stdoutPipe, err := s.shims.StdoutPipe(cmd)
	if err != nil {
		return "", err
	}
	stderrPipe, err := s.shims.StderrPipe(cmd)
	if err != nil {
		return "", err
	}

	if err := s.shims.CmdStart(cmd); err != nil {
		return "", werror.Wrap(err, "command start failed")
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	var bufMu sync.Mutex
	activity := make(chan struct{}, 1)
	notifyActivity := func() {
		select {
		case activity <- struct{}{}:
		default:
		}
	}

	scanErrs := make(chan error, 2)
	scan := func(pipe io.Reader, buf *bytes.Buffer) {
		scanner := s.shims.NewScanner(pipe)
		if scanner == nil {
			scanErrs <- fmt.Errorf("failed to create scanner")
			return
		}
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for s.shims.ScannerScan(scanner) {
			line := s.shims.ScannerText(scanner)
			bufMu.Lock()
			buf.WriteString(line + "\n")
			bufMu.Unlock()
			notifyActivity()
		}
		if err := s.shims.ScannerErr(scanner); err != nil && err != io.EOF && !isClosedPipe(err) {
			scanErrs <- werror.Wrap(err, "error reading output")
			return
		}
		scanErrs <- nil
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); scan(stdoutPipe, &stdoutBuf) }()
	go func() { defer wg.Done(); scan(stderrPipe, &stderrBuf) }()

	waitDone := make(chan error, 1)
	go func() {
		wg.Wait()
		var scanErr error
		for range [2]struct{}{} {
			if e := <-scanErrs; e != nil && scanErr == nil {
				scanErr = e
			}
		}
		cmdErr := s.shims.CmdWait(cmd)
		if scanErr != nil {
			if cmdErr != nil {
				waitDone <- errors.Join(scanErr, werror.Wrap(cmdErr, "command execution failed"))
				return
			}
			waitDone <- scanErr
			return
		}
		waitDone <- cmdErr
	}()

	output := func() string {
		bufMu.Lock()
		defer bufMu.Unlock()
		return s.scrubString(stdoutBuf.String())
	}

	interrupt := func() {
		_ = s.shims.InterruptProcessGroup(cmd)
		select {
		case <-waitDone:
			return
		case <-time.After(gracePeriod):
		}
		_ = s.shims.KillProcessGroup(cmd)
		<-waitDone
	}

	idleTimer := time.NewTimer(idleTimeout)
	defer idleTimer.Stop()
	absoluteTimer := time.NewTimer(absoluteTimeout)
	defer absoluteTimer.Stop()

	for {
		select {
		case waitErr := <-waitDone:
			if waitErr != nil {
				bufMu.Lock()
				errOut := s.scrubString(stderrBuf.String())
				bufMu.Unlock()
				return output(), execError(waitErr, errOut)
			}
			return output(), nil
		case <-idleTimer.C:
			interrupt()
			return output(), werror.Wrap(ErrCommandTimedOut, "the command printed no output for %v", idleTimeout)
		case <-absoluteTimer.C:
			interrupt()
			return output(), werror.Wrap(ErrCommandTimedOut, "the command ran for longer than %v", absoluteTimeout)
		case <-activity:
			if !idleTimer.Stop() {
				select {
				case <-idleTimer.C:
				default:
				}
			}
			idleTimer.Reset(idleTimeout)
		}
	}
}

// =============================================================================
// Private Methods
// =============================================================================

// runWithTimeout starts cmd and waits for it until timeout. It kills the process when the
// timeout passes. It returns the scrubbed stdout, and on failure it adds the scrubbed stderr.
func (s *DefaultShell) runWithTimeout(cmd *exec.Cmd, stdoutBuf, stderrBuf *bytes.Buffer, timeout time.Duration) (string, error) {
	if err := s.shims.CmdStart(cmd); err != nil {
		return "", werror.Wrap(err, "command start failed")
	}

	var waitOnce sync.Once
	execFn := func() (string, error) {
		var waitErr error
		waitOnce.Do(func() {
			waitErr = s.shims.CmdWait(cmd)
		})
		if waitErr != nil {
			return s.scrubString(stdoutBuf.String()), execError(waitErr, s.scrubString(stderrBuf.String()))
		}
		return s.scrubString(stdoutBuf.String()), nil
	}

	cleanupFn := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			waitOnce.Do(func() {
				_ = s.shims.CmdWait(cmd)
			})
		}
	}

	return executeWithTimeout(execFn, cleanupFn, timeout)
}

// =============================================================================
// Helpers
// =============================================================================

// executeWithTimeout executes a function with a timeout and ensures cleanup happens exactly once.
// It takes an execution function that returns output and error, a cleanup function to call on timeout,
// and a timeout duration. Returns the output and error from execution, or a timeout error if the timeout is exceeded.
func executeWithTimeout(execFn func() (string, error), cleanupFn func(), timeout time.Duration) (string, error) {
	type result struct {
		out string
		err error
	}
	resultChan := make(chan result, 1)
	var cleanupOnce sync.Once
	go func() {
		defer cleanupOnce.Do(cleanupFn)
		out, err := execFn()
		resultChan <- result{out: out, err: err}
	}()

	select {
	case res := <-resultChan:
		return res.out, res.err
	case <-time.After(timeout):
		cleanupOnce.Do(cleanupFn)
		return "", werror.Wrap(ErrCommandTimedOut, "the command ran for longer than %v", timeout)
	}
}
