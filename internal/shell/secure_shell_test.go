package shell

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"testing"

	"golang.org/x/crypto/ssh"
)

// Mock signer for sshParsePrivateKey
type mockSigner struct{}

func (m *mockSigner) PublicKey() ssh.PublicKey {
	return nil
}

func (m *mockSigner) Sign(rand io.Reader, data []byte) (*ssh.Signature, error) {
	return nil, nil
}

// Mock ssh.Client with Close method
type mockClient struct {
	ssh.Client
}

func (m *mockClient) Close() error {
	return nil
}

func (m *mockClient) NewSession() (*ssh.Session, error) {
	return &ssh.Session{}, nil
}

// Mock ssh.Session with Close method
type mockSession struct {
	Stdout *bytes.Buffer
}

func (m *mockSession) Close() error {
	return nil
}

func (m *mockSession) Run(command string) error {
	if m.Stdout != nil {
		m.Stdout.WriteString("mocked output\n")
	}
	return nil
}

func createSecureShellMocks() (func(), func()) {
	originalSSHParsePrivateKey := sshParsePrivateKey
	originalNewSession := newSession
	originalSSHDial := sshDial
	originalClientClose := clientClose
	originalSessionClose := sessionClose
	originalSessionRun := sessionRun
	originalSSHPublicKeysCallback := sshPublicKeysCallback
	originalSSHInsecureIgnoreHostKey := sshInsecureIgnoreHostKey

	resetMocks := func() {
		sshParsePrivateKey = originalSSHParsePrivateKey
		newSession = originalNewSession
		sshDial = originalSSHDial
		clientClose = originalClientClose
		sessionClose = originalSessionClose
		sessionRun = originalSessionRun
		sshPublicKeysCallback = originalSSHPublicKeysCallback
		sshInsecureIgnoreHostKey = originalSSHInsecureIgnoreHostKey
	}

	setSafeMocks := func() {
		sshParsePrivateKey = func(pemBytes []byte) (ssh.Signer, error) {
			return &mockSigner{}, nil
		}
		newSession = func(client sshClient) (sshSession, error) {
			return &mockSession{}, nil
		}
		sshDial = func(network, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
			return &ssh.Client{}, nil
		}
		clientClose = func(client sshClient) error {
			return nil
		}
		sessionClose = func(session sshSession) error {
			return nil
		}
		sessionRun = func(session sshSession, command string) error {
			return nil
		}
		sshPublicKeysCallback = func(getSigners func() (signers []ssh.Signer, err error)) ssh.AuthMethod {
			return ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
				return []ssh.Signer{&mockSigner{}}, nil
			})
		}
		sshInsecureIgnoreHostKey = func() ssh.HostKeyCallback {
			return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
				return nil
			}
		}
	}

	return setSafeMocks, resetMocks
}

func TestSecureShell_NewSecureShell(t *testing.T) {
	t.Run("ValidSSHParams", func(t *testing.T) {
		// Given valid SSH connection parameters
		sshParams := SSHConnectionParams{
			Host:         "localhost",
			Port:         22,
			Username:     "user",
			IdentityFile: "/path/to/identity/file",
		}
		// When creating a new secure shell
		secureShell := NewSecureShell(sshParams)
		// Then no error should be returned
		if secureShell == nil {
			t.Errorf("Expected secureShell, got nil")
		}
	})
}

func TestSecureShell_PrintEnvVars(t *testing.T) {
	envVars := map[string]string{
		"VAR1": "value1",
		"VAR2": "value2",
	}

	t.Run("DefaultPrintEnvVars", func(t *testing.T) {
		// Given a secure shell with default PrintEnvVars implementation
		sshParams := SSHConnectionParams{}
		secureShell := NewSecureShell(sshParams)
		// When calling PrintEnvVars
		output := captureStdout(t, func() {
			secureShell.PrintEnvVars(envVars)
		})
		// Then the output should match the expected output
		expectedOutput := "export VAR1=\"value1\"\nexport VAR2=\"value2\"\n"
		if output != expectedOutput {
			t.Errorf("PrintEnvVars() output = %v, want %v", output, expectedOutput)
		}
	})
}

func TestSecureShell_GetProjectRoot(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a secure shell that successfully retrieves the project root
		sshParams := SSHConnectionParams{}
		secureShell := NewSecureShell(sshParams)
		// When calling GetProjectRoot
		got, err := secureShell.GetProjectRoot()
		// Then the project root should be returned without error
		if err != nil {
			t.Errorf("GetProjectRoot() error = %v, want nil", err)
		}
		if got == "" {
			t.Errorf("GetProjectRoot() got = %v, want non-empty string", got)
		}
	})
}

