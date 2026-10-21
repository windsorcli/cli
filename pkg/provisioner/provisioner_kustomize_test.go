package provisioner

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/runtime/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProvisioner_ApplyKustomize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a provisioner with a kubernetes manager that applies successfully
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.ApplyBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			return nil
		}
		opts := &Provisioner{KubernetesManager: mocks.KubernetesManager}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When ApplyKustomize is called with a valid kustomization name
		err := provisioner.ApplyKustomize(context.Background(), createTestBlueprint(), "test-kustomization")

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("WaitsForOnlyTheAppliedKustomization", func(t *testing.T) {
		// Given a kubernetes manager whose readiness wait fails
		mocks := setupProvisionerMocks(t)
		var waitedFor []string
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, layers kubernetes.KustomizationReporter) error {
			for _, k := range blueprint.Kustomizations {
				waitedFor = append(waitedFor, k.Name)
			}
			return fmt.Errorf("wait failed")
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When ApplyKustomize is called
		err := provisioner.ApplyKustomize(context.Background(), createTestBlueprint(), "test-kustomization")

		// Then the wait covers only that kustomization and its failure wraps ErrKustomizationsNotReady
		if strings.Join(waitedFor, ",") != "test-kustomization" {
			t.Errorf("Expected a wait on test-kustomization only, got %v", waitedFor)
		}
		if !errors.Is(err, ErrKustomizationsNotReady) {
			t.Errorf("Expected a not-ready error, got %v", err)
		}
	})

	t.Run("AppliesCrdLayerByName", func(t *testing.T) {
		// Given a blueprint whose only kustomize layer is the crds: list
		mocks := setupProvisionerMocks(t)
		var applied *blueprintv1alpha1.Blueprint
		mocks.KubernetesManager.ApplyBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			applied = bp
			return nil
		}
		opts := &Provisioner{KubernetesManager: mocks.KubernetesManager}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When applying the crds layer by name
		if err := provisioner.ApplyKustomize(context.Background(), &blueprintv1alpha1.Blueprint{Crds: []string{"cert-manager-1.16.2"}}, "crds"); err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then the synthesized crds kustomization is the one applied
		if applied == nil || len(applied.Kustomizations) != 1 || applied.Kustomizations[0].Name != "crds" {
			t.Errorf("expected crds kustomization applied, got %+v", applied)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		// Given a provisioner with no blueprint
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		// When ApplyKustomize is called with a nil blueprint
		err := provisioner.ApplyKustomize(context.Background(), nil, "test-kustomization")

		// Then an error is returned
		if err == nil {
			t.Error("Expected error for nil blueprint")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorNilKubernetesManager", func(t *testing.T) {
		// Given a provisioner with no kubernetes manager
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		provisioner.KubernetesManager = nil

		// When ApplyKustomize is called
		err := provisioner.ApplyKustomize(context.Background(), createTestBlueprint(), "test-kustomization")

		// Then an error is returned
		if err == nil {
			t.Error("Expected error for nil kubernetes manager")
		}
		if !strings.Contains(err.Error(), "kubernetes manager not configured") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorKustomizationNotFound", func(t *testing.T) {
		// Given a provisioner and a blueprint that does not contain the requested kustomization
		mocks := setupProvisionerMocks(t)
		opts := &Provisioner{KubernetesManager: mocks.KubernetesManager}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When ApplyKustomize is called with a name not in the blueprint
		err := provisioner.ApplyKustomize(context.Background(), createTestBlueprint(), "nonexistent")

		// Then an error is returned
		if err == nil {
			t.Error("Expected error for missing kustomization")
		}
		if !strings.Contains(err.Error(), "not found in blueprint") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorDestroyOnly", func(t *testing.T) {
		// Given a blueprint whose kustomization is marked destroyOnly
		mocks := setupProvisionerMocks(t)
		opts := &Provisioner{KubernetesManager: mocks.KubernetesManager}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		destroyOnly := true
		bp := createTestBlueprint()
		bp.Kustomizations[0].DestroyOnly = &destroyOnly

		// When ApplyKustomize is called with that kustomization name
		err := provisioner.ApplyKustomize(context.Background(), bp, "test-kustomization")

		// Then an error is returned indicating it cannot be applied
		if err == nil {
			t.Error("Expected error for destroyOnly kustomization")
		}
		if !strings.Contains(err.Error(), "destroy-only") {
			t.Errorf("Expected destroy-only error message, got: %v", err)
		}
	})

	t.Run("ErrorApplyBlueprintFails", func(t *testing.T) {
		// Given a provisioner whose kubernetes manager returns an error
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.ApplyBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("apply blueprint failed")
		}
		opts := &Provisioner{KubernetesManager: mocks.KubernetesManager}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When ApplyKustomize is called
		err := provisioner.ApplyKustomize(context.Background(), createTestBlueprint(), "test-kustomization")

		// Then an error is returned
		if err == nil {
			t.Error("Expected error for apply blueprint failure")
		}
		if !strings.Contains(err.Error(), "failed to apply kustomization") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("FiltersToSingleKustomization", func(t *testing.T) {
		// Given a provisioner and a blueprint with multiple kustomizations
		mocks := setupProvisionerMocks(t)
		var appliedBlueprint *blueprintv1alpha1.Blueprint
		mocks.KubernetesManager.ApplyBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			appliedBlueprint = bp
			return nil
		}
		opts := &Provisioner{KubernetesManager: mocks.KubernetesManager}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When ApplyKustomize is called with a specific kustomization name
		err := provisioner.ApplyKustomize(context.Background(), createTestBlueprint(), "test-kustomization")

		// Then only the named kustomization is passed to ApplyBlueprint
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if len(appliedBlueprint.Kustomizations) != 1 {
			t.Errorf("Expected 1 kustomization in filtered blueprint, got %d", len(appliedBlueprint.Kustomizations))
		}
		if appliedBlueprint.Kustomizations[0].Name != "test-kustomization" {
			t.Errorf("Expected kustomization name 'test-kustomization', got %q", appliedBlueprint.Kustomizations[0].Name)
		}
	})

	t.Run("PlacesNamedKustomizationSecret", func(t *testing.T) {
		// Given a blueprint whose named kustomization declares a secret referencing a configured value
		mocks := setupProvisionerMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"hetzner": map[string]any{"token": "resolved-token"}}, nil
		}
		mocks.KubernetesManager.ApplyBlueprintFunc = func(*blueprintv1alpha1.Blueprint, string) error { return nil }
		mocks.KubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			return []kubernetes.InventoryEntry{{Kind: "Namespace", Name: "system-lb"}}, nil
		}
		var placedName, placedNs string
		mocks.KubernetesManager.ApplySecretFunc = func(name, namespace string, stringData map[string]string, owner string) error {
			placedName, placedNs = name, namespace
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			{Name: "lb-install", Secrets: map[string]blueprintv1alpha1.SecretEntry{
				"hcloud": {Data: map[string]string{"token": "${hetzner.token}"}},
			}},
		}}

		// When applying just that kustomization, its secret is placed into the namespace it created
		if err := provisioner.ApplyKustomize(context.Background(), blueprint, "lb-install"); err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if placedName != "hcloud" || placedNs != "system-lb" {
			t.Errorf("Expected hcloud placed into system-lb, got name=%q ns=%q", placedName, placedNs)
		}
	})
}

