package shell

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"text/template"

	"github.com/windsorcli/cli/pkg/di"
)

// The ShellTest is a test suite for the Shell interface and its implementations.
// It provides comprehensive test coverage for shell operations, command execution,
// project root detection, and environment management.
// The ShellTest acts as a validation framework for shell functionality,
// ensuring reliable command execution, proper error handling, and environment isolation.

// =============================================================================
// Test Setup
// =============================================================================

// setupShellTest creates a new DefaultShell for testing
func setupShellTest(t *testing.T) *DefaultShell {
	// Create a new injector
	injector := di.NewInjector()
	// Create a new default shell
	shell := NewDefaultShell(injector)
	// Initialize the shell
	err := shell.Initialize()
	if err != nil {
		t.Fatalf("Failed to initialize shell: %v", err)
	}
	return shell
}

// =============================================================================
// Test Helpers
// =============================================================================

// Helper function to test random string generation
func testRandomStringGeneration(t *testing.T, shell *DefaultShell, length int) {
	t.Helper()

	// Generate a string of the specified length
	token, err := shell.generateRandomString(length)

	// Verify no errors
	if err != nil {
		t.Fatalf("generateRandomString() error: %v", err)
	}

	// Verify correct length
	if len(token) != length {
		t.Errorf("Expected token to have length %d, got %d", length, len(token))
	}

	// Check that token only contains expected characters
	validChars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	for _, char := range token {
		if !strings.ContainsRune(validChars, char) {
			t.Errorf("Token contains unexpected character: %c", char)
		}
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestShell_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		injector := di.NewInjector()

		// Given a DefaultShell instance
		shell := NewDefaultShell(injector)

		// When calling Initialize
		err := shell.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Initialize() error = %v, wantErr %v", err, false)
		}
	})
}

func TestShell_SetVerbosity(t *testing.T) {
	t.Run("Set to True", func(t *testing.T) {
		shell := NewDefaultShell(nil)
		shell.SetVerbosity(true)
		if !shell.verbose {
			t.Fatalf("Expected verbosity to be true, got false")
		}
	})

	t.Run("Set to False", func(t *testing.T) {
		shell := NewDefaultShell(nil)
		shell.SetVerbosity(false)
		if shell.verbose {
			t.Fatalf("Expected verbosity to be false, got true")
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestShell_GetProjectRoot(t *testing.T) {
	t.Run("Cached", func(t *testing.T) {
		injector := di.NewInjector()

		// Given a temporary directory with a cached project root
		rootDir := createTempDir(t, "project-root")
		subDir := filepath.Join(rootDir, "subdir")
		if err := os.Mkdir(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdir: %v", err)
		}

		changeDir(t, subDir)

		// When calling GetProjectRoot
		shell := NewDefaultShell(injector)
		shell.projectRoot = rootDir // Simulate cached project root
		cachedProjectRoot, err := shell.GetProjectRoot()

		// Then the cached project root should be returned without error
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Normalize paths for Windows compatibility
		expectedRootDir := normalizePath(rootDir)
		cachedProjectRoot = normalizePath(cachedProjectRoot)

		if expectedRootDir != cachedProjectRoot {
			t.Errorf("Expected cached project root %q, got %q", expectedRootDir, cachedProjectRoot)
		}
	})

	t.Run("MaxDepthExceeded", func(t *testing.T) {
		injector := di.NewInjector()

		// Mock the getwd function to simulate directory structure
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return "/mock/deep/directory/structure/level1/level2/level3/level4/level5/level6/level7/level8/level9/level10/level11", nil
		}

		// Mock the osStat function to simulate file existence
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When calling GetProjectRoot
		shell := NewDefaultShell(injector)
		projectRoot, err := shell.GetProjectRoot()

		// Then the project root should be the original directory due to max depth exceeded
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expectedProjectRoot := "/mock/deep/directory/structure/level1/level2/level3/level4/level5/level6/level7/level8/level9/level10/level11"
		if projectRoot != expectedProjectRoot {
			t.Errorf("Expected project root to be %q, got %q", expectedProjectRoot, projectRoot)
		}
	})

	t.Run("NoGitNoYaml", func(t *testing.T) {
		injector := di.NewInjector()

		// Mock the getwd function to simulate directory structure
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return "/mock/current/dir/subdir", nil
		}

		// Mock the osStat function to simulate file existence
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When calling GetProjectRoot
		shell := NewDefaultShell(injector)
		projectRoot, err := shell.GetProjectRoot()

		// Then the project root should be the original directory
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if projectRoot != "/mock/current/dir/subdir" {
			t.Errorf("Expected project root to be %q, got %q", "/mock/current/dir/subdir", projectRoot)
		}
	})

	t.Run("GetwdFails", func(t *testing.T) {
		injector := di.NewInjector()

		// Given a simulated error in getwd
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "", errors.New("simulated error")
		}
		defer func() { getwd = originalGetwd }()

		// When calling GetProjectRoot
		shell := NewDefaultShell(injector)
		_, err := shell.GetProjectRoot()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})
}

func TestShell_Exec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		expectedOutput := "hello\n"
		command := "echo"
		args := []string{"hello"}

		// Mock execCommand to simulate command execution
		originalExecCommand := execCommand
		execCommand = func(name string, arg ...string) *exec.Cmd {
			cmd := exec.Command("echo", "hello")
			cmd.Stdout = &bytes.Buffer{}
			return cmd
		}
		defer func() { execCommand = originalExecCommand }()

		// Mock cmdStart to simulate successful command start
		originalCmdStart := cmdStart
		cmdStart = func(cmd *exec.Cmd) error {
			return nil
		}
		defer func() { cmdStart = originalCmdStart }()

		// Mock cmdWait to simulate successful command execution
		originalCmdWait := cmdWait
		cmdWait = func(cmd *exec.Cmd) error {
			cmd.Stdout.Write([]byte("hello\n"))
			return nil
		}
		defer func() { cmdWait = originalCmdWait }()

		injector := di.NewInjector()
		shell := NewDefaultShell(injector)

		output, err := shell.Exec(command, args...)
		if err != nil {
			t.Fatalf("Failed to execute command: %v", err)
		}
		if output != expectedOutput {
			t.Fatalf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorRunningCommand", func(t *testing.T) {
		command := "nonexistentcommand"
		args := []string{}

		// Mock cmdStart to simulate command execution failure
		originalCmdStart := cmdStart
		cmdStart = func(cmd *exec.Cmd) error {
			return fmt.Errorf("command start failed: exec: \"%s\": executable file not found in $PATH", command)
		}
		defer func() { cmdStart = originalCmdStart }()

		shell := NewDefaultShell(nil)

		_, err := shell.Exec(command, args...)
		if err == nil {
			t.Fatalf("Expected error when executing nonexistent command, got nil")
		}
		expectedError := fmt.Sprintf("command start failed: exec: \"%s\": executable file not found in $PATH", command)
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorWaitingForCommand", func(t *testing.T) {
		command := "echo"
		args := []string{"hello"}

		// Mock execCommand to simulate command execution
		originalExecCommand := execCommand
		execCommand = mockExecCommandError
		defer func() { execCommand = originalExecCommand }()

		// Mock cmdStart to simulate successful command start
		originalCmdStart := cmdStart
		cmdStart = func(cmd *exec.Cmd) error {
			return nil
		}
		defer func() { cmdStart = originalCmdStart }()

		// Mock cmdWait to simulate an error when waiting for the command
		originalCmdWait := cmdWait
		cmdWait = func(cmd *exec.Cmd) error {
			return fmt.Errorf("failed to wait for command")
		}
		defer func() { cmdWait = originalCmdWait }()

		shell := NewDefaultShell(nil)
		_, err := shell.Exec(command, args...)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "failed to wait for command"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})
}

