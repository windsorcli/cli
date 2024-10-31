package ssh

import (
	"io"

	gossh "golang.org/x/crypto/ssh"
)

// ClientConfig abstracts the SSH client configuration
type ClientConfig struct {
	User            string
	Auth            []AuthMethod
	HostKeyCallback HostKeyCallback
	HostName        string
	Port            string
}

// AuthMethod abstracts the SSH authentication method
type AuthMethod interface {
	Method() gossh.AuthMethod
}

// HostKeyCallback abstracts the SSH host key callback
type HostKeyCallback interface {
	Callback() gossh.HostKeyCallback
}

// Client interface abstracts the SSH client
type Client interface {
	Dial(network, addr string, config *ClientConfig) (ClientConn, error)
	Connect() (ClientConn, error)
	SetClientConfig(config *ClientConfig)
}

// ClientConn interface abstracts the SSH client connection
type ClientConn interface {
	Close() error
	NewSession() (Session, error)
}

// Session interface abstracts the SSH session
type Session interface {
	Run(cmd string) error
	CombinedOutput(cmd string) ([]byte, error)
	SetStdout(w io.Writer)
	SetStderr(w io.Writer)
	Close() error
}
