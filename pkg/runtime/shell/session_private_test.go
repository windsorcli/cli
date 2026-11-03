package shell

import (
	"fmt"
	"testing"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestShell_GenerateRandomString(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("Success", func(t *testing.T) {
		shell, mocks := setup(t)
		mocks.Shims.RandRead = func(b []byte) (n int, err error) {
			for i := range b {
				b[i] = byte(i % 62) // This will map to characters in the charset
			}
			return len(b), nil
		}
		result, err := shell.generateRandomString(10)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(result) != 10 {
			t.Errorf("Expected length 10, got %d", len(result))
		}
	})

	t.Run("ErrorWhenRandReadFails", func(t *testing.T) {
		shell, mocks := setup(t)
		mocks.Shims.RandRead = func(b []byte) (n int, err error) {
			return 0, fmt.Errorf("random read failed")
		}
		result, err := shell.generateRandomString(10)
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if result != "" {
			t.Errorf("Expected empty result on error, got %q", result)
		}
	})
}