func TestShell_ExecSudo(t *testing.T) {
	// Mock cmdRun, cmdStart, cmdWait, and osOpenFile to simulate command execution
	originalCmdRun := cmdRun
	originalCmdStart := cmdStart
	originalCmdWait := cmdWait
	originalOsOpenFile := osOpenFile

	defer func() {
		cmdRun = originalCmdRun
		cmdStart = originalCmdStart
		cmdWait = originalCmdWait
		osOpenFile = originalOsOpenFile
	}()

	cmdRun = func(cmd *exec.Cmd) error {
		_, _ = cmd.Stdout.Write([]byte("hello\n"))
		return nil
	}
	cmdStart = func(cmd *exec.Cmd) error {
		_, _ = cmd.Stdout.Write([]byte("hello\n"))
		return nil
	}
	cmdWait = func(_ *exec.Cmd) error {
		return nil
	}
	osOpenFile = func(_ string, _ int, _ os.FileMode) (*os.File, error) {
		return &os.File{}, nil
	}

	t.Run("Success", func(t *testing.T) {
		command := "echo"
		args := []string{"hello"}

		shell := NewDefaultShell(nil)

		output, err := shell.ExecSudo("Test Sudo Command", command, args...)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedOutput := "hello\n"
		if output != expectedOutput {
			t.Fatalf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorOpeningTTY", func(t *testing.T) {
		// Mock osOpenFile to simulate an error when opening /dev/tty
		originalOsOpenFile := osOpenFile
		osOpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
			if name == "/dev/tty" {
				return nil, fmt.Errorf("failed to open /dev/tty")
			}
			return originalOsOpenFile(name, flag, perm)
		}
		defer func() { osOpenFile = originalOsOpenFile }() // Restore original function after test

		shell := NewDefaultShell(nil)
		_, err := shell.ExecSudo("Test Sudo Command", "echo", "hello")
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "failed to open /dev/tty"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorStartingCommand", func(t *testing.T) {
		// Mock cmdStart to simulate an error when starting the command
		originalCmdStart := cmdStart
		cmdStart = func(cmd *exec.Cmd) error {
			return fmt.Errorf("failed to start command")
		}
		defer func() {
			cmdStart = originalCmdStart
		}()

		command := "echo"
		args := []string{"hello"}
		shell := NewDefaultShell(nil)
		_, err := shell.ExecSudo("Test Sudo Command", command, args...)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "failed to start command"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorWaitingForCommand", func(t *testing.T) {
		// Mock cmdWait to simulate an error when waiting for the command
		cmdWait = func(cmd *exec.Cmd) error {
			return fmt.Errorf("failed to wait for command")
		}
		defer func() { cmdWait = func(cmd *exec.Cmd) error { return cmd.Wait() } }() // Restore original function after test

		command := "echo"
		args := []string{"hello"}
		shell := NewDefaultShell(nil)
		_, err := shell.ExecSudo("Test Sudo Command", command, args...)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "failed to wait for command"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("VerboseOutput", func(t *testing.T) {
		command := "echo"
		args := []string{"hello"}

		shell := NewDefaultShell(nil)
		shell.SetVerbosity(true)

		// Mock execCommand to simulate command execution
		originalExecCommand := execCommand
		execCommand = func(name string, arg ...string) *exec.Cmd {
			cmd := &exec.Cmd{
				Stdout: &bytes.Buffer{},
				Stderr: &bytes.Buffer{},
			}
			return cmd
		}
		defer func() { execCommand = originalExecCommand }()

		// Mock cmdStart to simulate successful command start
		originalCmdStart := cmdStart
		cmdStart = func(cmd *exec.Cmd) error {
			_, _ = cmd.Stdout.Write([]byte("hello\n"))
			return nil
		}
		defer func() { cmdStart = originalCmdStart }()

		// Mock cmdWait to simulate successful command completion
		originalCmdWait := cmdWait
		cmdWait = func(cmd *exec.Cmd) error {
			return nil
		}
		defer func() { cmdWait = originalCmdWait }()

		stdout, stderr := captureStdoutAndStderr(t, func() {
			output, err := shell.ExecSudo("Test Sudo Command", command, args...)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			expectedOutput := "hello\n"
			if output != expectedOutput {
				t.Fatalf("Expected output %q, got %q", expectedOutput, output)
			}
		})

		// Validate stdout and stderr
		expectedStdout := "hello\n"
		if stdout != expectedStdout {
			t.Fatalf("Expected stdout %q, got %q", expectedStdout, stdout)
		}

		expectedVerboseOutput := "Test Sudo Command\n"
		if !strings.Contains(stderr, expectedVerboseOutput) {
			t.Fatalf("Expected verbose output %q, got stderr: %q", expectedVerboseOutput, stderr)
		}
	})
}

func TestShell_ExecSilent(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		command := "go"
		args := []string{"version"}

		shell := NewDefaultShell(nil)
		output, err := shell.ExecSilent(command, args...)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedOutputPrefix := "go version"
		if !strings.HasPrefix(output, expectedOutputPrefix) {
			t.Fatalf("Expected output to start with %q, got %q", expectedOutputPrefix, output)
		}
	})

	t.Run("ErrorRunningCommand", func(t *testing.T) {
		// Mock cmdRun to simulate an error when running the command
		cmdRun = func(cmd *exec.Cmd) error {
			return fmt.Errorf("failed to run command")
		}
		defer func() { cmdRun = func(cmd *exec.Cmd) error { return cmd.Run() } }() // Restore original function after test

		command := "nonexistentcommand"
		args := []string{}
		shell := NewDefaultShell(nil)
		_, err := shell.ExecSilent(command, args...)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "command execution failed"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("VerboseOutput", func(t *testing.T) {
		command := "go"
		args := []string{"version"}

		// Mock execCommand to simulate command execution
		originalExecCommand := execCommand
		execCommand = func(name string, arg ...string) *exec.Cmd {
			cmd := &exec.Cmd{
				Stdout: &bytes.Buffer{},
				Stderr: &bytes.Buffer{},
			}
			cmd.Stdout.Write([]byte("go version go1.16.3\n"))
			return cmd
		}
		defer func() { execCommand = originalExecCommand }()

		// Mock cmdStart and cmdWait to simulate command execution without hanging
		originalCmdStart := cmdStart
		cmdStart = func(cmd *exec.Cmd) error {
			cmd.Stdout.Write([]byte("go version go1.16.3\n"))
			return nil
		}
		defer func() { cmdStart = originalCmdStart }()

		originalCmdWait := cmdWait
		cmdWait = func(cmd *exec.Cmd) error {
			return nil
		}
		defer func() { cmdWait = originalCmdWait }()

		shell := NewDefaultShell(nil)
		shell.SetVerbosity(true)

		stdout, _ := captureStdoutAndStderr(t, func() {
			output, err := shell.ExecSilent(command, args...)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			expectedOutputPrefix := "go version"
			if !strings.HasPrefix(output, expectedOutputPrefix) {
				t.Fatalf("Expected output to start with %q, got %q", expectedOutputPrefix, output)
			}
		})

		expectedVerboseOutput := "go version"
		if !strings.Contains(stdout, expectedVerboseOutput) {
			t.Fatalf("Expected verbose output to contain %q, got %q", expectedVerboseOutput, stdout)
		}
	})
}

func TestShell_ExecProgress(t *testing.T) {
	// Helper function to mock a command execution
	mockCommandExecution := func() {
		execCommand = func(command string, args ...string) *exec.Cmd {
			return exec.Command("go", "version")
		}
	}

	// Helper function to mock stdout pipe
	mockStdoutPipe := func() {
		cmdStdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				defer w.Close()
				w.Write([]byte("go version go1.16.3\n"))
			}()
			return r, nil
		}
	}

	// Helper function to mock stderr pipe
	mockStderrPipe := func() {
		cmdStderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				defer w.Close()
			}()
			return r, nil
		}
	}

	// Save original functions
	originalExecCommand := execCommand
	originalCmdStdoutPipe := cmdStdoutPipe
	originalCmdStderrPipe := cmdStderrPipe

	// Mock functions
	mockCommandExecution()
	mockStdoutPipe()
	mockStderrPipe()

	// Restore original functions after test
	defer func() {
		execCommand = originalExecCommand
		cmdStdoutPipe = originalCmdStdoutPipe
		cmdStderrPipe = originalCmdStderrPipe
	}()

	t.Run("Success", func(t *testing.T) {
		command := "go"
		args := []string{"version"}

		shell := NewDefaultShell(nil)
		output, err := shell.ExecProgress("Test Progress Command", command, args...)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedOutput := "go version go1.16.3\n"
		if output != expectedOutput {
			t.Fatalf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrStdoutPipe", func(t *testing.T) {
		command := "go"
		args := []string{"version"}

		// Mock cmdStdoutPipe to simulate an error
		originalCmdStdoutPipe := cmdStdoutPipe
		cmdStdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			return nil, fmt.Errorf("failed to create stdout pipe")
		}
		defer func() { cmdStdoutPipe = originalCmdStdoutPipe }() // Restore original function after test

		shell := NewDefaultShell(nil)
		_, err := shell.ExecProgress("Test Progress Command", command, args...)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "failed to create stdout pipe"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrStderrPipe", func(t *testing.T) {
		command := "go"
		args := []string{"version"}

		// Mock cmdStderrPipe to simulate an error
		originalCmdStderrPipe := cmdStderrPipe
		cmdStderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			return nil, fmt.Errorf("failed to create stderr pipe")
		}
		defer func() { cmdStderrPipe = originalCmdStderrPipe }() // Restore original function after test

		shell := NewDefaultShell(nil)
		_, err := shell.ExecProgress("Test Progress Command", command, args...)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "failed to create stderr pipe"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrStartCommand", func(t *testing.T) {
		command := "go"
		args := []string{"version"}

		// Mock cmdStart to simulate an error
		originalCmdStart := cmdStart
		cmdStart = func(cmd *exec.Cmd) error {
			return fmt.Errorf("failed to start command")
		}
		defer func() { cmdStart = originalCmdStart }() // Restore original function after test

		shell := NewDefaultShell(nil)
		_, err := shell.ExecProgress("Test Progress Command", command, args...)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "failed to start command"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrBufioScannerScan", func(t *testing.T) {
		command := "go"
		args := []string{"version"}

		// Mock bufioScannerScan to simulate an error
		originalBufioScannerScan := bufioScannerScan
		bufioScannerScan = func(scanner *bufio.Scanner) bool {
			return false
		}
		defer func() { bufioScannerScan = originalBufioScannerScan }() // Restore original function after test

		// Mock bufioScannerErr to return an error
		originalBufioScannerErr := bufioScannerErr
		bufioScannerErr = func(scanner *bufio.Scanner) error {
			return fmt.Errorf("error reading stdout")
		}
		defer func() { bufioScannerErr = originalBufioScannerErr }() // Restore original function after test

		shell := NewDefaultShell(nil)
		_, err := shell.ExecProgress("Test Progress Command", command, args...)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "error reading stdout"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrBufioScannerErr", func(t *testing.T) {
		command := "go"
		args := []string{"version"}

		// Mock cmdStdoutPipe and cmdStderrPipe to return a pipe that can be scanned
		originalCmdStdoutPipe := cmdStdoutPipe
		cmdStdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				defer w.Close()
				w.Write([]byte("stdout line\n"))
			}()
			return r, nil
		}
		defer func() { cmdStdoutPipe = originalCmdStdoutPipe }()

		originalCmdStderrPipe := cmdStderrPipe
		cmdStderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
			r, w := io.Pipe()
			go func() {
				defer w.Close()
				w.Write([]byte("stderr line\n"))
			}()
			return r, nil
		}
		defer func() { cmdStderrPipe = originalCmdStderrPipe }()

		// Mock bufioScannerErr to return an error
		originalBufioScannerErr := bufioScannerErr
		bufioScannerErr = func(scanner *bufio.Scanner) error {
			return fmt.Errorf("error reading stderr")
		}
		defer func() { bufioScannerErr = originalBufioScannerErr }() // Restore original function after test

		shell := NewDefaultShell(nil)
		_, err := shell.ExecProgress("Test Progress Command", command, args...)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "error reading stderr"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrCmdWait", func(t *testing.T) {
		command := "go"
		args := []string{"version"}

		// Mock cmdWait to return an error
		originalCmdWait := cmdWait
		cmdWait = func(cmd *exec.Cmd) error {
			return fmt.Errorf("error waiting for command")
		}
		defer func() { cmdWait = originalCmdWait }() // Restore original function after test

		shell := NewDefaultShell(nil)
		_, err := shell.ExecProgress("Test Progress Command", command, args...)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "error waiting for command"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("VerboseOutput", func(t *testing.T) {
		command := "go"
		args := []string{"version"}

		shell := NewDefaultShell(nil)
		shell.SetVerbosity(true)

		// Mock execCommand to simulate command execution
		originalExecCommand := execCommand
		execCommand = func(name string, arg ...string) *exec.Cmd {
			cmd := &exec.Cmd{
				Stdout: &bytes.Buffer{},
				Stderr: &bytes.Buffer{},
			}
			return cmd
		}
		defer func() { execCommand = originalExecCommand }() // Restore original function after test

		// Mock cmdStart and cmdWait to simulate command execution without hanging
		originalCmdStart := cmdStart
		cmdStart = func(cmd *exec.Cmd) error {
			cmd.Stdout.Write([]byte("go version go1.16.3 darwin/amd64\n"))
			return nil
		}
		defer func() { cmdStart = originalCmdStart }() // Restore original function after test

		originalCmdWait := cmdWait
		cmdWait = func(cmd *exec.Cmd) error {
			return nil
		}
		defer func() { cmdWait = originalCmdWait }() // Restore original function after test

		stdout, stderr := captureStdoutAndStderr(t, func() {
			output, err := shell.ExecProgress("Test Progress Command", command, args...)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			expectedOutputPrefix := "go version"
			if !strings.HasPrefix(output, expectedOutputPrefix) {
				t.Fatalf("Expected output to start with %q, got %q", expectedOutputPrefix, output)
			}
		})

		expectedVerboseOutput := "Test Progress Command\n"
		if !strings.Contains(stderr, expectedVerboseOutput) {
			t.Fatalf("Expected verbose output %q, got %q", expectedVerboseOutput, stderr)
		}

		// Check the stdout value
		expectedStdoutPrefix := "go version"
		if !strings.HasPrefix(stdout, expectedStdoutPrefix) {
			t.Fatalf("Expected stdout to start with %q, got %q", expectedStdoutPrefix, stdout)
		}
	})
}

