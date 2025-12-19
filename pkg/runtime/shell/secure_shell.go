package shell

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/windsorcli/cli/pkg/runtime/shell/ssh"
)

// The SecureShell is a secure implementation of the Shell interface using SSH.
// It provides remote command execution capabilities through an SSH connection.
// It enables secure operations on remote systems by establishing an encrypted connection.
// Key features include remote command execution with proper error handling and resource cleanup.

// =============================================================================
// Types
// =============================================================================

type SecureShell struct {
	DefaultShell
	sshClient ssh.Client
}

// =============================================================================
// Constructor
// =============================================================================

// NewSecureShell creates a new instance of SecureShell.
func NewSecureShell(sshClient ssh.Client) *SecureShell {
	defaultShell := NewDefaultShell()
	return &SecureShell{
		DefaultShell: *defaultShell,
		sshClient:    sshClient,
	}
}

// Exec executes a command on the remote host via SSH and returns its output as a string.
func (s *SecureShell) Exec(command string, args ...string) (string, error) {
	clientConn, err := s.sshClient.Connect()
	if err != nil {
		return "", fmt.Errorf("failed to connect to SSH client: %w", err)
	}
	defer clientConn.Close()

	session, err := clientConn.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	// Concatenate command and arguments
	fullCommand := command
	if len(args) > 0 {
		fullCommand += " " + strings.Join(args, " ")
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	session.SetStdout(&stdoutBuf)
	session.SetStderr(&stderrBuf)

	// Run the command and wait for it to finish
	if err := session.Run(fullCommand); err != nil {
		return "", fmt.Errorf("command execution failed: %w\n%s", err, stderrBuf.String())
	}

	return stdoutBuf.String(), nil
}

// ExecProgress executes a command and returns its output as a string
func (s *SecureShell) ExecProgress(message string, command string, args ...string) (string, error) {
	// Not yet implemented for SecureShell
	return s.Exec(command, args...)
}

// ExecSilent executes a command and returns its output as a string without printing to stdout or stderr
func (s *SecureShell) ExecSilent(command string, args ...string) (string, error) {
	// Not yet implemented for SecureShell
	return s.Exec(command, args...)
}

// ExecSilentWithTimeout executes a command with a timeout and returns the output.
func (s *SecureShell) ExecSilentWithTimeout(command string, args []string, timeout time.Duration) (string, error) {
	return s.DefaultShell.ExecSilentWithTimeout(command, args, timeout)
}

// Ensure SecureShell implements the Shell interface
var _ Shell = (*SecureShell)(nil)
