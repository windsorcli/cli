// The Console is the Presenter for human-readable terminal output.
// In plain mode it writes one logfmt line per event, which suits verbose runs and CI logs.
// In interactive mode it draws a tree of operations: finished steps stay on screen under their
// parent. It animates the active leaf operations in a live region below the tree,
// and its Terminal hides the cursor and input echo while that region is on screen.

package presenter

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/windsorcli/cli/internal/logging"
)

// =============================================================================
// Constants
// =============================================================================

// spinnerFrames are the animation frames for an active operation.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// frameInterval is the time between animation frames.
const frameInterval = 100 * time.Millisecond

// progressHeartbeat is how often plain mode reprints unchanged progress text.
const progressHeartbeat = 30 * time.Second

// maxLiveLines is the most lines the live region draws on a terminal of any height.
const maxLiveLines = 10

const (
	ansiGreen     = "\033[32m"
	ansiRed       = "\033[31m"
	ansiYellow    = "\033[33m"
	ansiDim       = "\033[2m"
	ansiReset     = "\033[0m"
	ansiLineUp    = "\033[1A"
	ansiClearLine = "\033[2K"
)

// =============================================================================
// Types
// =============================================================================

// ConsoleOptions configures a Console.
type ConsoleOptions struct {
	// Level is the lowest log level that the console writes.
	Level slog.Level
	// Interactive enables the animated live region and colors.
	Interactive bool
	// Width returns the terminal width for line truncation, or 0 for no truncation.
	Width func() int
	// Height returns the terminal height that caps the live region, or 0 for the default cap.
	Height func() int
	// Terminal enters the live display mode while the live region is on screen. Nil skips it.
	Terminal Terminal
	// Now returns the current time for elapsed counters. Nil uses time.Now.
	Now func() time.Time
}

// Console renders events as an indented tree of lines, or as logfmt lines in plain mode.
type Console struct {
	mu      sync.Mutex
	w       io.Writer
	opts    ConsoleOptions
	logs    slog.Handler
	steps   slog.Handler
	ops     map[string]*consoleOp
	order   []*consoleOp
	drawn   int
	frame   int
	paused  int
	stopCh  chan struct{}
	entered bool
	closed  bool
}

// consoleOp is the render state of one running operation.
type consoleOp struct {
	id        string
	depth     int
	message   string
	path      string
	progress  string
	printed   bool
	children  int
	lastShown string
	started   time.Time
	hadChild  bool
	shownAt   time.Time
}

// =============================================================================
// Constructor
// =============================================================================

