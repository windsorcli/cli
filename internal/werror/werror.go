// The WindsorError is a coded, user-facing error with a remediation and a wrapped cause.
// It provides New for coded errors, Wrap for named breadcrumb frames, and chain predicates.
// Codes take the form DOMAIN-NNN, and the domain prefix is the error category.
// Breadcrumbs reads frame messages from the chain structure and never parses error text.

package werror

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// =============================================================================
// Constants
// =============================================================================

// docsBaseURL is the documentation page prefix that DocsURL appends the lowercased code to.
const docsBaseURL = "https://www.windsorcli.dev/errors/"

// =============================================================================
// Types
// =============================================================================

// WindsorError is an error with a stable code, a user-facing message, and a remediation.
type WindsorError struct {
	Code        string
	Category    string
	Message     string
	Remediation string
	DocsURL     string
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

// New returns a WindsorError for code. It derives Category from the code's domain prefix
// and DocsURL from the code. Cause may be nil when the error starts a new condition.
func New(code, message, remediation string, cause error) *WindsorError {
	category, _, _ := strings.Cut(code, "-")
	return &WindsorError{
		Code:        code,
		Category:    category,
		Message:     message,
		Remediation: remediation,
		DocsURL:     docsBaseURL + strings.ToLower(code),
		Cause:       cause,
	}
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

// LogValue returns the code, category, and message as structured log attributes.
func (e *WindsorError) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("code", e.Code),
		slog.String("category", e.Category),
		slog.String("message", e.Message),
	)
}

// Error returns the frame message followed by the wrapped error text.
func (f *frame) Error() string {
	return f.msg + ": " + f.err.Error()
}

// Unwrap returns the wrapped error.
func (f *frame) Unwrap() error {
	return f.err
}

// IsCode reports whether any WindsorError in the chain of err has the given code.
func IsCode(err error, code string) bool {
	return anyWindsorError(err, func(we *WindsorError) bool { return we.Code == code })
}

// IsCategory reports whether any WindsorError in the chain of err has the given category.
func IsCategory(err error, category string) bool {
	return anyWindsorError(err, func(we *WindsorError) bool { return we.Category == category })
}

// Breadcrumbs returns one message per Wrap frame and WindsorError in the chain, outermost first.
// It ends with the text of the innermost error when that error is neither a frame nor a
// WindsorError. Intermediate fmt.Errorf wraps add no entry, because their text repeats the chain.
func Breadcrumbs(err error) []string {
	var crumbs []string
	for err != nil {
		switch e := err.(type) {
		case *frame:
			crumbs = append(crumbs, e.msg)
		case *WindsorError:
			crumbs = append(crumbs, e.Message)
		default:
			if errors.Unwrap(err) == nil {
				crumbs = append(crumbs, err.Error())
			}
		}
		err = errors.Unwrap(err)
	}
	return crumbs
}

// =============================================================================
// Private Methods
// =============================================================================

// anyWindsorError reports whether match returns true for any WindsorError in the chain of err.
func anyWindsorError(err error, match func(*WindsorError) bool) bool {
	for err != nil {
		var we *WindsorError
		if !errors.As(err, &we) {
			return false
		}
		if match(we) {
			return true
		}
		err = we.Cause
	}
	return false
}
