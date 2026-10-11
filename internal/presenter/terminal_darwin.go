// The terminal_darwin file holds the termios request codes for macOS.

package presenter

import "golang.org/x/sys/unix"

// =============================================================================
// Constants
// =============================================================================

const (
	ioctlGetTermios = unix.TIOCGETA
	ioctlSetTermios = unix.TIOCSETA
)

// flushRead is FREAD from <sys/fcntl.h>, which selects the input queue for TIOCFLUSH.
const flushRead = 1

// =============================================================================
// Helpers
// =============================================================================

// flushInput discards input that the terminal behind fd has queued but no program has read.
func flushInput(fd int) {
	_ = unix.IoctlSetPointerInt(fd, unix.TIOCFLUSH, flushRead)
}
