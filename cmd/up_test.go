package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime/config"
	envvars "github.com/windsorcli/cli/pkg/runtime/env"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Test Setup
// =============================================================================

type UpMocks struct {
	Injector          di.Injector
	ConfigHandler     config.ConfigHandler
	Shell             *shell.MockShell
	Shims             *Shims
	BlueprintHandler  *blueprint.MockBlueprintHandler
	TerraformStack    *terraforminfra.MockStack
	KubernetesManager *kubernetes.MockKubernetesManager
}

func setupUpTest(t *testing.T, opts ...*SetupOptions) *UpMocks {
	t.Helper()

	// Set up temporary directory and change to it
	tmpDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	t.Cleanup(func() { os.Chdir(oldDir) })

	// Create mock config handler to control IsDevMode
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.InitializeFunc = func() error { return nil }
	mockConfigHandler.GetContextFunc = func() string { return "test-context" }
	mockConfigHandler.IsDevModeFunc = func(contextName string) bool { return false }
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return false
	}
	mockConfigHandler.IsLoadedFunc = func() bool { return true }
	mockConfigHandler.LoadConfigFunc = func() error { return nil }
	mockConfigHandler.SaveConfigFunc = func(hasSetFlags ...bool) error { return nil }
	mockConfigHandler.GenerateContextIDFunc = func() error { return nil }
	mockConfigHandler.GetConfigRootFunc = func() (string, error) { return tmpDir + "/contexts/test-context", nil }

	// Get base mocks with mock config handler
	testOpts := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		testOpts = opts[0]
	}
	testOpts.ConfigHandler = mockConfigHandler
	baseMocks := setupMocks(t, testOpts)

	// Add up-specific shell mock behaviors
	baseMocks.Shell.CheckTrustedDirectoryFunc = func() error { return nil }

	// Add blueprint handler mock
	mockBlueprintHandler := blueprint.NewMockBlueprintHandler(baseMocks.Injector)
	mockBlueprintHandler.InitializeFunc = func() error { return nil }
	mockBlueprintHandler.LoadBlueprintFunc = func() error { return nil }
	mockBlueprintHandler.WriteFunc = func(overwrite ...bool) error { return nil }
	testBlueprint := &blueprintv1alpha1.Blueprint{
		Metadata: blueprintv1alpha1.Metadata{Name: "test"},
	}
	mockBlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return testBlueprint }
	baseMocks.Injector.Register("blueprintHandler", mockBlueprintHandler)

	// Add terraform stack mock
	mockTerraformStack := terraforminfra.NewMockStack(baseMocks.Injector)
	mockTerraformStack.InitializeFunc = func() error { return nil }
	mockTerraformStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint) error { return nil }
	mockTerraformStack.DownFunc = func(blueprint *blueprintv1alpha1.Blueprint) error { return nil }
	baseMocks.Injector.Register("terraformStack", mockTerraformStack)

	// Add kubernetes manager mock
	mockKubernetesManager := kubernetes.NewMockKubernetesManager(baseMocks.Injector)
	mockKubernetesManager.InitializeFunc = func() error { return nil }
	mockKubernetesManager.ApplyBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error { return nil }
	mockKubernetesManager.WaitForKustomizationsFunc = func(message string, names ...string) error { return nil }
	baseMocks.Injector.Register("kubernetesManager", mockKubernetesManager)

	// Add terraform env printer (required by terraform stack)
	terraformEnvPrinter := envvars.NewTerraformEnvPrinter(baseMocks.Injector)
	baseMocks.Injector.Register("terraformEnv", terraformEnvPrinter)

	// Add mock tools manager (required by runInit)
	mockToolsManager := tools.NewMockToolsManager()
	mockToolsManager.InitializeFunc = func() error { return nil }
	mockToolsManager.CheckFunc = func() error { return nil }
	mockToolsManager.InstallFunc = func() error { return nil }
	baseMocks.Injector.Register("toolsManager", mockToolsManager)

	return &UpMocks{
		Injector:          baseMocks.Injector,
		ConfigHandler:     baseMocks.ConfigHandler,
		Shell:             baseMocks.Shell,
		Shims:             baseMocks.Shims,
		BlueprintHandler:  mockBlueprintHandler,
		TerraformStack:    mockTerraformStack,
		KubernetesManager: mockKubernetesManager,
	}
}

// =============================================================================
// Test Cases
// =============================================================================

func TestUpCmd(t *testing.T) {
	createTestUpCmd := func() *cobra.Command {
		// Create a new command with the same RunE as upCmd
		cmd := &cobra.Command{
			Use:   "up",
			Short: "Set up the Windsor environment",
			RunE:  upCmd.RunE,
		}

		// Copy all flags from upCmd to ensure they're available
		upCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})

		// Disable help text printing
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true

		return cmd
	}

	t.Run("Success", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupUpTest(t)

		// When executing the up command
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithInstallFlag", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupUpTest(t)

		// When executing the up command with install flag
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--install"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithWaitFlag", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupUpTest(t)

		// When executing the up command with wait flag
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--wait"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur (wait only works with install)
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithInstallAndWaitFlags", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupUpTest(t)

		// When executing the up command with both install and wait flags
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--install", "--wait"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("CheckTrustedDirectoryError", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupUpTest(t)

		// And CheckTrustedDirectory that fails
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not in trusted directory")
		}

		// When executing the up command
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected trusted directory error, got: %v", err)
		}
	})

	t.Run("ProvisionerUpError", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupUpTest(t)

		// And terraform stack Up that fails
		mocks.TerraformStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint) error {
			return fmt.Errorf("terraform stack up failed")
		}

		// When executing the up command
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "error starting infrastructure") {
			t.Errorf("Expected infrastructure error, got: %v", err)
		}
	})

	t.Run("ProvisionerInstallError", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupUpTest(t)

		// And kubernetes manager ApplyBlueprint that fails
		mocks.KubernetesManager.ApplyBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("kubernetes apply failed")
		}

		// When executing the up command with install flag
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--install"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "error installing blueprint") {
			t.Errorf("Expected install error, got: %v", err)
		}
	})

	t.Run("ProvisionerWaitError", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupUpTest(t)

		// And kubernetes manager WaitForKustomizations that fails
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(message string, names ...string) error {
			return fmt.Errorf("wait for kustomizations failed")
		}

		// When executing the up command with install and wait flags
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--install", "--wait"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error waiting for kustomizations") {
			t.Errorf("Expected wait error, got: %v", err)
		}
	})
}
