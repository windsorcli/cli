package ssh

import (
	"fmt"
	"io"

	gossh "golang.org/x/crypto/ssh"
)

// SSHClient is the real implementation of the Client interface
type SSHClient struct {
	BaseClient
}

// NewSSHClient creates a new SSHClient with the default SSH config path
func NewSSHClient() *SSHClient {
	return &SSHClient{}
}

// Dial connects to the SSH server and returns a client connection
func (c *SSHClient) Dial(network, addr string, config *ClientConfig) (ClientConn, error) {
	// Convert AuthMethods
	var authMethods []gossh.AuthMethod
	for _, am := range config.Auth {
		authMethods = append(authMethods, am.Method())
	}

	gosshConfig := &gossh.ClientConfig{
		User: config.User,
		Auth: authMethods,
		// Insecurely ignore host key checking as these are ephemeral local VMs
		// #nosec G106
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
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

// Connect connects to the SSH server using the provided client configuration
func (c *SSHClient) Connect() (ClientConn, error) {
	if c.clientConfig == nil {
		return nil, fmt.Errorf("client configuration is not set")
	}
	return c.Dial("tcp", "", c.clientConfig)
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