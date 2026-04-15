package cmd

import (
	"context"
	"io"
	"testing"

	"github.com/spf13/cobra"
)

// =============================================================================
// Test Setup
// =============================================================================

// createTestMirrorCmd returns an isolated cobra.Command wrapping the mirrorCmd
// RunE so tests can exercise the command without mutating global flag state.
func createTestMirrorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "mirror",
		RunE: mirrorCmd.RunE,
	}
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMirrorCmd(t *testing.T) {
	suppressProcessStdout(t)
	suppressProcessStderr(t)

	t.Run("FailsWithoutBlueprint", func(t *testing.T) {
		// Given a runtime whose blueprint handler cannot load a blueprint
		// because no project exists in the temp working directory
		mocks := setupMocks(t)
		cmd := createTestMirrorCmd()
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, mocks.Runtime)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})

		// When executing the mirror command
		err := cmd.Execute()

		// Then an error is returned (blueprint load or missing OCI sources)
		if err == nil {
			t.Error("expected error in an uninitialised project, got nil")
		}
	})
}
