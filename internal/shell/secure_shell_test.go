package shell

import (
	"errors"
	"io"
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
		ConnectFunc: func(host, user, identityFile, port string) (ssh.ClientConn, error) {
			return mockClientConn, nil
		},
	}

	mockShell := NewMockShell()

	c := container[0]
	c.Register("sshClient", mockClient)
	c.Register("defaultShell", mockShell)

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

func TestNewSecureShell(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setSafeSecureShellMocks()

		secureShell, err := NewSecureShell(mocks.Container, "host", "user", "identity", "22")
		assertNoError(t, err)
		if secureShell == nil || secureShell.client == nil {
			t.Fatalf("Expected secureShell to be initialized with mockClient")
		}
	})

	t.Run("ResolveError", func(t *testing.T) {
		container := di.NewMockContainer()
		container.SetResolveError("sshClient", errors.New("resolve error"))
		setSafeSecureShellMocks(container)

		secureShell, err := NewSecureShell(container, "host", "user", "identity", "22")
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if secureShell != nil {
			t.Fatalf("Expected secureShell to be nil on error")
		}
	})

	t.Run("CastClientNotOk", func(t *testing.T) {
		// Create a new mock container
		container := di.NewMockContainer()

		// Register an invalid client that doesn't implement ssh.Client
		invalidClient := &struct{}{}
		container.Register("sshClient", invalidClient)

		// Attempt to create a new SecureShell
		secureShell, err := NewSecureShell(container, "host", "user", "identity", "22")

		// Assert that an error occurred
		if err == nil {
			t.Fatalf("Expected an error due to invalid client type, but got nil")
		}

		// Check that the error message is as expected
		expectedError := "resolved SSH client does not implement Client interface"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}

		// Assert that secureShell is nil
		if secureShell != nil {
			t.Fatalf("Expected secureShell to be nil due to error, but got an instance")
		}
	})

	t.Run("ConnectError", func(t *testing.T) {
		mocks := setSafeSecureShellMocks()
		mocks.Client.ConnectFunc = func(host, user, identityFile, port string) (ssh.ClientConn, error) {
			return nil, errors.New("connection error")
		}

		secureShell, err := NewSecureShell(mocks.Container, "host", "user", "identity", "22")
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if secureShell != nil {
			t.Fatalf("Expected secureShell to be nil on error")
		}
	})

	t.Run("ResolveDefaultShellError", func(t *testing.T) {
		container := di.NewMockContainer()
		container.SetResolveError("defaultShell", errors.New("resolve error"))
		setSafeSecureShellMocks(container)

		secureShell, err := NewSecureShell(container, "host", "user", "identity", "22")
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if secureShell != nil {
			t.Fatalf("Expected secureShell to be nil on error")
		}
	})

	t.Run("CastDefaultShellNotOk", func(t *testing.T) {
		// Create a new mock container
		container := di.NewMockContainer()

		// Register an invalid default shell that doesn't implement Shell
		invalidClient := &struct{}{}
		container.Register("defaultShell", invalidClient)

		// Register a valid sshClient
		validSSHClient := &ssh.MockClient{}
		container.Register("sshClient", validSSHClient)

		// Attempt to create a new SecureShell
		secureShell, err := NewSecureShell(container, "host", "user", "identity", "22")

		// Assert that an error occurred
		if err == nil {
			t.Fatalf("Expected an error due to invalid default shell type, but got nil")
		}

		// Check that the error message is as expected
		expectedError := "resolved default shell does not implement Shell interface"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}

		// Assert that secureShell is nil
		if secureShell != nil {
			t.Fatalf("Expected secureShell to be nil due to error, but got an instance")
		}
	})
}

func TestSecureShell_PrintEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}

		mocks := setSafeSecureShellMocks()
		mocks.Shell.PrintEnvVarsFunc = func(vars map[string]string) {
			if len(vars) != len(envVars) {
				t.Fatalf("Expected %d env vars, got %d", len(envVars), len(vars))
			}
			for k, v := range envVars {
				if vars[k] != v {
					t.Fatalf("Expected env var %s to be %s, got %s", k, v, vars[k])
				}
			}
		}

		secureShell, err := NewSecureShell(mocks.Container, "host", "user", "identity", "22")
		assertNoError(t, err)

		secureShell.PrintEnvVars(envVars)
	})

	t.Run("Error", func(t *testing.T) {
		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}

		mocks := setSafeSecureShellMocks()
		mocks.Shell.PrintEnvVarsFunc = func(vars map[string]string) {
			t.Fatalf("PrintEnvVars should not be called")
		}

		secureShell, err := NewSecureShell(mocks.Container, "host", "user", "identity", "22")
		assertNoError(t, err)

		secureShell.PrintEnvVars(envVars)
	})
}

