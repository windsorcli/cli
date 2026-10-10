// The presenter package is the single seam between business code and terminal output.
// It provides one Event envelope, a Presenter port, and console and JSON renderers.
// Business code emits operations, messages, and progress, and never writes to a stream directly.
// Begin and Track carry the current operation in the context, so nested events find their parent.

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

// New returns the Presenter for format that writes to w. Log records below level are dropped.
// The console renderer is interactive when w is a terminal and interactive is true.
func New(format logging.Format, w io.Writer, level slog.Level, interactive bool) Presenter {
	if format == logging.FormatJSON {
		return NewJSON(w)
	}
	opts := ConsoleOptions{Level: level}
	if f, ok := w.(*os.File); ok && interactive && term.IsTerminal(int(f.Fd())) {
		opts.Interactive = true
		opts.Width = func() int {
			width, _, err := term.GetSize(int(f.Fd()))
			if err != nil {
				return 0
			}
			return width
		}
	}
	return NewConsole(w, opts)
}

// =============================================================================
// Public Methods
// =============================================================================

// Begin emits KindApplying for a new operation under the operation in ctx. It returns a context
// that carries the new operation and an end function that emits KindApplied for a nil error or
// KindFailed otherwise, with the elapsed time. Calls to end after the first do nothing.
func Begin(ctx context.Context, p Presenter, subject, message string) (context.Context, func(error)) {
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
func Track(ctx context.Context, p Presenter, subject, message string, fn func(context.Context) error) error {
	opCtx, end := Begin(ctx, p, subject, message)
	err := fn(opCtx)
	end(err)
	return err
}

// Message emits an info-level narrative line under the operation in ctx.
func Message(ctx context.Context, p Presenter, text string) {
	p.Emit(ctx, Event{Kind: KindMessage, ParentID: currentOp(ctx), Message: text, Level: slog.LevelInfo})
}

// Warn emits a warn-level narrative line under the operation in ctx.
func Warn(ctx context.Context, p Presenter, text string) {
	p.Emit(ctx, Event{Kind: KindMessage, ParentID: currentOp(ctx), Message: text, Level: slog.LevelWarn})
}

// Progress replaces the status text of the operation in ctx. It does nothing outside an operation.
func Progress(ctx context.Context, p Presenter, text string) {
	id := currentOp(ctx)
	if id == "" {
		return
	}
	p.Emit(ctx, Event{Kind: KindProgress, ID: id, Message: text})
}

// =============================================================================
// Helpers
// =============================================================================

// currentOp returns the ID of the operation in ctx, or an empty string when there is none.
func currentOp(ctx context.Context) string {
	id, _ := ctx.Value(opKey{}).(string)
	return id
}
