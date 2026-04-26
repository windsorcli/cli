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
)

// =============================================================================
// Test Setup
// =============================================================================

type DestroyMocks struct {
	ConfigHandler     config.ConfigHandler
	Shell             *shell.MockShell
	BlueprintHandler  *blueprint.MockBlueprintHandler
	TerraformStack    *terraforminfra.MockStack
	KubernetesManager *kubernetes.MockKubernetesManager
	ToolsManager      *tools.MockToolsManager
	Runtime           *runtime.Runtime
	TmpDir            string
}

func setupDestroyTest(t *testing.T, opts ...*SetupOptions) *DestroyMocks {
	t.Helper()

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

	testOpts := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		testOpts = opts[0]
	}
	testOpts.ConfigHandler = mockConfigHandler
	baseMocks := setupMocks(t, testOpts)
	tmpDir := baseMocks.TmpDir
	mockConfigHandler.GetConfigRootFunc = func() (string, error) { return tmpDir + "/contexts/test-context", nil }

	baseMocks.Shell.CheckTrustedDirectoryFunc = func() error { return nil }

	mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
	mockBlueprintHandler.LoadBlueprintFunc = func(...string) error { return nil }
	mockBlueprintHandler.WriteFunc = func(overwrite ...bool) error { return nil }
	testBlueprint := &blueprintv1alpha1.Blueprint{
		Metadata: blueprintv1alpha1.Metadata{Name: "test"},
		TerraformComponents: []blueprintv1alpha1.TerraformComponent{
			{Path: "cluster"},
		},
		Kustomizations: []blueprintv1alpha1.Kustomization{
			{Name: "my-app"},
		},
	}
	mockBlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return testBlueprint }

	mockTerraformStack := terraforminfra.NewMockStack()
	mockTerraformStack.DestroyAllFunc = func(bp *blueprintv1alpha1.Blueprint) error { return nil }
	mockTerraformStack.DestroyFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) error { return nil }

	mockKubernetesManager := kubernetes.NewMockKubernetesManager()
	mockKubernetesManager.DeleteBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error { return nil }
	mockKubernetesManager.DeleteKustomizationFunc = func(name, namespace string) error { return nil }

	rt := runtime.NewRuntime(&runtime.Runtime{
		Shell:         baseMocks.Shell,
		ConfigHandler: baseMocks.ConfigHandler,
		ProjectRoot:   tmpDir,
		ToolsManager:  baseMocks.ToolsManager,
		ContextName:   "test-context",
	})
	if rt == nil {
		t.Fatal("Failed to create runtime")
	}

	return &DestroyMocks{
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

// newDestroyProject wires mocks into a project for destroy tests.
func newDestroyProject(mocks *DestroyMocks) *project.Project {
	comp := composer.NewComposer(mocks.Runtime)
	comp.BlueprintHandler = mocks.BlueprintHandler
	mockProvisioner := provisioner.NewProvisioner(mocks.Runtime, comp.BlueprintHandler, &provisioner.Provisioner{
		TerraformStack:    mocks.TerraformStack,
		KubernetesManager: mocks.KubernetesManager,
	})
	return project.NewProject("", &project.Project{
		Runtime:     mocks.Runtime,
		Composer:    comp,
		Provisioner: mockProvisioner,
	})
}

// =============================================================================
// confirmDestroy Tests
// =============================================================================

func TestConfirmDestroy(t *testing.T) {
	t.Run("SuccessMatchingInput", func(t *testing.T) {
		r := strings.NewReader("production\n")
		w := io.Discard
		err := confirmDestroy(r, w, "This will destroy everything.", "production")
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithWhitespace", func(t *testing.T) {
		r := strings.NewReader("  production  \n")
		w := io.Discard
		err := confirmDestroy(r, w, "This will destroy everything.", "production")
		if err != nil {
			t.Errorf("Expected no error for trimmed whitespace, got: %v", err)
		}
	})

	t.Run("ErrorWrongInput", func(t *testing.T) {
		r := strings.NewReader("wrong\n")
		w := io.Discard
		err := confirmDestroy(r, w, "This will destroy everything.", "production")
		if err == nil {
			t.Error("Expected error for wrong input, got nil")
		}
		if !strings.Contains(err.Error(), "confirmation failed") {
			t.Errorf("Expected confirmation failed error, got: %v", err)
		}
	})

	t.Run("ErrorEmptyInput", func(t *testing.T) {
		r := strings.NewReader("\n")
		w := io.Discard
		err := confirmDestroy(r, w, "This will destroy everything.", "production")
		if err == nil {
			t.Error("Expected error for empty input, got nil")
		}
	})

	t.Run("ErrorNoInput", func(t *testing.T) {
		r := strings.NewReader("")
		w := io.Discard
		err := confirmDestroy(r, w, "This will destroy everything.", "production")
		if err == nil {
			t.Error("Expected error when no input provided, got nil")
		}
		if !strings.Contains(err.Error(), "confirmation aborted") {
			t.Errorf("Expected confirmation aborted error, got: %v", err)
		}
	})
}

// =============================================================================
// Test Cases
// =============================================================================

func TestDestroyCmd(t *testing.T) {
	createTestDestroyCmd := func() *cobra.Command {
		destroyConfirm = ""
		cmd := &cobra.Command{
			Use:  "destroy",
			RunE: destroyCmd.RunE,
		}
		destroyCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})
		destroyCmd.PersistentFlags().VisitAll(func(flag *pflag.Flag) {
			cmd.PersistentFlags().AddFlag(flag)
		})
		cmd.Args = destroyCmd.Args
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		return cmd
	}

	suppressProcessStdout(t)
	suppressProcessStderr(t)

	t.Run("SuccessDestroyAllWithConfirmFlag", func(t *testing.T) {
		// Given --confirm matches the context name, destroy all proceeds without a prompt.
		mocks := setupDestroyTest(t)
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=test-context"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorDestroyAllWithMismatchedConfirmFlag", func(t *testing.T) {
		// Given --confirm does not match the context name, destroy must refuse.
		mocks := setupDestroyTest(t)
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=wrong-context"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err == nil {
			t.Fatal("Expected confirmation error, got nil")
		}
		if !strings.Contains(err.Error(), "confirmation failed") {
			t.Errorf("Expected confirmation failed error, got: %v", err)
		}
	})

	t.Run("SuccessDestroyAllWithConfirmation", func(t *testing.T) {
		// Given correct interactive confirmation input.
		mocks := setupDestroyTest(t)
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetIn(strings.NewReader("test-context\n"))
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error with correct confirmation, got %v", err)
		}
	})

	t.Run("ErrorDestroyAllWrongConfirmation", func(t *testing.T) {
		// Given wrong interactive confirmation input.
		mocks := setupDestroyTest(t)
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetIn(strings.NewReader("wrong\n"))
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error for wrong confirmation, got nil")
		}
		if !strings.Contains(err.Error(), "confirmation failed") {
			t.Errorf("Expected confirmation failed error, got: %v", err)
		}
	})

	t.Run("SuccessDestroyComponentWithConfirmFlag", func(t *testing.T) {
		// Given --confirm matches the component name.
		mocks := setupDestroyTest(t)
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=cluster", "cluster"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorDestroyComponentWithMismatchedConfirmFlag", func(t *testing.T) {
		// Given --confirm does not match the component name.
		mocks := setupDestroyTest(t)
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=other", "cluster"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err == nil {
			t.Fatal("Expected confirmation error, got nil")
		}
		if !strings.Contains(err.Error(), "confirmation failed") {
			t.Errorf("Expected confirmation failed error, got: %v", err)
		}
	})

	t.Run("SuccessDestroyComponentWithConfirmation", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"cluster"})
		cmd.SetContext(ctx)
		cmd.SetIn(strings.NewReader("cluster\n"))
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error with correct confirmation, got %v", err)
		}
	})

	t.Run("ErrorComponentNotFound", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=nonexistent", "nonexistent"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error for unknown component, got nil")
		}
		if !strings.Contains(err.Error(), "not found in blueprint") {
			t.Errorf("Expected not-found error, got: %v", err)
		}
	})

	t.Run("ErrorCheckingTrustedDirectory", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not in trusted directory")
		}
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=test-context"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected trusted directory error, got: %v", err)
		}
	})

	t.Run("ErrorDestroyAllFails", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		mocks.KubernetesManager.DeleteBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("delete blueprint failed")
		}
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=test-context"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error destroying all components") {
			t.Errorf("Expected destroy error, got: %v", err)
		}
	})

	t.Run("CheckAuthFailureBlocksDestroyAllBeforeStateMigration", func(t *testing.T) {
		// Given expired/missing cloud credentials, destroy must fail at preflight rather than
		// after several minutes of init + state migration. This is the bug: a long destroy
		// would init every component, migrate state, then fail at the AWS provider.
		mocks := setupDestroyTest(t)
		mocks.ToolsManager.CheckAuthFunc = func() error { return fmt.Errorf("aws credentials did not resolve") }
		destroyAllCalled := false
		mocks.TerraformStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint) error {
			destroyAllCalled = true
			return nil
		}
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=test-context"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err == nil {
			t.Fatal("Expected credential preflight error, got nil")
		}
		if !strings.Contains(err.Error(), "aws credentials did not resolve") {
			t.Errorf("Expected pass-through credential error, got: %v", err)
		}
		if destroyAllCalled {
			t.Error("DestroyAll must not run when credential preflight fails")
		}
	})

	t.Run("CheckAuthFailureBlocksTerraformComponentDestroy", func(t *testing.T) {
		// Given a terraform component is targeted, the preflight must fire before migrate/destroy.
		mocks := setupDestroyTest(t)
		mocks.ToolsManager.CheckAuthFunc = func() error { return fmt.Errorf("aws credentials did not resolve") }
		destroyCalled := false
		mocks.TerraformStack.DestroyFunc = func(*blueprintv1alpha1.Blueprint, string) error {
			destroyCalled = true
			return nil
		}
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=cluster", "cluster"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err == nil {
			t.Fatal("Expected credential preflight error, got nil")
		}
		if !strings.Contains(err.Error(), "aws credentials did not resolve") {
			t.Errorf("Expected pass-through credential error, got: %v", err)
		}
		if destroyCalled {
			t.Error("Destroy must not run when credential preflight fails")
		}
	})

	t.Run("CheckAuthSkippedForKustomizeOnlyComponent", func(t *testing.T) {
		// Given a component that exists only in kustomize (not terraform), the cloud-credential
		// preflight must NOT fire — kustomize-only paths have no obligation to be authed.
		mocks := setupDestroyTest(t)
		checkAuthCalled := false
		mocks.ToolsManager.CheckAuthFunc = func() error {
			checkAuthCalled = true
			return fmt.Errorf("aws credentials did not resolve")
		}
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=my-app", "my-app"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected kustomize-only destroy to succeed without preflight, got %v", err)
		}
		if checkAuthCalled {
			t.Error("CheckAuth must not be invoked for a kustomize-only component destroy")
		}
	})
}

