// The presenter package is the single seam between business code and terminal output.
// It provides one Event envelope, a Presenter port, and console and JSON renderers.
// Business code emits operations, messages, and progress, and never writes to a stream directly.
// The context carries the presenter and the current operation, so nested events find their parent.

package presenter

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/term"

	"github.com/windsorcli/cli/internal/logging"
)

// =============================================================================
// Constants
// =============================================================================

// Kind names the type of an Event.
type Kind string

const (
	// KindApplying marks the start of an operation.
	KindApplying Kind = "applying"
	// KindApplied marks the successful end of an operation.
	KindApplied Kind = "applied"
	// KindFailed marks the failed end of an operation.
	KindFailed Kind = "failed"
	// KindProgress replaces the status text of a running operation.
	KindProgress Kind = "progress"
	// KindMessage is a narrative line for the user, at info or warn level.
	KindMessage Kind = "message"
	// KindLog carries a structured log record.
	KindLog Kind = "log"
	// KindInputNeeded is reserved for interactive input requests.
	KindInputNeeded Kind = "input_needed"
)

// =============================================================================
// Types
// =============================================================================

// Event is one entry in the output stream. Kind selects which optional fields are set.
type Event struct {
	Kind     Kind
	ID       string
	ParentID string
	Subject  string
	Message  string
	Level    slog.Level
	Duration time.Duration
	Err      error
	Record   *slog.Record
}

// opKey is the context key for the ID of the current operation.
type opKey struct{}

// presenterKey is the context key for the presenter.
type presenterKey struct{}

// discard is a Presenter that drops every event.
type discard struct{}

// =============================================================================
// Interfaces
// =============================================================================

// Presenter receives every event that business code emits.
type Presenter interface {
	Emit(ctx context.Context, event Event)
}

// =============================================================================
// Constructor
// =============================================================================

// nextID is the source of operation IDs, unique within the process.
var nextID atomic.Uint64

// Discard is a Presenter that drops every event. FromContext returns it when ctx has none.
var Discard Presenter = discard{}

// New returns the Presenter for format that writes to w. For text output, opts configures the
// console. Interactive stays on only when w is a terminal that accepts ANSI codes, and then the
// console hides the cursor and stdin echo while it animates. JSON output ignores opts.
func New(format logging.Format, w io.Writer, opts ConsoleOptions) Presenter {
	if format == logging.FormatJSON {
		return NewJSON(w)
	}
	f, ok := w.(*os.File)
	if !opts.Interactive || !ok || !term.IsTerminal(int(f.Fd())) || enableVirtualTerminal(f) != nil {
		opts.Interactive = false
		return NewConsole(w, opts)
	}
	var in *os.File
	if term.IsTerminal(int(os.Stdin.Fd())) {
		in = os.Stdin
	}
	opts.Terminal = newTTYModes(in, w)
	opts.Width = func() int {
		width, _, err := term.GetSize(int(f.Fd()))
		if err != nil {
			return 0
		}
		return width
	}
	return NewConsole(w, opts)
}

// =============================================================================
// Public Methods
// =============================================================================

// NewContext returns a copy of ctx that carries p.
func NewContext(ctx context.Context, p Presenter) context.Context {
	return context.WithValue(ctx, presenterKey{}, p)
}

// FromContext returns the presenter in ctx, or Discard when ctx has none.
func FromContext(ctx context.Context) Presenter {
	if p, ok := ctx.Value(presenterKey{}).(Presenter); ok && p != nil {
		return p
	}
	return Discard
}

// Begin emits KindApplying for a new operation under the operation in ctx. It returns a context
// that carries the new operation and an end function that emits KindApplied for a nil error or
// KindFailed otherwise, with the elapsed time. Calls to end after the first do nothing.
func Begin(ctx context.Context, subject, message string) (context.Context, func(error)) {
	p := FromContext(ctx)
	id := strconv.FormatUint(nextID.Add(1), 10)
	parent := currentOp(ctx)
	start := time.Now()
	p.Emit(ctx, Event{Kind: KindApplying, ID: id, ParentID: parent, Subject: subject, Message: message})
	var once sync.Once
	end := func(err error) {
		once.Do(func() {
			event := Event{Kind: KindApplied, ID: id, ParentID: parent, Subject: subject, Message: message, Duration: time.Since(start)}
			if err != nil {
				event.Kind = KindFailed
				event.Err = err
			}
			p.Emit(ctx, event)
		})
	}
	return context.WithValue(ctx, opKey{}, id), end
}

// Track runs fn as an operation that Begin starts and ends. It passes fn the context of the
// new operation and returns the error from fn unchanged.
func Track(ctx context.Context, subject, message string, fn func(context.Context) error) error {
	opCtx, end := Begin(ctx, subject, message)
	err := fn(opCtx)
	end(err)
	return err
}

// Message emits an info-level narrative line under the operation in ctx.
func Message(ctx context.Context, text string) {
	FromContext(ctx).Emit(ctx, Event{Kind: KindMessage, ParentID: currentOp(ctx), Message: text, Level: slog.LevelInfo})
}

// Warn emits a warn-level narrative line under the operation in ctx.
func Warn(ctx context.Context, text string) {
	FromContext(ctx).Emit(ctx, Event{Kind: KindMessage, ParentID: currentOp(ctx), Message: text, Level: slog.LevelWarn})
}

// Progress replaces the status text of the operation in ctx. It does nothing outside an operation.
func Progress(ctx context.Context, text string) {
	id := currentOp(ctx)
	if id == "" {
		return
	}
	FromContext(ctx).Emit(ctx, Event{Kind: KindProgress, ID: id, Message: text})
}

// Emit drops event.
func (discard) Emit(context.Context, Event) {}

// =============================================================================
// Helpers
// =============================================================================

// currentOp returns the ID of the operation in ctx, or an empty string when there is none.
func currentOp(ctx context.Context) string {
	id, _ := ctx.Value(opKey{}).(string)
	return id
}
