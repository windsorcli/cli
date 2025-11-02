package shell

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/context/shell/ssh"
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
func NewSecureShell(injector di.Injector) *SecureShell {
	defaultShell := NewDefaultShell(injector)
	return &SecureShell{
		DefaultShell: *defaultShell,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize initializes the SecureShell instance.
func (s *SecureShell) Initialize() error {
	// Get the SSH client first
	sshClient, ok := s.injector.Resolve("sshClient").(ssh.Client)
	if !ok {
		return fmt.Errorf("failed to resolve SSH client")
	}
	s.sshClient = sshClient

	return nil
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

// Ensure SecureShell implements the Shell interface
var _ Shell = (*SecureShell)(nil)
