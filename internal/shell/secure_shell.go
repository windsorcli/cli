package shell

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/ssh"
)

// SecureShell implements the Shell interface using SSH.
type SecureShell struct {
	client       ssh.Client
	clientConn   ssh.ClientConn
	projectRoot  string
	mu           sync.Mutex
	defaultShell Shell
}

// NewSecureShell creates a new instance of SecureShell and connects to the remote host.
func NewSecureShell(container di.ContainerInterface, host, user, identityFile, port string) (*SecureShell, error) {
	// Resolve the SSH client from the container
	clientInstance, err := container.Resolve("sshClient")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve SSH client: %w", err)
	}
	client, ok := clientInstance.(ssh.Client)
	if !ok {
		return nil, fmt.Errorf("resolved SSH client does not implement Client interface")
	}

	clientConn, err := client.Connect(host, user, identityFile, port)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to remote host: %w", err)
	}

	// Resolve the default shell from the container
	defaultShellInstance, err := container.Resolve("defaultShell")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve default shell: %w", err)
	}
	defaultShell, ok := defaultShellInstance.(Shell)
	if !ok {
		return nil, fmt.Errorf("resolved default shell does not implement Shell interface")
	}

	return &SecureShell{
		client:       client,
		clientConn:   clientConn,
		defaultShell: defaultShell,
	}, nil
}

// PrintEnvVars prints the provided environment variables.
func (s *SecureShell) PrintEnvVars(envVars map[string]string) {
	s.defaultShell.PrintEnvVars(envVars)
}

// GetProjectRoot retrieves the project root directory on the remote host.
func (s *SecureShell) GetProjectRoot() (string, error) {
	return s.defaultShell.GetProjectRoot()
}

// Exec executes a command on the remote host via SSH and returns its output as a string.
func (s *SecureShell) Exec(verbose bool, message string, command string, args ...string) (string, error) {
	session, err := s.clientConn.NewSession()
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
		return "", fmt.Errorf("command execution failed: %w", err)
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
