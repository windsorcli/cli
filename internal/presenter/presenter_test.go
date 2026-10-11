package presenter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/windsorcli/cli/internal/logging"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Test Setup
// =============================================================================

var errBoom = errors.New("boom")

type recorder struct {
	mu     sync.Mutex
	events []presenter.Event
}

func (r *recorder) Emit(_ context.Context, event presenter.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recorder) kinds() []presenter.Kind {
	var kinds []presenter.Kind
	for _, e := range r.events {
		kinds = append(kinds, e.Kind)
	}
	return kinds
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// screen replays terminal output and returns the visible lines. It applies line feeds,
// carriage returns, cursor-up, and clear-line codes, and drops color codes.
func screen(out string) []string {
	lines := []string{""}
	row, col := 0, 0
	runes := []rune(out)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '\033' && i+1 < len(runes) && runes[i+1] == '[':
			j := i + 2
			for j < len(runes) && (runes[j] < '@' || runes[j] > '~') {
				j++
			}
			param, final := string(runes[i+2:j]), runes[j]
			n, _ := strconv.Atoi(param)
			switch final {
			case 'A':
				if n == 0 {
					n = 1
				}
				row -= n
			case 'K':
				lines[row] = ""
				col = 0
			}
			i = j
		case r == '\n':
			row++
			col = 0
			if row == len(lines) {
				lines = append(lines, "")
			}
		case r == '\r':
			col = 0
		default:
			line := []rune(lines[row])
			if col < len(line) {
				line[col] = r
			} else {
				line = append(line, r)
			}
			lines[row] = string(line)
			col++
		}
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

type fakeTerminal struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeTerminal) Enter() { f.record("enter") }
func (f *fakeTerminal) Leave() { f.record("leave") }

func (f *fakeTerminal) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakeTerminal) String() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.calls, ",")
}

func plainLines(out string) []string {
	return strings.Split(strings.TrimRight(out, "\n"), "\n")
}

