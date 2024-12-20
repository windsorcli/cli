package shell

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/config"
	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/ssh"
)

// MockSpinner is used to override the spinner in tests
type MockSpinner struct{}

func (s *MockSpinner) Start() {}
func (s *MockSpinner) Stop()  {}

// setSafeSecureShellMocks creates a safe "supermock" where all components are mocked and return non-error responses.
func setSafeSecureShellMocks(injector ...di.Injector) struct {
	Injector      di.Injector
	Client        *ssh.MockClient
	ClientConn    *ssh.MockClientConn
	Session       *ssh.MockSession
	Shell         *MockShell
	ConfigHandler *config.MockConfigHandler
} {
	if len(injector) == 0 {
		injector = []di.Injector{di.NewMockInjector()}
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

	mockShell := NewMockShell(injector[0])

	mockConfigHandler := config.NewMockConfigHandler()

	i := injector[0]
	if i.Resolve("sshClient") == nil {
		i.Register("sshClient", mockClient)
	}
	if i.Resolve("defaultShell") == nil {
		i.Register("defaultShell", mockShell)
	}
	if i.Resolve("configHandler") == nil {
		i.Register("configHandler", mockConfigHandler)
	}

	return struct {
		Injector      di.Injector
		Client        *ssh.MockClient
		ClientConn    *ssh.MockClientConn
		Session       *ssh.MockSession
		Shell         *MockShell
		ConfigHandler *config.MockConfigHandler
	}{
		Injector:      i,
		Client:        mockClient,
		ClientConn:    mockClientConn,
		Session:       mockSession,
		Shell:         mockShell,
		ConfigHandler: mockConfigHandler,
	}
}

func TestSecureShell_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Call the setup function
		mocks := setSafeSecureShellMocks()

		// And a SecureShell instance
		secureShell := NewSecureShell(mocks.Injector)

		// When calling Initialize
		err := secureShell.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Initialize() error = %v, wantErr %v", err, false)
		}
	})

	t.Run("ErrorInitializingBaseShell", func(t *testing.T) {
		mocks := setSafeSecureShellMocks()
		mocks.Injector.Register("configHandler", "not a configHandler")
		secureShell := NewSecureShell(mocks.Injector)
		err := secureShell.Initialize()
		if err == nil {
			t.Errorf("Expected error when initializing base shell with invalid configHandler, got nil")
		}
	})

	t.Run("ErrorResolvingSSHClient", func(t *testing.T) {
		mocks := setSafeSecureShellMocks()
		mocks.Injector.Register("sshClient", "not a sshClient")
		secureShell := NewSecureShell(mocks.Injector)
		err := secureShell.Initialize()
		if err == nil {
			t.Errorf("Expected error when resolving SSH client, got nil")
		}
	})
}

func TestSecureShell_Exec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		expectedOutput := "command output"
		command := "echo"
		args := []string{"hello"}

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

		secureShell := NewSecureShell(mocks.Injector)
		secureShell.Initialize()

		output, err := secureShell.Exec(command, args...)
		if err != nil {
			t.Fatalf("Failed to execute command: %v", err)
		}
		if output != expectedOutput {
			t.Fatalf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("Error", func(t *testing.T) {
		command := "invalid_command"
		args := []string{}

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

		secureShell := NewSecureShell(mocks.Injector)
		secureShell.Initialize()

		output, err := secureShell.Exec(command, args...)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if output != "" {
			t.Fatalf("Expected output to be empty, got %q", output)
		}
	})

	t.Run("ConnectError", func(t *testing.T) {
		mocks := setSafeSecureShellMocks()
		mocks.Client.ConnectFunc = func() (ssh.ClientConn, error) {
			return nil, fmt.Errorf("failed to connect to SSH client")
		}

		secureShell := NewSecureShell(mocks.Injector)
		secureShell.Initialize()

		_, err := secureShell.Exec("Running command", "echo", "hello")
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

		secureShell := NewSecureShell(mocks.Injector)
		secureShell.Initialize()

		_, err := secureShell.Exec("Running command", "echo", "hello")
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

		secureShell := NewSecureShell(mocks.Injector)
		secureShell.Initialize()

		output, err := secureShell.Exec(command, args...)
		if err != nil {
			t.Fatalf("Failed to execute command: %v", err)
		}
		if output != expectedOutput {
			t.Fatalf("Expected output %q, got %q", expectedOutput, output)
		}
	})
}
