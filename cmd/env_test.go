package cmd

import (
	"bytes"
	"testing"
)

func TestEnvCmd(t *testing.T) {
	setup := func(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
		t.Helper()
		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)
		return stdout, stderr
	}

	t.Run("Success", func(t *testing.T) {
		// Given proper output capture
		_, stderr := setup(t)

		rootCmd.SetArgs([]string{"env"})

		// When executing the command
		err := Execute(nil)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SuccessWithDecrypt", func(t *testing.T) {
		// Given proper output capture
		_, stderr := setup(t)

		rootCmd.SetArgs([]string{"env", "--decrypt"})

		// When executing the command
		err := Execute(nil)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SuccessWithHook", func(t *testing.T) {
		// Given proper output capture
		_, stderr := setup(t)

		rootCmd.SetArgs([]string{"env", "--hook"})

		// When executing the command
		err := Execute(nil)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SuccessWithVerbose", func(t *testing.T) {
		// Given proper output capture
		_, stderr := setup(t)

		rootCmd.SetArgs([]string{"env", "--verbose"})

		// When executing the command
		err := Execute(nil)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SuccessWithAllFlags", func(t *testing.T) {
		// Given proper output capture
		_, stderr := setup(t)

		rootCmd.SetArgs([]string{"env", "--decrypt", "--hook", "--verbose"})

		// When executing the command
		err := Execute(nil)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})
}