func TestProvisioner_ApplyKustomizeAll(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a provisioner with a kubernetes manager that applies successfully
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.ApplyBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			return nil
		}
		opts := &Provisioner{KubernetesManager: mocks.KubernetesManager}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When ApplyKustomizeAll is called
		err := provisioner.ApplyKustomizeAll(context.Background(), createTestBlueprint())

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		// Given a provisioner with no blueprint
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		// When ApplyKustomizeAll is called with a nil blueprint
		err := provisioner.ApplyKustomizeAll(context.Background(), nil)

		// Then an error is returned
		if err == nil {
			t.Error("Expected error for nil blueprint")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorNilKubernetesManager", func(t *testing.T) {
		// Given a provisioner with no kubernetes manager
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		provisioner.KubernetesManager = nil

		// When ApplyKustomizeAll is called
		err := provisioner.ApplyKustomizeAll(context.Background(), createTestBlueprint())

		// Then an error is returned
		if err == nil {
			t.Error("Expected error for nil kubernetes manager")
		}
		if !strings.Contains(err.Error(), "kubernetes manager not configured") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorApplyBlueprintFails", func(t *testing.T) {
		// Given a provisioner whose kubernetes manager returns an error
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.ApplyBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("apply blueprint failed")
		}
		opts := &Provisioner{KubernetesManager: mocks.KubernetesManager}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When ApplyKustomizeAll is called
		err := provisioner.ApplyKustomizeAll(context.Background(), createTestBlueprint())

		// Then an error is returned
		if err == nil {
			t.Error("Expected error for apply blueprint failure")
		}
		if !strings.Contains(err.Error(), "failed to apply blueprint") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

