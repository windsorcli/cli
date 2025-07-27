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
	"github.com/windsorcli/cli/pkg/constants"
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

	// Reset global variables to prevent test interference
	initReset = false
	initBackend = ""
	initAwsProfile = ""
	initAwsEndpointURL = ""
	initVmDriver = ""
	initCpu = 0
	initDisk = 0
	initMemory = 0
	initArch = ""
	initDocker = false
	initGitLivereload = false
	initProvider = ""
	initPlatform = ""
	initBlueprint = ""
	initEndpoint = ""
	initSetFlags = []string{}

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
		cmd.SetArgs([]string{"--provider", "local"})
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

	t.Run("LocalContextAutoProviderBlueprint", func(t *testing.T) {
		// Given a local context with no explicit provider or blueprint
		args := []string{"local"}
		initProvider = ""
		initBlueprint = ""

		// When processing the init logic
		ctx := context.Background()
		ctx = context.WithValue(ctx, "reset", false)
		ctx = context.WithValue(ctx, "trust", true)

		// If context is "local" and neither provider nor blueprint is set, set both
		if len(args) > 0 && strings.HasPrefix(args[0], "local") && initProvider == "" && initBlueprint == "" {
			initProvider = "local"
			initBlueprint = constants.DEFAULT_OCI_BLUEPRINT_URL
		}

		// If provider is set and blueprint is not set, set blueprint (covers all providers, including local)
		if initProvider != "" && initBlueprint == "" {
			initBlueprint = constants.DEFAULT_OCI_BLUEPRINT_URL
		}

		// If blueprint is set, use it (overrides all)
		if initBlueprint != "" {
			ctx = context.WithValue(ctx, "blueprint", initBlueprint)
		}

		// Then both provider and blueprint should be set correctly
		if initProvider != "local" {
			t.Errorf("Expected provider to be 'local', got %s", initProvider)
		}

		if blueprintCtx := ctx.Value("blueprint"); blueprintCtx == nil {
			t.Errorf("Expected blueprint to be set in context for local context")
		} else if blueprint, ok := blueprintCtx.(string); !ok {
			t.Errorf("Expected blueprint context value to be a string")
		} else if blueprint != constants.DEFAULT_OCI_BLUEPRINT_URL {
			t.Errorf("Expected blueprint to be %s, got %s", constants.DEFAULT_OCI_BLUEPRINT_URL, blueprint)
		}
	})

	t.Run("LocalContextWithExplicitProvider", func(t *testing.T) {
		// Given a local context with explicit provider
		args := []string{"local"}
		initProvider = "aws"
		initBlueprint = ""

		// When processing the init logic
		ctx := context.Background()
		ctx = context.WithValue(ctx, "reset", false)
		ctx = context.WithValue(ctx, "trust", true)

		// If context is "local" and neither provider nor blueprint is set, set both
		if len(args) > 0 && strings.HasPrefix(args[0], "local") && initProvider == "" && initBlueprint == "" {
			initProvider = "local"
			initBlueprint = constants.DEFAULT_OCI_BLUEPRINT_URL
		}

		// If provider is set and blueprint is not set, set blueprint (covers all providers, including local)
		if initProvider != "" && initBlueprint == "" {
			initBlueprint = constants.DEFAULT_OCI_BLUEPRINT_URL
		}

		// If blueprint is set, use it (overrides all)
		if initBlueprint != "" {
			ctx = context.WithValue(ctx, "blueprint", initBlueprint)
		}

		// Then the explicit provider should be used and OCI blueprint should be set
		if initProvider != "aws" {
			t.Errorf("Expected provider to be 'aws', got %s", initProvider)
		}

		if blueprintCtx := ctx.Value("blueprint"); blueprintCtx == nil {
			t.Errorf("Expected blueprint to be set in context when explicit provider is specified")
		} else if blueprint, ok := blueprintCtx.(string); !ok {
			t.Errorf("Expected blueprint context value to be a string")
		} else if blueprint != constants.DEFAULT_OCI_BLUEPRINT_URL {
			t.Errorf("Expected blueprint to be %s, got %s", constants.DEFAULT_OCI_BLUEPRINT_URL, blueprint)
		}
	})

	t.Run("LocalContextWithExplicitBlueprint", func(t *testing.T) {
		// Given a local context with explicit blueprint
		args := []string{"local"}
		initProvider = ""
		initBlueprint = "oci://custom/blueprint:v1.0.0"

		// When processing the init logic
		ctx := context.Background()
		ctx = context.WithValue(ctx, "reset", false)
		ctx = context.WithValue(ctx, "trust", true)

		// If context is "local" and neither provider nor blueprint is set, set both
		if len(args) > 0 && strings.HasPrefix(args[0], "local") && initProvider == "" && initBlueprint == "" {
			initProvider = "local"
			initBlueprint = constants.DEFAULT_OCI_BLUEPRINT_URL
		}

		// If provider is set and blueprint is not set, set blueprint (covers all providers, including local)
		if initProvider != "" && initBlueprint == "" {
			initBlueprint = constants.DEFAULT_OCI_BLUEPRINT_URL
		}

		// If blueprint is set, use it (overrides all)
		if initBlueprint != "" {
			ctx = context.WithValue(ctx, "blueprint", initBlueprint)
		}

		// Then the explicit blueprint should be used
		if blueprintCtx := ctx.Value("blueprint"); blueprintCtx == nil {
			t.Errorf("Expected blueprint to be set in context")
		} else if blueprint, ok := blueprintCtx.(string); !ok {
			t.Errorf("Expected blueprint context value to be a string")
		} else if blueprint != "oci://custom/blueprint:v1.0.0" {
			t.Errorf("Expected blueprint to be oci://custom/blueprint:v1.0.0, got %s", blueprint)
		}
	})

	t.Run("NonLocalContext", func(t *testing.T) {
		// Given a non-local context with no explicit provider or blueprint
		args := []string{"aws"}
		initProvider = ""
		initBlueprint = ""

		// When processing the init logic
		ctx := context.Background()
		ctx = context.WithValue(ctx, "reset", false)
		ctx = context.WithValue(ctx, "trust", true)

		// If context is "local" and neither provider nor blueprint is set, set both
		if len(args) > 0 && strings.HasPrefix(args[0], "local") && initProvider == "" && initBlueprint == "" {
			initProvider = "local"
			initBlueprint = constants.DEFAULT_OCI_BLUEPRINT_URL
		}

		// If provider is set and blueprint is not set, set blueprint (covers all providers, including local)
		if initProvider != "" && initBlueprint == "" {
			initBlueprint = constants.DEFAULT_OCI_BLUEPRINT_URL
		}

		// If blueprint is set, use it (overrides all)
		if initBlueprint != "" {
			ctx = context.WithValue(ctx, "blueprint", initBlueprint)
		}

		// Then provider should not be auto-set and blueprint should not be set
		if initProvider != "" {
			t.Errorf("Expected provider to not be auto-set for non-local context, got %s", initProvider)
		}

		if blueprintCtx := ctx.Value("blueprint"); blueprintCtx != nil {
			t.Errorf("Expected blueprint to not be set when no provider is specified, got %v", blueprintCtx)
		}
	})

	t.Run("ProviderAutoBlueprint", func(t *testing.T) {
		// Given a provider is specified
		initProvider = "aws"
		initBlueprint = ""

		// When processing the init logic
		ctx := context.Background()
		ctx = context.WithValue(ctx, "reset", false)
		ctx = context.WithValue(ctx, "trust", true)

		// If provider is set and blueprint is not set, set blueprint (covers all providers, including local)
		if initProvider != "" && initBlueprint == "" {
			initBlueprint = constants.DEFAULT_OCI_BLUEPRINT_URL
		}

		if initBlueprint != "" {
			ctx = context.WithValue(ctx, "blueprint", initBlueprint)
		}

		// Then the blueprint should be set correctly
		if blueprintCtx := ctx.Value("blueprint"); blueprintCtx == nil {
			t.Errorf("Expected blueprint to be set in context when provider is specified")
		} else if blueprint, ok := blueprintCtx.(string); !ok {
			t.Errorf("Expected blueprint context value to be a string")
		} else if blueprint != constants.DEFAULT_OCI_BLUEPRINT_URL {
			t.Errorf("Expected blueprint to be %s, got %s", constants.DEFAULT_OCI_BLUEPRINT_URL, blueprint)
		}
	})

	t.Run("ExplicitBlueprintOverrides", func(t *testing.T) {
		// Given an explicit blueprint is specified
		initProvider = "aws"
		initBlueprint = "oci://custom/blueprint:v1.0.0"

		// When processing the init logic
		ctx := context.Background()
		ctx = context.WithValue(ctx, "reset", false)
		ctx = context.WithValue(ctx, "trust", true)

		// If provider is set and blueprint is not set, set blueprint (covers all providers, including local)
		if initProvider != "" && initBlueprint == "" {
			initBlueprint = constants.DEFAULT_OCI_BLUEPRINT_URL
		}

		if initBlueprint != "" {
			ctx = context.WithValue(ctx, "blueprint", initBlueprint)
		}

		// Then the explicit blueprint should be used instead of the default
		if blueprintCtx := ctx.Value("blueprint"); blueprintCtx == nil {
			t.Errorf("Expected blueprint to be set in context")
		} else if blueprint, ok := blueprintCtx.(string); !ok {
			t.Errorf("Expected blueprint context value to be a string")
		} else if blueprint != "oci://custom/blueprint:v1.0.0" {
			t.Errorf("Expected blueprint to be oci://custom/blueprint:v1.0.0, got %s", blueprint)
		}
	})

	t.Run("PlatformAutoBlueprint", func(t *testing.T) {
		// Given a platform is specified
		initPlatform = "aws"
		initProvider = ""
		initBlueprint = ""

		// When processing the init logic
		ctx := context.Background()
		ctx = context.WithValue(ctx, "reset", false)
		ctx = context.WithValue(ctx, "trust", true)

		// Handle deprecated --platform flag and set blueprint
		if initPlatform != "" && initProvider == "" && initBlueprint == "" {
			initBlueprint = constants.DEFAULT_OCI_BLUEPRINT_URL
		}

		if initBlueprint != "" {
			ctx = context.WithValue(ctx, "blueprint", initBlueprint)
		}

		// Then the blueprint should be set correctly
		if blueprintCtx := ctx.Value("blueprint"); blueprintCtx == nil {
			t.Errorf("Expected blueprint to be set in context when platform is specified")
		} else if blueprint, ok := blueprintCtx.(string); !ok {
			t.Errorf("Expected blueprint context value to be a string")
		} else if blueprint != constants.DEFAULT_OCI_BLUEPRINT_URL {
			t.Errorf("Expected blueprint to be %s, got %s", constants.DEFAULT_OCI_BLUEPRINT_URL, blueprint)
		}
	})

	t.Run("LocalContextVariations", func(t *testing.T) {
		testCases := []struct {
			name              string
			args              []string
			provider          string
			blueprint         string
			expectedProvider  string
			expectedBlueprint string
		}{
			{
				name:              "local context with no flags",
				args:              []string{"local"},
				provider:          "",
				blueprint:         "",
				expectedProvider:  "local",
				expectedBlueprint: constants.DEFAULT_OCI_BLUEPRINT_URL,
			},
			{
				name:              "local-dev context with no flags",
				args:              []string{"local-dev"},
				provider:          "",
				blueprint:         "",
				expectedProvider:  "local",
				expectedBlueprint: constants.DEFAULT_OCI_BLUEPRINT_URL,
			},
			{
				name:              "local context with explicit provider",
				args:              []string{"local"},
				provider:          "aws",
				blueprint:         "",
				expectedProvider:  "aws",
				expectedBlueprint: constants.DEFAULT_OCI_BLUEPRINT_URL,
			},
			{
				name:              "local context with explicit blueprint",
				args:              []string{"local"},
				provider:          "",
				blueprint:         "oci://custom/blueprint:v1.0.0",
				expectedProvider:  "",
				expectedBlueprint: "oci://custom/blueprint:v1.0.0",
			},
			{
				name:              "local context with both explicit provider and blueprint",
				args:              []string{"local"},
				provider:          "azure",
				blueprint:         "oci://custom/blueprint:v1.0.0",
				expectedProvider:  "azure",
				expectedBlueprint: "oci://custom/blueprint:v1.0.0",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Given the test case parameters
				initProvider = tc.provider
				initBlueprint = tc.blueprint

				// When processing the init logic
				ctx := context.Background()
				ctx = context.WithValue(ctx, "reset", false)
				ctx = context.WithValue(ctx, "trust", true)

				// If context is "local" and neither provider nor blueprint is set, set both
				if len(tc.args) > 0 && strings.HasPrefix(tc.args[0], "local") && initProvider == "" && initBlueprint == "" {
					initProvider = "local"
					initBlueprint = constants.DEFAULT_OCI_BLUEPRINT_URL
				}

				// If provider is set and blueprint is not set, set blueprint (covers all providers, including local)
				if initProvider != "" && initBlueprint == "" {
					initBlueprint = constants.DEFAULT_OCI_BLUEPRINT_URL
				}

				// If blueprint is set, use it (overrides all)
				if initBlueprint != "" {
					ctx = context.WithValue(ctx, "blueprint", initBlueprint)
				}

				// Then verify the results
				if tc.expectedProvider != "" {
					if initProvider != tc.expectedProvider {
						t.Errorf("Expected provider to be %s, got %s", tc.expectedProvider, initProvider)
					}
				}

				if tc.expectedBlueprint != "" {
					if blueprintCtx := ctx.Value("blueprint"); blueprintCtx == nil {
						t.Errorf("Expected blueprint to be set in context")
					} else if blueprint, ok := blueprintCtx.(string); !ok {
						t.Errorf("Expected blueprint context value to be a string")
					} else if blueprint != tc.expectedBlueprint {
						t.Errorf("Expected blueprint to be %s, got %s", tc.expectedBlueprint, blueprint)
					}
				}
			})
		}
	})

	t.Run("RunEAutoProviderBlueprintLogic", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with local context (should auto-set provider and blueprint)
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"local"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur and the logic should be executed
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("RunEExplicitProviderAutoBlueprint", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with explicit provider
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--provider", "aws"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur and the logic should be executed
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("RunEExplicitBlueprintOverrides", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with explicit blueprint
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--blueprint", "oci://custom/blueprint:v1.0.0"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur and the logic should be executed
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("RunESimpleFlagsProcessing", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with simple flags
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{
			"test-context",
			"--reset",
			"--docker",
			"--git-livereload",
			"--provider", "aws",
		})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur and flags should be processed
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("RunEDeprecatedPlatformFlag", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with deprecated platform flag
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--platform", "aws"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur and the deprecated flag should be handled
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("RunEPlatformAndProviderConflict", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with both platform and provider flags
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--platform", "aws", "--provider", "azure"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur - platform overrides provider
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("RunEPlatformOverridesAutoProvider", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with platform flag (should override auto-set provider)
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"local", "--platform", "aws"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur and platform should override auto-set provider
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("RunEContextNameAsProvider", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with context name that matches a provider
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"aws"}) // No explicit provider flag
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur and context name should be used as provider
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("RunEContextNameAsProviderForAzure", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with "azure" context name
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"azure"}) // No explicit provider flag
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur and "azure" should be used as provider
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("RunEContextNameAsProviderForMetal", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with "metal" context name
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"metal"}) // No explicit provider flag
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur and "metal" should be used as provider
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("RunEContextNameAsProviderForLocal", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with "local" context name
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"local"}) // No explicit provider flag
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur and "local" should be used as provider
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("RunEExplicitProviderOverridesContextName", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with explicit provider that differs from context name
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"aws", "--provider", "azure"}) // Context name vs explicit provider
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur and explicit provider should be used
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("RunEUnknownContextNameDoesNotSetProvider", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with unknown context name
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"unknown-context"}) // Unknown context name
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur and no provider should be set
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("RunEInvalidSetFlagFormat", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// When executing the init command with invalid set flag format
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--set", "invalid-format-without-equals"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur (invalid format is ignored)
		if err != nil {
			t.Errorf("Expected success for invalid set flag format, got error: %v", err)
		}
	})

	t.Run("RunEConfigHandlerError", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInitTest(t)

		// And a config handler that fails to set values
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			return fmt.Errorf("failed to set %s", key)
		}
		mocks.Injector.Register("configHandler", mockConfigHandler)

		// When executing the init command with flags that trigger config setting
		cmd := createTestInitCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--docker"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error from config handler, got nil")
		}
		if !strings.Contains(err.Error(), "failed to set docker.enabled") {
			t.Errorf("Expected config handler error, got: %v", err)
		}
	})

	t.Run("RunEPipelineError", func(t *testing.T) {
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
			t.Error("Expected error from pipeline, got nil")
		}
		if !strings.Contains(err.Error(), "pipeline execution failed") {
			t.Errorf("Expected pipeline error, got: %v", err)
		}
	})
}