func TestShell_InstallHook(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		shell := NewDefaultShell(nil)

		// Capture stdout to validate the output
		output := captureStdout(t, func() {
			if err := shell.InstallHook("bash"); err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
		})

		// Validate the output contains expected content
		expectedOutput := "_windsor_hook" // Replace with actual expected output
		if !strings.Contains(output, expectedOutput) {
			t.Fatalf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("PowerShellSuccess", func(t *testing.T) {
		shell := NewDefaultShell(nil)

		// Capture stdout to validate the output
		output := captureStdout(t, func() {
			if err := shell.InstallHook("powershell"); err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
		})

		// Validate the output contains expected content
		expectedOutput := "function prompt" // Replace with actual expected output for PowerShell
		if !strings.Contains(output, expectedOutput) {
			t.Fatalf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("UnsupportedShell", func(t *testing.T) {
		shell := NewDefaultShell(nil)
		err := shell.InstallHook("unsupported-shell")
		if err == nil {
			t.Fatalf("Expected an error for unsupported shell, but got nil")
		} else {
			expectedError := "Unsupported shell: unsupported-shell"
			if err.Error() != expectedError {
				t.Fatalf("Expected error message %q, but got %q", expectedError, err.Error())
			}
		}
	})

	t.Run("ErrorGettingSelfPath", func(t *testing.T) {
		shell := NewDefaultShell(nil)

		// Mock osExecutable to simulate an error
		originalOsExecutable := osExecutable
		osExecutable = func() (string, error) {
			return "", fmt.Errorf("executable file not found")
		}
		defer func() { osExecutable = originalOsExecutable }() // Restore original function after test

		err := shell.InstallHook("bash")
		if err == nil {
			t.Fatalf("Expected error due to self path retrieval failure, but got nil")
		} else {
			expectedError := "executable file not found"
			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("Expected error message to contain %q, but got %q", expectedError, err.Error())
			}
		}
	})

	t.Run("ErrorCreatingNewTemplate", func(t *testing.T) {
		shell := NewDefaultShell(nil)

		// Mock hookTemplateNew to simulate an error
		originalHookTemplateNew := hookTemplateNew
		hookTemplateNew = func(name string) *template.Template {
			return nil
		}
		defer func() { hookTemplateNew = originalHookTemplateNew }() // Restore original function after test

		err := shell.InstallHook("bash")
		if err == nil {
			t.Fatalf("Expected error due to hook template creation failure, but got nil")
		} else {
			expectedError := "failed to create new template"
			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("Expected error message to contain %q, but got %q", expectedError, err.Error())
			}
		}
	})

	t.Run("ErrorParsingHookTemplate", func(t *testing.T) {
		shell := NewDefaultShell(nil)

		// Mock hookTemplateParse to simulate a parsing error
		originalHookTemplateParse := hookTemplateParse
		hookTemplateParse = func(tmpl *template.Template, text string) (*template.Template, error) {
			return nil, fmt.Errorf("template parsing error")
		}
		defer func() { hookTemplateParse = originalHookTemplateParse }() // Restore original function after test

		err := shell.InstallHook("bash")
		if err == nil {
			t.Fatalf("Expected error due to hook template parsing failure, but got nil")
		} else {
			expectedError := "template parsing error"
			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("Expected error message to contain %q, but got %q", expectedError, err.Error())
			}
		}
	})

	t.Run("ErrorParsingHookTemplate", func(t *testing.T) {
		shell := NewDefaultShell(nil)

		// Mock shellHooks to provide an invalid template command
		originalShellHooks := shellHooks
		shellHooks = map[string]string{
			"bash": "{{ .InvalidField }}", // Invalid template field to cause parsing error
		}
		defer func() { shellHooks = originalShellHooks }() // Restore original shellHooks after test

		err := shell.InstallHook("bash")
		if err == nil {
			t.Fatalf("Expected error due to hook template parsing failure, but got nil")
		} else {
			expectedError := "can't evaluate field InvalidField"
			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("Expected error message to contain %q, but got %q", expectedError, err.Error())
			}
		}
	})
}

