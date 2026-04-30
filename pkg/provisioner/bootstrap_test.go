package provisioner

import (
	"fmt"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Bootstrap Tests
// =============================================================================

func TestProvisioner_Bootstrap(t *testing.T) {
	t.Run("OverridesUpAndMigratesWhenBackendComponentPresent", func(t *testing.T) {
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
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			return nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		applied, err := provisioner.Bootstrap(bp, nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !applied {
			t.Fatal("Expected applied=true with no confirm callback")
		}

		expected := []string{"set:local", "up", "set:kubernetes", "migrate"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected %d ops %v, got %d %v", len(expected), expected, len(ops), ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
	})

	t.Run("NoBackendComponentCollapsesToUp", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
			},
		}

		var ops []string
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				ops = append(ops, fmt.Sprintf("set:%v", value))
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			return nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		applied, err := provisioner.Bootstrap(bp, nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !applied {
			t.Fatal("Expected applied=true with no confirm callback")
		}
		if len(ops) != 1 || ops[0] != "up" {
			t.Errorf("Expected single Up call, got %v", ops)
		}
	})

	t.Run("UpFailureRestoresBackend", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
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
		var setValues []any
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				setValues = append(setValues, value)
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			return fmt.Errorf("forbidden")
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.Bootstrap(bp, nil)
		if err == nil {
			t.Fatal("Expected error when Up fails")
		}
		if len(setValues) < 2 || setValues[len(setValues)-1] != "s3" {
			t.Errorf("Expected deferred restore to land on s3, got %v", setValues)
		}
	})

	t.Run("MigrateStateErrorSurfaces", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
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
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			return nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			return nil, fmt.Errorf("backend bucket unreachable")
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if _, err := provisioner.Bootstrap(bp, nil); err == nil {
			t.Fatal("Expected error when MigrateState fails")
		}
	})

	t.Run("FailsWhenMigrateStateReportsSkippedComponents", func(t *testing.T) {
		// MigrateState skipping silently would leave state on local while config
		// points at remote. Bootstrap surfaces the offending IDs so the operator
		// can investigate.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
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
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			return nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			return []string{"network", "cluster"}, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.Bootstrap(bp, nil)
		if err == nil {
			t.Fatal("Expected bootstrap to fail when MigrateState reports skipped components")
		}
		if !strings.Contains(err.Error(), "skipped") {
			t.Errorf("Expected error to mention skipped components, got: %v", err)
		}
		if !strings.Contains(err.Error(), "network") || !strings.Contains(err.Error(), "cluster") {
			t.Errorf("Expected error to name the skipped components, got: %v", err)
		}
	})

	t.Run("NilBlueprintReturnsError", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		if _, err := provisioner.Bootstrap(nil, nil); err == nil {
			t.Fatal("Expected error for nil blueprint")
		}
	})

	t.Run("ConfirmCallbackPlansBeforeApplyWithBackendOverride", func(t *testing.T) {
		// Given a confirm callback, Bootstrap plans first under the same
		// backend.type=local override the apply uses, calls confirm with the
		// summary, then applies if confirm returns true. This guards the
		// shared-override invariant: terraform init for the plan must not
		// reach a remote backend that does not exist yet.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
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
		var ops []string
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				ops = append(ops, fmt.Sprintf("set:%v", value))
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.PlanSummaryFunc = func(_ *blueprintv1alpha1.Blueprint) []terraforminfra.TerraformComponentPlan {
			ops = append(ops, "plan")
			return []terraforminfra.TerraformComponentPlan{
				{ComponentID: "backend", Add: 3},
				{ComponentID: "vpc", Add: 1},
			}
		}
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			return nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		var receivedSummary *PlanSummary
		confirm := func(s *PlanSummary) bool {
			ops = append(ops, "confirm")
			receivedSummary = s
			return true
		}

		applied, err := provisioner.Bootstrap(bp, confirm)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !applied {
			t.Fatal("Expected applied=true when confirm returned true")
		}

		expected := []string{"set:local", "plan", "confirm", "up", "set:s3", "migrate"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected ops %v, got %v", expected, ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
		if receivedSummary == nil || len(receivedSummary.Terraform) != 2 {
			t.Errorf("expected confirm to receive summary with 2 terraform plans, got %#v", receivedSummary)
		}
	})

	t.Run("ConfirmDeclineSkipsApplyAndMigrate", func(t *testing.T) {
		// Given a confirm callback that returns false, Bootstrap aborts cleanly
		// after the plan: no Up, no MigrateState, applied=false. The backend
		// override is still restored via defer.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
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
		mockStack.PlanSummaryFunc = func(_ *blueprintv1alpha1.Blueprint) []terraforminfra.TerraformComponentPlan {
			ops = append(ops, "plan")
			return nil
		}
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			return nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		applied, err := provisioner.Bootstrap(bp, func(_ *PlanSummary) bool { return false })
		if err != nil {
			t.Fatalf("Expected no error on declined confirm, got %v", err)
		}
		if applied {
			t.Error("Expected applied=false when confirm returned false")
		}

		expected := []string{"set:local", "plan", "set:s3"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected ops %v, got %v", expected, ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
	})

	t.Run("AlreadyBootstrappedSkipsOverrideAndMigrate", func(t *testing.T) {
		// Given the configured remote backend is reachable (BackendReachable
		// returns true), Bootstrap takes the configured-backend path: no
		// backend override, no MigrateState, just plan + apply against the
		// real backend. This is the idempotent re-run case — state already
		// lives in the configured backend, so we don't pretend to recreate
		// from local empty state.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
		}

		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		var ops []string
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				ops = append(ops, fmt.Sprintf("set:%v", value))
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.BackendReachableFunc = func(_ *blueprintv1alpha1.Blueprint) bool {
			return true
		}
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			return nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		applied, err := provisioner.Bootstrap(bp, nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !applied {
			t.Fatal("Expected applied=true")
		}

		expected := []string{"up"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected ops %v, got %v", expected, ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
	})

	t.Run("FreshUsesLocalOverrideWhenBackendNotReachable", func(t *testing.T) {
		// Given BackendReachable returns false (fresh install — no remote
		// backend resource yet), Bootstrap takes the local-override path:
		// pin terraform.backend.type=local, run Up against local state, then
		// migrate state to the configured remote backend at the end.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
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
		mockStack.BackendReachableFunc = func(_ *blueprintv1alpha1.Blueprint) bool {
			return false
		}
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			return nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		applied, err := provisioner.Bootstrap(bp, nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !applied {
			t.Fatal("Expected applied=true")
		}

		expected := []string{"set:local", "up", "set:s3", "migrate"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected ops %v, got %v", expected, ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
	})

	t.Run("ConfirmReceivesKustomizePlansAlongsideTerraform", func(t *testing.T) {
		// The bootstrap plan summary must include Kustomize plans, not just
		// Terraform — operators bootstrapping a re-deploy need to see what
		// flux will reconcile after the terraform layer lands. On first-time
		// bootstrap the kustomize layer plans against an empty cluster and
		// shows "(new)" / errors per row, which is still more useful than
		// hiding the layer entirely.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "system-dns"},
				{Name: "system-gitops"},
			},
		}

		mockTerraformStack := terraforminfra.NewMockStack()
		mockTerraformStack.PlanSummaryFunc = func(_ *blueprintv1alpha1.Blueprint) []terraforminfra.TerraformComponentPlan {
			return []terraforminfra.TerraformComponentPlan{{ComponentID: "vpc", Add: 1}}
		}
		mockTerraformStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			return nil
		}
		mockFluxStack := fluxinfra.NewMockStack()
		mockFluxStack.PlanSummaryFunc = func(_ *blueprintv1alpha1.Blueprint) ([]fluxinfra.KustomizePlan, []string) {
			return []fluxinfra.KustomizePlan{
				{Name: "system-dns", IsNew: true},
				{Name: "system-gitops", IsNew: true},
			}, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack: mockTerraformStack,
			FluxStack:      mockFluxStack,
		})

		var receivedSummary *PlanSummary
		confirm := func(s *PlanSummary) bool {
			receivedSummary = s
			return true
		}

		if _, err := provisioner.Bootstrap(bp, confirm); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if receivedSummary == nil {
			t.Fatal("Expected confirm to receive a summary")
		}
		if len(receivedSummary.Terraform) != 1 {
			t.Errorf("Expected 1 terraform plan, got %d", len(receivedSummary.Terraform))
		}
		if len(receivedSummary.Kustomize) != 2 {
			t.Errorf("Expected 2 kustomize plans, got %d", len(receivedSummary.Kustomize))
		}
	})
}