func TestSecureShell_GetProjectRoot(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		expectedOutput := "/remote/project/root"

		mocks := setSafeSecureShellMocks()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return expectedOutput, nil
		}

		secureShell, err := NewSecureShell(mocks.Container, "host", "user", "identity", "22")
		assertNoError(t, err)

		projectRoot, err := secureShell.GetProjectRoot()
		assertNoError(t, err)
		if projectRoot != expectedOutput {
			t.Fatalf("Expected projectRoot %q, got %q", expectedOutput, projectRoot)
		}

		// Test caching by calling GetProjectRoot again
		projectRoot, err = secureShell.GetProjectRoot()
		assertNoError(t, err)
		if projectRoot != expectedOutput {
			t.Fatalf("Expected cached projectRoot %q, got %q", expectedOutput, projectRoot)
		}
	})

	t.Run("Error", func(t *testing.T) {
		mocks := setSafeSecureShellMocks()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", errors.New("command failed")
		}

		secureShell, err := NewSecureShell(mocks.Container, "host", "user", "identity", "22")
		assertNoError(t, err)

		projectRoot, err := secureShell.GetProjectRoot()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if projectRoot != "" {
			t.Fatalf("Expected projectRoot to be empty, got %q", projectRoot)
		}
	})
}

func TestSecureShell_Exec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		expectedOutput := "command output"

		mocks := setSafeSecureShellMocks()
		mocks.Session.RunFunc = func(cmd string) error {
			return nil
		}
		mocks.Session.SetStdoutFunc = func(w io.Writer) {
			w.Write([]byte(expectedOutput))
		}

		secureShell, err := NewSecureShell(mocks.Container, "host", "user", "identity", "22")
		assertNoError(t, err)

		output, err := secureShell.Exec(false, "Executing command", "echo", "hello")
		assertNoError(t, err)
		if output != expectedOutput {
			t.Fatalf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("Error", func(t *testing.T) {
		mocks := setSafeSecureShellMocks()
		mocks.Session.RunFunc = func(cmd string) error {
			return errors.New("execution failed")
		}
		mocks.Session.SetStderrFunc = func(w io.Writer) {
			w.Write([]byte("error output"))
		}

		secureShell, err := NewSecureShell(mocks.Container, "host", "user", "identity", "22")
		assertNoError(t, err)

		output, err := secureShell.Exec(false, "Executing command", "badcommand")
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if output != "" {
			t.Fatalf("Expected empty output on error, got %q", output)
		}
	})

	t.Run("SessionError", func(t *testing.T) {
		mocks := setSafeSecureShellMocks()
		mocks.ClientConn.NewSessionFunc = func() (ssh.Session, error) {
			return nil, errors.New("session creation failed")
		}

		secureShell, err := NewSecureShell(mocks.Container, "host", "user", "identity", "22")
		assertNoError(t, err)

		output, err := secureShell.Exec(false, "Executing command", "echo", "hello")
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if output != "" {
			t.Fatalf("Expected empty output on error, got %q", output)
		}
	})

	t.Run("SuccessVerbose", func(t *testing.T) {
		mocks := setSafeSecureShellMocks()
		mocks.Session.RunFunc = func(cmd string) error {
			return nil
		}
		mocks.Session.SetStdoutFunc = func(w io.Writer) {
			w.Write([]byte("hello\n"))
		}

		secureShell, err := NewSecureShell(mocks.Container, "host", "user", "identity", "22")
		assertNoError(t, err)

		output, err := secureShell.Exec(true, "Executing command", "echo", "hello")
		assertNoError(t, err)
		expectedOutput := "hello\n"
		if output != expectedOutput {
			t.Fatalf("Expected output %q, got %q", expectedOutput, output)
		}
	})
}
