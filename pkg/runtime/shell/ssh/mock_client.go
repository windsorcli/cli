package ssh

import (
	"io"

	gossh "golang.org/x/crypto/ssh"
)

// The MockClient is a mock implementation of the Client interface
// It provides testable alternatives to the real SSH client implementation
// It serves as a testing utility for components that depend on SSH functionality
// It supports customizable behavior through function fields for all interface methods

// =============================================================================
// Types
// =============================================================================

// MockClient is the mock implementation of the Client interface
type MockClient struct {
	DialFunc                func(network, addr string, config *ClientConfig) (ClientConn, error)
	ConnectFunc             func() (ClientConn, error)
	SetClientConfigFunc     func(config *ClientConfig)
	SetClientConfigFileFunc func(configStr, hostname string) error
}

// MockClientConn is the mock implementation of the ClientConn interface
type MockClientConn struct {
	NewSessionFunc func() (Session, error)
	CloseFunc      func() error
}

// MockSession is the mock implementation of the Session interface
type MockSession struct {
	RunFunc            func(cmd string) error
	CombinedOutputFunc func(cmd string) ([]byte, error)
	SetStdoutFunc      func(w io.Writer)
	SetStderrFunc      func(w io.Writer)
	CloseFunc          func() error
}

// MockAuthMethod is the mock implementation of the AuthMethod interface
type MockAuthMethod struct {
	MethodFunc func() any
}

// MockHostKeyCallback is the mock implementation of the HostKeyCallback interface
type MockHostKeyCallback struct {
	CallbackFunc func() any
}

// MockPublicKeyAuthMethod is the mock implementation of the PublicKeyAuthMethod interface
type MockPublicKeyAuthMethod struct {
	SignerFunc func() gossh.Signer
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockSSHClient creates a new MockClient with default function implementations
func NewMockSSHClient() *MockClient {
	return &MockClient{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Dial connects to the SSH server and returns a client connection
func (m *MockClient) Dial(network, addr string, config *ClientConfig) (ClientConn, error) {
	if m.DialFunc != nil {
		return m.DialFunc(network, addr, config)
	}
	return &MockClientConn{}, nil
}

// Connect connects to the SSH server using the provided client configuration
func (m *MockClient) Connect() (ClientConn, error) {
	if m.ConnectFunc != nil {
		return m.ConnectFunc()
	}
	return &MockClientConn{}, nil
}

// SetClientConfig sets the client configuration
func (m *MockClient) SetClientConfig(config *ClientConfig) {
	if m.SetClientConfigFunc != nil {
		m.SetClientConfigFunc(config)
	}
}

// SetClientConfigFile sets the client configuration from a file
func (m *MockClient) SetClientConfigFile(configStr, hostname string) error {
	if m.SetClientConfigFileFunc != nil {
		return m.SetClientConfigFileFunc(configStr, hostname)
	}
	return nil
}

// NewSession creates a new SSH session
func (m *MockClientConn) NewSession() (Session, error) {
	if m.NewSessionFunc != nil {
		return m.NewSessionFunc()
	}
	return &MockSession{}, nil
}

// Close closes the client connection
func (m *MockClientConn) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// Run executes a command on the remote server
func (m *MockSession) Run(cmd string) error {
	if m.RunFunc != nil {
		return m.RunFunc(cmd)
	}
	return nil
}

// CombinedOutput executes a command and returns its combined stdout and stderr
func (m *MockSession) CombinedOutput(cmd string) ([]byte, error) {
	if m.CombinedOutputFunc != nil {
		return m.CombinedOutputFunc(cmd)
	}
	return []byte("mock output"), nil
}

// SetStdout sets the stdout writer for the session
func (m *MockSession) SetStdout(w io.Writer) {
	if m.SetStdoutFunc != nil {
		m.SetStdoutFunc(w)
	}
}

// SetStderr sets the stderr writer for the session
func (m *MockSession) SetStderr(w io.Writer) {
	if m.SetStderrFunc != nil {
		m.SetStderrFunc(w)
	}
}

// Close closes the session
func (m *MockSession) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// Method returns the SSH authentication method
func (m *MockAuthMethod) Method() gossh.AuthMethod {
	if m.MethodFunc != nil {
		return m.MethodFunc().(gossh.AuthMethod)
	}
	return nil
}

// Callback returns the SSH host key callback
func (m *MockHostKeyCallback) Callback() gossh.HostKeyCallback {
	if m.CallbackFunc != nil {
		return m.CallbackFunc().(gossh.HostKeyCallback)
	}
	return nil
}

// Method returns the SSH authentication method
func (m *MockPublicKeyAuthMethod) Method() gossh.AuthMethod {
	if m.SignerFunc != nil {
		return gossh.PublicKeys(m.SignerFunc())
	}
	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure MockClient implements the Client interface
var _ Client = (*MockClient)(nil)

// Ensure MockClientConn implements the ClientConn interface
var _ ClientConn = (*MockClientConn)(nil)

// Ensure MockSession implements the Session interface
var _ Session = (*MockSession)(nil)

// Ensure MockAuthMethod implements the AuthMethod interface
var _ AuthMethod = (*MockAuthMethod)(nil)

// Ensure MockPublicKeyAuthMethod implements the AuthMethod interface
var _ AuthMethod = (*MockPublicKeyAuthMethod)(nil)

// Ensure MockHostKeyCallback implements the HostKeyCallback interface
var _ HostKeyCallback = (*MockHostKeyCallback)(nil)
