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
