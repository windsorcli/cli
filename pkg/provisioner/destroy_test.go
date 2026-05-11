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
			Backend: "backend",
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
			Backend: "backend",
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
			Backend: "backend",
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

	t.Run("KubernetesBackendMigratesAllThenDestroysAgainstLocal", func(t *testing.T) {
		// Given a kubernetes-configured backend with no explicit IsBackend()
		// module, the cluster module IS the backend storage — every component's
		// state lives inside the cluster the pivot provisions. The pivot dance
		// used for external backends (e.g. S3) cannot work here: destroying any
		// component that the cluster depends on kills the backend for every
		// remaining component. Teardown must pin local up front, migrate every
		// component's state out of the cluster, then destroy everything in
		// reverse order against local with no exclusions.
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
		var seenExclude []string
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, excludeIDs ...string) ([]string, error) {
			seenExclude = excludeIDs
			ops = append(ops, "destroyAll")
			return nil, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrateAll")
			return nil, nil
		}
		perComponentMigrate := false
		mockStack.MigrateComponentStateFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error {
			perComponentMigrate = true
			return nil
		}
		perComponentDestroy := false
		mockStack.DestroyFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) (bool, error) {
			perComponentDestroy = true
			return false, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if _, err := provisioner.Teardown(bp, true); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := []string{
			"set:local",
			"migrateAll",
			"destroyAll",
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
		if len(seenExclude) != 0 {
			t.Errorf("Expected DestroyAll to be called with no excludes, got %v", seenExclude)
		}
		if perComponentMigrate {
			t.Error("MigrateComponentState must not be called on the full-migration path")
		}
		if perComponentDestroy {
			t.Error("Per-component Destroy must not be called on the full-migration path")
		}
	})

	t.Run("KubernetesMigrationFailureAbortsDestroyAndRestoresBackend", func(t *testing.T) {
		// When MigrateState fails (e.g. the k8s API is intermittent during
		// migration, or a component's state can't be pulled), no destroy must
		// run — operating against partially-migrated state would tear down
		// resources without local-side bookkeeping, stranding the operator. The
		// configured backend restores via defer so the next command sees
		// kubernetes again, not the in-flight local override.
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
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate-fail")
			return nil, fmt.Errorf("cluster unreachable")
		}
		destroyAllCalled := false
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...string) ([]string, error) {
			destroyAllCalled = true
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.Teardown(bp, true)
		if err == nil {
			t.Fatal("Expected migration error to surface, got nil")
		}
		if !strings.Contains(err.Error(), "cluster unreachable") {
			t.Errorf("Expected underlying migration cause in surfaced message, got %v", err)
		}
		if destroyAllCalled {
			t.Error("DestroyAll must not run after MigrateState fails")
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

	t.Run("KubernetesWithExplicitIsBackendModuleUsesPivotDance", func(t *testing.T) {
		// When an operator explicitly marks one component IsBackend() while
		// running terraform.backend.type=kubernetes, that component owns the
		// backend storage out-of-band (the kubernetes namespace/secret backing
		// hostname lives outside the cluster the rest of the blueprint
		// provisions). The pivot dance applies: bulk-destroy non-backend
		// against the live remote, migrate the backend component, destroy it
		// last. This is the s3/azurerm flow, just with kubernetes as the
		// configured type — full-migration would be wrong here because other
		// components' state survives the bulk destroy.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend: "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
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
			"destroyAll:exclude=[backend]",
			"set:local",
			"migrate:backend",
			"destroy:backend",
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
			t.Error("Bulk MigrateState must not be called when an explicit IsBackend() component is present")
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

	t.Run("KubernetesFullMigrationMergesSkippedWithoutDuplicates", func(t *testing.T) {
		// MigrateState and DestroyAll independently report dir-missing
		// components: MigrateState because its loop stat-checks before
		// invoking terraform init, DestroyAll for the same reason in its
		// reverse-iter loop. Naive concatenation would double-count any
		// component both saw. The returned skipped slice must be the
		// dedup'd union, in input order (migration findings first, then
		// any destroy-side skips DestroyAll discovered that MigrateState
		// didn't surface — e.g. components with a dir but empty state).
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
		mockCH.SetFunc = func(_ string, _ any) error { return nil }
		mockStack := terraforminfra.NewMockStack()
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			return []string{"cni/cilium", "gitops/flux"}, nil
		}
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...string) ([]string, error) {
			return []string{"gitops/flux", "cluster/talos"}, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		skipped, err := provisioner.Teardown(bp, true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expected := []string{"cni/cilium", "gitops/flux", "cluster/talos"}
		if len(skipped) != len(expected) {
			t.Fatalf("Expected %d skipped IDs %v, got %d %v", len(expected), expected, len(skipped), skipped)
		}
		for i, want := range expected {
			if skipped[i] != want {
				t.Errorf("skipped[%d]: got %q, want %q (full: %v)", i, skipped[i], want, skipped)
			}
		}
	})

	t.Run("KubernetesFullMigrationSurfacesSkippedOnDestroyError", func(t *testing.T) {
		// When DestroyAll errors before iterating to every component,
		// MigrateState's skipped IDs are the only record we have of
		// components that were dir-missing at migration time. Returning
		// them paired with the error keeps the caller-visible contract
		// intact: the slice is what we know was skipped; the error is
		// what stopped us before we could enumerate more.
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
		mockCH.SetFunc = func(_ string, _ any) error { return nil }
		mockStack := terraforminfra.NewMockStack()
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			return []string{"gitops/flux"}, nil
		}
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...string) ([]string, error) {
			return nil, fmt.Errorf("destroy failed before completion")
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		skipped, err := provisioner.Teardown(bp, true)
		if err == nil {
			t.Fatal("Expected destroy error to surface, got nil")
		}
		if len(skipped) != 1 || skipped[0] != "gitops/flux" {
			t.Errorf("Expected MigrateState's skipped IDs paired with the error, got %v", skipped)
		}
	})
}

// =============================================================================
// TeardownComponent Tests
// =============================================================================

func TestProvisioner_TeardownComponent(t *testing.T) {
	bpWithBackend := func() *blueprintv1alpha1.Blueprint {
		return &blueprintv1alpha1.Blueprint{
			Backend: "backend",
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
