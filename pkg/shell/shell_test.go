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

// Helper function to resolve symlinks
func resolveSymlinks(t *testing.T, path string) string {
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("Failed to evaluate symlinks for %s: %v", path, err)
	}
	return resolvedPath
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

// Helper function to initialize a git repository
func initGitRepo(t *testing.T, dir string) {
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to initialize git repository: %v", err)
	}
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

// Mock execCommand to simulate git command failure
func mockCommand(_ string, _ ...string) *exec.Cmd {
	return exec.Command("false")
}

// Updated helper function to mock exec.Command for successful execution using PowerShell
func mockExecCommandSuccess(command string, args ...string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		// Use PowerShell to execute the echo command
		fullCommand := fmt.Sprintf("Write-Output 'mock output for: %s %s'", command, strings.Join(args, " "))
		cmdArgs := []string{"-Command", fullCommand}
		return exec.Command("powershell.exe", cmdArgs...)
	} else {
		// Use 'echo' on Unix-like systems
		fullArgs := append([]string{"mock output for:", command}, args...)
		return exec.Command("echo", fullArgs...)
	}
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

func TestDefaultShell_AddCurrentDirToTrustedFile(t *testing.T) {
	// Mock the os functions at the top
	originalGetwd := getwd
	originalOsUserHomeDir := osUserHomeDir
	originalReadFile := osReadFile
	originalMkdirAll := osMkdirAll
	originalWriteFile := osWriteFile

	defer func() {
		getwd = originalGetwd
		osUserHomeDir = originalOsUserHomeDir
		osReadFile = originalReadFile
		osMkdirAll = originalMkdirAll
		osWriteFile = originalWriteFile
	}()

	// Default mock implementations for success scenarios
	getwd = func() (string, error) {
		return "/mock/current/dir", nil
	}

	osUserHomeDir = func() (string, error) {
		return "/mock/home/dir", nil
	}

	osReadFile = func(filename string) ([]byte, error) {
		return []byte{}, nil
	}

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}

	var capturedData []byte
	osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
		capturedData = data
		return nil
	}

	t.Run("Success", func(t *testing.T) {
		// Create a default shell for testing
		shell := &DefaultShell{}

		// Call AddCurrentDirToTrustedFile and check for errors
		err := shell.AddCurrentDirToTrustedFile()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that the current directory was added to the trusted file
		expectedData := "/mock/current/dir\n"
		if string(capturedData) != expectedData {
			t.Errorf("capturedData = %v, want %v", string(capturedData), expectedData)
		}
	})

	t.Run("SuccessAlreadyTrusted", func(t *testing.T) {
		// Save the original osReadFile function
		originalReadFile := osReadFile
		defer func() { osReadFile = originalReadFile }()

		// Override the osReadFile function locally to simulate a trusted directory already present
		osReadFile = func(filename string) ([]byte, error) {
			return []byte("/mock/current/dir\n"), nil
		}

		// Create a default shell for testing
		shell := &DefaultShell{}

		// Call AddCurrentDirToTrustedFile and check for errors
		err := shell.AddCurrentDirToTrustedFile()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Save the original getwd function
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()

		// Override the getwd function locally to simulate an error
		getwd = func() (string, error) {
			return "", fmt.Errorf("error getting project root directory")
		}

		// Create a default shell for testing
		shell := &DefaultShell{}

		// Call AddCurrentDirToTrustedFile and expect an error
		err := shell.AddCurrentDirToTrustedFile()
		if err == nil {
			t.Errorf("expected error, got nil")
		}
		expectedError := "Error getting project root directory: error getting project root directory"
		if err.Error() != expectedError {
			t.Errorf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorGettingUserHomeDir", func(t *testing.T) {
		// Save the original osUserHomeDir function
		originalOsUserHomeDir := osUserHomeDir
		defer func() { osUserHomeDir = originalOsUserHomeDir }()

		// Override the osUserHomeDir function locally to simulate an error
		osUserHomeDir = func() (string, error) {
			return "", fmt.Errorf("error getting user home directory")
		}

		// Create a default shell for testing
		shell := &DefaultShell{}

		// Call AddCurrentDirToTrustedFile and expect an error
		err := shell.AddCurrentDirToTrustedFile()
		if err == nil {
			t.Errorf("expected error, got nil")
		}
		expectedError := "Error getting user home directory: error getting user home directory"
		if err.Error() != expectedError {
			t.Errorf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCreatingDirectories", func(t *testing.T) {
		// Save the original osMkdirAll function
		originalMkdirAll := osMkdirAll
		defer func() { osMkdirAll = originalMkdirAll }()

		// Override the osMkdirAll function locally to simulate an error
		osMkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("error creating directories")
		}

		// Create a default shell for testing
		shell := &DefaultShell{}

		// Call AddCurrentDirToTrustedFile and expect an error
		err := shell.AddCurrentDirToTrustedFile()
		if err == nil {
			t.Errorf("expected error, got nil")
		}
		expectedError := "Error creating directories for trusted file: error creating directories"
		if err.Error() != expectedError {
			t.Errorf("expected error %q, got %q", expectedError, err.Error())
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

		// Create a default shell for testing
		shell := &DefaultShell{}

		// Call AddCurrentDirToTrustedFile and expect an error
		err := shell.AddCurrentDirToTrustedFile()
		if err == nil {
			t.Errorf("expected error, got nil")
		}
		expectedError := "Error reading trusted file: error reading trusted file"
		if err.Error() != expectedError {
			t.Errorf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorWritingToTrustedFile", func(t *testing.T) {
		// Save the original osWriteFile function
		originalWriteFile := osWriteFile
		defer func() { osWriteFile = originalWriteFile }()

		// Override the osWriteFile function locally to simulate an error
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("error writing to trusted file")
		}

		// Create a default shell for testing
		shell := &DefaultShell{}

		// Call AddCurrentDirToTrustedFile and expect an error
		err := shell.AddCurrentDirToTrustedFile()
		if err == nil {
			t.Errorf("expected error, got nil")
		}
		expectedError := "Error writing to trusted file: error writing to trusted file"
		if err.Error() != expectedError {
			t.Errorf("expected error %q, got %q", expectedError, err.Error())
		}
	})
}

func TestDefaultShell_WriteResetToken(t *testing.T) {
	// Save original OS functions and environment variables
	originalOsMkdirAll := osMkdirAll
	originalOsWriteFile := osWriteFile
	originalSessionToken := os.Getenv("WINDSOR_SESSION_TOKEN")

	// Restore original functions and environment after all tests
	defer func() {
		osMkdirAll = originalOsMkdirAll
		osWriteFile = originalOsWriteFile
		os.Setenv("WINDSOR_SESSION_TOKEN", originalSessionToken)
	}()

	t.Run("NoSessionToken", func(t *testing.T) {
		// Use the real DefaultShell implementation for this test
		shell := NewDefaultShell(nil)

		// Ensure the environment variable is not set
		os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// When calling WriteResetToken
		path, err := shell.WriteResetToken()

		// Then no error should be returned and path should be empty
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if path != "" {
			t.Errorf("expected empty path, got %s", path)
		}
	})

	t.Run("SuccessfulTokenWrite", func(t *testing.T) {
		// Set up test data
		testProjectRoot := "/test/project/root"
		testToken := "test-token-123"
		expectedDirPath := filepath.Join(testProjectRoot, ".windsor")
		expectedFilePath := filepath.Join(expectedDirPath, SessionTokenPrefix+testToken)

		// Create a DefaultShell for testing
		shell := NewDefaultShell(nil)

		// Mock GetProjectRoot to return our test path
		// We don't need to keep a reference to the original since we're not restoring it
		// This is just to show our intention in the test

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

		// Set the environment variable
		os.Setenv("WINDSOR_SESSION_TOKEN", testToken)

		// When calling WriteResetToken - use our helper to call the real function with mocked deps
		var path string
		var err error

		// Create a function that will run with our mocked GetProjectRoot
		testFunc := func() {
			// Override getwd to return our test project root
			originalGetwd := getwd
			defer func() { getwd = originalGetwd }()
			getwd = func() (string, error) {
				return testProjectRoot, nil
			}

			// Call the real function
			path, err = shell.WriteResetToken()
		}
		testFunc()

		// Then no error should be returned and path should match expected value
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if path != expectedFilePath {
			t.Errorf("expected path %s, got %s", expectedFilePath, path)
		}

		// Verify that MkdirAll was called with correct parameters
		if !mkdirAllCalled {
			t.Error("expected MkdirAll to be called, but it wasn't")
		} else {
			if mkdirAllPath != expectedDirPath {
				t.Errorf("expected MkdirAll path %s, got %s", expectedDirPath, mkdirAllPath)
			}
			if mkdirAllPerm != 0750 {
				t.Errorf("expected MkdirAll permissions 0750, got %v", mkdirAllPerm)
			}
		}

		// Verify that WriteFile was called with correct parameters
		if !writeFileCalled {
			t.Error("expected WriteFile to be called, but it wasn't")
		} else {
			if writeFilePath != expectedFilePath {
				t.Errorf("expected WriteFile path %s, got %s", expectedFilePath, writeFilePath)
			}
			if len(writeFileData) != 0 {
				t.Errorf("expected empty file, got %v bytes", len(writeFileData))
			}
			if writeFilePerm != 0600 {
				t.Errorf("expected WriteFile permissions 0600, got %v", writeFilePerm)
			}
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Create a DefaultShell for testing
		shell := NewDefaultShell(nil)

		// Override getwd to simulate an error
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return "", fmt.Errorf("error getting project root")
		}

		// Set the environment variable
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")

		// When calling WriteResetToken
		path, err := shell.WriteResetToken()

		// Then the expected error should be returned and path should be empty
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting project root") {
			t.Errorf("expected error containing %q, got %q", "error getting project root", err.Error())
		}
		if path != "" {
			t.Errorf("expected empty path, got %s", path)
		}
	})

	t.Run("ErrorCreatingDirectory", func(t *testing.T) {
		// Create a DefaultShell for testing
		shell := NewDefaultShell(nil)

		// Override getwd to return a test path
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return "/test/project/root", nil
		}

		// Override MkdirAll to simulate an error
		expectedError := fmt.Errorf("error creating directory")
		osMkdirAll = func(path string, perm os.FileMode) error {
			return expectedError
		}

		// Set the environment variable
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")

		// When calling WriteResetToken
		path, err := shell.WriteResetToken()

		// Then the expected error should be returned and path should be empty
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), expectedError.Error()) {
			t.Errorf("expected error containing %q, got %q", expectedError.Error(), err.Error())
		}
		if path != "" {
			t.Errorf("expected empty path, got %s", path)
		}
	})

	t.Run("ErrorWritingFile", func(t *testing.T) {
		// Create a DefaultShell for testing
		shell := NewDefaultShell(nil)

		// Override getwd to return a test path
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return "/test/project/root", nil
		}

		// Override MkdirAll to succeed but WriteFile to fail
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

		// Then the expected error should be returned and path should be empty
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), expectedError.Error()) {
			t.Errorf("expected error containing %q, got %q", expectedError.Error(), err.Error())
		}
		if path != "" {
			t.Errorf("expected empty path, got %s", path)
		}
	})
}
