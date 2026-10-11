// The terminal_windows file holds the console mode and signal code for Windows.
// It provides echo control on a console input handle and virtual terminal processing on output.
// Windows cannot re-raise a console interrupt, so raise exits with the interrupt status.
// The caller restores the terminal before raise runs.

package presenter

import (
	"os"

	"golang.org/x/sys/windows"
)

// =============================================================================
// Helpers
// =============================================================================

// disableEcho turns off input echo on the console behind f and returns a function that restores
// the previous mode. The restore first discards keys typed while echo was off.
func disableEcho(f *os.File) (func(), error) {
	h := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return nil, err
	}
	if err := windows.SetConsoleMode(h, mode&^windows.ENABLE_ECHO_INPUT); err != nil {
		return nil, err
	}
	return func() {
		_ = windows.FlushConsoleInputBuffer(h)
		_ = windows.SetConsoleMode(h, mode)
	}, nil
}

// enableVirtualTerminal turns on ANSI escape processing for the console behind f.
func enableVirtualTerminal(f *os.File) error {
	h := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return err
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
}

// raise exits with status 130, the conventional status for an interrupted process.
func raise(os.Signal) {
	os.Exit(130)
}
