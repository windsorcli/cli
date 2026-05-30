package provisioner

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProvisioner_Teardown(t *testing.T) {
	t.Run("KubernetesWithoutBackendFieldErrorsBeforeAnyDestroy", func(t *testing.T) {
		// A kubernetes-configured backend with no Blueprint.Backend would silently
		// fall through to plain DestroyAll, destroying the cluster while other
		// components' state still lives in it. Refuse before any destroy runs.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/talos"},
				{Path: "workloads/argocd"},
			},
		}
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
		destroyAllCalled := false
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalled = true
			return terraforminfra.DestroyOutcome{}, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.Teardown(bp, true, false)
		if err == nil {
			t.Fatal("Expected error for kubernetes backend without Blueprint.Backend, got nil")
		}
		if !strings.Contains(err.Error(), "Blueprint.Backend") {
			t.Errorf("Expected error to name the missing field, got: %v", err)
		}
		if !strings.Contains(err.Error(), "kubernetes") {
			t.Errorf("Expected error to name the backend type, got: %v", err)
		}
		if destroyAllCalled {
			t.Error("DestroyAll must not run when refusing")
		}
	})

	t.Run("LocalBackendCollapsesToDestroyAllTerraform", func(t *testing.T) {
		// Local backend has no remote storage; Teardown forwards to
		// DestroyAllTerraform without any pivot.
		mocks := setupProvisionerMocks(t)
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		setCalled := false
		mockCH.SetFunc = func(_ string, _ any) error {
			setCalled = true
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		destroyAllCalls := 0
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalls++
			return terraforminfra.DestroyOutcome{}, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if _, err := provisioner.Teardown(createTestBlueprint(), true, false); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if destroyAllCalls != 1 {
			t.Errorf("Expected single DestroyAll call for local backend, got %d", destroyAllCalls)
		}
		if setCalled {
			t.Error("Expected backend.type Set NOT to be called when backend is local")
		}
	})

	t.Run("NoBackendFieldCollapsesToDestroyAllTerraform", func(t *testing.T) {
		// Without Blueprint.Backend, there is no in-blueprint backend tier;
		// every component uses the configured backend. Teardown forwards to
		// DestroyAllTerraform.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "cluster"},
			},
		}
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
		setCalled := false
		mockCH.SetFunc = func(_ string, _ any) error {
			setCalled = true
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		var seenExclude []string
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, excludeIDs ...string) (terraforminfra.DestroyOutcome, error) {
			seenExclude = excludeIDs
			return terraforminfra.DestroyOutcome{}, nil
		}
		migrateCalled := false
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			migrateCalled = true
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if _, err := provisioner.Teardown(bp, true, false); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if setCalled {
			t.Error("Expected no backend override when blueprint has no backend tier")
		}
		if len(seenExclude) != 0 {
			t.Errorf("Expected no excludes when there is no backend tier, got %v", seenExclude)
		}
		if migrateCalled {
			t.Error("MigrateState must not run when there is no backend tier")
		}
	})

	t.Run("SingleTierComponentDestroysNonTierThenTier", func(t *testing.T) {
		// Stage 1: destroy non-tier against the configured backend.
		// Stage 2: pin local, migrate tier state to local, destroy tier against local.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
		}

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
		var destroyAllBlueprints []*blueprintv1alpha1.Blueprint
		var destroyAllExcludes [][]string
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(b *blueprintv1alpha1.Blueprint, _ bool, excludeIDs ...string) (terraforminfra.DestroyOutcome, error) {
			ops = append(ops, fmt.Sprintf("destroyAll:exclude=%v", excludeIDs))
			destroyAllBlueprints = append(destroyAllBlueprints, b)
			destroyAllExcludes = append(destroyAllExcludes, excludeIDs)
			return terraforminfra.DestroyOutcome{}, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if _, err := provisioner.Teardown(bp, true, false); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := []string{
			"destroyAll:exclude=[backend]",
			"set:local",
			"migrate",
			"destroyAll:exclude=[]",
			"set:s3",
		}
		if len(ops) != len(expected) {
			t.Fatalf("Expected %v, got %v", expected, ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}

		if len(destroyAllBlueprints) != 2 {
			t.Fatalf("Expected two DestroyAll calls, got %d", len(destroyAllBlueprints))
		}
		stage2Components := destroyAllBlueprints[1].TerraformComponents
		if len(stage2Components) != 1 || stage2Components[0].Path != "backend" {
			t.Errorf("Stage 2 DestroyAll should target tier [backend], got %#v", stage2Components)
		}
	})

	t.Run("MultiComponentTierDestroyedTogether", func(t *testing.T) {
		// VPC + IAM + cluster as the tier. Stage 1 destroys non-tier (workloads)
		// against the configured backend; Stage 2a migrates all three tier members'
		// state to local; Stage 2b destroys the whole tier against local.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "cluster",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "networking/vpc"},
				{Path: "iam"},
				{Name: "cluster", Path: "cluster/eks"},
				{Path: "workloads/argocd"},
			},
		}

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
		mockCH.SetFunc = func(_ string, _ any) error { return nil }

		var destroyAllBlueprints []*blueprintv1alpha1.Blueprint
		var destroyAllExcludes [][]string
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(b *blueprintv1alpha1.Blueprint, _ bool, excludeIDs ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllBlueprints = append(destroyAllBlueprints, b)
			destroyAllExcludes = append(destroyAllExcludes, excludeIDs)
			return terraforminfra.DestroyOutcome{}, nil
		}
		var migrateBlueprints []*blueprintv1alpha1.Blueprint
		mockStack.MigrateStateFunc = func(b *blueprintv1alpha1.Blueprint) ([]string, error) {
			migrateBlueprints = append(migrateBlueprints, b)
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if _, err := provisioner.Teardown(bp, true, false); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(destroyAllBlueprints) != 2 {
			t.Fatalf("Expected two DestroyAll calls, got %d", len(destroyAllBlueprints))
		}
		expectedExcludes := []string{"networking/vpc", "iam", "cluster"}
		if len(destroyAllExcludes[0]) != 3 {
			t.Fatalf("Stage 1 excludes should list all 3 tier IDs, got %v", destroyAllExcludes[0])
		}
		for i, want := range expectedExcludes {
			if destroyAllExcludes[0][i] != want {
				t.Errorf("Stage 1 exclude[%d]: got %q, want %q", i, destroyAllExcludes[0][i], want)
			}
		}
		stage2Components := destroyAllBlueprints[1].TerraformComponents
		if len(stage2Components) != 3 {
			t.Fatalf("Stage 2 DestroyAll should target 3-component tier, got %d", len(stage2Components))
		}

		if len(migrateBlueprints) != 1 {
			t.Fatalf("Expected one MigrateState call, got %d", len(migrateBlueprints))
		}
		if len(migrateBlueprints[0].TerraformComponents) != 3 {
			t.Errorf("MigrateState should target the 3-component tier, got %d", len(migrateBlueprints[0].TerraformComponents))
		}
	})

	t.Run("Stage1FailureAbortsBeforeStage2", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
		}
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
		setCalled := false
		mockCH.SetFunc = func(_ string, _ any) error {
			setCalled = true
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			return terraforminfra.DestroyOutcome{}, fmt.Errorf("non-tier destroy failed")
		}
		migrateCalled := false
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			migrateCalled = true
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.Teardown(bp, true, false)
		if err == nil {
			t.Fatal("Expected error from Stage 1 failure, got nil")
		}
		if !strings.Contains(err.Error(), "non-tier destroy failed") {
			t.Errorf("Expected error to wrap Stage 1 failure, got %v", err)
		}
		if migrateCalled {
			t.Error("MigrateState must not run after Stage 1 fails")
		}
		if setCalled {
			t.Error("Backend override must not engage after Stage 1 fails")
		}
	})

	t.Run("Stage2MigrationFailureAbortsTierDestroyAndRestoresBackend", func(t *testing.T) {
		// When the Stage 2 MigrateState fails (e.g. configured backend is intermittent),
		// no tier destroy may run — operating against partially-migrated state could
		// strand the operator. The deferred restore in withBackendOverride must still
		// fire so subsequent commands see the configured backend.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
		}
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
		mockStack := terraforminfra.NewMockStack()
		destroyAllCalls := 0
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalls++
			ops = append(ops, "destroyAll")
			return terraforminfra.DestroyOutcome{}, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate-fail")
			return nil, fmt.Errorf("configured backend unreachable")
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.Teardown(bp, true, false)
		if err == nil {
			t.Fatal("Expected error from Stage 2 migration failure, got nil")
		}
		if !strings.Contains(err.Error(), "configured backend unreachable") {
			t.Errorf("Expected underlying migration cause in error, got %v", err)
		}
		if destroyAllCalls != 1 {
			t.Errorf("Stage 2 tier destroy must not run when migration fails (expected 1 DestroyAll from Stage 1, got %d)", destroyAllCalls)
		}
		// Restore must still fire via defer.
		var sawRestore bool
		for _, op := range ops {
			if op == "set:s3" {
				sawRestore = true
			}
		}
		if !sawRestore {
			t.Errorf("Expected backend restore via defer after Stage 2 failure, got %v", ops)
		}
	})

	t.Run("BackendRestoreFailureEmitsStderrWarning", func(t *testing.T) {
		// When the deferred Set (restore to configured backend) fails after a
		// successful tier destroy, the call still returns nil but the operator
		// needs to know so subsequent commands aren't surprised by a stale
		// in-memory override.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
		}
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
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) { return terraforminfra.DestroyOutcome{}, nil }
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) { return nil, nil }

		r, w, pipeErr := os.Pipe()
		if pipeErr != nil {
			t.Fatalf("Pipe failed: %v", pipeErr)
		}
		origStderr := os.Stderr
		os.Stderr = w
		defer func() { os.Stderr = origStderr }()

		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})
		_, err := provisioner.Teardown(bp, true, false)

		w.Close()
		stderrBytes, _ := io.ReadAll(r)
		stderrOutput := string(stderrBytes)

		if err != nil {
			t.Fatalf("Expected destroy to succeed despite restore failure, got %v", err)
		}
		if !strings.Contains(stderrOutput, "failed to restore terraform.backend.type") {
			t.Errorf("Expected stderr warning about restore failure, got: %q", stderrOutput)
		}
		if !strings.Contains(stderrOutput, "mock restore failure") {
			t.Errorf("Expected stderr warning to include underlying cause, got: %q", stderrOutput)
		}
	})

	t.Run("SkippedComponentsMergeAcrossStages", func(t *testing.T) {
		// Stage 1's DestroyAll, Stage 2a's MigrateState, and Stage 2b's
		// DestroyAllTerraform all independently report dir-missing
		// components. Naive concat would double-count overlaps.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
				{Path: "gitops"},
			},
		}
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
		mockCH.SetFunc = func(_ string, _ any) error { return nil }
		mockStack := terraforminfra.NewMockStack()
		destroyAllCalls := 0
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalls++
			if destroyAllCalls == 1 {
				return terraforminfra.DestroyOutcome{Skipped: []string{"gitops"}}, nil
			}
			return terraforminfra.DestroyOutcome{Skipped: []string{"backend"}}, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			return []string{"backend"}, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		result, err := provisioner.Teardown(bp, true, false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expected := []string{"gitops", "backend"}
		if len(result.Skipped) != len(expected) {
			t.Fatalf("Expected %v, got %v", expected, result.Skipped)
		}
		for i, want := range expected {
			if result.Skipped[i] != want {
				t.Errorf("skipped[%d]: got %q, want %q", i, result.Skipped[i], want)
			}
		}
	})

	t.Run("ContinueDefersTierWhenNonTierHasFailures", func(t *testing.T) {
		// Given a blueprint with a backend tier and a non-tier component whose
		// Stage 1 destroy reported a failure under continueOnError mode
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
		}
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
		setCalled := false
		mockCH.SetFunc = func(_ string, _ any) error {
			setCalled = true
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		migrateCalled := false
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			return terraforminfra.DestroyOutcome{
				Failed: []terraforminfra.ComponentFailure{{ID: "cluster", Err: fmt.Errorf("cluster destroy failed")}},
			}, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			migrateCalled = true
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		// When Teardown runs with continueOnError=true
		result, err := provisioner.Teardown(bp, true, true)

		// Then no error is returned (failure is collected), the tier is deferred,
		// and Stage 2 backend migration never engages
		if err != nil {
			t.Fatalf("Expected continueOnError to absorb per-component failure, got %v", err)
		}
		if !result.TierDeferred {
			t.Error("Expected TierDeferred=true when non-tier destroy left a failure")
		}
		if len(result.Failed) != 1 || result.Failed[0].ID != "cluster" {
			t.Errorf("Expected failures to include cluster, got %v", result.Failed)
		}
		if migrateCalled {
			t.Error("MigrateState must not run when non-tier failures left the tier deferred")
		}
		if setCalled {
			t.Error("Backend override must not engage when tier is deferred")
		}
	})

	t.Run("ContinueAttemptsTierWhenStage1Clean", func(t *testing.T) {
		// Given a Stage 1 destroy that completes with zero failures
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
		}
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
		mockCH.SetFunc = func(_ string, _ any) error { return nil }
		mockStack := terraforminfra.NewMockStack()
		migrateCalled := false
		destroyAllCalls := 0
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalls++
			return terraforminfra.DestroyOutcome{Destroyed: []string{fmt.Sprintf("pass%d", destroyAllCalls)}}, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			migrateCalled = true
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		// When Teardown runs with continueOnError=true
		result, err := provisioner.Teardown(bp, true, true)

		// Then the backend tier is attempted (Stage 2 runs) and TierDeferred stays false
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result.TierDeferred {
			t.Error("Expected TierDeferred=false when Stage 1 was clean")
		}
		if !migrateCalled {
			t.Error("Expected MigrateState to run when Stage 1 produced no failures")
		}
		if destroyAllCalls != 2 {
			t.Errorf("Expected Stage 1 + Stage 2 DestroyAll calls (2), got %d", destroyAllCalls)
		}
	})

	t.Run("ContinueCollectsLocalBackendFailures", func(t *testing.T) {
		// Given a local backend (no tier) and a stack that reports per-component
		// failures via DestroyResult.Failed in continueOnError mode
		mocks := setupProvisionerMocks(t)
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, continueOnError bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			if !continueOnError {
				t.Error("Expected continueOnError=true to be propagated to stack")
			}
			return terraforminfra.DestroyOutcome{
				Destroyed: []string{"vpc"},
				Failed:    []terraforminfra.ComponentFailure{{ID: "iam", Err: fmt.Errorf("permission denied")}},
			}, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		// When Teardown runs with continueOnError=true
		result, err := provisioner.Teardown(createTestBlueprint(), true, true)

		// Then per-component failures surface in result.Failed without aborting
		if err != nil {
			t.Fatalf("Expected continueOnError to absorb per-component failure, got %v", err)
		}
		if len(result.Failed) != 1 || result.Failed[0].ID != "iam" {
			t.Errorf("Expected failure list to contain iam, got %v", result.Failed)
		}
		if len(result.Destroyed) != 1 || result.Destroyed[0] != "vpc" {
			t.Errorf("Expected Destroyed=[vpc], got %v", result.Destroyed)
		}
	})
}

