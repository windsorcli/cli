// The logging package is the structured logging contract for Windsor.
// It provides slog handlers for logfmt and JSON output and a typed context key for the logger.
// Callers log through the *slog.Logger in their context and never construct a handler.
// FromContext never returns nil, so code without an injected logger stays silent.

package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
)

// =============================================================================
// Constants
// =============================================================================

// Format names an output format that the --format flag selects.
type Format string

const (
	// FormatText selects logfmt text output.
	FormatText Format = "text"
	// FormatJSON selects one JSON object per line.
	FormatJSON Format = "json"
)

// =============================================================================
// Types
// =============================================================================

// ctxKey is the context key for the injected logger.
type ctxKey struct{}

// =============================================================================
// Constructor
// =============================================================================

// NewHandler returns the slog handler for format that writes records at level or above to w.
// FormatText uses slog.TextHandler, which writes logfmt key=value lines. FormatJSON uses
// slog.JSONHandler. Both resolve slog.LogValuer attributes.
func NewHandler(w io.Writer, format Format, level slog.Level) slog.Handler {
	opts := &slog.HandlerOptions{Level: level}
	if format == FormatJSON {
		return slog.NewJSONHandler(w, opts)
	}
	return slog.NewTextHandler(w, opts)
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