// NewConsole returns a Console that writes to w.
func NewConsole(w io.Writer, opts ConsoleOptions) *Console {
	return &Console{
		w:     w,
		opts:  opts,
		logs:  logging.NewHandler(w, logging.FormatText, opts.Level),
		steps: logging.NewHandler(w, logging.FormatText, slog.LevelInfo),
		ops:   map[string]*consoleOp{},
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// FormatDuration returns d in whole seconds below one minute ("14s"), in minutes and seconds
// below one hour ("2m05s"), and in hours and minutes from one hour ("1h02m").
func FormatDuration(d time.Duration) string {
	d = d.Truncate(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

// Emit renders event. In plain mode it writes one logfmt line, as emitPlain describes. In
// interactive mode an operation header prints when its first child appears, and end lines show
// the elapsed time. Events that refer to an unknown operation render at the top level.
func (c *Console) Emit(ctx context.Context, event Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.opts.Interactive {
		c.emitPlain(ctx, event)
		return
	}
	switch event.Kind {
	case KindApplying:
		c.start(event)
	case KindApplied, KindFailed:
		c.finish(event)
	case KindProgress:
		c.progress(event)
	case KindMessage:
		c.message(event)
	case KindLog:
		if event.Record != nil && c.logs.Enabled(ctx, event.Record.Level) {
			c.print(func() { _ = c.logs.Handle(ctx, *event.Record) })
		}
	}
	c.syncTicker()
}

// Pause erases the live region and stops the animation until a matching Resume. Calls nest. On the
// first Pause it prints the header of each active step, so the other writer's output, such as a
// password prompt, appears under the step that asked for it.
func (c *Console) Pause() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.paused == 0 {
		c.clear()
		c.paused++
		for _, op := range c.order {
			if op.children == 0 && !op.printed {
				op.printed = true
				fmt.Fprintln(c.w, indent(op.depth)+"● "+op.message)
			}
		}
	} else {
		c.paused++
	}
	c.syncTicker()
}

// Resume redraws the live region and restarts the animation when the last Pause ends.
func (c *Console) Resume() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.paused == 0 {
		return
	}
	c.paused--
	c.redraw()
	c.syncTicker()
}

// Close stops the animation and leaves the live display mode for the rest of the run. The
// console keeps rendering events afterward, with the cursor and input echo left on.
func (c *Console) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.syncTicker()
}

// WatchInterrupts restores the terminal when the process receives an interrupt or termination
// signal. After the restore it stops watching and re-raises the signal, unless guarded reports
// that another handler owns it. The returned function stops the watch.
func (c *Console) WatchInterrupts(guarded func() bool) func() {
	if !c.opts.Interactive {
		return func() {}
	}
	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-done:
		case sig := <-sigCh:
			c.Close()
			signal.Stop(sigCh)
			if guarded == nil || !guarded() {
				raise(sig)
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			signal.Stop(sigCh)
			close(done)
		})
	}
}

// =============================================================================
// Private Methods
// =============================================================================

// emitPlain writes event as one logfmt line. Each step line names the full step path, so lines
// from concurrent steps stay readable when they interleave. Its status field is started, running,
// done, or failed. Step and message lines show at info level or above, whatever the log level.
// Progress lines repeat only when the text changes, or after progressHeartbeat.
func (c *Console) emitPlain(ctx context.Context, event Event) {
	switch event.Kind {
	case KindApplying:
		op := &consoleOp{id: event.ID, message: event.Message, path: event.Message, started: c.now()}
		if parent := c.ops[event.ParentID]; parent != nil {
			op.path = parent.path + " › " + event.Message
		}
		c.ops[op.id] = op
		c.writeStep(ctx, slog.LevelInfo, op.path, slog.String("status", "started"))
	case KindApplied, KindFailed:
		op := c.ops[event.ID]
		if op == nil {
			return
		}
		delete(c.ops, op.id)
		level, attrs := slog.LevelInfo, []slog.Attr{slog.String("status", "done")}
		if event.Kind == KindFailed {
			level, attrs = slog.LevelError, []slog.Attr{slog.String("status", "failed")}
		}
		attrs = append(attrs, elapsedAttrs(event.Duration)...)
		if event.Err != nil {
			attrs = append(attrs, slog.Any("error", event.Err))
		}
		c.writeStep(ctx, level, op.path, attrs...)
	case KindProgress:
		op := c.ops[event.ID]
		if op == nil {
			return
		}
		now := c.now()
		if event.Message == op.lastShown && now.Sub(op.shownAt) < progressHeartbeat {
			return
		}
		op.lastShown, op.shownAt = event.Message, now
		attrs := append([]slog.Attr{slog.String("status", "running"), slog.String("progress", event.Message)}, elapsedAttrs(now.Sub(op.started))...)
		c.writeStep(ctx, slog.LevelInfo, op.path, attrs...)
	case KindMessage:
		var attrs []slog.Attr
		if parent := c.ops[event.ParentID]; parent != nil {
			attrs = append(attrs, slog.String("step", parent.path))
		}
		c.writeStep(ctx, event.Level, event.Message, attrs...)
	case KindLog:
		if event.Record != nil && c.logs.Enabled(ctx, event.Record.Level) {
			_ = c.logs.Handle(ctx, *event.Record)
		}
	}
}

