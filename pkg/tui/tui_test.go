package tui

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupMock(t *testing.T) *MockSpinner {
	t.Helper()
	mock := NewMockSpinner()
	original := Active
	Active = mock
	t.Cleanup(func() { Active = original })
	return mock
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
// Test Constructor
// =============================================================================

// =============================================================================
// Test Public Methods
// =============================================================================

// Tests for package-level Start delegation
func TestStart(t *testing.T) {
	t.Run("DelegatesToActive", func(t *testing.T) {
		// Given a mock Active spinner
		var got string
		mock := setupMock(t)
		mock.StartFunc = func(message string) { got = message }

		// When Start is called
		Start("loading")

		// Then the mock receives the message
		if got != "loading" {
			t.Errorf("expected %q, got %q", "loading", got)
		}
	})
}

// Tests for package-level Update delegation
func TestUpdate(t *testing.T) {
	t.Run("DelegatesToActive", func(t *testing.T) {
		// Given a mock Active spinner
		var got string
		mock := setupMock(t)
		mock.UpdateFunc = func(message string) { got = message }

		// When Update is called
		Update("refreshing")

		// Then the mock receives the message
		if got != "refreshing" {
			t.Errorf("expected %q, got %q", "refreshing", got)
		}
	})
}

// Tests for package-level Done delegation
func TestDone(t *testing.T) {
	t.Run("DelegatesToActive", func(t *testing.T) {
		// Given a mock Active spinner
		called := false
		mock := setupMock(t)
		mock.DoneFunc = func() { called = true }

		// When Done is called
		Done()

		// Then the mock DoneFunc is invoked
		if !called {
			t.Error("expected DoneFunc to be called")
		}
	})
}

// Tests for package-level Fail delegation
func TestFail(t *testing.T) {
	t.Run("DelegatesToActive", func(t *testing.T) {
		// Given a mock Active spinner
		called := false
		mock := setupMock(t)
		mock.FailFunc = func() { called = true }

		// When Fail is called
		Fail()

		// Then the mock FailFunc is invoked
		if !called {
			t.Error("expected FailFunc to be called")
		}
	})
}

// Tests for WithProgress success and error flows
func TestWithProgress(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock Active spinner and a fn that succeeds
		started, done, failed := "", false, false
		mock := setupMock(t)
		mock.StartFunc = func(message string) { started = message }
		mock.DoneFunc = func() { done = true }
		mock.FailFunc = func() { failed = true }

		// When WithProgress is called with a successful fn
		err := WithProgress("working", func() error { return nil })

		// Then Start and Done are called, Fail is not, and nil is returned
		if started != "working" {
			t.Errorf("expected Start(%q), got Start(%q)", "working", started)
		}
		if !done {
			t.Error("expected Done to be called")
		}
		if failed {
			t.Error("expected Fail not to be called")
		}
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock Active spinner and a fn that returns an error
		started, done, failed := "", false, false
		mock := setupMock(t)
		mock.StartFunc = func(message string) { started = message }
		mock.DoneFunc = func() { done = true }
		mock.FailFunc = func() { failed = true }
		fnErr := fmt.Errorf("boom")

		// When WithProgress is called with a failing fn
		err := WithProgress("working", func() error { return fnErr })

		// Then Start and Fail are called, Done is not, and the error is returned
		if started != "working" {
			t.Errorf("expected Start(%q), got Start(%q)", "working", started)
		}
		if done {
			t.Error("expected Done not to be called")
		}
		if !failed {
			t.Error("expected Fail to be called")
		}
		if err != fnErr {
			t.Errorf("expected %v, got %v", fnErr, err)
		}
	})
}

// Tests for termSpinner Start behavior
func TestTermSpinner_Start(t *testing.T) {
	t.Run("SetsMessage", func(t *testing.T) {
		// Given a termSpinner
		s := &termSpinner{}
		t.Cleanup(func() { s.Done() })

		// When Start is called
		captureStderr(t, func() { s.Start("loading") })

		// Then the message is stored and a spinner is created
		if s.message != "loading" {
			t.Errorf("expected message %q, got %q", "loading", s.message)
		}
		if s.spin == nil {
			t.Error("expected spin to be non-nil after Start")
		}
	})

	t.Run("StopsExistingSpinFirst", func(t *testing.T) {
		// Given a termSpinner that has already been started
		s := &termSpinner{}
		captureStderr(t, func() { s.Start("first") })
		first := s.spin
		t.Cleanup(func() { captureStderr(t, s.Done) })

		// When Start is called again
		captureStderr(t, func() { s.Start("second") })

		// Then a new spinner is created, distinct from the first
		if s.spin == first {
			t.Error("expected a new spinner to be created")
		}
		if s.message != "second" {
			t.Errorf("expected message %q, got %q", "second", s.message)
		}
	})
}

