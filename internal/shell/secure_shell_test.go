package shell

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/ssh"
)

// MockSpinner is used to override the spinner in tests
type MockSpinner struct{}

func (s *MockSpinner) Start() {}
func (s *MockSpinner) Stop()  {}

// setSafeSecureShellMocks creates a safe "supermock" where all things are mocked and everything returns a non-error.
func setSafeSecureShellMocks(container ...di.ContainerInterface) struct {
	Container  di.ContainerInterface
	Client     *ssh.MockClient
	ClientConn *ssh.MockClientConn
	Session    *ssh.MockSession
	Shell      *MockShell
} {
	if len(container) == 0 {
		container = []di.ContainerInterface{di.NewMockContainer()}
	}

	mockSession := &ssh.MockSession{
		RunFunc: func(cmd string) error {
			return nil
		},
		SetStdoutFunc: func(w io.Writer) {
			w.Write([]byte("command output"))
		},
		SetStderrFunc: func(w io.Writer) {},
	}

	mockClientConn := &ssh.MockClientConn{
		NewSessionFunc: func() (ssh.Session, error) {
			return mockSession, nil
		},
	}

	mockClient := &ssh.MockClient{
		ConnectFunc: func() (ssh.ClientConn, error) {
			return mockClientConn, nil
		},
	}

	mockShell := NewMockShell()

	c := container[0]
	if _, err := c.Resolve("sshClient"); err != nil {
		c.Register("sshClient", mockClient)
	}
	if _, err := c.Resolve("defaultShell"); err != nil {
		c.Register("defaultShell", mockShell)
	}

	return struct {
		Container  di.ContainerInterface
		Client     *ssh.MockClient
		ClientConn *ssh.MockClientConn
		Session    *ssh.MockSession
		Shell      *MockShell
	}{
		Container:  c,
		Client:     mockClient,
		ClientConn: mockClientConn,
		Session:    mockSession,
		Shell:      mockShell,
	}
}

func TestSecureShell_Exec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		expectedOutput := "command output"
		command := "echo"
		args := []string{"hello"}
		message := "Running echo command"

		mocks := setSafeSecureShellMocks()
		mocks.ClientConn.NewSessionFunc = func() (ssh.Session, error) {
			return &ssh.MockSession{
				RunFunc: func(cmd string) error {
					if cmd != command+" "+strings.Join(args, " ") {
						return fmt.Errorf("unexpected command: %s", cmd)
					}
					return nil
				},
				SetStdoutFunc: func(w io.Writer) {
					w.Write([]byte(expectedOutput))
				},
				SetStderrFunc: func(w io.Writer) {},
			}, nil
		}

		secureShell, err := NewSecureShell(mocks.Container)
		assertNoError(t, err)

		output, err := secureShell.Exec(false, message, command, args...)
		assertNoError(t, err)
		if output != expectedOutput {
			t.Fatalf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("Error", func(t *testing.T) {
		command := "invalid_command"
		args := []string{}
		message := "Running invalid command"

		mocks := setSafeSecureShellMocks()
		mocks.ClientConn.NewSessionFunc = func() (ssh.Session, error) {
			return &ssh.MockSession{
				RunFunc: func(cmd string) error {
					return fmt.Errorf("command execution failed")
				},
				SetStdoutFunc: func(w io.Writer) {},
				SetStderrFunc: func(w io.Writer) {
					w.Write([]byte("error output"))
				},
			}, nil
		}

		secureShell, err := NewSecureShell(mocks.Container)
		assertNoError(t, err)

		output, err := secureShell.Exec(false, message, command, args...)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if output != "" {
			t.Fatalf("Expected output to be empty, got %q", output)
		}
	})

	t.Run("ResolveSSHClientError", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveError("sshClient", fmt.Errorf("failed to resolve SSH client"))

		mocks := setSafeSecureShellMocks(mockContainer)

		secureShell, err := NewSecureShell(mocks.Container)
		assertNoError(t, err)

		_, err = secureShell.Exec(false, "Running command", "echo", "hello")
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "failed to resolve SSH client"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("InvalidSSHClient", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		mockContainer.Register("sshClient", "not_a_valid_ssh_client")

		mocks := setSafeSecureShellMocks(mockContainer)

		secureShell, err := NewSecureShell(mocks.Container)
		assertNoError(t, err)

		_, err = secureShell.Exec(false, "Running command", "echo", "hello")
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "resolved SSH client does not implement Client interface"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ConnectError", func(t *testing.T) {
		mocks := setSafeSecureShellMocks()
		mocks.Client.ConnectFunc = func() (ssh.ClientConn, error) {
			return nil, fmt.Errorf("failed to connect to SSH client")
		}

		secureShell, err := NewSecureShell(mocks.Container)
		assertNoError(t, err)

		_, err = secureShell.Exec(false, "Running command", "echo", "hello")
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "failed to connect to SSH client"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NewSessionError", func(t *testing.T) {
		mocks := setSafeSecureShellMocks()
		mocks.ClientConn.NewSessionFunc = func() (ssh.Session, error) {
			return nil, fmt.Errorf("failed to create SSH session")
		}

		secureShell, err := NewSecureShell(mocks.Container)
		assertNoError(t, err)

		_, err = secureShell.Exec(false, "Running command", "echo", "hello")
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "failed to create SSH session"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("Verbose", func(t *testing.T) {
		expectedOutput := "command output"
		command := "echo"
		args := []string{"hello"}
		message := "Running echo command"

		mocks := setSafeSecureShellMocks()
		mocks.ClientConn.NewSessionFunc = func() (ssh.Session, error) {
			return &ssh.MockSession{
				RunFunc: func(cmd string) error {
					if cmd != command+" "+strings.Join(args, " ") {
						return fmt.Errorf("unexpected command: %s", cmd)
					}
					return nil
				},
				SetStdoutFunc: func(w io.Writer) {
					w.Write([]byte(expectedOutput))
				},
				SetStderrFunc: func(w io.Writer) {},
			}, nil
		}

		secureShell, err := NewSecureShell(mocks.Container)
		assertNoError(t, err)

		output, err := secureShell.Exec(true, message, command, args...)
		assertNoError(t, err)
		if output != expectedOutput {
			t.Fatalf("Expected output %q, got %q", expectedOutput, output)
		}
	})
}
