//go:build integration
// +build integration

package cmd

// The IntegrationTest provides integration coverage for Windsor CLI commands.
// It runs commands with a real runtime and real project layout in a temp directory,
// It validates end-to-end behavior without mocking the runtime,
// and supports optional partial overrides (e.g. mock APIs) via runtimeOverridesKey.

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestIntegration_BuildID(t *testing.T) {
	t.Run("BuildIDCommandConfiguration", func(t *testing.T) {
		if buildIdCmd.Use != "build-id" {
			t.Errorf("Expected Use to be 'build-id', got %s", buildIdCmd.Use)
		}
		if buildIdCmd.Short == "" {
			t.Error("Expected non-empty Short description")
		}
		if buildIdCmd.Long == "" {
			t.Error("Expected non-empty Long description")
		}
		if !buildIdCmd.SilenceUsage {
			t.Error("Expected SilenceUsage to be true")
		}
	})

	t.Run("BuildIDCommandFlags", func(t *testing.T) {
		newFlag := buildIdCmd.Flags().Lookup("new")
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

	t.Run("BuildIDIsSubcommandOfRoot", func(t *testing.T) {
		found := false
		for _, subCmd := range rootCmd.Commands() {
			if subCmd.Use == "build-id" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected 'build-id' to be a subcommand of root")
		}
	})

	t.Run("GetBuildIDReturnsEmptyWhenNoBuildIDFile", func(t *testing.T) {
		SetupIntegrationProject(t, minimalWindsorYAML)
		stdout, stderr, err := runCmd(t, context.Background(), []string{"build-id"})
		assertSuccessAndNoStderr(t, err, stderr)
		if stdout != "" {
			t.Errorf("Expected empty build ID when no file, got %q", stdout)
		}
	})

	t.Run("GenerateBuildIDSucceedsAndPrintsBuildID", func(t *testing.T) {
		SetupIntegrationProject(t, minimalWindsorYAML)
		stdout, stderr, err := runCmd(t, context.Background(), []string{"build-id", "--new"})
		assertSuccessAndNoStderr(t, err, stderr)
		if stdout == "" {
			t.Error("Expected non-empty build ID from --new")
		}
		if len(stdout) < 10 {
			t.Errorf("Expected build ID format like YYMMDD.XXX.N, got %q", stdout)
		}
	})

	t.Run("BuildIDFormatMatchesYYMMDDDotXXXDotN", func(t *testing.T) {
		SetupIntegrationProject(t, minimalWindsorYAML)
		stdout, stderr, err := runCmd(t, context.Background(), []string{"build-id", "--new"})
		assertSuccessAndNoStderr(t, err, stderr)
		re := regexp.MustCompile(`^\d{6}\.\d{3}\.\d+$`)
		if !re.MatchString(stdout) {
			t.Errorf("Expected build ID format YYMMDD.XXX.N, got %q", stdout)
		}
	})

	t.Run("GetBuildIDFailsWhenBuildIDFileUnreadable", func(t *testing.T) {
		projectRoot := SetupIntegrationProject(t, minimalWindsorYAML)
		windsorDir := filepath.Join(projectRoot, ".windsor")
		if err := os.MkdirAll(windsorDir, 0750); err != nil {
			t.Fatalf("Failed to create .windsor: %v", err)
		}
		buildIDPath := filepath.Join(windsorDir, ".build-id")
		if err := os.MkdirAll(buildIDPath, 0750); err != nil {
			t.Fatalf("Failed to create .build-id as dir: %v", err)
		}
		_, _, err := runCmd(t, context.Background(), []string{"build-id"})
		assertFailureAndErrorContains(t, err, "failed to manage build ID")
	})

	t.Run("BuildIDWithRuntimeOverrideUsesInjectedRuntime", func(t *testing.T) {
		SetupIntegrationProject(t, minimalWindsorYAML)
		rt := runtime.NewRuntime()
		ctx := context.WithValue(context.Background(), runtimeOverridesKey, rt)
		stdout, stderr, err := runCmd(t, ctx, []string{"build-id", "--new"})
		assertSuccessAndNoStderr(t, err, stderr)
		if stdout == "" {
			t.Error("Expected non-empty build ID when using runtime override")
		}
		re := regexp.MustCompile(`^\d{6}\.\d{3}\.\d+$`)
		if !re.MatchString(stdout) {
			t.Errorf("Expected build ID format YYMMDD.XXX.N, got %q", stdout)
		}
	})

	t.Run("GetBuildIDReturnsValueAfterGenerateBuildID", func(t *testing.T) {
		projectRoot := SetupIntegrationProject(t, minimalWindsorYAML)
		_, _, err := runCmd(t, context.Background(), []string{"build-id", "--new"})
		if err != nil {
			t.Fatalf("Generate build ID failed: %v", err)
		}
		if _, err := os.Stat(filepath.Join(projectRoot, ".windsor", ".build-id")); err != nil {
			t.Fatalf("Expected .windsor/.build-id to exist: %v", err)
		}
		stdout, stderr, err := runCmd(t, context.Background(), []string{"build-id"})
		assertSuccessAndNoStderr(t, err, stderr)
		if stdout == "" {
			t.Error("Expected build ID to be printed after previous --new")
		}
	})
}
