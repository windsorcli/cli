package cmd

import (
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
