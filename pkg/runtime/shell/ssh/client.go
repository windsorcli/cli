package ssh

import (
	"fmt"
	"io"
	"strings"

	gossh "golang.org/x/crypto/ssh"
)

// The BaseClient is a base implementation of the Client interface
// It provides common functionality for SSH client implementations
// It serves as the foundation for both real and mock SSH clients
// It handles client configuration management and SSH config parsing

// =============================================================================
// Types
// =============================================================================

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
	SetClientConfigFile(configStr, hostname string) error
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

// BaseClient provides a base implementation of the Client interface
type BaseClient struct {
	clientConfig *ClientConfig
}

// =============================================================================
// Public Methods
// =============================================================================

// SetClientConfig sets the client configuration for the SSH client
func (c *BaseClient) SetClientConfig(config *ClientConfig) {
	c.clientConfig = config
}

// SetClientConfigFile sets the client configuration from a string (either a filename or config content) and a hostname
func (c *BaseClient) SetClientConfigFile(configStr, hostname string) error {
	var configContent string
	if _, err := stat(configStr); err == nil {
		// It's a file
		content, err := readFile(configStr)
		if err != nil {
			return err
		}
		configContent = string(content)
	} else {
		// It's a config content
		configContent = configStr
	}

	clientConfig, err := parseSSHConfig(configContent, hostname)
	if err != nil {
		return err
	}
	c.clientConfig = clientConfig
	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// parseSSHConfig parses the SSH config content and extracts the ClientConfig for the given hostname
func parseSSHConfig(configContent, hostname string) (*ClientConfig, error) {
	lines := strings.Split(configContent, "\n")
	var clientConfig ClientConfig
	var currentHost string
	var configFilled bool

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}
		if strings.HasPrefix(line, "Host ") {
			currentHost = strings.TrimSpace(strings.TrimPrefix(line, "Host"))
		} else if currentHost == hostname {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			key := fields[0]
			value := strings.Join(fields[1:], " ")
			value = strings.Trim(value, "\"") // Remove quotes if present
			switch key {
			case "IdentityFile":
				signer, err := loadSigner(value)
				if err != nil {
					return nil, err
				}
				clientConfig.Auth = append(clientConfig.Auth, &PublicKeyAuthMethod{signer: signer})
				configFilled = true
			case "User":
				clientConfig.User = value
				configFilled = true
			case "Hostname":
				clientConfig.HostName = value
				configFilled = true
			case "Port":
				clientConfig.Port = value
				configFilled = true
			}
		}
	}

	if !configFilled {
		return nil, fmt.Errorf("failed to parse SSH config for host: %s", hostname)
	}

	return &clientConfig, nil
}

// loadSigner loads the signer from the identity file
func loadSigner(identityFile string) (gossh.Signer, error) {
	key, err := readFile(identityFile)
	if err != nil {
		return nil, err
	}
	signer, err := gossh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}
	return signer, nil
}
