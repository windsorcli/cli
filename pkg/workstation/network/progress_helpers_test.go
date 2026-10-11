package network

import (
	"context"
	"sync"

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

// steps returns each operation as "kind:message", in order, ignoring warnings and progress.
func (r *progressRecorder) steps() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, e := range r.events {
		if e.Kind == presenter.KindApplying || e.Kind == presenter.KindApplied || e.Kind == presenter.KindFailed {
			out = append(out, string(e.Kind)+":"+e.Message)
		}
	}
	return out
}
