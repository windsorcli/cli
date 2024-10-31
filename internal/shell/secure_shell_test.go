package shell

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/ssh"
	sshWrapper "github.com/windsor-hotel/cli/internal/ssh"
)

func TestSecureShell_NewSecureShell(t *testing.T) {
	t.Run("SuccessfulCreation", func(t *testing.T) {
		// Create a DI container and register the necessary dependencies
		diContainer := di.NewContainer()
		diContainer.Register("sshParams", SSHConnectionParams{
			Host:         "localhost",
			Port:         22,
			Username:     "user",
			IdentityFile: "/path/to/private/key",
		})
		diContainer.Register("sshClient", &ssh.MockClient{})
		diContainer.Register("sshAuthMethod", &ssh.MockAuthMethod{})
		diContainer.Register("sshHostKeyCallback", &ssh.MockHostKeyCallback{})

		// When creating a new SecureShell instance
		secureShell, err := NewSecureShell(diContainer)

		// Then no error should be returned and the instance should be created
		assertNoError(t, err)
		if secureShell == nil {
			t.Fatalf("Expected SecureShell instance, got nil")
		}
	})

	t.Run("ErrorResolvingSSHParams", func(t *testing.T) {
		// Create a DI container without registering sshParams
		diContainer := di.NewContainer()

		// When creating a new SecureShell instance
		_, err := NewSecureShell(diContainer)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error resolving sshParams") {
			t.Errorf("Expected error message to contain 'error resolving sshParams', got %v", err)
		}
	})

	t.Run("ErrorResolvingSSHClient", func(t *testing.T) {
		// Create a DI container and register sshParams but not sshClient
		diContainer := di.NewContainer()
		diContainer.Register("sshParams", SSHConnectionParams{
			Host:         "localhost",
			Port:         22,
			Username:     "user",
			IdentityFile: "/path/to/private/key",
		})

		// When creating a new SecureShell instance
		_, err := NewSecureShell(diContainer)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error resolving sshClient") {
			t.Errorf("Expected error message to contain 'error resolving sshClient', got %v", err)
		}
	})

	t.Run("ErrorResolvingSSHAuthMethod", func(t *testing.T) {
		// Create a DI container and register sshParams and sshClient but not sshAuthMethod
		diContainer := di.NewContainer()
		diContainer.Register("sshParams", SSHConnectionParams{
			Host:         "localhost",
			Port:         22,
			Username:     "user",
			IdentityFile: "/path/to/private/key",
		})
		diContainer.Register("sshClient", &ssh.MockClient{})

		// When creating a new SecureShell instance
		_, err := NewSecureShell(diContainer)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error resolving sshAuthMethod") {
			t.Errorf("Expected error message to contain 'error resolving sshAuthMethod', got %v", err)
		}
	})

	t.Run("ErrorResolvingSSHHostKeyCallback", func(t *testing.T) {
		// Create a DI container and register sshParams, sshClient, and sshAuthMethod but not sshHostKeyCallback
		diContainer := di.NewContainer()
		diContainer.Register("sshParams", SSHConnectionParams{
			Host:         "localhost",
			Port:         22,
			Username:     "user",
			IdentityFile: "/path/to/private/key",
		})
		diContainer.Register("sshClient", &ssh.MockClient{})
		diContainer.Register("sshAuthMethod", &ssh.MockAuthMethod{})

		// When creating a new SecureShell instance
		_, err := NewSecureShell(diContainer)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error resolving sshHostKeyCallback") {
			t.Errorf("Expected error message to contain 'error resolving sshHostKeyCallback', got %v", err)
		}
	})
}

