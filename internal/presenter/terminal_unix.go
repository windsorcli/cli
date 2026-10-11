//go:build darwin || linux

// The terminal_unix file holds the termios and signal code for macOS and Linux.
// It provides echo control on a terminal file and the re-raise of an interrupt signal.
// It changes only the ECHO flag, so line input and signal keys keep working.
// Re-raising runs the default action of the signal after the caller stops its own handler.

package presenter

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// =============================================================================
// Helpers
// =============================================================================

// disableEcho turns off input echo on f and returns a function that restores the previous mode.
// The restore first discards keys typed while echo was off, so they never reach the next program.
func disableEcho(f *os.File) (func(), error) {
	fd := int(f.Fd())
	state, err := unix.IoctlGetTermios(fd, ioctlGetTermios)
	if err != nil {
		return nil, err
	}
	original := *state
	state.Lflag &^= unix.ECHO
	if err := unix.IoctlSetTermios(fd, ioctlSetTermios, state); err != nil {
		return nil, err
	}
	return func() {
		flushInput(fd)
		_ = unix.IoctlSetTermios(fd, ioctlSetTermios, &original)
	}, nil
}

// enableVirtualTerminal does nothing, because macOS and Linux terminals accept ANSI codes.
func enableVirtualTerminal(*os.File) error {
	return nil
}

// raise sends sig to the current process, so its default action runs.
func raise(sig os.Signal) {
	if s, ok := sig.(syscall.Signal); ok {
		_ = syscall.Kill(os.Getpid(), s)
	}
}