func TestProvisioner_Install(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.ApplyBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		blueprint := createTestBlueprint()
		err := provisioner.Install(context.Background(), blueprint, false)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("AppliesKustomizationsAsOneSectionWithAStepPerLayer", func(t *testing.T) {
		// Given a provisioner with a recording presenter
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.ApplyBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})
		rec := &eventRecorder{}

		// When the blueprint resources are installed
		err := provisioner.Install(presenter.NewContext(context.Background(), rec), createTestBlueprint(), false)

		// Then one "Applying kustomizations" section holds a step per kustomization, and both end as applied
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if ops := rec.operations(); len(ops) != 2 || ops[0] != "Applying kustomizations" || ops[1] != "test-kustomization" {
			t.Fatalf("unexpected operations %v", ops)
		}
		if rec.events[1].ParentID != rec.events[0].ID {
			t.Errorf("expected the layer step under the section, got %+v", rec.events[1])
		}
		var applied []string
		for _, e := range rec.events {
			if e.Kind == presenter.KindApplied {
				applied = append(applied, e.Message)
			}
		}
		if strings.Join(applied, ",") != "test-kustomization,Applying kustomizations" {
			t.Errorf("expected the layer then the section to end as applied, got %v", applied)
		}
	})

	t.Run("MaterializesCrdLayerFromLocalCrdsList", func(t *testing.T) {
		// Given a blueprint whose crds: is a flat list of local references
		mocks := setupProvisionerMocks(t)
		var applied *blueprintv1alpha1.Blueprint
		mocks.KubernetesManager.ApplyBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			applied = blueprint
			return nil
		}
		opts := &Provisioner{KubernetesManager: mocks.KubernetesManager}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)
		blueprint := &blueprintv1alpha1.Blueprint{
			Crds:           []string{"gateway-api-experimental-1.5.1", "envoy-gateway-1.7.1"},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "cert-manager", DependsOn: []string{"crds"}}},
		}

		// When installing
		if err := provisioner.Install(context.Background(), blueprint, false); err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then a "crds" kustomization is materialized ahead of the stack, bound to the default/project
		// source, with the references as its components, pruning disabled and wait enabled
		if applied == nil || len(applied.Kustomizations) != 2 {
			t.Fatalf("expected crds kustomization prepended, got %+v", applied)
		}
		crd := applied.Kustomizations[0]
		if crd.Name != "crds" || crd.Path != "crds" || crd.Source != "" {
			t.Errorf("expected name 'crds' path 'crds' source '', got name=%q path=%q source=%q", crd.Name, crd.Path, crd.Source)
		}
		if !slices.Equal(crd.Components, []string{"gateway-api-experimental-1.5.1", "envoy-gateway-1.7.1"}) {
			t.Errorf("expected components from the local crds list, got %v", crd.Components)
		}
		if crd.Prune == nil || *crd.Prune != false || crd.Wait == nil || *crd.Wait != true {
			t.Errorf("expected prune=false wait=true, got prune=%v wait=%v", crd.Prune, crd.Wait)
		}
		// And the caller's blueprint is not mutated
		if len(blueprint.Kustomizations) != 1 {
			t.Errorf("expected caller blueprint untouched, got %+v", blueprint.Kustomizations)
		}
	})

	t.Run("MaterializesBackgroundLayerPerInstallSource", func(t *testing.T) {
		// Given CRDs carried by two different install sources
		mocks := setupProvisionerMocks(t)
		var applied *blueprintv1alpha1.Blueprint
		mocks.KubernetesManager.ApplyBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			applied = blueprint
			return nil
		}
		opts := &Provisioner{KubernetesManager: mocks.KubernetesManager}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)
		installed := true
		install := &blueprintv1alpha1.BoolExpression{Value: &installed}
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Install: install, Crds: []string{"cert-manager-1.16.2"}},
				{Name: "acme-oci", Install: install, Crds: []string{"acme-crd-2.0.0"}},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "app", DependsOn: []string{"crds-core", "crds-acme-oci"}}},
		}

		// When installing
		if err := provisioner.Install(context.Background(), blueprint, false); err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then each install source gets its own background layer (alphabetical), bound to its source
		if applied == nil || len(applied.Kustomizations) != 3 {
			t.Fatalf("expected two crds layers prepended, got %+v", applied)
		}
		acme, core := applied.Kustomizations[0], applied.Kustomizations[1]
		if acme.Name != "crds-acme-oci" || acme.Source != "acme-oci" {
			t.Errorf("expected crds-acme-oci bound to acme-oci, got name=%q source=%q", acme.Name, acme.Source)
		}
		if core.Name != "crds-core" || core.Source != "core" {
			t.Errorf("expected crds-core bound to core, got name=%q source=%q", core.Name, core.Source)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		err := provisioner.Install(context.Background(), nil, false)

		if err == nil {
			t.Error("Expected error for nil blueprint")
		}

		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorNilKubernetesManager", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		provisioner.KubernetesManager = nil

		blueprint := createTestBlueprint()
		err := provisioner.Install(context.Background(), blueprint, false)

		if err == nil {
			t.Error("Expected error for nil kubernetes manager")
		}

		if !strings.Contains(err.Error(), "kubernetes manager not configured") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorApplyBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.ApplyBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("apply blueprint failed")
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		blueprint := createTestBlueprint()
		err := provisioner.Install(context.Background(), blueprint, false)

		if err == nil {
			t.Error("Expected error for apply blueprint failure")
		}

		if !strings.Contains(err.Error(), "failed to apply blueprint") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ResolvesAndPlacesDeclaredSecrets", func(t *testing.T) {
		// Given a blueprint whose kustomization declares a secret referencing a configured value
		mocks := setupProvisionerMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"hetzner": map[string]any{"token": "resolved-token"}}, nil
		}
		mocks.KubernetesManager.ApplyBlueprintFunc = func(*blueprintv1alpha1.Blueprint, string) error { return nil }
		mocks.KubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			return []kubernetes.InventoryEntry{{Kind: "Namespace", Name: "system-lb"}}, nil
		}
		var placedName, placedNs string
		var placedData map[string]string
		mocks.KubernetesManager.ApplySecretFunc = func(name, namespace string, stringData map[string]string, owner string) error {
			placedName, placedNs, placedData = name, namespace, stringData
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			{Name: "lb-install", Secrets: map[string]blueprintv1alpha1.SecretEntry{
				"hcloud": {Data: map[string]string{"token": "${hetzner.token}"}},
			}},
		}}

		// When installing, the declared secret is placed into the namespace its kustomization created
		if err := provisioner.Install(context.Background(), blueprint, false); err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if placedName != "hcloud" || placedNs != "system-lb" || placedData["token"] != "resolved-token" {
			t.Errorf("Expected hcloud/token placed into system-lb, got name=%q ns=%q data=%v", placedName, placedNs, placedData)
		}
	})

	t.Run("FailsBeforeApplyWhenSecretResolvesEmpty", func(t *testing.T) {
		// Given a required secret reference whose configured value is empty
		mocks := setupProvisionerMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"hetzner": map[string]any{"token": ""}}, nil
		}
		applied := false
		mocks.KubernetesManager.ApplyBlueprintFunc = func(*blueprintv1alpha1.Blueprint, string) error {
			applied = true
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			{Name: "lb-install", Secrets: map[string]blueprintv1alpha1.SecretEntry{
				"hcloud": {Data: map[string]string{"token": "${hetzner.token}"}},
			}},
		}}

		// When installing, resolution fails closed before anything is applied to the cluster
		err := provisioner.Install(context.Background(), blueprint, false)
		if err == nil || !strings.Contains(err.Error(), "error resolving secrets") {
			t.Errorf("Expected resolve error, got: %v", err)
		}
		if applied {
			t.Error("Expected no apply when secret resolution fails")
		}
	})

	t.Run("FailsWhenRequiredSecretResolvesToNil", func(t *testing.T) {
		// Given a required secret reference to a value absent from configuration (resolves to nil), as
		// when a secret() lookup fails to resolve non-interactively
		mocks := setupProvisionerMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		mocks.KubernetesManager.ApplyBlueprintFunc = func(*blueprintv1alpha1.Blueprint, string) error { return nil }
		placed := false
		mocks.KubernetesManager.ApplySecretFunc = func(name, namespace string, stringData map[string]string, owner string) error {
			placed = true
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			{Name: "lb-install", Secrets: map[string]blueprintv1alpha1.SecretEntry{
				"hcloud": {Data: map[string]string{"token": "${hetzner.token}"}},
			}},
		}}

		// When installing, it fails closed rather than placing an hcloud Secret missing its token key
		if err := provisioner.Install(context.Background(), blueprint, false); err == nil {
			t.Fatal("Expected error for a nil-resolving required secret, got nil")
		}
		if placed {
			t.Error("Expected no secret placed for a nil-resolving reference")
		}
	})

	t.Run("OmitsUnconfiguredOptionalSecretWithoutFailing", func(t *testing.T) {
		// Given an optional secret reference (?? default) to a value absent from configuration
		mocks := setupProvisionerMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		mocks.KubernetesManager.ApplyBlueprintFunc = func(*blueprintv1alpha1.Blueprint, string) error { return nil }
		placed := false
		mocks.KubernetesManager.ApplySecretFunc = func(name, namespace string, stringData map[string]string, owner string) error {
			placed = true
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			{Name: "lb-install", Secrets: map[string]blueprintv1alpha1.SecretEntry{
				"hcloud": {Data: map[string]string{"token": "${hetzner.token ?? ''}"}},
			}},
		}}

		// When installing, the optional secret is skipped and install still succeeds
		if err := provisioner.Install(context.Background(), blueprint, false); err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if placed {
			t.Error("Expected no secret placed for a nil-resolving optional reference")
		}
	})
}