func TestDestroyTerraformCmd(t *testing.T) {
	createTestDestroyTerraformCmd := func() *cobra.Command {
		destroyConfirm = ""
		cmd := &cobra.Command{
			Use:  "terraform",
			RunE: destroyTerraformCmd.RunE,
		}
		destroyTerraformCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})
		destroyCmd.PersistentFlags().VisitAll(func(flag *pflag.Flag) {
			cmd.PersistentFlags().AddFlag(flag)
		})
		cmd.Args = destroyTerraformCmd.Args
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		return cmd
	}

	suppressProcessStdout(t)
	suppressProcessStderr(t)

	t.Run("SuccessAllWithConfirmFlag", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=test-context"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessAllWithConfirmation", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetIn(strings.NewReader("test-context\n"))
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error with correct confirmation, got %v", err)
		}
	})

	t.Run("SuccessSpecificWithConfirmFlag", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=cluster", "cluster"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessSpecificWithConfirmation", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"cluster"})
		cmd.SetContext(ctx)
		cmd.SetIn(strings.NewReader("cluster\n"))
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error with correct confirmation, got %v", err)
		}
	})

	t.Run("ErrorWrongConfirmation", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"cluster"})
		cmd.SetContext(ctx)
		cmd.SetIn(strings.NewReader("wrong\n"))
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error for wrong confirmation, got nil")
		}
		if !strings.Contains(err.Error(), "confirmation failed") {
			t.Errorf("Expected confirmation failed error, got: %v", err)
		}
	})

	t.Run("ErrorMismatchedConfirmFlag", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=other", "cluster"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err == nil {
			t.Fatal("Expected confirmation error, got nil")
		}
		if !strings.Contains(err.Error(), "confirmation failed") {
			t.Errorf("Expected confirmation failed error, got: %v", err)
		}
	})

	t.Run("ErrorCheckingTrustedDirectory", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not in trusted directory")
		}
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=cluster", "cluster"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected trusted directory error, got: %v", err)
		}
	})

	t.Run("ErrorDestroyFails", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		mocks.TerraformStack.DestroyFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) error {
			return fmt.Errorf("terraform destroy failed")
		}
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=cluster", "cluster"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error destroying terraform") {
			t.Errorf("Expected destroy error, got: %v", err)
		}
	})
}

