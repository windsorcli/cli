package workstation

import "github.com/windsorcli/cli/pkg/runtime/shell"

// canElevateNonInteractivelyFunc is the swappable seam tests use to simulate the "can elevate"
// answer without touching OS-specific internals (geteuid on unix, TokenElevation on windows).
// Default points at the OS-specific impl in elevation_unix.go / elevation_windows.go.
var canElevateNonInteractivelyFunc = canElevateNonInteractivelyImpl

// canElevateNonInteractively dispatches through the swappable seam so callers see the real
// behavior in production and tests can drive specific outcomes regardless of host OS.
func canElevateNonInteractively(sh shell.Shell) bool {
	return canElevateNonInteractivelyFunc(sh)
}

// SetCanElevateNonInteractivelyForTest replaces the package's elevation probe for the duration
// of a test and returns a restore function. Used by tests across platforms to simulate
// "can elevate" / "cannot elevate" without depending on the OS-specific implementation.
func SetCanElevateNonInteractivelyForTest(fn func(shell.Shell) bool) func() {
	original := canElevateNonInteractivelyFunc
	canElevateNonInteractivelyFunc = fn
	return func() { canElevateNonInteractivelyFunc = original }
}
