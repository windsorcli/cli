package cmd_test

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"

	cliCmd "github.com/windsorcli/cli/cmd"

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
	mockTerraformStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
		return false, nil
	}

	// Add kubernetes manager mock
	mockKubernetesManager := kubernetes.NewMockKubernetesManager()
	mockKubernetesManager.ApplyBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error { return nil }
	mockKubernetesManager.WaitForKustomizationsFunc = func(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, layers kubernetes.KustomizationReporter) error {
		return nil
	}

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
			RunE:  cliCmd.UpCmd.RunE,
		}

		// Copy all flags from upCmd to ensure they're available
		cliCmd.UpCmd.Flags().VisitAll(func(flag *pflag.Flag) {
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
		ctx := context.WithValue(context.Background(), cliCmd.ProjectOverridesKey, proj)
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
		ctx := context.WithValue(context.Background(), cliCmd.ProjectOverridesKey, proj)
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
		ctx := context.WithValue(context.Background(), cliCmd.ProjectOverridesKey, proj)
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
		ctx := context.WithValue(context.Background(), cliCmd.ProjectOverridesKey, proj)
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
		mocks.TerraformStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			return false, fmt.Errorf("terraform stack up failed")
		}
		proj := newUpTestProject(mocks, true)

		// When executing the up command
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), cliCmd.ProjectOverridesKey, proj)
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
		ctx := context.WithValue(context.Background(), cliCmd.ProjectOverridesKey, proj)
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

	t.Run("BareUpDoesNotIssueExtraSaveConfig", func(t *testing.T) {
		// Given a workstation project and no flags overriding config
		mocks := setupUpTest(t)
		var overwriteArgs [][]bool
		mocks.ConfigHandler.(*config.MockConfigHandler).SaveConfigFunc = func(overwrite ...bool) error {
			overwriteArgs = append(overwriteArgs, append([]bool(nil), overwrite...))
			return nil
		}
		proj := newUpTestProject(mocks, true)

		// When executing a bare `windsor up`
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), cliCmd.ProjectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}

		// Then SaveConfig is invoked exactly once (by Initialize) with overwrite=false;
		// no extra explicit save fires from the up command.
		if len(overwriteArgs) != 1 {
			t.Fatalf("Expected 1 SaveConfig call from Initialize only, got %d: %v", len(overwriteArgs), overwriteArgs)
		}
		if len(overwriteArgs[0]) == 0 || overwriteArgs[0][0] {
			t.Errorf("Expected Initialize SaveConfig overwrite=false, got %v", overwriteArgs[0])
		}
	})

	t.Run("SetFlagAddsOverwriteSaveConfig", func(t *testing.T) {
		// Given a workstation project and a --set flag override
		mocks := setupUpTest(t)
		var overwriteArgs [][]bool
		mocks.ConfigHandler.(*config.MockConfigHandler).SaveConfigFunc = func(overwrite ...bool) error {
			overwriteArgs = append(overwriteArgs, append([]bool(nil), overwrite...))
			return nil
		}
		proj := newUpTestProject(mocks, true)

		// When executing `windsor up --set foo=bar`
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), cliCmd.ProjectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"--set", "foo=bar"})
		t.Cleanup(func() { *cliCmd.UpSetFlags = nil })
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}

		// Then SaveConfig is called twice: once by Initialize (overwrite=false), then
		// explicitly by up to elevate values.yaml replacement (overwrite=true).
		if len(overwriteArgs) != 2 {
			t.Fatalf("Expected 2 SaveConfig calls, got %d: %v", len(overwriteArgs), overwriteArgs)
		}
		last := overwriteArgs[1]
		if len(last) == 0 || !last[0] {
			t.Errorf("Expected explicit SaveConfig overwrite=true, got %v", last)
		}
	})

	t.Run("WorkstationFlagAloneSkipsExplicitSaveConfig", func(t *testing.T) {
		// Given a workstation project and a --vm-driver flag (no --set)
		mocks := setupUpTest(t)
		var overwriteArgs [][]bool
		mocks.ConfigHandler.(*config.MockConfigHandler).SaveConfigFunc = func(overwrite ...bool) error {
			overwriteArgs = append(overwriteArgs, append([]bool(nil), overwrite...))
			return nil
		}
		proj := newUpTestProject(mocks, true)

		// When executing `windsor up --vm-driver=docker-desktop`
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), cliCmd.ProjectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"--vm-driver=docker-desktop"})
		t.Cleanup(func() { *cliCmd.UpVmDriver = "" })
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}

		// Then SaveConfig fires once via Initialize only — --vm-driver alone does not
		// require an overwrite re-save (workstation keys are persisted via workstation.yaml).
		if len(overwriteArgs) != 1 {
			t.Fatalf("Expected 1 SaveConfig call (Initialize only), got %d: %v", len(overwriteArgs), overwriteArgs)
		}
	})

	t.Run("WorkstationDisabledStillPersistsSetFlags", func(t *testing.T) {
		// Given a project with no workstation but --set provided
		mocks := setupUpTest(t)
		var overwriteArgs [][]bool
		mocks.ConfigHandler.(*config.MockConfigHandler).SaveConfigFunc = func(overwrite ...bool) error {
			overwriteArgs = append(overwriteArgs, append([]bool(nil), overwrite...))
			return nil
		}
		proj := newUpTestProject(mocks, false)

		// When executing `windsor up --set foo=bar`
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), cliCmd.ProjectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"--set", "foo=bar"})
		t.Cleanup(func() { *cliCmd.UpSetFlags = nil })
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}

		// Then the --set values must still land in values.yaml (overwrite=true)
		// before the no-workstation guard short-circuits the rest of the flow.
		if len(overwriteArgs) != 2 {
			t.Fatalf("Expected 2 SaveConfig calls (Initialize + explicit --set save), got %d: %v", len(overwriteArgs), overwriteArgs)
		}
		last := overwriteArgs[1]
		if len(last) == 0 || !last[0] {
			t.Errorf("Expected explicit SaveConfig overwrite=true, got %v", last)
		}
	})

	t.Run("WorkstationDisabledBareUpSkipsExplicitSaveConfig", func(t *testing.T) {
		// Given a project with no workstation and no flags
		mocks := setupUpTest(t)
		var overwriteArgs [][]bool
		mocks.ConfigHandler.(*config.MockConfigHandler).SaveConfigFunc = func(overwrite ...bool) error {
			overwriteArgs = append(overwriteArgs, append([]bool(nil), overwrite...))
			return nil
		}
		proj := newUpTestProject(mocks, false)

		// When executing a bare `windsor up`
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), cliCmd.ProjectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}

		// Then SaveConfig fires only via Initialize (overwrite=false); no extra
		// explicit save runs since --set was not provided.
		if len(overwriteArgs) != 1 {
			t.Fatalf("Expected 1 SaveConfig call from Initialize only, got %d: %v", len(overwriteArgs), overwriteArgs)
		}
		if len(overwriteArgs[0]) == 0 || overwriteArgs[0][0] {
			t.Errorf("Expected Initialize SaveConfig overwrite=false, got %v", overwriteArgs[0])
		}
	})

	t.Run("ProvisionerWaitError", func(t *testing.T) {
		// Given a kubernetes manager whose WaitForKustomizations fails
		mocks := setupUpTest(t)
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, layers kubernetes.KustomizationReporter) error {
			return fmt.Errorf("wait for kustomizations failed")
		}
		proj := newUpTestProject(mocks, true)

		// When executing the up command with --wait flag
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), cliCmd.ProjectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"--wait"})
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "kustomizations not ready") {
			t.Errorf("Expected wait error, got: %v", err)
		}
	})

	t.Run("InProgressIsAWarningWhenBlueprintHasMessages", func(t *testing.T) {
		// Given a blueprint with post-run messages and a wait that times out on a kustomization in progress
		mocks := setupUpTest(t)
		mocks.BlueprintHandler.GenerateResolvedFunc = func() (*blueprintv1alpha1.Blueprint, error) {
			return &blueprintv1alpha1.Blueprint{
				Metadata: blueprintv1alpha1.Metadata{Name: "test"},
				Messages: []blueprintv1alpha1.Message{{Text: "Delegate ns1.example at your registrar."}},
			}, nil
		}
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, layers kubernetes.KustomizationReporter) error {
			return fmt.Errorf("timeout waiting for kustomizations: dns")
		}
		proj := newUpTestProject(mocks, true)

		// When executing the up command
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), cliCmd.ProjectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then up succeeds
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("FailedKustomizationFailsWhenBlueprintHasMessages", func(t *testing.T) {
		// Given a blueprint with post-run messages and a wait that fails on a BuildFailed kustomization
		mocks := setupUpTest(t)
		mocks.BlueprintHandler.GenerateResolvedFunc = func() (*blueprintv1alpha1.Blueprint, error) {
			return &blueprintv1alpha1.Blueprint{
				Metadata: blueprintv1alpha1.Metadata{Name: "test"},
				Messages: []blueprintv1alpha1.Message{{Text: "Delegate ns1.example at your registrar."}},
			}, nil
		}
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, layers kubernetes.KustomizationReporter) error {
			return fmt.Errorf("%w: kustomization observability failed (BuildFailed)", kubernetes.ErrKustomizationFailed)
		}
		proj := newUpTestProject(mocks, true)

		// When executing the up command
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), cliCmd.ProjectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then up fails and names the failed kustomization
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "observability") {
			t.Errorf("Expected the error to name the kustomization, got: %v", err)
		}
	})

	t.Run("CheckAuthFailureBlocksUpBeforeProjectUp", func(t *testing.T) {
		// Given expired credentials, up must fail at preflight before kicking off the
		// workstation/terraform path.
		mocks := setupUpTest(t)
		mocks.ToolsManager.CheckAuthFunc = func() error { return fmt.Errorf("aws credentials did not resolve") }
		upCalled := false
		mocks.TerraformStack.UpFunc = func(*blueprintv1alpha1.Blueprint, ...func(context.Context, string) (bool, error)) (bool, error) {
			upCalled = true
			return false, nil
		}
		proj := newUpTestProject(mocks, true)

		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), cliCmd.ProjectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err == nil {
			t.Fatal("Expected credential preflight error, got nil")
		}
		if !strings.Contains(err.Error(), "aws credentials did not resolve") {
			t.Errorf("Expected pass-through credential error, got: %v", err)
		}
		if upCalled {
			t.Error("Provisioner.Up must not run when credential preflight fails")
		}
	})

	t.Run("HaltedUpSkipsBlueprintInstallAndSuccessLine", func(t *testing.T) {
		// Given a project where the terraform stack halts after the workstation component
		// (the apply hook needed host configuration the operator hasn't done yet)
		mocks := setupUpTest(t)
		applyBlueprintCalled := false
		mocks.TerraformStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(context.Context, string) (bool, error)) (bool, error) {
			return true, nil
		}
		mocks.KubernetesManager.ApplyBlueprintFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error {
			applyBlueprintCalled = true
			return nil
		}
		proj := newUpTestProject(mocks, true)

		stderrBuf, restore := captureProcessStderr(t)
		t.Cleanup(restore)

		// When executing up
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), cliCmd.ProjectOverridesKey, proj)
		cmd.SetContext(ctx)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("expected halt to exit 0 (clean stop, not failure), got error: %v", err)
		}
		restore()

		// Then blueprint install is skipped and the success line is suppressed
		if applyBlueprintCalled {
			t.Error("expected blueprint install to be skipped after halt")
		}
		stderr := stderrBuf.String()
		if strings.Contains(stderr, "Windsor environment set up successfully") {
			t.Errorf("expected success line to be suppressed after halt, got: %q", stderr)
		}
	})

	t.Run("SuccessLineShowsTheElapsedTime", func(t *testing.T) {
		// Given a project with a workstation configured
		mocks := setupUpTest(t)
		proj := newUpTestProject(mocks, true)

		stderrBuf, restore := captureProcessStderr(t)
		t.Cleanup(restore)

		// When executing up
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), cliCmd.ProjectOverridesKey, proj)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		restore()

		// Then the success line names the wall-clock time of the run
		if !regexp.MustCompile(`Windsor environment set up successfully in \d+s\.\n`).MatchString(stderrBuf.String()) {
			t.Errorf("Expected success line with elapsed time, got: %q", stderrBuf.String())
		}
	})
}
