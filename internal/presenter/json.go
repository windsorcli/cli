// The JSON is the Presenter for machine-readable output.
// It provides one JSON object per event, written as one line to the writer.
// A coded error adds its code, category, and remediation; an untyped error adds only its message.
// Log records add their level and attributes, with groups as nested objects.

package presenter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Types
// =============================================================================

// JSON renders events as newline-delimited JSON objects.
type JSON struct {
	mu  sync.Mutex
	enc *json.Encoder
}

// jsonEvent is the wire format of one event.
type jsonEvent struct {
	Time       time.Time      `json:"time"`
	Kind       Kind           `json:"kind"`
	ID         string         `json:"id,omitempty"`
	ParentID   string         `json:"parent_id,omitempty"`
	Subject    string         `json:"subject,omitempty"`
	Message    string         `json:"message,omitempty"`
	Level      string         `json:"level,omitempty"`
	DurationMS int64          `json:"duration_ms,omitempty"`
	Attrs      map[string]any `json:"attrs,omitempty"`
	Error      *jsonError     `json:"error,omitempty"`
}

// jsonError is the wire format of an event error.
type jsonError struct {
	Code        string `json:"code,omitempty"`
	Category    string `json:"category,omitempty"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

// =============================================================================
// Constructor
// =============================================================================

// NewJSON returns a JSON presenter that writes to w.
func NewJSON(w io.Writer) *JSON {
	return &JSON{enc: json.NewEncoder(w)}
}

// =============================================================================
// Public Methods
// =============================================================================

// Emit writes event as one JSON line. Messages add their level. A log record supplies the time,
// level, message, and attrs.
func (j *JSON) Emit(_ context.Context, event Event) {
	out := jsonEvent{
		Time:       time.Now(),
		Kind:       event.Kind,
		ID:         event.ID,
		ParentID:   event.ParentID,
		Subject:    event.Subject,
		Message:    event.Message,
		DurationMS: event.Duration.Milliseconds(),
		Error:      toJSONError(event.Err),
	}
	if event.Kind == KindMessage {
		out.Level = event.Level.String()
	}
	if r := event.Record; r != nil {
		out.Time = r.Time
		out.Level = r.Level.String()
		out.Message = r.Message
		out.Attrs = map[string]any{}
		r.Attrs(func(a slog.Attr) bool {
			addAttr(out.Attrs, a)
			return true
		})
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	_ = j.enc.Encode(out)
}

// =============================================================================
// Helpers
// =============================================================================

// toJSONError returns the wire format of err, or nil when err is nil.
func toJSONError(err error) *jsonError {
	if err == nil {
		return nil
	}
	var we *werror.WindsorError
	if errors.As(err, &we) {
		return &jsonError{Code: we.Code, Category: we.Category, Message: we.Message, Remediation: we.Remediation}
	}
	return &jsonError{Message: err.Error()}
}

// addAttr stores a in m after it resolves the value. Groups become nested maps that merge with
// an existing map under the same key, and an empty-keyed group merges into m. Values without a
// JSON scalar kind use their string form.
func addAttr(m map[string]any, a slog.Attr) {
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindGroup:
		target := m
		if a.Key != "" {
			existing, ok := m[a.Key].(map[string]any)
			if !ok {
				existing = map[string]any{}
				m[a.Key] = existing
			}
			target = existing
		}
		for _, ga := range v.Group() {
			addAttr(target, ga)
		}
	case slog.KindString, slog.KindInt64, slog.KindUint64, slog.KindFloat64, slog.KindBool:
		m[a.Key] = v.Any()
	default:
		m[a.Key] = v.String()
	}
}
