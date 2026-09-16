package provisioner

import (
	"context"
	"fmt"
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

	t.Run("RefusesBackendComponentOnKubernetesBackend", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		kubernetesBackend(mocks)
		prov := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{})
		err := prov.CheckComponentDestroyable(bp, "compute")
		if err == nil || !strings.Contains(err.Error(), "backend") {
			t.Errorf("expected refusal naming backend, got %v", err)
		}
	})

	t.Run("AllowsNonBackendComponentOnKubernetesBackend", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		kubernetesBackend(mocks)
		prov := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{})
		if err := prov.CheckComponentDestroyable(bp, "dns"); err != nil {
			t.Errorf("expected non-backend component allowed, got %v", err)
		}
	})

	t.Run("AllowsBackendComponentOnLocalBackend", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		prov := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{})
		if err := prov.CheckComponentDestroyable(bp, "compute"); err != nil {
			t.Errorf("expected local backend to allow any component, got %v", err)
		}
	})

	t.Run("RefusesWhenBackendFieldUnresolved", func(t *testing.T) {
		// Backend membership is undeterminable; refuse rather than guess.
		unresolved := &blueprintv1alpha1.Blueprint{
			Backend: "ghost",
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "compute", Path: "compute/hcloud"},
			},
		}
		mocks := setupProvisionerMocks(t)
		kubernetesBackend(mocks)
		prov := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{})
		err := prov.CheckComponentDestroyable(unresolved, "compute")
		if err == nil || !strings.Contains(err.Error(), `"ghost"`) {
			t.Errorf("expected refusal naming the unresolved backend, got %v", err)
		}
	})
}

