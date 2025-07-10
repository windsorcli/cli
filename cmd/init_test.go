package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
)

func TestInitCmd(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a temporary directory
		tmpDir := t.TempDir()
		oldDir, _ := os.Getwd()
		defer os.Chdir(oldDir)
		os.Chdir(tmpDir)

		// Create a pipe to capture os.Stderr
		r, w, _ := os.Pipe()
		oldStderr := os.Stderr
		os.Stderr = w

		// Set up command arguments
		rootCmd.SetArgs([]string{"init"})

		// When executing the command
		err := rootCmd.Execute()

		// Close the writer and restore os.Stderr
		w.Close()
		os.Stderr = oldStderr

		// Read the captured output
		var buf bytes.Buffer
		io.Copy(&buf, r)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should contain success message
		output := buf.String()
		if !strings.Contains(output, "Initialization successful") {
			t.Errorf("Expected message to contain 'Initialization successful', got %q", output)
		}
	})

	t.Run("WithContext", func(t *testing.T) {
		// Given a temporary directory
		tmpDir := t.TempDir()
		oldDir, _ := os.Getwd()
		defer os.Chdir(oldDir)
		os.Chdir(tmpDir)

		// Create a pipe to capture os.Stderr
		r, w, _ := os.Pipe()
		oldStderr := os.Stderr
		os.Stderr = w

		// Set up command arguments
		rootCmd.SetArgs([]string{"init", "test-context"})

		// When executing the command
		err := rootCmd.Execute()

		// Close the writer and restore os.Stderr
		w.Close()
		os.Stderr = oldStderr

		// Read the captured output
		var buf bytes.Buffer
		io.Copy(&buf, r)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should contain success message
		output := buf.String()
		if !strings.Contains(output, "Initialization successful") {
			t.Errorf("Expected message to contain 'Initialization successful', got %q", output)
		}
	})

	t.Run("SetFlagSuccess", func(t *testing.T) {
		// Given a temporary directory
		tmpDir := t.TempDir()
		oldDir, _ := os.Getwd()
		defer os.Chdir(oldDir)
		os.Chdir(tmpDir)

		// Create a pipe to capture os.Stderr
		r, w, _ := os.Pipe()
		oldStderr := os.Stderr
		os.Stderr = w

		// Set up command arguments with set flags
		rootCmd.SetArgs([]string{"init", "--set", "dns.enabled=false", "--set", "cluster.endpoint=https://localhost:6443"})

		// When executing the command
		err := rootCmd.Execute()

		// Close the writer and restore os.Stderr
		w.Close()
		os.Stderr = oldStderr

		// Read the captured output
		var buf bytes.Buffer
		io.Copy(&buf, r)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should contain success message
		output := buf.String()
		if !strings.Contains(output, "Initialization successful") {
			t.Errorf("Expected message to contain 'Initialization successful', got %q", output)
		}
	})

	t.Run("PlatformConfiguration", func(t *testing.T) {
		// Given a temporary directory
		tmpDir := t.TempDir()
		oldDir, _ := os.Getwd()
		defer os.Chdir(oldDir)
		os.Chdir(tmpDir)

		// Create a pipe to capture os.Stderr
		r, w, _ := os.Pipe()
		oldStderr := os.Stderr
		os.Stderr = w

		// Set up command arguments with platform flag
		rootCmd.SetArgs([]string{"init", "--platform", "aws"})

		// When executing the command
		err := rootCmd.Execute()

		// Close the writer and restore os.Stderr
		w.Close()
		os.Stderr = oldStderr

		// Read the captured output
		var buf bytes.Buffer
		io.Copy(&buf, r)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should contain success message
		output := buf.String()
		if !strings.Contains(output, "Initialization successful") {
			t.Errorf("Expected message to contain 'Initialization successful', got %q", output)
		}
	})

	t.Run("ResetFlag", func(t *testing.T) {
		// Given a temporary directory
		tmpDir := t.TempDir()
		oldDir, _ := os.Getwd()
		defer os.Chdir(oldDir)
		os.Chdir(tmpDir)

		// Create a pipe to capture os.Stderr
		r, w, _ := os.Pipe()
		oldStderr := os.Stderr
		os.Stderr = w

		// Set up command arguments with reset flag
		rootCmd.SetArgs([]string{"init", "--reset"})

		// When executing the command
		err := rootCmd.Execute()

		// Close the writer and restore os.Stderr
		w.Close()
		os.Stderr = oldStderr

		// Read the captured output
		var buf bytes.Buffer
		io.Copy(&buf, r)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should contain success message
		output := buf.String()
		if !strings.Contains(output, "Initialization successful") {
			t.Errorf("Expected message to contain 'Initialization successful', got %q", output)
		}
	})
}

func TestInitPipeline(t *testing.T) {
	t.Run("Initialize", func(t *testing.T) {
		// Given a new init pipeline
		pipeline := pipelines.NewInitPipeline()
		injector := di.NewInjector()
		ctx := context.Background()

		// When initializing the pipeline
		err := pipeline.Initialize(injector, ctx)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected successful initialization, got error: %v", err)
		}
	})

	t.Run("InitializeWithContext", func(t *testing.T) {
		// Given a new init pipeline
		pipeline := pipelines.NewInitPipeline()
		injector := di.NewInjector()
		ctx := context.WithValue(context.Background(), "contextName", "test-context")

		// When initializing the pipeline
		err := pipeline.Initialize(injector, ctx)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected successful initialization, got error: %v", err)
		}
	})

	t.Run("InitializeWithReset", func(t *testing.T) {
		// Given a new init pipeline
		pipeline := pipelines.NewInitPipeline()
		injector := di.NewInjector()
		ctx := context.WithValue(context.Background(), "reset", true)

		// When initializing the pipeline
		err := pipeline.Initialize(injector, ctx)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected successful initialization, got error: %v", err)
		}
	})
}
