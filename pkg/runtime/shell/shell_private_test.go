package shell

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"text/template"
)

// =============================================================================
// Test Setup
// =============================================================================

type ShellTestMocks struct {
	Shims  *Shims
	TmpDir string
}

// setupDefaultShims creates a new Shims instance with default implementations
func setupDefaultShims(tmpDir string) *Shims {
	shims := NewShims()

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
// Test Private Methods
// =============================================================================

// TestDefaultShell_renderEnvVarsPlain tests the renderEnvVarsPlain method
func TestDefaultShell_renderEnvVarsPlain(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("RendersPlainEnvVars", func(t *testing.T) {
		// Given a shell with environment variables
		shell, _ := setup(t)
		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
			"VAR3": "",
		}

		// When rendering plain environment variables
		result := shell.renderEnvVarsPlain(envVars)

		// Then the output should contain sorted KEY=value format
		if !strings.Contains(result, "VAR1=value1\n") {
			t.Errorf("Expected VAR1=value1, got: %s", result)
		}
		if !strings.Contains(result, "VAR2=value2\n") {
			t.Errorf("Expected VAR2=value2, got: %s", result)
		}
		if !strings.Contains(result, "VAR3=\n") {
			t.Errorf("Expected VAR3=, got: %s", result)
		}
	})

	t.Run("RendersEmptyEnvVars", func(t *testing.T) {
		// Given a shell with empty environment variables map
		shell, _ := setup(t)
		envVars := map[string]string{}

		// When rendering plain environment variables
		result := shell.renderEnvVarsPlain(envVars)

		// Then the output should be empty
		if result != "" {
			t.Errorf("Expected empty output for empty env vars, got: %s", result)
		}
	})

	t.Run("QuotesValuesWithSpecialCharacters", func(t *testing.T) {
		// Given a shell with environment variables containing special characters
		shell, _ := setup(t)
		envVars := map[string]string{
			"VAR1": `["item1","item2"]`,
			"VAR2": `{"key": "value"}`,
			"VAR3": "simple_value",
			"VAR4": "value with spaces",
			"VAR5": "value$with$dollars",
		}

		// When rendering plain environment variables
		result := shell.renderEnvVarsPlain(envVars)

		// Then values with special characters should be quoted
		if !strings.Contains(result, "VAR1='[\"item1\",\"item2\"]'\n") {
			t.Errorf("Expected VAR1 to be quoted, got: %s", result)
		}
		if !strings.Contains(result, "VAR2='{\"key\": \"value\"}'\n") {
			t.Errorf("Expected VAR2 to be quoted, got: %s", result)
		}
		if !strings.Contains(result, "VAR3=simple_value\n") {
			t.Errorf("Expected VAR3 to not be quoted, got: %s", result)
		}
		if !strings.Contains(result, "VAR4='value with spaces'\n") {
			t.Errorf("Expected VAR4 to be quoted, got: %s", result)
		}
		if !strings.Contains(result, "VAR5='value$with$dollars'\n") {
			t.Errorf("Expected VAR5 to be quoted, got: %s", result)
		}
	})
}

// TestDefaultShell_quoteValueForShell tests the quoteValueForShell method
func TestDefaultShell_quoteValueForShell(t *testing.T) {
	setup := func(t *testing.T) *DefaultShell {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell
	}

	t.Run("QuotesValuesWithSpecialCharacters", func(t *testing.T) {
		shell := setup(t)

		testCases := []struct {
			name     string
			value    string
			expected string
		}{
			{"JSONArray", `["item1","item2"]`, `'["item1","item2"]'`},
			{"JSONObject", `{"key": "value"}`, `'{"key": "value"}'`},
			{"WithSpaces", "value with spaces", "'value with spaces'"},
			{"WithDollars", "value$test", "'value$test'"},
			{"WithBrackets", "[test]", "'[test]'"},
			{"SimpleValue", "simple", "simple"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := shell.quoteValueForShell(tc.value, false)
				if result != tc.expected {
					t.Errorf("Expected %q, got %q", tc.expected, result)
				}
			})
		}
	})

	t.Run("UsesDoubleQuotesWhenRequested", func(t *testing.T) {
		shell := setup(t)

		result := shell.quoteValueForShell("simple", true)
		if result != `"simple"` {
			t.Errorf("Expected \"simple\", got %q", result)
		}
	})

	t.Run("EscapesSingleQuotesInValue", func(t *testing.T) {
		shell := setup(t)

		result := shell.quoteValueForShell("value'with'quotes", false)
		expected := `'value'"'"'with'"'"'quotes'`
		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

// syncWriter wraps a bytes.Buffer with a channel signaled after every Write. A test
// can wait on the channel instead of racing on a sleep. onWrite, when set, runs
// before the write is recorded. Use it to place a write among other events, such
// as a spinner pause and resume.
type syncWriter struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	written chan struct{}
	onWrite func()
}

func newSyncWriter() *syncWriter {
	return &syncWriter{written: make(chan struct{}, 16)}
}

func (w *syncWriter) Write(p []byte) (int, error) {
	if w.onWrite != nil {
		w.onWrite()
	}
	w.mu.Lock()
	n, err := w.buf.Write(p)
	w.mu.Unlock()
	w.written <- struct{}{}
	return n, err
}

func (w *syncWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}
