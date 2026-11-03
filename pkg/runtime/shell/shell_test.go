package shell_test

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type ShellTestMocks struct {
	Shims  *shell.Shims
	TmpDir string
}

// setupDefaultShims creates a new Shims instance with default implementations
func setupDefaultShims(tmpDir string) *shell.Shims {
	shims := shell.NewShims()

	// Mock command execution with proper cleanup
	shims.Command = func(name string, args ...string) *exec.Cmd {
		cmd := exec.Command("echo", "test")
		cmd.Stdout = new(bytes.Buffer)
		cmd.Stderr = new(bytes.Buffer)
		return cmd
	}

	// Mock command execution methods with proper cleanup
	shims.CmdStart = func(cmd *exec.Cmd) error {
		if cmd.Stdout != nil {
			if _, err := cmd.Stdout.Write([]byte("test\n")); err != nil {
				return fmt.Errorf("failed to write to stdout: %v", err)
			}
		}
		return nil
	}

	shims.CmdWait = func(cmd *exec.Cmd) error {
		return nil
	}

	shims.CmdRun = func(cmd *exec.Cmd) error {
		if cmd.Stdout != nil {
			if _, err := cmd.Stdout.Write([]byte("test\n")); err != nil {
				return fmt.Errorf("failed to write to stdout: %v", err)
			}
		}
		return nil
	}

	// Mock pipes with proper cleanup
	shims.StdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
		r, w := io.Pipe()
		go func() {
			if _, err := w.Write([]byte("test output\n")); err != nil {
				// Ignore error in goroutine
			}
			w.Close()
		}()
		return r, nil
	}

	shims.StderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) {
		r, w := io.Pipe()
		go func() {
			if _, err := w.Write([]byte("error\n")); err != nil {
				// Ignore error in goroutine
			}
			w.Close()
		}()
		return r, nil
	}

	// Mock file operations
	shims.Getwd = func() (string, error) {
		return tmpDir, nil
	}

	shims.Stat = func(name string) (os.FileInfo, error) {
		if name == "trusted_dirs" {
			return nil, os.ErrNotExist
		}
		return nil, nil
	}

	// Mock file operations with proper cleanup
	shims.OpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		return os.NewFile(0, "test"), nil
	}

	shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
		return nil
	}

	shims.ReadFile = func(name string) ([]byte, error) {
		return []byte("test\n"), nil
	}

	shims.MkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}

	shims.Remove = func(name string) error {
		return nil
	}

	shims.RemoveAll = func(path string) error {
		return nil
	}

	shims.Setenv = func(key, value string) error {
		return nil
	}

	shims.Getenv = func(key string) string {
		return ""
	}

	shims.UserHomeDir = func() (string, error) {
		return tmpDir, nil
	}

	// Mock random operations with proper cleanup
	shims.RandRead = func(b []byte) (n int, err error) {
		for i := range b {
			b[i] = byte(i % 62)
		}
		return len(b), nil
	}

	// Mock template operations with proper cleanup
	shims.NewTemplate = func(name string) *template.Template {
		return template.New(name)
	}

	shims.TemplateParse = func(tmpl *template.Template, text string) (*template.Template, error) {
		return tmpl.Parse(text)
	}

	shims.TemplateExecute = func(tmpl *template.Template, wr io.Writer, data any) error {
		return nil
	}

	// Mock bufio operations with proper cleanup
	shims.ScannerErr = func(scanner *bufio.Scanner) error {
		return nil
	}

	// Mock filepath operations
	shims.Glob = func(pattern string) ([]string, error) {
		return []string{filepath.Join(tmpDir, "test")}, nil
	}

	shims.ScannerText = func(scanner *bufio.Scanner) string {
		return scanner.Text()
	}

	shims.IsTerminal = func(fd int) bool {
		return true
	}

	return shims
}

