package cmd_test

import (
	"strings"
	"testing"

	cliCmd "github.com/windsorcli/cli/cmd"
)

func TestVersionCmd(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given version command args
		cliCmd.RootCmd().SetArgs([]string{"version"})

		// And captured output
		stdout, stderr := captureOutput(t)
		cliCmd.RootCmd().SetOut(stdout)
		cliCmd.RootCmd().SetErr(stderr)

		// When executing the command
		err := cliCmd.Execute()

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
		cmd := cliCmd.VersionCmd

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
		cliCmd.RootCmd().SetArgs([]string{"version"})

		// And captured output
		stdout, stderr := captureOutput(t)
		cliCmd.RootCmd().SetOut(stdout)
		cliCmd.RootCmd().SetErr(stderr)

		// And a custom platform
		originalGoos := cliCmd.Goos
		defer func() { cliCmd.Goos = originalGoos }()
		cliCmd.Goos = "testos"

		// When executing the command
		err := cliCmd.Execute()

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
	cliCmd.RootCmd().SetArgs([]string{"version"})
	stdout, _ := captureOutput(t)
	cliCmd.RootCmd().SetOut(stdout)
	cliCmd.RootCmd().SetErr(stdout)
	if err := cliCmd.Execute(); err != nil {
		t.Fatalf("version command: %v", err)
	}
	for _, label := range []string{"Version:", "Commit SHA:", "Build Date:", "Go:", "Platform:"} {
		if !strings.Contains(stdout.String(), label) {
			t.Errorf("expected output to contain %q, got:\n%s", label, stdout.String())
		}
	}
}
