// The elevation functions of the workstation package report whether the process can run privileged commands.
// The OS-specific files give the implementation. Tests replace it through SetCanElevateNonInteractivelyForTest.

package workstation

import "github.com/windsorcli/cli/pkg/runtime/shell"

// canElevateNonInteractivelyFunc is the elevation probe. SetCanElevateNonInteractivelyForTest replaces it.
var canElevateNonInteractivelyFunc = canElevateNonInteractivelyImpl

// canElevateNonInteractively reports whether the process can run privileged commands without a prompt.
func canElevateNonInteractively(sh shell.Shell) bool {
	return canElevateNonInteractivelyFunc(sh)
}

// SetCanElevateNonInteractivelyForTest replaces the elevation probe with fn and returns a function that restores it.
// Tests on each OS use it to simulate either answer.
func SetCanElevateNonInteractivelyForTest(fn func(shell.Shell) bool) func() {
	original := canElevateNonInteractivelyFunc
	canElevateNonInteractivelyFunc = fn
	return func() { canElevateNonInteractivelyFunc = original }
}
