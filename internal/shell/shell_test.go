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

	"github.com/windsorcli/cli/internal/config"
	"github.com/windsorcli/cli/internal/di"
)

func TestShell_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		injector := di.NewInjector()

		// Register a mock ConfigHandler to avoid resolution errors
		mockConfigHandler := config.NewMockConfigHandler()
		injector.Register("configHandler", mockConfigHandler)

		// Given a DefaultShell instance
		shell := NewDefaultShell(injector)

		// When calling Initialize
		err := shell.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Initialize() error = %v, wantErr %v", err, false)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		injector := di.NewInjector()
		shell := NewDefaultShell(injector)
		err := shell.Initialize()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		} else {
			t.Logf("Received expected error: %v", err)
		}
	})
}

func TestShell_GetProjectRoot(t *testing.T) {
	t.Run("GitRepo", func(t *testing.T) {
		injector := di.NewInjector()

		// Given a temporary directory with a git repository
		rootDir := createTempDir(t, "project-root")
		subDir := filepath.Join(rootDir, "subdir")
		if err := os.Mkdir(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdir: %v", err)
		}

		initGitRepo(t, rootDir)
		changeDir(t, subDir)

		// When calling GetProjectRoot
		shell := NewDefaultShell(injector)
		projectRoot, err := shell.GetProjectRoot()

		// Then the project root should be returned without error
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expectedRootDir := resolveSymlinks(t, rootDir)

		// Normalize paths for Windows compatibility
		expectedRootDir = normalizePath(expectedRootDir)
		projectRoot = normalizePath(projectRoot)
		if expectedRootDir != projectRoot {
			t.Errorf("Expected project root %q, got %q", expectedRootDir, projectRoot)
		}
	})

	t.Run("Cached", func(t *testing.T) {
		injector := di.NewInjector()

		// Given a temporary directory with a git repository and cached project root
		rootDir := createTempDir(t, "project-root")
		subDir := filepath.Join(rootDir, "subdir")
		if err := os.Mkdir(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdir: %v", err)
		}

		initGitRepo(t, rootDir)
		changeDir(t, subDir)

		// When calling GetProjectRoot
		shell := NewDefaultShell(injector)
		projectRoot, err := shell.GetProjectRoot()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		expectedRootDir := resolveSymlinks(t, rootDir)

		// Normalize paths for Windows compatibility
		expectedRootDir = normalizePath(expectedRootDir)
		projectRoot = normalizePath(projectRoot)

		if expectedRootDir != projectRoot {
			t.Errorf("Expected project root %q, got %q", expectedRootDir, projectRoot)
		}

		// When calling GetProjectRoot again with cached project root
		shell.projectRoot = expectedRootDir
		cachedProjectRoot, err := shell.GetProjectRoot()

		// Then the cached project root should be returned without error
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Normalize cached project root for Windows compatibility
		cachedProjectRoot = normalizePath(cachedProjectRoot)

		if expectedRootDir != cachedProjectRoot {
			t.Errorf("Expected cached project root %q, got %q", expectedRootDir, cachedProjectRoot)
		}
	})

	t.Run("MaxDepth", func(t *testing.T) {
		injector := di.NewInjector()

		// Given a directory structure exceeding max depth
		rootDir := createTempDir(t, "project-root")
		currentDir := rootDir
		for i := 0; i <= maxFolderSearchDepth; i++ {
			subDir := filepath.Join(currentDir, "subdir")
			if err := os.Mkdir(subDir, 0755); err != nil {
				t.Fatalf("Failed to create subdir %d: %v", i, err)
			}
			currentDir = subDir
		}

		changeDir(t, currentDir)

		// When calling GetProjectRoot
		shell := NewDefaultShell(injector)
		projectRoot, err := shell.GetProjectRoot()

		// Then the project root should be empty
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if projectRoot != "" {
			t.Errorf("Expected project root to be empty, got %q", projectRoot)
		}
	})

	t.Run("NoGitNoYaml", func(t *testing.T) {
		injector := di.NewInjector()

		// Given a directory without git repository or yaml file
		rootDir := createTempDir(t, "project-root")
		subDir := filepath.Join(rootDir, "subdir")
		if err := os.Mkdir(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdir: %v", err)
		}

		changeDir(t, subDir)

		// When calling GetProjectRoot
		shell := NewDefaultShell(injector)
		projectRoot, err := shell.GetProjectRoot()

		// Then the project root should be empty
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if projectRoot != "" {
			t.Errorf("Expected project root to be empty, got %q", projectRoot)
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

		originalExecCommand := execCommand
		execCommand = mockCommand
		defer func() { execCommand = originalExecCommand }()

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
		expectedOutput := "command output"
		command := "echo"
		args := []string{"hello"}

		// Mock cmdRun to simulate command execution
		originalCmdRun := cmdRun
		cmdRun = func(cmd *exec.Cmd) error {
			_, _ = cmd.Stdout.Write([]byte("command output"))
			return nil
		}
		defer func() { cmdRun = originalCmdRun }()

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

	t.Run("SuccessWithSudo", func(t *testing.T) {
		expectedOutput := "hello\n"
		command := "sudo"
		args := []string{"echo", "hello"}

		// Mock cmdRun to simulate command execution
		originalCmdRun := cmdRun
		cmdRun = func(cmd *exec.Cmd) error {
			_, _ = cmd.Stdout.Write([]byte("hello\n"))
			return nil
		}
		defer func() { cmdRun = originalCmdRun }()

		shell := NewDefaultShell(nil)

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

		// Mock cmdRun to simulate command execution failure
		originalCmdRun := cmdRun
		cmdRun = func(cmd *exec.Cmd) error {
			return fmt.Errorf("command not found")
		}
		defer func() { cmdRun = originalCmdRun }()

		shell := NewDefaultShell(nil)

		_, err := shell.Exec(command, args...)
		if err == nil {
			t.Fatalf("Expected error when executing nonexistent command, got nil")
		}
		expectedError := "command not found"
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
func mockCommand(name string, arg ...string) *exec.Cmd {
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
