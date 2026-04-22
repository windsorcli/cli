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

	t.Run("MigratesStateToLocalBeforeDestroyAll", func(t *testing.T) {
		// Given a context configured for a remote backend (e.g. S3). Before tearing
		// anything down the destroy flow must pull state back to local — otherwise
		// destroying backend/s3 itself would try to delete the bucket while its own
		// state file still lives inside it.
		mocks := setupDestroyTest(t)
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "s3"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		var ops []string
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				ops = append(ops, fmt.Sprintf("set:%v", value))
			}
			return nil
		}
		mocks.TerraformStack.MigrateStateFunc = func(bp *blueprintv1alpha1.Blueprint) error {
			ops = append(ops, "migrate")
			return nil
		}
		mocks.TerraformStack.DestroyAllFunc = func(bp *blueprintv1alpha1.Blueprint) error {
			ops = append(ops, "destroy")
			return nil
		}

		proj := newDestroyProject(mocks)
		cmd := createTestDestroyTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=test-context"})
		cmd.SetContext(ctx)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the sequence must be: override backend to local, migrate state to
		// local, run destroy, restore the originally configured backend.
		expected := []string{"set:local", "migrate", "destroy", "set:s3"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected %d ops %v, got %d %v", len(expected), expected, len(ops), ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
	})

	t.Run("MigrationFailureAbortsDestroyAndRestoresBackend", func(t *testing.T) {
		// Given pre-destroy state migration fails (e.g. remote backend unreachable),
		// the destroy pass must not run — otherwise we'd attempt teardown against an
		// inconsistent or empty local state. The originally configured backend must
		// still be restored so subsequent windsor invocations see the real config.
		mocks := setupDestroyTest(t)
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "s3"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		var ops []string
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				ops = append(ops, fmt.Sprintf("set:%v", value))
			}
			return nil
		}
		mocks.TerraformStack.MigrateStateFunc = func(bp *blueprintv1alpha1.Blueprint) error {
			ops = append(ops, "migrate-fail")
			return fmt.Errorf("remote backend unreachable")
		}
		destroyCalled := false
		mocks.TerraformStack.DestroyAllFunc = func(bp *blueprintv1alpha1.Blueprint) error {
			destroyCalled = true
			return nil
		}

		proj := newDestroyProject(mocks)
		cmd := createTestDestroyTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=test-context"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err == nil {
			t.Fatal("Expected migration error to surface, got nil")
		}
		if !strings.Contains(err.Error(), "failed to migrate state to local before destroy") {
			t.Errorf("Expected migration wrapper error, got %v", err)
		}
		if destroyCalled {
			t.Error("Destroy must not run after migration failure")
		}
		// Backend override must be restored on the failure path.
		expected := []string{"set:local", "migrate-fail", "set:s3"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected %d ops %v, got %d %v", len(expected), expected, len(ops), ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
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
