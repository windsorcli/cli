package cmd

import (
	"bytes"
	stdcontext "context"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

func TestUpgradeCmd(t *testing.T) {
	createTestUpgradeCmd := func() *cobra.Command { return makeApplyTestCmd(upgradeCmd) }

	suppressProcessStdout(t)
	suppressProcessStderr(t)

	// These subtests exercise the executing path; upgrade requires --yes to mutate, so confirm it.
	upgradeYes = true
	t.Cleanup(func() { upgradeYes = false })

	t.Run("PrunesAfterSuccessfulWait", func(t *testing.T) {
		// Given an upgrade with an orphaned kustomization
		mocks := setupApplyTest(t)
		mocks.KubernetesManager.ListPrunableKustomizationsFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) ([]string, error) {
			return []string{"old-thing"}, nil
		}
		pruned := false
		mocks.KubernetesManager.PruneBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			pruned = true
			return nil
		}
		proj := newApplyAllProject(mocks)

		// When executing the bare upgrade command with --yes
		cmd := createTestUpgradeCmd()
		cmd.SetArgs([]string{"--yes"})
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
		// Given an orphan to prune and a prune that fails after install and wait succeed
		mocks := setupApplyTest(t)
		mocks.KubernetesManager.ListPrunableKustomizationsFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) ([]string, error) {
			return []string{"old-thing"}, nil
		}
		mocks.KubernetesManager.PruneBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("prune failed")
		}
		var phases []string
		mocks.KubernetesManager.ApplyVersionMarkerFunc = func(namespace string, marker kubernetes.VersionMarker) error {
			phases = append(phases, marker.Phase)
			return nil
		}
		proj := newApplyAllProject(mocks)

		// When executing the upgrade command with --yes
		cmd := createTestUpgradeCmd()
		cmd.SetArgs([]string{"--yes"})
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
		// Given an orphan to prune and a prune step that fails after a successful wait
		mocks := setupApplyTest(t)
		mocks.KubernetesManager.ListPrunableKustomizationsFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) ([]string, error) {
			return []string{"old-thing"}, nil
		}
		mocks.KubernetesManager.PruneBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("delete kustomization failed")
		}
		proj := newApplyAllProject(mocks)

		// When executing the upgrade command with --yes
		cmd := createTestUpgradeCmd()
		cmd.SetArgs([]string{"--yes"})
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