var tempDirs []string

// Helper function to create a temporary directory
func createTempDir(t *testing.T, name string) string {
	dir, err := os.MkdirTemp("", name)
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	tempDirs = append(tempDirs, dir)
	return dir
}

// Helper function to create a file with specified content
func createFile(t *testing.T, dir, name, content string) {
	filePath := filepath.Join(dir, name)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create file %s: %v", filePath, err)
	}
}

// Helper function to change the working directory
func changeDir(t *testing.T, dir string) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Fatalf("Failed to revert to original directory: %v", err)
		}
	})
}

// Helper function to normalize a path
func normalizePath(path string) string {
	return strings.ReplaceAll(filepath.Clean(path), "\\", "/")
}

// Helper function to capture stdout
func captureStdout(t *testing.T, f func()) string {
	var output bytes.Buffer
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
		w.Close()
	}()

	_, err := output.ReadFrom(r)
	if err != nil {
		t.Fatalf("Failed to read from pipe: %v", err)
	}
	<-done
	os.Stdout = originalStdout
	return output.String()
}

// Updated helper function to mock exec.Command for failed execution using PowerShell
func mockExecCommandError(command string, args ...string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		// Use PowerShell to simulate a failing command
		fullCommand := fmt.Sprintf("exit 1; Write-Error 'mock error for: %s %s'", command, strings.Join(args, " "))
		cmdArgs := []string{"-Command", fullCommand}
		return exec.Command("powershell.exe", cmdArgs...)
	} else {
		// Use 'false' command on Unix-like systems
		return exec.Command("false")
	}
}

// captureStdoutAndStderr captures output sent to os.Stdout and os.Stderr during the execution of f()
func captureStdoutAndStderr(t *testing.T, f func()) (string, string) {
	// Save the original os.Stdout and os.Stderr
	originalStdout := os.Stdout
	originalStderr := os.Stderr

	// Create pipes for os.Stdout and os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	// Channel to signal completion
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
		wOut.Close()
		wErr.Close()
	}()

	// Read from the pipes
	var stdoutBuf, stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	readFromPipe := func(pipe *os.File, buf *bytes.Buffer, pipeName string) {
		defer wg.Done()
		if _, err := buf.ReadFrom(pipe); err != nil {
			t.Errorf("Failed to read from %s pipe: %v", pipeName, err)
		}
	}
	go readFromPipe(rOut, &stdoutBuf, "stdout")
	go readFromPipe(rErr, &stderrBuf, "stderr")

	// Wait for reading to complete
	wg.Wait()
	<-done

	// Restore os.Stdout and os.Stderr
	os.Stdout = originalStdout
	os.Stderr = originalStderr

	return stdoutBuf.String(), stderrBuf.String()
}

