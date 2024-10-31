package shell

import (
	"fmt"
	"os"
	"strings"

	"github.com/windsor-hotel/cli/internal/di"
	sshWrapper "github.com/windsor-hotel/cli/internal/ssh"
)

// SSHConnectionParams defines basic SSH connection parameters
type SSHConnectionParams struct {
	Host         string
	Port         int
	Username     string
	IdentityFile string
}

// SecureShell implements the Shell interface for SSH
type SecureShell struct {
	DefaultShell
	sshParams      SSHConnectionParams
	client         sshWrapper.Client
	authMethod     sshWrapper.AuthMethod
	hostKeyChecker sshWrapper.HostKeyCallback
}

// NewSecureShell creates a new instance of SecureShell using the DI container
func NewSecureShell(di *di.DIContainer) (*SecureShell, error) {
	// Resolve SSH connection parameters
	sshParamsInterface, err := di.Resolve("sshParams")
	if err != nil {
		return nil, fmt.Errorf("error resolving sshParams: %w", err)
	}
	sshParams := sshParamsInterface.(SSHConnectionParams)

	// Resolve SSH client
	sshClientInterface, err := di.Resolve("sshClient")
	if err != nil {
		return nil, fmt.Errorf("error resolving sshClient: %w", err)
	}
	sshClient := sshClientInterface.(sshWrapper.Client)

	// Resolve authentication method
	authMethodInterface, err := di.Resolve("sshAuthMethod")
	if err != nil {
		return nil, fmt.Errorf("error resolving sshAuthMethod: %w", err)
	}
	authMethod := authMethodInterface.(sshWrapper.AuthMethod)

	// Resolve host key callback
	hostKeyCallbackInterface, err := di.Resolve("sshHostKeyCallback")
	if err != nil {
		return nil, fmt.Errorf("error resolving sshHostKeyCallback: %w", err)
	}
	hostKeyCallback := hostKeyCallbackInterface.(sshWrapper.HostKeyCallback)

	return &SecureShell{
		DefaultShell:   *NewDefaultShell(),
		sshParams:      sshParams,
		client:         sshClient,
		authMethod:     authMethod,
		hostKeyChecker: hostKeyCallback,
	}, nil
}

// Exec executes a command over SSH and returns its output as a string
func (s *SecureShell) Exec(verbose bool, message string, command string, args ...string) (string, error) {
	// Create the SSH client configuration using the injected authentication method and host key callback
	config := &sshWrapper.ClientConfig{
		User:            s.sshParams.Username,
		Auth:            []sshWrapper.AuthMethod{s.authMethod},
		HostKeyCallback: s.hostKeyChecker,
	}

	// Connect to the SSH server using the injected client
	address := fmt.Sprintf("%s:%d", s.sshParams.Host, s.sshParams.Port)
	clientConn, err := s.client.Dial("tcp", address, config)
	if err != nil {
		return "", fmt.Errorf("failed to dial: %v", err)
	}
	defer clientConn.Close()

	// Create a session
	session, err := clientConn.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	// Build the full command
	fullCommand := fmt.Sprintf("%s %s", command, strings.Join(args, " "))

	if verbose {
		// Set session output to stdout
		session.SetStdout(os.Stdout)
		session.SetStderr(os.Stderr)

		// Run the command
		if err := session.Run(fullCommand); err != nil {
			return "", fmt.Errorf("failed to run command: %v", err)
		}
		return "", nil
	} else {
		// Run the command and capture output
		output, err := session.CombinedOutput(fullCommand)
		if err != nil {
			return "", fmt.Errorf("failed to run command: %v", err)
		}
		return string(output), nil
	}
}

// Ensure SecureShell implements the Shell interface
var _ Shell = (*SecureShell)(nil)
