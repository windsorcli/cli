//go:build !windows
// +build !windows

package workstation

import (
	"os"

	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// geteuidFunc is the seam for the unix-impl tests in elevation_unix_test.go. Cross-platform
// tests use SetCanElevateNonInteractivelyForTest instead.
var geteuidFunc = os.Geteuid

// canElevateNonInteractivelyImpl reports whether the current process can run privileged
// commands without prompting: either it is already root (CI as root, sudo windsor up) or
// passwordless sudo is cached for the current user. Used by Down() to decide between silent
// revert and the operator-facing deferred-work hint.
func canElevateNonInteractivelyImpl(sh shell.Shell) bool {
	if geteuidFunc() == 0 {
		return true
	}
	_, err := sh.ExecSilent("sudo", "-n", "true")
	return err == nil
}

// PreflightConfigureNetwork gates 'windsor configure network' on platforms that need the
// process itself to be elevated. On unix the privileged steps each invoke per-op sudo (cached
// after the first prompt), so the process can start unprivileged and this returns nil.
func PreflightConfigureNetwork() error { return nil }
