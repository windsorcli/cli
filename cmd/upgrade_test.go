package cmd

import (
	"bytes"
	stdcontext "context"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

func TestUpgradeCmd(t *testing.T) {
	createTestUpgradeCmd := func() *cobra.Command { return makeApplyTestCmd(upgradeCmd) }

	suppressProcessStdout(t)
	suppressProcessStderr(t)

	t.Run("PrunesAfterSuccessfulWait", func(t *testing.T) {
		// Given an upgrade command whose terraform, install, and wait all succeed
		mocks := setupApplyTest(t)
		pruned := false
		mocks.KubernetesManager.PruneBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			pruned = true
			return nil
		}
		proj := newApplyAllProject(mocks)

		// When executing the bare upgrade command
		cmd := createTestUpgradeCmd()
		ctx := stdcontext.WithValue(stdcontext.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then it completes and prunes orphaned kustomizations
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !pruned {
			t.Error("Expected upgrade to prune orphaned kustomizations")
		}
	})

	t.Run("WritesInFlightMarkerThenSettledMarker", func(t *testing.T) {
		// Given an upgrade whose every step succeeds
		mocks := setupApplyTest(t)
		var phases []string
		mocks.KubernetesManager.ApplyVersionMarkerFunc = func(namespace string, marker kubernetes.VersionMarker) error {
			phases = append(phases, marker.Phase)
			return nil
		}
		proj := newApplyAllProject(mocks)

		// When executing the bare upgrade command
		cmd := createTestUpgradeCmd()
		ctx := stdcontext.WithValue(stdcontext.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it records the in-flight marker first, then settles it to idle on success
		if len(phases) != 2 || phases[0] != kubernetes.VersionMarkerPhaseUpgrading || phases[1] != kubernetes.VersionMarkerPhaseIdle {
			t.Errorf("Expected marker writes [upgrading idle], got %v", phases)
		}
	})

	t.Run("LeavesInFlightMarkerWhenWaitFails", func(t *testing.T) {
		// Given a wait that fails after the transition has been recorded
		mocks := setupApplyTest(t)
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(ctx stdcontext.Context, message string, bp *blueprintv1alpha1.Blueprint) error {
			return fmt.Errorf("wait failed")
		}
		var phases []string
		mocks.KubernetesManager.ApplyVersionMarkerFunc = func(namespace string, marker kubernetes.VersionMarker) error {
			phases = append(phases, marker.Phase)
			return nil
		}
		proj := newApplyAllProject(mocks)

		// When executing the upgrade command
		cmd := createTestUpgradeCmd()
		ctx := stdcontext.WithValue(stdcontext.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then the failure surfaces and the marker is left in-flight (never settled) so apply refuses
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if len(phases) != 1 || phases[0] != kubernetes.VersionMarkerPhaseUpgrading {
			t.Errorf("Expected only the in-flight marker write, got %v", phases)
		}
	})

	t.Run("LeavesInFlightMarkerWhenInstallFails", func(t *testing.T) {
		// Given an install that fails after the transition has been recorded
		mocks := setupApplyTest(t)
		mocks.KubernetesManager.ApplyBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("install failed")
		}
		var phases []string
		mocks.KubernetesManager.ApplyVersionMarkerFunc = func(namespace string, marker kubernetes.VersionMarker) error {
			phases = append(phases, marker.Phase)
			return nil
		}
		proj := newApplyAllProject(mocks)

		// When executing the upgrade command
		cmd := createTestUpgradeCmd()
		ctx := stdcontext.WithValue(stdcontext.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then the failure surfaces and the marker is never settled
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if len(phases) != 1 || phases[0] != kubernetes.VersionMarkerPhaseUpgrading {
			t.Errorf("Expected only the in-flight marker write, got %v", phases)
		}
	})

	t.Run("LeavesInFlightMarkerWhenPruneFails", func(t *testing.T) {
		// Given a prune that fails after install and wait succeed
		mocks := setupApplyTest(t)
		mocks.KubernetesManager.PruneBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("prune failed")
		}
		var phases []string
		mocks.KubernetesManager.ApplyVersionMarkerFunc = func(namespace string, marker kubernetes.VersionMarker) error {
			phases = append(phases, marker.Phase)
			return nil
		}
		proj := newApplyAllProject(mocks)

		// When executing the upgrade command
		cmd := createTestUpgradeCmd()
		ctx := stdcontext.WithValue(stdcontext.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then the failure surfaces and the marker is never settled
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if len(phases) != 1 || phases[0] != kubernetes.VersionMarkerPhaseUpgrading {
			t.Errorf("Expected only the in-flight marker write, got %v", phases)
		}
	})

	t.Run("WaitFailureSkipsPrune", func(t *testing.T) {
		// Given a wait that fails before the desired set has reconciled
		mocks := setupApplyTest(t)
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(ctx stdcontext.Context, message string, bp *blueprintv1alpha1.Blueprint) error {
			return fmt.Errorf("wait failed")
		}
		pruned := false
		mocks.KubernetesManager.PruneBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			pruned = true
			return nil
		}
		proj := newApplyAllProject(mocks)

		// When executing the upgrade command
		cmd := createTestUpgradeCmd()
		ctx := stdcontext.WithValue(stdcontext.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then the wait error surfaces and prune never runs — no deletion before adoption
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error waiting for kustomizations") {
			t.Errorf("Expected wait error, got: %v", err)
		}
		if pruned {
			t.Error("Expected prune to be skipped when the wait fails")
		}
	})

	t.Run("ErrorPruneFails", func(t *testing.T) {
		// Given a prune step that fails after a successful wait
		mocks := setupApplyTest(t)
		mocks.KubernetesManager.PruneBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("delete kustomization failed")
		}
		proj := newApplyAllProject(mocks)

		// When executing the upgrade command
		cmd := createTestUpgradeCmd()
		ctx := stdcontext.WithValue(stdcontext.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then the prune error surfaces
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error pruning orphaned kustomizations") {
			t.Errorf("Expected prune error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		// Given a blueprint handler that returns nil
		mocks := setupApplyTest(t)
		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return nil }
		proj := newApplyAllProject(mocks)

		// When executing the upgrade command
		cmd := createTestUpgradeCmd()
		ctx := stdcontext.WithValue(stdcontext.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then it reports the missing blueprint
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "blueprint is not available") {
			t.Errorf("Expected blueprint error, got: %v", err)
		}
	})

	t.Run("ErrorNilResolvedBlueprint", func(t *testing.T) {
		// Given a handler whose resolved blueprint comes back nil after terraform runs
		mocks := setupApplyTest(t)
		mocks.BlueprintHandler.GenerateResolvedFunc = func() (*blueprintv1alpha1.Blueprint, error) { return nil, nil }
		proj := newApplyAllProject(mocks)

		// When executing the upgrade command
		cmd := createTestUpgradeCmd()
		ctx := stdcontext.WithValue(stdcontext.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then it reports the missing resolved blueprint rather than feeding nil downstream
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "resolved blueprint is not available") {
			t.Errorf("Expected resolved-blueprint error, got: %v", err)
		}
	})

	t.Run("ErrorTerraformFails", func(t *testing.T) {
		// Given a terraform stack whose Up fails
		mocks := setupApplyTest(t)
		mocks.TerraformStack.UpFunc = func(bp *blueprintv1alpha1.Blueprint, onApply ...func(id string) (bool, error)) (bool, error) {
			return false, fmt.Errorf("terraform up failed")
		}
		proj := newApplyAllProject(mocks)

		// When executing the upgrade command
		cmd := createTestUpgradeCmd()
		ctx := stdcontext.WithValue(stdcontext.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then the terraform error surfaces before any kustomization work
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error applying terraform") {
			t.Errorf("Expected terraform error, got: %v", err)
		}
	})

	t.Run("RejectsUnexpectedArgs", func(t *testing.T) {
		// Given a bare upgrade command with a stray positional argument
		mocks := setupApplyTest(t)
		proj := newApplyAllProject(mocks)

		// When executing with an unexpected argument
		cmd := createTestUpgradeCmd()
		ctx := stdcontext.WithValue(stdcontext.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"bogus"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then it is rejected — bare upgrade takes no positional args
		if err == nil {
			t.Error("Expected error for unexpected argument, got nil")
		}
	})
}

func TestUpgradeNodeCmd(t *testing.T) {
	t.Cleanup(func() {
		rootCmd.SetContext(stdcontext.Background())
		upgradeNodeAddr = ""
		upgradeNodeImage = ""
		upgradeNodeTimeout = 0
	})

	setup := func(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
		t.Helper()
		upgradeNodeAddr = ""
		upgradeNodeImage = ""
		upgradeNodeTimeout = 0

		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)
		return stdout, stderr
	}

	t.Run("MissingNodeFlag", func(t *testing.T) {
		setup(t)
		rootCmd.SetArgs([]string{"upgrade", "node", "--image", "img"})

		err := Execute()

		if err == nil {
			t.Error("Expected error for missing --node flag, got nil")
		}
	})

	t.Run("MissingImageFlag", func(t *testing.T) {
		setup(t)
		rootCmd.SetArgs([]string{"upgrade", "node", "--node", "10.0.0.1"})

		err := Execute()

		if err == nil {
			t.Error("Expected error for missing --image flag, got nil")
		}
	})

	t.Run("CheckTrustedDirectoryError", func(t *testing.T) {
		setup(t)
		mocks := setupMocks(t)
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return stdcontext.DeadlineExceeded
		}

		ctx := stdcontext.WithValue(stdcontext.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)
		rootCmd.SetArgs([]string{"upgrade", "node", "--node", "10.0.0.1", "--image", "img"})

		err := Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected trusted directory error, got: %v", err)
		}
	})

	t.Run("ConfigNotLoaded", func(t *testing.T) {
		setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error { return nil }
		mockConfigHandler.IsLoadedFunc = func() bool { return false }
		mocks := setupMocks(t, &SetupOptions{ConfigHandler: mockConfigHandler})

		ctx := stdcontext.WithValue(stdcontext.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)
		rootCmd.SetArgs([]string{"upgrade", "node", "--node", "10.0.0.1", "--image", "img"})

		err := Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Nothing to upgrade") {
			t.Errorf("Expected not-loaded error, got: %v", err)
		}
	})

	t.Run("UpgradeNodeError", func(t *testing.T) {
		setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error { return nil }
		mockConfigHandler.IsLoadedFunc = func() bool { return true }
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return ""
		}
		mocks := setupMocks(t, &SetupOptions{ConfigHandler: mockConfigHandler})

		ctx := stdcontext.WithValue(stdcontext.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)
		rootCmd.SetArgs([]string{"upgrade", "node", "--node", "10.0.0.1", "--image", "img"})

		err := Execute()

		// TALOSCONFIG not set — confirms upgrade node code path was reached
		if err == nil {
			t.Error("Expected error (no TALOSCONFIG), got nil")
		}
		if !strings.Contains(err.Error(), "node upgrade failed") {
			t.Errorf("Expected node upgrade error, got: %v", err)
		}
	})

}
