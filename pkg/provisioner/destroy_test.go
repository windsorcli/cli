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
// Teardown Tests
// =============================================================================

func TestProvisioner_Teardown(t *testing.T) {
	t.Run("DestroysNonBackendThenMigratesAndDestroysBackend", func(t *testing.T) {
		// Given a blueprint that declares both a "backend" and a non-backend
		// terraform component, and the configured backend is non-local (s3), the
		// symmetric flow must: destroy non-backend components first against the
		// live remote backend (excludeIDs="backend"), then pin backend.type=local,
		// migrate just the backend component's state, destroy it, and restore the
		// configured backend on defer.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
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
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, excludeIDs ...string) ([]string, error) {
			ops = append(ops, fmt.Sprintf("destroyAll:exclude=%v", excludeIDs))
			return nil, nil
		}
		mockStack.MigrateComponentStateFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) error {
			ops = append(ops, fmt.Sprintf("migrate:%s", componentID))
			return nil
		}
		mockStack.DestroyFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
			ops = append(ops, fmt.Sprintf("destroy:%s", componentID))
			return false, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if _, err := provisioner.Teardown(bp, true); err != nil {
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
		// half-migrated state would corrupt the bucket teardown. The configured
		// backend is restored via defer for subsequent operations.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
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
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...string) ([]string, error) {
			ops = append(ops, "destroyAll")
			return nil, nil
		}
		mockStack.MigrateComponentStateFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error {
			ops = append(ops, "migrate-fail")
			return fmt.Errorf("remote backend unreachable")
		}
		destroyCalled := false
		mockStack.DestroyFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) (bool, error) {
			destroyCalled = true
			return false, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.Teardown(bp, true)

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
		// so the call returns nil — but silent restore failure would be a
		// debugging black hole.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
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
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...string) ([]string, error) {
			return nil, nil
		}
		mockStack.MigrateComponentStateFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error { return nil }
		mockStack.DestroyFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) (bool, error) { return false, nil }

		r, w, pipeErr := os.Pipe()
		if pipeErr != nil {
			t.Fatalf("Pipe failed: %v", pipeErr)
		}
		origStderr := os.Stderr
		os.Stderr = w
		defer func() { os.Stderr = origStderr }()

		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})
		_, err := provisioner.Teardown(bp, true)

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

	t.Run("SkipsBackendDanceWhenNoBackendComponent", func(t *testing.T) {
		// Given a blueprint with no backend component, the destroy flow collapses
		// to a single DestroyAllTerraform pass with no migration dance. This is the
		// path for blueprints that reference an out-of-band remote backend.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
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
		var seenExclude []string
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, excludeIDs ...string) ([]string, error) {
			seenExclude = excludeIDs
			ops = append(ops, "destroyAll")
			return nil, nil
		}
		migrateCalled := false
		mockStack.MigrateComponentStateFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error {
			migrateCalled = true
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if _, err := provisioner.Teardown(bp, true); err != nil {
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

	t.Run("KubernetesBackendUsesPivotDanceWithFirstComponent", func(t *testing.T) {
		// Given a kubernetes-configured backend with no explicit IsBackend()
		// module, the cluster module IS the backend. Teardown must mirror
		// bootstrap's pivot dance reversed: destroy non-pivot components first
		// against the live k8s API (cluster still healthy), then pin local,
		// migrate the cluster's own state out, destroy the cluster last.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/talos"},
				{Path: "cni/cilium"},
				{Path: "gitops/flux"},
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

		var ops []string
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				ops = append(ops, fmt.Sprintf("set:%v", value))
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, excludeIDs ...string) ([]string, error) {
			ops = append(ops, fmt.Sprintf("destroyAll:exclude=%v", excludeIDs))
			return nil, nil
		}
		mockStack.MigrateComponentStateFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) error {
			ops = append(ops, fmt.Sprintf("migrate:%s", componentID))
			return nil
		}
		mockStack.DestroyFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
			ops = append(ops, fmt.Sprintf("destroy:%s", componentID))
			return false, nil
		}
		migrateAllCalled := false
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			migrateAllCalled = true
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if _, err := provisioner.Teardown(bp, true); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := []string{
			"destroyAll:exclude=[cluster/talos]",
			"set:local",
			"migrate:cluster/talos",
			"destroy:cluster/talos",
			"set:kubernetes",
		}
		if len(ops) != len(expected) {
			t.Fatalf("Expected %d ops %v, got %d %v", len(expected), expected, len(ops), ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
		if migrateAllCalled {
			t.Error("Expected bulk MigrateState NOT to be called on the unified pivot path")
		}
	})

	t.Run("PivotMigrationFailureAbortsPivotDestroyOnKubernetes", func(t *testing.T) {
		// Mirrors BackendMigrationFailureAbortsBackendDestroyAndRestoresBackend
		// for the kubernetes-pivot case. Migration of the cluster's state from
		// the cluster's k8s backend to local fails — the cluster destroy must
		// not run, and the configured backend must restore via defer.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/talos"},
				{Path: "gitops/flux"},
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

		var ops []string
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				ops = append(ops, fmt.Sprintf("set:%v", value))
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...string) ([]string, error) {
			ops = append(ops, "destroyAll")
			return nil, nil
		}
		mockStack.MigrateComponentStateFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error {
			ops = append(ops, "migrate-fail")
			return fmt.Errorf("cluster unreachable")
		}
		destroyCalled := false
		mockStack.DestroyFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) (bool, error) {
			destroyCalled = true
			return false, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.Teardown(bp, true)
		if err == nil {
			t.Fatal("Expected migration error to surface, got nil")
		}
		if !strings.Contains(err.Error(), "cluster unreachable") {
			t.Errorf("Expected underlying migration cause in surfaced message, got %v", err)
		}
		if destroyCalled {
			t.Error("Pivot Destroy must not run after MigrateComponentState fails")
		}
		expected := []string{"destroyAll", "set:local", "migrate-fail", "set:kubernetes"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected %d ops %v, got %d %v", len(expected), expected, len(ops), ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
	})

	t.Run("LocalBackendCollapsesToDirectDestroyAll", func(t *testing.T) {
		// Given a local (or unset) backend, no migration dance fires — the call
		// is a straight pass-through to DestroyAllTerraform. This is the no-op
		// path for users who never configured a remote backend.
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
		destroyAllCalled := false
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...string) ([]string, error) {
			destroyAllCalled = true
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if _, err := provisioner.Teardown(createTestBlueprint(), true); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !destroyAllCalled {
			t.Error("Expected DestroyAllTerraform to be called for local backend")
		}
		if setCalled {
			t.Error("Expected backend.type Set to NOT be called when backend is already local")
		}
	})
}

// =============================================================================
// TeardownComponent Tests
// =============================================================================

func TestProvisioner_TeardownComponent(t *testing.T) {
	bpWithBackend := func() *blueprintv1alpha1.Blueprint {
		return &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
		}
	}

	t.Run("RefusesPivotComponentOnAnyRemoteBackend", func(t *testing.T) {
		// Targeting the bootstrap pivot on any remote backend would orphan
		// state for every other component — the storage backing the configured
		// remote backend is what the pivot's state lives in. Refusal applies
		// symmetrically across remote backend types; operator must use the
		// full-cycle `windsor destroy`.
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
				if !strings.Contains(err.Error(), "bootstrap pivot component") {
					t.Errorf("Expected refusal message naming the pivot, got: %v", err)
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

	t.Run("RefusesClusterPivotOnKubernetesWithoutExplicitBackendModule", func(t *testing.T) {
		// Given backendType=kubernetes and no IsBackend() module, the cluster
		// module IS the pivot. Targeting it directly would orphan state for
		// every other component the same way targeting "backend" would on
		// s3/azurerm — refusal must fire on this case too. This is the gap the
		// old TeardownComponent (keyed on BackendComponentID alone) missed.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/talos"},
				{Path: "gitops/flux"},
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
		destroyCalled := false
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) (bool, error) {
			destroyCalled = true
			return false, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.TeardownComponent(bp, "cluster/talos")
		if err == nil {
			t.Fatal("Expected refusal for direct destroy of cluster pivot, got nil")
		}
		if !strings.Contains(err.Error(), "cluster/talos") {
			t.Errorf("Expected error to name cluster/talos, got: %v", err)
		}
		if !strings.Contains(err.Error(), "bootstrap pivot component") {
			t.Errorf("Expected error to mention the pivot, got: %v", err)
		}
		if destroyCalled {
			t.Error("Destroy must not run when refusing the pivot")
		}
	})

	t.Run("AllowsBackendComponentOnLocalBackend", func(t *testing.T) {
		// Local backend has no shared bucket/cluster; destroying the "backend"
		// component (whatever it is, since there is no remote storage tied to
		// it) is just a plain destroy.
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

	t.Run("NonBackendComponentUsesDirectDestroy", func(t *testing.T) {
		// Non-backend components destroy directly regardless of backend type —
		// the bucket/cluster still exists, no migration needed.
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
					t.Error("Expected MigrateComponentState NOT to be called for non-backend component")
				}
				if setCalled {
					t.Error("Expected backend.type Set NOT to be called for non-backend component")
				}
			})
		}
	})
}
