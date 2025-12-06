package shell

import (
	"bufio"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestShell_NewShims(t *testing.T) {
	t.Run("InitializesAllShims", func(t *testing.T) {
		// When we create new shims
		shims := NewShims()

		// Then all shims should be non-nil
		if shims == nil {
			t.Fatal("Expected non-nil shims")
		}

		// We don't test the actual implementations since they are real system calls
		// Instead we just verify that all fields are initialized
		if shims.Getwd == nil {
			t.Error("Expected Getwd to be initialized")
		}
		if shims.Stat == nil {
			t.Error("Expected Stat to be initialized")
		}
		if shims.Executable == nil {
			t.Error("Expected Executable to be initialized")
		}
		if shims.Stderr == nil {
			t.Error("Expected Stderr to be initialized")
		}
		if shims.SetStderr == nil {
			t.Error("Expected SetStderr to be initialized")
		}
		if shims.Stdout == nil {
			t.Error("Expected Stdout to be initialized")
		}
		if shims.SetStdout == nil {
			t.Error("Expected SetStdout to be initialized")
		}
		if shims.Pipe == nil {
			t.Error("Expected Pipe to be initialized")
		}
		if shims.Command == nil {
			t.Error("Expected Command to be initialized")
		}
		if shims.LookPath == nil {
			t.Error("Expected LookPath to be initialized")
		}
		if shims.OpenFile == nil {
			t.Error("Expected OpenFile to be initialized")
		}
		if shims.WriteFile == nil {
			t.Error("Expected WriteFile to be initialized")
		}
		if shims.ReadFile == nil {
			t.Error("Expected ReadFile to be initialized")
		}
		if shims.MkdirAll == nil {
			t.Error("Expected MkdirAll to be initialized")
		}
		if shims.Remove == nil {
			t.Error("Expected Remove to be initialized")
		}
		if shims.RemoveAll == nil {
			t.Error("Expected RemoveAll to be initialized")
		}
		if shims.Chdir == nil {
			t.Error("Expected Chdir to be initialized")
		}
		if shims.Setenv == nil {
			t.Error("Expected Setenv to be initialized")
		}
		if shims.Getenv == nil {
			t.Error("Expected Getenv to be initialized")
		}
		if shims.UserHomeDir == nil {
			t.Error("Expected UserHomeDir to be initialized")
		}
		if shims.CmdRun == nil {
			t.Error("Expected CmdRun to be initialized")
		}
		if shims.CmdStart == nil {
			t.Error("Expected CmdStart to be initialized")
		}
		if shims.CmdWait == nil {
			t.Error("Expected CmdWait to be initialized")
		}
		if shims.StdoutPipe == nil {
			t.Error("Expected StdoutPipe to be initialized")
		}
		if shims.StderrPipe == nil {
			t.Error("Expected StderrPipe to be initialized")
		}
		if shims.StdinPipe == nil {
			t.Error("Expected StdinPipe to be initialized")
		}
		if shims.NewTemplate == nil {
			t.Error("Expected NewTemplate to be initialized")
		}
		if shims.TemplateParse == nil {
			t.Error("Expected TemplateParse to be initialized")
		}
		if shims.TemplateExecute == nil {
			t.Error("Expected TemplateExecute to be initialized")
		}
		if shims.ExecuteTemplate == nil {
			t.Error("Expected ExecuteTemplate to be initialized")
		}
		if shims.ScannerScan == nil {
			t.Error("Expected ScannerScan to be initialized")
		}
		if shims.ScannerErr == nil {
			t.Error("Expected ScannerErr to be initialized")
		}
		if shims.ScannerText == nil {
			t.Error("Expected ScannerText to be initialized")
		}
		if shims.NewWriter == nil {
			t.Error("Expected NewWriter to be initialized")
		}
		if shims.Glob == nil {
			t.Error("Expected Glob to be initialized")
		}
		if shims.Join == nil {
			t.Error("Expected Join to be initialized")
		}
		if shims.RandRead == nil {
			t.Error("Expected RandRead to be initialized")
		}
		// Int63n is allowed to be nil as it's not initialized in NewShims
	})
}

func TestNewShims(t *testing.T) {
	t.Run("InitializesAllShims", func(t *testing.T) {
		// When we create new shims
		shims := NewShims()

		// Then all shims should be initialized
		if shims.Getwd == nil {
			t.Error("Expected Getwd to be initialized")
		}
		if shims.Stat == nil {
			t.Error("Expected Stat to be initialized")
		}
		if shims.Executable == nil {
			t.Error("Expected Executable to be initialized")
		}
		if shims.Stderr == nil {
			t.Error("Expected Stderr to be initialized")
		}
		if shims.Stdout == nil {
			t.Error("Expected Stdout to be initialized")
		}
		if shims.Command == nil {
			t.Error("Expected Command to be initialized")
		}
		if shims.UserHomeDir == nil {
			t.Error("Expected UserHomeDir to be initialized")
		}

		// Verify a shim actually works
		stderr := shims.Stderr()
		if stderr != os.Stderr {
			t.Error("Expected Stderr to return os.Stderr")
		}
	})

	t.Run("SetStderr", func(t *testing.T) {
		// Given
		shims := NewShims()
		origStderr := os.Stderr
		defer func() { os.Stderr = origStderr }()

		// When setting a non-file writer
		shims.SetStderr(&mockWriter{})
		// Then stderr should not change
		if os.Stderr != origStderr {
			t.Error("Expected Stderr to remain unchanged for non-file writer")
		}

		// When setting a file writer
		tmpFile, err := os.CreateTemp("", "stderr")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpFile.Name())
		shims.SetStderr(tmpFile)
		// Then stderr should change
		if os.Stderr == origStderr {
			t.Error("Expected Stderr to change for file writer")
		}
	})

	t.Run("SetStdout", func(t *testing.T) {
		// Given
		shims := NewShims()
		origStdout := os.Stdout
		defer func() { os.Stdout = origStdout }()

		// When setting a non-file writer
		shims.SetStdout(&mockWriter{})
		// Then stdout should not change
		if os.Stdout != origStdout {
			t.Error("Expected Stdout to remain unchanged for non-file writer")
		}

		// When setting a file writer
		tmpFile, err := os.CreateTemp("", "stdout")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpFile.Name())
		shims.SetStdout(tmpFile)
		// Then stdout should change
		if os.Stdout == origStdout {
			t.Error("Expected Stdout to change for file writer")
		}
	})

	t.Run("Pipe", func(t *testing.T) {
		// Given
		shims := NewShims()

		// When creating a pipe
		r, w, err := shims.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		defer w.Close()

		// Then we should be able to write and read
		testData := []byte("test data")
		if _, err := w.Write(testData); err != nil {
			t.Errorf("Failed to write test data: %v", err)
		}

		buf := make([]byte, len(testData))
		n, err := r.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		if n != len(testData) {
			t.Errorf("Expected to read %d bytes, got %d", len(testData), n)
		}
		if string(buf) != string(testData) {
			t.Errorf("Expected %q, got %q", testData, buf)
		}
	})

	t.Run("RandRead", func(t *testing.T) {
		// Given
		shims := NewShims()
		buf := make([]byte, 32)

		// When reading random bytes
		n, err := shims.RandRead(buf)
		if err != nil {
			t.Fatal(err)
		}
		if n != len(buf) {
			t.Errorf("Expected to read %d bytes, got %d", len(buf), n)
		}

		// Then we should get random data
		zeroCount := 0
		for _, b := range buf {
			if b == 0 {
				zeroCount++
			}
		}
		if zeroCount == len(buf) {
			t.Error("Expected random data, got all zeros")
		}
	})
}

