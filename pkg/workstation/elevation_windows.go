//go:build windows
// +build windows

package workstation

import (
	"fmt"

	"github.com/windsorcli/cli/pkg/runtime/shell"
	"golang.org/x/sys/windows"
)

// canElevateNonInteractivelyImpl reports whether the current process is running as Administrator.
// Windows has no "elevate later silently" model — either the process started elevated or it did
// not — so this is the cross-platform analog of unix's "root or cached sudo". The shell argument
// is accepted for API parity with the unix variant and is unused here.
func canElevateNonInteractivelyImpl(_ shell.Shell) bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// PreflightConfigureNetwork errors when the current process is not running as Administrator.
// The privileged Windows cmdlets used to install the host route + DNS resolver require an
// elevated process; running them from a standard PowerShell surfaces access-denied errors that
// are confusing to operators. Failing fast here surfaces the actionable remediation up front.
func PreflightConfigureNetwork() error {
	if windows.GetCurrentProcessToken().IsElevated() {
		return nil
	}
	return fmt.Errorf("'windsor configure network' must be run from an Administrator PowerShell. Right-click PowerShell → Run as Administrator, then re-run")
}
