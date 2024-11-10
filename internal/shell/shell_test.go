package shell

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/windsor-hotel/cli/internal/di"
)

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
		cmdArgs := []string{"-Command", "exit 1"}
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
		assertNoError(t, err)
		expectedRootDir := resolveSymlinks(t, rootDir)
		assertEqual(t, expectedRootDir, projectRoot, "project root")
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
		assertNoError(t, err)

		expectedRootDir := resolveSymlinks(t, rootDir)
		assertEqual(t, expectedRootDir, projectRoot, "project root")

		// When calling GetProjectRoot again with cached project root
		shell.projectRoot = expectedRootDir
		cachedProjectRoot, err := shell.GetProjectRoot()

		// Then the cached project root should be returned without error
		assertNoError(t, err)
		assertEqual(t, expectedRootDir, cachedProjectRoot, "cached project root")
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
		assertNoError(t, err)
		assertEqual(t, "", projectRoot, "project root")
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
		assertNoError(t, err)
		assertEqual(t, "", projectRoot, "project root")
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
	t.Run("CommandSuccess", func(t *testing.T) {
		injector := di.NewInjector()

		// Override execCommand to simulate successful command execution
		originalExecCommand := execCommand
		execCommand = mockExecCommandSuccess
		defer func() {
			execCommand = originalExecCommand
		}()

		// When executing a command that succeeds
		shell := NewDefaultShell(injector)
		result, err := shell.Exec(false, "Executing echo command", "echo", "hello")
		// Then no error should be returned
		assertNoError(t, err)
		// And the result should be as expected
		expectedOutput := "mock output for: echo hello\n"
		// Normalize the result to handle different line endings
		normalizedResult := strings.ReplaceAll(result, "\r\n", "\n")
		if normalizedResult != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, result)
		}
	})

	t.Run("CommandWithError", func(t *testing.T) {
		injector := di.NewInjector()

		// Override execCommand to simulate command failure
		originalExecCommand := execCommand
		execCommand = mockExecCommandError
		defer func() {
			execCommand = originalExecCommand
		}()

		// When executing a command that fails
		shell := NewDefaultShell(injector)
		output := captureStdout(t, func() {
			// When executing a command that fails
			_, err := shell.Exec(false, "Executing failing command", "somecommand", "arg1")
			// Then an error should be returned
			if err == nil {
				t.Fatal("Expected an error, but got nil")
			}
		})

		// Output should be empty
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
	})

	t.Run("VerboseCommandSuccess", func(t *testing.T) {
		injector := di.NewInjector()

		// Override execCommand to simulate successful command execution
		originalExecCommand := execCommand
		execCommand = mockExecCommandSuccess
		defer func() {
			execCommand = originalExecCommand
		}()

		// Capture stdout
		output := captureStdout(t, func() {
			// When executing a command that succeeds with verbose output
			shell := NewDefaultShell(injector)
			result, err := shell.Exec(true, "Executing echo command", "echo", "hello")
			// Then no error should be returned
			assertNoError(t, err)
			// And the result should be as expected
			expectedOutput := "mock output for: echo hello\n"
			// Normalize the result to handle different line endings
			normalizedResult := strings.ReplaceAll(result, "\r\n", "\n")
			if normalizedResult != expectedOutput {
				t.Errorf("Expected output %q, got %q", expectedOutput, result)
			}
		})

		// Output should match the expected verbose output
		expectedVerboseOutput := "mock output for: echo hello\n"
		normalizedOutput := strings.ReplaceAll(output, "\r\n", "\n")
		if normalizedOutput != expectedVerboseOutput {
			t.Errorf("Expected verbose output %q, got %q", expectedVerboseOutput, output)
		}
	})

	t.Run("FailedToStartCommand", func(t *testing.T) {
		injector := di.NewInjector()

		// Override cmdStart to simulate command start failure
		originalCmdStart := cmdStart
		cmdStart = func(cmd *exec.Cmd) error {
			return errors.New("mock start failure")
		}
		defer func() {
			cmdStart = originalCmdStart
		}()

		// Capture stdout
		output := captureStdout(t, func() {
			// When executing a command that fails to start
			shell := NewDefaultShell(injector)
			_, err := shell.Exec(false, "Executing failing start command", "somecommand", "arg1")
			// Then an error should be returned
			if err == nil {
				t.Fatal("Expected an error, but got nil")
			}
		})

		// Output should be empty
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
	})
}

func TestMain(m *testing.M) {
	code := m.Run()
	for _, dir := range tempDirs {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Printf("Failed to remove temp dir %s: %v\n", dir, err)
		}
	}
	os.Exit(code)
}

// Helper function to assert no error
func assertNoError(t *testing.T, err error) {
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

// Helper function to assert equality
func assertEqual(t *testing.T, expected, actual, name string) {
	expected = normalizePath(expected)
	actual = normalizePath(actual)
	if expected != actual {
		t.Errorf("Expected %s to be %s, got %s", name, expected, actual)
	}
}

// Helper function to resolve symlinks
func resolveSymlinks(t *testing.T, path string) string {
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("Failed to evaluate symlinks for %s: %v", path, err)
	}
	return resolvedPath
}
