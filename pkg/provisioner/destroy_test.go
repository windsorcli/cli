package provisioner

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

func TestProvisioner_CheckComponentDestroyable(t *testing.T) {
	bp := &blueprintv1alpha1.Blueprint{
		Backend: "cluster",
		TerraformComponents: []blueprintv1alpha1.TerraformComponent{
			{Name: "compute", Path: "compute/hcloud"},
			{Name: "cluster", Path: "cluster/talos"},
			{Name: "dns", Path: "dns/zone/hetzner"},
		},
	}
	kubernetesBackend := func(mocks *ProvisionerTestMocks) {
		mocks.ConfigHandler.(*config.MockConfigHandler).GetStringFunc = func(key string, dv ...string) string {
			if key == "terraform.backend.type" {
				return "kubernetes"
			}
			if len(dv) > 0 {
				return dv[0]
			}
			return ""
		}
	}

	t.Run("RefusesBackendTierMemberOnKubernetesBackend", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		kubernetesBackend(mocks)
		prov := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{})
		err := prov.CheckComponentDestroyable(bp, "compute")
		if err == nil || !strings.Contains(err.Error(), "backend-tier") {
			t.Errorf("expected refusal naming backend-tier, got %v", err)
		}
	})

	t.Run("AllowsNonTierMemberOnKubernetesBackend", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		kubernetesBackend(mocks)
		prov := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{})
		if err := prov.CheckComponentDestroyable(bp, "dns"); err != nil {
			t.Errorf("expected non-tier member allowed, got %v", err)
		}
	})

	t.Run("AllowsBackendTierMemberOnLocalBackend", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		prov := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{})
		if err := prov.CheckComponentDestroyable(bp, "compute"); err != nil {
			t.Errorf("expected local backend to allow any component, got %v", err)
		}
	})
}

func TestProvisioner_PivotToLocalIfClusterGone(t *testing.T) {
	bp := &blueprintv1alpha1.Blueprint{Backend: "cluster", TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Name: "cluster"}}}
	kubernetesBackend := func(mocks *ProvisionerTestMocks, set *bool) {
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, dv ...string) string {
			if key == "terraform.backend.type" {
				return "kubernetes"
			}
			if len(dv) > 0 {
				return dv[0]
			}
			return ""
		}
		mockCH.SetFunc = func(key string, _ any) error {
			if key == "terraform.backend.type" {
				*set = true
			}
			return nil
		}
	}

	t.Run("PivotsWhenKubeconfigAbsent", func(t *testing.T) {
		// Given a kubernetes backend and no kubeconfig — the cluster is gone, state is already local
		mocks := setupProvisionerMocks(t)
		set := false
		kubernetesBackend(mocks, &set)
		prov := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{})
		prov.configRoot = t.TempDir()

		// When checking, it pivots to local without migrating
		pivoted, err := prov.PivotToLocalIfClusterGone(bp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !pivoted || !set {
			t.Errorf("expected pivot to local when kubeconfig absent, pivoted=%v set=%v", pivoted, set)
		}
	})

	t.Run("DoesNotPivotWhenClusterReachable", func(t *testing.T) {
		// Given a kubernetes backend, present kubeconfig, reachable cluster (first-run targeted destroy)
		mocks := setupProvisionerMocks(t)
		set := false
		kubernetesBackend(mocks, &set)
		dir := t.TempDir()
		_ = os.MkdirAll(filepath.Join(dir, ".kube"), 0o755)
		_ = os.WriteFile(filepath.Join(dir, ".kube", "config"), []byte("x"), 0o644)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, out func(string), nodes ...string) error {
			return nil
		}
		prov := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})
		prov.configRoot = dir

		pivoted, err := prov.PivotToLocalIfClusterGone(bp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pivoted || set {
			t.Errorf("expected no pivot when cluster reachable, pivoted=%v set=%v", pivoted, set)
		}
	})

	t.Run("NoOpOnLocalBackend", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		set := false
		mocks.ConfigHandler.(*config.MockConfigHandler).SetFunc = func(_ string, _ any) error { set = true; return nil }
		prov := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{})
		prov.configRoot = t.TempDir()

		pivoted, err := prov.PivotToLocalIfClusterGone(bp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pivoted || set {
			t.Errorf("expected no pivot on local backend, pivoted=%v set=%v", pivoted, set)
		}
	})
}