// setupShellMocks creates a new set of mocks for testing
func setupShellMocks(t *testing.T) *ShellTestMocks {
	t.Helper()

	// Create temp dir
	tmpDir := t.TempDir()

	// Create initial mocks with defaults
	mocks := &ShellTestMocks{
		Shims:  setupDefaultShims(tmpDir),
		TmpDir: tmpDir,
	}

	return mocks
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestShell_NewDefaultShell(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a shell
		subject := shell.NewDefaultShell()

		// Then it should be created
		if subject == nil {
			t.Error("Expected shell to be created")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestShell_SetVerbosity(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a shell
		subject := shell.NewDefaultShell()

		// When setting verbosity to true
		subject.SetVerbosity(true)

		// Then verbosity should be true
		if !subject.Verbose() {
			t.Fatalf("Expected verbosity to be true, got false")
		}
	})

	t.Run("DisableVerbosity", func(t *testing.T) {
		// Given a shell
		subject := shell.NewDefaultShell()

		// When setting verbosity to false
		subject.SetVerbosity(false)

		// Then verbosity should be false
		if subject.Verbose() {
			t.Fatalf("Expected verbosity to be false, got true")
		}
	})
}

func TestShell_GetProjectRoot(t *testing.T) {
	setup := func(t *testing.T) (*shell.DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		subject := shell.NewDefaultShell()
		subject.SetShims(mocks.Shims)
		return subject, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a shell with project root set
		subject, _ := setup(t)
		subject.SetProjectRoot("/test/root")

		// When getting the project root
		root, err := subject.GetProjectRoot()

		// Then it should return the expected root
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if root != "/test/root" {
			t.Errorf("Expected /test/root, got %s", root)
		}
	})

	t.Run("FindsProjectRoot", func(t *testing.T) {
		// Given a shell in a directory with windsor.yaml
		subject, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/current", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/current", "windsor.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// When getting the project root
		root, err := subject.GetProjectRoot()

		// Then it should find the root directory
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if root != "/test/current" {
			t.Errorf("Expected /test/current, got %s", root)
		}
	})

	t.Run("FindsProjectRootWithYml", func(t *testing.T) {
		// Given a shell in a directory with windsor.yml
		subject, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/current", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/current", "windsor.yml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// When getting the project root
		root, err := subject.GetProjectRoot()

		// Then it should find the root directory
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if root != "/test/current" {
			t.Errorf("Expected /test/current, got %s", root)
		}
	})

	t.Run("ErrorOnGetwdFailure", func(t *testing.T) {
		// Given a shell with failing Getwd
		subject, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("getwd failed")
		}

		// When getting project root
		_, err := subject.GetProjectRoot()

		// Then it should return an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "getwd failed") {
			t.Errorf("Expected error to contain 'getwd failed', got %v", err)
		}
	})

	t.Run("MaxDepthExceededFallsBackToGlobalRoot", func(t *testing.T) {
		// Given a shell in a deep directory structure without windsor.yaml/yml
		subject, mocks := setup(t)
		originalDir := "/test/very/deep/directory/structure/without/config/file"
		mocks.Shims.Getwd = func() (string, error) {
			return originalDir, nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "/home/user", nil
		}

		// When getting the project root
		root, err := subject.GetProjectRoot()

		// Then it should fall back to $HOME/.config/windsor in global mode
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		expected := filepath.Join("/home/user", ".config", "windsor")
		if root != expected {
			t.Errorf("Expected %s, got %s", expected, root)
		}
		if !subject.IsGlobal() {
			t.Error("Expected IsGlobal() to be true when falling back")
		}
	})

	t.Run("FallsBackToGlobalRootWhenFilesystemRootReached", func(t *testing.T) {
		// Given a shell starting at filesystem root without windsor.yaml/yml
		subject, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "/home/user", nil
		}

		// When getting the project root
		root, err := subject.GetProjectRoot()

		// Then it should fall back to $HOME/.config/windsor in global mode
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		expected := filepath.Join("/home/user", ".config", "windsor")
		if root != expected {
			t.Errorf("Expected %s, got %s", expected, root)
		}
		if !subject.IsGlobal() {
			t.Error("Expected IsGlobal() to be true when falling back")
		}
	})

	t.Run("GlobalFallbackCreatesDirectory", func(t *testing.T) {
		// Given a shell that will fall back to global mode
		subject, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "/home/user", nil
		}
		var createdPath string
		var createdMode os.FileMode
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			createdPath = path
			createdMode = perm
			return nil
		}

		// When getting the project root
		_, err := subject.GetProjectRoot()

		// Then MkdirAll should be called on the global root path
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		expected := filepath.Join("/home/user", ".config", "windsor")
		if createdPath != expected {
			t.Errorf("Expected MkdirAll called with %s, got %s", expected, createdPath)
		}
		if createdMode != 0750 {
			t.Errorf("Expected mode 0750, got %o", createdMode)
		}
	})

	t.Run("GlobalFallbackReturnsOriginalDirWhenHomeDirFails", func(t *testing.T) {
		// Given a shell where UserHomeDir fails during fallback
		subject, mocks := setup(t)
		originalDir := "/some/dir"
		mocks.Shims.Getwd = func() (string, error) {
			return originalDir, nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "", fmt.Errorf("no home dir")
		}

		// When getting the project root
		root, err := subject.GetProjectRoot()

		// Then it should return the original dir without marking global mode
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if root != originalDir {
			t.Errorf("Expected %s, got %s", originalDir, root)
		}
		if subject.IsGlobal() {
			t.Error("Expected IsGlobal() to be false when home dir resolution fails")
		}
	})

	t.Run("IsGlobalFalseWhenProjectFound", func(t *testing.T) {
		// Given a shell in a directory with windsor.yaml
		subject, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "/test/current", nil
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/current", "windsor.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// When getting the project root
		_, err := subject.GetProjectRoot()

		// Then IsGlobal() should be false
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if subject.IsGlobal() {
			t.Error("Expected IsGlobal() to be false when windsor.yaml is found")
		}
	})

	t.Run("MkdirAllFailureCachesOriginalDirAndShortCircuitsSubsequentCalls", func(t *testing.T) {
		// Given a shell that will fall back to global mode but MkdirAll fails
		// (e.g. read-only filesystem or restrictive $HOME/.config permissions)
		subject, mocks := setup(t)
		originalDir := "/some/dir"
		mocks.Shims.Getwd = func() (string, error) {
			return originalDir, nil
		}
		statCalls := 0
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			statCalls++
			return nil, os.ErrNotExist
		}
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "/home/user", nil
		}
		mkdirCalls := 0
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			mkdirCalls++
			return fmt.Errorf("permission denied")
		}

		// When getting the project root twice
		root1, err1 := subject.GetProjectRoot()
		statCallsAfterFirst := statCalls
		root2, err2 := subject.GetProjectRoot()

		// Then both calls return the original dir without erroring
		if err1 != nil {
			t.Errorf("Expected no error on first call, got %v", err1)
		}
		if err2 != nil {
			t.Errorf("Expected no error on second call, got %v", err2)
		}
		if root1 != originalDir || root2 != originalDir {
			t.Errorf("Expected %s on both calls, got %s and %s", originalDir, root1, root2)
		}
		// And global mode should not be claimed in the degraded state
		if subject.IsGlobal() {
			t.Error("Expected IsGlobal() to be false when MkdirAll fails")
		}
		// And the second call should not re-traverse the directory tree
		if statCalls != statCallsAfterFirst {
			t.Errorf("Expected no additional Stat calls on second invocation, got %d additional", statCalls-statCallsAfterFirst)
		}
		// And the failing MkdirAll should not be re-attempted
		if mkdirCalls != 1 {
			t.Errorf("Expected MkdirAll to be called exactly once, got %d", mkdirCalls)
		}
	})

	t.Run("UserHomeDirFailureCachesOriginalDirAndShortCircuitsSubsequentCalls", func(t *testing.T) {
		// Given a shell where UserHomeDir fails during fallback
		subject, mocks := setup(t)
		originalDir := "/some/dir"
		mocks.Shims.Getwd = func() (string, error) {
			return originalDir, nil
		}
		statCalls := 0
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			statCalls++
			return nil, os.ErrNotExist
		}
		homeDirCalls := 0
		mocks.Shims.UserHomeDir = func() (string, error) {
			homeDirCalls++
			return "", fmt.Errorf("no home dir")
		}

		// When getting the project root twice
		_, err1 := subject.GetProjectRoot()
		statCallsAfterFirst := statCalls
		_, err2 := subject.GetProjectRoot()

		// Then neither call errors and the second short-circuits
		if err1 != nil || err2 != nil {
			t.Errorf("Expected no errors, got %v and %v", err1, err2)
		}
		if statCalls != statCallsAfterFirst {
			t.Errorf("Expected no additional Stat calls on second invocation, got %d additional", statCalls-statCallsAfterFirst)
		}
		if homeDirCalls != 1 {
			t.Errorf("Expected UserHomeDir to be called exactly once, got %d", homeDirCalls)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

// mockReadCloser implements io.ReadCloser for testing
type mockReadCloser struct {
	io.Reader
	closeFunc func() error
}

func (m *mockReadCloser) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

// captureStderr returns what f writes to os.Stderr.
func captureStderr(t *testing.T, f func()) string {
	var output bytes.Buffer
	originalStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

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
	os.Stderr = originalStderr
	return output.String()
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