func TestProvisioner_InstallReadinessWait(t *testing.T) {
	t.Run("WaitsInsideTheApplyStep", func(t *testing.T) {
		// Given a kubernetes manager whose readiness wait marks each kustomization done
		mocks := setupProvisionerMocks(t)
		var waitedFor []string
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, layers kubernetes.KustomizationReporter) error {
			for _, k := range blueprint.Kustomizations {
				waitedFor = append(waitedFor, k.Name)
				layers.Done(k.Name)
			}
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})
		rec := &eventRecorder{}

		// When installing
		err := provisioner.Install(presenter.NewContext(context.Background(), rec), createTestBlueprint(), false)

		// Then the wait covers the kustomization and adds no operation beyond the apply step and its one layer
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if strings.Join(waitedFor, ",") != "test-kustomization" {
			t.Errorf("Expected a wait on test-kustomization, got %v", waitedFor)
		}
		if ops := rec.operations(); len(ops) != 2 || ops[0] != "Applying kustomizations" || ops[1] != "test-kustomization" {
			t.Errorf("Unexpected operations %v", ops)
		}
	})

	t.Run("WrapsAWaitFailureAsNotReady", func(t *testing.T) {
		// Given a kubernetes manager whose readiness wait fails
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, layers kubernetes.KustomizationReporter) error {
			return fmt.Errorf("wait failed")
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When installing
		err := provisioner.Install(context.Background(), createTestBlueprint(), false)

		// Then the error wraps ErrKustomizationsNotReady and the cause
		if !errors.Is(err, ErrKustomizationsNotReady) || !strings.Contains(err.Error(), "wait failed") {
			t.Errorf("Expected a not-ready error wrapping the cause, got %v", err)
		}
	})
}

