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
// DestroyAllWithBackendLifecycle Tests
// =============================================================================

func TestProvisioner_DestroyAllWithBackendLifecycle(t *testing.T) {
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

		if _, err := provisioner.DestroyAllWithBackendLifecycle(bp, true); err != nil {
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

		_, err := provisioner.DestroyAllWithBackendLifecycle(bp, true)

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
		_, err := provisioner.DestroyAllWithBackendLifecycle(bp, true)

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

		if _, err := provisioner.DestroyAllWithBackendLifecycle(bp, true); err != nil {
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
		// cycle path: pin backend.type to local, MigrateState pulls every
		// component's state from the cluster's Secrets to local files, DestroyAll
		// tears everything down in reverse against local state, restore on defer.
		// The per-component dance with excludeIDs must NOT fire — kubernetes can't
		// peel the backend off because the cluster IS the backend, and once the
		// cluster is going away every component's state has to be local already.
		mocks := setupProvisionerMocks(t)
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
			ops = append(ops, "migrate-all")
			return nil, nil
		}
		var seenExclude []string
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, excludeIDs ...string) ([]string, error) {
			seenExclude = excludeIDs
			ops = append(ops, "destroyAll")
			return nil, nil
		}
		migrateComponentCalled := false
		mockStack.MigrateComponentStateFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error {
			migrateComponentCalled = true
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if _, err := provisioner.DestroyAllWithBackendLifecycle(createTestBlueprint(), true); err != nil {
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
		mocks := setupProvisionerMocks(t)
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
		destroyCalled := false
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...string) ([]string, error) {
			destroyCalled = true
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.DestroyAllWithBackendLifecycle(createTestBlueprint(), true)

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

	t.Run("KubernetesBackendMigrationSkipAbortsDestroy", func(t *testing.T) {
		// Given the kubernetes full-cycle destroy's MigrateState reports a non-empty
		// skip list with no error — i.e. one or more component directories were
		// missing on disk and got skipped silently. Their state may still live on
		// the cluster's kubernetes Secret store, which is about to be destroyed.
		// DestroyAll must NOT proceed: doing so would mark those components as
		// "empty state" and leave their cloud resources orphaned with no terraform
		// record anywhere. The error must name the skipped component IDs so the
		// operator can investigate, and the configured backend must still restore
		// via defer for any subsequent operations in the same process.
		mocks := setupProvisionerMocks(t)
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
			ops = append(ops, "migrate-skip")
			return []string{"vpc", "iam"}, nil
		}
		destroyCalled := false
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...string) ([]string, error) {
			destroyCalled = true
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.DestroyAllWithBackendLifecycle(createTestBlueprint(), true)

		if err == nil {
			t.Fatal("Expected skip-list to surface as a hard error, got nil")
		}
		if destroyCalled {
			t.Error("DestroyAll must not run after MigrateState skips components on the kubernetes path")
		}
		msg := err.Error()
		// The skipped IDs must appear by name so the operator can investigate which
		// component dirs went missing — without them the operator gets a generic
		// "skipped components" message and no diagnostic foothold.
		for _, id := range []string{"vpc", "iam"} {
			if !strings.Contains(msg, id) {
				t.Errorf("Expected error to name skipped component %q, got %v", id, err)
			}
		}
		// The orphaned-resources framing is the load-bearing piece of the message —
		// asserting it locks in the explanation that distinguishes this hard-stop
		// from a routine "skipped" warning.
		if !strings.Contains(msg, "orphan") {
			t.Errorf("Expected error to explain the orphaned-resources risk, got %v", err)
		}
		expected := []string{"set:local", "migrate-skip", "set:kubernetes"}
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

		if _, err := provisioner.DestroyAllWithBackendLifecycle(createTestBlueprint(), true); err != nil {
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
// DestroyTerraformComponentWithBackendLifecycle Tests
// =============================================================================

func TestProvisioner_DestroyTerraformComponentWithBackendLifecycle(t *testing.T) {
	t.Run("MigratesAndDestroysBackendComponentOnS3", func(t *testing.T) {
		// Given the configured backend is s3 and the operator targets the backend
		// component itself, its state must be migrated to local before destroy —
		// destroying the bucket against the bucket's own remote state is a chicken-
		// and-egg failure.
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
		mockStack.MigrateComponentStateFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) error {
			ops = append(ops, fmt.Sprintf("migrate:%s", componentID))
			return nil
		}
		mockStack.DestroyFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
			ops = append(ops, fmt.Sprintf("destroy:%s", componentID))
			return false, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if _, err := provisioner.DestroyTerraformComponentWithBackendLifecycle(bp, "backend"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := []string{"set:local", "migrate:backend", "destroy:backend", "set:s3"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected %d ops %v, got %d %v", len(expected), expected, len(ops), ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
	})

	t.Run("NonBackendComponentOnS3UsesDirectDestroy", func(t *testing.T) {
		// Given the configured backend is s3 and the operator targets a non-
		// backend component, destroy runs directly against the live remote backend
		// — no migration needed, the bucket still exists.
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

		if _, err := provisioner.DestroyTerraformComponentWithBackendLifecycle(bp, "cluster"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if !destroyCalled {
			t.Error("Expected Destroy to be called")
		}
		if migrateCalled {
			t.Error("Expected MigrateComponentState NOT to run for non-backend component on s3")
		}
		if setCalled {
			t.Error("Expected backend.type Set to NOT be called for non-backend component on s3")
		}
	})

	t.Run("KubernetesBackendUsesDirectDestroy", func(t *testing.T) {
		// Given a kubernetes-configured backend, single-component destroy is a
		// direct call — the cluster (and its Secret-backed state store) is still
		// alive, and the full-cycle dance is reserved for bulk destroy when the
		// cluster itself is going away.
		mocks := setupProvisionerMocks(t)
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
		setCalled := false
		mockCH.SetFunc = func(_ string, _ any) error {
			setCalled = true
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		destroyCalled := false
		mockStack.DestroyFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) (bool, error) {
			destroyCalled = true
			return false, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if _, err := provisioner.DestroyTerraformComponentWithBackendLifecycle(createTestBlueprint(), "remote/path"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !destroyCalled {
			t.Error("Expected direct Destroy on kubernetes path")
		}
		if setCalled {
			t.Error("Expected backend.type Set to NOT be called on kubernetes path for single-component destroy")
		}
	})
}
