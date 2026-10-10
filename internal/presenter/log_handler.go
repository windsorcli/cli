// The LogHandler is the slog handler that routes log records into a Presenter.
// It provides the bridge that makes logs part of the same event stream as progress and errors.
// It filters by level and emits each kept record as a KindLog event.
// Attributes from With and WithGroup are nested into each emitted record.

package presenter

import (
	"context"
	"log/slog"
)

// =============================================================================
// Types
// =============================================================================

// LogHandler emits slog records as KindLog events.
type LogHandler struct {
	p      Presenter
	level  slog.Leveler
	attrs  []slog.Attr
	groups []string
}

// =============================================================================
// Constructor
// =============================================================================

// NewLogHandler returns a handler that emits records at level or above to p.
func NewLogHandler(p Presenter, level slog.Leveler) *LogHandler {
	return &LogHandler{p: p, level: level}
}

// =============================================================================
// Public Methods
// =============================================================================

// Enabled reports whether level is at or above the handler level.
func (h *LogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

// Handle emits a copy of r that holds the handler attrs and the record attrs under open groups.
// The event parent is the operation in ctx.
func (h *LogHandler) Handle(ctx context.Context, r slog.Record) error {
	var own []slog.Attr
	r.Attrs(func(a slog.Attr) bool {
		own = append(own, a)
		return true
	})
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	out.AddAttrs(h.attrs...)
	out.AddAttrs(nest(h.groups, own)...)
	h.p.Emit(ctx, Event{Kind: KindLog, ParentID: currentOp(ctx), Message: r.Message, Record: &out})
	return nil
}

// WithAttrs returns a handler that adds attrs, nested under the open groups, to every record.
func (h *LogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = append(append([]slog.Attr{}, h.attrs...), nest(h.groups, attrs)...)
	return &next
}

// WithGroup returns a handler that nests later attrs under name. An empty name is ignored.
func (h *LogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := *h
	next.groups = append(append([]string{}, h.groups...), name)
	return &next
}

// =============================================================================
// Helpers
// =============================================================================

// nest wraps attrs in one group per name, with the last name innermost.
func nest(groups []string, attrs []slog.Attr) []slog.Attr {
	if len(attrs) == 0 {
		return nil
	}
	for i := len(groups) - 1; i >= 0; i-- {
		attrs = []slog.Attr{{Key: groups[i], Value: slog.GroupValue(attrs...)}}
	}
	return attrs
}