func TestEnv_CheckTrustedDirectory(t *testing.T) {
	// Mock the getwd function
	originalGetwd := getwd
	originalOsUserHomeDir := osUserHomeDir
	originalReadFile := osReadFile

	defer func() {
		getwd = originalGetwd
		osUserHomeDir = originalOsUserHomeDir
		osReadFile = originalReadFile
	}()

	getwd = func() (string, error) {
		return "/mock/current/dir", nil
	}

	osUserHomeDir = func() (string, error) {
		return "/mock/home/dir", nil
	}

	osReadFile = func(filename string) ([]byte, error) {
		return []byte("/mock/current/dir\n"), nil
	}

	t.Run("Success", func(t *testing.T) {
		shell := NewDefaultShell(di.NewInjector())
		shell.Initialize()

		// Call CheckTrustedDirectory and check for errors
		err := shell.CheckTrustedDirectory()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorGettingCurrentDir", func(t *testing.T) {
		// Save the original getwd function
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()

		// Override the getwd function locally to simulate an error
		getwd = func() (string, error) {
			return "", fmt.Errorf("Error getting current directory: error getting current directory")
		}

		// Call CheckTrustedDirectory and expect an error
		shell := &DefaultShell{}
		err := shell.CheckTrustedDirectory()
		if err == nil || !strings.Contains(err.Error(), "error getting current directory") {
			t.Errorf("expected error containing 'error getting current directory', got %v", err)
		}
	})

	t.Run("ErrorGettingUserHomeDir", func(t *testing.T) {
		// Save the original osUserHomeDir function
		originalOsUserHomeDir := osUserHomeDir
		defer func() { osUserHomeDir = originalOsUserHomeDir }()

		// Override the osUserHomeDir function locally to simulate an error
		osUserHomeDir = func() (string, error) {
			return "", fmt.Errorf("Error getting user home directory: error getting user home directory")
		}

		// Call CheckTrustedDirectory and expect an error
		shell := &DefaultShell{}
		err := shell.CheckTrustedDirectory()
		if err == nil || !strings.Contains(err.Error(), "Error getting user home directory") {
			t.Errorf("expected error containing 'Error getting user home directory', got %v", err)
		}
	})

	t.Run("ErrorReadingTrustedFile", func(t *testing.T) {
		// Save the original osReadFile function
		originalReadFile := osReadFile
		defer func() { osReadFile = originalReadFile }()

		// Override the osReadFile function locally to simulate an error
		osReadFile = func(filename string) ([]byte, error) {
			return nil, fmt.Errorf("error reading trusted file")
		}

		// Call CheckTrustedDirectory and expect an error
		shell := &DefaultShell{}
		err := shell.CheckTrustedDirectory()
		if err == nil || !strings.Contains(err.Error(), "error reading trusted file") {
			t.Errorf("expected error containing 'error reading trusted file', got %v", err)
		}
	})

	t.Run("TrustedFileDoesNotExist", func(t *testing.T) {
		// Save the original osReadFile function
		originalReadFile := osReadFile
		defer func() { osReadFile = originalReadFile }()

		// Override the osReadFile function locally to simulate a non-existent trusted file
		osReadFile = func(filename string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		// Call CheckTrustedDirectory and expect an error
		shell := &DefaultShell{}
		err := shell.CheckTrustedDirectory()
		if err == nil || err.Error() != "Trusted file does not exist" {
			t.Errorf("expected error 'Trusted file does not exist', got %v", err)
		}
	})

	t.Run("CurrentDirNotInTrustedList", func(t *testing.T) {
		// Mock the getwd function to return a specific current directory
		getwd = func() (string, error) {
			return "/mock/current/dir", nil
		}

		// Mock the osReadFile function to simulate a trusted file without the current directory
		osReadFile = func(filename string) ([]byte, error) {
			return []byte("/mock/other/dir\n"), nil
		}

		// Execute CheckTrustedDirectory and verify it returns the expected error
		shell := &DefaultShell{}
		err := shell.CheckTrustedDirectory()
		if err == nil || !strings.Contains(err.Error(), "Current directory not in the trusted list") {
			t.Errorf("expected error 'Current directory not in the trusted list', got %v", err)
		}
	})
}

func TestDefaultShell_GetSessionToken(t *testing.T) {
	t.Run("GenerateNewToken", func(t *testing.T) {
		// Given
		ResetSessionToken()
		shell := setupShellTest(t)

		// Save original functions to restore later
		originalRandRead := randRead
		originalOsGetenv := osGetenv
		defer func() {
			randRead = originalRandRead
			osGetenv = originalOsGetenv
		}()

		// Mock osGetenv to return empty string (no env token)
		osGetenv = func(key string) string {
			return ""
		}

		// Create a deterministic token generator
		randRead = func(b []byte) (n int, err error) {
			for i := range b {
				b[i] = byte(i % 62) // Use a deterministic pattern
			}
			return len(b), nil
		}

		// When
		token, err := shell.GetSessionToken()

		// Then
		if err != nil {
			t.Errorf("GetSessionToken() error = %v, want nil", err)
		}
		if len(token) != 7 {
			t.Errorf("GetSessionToken() token length = %d, want 7", len(token))
		}
	})

	t.Run("ReuseExistingToken", func(t *testing.T) {
		// Given
		ResetSessionToken()
		shell := setupShellTest(t)

		// Save original functions
		originalRandRead := randRead
		originalOsGetenv := osGetenv
		defer func() {
			randRead = originalRandRead
			osGetenv = originalOsGetenv
		}()

		// Mock rand.Read to generate a predictable token
		randRead = func(b []byte) (n int, err error) {
			for i := range b {
				b[i] = byte(i % 62) // Use a deterministic pattern
			}
			return len(b), nil
		}

		// Generate a first token to cache it
		firstToken, _ := shell.GetSessionToken()

		// When getting a second token
		secondToken, err := shell.GetSessionToken()

		// Then
		if err != nil {
			t.Errorf("GetSessionToken() error = %v, want nil", err)
		}
		if firstToken != secondToken {
			t.Errorf("GetSessionToken() token = %s, want %s", secondToken, firstToken)
		}
	})

	t.Run("UseEnvironmentToken", func(t *testing.T) {
		// Given
		ResetSessionToken()
		shell := setupShellTest(t)

		// Save original functions
		originalOsGetenv := osGetenv
		defer func() {
			osGetenv = originalOsGetenv
		}()

		// Mock osGetenv to return a specific token
		osGetenv = func(key string) string {
			if key == "WINDSOR_SESSION_TOKEN" {
				return "testtoken"
			}
			return ""
		}

		// When
		token, err := shell.GetSessionToken()

		// Then
		if err != nil {
			t.Errorf("GetSessionToken() error = %v, want nil", err)
		}
		if token != "testtoken" {
			t.Errorf("GetSessionToken() token = %s, want testtoken", token)
		}
	})

	t.Run("ErrorGeneratingRandomString", func(t *testing.T) {
		// Given
		ResetSessionToken()
		shell := setupShellTest(t)

		// Save original functions
		originalRandRead := randRead
		originalOsGetenv := osGetenv
		defer func() {
			randRead = originalRandRead
			osGetenv = originalOsGetenv
		}()

		// Mock osGetenv to return empty string (no env token)
		osGetenv = func(key string) string {
			return ""
		}

		// Mock random generation to fail
		randRead = func(b []byte) (n int, err error) {
			return 0, fmt.Errorf("mock random generation error")
		}

		// When
		token, err := shell.GetSessionToken()

		// Then
		if err == nil {
			t.Error("GetSessionToken() expected error, got nil")
			return
		}
		if token != "" {
			t.Errorf("GetSessionToken() token = %s, want empty string", token)
		}
		expectedErr := "error generating session token: mock random generation error"
		if err.Error() != expectedErr {
			t.Errorf("GetSessionToken() error = %v, want %v", err, expectedErr)
		}
	})

	t.Run("GenerateRandomString", func(t *testing.T) {
		// This test checks that generateRandomString properly generates strings of the right length
		shell := setupShellTest(t)

		// Save original randRead
		originalRandRead := randRead
		defer func() { randRead = originalRandRead }()

		// Make randRead produce deterministic output for testing
		randRead = func(b []byte) (n int, err error) {
			for i := range b {
				b[i] = byte(i % 62) // Use a deterministic pattern
			}
			return len(b), nil
		}

		testRandomStringGeneration(t, shell, 7)
		testRandomStringGeneration(t, shell, 10)
		testRandomStringGeneration(t, shell, 15)
	})
}

// TestDefaultShell_WriteResetToken tests the WriteResetToken method
func TestDefaultShell_WriteResetToken(t *testing.T) {
	// Save original functions and environment
	originalOsMkdirAll := osMkdirAll
	originalOsWriteFile := osWriteFile
	originalEnvValue := os.Getenv("WINDSOR_SESSION_TOKEN")

	// Restore original functions and environment after all tests
	defer func() {
		osMkdirAll = originalOsMkdirAll
		osWriteFile = originalOsWriteFile
		if originalEnvValue != "" {
			os.Setenv("WINDSOR_SESSION_TOKEN", originalEnvValue)
		} else {
			os.Unsetenv("WINDSOR_SESSION_TOKEN")
		}
	}()

	t.Run("NoSessionToken", func(t *testing.T) {
		// Given a default shell with no session token in environment
		shell := setupShellTest(t)

		// Ensure the environment variable is not set
		os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// When calling WriteResetToken
		path, err := shell.WriteResetToken()

		// Then no error should be returned and path should be empty
		if err != nil {
			t.Errorf("WriteResetToken() error = %v, want nil", err)
		}
		if path != "" {
			t.Errorf("WriteResetToken() path = %v, want empty string", path)
		}
	})

	t.Run("SuccessfulTokenWrite", func(t *testing.T) {
		// Given a default shell with a session token
		shell := setupShellTest(t)

		// Set up test data using platform-specific path functions
		testProjectRoot := filepath.FromSlash("/test/project/root")
		testToken := "test-token-123"
		expectedDirPath := filepath.Join(testProjectRoot, ".windsor")
		expectedFilePath := filepath.Join(expectedDirPath, SessionTokenPrefix+testToken)

		// For comparison in errors, we'll use ToSlash to show normalized paths
		expectedDirPathNormalized := filepath.ToSlash(expectedDirPath)
		expectedFilePathNormalized := filepath.ToSlash(expectedFilePath)

		// Track function calls
		var mkdirAllCalled bool
		var writeFileCalled bool
		var mkdirAllPath string
		var mkdirAllPerm os.FileMode
		var writeFilePath string
		var writeFileData []byte
		var writeFilePerm os.FileMode

		// Mock OS functions
		osMkdirAll = func(path string, perm os.FileMode) error {
			mkdirAllCalled = true
			mkdirAllPath = path
			mkdirAllPerm = perm
			return nil
		}

		osWriteFile = func(name string, data []byte, perm os.FileMode) error {
			writeFileCalled = true
			writeFilePath = name
			writeFileData = data
			writeFilePerm = perm
			return nil
		}

		// Mock getwd to return our test project root
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return testProjectRoot, nil
		}

		// Set the environment variable
		os.Setenv("WINDSOR_SESSION_TOKEN", testToken)

		// When calling WriteResetToken
		path, err := shell.WriteResetToken()

		// Then no error should be returned and the path should match expected
		if err != nil {
			t.Errorf("WriteResetToken() error = %v, want nil", err)
		}

		// Use ToSlash to normalize paths for comparison
		if filepath.ToSlash(path) != expectedFilePathNormalized {
			t.Errorf("WriteResetToken() path = %v, want %v",
				filepath.ToSlash(path), expectedFilePathNormalized)
		}

		// Verify that MkdirAll was called with correct parameters
		if !mkdirAllCalled {
			t.Error("Expected MkdirAll to be called, but it wasn't")
		} else {
			if filepath.ToSlash(mkdirAllPath) != expectedDirPathNormalized {
				t.Errorf("Expected MkdirAll path %s, got %s",
					expectedDirPathNormalized, filepath.ToSlash(mkdirAllPath))
			}
			if mkdirAllPerm != 0750 {
				t.Errorf("Expected MkdirAll permissions 0750, got %v", mkdirAllPerm)
			}
		}

		// Verify that WriteFile was called with correct parameters
		if !writeFileCalled {
			t.Error("Expected WriteFile to be called, but it wasn't")
		} else {
			if filepath.ToSlash(writeFilePath) != expectedFilePathNormalized {
				t.Errorf("Expected WriteFile path %s, got %s",
					expectedFilePathNormalized, filepath.ToSlash(writeFilePath))
			}
			if len(writeFileData) != 0 {
				t.Errorf("Expected empty file, got %v bytes", len(writeFileData))
			}
			if writeFilePerm != 0600 {
				t.Errorf("Expected WriteFile permissions 0600, got %v", writeFilePerm)
			}
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a default shell with a session token
		shell := setupShellTest(t)

		// Mock getwd to return an error
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return "", fmt.Errorf("error getting project root")
		}

		// Set the environment variable
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")

		// When calling WriteResetToken
		path, err := shell.WriteResetToken()

		// Then an error should be returned and the path should be empty
		if err == nil {
			t.Error("WriteResetToken() expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting project root") {
			t.Errorf("WriteResetToken() error = %v, want error containing 'error getting project root'", err)
		}
		if path != "" {
			t.Errorf("WriteResetToken() path = %v, want empty string", path)
		}
	})

	t.Run("ErrorCreatingDirectory", func(t *testing.T) {
		// Given a default shell with a session token
		shell := setupShellTest(t)

		// Mock getwd to return a test path
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return "/test/project/root", nil
		}

		// Mock MkdirAll to return an error
		expectedError := fmt.Errorf("error creating directory")
		osMkdirAll = func(path string, perm os.FileMode) error {
			return expectedError
		}

		// Set the environment variable
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")

		// When calling WriteResetToken
		path, err := shell.WriteResetToken()

		// Then an error should be returned and the path should be empty
		if err == nil {
			t.Error("WriteResetToken() expected error, got nil")
		}
		if !strings.Contains(err.Error(), expectedError.Error()) {
			t.Errorf("WriteResetToken() error = %v, want error containing %v", err, expectedError)
		}
		if path != "" {
			t.Errorf("WriteResetToken() path = %v, want empty string", path)
		}
	})

	t.Run("ErrorWritingFile", func(t *testing.T) {
		// Given a default shell with a session token
		shell := setupShellTest(t)

		// Mock getwd to return a test path
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return "/test/project/root", nil
		}

		// Mock MkdirAll to succeed but WriteFile to fail
		osMkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		expectedError := fmt.Errorf("error writing file")
		osWriteFile = func(name string, data []byte, perm os.FileMode) error {
			return expectedError
		}

		// Set the environment variable
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")

		// When calling WriteResetToken
		path, err := shell.WriteResetToken()

		// Then an error should be returned and the path should be empty
		if err == nil {
			t.Error("WriteResetToken() expected error, got nil")
		}
		if !strings.Contains(err.Error(), expectedError.Error()) {
			t.Errorf("WriteResetToken() error = %v, want error containing %v", err, expectedError)
		}
		if path != "" {
			t.Errorf("WriteResetToken() path = %v, want empty string", path)
		}
	})
}

