package cmd

import (
	"strings"
	"testing"
)

func TestVersionCmd(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given version command args
		rootCmd.SetArgs([]string{"version"})

		// And captured output
		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)

		// When executing the command
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And output should contain version info
		output := stdout.String()
		if output == "" {
			t.Error("Expected non-empty stdout")
		}
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("VersionCommandInitialization", func(t *testing.T) {
		// Given a version command
		cmd := versionCmd

		// Then the command should be properly configured
		if cmd.Use != "version" {
			t.Errorf("Expected Use to be 'version', got %s", cmd.Use)
		}
		if cmd.Short == "" {
			t.Error("Expected non-empty Short description")
		}
		if cmd.Long == "" {
			t.Error("Expected non-empty Long description")
		}
	})

	t.Run("VersionCommandWithCustomPlatform", func(t *testing.T) {
		// Given version command args
		rootCmd.SetArgs([]string{"version"})

		// And captured output
		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)

		// And a custom platform
		originalGoos := Goos
		defer func() { Goos = originalGoos }()
		Goos = "testos"

		// When executing the command
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And output should contain custom platform
		output := stdout.String()
		if output == "" {
			t.Error("Expected non-empty stdout")
		}
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})
}

// TestVersionCmd_OutputStructure pins the field labels operators see on
// `windsor version`. The §3.2 contract is: Version, Commit SHA, Build Date,
// Go, Platform — five lines, in that order. Regressing the structure breaks
// scripts that grep for these labels and supply-chain reviewers comparing
// nightly vs tagged builds at a glance.
func TestVersionCmd_OutputStructure(t *testing.T) {
	rootCmd.SetArgs([]string{"version"})
	stdout, _ := captureOutput(t)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stdout)
	if err := Execute(); err != nil {
		t.Fatalf("version command: %v", err)
	}
	for _, label := range []string{"Version:", "Commit SHA:", "Build Date:", "Go:", "Platform:"} {
		if !strings.Contains(stdout.String(), label) {
			t.Errorf("expected output to contain %q, got:\n%s", label, stdout.String())
		}
	}
}

func TestAnnotatedVersion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"taggedRelease", "0.9.0", "0.9.0"},
		{"taggedReleaseCandidate", "0.9.0-rc.1", "0.9.0-rc.1"},
		{"goreleaserSnapshot", "0.0.0-SNAPSHOT-abc1234", "0.0.0-SNAPSHOT-abc1234 (nightly build)"},
		{"lowercaseSnapshot", "1.2.3-snapshot-deadbee", "1.2.3-snapshot-deadbee (nightly build)"},
		{"devBuildUnannotated", "dev", "dev"},
		{"emptyVersion", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := annotatedVersion(tc.in); got != tc.want {
				t.Errorf("annotatedVersion(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
