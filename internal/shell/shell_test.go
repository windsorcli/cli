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
	"testing"
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

func TestShell_GetProjectRoot(t *testing.T) {
	t.Run("GitRepo", func(t *testing.T) {
		// Given a temporary directory with a git repository
		rootDir := createTempDir(t, "project-root")
		subDir := filepath.Join(rootDir, "subdir")
		if err := os.Mkdir(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdir: %v", err)
		}

		initGitRepo(t, rootDir)
		changeDir(t, subDir)

		// When calling GetProjectRoot
		shell := NewDefaultShell()
		projectRoot, err := shell.GetProjectRoot()

		// Then the project root should be returned without error
		assertNoError(t, err)
		expectedRootDir := resolveSymlinks(t, rootDir)
		assertEqual(t, expectedRootDir, projectRoot, "project root")
	})

	t.Run("Cached", func(t *testing.T) {
		// Given a temporary directory with a git repository and cached project root
		rootDir := createTempDir(t, "project-root")
		subDir := filepath.Join(rootDir, "subdir")
		if err := os.Mkdir(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdir: %v", err)
		}

		initGitRepo(t, rootDir)
		changeDir(t, subDir)

		// When calling GetProjectRoot
		shell := NewDefaultShell()
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
		// Given a directory structure exceeding max depth
		rootDir := createTempDir(t, "project-root")
		currentDir := rootDir
		for i := 0; i <= maxDepth; i++ {
			subDir := filepath.Join(currentDir, "subdir")
			if err := os.Mkdir(subDir, 0755); err != nil {
				t.Fatalf("Failed to create subdir %d: %v", i, err)
			}
			currentDir = subDir
		}

		changeDir(t, currentDir)

		// When calling GetProjectRoot
		shell := NewDefaultShell()
		projectRoot, err := shell.GetProjectRoot()

		// Then the project root should be empty
		assertNoError(t, err)
		assertEqual(t, "", projectRoot, "project root")
	})

	t.Run("NoGitNoYaml", func(t *testing.T) {
		// Given a directory without git repository or yaml file
		rootDir := createTempDir(t, "project-root")
		subDir := filepath.Join(rootDir, "subdir")
		if err := os.Mkdir(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdir: %v", err)
		}

		changeDir(t, subDir)

		// When calling GetProjectRoot
		shell := NewDefaultShell()
		projectRoot, err := shell.GetProjectRoot()

		// Then the project root should be empty
		assertNoError(t, err)
		assertEqual(t, "", projectRoot, "project root")
	})

	t.Run("GetwdFails", func(t *testing.T) {
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
		shell := NewDefaultShell()
		_, err := shell.GetProjectRoot()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})
}

func TestShell_Exec(t *testing.T) {
	shell := NewDefaultShell()

	t.Run("SuccessfulCommand", func(t *testing.T) {
		// Override execCommand to simulate successful execution using PowerShell
		originalExecCommand := execCommand
		execCommand = mockExecCommandSuccess
		defer func() { execCommand = originalExecCommand }()

		// When executing a command
		output, err := shell.Exec("somecommand", "arg1")

		// Then there should be no error and output should match
		assertNoError(t, err)
		expectedOutput := "mock output for: somecommand arg1"

		// Account for line endings
		if runtime.GOOS == "windows" {
			expectedOutput += "\r\n" // PowerShell adds \r\n
		} else {
			expectedOutput += "\n" // echo adds \n
		}

		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("CommandWithError", func(t *testing.T) {
		// Override execCommand to simulate command failure using PowerShell
		originalExecCommand := execCommand
		execCommand = mockExecCommandError
		defer func() { execCommand = originalExecCommand }()

		// When executing a command that fails
		output, err := shell.Exec("somecommand", "arg1")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected an error, but got nil")
		}

		// Output should be empty
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
	})

	t.Run("CommandWithStdErr", func(t *testing.T) {
		// Override execCommand to simulate a command that writes to stderr
		originalExecCommand := execCommand
		execCommand = func(command string, args ...string) *exec.Cmd {
			cmd := exec.Command(command, args...)
			// Simulate a command that writes to stderr
			cmd.Stderr = bytes.NewBufferString("mock stderr output")
			return cmd
		}
		defer func() { execCommand = originalExecCommand }()

		// When executing a command that produces stderr output
		output, err := shell.Exec("somecommand", "arg1")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected an error, but got nil")
		}

		// And output should be empty
		if output != "" {
			t.Errorf("Expected empty output, got %q", output)
		}
	})

	t.Run("CommandWithArguments", func(t *testing.T) {
		// Override execCommand to simulate command execution with arguments
		originalExecCommand := execCommand
		execCommand = func(command string, args ...string) *exec.Cmd {
			// Verify that the command and arguments are as expected
			if command != "somecommand" || len(args) != 2 || args[0] != "arg1" || args[1] != "arg2" {
				t.Fatalf("Unexpected command or arguments: %s %v", command, args)
			}

			if runtime.GOOS == "windows" {
				// Use PowerShell to output a mock message
				fullCommand := fmt.Sprintf("Write-Output 'mock output with args'")
				cmdArgs := []string{"-Command", fullCommand}
				return exec.Command("powershell.exe", cmdArgs...)
			} else {
				// Use 'echo' on Unix-like systems
				return exec.Command("echo", "mock output with args")
			}
		}
		defer func() { execCommand = originalExecCommand }()

		// When executing a command with arguments
		output, err := shell.Exec("somecommand", "arg1", "arg2")

		// Then there should be no error and output should match
		assertNoError(t, err)
		expectedOutput := "mock output with args"

		// Account for line endings
		if runtime.GOOS == "windows" {
			expectedOutput += "\r\n" // PowerShell adds \r\n
		} else {
			expectedOutput += "\n" // echo adds \n
		}

		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
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
