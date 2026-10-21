//go:build windows
// +build windows

// The Windows elevation functions check whether the process runs as Administrator.
// The network preflight check fails when the process does not run as Administrator.

package workstation

import (
	"github.com/windsorcli/cli/internal/werror"
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

// PreflightConfigureNetwork returns an error when the process does not run as Administrator.
// The Windows cmdlets that install the host route and the DNS rule need an elevated process.
// Without this check, they fail with access-denied errors that do not tell the operator what to do.
// The error has a remediation hint that tells the operator to start an Administrator PowerShell.
func PreflightConfigureNetwork() error {
	if windows.GetCurrentProcessToken().IsElevated() {
		return nil
	}
	return werror.New("'windsor configure network' must run in an Administrator PowerShell", "Right-click PowerShell, select Run as Administrator, then run the command again.", nil)
}
