// The WindsorError is a user-facing error with a remediation and a wrapped cause.
// It provides New for user-facing errors and Wrap for named breadcrumb frames.
// Breadcrumbs reads frame messages from the chain structure and never parses error text.

package werror

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// =============================================================================
// Types
// =============================================================================

// WindsorError is an error with a user-facing message and a remediation.
type WindsorError struct {
	Message     string
	Remediation string
	Cause       error
}

// frame is one named breadcrumb in an error chain.
type frame struct {
	msg string
	err error
}

// =============================================================================
// Constructor
// =============================================================================

// New returns a WindsorError. Cause may be nil when the error starts a new condition.
func New(message, remediation string, cause error) *WindsorError {
	return &WindsorError{Message: message, Remediation: remediation, Cause: cause}
}

// Wrap returns err with one named breadcrumb frame added, or nil when err is nil.
// The frame message is the formatted text. errors.Is and errors.As see through the frame.
func Wrap(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return &frame{msg: fmt.Sprintf(format, args...), err: err}
}

// =============================================================================
// Public Methods
// =============================================================================

// Error returns the user-facing message without the cause.
func (e *WindsorError) Error() string {
	return e.Message
}

// Unwrap returns the wrapped cause.
func (e *WindsorError) Unwrap() error {
	return e.Cause
}

// LogValue returns the message and remediation as structured log attributes.
func (e *WindsorError) LogValue() slog.Value {
	attrs := []slog.Attr{slog.String("message", e.Message)}
	if e.Remediation != "" {
		attrs = append(attrs, slog.String("remediation", e.Remediation))
	}
	return slog.GroupValue(attrs...)
}

// Error returns the frame message followed by the wrapped error text.
func (f *frame) Error() string {
	return f.msg + ": " + f.err.Error()
}

// Unwrap returns the wrapped error.
func (f *frame) Unwrap() error {
	return f.err
}

// Breadcrumbs returns one message per Wrap frame and WindsorError in the chain, outermost first.
// It ends with the text of the innermost error when that error is neither a frame nor a
// WindsorError. Intermediate fmt.Errorf wraps add no entry, because their text repeats the chain.
// Each entry has no surrounding whitespace. A message that ends the previous entry adds no entry.
func Breadcrumbs(err error) []string {
	var crumbs []string
	for err != nil {
		switch e := err.(type) {
		case *frame:
			crumbs = appendCrumb(crumbs, e.msg)
		case *WindsorError:
			crumbs = appendCrumb(crumbs, e.Message)
		default:
			if errors.Unwrap(err) == nil {
				crumbs = appendCrumb(crumbs, err.Error())
			}
		}
		err = errors.Unwrap(err)
	}
	return crumbs
}

// =============================================================================
// Helpers
// =============================================================================

// appendCrumb adds msg to crumbs without surrounding whitespace, unless the last entry ends with it.
func appendCrumb(crumbs []string, msg string) []string {
	msg = strings.TrimSpace(msg)
	if n := len(crumbs); n > 0 && strings.HasSuffix(crumbs[n-1], msg) {
		return crumbs
	}
	return append(crumbs, msg)
}