func TestDestroyKustomizeCmd(t *testing.T) {
	createTestDestroyKustomizeCmd := func() *cobra.Command {
		destroyConfirm = ""
		cmd := &cobra.Command{
			Use:  "kustomize",
			RunE: destroyKustomizeCmd.RunE,
		}
		destroyKustomizeCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})
		destroyCmd.PersistentFlags().VisitAll(func(flag *pflag.Flag) {
			cmd.PersistentFlags().AddFlag(flag)
		})
		cmd.Args = destroyKustomizeCmd.Args
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		return cmd
	}

	suppressProcessStdout(t)
	suppressProcessStderr(t)

	t.Run("SuccessAllWithConfirmFlag", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyKustomizeCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=test-context"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessAllWithConfirmation", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyKustomizeCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetIn(strings.NewReader("test-context\n"))
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error with correct confirmation, got %v", err)
		}
	})

	t.Run("SuccessSpecificWithConfirmFlag", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyKustomizeCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=my-app", "my-app"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessSpecificWithConfirmation", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyKustomizeCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"my-app"})
		cmd.SetContext(ctx)
		cmd.SetIn(strings.NewReader("my-app\n"))
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error with correct confirmation, got %v", err)
		}
	})

	t.Run("ErrorWrongConfirmation", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyKustomizeCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"my-app"})
		cmd.SetContext(ctx)
		cmd.SetIn(strings.NewReader("wrong\n"))
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error for wrong confirmation, got nil")
		}
		if !strings.Contains(err.Error(), "confirmation failed") {
			t.Errorf("Expected confirmation failed error, got: %v", err)
		}
	})

	t.Run("ErrorMismatchedConfirmFlag", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyKustomizeCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=other", "my-app"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err == nil {
			t.Fatal("Expected confirmation error, got nil")
		}
		if !strings.Contains(err.Error(), "confirmation failed") {
			t.Errorf("Expected confirmation failed error, got: %v", err)
		}
	})

	t.Run("ErrorCheckingTrustedDirectory", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not in trusted directory")
		}
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyKustomizeCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=my-app", "my-app"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected trusted directory error, got: %v", err)
		}
	})

	t.Run("ErrorDestroyFails", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		mocks.KubernetesManager.DeleteKustomizationFunc = func(name, namespace string) error {
			return fmt.Errorf("kustomization not found")
		}
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyKustomizeCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=my-app", "my-app"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error destroying kustomization") {
			t.Errorf("Expected destroy error, got: %v", err)
		}
	})
}