func TestProvisioner_Prune(t *testing.T) {
	t.Run("DelegatesToManagerWithGitopsNamespace", func(t *testing.T) {
		// Given a provisioner capturing the prune call
		mocks := setupProvisionerMocks(t)
		var capturedNamespace string
		var capturedBlueprint *blueprintv1alpha1.Blueprint
		mocks.KubernetesManager.PruneBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			capturedNamespace = namespace
			capturedBlueprint = blueprint
			return nil
		}
		opts := &Provisioner{KubernetesManager: mocks.KubernetesManager}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// And a blueprint that vendors CRDs, so Prune must synthesize the CRD layer
		blueprint := createTestBlueprint()
		blueprint.Crds = []string{"example.com_widgets"}

		// When pruning
		if err := provisioner.Prune(context.Background(), blueprint); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it delegates to the manager against the gitops namespace
		if capturedNamespace != provisioner.fluxNamespace() {
			t.Errorf("Expected namespace %q, got %q", provisioner.fluxNamespace(), capturedNamespace)
		}

		// And the captured blueprint carries the synthesized CRD layer, proving withCrdLayer
		// was applied so the CRD kustomizations stay in the desired set and are never pruned
		if capturedBlueprint == nil {
			t.Fatal("Expected a blueprint to be passed to the manager")
		}
		crdName := blueprintv1alpha1.CrdKustomizationName("")
		found := false
		names := make([]string, 0, len(capturedBlueprint.Kustomizations))
		for _, k := range capturedBlueprint.Kustomizations {
			names = append(names, k.Name)
			if k.Name == crdName {
				found = true
			}
		}
		if !found {
			t.Errorf("Expected synthesized CRD kustomization %q in pruned blueprint, got %v", crdName, names)
		}
	})

	t.Run("NilBlueprintReturnsError", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})
		if err := provisioner.Prune(context.Background(), nil); err == nil {
			t.Error("Expected error for nil blueprint")
		}
	})

	t.Run("NilManagerReturnsError", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})
		provisioner.KubernetesManager = nil
		if err := provisioner.Prune(context.Background(), createTestBlueprint()); err == nil {
			t.Error("Expected error for nil kubernetes manager")
		}
	})
}

func TestProvisioner_PrunableKustomizations(t *testing.T) {
	t.Run("DelegatesAndReturnsNames", func(t *testing.T) {
		// Given a manager that reports an orphan kustomization
		mocks := setupProvisionerMocks(t)
		var capturedNamespace string
		mocks.KubernetesManager.ListPrunableKustomizationsFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) ([]string, error) {
			capturedNamespace = namespace
			return []string{"old-thing"}, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When listing prunable kustomizations
		got, err := provisioner.PrunableKustomizations(createTestBlueprint())
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it delegates against the gitops namespace and returns the names
		if capturedNamespace != provisioner.fluxNamespace() {
			t.Errorf("Expected namespace %q, got %q", provisioner.fluxNamespace(), capturedNamespace)
		}
		if len(got) != 1 || got[0] != "old-thing" {
			t.Errorf("Expected [old-thing], got %v", got)
		}
	})

	t.Run("NilBlueprintReturnsError", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})
		if _, err := provisioner.PrunableKustomizations(nil); err == nil {
			t.Error("Expected error for nil blueprint")
		}
	})

	t.Run("NilManagerReturnsError", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})
		provisioner.KubernetesManager = nil
		if _, err := provisioner.PrunableKustomizations(createTestBlueprint()); err == nil {
			t.Error("Expected error for nil kubernetes manager")
		}
	})
}

