package shell

import (
	"bufio"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestNewShims(t *testing.T) {
	t.Run("CreatesShimsWithAllFunctions", func(t *testing.T) {
		// Given a new Shims instance
		// When NewShims is called
		shims := NewShims()

		// Then all shim functions should be initialized
		if shims.Getwd == nil {
			t.Error("Getwd shim not initialized")
		}
		if shims.Stat == nil {
			t.Error("Stat shim not initialized")
		}
		if shims.ReadFile == nil {
			t.Error("ReadFile shim not initialized")
		}
		if shims.WriteFile == nil {
			t.Error("WriteFile shim not initialized")
		}
		if shims.MkdirAll == nil {
			t.Error("MkdirAll shim not initialized")
		}
		if shims.RemoveAll == nil {
			t.Error("RemoveAll shim not initialized")
		}
		if shims.Getenv == nil {
			t.Error("Getenv shim not initialized")
		}
		if shims.UserHomeDir == nil {
			t.Error("UserHomeDir shim not initialized")
		}
		if shims.Glob == nil {
			t.Error("Glob shim not initialized")
		}
		if shims.Executable == nil {
			t.Error("Executable shim not initialized")
		}
		if shims.Command == nil {
			t.Error("Command shim not initialized")
		}
		if shims.Environ == nil {
			t.Error("Environ shim not initialized")
		}
		if shims.OpenFile == nil {
			t.Error("OpenFile shim not initialized")
		}
		if shims.CmdRun == nil {
			t.Error("CmdRun shim not initialized")
		}
		if shims.CmdStart == nil {
			t.Error("CmdStart shim not initialized")
		}
		if shims.CmdWait == nil {
			t.Error("CmdWait shim not initialized")
		}
		if shims.StdoutPipe == nil {
			t.Error("StdoutPipe shim not initialized")
		}
		if shims.StderrPipe == nil {
			t.Error("StderrPipe shim not initialized")
		}
		if shims.NewTemplate == nil {
			t.Error("NewTemplate shim not initialized")
		}
		if shims.TemplateParse == nil {
			t.Error("TemplateParse shim not initialized")
		}
		if shims.TemplateExecute == nil {
			t.Error("TemplateExecute shim not initialized")
		}
		if shims.NewScanner == nil {
			t.Error("NewScanner shim not initialized")
		}
		if shims.ScannerScan == nil {
			t.Error("ScannerScan shim not initialized")
		}
		if shims.ScannerErr == nil {
			t.Error("ScannerErr shim not initialized")
		}
		if shims.ScannerText == nil {
			t.Error("ScannerText shim not initialized")
		}
		if shims.RandRead == nil {
			t.Error("RandRead shim not initialized")
		}
		if shims.IsTerminal == nil {
			t.Error("IsTerminal shim not initialized")
		}
		if shims.SignalNotify == nil {
			t.Error("SignalNotify shim not initialized")
		}
		if shims.SignalStop == nil {
			t.Error("SignalStop shim not initialized")
		}
		if shims.InterruptProcessGroup == nil {
			t.Error("InterruptProcessGroup shim not initialized")
		}
		if shims.KillProcessGroup == nil {
			t.Error("KillProcessGroup shim not initialized")
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
