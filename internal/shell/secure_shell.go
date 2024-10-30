package shell

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

// Mockable sshClient interface
type sshClient interface {
	Close() error
	NewSession() (*ssh.Session, error)
}

// Mockable sshSession interface
type sshSession interface {
	Close() error
	Run(command string) error
}

// Aliases for ssh functions to allow mocking in tests
var (
	sshDial                  = ssh.Dial
	sshParsePrivateKey       = ssh.ParsePrivateKey
	newSession               = func(client sshClient) (sshSession, error) { return client.NewSession() }
	clientClose              = func(client sshClient) error { return client.Close() }
	sessionClose             = func(session sshSession) error { return session.Close() }
	sessionRun               = func(session sshSession, command string) error { return session.Run(command) }
	sshPublicKeysCallback    = ssh.PublicKeysCallback
	sshInsecureIgnoreHostKey = ssh.InsecureIgnoreHostKey
)

// Defines basic SSH connection parameters
type SSHConnectionParams struct {
	Host         string
	Port         int
	Username     string
	IdentityFile string
}

// SecureShell is the implementation of the Shell interface for Colima
type SecureShell struct {
	DefaultShell
	sshParams SSHConnectionParams
}

// NewSecureShell creates a new instance of SecureShell
func NewSecureShell(sshParams SSHConnectionParams) *SecureShell {
	return &SecureShell{
		DefaultShell: *NewDefaultShell(),
		sshParams:    sshParams,
	}
}

// PrintEnvVars prints the environment variables in a sorted order.
// If a custom PrintEnvVarsFn is provided, it will use that function instead.
func (d *SecureShell) PrintEnvVars(envVars map[string]string) {
	d.DefaultShell.PrintEnvVars(envVars)
}

// GetProjectRoot returns the project root directory.
// If a custom GetProjectRootFunc is provided, it will use that function instead.
func (d *SecureShell) GetProjectRoot() (string, error) {
	return d.DefaultShell.GetProjectRoot()
}

// Exec executes a command over SSH and returns its output as a string
func (d *SecureShell) Exec(verbose bool, message string, command string, args ...string) (string, error) {
	// Create the SSH client configuration
	config := &ssh.ClientConfig{
		User: d.sshParams.Username,
		Auth: []ssh.AuthMethod{
			sshPublicKeysCallback(d.publicKeysCallback),
		},
		HostKeyCallback: sshInsecureIgnoreHostKey(),
	}

	// Connect to the SSH server
	address := fmt.Sprintf("%s:%d", d.sshParams.Host, d.sshParams.Port)
	client, err := sshDial("tcp", address, config)
	if err != nil {
		return "", fmt.Errorf("failed to dial: %v", err)
	}
	defer clientClose(client)

	// Create a session
	session, err := newSession(client)
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer sessionClose(session)

	// Prepare the command
	fullCommand := fmt.Sprintf("echo %s", strings.Join(args, " "))

	// Capture the output
	var stdoutBuf bytes.Buffer

	// Run the command
	if err := sessionRun(session, fullCommand); err != nil {
		return "", fmt.Errorf("failed to run command: %v", err)
	}

	// Return the output
	return stdoutBuf.String(), nil
}

// publicKeysCallback is the function to be passed into sshPublicKeysCallback for public key authentication
func (d *SecureShell) publicKeysCallback() ([]ssh.Signer, error) {
	key, err := sshParsePrivateKey([]byte(d.sshParams.IdentityFile))
	if err != nil {
		return nil, fmt.Errorf("unable to parse private key: %v", err)
	}
	return []ssh.Signer{key}, nil
}

// Ensure secure shell is an instance of Shell
var _ Shell = (*SecureShell)(nil)
