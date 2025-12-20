package shell

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/runtime/shell/ssh"
)

// The SecureShellTest is a test suite for the SecureShell implementation.
// It provides comprehensive test coverage for SSH-based command execution,
// connection management, and session handling.
// The SecureShellTest ensures reliable remote command execution through SSH,
// proper error handling, and session lifecycle management.

// =============================================================================
// Test Setup
// =============================================================================

type SecureMocks struct {
	*ShellTestMocks
	Client     *ssh.MockClient
	ClientConn *ssh.MockClientConn
	Session    *ssh.MockSession
}

// setupSecureShellMocks creates a new set of mocks for testing SecureShell
func setupSecureShellMocks(t *testing.T) *SecureMocks {
	t.Helper()

	// Set up base mocks first
	baseMocks := setupShellMocks(t)

	// Create default mock components
	mockSession := &ssh.MockSession{
		RunFunc: func(cmd string) error {
			return nil
		},
		SetStdoutFunc: func(w io.Writer) {
			if _, err := w.Write([]byte("command output")); err != nil {
				t.Errorf("Failed to write output: %v", err)
			}
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

	return &SecureMocks{
		ShellTestMocks: baseMocks,
		Client:         mockClient,
		ClientConn:     mockClientConn,
		Session:        mockSession,
	}
}

// MockSpinner is used to override the spinner in tests
type MockSpinner struct{}

func (s *MockSpinner) Start() {}
func (s *MockSpinner) Stop()  {}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestSecureShell_NewSecureShell tests the NewSecureShell constructor
func TestSecureShell_NewSecureShell(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a secure shell with valid SSH client
		mocks := setupSecureShellMocks(t)
		shell := NewSecureShell(mocks.Client)

		// Then it should be created
		if shell == nil {
			t.Error("Expected shell to be created")
		}
	})
}

