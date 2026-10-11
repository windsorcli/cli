// The terminal_linux file holds the termios request codes for Linux.

package presenter

import "golang.org/x/sys/unix"

// =============================================================================
// Constants
// =============================================================================

const (
	ioctlGetTermios = unix.TCGETS
	ioctlSetTermios = unix.TCSETS
)

// =============================================================================
// Helpers
// =============================================================================

// flushInput discards input that the terminal behind fd has queued but no program has read.
func flushInput(fd int) {
	_ = unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIFLUSH)
}