func TestUpgradeCmd_Latest(t *testing.T) {
	createTestUpgradeCmd := func() *cobra.Command { return makeApplyTestCmd(upgradeCmd) }

	suppressProcessStdout(t)
	suppressProcessStderr(t)

	upgradeYes = true
	t.Cleanup(func() { upgradeYes = false })

	t.Run("ResolvesLatestPersistsAndUpgrades", func(t *testing.T) {
		// Given sources with a newer version available (implicit upgrade, no --source)
		mocks := setupApplyTest(t)
		mocks.BlueprintHandler.UpgradeSourcesToLatestFunc = func() ([]blueprint.SourceUpgrade, error) {
			return []blueprint.SourceUpgrade{{
				Name: "core",
				From: "oci://ghcr.io/windsorcli/core:v0.5.0",
				To:   "oci://ghcr.io/windsorcli/core:v0.6.0",
			}}, nil
		}
		persisted := false
		mocks.BlueprintHandler.WriteFunc = func(overwrite ...bool) error {
			if len(overwrite) > 0 && overwrite[0] {
				persisted = true
			}
			return nil
		}
		proj := newApplyAllProject(mocks)

		// When running bare upgrade
		var out bytes.Buffer
		cmd := createTestUpgradeCmd()
		cmd.SetOut(&out)
		ctx := stdcontext.WithValue(stdcontext.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it resolves latest, persists the bump, reports it, and proceeds
		if !persisted {
			t.Error("Expected the resolved versions to be persisted via Write(true)")
		}
		if !strings.Contains(out.String(), "Upgraded core") {
			t.Errorf("Expected an upgrade report, got: %q", out.String())
		}
	})

	t.Run("ReportsWhenAlreadyLatest", func(t *testing.T) {
		// Given nothing newer is available
		mocks := setupApplyTest(t)
		mocks.BlueprintHandler.UpgradeSourcesToLatestFunc = func() ([]blueprint.SourceUpgrade, error) {
			return nil, nil
		}
		proj := newApplyAllProject(mocks)

		// When running bare upgrade
		var out bytes.Buffer
		cmd := createTestUpgradeCmd()
		cmd.SetOut(&out)
		ctx := stdcontext.WithValue(stdcontext.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it reports already-latest and still reconciles
		if !strings.Contains(out.String(), "already at their latest") {
			t.Errorf("Expected an already-latest report, got: %q", out.String())
		}
	})

	t.Run("ResolutionErrorAborts", func(t *testing.T) {
		// Given resolving latest fails
		mocks := setupApplyTest(t)
		mocks.BlueprintHandler.UpgradeSourcesToLatestFunc = func() ([]blueprint.SourceUpgrade, error) {
			return nil, fmt.Errorf("registry unreachable")
		}
		proj := newApplyAllProject(mocks)

		// When running bare upgrade
		cmd := createTestUpgradeCmd()
		ctx := stdcontext.WithValue(stdcontext.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then it aborts with the resolution error
		if err == nil {
			t.Fatal("Expected an error when resolving latest fails, got nil")
		}
		if !strings.Contains(err.Error(), "resolving latest source versions") {
			t.Errorf("Expected a resolution error, got: %v", err)
		}
	})
}

func TestUpgradeCmd_Source(t *testing.T) {
	createTestUpgradeCmd := func() *cobra.Command { return makeApplyTestCmd(upgradeCmd) }

	suppressProcessStdout(t)
	suppressProcessStderr(t)

	upgradeYes = true
	t.Cleanup(func() { upgradeYes = false })

	t.Run("RetargetsPersistsAndUpgrades", func(t *testing.T) {
		t.Cleanup(func() { upgradeSources = nil })
		// Given a declared source being bumped to a new tag
		mocks := setupApplyTest(t)
		var gotName, gotURL string
		persisted := false
		mocks.BlueprintHandler.RetargetSourceFunc = func(name, url string) (string, error) {
			gotName, gotURL = name, url
			return "oci://ghcr.io/windsorcli/core:v0.3.0", nil
		}
		mocks.BlueprintHandler.WriteFunc = func(overwrite ...bool) error {
			if len(overwrite) > 0 && overwrite[0] {
				persisted = true
			}
			return nil
		}
		proj := newApplyAllProject(mocks)

		// When upgrading with --source
		var out bytes.Buffer
		cmd := createTestUpgradeCmd()
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"--source", "core=oci://ghcr.io/windsorcli/core:v0.6.0"})
		ctx := stdcontext.WithValue(stdcontext.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the source is retargeted, persisted, reported, and the upgrade proceeds
		if gotName != "core" || gotURL != "oci://ghcr.io/windsorcli/core:v0.6.0" {
			t.Errorf("Expected retarget core to new URL, got %q=%q", gotName, gotURL)
		}
		if !persisted {
			t.Error("Expected the bump to be persisted via Write(true)")
		}
		if !strings.Contains(out.String(), "Retargeted core") {
			t.Errorf("Expected a retarget report, got: %q", out.String())
		}
	})

	t.Run("UnknownSourceAbortsBeforeWrite", func(t *testing.T) {
		t.Cleanup(func() { upgradeSources = nil })
		// Given a source name that is not declared
		mocks := setupApplyTest(t)
		persisted := false
		mocks.BlueprintHandler.RetargetSourceFunc = func(name, url string) (string, error) {
			return "", fmt.Errorf("source %q is not declared in the blueprint", name)
		}
		mocks.BlueprintHandler.WriteFunc = func(overwrite ...bool) error {
			if len(overwrite) > 0 && overwrite[0] {
				persisted = true
			}
			return nil
		}
		proj := newApplyAllProject(mocks)

		// When upgrading with an undeclared --source
		cmd := createTestUpgradeCmd()
		cmd.SetArgs([]string{"--source", "addons=oci://ghcr.io/acme/addons:v0.5.0"})
		ctx := stdcontext.WithValue(stdcontext.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then it aborts with the error and never writes blueprint.yaml
		if err == nil {
			t.Fatal("Expected error for an undeclared source, got nil")
		}
		if !strings.Contains(err.Error(), "not declared") {
			t.Errorf("Expected an undeclared-source error, got: %v", err)
		}
		if persisted {
			t.Error("Expected no Write(true) when a retarget fails")
		}
	})

	t.Run("MalformedSourceSpecIsRejected", func(t *testing.T) {
		t.Cleanup(func() { upgradeSources = nil })
		// Given a --source value missing the name=url separator
		mocks := setupApplyTest(t)
		proj := newApplyAllProject(mocks)

		// When upgrading with the malformed spec
		cmd := createTestUpgradeCmd()
		cmd.SetArgs([]string{"--source", "coreonly"})
		ctx := stdcontext.WithValue(stdcontext.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then it is rejected with a format error
		if err == nil {
			t.Fatal("Expected error for a malformed --source, got nil")
		}
		if !strings.Contains(err.Error(), "name=url") {
			t.Errorf("Expected a name=url format error, got: %v", err)
		}
	})
}

func TestUpgradeCmd_Confirmation(t *testing.T) {
	createTestUpgradeCmd := func() *cobra.Command { return makeApplyTestCmd(upgradeCmd) }

	suppressProcessStdout(t)
	suppressProcessStderr(t)

	t.Run("RefusesWithoutYes", func(t *testing.T) {
		t.Cleanup(func() { upgradeYes = false })
		// Given an upgrade that would rewrite blueprint.yaml and reconcile
		mocks := setupApplyTest(t)
		installed := false
		mocks.KubernetesManager.ApplyBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			installed = true
			return nil
		}
		pruned := false
		mocks.KubernetesManager.PruneBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			pruned = true
			return nil
		}
		proj := newApplyAllProject(mocks)

		// When upgrading without --yes
		cmd := createTestUpgradeCmd()
		ctx := stdcontext.WithValue(stdcontext.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then it refuses up front and reconciles nothing
		if err == nil {
			t.Fatal("Expected refusal without --yes, got nil")
		}
		if !strings.Contains(err.Error(), "--yes") {
			t.Errorf("Expected the message to point at --yes, got: %v", err)
		}
		if installed {
			t.Error("Expected install to be skipped when the guard refuses")
		}
		if pruned {
			t.Error("Expected prune to be skipped when the guard refuses")
		}
	})

	t.Run("ProceedsWithYes", func(t *testing.T) {
		t.Cleanup(func() { upgradeYes = false })
		// Given a pending prune
		mocks := setupApplyTest(t)
		mocks.KubernetesManager.ListPrunableKustomizationsFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) ([]string, error) {
			return []string{"old-thing"}, nil
		}
		pruned := false
		mocks.KubernetesManager.PruneBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			pruned = true
			return nil
		}
		proj := newApplyAllProject(mocks)

		// When upgrading with --yes
		cmd := createTestUpgradeCmd()
		cmd.SetArgs([]string{"--yes"})
		ctx := stdcontext.WithValue(stdcontext.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Expected --yes to proceed, got %v", err)
		}

		// Then the prune runs
		if !pruned {
			t.Error("Expected prune to run under --yes")
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
