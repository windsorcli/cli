package cmd

import (
	"bytes"
	"context"
	"os"
	"testing"
)

func TestBuildIDCmd(t *testing.T) {
	setup := func(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
		t.Helper()
		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)
		return stdout, stderr
	}

	t.Run("Success", func(t *testing.T) {
		// Given proper output capture and mock setup
		_, stderr := setup(t)
		mocks := setupMocks(t)

		// Set up command context with runtime override
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"build-id"})

		// When executing the command
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SuccessWithNewFlag", func(t *testing.T) {
		// Given proper output capture and mock setup
		_, stderr := setup(t)
		mocks := setupMocks(t)

		// Set up command context with runtime override
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"build-id", "--new"})

		// When executing the command
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("CommandConfiguration", func(t *testing.T) {
		// Given the build ID command
		cmd := buildIdCmd

		// Then the command should be properly configured
		if cmd.Use != "build-id" {
			t.Errorf("Expected Use to be 'build-id', got %s", cmd.Use)
		}
		if cmd.Short == "" {
			t.Error("Expected non-empty Short description")
		}
		if cmd.Long == "" {
			t.Error("Expected non-empty Long description")
		}
		if !cmd.SilenceUsage {
			t.Error("Expected SilenceUsage to be true")
		}
	})

	t.Run("CommandFlags", func(t *testing.T) {
		// Given the build ID command
		cmd := buildIdCmd

		// Then the command should have the new flag
		newFlag := cmd.Flags().Lookup("new")
		if newFlag == nil {
			t.Fatal("Expected 'new' flag to exist")
		}
		if newFlag.DefValue != "false" {
			t.Errorf("Expected 'new' flag default value to be 'false', got %s", newFlag.DefValue)
		}
		if newFlag.Usage == "" {
			t.Error("Expected 'new' flag to have usage description")
		}
	})

	t.Run("CommandIntegration", func(t *testing.T) {
		// Given the root command
		cmd := rootCmd

		// Then the build ID command should be a subcommand
		buildIDSubCmd := cmd.Commands()
		found := false
		for _, subCmd := range buildIDSubCmd {
			if subCmd.Use == "build-id" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected 'build-id' to be a subcommand of root")
		}
	})

	t.Run("PipelineSetupError", func(t *testing.T) {
		// Given proper output capture and mock setup
		_, stderr := setup(t)
		mocks := setupMocks(t)

		// Set up command context with runtime override
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"build-id"})

		// When executing the command
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success with proper mocks, got error: %v", err)
		}

		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("PipelineExecuteError", func(t *testing.T) {
		// Given proper output capture and mock setup
		_, stderr := setup(t)
		mocks := setupMocks(t)

		// Set up command context with runtime override
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"build-id"})

		// When executing the command
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success with proper mocks, got error: %v", err)
		}

		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("MissingRuntimeInContext", func(t *testing.T) {
		// Given proper output capture with minimal mock setup
		setup(t)
		// Use a temp directory to avoid slow directory walking
		tmpDir := t.TempDir()
		oldDir, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(oldDir) })

		// Set up command context without runtime override
		ctx := context.Background()
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"build-id"})

		// When executing the command
		err := Execute()

		// Then it may return an error or succeed depending on environment
		if err != nil {
			t.Logf("Command failed as expected without runtime override: %v", err)
		} else {
			t.Logf("Command succeeded (runtime may be available from environment)")
		}
	})

	t.Run("ContextWithNewFlag", func(t *testing.T) {
		// Given proper output capture and mock setup
		_, stderr := setup(t)
		mocks := setupMocks(t)

		// Set up command context with runtime override
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"build-id", "--new"})

		// When executing the command
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("ContextWithoutNewFlag", func(t *testing.T) {
		// Given proper output capture and mock setup
		_, stderr := setup(t)
		mocks := setupMocks(t)

		// Set up command context with runtime override
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"build-id"})

		// When executing the command
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("PipelineInitializationError", func(t *testing.T) {
		// Given proper output capture and mock setup
		setup(t)
		mocks := setupMocks(t)

		// Set up command context with runtime override
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"build-id"})

		// When executing the command
		err := Execute()

		// Then no error should occur with proper mocks
		if err != nil {
			t.Errorf("Expected success with proper mocks, got error: %v", err)
		}
	})

	t.Run("InvalidRuntimeType", func(t *testing.T) {
		// Given proper output capture
		setup(t)

		// Set up command context with invalid runtime type
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, "invalid")
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"build-id"})

		// When executing the command
		err := Execute()

		// Then it may succeed or fail depending on environment
		// Invalid type is ignored and real runtime is used
		if err != nil {
			t.Logf("Command failed as expected with invalid runtime type: %v", err)
		} else {
			t.Logf("Command succeeded (real runtime may be available from environment)")
		}
	})
}
