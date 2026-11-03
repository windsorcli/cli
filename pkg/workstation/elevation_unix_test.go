//go:build !windows
// +build !windows

package workstation

import (
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestCanElevateNonInteractivelyImpl_Unix(t *testing.T) {
	originalGeteuid := geteuidFunc
	t.Cleanup(func() { geteuidFunc = originalGeteuid })

	t.Run("TrueWhenRoot", func(t *testing.T) {
		// Given the process appears to be running as root (CI typical, or 'sudo windsor up')
		geteuidFunc = func() int { return 0 }
		mockShell := shell.NewMockShell()
		sudoChecked := false
		mockShell.ExecSilentFunc = func(command string, _ ...string) (string, error) {
			if command == "sudo" {
				sudoChecked = true
			}
			return "", nil
		}

		// Then the helper short-circuits without probing sudo
		if !canElevateNonInteractivelyImpl(mockShell) {
			t.Errorf("expected true when running as root")
		}
		if sudoChecked {
			t.Errorf("expected sudo -n probe to be skipped when already root")
		}
	})

	t.Run("TrueWhenPasswordlessSudoCached", func(t *testing.T) {
		// Given a non-root process with cached passwordless sudo credentials
		geteuidFunc = func() int { return 1000 }
		mockShell := shell.NewMockShell()
		mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "sudo" && len(args) >= 2 && args[0] == "-n" && args[1] == "true" {
				return "", nil
			}
			return "", fmt.Errorf("unexpected command")
		}

		// Then the helper reports it can elevate without prompting
		if !canElevateNonInteractivelyImpl(mockShell) {
			t.Errorf("expected true when sudo -n true succeeds")
		}
	})

	t.Run("FalseWhenNeitherRootNorCachedSudo", func(t *testing.T) {
		// Given a non-root process with no cached sudo credentials
		geteuidFunc = func() int { return 1000 }
		mockShell := shell.NewMockShell()
		mockShell.ExecSilentFunc = func(_ string, _ ...string) (string, error) {
			return "", fmt.Errorf("a password is required")
		}

		// Then the helper reports it cannot elevate without prompting
		if canElevateNonInteractivelyImpl(mockShell) {
			t.Errorf("expected false when not root and sudo -n true fails")
		}
	})
}

func TestPreflightConfigureNetwork_Unix(t *testing.T) {
	t.Run("ReturnsNilSoPerOpSudoHandlesElevation", func(t *testing.T) {
		// Given a unix host

		// When running the preflight check
		err := PreflightConfigureNetwork()

		// Then it passes, because each privileged step runs its own sudo
		if err != nil {
			t.Errorf("expected nil on unix, got %v", err)
		}
	})
}
