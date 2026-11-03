package debug_test

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/windsorcli/cli/pkg/debug"
)

// =============================================================================
// Test Setup
// =============================================================================

func resetDebug(t *testing.T) {
	t.Helper()
	debug.Init(false)
	t.Cleanup(func() { debug.Init(false) })
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	old := os.Stderr
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()
	return buf.String()
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestInit(t *testing.T) {
	t.Run("EnablesDebugOutput", func(t *testing.T) {
		// Given debug output is off
		resetDebug(t)

		// When Init is called with true
		debug.Init(true)

		// Then Log writes output
		output := captureStderr(t, func() { debug.Log("on") })
		if output != "[debug] on\n" {
			t.Fatalf("expected debug output, got %q", output)
		}
	})

	t.Run("DisablesDebugOutput", func(t *testing.T) {
		// Given debug output is on
		resetDebug(t)
		debug.Init(true)

		// When Init is called with false
		debug.Init(false)

		// Then Log writes nothing
		output := captureStderr(t, func() { debug.Log("off") })
		if output != "" {
			t.Fatalf("expected no output, got %q", output)
		}
	})
}

func TestLog(t *testing.T) {
	t.Run("WritesToStderrWhenEnabled", func(t *testing.T) {
		// Given debug output is enabled
		resetDebug(t)
		debug.Init(true)

		// When Log is called with a formatted message
		output := captureStderr(t, func() {
			debug.Log("value is %s", "workloadidentity")
		})

		// Then the message is written with the debug prefix
		want := "[debug] value is workloadidentity\n"
		if output != want {
			t.Fatalf("expected output %q, got %q", want, output)
		}
	})

	t.Run("WritesNothingWhenDisabled", func(t *testing.T) {
		// Given debug output is disabled
		resetDebug(t)

		// When Log is called
		output := captureStderr(t, func() {
			debug.Log("value is %s", "workloadidentity")
		})

		// Then nothing is written
		if output != "" {
			t.Fatalf("expected no output, got %q", output)
		}
	})
}
