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
// Bootstrap Tests
// =============================================================================

func TestPivot(t *testing.T) {
	t.Run("ReturnsNilForLocalBackendType", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "vpc"},
			},
		}
		if got := pivot(bp, "local"); got != nil {
			t.Errorf("Expected nil for local backendType, got %#v", got)
		}
	})

	t.Run("ReturnsNilForEmptyBackendType", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
			},
		}
		if got := pivot(bp, ""); got != nil {
			t.Errorf("Expected nil for empty backendType, got %#v", got)
		}
	})

	t.Run("ReturnsExplicitBackendComponent", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "vpc"},
			},
		}
		got := pivot(bp, "azurerm")
		if got == nil || got.GetID() != "backend" {
			t.Errorf("Expected pivot=backend, got %#v", got)
		}
	})

	t.Run("ExplicitBackendWinsOverKubernetesConvention", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/talos"},
				{Path: "backend"},
			},
		}
		got := pivot(bp, "kubernetes")
		if got == nil || got.GetID() != "backend" {
			t.Errorf("Expected pivot=backend (explicit wins), got %#v", got)
		}
	})

	t.Run("KubernetesUsesFirstEnabledComponentWhenNoExplicitBackend", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/talos"},
				{Path: "cni/cilium"},
				{Path: "gitops/flux"},
			},
		}
		got := pivot(bp, "kubernetes")
		if got == nil || got.GetID() != "cluster/talos" {
			t.Errorf("Expected pivot=cluster/talos, got %#v", got)
		}
	})

	t.Run("KubernetesSkipsDisabledComponentsForFirst", func(t *testing.T) {
		falseVal := false
		falseExpr := &blueprintv1alpha1.BoolExpression{Value: &falseVal, IsExpr: false}
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/disabled-cluster", Enabled: falseExpr},
				{Path: "cluster/talos"},
				{Path: "gitops/flux"},
			},
		}
		got := pivot(bp, "kubernetes")
		if got == nil || got.GetID() != "cluster/talos" {
			t.Errorf("Expected pivot=cluster/talos (first enabled), got %#v", got)
		}
	})

	t.Run("ReturnsNilForRemoteBackendWithoutModule", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "cluster"},
			},
		}
		if got := pivot(bp, "s3"); got != nil {
			t.Errorf("Expected nil for s3 without backend module, got %#v", got)
		}
		if got := pivot(bp, "azurerm"); got != nil {
			t.Errorf("Expected nil for azurerm without backend module, got %#v", got)
		}
	})

	t.Run("ReturnsNilForKubernetesWithNoComponents", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{}
		if got := pivot(bp, "kubernetes"); got != nil {
			t.Errorf("Expected nil for empty blueprint, got %#v", got)
		}
	})

	t.Run("ReturnsNilForKubernetesWhenAllComponentsDisabled", func(t *testing.T) {
		falseVal := false
		falseExpr := &blueprintv1alpha1.BoolExpression{Value: &falseVal, IsExpr: false}
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/talos", Enabled: falseExpr},
				{Path: "gitops/flux", Enabled: falseExpr},
			},
		}
		if got := pivot(bp, "kubernetes"); got != nil {
			t.Errorf("Expected nil when all components disabled, got %#v", got)
		}
	})

	t.Run("ReturnsNilForNilBlueprint", func(t *testing.T) {
		if got := pivot(nil, "azurerm"); got != nil {
			t.Errorf("Expected nil for nil blueprint, got %#v", got)
		}
	})

	t.Run("RecognisesNestedBackendForExplicitMatch", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "terraform/backend"},
			},
		}
		got := pivot(bp, "azurerm")
		if got == nil || got.GetID() != "terraform/backend" {
			t.Errorf("Expected pivot=terraform/backend, got %#v", got)
		}
	})
}

