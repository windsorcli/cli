package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type InitMocks struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	Shell         *shell.MockShell
	Shims         *Shims
}

func setupInitTest(t *testing.T, opts ...*SetupOptions) *InitMocks {
	t.Helper()

	// Set up temporary directory and change to it
	tmpDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	t.Cleanup(func() { os.Chdir(oldDir) })

	// Get base mocks
	baseMocks := setupMocks(t, opts...)

	// Add init-specific shell mock behaviors
	baseMocks.Shell.AddCurrentDirToTrustedFileFunc = func() error { return nil }
	baseMocks.Shell.WriteResetTokenFunc = func() (string, error) { return "test-token", nil }

	// Register mock env pipeline in injector (needed since init now runs env pipeline first)
	mockEnvPipeline := pipelines.NewMockBasePipeline()
	mockEnvPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
	mockEnvPipeline.ExecuteFunc = func(ctx context.Context) error { return nil }
	baseMocks.Injector.Register("envPipeline", mockEnvPipeline)

	// Register mock init pipeline in injector (following exec_test.go pattern)
	mockInitPipeline := pipelines.NewMockBasePipeline()
	mockInitPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
	mockInitPipeline.ExecuteFunc = func(ctx context.Context) error { return nil }
	baseMocks.Injector.Register("initPipeline", mockInitPipeline)

	return &InitMocks{
		Injector:      baseMocks.Injector,
		ConfigHandler: baseMocks.ConfigHandler,
		Shell:         baseMocks.Shell,
		Shims:         baseMocks.Shims,
	}
}

// =============================================================================
// Test Cases
// =============================================================================

func TestInitCmd(t *testing.T) {
	createTestInitCmd := func() *cobra.Command {
		// Create a new command with the same RunE as initCmd
		cmd := &cobra.Command{
			Use:   "init [context]",
			Short: "Initialize the application environment",
			RunE:  initCmd.RunE,
		}

		// Copy all flags from initCmd to ensure they're available
		initCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})

		// Disable help text printing
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true

		return cmd
	}

	t.Run("Success", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithReset", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with reset flag
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		ctx = context.WithValue(ctx, "reset", true)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithContext", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with context
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		ctx = context.WithValue(ctx, "contextName", "local")
		cmd.SetArgs([]string{"test-context"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithContextAndReset", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with context and reset
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		ctx = context.WithValue(ctx, "contextName", "local")
		ctx = context.WithValue(ctx, "reset", true)
		cmd.SetArgs([]string{"test-context"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithAllFlags", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with all flags
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		ctx = context.WithValue(ctx, "contextName", "local")
		ctx = context.WithValue(ctx, "reset", true)
		ctx = context.WithValue(ctx, "verbose", true)
		cmd.SetArgs([]string{"test-context"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("PipelineExecutionError", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// And a pipeline that fails to execute
		mockPipeline := pipelines.NewMockBasePipeline()
		mockPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
		mockPipeline.ExecuteFunc = func(ctx context.Context) error {
			return fmt.Errorf("pipeline execution failed")
		}
		mocks.Injector.Register("initPipeline", mockPipeline)

		// When executing the init command
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "pipeline execution failed") {
			t.Errorf("Expected pipeline execution error, got: %v", err)
		}
	})

	t.Run("ConfigHandlerSetContextValueError", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// And a config handler that fails to set context values
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			return fmt.Errorf("failed to set %s", key)
		}
		mocks.Injector.Register("configHandler", mockConfigHandler)

		// When executing the init command with flags that trigger config setting
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--backend", "local"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to set terraform.backend.type") {
			t.Errorf("Expected config handler error, got: %v", err)
		}
	})

	t.Run("SuccessWithBackendFlag", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with backend flag
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--backend", "s3"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithVmDriverFlag", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with VM driver flag
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--vm-driver", "colima"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithVmCpuFlag", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with VM CPU flag
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--vm-cpu", "4"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithVmDiskFlag", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with VM disk flag
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--vm-disk", "100"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithVmMemoryFlag", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with VM memory flag
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--vm-memory", "8192"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithVmArchFlag", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with VM arch flag
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--vm-arch", "x86_64"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithDockerFlag", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with docker flag
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--docker"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithGitLivereloadFlag", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with git livereload flag
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--git-livereload"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithBlueprintFlag", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with blueprint flag
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--blueprint", "full"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithPlatformFlag", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with platform flag
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--platform", "local"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithSetFlags", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with set flags
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--set", "cluster.endpoint=https://localhost:6443", "--set", "dns.domain=test.local"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SetFlagInvalidFormat", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with invalid set flag format
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--set", "invalid-format"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur (invalid format is ignored)
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("MultipleFlagsCombination", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with multiple flags
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--backend", "s3", "--vm-driver", "colima", "--docker", "--blueprint", "full"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})
}