// Tests for termSpinner Update behavior
func TestTermSpinner_Update(t *testing.T) {
	t.Run("UpdatesMessage", func(t *testing.T) {
		// Given a termSpinner with no active spin
		s := &termSpinner{message: "old"}

		// When Update is called
		s.Update("new")

		// Then the stored message is unchanged (Update only changes the suffix)
		if s.message != "old" {
			t.Errorf("expected message %q, got %q", "old", s.message)
		}
	})

	t.Run("UpdatesSpinSuffix", func(t *testing.T) {
		// Given a termSpinner with an active spin
		s := &termSpinner{}
		captureStderr(t, func() { s.Start("old") })
		t.Cleanup(func() { captureStderr(t, s.Done) })

		// When Update is called
		s.Update("new")

		// Then the spin suffix is updated but the stored message remains unchanged
		if s.message != "old" {
			t.Errorf("expected message %q, got %q", "old", s.message)
		}
		if s.spin.Suffix != " new" {
			t.Errorf("expected spin suffix %q, got %q", " new", s.spin.Suffix)
		}
	})

	t.Run("NoopSuffixWhenNilSpin", func(t *testing.T) {
		// Given a termSpinner with no active spin
		s := &termSpinner{}

		// When Update is called
		// Then no panic occurs and the stored message remains unchanged
		s.Update("msg")
		if s.message != "" {
			t.Errorf("expected message %q, got %q", "", s.message)
		}
	})
}

// Tests for termSpinner Done output
func TestTermSpinner_Done(t *testing.T) {
	t.Run("PrintsSuccessLine", func(t *testing.T) {
		// Given a termSpinner with a message but no active spin
		s := &termSpinner{message: "deploying"}

		// When Done is called
		out := captureStderr(t, s.Done)

		// Then a green success line is written to stderr
		want := "\033[32m✔\033[0m deploying - \033[32mDone\033[0m\n"
		if out != want {
			t.Errorf("expected %q, got %q", want, out)
		}
	})
}

// Tests for termSpinner Fail output
func TestTermSpinner_Fail(t *testing.T) {
	t.Run("PrintsFailureLine", func(t *testing.T) {
		// Given a termSpinner with a message but no active spin
		s := &termSpinner{message: "deploying"}

		// When Fail is called
		out := captureStderr(t, s.Fail)

		// Then a red failure line is written to stderr
		want := "\033[31m✗ deploying - Failed\033[0m\n"
		if out != want {
			t.Errorf("expected %q, got %q", want, out)
		}
	})

	t.Run("StopsActiveSpin", func(t *testing.T) {
		// Given a termSpinner with an active spin
		s := &termSpinner{}
		captureStderr(t, func() { s.Start("deploying") })

		// When Fail is called
		captureStderr(t, s.Fail)

		// Then spin is nil and the spinner was stopped
		if s.spin != nil {
			t.Error("expected spin to be nil after Fail")
		}
	})
}

// Tests for verboseSpinner output behavior
func TestVerboseSpinner_Start(t *testing.T) {
	t.Run("PrintsMessage", func(t *testing.T) {
		// Given a verboseSpinner
		s := &verboseSpinner{}

		// When Start is called
		out := captureStderr(t, func() { s.Start("loading") })

		// Then the message is written to stderr
		if out != "loading\n" {
			t.Errorf("expected %q, got %q", "loading\n", out)
		}
	})
}

// Tests for verboseSpinner Update output
func TestVerboseSpinner_Update(t *testing.T) {
	t.Run("PrintsMessage", func(t *testing.T) {
		// Given a verboseSpinner
		s := &verboseSpinner{}

		// When Update is called
		out := captureStderr(t, func() { s.Update("refreshing") })

		// Then the message is written to stderr
		if out != "refreshing\n" {
			t.Errorf("expected %q, got %q", "refreshing\n", out)
		}
	})
}

// Tests for verboseSpinner Done no-op
func TestVerboseSpinner_Done(t *testing.T) {
	t.Run("IsNoop", func(t *testing.T) {
		// Given a verboseSpinner
		s := &verboseSpinner{}

		// When Done is called
		out := captureStderr(t, s.Done)

		// Then nothing is written to stderr
		if out != "" {
			t.Errorf("expected empty output, got %q", out)
		}
	})
}

// Tests for verboseSpinner Fail no-op
func TestVerboseSpinner_Fail(t *testing.T) {
	t.Run("IsNoop", func(t *testing.T) {
		// Given a verboseSpinner
		s := &verboseSpinner{}

		// When Fail is called
		out := captureStderr(t, s.Fail)

		// Then nothing is written to stderr
		if out != "" {
			t.Errorf("expected empty output, got %q", out)
		}
	})
}

// Tests for Init spinner configuration
func TestInit(t *testing.T) {
	t.Run("SetsVerboseSpinner", func(t *testing.T) {
		// Given the default Active spinner
		original := Active
		t.Cleanup(func() { Active = original })

		// When Init is called with verbose true
		Init(true)

		// Then Active should be a *verboseSpinner
		if _, ok := Active.(*verboseSpinner); !ok {
			t.Errorf("expected Active to be *verboseSpinner, got %T", Active)
		}
	})

	t.Run("SetsTermSpinner", func(t *testing.T) {
		// Given Active has been set to verbose
		original := Active
		t.Cleanup(func() { Active = original })
		Active = &verboseSpinner{}

		// When Init is called with verbose false
		Init(false)

		// Then Active should be a *termSpinner
		if _, ok := Active.(*termSpinner); !ok {
			t.Errorf("expected Active to be *termSpinner, got %T", Active)
		}
	})
}
