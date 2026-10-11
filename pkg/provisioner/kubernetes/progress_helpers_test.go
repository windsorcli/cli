package kubernetes

import (
	"context"
	"sync"
	"testing"

	"github.com/windsorcli/cli/internal/presenter"
)

// =============================================================================
// Test Helpers
// =============================================================================

// progressRecorder records presenter events for assertions on operator-facing progress.
type progressRecorder struct {
	mu     sync.Mutex
	events []presenter.Event
}

func (r *progressRecorder) Emit(_ context.Context, e presenter.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

// started returns the messages of the operations that started.
func (r *progressRecorder) started() []string {
	return r.messages(presenter.KindApplying)
}

// progress returns the progress messages.
func (r *progressRecorder) progress() []string {
	return r.messages(presenter.KindProgress)
}

func (r *progressRecorder) messages(kind presenter.Kind) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, e := range r.events {
		if e.Kind == kind {
			out = append(out, e.Message)
		}
	}
	return out
}

// recordProgress returns a context inside an open operation whose presenter records every event.
func recordProgress(t *testing.T) (context.Context, *progressRecorder) {
	t.Helper()
	rec := &progressRecorder{}
	ctx, end := presenter.Begin(presenter.NewContext(context.Background(), rec), "test", "test operation")
	t.Cleanup(func() { end(nil) })
	return ctx, rec
}