// TestDefaultShell_AddCurrentDirToTrustedFile tests the AddCurrentDirToTrustedFile method
func TestDefaultShell_AddCurrentDirToTrustedFile(t *testing.T) {
	// Save original functions and environment
	originalGetwd := getwd
	originalOsUserHomeDir := osUserHomeDir
	originalOsReadFile := osReadFile
	originalOsMkdirAll := osMkdirAll
	originalOsWriteFile := osWriteFile

	// Restore original functions after all tests
	defer func() {
		getwd = originalGetwd
		osUserHomeDir = originalOsUserHomeDir
		osReadFile = originalOsReadFile
		osMkdirAll = originalOsMkdirAll
		osWriteFile = originalOsWriteFile
	}()

	t.Run("Success", func(t *testing.T) {
		// Given a default shell
		shell := setupShellTest(t)

		// Mock required functions
		getwd = func() (string, error) {
			return "/mock/current/dir", nil
		}

		osUserHomeDir = func() (string, error) {
			return "/mock/home/dir", nil
		}

		osReadFile = func(filename string) ([]byte, error) {
			return []byte{}, nil // Empty trusted file
		}

		osMkdirAll = func(path string, perm os.FileMode) error {
			expectedPath := "/mock/home/dir/.config/windsor"
			if filepath.ToSlash(path) != expectedPath {
				t.Errorf("Expected MkdirAll path %s, got %s", expectedPath, path)
			}
			if perm != 0750 {
				t.Errorf("Expected MkdirAll permissions 0750, got %v", perm)
			}
			return nil
		}

		var capturedData []byte
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			expectedPath := "/mock/home/dir/.config/windsor/.trusted"
			if filepath.ToSlash(filename) != expectedPath {
				t.Errorf("Expected WriteFile path %s, got %s", expectedPath, filename)
			}
			capturedData = data
			if perm != 0600 {
				t.Errorf("Expected WriteFile permissions 0600, got %v", perm)
			}
			return nil
		}

		// When adding the current directory to the trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then no error should be returned
		if err != nil {
			t.Errorf("AddCurrentDirToTrustedFile() error = %v, want nil", err)
		}

		// Verify that the current directory was added to the trusted file
		expectedData := "/mock/current/dir\n"
		if string(capturedData) != expectedData {
			t.Errorf("Expected data %q, got %q", expectedData, string(capturedData))
		}
	})

	t.Run("SuccessAlreadyTrusted", func(t *testing.T) {
		// Given a default shell
		shell := setupShellTest(t)

		// Mock required functions
		getwd = func() (string, error) {
			return "/mock/current/dir", nil
		}

		osUserHomeDir = func() (string, error) {
			return "/mock/home/dir", nil
		}

		osReadFile = func(filename string) ([]byte, error) {
			return []byte("/mock/current/dir\n"), nil // Directory already in trusted file
		}

		// Track if WriteFile is called (it shouldn't be)
		writeFileCalled := false
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writeFileCalled = true
			return nil
		}

		// When adding the current directory to the trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then no error should be returned
		if err != nil {
			t.Errorf("AddCurrentDirToTrustedFile() error = %v, want nil", err)
		}

		// Verify that WriteFile was not called
		if writeFileCalled {
			t.Error("Expected WriteFile not to be called, but it was")
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a default shell
		shell := setupShellTest(t)

		// Mock getwd to return an error
		getwd = func() (string, error) {
			return "", fmt.Errorf("error getting project root directory")
		}

		// When adding the current directory to the trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then an error should be returned
		if err == nil {
			t.Error("AddCurrentDirToTrustedFile() expected error, got nil")
		}
		expectedError := "Error getting project root directory: error getting project root directory"
		if err.Error() != expectedError {
			t.Errorf("AddCurrentDirToTrustedFile() error = %q, want %q", err.Error(), expectedError)
		}
	})

	t.Run("ErrorGettingUserHomeDir", func(t *testing.T) {
		// Given a default shell
		shell := setupShellTest(t)

		// Mock getwd to succeed but osUserHomeDir to fail
		getwd = func() (string, error) {
			return "/mock/current/dir", nil
		}

		osUserHomeDir = func() (string, error) {
			return "", fmt.Errorf("error getting user home directory")
		}

		// When adding the current directory to the trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then an error should be returned
		if err == nil {
			t.Error("AddCurrentDirToTrustedFile() expected error, got nil")
		}
		expectedError := "Error getting user home directory: error getting user home directory"
		if err.Error() != expectedError {
			t.Errorf("AddCurrentDirToTrustedFile() error = %q, want %q", err.Error(), expectedError)
		}
	})

	t.Run("ErrorCreatingDirectories", func(t *testing.T) {
		// Given a default shell
		shell := setupShellTest(t)

		// Mock getwd and osUserHomeDir to succeed but osMkdirAll to fail
		getwd = func() (string, error) {
			return "/mock/current/dir", nil
		}

		osUserHomeDir = func() (string, error) {
			return "/mock/home/dir", nil
		}

		expectedError := fmt.Errorf("error creating directories")
		osMkdirAll = func(path string, perm os.FileMode) error {
			return expectedError
		}

		// When adding the current directory to the trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then an error should be returned
		if err == nil {
			t.Error("AddCurrentDirToTrustedFile() expected error, got nil")
		}

		expectedErrorMsg := "Error creating directories for trusted file: error creating directories"
		if err.Error() != expectedErrorMsg {
			t.Errorf("AddCurrentDirToTrustedFile() error = %q, want %q", err.Error(), expectedErrorMsg)
		}
	})

	t.Run("ErrorReadingTrustedFile", func(t *testing.T) {
		// Given a default shell
		shell := setupShellTest(t)

		// Mock getwd, osUserHomeDir, and osMkdirAll to succeed but osReadFile to fail
		getwd = func() (string, error) {
			return "/mock/current/dir", nil
		}

		osUserHomeDir = func() (string, error) {
			return "/mock/home/dir", nil
		}

		osMkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		expectedError := fmt.Errorf("error reading trusted file")
		osReadFile = func(filename string) ([]byte, error) {
			return nil, expectedError
		}

		// When adding the current directory to the trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then an error should be returned
		if err == nil {
			t.Error("AddCurrentDirToTrustedFile() expected error, got nil")
		}

		expectedErrorMsg := "Error reading trusted file: error reading trusted file"
		if err.Error() != expectedErrorMsg {
			t.Errorf("AddCurrentDirToTrustedFile() error = %q, want %q", err.Error(), expectedErrorMsg)
		}
	})

	t.Run("ErrorReadingNonExistentTrustedFile", func(t *testing.T) {
		// Given a default shell
		shell := setupShellTest(t)

		// Mock getwd, osUserHomeDir, and osMkdirAll to succeed but osReadFile to return file not exist error
		getwd = func() (string, error) {
			return "/mock/current/dir", nil
		}

		osUserHomeDir = func() (string, error) {
			return "/mock/home/dir", nil
		}

		osMkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		osReadFile = func(filename string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		var capturedData []byte
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			capturedData = data
			return nil
		}

		// When adding the current directory to the trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then no error should be returned
		if err != nil {
			t.Errorf("AddCurrentDirToTrustedFile() error = %v, want nil", err)
		}

		// Verify that the current directory was added to the trusted file
		expectedData := "/mock/current/dir\n"
		if string(capturedData) != expectedData {
			t.Errorf("Expected data %q, got %q", expectedData, string(capturedData))
		}
	})

	t.Run("ErrorWritingToTrustedFile", func(t *testing.T) {
		// Given a default shell
		shell := setupShellTest(t)

		// Mock getwd, osUserHomeDir, osMkdirAll, and osReadFile to succeed but osWriteFile to fail
		getwd = func() (string, error) {
			return "/mock/current/dir", nil
		}

		osUserHomeDir = func() (string, error) {
			return "/mock/home/dir", nil
		}

		osMkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		osReadFile = func(filename string) ([]byte, error) {
			return []byte{}, nil
		}

		expectedError := fmt.Errorf("error writing to trusted file")
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return expectedError
		}

		// When adding the current directory to the trusted file
		err := shell.AddCurrentDirToTrustedFile()

		// Then an error should be returned
		if err == nil {
			t.Error("AddCurrentDirToTrustedFile() expected error, got nil")
		}

		expectedErrorMsg := "Error writing to trusted file: error writing to trusted file"
		if err.Error() != expectedErrorMsg {
			t.Errorf("AddCurrentDirToTrustedFile() error = %q, want %q", err.Error(), expectedErrorMsg)
		}
	})
}

