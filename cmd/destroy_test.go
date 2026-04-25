package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
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
	mockTerraformStack.DestroyAllFunc = func(bp *blueprintv1alpha1.Blueprint, excludeIDs ...string) ([]string, error) { return nil, nil }
	mockTerraformStack.DestroyFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) (bool, error) { return false, nil }

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
		mocks.TerraformStack.DestroyFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
			return false, fmt.Errorf("terraform destroy failed")
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

	t.Run("DestroysNonBackendThenMigratesAndDestroysBackend", func(t *testing.T) {
		// Given a blueprint that declares both a "backend" and a non-backend
		// terraform component, and the configured backend is non-local (s3), the
		// new symmetric flow must: destroy non-backend components first against the
		// live remote backend (excludeIDs="backend"), then pin backend.type=local,
		// migrate just the backend component's state, destroy it, and restore the
		// configured backend on defer. The old "migrate everything to local first"
		// flow is gone — the rest of the bucket's state stays remote until the
		// non-backend layers are torn down.
		mocks := setupDestroyTest(t)
		bpWithBackend := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
		}
		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return bpWithBackend }

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
		mocks.TerraformStack.DestroyAllFunc = func(bp *blueprintv1alpha1.Blueprint, excludeIDs ...string) ([]string, error) {
			ops = append(ops, fmt.Sprintf("destroyAll:exclude=%v", excludeIDs))
			return nil, nil
		}
		mocks.TerraformStack.MigrateComponentStateFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) error {
			ops = append(ops, fmt.Sprintf("migrate:%s", componentID))
			return nil
		}
		mocks.TerraformStack.DestroyFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
			ops = append(ops, fmt.Sprintf("destroy:%s", componentID))
			return false, nil
		}

		proj := newDestroyProject(mocks)
		cmd := createTestDestroyTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=test-context"})
		cmd.SetContext(ctx)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := []string{
			"destroyAll:exclude=[backend]",
			"set:local",
			"migrate:backend",
			"destroy:backend",
			"set:s3",
		}
		if len(ops) != len(expected) {
			t.Fatalf("Expected %d ops %v, got %d %v", len(expected), expected, len(ops), ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
	})

	t.Run("BackendMigrationFailureAbortsBackendDestroyAndRestoresBackend", func(t *testing.T) {
		// Given the backend component's state migration fails (e.g. remote backend
		// unreachable), the backend's destroy must not run — destroy against a
		// half-migrated state would corrupt the bucket teardown. Non-backend
		// components have already been successfully destroyed at this point, so
		// the surfaced error names the backend migration failure specifically; the
		// configured backend is restored via defer for subsequent operations.
		mocks := setupDestroyTest(t)
		bpWithBackend := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
		}
		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return bpWithBackend }

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
		mocks.TerraformStack.DestroyAllFunc = func(bp *blueprintv1alpha1.Blueprint, excludeIDs ...string) ([]string, error) {
			ops = append(ops, "destroyAll")
			return nil, nil
		}
		mocks.TerraformStack.MigrateComponentStateFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) error {
			ops = append(ops, "migrate-fail")
			return fmt.Errorf("remote backend unreachable")
		}
		destroyCalled := false
		mocks.TerraformStack.DestroyFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
			destroyCalled = true
			return false, nil
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
		if !strings.Contains(err.Error(), "remote backend unreachable") {
			t.Errorf("Expected underlying migration cause in surfaced message, got %v", err)
		}
		if destroyCalled {
			t.Error("Backend Destroy must not run after MigrateComponentState fails")
		}
		expected := []string{"destroyAll", "set:local", "migrate-fail", "set:s3"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected %d ops %v, got %d %v", len(expected), expected, len(ops), ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
	})

	t.Run("BackendRestoreFailureEmitsStderrWarning", func(t *testing.T) {
		// Given the deferred restore (ch.Set with the original backend value) fails
		// after a successful backend destroy, the error must surface on stderr so
		// the operator notices that subsequent commands in the same process will
		// see backend.type stuck on "local". Destroy itself has already succeeded,
		// so the command exits zero — but silent restore failure would be a
		// debugging black hole.
		mocks := setupDestroyTest(t)
		bpWithBackend := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
		}
		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return bpWithBackend }
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
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" && value == "s3" {
				return fmt.Errorf("mock restore failure")
			}
			return nil
		}

		r, w, pipeErr := os.Pipe()
		if pipeErr != nil {
			t.Fatalf("Pipe failed: %v", pipeErr)
		}
		origStderr := os.Stderr
		os.Stderr = w
		defer func() { os.Stderr = origStderr }()

		proj := newDestroyProject(mocks)
		cmd := createTestDestroyTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--confirm=test-context"})
		cmd.SetContext(ctx)
		execErr := cmd.Execute()

		w.Close()
		stderrBytes, _ := io.ReadAll(r)
		stderrOutput := string(stderrBytes)

		if execErr != nil {
			t.Fatalf("Expected destroy to succeed despite restore failure, got %v", execErr)
		}
		if !strings.Contains(stderrOutput, "failed to restore terraform.backend.type") {
			t.Errorf("Expected stderr warning about restore failure, got: %q", stderrOutput)
		}
		if !strings.Contains(stderrOutput, "mock restore failure") {
			t.Errorf("Expected stderr warning to include underlying cause, got: %q", stderrOutput)
		}
	})

	t.Run("SkipsBackendDanceWhenNoBackendComponent", func(t *testing.T) {
		// Given a blueprint with no backend component, the destroy flow collapses
		// to a single DestroyAllTerraform pass with no migration dance. This is the
		// path for blueprints that reference an out-of-band remote backend.
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
		var seenExclude []string
		mocks.TerraformStack.DestroyAllFunc = func(bp *blueprintv1alpha1.Blueprint, excludeIDs ...string) ([]string, error) {
			seenExclude = excludeIDs
			ops = append(ops, "destroyAll")
			return nil, nil
		}
		migrateCalled := false
		mocks.TerraformStack.MigrateComponentStateFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) error {
			migrateCalled = true
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

		if migrateCalled {
			t.Error("Expected MigrateComponentState NOT to be called when no backend component is declared")
		}
		if len(seenExclude) != 0 {
			t.Errorf("Expected no excludes when no backend component, got %v", seenExclude)
		}
		if len(ops) != 1 || ops[0] != "destroyAll" {
			t.Errorf("Expected single destroyAll op, got %v", ops)
		}
	})

	t.Run("KubernetesBackendRunsFullCycleDestroy", func(t *testing.T) {
		// Given a kubernetes-configured backend, the destroy flow takes the full-
		// cycle path (mirror image of bootstrap's full-cycle): pin backend.type to
		// local, MigrateState pulls every component's state from the cluster's
		// Secrets to local files, DestroyAll tears everything down in reverse
		// against local state, restore on defer. The per-component dance with
		// excludeIDs must NOT fire — kubernetes can't peel the backend off because
		// the cluster IS the backend, and once the cluster is going away every
		// component's state has to be local already.
		mocks := setupDestroyTest(t)
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "kubernetes"
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
		mocks.TerraformStack.MigrateStateFunc = func(bp *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate-all")
			return nil, nil
		}
		var seenExclude []string
		mocks.TerraformStack.DestroyAllFunc = func(bp *blueprintv1alpha1.Blueprint, excludeIDs ...string) ([]string, error) {
			seenExclude = excludeIDs
			ops = append(ops, "destroyAll")
			return nil, nil
		}
		migrateComponentCalled := false
		mocks.TerraformStack.MigrateComponentStateFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) error {
			migrateComponentCalled = true
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

		expected := []string{"set:local", "migrate-all", "destroyAll", "set:kubernetes"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected %d ops %v, got %d %v", len(expected), expected, len(ops), ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
		if len(seenExclude) != 0 {
			t.Errorf("Expected no excludeIDs on the kubernetes path, got %v", seenExclude)
		}
		if migrateComponentCalled {
			t.Error("Expected per-component MigrateComponentState NOT to be called on the kubernetes path")
		}
	})

	t.Run("KubernetesBackendMigrationFailureAbortsDestroy", func(t *testing.T) {
		// Given the kubernetes full-cycle destroy's pre-destroy state migration
		// fails (e.g. the cluster is unreachable or the kubernetes provider
		// rejects auth), DestroyAll must not run — destroying against an
		// inconsistent local state would partially tear down resources whose state
		// terraform doesn't track. The configured backend must still be restored
		// via defer for any subsequent operations in the same process.
		mocks := setupDestroyTest(t)
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "kubernetes"
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
		mocks.TerraformStack.MigrateStateFunc = func(bp *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate-fail")
			return nil, fmt.Errorf("cluster unreachable")
		}
		destroyCalled := false
		mocks.TerraformStack.DestroyAllFunc = func(bp *blueprintv1alpha1.Blueprint, excludeIDs ...string) ([]string, error) {
			destroyCalled = true
			return nil, nil
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
		if !strings.Contains(err.Error(), "cluster unreachable") {
			t.Errorf("Expected underlying migration cause in surfaced message, got %v", err)
		}
		if destroyCalled {
			t.Error("DestroyAll must not run after MigrateState fails on the kubernetes path")
		}
		expected := []string{"set:local", "migrate-fail", "set:kubernetes"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected %d ops %v, got %d %v", len(expected), expected, len(ops), ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
	})

	t.Run("CheckAuthFailureBlocksDestroyAllTerraformBeforeStateMigration", func(t *testing.T) {
		// Given expired credentials, destroy terraform (no arg) must fail at preflight rather
		// than after init + state migration. Mirrors TestDestroyCmd's coverage to make sure
		// the dedicated terraform subcommand is gated too.
		mocks := setupDestroyTest(t)
		mocks.ToolsManager.CheckAuthFunc = func() error { return fmt.Errorf("aws credentials did not resolve") }
		destroyAllCalled := false
		mocks.TerraformStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...string) ([]string, error) {
			destroyAllCalled = true
			return nil, nil
		}
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyTerraformCmd()
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

	t.Run("CheckAuthFailureBlocksDestroyTerraformComponent", func(t *testing.T) {
		mocks := setupDestroyTest(t)
		mocks.ToolsManager.CheckAuthFunc = func() error { return fmt.Errorf("aws credentials did not resolve") }
		destroyCalled := false
		mocks.TerraformStack.DestroyFunc = func(*blueprintv1alpha1.Blueprint, string) (bool, error) {
			destroyCalled = true
			return false, nil
		}
		proj := newDestroyProject(mocks)

		cmd := createTestDestroyTerraformCmd()
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
