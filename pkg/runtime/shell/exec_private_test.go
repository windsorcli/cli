package shell

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestInterruptGuard_Watch(t *testing.T) {
	setup := func() (*interruptGuard, *syncWriter, func() int) {
		warn := newSyncWriter()
		var mu sync.Mutex
		forwardCount := 0
		g := &interruptGuard{
			forward: func() error {
				mu.Lock()
				forwardCount++
				mu.Unlock()
				return nil
			},
			warn:      warn,
			component: "cluster/aws-eks",
		}
		count := func() int {
			mu.Lock()
			defer mu.Unlock()
			return forwardCount
		}
		return g, warn, count
	}

	runWatch := func(t *testing.T, g *interruptGuard, sigCh chan os.Signal, done chan struct{}) <-chan struct{} {
		t.Helper()
		finished := make(chan struct{})
		go func() {
			g.watch(sigCh, done)
			close(finished)
		}()
		return finished
	}

	waitFinished := func(t *testing.T, finished <-chan struct{}) {
		t.Helper()
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatal("watch did not return after done was closed")
		}
	}

	t.Run("ReturnsWithoutForwardingWhenNoSignalArrives", func(t *testing.T) {
		g, warn, count := setup()
		sigCh := make(chan os.Signal, 1)
		done := make(chan struct{})

		finished := runWatch(t, g, sigCh, done)
		close(done)
		waitFinished(t, finished)

		if count() != 0 {
			t.Errorf("expected forward not to be called, got %d calls", count())
		}
		if warn.String() != "" {
			t.Errorf("expected no warning output, got %q", warn.String())
		}
	})

	t.Run("ForwardsOnlyTheFirstOfSeveralSignals", func(t *testing.T) {
		g, warn, count := setup()
		sigCh := make(chan os.Signal, 4)
		done := make(chan struct{})

		finished := runWatch(t, g, sigCh, done)

		sigCh <- os.Interrupt
		<-warn.written
		sigCh <- os.Interrupt
		<-warn.written
		sigCh <- os.Interrupt
		<-warn.written

		close(done)
		waitFinished(t, finished)

		if count() != 1 {
			t.Errorf("expected forward to be called exactly once, got %d", count())
		}
		lines := strings.Count(warn.String(), "\n")
		if lines != 3 {
			t.Errorf("expected one warning line per signal (3), got %d in %q", lines, warn.String())
		}
		if !strings.Contains(warn.String(), "cluster/aws-eks") {
			t.Errorf("expected warning to mention the component, got %q", warn.String())
		}
	})

	t.Run("SecondWarningIsDistinctFromTheFirst", func(t *testing.T) {
		g, warn, _ := setup()
		sigCh := make(chan os.Signal, 2)
		done := make(chan struct{})

		finished := runWatch(t, g, sigCh, done)

		sigCh <- os.Interrupt
		<-warn.written
		sigCh <- os.Interrupt
		<-warn.written

		close(done)
		waitFinished(t, finished)

		lines := strings.Split(strings.TrimRight(warn.String(), "\n"), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected exactly two warning lines, got %d: %q", len(lines), warn.String())
		}
		if lines[0] == lines[1] {
			t.Errorf("expected a distinct follow-up warning, got the same line twice: %q", lines[0])
		}
		if !strings.Contains(lines[1], "corrupt") {
			t.Errorf("expected the second warning to warn that forcing can corrupt state, got %q", lines[1])
		}
	})

	t.Run("ReturnsPromptlyOnceDoneIsClosed", func(t *testing.T) {
		g, _, _ := setup()
		sigCh := make(chan os.Signal, 1)
		done := make(chan struct{})

		finished := runWatch(t, g, sigCh, done)
		close(done)
		waitFinished(t, finished)
	})

	t.Run("HandsTheTerminalOffAroundEachWarning", func(t *testing.T) {
		// The console redraws its live region on a timer. An unpaused write here would
		// get drawn over by the next frame. Each warning must hand off, then take back.
		var mu sync.Mutex
		var events []string
		presenter.SetDefault(&handoffRecorder{record: func(e string) {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		}})
		t.Cleanup(func() { presenter.SetDefault(nil) })

		warn := newSyncWriter()
		warn.onWrite = func() {
			mu.Lock()
			events = append(events, "write")
			mu.Unlock()
		}
		g := &interruptGuard{
			forward:   func() error { return nil },
			warn:      warn,
			component: "cluster/aws-eks",
		}
		sigCh := make(chan os.Signal, 1)
		done := make(chan struct{})

		finished := runWatch(t, g, sigCh, done)
		sigCh <- os.Interrupt
		<-warn.written
		close(done)
		waitFinished(t, finished)

		mu.Lock()
		got := append([]string(nil), events...)
		mu.Unlock()
		want := []string{"pause", "write", "resume"}
		if !slices.Equal(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})
}

func Test_execError(t *testing.T) {
	cause := errors.New("exit status 1")

	t.Run("KeepsStderrAfterTheCause", func(t *testing.T) {
		// Given a failed command that wrote to stderr
		// When execError wraps the failure
		err := execError(cause, "permission denied")

		// Then the text and the trace keep the stderr, and the cause stays in the chain
		if err.Error() != "command execution failed: exit status 1\npermission denied" {
			t.Errorf("Expected the cause then the stderr, got %q", err.Error())
		}
		if !errors.Is(err, cause) {
			t.Error("Expected the cause to stay in the error chain")
		}
		crumbs := werror.Breadcrumbs(err)
		if len(crumbs) == 0 || !strings.Contains(crumbs[len(crumbs)-1], "permission denied") {
			t.Errorf("Expected the trace to show the stderr, got %v", crumbs)
		}
	})

	t.Run("OmitsEmptyStderr", func(t *testing.T) {
		// Given a failed command that wrote nothing to stderr
		// When execError wraps the failure
		err := execError(cause, "")

		// Then the text ends with the cause
		if err.Error() != "command execution failed: exit status 1" {
			t.Errorf("Expected no trailing stderr, got %q", err.Error())
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

// handoffRecorder is a presenter that records Pause and Resume calls.
type handoffRecorder struct {
	record func(string)
}

func (h *handoffRecorder) Emit(context.Context, presenter.Event) {}

func (h *handoffRecorder) Pause() { h.record("pause") }

func (h *handoffRecorder) Resume() { h.record("resume") }
