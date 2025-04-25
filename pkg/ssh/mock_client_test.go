package ssh

import (
	"bytes"
	"errors"
	"io"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

// The MockClientTest is a test suite for the MockClient implementation.
// It provides comprehensive testing of the mock SSH client functionality.
// The MockClientTest ensures that mock implementations behave as expected.
// Key features include testing of all mock methods and interface compliance.

// =============================================================================
// Test Methods
// =============================================================================

func TestMockClient_Dial(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock client with a custom DialFunc
		client := NewMockSSHClient()
		expectedConn := &MockClientConn{}
		client.DialFunc = func(network, addr string, config *ClientConfig) (ClientConn, error) {
			return expectedConn, nil
		}

		// When calling Dial
		conn, err := client.Dial("tcp", "localhost:22", &ClientConfig{})

		// Then it should return the expected connection without error
		if conn != expectedConn {
			t.Errorf("Dial() conn = %v, want %v", conn, expectedConn)
		}
		if err != nil {
			t.Errorf("Dial() err = %v, want nil", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock client with a custom DialFunc that returns an error
		client := NewMockSSHClient()
		expectedErr := errors.New("dial error")
		client.DialFunc = func(network, addr string, config *ClientConfig) (ClientConn, error) {
			return nil, expectedErr
		}

		// When calling Dial
		conn, err := client.Dial("tcp", "localhost:22", &ClientConfig{})

		// Then it should return nil connection and the expected error
		if conn != nil {
			t.Errorf("Dial() conn = %v, want nil", conn)
		}
		if err != expectedErr {
			t.Errorf("Dial() err = %v, want %v", err, expectedErr)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock client with no DialFunc implementation
		client := NewMockSSHClient()

		// When calling Dial
		conn, err := client.Dial("tcp", "localhost:22", &ClientConfig{})

		// Then it should return an empty connection and no error
		if conn == nil {
			t.Error("Dial() returned nil connection")
		}
		if err != nil {
			t.Errorf("Dial() err = %v, want nil", err)
		}
	})
}

func TestMockClient_Connect(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock client with a custom ConnectFunc
		client := NewMockSSHClient()
		expectedConn := &MockClientConn{}
		client.ConnectFunc = func() (ClientConn, error) {
			return expectedConn, nil
		}

		// When calling Connect
		conn, err := client.Connect()

		// Then it should return the expected connection without error
		if conn != expectedConn {
			t.Errorf("Connect() conn = %v, want %v", conn, expectedConn)
		}
		if err != nil {
			t.Errorf("Connect() err = %v, want nil", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock client with a custom ConnectFunc that returns an error
		client := NewMockSSHClient()
		expectedErr := errors.New("connect error")
		client.ConnectFunc = func() (ClientConn, error) {
			return nil, expectedErr
		}

		// When calling Connect
		conn, err := client.Connect()

		// Then it should return nil connection and the expected error
		if conn != nil {
			t.Errorf("Connect() conn = %v, want nil", conn)
		}
		if err != expectedErr {
			t.Errorf("Connect() err = %v, want %v", err, expectedErr)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock client with no ConnectFunc implementation
		client := NewMockSSHClient()

		// When calling Connect
		conn, err := client.Connect()

		// Then it should return an empty connection and no error
		if conn == nil {
			t.Error("Connect() returned nil connection")
		}
		if err != nil {
			t.Errorf("Connect() err = %v, want nil", err)
		}
	})
}

func TestMockClient_SetClientConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock client with a custom SetClientConfigFunc
		client := NewMockSSHClient()
		expectedConfig := &ClientConfig{User: "test"}
		var actualConfig *ClientConfig
		client.SetClientConfigFunc = func(config *ClientConfig) {
			actualConfig = config
		}

		// When calling SetClientConfig
		client.SetClientConfig(expectedConfig)

		// Then it should pass the config to the function
		if actualConfig != expectedConfig {
			t.Errorf("SetClientConfig() config = %v, want %v", actualConfig, expectedConfig)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock client with no SetClientConfigFunc implementation
		client := NewMockSSHClient()
		config := &ClientConfig{User: "test"}

		// When calling SetClientConfig
		client.SetClientConfig(config)

		// Then it should not panic
	})
}

func TestMockClient_SetClientConfigFile(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock client with a custom SetClientConfigFileFunc
		client := NewMockSSHClient()
		client.SetClientConfigFileFunc = func(configStr, hostname string) error {
			return nil
		}

		// When calling SetClientConfigFile
		err := client.SetClientConfigFile("config", "host")

		// Then it should return no error
		if err != nil {
			t.Errorf("SetClientConfigFile() err = %v, want nil", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock client with a custom SetClientConfigFileFunc that returns an error
		client := NewMockSSHClient()
		expectedErr := errors.New("config file error")
		client.SetClientConfigFileFunc = func(configStr, hostname string) error {
			return expectedErr
		}

		// When calling SetClientConfigFile
		err := client.SetClientConfigFile("config", "host")

		// Then it should return the expected error
		if err != expectedErr {
			t.Errorf("SetClientConfigFile() err = %v, want %v", err, expectedErr)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock client with no SetClientConfigFileFunc implementation
		client := NewMockSSHClient()

		// When calling SetClientConfigFile
		err := client.SetClientConfigFile("config", "host")

		// Then it should return no error
		if err != nil {
			t.Errorf("SetClientConfigFile() err = %v, want nil", err)
		}
	})
}

func TestMockClientConn_NewSession(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock client connection with a custom NewSessionFunc
		conn := &MockClientConn{}
		expectedSession := &MockSession{}
		conn.NewSessionFunc = func() (Session, error) {
			return expectedSession, nil
		}

		// When calling NewSession
		session, err := conn.NewSession()

		// Then it should return the expected session without error
		if session != expectedSession {
			t.Errorf("NewSession() session = %v, want %v", session, expectedSession)
		}
		if err != nil {
			t.Errorf("NewSession() err = %v, want nil", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock client connection with a custom NewSessionFunc that returns an error
		conn := &MockClientConn{}
		expectedErr := errors.New("session error")
		conn.NewSessionFunc = func() (Session, error) {
			return nil, expectedErr
		}

		// When calling NewSession
		session, err := conn.NewSession()

		// Then it should return nil session and the expected error
		if session != nil {
			t.Errorf("NewSession() session = %v, want nil", session)
		}
		if err != expectedErr {
			t.Errorf("NewSession() err = %v, want %v", err, expectedErr)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock client connection with no NewSessionFunc implementation
		conn := &MockClientConn{}

		// When calling NewSession
		session, err := conn.NewSession()

		// Then it should return an empty session and no error
		if session == nil {
			t.Error("NewSession() returned nil session")
		}
		if err != nil {
			t.Errorf("NewSession() err = %v, want nil", err)
		}
	})
}

func TestMockClientConn_Close(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock client connection with a custom CloseFunc
		conn := &MockClientConn{}
		conn.CloseFunc = func() error {
			return nil
		}

		// When calling Close
		err := conn.Close()

		// Then it should return no error
		if err != nil {
			t.Errorf("Close() err = %v, want nil", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock client connection with a custom CloseFunc that returns an error
		conn := &MockClientConn{}
		expectedErr := errors.New("close error")
		conn.CloseFunc = func() error {
			return expectedErr
		}

		// When calling Close
		err := conn.Close()

		// Then it should return the expected error
		if err != expectedErr {
			t.Errorf("Close() err = %v, want %v", err, expectedErr)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock client connection with no CloseFunc implementation
		conn := &MockClientConn{}

		// When calling Close
		err := conn.Close()

		// Then it should return no error
		if err != nil {
			t.Errorf("Close() err = %v, want nil", err)
		}
	})
}

func TestMockSession_Run(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock session with a custom RunFunc
		session := &MockSession{}
		session.RunFunc = func(cmd string) error {
			return nil
		}

		// When calling Run
		err := session.Run("test command")

		// Then it should return no error
		if err != nil {
			t.Errorf("Run() err = %v, want nil", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock session with a custom RunFunc that returns an error
		session := &MockSession{}
		expectedErr := errors.New("run error")
		session.RunFunc = func(cmd string) error {
			return expectedErr
		}

		// When calling Run
		err := session.Run("test command")

		// Then it should return the expected error
		if err != expectedErr {
			t.Errorf("Run() err = %v, want %v", err, expectedErr)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock session with no RunFunc implementation
		session := &MockSession{}

		// When calling Run
		err := session.Run("test command")

		// Then it should return no error
		if err != nil {
			t.Errorf("Run() err = %v, want nil", err)
		}
	})
}

func TestMockSession_CombinedOutput(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock session with a custom CombinedOutputFunc
		session := &MockSession{}
		expectedOutput := []byte("test output")
		session.CombinedOutputFunc = func(cmd string) ([]byte, error) {
			return expectedOutput, nil
		}

		// When calling CombinedOutput
		output, err := session.CombinedOutput("test command")

		// Then it should return the expected output without error
		if !bytes.Equal(output, expectedOutput) {
			t.Errorf("CombinedOutput() output = %v, want %v", output, expectedOutput)
		}
		if err != nil {
			t.Errorf("CombinedOutput() err = %v, want nil", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock session with a custom CombinedOutputFunc that returns an error
		session := &MockSession{}
		expectedErr := errors.New("output error")
		session.CombinedOutputFunc = func(cmd string) ([]byte, error) {
			return nil, expectedErr
		}

		// When calling CombinedOutput
		output, err := session.CombinedOutput("test command")

		// Then it should return nil output and the expected error
		if output != nil {
			t.Errorf("CombinedOutput() output = %v, want nil", output)
		}
		if err != expectedErr {
			t.Errorf("CombinedOutput() err = %v, want %v", err, expectedErr)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock session with no CombinedOutputFunc implementation
		session := &MockSession{}

		// When calling CombinedOutput
		output, err := session.CombinedOutput("test command")

		// Then it should return mock output and no error
		if output == nil {
			t.Error("CombinedOutput() returned nil output")
		}
		if err != nil {
			t.Errorf("CombinedOutput() err = %v, want nil", err)
		}
	})
}

func TestMockSession_SetStdout(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock session with a custom SetStdoutFunc
		session := &MockSession{}
		var actualWriter io.Writer
		session.SetStdoutFunc = func(w io.Writer) {
			actualWriter = w
		}

		// When calling SetStdout
		expectedWriter := &bytes.Buffer{}
		session.SetStdout(expectedWriter)

		// Then it should pass the writer to the function
		if actualWriter != expectedWriter {
			t.Errorf("SetStdout() writer = %v, want %v", actualWriter, expectedWriter)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock session with no SetStdoutFunc implementation
		session := &MockSession{}
		writer := &bytes.Buffer{}

		// When calling SetStdout
		session.SetStdout(writer)

		// Then it should not panic
	})
}

func TestMockSession_SetStderr(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock session with a custom SetStderrFunc
		session := &MockSession{}
		var actualWriter io.Writer
		session.SetStderrFunc = func(w io.Writer) {
			actualWriter = w
		}

		// When calling SetStderr
		expectedWriter := &bytes.Buffer{}
		session.SetStderr(expectedWriter)

		// Then it should pass the writer to the function
		if actualWriter != expectedWriter {
			t.Errorf("SetStderr() writer = %v, want %v", actualWriter, expectedWriter)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock session with no SetStderrFunc implementation
		session := &MockSession{}
		writer := &bytes.Buffer{}

		// When calling SetStderr
		session.SetStderr(writer)

		// Then it should not panic
	})
}

func TestMockSession_Close(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock session with a custom CloseFunc
		session := &MockSession{}
		session.CloseFunc = func() error {
			return nil
		}

		// When calling Close
		err := session.Close()

		// Then it should return no error
		if err != nil {
			t.Errorf("Close() err = %v, want nil", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock session with a custom CloseFunc that returns an error
		session := &MockSession{}
		expectedErr := errors.New("close error")
		session.CloseFunc = func() error {
			return expectedErr
		}

		// When calling Close
		err := session.Close()

		// Then it should return the expected error
		if err != expectedErr {
			t.Errorf("Close() err = %v, want %v", err, expectedErr)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock session with no CloseFunc implementation
		session := &MockSession{}

		// When calling Close
		err := session.Close()

		// Then it should return no error
		if err != nil {
			t.Errorf("Close() err = %v, want nil", err)
		}
	})
}

func TestMockAuthMethod_Method(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock auth method with a custom MethodFunc
		auth := &MockAuthMethod{}
		expectedMethod := gossh.Password("test")
		auth.MethodFunc = func() any {
			return expectedMethod
		}

		// When calling Method
		method := auth.Method()

		// Then it should return a non-nil method
		if method == nil {
			t.Error("Method() returned nil")
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock auth method with no MethodFunc implementation
		auth := &MockAuthMethod{}

		// When calling Method
		method := auth.Method()

		// Then it should return nil
		if method != nil {
			t.Errorf("Method() returned %v, want nil", method)
		}
	})
}

func TestMockHostKeyCallback_Callback(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock host key callback with a custom CallbackFunc
		callback := &MockHostKeyCallback{}
		expectedCallback := gossh.InsecureIgnoreHostKey()
		callback.CallbackFunc = func() any {
			return expectedCallback
		}

		// When calling Callback
		result := callback.Callback()

		// Then it should return a non-nil callback
		if result == nil {
			t.Error("Callback() returned nil")
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock host key callback with no CallbackFunc implementation
		callback := &MockHostKeyCallback{}

		// When calling Callback
		result := callback.Callback()

		// Then it should return nil
		if result != nil {
			t.Errorf("Callback() returned %v, want nil", result)
		}
	})
}

func TestMockPublicKeyAuthMethod_Method(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock public key auth method with a custom SignerFunc
		auth := &MockPublicKeyAuthMethod{}
		expectedSigner := &mockSigner{}
		auth.SignerFunc = func() gossh.Signer {
			return expectedSigner
		}

		// When calling Method
		method := auth.Method()

		// Then it should return a non-nil method
		if method == nil {
			t.Error("Method() returned nil")
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock public key auth method with no SignerFunc implementation
		auth := &MockPublicKeyAuthMethod{}

		// When calling Method
		method := auth.Method()

		// Then it should return nil
		if method != nil {
			t.Errorf("Method() returned %v, want nil", method)
		}
	})
}

// =============================================================================
// Helpers
// =============================================================================

type mockSigner struct{}

func (s *mockSigner) PublicKey() gossh.PublicKey {
	return nil
}

func (s *mockSigner) Sign(rand io.Reader, data []byte) (*gossh.Signature, error) {
	return nil, nil
}