func TestProvisioner_Uninstall(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.DeleteBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		blueprint := createTestBlueprint()
		err := provisioner.Uninstall(context.Background(), blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithCleanupKustomizations", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.DeleteBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		blueprint := createTestBlueprint()
		blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{
				Name: "test-kustomization",
			},
		}

		err := provisioner.Uninstall(context.Background(), blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessSkipsDestroyFalse", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.DeleteBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		blueprint := createTestBlueprint()
		destroyFalse := false
		blueprint.Kustomizations = []blueprintv1alpha1.Kustomization{
			{
				Name:    "test-kustomization-1",
				Destroy: &blueprintv1alpha1.BoolExpression{Value: &destroyFalse, IsExpr: false},
			},
			{
				Name:    "test-kustomization-2",
				Destroy: nil,
			},
		}

		err := provisioner.Uninstall(context.Background(), blueprint)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("MaterializesCrdLayerForDestroy", func(t *testing.T) {
		// Given a blueprint whose crds: layer must be torn down with the stack
		mocks := setupProvisionerMocks(t)
		var deleted *blueprintv1alpha1.Blueprint
		mocks.KubernetesManager.DeleteBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			deleted = blueprint
			return nil
		}
		opts := &Provisioner{KubernetesManager: mocks.KubernetesManager}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)
		installed := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Install: &blueprintv1alpha1.BoolExpression{Value: &installed}, Crds: []string{"cert-manager-1.16.2"}},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "cert-manager", DependsOn: []string{"crds-core"}}},
		}

		// When uninstalling
		if err := provisioner.Uninstall(context.Background(), blueprint); err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then the synthesized crds layer is part of the destroy walk so the Flux
		// Kustomization object is removed, with pruning left disabled so the CRDs survive
		if deleted == nil || len(deleted.Kustomizations) != 2 {
			t.Fatalf("expected crds kustomization included in destroy, got %+v", deleted)
		}
		crd := deleted.Kustomizations[0]
		if crd.Name != "crds-core" || crd.Source != "core" {
			t.Errorf("expected crds-core bound to core, got name=%q source=%q", crd.Name, crd.Source)
		}
		if crd.Prune == nil || *crd.Prune != false || crd.Wait == nil || *crd.Wait != true {
			t.Errorf("expected prune=false wait=true, got prune=%v wait=%v", crd.Prune, crd.Wait)
		}
	})

	t.Run("SkipsWhenKubeconfigMissing", func(t *testing.T) {
		// A missing kubeconfig means the cluster is already gone. Uninstall must not attempt
		// to build a client against it. See #3397.
		mocks := setupProvisionerMocks(t)
		mocks.Runtime.ConfigRoot = t.TempDir()

		var deleteBlueprintCalled bool
		mocks.KubernetesManager.DeleteBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			deleteBlueprintCalled = true
			return nil
		}
		opts := &Provisioner{KubernetesManager: mocks.KubernetesManager}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		err := provisioner.Uninstall(context.Background(), createTestBlueprint())

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if deleteBlueprintCalled {
			t.Error("expected DeleteBlueprint not to run when kubeconfig is missing")
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		err := provisioner.Uninstall(context.Background(), nil)

		if err == nil {
			t.Error("Expected error for nil blueprint")
		}

		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorNilKubernetesManager", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		provisioner.KubernetesManager = nil

		blueprint := createTestBlueprint()
		err := provisioner.Uninstall(context.Background(), blueprint)

		if err == nil {
			t.Error("Expected error for nil kubernetes manager")
		}

		if !strings.Contains(err.Error(), "kubernetes manager not configured") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorDeleteBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.DeleteBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("delete blueprint failed")
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		blueprint := createTestBlueprint()
		err := provisioner.Uninstall(context.Background(), blueprint)

		if err == nil {
			t.Error("Expected error for delete blueprint failure")
		}

		if !strings.Contains(err.Error(), "failed to delete blueprint") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestProvisioner_converge(t *testing.T) {
	newProv := func(mocks *ProvisionerTestMocks, notifier *fluxinfra.MockNotifier) *Provisioner {
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, Notifier: notifier})
		p.secretPollInterval = time.Millisecond
		return p
	}
	converge := func(p *Provisioner, ctx context.Context, bp *blueprintv1alpha1.Blueprint, timeout time.Duration) {
		layers := presenter.NewGroup(ctx, "kustomization:")
		convergeCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		p.converge(convergeCtx, layers, bp, convergeNames(bp), p.pollInterval())
	}

	t.Run("ReportsEachKustomizationAsAChildStep", func(t *testing.T) {
		// Given one kustomization that becomes Ready on the second read and one that never does
		mocks := setupProvisionerMocks(t)
		var reads int
		mocks.KubernetesManager.GetKustomizationReadinessFunc = func(names []string) (map[string]bool, error) {
			reads++
			return map[string]bool{"pki-base": reads >= 2, "dns": false}, nil
		}
		bp := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "pki-base"}, {Name: "dns"}}}
		rec := &eventRecorder{}

		// When converging with a short timeout
		converge(newProv(mocks, fluxinfra.NewMockNotifier()), presenter.NewContext(context.Background(), rec), bp, 50*time.Millisecond)

		// Then the ready kustomization's step ends applied and the unready one's stays open
		ends := map[string]presenter.Kind{}
		for _, e := range rec.events {
			if e.Kind == presenter.KindApplied || e.Kind == presenter.KindFailed {
				ends[e.Message] = e.Kind
			}
		}
		if !reflect.DeepEqual(ends, map[string]presenter.Kind{"pki-base": presenter.KindApplied}) {
			t.Errorf("unexpected step ends %v", ends)
		}
	})

	t.Run("HidesKustomizationsBlockedOnADependency", func(t *testing.T) {
		// Given a kustomization that never becomes Ready and one that depends on it
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetKustomizationReadinessFunc = func(names []string) (map[string]bool, error) {
			return map[string]bool{"pki-base": false, "dns": false}, nil
		}
		bp := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			{Name: "pki-base"},
			{Name: "dns", DependsOn: []string{"pki-base"}},
		}}
		rec := &eventRecorder{}

		// When converging with a short timeout
		converge(newProv(mocks, fluxinfra.NewMockNotifier()), presenter.NewContext(context.Background(), rec), bp, 30*time.Millisecond)

		// Then only the reconciling kustomization appears, and the blocked one never does
		if ops := rec.operations(); !reflect.DeepEqual(ops, []string{"pki-base"}) {
			t.Errorf("expected only pki-base to appear, got %v", ops)
		}
	})

	t.Run("DrivesNotReadyKustomizationsAndForcesStalledHelmReleases", func(t *testing.T) {
		// Given a kustomization not Ready on the first read then Ready, owning a stalled HelmRelease
		mocks := setupProvisionerMocks(t)
		var reads int
		mocks.KubernetesManager.GetKustomizationReadinessFunc = func(names []string) (map[string]bool, error) {
			reads++
			out := make(map[string]bool)
			for _, n := range names {
				out[n] = reads >= 2
			}
			return out, nil
		}
		mocks.KubernetesManager.GetHelmReleasesForKustomizationFunc = func(name, namespace string) ([]helmv2.HelmRelease, error) {
			return []helmv2.HelmRelease{{
				ObjectMeta: metav1.ObjectMeta{Namespace: "system-lb", Name: "hcloud-ccm"},
				Status:     helmv2.HelmReleaseStatus{Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionFalse}}},
			}}, nil
		}
		notifier := fluxinfra.NewMockNotifier()
		var reconciledK [][]string
		var forcedRefs [][]fluxinfra.HelmReleaseRef
		var forcedFlags []bool
		notifier.ReconcileKustomizationsFunc = func(ctx context.Context, names []string) error {
			reconciledK = append(reconciledK, names)
			return nil
		}
		notifier.ReconcileHelmReleasesFunc = func(ctx context.Context, refs []fluxinfra.HelmReleaseRef, force bool) error {
			forcedRefs = append(forcedRefs, refs)
			forcedFlags = append(forcedFlags, force)
			return nil
		}
		bp := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "lb-install"}}}

		// When converging, it nudges the not-ready kustomization and force-reconciles its stalled HR, then
		// returns once everything reports Ready
		converge(newProv(mocks, notifier), context.Background(), bp, time.Second)
		if len(reconciledK) == 0 || !slices.Contains(reconciledK[0], "lb-install") {
			t.Errorf("expected lb-install reconciled, got %v", reconciledK)
		}
		if len(forcedRefs) == 0 || forcedRefs[0][0].Name != "hcloud-ccm" || !forcedFlags[0] {
			t.Errorf("expected stalled HR force-reconciled, got refs=%v force=%v", forcedRefs, forcedFlags)
		}
	})

	t.Run("ReturnsImmediatelyWhenAllReady", func(t *testing.T) {
		// Given everything already Ready
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetKustomizationReadinessFunc = func(names []string) (map[string]bool, error) {
			out := make(map[string]bool)
			for _, n := range names {
				out[n] = true
			}
			return out, nil
		}
		notifier := fluxinfra.NewMockNotifier()
		nudged := false
		notifier.ReconcileKustomizationsFunc = func(ctx context.Context, names []string) error {
			nudged = true
			return nil
		}
		bp := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "lb-install"}}}

		// When converging, it returns without nudging anything
		converge(newProv(mocks, notifier), context.Background(), bp, time.Second)
		if nudged {
			t.Error("expected no reconcile when everything is already Ready")
		}
	})

	t.Run("LeavesHealthyHelmReleasesUntouched", func(t *testing.T) {
		// Given a not-ready kustomization whose HelmRelease is nonetheless Ready
		mocks := setupProvisionerMocks(t)
		var reads int
		mocks.KubernetesManager.GetKustomizationReadinessFunc = func(names []string) (map[string]bool, error) {
			reads++
			out := make(map[string]bool)
			for _, n := range names {
				out[n] = reads >= 2
			}
			return out, nil
		}
		mocks.KubernetesManager.GetHelmReleasesForKustomizationFunc = func(name, namespace string) ([]helmv2.HelmRelease, error) {
			return []helmv2.HelmRelease{{
				ObjectMeta: metav1.ObjectMeta{Namespace: "system-lb", Name: "healthy"},
				Status:     helmv2.HelmReleaseStatus{Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}},
			}}, nil
		}
		notifier := fluxinfra.NewMockNotifier()
		forcedAny := false
		notifier.ReconcileHelmReleasesFunc = func(ctx context.Context, refs []fluxinfra.HelmReleaseRef, force bool) error {
			forcedAny = true
			return nil
		}
		bp := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "lb-install"}}}

		// When converging, a healthy HelmRelease is not force-reconciled
		converge(newProv(mocks, notifier), context.Background(), bp, time.Second)
		if forcedAny {
			t.Error("expected healthy HelmRelease left untouched")
		}
	})
}

