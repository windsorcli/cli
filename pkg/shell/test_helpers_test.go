package shell

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

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