func TestSecureShell_Exec(t *testing.T) {
	t.Run("CommandSuccess", func(t *testing.T) {
		// Given a mock session that returns expected output
		expectedCommand := "ls -la"
		expectedOutput := "command output"
		mockSession := &ssh.MockSession{
			CombinedOutputFunc: func(cmd string) ([]byte, error) {
				assertEqual(t, expectedCommand, cmd, "command")
				return []byte(expectedOutput), nil
			},
			RunFunc:       func(cmd string) error { return nil },
			SetStdoutFunc: func(w io.Writer) {},
			SetStderrFunc: func(w io.Writer) {},
			CloseFunc:     func() error { return nil },
		}

		// Given a mock client that returns the mock session
		mockClient := &ssh.MockClient{
			DialFunc: func(network, addr string, config *sshWrapper.ClientConfig) (sshWrapper.ClientConn, error) {
				return &ssh.MockClientConn{
					NewSessionFunc: func() (sshWrapper.Session, error) { return mockSession, nil },
					CloseFunc:      func() error { return nil },
				}, nil
			},
		}

		// Create a DI container and register the mock client and other dependencies
		diContainer := di.NewContainer()
		diContainer.Register("sshParams", SSHConnectionParams{
			Host:         "localhost",
			Port:         22,
			Username:     "user",
			IdentityFile: "/path/to/private/key",
		})
		diContainer.Register("sshClient", mockClient)
		diContainer.Register("sshAuthMethod", &ssh.MockAuthMethod{})
		diContainer.Register("sshHostKeyCallback", &ssh.MockHostKeyCallback{})

		// When executing the command
		secureShell, err := NewSecureShell(diContainer)
		assertNoError(t, err)
		result, err := secureShell.Exec(false, "Testing command", "ls", "-la")

		// Then the output should be as expected
		assertNoError(t, err)
		assertEqual(t, expectedOutput, result, "command output")
	})

	t.Run("CommandVerboseSuccess", func(t *testing.T) {
		// Given a mock session that writes output to stdout
		expectedCommand := "ls -la"
		outputMessage := "command output"
		mockSession := &ssh.MockSession{
			RunFunc: func(cmd string) error {
				assertEqual(t, expectedCommand, cmd, "command")
				fmt.Fprint(os.Stdout, outputMessage)
				return nil
			},
			SetStdoutFunc: func(w io.Writer) {
				// Simulate setting stdout in the session
			},
			SetStderrFunc: func(w io.Writer) {
				// Simulate setting stderr in the session
			},
			CloseFunc: func() error { return nil },
		}

		// Given a mock client that returns the mock session
		mockClient := &ssh.MockClient{
			DialFunc: func(network, addr string, config *sshWrapper.ClientConfig) (sshWrapper.ClientConn, error) {
				return &ssh.MockClientConn{
					NewSessionFunc: func() (sshWrapper.Session, error) { return mockSession, nil },
					CloseFunc:      func() error { return nil },
				}, nil
			},
		}

		// Create a DI container and register the mock client and other dependencies
		diContainer := di.NewContainer()
		diContainer.Register("sshParams", SSHConnectionParams{
			Host:         "localhost",
			Port:         22,
			Username:     "user",
			IdentityFile: "/path/to/private/key",
		})
		diContainer.Register("sshClient", mockClient)
		diContainer.Register("sshAuthMethod", &ssh.MockAuthMethod{})
		diContainer.Register("sshHostKeyCallback", &ssh.MockHostKeyCallback{})

		// Capture stdout during the command execution
		output := captureStdout(t, func() {
			// When executing the command in verbose mode
			secureShell, err := NewSecureShell(diContainer)
			assertNoError(t, err)
			_, err = secureShell.Exec(true, "Testing command", "ls", "-la")
			assertNoError(t, err)
		})

		// Then the output should be captured correctly
		assertEqual(t, outputMessage, output, "stdout output")
	})

	t.Run("CommandError", func(t *testing.T) {
		// Given a mock session that returns an error
		expectedCommand := "ls -la"
		expectedError := errors.New("command failed")
		mockSession := &ssh.MockSession{
			CombinedOutputFunc: func(cmd string) ([]byte, error) {
				assertEqual(t, expectedCommand, cmd, "command")
				return nil, expectedError
			},
			RunFunc:       func(cmd string) error { return expectedError },
			SetStdoutFunc: func(w io.Writer) {},
			SetStderrFunc: func(w io.Writer) {},
			CloseFunc:     func() error { return nil },
		}

		// Given a mock client that returns the mock session
		mockClient := &ssh.MockClient{
			DialFunc: func(network, addr string, config *sshWrapper.ClientConfig) (sshWrapper.ClientConn, error) {
				return &ssh.MockClientConn{
					NewSessionFunc: func() (sshWrapper.Session, error) { return mockSession, nil },
					CloseFunc:      func() error { return nil },
				}, nil
			},
		}

		// Create a DI container and register the mock client and other dependencies
		diContainer := di.NewContainer()
		diContainer.Register("sshParams", SSHConnectionParams{
			Host:         "localhost",
			Port:         22,
			Username:     "user",
			IdentityFile: "/path/to/private/key",
		})
		diContainer.Register("sshClient", mockClient)
		diContainer.Register("sshAuthMethod", &ssh.MockAuthMethod{})
		diContainer.Register("sshHostKeyCallback", &ssh.MockHostKeyCallback{})

		// When executing the command
		secureShell, err := NewSecureShell(diContainer)
		assertNoError(t, err)
		_, err = secureShell.Exec(false, "Testing command", "ls", "-la")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedErrMsg := fmt.Sprintf("failed to run command: %v", expectedError)
		assertEqual(t, expectedErrMsg, err.Error(), "error message")
	})

	t.Run("DialError", func(t *testing.T) {
		// Given ssh.Dial returns an error
		expectedError := errors.New("failed to dial")
		mockClient := &ssh.MockClient{
			DialFunc: func(network, addr string, config *sshWrapper.ClientConfig) (sshWrapper.ClientConn, error) {
				return nil, expectedError
			},
		}

		// Create a DI container and register the mock client and other dependencies
		diContainer := di.NewContainer()
		diContainer.Register("sshParams", SSHConnectionParams{
			Host:         "localhost",
			Port:         22,
			Username:     "user",
			IdentityFile: "/path/to/private/key",
		})
		diContainer.Register("sshClient", mockClient)
		diContainer.Register("sshAuthMethod", &ssh.MockAuthMethod{})
		diContainer.Register("sshHostKeyCallback", &ssh.MockHostKeyCallback{})

		// When executing the command
		secureShell, err := NewSecureShell(diContainer)
		assertNoError(t, err)
		_, err = secureShell.Exec(false, "Testing command", "ls", "-la")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedErrMsg := fmt.Sprintf("failed to dial: %v", expectedError)
		assertEqual(t, expectedErrMsg, err.Error(), "error message")
	})

	t.Run("NewSessionError", func(t *testing.T) {
		// Given a mock client that returns an error when creating a session
		expectedError := errors.New("failed to create session")
		mockClient := &ssh.MockClient{
			DialFunc: func(network, addr string, config *sshWrapper.ClientConfig) (sshWrapper.ClientConn, error) {
				return &ssh.MockClientConn{
					NewSessionFunc: func() (sshWrapper.Session, error) { return nil, expectedError },
					CloseFunc:      func() error { return nil },
				}, nil
			},
		}

		// Create a DI container and register the mock client and other dependencies
		diContainer := di.NewContainer()
		diContainer.Register("sshParams", SSHConnectionParams{
			Host:         "localhost",
			Port:         22,
			Username:     "user",
			IdentityFile: "/path/to/private/key",
		})
		diContainer.Register("sshClient", mockClient)
		diContainer.Register("sshAuthMethod", &ssh.MockAuthMethod{})
		diContainer.Register("sshHostKeyCallback", &ssh.MockHostKeyCallback{})

		// When executing the command
		secureShell, err := NewSecureShell(diContainer)
		assertNoError(t, err)
		_, err = secureShell.Exec(false, "Testing command", "ls", "-la")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedErrMsg := fmt.Sprintf("failed to create session: %v", expectedError)
		assertEqual(t, expectedErrMsg, err.Error(), "error message")
	})

	t.Run("RunCommandError", func(t *testing.T) {
		// Given a mock session that returns an error when running the command
		expectedCommand := "ls -la"
		expectedError := errors.New("command failed")
		mockSession := &ssh.MockSession{
			RunFunc: func(cmd string) error {
				assertEqual(t, expectedCommand, cmd, "command")
				return expectedError
			},
			SetStdoutFunc: func(w io.Writer) {},
			SetStderrFunc: func(w io.Writer) {},
			CloseFunc:     func() error { return nil },
		}

		// Given a mock client that returns the mock session
		mockClient := &ssh.MockClient{
			DialFunc: func(network, addr string, config *sshWrapper.ClientConfig) (sshWrapper.ClientConn, error) {
				return &ssh.MockClientConn{
					NewSessionFunc: func() (sshWrapper.Session, error) { return mockSession, nil },
					CloseFunc:      func() error { return nil },
				}, nil
			},
		}

		// Create a DI container and register the mock client and other dependencies
		diContainer := di.NewContainer()
		diContainer.Register("sshParams", SSHConnectionParams{
			Host:         "localhost",
			Port:         22,
			Username:     "user",
			IdentityFile: "/path/to/private/key",
		})
		diContainer.Register("sshClient", mockClient)
		diContainer.Register("sshAuthMethod", &ssh.MockAuthMethod{})
		diContainer.Register("sshHostKeyCallback", &ssh.MockHostKeyCallback{})

		// When executing the command
		secureShell, err := NewSecureShell(diContainer)
		assertNoError(t, err)
		_, err = secureShell.Exec(true, "Testing command", "ls", "-la")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedErrMsg := fmt.Sprintf("failed to run command: %v", expectedError)
		assertEqual(t, expectedErrMsg, err.Error(), "error message")
	})

	t.Run("PublicKeysCallbackError", func(t *testing.T) {
		// Given a mock client that returns an error when creating a session due to invalid identity file
		expectedError := errors.New("unable to read private key file")
		mockClient := &ssh.MockClient{
			DialFunc: func(network, addr string, config *sshWrapper.ClientConfig) (sshWrapper.ClientConn, error) {
				return &ssh.MockClientConn{
					NewSessionFunc: func() (sshWrapper.Session, error) { return nil, expectedError },
					CloseFunc:      func() error { return nil },
				}, nil
			},
		}

		// Create a DI container and register the mock client and other dependencies
		diContainer := di.NewContainer()
		diContainer.Register("sshParams", SSHConnectionParams{
			Host:         "localhost",
			Port:         22,
			Username:     "user",
			IdentityFile: "/invalid/path/to/private/key",
		})
		diContainer.Register("sshClient", mockClient)
		diContainer.Register("sshAuthMethod", &ssh.MockAuthMethod{})
		diContainer.Register("sshHostKeyCallback", &ssh.MockHostKeyCallback{})

		// When executing the command
		secureShell, err := NewSecureShell(diContainer)
		assertNoError(t, err)
		_, err = secureShell.Exec(false, "Testing command", "ls", "-la")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "unable to read private key file") {
			t.Errorf("Expected error message to contain 'unable to read private key file', got %v", err)
		}
	})
}