// TestSecureShell_Exec tests the Exec method of SecureShell
func TestSecureShell_Exec(t *testing.T) {
	setup := func(t *testing.T) (*SecureShell, *SecureMocks) {
		t.Helper()
		mocks := setupSecureShellMocks(t)
		shell := NewSecureShell(mocks.Client)
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		expectedOutput := "command output"
		command := "echo"
		args := []string{"hello"}

		// Given a SecureShell instance with mocks
		shell, mocks := setup(t)
		mocks.Session.RunFunc = func(cmd string) error {
			if cmd != command+" "+strings.Join(args, " ") {
				return fmt.Errorf("unexpected command: %s", cmd)
			}
			return nil
		}
		mocks.Session.SetStdoutFunc = func(w io.Writer) {
			if _, err := w.Write([]byte(expectedOutput)); err != nil {
				t.Errorf("Failed to write output: %v", err)
			}
		}

		// When calling Exec
		output, err := shell.Exec(command, args...)

		// Then no error should be returned and output should match
		if err != nil {
			t.Fatalf("Failed to execute command: %v", err)
		}
		if output != expectedOutput {
			t.Fatalf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorConnectingToSSH", func(t *testing.T) {
		// Given a SecureShell instance with connection failure
		shell, mocks := setup(t)
		expectedError := fmt.Errorf("connection failed")
		mocks.Client.ConnectFunc = func() (ssh.ClientConn, error) {
			return nil, expectedError
		}

		// When calling Exec
		output, err := shell.Exec("command")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error from Connect, got nil")
		}
		if !strings.Contains(err.Error(), "failed to connect to SSH client") {
			t.Errorf("Expected error to contain 'failed to connect to SSH client', got %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
	})

	t.Run("ErrorCreatingSSHSession", func(t *testing.T) {
		// Given a SecureShell instance with session creation failure
		shell, mocks := setup(t)
		expectedError := fmt.Errorf("session creation failed")
		mocks.ClientConn.NewSessionFunc = func() (ssh.Session, error) {
			return nil, expectedError
		}

		// When calling Exec
		output, err := shell.Exec("command")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error from NewSession, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create SSH session") {
			t.Errorf("Expected error to contain 'failed to create SSH session', got %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
	})

	t.Run("ErrorExecutingCommand", func(t *testing.T) {
		command := "invalid_command"
		args := []string{}

		// Given a SecureShell instance with mocks
		shell, mocks := setup(t)
		expectedError := fmt.Errorf("command failed")
		mocks.Session.RunFunc = func(cmd string) error {
			return expectedError
		}

		// When calling Exec
		output, err := shell.Exec(command, args...)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error from Run, got nil")
		}
		if !strings.Contains(err.Error(), "command execution failed") {
			t.Errorf("Expected error to contain 'command execution failed', got %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
	})
}

// TestSecureShell_ExecProgress tests the ExecProgress method of SecureShell
func TestSecureShell_ExecProgress(t *testing.T) {
	setup := func(t *testing.T) (*SecureShell, *SecureMocks) {
		t.Helper()
		mocks := setupSecureShellMocks(t)
		shell := NewSecureShell(mocks.Client)
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		expectedOutput := "command output"
		message := "Running command..."
		command := "echo"
		args := []string{"hello"}

		// Given a SecureShell instance with mocks
		shell, mocks := setup(t)
		mocks.Session.RunFunc = func(cmd string) error {
			if cmd != command+" "+strings.Join(args, " ") {
				return fmt.Errorf("unexpected command: %s", cmd)
			}
			return nil
		}
		mocks.Session.SetStdoutFunc = func(w io.Writer) {
			if _, err := w.Write([]byte(expectedOutput)); err != nil {
				t.Errorf("Failed to write output: %v", err)
			}
		}

		// When calling ExecProgress
		output, err := shell.ExecProgress(message, command, args...)

		// Then no error should be returned and output should match
		if err != nil {
			t.Fatalf("Failed to execute command: %v", err)
		}
		if output != expectedOutput {
			t.Fatalf("Expected output %q, got %q", expectedOutput, output)
		}
	})
}

// TestSecureShell_ExecSilent tests the ExecSilent method of SecureShell
func TestSecureShell_ExecSilent(t *testing.T) {
	setup := func(t *testing.T) (*SecureShell, *SecureMocks) {
		t.Helper()
		mocks := setupSecureShellMocks(t)
		shell := NewSecureShell(mocks.Client)
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		expectedOutput := "command output"
		command := "echo"
		args := []string{"hello"}

		// Given a SecureShell instance with mocks
		shell, mocks := setup(t)
		mocks.Session.RunFunc = func(cmd string) error {
			if cmd != command+" "+strings.Join(args, " ") {
				return fmt.Errorf("unexpected command: %s", cmd)
			}
			return nil
		}
		mocks.Session.SetStdoutFunc = func(w io.Writer) {
			if _, err := w.Write([]byte(expectedOutput)); err != nil {
				t.Errorf("Failed to write output: %v", err)
			}
		}

		// When calling ExecSilent
		output, err := shell.ExecSilent(command, args...)

		// Then no error should be returned and output should match
		if err != nil {
			t.Fatalf("Failed to execute command: %v", err)
		}
		if output != expectedOutput {
			t.Fatalf("Expected output %q, got %q", expectedOutput, output)
		}
	})
}

// TestSecureShell_ExecSilentWithTimeout tests the ExecSilentWithTimeout method of SecureShell
func TestSecureShell_ExecSilentWithTimeout(t *testing.T) {
	setup := func(t *testing.T) (*SecureShell, *SecureMocks) {
		t.Helper()
		mocks := setupSecureShellMocks(t)
		shell := NewSecureShell(mocks.Client)
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		expectedOutput := "command output"
		command := "echo"
		args := []string{"test"}

		// Given a SecureShell instance with mocks
		shell, mocks := setup(t)
		mocks.Session.RunFunc = func(cmd string) error {
			expectedCmd := command + " " + strings.Join(args, " ")
			if cmd != expectedCmd {
				return fmt.Errorf("unexpected command: %s, expected: %s", cmd, expectedCmd)
			}
			return nil
		}
		mocks.Session.SetStdoutFunc = func(w io.Writer) {
			if _, err := w.Write([]byte(expectedOutput)); err != nil {
				t.Errorf("Failed to write output: %v", err)
			}
		}

		// When calling ExecSilentWithTimeout
		output, err := shell.ExecSilentWithTimeout(command, args, 5*time.Second)

		// Then no error should be returned and output should match
		if err != nil {
			t.Fatalf("Failed to execute command: %v", err)
		}
		if output != expectedOutput {
			t.Fatalf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("UsesSSHNotLocalExecution", func(t *testing.T) {
		expectedOutput := "ssh output"
		command := "remote-command"
		args := []string{"arg1", "arg2"}

		// Given a SecureShell instance with mocks
		shell, mocks := setup(t)
		sshSessionCalled := false
		mocks.Session.RunFunc = func(cmd string) error {
			sshSessionCalled = true
			expectedCmd := command + " " + strings.Join(args, " ")
			if cmd != expectedCmd {
				return fmt.Errorf("unexpected command: %s, expected: %s", cmd, expectedCmd)
			}
			return nil
		}
		mocks.Session.SetStdoutFunc = func(w io.Writer) {
			if _, err := w.Write([]byte(expectedOutput)); err != nil {
				t.Errorf("Failed to write output: %v", err)
			}
		}

		// When calling ExecSilentWithTimeout
		output, err := shell.ExecSilentWithTimeout(command, args, 5*time.Second)

		// Then SSH session should be used (not local execution)
		if err != nil {
			t.Fatalf("Failed to execute command: %v", err)
		}
		if !sshSessionCalled {
			t.Error("Expected SSH session to be called, but it was not")
		}
		if output != expectedOutput {
			t.Fatalf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		command := "slow-command"
		args := []string{"arg"}

		// Given a SecureShell instance with a slow command
		shell, mocks := setup(t)
		mocks.Session.RunFunc = func(cmd string) error {
			time.Sleep(200 * time.Millisecond)
			return nil
		}

		// When calling ExecSilentWithTimeout with a short timeout
		output, err := shell.ExecSilentWithTimeout(command, args, 50*time.Millisecond)

		// Then a timeout error should be returned
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Errorf("Expected timeout error, got: %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output on timeout, got %q", output)
		}
	})

	t.Run("Error", func(t *testing.T) {
		command := "failing-command"
		args := []string{"arg"}

		// Given a SecureShell instance with a failing command
		shell, mocks := setup(t)
		expectedError := fmt.Errorf("command execution failed")
		mocks.Session.RunFunc = func(cmd string) error {
			return expectedError
		}

		// When calling ExecSilentWithTimeout
		output, err := shell.ExecSilentWithTimeout(command, args, 5*time.Second)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "command execution failed") {
			t.Errorf("Expected error to contain 'command execution failed', got %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output on error, got %q", output)
		}
	})

	t.Run("ErrorConnectingToSSH", func(t *testing.T) {
		command := "command"
		args := []string{"arg"}

		// Given a SecureShell instance with connection failure
		shell, mocks := setup(t)
		expectedError := fmt.Errorf("connection failed")
		mocks.Client.ConnectFunc = func() (ssh.ClientConn, error) {
			return nil, expectedError
		}

		// When calling ExecSilentWithTimeout
		output, err := shell.ExecSilentWithTimeout(command, args, 5*time.Second)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error from Connect, got nil")
		}
		if !strings.Contains(err.Error(), "failed to connect to SSH client") {
			t.Errorf("Expected error to contain 'failed to connect to SSH client', got %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
	})
}