func decodeLines(t *testing.T, out string) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, line := range plainLines(out) {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("expected a JSON line, got %q: %v", line, err)
		}
		lines = append(lines, m)
	}
	return lines
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNew(t *testing.T) {
	t.Run("SelectsTheRendererForTheFormat", func(t *testing.T) {
		// Given a writer that is not a terminal
		var buf bytes.Buffer

		// When a presenter is created for each format
		text := presenter.New(logging.FormatText, &buf, presenter.ConsoleOptions{Level: slog.LevelInfo, Interactive: true})
		jsonOut := presenter.New(logging.FormatJSON, &buf, presenter.ConsoleOptions{Level: slog.LevelInfo, Interactive: true})

		// Then text gets the console renderer and json gets the JSON renderer
		if _, ok := text.(*presenter.Console); !ok {
			t.Errorf("expected *Console for text, got %T", text)
		}
		if _, ok := jsonOut.(*presenter.JSON); !ok {
			t.Errorf("expected *JSON for json, got %T", jsonOut)
		}
	})

	t.Run("RendersPlainOutputWhenTheWriterIsNotATerminal", func(t *testing.T) {
		// Given a text presenter on a buffer with interactive output requested
		var buf bytes.Buffer
		p := presenter.New(logging.FormatText, &buf, presenter.ConsoleOptions{Level: slog.LevelInfo, Interactive: true})

		// When an operation runs
		_ = presenter.Track(presenter.NewContext(context.Background(), p), "s", "Applying", func(context.Context) error { return nil })

		// Then the output holds no escape codes
		if strings.Contains(buf.String(), "\033") {
			t.Errorf("expected plain output, got %q", buf.String())
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestFromContext(t *testing.T) {
	t.Run("ReturnsTheInjectedPresenter", func(t *testing.T) {
		// Given a context that carries a presenter
		rec := &recorder{}
		ctx := presenter.NewContext(context.Background(), rec)

		// When the presenter is read back
		got := presenter.FromContext(ctx)

		// Then it is the injected presenter
		if got != rec {
			t.Errorf("expected the injected presenter, got %T", got)
		}
	})

	t.Run("ReturnsDiscardWhenNoneIsInjected", func(t *testing.T) {
		// Given a context without a presenter
		ctx := context.Background()

		// When helpers run and the presenter is read
		err := presenter.Track(ctx, "s", "Applying", func(ctx context.Context) error {
			presenter.Message(ctx, "hello")
			return nil
		})

		// Then the helpers succeed and the presenter is Discard
		if err != nil || presenter.FromContext(ctx) != presenter.Discard {
			t.Errorf("expected Discard and no error, got %T and %v", presenter.FromContext(ctx), err)
		}
	})
}

func TestTrack(t *testing.T) {
	t.Run("EmitsApplyingThenAppliedForOneOperation", func(t *testing.T) {
		// Given a recording presenter
		rec := &recorder{}

		// When a successful operation is tracked
		err := presenter.Track(presenter.NewContext(context.Background(), rec), "terraform:cluster", "Applying cluster", func(context.Context) error { return nil })

		// Then applying and applied share one ID and carry the subject and message
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !reflect.DeepEqual(rec.kinds(), []presenter.Kind{presenter.KindApplying, presenter.KindApplied}) {
			t.Fatalf("unexpected kinds %v", rec.kinds())
		}
		start, end := rec.events[0], rec.events[1]
		if start.ID == "" || start.ID != end.ID || start.ParentID != "" {
			t.Errorf("expected one top-level ID, got %q and %q (parent %q)", start.ID, end.ID, start.ParentID)
		}
		if end.Subject != "terraform:cluster" || end.Message != "Applying cluster" {
			t.Errorf("unexpected end event %+v", end)
		}
	})

	t.Run("SetsTheParentOfNestedOperations", func(t *testing.T) {
		// Given a recording presenter
		rec := &recorder{}
		ctx := presenter.NewContext(context.Background(), rec)

		// When one operation runs inside another
		_ = presenter.Track(ctx, "outer", "Outer", func(ctx context.Context) error {
			return presenter.Track(ctx, "inner", "Inner", func(context.Context) error { return nil })
		})

		// Then the inner events name the outer operation as parent
		outer, inner := rec.events[0], rec.events[1]
		if inner.ParentID != outer.ID || inner.ID == outer.ID {
			t.Errorf("expected inner parent %q, got %q", outer.ID, inner.ParentID)
		}
		if rec.events[2].ParentID != outer.ID {
			t.Errorf("expected inner end parent %q, got %q", outer.ID, rec.events[2].ParentID)
		}
	})

	t.Run("EmitsFailedAndReturnsTheError", func(t *testing.T) {
		// Given a recording presenter
		rec := &recorder{}

		// When a failing operation is tracked
		err := presenter.Track(presenter.NewContext(context.Background(), rec), "s", "Applying", func(context.Context) error { return errBoom })

		// Then failed carries the error and the same error is returned
		if !errors.Is(err, errBoom) {
			t.Errorf("expected errBoom, got %v", err)
		}
		if !reflect.DeepEqual(rec.kinds(), []presenter.Kind{presenter.KindApplying, presenter.KindFailed}) {
			t.Errorf("unexpected kinds %v", rec.kinds())
		}
		if !errors.Is(rec.events[1].Err, errBoom) {
			t.Errorf("expected failed event to carry errBoom, got %v", rec.events[1].Err)
		}
	})
}

func TestBegin(t *testing.T) {
	t.Run("EndsOnlyOnce", func(t *testing.T) {
		// Given an operation started with Begin
		rec := &recorder{}
		_, end := presenter.Begin(presenter.NewContext(context.Background(), rec), "s", "Waiting")

		// When end is called twice
		end(nil)
		end(errBoom)

		// Then only the first call emits an event
		if !reflect.DeepEqual(rec.kinds(), []presenter.Kind{presenter.KindApplying, presenter.KindApplied}) {
			t.Errorf("unexpected kinds %v", rec.kinds())
		}
	})
}

func TestMessageWarnProgress(t *testing.T) {
	t.Run("AttachToTheCurrentOperation", func(t *testing.T) {
		// Given an operation context
		rec := &recorder{}
		ctx, end := presenter.Begin(presenter.NewContext(context.Background(), rec), "s", "Applying")
		id := rec.events[0].ID

		// When a message, a warning, and progress are emitted inside it
		presenter.Message(ctx, "found 3 modules")
		presenter.Warn(ctx, "module is slow")
		presenter.Progress(ctx, "waiting")
		end(nil)

		// Then each event refers to the operation
		msg, warn, prog := rec.events[1], rec.events[2], rec.events[3]
		if msg.Kind != presenter.KindMessage || msg.ParentID != id || msg.Level != slog.LevelInfo {
			t.Errorf("unexpected message %+v", msg)
		}
		if warn.Kind != presenter.KindMessage || warn.ParentID != id || warn.Level != slog.LevelWarn {
			t.Errorf("unexpected warning %+v", warn)
		}
		if prog.Kind != presenter.KindProgress || prog.ID != id || prog.Message != "waiting" {
			t.Errorf("unexpected progress %+v", prog)
		}
	})

	t.Run("StayAtTheTopLevelOutsideAnOperation", func(t *testing.T) {
		// Given a context without an operation
		rec := &recorder{}
		ctx := presenter.NewContext(context.Background(), rec)

		// When a message and progress are emitted
		presenter.Message(ctx, "hello")
		presenter.Progress(ctx, "waiting")

		// Then the message has no parent and progress emits nothing
		if len(rec.events) != 1 || rec.events[0].ParentID != "" {
			t.Errorf("expected one top-level message, got %+v", rec.events)
		}
	})
}

func TestConsole_Plain(t *testing.T) {
	t.Run("PrintsATreeOfOperations", func(t *testing.T) {
		// Given a plain console
		var buf bytes.Buffer
		c := presenter.NewConsole(&buf, presenter.ConsoleOptions{Level: slog.LevelInfo})
		ctx := presenter.NewContext(context.Background(), c)

		// When nested operations, a message, and a warning run and one step fails
		_ = presenter.Track(ctx, "tf", "Applying terraform", func(ctx context.Context) error {
			_ = presenter.Track(ctx, "tf:network", "network", func(context.Context) error { return nil })
			presenter.Message(ctx, "found 2 modules")
			presenter.Warn(ctx, "module is slow")
			return presenter.Track(ctx, "tf:cluster", "cluster", func(context.Context) error { return errBoom })
		})

		// Then each event prints one line, indented under its parent
		want := []string{
			"● Applying terraform",
			"  ● network",
			"  ✔ network",
			"  found 2 modules",
			"  ⚠ module is slow",
			"  ● cluster",
			"  ✗ cluster",
			"✗ Applying terraform",
		}
		if got := plainLines(buf.String()); !reflect.DeepEqual(got, want) {
			t.Errorf("expected\n%s\ngot\n%s", strings.Join(want, "\n"), strings.Join(got, "\n"))
		}
	})

	t.Run("IndentsEveryLineOfAMultiLineWarning", func(t *testing.T) {
		// Given a plain console
		var buf bytes.Buffer
		c := presenter.NewConsole(&buf, presenter.ConsoleOptions{Level: slog.LevelInfo})

		// When a multi-line warning is emitted inside an operation
		_ = presenter.Track(presenter.NewContext(context.Background(), c), "s", "Destroying", func(ctx context.Context) error {
			presenter.Warn(ctx, "destroy output:\nError: bucket not empty")
			return nil
		})

		// Then the second line aligns under the first line's text
		want := []string{"● Destroying", "  ⚠ destroy output:", "    Error: bucket not empty", "✔ Destroying"}
		if got := plainLines(buf.String()); !reflect.DeepEqual(got, want) {
			t.Errorf("expected %q, got %q", want, got)
		}
	})

	t.Run("EndsASuccessfulParentWithoutARepeatedLine", func(t *testing.T) {
		// Given a plain console
		var buf bytes.Buffer
		c := presenter.NewConsole(&buf, presenter.ConsoleOptions{Level: slog.LevelInfo})

		// When a parent with one child succeeds, then a leaf with a message succeeds
		ctx := presenter.NewContext(context.Background(), c)
		_ = presenter.Track(ctx, "tf", "Applying terraform", func(ctx context.Context) error {
			return presenter.Track(ctx, "tf:network", "network", func(context.Context) error { return nil })
		})
		_ = presenter.Track(ctx, "flux", "gitops", func(ctx context.Context) error {
			presenter.Message(ctx, "found 2 kustomizations")
			return nil
		})

		// Then the parent has no end line, and the leaves keep theirs
		want := []string{"● Applying terraform", "  ● network", "  ✔ network", "● gitops", "  found 2 kustomizations", "✔ gitops"}
		if got := plainLines(buf.String()); !reflect.DeepEqual(got, want) {
			t.Errorf("expected\n%s\ngot\n%s", strings.Join(want, "\n"), strings.Join(got, "\n"))
		}
	})

	t.Run("PrintsProgressOncePerChangeWithAHeartbeat", func(t *testing.T) {
		// Given a plain console on a fake clock
		clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		var buf bytes.Buffer
		c := presenter.NewConsole(&buf, presenter.ConsoleOptions{Level: slog.LevelInfo, Now: func() time.Time { return clock }})

		// When an operation repeats progress text, changes it, and repeats it past the heartbeat
		_ = presenter.Track(presenter.NewContext(context.Background(), c), "s", "Waiting", func(ctx context.Context) error {
			presenter.Progress(ctx, "2 pending")
			clock = clock.Add(10 * time.Second)
			presenter.Progress(ctx, "2 pending")
			presenter.Progress(ctx, "1 pending")
			clock = clock.Add(20 * time.Second)
			presenter.Progress(ctx, "1 pending")
			clock = clock.Add(15 * time.Second)
			presenter.Progress(ctx, "1 pending")
			return nil
		})

		// Then each change prints once, an unchanged line reprints after 30 seconds, and lines show elapsed time
		want := []string{"● Waiting", "  2 pending", "  1 pending  10s", "  1 pending  45s", "✔ Waiting"}
		if got := plainLines(buf.String()); !reflect.DeepEqual(got, want) {
			t.Errorf("expected %q, got %q", want, got)
		}
	})

	t.Run("ShowsElapsedTimeFromOneSecond", func(t *testing.T) {
		// Given a plain console
		var buf bytes.Buffer
		c := presenter.NewConsole(&buf, presenter.ConsoleOptions{Level: slog.LevelInfo})
		ctx := context.Background()

		// When operations end after different durations
		for id, d := range map[string]time.Duration{"a": 400 * time.Millisecond, "b": 14 * time.Second, "c": 125 * time.Second, "d": 3725 * time.Second} {
			c.Emit(ctx, presenter.Event{Kind: presenter.KindApplying, ID: id, Message: id})
			c.Emit(ctx, presenter.Event{Kind: presenter.KindApplied, ID: id, Duration: d})
		}

		// Then the end lines show seconds, minutes, and hours, and omit times below one second
		out := buf.String()
		for _, want := range []string{"✔ a\n", "✔ b  14s\n", "✔ c  2m05s\n", "✔ d  1h02m\n"} {
			if !strings.Contains(out, want) {
				t.Errorf("expected %q in %q", want, out)
			}
		}
	})

	t.Run("IgnoresEventsForUnknownOperations", func(t *testing.T) {
		// Given a plain console
		var buf bytes.Buffer
		c := presenter.NewConsole(&buf, presenter.ConsoleOptions{Level: slog.LevelInfo})
		ctx := context.Background()

		// When an end and a progress event arrive for an operation that never started
		c.Emit(ctx, presenter.Event{Kind: presenter.KindApplied, ID: "missing"})
		c.Emit(ctx, presenter.Event{Kind: presenter.KindProgress, ID: "missing", Message: "x"})

		// Then nothing is printed
		if buf.Len() != 0 {
			t.Errorf("expected no output, got %q", buf.String())
		}
	})

	t.Run("WritesLogRecordsAtOrAboveTheLevel", func(t *testing.T) {
		// Given a logger that routes into a console at info level
		var buf bytes.Buffer
		c := presenter.NewConsole(&buf, presenter.ConsoleOptions{Level: slog.LevelInfo})
		logger := slog.New(presenter.NewLogHandler(c, slog.LevelDebug))

		// When records at debug and warn level are logged
		logger.Debug("hidden")
		logger.Warn("slow apply", "component", "cluster")

		// Then only the warn record is written
		out := buf.String()
		if strings.Contains(out, "hidden") || !strings.Contains(out, "slow apply") || !strings.Contains(out, "component=cluster") {
			t.Errorf("unexpected output %q", out)
		}
	})
}

func TestConsole_Interactive(t *testing.T) {
	t.Run("LeavesOnlyFinishedLinesOnScreen", func(t *testing.T) {
		// Given an interactive console
		var buf syncBuffer
		c := presenter.NewConsole(&buf, presenter.ConsoleOptions{Level: slog.LevelInfo, Interactive: true})
		ctx := presenter.NewContext(context.Background(), c)

		// When nested operations run to completion
		_ = presenter.Track(ctx, "tf", "Applying terraform", func(ctx context.Context) error {
			_ = presenter.Track(ctx, "tf:network", "network", func(context.Context) error { return nil })
			return presenter.Track(ctx, "tf:cluster", "cluster", func(context.Context) error { return nil })
		})

		// Then the screen holds the tree with no live lines left and no repeated parent line
		want := []string{"● Applying terraform", "  ✔ network", "  ✔ cluster"}
		if got := screen(buf.String()); !reflect.DeepEqual(got, want) {
			t.Errorf("expected\n%s\ngot\n%s", strings.Join(want, "\n"), strings.Join(got, "\n"))
		}
	})

	t.Run("ShowsTheActiveLeafWithItsProgress", func(t *testing.T) {
		// Given an interactive console with a running operation
		var buf syncBuffer
		c := presenter.NewConsole(&buf, presenter.ConsoleOptions{Level: slog.LevelInfo, Interactive: true})
		ctx, end := presenter.Begin(presenter.NewContext(context.Background(), c), "s", "gitops")
		defer end(nil)

		// When the operation reports progress and a message
		presenter.Progress(ctx, "waiting on 2 kustomizations")
		presenter.Message(ctx, "found 2 kustomizations")

		// Then the message prints above a live line that shows the progress
		got := screen(buf.String())
		if len(got) != 3 || got[0] != "● gitops" || got[1] != "  found 2 kustomizations" {
			t.Fatalf("unexpected screen %q", got)
		}
		if !strings.HasSuffix(got[2], " gitops  waiting on 2 kustomizations") {
			t.Errorf("expected live line with progress, got %q", got[2])
		}
	})

	t.Run("CountsElapsedTimeOnTheLiveLine", func(t *testing.T) {
		// Given an interactive console on a fake clock with a running operation
		var mu sync.Mutex
		clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		now := func() time.Time { mu.Lock(); defer mu.Unlock(); return clock }
		advance := func(d time.Duration) { mu.Lock(); defer mu.Unlock(); clock = clock.Add(d) }
		var buf syncBuffer
		c := presenter.NewConsole(&buf, presenter.ConsoleOptions{Interactive: true, Now: now})
		ctx, end := presenter.Begin(presenter.NewContext(context.Background(), c), "s", "gitops")
		defer end(nil)
		before := screen(buf.String())

		// When time passes and progress redraws the line
		advance(14 * time.Second)
		presenter.Progress(ctx, "waiting")
		seconds := screen(buf.String())
		advance(111 * time.Second)
		presenter.Progress(ctx, "still waiting")
		minutes := screen(buf.String())

		// Then the line has no counter under a second and then counts seconds and minutes
		if !strings.HasSuffix(before[0], " gitops") {
			t.Errorf("expected no counter at start, got %q", before)
		}
		if !strings.HasSuffix(seconds[0], " gitops  14s  waiting") {
			t.Errorf("expected 14s, got %q", seconds)
		}
		if !strings.HasSuffix(minutes[0], " gitops  2m05s  still waiting") {
			t.Errorf("expected 2m05s, got %q", minutes)
		}
	})

	t.Run("TruncatesLiveLinesToTheWidth", func(t *testing.T) {
		// Given an interactive console 20 columns wide
		var buf syncBuffer
		c := presenter.NewConsole(&buf, presenter.ConsoleOptions{Interactive: true, Width: func() int { return 20 }})

		// When an operation with a long message starts
		_, end := presenter.Begin(presenter.NewContext(context.Background(), c), "s", "a message much longer than twenty columns")
		defer end(nil)

		// Then the live line fits within the width
		got := screen(buf.String())
		if len(got) != 1 || utf8.RuneCountInString(got[0]) >= 20 {
			t.Errorf("expected one line under 20 columns, got %q", got)
		}
	})

	t.Run("AnimatesTheSpinnerWhileActive", func(t *testing.T) {
		// Given an interactive console with a running operation
		var buf syncBuffer
		c := presenter.NewConsole(&buf, presenter.ConsoleOptions{Interactive: true})
		_, end := presenter.Begin(presenter.NewContext(context.Background(), c), "s", "waiting")

		// When time passes
		time.Sleep(350 * time.Millisecond)
		end(nil)

		// Then later frames were drawn and the live line is gone
		if !strings.Contains(buf.String(), "⠙") {
			t.Errorf("expected a second spinner frame in %q", buf.String())
		}
		if got := screen(buf.String()); !reflect.DeepEqual(got, []string{"✔ waiting"}) {
			t.Errorf("expected only the end line, got %q", got)
		}
	})
}

func TestConsole_Lifecycle(t *testing.T) {
	t.Run("ReportsWhenOperationsAreActive", func(t *testing.T) {
		// Given a console that records activity changes
		var changes []bool
		c := presenter.NewConsole(&bytes.Buffer{}, presenter.ConsoleOptions{OnActive: func(active bool) { changes = append(changes, active) }})
		ctx := presenter.NewContext(context.Background(), c)

		// When nested operations run twice in a row
		for i := 0; i < 2; i++ {
			_ = presenter.Track(ctx, "outer", "Outer", func(ctx context.Context) error {
				return presenter.Track(ctx, "inner", "Inner", func(context.Context) error { return nil })
			})
		}

		// Then activity starts and ends once per outer operation
		if !reflect.DeepEqual(changes, []bool{true, false, true, false}) {
			t.Errorf("unexpected activity changes %v", changes)
		}
	})

	t.Run("PausesTheLiveRegionForAnotherWriter", func(t *testing.T) {
		// Given an interactive console with a running operation
		var buf syncBuffer
		c := presenter.NewConsole(&buf, presenter.ConsoleOptions{Interactive: true})
		_, end := presenter.Begin(presenter.NewContext(context.Background(), c), "s", "configuring DNS")

		// When the console pauses twice, another writer prints, and one resume follows
		c.Pause()
		c.Pause()
		_, _ = buf.Write([]byte("Password:\n"))
		c.Resume()
		paused := screen(buf.String())
		c.Resume()
		resumed := screen(buf.String())
		end(nil)

		// Then the live line stays hidden until the last resume and the prompt is never erased
		if !reflect.DeepEqual(paused, []string{"Password:"}) {
			t.Errorf("expected only the prompt while paused, got %q", paused)
		}
		if len(resumed) != 2 || resumed[0] != "Password:" || !strings.HasSuffix(resumed[1], " configuring DNS") {
			t.Errorf("expected the prompt and the live line, got %q", resumed)
		}
		if got := screen(buf.String()); !reflect.DeepEqual(got, []string{"Password:", "✔ configuring DNS"}) {
			t.Errorf("expected the prompt and the end line, got %q", got)
		}
	})

	t.Run("EndsAnUnmatchedPauseWhenTheNextOperationStarts", func(t *testing.T) {
		// Given an interactive console that a caller paused without resuming
		var buf syncBuffer
		c := presenter.NewConsole(&buf, presenter.ConsoleOptions{Interactive: true})
		ctx := presenter.NewContext(context.Background(), c)
		_ = presenter.Track(ctx, "s", "configuring DNS", func(context.Context) error {
			c.Pause()
			return nil
		})

		// When the next operation starts
		_, end := presenter.Begin(ctx, "s", "applying cluster")
		defer end(nil)

		// Then its live line is drawn
		got := screen(buf.String())
		if len(got) != 2 || !strings.HasSuffix(got[1], " applying cluster") {
			t.Errorf("expected the end line and a live line, got %q", got)
		}
	})

	t.Run("IgnoresAResumeWithoutAPause", func(t *testing.T) {
		// Given an interactive console with nothing running
		var buf syncBuffer
		c := presenter.NewConsole(&buf, presenter.ConsoleOptions{Interactive: true})

		// When resume is called without a pause
		c.Resume()

		// Then nothing is written
		if buf.String() != "" {
			t.Errorf("expected no output, got %q", buf.String())
		}
	})
}

func TestConsole_Terminal(t *testing.T) {
	t.Run("EntersOnlyWhileTheLiveRegionIsShown", func(t *testing.T) {
		// Given an interactive console with a recording terminal
		term := &fakeTerminal{}
		c := presenter.NewConsole(&syncBuffer{}, presenter.ConsoleOptions{Interactive: true, Terminal: term})

		// When an operation starts, pauses, resumes, and ends
		_, end := presenter.Begin(presenter.NewContext(context.Background(), c), "s", "applying")
		c.Pause()
		c.Resume()
		end(nil)

		// Then the terminal enters and leaves around each visible stretch
		if term.String() != "enter,leave,enter,leave" {
			t.Errorf("unexpected terminal calls %q", term.String())
		}
	})

	t.Run("LeavesForTheRestOfTheRunAfterClose", func(t *testing.T) {
		// Given an interactive console with a running operation
		term := &fakeTerminal{}
		var buf syncBuffer
		c := presenter.NewConsole(&buf, presenter.ConsoleOptions{Interactive: true, Terminal: term})
		ctx := presenter.NewContext(context.Background(), c)
		_, end := presenter.Begin(ctx, "s", "first")

		// When the console closes and a second operation runs
		c.Close()
		end(nil)
		_ = presenter.Track(ctx, "s", "second", func(context.Context) error { return nil })

		// Then the terminal leaves once and never enters again, and output continues
		if term.String() != "enter,leave" {
			t.Errorf("unexpected terminal calls %q", term.String())
		}
		if got := screen(buf.String()); !reflect.DeepEqual(got, []string{"✔ first", "✔ second"}) {
			t.Errorf("expected both end lines, got %q", got)
		}
	})

	t.Run("NeverEntersInPlainMode", func(t *testing.T) {
		// Given a plain console with a recording terminal
		term := &fakeTerminal{}
		c := presenter.NewConsole(&bytes.Buffer{}, presenter.ConsoleOptions{Terminal: term})

		// When an operation runs
		_ = presenter.Track(presenter.NewContext(context.Background(), c), "s", "applying", func(context.Context) error { return nil })

		// Then the terminal is never touched
		if term.String() != "" {
			t.Errorf("expected no terminal calls, got %q", term.String())
		}
	})

	t.Run("IgnoresInterruptsInPlainMode", func(t *testing.T) {
		// Given a plain console
		c := presenter.NewConsole(&bytes.Buffer{}, presenter.ConsoleOptions{})

		// When interrupts are watched and the watch stops twice
		stop := c.WatchInterrupts(nil)
		stop()
		stop()

		// Then nothing panics and nothing is written
	})
}

func TestJSON_Emit(t *testing.T) {
	t.Run("WritesOneLinePerEvent", func(t *testing.T) {
		// Given a JSON presenter
		var buf bytes.Buffer
		j := presenter.NewJSON(&buf)

		// When an operation with a warning is tracked
		_ = presenter.Track(presenter.NewContext(context.Background(), j), "terraform:cluster", "Applying cluster", func(ctx context.Context) error {
			presenter.Warn(ctx, "slow")
			return nil
		})

		// Then each event is one JSON line with its kind, ID, parent, and level
		lines := decodeLines(t, buf.String())
		if len(lines) != 3 || lines[0]["kind"] != "applying" || lines[1]["kind"] != "message" || lines[2]["kind"] != "applied" {
			t.Fatalf("unexpected lines %v", lines)
		}
		if lines[0]["id"] == nil || lines[1]["parent_id"] != lines[0]["id"] || lines[1]["level"] != "WARN" {
			t.Errorf("unexpected lines %v", lines)
		}
		if lines[0]["subject"] != "terraform:cluster" || lines[0]["time"] == nil {
			t.Errorf("unexpected first line %v", lines[0])
		}
	})

	t.Run("WritesTheDurationOfFinishedOperations", func(t *testing.T) {
		// Given a JSON presenter
		var buf bytes.Buffer
		j := presenter.NewJSON(&buf)

		// When an applied event carries a duration
		j.Emit(context.Background(), presenter.Event{Kind: presenter.KindApplied, ID: "1", Duration: 1500 * time.Millisecond})

		// Then the line holds the duration in milliseconds
		if got := decodeLines(t, buf.String())[0]["duration_ms"]; got != float64(1500) {
			t.Errorf("expected 1500, got %v", got)
		}
	})

	t.Run("WritesCodedErrorFields", func(t *testing.T) {
		// Given a JSON presenter and a coded error
		var buf bytes.Buffer
		j := presenter.NewJSON(&buf)
		we := werror.New("TERRAFORM-002", "apply failed", "Run windsor plan to see the change.", errBoom)

		// When a failed event carries the error under a frame
		j.Emit(context.Background(), presenter.Event{Kind: presenter.KindFailed, Err: werror.Wrap(we, "applying")})

		// Then the error object holds the code, category, message, and remediation
		got := decodeLines(t, buf.String())[0]["error"]
		want := map[string]any{"code": "TERRAFORM-002", "category": "TERRAFORM", "message": "apply failed", "remediation": "Run windsor plan to see the change."}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("WritesUntypedErrorMessage", func(t *testing.T) {
		// Given a JSON presenter
		var buf bytes.Buffer
		j := presenter.NewJSON(&buf)

		// When a failed event carries an untyped error
		j.Emit(context.Background(), presenter.Event{Kind: presenter.KindFailed, Err: errBoom})

		// Then the error object holds only the message
		got := decodeLines(t, buf.String())[0]["error"]
		if !reflect.DeepEqual(got, map[string]any{"message": "boom"}) {
			t.Errorf("unexpected error object %v", got)
		}
	})

	t.Run("WritesLogRecordsWithLevelAndNestedAttrs", func(t *testing.T) {
		// Given a logger with a group and attrs that routes into a JSON presenter
		var buf bytes.Buffer
		logger := slog.New(presenter.NewLogHandler(presenter.NewJSON(&buf), slog.LevelDebug)).
			With("run", "r1").WithGroup("terraform").With("component", "cluster")

		// When a record with a duration and a coded error attribute is logged
		logger.Warn("slow apply", "seconds", 3, "elapsed", 3*time.Second, "error", werror.New("TERRAFORM-001", "slow", "", nil))

		// Then the line holds the level, message, and nested attrs
		line := decodeLines(t, buf.String())[0]
		if line["kind"] != "log" || line["level"] != "WARN" || line["message"] != "slow apply" {
			t.Fatalf("unexpected line %v", line)
		}
		want := map[string]any{
			"run": "r1",
			"terraform": map[string]any{
				"component": "cluster",
				"seconds":   float64(3),
				"elapsed":   "3s",
				"error":     map[string]any{"code": "TERRAFORM-001", "category": "TERRAFORM", "message": "slow"},
			},
		}
		if !reflect.DeepEqual(line["attrs"], want) {
			t.Errorf("expected attrs %v, got %v", want, line["attrs"])
		}
	})
}

func TestLogHandler(t *testing.T) {
	t.Run("DropsRecordsBelowTheLevel", func(t *testing.T) {
		// Given a log handler at info level
		rec := &recorder{}
		logger := slog.New(presenter.NewLogHandler(rec, slog.LevelInfo))

		// When a debug record is logged
		logger.Debug("hidden")

		// Then no event is emitted
		if len(rec.events) != 0 {
			t.Errorf("expected no events, got %v", rec.kinds())
		}
	})

	t.Run("AttachesRecordsToTheCurrentOperation", func(t *testing.T) {
		// Given a logger and an operation context
		rec := &recorder{}
		logger := slog.New(presenter.NewLogHandler(rec, slog.LevelInfo))
		ctx, end := presenter.Begin(presenter.NewContext(context.Background(), rec), "s", "Applying")
		defer end(nil)

		// When a record is logged with the operation context
		logger.InfoContext(ctx, "applied")

		// Then the log event names the operation as parent
		if got := rec.events[1]; got.Kind != presenter.KindLog || got.ParentID != rec.events[0].ID {
			t.Errorf("unexpected log event %+v", got)
		}
	})

	t.Run("IgnoresAnEmptyGroupName", func(t *testing.T) {
		// Given a log handler with an empty group name
		rec := &recorder{}
		logger := slog.New(presenter.NewLogHandler(rec, slog.LevelInfo).WithGroup(""))

		// When a record with an attribute is logged
		logger.Info("applied", "component", "cluster")

		// Then the attribute stays at the top level
		var keys []string
		rec.events[0].Record.Attrs(func(a slog.Attr) bool {
			keys = append(keys, a.Key)
			return true
		})
		if !reflect.DeepEqual(keys, []string{"component"}) {
			t.Errorf("expected top-level component, got %v", keys)
		}
	})
}
