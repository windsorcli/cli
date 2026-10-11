// The Terminal is the switch between the normal terminal and the live display mode.
// It provides cursor hiding and input echo control while the console animates.
// Echo stays off only while the live region is on screen, so typed keys cannot move the cursor.
// Signals stay enabled, so Ctrl-C still interrupts the process.

package presenter

import (
	"fmt"
	"io"
	"os"
)

// =============================================================================
// Constants
// =============================================================================

const (
	ansiHideCursor = "\033[?25l"
	ansiShowCursor = "\033[?25h"
)

// =============================================================================
// Types
// =============================================================================

// ttyModes is the Terminal for a real terminal.
type ttyModes struct {
	in      *os.File
	out     io.Writer
	restore func()
}

// =============================================================================
// Interfaces
// =============================================================================

// Terminal enters and leaves the live display mode.
type Terminal interface {
	Enter()
	Leave()
}

// =============================================================================
// Constructor
// =============================================================================

// newTTYModes returns a Terminal that hides the cursor on out and turns off echo on in.
// A nil in leaves echo unchanged.
func newTTYModes(in *os.File, out io.Writer) *ttyModes {
	return &ttyModes{in: in, out: out}
}

// =============================================================================
// Public Methods
// =============================================================================

// Enter hides the cursor and turns off input echo. Echo stays on when the input is not a console.
func (t *ttyModes) Enter() {
	fmt.Fprint(t.out, ansiHideCursor)
	if t.in == nil || t.restore != nil {
		return
	}
	if restore, err := disableEcho(t.in); err == nil {
		t.restore = restore
	}
}

// Leave restores input echo and shows the cursor.
func (t *ttyModes) Leave() {
	if t.restore != nil {
		t.restore()
		t.restore = nil
	}
	fmt.Fprint(t.out, ansiShowCursor)
}
