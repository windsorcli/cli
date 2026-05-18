//go:build !windows
// +build !windows

package workstation

import (
	"fmt"
	"testing"
)

func TestCanElevateNonInteractivelyImpl_Unix(t *testing.T) {
	// Save and restore the geteuid override around each subtest
	originalGeteuid := geteuidFunc
	t.Cleanup(func() { geteuidFunc = originalGeteuid })

	t.Run("TrueWhenRoot", func(t *testing.T) {
		// Given the process appears to be running as root (CI typical, or 'sudo windsor up')
		geteuidFunc = func() int { return 0 }
		mocks := setupWorkstationMocks(t)
		sudoChecked := false
		mocks.Shell.ExecSilentFunc = func(command string, _ ...string) (string, error) {
			if command == "sudo" {
				sudoChecked = true
			}
			return "", nil
		}

		// Then the helper short-circuits without probing sudo
		if !canElevateNonInteractivelyImpl(mocks.Shell) {
			t.Errorf("expected true when running as root")
		}
		if sudoChecked {
			t.Errorf("expected sudo -n probe to be skipped when already root")
		}
	})

	t.Run("TrueWhenPasswordlessSudoCached", func(t *testing.T) {
		// Given a non-root process with cached passwordless sudo credentials
		geteuidFunc = func() int { return 1000 }
		mocks := setupWorkstationMocks(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "sudo" && len(args) >= 2 && args[0] == "-n" && args[1] == "true" {
				return "", nil
			}
			return "", fmt.Errorf("unexpected command")
		}

		// Then the helper reports it can elevate without prompting
		if !canElevateNonInteractivelyImpl(mocks.Shell) {
			t.Errorf("expected true when sudo -n true succeeds")
		}
	})

	t.Run("FalseWhenNeitherRootNorCachedSudo", func(t *testing.T) {
		// Given a non-root process with no cached sudo credentials
		geteuidFunc = func() int { return 1000 }
		mocks := setupWorkstationMocks(t)
		mocks.Shell.ExecSilentFunc = func(_ string, _ ...string) (string, error) {
			return "", fmt.Errorf("a password is required")
		}

		// Then the helper reports it cannot elevate without prompting
		if canElevateNonInteractivelyImpl(mocks.Shell) {
			t.Errorf("expected false when not root and sudo -n true fails")
		}
	})
}

func TestPreflightConfigureNetwork_Unix(t *testing.T) {
	t.Run("ReturnsNilSoPerOpSudoHandlesElevation", func(t *testing.T) {
		// On unix the privileged steps each invoke their own sudo (with caching), so the
		// process itself does not need to start elevated. The unix impl must never block
		// 'windsor configure network' on the standard developer setup.
		if err := PreflightConfigureNetwork(); err != nil {
			t.Errorf("expected nil on unix, got %v", err)
		}
	})
}
