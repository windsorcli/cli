// The shims_test package provides test utilities for the shims package
// It contains mock implementations of system call wrappers
// It enables testing of system-level operations
// It facilitates dependency injection and test isolation

package cmd

import (
	"runtime"
	"testing"
)

func TestShims_Goos(t *testing.T) {
	// Given a new shims instance
	shims := NewShims()

	// When getting the OS
	os := shims.Goos()

	// Then it should match runtime.GOOS
	if os != runtime.GOOS {
		t.Errorf("Expected OS %q, got %q", runtime.GOOS, os)
	}
}
