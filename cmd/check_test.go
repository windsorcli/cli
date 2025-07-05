package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// Helper function to check if a string contains a substring
func checkContains(str, substr string) bool {
	return strings.Contains(str, substr)
}

func TestCheckCmd(t *testing.T) {
	setup := func(t *testing.T, withConfig bool) (*bytes.Buffer, *bytes.Buffer) {
		t.Helper()

		// Change to a temporary directory
		origDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get working directory: %v", err)
		}

		tmpDir := t.TempDir()
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("Failed to change to temp directory: %v", err)
		}

		// Cleanup to change back to original directory
		t.Cleanup(func() {
			if err := os.Chdir(origDir); err != nil {
				t.Logf("Warning: Failed to change back to original directory: %v", err)
			}
		})

		// Create config file if requested
		if withConfig {
			configContent := `contexts:
  default:
    tools:
      enabled: true`
			if err := os.WriteFile("windsor.yaml", []byte(configContent), 0644); err != nil {
				t.Fatalf("Failed to create config file: %v", err)
			}
		}

		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)
		rootCmd.SetArgs([]string{"check"})

		return stdout, stderr
	}

	t.Run("Success", func(t *testing.T) {
		// Given a directory with proper configuration
		stdout, stderr := setup(t, true)

		// When executing the command
		err := Execute(nil)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And output should contain success message
		output := stdout.String()
		if output != "All tools are up to date.\n" {
			t.Errorf("Expected 'All tools are up to date.', got: %q", output)
		}
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("ConfigNotLoaded", func(t *testing.T) {
		// Given a directory with no configuration
		_, _ = setup(t, false)

		// When executing the command
		err := Execute(nil)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain init message
		expectedError := "Error executing check pipeline: Nothing to check. Have you run \033[1mwindsor init\033[0m?"
		if err.Error() != expectedError {
			t.Errorf("Expected error about init, got: %v", err)
		}
	})
}

func TestCheckNodeHealthCmd(t *testing.T) {
	setup := func(t *testing.T, withConfig bool) (*bytes.Buffer, *bytes.Buffer) {
		t.Helper()

		// Change to a temporary directory
		origDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get working directory: %v", err)
		}

		tmpDir := t.TempDir()
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("Failed to change to temp directory: %v", err)
		}

		// Cleanup to change back to original directory
		t.Cleanup(func() {
			if err := os.Chdir(origDir); err != nil {
				t.Logf("Warning: Failed to change back to original directory: %v", err)
			}
		})

		// Create config file if requested
		if withConfig {
			configContent := `contexts:
  default:
    cluster:
      enabled: true`
			if err := os.WriteFile("windsor.yaml", []byte(configContent), 0644); err != nil {
				t.Fatalf("Failed to create config file: %v", err)
			}
		}

		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)

		// Reset global command flags to avoid state leakage
		nodeHealthTimeout = 0
		nodeHealthNodes = []string{}
		nodeHealthVersion = ""

		// Reset command flags
		checkNodeHealthCmd.ResetFlags()
		checkNodeHealthCmd.Flags().DurationVar(&nodeHealthTimeout, "timeout", 0, "Maximum time to wait for nodes to be ready (default 5m)")
		checkNodeHealthCmd.Flags().StringSliceVar(&nodeHealthNodes, "nodes", []string{}, "Nodes to check (required)")
		checkNodeHealthCmd.Flags().StringVar(&nodeHealthVersion, "version", "", "Expected version to check against (optional)")

		return stdout, stderr
	}

	t.Run("ClusterClientError", func(t *testing.T) {
		// Given a directory with proper configuration
		_, _ = setup(t, true)

		// Setup command args with nodes
		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", "10.0.0.1,10.0.0.2"})

		// When executing the command
		err := Execute(nil)

		// Then an error should occur (because cluster client can't be initialized without proper config)
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain cluster client message
		if !checkContains(err.Error(), "Error executing check pipeline") {
			t.Errorf("Expected error about pipeline execution, got: %v", err)
		}
	})

	t.Run("ClusterClientErrorWithVersion", func(t *testing.T) {
		// Given a directory with proper configuration
		_, _ = setup(t, true)

		// Setup command args with nodes and version
		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", "10.0.0.1", "--version", "1.0.0"})

		// When executing the command
		err := Execute(nil)

		// Then an error should occur (because cluster client can't be initialized without proper config)
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain cluster client message
		if !checkContains(err.Error(), "Error executing check pipeline") {
			t.Errorf("Expected error about pipeline execution, got: %v", err)
		}
	})

	t.Run("ClusterClientErrorWithTimeout", func(t *testing.T) {
		// Given a directory with proper configuration
		_, _ = setup(t, true)

		// Setup command args with nodes and timeout
		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", "10.0.0.1", "--timeout", "10s"})

		// When executing the command
		err := Execute(nil)

		// Then an error should occur (because cluster client can't be initialized without proper config)
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain cluster client message
		if !checkContains(err.Error(), "Error executing check pipeline") {
			t.Errorf("Expected error about pipeline execution, got: %v", err)
		}
	})

	t.Run("ConfigNotLoaded", func(t *testing.T) {
		// Given a directory with no configuration
		_, _ = setup(t, false)

		// Setup command args
		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", "10.0.0.1"})

		// When executing the command
		err := Execute(nil)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain init message
		expectedError := "Error executing check pipeline: Nothing to check. Have you run \033[1mwindsor init\033[0m?"
		if err.Error() != expectedError {
			t.Errorf("Expected error about init, got: %v", err)
		}
	})

	t.Run("NoNodesSpecified", func(t *testing.T) {
		// Given a directory with proper configuration
		_, _ = setup(t, true)

		// Setup command args without nodes
		rootCmd.SetArgs([]string{"check", "node-health"})

		// When executing the command
		err := Execute(nil)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain nodes message
		expectedError := "No nodes specified. Use --nodes flag to specify nodes to check"
		if err.Error() != expectedError {
			t.Errorf("Expected error about nodes, got: %v", err)
		}
	})

	t.Run("EmptyNodesFlag", func(t *testing.T) {
		// Given a directory with proper configuration
		_, _ = setup(t, true)

		// Setup command args with empty nodes flag
		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", ""})

		// When executing the command
		err := Execute(nil)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain nodes message
		expectedError := "No nodes specified. Use --nodes flag to specify nodes to check"
		if err.Error() != expectedError {
			t.Errorf("Expected error about nodes, got: %v", err)
		}
	})
}