func TestProvisioner_nudgeFrontier(t *testing.T) {
	// crds Ready; lb depends on crds (frontier); gateway depends on the not-ready lb (blocked, not frontier).
	bp := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
		{Name: "crds"},
		{Name: "lb-install", DependsOn: []string{"crds"}},
		{Name: "gateway-install", DependsOn: []string{"lb-install"}},
	}}
	newProv := func(mocks *ProvisionerTestMocks, notifier *fluxinfra.MockNotifier) *Provisioner {
		return NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager, Notifier: notifier})
	}

	t.Run("NudgesOnlyNotReadyFrontierAndNeverReadyOrBlocked", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetKustomizationReadinessFunc = func(names []string) (map[string]bool, error) {
			return map[string]bool{"crds": true, "lb-install": false, "gateway-install": false}, nil
		}
		notifier := fluxinfra.NewMockNotifier()
		var nudgedNames [][]string
		notifier.ReconcileKustomizationsFunc = func(ctx context.Context, names []string) error {
			nudgedNames = append(nudgedNames, names)
			return nil
		}

		// When nudging, only lb-install (not-ready, deps ready) is reconciled — not crds (Ready) nor
		// gateway-install (blocked on not-ready lb-install)
		newProv(mocks, notifier).nudgeFrontier(context.Background(), bp, map[string]struct{}{})
		if len(nudgedNames) != 1 || !slices.Equal(nudgedNames[0], []string{"lb-install"}) {
			t.Errorf("expected only lb-install nudged, got %v", nudgedNames)
		}
	})

	t.Run("DuplicateNameIsQueriedOnce", func(t *testing.T) {
		// Given a blueprint whose crds kustomization name is declared twice (e.g. a collision
		// with withCrdLayer's synthesized entry), all Ready
		dup := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			{Name: "crds"},
			{Name: "crds"},
			{Name: "lb-install", DependsOn: []string{"crds"}},
		}}
		mocks := setupProvisionerMocks(t)
		var queried []string
		mocks.KubernetesManager.GetKustomizationReadinessFunc = func(names []string) (map[string]bool, error) {
			queried = names
			return map[string]bool{"crds": true, "lb-install": false}, nil
		}
		notifier := fluxinfra.NewMockNotifier()

		// When nudging, the duplicate is queried once rather than twice
		newProv(mocks, notifier).nudgeFrontier(context.Background(), dup, map[string]struct{}{})
		if len(queried) != 2 || !slices.Contains(queried, "crds") || !slices.Contains(queried, "lb-install") {
			t.Errorf("expected [crds lb-install] queried once each, got %v", queried)
		}
	})

	t.Run("NudgesEachFrontierMemberOnlyOnce", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetKustomizationReadinessFunc = func(names []string) (map[string]bool, error) {
			return map[string]bool{"crds": true, "lb-install": false, "gateway-install": false}, nil
		}
		notifier := fluxinfra.NewMockNotifier()
		calls := 0
		notifier.ReconcileKustomizationsFunc = func(ctx context.Context, names []string) error {
			calls++
			return nil
		}
		prov := newProv(mocks, notifier)
		nudged := map[string]struct{}{}

		// When nudging twice with lb-install still not-Ready, it is reconciled only on the first pass
		prov.nudgeFrontier(context.Background(), bp, nudged)
		prov.nudgeFrontier(context.Background(), bp, nudged)
		if calls != 1 {
			t.Errorf("expected the frontier member nudged once, got %d nudges", calls)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestConvergeNames(t *testing.T) {
	t.Run("DeduplicatesACollidingName", func(t *testing.T) {
		// Given a blueprint where a kustomization name appears twice — e.g. withCrdLayer's
		// synthesized CRD kustomization colliding with a name already declared elsewhere
		bp := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			{Name: "crds-core"},
			{Name: "crds-core"},
			{Name: "lb-install"},
		}}

		// When collecting the names converge drives toward Ready
		names := convergeNames(bp)

		// Then the duplicate collapses to one entry, so a nudgeFrontier readiness comparison
		// against this set can't be inflated past what is actually reachable
		if !slices.Equal(names, []string{"crds-core", "lb-install"}) {
			t.Errorf("expected deduplicated [crds-core lb-install], got %v", names)
		}
	})

	t.Run("SkipsDestroyOnlyRegardlessOfADuplicateNonDestroyOnlyEntry", func(t *testing.T) {
		destroyOnly := true
		bp := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			{Name: "a"},
			{Name: "a", DestroyOnly: &destroyOnly},
		}}

		names := convergeNames(bp)

		if !slices.Equal(names, []string{"a"}) {
			t.Errorf("expected [a], got %v", names)
		}
	})
}