func TestProvisioner_PivotToLocalIfClusterGone(t *testing.T) {
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
		pivoted, err := prov.PivotToLocalIfClusterGone()
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

		pivoted, err := prov.PivotToLocalIfClusterGone()
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

		pivoted, err := prov.PivotToLocalIfClusterGone()
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

	t.Run("NoOpOnNonKubernetesRemoteBackend", func(t *testing.T) {
		// Given a remote non-kubernetes backend (its state store isn't going away with the cluster)
		mocks := setupProvisionerMocks(t)
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetTerraformBackendTypeFunc = func() string { return "gcs" }
		set := false
		mockCH.SetFunc = func(_ string, _ any) error { set = true; return nil }
		migrated := false
		mockStack := terraforminfra.NewMockStack()
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) { migrated = true; return nil, nil }
		prov := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		// When preparing, it leaves the backend type alone so Teardown still gates the backend components
		pivoted, err := prov.PrepareLocalTeardown(bp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pivoted || set || migrated {
			t.Errorf("expected no-op on gcs backend, pivoted=%v set=%v migrated=%v", pivoted, set, migrated)
		}
	})

	t.Run("AbortsAndRevertsPivotOnMigrationFailureWhileClusterReachable", func(t *testing.T) {
		// Given a migration failure while the cluster is still up — destroying now would orphan resources
		mocks := setupProvisionerMocks(t)
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
		var sets []string
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				sets = append(sets, fmt.Sprintf("%v", value))
			}
			return nil
		}
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

		// When preparing, it aborts and reverts the pivot so nothing later reads a local backend with no
		// migrated state behind it
		_, err := prov.PrepareLocalTeardown(bp)
		if err == nil || !strings.Contains(err.Error(), "migrate terraform state") {
			t.Errorf("expected abort on migration failure with cluster up, got %v", err)
		}
		if len(sets) != 2 || sets[0] != "local" || sets[1] != "kubernetes" {
			t.Errorf("expected pivot to local then revert to kubernetes, got %v", sets)
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
	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		// checkOrphanedLocalState must fail gracefully on a nil blueprint, the same
		// way DestroyAll/DestroyAllTerraform already do downstream, rather than
		// panicking on blueprint.TerraformComponents.
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		_, err := provisioner.Teardown(nil, false, false)

		if err == nil {
			t.Fatal("Expected error for nil blueprint, got nil")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

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
		// Without Blueprint.Backend, there is no declared backend;
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
			t.Error("Expected no backend override when blueprint has no declared backend")
		}
		if len(seenExclude) != 0 {
			t.Errorf("Expected no excludes when there is no declared backend, got %v", seenExclude)
		}
		if migrateCalled {
			t.Error("MigrateState must not run when there is no declared backend")
		}
	})

	t.Run("WarnsAboutOrphanedLocalStateBeforeDestroying", func(t *testing.T) {
		// windsorcli/cli#3367: `windsor destroy` never ran the orphaned-local-state
		// check at all. Surface the same warning apply's Up already gives, before any
		// destroy runs.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
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
		mockStack := terraforminfra.NewMockStack()
		mockStack.ListLocalStateComponentIDsFunc = func() ([]string, error) {
			return []string{"vpc-old"}, nil
		}
		mockStack.HasLocalStateWithResourcesFunc = func(componentID string) (bool, error) {
			return componentID == "vpc-old", nil
		}
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			return terraforminfra.DestroyOutcome{}, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		var err error
		stderrOutput := captureStderr(t, func() {
			_, err = provisioner.Teardown(bp, true, false)
		})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !strings.Contains(stderrOutput, "vpc-old") {
			t.Errorf("Expected a warning naming the orphaned componentID, got: %q", stderrOutput)
		}
	})

	t.Run("OrphanInspectionFailureWarnsButDoesNotBlockDestroy", func(t *testing.T) {
		// The orphan check is advisory. A corrupted or unreadable state file under
		// some unrelated, already-orphaned componentID must not block destroy — the
		// tool an operator reaches for to recover from exactly this kind of drift.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
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
		mockStack := terraforminfra.NewMockStack()
		mockStack.ListLocalStateComponentIDsFunc = func() ([]string, error) {
			return []string{"vpc-old"}, nil
		}
		mockStack.HasLocalStateWithResourcesFunc = func(_ string) (bool, error) {
			return false, fmt.Errorf("corrupted terraform.tfstate: unexpected end of JSON input")
		}
		destroyAllCalled := false
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalled = true
			return terraforminfra.DestroyOutcome{}, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		var err error
		stderrOutput := captureStderr(t, func() {
			_, err = provisioner.Teardown(bp, true, false)
		})

		if err != nil {
			t.Fatalf("Expected no error — an orphan-inspection failure must only warn, got %v", err)
		}
		if !destroyAllCalled {
			t.Error("Expected DestroyAll to still run despite the orphan-inspection failure")
		}
		if !strings.Contains(stderrOutput, "vpc-old") {
			t.Errorf("Expected a warning naming the componentID that failed inspection, got: %q", stderrOutput)
		}
	})

	t.Run("RefusesUnresolvedBackendField", func(t *testing.T) {
		// Backend names no real component; must not collapse to "no backend".
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend: "backend",
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "network/gcp-vpc"},
				{Path: "cluster/gcp-gke"},
			},
		}
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "gcs"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mockStack := terraforminfra.NewMockStack()
		destroyAllCalled := false
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalled = true
			return terraforminfra.DestroyOutcome{}, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		_, err := provisioner.Teardown(bp, true, false)
		if err == nil {
			t.Fatal("Expected error for unresolved Backend field, got nil")
		}
		if !strings.Contains(err.Error(), `"backend"`) {
			t.Errorf("Expected error to name the unresolved backend, got: %v", err)
		}
		if destroyAllCalled {
			t.Error("DestroyAll must not run when the backend components cannot be resolved")
		}
	})

	t.Run("SingleBackendComponentDestroysNonBackendThenBackend", func(t *testing.T) {
		// Stage 1: destroy non-backend against the configured backend.
		// Stage 2: pin local, migrate backend-component state to local, destroy the backend components against local.
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
			t.Errorf("Stage 2 DestroyAll should target the backend [backend], got %#v", stage2Components)
		}
	})

	t.Run("MultiComponentBackendDestroysMembersThenBackend", func(t *testing.T) {
		// VPC + IAM + cluster as the backend components, Backend="cluster" (the last-declared
		// member, not the first). Stage 1 destroys non-backend (workloads) against the
		// configured backend; Stage 2a migrates all three backend components. state to
		// local and destroys the other backend components (vpc, iam); Stage 2b
		// destroys the backend component (cluster) alone, only after Stage 2a comes
		// back clean.
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

		if len(destroyAllBlueprints) != 3 {
			t.Fatalf("Expected three DestroyAll calls (Stage 1, Stage 2a, Stage 2b), got %d", len(destroyAllBlueprints))
		}
		expectedStage1Excludes := []string{"networking/vpc", "iam", "cluster"}
		if len(destroyAllExcludes[0]) != 3 {
			t.Fatalf("Stage 1 excludes should list all 3 backend component IDs, got %v", destroyAllExcludes[0])
		}
		for i, want := range expectedStage1Excludes {
			if destroyAllExcludes[0][i] != want {
				t.Errorf("Stage 1 exclude[%d]: got %q, want %q", i, destroyAllExcludes[0][i], want)
			}
		}

		stage2aComponents := destroyAllBlueprints[1].TerraformComponents
		if len(stage2aComponents) != 3 {
			t.Fatalf("Stage 2a should still target the full 3-component backend set (member exclusion happens via excludeIDs), got %d", len(stage2aComponents))
		}
		if len(destroyAllExcludes[1]) != 1 || destroyAllExcludes[1][0] != "cluster" {
			t.Errorf("Stage 2a should exclude only the backend component %q, got %v", "cluster", destroyAllExcludes[1])
		}

		stage2bComponents := destroyAllBlueprints[2].TerraformComponents
		if len(stage2bComponents) != 1 || stage2bComponents[0].GetID() != "cluster" {
			t.Fatalf("Stage 2b should target only the backend component [cluster], got %#v", stage2bComponents)
		}
		if len(destroyAllExcludes[2]) != 0 {
			t.Errorf("Stage 2b should exclude nothing, got %v", destroyAllExcludes[2])
		}

		if len(migrateBlueprints) != 1 {
			t.Fatalf("Expected one MigrateState call, got %d", len(migrateBlueprints))
		}
		if len(migrateBlueprints[0].TerraformComponents) != 3 {
			t.Errorf("MigrateState should target the 3-component backend set, got %d", len(migrateBlueprints[0].TerraformComponents))
		}
	})

	t.Run("MultiComponentBackendMateFailureDefersBackend", func(t *testing.T) {
		// Regression for #3355: a backend with more than one component (vpc + cluster,
		// Backend="cluster") where a fellow backend component (vpc) fails during Stage 2a. The
		// backend component (cluster) must NOT be destroyed in the same pass —
		// otherwise every other backend component.s state (and any `--continue` retry)
		// is stranded once the backend that stores it is gone.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "cluster",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "networking/vpc"},
				{Name: "cluster", Path: "cluster/eks"},
			},
		}
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "gcs"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mockCH.SetFunc = func(_ string, _ any) error { return nil }

		destroyAllCalls := 0
		var stage2bCalled bool
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(b *blueprintv1alpha1.Blueprint, _ bool, excludeIDs ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalls++
			if destroyAllCalls == 1 {
				// Stage 1: no non-backend components in this blueprint.
				return terraforminfra.DestroyOutcome{}, nil
			}
			if len(excludeIDs) == 1 && excludeIDs[0] == "cluster" {
				// Stage 2a: the fellow backend component fails.
				return terraforminfra.DestroyOutcome{
					Failed: []terraforminfra.ComponentFailure{{ID: "networking/vpc", Err: fmt.Errorf("network still in use")}},
				}, nil
			}
			// Stage 2b would destroy the backend component alone — must not be reached.
			stage2bCalled = true
			return terraforminfra.DestroyOutcome{Destroyed: []string{"cluster"}}, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) { return nil, nil }
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		result, err := provisioner.Teardown(bp, true, true)
		if err != nil {
			t.Fatalf("Expected continueOnError to absorb the fellow-component failure, got %v", err)
		}
		if stage2bCalled {
			t.Error("Backend component must not be destroyed in the same pass a fellow backend component failed in")
		}
		if !result.TerraformDeferred {
			t.Error("Expected TerraformDeferred=true when a fellow backend component failed during Stage 2a")
		}
		if len(result.Failed) != 1 || result.Failed[0].ID != "networking/vpc" {
			t.Errorf("Expected the fellow-component failure to surface in result.Failed, got %v", result.Failed)
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
			return terraforminfra.DestroyOutcome{}, fmt.Errorf("non-backend destroy failed")
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
		if !strings.Contains(err.Error(), "non-backend destroy failed") {
			t.Errorf("Expected error to wrap Stage 1 failure, got %v", err)
		}
		if migrateCalled {
			t.Error("MigrateState must not run after Stage 1 fails")
		}
		if setCalled {
			t.Error("Backend override must not engage after Stage 1 fails")
		}
	})

	t.Run("Stage2MigrationFailureAbortsBackendDestroyAndRestoresBackend", func(t *testing.T) {
		// When the Stage 2 MigrateState fails (e.g. configured backend is intermittent),
		// no backend destroy may run — operating against partially-migrated state could
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
			t.Errorf("Stage 2 backend destroy must not run when migration fails (expected 1 DestroyAll from Stage 1, got %d)", destroyAllCalls)
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
		// successful backend destroy, the call still returns nil but the operator
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

		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})
		var err error
		stderrOutput := captureStderr(t, func() {
			_, err = provisioner.Teardown(bp, true, false)
		})

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

	t.Run("ContinueDefersTerraformWhenNonBackendHasFailures", func(t *testing.T) {
		// Given a blueprint with a declared backend and a non-backend component whose
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

		// Then no error is returned (failure is collected), terraform is deferred,
		// and Stage 2 backend migration never engages
		if err != nil {
			t.Fatalf("Expected continueOnError to absorb per-component failure, got %v", err)
		}
		if !result.TerraformDeferred {
			t.Error("Expected TerraformDeferred=true when non-backend destroy left a failure")
		}
		if len(result.Failed) != 1 || result.Failed[0].ID != "cluster" {
			t.Errorf("Expected failures to include cluster, got %v", result.Failed)
		}
		if migrateCalled {
			t.Error("MigrateState must not run when non-backend failures left terraform deferred")
		}
		if setCalled {
			t.Error("Backend override must not engage when terraform is deferred")
		}
	})

	t.Run("ContinueAttemptsTerraformWhenStage1Clean", func(t *testing.T) {
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

		// Then terraform is attempted (Stage 2 runs) and TerraformDeferred stays false
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result.TerraformDeferred {
			t.Error("Expected TerraformDeferred=false when Stage 1 was clean")
		}
		if !migrateCalled {
			t.Error("Expected MigrateState to run when Stage 1 produced no failures")
		}
		if destroyAllCalls != 2 {
			t.Errorf("Expected Stage 1 + Stage 2 DestroyAll calls (2), got %d", destroyAllCalls)
		}
	})

	t.Run("StillDefersTerraformAfterPrepareLocalTeardownOnRemoteBackend", func(t *testing.T) {
		// Given a gcs backend whose PrepareLocalTeardown ran first, then a non-backend
		// destroy failure under continueOnError mode
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
		}
		backendType := "gcs"
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetTerraformBackendTypeFunc = func() string { return backendType }
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				backendType = fmt.Sprintf("%v", value)
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			return terraforminfra.DestroyOutcome{
				Failed: []terraforminfra.ComponentFailure{{ID: "cluster", Err: fmt.Errorf("cluster destroy failed")}},
			}, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) { return nil, nil }
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		// When PrepareLocalTeardown runs first, then Teardown with continueOnError=true
		if _, err := provisioner.PrepareLocalTeardown(bp); err != nil {
			t.Fatalf("Expected no error from PrepareLocalTeardown, got %v", err)
		}
		result, err := provisioner.Teardown(bp, false, true)

		// Then the backend type is untouched, terraform is still deferred, and the
		// backend component is never destroyed alongside the failed component
		if err != nil {
			t.Fatalf("Expected continueOnError to absorb per-component failure, got %v", err)
		}
		if backendType != "gcs" {
			t.Errorf("Expected PrepareLocalTeardown to leave terraform.backend.type=gcs untouched, got %q", backendType)
		}
		if !result.TerraformDeferred {
			t.Error("Expected TerraformDeferred=true when non-backend destroy left a failure, even after PrepareLocalTeardown ran")
		}
	})

	t.Run("ContinueCollectsLocalBackendFailures", func(t *testing.T) {
		// Given a local backend (no declared backend) and a stack that reports per-component
		// failures under continueOnError mode
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

	t.Run("ContinueDefersTerraformWhenKustomizeFailsAndClusterReachable", func(t *testing.T) {
		// Given a declared-backend blueprint where kustomize Uninstall fails while the cluster is
		// still reachable. A live controller (e.g. Crossplane) may still be mid-teardown
		// of a cloud-backed resource terraform never tracked, so terraform —
		// which would destroy the cluster hosting that controller — MUST be deferred,
		// not attempted. See #3385.
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
			return fmt.Errorf("destroy aborted: timeout waiting for load balancer teardown")
		}
		// WaitForKubernetesHealthyFunc left unset: the default mock returns nil, i.e. the
		// cluster is reachable — the scenario this test exercises.
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

		// Then the kustomize failure is recorded and terraform is deferred entirely —
		// Stage 1 itself aborts before touching terraform (via Provisioner.DestroyAll's
		// own gate), so Stage 2/3 never run either.
		if err != nil {
			t.Fatalf("Expected continueOnError to absorb kustomize failure, got %v", err)
		}
		if !result.TerraformDeferred {
			t.Error("Expected TerraformDeferred=true when kustomize failed while the cluster is reachable")
		}
		if migrateCalled {
			t.Error("MigrateState must not run when terraform is deferred")
		}
		if destroyAllCalls != 0 {
			t.Errorf("Expected no terraform DestroyAll calls, got %d", destroyAllCalls)
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

	t.Run("ContinueAttemptsTerraformWhenKustomizeFailsAndClusterUnreachable", func(t *testing.T) {
		// Given a declared-backend blueprint where kustomize Uninstall fails because the cluster
		// itself is unreachable. Nothing is left alive to orphan, and deferring here
		// would deadlock every rerun — kustomize is most likely to fail against a
		// cluster that's already gone. Terraform MUST still be attempted.
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
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(_ context.Context, _ string, _ func(string), _ ...string) error {
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

		// Then the kustomize failure is recorded, but terraform is NOT deferred.
		if err != nil {
			t.Fatalf("Expected continueOnError to absorb kustomize failure, got %v", err)
		}
		if result.TerraformDeferred {
			t.Error("Expected TerraformDeferred=false when kustomize failed because the cluster is unreachable")
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

func TestProvisioner_blocksNextStage(t *testing.T) {
	tests := []struct {
		name      string
		failed    []ComponentFailure
		reachable bool
		want      bool
	}{
		{
			name:      "NoFailures",
			failed:    nil,
			reachable: true,
			want:      false,
		},
		{
			name:      "TerraformFailureAlwaysBlocks",
			failed:    []ComponentFailure{{ID: "cluster", Err: fmt.Errorf("boom")}},
			reachable: false,
			want:      true,
		},
		{
			name:      "KustomizeFailureBlocksWhileClusterReachable",
			failed:    []ComponentFailure{{ID: KustomizeFailureID, Err: fmt.Errorf("boom")}},
			reachable: true,
			want:      true,
		},
		{
			name:      "KustomizeFailureDoesNotBlockWhenClusterUnreachable",
			failed:    []ComponentFailure{{ID: KustomizeFailureID, Err: fmt.Errorf("boom")}},
			reachable: false,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks := setupProvisionerMocks(t)
			if !tt.reachable {
				mocks.Runtime.ConfigRoot = t.TempDir()
			}
			provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})

			got := provisioner.blocksNextStage(tt.failed)

			if got != tt.want {
				t.Errorf("blocksNextStage() = %v, want %v", got, tt.want)
			}
		})
	}
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

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		// checkOrphanedLocalState must fail gracefully on a nil blueprint, the same
		// way Destroy already does downstream, rather than panicking on
		// blueprint.TerraformComponents.
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		_, err := provisioner.TeardownComponent(nil, "cluster")

		if err == nil {
			t.Fatal("Expected error for nil blueprint, got nil")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("RefusesAnyBackendComponentOnRemoteBackend", func(t *testing.T) {
		// A backend component on any remote backend (s3, azurerm, kubernetes) is refused —
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
				if !strings.Contains(err.Error(), "backend component") {
					t.Errorf("Expected refusal message naming the backend component, got: %v", err)
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

	t.Run("RefusesPreBackendComponentOnRemoteBackend", func(t *testing.T) {
		// A component declared before the named backend is also a backend component
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
			t.Fatal("Expected refusal for a pre-backend component, got nil")
		}
		if !strings.Contains(err.Error(), "networking/vpc") {
			t.Errorf("Expected error to name the backend component, got: %v", err)
		}
		if destroyCalled {
			t.Error("Destroy must not run when refusing")
		}
	})

	t.Run("AllowsBackendComponentOnLocalBackend", func(t *testing.T) {
		// On a local backend there is no shared remote storage to orphan; a backend component
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

	t.Run("NonBackendComponentUsesDirectDestroyOnAnyBackend", func(t *testing.T) {
		// Non-backend components destroy directly regardless of backend type — the
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
					t.Error("Expected MigrateComponentState NOT to be called for non-backend component")
				}
				if setCalled {
					t.Error("Expected backend.type Set NOT to be called for non-backend component")
				}
			})
		}
	})

	t.Run("WarnsAboutOrphanedLocalStateBeforeDestroying", func(t *testing.T) {
		// windsorcli/cli#3367: a targeted `windsor destroy terraform <component>` never
		// ran the orphaned-local-state check at all. Surface the same warning here as
		// on a full Teardown, so a drifted componentID is flagged before destroy runs.
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
		mockStack.ListLocalStateComponentIDsFunc = func() ([]string, error) {
			return []string{"cluster-old"}, nil
		}
		mockStack.HasLocalStateWithResourcesFunc = func(componentID string) (bool, error) {
			return componentID == "cluster-old", nil
		}
		mockStack.DestroyFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) (bool, error) { return false, nil }
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		var err error
		stderrOutput := captureStderr(t, func() {
			_, err = provisioner.TeardownComponent(bpWithBackend(), "cluster")
		})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !strings.Contains(stderrOutput, "cluster-old") {
			t.Errorf("Expected a warning naming the orphaned componentID, got: %q", stderrOutput)
		}
	})
}
