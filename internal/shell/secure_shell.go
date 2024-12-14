package shell

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/ssh"
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
func (s *SecureShell) Exec(message string, command string, args ...string) (string, error) {
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

	// Always print the message if it is not empty
	if message != "" {
		fmt.Println(message)
	}

	// Start the command and handle errors
	errChan := make(chan error, 1)
	go func() {
		if err := session.Run(fullCommand); err != nil {
			errChan <- fmt.Errorf("command execution failed: %w\n%s", err, stderrBuf.String())
			return
		}
		errChan <- nil
	}()

	// Wait for the command to finish or an error to occur
	select {
	case err := <-errChan:
		if err != nil {
			return "", err
		}
	}

	output := stdoutBuf.String()

	return output, nil
}

// Ensure SecureShell implements the Shell interface
var _ Shell = (*SecureShell)(nil)
