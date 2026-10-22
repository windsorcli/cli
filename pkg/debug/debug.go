// The debug package controls the debug output of the Windsor CLI.
// It provides one switch for the process that turns the diagnostic log on or off.
// Init sets the switch at startup. Log writes nothing until the switch is on.
// Callers MUST NOT pass secret values to Log, because it does not remove them.

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

// Log writes one line to stderr, with the prefix "[debug]", when debug output is on.
// It formats format and args as fmt.Sprintf does.
func Log(format string, args ...any) {
	if !enabled {
		return
	}
	fmt.Fprintf(os.Stderr, "[debug] "+format+"\n", args...)
}