func TestSecureShell_Exec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		setSafeMocks, resetMocks := createSecureShellMocks()
		setSafeMocks()
		defer resetMocks()

		// Mock the sshDial function to return a mock client
		sshDial = func(network, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
			return &ssh.Client{}, nil
		}

		// Mock the newSession function to return a mock session
		newSession = func(client sshClient) (sshSession, error) {
			return &ssh.Session{}, nil
		}
		// Mock the sessionRun function to simulate command execution
		sessionRun = func(_ sshSession, _ string) error {
			return nil
		}

		// Mock the sshParsePrivateKey function to return a mock signer
		mockSigner := &mockSigner{}
		sshParsePrivateKey = func(pemBytes []byte) (ssh.Signer, error) {
			return mockSigner, nil
		}

		// Given a secure shell with a custom Exec implementation
		sshParams := SSHConnectionParams{
			Host:         "localhost",
			Port:         22,
			Username:     "user",
			IdentityFile: "mocked_identity_file",
		}
		secureShell := NewSecureShell(sshParams)

		// When calling Exec
		output, err := secureShell.Exec(false, "Executing command", "echo", "mocked output")

		// Then no error should be returned and output should be as expected
		if err != nil {
			t.Errorf("Exec() error = %v, want nil", err)
		}
		expectedOutput := ""
		if output != expectedOutput {
			t.Errorf("Exec() output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("FailRunCommand", func(t *testing.T) {
		setSafeMocks, resetMocks := createSecureShellMocks()
		setSafeMocks()
		defer resetMocks()

		// Mock the sessionRun function to return an error
		sessionRun = func(_ sshSession, _ string) error {
			return fmt.Errorf("failed to run command")
		}

		// Mock the sshParsePrivateKey function to return a mock signer
		mockSigner := &mockSigner{}
		sshParsePrivateKey = func(pemBytes []byte) (ssh.Signer, error) {
			return mockSigner, nil
		}

		// Given a secure shell with a custom Exec implementation
		sshParams := SSHConnectionParams{
			Host:         "localhost",
			Port:         22,
			Username:     "user",
			IdentityFile: "mocked_identity_file",
		}
		secureShell := NewSecureShell(sshParams)

		// When calling Exec
		_, err := secureShell.Exec(false, "Executing command", "echo", "mocked output")

		// Then an error should be returned
		if err == nil {
			t.Errorf("Exec() error = nil, want non-nil")
		}
	})

	t.Run("FailToDial", func(t *testing.T) {
		setSafeMocks, resetMocks := createSecureShellMocks()
		setSafeMocks()
		defer resetMocks()

		// Mock the sshDial function to return an error
		sshDial = func(network, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
			return nil, fmt.Errorf("failed to dial")
		}

		// Mock the sshParsePrivateKey function to return a mock signer
		mockSigner := &mockSigner{}
		sshParsePrivateKey = func(pemBytes []byte) (ssh.Signer, error) {
			return mockSigner, nil
		}

		// Given a secure shell with a custom Exec implementation
		sshParams := SSHConnectionParams{
			Host:         "localhost",
			Port:         22,
			Username:     "user",
			IdentityFile: "mocked_identity_file",
		}
		secureShell := NewSecureShell(sshParams)

		// When calling Exec
		_, err := secureShell.Exec(false, "Executing command", "echo", "mocked output")

		// Then an error should be returned
		if err == nil {
			t.Errorf("Exec() error = nil, want non-nil")
		}
	})

	t.Run("FailToCreateSession", func(t *testing.T) {
		setSafeMocks, resetMocks := createSecureShellMocks()
		setSafeMocks()
		defer resetMocks()

		// Mock the NewSession function to return an error
		newSession = func(client sshClient) (sshSession, error) {
			return nil, fmt.Errorf("failed to create session")
		}

		// Mock the sshParsePrivateKey function to return a mock signer
		mockSigner := &mockSigner{}
		sshParsePrivateKey = func(pemBytes []byte) (ssh.Signer, error) {
			return mockSigner, nil
		}

		// Given a secure shell with a custom Exec implementation
		sshParams := SSHConnectionParams{
			Host:         "localhost",
			Port:         22,
			Username:     "user",
			IdentityFile: "mocked_identity_file",
		}
		secureShell := NewSecureShell(sshParams)

		// When calling Exec
		_, err := secureShell.Exec(false, "Executing command", "echo", "mocked output")

		// Then an error should be returned
		if err == nil {
			t.Errorf("Exec() error = nil, want non-nil")
		}
	})
}

func TestSecureShell_publicKeysCallback(t *testing.T) {
	setSafeMocks, resetMocks := createSecureShellMocks()
	setSafeMocks()
	defer resetMocks()

	// Given a secure shell with a custom publicKeysCallback implementation
	sshParams := SSHConnectionParams{
		Host:         "localhost",
		Port:         22,
		Username:     "user",
		IdentityFile: "mocked_identity_file",
	}
	secureShell := NewSecureShell(sshParams)

	t.Run("ValidSigner", func(t *testing.T) {
		// Mock the sshParsePrivateKey function to return a mock signer
		mockSigner := &mockSigner{}
		sshParsePrivateKey = func(pemBytes []byte) (ssh.Signer, error) {
			return mockSigner, nil
		}

		// When calling publicKeysCallback
		signers, err := secureShell.publicKeysCallback()

		// Then the signers should not be nil and error should be nil
		if err != nil {
			t.Errorf("publicKeysCallback() error = %v, want nil", err)
		}
		if signers == nil {
			t.Errorf("publicKeysCallback() signers = nil, want non-nil")
		}
	})

	t.Run("InvalidSigner", func(t *testing.T) {
		// Mock the sshParsePrivateKey function to return an error
		sshParsePrivateKey = func(pemBytes []byte) (ssh.Signer, error) {
			return nil, fmt.Errorf("unable to parse private key")
		}

		// When calling publicKeysCallback again
		signers, err := secureShell.publicKeysCallback()

		// Then the signers should be nil and error should not be nil
		if err == nil {
			t.Errorf("publicKeysCallback() error = nil, want non-nil")
		}
		if signers != nil {
			t.Errorf("publicKeysCallback() signers = %v, want nil", signers)
		}
	})
}

func TestSecureShell_FunctionCalls(t *testing.T) {
	t.Run("TestNewSessionCall", func(t *testing.T) {
		client := &mockClient{}
		_, err := newSession(client)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("TestClientCloseCall", func(t *testing.T) {
		client := &mockClient{}
		err := clientClose(client)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("TestSessionCloseCall", func(t *testing.T) {
		session := &mockSession{}
		err := sessionClose(session)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("TestSessionRunCall", func(t *testing.T) {
		session := &mockSession{}
		err := sessionRun(session, "echo test")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}
