// The debug package provides centralized debug-output control for the Windsor CLI.
// It gates internal diagnostic logging behind a single process-wide switch.
// Init sets the switch once at startup; Log is a no-op until debug output is enabled.
// Callers must not pass secret values to Log, since output is not scrubbed.

package debug

import (
	"fmt"
	"os"
)

// =============================================================================
// Constructor
// =============================================================================

// enabled tracks whether debug output is turned on for the current process.
var enabled bool

// Init turns debug output on or off for the current run.
func Init(on bool) {
	enabled = on
}

// =============================================================================
// Public Methods
// =============================================================================

// Enabled reports whether debug output is currently turned on.
func Enabled() bool {
	return enabled
}

// Log writes one line to stderr, prefixed "[debug]", when debug output is
// enabled. It is a no-op otherwise. Args follow fmt.Sprintf conventions.
func Log(format string, args ...any) {
	if !enabled {
		return
	}
	fmt.Fprintf(os.Stderr, "[debug] "+format+"\n", args...)
}