// writeStep writes one logfmt record at level with msg and attrs, stamped with the console clock.
func (c *Console) writeStep(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	record := slog.NewRecord(c.now(), level, msg, 0)
	record.AddAttrs(attrs...)
	_ = c.steps.Handle(ctx, record)
}

// start registers an operation and redraws the live region.
func (c *Console) start(event Event) {
	op := &consoleOp{id: event.ID, message: event.Message, started: c.now()}
	if parent := c.ops[event.ParentID]; parent != nil {
		op.depth = parent.depth + 1
		parent.children++
		parent.hadChild = true
		c.printHeader(parent)
	}
	c.ops[op.id] = op
	c.order = append(c.order, op)
	c.redraw()
}

// finish removes an operation and prints its end line with the elapsed time. An operation that
// had child operations prints no end line on success, because its header and its children's end
// lines already show the result. It still prints one on failure.
func (c *Console) finish(event Event) {
	op := c.ops[event.ID]
	if op == nil {
		return
	}
	delete(c.ops, op.id)
	for i, o := range c.order {
		if o == op {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
	if parent := c.ops[event.ParentID]; parent != nil {
		parent.children--
	}
	if op.hadChild && event.Kind == KindApplied {
		c.redraw()
		return
	}
	icon := c.color(ansiGreen, "✔")
	if event.Kind == KindFailed {
		icon = c.color(ansiRed, "✗")
	}
	line := indent(op.depth) + icon + " " + op.message + c.elapsed(event.Duration)
	c.print(func() { fmt.Fprintln(c.w, line) })
}

// progress stores the status text of an operation and redraws the live region.
func (c *Console) progress(event Event) {
	op := c.ops[event.ID]
	if op == nil {
		return
	}
	op.progress = event.Message
	c.redraw()
}

// message prints a narrative line under its parent operation, with a marker for warnings.
// Each further line of a multi-line message is indented to align under the first line's text.
func (c *Console) message(event Event) {
	depth := 0
	if parent := c.ops[event.ParentID]; parent != nil {
		depth = parent.depth + 1
		c.printHeader(parent)
	}
	prefix, cont := "", indent(depth)
	if event.Level >= slog.LevelWarn {
		prefix, cont = c.color(ansiYellow, "⚠")+" ", indent(depth)+"  "
	}
	lines := strings.Split(event.Message, "\n")
	text := indent(depth) + prefix + lines[0]
	for _, l := range lines[1:] {
		text += "\n" + cont + l
	}
	c.print(func() { fmt.Fprintln(c.w, text) })
}

// printHeader prints the header line of op once.
func (c *Console) printHeader(op *consoleOp) {
	if op.printed {
		return
	}
	op.printed = true
	line := indent(op.depth) + "● " + op.message
	c.print(func() { fmt.Fprintln(c.w, line) })
}

// print runs write between a clear and a redraw of the live region.
func (c *Console) print(write func()) {
	c.clear()
	write()
	c.redraw()
}

// clear erases the live region and leaves the cursor where the region began.
func (c *Console) clear() {
	if c.drawn == 0 {
		return
	}
	var b strings.Builder
	for i := 0; i < c.drawn; i++ {
		b.WriteString(ansiLineUp + ansiClearLine)
	}
	b.WriteString("\r")
	fmt.Fprint(c.w, b.String())
	c.drawn = 0
}

// redraw replaces the live region with one line per active leaf operation in interactive mode.
// Each line shows the time its operation has run so far. When more leaves are active than the
// region holds, the last line counts the rest. It draws nothing while paused.
func (c *Console) redraw() {
	if !c.opts.Interactive || c.paused > 0 {
		return
	}
	c.clear()
	frame := spinnerFrames[c.frame%len(spinnerFrames)]
	width := 0
	if c.opts.Width != nil {
		width = c.opts.Width()
	}
	var leaves []*consoleOp
	for _, op := range c.order {
		if op.children == 0 {
			leaves = append(leaves, op)
		}
	}
	shown, hidden := leaves, 0
	if limit := c.liveLimit(); len(leaves) > limit {
		shown, hidden = leaves[:limit-1], len(leaves)-(limit-1)
	}
	var b strings.Builder
	for _, op := range shown {
		line := indent(op.depth) + c.color(ansiGreen, frame) + " " + op.message + c.elapsed(c.now().Sub(op.started))
		if op.progress != "" {
			line += "  " + c.color(ansiDim, op.progress)
		}
		b.WriteString(truncate(line, width) + "\n")
		c.drawn++
	}
	if hidden > 0 {
		line := indent(leaves[len(shown)].depth) + c.color(ansiDim, fmt.Sprintf("… and %d more", hidden))
		b.WriteString(truncate(line, width) + "\n")
		c.drawn++
	}
	fmt.Fprint(c.w, b.String())
}

// liveLimit returns how many lines the live region may draw: maxLiveLines, or two fewer than the
// terminal height when that is smaller, and never fewer than two.
func (c *Console) liveLimit() int {
	limit := maxLiveLines
	if c.opts.Height != nil {
		if h := c.opts.Height() - 2; h > 0 && h < limit {
			limit = h
		}
	}
	if limit < 2 {
		limit = 2
	}
	return limit
}

// syncTicker starts the animation ticker and enters the live display mode when the live region
// has lines, and stops both otherwise. After Close it never enters the display mode again.
func (c *Console) syncTicker() {
	active := c.opts.Interactive && c.drawn > 0
	if active && c.stopCh == nil && !c.closed {
		c.stopCh = make(chan struct{})
		go c.tick(c.stopCh)
	}
	if (!active || c.closed) && c.stopCh != nil {
		close(c.stopCh)
		c.stopCh = nil
	}
	if c.opts.Terminal == nil {
		return
	}
	if active && !c.entered && !c.closed {
		c.opts.Terminal.Enter()
		c.entered = true
	}
	if (!active || c.closed) && c.entered {
		c.opts.Terminal.Leave()
		c.entered = false
	}
}

// tick advances the animation frame until stop closes.
func (c *Console) tick(stop chan struct{}) {
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			c.mu.Lock()
			c.frame++
			c.redraw()
			c.mu.Unlock()
		}
	}
}