// TestMockShell_GetSessionToken tests the MockShell's GetSessionToken method
func TestMockShell_GetSessionToken(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)

		expectedToken := "mock-token"
		mockShell.GetSessionTokenFunc = func() (string, error) {
			return expectedToken, nil
		}

		// When
		token, err := mockShell.GetSessionToken()

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if token != expectedToken {
			t.Errorf("Expected token %s, got %s", expectedToken, token)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)

		expectedError := "custom error"
		mockShell.GetSessionTokenFunc = func() (string, error) {
			return "", fmt.Errorf(expectedError)
		}

		// When
		token, err := mockShell.GetSessionToken()

		// Then
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError {
			t.Errorf("Expected error %s, got %s", expectedError, err.Error())
		}
		if token != "" {
			t.Errorf("Expected empty token, got %s", token)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)

		// Don't set GetSessionTokenFunc

		// When
		token, err := mockShell.GetSessionToken()

		// Then
		if err == nil {
			t.Error("Expected error, got nil")
		}
		expectedError := "GetSessionToken not implemented"
		if err.Error() != expectedError {
			t.Errorf("Expected error %s, got %s", expectedError, err.Error())
		}
		if token != "" {
			t.Errorf("Expected empty token, got %s", token)
		}
	})
}

// TestDefaultShell_CheckResetFlags tests the CheckResetFlags method of DefaultShell
func TestDefaultShell_CheckResetFlags(t *testing.T) {
	// Save original environment variable and restore it after all tests
	origEnv := os.Getenv("WINDSOR_SESSION_TOKEN")
	defer func() { os.Setenv("WINDSOR_SESSION_TOKEN", origEnv) }()

	// Save original session token and restore it after all tests
	origSessionToken := sessionToken
	defer func() { sessionToken = origSessionToken }()

	t.Run("NoSessionToken", func(t *testing.T) {
		// Given
		shell := setupShellTest(t)
		ResetSessionToken()

		// When no session token is set in the environment
		osSetenv("WINDSOR_SESSION_TOKEN", "")
		result, err := shell.CheckResetFlags()

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result {
			t.Errorf("Expected result to be false when no session token exists")
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given
		shell := setupShellTest(t)
		ResetSessionToken()

		// Save original getwd function
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()

		// Mock the getwd function to return an error
		getwd = func() (string, error) {
			return "", fmt.Errorf("error getting working directory")
		}

		// Set a test session token
		osSetenv("WINDSOR_SESSION_TOKEN", "test-token")

		// When
		result, err := shell.CheckResetFlags()

		// Then
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting project root") {
			t.Errorf("Expected error to contain 'error getting project root', got: %v", err)
		}
		if result {
			t.Errorf("Expected result to be false when error occurs")
		}
	})

	t.Run("WindsorDirectoryDoesNotExist", func(t *testing.T) {
		// Given
		shell := setupShellTest(t)
		ResetSessionToken()

		// Save original functions
		originalGetwd := getwd
		originalOsStat := osStat
		defer func() {
			getwd = originalGetwd
			osStat = originalOsStat
		}()

		// Mock the getwd function
		getwd = func() (string, error) {
			return "/test/project", nil
		}

		// Mock the osStat function to simulate .windsor directory not existing
		osStat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "windsor.yaml") || strings.Contains(name, "windsor.yml") {
				return nil, os.ErrNotExist
			}
			if strings.Contains(name, ".windsor") {
				return nil, os.ErrNotExist
			}
			return nil, os.ErrNotExist
		}

		// Set a test session token
		osSetenv("WINDSOR_SESSION_TOKEN", "test-token")

		// When
		result, err := shell.CheckResetFlags()

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result {
			t.Errorf("Expected result to be false when .windsor directory doesn't exist")
		}
	})

	t.Run("ResetFileExists", func(t *testing.T) {
		// Given
		shell := setupShellTest(t)
		ResetSessionToken()

		// Save original functions
		originalGetwd := getwd
		originalOsStat := osStat
		defer func() {
			getwd = originalGetwd
			osStat = originalOsStat
		}()

		// Mock the getwd function
		getwd = func() (string, error) {
			return "/test/project", nil
		}

		// Mock the osStat function to simulate reset file existing
		osStat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "windsor.yaml") {
				return nil, nil // windsor.yaml exists
			}
			if strings.Contains(name, ".windsor") {
				return nil, nil // .windsor directory exists
			}
			if strings.Contains(name, ".session.test-token") {
				return nil, nil // Reset file exists
			}
			return nil, os.ErrNotExist
		}

		// Set a test session token
		osSetenv("WINDSOR_SESSION_TOKEN", "test-token")

		// When
		result, err := shell.CheckResetFlags()

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !result {
			t.Errorf("Expected result to be true when reset file exists")
		}
	})

	t.Run("ResetFileDoesNotExist", func(t *testing.T) {
		// Given
		shell := setupShellTest(t)
		ResetSessionToken()

		// Save original functions
		originalGetwd := getwd
		originalOsStat := osStat
		defer func() {
			getwd = originalGetwd
			osStat = originalOsStat
		}()

		// Mock the getwd function
		getwd = func() (string, error) {
			return "/test/project", nil
		}

		// Mock the osStat function to simulate reset file not existing
		osStat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "windsor.yaml") {
				return nil, nil // windsor.yaml exists
			}
			if strings.Contains(name, ".windsor") && !strings.Contains(name, ".session.") {
				return nil, nil // .windsor directory exists
			}
			// Reset file does not exist
			return nil, os.ErrNotExist
		}

		// Set a test session token
		osSetenv("WINDSOR_SESSION_TOKEN", "test-token")

		// When
		result, err := shell.CheckResetFlags()

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result {
			t.Errorf("Expected result to be false when reset file doesn't exist")
		}
	})

	t.Run("ErrorFindingSessionFiles", func(t *testing.T) {
		// Given
		shell := setupShellTest(t)
		ResetSessionToken()

		// Save original functions
		originalGetwd := getwd
		originalOsStat := osStat
		originalFilepathGlob := filepathGlob
		defer func() {
			getwd = originalGetwd
			osStat = originalOsStat
			filepathGlob = originalFilepathGlob
		}()

		// Mock the getwd function
		getwd = func() (string, error) {
			return "/test/project", nil
		}

		// Mock osStat to simulate .windsor dir exists
		osStat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "windsor.yaml") {
				return nil, nil // windsor.yaml exists
			}
			return nil, os.ErrNotExist
		}

		// Mock filepath.Glob to return an error
		filepathGlob = func(pattern string) ([]string, error) {
			return nil, fmt.Errorf("mock error finding session files")
		}

		// Set a test session token
		osSetenv("WINDSOR_SESSION_TOKEN", "test-token")

		// When
		result, err := shell.CheckResetFlags()

		// Then
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error finding session files") {
			t.Errorf("Expected error to contain 'error finding session files', got: %v", err)
		}
		if result {
			t.Errorf("Expected result to be false when error occurs")
		}
	})

	t.Run("ErrorRemovingSessionFiles", func(t *testing.T) {
		// Given
		shell := setupShellTest(t)
		ResetSessionToken()

		// Save original functions
		originalGetwd := getwd
		originalOsStat := osStat
		originalFilepathGlob := filepathGlob
		originalOsRemoveAll := osRemoveAll
		defer func() {
			getwd = originalGetwd
			osStat = originalOsStat
			filepathGlob = originalFilepathGlob
			osRemoveAll = originalOsRemoveAll
		}()

		// Mock the getwd function
		getwd = func() (string, error) {
			return "/test/project", nil
		}

		// Mock osStat to simulate .windsor dir exists
		osStat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "windsor.yaml") || strings.Contains(name, ".windsor") {
				return nil, nil // both config file and directory exist
			}
			return nil, os.ErrNotExist
		}

		// Mock filepath.Glob to return some session files
		filepathGlob = func(pattern string) ([]string, error) {
			return []string{"/test/project/.windsor/.session.test-token"}, nil
		}

		// Mock osRemoveAll to return an error
		osRemoveAll = func(path string) error {
			return fmt.Errorf("mock error removing session file")
		}

		// Set a test session token
		osSetenv("WINDSOR_SESSION_TOKEN", "test-token")

		// When
		result, err := shell.CheckResetFlags()

		// Then
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error removing session file") {
			t.Errorf("Expected error to contain 'error removing session file', got: %v", err)
		}
		if result {
			t.Errorf("Expected result to be false when error occurs")
		}
	})
}

