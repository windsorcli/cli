// internal/ssh/real_client.go
package ssh

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	gossh "golang.org/x/crypto/ssh"
)

// SSHClient is the real implementation of the Client interface
type SSHClient struct {
	sshConfigPath string
}

// NewSSHClient creates a new SSHClient with the default SSH config path
func NewSSHClient() *SSHClient {
	sshConfigPath := filepath.Join(os.Getenv("HOME"), ".ssh", "config")
	return &SSHClient{
		sshConfigPath: sshConfigPath,
	}
}

// Dial connects to the SSH server and returns a client connection
func (c *SSHClient) Dial(network, addr string, config *ClientConfig) (ClientConn, error) {
	// Convert AuthMethods
	var authMethods []gossh.AuthMethod
	for _, am := range config.Auth {
		authMethods = append(authMethods, am.Method())
	}

	gosshConfig := &gossh.ClientConfig{
		User:            config.User,
		Auth:            authMethods,
		HostKeyCallback: config.HostKeyCallback.Callback(),
	}

	// Construct the address using HostName and Port from config
	if addr == "" {
		addr = fmt.Sprintf("%s:%s", config.HostName, config.Port)
	}

	client, err := gossh.Dial(network, addr, gosshConfig)
	if err != nil {
		return nil, err
	}
	return &RealClientConn{client: client}, nil
}

// Connect connects to the SSH server using the provided host, user, and identity file
func (c *SSHClient) Connect(host, user, identityFile, port string) (ClientConn, error) {
	config, err := c.NewClientConfig(host, user, identityFile, port)
	if err != nil {
		return nil, err
	}
	return c.Dial("tcp", "", config)
}

// NewClientConfig creates a ClientConfig using the provided host, user, identity file, and port
func (c *SSHClient) NewClientConfig(host, user, identityFile, port string) (*ClientConfig, error) {
	// Set default values if necessary
	if user == "" {
		user = os.Getenv("USER")
	}
	if identityFile == "" {
		identityFile = filepath.Join(os.Getenv("HOME"), ".ssh", "id_rsa")
	}
	if port == "" {
		port = "22"
	}

	// Read the private key file
	key, err := os.ReadFile(identityFile)
	if err != nil {
		return nil, fmt.Errorf("unable to read identity file: %w", err)
	}

	// Parse the private key
	signer, err := gossh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("unable to parse private key: %w", err)
	}

	// Create AuthMethod
	authMethod := &PublicKeyAuthMethod{signer: signer}

	// Create HostKeyCallback
	hostKeyCallback := &InsecureIgnoreHostKeyCallback{}
	// In production, replace the above with a proper host key callback

	// Create ClientConfig
	clientConfig := &ClientConfig{
		User:            user,
		Auth:            []AuthMethod{authMethod},
		HostKeyCallback: hostKeyCallback,
		HostName:        host,
		Port:            port,
	}

	return clientConfig, nil
}

// RealClientConn wraps *gossh.Client and implements the ClientConn interface
type RealClientConn struct {
	client *gossh.Client
}

// Close closes the client connection
func (c *RealClientConn) Close() error {
	return c.client.Close()
}

// NewSession creates a new SSH session
func (c *RealClientConn) NewSession() (Session, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return nil, err
	}
	return &RealSession{session: session}, nil
}

// RealSession wraps *gossh.Session and implements the Session interface
type RealSession struct {
	session *gossh.Session
}

func (s *RealSession) Run(cmd string) error {
	return s.session.Run(cmd)
}

func (s *RealSession) CombinedOutput(cmd string) ([]byte, error) {
	return s.session.CombinedOutput(cmd)
}

func (s *RealSession) SetStdout(w io.Writer) {
	s.session.Stdout = w
}

func (s *RealSession) SetStderr(w io.Writer) {
	s.session.Stderr = w
}

func (s *RealSession) Close() error {
	return s.session.Close()
}

// Ensure SSHClient implements the Client interface
var _ Client = (*SSHClient)(nil)

// Ensure RealClientConn implements the ClientConn interface
var _ ClientConn = (*RealClientConn)(nil)

// Ensure RealSession implements the Session interface
var _ Session = (*RealSession)(nil)

// PublicKeyAuthMethod implements the AuthMethod interface using a public key
type PublicKeyAuthMethod struct {
	signer gossh.Signer
}

func (p *PublicKeyAuthMethod) Method() gossh.AuthMethod {
	return gossh.PublicKeys(p.signer)
}

// InsecureIgnoreHostKeyCallback implements HostKeyCallback and ignores host key checking
type InsecureIgnoreHostKeyCallback struct{}

func (h *InsecureIgnoreHostKeyCallback) Callback() gossh.HostKeyCallback {
	return gossh.InsecureIgnoreHostKey()
}