func TestProvisioner_Bootstrap(t *testing.T) {
	t.Run("PivotIsAppliedLocalThenMigratedThenRestRunsAgainstRemote", func(t *testing.T) {
		// Given a blueprint with an explicit backend pivot and additional
		// components, bootstrap must: pin local → Up(pivotOnly) → restore →
		// MigrateState(pivotOnly) → Up(rest). The pivot does NOT appear in
		// the second Up's blueprint; the rest does NOT appear in the first.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
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
				return "azurerm"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		var ops []string
		var upBlueprints []*blueprintv1alpha1.Blueprint
		var migrateBlueprints []*blueprintv1alpha1.Blueprint
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				ops = append(ops, fmt.Sprintf("set:%v", value))
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(b *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			upBlueprints = append(upBlueprints, b)
			return nil
		}
		mockStack.MigrateStateFunc = func(b *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			migrateBlueprints = append(migrateBlueprints, b)
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

		expected := []string{"set:local", "up", "set:azurerm", "migrate", "up"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected %d ops %v, got %d %v", len(expected), expected, len(ops), ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}

		if len(upBlueprints) != 2 {
			t.Fatalf("Expected 2 Up calls, got %d", len(upBlueprints))
		}
		first := upBlueprints[0].TerraformComponents
		if len(first) != 1 || first[0].Path != "backend" {
			t.Errorf("Phase-1 Up should run pivot only [backend], got %#v", first)
		}
		second := upBlueprints[1].TerraformComponents
		if len(second) != 2 || second[0].Path != "cluster" || second[1].Path != "gitops" {
			t.Errorf("Phase-3 Up should run [cluster, gitops], got %#v", second)
		}
		if len(migrateBlueprints) != 1 || len(migrateBlueprints[0].TerraformComponents) != 1 || migrateBlueprints[0].TerraformComponents[0].Path != "backend" {
			t.Errorf("MigrateState should run on pivot-only blueprint, got %#v", migrateBlueprints)
		}
	})

	t.Run("KubernetesPivotUsesFirstComponentWhenNoExplicitBackend", func(t *testing.T) {
		// Given backendType=kubernetes and no IsBackend() module, the cluster
		// module is the pivot. Bootstrap applies it locally, migrates state
		// into the new cluster's k8s backend, then runs the rest normally.
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
		var upBlueprints []*blueprintv1alpha1.Blueprint
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				ops = append(ops, fmt.Sprintf("set:%v", value))
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(b *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			upBlueprints = append(upBlueprints, b)
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

		expected := []string{"set:local", "up", "set:kubernetes", "migrate", "up"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected ops %v, got %v", expected, ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
		if len(upBlueprints) != 2 {
			t.Fatalf("Expected 2 Up calls, got %d", len(upBlueprints))
		}
		first := upBlueprints[0].TerraformComponents
		if len(first) != 1 || first[0].Path != "cluster/talos" {
			t.Errorf("Phase-1 Up should run pivot only [cluster/talos], got %#v", first)
		}
		second := upBlueprints[1].TerraformComponents
		if len(second) != 2 || second[0].Path != "cni/cilium" || second[1].Path != "gitops/flux" {
			t.Errorf("Phase-3 Up should run [cni/cilium, gitops/flux], got %#v", second)
		}
	})

	t.Run("PivotNotFirstFailsBeforeAnyApply", func(t *testing.T) {
		// Given a blueprint where the pivot is not the first enabled
		// terraform component, bootstrap aborts before touching backend.type
		// or running any apply. The operator must reorder their blueprint.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "backend"},
			},
		}

		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "azurerm"
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

		_, err := provisioner.Bootstrap(bp, nil)
		if err == nil {
			t.Fatal("Expected ordering error when pivot is not first")
		}
		if !strings.Contains(err.Error(), "first enabled terraform component") {
			t.Errorf("Expected error to mention ordering, got: %v", err)
		}
		if len(ops) != 0 {
			t.Errorf("Expected no Up/Migrate/Set ops on validation failure, got %v", ops)
		}
	})

	t.Run("RemoteBackendWithoutPivotFallsThroughToPlainUp", func(t *testing.T) {
		// Given backendType=s3 and no IsBackend() module, the storage is
		// presumed to be operator-provisioned out of band. Bootstrap calls
		// Up directly with the configured remote backend; no override, no
		// migrate.
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
			t.Fatal("Expected applied=true")
		}
		if len(ops) != 1 || ops[0] != "up" {
			t.Errorf("Expected single Up call (no override, no migrate), got %v", ops)
		}
	})

	t.Run("PivotOnlyBlueprintSkipsPhase3Up", func(t *testing.T) {
		// Given a blueprint whose only terraform component is the pivot,
		// the post-migrate Up is a no-op and must not be invoked.
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
				return "azurerm"
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
			t.Fatal("Expected applied=true")
		}

		expected := []string{"set:local", "up", "set:azurerm", "migrate"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected ops %v, got %v", expected, ops)
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

	t.Run("ConfirmReceivesSummaryBeforeAnyMutation", func(t *testing.T) {
		// The bootstrap summary is built from blueprint + config and passed to
		// confirm BEFORE the override pin or any apply happens. No terraform
		// plan, no PlanSummary call — the summary is intent only. This guards
		// the contract: the operator sees what bootstrap will attempt before
		// any state-touching work begins.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "vpc"},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "system-dns"},
				{Name: "system-gitops"},
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

		var receivedSummary *BootstrapSummary
		confirm := func(s *BootstrapSummary) bool {
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

		// confirm must run BEFORE any mutation — the override pin and pivot
		// apply follow it, then MigrateState, then a second Up for the rest
		// against the live remote backend. PlanSummary must NOT be called.
		expected := []string{"confirm", "set:local", "up", "set:s3", "migrate", "up"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected ops %v, got %v", expected, ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
		if receivedSummary == nil {
			t.Fatal("expected confirm to receive a summary")
		}
		if receivedSummary.BackendType != "s3" {
			t.Errorf("expected BackendType=s3, got %q", receivedSummary.BackendType)
		}
		if len(receivedSummary.Terraform) != 2 {
			t.Errorf("expected 2 terraform entries, got %d", len(receivedSummary.Terraform))
		}
		if len(receivedSummary.Kustomize) != 2 {
			t.Errorf("expected 2 kustomize entries, got %d", len(receivedSummary.Kustomize))
		}
	})

	t.Run("ConfirmDeclineSkipsAllWork", func(t *testing.T) {
		// Given a confirm callback that returns false, Bootstrap aborts before
		// touching backend.type, before Up, before MigrateState. applied=false,
		// no error.
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
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			return nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		applied, err := provisioner.Bootstrap(bp, func(_ *BootstrapSummary) bool { return false })
		if err != nil {
			t.Fatalf("Expected no error on declined confirm, got %v", err)
		}
		if applied {
			t.Error("Expected applied=false when confirm returned false")
		}

		// Decline must short-circuit before any mutation.
		if len(ops) != 0 {
			t.Errorf("Expected no ops on declined confirm, got %v", ops)
		}
	})

	t.Run("BootstrapAlwaysUsesLocalOverrideAndMigrates", func(t *testing.T) {
		// When a backend component is declared, Bootstrap always pins
		// terraform.backend.type=local for one Up pass against local state,
		// then migrates state to the configured remote backend. This is the
		// only path — there is no probe-based optimization. On a re-bootstrap
		// the override path's apply will fail loudly when the cloud rejects
		// "create" against existing infra; that's the operator's signal to
		// run `windsor apply` instead.
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

	t.Run("BootstrapSummaryIncludesAllBlueprintLayers", func(t *testing.T) {
		// The bootstrap summary lists Terraform and Kustomize entries directly
		// from the blueprint — operators see exactly what is declared, not
		// what some plan engine computes. No terraform invocation, no flux
		// diff. Layer ordering follows blueprint declaration order.
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
		mockTerraformStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack: mockTerraformStack,
		})

		var receivedSummary *BootstrapSummary
		confirm := func(s *BootstrapSummary) bool {
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
			t.Errorf("Expected 1 terraform entry, got %d", len(receivedSummary.Terraform))
		}
		if receivedSummary.Terraform[0].Path != "vpc" {
			t.Errorf("Expected terraform path 'vpc', got %q", receivedSummary.Terraform[0].Path)
		}
		if len(receivedSummary.Kustomize) != 2 {
			t.Errorf("Expected 2 kustomize entries, got %d", len(receivedSummary.Kustomize))
		}
	})

	t.Run("ProbeRemoteStateHitSkipsDanceAndRunsPlainUpOnFullBlueprint", func(t *testing.T) {
		// Given a previously-bootstrapped context (HasRemoteState reports true
		// for the pivot), Bootstrap must skip the local-then-migrate dance
		// entirely and run a single Up against the full blueprint with the
		// configured remote backend in effect. No backend.type override, no
		// MigrateState, no RemoveLocalState.
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
				return "azurerm"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		var ops []string
		var upBlueprints []*blueprintv1alpha1.Blueprint
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				ops = append(ops, fmt.Sprintf("set:%v", value))
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.HasRemoteStateFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
			ops = append(ops, fmt.Sprintf("probe:%s", componentID))
			return true, nil
		}
		mockStack.UpFunc = func(b *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			upBlueprints = append(upBlueprints, b)
			return nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			return nil, nil
		}
		mockStack.RemoveLocalStateFunc = func(_ string) error {
			ops = append(ops, "remove-local")
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		applied, err := provisioner.Bootstrap(bp, nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !applied {
			t.Fatal("Expected applied=true")
		}

		expected := []string{"probe:backend", "up"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected ops %v, got %v", expected, ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
		if len(upBlueprints) != 1 || len(upBlueprints[0].TerraformComponents) != 2 {
			t.Errorf("Expected single Up call against full blueprint, got %#v", upBlueprints)
		}
	})

	t.Run("ProbeErrorFallsThroughToDance", func(t *testing.T) {
		// Given the probe errors (auth issue, network blip, missing remote
		// backend storage), Bootstrap must NOT silently skip the dance —
		// errors degrade safely by running the dance, where Phase 1 will
		// surface a persistent issue with a cloud-side error.
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
				return "azurerm"
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
		mockStack.HasRemoteStateFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) (bool, error) {
			ops = append(ops, "probe-err")
			return false, fmt.Errorf("backend bucket missing")
		}
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			return nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			return nil, nil
		}
		mockStack.RemoveLocalStateFunc = func(_ string) error {
			ops = append(ops, "remove-local")
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		applied, err := provisioner.Bootstrap(bp, nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !applied {
			t.Fatal("Expected applied=true")
		}

		expected := []string{"probe-err", "set:local", "up", "set:azurerm", "migrate", "remove-local"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected ops %v, got %v", expected, ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
	})

	t.Run("PostMigrateRemovesLocalStateForPivot", func(t *testing.T) {
		// After Phase-2 migrate succeeds, Bootstrap must call
		// RemoveLocalState(pivotID) so a future rerun's behavior is
		// deterministic — the probe is the source of truth, not leftover
		// local-state residue. The cleanup must run BEFORE Phase-3 Up so a
		// failure in Phase 3 doesn't leave the local file lingering.
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
				return "azurerm"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		var ops []string
		var removedID string
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
		mockStack.RemoveLocalStateFunc = func(componentID string) error {
			ops = append(ops, "remove-local")
			removedID = componentID
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if _, err := provisioner.Bootstrap(bp, nil); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := []string{"set:local", "up", "set:azurerm", "migrate", "remove-local", "up"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected ops %v, got %v", expected, ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
		if removedID != "backend" {
			t.Errorf("Expected RemoveLocalState called with pivot id 'backend', got %q", removedID)
		}
	})

	t.Run("UpRecoversLeftoverLocalStateBeforeApply", func(t *testing.T) {
		// Given a probe hit (pivot is in remote) and a non-pivot component
		// with leftover local state from a previous interrupted bootstrap
		// but no remote state, Bootstrap calls Up which now runs the
		// recovery sweep internally before delegating to TerraformStack.Up.
		// The sweep does the two-step reset-and-migrate: under
		// withBackendOverride("local") run InitComponent (writes local
		// pointer file), exit override, then MigrateComponentState (copies
		// local → configured remote). This protects the user-reported
		// scenario where the pointer was left recording azurerm even though
		// the actual state lived in the local file — without the two-step
		// reset, init -migrate-state would migrate empty remote → local and
		// destroy the local state.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "dns-zone"},
				{Path: "cluster"},
			},
		}

		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "azurerm"
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
		mockStack.HasRemoteStateFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
			ops = append(ops, fmt.Sprintf("probe-remote:%s", componentID))
			switch componentID {
			case "backend", "cluster":
				return true, nil
			case "dns-zone":
				return false, nil
			}
			return false, nil
		}
		mockStack.HasLocalStateWithResourcesFunc = func(componentID string) (bool, error) {
			ops = append(ops, fmt.Sprintf("probe-local:%s", componentID))
			return componentID == "dns-zone", nil
		}
		mockStack.InitComponentFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) error {
			ops = append(ops, fmt.Sprintf("init-component:%s", componentID))
			return nil
		}
		mockStack.MigrateComponentStateFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) error {
			ops = append(ops, fmt.Sprintf("migrate-component:%s", componentID))
			return nil
		}
		mockStack.RemoveLocalStateFunc = func(componentID string) error {
			ops = append(ops, fmt.Sprintf("remove-local:%s", componentID))
			return nil
		}
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		applied, err := provisioner.Bootstrap(bp, nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !applied {
			t.Fatal("Expected applied=true")
		}

		// Bootstrap probes the pivot (returns hit, skip dance), then calls
		// Up which sweeps every enabled component:
		//   - backend: probe-local=false → skip
		//   - dns-zone: probe-local=true, probe-remote=false → reset-and-
		//     migrate (set:local, init-component, set:azurerm,
		//     migrate-component, remove-local)
		//   - cluster: probe-local=false → skip
		// Then TerraformStack.Up runs.
		expected := []string{
			"probe-remote:backend",
			"probe-local:backend",
			"probe-local:dns-zone",
			"probe-remote:dns-zone",
			"set:local",
			"init-component:dns-zone",
			"set:azurerm",
			"migrate-component:dns-zone",
			"remove-local:dns-zone",
			"probe-local:cluster",
			"up",
		}
		if len(ops) != len(expected) {
			t.Fatalf("Expected ops %v, got %v", expected, ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
	})

	t.Run("UpRecoverySweepIsCheapWhenNoLocalStateExists", func(t *testing.T) {
		// Given a probe hit and no non-pivot components with leftover local
		// state, the recovery sweep inside Up must run cheaply (one local
		// probe per component) and not invoke any migrate, init, or
		// override. The clean rerun case must remain near-zero cost.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "dns-zone"},
				{Path: "cluster"},
			},
		}

		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "azurerm"
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
		mockStack.HasRemoteStateFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
			ops = append(ops, fmt.Sprintf("probe-remote:%s", componentID))
			return true, nil
		}
		mockStack.HasLocalStateWithResourcesFunc = func(componentID string) (bool, error) {
			ops = append(ops, fmt.Sprintf("probe-local:%s", componentID))
			return false, nil
		}
		initCalled := false
		mockStack.InitComponentFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error {
			initCalled = true
			return nil
		}
		migrateCalled := false
		mockStack.MigrateComponentStateFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error {
			migrateCalled = true
			return nil
		}
		removeCalled := false
		mockStack.RemoveLocalStateFunc = func(_ string) error {
			removeCalled = true
			return nil
		}
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if _, err := provisioner.Bootstrap(bp, nil); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := []string{
			"probe-remote:backend",
			"probe-local:backend",
			"probe-local:dns-zone",
			"probe-local:cluster",
			"up",
		}
		if len(ops) != len(expected) {
			t.Fatalf("Expected ops %v, got %v", expected, ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
		if initCalled {
			t.Error("InitComponent must not be called when no leftover local state exists")
		}
		if migrateCalled {
			t.Error("MigrateComponentState must not be called when no leftover local state exists")
		}
		if removeCalled {
			t.Error("RemoveLocalState must not be called when no leftover local state exists")
		}
	})

	t.Run("DirectUpAlsoRecoversLeftoverLocalState", func(t *testing.T) {
		// Provisioner.Up runs the recovery sweep regardless of whether the
		// caller is Bootstrap. This is the path that fixes `windsor up` /
		// `windsor apply` against a context with leftover local state — the
		// scenario the user hit while reading "already exists" errors after
		// manually importing one component. Without recovery in Up, each
		// remaining component requires a separate manual import.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "network"},
				{Path: "cluster"},
			},
		}

		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "azurerm"
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
		mockStack.HasLocalStateWithResourcesFunc = func(componentID string) (bool, error) {
			ops = append(ops, fmt.Sprintf("probe-local:%s", componentID))
			return true, nil
		}
		mockStack.HasRemoteStateFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
			ops = append(ops, fmt.Sprintf("probe-remote:%s", componentID))
			return false, nil
		}
		mockStack.InitComponentFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) error {
			ops = append(ops, fmt.Sprintf("init-component:%s", componentID))
			return nil
		}
		mockStack.MigrateComponentStateFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) error {
			ops = append(ops, fmt.Sprintf("migrate-component:%s", componentID))
			return nil
		}
		mockStack.RemoveLocalStateFunc = func(componentID string) error {
			ops = append(ops, fmt.Sprintf("remove-local:%s", componentID))
			return nil
		}
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if err := provisioner.Up(bp); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Each component goes through the full reset-and-migrate sequence
		// before TerraformStack.Up is invoked.
		expected := []string{
			"probe-local:network",
			"probe-remote:network",
			"set:local",
			"init-component:network",
			"set:azurerm",
			"migrate-component:network",
			"remove-local:network",
			"probe-local:cluster",
			"probe-remote:cluster",
			"set:local",
			"init-component:cluster",
			"set:azurerm",
			"migrate-component:cluster",
			"remove-local:cluster",
			"up",
		}
		if len(ops) != len(expected) {
			t.Fatalf("Expected ops %v, got %v", expected, ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
	})

	t.Run("LocalStateRemovalFailureWarnsButDoesNotAbortBootstrap", func(t *testing.T) {
		// If RemoveLocalState fails after a successful migrate, the bootstrap
		// itself has already done the operationally meaningful work — pivot
		// is in cloud, state is in remote, rest of stack still needs to run.
		// Surface the failure as a stderr warning and continue to Phase 3
		// rather than aborting; a stale local state file is recoverable but
		// an unbootstrapped rest-of-stack is not.
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
				return "azurerm"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		var ops []string
		mockCH.SetFunc = func(_ string, _ any) error { return nil }
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			return nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			return nil, nil
		}
		mockStack.RemoveLocalStateFunc = func(_ string) error {
			return fmt.Errorf("permission denied")
		}

		r, w, pipeErr := os.Pipe()
		if pipeErr != nil {
			t.Fatalf("Pipe failed: %v", pipeErr)
		}
		origStderr := os.Stderr
		os.Stderr = w
		defer func() { os.Stderr = origStderr }()

		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})
		applied, err := provisioner.Bootstrap(bp, nil)

		w.Close()
		stderrBytes, _ := io.ReadAll(r)
		stderrOutput := string(stderrBytes)

		if err != nil {
			t.Fatalf("Expected bootstrap to succeed despite cleanup failure, got %v", err)
		}
		if !applied {
			t.Fatal("Expected applied=true")
		}
		if !strings.Contains(stderrOutput, "failed to remove local state file") {
			t.Errorf("Expected stderr warning about cleanup failure, got: %q", stderrOutput)
		}
		if !strings.Contains(stderrOutput, "permission denied") {
			t.Errorf("Expected stderr warning to include underlying cause, got: %q", stderrOutput)
		}
		// Phase 3 must still run.
		if len(ops) != 3 || ops[0] != "up" || ops[1] != "migrate" || ops[2] != "up" {
			t.Errorf("Expected [up migrate up] sequence after cleanup failure, got %v", ops)
		}
	})
}
