//go:build integration
// +build integration

package helpers

import "testing"

// =============================================================================
// Test Helpers
// =============================================================================

func TestStripCoverageErrors(t *testing.T) {
	t.Run("RemovesOnlyTheCoverageRuntimeLines", func(t *testing.T) {
		// Given stderr with a coverage runtime error between two windsor lines
		stderr := []byte("error: coverage meta-data emit failed: rename: file exists\nerror: plan failed\nerror: coverage counter data emit failed: x\nhint: Run windsor plan.\n")

		// When the coverage errors are stripped
		got := string(stripCoverageErrors(stderr))

		// Then the windsor lines remain in order
		want := "error: plan failed\nhint: Run windsor plan.\n"
		if got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	})

	t.Run("KeepsAWindsorErrorThatMentionsCoverage", func(t *testing.T) {
		// Given a windsor error whose text contains the word coverage
		stderr := []byte("error: coverage report is missing\n")

		// When the coverage errors are stripped
		got := string(stripCoverageErrors(stderr))

		// Then the line remains
		if got != string(stderr) {
			t.Errorf("expected %q, got %q", stderr, got)
		}
	})
}
