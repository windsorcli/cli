package shell

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"text/template"

	"github.com/windsorcli/cli/pkg/di"
)

type MockObjects struct {
	Injector *di.BaseInjector
	Shell    *MockShell
}

func setupSafeShellTestMocks(injector ...*di.BaseInjector) *MockObjects {
	var inj *di.BaseInjector
	if len(injector) == 0 {
		inj = di.NewInjector()
	} else {
		inj = injector[0]
	}

	mocks := &MockObjects{
		Injector: inj,
		Shell:    NewMockShell(inj),
	}

	// Mock execCommand to simulate command execution
	execCommand = func(command string, args ...string) *exec.Cmd {
		cmd := exec.Command("echo", append([]string{command}, args...)...)
		return cmd
	}

	// Register the mock shell in the injector
	inj.Register("shell", mocks.Shell)

	cachedContainerID = ""

	return mocks
}

func TestShell_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSafeShellTestMocks to set up the mock environment
		mocks := setupSafeShellTestMocks()

		// Given a DefaultShell instance
		shell := NewDefaultShell(mocks.Injector)

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
		command := "echo"
		args := []string{"hello"}

		// Track if execCommand, cmdStart, and cmdWait were called and their arguments
		execCommandCalled := false
		execCommandArgs := []string{}
		cmdStartCalled := false
		cmdWaitCalled := false

		// Mock execCommand to track its invocation and arguments
		originalExecCommand := execCommand
		execCommand = func(name string, arg ...string) *exec.Cmd {
			execCommandCalled = true
			execCommandArgs = append([]string{name}, arg...)
			return &exec.Cmd{}
		}
		defer func() { execCommand = originalExecCommand }()

		// Mock cmdStart to track its invocation
		originalCmdStart := cmdStart
		cmdStart = func(cmd *exec.Cmd) error {
			cmdStartCalled = true
			return nil
		}
		defer func() { cmdStart = originalCmdStart }()

		// Mock cmdWait to track its invocation
		originalCmdWait := cmdWait
		cmdWait = func(cmd *exec.Cmd) error {
			cmdWaitCalled = true
			return nil
		}
		defer func() { cmdWait = originalCmdWait }()

		injector := di.NewInjector()
		shell := NewDefaultShell(injector)

		_, _, err := shell.Exec(command, args...)
		if err != nil {
			t.Fatalf("Failed to execute command: %v", err)
		}
		if !execCommandCalled {
			t.Fatalf("Expected execCommand to be called")
		}
		if !cmdStartCalled {
			t.Fatalf("Expected cmdStart to be called")
		}
		if !cmdWaitCalled {
			t.Fatalf("Expected cmdWait to be called")
		}
		if len(execCommandArgs) != 2 || execCommandArgs[0] != "echo" || execCommandArgs[1] != "hello" {
			t.Fatalf("Expected execCommand to be called with %q, got %q", []string{"echo", "hello"}, execCommandArgs)
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

		_, _, err := shell.Exec(command, args...)
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
		execCommand = func(name string, arg ...string) *exec.Cmd {
			return exec.Command("false")
		}
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
		_, _, err := shell.Exec(command, args...)
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
	// Mock cmdRun, cmdStart, cmdWait, osOpenFile, and ProcessState to simulate command execution
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
		cmd.ProcessState = &os.ProcessState{}
		return nil
	}
	cmdStart = func(cmd *exec.Cmd) error {
		cmd.ProcessState = &os.ProcessState{}
		return nil
	}
	cmdWait = func(cmd *exec.Cmd) error {
		cmd.ProcessState = &os.ProcessState{}
		return nil
	}
	osOpenFile = func(_ string, _ int, _ os.FileMode) (*os.File, error) {
		return &os.File{}, nil
	}

	t.Run("Success", func(t *testing.T) {
		command := "echo"
		args := []string{"hello"}

		var capturedCommand string
		var capturedArgs []string

		// Mock execCommand to capture the command and arguments
		originalExecCommand := execCommand
		execCommand = func(cmd string, args ...string) *exec.Cmd {
			capturedCommand = cmd
			capturedArgs = args
			return originalExecCommand(cmd, args...)
		}
		defer func() { execCommand = originalExecCommand }()

		shell := NewDefaultShell(nil)
		_, _, err := shell.ExecSudo("Test Sudo Command", command, args...)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expectedCommand := "sudo"
		expectedArgs := []string{"echo", "hello"}

		if capturedCommand != expectedCommand {
			t.Fatalf("Expected command %q, got %q", expectedCommand, capturedCommand)
		}
		if !reflect.DeepEqual(capturedArgs, expectedArgs) {
			t.Fatalf("Expected args %v, got %v", expectedArgs, capturedArgs)
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
		_, _, err := shell.ExecSudo("Test Sudo Command", "echo", "hello")
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
		_, _, err := shell.ExecSudo("Test Sudo Command", command, args...)
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
		_, _, err := shell.ExecSudo("Test Sudo Command", command, args...)
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

		// Mock execCommand to confirm it was called without executing
		execCommandCalled := false
		originalExecCommand := execCommand
		execCommand = func(name string, arg ...string) *exec.Cmd {
			execCommandCalled = true
			return &exec.Cmd{}
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

		// Execute the command and verify the output
		output, _, err := shell.ExecSudo("Test Sudo Command", command, args...)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedOutput := "hello\n"
		if output != expectedOutput {
			t.Fatalf("Expected output %q, got %q", expectedOutput, output)
		}

		// Verify that execCommand was called
		if !execCommandCalled {
			t.Fatalf("Expected execCommand to be called, but it was not")
		}
	})
}

func TestShell_ExecSilent(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		command := "go"
		args := []string{"version"}

		// Mock execCommand to validate it was called with the correct parameters
		execCommandCalled := false
		originalExecCommand := execCommand
		execCommand = func(name string, arg ...string) *exec.Cmd {
			execCommandCalled = true
			if name != command {
				t.Fatalf("Expected command %q, got %q", command, name)
			}
			if len(arg) != len(args) || arg[0] != args[0] {
				t.Fatalf("Expected args %v, got %v", args, arg)
			}
			return &exec.Cmd{}
		}
		defer func() { execCommand = originalExecCommand }()

		// Mock cmdRun to simulate successful command execution
		originalCmdRun := cmdRun
		cmdRun = func(cmd *exec.Cmd) error {
			return nil
		}
		defer func() { cmdRun = originalCmdRun }()

		shell := NewDefaultShell(nil)
		_, _, err := shell.ExecSilent(command, args...)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Verify that execCommand was called
		if !execCommandCalled {
			t.Fatalf("Expected execCommand to be called, but it was not")
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
		_, _, err := shell.ExecSilent(command, args...)
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
		execCommandCalled := false
		originalExecCommand := execCommand
		execCommand = func(name string, arg ...string) *exec.Cmd {
			execCommandCalled = true
			return &exec.Cmd{}
		}
		defer func() { execCommand = originalExecCommand }()

		// Mock cmdStart to simulate successful command start
		originalCmdStart := cmdStart
		cmdStart = func(cmd *exec.Cmd) error {
			return nil
		}
		defer func() { cmdStart = originalCmdStart }()

		// Mock cmdWait to simulate successful command completion
		originalCmdWait := cmdWait
		cmdWait = func(cmd *exec.Cmd) error {
			return nil
		}
		defer func() { cmdWait = originalCmdWait }()

		shell := NewDefaultShell(nil)
		shell.SetVerbosity(true)

		_, _, err := shell.ExecSilent(command, args...)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Verify that execCommand was called
		if !execCommandCalled {
			t.Fatalf("Expected execCommand to be called, but it was not")
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

	// t.Run("Success", func(t *testing.T) {
	// 	injector := di.NewMockInjector()
	// 	mocks := setSafeDockerShellMocks(injector)
	// 	shell := NewDefaultShell(mocks.Injector)

	// 	command := "go"
	// 	args := []string{"version"}

	// 	output, code, err := shell.ExecProgress("Test Progress Command", command, args...)
	// 	if err != nil {
	// 		t.Fatalf("Expected no error, got %v", err)
	// 	}
	// 	expectedOutput := "go version go1.16.3\n"
	// 	if output != expectedOutput {
	// 		t.Fatalf("Expected output %q, got %q", expectedOutput, output)
	// 	}
	// 	if code != 0 {
	// 		t.Fatalf("Expected exit code 0, got %d", code)
	// 	}
	// })

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
		_, _, err := shell.ExecProgress("Test Progress Command", command, args...)
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
		_, _, err := shell.ExecProgress("Test Progress Command", command, args...)
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
		_, _, err := shell.ExecProgress("Test Progress Command", command, args...)
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
		_, _, err := shell.ExecProgress("Test Progress Command", command, args...)
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
		_, _, err := shell.ExecProgress("Test Progress Command", command, args...)
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
		_, _, err := shell.ExecProgress("Test Progress Command", command, args...)
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
		execCommandCalled := false
		originalExecCommand := execCommand
		execCommand = func(name string, arg ...string) *exec.Cmd {
			execCommandCalled = true
			return &exec.Cmd{}
		}
		defer func() { execCommand = originalExecCommand }() // Restore original function after test

		// Mock cmdStart to simulate successful command start
		originalCmdStart := cmdStart
		cmdStart = func(cmd *exec.Cmd) error {
			_, _ = cmd.Stdout.Write([]byte("go version go1.16.3\n"))
			return nil
		}
		defer func() { cmdStart = originalCmdStart }() // Restore original function after test

		// Mock cmdWait to simulate successful command completion
		originalCmdWait := cmdWait
		cmdWait = func(cmd *exec.Cmd) error {
			return nil
		}
		defer func() { cmdWait = originalCmdWait }() // Restore original function after test

		output, _, err := shell.ExecProgress("Test Progress Command", command, args...)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedOutputPrefix := "go version"
		if !strings.HasPrefix(output, expectedOutputPrefix) {
			t.Fatalf("Expected output to start with %q, got %q", expectedOutputPrefix, output)
		}

		// Verify that execCommand was called
		if !execCommandCalled {
			t.Fatalf("Expected execCommand to be called, but it was not")
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
	shell := &DefaultShell{}

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
