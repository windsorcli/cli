package ssh

import (
	"io"

	gossh "golang.org/x/crypto/ssh"
)

// MockClient is the mock implementation of the Client interface
type MockClient struct {
	DialFunc                func(network, addr string, config *ClientConfig) (ClientConn, error)
	ConnectFunc             func() (ClientConn, error)
	SetClientConfigFunc     func(config *ClientConfig)
	SetClientConfigFileFunc func(configStr, hostname string) error
}

func (m *MockClient) Dial(network, addr string, config *ClientConfig) (ClientConn, error) {
	if m.DialFunc != nil {
		return m.DialFunc(network, addr, config)
	}
	return &MockClientConn{}, nil
}

func (m *MockClient) Connect() (ClientConn, error) {
	if m.ConnectFunc != nil {
		return m.ConnectFunc()
	}
	return &MockClientConn{}, nil
}

func (m *MockClient) SetClientConfig(config *ClientConfig) {
	if m.SetClientConfigFunc != nil {
		m.SetClientConfigFunc(config)
	}
}

func (m *MockClient) SetClientConfigFile(configStr, hostname string) error {
	if m.SetClientConfigFileFunc != nil {
		return m.SetClientConfigFileFunc(configStr, hostname)
	}
	return nil
}

// NewMockSSHClient creates a new MockClient with default function implementations
func NewMockSSHClient() *MockClient {
	return &MockClient{}
}

// MockClientConn is the mock implementation of the ClientConn interface
type MockClientConn struct {
	NewSessionFunc func() (Session, error)
	CloseFunc      func() error
}

func (m *MockClientConn) NewSession() (Session, error) {
	if m.NewSessionFunc != nil {
		return m.NewSessionFunc()
	}
	return &MockSession{}, nil
}

func (m *MockClientConn) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// MockSession is the mock implementation of the Session interface
type MockSession struct {
	RunFunc            func(cmd string) error
	CombinedOutputFunc func(cmd string) ([]byte, error)
	SetStdoutFunc      func(w io.Writer)
	SetStderrFunc      func(w io.Writer)
	CloseFunc          func() error
}

func (m *MockSession) Run(cmd string) error {
	if m.RunFunc != nil {
		return m.RunFunc(cmd)
	}
	return nil
}

func (m *MockSession) CombinedOutput(cmd string) ([]byte, error) {
	if m.CombinedOutputFunc != nil {
		return m.CombinedOutputFunc(cmd)
	}
	return []byte("mock output"), nil
}

func (m *MockSession) SetStdout(w io.Writer) {
	if m.SetStdoutFunc != nil {
		m.SetStdoutFunc(w)
	}
}

func (m *MockSession) SetStderr(w io.Writer) {
	if m.SetStderrFunc != nil {
		m.SetStderrFunc(w)
	}
}

func (m *MockSession) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// MockAuthMethod is the mock implementation of the AuthMethod interface
type MockAuthMethod struct {
	MethodFunc func() interface{}
}

func (m *MockAuthMethod) Method() gossh.AuthMethod {
	if m.MethodFunc != nil {
		return m.MethodFunc().(gossh.AuthMethod)
	}
	return nil
}

// MockHostKeyCallback is the mock implementation of the HostKeyCallback interface
type MockHostKeyCallback struct {
	CallbackFunc func() interface{}
}

func (m *MockHostKeyCallback) Callback() gossh.HostKeyCallback {
	if m.CallbackFunc != nil {
		return m.CallbackFunc().(gossh.HostKeyCallback)
	}
	return nil
}

// MockPublicKeyAuthMethod is the mock implementation of the PublicKeyAuthMethod interface
type MockPublicKeyAuthMethod struct {
	SignerFunc func() gossh.Signer
}

func (m *MockPublicKeyAuthMethod) Method() gossh.AuthMethod {
	if m.SignerFunc != nil {
		return gossh.PublicKeys(m.SignerFunc())
	}
	return nil
}

// MockInsecureIgnoreHostKeyCallback is the mock implementation of the InsecureIgnoreHostKeyCallback interface
type MockInsecureIgnoreHostKeyCallback struct {
	CallbackFunc func() gossh.HostKeyCallback
}

func (m *MockInsecureIgnoreHostKeyCallback) Callback() gossh.HostKeyCallback {
	if m.CallbackFunc != nil {
		return m.CallbackFunc()
	}
	return gossh.InsecureIgnoreHostKey()
}

// Ensure MockClient implements the Client interface
var _ Client = (*MockClient)(nil)

// Ensure MockClientConn implements the ClientConn interface
var _ ClientConn = (*MockClientConn)(nil)

// Ensure MockSession implements the Session interface
var _ Session = (*MockSession)(nil)

// Ensure MockAuthMethod implements the AuthMethod interface
var _ AuthMethod = (*MockAuthMethod)(nil)

// Ensure MockHostKeyCallback implements the HostKeyCallback interface
var _ HostKeyCallback = (*MockHostKeyCallback)(nil)

// Ensure MockPublicKeyAuthMethod implements the AuthMethod interface
var _ AuthMethod = (*MockPublicKeyAuthMethod)(nil)

// Ensure MockInsecureIgnoreHostKeyCallback implements the HostKeyCallback interface
var _ HostKeyCallback = (*MockInsecureIgnoreHostKeyCallback)(nil)