// TestMockShell_CheckReset tests the MockShell's CheckReset method
func TestMockShell_CheckReset(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)

		// Configure the mock to return a success response
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return true, nil
		}

		// When
		result, err := mockShell.CheckResetFlags()

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !result {
			t.Errorf("Expected result to be true, got false")
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)

		// Configure the mock to return an error
		expectedError := fmt.Errorf("custom error")
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, expectedError
		}

		// When
		result, err := mockShell.CheckResetFlags()

		// Then
		if err == nil || err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
		if result {
			t.Errorf("Expected result to be false, got true")
		}
	})

	t.Run("DefaultImplementation", func(t *testing.T) {
		// Given
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)

		// When CheckResetFunc isn't set, the default implementation should be used
		result, err := mockShell.CheckResetFlags()

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result {
			t.Errorf("Expected result to be false by default, got true")
		}
	})
}

// TestDefaultShell_Reset tests the Reset method of the DefaultShell struct
func TestDefaultShell_Reset(t *testing.T) {
	t.Run("ResetWithNoEnvVars", func(t *testing.T) {
		// Given a default shell
		shell := setupShellTest(t)

		// Make sure environment variables are not set
		os.Unsetenv("WINDSOR_MANAGED_ENV")
		os.Unsetenv("WINDSOR_MANAGED_ALIAS")

		// Set up the test
		origStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// When calling Reset
		shell.Reset()

		// Capture and restore stdout
		w.Close()
		var buf bytes.Buffer
		io.Copy(&buf, r)
		os.Stdout = origStdout

		// Then no unset commands should be issued
		output := buf.String()
		if strings.Contains(output, "unset") {
			t.Errorf("Expected no unset commands, but got: %s", output)
		}
	})

	t.Run("ResetWithEnvironmentVariables", func(t *testing.T) {
		// Given a default shell
		shell := setupShellTest(t)

		// Set environment variables
		os.Setenv("WINDSOR_MANAGED_ENV", "ENV1,ENV2, ENV3")
		os.Setenv("WINDSOR_MANAGED_ALIAS", "alias1,alias2, alias3")
		defer func() {
			os.Unsetenv("WINDSOR_MANAGED_ENV")
			os.Unsetenv("WINDSOR_MANAGED_ALIAS")
		}()

		// Set up the test
		origStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// When calling Reset
		shell.Reset()

		// Capture and restore stdout
		w.Close()
		var buf bytes.Buffer
		io.Copy(&buf, r)
		os.Stdout = origStdout

		// Then unset commands should be issued
		output := buf.String()
		// Check for unset ENV1 ENV2 ENV3 (on Unix) or Remove-Item ENV:ENV1, etc on Windows
		if runtime.GOOS == "windows" {
			if !strings.Contains(output, "Remove-Item Env:ENV1") {
				t.Errorf("Expected Remove-Item Env:ENV1 command, but got: %s", output)
			}
			if !strings.Contains(output, "Remove-Item Env:ENV2") {
				t.Errorf("Expected Remove-Item Env:ENV2 command, but got: %s", output)
			}
			if !strings.Contains(output, "Remove-Item Env:ENV3") {
				t.Errorf("Expected Remove-Item Env:ENV3 command, but got: %s", output)
			}
			// And unalias for aliases
			if !strings.Contains(output, "Remove-Item Alias:alias1") {
				t.Errorf("Expected Remove-Item Alias:alias1 command, but got: %s", output)
			}
		} else {
			// For Unix
			if !strings.Contains(output, "unset ENV1 ENV2 ENV3") {
				t.Errorf("Expected unset ENV1 ENV2 ENV3 command, but got: %s", output)
			}
			// And unalias for aliases
			if !strings.Contains(output, "unalias alias1") {
				t.Errorf("Expected unalias alias1 command, but got: %s", output)
			}
			if !strings.Contains(output, "unalias alias2") {
				t.Errorf("Expected unalias alias2 command, but got: %s", output)
			}
			if !strings.Contains(output, "unalias alias3") {
				t.Errorf("Expected unalias alias3 command, but got: %s", output)
			}
		}
	})
}