type mockWriter struct{}

func (m *mockWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func TestShims_ExecOperations(t *testing.T) {
	t.Run("CmdOperations", func(t *testing.T) {
		// Given
		shims := NewShims()

		// Use Go's built-in test binary as a reliable cross-platform command
		cmd := shims.Command(os.Args[0], "-test.run=TestShims_ExecOperations/ThisTestDoesNotExist")

		// Test CmdRun
		if err := shims.CmdRun(cmd); err != nil {
			// We expect an error since the test doesn't exist, but it should run
			if e, ok := err.(*exec.ExitError); !ok || e.ExitCode() != 1 {
				t.Errorf("Expected exit code 1 for non-existent test, got: %v", err)
			}
		}

		// Test CmdStart and CmdWait
		cmd = shims.Command(os.Args[0], "-test.run=TestShims_ExecOperations/ThisTestDoesNotExist")
		if err := shims.CmdStart(cmd); err != nil {
			t.Errorf("Expected CmdStart to succeed, got error: %v", err)
		}
		if err := shims.CmdWait(cmd); err != nil {
			// We expect an error since the test doesn't exist, but it should complete
			if e, ok := err.(*exec.ExitError); !ok || e.ExitCode() != 1 {
				t.Errorf("Expected exit code 1 for non-existent test, got: %v", err)
			}
		}
	})

	t.Run("PipeOperations", func(t *testing.T) {
		// Given
		shims := NewShims()
		cmd := shims.Command(os.Args[0], "-test.run=TestShims_ExecOperations/ThisTestDoesNotExist")

		// Test StdinPipe
		stdin, err := shims.StdinPipe(cmd)
		if err != nil {
			t.Errorf("Expected StdinPipe to succeed, got error: %v", err)
		}
		defer stdin.Close()

		// Test StdoutPipe
		stdout, err := shims.StdoutPipe(cmd)
		if err != nil {
			t.Errorf("Expected StdoutPipe to succeed, got error: %v", err)
		}
		defer stdout.Close()

		// Test StderrPipe
		stderr, err := shims.StderrPipe(cmd)
		if err != nil {
			t.Errorf("Expected StderrPipe to succeed, got error: %v", err)
		}
		defer stderr.Close()
	})
}

func TestShims_TemplateOperations(t *testing.T) {
	t.Run("TemplateOperations", func(t *testing.T) {
		// Given
		shims := NewShims()
		tmpl := shims.NewTemplate("test")

		// Test TemplateParse
		parsed, err := shims.TemplateParse(tmpl, "Hello {{.}}")
		if err != nil {
			t.Errorf("Expected TemplateParse to succeed, got error: %v", err)
		}

		// Test TemplateExecute
		var buf mockWriter
		if err := shims.TemplateExecute(parsed, &buf, "World"); err != nil {
			t.Errorf("Expected TemplateExecute to succeed, got error: %v", err)
		}

		// Test ExecuteTemplate
		if err := shims.ExecuteTemplate(parsed, "World"); err != nil {
			t.Errorf("Expected ExecuteTemplate to succeed, got error: %v", err)
		}
	})

	t.Run("TemplateParseError", func(t *testing.T) {
		// Given
		shims := NewShims()
		tmpl := shims.NewTemplate("test")

		// When parsing an invalid template
		_, err := shims.TemplateParse(tmpl, "{{.Invalid}")
		if err == nil {
			t.Error("Expected error for invalid template")
		}
	})

	t.Run("TemplateExecuteError", func(t *testing.T) {
		// Given
		shims := NewShims()
		tmpl := shims.NewTemplate("test")
		tmpl, err := shims.TemplateParse(tmpl, "{{.MissingField}}")
		if err != nil {
			t.Fatal(err)
		}

		// When executing with invalid data
		err = shims.TemplateExecute(tmpl, &strings.Builder{}, struct{}{})
		if err == nil {
			t.Error("Expected error for missing field")
		}
	})
}

func TestShims_ScannerOperations(t *testing.T) {
	t.Run("ScannerOperations", func(t *testing.T) {
		// Given
		shims := NewShims()
		scanner := bufio.NewScanner(strings.NewReader("test\n"))

		// Test ScannerScan
		if !shims.ScannerScan(scanner) {
			t.Error("Expected ScannerScan to return true")
		}

		// Test ScannerText
		if text := shims.ScannerText(scanner); text != "test" {
			t.Errorf("Expected ScannerText to return 'test', got %q", text)
		}

		// Test ScannerErr
		if err := shims.ScannerErr(scanner); err != nil {
			t.Errorf("Expected ScannerErr to return nil, got %v", err)
		}
	})

	t.Run("ScannerError", func(t *testing.T) {
		// Given
		shims := NewShims()
		r := strings.NewReader(strings.Repeat("x", bufio.MaxScanTokenSize+1))
		scanner := bufio.NewScanner(r)
		scanner.Split(bufio.ScanWords)

		// When scanning a token that's too large
		ok := shims.ScannerScan(scanner)
		if ok {
			t.Error("Expected scan to fail")
		}

		// Then we should get an error
		err := shims.ScannerErr(scanner)
		if err == nil {
			t.Error("Expected error for token too large")
		}
	})
}
