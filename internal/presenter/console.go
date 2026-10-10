// The Console is the Presenter for human-readable terminal output.
// It provides a tree of operations: finished steps stay on screen and nest under their parent.
// In plain mode it prints one line per event, which suits logs and CI output.
// In interactive mode it animates the active leaf operations in a live region below the tree.

package presenter

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
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
}

// Console renders events as an indented tree of lines.
type Console struct {
	mu     sync.Mutex
	w      io.Writer
	opts   ConsoleOptions
	logs   slog.Handler
	ops    map[string]*consoleOp
	order  []*consoleOp
	drawn  int
	frame  int
	stopCh chan struct{}
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

// =============================================================================
// Private Methods
// =============================================================================

// start registers an operation and prints its header in plain mode.
func (c *Console) start(event Event) {
	op := &consoleOp{id: event.ID, message: event.Message}
	if parent := c.ops[event.ParentID]; parent != nil {
		op.depth = parent.depth + 1
		parent.children++
		c.printHeader(parent)
	}
	c.ops[op.id] = op
	c.order = append(c.order, op)
	if !c.opts.Interactive {
		c.printHeader(op)
		return
	}
	c.redraw()
}

// finish removes an operation and prints its end line with the elapsed time.
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
func (c *Console) message(event Event) {
	depth := 0
	if parent := c.ops[event.ParentID]; parent != nil {
		depth = parent.depth + 1
		c.printHeader(parent)
	}
	text := event.Message
	if event.Level >= slog.LevelWarn {
		text = c.color(ansiYellow, "⚠") + " " + text
	}
	line := indent(depth) + text
	c.print(func() { fmt.Fprintln(c.w, line) })
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
func (c *Console) redraw() {
	if !c.opts.Interactive {
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
		line := indent(op.depth) + c.color(ansiGreen, frame) + " " + op.message
		if op.progress != "" {
			line += "  " + c.color(ansiDim, op.progress)
		}
		b.WriteString(truncate(line, width) + "\n")
		c.drawn++
	}
	fmt.Fprint(c.w, b.String())
}

// syncTicker starts the animation ticker when the live region has lines and stops it otherwise.
func (c *Console) syncTicker() {
	active := c.opts.Interactive && c.drawn > 0
	if active && c.stopCh == nil {
		c.stopCh = make(chan struct{})
		go c.tick(c.stopCh)
	}
	if !active && c.stopCh != nil {
		close(c.stopCh)
		c.stopCh = nil
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
	d = d.Round(time.Second)
	text := fmt.Sprintf("%ds", int(d.Seconds()))
	if d >= time.Minute {
		text = fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return "  " + c.color(ansiDim, text)
}

// =============================================================================
// Helpers
// =============================================================================

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
