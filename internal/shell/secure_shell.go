package shell

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/ssh"
)

// SecureShell implements the Shell interface using SSH.
type SecureShell struct {
	container di.ContainerInterface
	Shell
}

// NewSecureShell creates a new instance of SecureShell.
func NewSecureShell(container di.ContainerInterface) (*SecureShell, error) {
	return &SecureShell{
		container: container,
	}, nil
}

// Exec executes a command on the remote host via SSH and returns its output as a string.
func (s *SecureShell) Exec(verbose bool, message string, command string, args ...string) (string, error) {
	clientInstance, err := s.container.Resolve("sshClient")
	if err != nil {
		return "", fmt.Errorf("failed to resolve SSH client: %w", err)
	}
	client, ok := clientInstance.(ssh.Client)
	if !ok {
		return "", fmt.Errorf("resolved SSH client does not implement Client interface")
	}

	clientConn, err := client.Connect()
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

	// Initialize spinner
	spr := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	spr.Suffix = " " + message
	spr.Start()

	err = session.Run(fullCommand)

	spr.Stop()

	if err != nil {
		// Print stderr on error
		fmt.Print(stderrBuf.String())
		return "", fmt.Errorf("command execution failed: %w, stderr: %s", err, stderrBuf.String())
	}

	output := stdoutBuf.String()

	if verbose {
		// Print stdout if verbose
		fmt.Print(output)
	}

	return output, nil
}

// Ensure SecureShell implements the Shell interface
var _ Shell = (*SecureShell)(nil)
