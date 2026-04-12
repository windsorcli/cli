package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/provisioner"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
	"github.com/windsorcli/cli/pkg/workstation"
)

// =============================================================================
// Test Setup
// =============================================================================

type UpMocks struct {
	ConfigHandler     config.ConfigHandler
	Shell             *shell.MockShell
	Shims             *Shims
	BlueprintHandler  *blueprint.MockBlueprintHandler
	TerraformStack    *terraforminfra.MockStack
	KubernetesManager *kubernetes.MockKubernetesManager
	ToolsManager      *tools.MockToolsManager
	Runtime           *runtime.Runtime
	TmpDir            string
}

func setupUpTest(t *testing.T, opts ...*SetupOptions) *UpMocks {
	t.Helper()

	// Create mock config handler to control IsDevMode
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetContextFunc = func() string { return "test-context" }
	mockConfigHandler.IsDevModeFunc = func(contextName string) bool { return false }
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		switch key {
		case "terraform.enabled":
			return true
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
	}
	mockConfigHandler.IsLoadedFunc = func() bool { return true }
	mockConfigHandler.LoadConfigFunc = func() error { return nil }
	mockConfigHandler.SaveConfigFunc = func(hasSetFlags ...bool) error { return nil }
	mockConfigHandler.GenerateContextIDFunc = func() error { return nil }

	// Get base mocks with mock config handler
	testOpts := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		testOpts = opts[0]
	}
	testOpts.ConfigHandler = mockConfigHandler
	baseMocks := setupMocks(t, testOpts)
	tmpDir := baseMocks.TmpDir
	mockConfigHandler.GetConfigRootFunc = func() (string, error) { return tmpDir + "/contexts/test-context", nil }

	// Add up-specific shell mock behaviors
	baseMocks.Shell.CheckTrustedDirectoryFunc = func() error { return nil }

	// Add blueprint handler mock
	mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
	mockBlueprintHandler.LoadBlueprintFunc = func(...string) error { return nil }
	mockBlueprintHandler.WriteFunc = func(overwrite ...bool) error { return nil }
	testBlueprint := &blueprintv1alpha1.Blueprint{
		Metadata: blueprintv1alpha1.Metadata{Name: "test"},
	}
	mockBlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return testBlueprint }

	// Add terraform stack mock
	mockTerraformStack := terraforminfra.NewMockStack()
	mockTerraformStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error { return nil }

	// Add kubernetes manager mock
	mockKubernetesManager := kubernetes.NewMockKubernetesManager()
	mockKubernetesManager.ApplyBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error { return nil }
	mockKubernetesManager.WaitForKustomizationsFunc = func(message string, blueprint *blueprintv1alpha1.Blueprint) error { return nil }

	// Create runtime with all mocked dependencies
	rt := runtime.NewRuntime(&runtime.Runtime{
		Shell:         baseMocks.Shell,
		ConfigHandler: baseMocks.ConfigHandler,
		ProjectRoot:   tmpDir,
		ToolsManager:  baseMocks.ToolsManager,
	})
	if rt == nil {
		t.Fatal("Failed to create runtime")
	}

	return &UpMocks{
		ConfigHandler:     baseMocks.ConfigHandler,
		Shell:             baseMocks.Shell,
		Shims:             baseMocks.Shims,
		BlueprintHandler:  mockBlueprintHandler,
		TerraformStack:    mockTerraformStack,
		KubernetesManager: mockKubernetesManager,
		ToolsManager:      baseMocks.ToolsManager,
		Runtime:           rt,
		TmpDir:            tmpDir,
	}
}

// newUpTestProject builds a project override with the given mocks and an optional workstation.
func newUpTestProject(mocks *UpMocks, withWorkstation bool) *project.Project {
	comp := composer.NewComposer(mocks.Runtime)
	comp.BlueprintHandler = mocks.BlueprintHandler
	mockProvisioner := provisioner.NewProvisioner(mocks.Runtime, comp.BlueprintHandler, &provisioner.Provisioner{
		TerraformStack:    mocks.TerraformStack,
		KubernetesManager: mocks.KubernetesManager,
	})
	proj := project.NewProject("", &project.Project{
		Runtime:     mocks.Runtime,
		Composer:    comp,
		Provisioner: mockProvisioner,
	})
	if withWorkstation {
		proj.Workstation = workstation.NewWorkstation(mocks.Runtime)
	}
	return proj
}

// =============================================================================
// Test Cases
// =============================================================================

func TestUpCmd(t *testing.T) {
	createTestUpCmd := func() *cobra.Command {
		// Create a new command with the same RunE as upCmd
		cmd := &cobra.Command{
			Use:   "up",
			Short: "Bring up the local workstation environment",
			RunE:  upCmd.RunE,
		}

		// Copy all flags from upCmd to ensure they're available
		upCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})

		// Disable help text printing
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)

		return cmd
	}

	suppressProcessStdout(t)
	suppressProcessStderr(t)

	t.Run("Success", func(t *testing.T) {
		// Given a project with a workstation configured
		mocks := setupUpTest(t)
		proj := newUpTestProject(mocks, true)

		// When executing the up command
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("NoOpWhenWorkstationDisabled", func(t *testing.T) {
		// Given a project with no workstation configured
		mocks := setupUpTest(t)
		proj := newUpTestProject(mocks, false)

		var stderrBuf strings.Builder
		cmd := createTestUpCmd()
		cmd.SetErr(&stderrBuf)
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur and a descriptive message is printed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithWait", func(t *testing.T) {
		// Given a project with a workstation configured
		mocks := setupUpTest(t)
		proj := newUpTestProject(mocks, true)

		// When executing the up command with --wait flag
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"--wait"})
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("CheckTrustedDirectoryError", func(t *testing.T) {
		// Given a shell that rejects the trusted directory check
		mocks := setupUpTest(t)
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not in trusted directory")
		}
		proj := newUpTestProject(mocks, false)

		// When executing the up command
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected trusted directory error, got: %v", err)
		}
	})

	t.Run("ProvisionerUpError", func(t *testing.T) {
		// Given a terraform stack that fails during Up
		mocks := setupUpTest(t)
		mocks.TerraformStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error {
			return fmt.Errorf("terraform stack up failed")
		}
		proj := newUpTestProject(mocks, true)

		// When executing the up command
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})
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
		// Given a kubernetes manager whose ApplyBlueprint fails
		mocks := setupUpTest(t)
		mocks.KubernetesManager.ApplyBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("kubernetes apply failed")
		}
		proj := newUpTestProject(mocks, true)

		// When executing the up command (install always runs)
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})
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
		// Given a kubernetes manager whose WaitForKustomizations fails
		mocks := setupUpTest(t)
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(message string, blueprint *blueprintv1alpha1.Blueprint) error {
			return fmt.Errorf("wait for kustomizations failed")
		}
		proj := newUpTestProject(mocks, true)

		// When executing the up command with --wait flag
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"--wait"})
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "error waiting for kustomizations") {
			t.Errorf("Expected wait error, got: %v", err)
		}
	})
}
