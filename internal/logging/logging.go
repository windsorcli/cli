// The logging package is the structured logging contract for Windsor.
// It provides slog handlers for console and JSON output and a typed context key for the logger.
// Callers log through the *slog.Logger in their context and never construct a handler.
// FromContext never returns nil, so code without an injected logger stays silent.

package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	charmlog "github.com/charmbracelet/log"
)

// =============================================================================
// Constants
// =============================================================================

// Format names an output format that the --format flag selects.
type Format string

const (
	// FormatText selects human-readable console output.
	FormatText Format = "text"
	// FormatJSON selects one JSON object per line.
	FormatJSON Format = "json"
)

// =============================================================================
// Types
// =============================================================================

// ctxKey is the context key for the injected logger.
type ctxKey struct{}

// resolvingHandler resolves slog.LogValuer attributes before it passes them to the next handler.
type resolvingHandler struct {
	next slog.Handler
}

// =============================================================================
// Constructor
// =============================================================================

// NewHandler returns the slog handler for format that writes records at level or above to w.
// FormatText uses a charmbracelet/log console handler, which does not resolve slog.LogValuer
// attributes itself. FormatJSON uses slog.JSONHandler.
func NewHandler(w io.Writer, format Format, level slog.Level) slog.Handler {
	if format == FormatJSON {
		return slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	}
	return &resolvingHandler{next: charmlog.NewWithOptions(w, charmlog.Options{Level: charmlog.Level(level)})}
}

// =============================================================================
// Public Methods
// =============================================================================

// ParseFormat returns the Format named by s, or an error that lists the valid formats.
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case FormatText, FormatJSON:
		return Format(s), nil
	}
	return "", fmt.Errorf("unknown format %q: use %q or %q", s, FormatText, FormatJSON)
}

// NewContext returns a copy of ctx that carries logger.
func NewContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, logger)
}

// FromContext returns the logger in ctx, or a logger that discards every record when ctx has none.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return slog.New(slog.DiscardHandler)
}

// Enabled reports whether the next handler accepts records at level.
func (h *resolvingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// Handle passes a copy of r with every attribute resolved to the next handler.
func (h *resolvingHandler) Handle(ctx context.Context, r slog.Record) error {
	resolved := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		resolved.AddAttrs(resolveAttr(a))
		return true
	})
	return h.next.Handle(ctx, resolved)
}

// WithAttrs returns a handler whose next handler carries the resolved attrs.
func (h *resolvingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	resolved := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		resolved[i] = resolveAttr(a)
	}
	return &resolvingHandler{next: h.next.WithAttrs(resolved)}
}

// WithGroup returns a handler whose next handler opens the named group.
func (h *resolvingHandler) WithGroup(name string) slog.Handler {
	return &resolvingHandler{next: h.next.WithGroup(name)}
}

// =============================================================================
// Helpers
// =============================================================================

// resolveAttr returns a with its value resolved, so a slog.LogValuer becomes its logged value.
func resolveAttr(a slog.Attr) slog.Attr {
	a.Value = a.Value.Resolve()
	return a
}