func TestProvisioner_PrepareLocalTeardown(t *testing.T) {
	bp := &blueprintv1alpha1.Blueprint{Backend: "cluster", TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Name: "cluster", Path: "cluster/talos"}}}
	kubernetesBackend := func(mocks *ProvisionerTestMocks, set *bool) {
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, dv ...string) string {
			if key == "terraform.backend.type" {
				return "kubernetes"
			}
			if len(dv) > 0 {
				return dv[0]
			}
			return ""
		}
		mockCH.SetFunc = func(key string, _ any) error {
			if key == "terraform.backend.type" {
				*set = true
			}
			return nil
		}
	}

	t.Run("PivotsToLocalAndMigratesUpFront", func(t *testing.T) {
		// Given a kubernetes backend and a migration that succeeds (cluster up, first-run teardown)
		mocks := setupProvisionerMocks(t)
		set := false
		kubernetesBackend(mocks, &set)
		migrated := false
		mockStack := terraforminfra.NewMockStack()
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			migrated = true
			return nil, nil
		}
		prov := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		// When preparing, it pivots the backend to local and migrates state up front
		pivoted, err := prov.PrepareLocalTeardown(bp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !pivoted || !set || !migrated {
			t.Errorf("expected pivot + migration, pivoted=%v set=%v migrated=%v", pivoted, set, migrated)
		}
	})

	t.Run("NoOpOnLocalBackend", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		set := false
		mocks.ConfigHandler.(*config.MockConfigHandler).SetFunc = func(_ string, _ any) error { set = true; return nil }
		migrated := false
		mockStack := terraforminfra.NewMockStack()
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) { migrated = true; return nil, nil }
		prov := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		pivoted, err := prov.PrepareLocalTeardown(bp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pivoted || set || migrated {
			t.Errorf("expected no-op on local backend, pivoted=%v set=%v migrated=%v", pivoted, set, migrated)
		}
	})

	t.Run("AbortsOnMigrationFailureWhileClusterReachable", func(t *testing.T) {
		// Given a migration failure while the cluster is still up — destroying now would orphan resources
		mocks := setupProvisionerMocks(t)
		set := false
		kubernetesBackend(mocks, &set)
		mockStack := terraforminfra.NewMockStack()
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			return nil, fmt.Errorf("boom")
		}
		dir := t.TempDir()
		_ = os.MkdirAll(filepath.Join(dir, ".kube"), 0o755)
		_ = os.WriteFile(filepath.Join(dir, ".kube", "config"), []byte("x"), 0o644)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, out func(string), nodes ...string) error {
			return nil
		}
		prov := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack, KubernetesManager: mocks.KubernetesManager})
		prov.configRoot = dir

		// When preparing, the failure aborts rather than proceeding against empty local state
		_, err := prov.PrepareLocalTeardown(bp)
		if err == nil || !strings.Contains(err.Error(), "migrate terraform state") {
			t.Errorf("expected abort on migration failure with cluster up, got %v", err)
		}
	})

	t.Run("ProceedsOnMigrationFailureWhenClusterGone", func(t *testing.T) {
		// Given a migration failure but the cluster is already gone (resume) — state is already local
		mocks := setupProvisionerMocks(t)
		set := false
		kubernetesBackend(mocks, &set)
		mockStack := terraforminfra.NewMockStack()
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			return nil, fmt.Errorf("dial tcp: connection refused")
		}
		prov := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})
		prov.configRoot = t.TempDir() // no kubeconfig → cluster gone

		// When preparing, it pivots and proceeds against the already-migrated local state
		pivoted, err := prov.PrepareLocalTeardown(bp)
		if err != nil {
			t.Fatalf("expected resume to proceed, got %v", err)
		}
		if !pivoted || !set {
			t.Errorf("expected pivot on resume, pivoted=%v set=%v", pivoted, set)
		}
	})
}

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
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

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
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

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
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

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

	t.Run("Stage2SkipsReachabilityCheckAfterClusterAlreadyDestroyed", func(t *testing.T) {
		// Stage 1 destroys the cluster component itself (it is never a tier
		// member), so by the time Stage 2 runs the kubeconfig on disk is stale
		// and WaitForKubernetesHealthy fails against a host that no longer
		// exists. That is the expected post-Stage-1 state, not broken auth —
		// Stage 2's tier-only destroy must still proceed.
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
		destroyAllCalls := 0
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalls++
			return terraforminfra.DestroyOutcome{}, nil
		}
		// Stage 1's own reachability check must still pass (the cluster is still up
		// going into Stage 1); only once Stage 1's DestroyAll has torn it down does
		// the kubeconfig on disk go stale, which is what Stage 2 must tolerate.
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			if destroyAllCalls == 0 {
				return nil
			}
			return fmt.Errorf("dial tcp: lookup cluster-w0820el4.hcp.eastus.azmk8s.io: no such host")
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		if _, err := provisioner.Teardown(bp, true, false); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if destroyAllCalls != 2 {
			t.Errorf("Expected Stage 1 + Stage 2 DestroyAll calls (2), got %d", destroyAllCalls)
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
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

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
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

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
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

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
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			return terraforminfra.DestroyOutcome{}, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) { return nil, nil }

		r, w, pipeErr := os.Pipe()
		if pipeErr != nil {
			t.Fatalf("Pipe failed: %v", pipeErr)
		}
		origStderr := os.Stderr
		os.Stderr = w
		defer func() { os.Stderr = origStderr }()

		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})
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
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

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
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

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
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

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
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

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

	t.Run("ContinueAttemptsTierWhenOnlyKustomizeFailed", func(t *testing.T) {
		// Given a tier blueprint where kustomize Uninstall fails but every
		// non-tier terraform component destroys cleanly. Kustomize does not
		// depend on terraform state, so the backend tier MUST still be
		// attempted — otherwise the tier is permanently deferred on every
		// rerun (kustomize is most likely to fail against a cluster that's
		// already partially gone).
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
		mocks.KubernetesManager.DeleteBlueprintFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error {
			return fmt.Errorf("cluster API unreachable")
		}
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
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mockStack,
			KubernetesManager: mocks.KubernetesManager,
		})

		// When Teardown runs the full destroy (terraformOnly=false) with continueOnError=true
		result, err := provisioner.Teardown(bp, false, true)

		// Then the kustomize failure is recorded, but the tier is NOT deferred —
		// terraform component failures are the only thing that should defer the tier.
		if err != nil {
			t.Fatalf("Expected continueOnError to absorb kustomize failure, got %v", err)
		}
		if result.TierDeferred {
			t.Error("Expected TierDeferred=false when only kustomize failed (kustomize does not depend on terraform state)")
		}
		if !migrateCalled {
			t.Error("Expected MigrateState to run when terraform stage 1 was clean")
		}
		if destroyAllCalls != 2 {
			t.Errorf("Expected Stage 1 + Stage 2 DestroyAll calls (2), got %d", destroyAllCalls)
		}
		var foundKustomize bool
		for _, f := range result.Failed {
			if f.ID == KustomizeFailureID {
				foundKustomize = true
			}
		}
		if !foundKustomize {
			t.Errorf("Expected kustomize failure to remain in result.Failed, got %v", result.Failed)
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
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

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
				provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

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
