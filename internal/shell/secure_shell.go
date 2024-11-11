package shell

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/windsor-hotel/cli/internal/di"
)

// SecureShell implements the Shell interface using SSH.
type SecureShell struct {
	DefaultShell
}

// NewSecureShell creates a new instance of SecureShell.
func NewSecureShell(injector di.Injector) (*SecureShell, error) {
	return &SecureShell{
		DefaultShell: DefaultShell{
			injector: injector,
		},
	}, nil
}

// Exec executes a command on the remote host via SSH and returns its output as a string.
func (s *SecureShell) Exec(verbose bool, message string, command string, args ...string) (string, error) {
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

	if verbose {
		fmt.Print(output)
	}

	return output, nil
}

// Ensure SecureShell implements the Shell interface
var _ Shell = (*SecureShell)(nil)
