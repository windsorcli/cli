// The Console is the Presenter for human-readable terminal output.
// It provides a tree of operations: finished steps stay on screen and nest under their parent.
// In plain mode it prints one line per event, which suits logs and CI output.
// In interactive mode it animates the active leaf operations in a live region below the tree,
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
	// OnActive runs with true when the first operation starts and false when the last one ends.
	OnActive func(active bool)
	// Terminal enters the live display mode while the live region is on screen. Nil skips it.
	Terminal Terminal
	// Now returns the current time for elapsed counters. Nil uses time.Now.
	Now func() time.Time
}

// Console renders events as an indented tree of lines.
type Console struct {
	mu      sync.Mutex
	w       io.Writer
	opts    ConsoleOptions
	logs    slog.Handler
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
	progress  string
	printed   bool
	children  int
	lastShown string
	started   time.Time
	hadChild  bool
}

// =============================================================================
// Constructor
// =============================================================================

// NewConsole returns a Console that writes to w.
func NewConsole(w io.Writer, opts ConsoleOptions) *Console {
	return &Console{
		w:    w,
		opts: opts,
		logs: logging.NewHandler(w, logging.FormatText, opts.Level),
		ops:  map[string]*consoleOp{},
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Emit renders event. Operation headers print when the operation starts in plain mode, or when
// its first child appears in interactive mode. End lines show the elapsed time. Events that
// refer to an unknown operation render at the top level.
func (c *Console) Emit(ctx context.Context, event Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
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

// Pause erases the live region and stops the animation until a matching Resume. Calls nest.
// Use it before another writer, such as a password prompt, takes the terminal.
func (c *Console) Pause() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.paused == 0 {
		c.clear()
	}
	c.paused++
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

// start registers an operation and prints its header in plain mode. It ends any pause, the
// same as the legacy spinner, because legacy callers pause without a matching Resume.
func (c *Console) start(event Event) {
	c.paused = 0
	op := &consoleOp{id: event.ID, message: event.Message, started: c.now()}
	if parent := c.ops[event.ParentID]; parent != nil {
		op.depth = parent.depth + 1
		parent.children++
		parent.hadChild = true
		c.printHeader(parent)
	}
	c.ops[op.id] = op
	c.order = append(c.order, op)
	if len(c.order) == 1 && c.opts.OnActive != nil {
		c.opts.OnActive(true)
	}
	if !c.opts.Interactive {
		c.printHeader(op)
		return
	}
	c.redraw()
}

// finish removes an operation and prints its end line with the elapsed time. An operation that
// had child operations prints no end line on success, because its header and its children's end
// lines already show the result. It still prints one on failure. It ends any pause, the same
// as start.
func (c *Console) finish(event Event) {
	op := c.ops[event.ID]
	if op == nil {
		return
	}
	c.paused = 0
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
	if len(c.order) == 0 && c.opts.OnActive != nil {
		c.opts.OnActive(false)
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

// progress stores the status text of an operation. Plain mode prints it once per change.
func (c *Console) progress(event Event) {
	op := c.ops[event.ID]
	if op == nil {
		return
	}
	op.progress = event.Message
	if c.opts.Interactive {
		c.redraw()
		return
	}
	if event.Message == op.lastShown {
		return
	}
	op.lastShown = event.Message
	line := indent(op.depth+1) + event.Message
	c.print(func() { fmt.Fprintln(c.w, line) })
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
// Each line shows the time its operation has run so far. It draws nothing while paused.
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
	var b strings.Builder
	for _, op := range c.order {
		if op.children > 0 {
			continue
		}
		line := indent(op.depth) + c.color(ansiGreen, frame) + " " + op.message + c.elapsed(c.now().Sub(op.started))
		if op.progress != "" {
			line += "  " + c.color(ansiDim, op.progress)
		}
		b.WriteString(truncate(line, width) + "\n")
		c.drawn++
	}
	fmt.Fprint(c.w, b.String())
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
	return "  " + c.color(ansiDim, formatDuration(d))
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

// formatDuration returns d in whole seconds below one minute ("14s"), in minutes and seconds
// below one hour ("2m05s"), and in hours and minutes from one hour ("1h02m").
func formatDuration(d time.Duration) string {
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