func TestProvisioner_TeardownComponent(t *testing.T) {
	bpWithBackend := func() *blueprintv1alpha1.Blueprint {
		return &blueprintv1alpha1.Blueprint{
			Backend:  "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
		}
	}

	t.Run("RefusesAnyTierMemberOnRemoteBackend", func(t *testing.T) {
		// A tier member on any remote backend (s3, azurerm, kubernetes) is refused —
		// destroying it in isolation would orphan state for downstream components.
		for _, backendType := range []string{"kubernetes", "s3", "azurerm"} {
			t.Run(backendType, func(t *testing.T) {
				mocks := setupProvisionerMocks(t)
				mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
				mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
					if key == "terraform.backend.type" {
						return backendType
					}
					if len(defaultValue) > 0 {
						return defaultValue[0]
					}
					return ""
				}
				destroyCalled := false
				mockStack := terraforminfra.NewMockStack()
				mockStack.DestroyFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) (bool, error) {
					destroyCalled = true
					return false, nil
				}
				provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

				_, err := provisioner.TeardownComponent(bpWithBackend(), "backend")
				if err == nil {
					t.Fatal("Expected refusal error, got nil")
				}
				if !strings.Contains(err.Error(), "backend-tier component") {
					t.Errorf("Expected refusal message naming the tier, got: %v", err)
				}
				if !strings.Contains(err.Error(), backendType) {
					t.Errorf("Expected refusal message naming %s backend, got: %v", backendType, err)
				}
				if !strings.Contains(err.Error(), "windsor destroy") {
					t.Errorf("Expected error to point at full-cycle teardown, got: %v", err)
				}
				if destroyCalled {
					t.Error("Expected Destroy NOT to be called when refusing")
				}
			})
		}
	})

	t.Run("RefusesPreBackendTierMemberOnRemoteBackend", func(t *testing.T) {
		// A component declared before the named backend is also a tier member
		// and triggers the same refusal.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "cluster",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "networking/vpc"},
				{Name: "cluster", Path: "cluster/eks"},
				{Path: "workloads/argocd"},
			},
		}
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
		destroyCalled := false
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) (bool, error) {
			destroyCalled = true
			return false, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.TeardownComponent(bp, "networking/vpc")
		if err == nil {
			t.Fatal("Expected refusal for pre-backend tier member, got nil")
		}
		if !strings.Contains(err.Error(), "networking/vpc") {
			t.Errorf("Expected error to name the tier member, got: %v", err)
		}
		if destroyCalled {
			t.Error("Destroy must not run when refusing")
		}
	})

	t.Run("AllowsBackendTierMemberOnLocalBackend", func(t *testing.T) {
		// On a local backend there is no shared remote storage to orphan; a tier
		// member destroys directly.
		mocks := setupProvisionerMocks(t)
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		destroyCalled := false
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
			destroyCalled = true
			if componentID != "backend" {
				t.Errorf("Expected destroy for backend, got %s", componentID)
			}
			return false, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if _, err := provisioner.TeardownComponent(bpWithBackend(), "backend"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !destroyCalled {
			t.Error("Expected Destroy to be called on local backend")
		}
	})

	t.Run("NonTierComponentUsesDirectDestroyOnAnyBackend", func(t *testing.T) {
		// Non-tier components destroy directly regardless of backend type — the
		// configured backend still exists, no migration needed.
		for _, backendType := range []string{"local", "kubernetes", "s3", "azurerm"} {
			t.Run(backendType, func(t *testing.T) {
				mocks := setupProvisionerMocks(t)
				mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
				mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
					if key == "terraform.backend.type" {
						return backendType
					}
					if len(defaultValue) > 0 {
						return defaultValue[0]
					}
					return ""
				}
				setCalled := false
				mockCH.SetFunc = func(_ string, _ any) error {
					setCalled = true
					return nil
				}
				migrateCalled := false
				mockStack := terraforminfra.NewMockStack()
				mockStack.MigrateComponentStateFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error {
					migrateCalled = true
					return nil
				}
				destroyCalled := false
				mockStack.DestroyFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
					destroyCalled = true
					if componentID != "cluster" {
						t.Errorf("Expected destroy for cluster, got %s", componentID)
					}
					return false, nil
				}
				provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

				if _, err := provisioner.TeardownComponent(bpWithBackend(), "cluster"); err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if !destroyCalled {
					t.Error("Expected Destroy to be called")
				}
				if migrateCalled {
					t.Error("Expected MigrateComponentState NOT to be called for non-tier component")
				}
				if setCalled {
					t.Error("Expected backend.type Set NOT to be called for non-tier component")
				}
			})
		}
	})
}
