package kubernetes

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseKubernetesManager_DeleteKustomization(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		// Use shorter timeouts for tests
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationReconcileSleep = 50 * time.Millisecond
		clock := newFakeClock()
		manager.shims.TimeNow = clock.Now
		manager.shims.TimeSleep = clock.Sleep
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		// Mock GetResource to return "not found" immediately to simulate successful deletion
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("DeleteError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return fmt.Errorf("delete error")
		}
		manager.client = kubernetesClient

		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("KustomizationNotFound", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")
		if err != nil {
			t.Errorf("Expected no error for not found resource, got %v", err)
		}
	})

	t.Run("TimeoutWaitingForDeletion", func(t *testing.T) {
		// Given a kustomization that never disappears from etcd (stuck in
		// Terminating because some inventory item is held by a cloud-controller
		// finalizer)
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		}
		manager.client = kubernetesClient
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond

		// When DeleteKustomization is called
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then an error is returned describing the timeout — no finalizer strip
		// happens. Stripping the Kustomization's own finalizer would reap it
		// from etcd while leaving inventory items orphaned in their namespace,
		// hiding the failure. The operator must investigate and re-run.
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Errorf("Expected error mentioning timeout, got: %v", err)
		}
		if !strings.Contains(err.Error(), "test-kustomization") {
			t.Errorf("Expected error to name the stuck kustomization, got: %v", err)
		}
	})

	t.Run("TimeoutSurfacesStatusCondition", func(t *testing.T) {
		// Given a kustomization that never disappears and whose status reports the
		// real reason it is stuck (Flux records the prune failure in a condition)
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{
				"status": map[string]any{
					"conditions": []any{
						map[string]any{
							"type":    "Ready",
							"status":  "False",
							"reason":  "PruneFailed",
							"message": "ServiceAccount/telemetry/collector still has finalizer",
						},
					},
				},
			}}, nil
		}
		manager.client = kubernetesClient
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then the error surfaces the Flux condition reason and message inline so the
		// operator sees why without a manual kubectl round-trip
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "PruneFailed") {
			t.Errorf("Expected error to surface the condition reason, got: %v", err)
		}
		if !strings.Contains(err.Error(), "still has finalizer") {
			t.Errorf("Expected error to surface the condition message, got: %v", err)
		}
	})

	t.Run("TimeoutFlattensMultilineConditionMessage", func(t *testing.T) {
		// Given a stuck kustomization whose condition message spans multiple lines
		// (Flux records YAML snippets and chained failures this way)
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{
				"status": map[string]any{
					"conditions": []any{
						map[string]any{
							"type":    "Stalled",
							"status":  "True",
							"reason":  "ReconciliationFailed",
							"message": "prune failed:\nSecret/telemetry/token dependency not ready\nretrying",
						},
					},
				},
			}}, nil
		}
		manager.client = kubernetesClient
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then the embedded newlines are flattened so the error stays on one line
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		condPart := err.Error()
		if i := strings.Index(condPart, "(Stalled"); i >= 0 {
			condPart = condPart[i:]
		}
		if strings.Contains(condPart, "\n") {
			t.Errorf("Expected condition detail to be single-line, got embedded newline: %q", err)
		}
		if !strings.Contains(err.Error(), "prune failed: Secret/telemetry/token dependency not ready retrying") {
			t.Errorf("Expected flattened message, got: %v", err)
		}
	})

	t.Run("TimeoutSurfacesHelmReleaseCondition", func(t *testing.T) {
		// Given a stuck kustomization with a bare "Progressing" status, but its
		// inventory names a HelmRelease that is itself stalled
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Group == "helm.toolkit.fluxcd.io" && gvr.Resource == "helmreleases" {
				return &unstructured.Unstructured{Object: map[string]any{
					"apiVersion": "helm.toolkit.fluxcd.io/v2",
					"kind":       "HelmRelease",
					"metadata": map[string]any{
						"name":       "cloudnativepg",
						"namespace":  "system-database",
						"finalizers": []any{"finalizers.fluxcd.io"},
					},
					"status": map[string]any{
						"conditions": []any{
							map[string]any{
								"type":    "Stalled",
								"status":  "True",
								"reason":  "UninstallFailed",
								"message": "CustomResourceDefinition/clusters.postgresql.cnpg.io: still has 1 instance(s)",
							},
						},
					},
				}}, nil
			}
			return &unstructured.Unstructured{Object: map[string]any{
				"status": map[string]any{
					"conditions": []any{
						map[string]any{
							"type":   "Ready",
							"status": "Unknown",
							"reason": "Progressing",
						},
					},
					"inventory": map[string]any{
						"entries": []any{
							map[string]any{
								"id": "system-database_cloudnativepg_helm.toolkit.fluxcd.io_HelmRelease",
							},
						},
					},
				},
			}}, nil
		}
		manager.client = kubernetesClient
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "database-install", "system-gitops")

		// Then the error surfaces the HelmRelease's own stuck reason
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "HelmRelease system-database/cloudnativepg") {
			t.Errorf("Expected error to name the stuck HelmRelease, got: %v", err)
		}
		if !strings.Contains(err.Error(), "UninstallFailed") {
			t.Errorf("Expected error to surface the HelmRelease's condition reason, got: %v", err)
		}
		if !strings.Contains(err.Error(), "still has 1 instance(s)") {
			t.Errorf("Expected error to surface the HelmRelease's condition message, got: %v", err)
		}
	})

	t.Run("TimeoutHelmReleaseLookupErrorIsSwallowed", func(t *testing.T) {
		// Given a stuck kustomization whose HelmRelease lookup itself fails
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		calls := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			calls++
			if calls == 1 {
				// deletion-status poll: one call, then timeout
				return &unstructured.Unstructured{}, nil
			}
			// describeStuckHelmReleases' own re-fetch
			return nil, fmt.Errorf("apiserver unreachable")
		}
		manager.client = kubernetesClient
		// timeout < poll interval: deletion-status loop makes exactly one call
		manager.kustomizationReconcileTimeout = 10 * time.Millisecond
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then the timeout error still surfaces; the lookup failure is swallowed
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Errorf("Expected error mentioning timeout, got: %v", err)
		}
	})

	t.Run("ErrorCheckingDeletionStatus", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("error checking status")
		}
		manager.client = kubernetesClient
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond
		manager.kustomizationWaitMaxReadFailures = 0

		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error checking kustomization test-namespace/test-kustomization deletion status") {
			t.Errorf("Expected error checking status, got: %v", err)
		}
	})

	t.Run("UsesCorrectDeleteOptions", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		var capturedOptions metav1.DeleteOptions
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			capturedOptions = opts
			return nil
		}
		// Mock GetResource to return "not found" immediately to simulate successful deletion
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify the correct delete options were used
		if capturedOptions.PropagationPolicy == nil {
			t.Error("Expected PropagationPolicy to be set")
		} else if *capturedOptions.PropagationPolicy != metav1.DeletePropagationBackground {
			t.Errorf("Expected PropagationPolicy to be DeletePropagationBackground, got %s", *capturedOptions.PropagationPolicy)
		}
	})

	t.Run("TimeoutWithoutConfirmedReasonIsHonestAboutUncertainty", func(t *testing.T) {
		// Given a kustomization that never disappears and carries no diagnostic
		// condition, so a still-deleting kustomization looks identical to a
		// genuinely stuck one from status.conditions alone
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		}
		manager.client = kubernetesClient
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then the error does not assert a stuck finalizer it cannot confirm, and
		// tells the operator to check whether deletion is still progressing
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if strings.Contains(err.Error(), "is likely stuck on a cloud-controller finalizer") {
			t.Errorf("Expected no unconfirmed stuck-finalizer claim, got: %v", err)
		}
		if !strings.Contains(err.Error(), "No status condition confirms a stuck finalizer") {
			t.Errorf("Expected error to acknowledge the uncertainty, got: %v", err)
		}
	})

	t.Run("TimeoutWithConfirmedReasonKeepsStuckFinalizerWording", func(t *testing.T) {
		// Given a kustomization that never disappears and carries a diagnostic
		// condition confirming it is genuinely stuck
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{
				"status": map[string]any{
					"conditions": []any{
						map[string]any{
							"type":    "Stalled",
							"status":  "True",
							"reason":  "PruneFailed",
							"message": "ServiceAccount/telemetry/collector still has finalizer",
						},
					},
				},
			}}, nil
		}
		manager.client = kubernetesClient
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then the error keeps asserting the stuck finalizer, since Flux's own
		// condition confirms it, and does not use the unconfirmed wording
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "is likely stuck on a cloud-controller finalizer") {
			t.Errorf("Expected confirmed stuck-finalizer wording, got: %v", err)
		}
		if strings.Contains(err.Error(), "No status condition confirms a stuck finalizer") {
			t.Errorf("Expected no unconfirmed-deletion wording, got: %v", err)
		}
	})

	t.Run("TimeoutWithMirrorPruneDoesNotClaimStuckFinalizer", func(t *testing.T) {
		// Given a MirrorPrune kustomization (destroy:false) that never disappears and
		// carries no diagnostic condition — MirrorPrune never waits on inventory, so a
		// timeout here cannot be a stuck cloud-controller finalizer
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{
				"spec": map[string]any{"deletionPolicy": "MirrorPrune"},
			}}, nil
		}
		manager.client = kubernetesClient
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then the error names MirrorPrune instead of blaming a stuck finalizer
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if strings.Contains(err.Error(), "stuck on a cloud-controller finalizer") {
			t.Errorf("Expected no cloud-controller-finalizer wording for MirrorPrune, got: %v", err)
		}
		if !strings.Contains(err.Error(), "MirrorPrune") {
			t.Errorf("Expected error to name MirrorPrune, got: %v", err)
		}
	})

	t.Run("TimeoutWithMirrorPruneAndConfirmedReasonNamesConditionNotInventory", func(t *testing.T) {
		// Given a MirrorPrune kustomization with a diagnostic condition confirming a
		// suspended reconcile, not a stuck inventory item
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{
				"spec": map[string]any{"deletionPolicy": "MirrorPrune"},
				"status": map[string]any{
					"conditions": []any{
						map[string]any{
							"type":    "Ready",
							"status":  "False",
							"reason":  "ReconciliationSuspended",
							"message": "reconciliation is suspended",
						},
					},
				},
			}}, nil
		}
		manager.client = kubernetesClient
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then the error keeps the condition detail but drops the cloud-controller-finalizer wording
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "ReconciliationSuspended") {
			t.Errorf("Expected error to keep the status condition detail, got: %v", err)
		}
		if strings.Contains(err.Error(), "stuck on a cloud-controller finalizer") {
			t.Errorf("Expected no cloud-controller-finalizer wording for MirrorPrune, got: %v", err)
		}
	})

	t.Run("TimeoutWindowScalesWithInventorySize", func(t *testing.T) {
		// Given a kustomization with a large inventory (a CRD-heavy layer)
		// that never disappears within the base timeout
		manager := setup(t)
		manager.kustomizationReconcileTimeout = 20 * time.Millisecond
		manager.kustomizationWaitPollInterval = 10 * time.Millisecond
		manager.kustomizationDeletionPerEntryTimeout = 1 * time.Millisecond
		manager.kustomizationDeletionMaxExtraTimeout = time.Second

		inventoryEntries := make([]any, 100)
		for i := range inventoryEntries {
			inventoryEntries[i] = map[string]any{"id": fmt.Sprintf("ns_res%d_v1_ConfigMap", i)}
		}

		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{
				"status": map[string]any{
					"inventory": map[string]any{
						"entries": inventoryEntries,
					},
				},
			}}, nil
		}
		manager.client = kubernetesClient

		// When DeleteKustomization times out
		start := manager.shims.TimeNow()
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")
		elapsed := manager.shims.TimeNow().Sub(start)

		// Then it waits past the base timeout, scaled by the 100-entry inventory
		// (100 * 1ms = 100ms extra), instead of aborting at the fixed 20ms window
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if elapsed < 100*time.Millisecond {
			t.Errorf("Expected wait window scaled by inventory size (>=100ms), got %s", elapsed)
		}
	})

	t.Run("TimeoutWindowScalesEvenWhenFirstReadHasNoInventory", func(t *testing.T) {
		// Given a kustomization whose inventory is empty on the first poll (a status
		// write racing the delete) but populated on every later poll
		manager := setup(t)
		manager.kustomizationReconcileTimeout = 20 * time.Millisecond
		manager.kustomizationWaitPollInterval = 10 * time.Millisecond
		manager.kustomizationDeletionPerEntryTimeout = 1 * time.Millisecond
		manager.kustomizationDeletionMaxExtraTimeout = time.Second

		inventoryEntries := make([]any, 100)
		for i := range inventoryEntries {
			inventoryEntries[i] = map[string]any{"id": fmt.Sprintf("ns_res%d_v1_ConfigMap", i)}
		}

		calls := 0
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			calls++
			if calls == 1 {
				return &unstructured.Unstructured{}, nil
			}
			return &unstructured.Unstructured{Object: map[string]any{
				"status": map[string]any{
					"inventory": map[string]any{
						"entries": inventoryEntries,
					},
				},
			}}, nil
		}
		manager.client = kubernetesClient

		// When DeleteKustomization times out
		start := manager.shims.TimeNow()
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")
		elapsed := manager.shims.TimeNow().Sub(start)

		// Then a later, populated read still scales the wait window — the empty
		// first read does not permanently lock in the unscaled base timeout
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if elapsed < 100*time.Millisecond {
			t.Errorf("Expected wait window scaled by a later inventory read (>=100ms), got %s", elapsed)
		}
	})

	t.Run("TimeoutWindowRespectsSpecTimeout", func(t *testing.T) {
		// Given a kustomization whose own spec.timeout is larger than the base window
		// (an operator-configured allowance for a slow-to-delete resource)
		manager := setup(t)
		manager.kustomizationReconcileTimeout = 20 * time.Millisecond
		manager.kustomizationWaitPollInterval = 10 * time.Millisecond

		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{
				"spec": map[string]any{"timeout": "150ms"},
			}}, nil
		}
		manager.client = kubernetesClient

		// When DeleteKustomization times out
		start := manager.shims.TimeNow()
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")
		elapsed := manager.shims.TimeNow().Sub(start)

		// Then it waits out spec.timeout instead of aborting at the base window,
		// and does not run away past it on every poll tick
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if elapsed < 150*time.Millisecond {
			t.Errorf("Expected wait window raised to spec.timeout (>=150ms), got %s", elapsed)
		}
		if elapsed > 250*time.Millisecond {
			t.Errorf("Expected wait window bounded near spec.timeout (<=250ms), got %s", elapsed)
		}
	})

	t.Run("TimeoutWindowCapsSpecTimeoutAtCeiling", func(t *testing.T) {
		// Given a spec.timeout far larger than the configured ceiling (a corrupted
		// or absurd value, e.g. from a hand-edited blueprint)
		manager := setup(t)
		manager.kustomizationReconcileTimeout = 20 * time.Millisecond
		manager.kustomizationWaitPollInterval = 10 * time.Millisecond
		manager.kustomizationSpecTimeoutCeiling = 100 * time.Millisecond

		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{
				"spec": map[string]any{"timeout": "876000h"},
			}}, nil
		}
		manager.client = kubernetesClient

		// When DeleteKustomization times out
		start := manager.shims.TimeNow()
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")
		elapsed := manager.shims.TimeNow().Sub(start)

		// Then the wait is bounded by the ceiling, not the declared spec.timeout
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if elapsed < 100*time.Millisecond || elapsed > 250*time.Millisecond {
			t.Errorf("Expected wait window capped near the ceiling (~100ms), got %s", elapsed)
		}
	})

	t.Run("TimeoutIgnoresUnparseableSpecTimeout", func(t *testing.T) {
		// Given a kustomization whose spec.timeout is present but malformed
		manager := setup(t)
		manager.kustomizationReconcileTimeout = 20 * time.Millisecond
		manager.kustomizationWaitPollInterval = 10 * time.Millisecond

		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{
				"spec": map[string]any{"timeout": "not-a-duration"},
			}}, nil
		}
		manager.client = kubernetesClient

		// When DeleteKustomization times out
		start := manager.shims.TimeNow()
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")
		elapsed := manager.shims.TimeNow().Sub(start)

		// Then it falls back to the base window rather than erroring on the bad value
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if elapsed > 100*time.Millisecond {
			t.Errorf("Expected base window (~20ms) when spec.timeout is unparseable, got %s", elapsed)
		}
	})

	drainedKustomization := func() *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{
			"status": map[string]any{
				"inventory": map[string]any{
					"entries": []any{
						map[string]any{"id": "test-namespace_entry-one__ConfigMap"},
						map[string]any{"id": "test-namespace_entry-two_apps_Deployment"},
					},
				},
			},
		}}
	}

	waitForTerminationKustomization := func() *unstructured.Unstructured {
		obj := drainedKustomization()
		obj.Object["spec"] = map[string]any{"deletionPolicy": "WaitForTermination"}
		return obj
	}

	t.Run("TimeoutReportsDrainedKustomizationAsStuckFinalizer", func(t *testing.T) {
		// Given every inventory entry's own live object confirmed gone
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if name == "test-kustomization" {
				return drainedKustomization(), nil
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it reports the drained state and a manual fix, not a suspected leak
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "fully drained") || !strings.Contains(err.Error(), "kubectl patch") {
			t.Errorf("Expected drained-and-stuck-finalizer wording naming a fix, got: %v", err)
		}
	})

	t.Run("TimeoutDoesNotClaimDrainedWhenEntryStillLive", func(t *testing.T) {
		// Given one inventory entry whose live object still exists — the safety-critical
		// case: nothing here may be treated as drained
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if name == "test-kustomization" {
				return drainedKustomization(), nil
			}
			if name == "entry-one" {
				return blockingObject(), nil
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it does not claim the kustomization is drained
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if strings.Contains(err.Error(), "fully drained") {
			t.Errorf("Expected fallback wording when an entry is still live, got: %v", err)
		}
	})

	t.Run("TimeoutTriggersReconcileOnTheBlockingEntryBeforeReporting", func(t *testing.T) {
		// Given the same still-live, still-blocking entry as above, staying live
		// through the post-timeout recheck too
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if name == "test-kustomization" {
				return drainedKustomization(), nil
			}
			if name == "entry-one" {
				return blockingObject(), nil
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		var triggered []string
		kubernetesClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			triggered = append(triggered, name)
			return nil, nil
		}
		manager.client = kubernetesClient
		ctx, rec := recordProgress(t)

		// When DeleteKustomization times out with the entry never clearing
		err := manager.DeleteKustomization(ctx, "test-kustomization", "test-namespace")

		// Then it still reports the timeout, but only after asking the blocking
		// entry's controller to look again
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if len(triggered) == 0 {
			t.Fatal("Expected at least one reconcile trigger against the blocking entry, got none")
		}
		for _, name := range triggered {
			if name != "entry-one" {
				t.Errorf("Expected the trigger to target entry-one, got: %s", name)
			}
		}
		// And an operator watching the run sees why it is still waiting, not silence
		found := false
		for _, u := range rec.progress() {
			if strings.Contains(u, "entry-one") && strings.Contains(u, "asked it to reconcile again") {
				found = true
			}
		}
		if !found {
			t.Errorf("Expected a progress update naming entry-one, got: %v", rec.progress())
		}
	})

	t.Run("TimeoutSucceedsWhenTheKustomizationDisappearsDuringTheRecheck", func(t *testing.T) {
		// Given the blocking entry clears and the kustomization itself vanishes
		// shortly after the post-timeout trigger — the main wait loop alone
		// would have reported a timeout, never learning either happened
		manager := setup(t)
		// A 2-entry inventory would otherwise extend waitFor via
		// kustomizationDeletionTimeout, changing how many reads the main loop
		// itself makes before this test's post-timeout recheck is even reached.
		manager.kustomizationDeletionPerEntryTimeout = 0
		manager.kustomizationDeletionMaxExtraTimeout = 0
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kustCalls := 0
		entryCalls := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			switch name {
			case "test-kustomization":
				kustCalls++
				if kustCalls <= 2 {
					return waitForTerminationKustomization(), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			case "entry-one":
				entryCalls++
				if entryCalls == 1 {
					return blockingObject(), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			default:
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
		}
		manager.client = kubernetesClient

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then the recheck catches the disappearance and the clear entry, and
		// the delete succeeds instead of reporting a timeout
		if err != nil {
			t.Errorf("Expected the recheck to catch the disappearance and succeed, got: %v", err)
		}
	})

	t.Run("TimeoutWithMirrorPruneSkipsTheRecheckTrigger", func(t *testing.T) {
		// Given a MirrorPrune kustomization that times out with a still-live entry —
		// its normal, by-design outcome, since MirrorPrune never waits on inventory
		manager := setup(t)
		manager.kustomizationReconcileTimeout = 20 * time.Millisecond
		manager.kustomizationWaitPollInterval = 10 * time.Millisecond
		// A 2-entry inventory would otherwise extend waitFor via kustomizationDeletionTimeout,
		// making the assertion below about elapsed time depend on inventory size.
		manager.kustomizationDeletionPerEntryTimeout = 0
		manager.kustomizationDeletionMaxExtraTimeout = 0
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if name == "test-kustomization" {
				obj := drainedKustomization()
				obj.Object["spec"] = map[string]any{"deletionPolicy": "MirrorPrune"}
				return obj, nil
			}
			if name == "entry-one" {
				return blockingObject(), nil
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		var triggered []string
		kubernetesClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			triggered = append(triggered, name)
			return nil, nil
		}
		manager.client = kubernetesClient

		// When DeleteKustomization times out
		start := manager.shims.TimeNow()
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")
		elapsed := manager.shims.TimeNow().Sub(start)

		// Then it reports the timeout without triggering a reconcile or spending
		// the post-timeout recheck window on an entry MirrorPrune expects to
		// leave behind
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if len(triggered) != 0 {
			t.Errorf("Expected no reconcile trigger for MirrorPrune, got: %v", triggered)
		}
		if elapsed > 100*time.Millisecond {
			t.Errorf("Expected no post-timeout recheck delay for MirrorPrune, got %s", elapsed)
		}
	})

	t.Run("TimeoutNamesResidueInsteadOfClaimingFullyDrained", func(t *testing.T) {
		// Given one inventory entry gone and one live with no finalizer, during a
		// destroy — real residue, not a confirmed-gone state
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			switch name {
			case "test-kustomization":
				return drainedKustomization(), nil
			case "entry-one":
				return &unstructured.Unstructured{}, nil
			default:
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
		}
		manager.client = kubernetesClient
		expectWaitForTermination := true

		// When the destroy walk times out
		err := manager.deleteKustomization(context.Background(), "test-kustomization", "test-namespace", &expectWaitForTermination, nil, true)

		// Then it names the leftover count, not the fully-drained wording
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if strings.Contains(err.Error(), "fully drained") {
			t.Errorf("Expected residue wording, got: %v", err)
		}
		if !strings.Contains(err.Error(), "left 1 resource(s) behind") {
			t.Errorf("Expected the leftover count named, got: %v", err)
		}
	})

	t.Run("TimeoutFallsBackWhenInventoryCheckInconclusive", func(t *testing.T) {
		// Given an inventory entry whose live status can't be determined (a real API error,
		// not a NotFound) — a false "all gone" reading here is the exact risk to avoid
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if name == "test-kustomization" {
				return drainedKustomization(), nil
			}
			return nil, fmt.Errorf("connection refused")
		}
		manager.client = kubernetesClient

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it falls back to the original diagnosis rather than claiming drained
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if strings.Contains(err.Error(), "fully drained") {
			t.Errorf("Expected fallback wording on an inconclusive check, got: %v", err)
		}
	})

	t.Run("TimeoutTreatsDeletedResourceTypeAsGone", func(t *testing.T) {
		// Given an inventory entry whose CRD no longer exists — nothing left to leak
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.ResourceForFunc = func(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
			return schema.GroupVersionResource{}, &apimeta.NoKindMatchError{GroupKind: gvk.GroupKind()}
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if name == "test-kustomization" {
				return drainedKustomization(), nil
			}
			return nil, fmt.Errorf("should not be called when the type cannot be resolved")
		}
		manager.client = kubernetesClient

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then a deleted resource type still counts as drained
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "fully drained") {
			t.Errorf("Expected drained wording when the entry's type no longer exists, got: %v", err)
		}
	})

	t.Run("TimeoutIgnoresInventoryNeverReported", func(t *testing.T) {
		// Given a Kustomization that has never reconciled far enough to report an
		// inventory at all — not the same as a reconciled, now-empty one
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return blockingObject(), nil
		}
		manager.client = kubernetesClient

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it does not claim the kustomization is drained
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if strings.Contains(err.Error(), "fully drained") {
			t.Errorf("Expected fallback wording when no inventory was ever reported, got: %v", err)
		}
	})

	t.Run("DisappearedReportsAbandonedInventoryWhenEntryStillLive", func(t *testing.T) {
		// Given a WaitForTermination kustomization that disappears after one tick, and
		// its last-known inventory entry stays live through the grace retries —
		// kustomize-controller gave up and stripped its own finalizer early
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		calls := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if name == "test-kustomization" {
				calls++
				if calls <= 2 {
					return waitForTerminationKustomization(), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			if name == "entry-one" {
				return blockingObject(), nil
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		// When DeleteKustomization sees the object disappear
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it reports the still-live entry instead of a clean delete
		if err == nil {
			t.Fatal("Expected an error naming the abandoned inventory, got nil")
		}
		if !strings.Contains(err.Error(), "entry-one") {
			t.Errorf("Expected error to name the still-live entry, got: %v", err)
		}
	})

	t.Run("DisappearedTriggersReconcileOnTheBlockingEntry", func(t *testing.T) {
		// Given the same still-live, still-blocking entry as above
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		calls := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if name == "test-kustomization" {
				calls++
				if calls <= 2 {
					return waitForTerminationKustomization(), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			if name == "entry-one" {
				return blockingObject(), nil
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		var triggered []string
		kubernetesClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			triggered = append(triggered, name)
			return nil, nil
		}
		manager.client = kubernetesClient
		ctx, rec := recordProgress(t)

		// When DeleteKustomization waits out the grace retries
		_ = manager.DeleteKustomization(ctx, "test-kustomization", "test-namespace")

		// Then the blocking entry is asked to reconcile, not just re-read
		if len(triggered) == 0 {
			t.Fatal("Expected at least one reconcile trigger against the blocking entry, got none")
		}
		for _, name := range triggered {
			if name != "entry-one" {
				t.Errorf("Expected the trigger to target entry-one, got: %s", name)
			}
		}
		// And an operator watching the run sees why it is still waiting, not silence
		found := false
		for _, u := range rec.progress() {
			if strings.Contains(u, "entry-one") && strings.Contains(u, "asked it to reconcile again") {
				found = true
			}
		}
		if !found {
			t.Errorf("Expected a progress update naming entry-one, got: %v", rec.progress())
		}
	})

	t.Run("DisappearedRecoversWhenEntrySettlesDuringGracePeriod", func(t *testing.T) {
		// Given a still-live entry that finishes its own normal termination within the
		// grace retries — not a sign Flux gave up, just a resource a moment from gone
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		calls, entryCalls := 0, 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if name == "test-kustomization" {
				calls++
				if calls <= 2 {
					return waitForTerminationKustomization(), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			if name == "entry-one" {
				entryCalls++
				if entryCalls == 1 {
					return blockingObject(), nil
				}
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		// When DeleteKustomization sees the object disappear
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it reports success once the entry settles
		if err != nil {
			t.Errorf("Expected no error once the entry settles within the grace period, got %v", err)
		}
	})

	t.Run("DisappearedSucceedsWhenOnlyResidueSurvives", func(t *testing.T) {
		// Given a destroy where the kustomization disappears and its last-known
		// inventory entries stay live throughout, but none carry a finalizer —
		// real residue, never a reason to block
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		calls := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if name == "test-kustomization" {
				calls++
				if calls <= 2 {
					return drainedKustomization(), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			// entry-one and entry-two: live for the rest of the run, no finalizer
			return &unstructured.Unstructured{}, nil
		}
		manager.client = kubernetesClient
		expectWaitForTermination := true

		// When the destroy walk sees the kustomization disappear
		err := manager.deleteKustomization(context.Background(), "test-kustomization", "test-namespace", &expectWaitForTermination, nil, true)

		// Then the delete succeeds: neither entry ever blocked it
		if err != nil {
			t.Errorf("Expected residue to let the delete succeed, got: %v", err)
		}
	})

	t.Run("DisappearedGraceWindowScalesWithInventorySize", func(t *testing.T) {
		// Given a still-live entry that settles after more polls than the fixed
		// abandonedInventoryGraceChecks floor allows, but within the window the
		// 2-entry last-known inventory extends past that floor
		manager := setup(t)
		manager.kustomizationDeletionPerEntryTimeout = 100 * time.Millisecond
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		calls, entryCalls := 0, 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if name == "test-kustomization" {
				calls++
				if calls <= 2 {
					return waitForTerminationKustomization(), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			if name == "entry-one" {
				entryCalls++
				if entryCalls <= 5 {
					return blockingObject(), nil
				}
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		// When DeleteKustomization sees the object disappear
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it still recovers, because the inventory size extended the grace
		// window past the abandonedInventoryGraceChecks floor
		if err != nil {
			t.Errorf("Expected no error once the entry settles within the scaled grace window, got %v", err)
		}
		if entryCalls <= abandonedInventoryGraceChecks {
			t.Fatalf("Test invalid: entry settled within the old fixed floor (%d calls), does not exercise scaling", entryCalls)
		}
	})

	t.Run("DisappearedIsCleanWhenPolicyIsMirrorPrune", func(t *testing.T) {
		// Given a MirrorPrune kustomization (destroy:false) whose last-known inventory
		// entry is still live — MirrorPrune deletes resources without waiting for them,
		// so a live entry here is expected, not a sign of a stuck finalizer
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		calls := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if name == "test-kustomization" {
				calls++
				if calls <= 2 {
					return drainedKustomization(), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			if name == "entry-one" {
				return blockingObject(), nil
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		// When DeleteKustomization sees the object disappear
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it reports success rather than a false abandoned-inventory error
		if err != nil {
			t.Errorf("Expected no error for a MirrorPrune delete, got %v", err)
		}
	})

	t.Run("DisappearedIsCleanWhenLastKnownInventoryIsDrained", func(t *testing.T) {
		// Given a kustomization that disappears after one tick, and every last-known
		// inventory entry is confirmed gone — a genuinely clean delete
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		calls := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if name == "test-kustomization" {
				calls++
				if calls <= 2 {
					return waitForTerminationKustomization(), nil
				}
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		// When DeleteKustomization sees the object disappear
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it reports success
		if err != nil {
			t.Errorf("Expected no error for a drained delete, got %v", err)
		}
	})

	t.Run("DisappearedFailsWhenInventoryCheckInconclusive", func(t *testing.T) {
		// Given a kustomization that disappears after one tick, but the live-check on
		// its last-known inventory entry fails with a real API error — nothing may be
		// asserted from an inconclusive lookup, least of all that the resources are gone
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		calls := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if name == "test-kustomization" {
				calls++
				if calls <= 2 {
					return waitForTerminationKustomization(), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return nil, fmt.Errorf("connection refused")
		}
		manager.client = kubernetesClient

		// When DeleteKustomization sees the object disappear
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then the destroy stops rather than reporting a delete it could not confirm
		if err == nil {
			t.Fatal("Expected an inconclusive check to fail, got nil")
		}
		if !strings.Contains(err.Error(), "could not confirm its resources are gone") {
			t.Errorf("Expected an unconfirmed-delete error, got: %v", err)
		}
	})

	t.Run("DisappearedIsCleanWhenNoInventoryWasEverReported", func(t *testing.T) {
		// Given a kustomization that disappears on the very first check — no
		// last-known object was ever captured
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		// When DeleteKustomization sees the object disappear
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it reports success
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// TestBaseKubernetesManager_DeleteKustomizationOutsideADestroy covers PruneBlueprint's call
// path: the public DeleteKustomization, run while the cluster stays up. There a finalizer-free
// object still blocks, unlike during an actual destroy.
func TestBaseKubernetesManager_DeleteKustomizationOutsideADestroy(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		manager.kustomizationWaitPollInterval = 5 * time.Millisecond
		manager.kustomizationReconcileTimeout = 40 * time.Millisecond
		manager.kustomizationDeletionPerEntryTimeout = 0
		manager.kustomizationDeletionMaxExtraTimeout = 0
		manager.kustomizationAbandonedGraceMaxExtra = 0
		clock := newFakeClock()
		manager.shims.TimeNow = clock.Now
		manager.shims.TimeSleep = clock.Sleep
		return manager
	}

	t.Run("StillBlocksOnALiveObjectWithNoFinalizer", func(t *testing.T) {
		// Given an orphaned kustomization whose inventory names a live,
		// finalizer-free object
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		reads := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			switch {
			case gvr.Resource == "kustomizations":
				reads++
				if reads <= 2 {
					return &unstructured.Unstructured{Object: map[string]any{
						"spec": map[string]any{"deletionPolicy": "WaitForTermination"},
						"status": map[string]any{"inventory": map[string]any{"entries": []any{
							map[string]any{"id": "test-namespace_leftover_example.com_Widget", "v": "v1"},
						}}},
					}}, nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			default:
				// The leftover object is live. Nothing holds a finalizer on it.
				return &unstructured.Unstructured{}, nil
			}
		}
		manager.client = kubernetesClient

		// When DeleteKustomization runs outside a destroy, the way PruneBlueprint calls it
		err := manager.DeleteKustomization(context.Background(), "orphaned-kustomization", "test-namespace")

		// Then the finalizer-free object still blocks. The cluster keeps running, so
		// nothing else will remove it.
		if err == nil {
			t.Fatal("Expected a live object with no finalizer to still block outside a destroy, got nil")
		}
		if !strings.Contains(err.Error(), "Widget/leftover") {
			t.Errorf("Expected the leftover object to be named, got: %v", err)
		}
	})
}

func TestBaseKubernetesManager_DeleteKustomizationHelmUninstallTimeout(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		manager.kustomizationWaitPollInterval = 5 * time.Millisecond
		manager.kustomizationReconcileTimeout = 20 * time.Millisecond
		manager.kustomizationDeletionPerEntryTimeout = 0
		manager.kustomizationDeletionMaxExtraTimeout = 0
		clock := newFakeClock()
		manager.shims.TimeNow = clock.Now
		manager.shims.TimeSleep = clock.Sleep
		return manager
	}

	kustomizationWithHelmRelease := &unstructured.Unstructured{Object: map[string]any{
		"status": map[string]any{"inventory": map[string]any{"entries": []any{
			map[string]any{"id": "system-gitops_cert-manager_helm.toolkit.fluxcd.io_HelmRelease", "v": "v2"},
		}}},
	}}

	helmRelease := func(specTimeout, uninstallTimeout string) *unstructured.Unstructured {
		spec := map[string]any{}
		if specTimeout != "" {
			spec["timeout"] = specTimeout
		}
		if uninstallTimeout != "" {
			spec["uninstall"] = map[string]any{"timeout": uninstallTimeout}
		}
		return &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "helm.toolkit.fluxcd.io/v2",
			"kind":       "HelmRelease",
			"metadata": map[string]any{
				"name": "cert-manager", "namespace": "system-gitops",
				"finalizers": []any{"finalizers.fluxcd.io"},
			},
			"spec": spec,
		}}
	}

	// resolvedBudget runs a delete that never completes and returns the wait budget the
	// timeout error reports, which is the value the extension logic resolved.
	resolvedBudget := func(t *testing.T, manager *BaseKubernetesManager, hr *unstructured.Unstructured, kustomization *unstructured.Unstructured) string {
		t.Helper()
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "helmreleases" {
				if hr == nil {
					return nil, fmt.Errorf("etcdserver: request timed out")
				}
				return hr, nil
			}
			return kustomization, nil
		}
		manager.client = kubernetesClient

		err := manager.DeleteKustomization(context.Background(), "pki-install", "system-gitops")
		if err == nil {
			t.Fatal("Expected the never-deleted kustomization to time out, got nil")
		}
		match := regexp.MustCompile(`timed out after (\S+) waiting`).FindStringSubmatch(err.Error())
		if match == nil {
			t.Fatalf("Expected a timeout error naming its budget, got: %v", err)
		}
		return match[1]
	}

	t.Run("AdoptsUninstallTimeout", func(t *testing.T) {
		// Given a HelmRelease declaring an uninstall budget longer than the
		// kustomization's own reconcile timeout
		manager := setup(t)

		// When the delete waits
		budget := resolvedBudget(t, manager, helmRelease("", "45m"), kustomizationWithHelmRelease)

		// Then the barrier honors the chart's declared teardown budget
		if budget != "45m0s" {
			t.Errorf("Expected the wait to adopt spec.uninstall.timeout (45m0s), got %s", budget)
		}
	})

	t.Run("FallsBackToHelmReleaseSpecTimeout", func(t *testing.T) {
		// Given a HelmRelease that declares only spec.timeout, which is how flux
		// itself resolves an unset uninstall timeout
		manager := setup(t)

		// When the delete waits
		budget := resolvedBudget(t, manager, helmRelease("45m", ""), kustomizationWithHelmRelease)

		// Then that value stands in for the uninstall budget
		if budget != "45m0s" {
			t.Errorf("Expected the wait to fall back to the HelmRelease spec.timeout (45m0s), got %s", budget)
		}
	})

	t.Run("PrefersUninstallTimeoutOverSpecTimeout", func(t *testing.T) {
		// Given a HelmRelease declaring both, where the uninstall budget is the
		// shorter and more specific of the two
		manager := setup(t)

		// When the delete waits
		budget := resolvedBudget(t, manager, helmRelease("45m", "25m"), kustomizationWithHelmRelease)

		// Then the uninstall budget wins rather than the larger install one
		if budget != "25m0s" {
			t.Errorf("Expected spec.uninstall.timeout (25m0s) to win, got %s", budget)
		}
	})

	t.Run("TakesTheLongestAcrossHelmReleases", func(t *testing.T) {
		// Given a kustomization wrapping two HelmReleases, since the barrier clears
		// only when the slowest chart does
		manager := setup(t)
		twoReleases := &unstructured.Unstructured{Object: map[string]any{
			"status": map[string]any{"inventory": map[string]any{"entries": []any{
				map[string]any{"id": "system-gitops_cert-manager_helm.toolkit.fluxcd.io_HelmRelease", "v": "v2"},
				map[string]any{"id": "system-gitops_trust-manager_helm.toolkit.fluxcd.io_HelmRelease", "v": "v2"},
			}}},
		}}

		// When the delete waits, with both releases answering the same 40m budget
		budget := resolvedBudget(t, manager, helmRelease("", "40m"), twoReleases)

		// Then the longest declared budget governs
		if budget != "40m0s" {
			t.Errorf("Expected the longest declared budget (40m0s), got %s", budget)
		}
	})

	t.Run("IgnoresHelmReleaseDeclaringNoTimeout", func(t *testing.T) {
		// Given a HelmRelease that declares neither timeout
		manager := setup(t)

		// When the delete waits
		budget := resolvedBudget(t, manager, helmRelease("", ""), kustomizationWithHelmRelease)

		// Then the wait keeps its existing budget rather than inventing one
		if budget != "20ms" {
			t.Errorf("Expected the unextended budget (20ms), got %s", budget)
		}
	})

	t.Run("SurvivesUnreadableHelmRelease", func(t *testing.T) {
		// Given a HelmRelease entry whose object cannot be fetched
		manager := setup(t)

		// When the delete waits
		budget := resolvedBudget(t, manager, nil, kustomizationWithHelmRelease)

		// Then the lookup failure neither extends nor breaks the wait
		if budget != "20ms" {
			t.Errorf("Expected the unextended budget (20ms), got %s", budget)
		}
	})

	t.Run("ExplicitDeleteTimeoutBoundsTheChartBudget", func(t *testing.T) {
		// Given a blueprint that sets DeleteTimeout to bound this delete, on a
		// kustomization wrapping a chart declaring far longer
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "helmreleases" {
				return helmRelease("", "45m"), nil
			}
			return kustomizationWithHelmRelease, nil
		}
		manager.client = kubernetesClient
		override := 10 * time.Minute

		// When the delete waits
		err := manager.deleteKustomization(context.Background(), "pki-install", "system-gitops", nil, &override, true)

		// Then the operator's explicit bound wins over the chart's declaration
		if err == nil {
			t.Fatal("Expected the never-deleted kustomization to time out, got nil")
		}
		if !strings.Contains(err.Error(), "timed out after 10m0s") {
			t.Errorf("Expected DeleteTimeout (10m0s) to bound the wait, got: %v", err)
		}
	})
}

func TestBaseKubernetesManager_DeleteKustomizationReadFailures(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		manager.kustomizationWaitPollInterval = 10 * time.Millisecond
		manager.kustomizationReconcileTimeout = 500 * time.Millisecond
		manager.kustomizationDeletionPerEntryTimeout = time.Millisecond
		return manager
	}

	t.Run("ToleratesTransientReadFailure", func(t *testing.T) {
		// Given a delete whose status reads fail twice with a transient error, then
		// succeed across two still-live polls, before the kustomization goes away
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		reads := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			reads++
			if reads <= 2 {
				return nil, fmt.Errorf("etcdserver: request timed out")
			}
			if reads <= 4 {
				return blockingObject(), nil
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient
		ctx, rec := recordProgress(t)

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(ctx, "demo-resources", "system-gitops")

		// Then the blip does not abort a delete flux went on to complete
		if err != nil {
			t.Errorf("Expected transient read failures to be tolerated, got error: %v", err)
		}
		if reads < 4 {
			t.Errorf("Expected the wait to poll past the failures, got %d reads", reads)
		}
		// And an operator watching the run sees the retry, not silence for the whole poll
		sawRetry := false
		for _, u := range rec.progress() {
			if strings.Contains(u, "retrying after a read error") {
				sawRetry = true
			}
		}
		if !sawRetry {
			t.Errorf("Expected a progress update naming the read-error retry, got: %v", rec.progress())
		}
	})

	t.Run("FailsWhenKustomizationDisappearsBeforeAnySuccessfulRead", func(t *testing.T) {
		// Given reads that fail transiently until the kustomization vanishes, so
		// windsor never read the inventory it would verify against
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		reads := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			reads++
			if reads <= 2 {
				return nil, fmt.Errorf("etcdserver: request timed out")
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "demo-resources", "system-gitops")

		// Then an unverifiable disappearance fails rather than reporting a clean delete
		if err == nil {
			t.Fatal("Expected an unverified disappearance to fail, got nil")
		}
		if !strings.Contains(err.Error(), "before windsor could read its inventory") {
			t.Errorf("Expected an unread-inventory error, got: %v", err)
		}
	})

	t.Run("FailsAfterMaxConsecutiveReadFailures", func(t *testing.T) {
		// Given status reads that keep failing past the tolerance
		manager := setup(t)
		manager.kustomizationWaitMaxReadFailures = 2
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		reads := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			reads++
			return nil, fmt.Errorf("etcdserver: request timed out")
		}
		manager.client = kubernetesClient

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "demo-resources", "system-gitops")

		// Then it absorbs exactly the configured tolerance before surfacing
		if err == nil {
			t.Fatal("Expected persistent read failures to surface, got nil")
		}
		if !strings.Contains(err.Error(), "deletion status") {
			t.Errorf("Expected a deletion-status error, got: %v", err)
		}
		if reads != 4 {
			t.Errorf("Expected 4 reads (the pre-delete snapshot plus 3 for a tolerance of 2), got %d", reads)
		}
	})

	t.Run("ResetsToleranceAfterASuccessfulRead", func(t *testing.T) {
		// Given reads that alternate failure and success, never exceeding the
		// tolerance consecutively, before the kustomization goes away
		manager := setup(t)
		manager.kustomizationWaitMaxReadFailures = 1
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		reads := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			reads++
			if reads >= 7 {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			if reads%2 == 1 {
				return nil, fmt.Errorf("etcdserver: request timed out")
			}
			return blockingObject(), nil
		}
		manager.client = kubernetesClient

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "demo-resources", "system-gitops")

		// Then a successful read clears the count rather than accumulating
		if err != nil {
			t.Errorf("Expected alternating failures to be tolerated, got error: %v", err)
		}
	})
}

func TestBaseKubernetesManager_DeleteKustomizationVerifiesChartResources(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		manager.kustomizationWaitPollInterval = 5 * time.Millisecond
		manager.kustomizationReconcileTimeout = 40 * time.Millisecond
		manager.kustomizationDeletionPerEntryTimeout = 0
		manager.kustomizationDeletionMaxExtraTimeout = 0
		manager.kustomizationAbandonedGraceMaxExtra = 0
		clock := newFakeClock()
		manager.shims.TimeNow = clock.Now
		manager.shims.TimeSleep = clock.Sleep
		return manager
	}

	// A kustomization whose whole inventory is one HelmRelease, as 27 of core's 31
	// install tiers are built.
	kustomizationWrappingChart := func() *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{
			"spec": map[string]any{"deletionPolicy": "WaitForTermination"},
			"status": map[string]any{"inventory": map[string]any{"entries": []any{
				map[string]any{"id": "system-pki_cert-manager_helm.toolkit.fluxcd.io_HelmRelease", "v": "v2"},
			}}},
		}}
	}

	helmReleaseManaging := func(ids ...string) *unstructured.Unstructured {
		raw := make([]any, 0, len(ids))
		for _, id := range ids {
			raw = append(raw, map[string]any{"id": id, "v": "v1"})
		}
		return &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "helm.toolkit.fluxcd.io/v2",
			"kind":       "HelmRelease",
			"metadata": map[string]any{
				"name": "cert-manager", "namespace": "system-pki",
				"finalizers": []any{"finalizers.fluxcd.io"},
			},
			"status": map[string]any{"inventory": map[string]any{"entries": raw}},
		}}
	}

	const chartWebhook = "system-pki_cert-manager-webhook__dynamic-serving_rbac.authorization.k8s.io_Role"

	t.Run("ReportsAChartResourceLeftBehindByAVanishedHelmRelease", func(t *testing.T) {
		// Given a kustomization that disappears along with its HelmRelease, while a
		// resource the chart managed is still live
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		reads := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			switch {
			case gvr.Resource == "kustomizations":
				reads++
				if reads <= 2 {
					return kustomizationWrappingChart(), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			case gvr.Resource == "helmreleases":
				if reads <= 1 {
					return helmReleaseManaging(chartWebhook), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			default:
				return blockingObject(), nil
			}
		}
		manager.client = kubernetesClient

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "cert-manager", "system-pki")

		// Then the leftover is named, rather than the wrapper's absence being trusted
		if err == nil {
			t.Fatal("Expected a live chart resource to fail the delete, got nil")
		}
		if !strings.Contains(err.Error(), "cert-manager-webhook:dynamic-serving") {
			t.Errorf("Expected the leftover chart resource to be named, got: %v", err)
		}
	})

	t.Run("AcceptsADeleteWhoseChartResourcesAreAllGone", func(t *testing.T) {
		// Given the same teardown, with every chart resource confirmed absent
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		reads := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			switch {
			case gvr.Resource == "kustomizations":
				reads++
				if reads <= 2 {
					return kustomizationWrappingChart(), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			case gvr.Resource == "helmreleases":
				if reads <= 1 {
					return helmReleaseManaging(chartWebhook), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			default:
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
		}
		manager.client = kubernetesClient

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "cert-manager", "system-pki")

		// Then the delete is clean
		if err != nil {
			t.Errorf("Expected a fully drained chart to pass, got: %v", err)
		}
	})

	t.Run("FailsWhenAHelmReleaseWasNeverReadable", func(t *testing.T) {
		// Given a HelmRelease whose reads fail with a transient API error, so nothing
		// is ever learned about what its chart managed
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		reads := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			switch {
			case gvr.Resource == "kustomizations":
				reads++
				if reads <= 2 {
					return kustomizationWrappingChart(), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			case gvr.Resource == "helmreleases" && reads <= 1:
				return nil, fmt.Errorf("etcdserver: request timed out")
			default:
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
		}
		manager.client = kubernetesClient

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "cert-manager", "system-pki")

		// Then an unread chart inventory is not treated as an empty one
		if err == nil {
			t.Fatal("Expected an unverifiable HelmRelease to fail the delete, got nil")
		}
		if !strings.Contains(err.Error(), "never read what helmrelease system-pki/cert-manager managed") {
			t.Errorf("Expected an unread-chart error naming the release, got: %v", err)
		}
	})

	t.Run("FailsWhenAHelmReleaseWasAlreadyGoneBeforeTheFirstPoll", func(t *testing.T) {
		// Given a HelmRelease already absent on the first poll, which is what a rerun
		// sees after helm-controller abandoned an uninstall and left resources behind
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		reads := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				reads++
				if reads <= 2 {
					return kustomizationWrappingChart(), nil
				}
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "cert-manager", "system-pki")

		// Then an unobserved chart is not credited as a clean teardown
		if err == nil {
			t.Fatal("Expected an unobserved HelmRelease to fail the delete, got nil")
		}
		if !strings.Contains(err.Error(), "never read what helmrelease system-pki/cert-manager managed") {
			t.Errorf("Expected an unread-chart error naming the release, got: %v", err)
		}
	})

	t.Run("AcceptsAHelmReleaseThatPublishesNoInventory", func(t *testing.T) {
		// Given a HelmRelease observed live on a flux that never reports what a chart
		// owns, where the deeper answer is simply unavailable
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		reads := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			switch {
			case gvr.Resource == "kustomizations":
				reads++
				if reads <= 2 {
					return kustomizationWrappingChart(), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			case gvr.Resource == "helmreleases" && reads <= 1:
				return &unstructured.Unstructured{Object: map[string]any{
					"apiVersion": "helm.toolkit.fluxcd.io/v2",
					"kind":       "HelmRelease",
					"metadata":   map[string]any{"name": "cert-manager", "namespace": "system-pki"},
				}}, nil
			default:
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
		}
		manager.client = kubernetesClient

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "cert-manager", "system-pki")

		// Then it falls back to the older answer rather than failing every destroy
		if err != nil {
			t.Errorf("Expected a flux without chart inventory to degrade, got: %v", err)
		}
	})

	t.Run("FailsWhenAChartInventoryOnlyPartlyDecodes", func(t *testing.T) {
		// Given a HelmRelease whose reported inventory holds an entry windsor cannot
		// decode, so the list it can read is not the whole chart
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		reads := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			switch {
			case gvr.Resource == "kustomizations":
				reads++
				if reads <= 2 {
					return kustomizationWrappingChart(), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			case gvr.Resource == "helmreleases" && reads <= 1:
				return helmReleaseManaging("malformed-id"), nil
			default:
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
		}
		manager.client = kubernetesClient

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "cert-manager", "system-pki")

		// Then a partial reading is not verified as if it were complete
		if err == nil {
			t.Fatal("Expected a partly-decoded chart inventory to fail the delete, got nil")
		}
		if !strings.Contains(err.Error(), "could not fully decode") {
			t.Errorf("Expected a partial-inventory error, got: %v", err)
		}
	})

	t.Run("ReportsALiveNestedHelmRelease", func(t *testing.T) {
		// Given a chart that itself deploys a HelmRelease, which is still live after
		// the outer release vanished
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		reads := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			switch {
			case gvr.Resource == "kustomizations":
				reads++
				if reads <= 2 {
					return kustomizationWrappingChart(), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			case gvr.Resource == "helmreleases" && name == "cert-manager":
				if reads <= 1 {
					return helmReleaseManaging("system-pki_inner_helm.toolkit.fluxcd.io_HelmRelease"), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			default:
				return blockingObject(), nil
			}
		}
		manager.client = kubernetesClient

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "cert-manager", "system-pki")

		// Then the nested release is reported rather than skipped as gone
		if err == nil {
			t.Fatal("Expected a live nested HelmRelease to fail the delete, got nil")
		}
		if !strings.Contains(err.Error(), "HelmRelease/inner") {
			t.Errorf("Expected the nested release to be named, got: %v", err)
		}
	})

	t.Run("AcceptsAHelmReleaseThatDeletesBeforeTheFirstPoll", func(t *testing.T) {
		// Given a HelmRelease that helm-controller tears down as soon as delete is
		// requested, faster than one poll interval, but still alive when windsor
		// reads it immediately beforehand
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		deleted := false
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleted = true
			return nil
		}
		reads := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			switch {
			case gvr.Resource == "kustomizations":
				reads++
				if reads <= 2 {
					return kustomizationWrappingChart(), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			case gvr.Resource == "helmreleases":
				if !deleted {
					return helmReleaseManaging(chartWebhook), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			default:
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
		}
		manager.client = kubernetesClient

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "cert-manager", "system-pki")

		// Then the pre-delete snapshot still verifies the chart's resources are gone
		if err != nil {
			t.Errorf("Expected a fast-deleting chart captured before delete to pass, got: %v", err)
		}
	})

	t.Run("WillNotClaimDrainedWhenAnEntryCannotBeDecoded", func(t *testing.T) {
		// Given a kustomization that never disappears, holding an inventory entry
		// windsor cannot decode
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				return &unstructured.Unstructured{Object: map[string]any{
					"spec": map[string]any{"deletionPolicy": "WaitForTermination"},
					"status": map[string]any{"inventory": map[string]any{"entries": []any{
						map[string]any{"id": "malformed-id", "v": "v1"},
					}}},
				}}, nil
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		// When the delete times out
		err := manager.DeleteKustomization(context.Background(), "cert-manager", "system-pki")

		// Then an undecodable inventory is not reported as a confirmed-empty one
		if err == nil {
			t.Fatal("Expected the delete to time out, got nil")
		}
		if strings.Contains(err.Error(), "fully drained") {
			t.Errorf("Expected no drained claim over an undecodable inventory, got: %v", err)
		}
	})

	t.Run("RemembersChartResourcesAfterTheInventoryShrinks", func(t *testing.T) {
		// Given a HelmRelease whose reported inventory empties as its uninstall runs,
		// while one of the resources it listed earlier is still live
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		reads := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			switch {
			case gvr.Resource == "kustomizations":
				reads++
				if reads <= 2 {
					return kustomizationWrappingChart(), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			case gvr.Resource == "helmreleases":
				if reads == 1 {
					return helmReleaseManaging(chartWebhook), nil
				}
				if reads == 2 {
					return helmReleaseManaging(), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			default:
				return blockingObject(), nil
			}
		}
		manager.client = kubernetesClient

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "cert-manager", "system-pki")

		// Then the earlier reading still governs, rather than the empty later one
		if err == nil {
			t.Fatal("Expected the remembered chart resource to fail the delete, got nil")
		}
		if !strings.Contains(err.Error(), "cert-manager-webhook:dynamic-serving") {
			t.Errorf("Expected the remembered chart resource to be named, got: %v", err)
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestBaseKubernetesManager_deleteKustomization_DeleteTimeoutOverride(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		manager.kustomizationReconcileTimeout = 20 * time.Millisecond
		manager.kustomizationWaitPollInterval = 10 * time.Millisecond
		clock := newFakeClock()
		manager.shims.TimeNow = clock.Now
		manager.shims.TimeSleep = clock.Sleep
		return manager
	}

	t.Run("OverrideRaisesWaitPastSmallerSpecTimeout", func(t *testing.T) {
		// Given a DeleteTimeout override larger than the live spec.timeout (a
		// blueprint declaring that delete legitimately outlasts install)
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{
				"spec": map[string]any{"timeout": "30ms"},
			}}, nil
		}
		manager.client = kubernetesClient
		override := 150 * time.Millisecond

		// When deleteKustomization times out
		start := manager.shims.TimeNow()
		err := manager.deleteKustomization(context.Background(), "test-kustomization", "test-namespace", nil, &override, true)
		elapsed := manager.shims.TimeNow().Sub(start)

		// Then it waits out the override instead of the smaller spec.timeout
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if elapsed < 150*time.Millisecond {
			t.Errorf("Expected wait window raised to the override (>=150ms), got %s", elapsed)
		}
	})

	t.Run("OverrideNotClampedByInstallCeiling", func(t *testing.T) {
		// Given an install-oriented ceiling too small for the override (a delete
		// budget is allowed to exceed what a sane install timeout would be)
		manager := setup(t)
		manager.kustomizationSpecTimeoutCeiling = 50 * time.Millisecond
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		}
		manager.client = kubernetesClient
		override := 150 * time.Millisecond

		// When deleteKustomization times out
		start := manager.shims.TimeNow()
		err := manager.deleteKustomization(context.Background(), "test-kustomization", "test-namespace", nil, &override, true)
		elapsed := manager.shims.TimeNow().Sub(start)

		// Then the wait reaches the override, unaffected by the smaller install ceiling
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if elapsed < 150*time.Millisecond {
			t.Errorf("Expected wait window to reach the override (>=150ms), got %s", elapsed)
		}
	})

	t.Run("OverrideCapsAtCeiling", func(t *testing.T) {
		// Given an override far larger than the configured ceiling
		manager := setup(t)
		manager.kustomizationDeleteTimeoutCeiling = 100 * time.Millisecond
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		}
		manager.client = kubernetesClient
		override := 876000 * time.Hour

		// When deleteKustomization times out
		start := manager.shims.TimeNow()
		err := manager.deleteKustomization(context.Background(), "test-kustomization", "test-namespace", nil, &override, true)
		elapsed := manager.shims.TimeNow().Sub(start)

		// Then the wait is bounded by the ceiling, not the declared override
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if elapsed < 100*time.Millisecond || elapsed > 250*time.Millisecond {
			t.Errorf("Expected wait window capped near the ceiling (~100ms), got %s", elapsed)
		}
	})

	t.Run("NoOverrideFallsBackToSpecTimeout", func(t *testing.T) {
		// Given no override and a live spec.timeout larger than the base window
		// (today's behavior, unaffected by this blueprint remaining unset)
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{
				"spec": map[string]any{"timeout": "150ms"},
			}}, nil
		}
		manager.client = kubernetesClient

		// When deleteKustomization times out with no override
		start := manager.shims.TimeNow()
		err := manager.deleteKustomization(context.Background(), "test-kustomization", "test-namespace", nil, nil, true)
		elapsed := manager.shims.TimeNow().Sub(start)

		// Then it still waits out spec.timeout, same as before this change
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if elapsed < 150*time.Millisecond {
			t.Errorf("Expected wait window raised to spec.timeout (>=150ms), got %s", elapsed)
		}
	})
}

func TestBaseKubernetesManager_triggerReconcile(t *testing.T) {
	t.Run("UsesFluxAnnotationForAKustomization", func(t *testing.T) {
		mocks := setupKubernetesMocks(t)
		kubernetesClient := client.NewMockKubernetesClient()
		var gotGVR schema.GroupVersionResource
		var gotName string
		var gotPatch []byte
		kubernetesClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			gotGVR, gotName, gotPatch = gvr, name, data
			return nil, nil
		}
		manager := NewKubernetesManager(kubernetesClient, mocks.ConfigHandler)
		entry := &liveInventoryEntry{
			InventoryEntry: InventoryEntry{Namespace: "system-gitops", Name: "crds", Group: "kustomize.toolkit.fluxcd.io", Kind: "Kustomization"},
			gvr:            schema.GroupVersionResource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"},
		}

		// Given a blocking entry that is a Kustomization
		// When triggerReconcile runs
		manager.triggerReconcile(entry)

		// Then it patches Flux's own documented reconcile-trigger annotation
		if gotName != "crds" || gotGVR.Resource != "kustomizations" {
			t.Fatalf("expected a patch against the Kustomization, got %q %v", gotName, gotGVR)
		}
		if !strings.Contains(string(gotPatch), fluxReconcileAnnotation) {
			t.Errorf("expected the flux reconcile annotation in the patch, got: %s", gotPatch)
		}
	})

	t.Run("UsesWindsorAnnotationForANonFluxObject", func(t *testing.T) {
		mocks := setupKubernetesMocks(t)
		kubernetesClient := client.NewMockKubernetesClient()
		var gotPatch []byte
		kubernetesClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			gotPatch = data
			return nil, nil
		}
		manager := NewKubernetesManager(kubernetesClient, mocks.ConfigHandler)
		entry := &liveInventoryEntry{
			InventoryEntry: InventoryEntry{Namespace: "demo-database", Name: "provider-sql-demo-db", Group: "postgresql.sql.m.crossplane.io", Kind: "ProviderConfig"},
			gvr:            schema.GroupVersionResource{Group: "postgresql.sql.m.crossplane.io", Version: "v1alpha1", Resource: "providerconfigs"},
		}

		// Given a blocking entry outside Flux's own kinds
		// When triggerReconcile runs
		manager.triggerReconcile(entry)

		// Then it patches windsor's own annotation, not Flux's
		if !strings.Contains(string(gotPatch), windsorReconcileAnnotation) {
			t.Errorf("expected the windsor reconcile annotation in the patch, got: %s", gotPatch)
		}
		if strings.Contains(string(gotPatch), fluxReconcileAnnotation) {
			t.Errorf("expected no flux annotation on a non-flux object, got: %s", gotPatch)
		}
	})

	t.Run("NeverTouchesFinalizers", func(t *testing.T) {
		mocks := setupKubernetesMocks(t)
		kubernetesClient := client.NewMockKubernetesClient()
		var gotPatch []byte
		kubernetesClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			gotPatch = data
			return nil, nil
		}
		manager := NewKubernetesManager(kubernetesClient, mocks.ConfigHandler)
		entry := &liveInventoryEntry{
			InventoryEntry: InventoryEntry{Namespace: "demo-database", Name: "provider-sql-demo-db", Group: "postgresql.sql.m.crossplane.io", Kind: "ProviderConfig"},
			gvr:            schema.GroupVersionResource{Group: "postgresql.sql.m.crossplane.io", Version: "v1alpha1", Resource: "providerconfigs"},
		}

		// Given any blocking entry
		// When triggerReconcile runs
		manager.triggerReconcile(entry)

		// Then the patch never names finalizers
		if strings.Contains(string(gotPatch), "finalizers") {
			t.Errorf("expected the patch to never touch finalizers, got: %s", gotPatch)
		}
	})

	t.Run("IgnoresAPatchFailure", func(t *testing.T) {
		mocks := setupKubernetesMocks(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("forbidden")
		}
		manager := NewKubernetesManager(kubernetesClient, mocks.ConfigHandler)
		entry := &liveInventoryEntry{
			InventoryEntry: InventoryEntry{Namespace: "demo-database", Name: "provider-sql-demo-db", Group: "postgresql.sql.m.crossplane.io", Kind: "ProviderConfig"},
			gvr:            schema.GroupVersionResource{Group: "postgresql.sql.m.crossplane.io", Version: "v1alpha1", Resource: "providerconfigs"},
		}

		// Given a patch that fails
		// When triggerReconcile runs
		// Then it does not panic or otherwise surface the failure
		manager.triggerReconcile(entry)
	})
}