// color wraps text in an ANSI color in interactive mode and returns it unchanged otherwise.
func (c *Console) color(code, text string) string {
	if !c.opts.Interactive {
		return text
	}
	return code + text + ansiReset
}

// elapsed returns the formatted duration suffix, or an empty string below one second.
func (c *Console) elapsed(d time.Duration) string {
	if d < time.Second {
		return ""
	}
	return "  " + c.color(ansiDim, FormatDuration(d))
}

// now returns the current time from the configured clock.
func (c *Console) now() time.Time {
	if c.opts.Now != nil {
		return c.opts.Now()
	}
	return time.Now()
}

// =============================================================================
// Helpers
// =============================================================================

// elapsedAttrs returns an elapsed attribute for d, or none below one second.
func elapsedAttrs(d time.Duration) []slog.Attr {
	if d < time.Second {
		return nil
	}
	return []slog.Attr{slog.String("elapsed", FormatDuration(d))}
}

// indent returns two spaces per depth level.
func indent(depth int) string {
	return strings.Repeat("  ", depth)
}

// truncate shortens line to width visible columns, keeping ANSI codes intact. A width of 0 or
// less keeps the whole line.
func truncate(line string, width int) string {
	if width <= 0 {
		return line
	}
	var b strings.Builder
	visible := 0
	inEscape := false
	for _, r := range line {
		switch {
		case r == '\033':
			inEscape = true
		case inEscape:
			if r == 'm' {
				inEscape = false
			}
		default:
			if visible >= width-1 {
				return b.String() + ansiReset
			}
			visible++
		}
		b.WriteRune(r)
	}
	return b.String()
}
