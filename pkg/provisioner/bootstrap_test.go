package provisioner

import (
	"fmt"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
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

		// confirm must run BEFORE any mutation — the override pin and apply
		// follow it, MigrateState last. PlanSummary must NOT be called.
		expected := []string{"confirm", "set:local", "up", "set:s3", "migrate"}
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
}
