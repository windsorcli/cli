package shell

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/ssh"
)

// SecureShell implements the Shell interface using SSH.
type SecureShell struct {
	DefaultShell
	sshClient ssh.Client
}

// NewSecureShell creates a new instance of SecureShell.
func NewSecureShell(injector di.Injector) *SecureShell {
	return &SecureShell{
		DefaultShell: DefaultShell{
			injector: injector,
		},
	}
}

// Initialize initializes the SecureShell instance.
func (s *SecureShell) Initialize() error {
	// Call the base Initialize method
	if err := s.DefaultShell.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize default shell: %w", err)
	}

	// Get the SSH client
	sshClient, ok := s.injector.Resolve("sshClient").(ssh.Client)
	if !ok {
		return fmt.Errorf("failed to resolve SSH client")
	}
	s.sshClient = sshClient

	return nil
}

// Exec executes a command on the remote host via SSH and returns its output as a string.
func (s *SecureShell) Exec(command string, args ...string) (string, int, error) {
	clientConn, err := s.sshClient.Connect()
	if err != nil {
		return "", 0, fmt.Errorf("failed to connect to SSH client: %w", err)
	}
	defer clientConn.Close()

	session, err := clientConn.NewSession()
	if err != nil {
		return "", 0, fmt.Errorf("failed to create SSH session: %w", err)
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
	err = session.Run(fullCommand)
	exitCode := 0
	if err != nil {
		// Since ssh.ExitError is not defined, we will assume a non-zero exit code on error
		exitCode = 1
		return stdoutBuf.String(), exitCode, fmt.Errorf("command execution failed: %w\n%s", err, stderrBuf.String())
	}

	return stdoutBuf.String(), exitCode, nil
}

// Ensure SecureShell implements the Shell interface
var _ Shell = (*SecureShell)(nil)
