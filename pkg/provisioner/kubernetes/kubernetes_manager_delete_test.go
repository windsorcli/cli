package kubernetes_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
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
	setup := func(t *testing.T, kubeClient client.KubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := kubernetes.NewKubernetesManager(kubeClient, mocks.ConfigHandler)
		manager.Timing.KustomizationWaitPollInterval = 50 * time.Millisecond
		manager.Timing.KustomizationReconcileTimeout = 100 * time.Millisecond
		manager.Timing.KustomizationReconcileSleep = 50 * time.Millisecond
		clock := newFakeClock()
		manager.Shims.TimeNow = clock.Now
		manager.Shims.TimeSleep = clock.Sleep
		return manager
	}

	t.Run("SucceedsWhenKustomizationIsGone", func(t *testing.T) {
		// Given a kustomization that is gone right after the delete
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it returns no error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("DeleteError", func(t *testing.T) {
		// Given a delete request that fails
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return fmt.Errorf("delete error")
		}
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it returns an error
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("KustomizationNotFound", func(t *testing.T) {
		// Given a delete request that reports NotFound
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return fmt.Errorf("the server could not find the requested resource")
		}
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it treats the missing kustomization as deleted
		if err != nil {
			t.Errorf("Expected no error for not found resource, got %v", err)
		}
	})

	t.Run("TimeoutWaitingForDeletion", func(t *testing.T) {
		// Given a kustomization that never disappears
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationReconcileTimeout = 100 * time.Millisecond
		manager.Timing.KustomizationWaitPollInterval = 50 * time.Millisecond

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it returns a timeout error that names the kustomization
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
		// Given a stuck kustomization whose status condition gives the reason
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
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationReconcileTimeout = 100 * time.Millisecond
		manager.Timing.KustomizationWaitPollInterval = 50 * time.Millisecond

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then the error shows the condition reason and message
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
		// Given a stuck kustomization whose condition message has several lines
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
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationReconcileTimeout = 100 * time.Millisecond
		manager.Timing.KustomizationWaitPollInterval = 50 * time.Millisecond

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then the condition detail stays on one line
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
		// Given a stuck kustomization whose inventory names a stalled HelmRelease
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
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationReconcileTimeout = 100 * time.Millisecond
		manager.Timing.KustomizationWaitPollInterval = 50 * time.Millisecond

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "database-install", "system-gitops")

		// Then the error shows the stuck reason of the HelmRelease
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
		// Given a stuck kustomization whose HelmRelease lookup fails
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		calls := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			calls++
			if calls == 1 {
				return &unstructured.Unstructured{}, nil
			}
			return nil, fmt.Errorf("apiserver unreachable")
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationReconcileTimeout = 10 * time.Millisecond
		manager.Timing.KustomizationWaitPollInterval = 50 * time.Millisecond

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then the timeout error still shows and the lookup failure does not
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Errorf("Expected error mentioning timeout, got: %v", err)
		}
	})

	t.Run("ErrorCheckingDeletionStatus", func(t *testing.T) {
		// Given status reads that fail and no read-failure tolerance
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("error checking status")
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationReconcileTimeout = 100 * time.Millisecond
		manager.Timing.KustomizationWaitPollInterval = 50 * time.Millisecond
		manager.Timing.KustomizationWaitMaxReadFailures = 0

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it returns a deletion-status error
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error checking kustomization test-namespace/test-kustomization deletion status") {
			t.Errorf("Expected error checking status, got: %v", err)
		}
	})

	t.Run("UsesCorrectDeleteOptions", func(t *testing.T) {
		// Given a client that records the delete options
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		var capturedOptions metav1.DeleteOptions
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			capturedOptions = opts
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it deletes with background propagation
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if capturedOptions.PropagationPolicy == nil {
			t.Error("Expected PropagationPolicy to be set")
		} else if *capturedOptions.PropagationPolicy != metav1.DeletePropagationBackground {
			t.Errorf("Expected PropagationPolicy to be DeletePropagationBackground, got %s", *capturedOptions.PropagationPolicy)
		}
	})

	t.Run("TimeoutWithoutConfirmedReasonIsHonestAboutUncertainty", func(t *testing.T) {
		// Given a stuck kustomization with no diagnostic condition
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationReconcileTimeout = 100 * time.Millisecond
		manager.Timing.KustomizationWaitPollInterval = 50 * time.Millisecond

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then the error does not claim a stuck finalizer it cannot confirm
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
		// Given a stuck kustomization with a condition that confirms the stuck state
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
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationReconcileTimeout = 100 * time.Millisecond
		manager.Timing.KustomizationWaitPollInterval = 50 * time.Millisecond

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then the error keeps the stuck-finalizer wording
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
		// Given a stuck MirrorPrune kustomization with no diagnostic condition
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{
				"spec": map[string]any{"deletionPolicy": "MirrorPrune"},
			}}, nil
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationReconcileTimeout = 100 * time.Millisecond
		manager.Timing.KustomizationWaitPollInterval = 50 * time.Millisecond

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then the error names MirrorPrune and does not blame a stuck finalizer
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
		// Given a stuck MirrorPrune kustomization with a suspended-reconcile condition
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
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationReconcileTimeout = 100 * time.Millisecond
		manager.Timing.KustomizationWaitPollInterval = 50 * time.Millisecond

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then the error keeps the condition and drops the stuck-finalizer wording
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
		// Given a stuck kustomization with a 100-entry inventory
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
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationReconcileTimeout = 20 * time.Millisecond
		manager.Timing.KustomizationWaitPollInterval = 10 * time.Millisecond
		manager.Timing.KustomizationDeletionPerEntryTimeout = 1 * time.Millisecond
		manager.Timing.KustomizationDeletionMaxExtraTimeout = time.Second

		// When DeleteKustomization times out
		start := manager.Shims.TimeNow()
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")
		elapsed := manager.Shims.TimeNow().Sub(start)

		// Then the wait grows by 1ms per entry past the 20ms base window
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if elapsed < 100*time.Millisecond {
			t.Errorf("Expected wait window scaled by inventory size (>=100ms), got %s", elapsed)
		}
	})

	t.Run("TimeoutWindowScalesEvenWhenFirstReadHasNoInventory", func(t *testing.T) {
		// Given an inventory that is empty on the first read and has 100 entries later
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
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationReconcileTimeout = 20 * time.Millisecond
		manager.Timing.KustomizationWaitPollInterval = 10 * time.Millisecond
		manager.Timing.KustomizationDeletionPerEntryTimeout = 1 * time.Millisecond
		manager.Timing.KustomizationDeletionMaxExtraTimeout = time.Second

		// When DeleteKustomization times out
		start := manager.Shims.TimeNow()
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")
		elapsed := manager.Shims.TimeNow().Sub(start)

		// Then a later populated read still scales the wait window
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if elapsed < 100*time.Millisecond {
			t.Errorf("Expected wait window scaled by a later inventory read (>=100ms), got %s", elapsed)
		}
	})

	t.Run("TimeoutWindowRespectsSpecTimeout", func(t *testing.T) {
		// Given a spec.timeout of 150ms, larger than the 20ms base window
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{
				"spec": map[string]any{"timeout": "150ms"},
			}}, nil
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationReconcileTimeout = 20 * time.Millisecond
		manager.Timing.KustomizationWaitPollInterval = 10 * time.Millisecond

		// When DeleteKustomization times out
		start := manager.Shims.TimeNow()
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")
		elapsed := manager.Shims.TimeNow().Sub(start)

		// Then it waits out spec.timeout and stops near it
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
		// Given a spec.timeout far larger than the 100ms ceiling
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{
				"spec": map[string]any{"timeout": "876000h"},
			}}, nil
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationReconcileTimeout = 20 * time.Millisecond
		manager.Timing.KustomizationWaitPollInterval = 10 * time.Millisecond
		manager.Timing.KustomizationSpecTimeoutCeiling = 100 * time.Millisecond

		// When DeleteKustomization times out
		start := manager.Shims.TimeNow()
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")
		elapsed := manager.Shims.TimeNow().Sub(start)

		// Then the ceiling bounds the wait
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if elapsed < 100*time.Millisecond || elapsed > 250*time.Millisecond {
			t.Errorf("Expected wait window capped near the ceiling (~100ms), got %s", elapsed)
		}
	})

	t.Run("TimeoutIgnoresUnparseableSpecTimeout", func(t *testing.T) {
		// Given a malformed spec.timeout
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{
				"spec": map[string]any{"timeout": "not-a-duration"},
			}}, nil
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationReconcileTimeout = 20 * time.Millisecond
		manager.Timing.KustomizationWaitPollInterval = 10 * time.Millisecond

		// When DeleteKustomization times out
		start := manager.Shims.TimeNow()
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")
		elapsed := manager.Shims.TimeNow().Sub(start)

		// Then it keeps the base window
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
		// Given a stuck kustomization whose inventory entries are all gone
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
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it reports the drained state and a manual fix
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "fully drained") || !strings.Contains(err.Error(), "kubectl patch") {
			t.Errorf("Expected drained-and-stuck-finalizer wording naming a fix, got: %v", err)
		}
	})

	t.Run("TimeoutDoesNotClaimDrainedWhenEntryStillLive", func(t *testing.T) {
		// Given a stuck kustomization with one live, blocking inventory entry
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
		manager := setup(t, kubernetesClient)

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
		// Given a stuck kustomization with an entry that stays live and blocking
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
		manager := setup(t, kubernetesClient)
		ctx, rec := recordProgress(t)

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(ctx, "test-kustomization", "test-namespace")

		// Then it asks the blocking entry to reconcile and reports progress before the timeout
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
		// Given a blocking entry and kustomization that both go away during the post-timeout recheck
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
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationDeletionPerEntryTimeout = 0
		manager.Timing.KustomizationDeletionMaxExtraTimeout = 0

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then the recheck sees both go away and the delete succeeds
		if err != nil {
			t.Errorf("Expected the recheck to catch the disappearance and succeed, got: %v", err)
		}
	})

	t.Run("TimeoutWithMirrorPruneSkipsTheRecheckTrigger", func(t *testing.T) {
		// Given a stuck MirrorPrune kustomization with a live, blocking entry
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
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationReconcileTimeout = 20 * time.Millisecond
		manager.Timing.KustomizationWaitPollInterval = 10 * time.Millisecond
		manager.Timing.KustomizationDeletionPerEntryTimeout = 0
		manager.Timing.KustomizationDeletionMaxExtraTimeout = 0

		// When DeleteKustomization times out
		start := manager.Shims.TimeNow()
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")
		elapsed := manager.Shims.TimeNow().Sub(start)

		// Then it reports the timeout with no reconcile trigger and no recheck delay
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

	t.Run("TimeoutFallsBackWhenInventoryCheckInconclusive", func(t *testing.T) {
		// Given an inventory entry whose lookup fails with an API error
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
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it does not claim the kustomization is drained
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if strings.Contains(err.Error(), "fully drained") {
			t.Errorf("Expected fallback wording on an inconclusive check, got: %v", err)
		}
	})

	t.Run("TimeoutTreatsDeletedResourceTypeAsGone", func(t *testing.T) {
		// Given inventory entries whose resource types no longer exist
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
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it reports the kustomization as drained
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "fully drained") {
			t.Errorf("Expected drained wording when the entry's type no longer exists, got: %v", err)
		}
	})

	t.Run("TimeoutIgnoresInventoryNeverReported", func(t *testing.T) {
		// Given a stuck kustomization that never reported an inventory
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return blockingObject(), nil
		}
		manager := setup(t, kubernetesClient)

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
		// Given a WaitForTermination kustomization that disappears while an entry stays live
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
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization sees the kustomization disappear
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it names the live entry and gives an inspect hint
		if err == nil {
			t.Fatal("Expected an error naming the abandoned inventory, got nil")
		}
		if !strings.Contains(err.Error(), "entry-one") {
			t.Errorf("Expected error to name the still-live entry, got: %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "kubectl get") {
			t.Errorf("Expected a hint with the inspect command, got: %v", we)
		}
	})

	t.Run("DisappearedTriggersReconcileOnTheBlockingEntry", func(t *testing.T) {
		// Given a kustomization that disappears while a blocking entry stays live
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
		manager := setup(t, kubernetesClient)
		ctx, rec := recordProgress(t)

		// When DeleteKustomization waits out the grace retries
		_ = manager.DeleteKustomization(ctx, "test-kustomization", "test-namespace")

		// Then it asks the blocking entry to reconcile and reports progress
		if len(triggered) == 0 {
			t.Fatal("Expected at least one reconcile trigger against the blocking entry, got none")
		}
		for _, name := range triggered {
			if name != "entry-one" {
				t.Errorf("Expected the trigger to target entry-one, got: %s", name)
			}
		}
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
		// Given a live entry that goes away within the grace retries
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
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization sees the kustomization disappear
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it returns no error
		if err != nil {
			t.Errorf("Expected no error once the entry settles within the grace period, got %v", err)
		}
	})

	t.Run("DisappearedGraceWindowScalesWithInventorySize", func(t *testing.T) {
		// Given a live entry that settles after more polls than the 3-poll grace floor
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
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationDeletionPerEntryTimeout = 100 * time.Millisecond

		// When DeleteKustomization sees the kustomization disappear
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then the inventory size extends the grace window and the delete succeeds
		if err != nil {
			t.Errorf("Expected no error once the entry settles within the scaled grace window, got %v", err)
		}
		if entryCalls <= 3 {
			t.Fatalf("Test invalid: entry settled within the old fixed floor (%d calls), does not exercise scaling", entryCalls)
		}
	})

	t.Run("DisappearedIsCleanWhenPolicyIsMirrorPrune", func(t *testing.T) {
		// Given a MirrorPrune kustomization that disappears while an entry stays live
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
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization sees the kustomization disappear
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it returns no error
		if err != nil {
			t.Errorf("Expected no error for a MirrorPrune delete, got %v", err)
		}
	})

	t.Run("DisappearedIsCleanWhenLastKnownInventoryIsDrained", func(t *testing.T) {
		// Given a kustomization that disappears with every inventory entry gone
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
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization sees the kustomization disappear
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it returns no error
		if err != nil {
			t.Errorf("Expected no error for a drained delete, got %v", err)
		}
	})

	t.Run("DisappearedFailsWhenInventoryCheckInconclusive", func(t *testing.T) {
		// Given a kustomization that disappears while its entry lookup fails with an API error
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
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization sees the kustomization disappear
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it fails with an unconfirmed-delete error and an inspect hint
		if err == nil {
			t.Fatal("Expected an inconclusive check to fail, got nil")
		}
		if !strings.Contains(err.Error(), "could not confirm its resources are gone") {
			t.Errorf("Expected an unconfirmed-delete error, got: %v", err)
		}
		if we := werror.Find(err); we == nil || we.Remediation != "Inspect the namespace, then retry." {
			t.Errorf("Expected the inspect-namespace hint, got: %v", we)
		}
	})

	t.Run("DisappearedIsCleanWhenNoInventoryWasEverReported", func(t *testing.T) {
		// Given a kustomization that is gone on the first read
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization sees the kustomization disappear
		err := manager.DeleteKustomization(context.Background(), "test-kustomization", "test-namespace")

		// Then it returns no error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// TestBaseKubernetesManager_DeleteKustomizationDuringADestroy covers the DeleteBlueprint path,
// where a live object with no finalizer is residue that does not block the delete.
func TestBaseKubernetesManager_DeleteKustomizationDuringADestroy(t *testing.T) {
	setup := func(t *testing.T, kubeClient client.KubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := kubernetes.NewKubernetesManager(kubeClient, mocks.ConfigHandler)
		manager.Timing.KustomizationWaitPollInterval = 50 * time.Millisecond
		manager.Timing.KustomizationReconcileTimeout = 100 * time.Millisecond
		manager.Timing.KustomizationReconcileSleep = 50 * time.Millisecond
		clock := newFakeClock()
		manager.Shims.TimeNow = clock.Now
		manager.Shims.TimeSleep = clock.Sleep
		return manager
	}

	blueprint := &blueprintv1alpha1.Blueprint{
		Metadata:       blueprintv1alpha1.Metadata{Name: "test-blueprint"},
		Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "test-kustomization"}},
	}

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

	t.Run("TimeoutNamesResidueInsteadOfClaimingFullyDrained", func(t *testing.T) {
		// Given a stuck kustomization with one entry gone and one live with no finalizer
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
		manager := setup(t, kubernetesClient)

		// When DeleteBlueprint times out on the kustomization
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the error names the leftover count, not the fully-drained wording
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

	t.Run("DisappearedSucceedsWhenOnlyResidueSurvives", func(t *testing.T) {
		// Given a kustomization that disappears while its entries stay live with no finalizer
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		deleted := false
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleted = true
			return nil
		}
		postDeleteReads := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if name == "test-kustomization" {
				if !deleted {
					return drainedKustomization(), nil
				}
				postDeleteReads++
				if postDeleteReads <= 1 {
					return drainedKustomization(), nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return &unstructured.Unstructured{}, nil
		}
		manager := setup(t, kubernetesClient)

		// When DeleteBlueprint sees the kustomization disappear
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the delete succeeds
		if err != nil {
			t.Errorf("Expected residue to let the delete succeed, got: %v", err)
		}
	})
}

// TestBaseKubernetesManager_DeleteKustomizationOutsideADestroy covers PruneBlueprint's call
// path: the public DeleteKustomization, run while the cluster stays up. There a finalizer-free
// object still blocks, unlike during an actual destroy.
func TestBaseKubernetesManager_DeleteKustomizationOutsideADestroy(t *testing.T) {
	setup := func(t *testing.T, kubeClient client.KubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := kubernetes.NewKubernetesManager(kubeClient, mocks.ConfigHandler)
		manager.Timing.KustomizationWaitPollInterval = 5 * time.Millisecond
		manager.Timing.KustomizationReconcileTimeout = 40 * time.Millisecond
		manager.Timing.KustomizationDeletionPerEntryTimeout = 0
		manager.Timing.KustomizationDeletionMaxExtraTimeout = 0
		manager.Timing.KustomizationAbandonedGraceMaxExtra = 0
		clock := newFakeClock()
		manager.Shims.TimeNow = clock.Now
		manager.Shims.TimeSleep = clock.Sleep
		return manager
	}

	t.Run("StillBlocksOnALiveObjectWithNoFinalizer", func(t *testing.T) {
		// Given an orphaned kustomization whose inventory names a live object with no finalizer
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
				return &unstructured.Unstructured{}, nil
			}
		}
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization runs outside a destroy
		err := manager.DeleteKustomization(context.Background(), "orphaned-kustomization", "test-namespace")

		// Then the object with no finalizer still blocks the delete
		if err == nil {
			t.Fatal("Expected a live object with no finalizer to still block outside a destroy, got nil")
		}
		if !strings.Contains(err.Error(), "Widget/leftover") {
			t.Errorf("Expected the leftover object to be named, got: %v", err)
		}
	})
}

func TestBaseKubernetesManager_DeleteKustomizationReconcileTrigger(t *testing.T) {
	setup := func(t *testing.T, kubeClient client.KubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := kubernetes.NewKubernetesManager(kubeClient, mocks.ConfigHandler)
		manager.Timing.KustomizationWaitPollInterval = 5 * time.Millisecond
		manager.Timing.KustomizationReconcileTimeout = 40 * time.Millisecond
		manager.Timing.KustomizationDeletionPerEntryTimeout = 0
		manager.Timing.KustomizationDeletionMaxExtraTimeout = 0
		manager.Timing.KustomizationAbandonedGraceMaxExtra = 0
		clock := newFakeClock()
		manager.Shims.TimeNow = clock.Now
		manager.Shims.TimeSleep = clock.Sleep
		return manager
	}

	type patchCall struct {
		gvr  schema.GroupVersionResource
		name string
		data []byte
	}

	// clientWithBlockingEntry returns a client whose kustomization disappears while the entry entryName stays live.
	clientWithBlockingEntry := func(entryID, entryName string, patchErr error, calls *[]patchCall) *client.MockKubernetesClient {
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		reads := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			switch name {
			case "cluster-install":
				reads++
				if reads <= 2 {
					return &unstructured.Unstructured{Object: map[string]any{
						"spec": map[string]any{"deletionPolicy": "WaitForTermination"},
						"status": map[string]any{"inventory": map[string]any{"entries": []any{
							map[string]any{"id": entryID, "v": "v1"},
						}}},
					}}, nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			case entryName:
				return blockingObject(), nil
			default:
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
		}
		kubernetesClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			*calls = append(*calls, patchCall{gvr: gvr, name: name, data: data})
			return nil, patchErr
		}
		return kubernetesClient
	}

	t.Run("UsesFluxAnnotationForAKustomization", func(t *testing.T) {
		// Given a blocking inventory entry that is a Kustomization
		var calls []patchCall
		kubernetesClient := clientWithBlockingEntry("system-gitops_crds_kustomize.toolkit.fluxcd.io_Kustomization", "crds", nil, &calls)
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization waits on the entry
		_ = manager.DeleteKustomization(context.Background(), "cluster-install", "system-gitops")

		// Then it patches the Kustomization with the flux reconcile annotation
		if len(calls) == 0 {
			t.Fatal("Expected at least one reconcile patch, got none")
		}
		for _, call := range calls {
			if call.name != "crds" || call.gvr.Resource != "kustomizations" {
				t.Errorf("Expected a patch against the Kustomization, got %q %v", call.name, call.gvr)
			}
			if !strings.Contains(string(call.data), "reconcile.fluxcd.io/requestedAt") {
				t.Errorf("Expected the flux reconcile annotation in the patch, got: %s", call.data)
			}
		}
	})

	t.Run("UsesWindsorAnnotationForANonFluxObject", func(t *testing.T) {
		// Given a blocking inventory entry outside the flux kinds
		var calls []patchCall
		kubernetesClient := clientWithBlockingEntry("demo-database_provider-sql-demo-db_postgresql.sql.m.crossplane.io_ProviderConfig", "provider-sql-demo-db", nil, &calls)
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization waits on the entry
		_ = manager.DeleteKustomization(context.Background(), "cluster-install", "system-gitops")

		// Then it patches the windsor annotation and not the flux annotation
		if len(calls) == 0 {
			t.Fatal("Expected at least one reconcile patch, got none")
		}
		for _, call := range calls {
			if !strings.Contains(string(call.data), "windsorcli.dev/reconcile-requested-at") {
				t.Errorf("Expected the windsor reconcile annotation in the patch, got: %s", call.data)
			}
			if strings.Contains(string(call.data), "reconcile.fluxcd.io/requestedAt") {
				t.Errorf("Expected no flux annotation on a non-flux object, got: %s", call.data)
			}
		}
	})

	t.Run("NeverTouchesFinalizers", func(t *testing.T) {
		// Given any blocking inventory entry
		var calls []patchCall
		kubernetesClient := clientWithBlockingEntry("demo-database_provider-sql-demo-db_postgresql.sql.m.crossplane.io_ProviderConfig", "provider-sql-demo-db", nil, &calls)
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization waits on the entry
		_ = manager.DeleteKustomization(context.Background(), "cluster-install", "system-gitops")

		// Then no reconcile patch names finalizers
		if len(calls) == 0 {
			t.Fatal("Expected at least one reconcile patch, got none")
		}
		for _, call := range calls {
			if strings.Contains(string(call.data), "finalizers") {
				t.Errorf("Expected the patch to never touch finalizers, got: %s", call.data)
			}
		}
	})

	t.Run("IgnoresAPatchFailure", func(t *testing.T) {
		// Given a blocking inventory entry whose reconcile patch fails
		var calls []patchCall
		kubernetesClient := clientWithBlockingEntry("demo-database_provider-sql-demo-db_postgresql.sql.m.crossplane.io_ProviderConfig", "provider-sql-demo-db", fmt.Errorf("forbidden"), &calls)
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization waits on the entry
		err := manager.DeleteKustomization(context.Background(), "cluster-install", "system-gitops")

		// Then it reports the live entry and not the patch failure
		if len(calls) == 0 {
			t.Fatal("Expected at least one reconcile patch, got none")
		}
		if err == nil {
			t.Fatal("Expected an error naming the live entry, got nil")
		}
		if strings.Contains(err.Error(), "forbidden") {
			t.Errorf("Expected the patch failure to stay hidden, got: %v", err)
		}
		if !strings.Contains(err.Error(), "ProviderConfig/provider-sql-demo-db") {
			t.Errorf("Expected the live entry to be named, got: %v", err)
		}
	})
}

func TestBaseKubernetesManager_DeleteKustomizationHelmUninstallTimeout(t *testing.T) {
	setup := func(t *testing.T, kubeClient client.KubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := kubernetes.NewKubernetesManager(kubeClient, mocks.ConfigHandler)
		manager.Timing.KustomizationWaitPollInterval = 5 * time.Millisecond
		manager.Timing.KustomizationReconcileTimeout = 20 * time.Millisecond
		manager.Timing.KustomizationReconcileSleep = 0
		manager.Timing.KustomizationDeletionPerEntryTimeout = 0
		manager.Timing.KustomizationDeletionMaxExtraTimeout = 0
		clock := newFakeClock()
		manager.Shims.TimeNow = clock.Now
		manager.Shims.TimeSleep = clock.Sleep
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

	// neverDeletedClient returns a client whose kustomization never goes away. A nil hr fails every HelmRelease read.
	neverDeletedClient := func(hr, kustomization *unstructured.Unstructured) *client.MockKubernetesClient {
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
		return kubernetesClient
	}

	// resolvedBudget runs a delete that never completes and returns the wait budget its timeout error reports.
	resolvedBudget := func(t *testing.T, hr, kustomization *unstructured.Unstructured) string {
		t.Helper()
		manager := setup(t, neverDeletedClient(hr, kustomization))
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
		// Given a HelmRelease whose uninstall budget exceeds the reconcile timeout
		hr := helmRelease("", "45m")

		// When the delete waits
		budget := resolvedBudget(t, hr, kustomizationWithHelmRelease)

		// Then the wait adopts spec.uninstall.timeout
		if budget != "45m0s" {
			t.Errorf("Expected the wait to adopt spec.uninstall.timeout (45m0s), got %s", budget)
		}
	})

	t.Run("FallsBackToHelmReleaseSpecTimeout", func(t *testing.T) {
		// Given a HelmRelease that declares only spec.timeout
		hr := helmRelease("45m", "")

		// When the delete waits
		budget := resolvedBudget(t, hr, kustomizationWithHelmRelease)

		// Then the wait uses spec.timeout as the uninstall budget
		if budget != "45m0s" {
			t.Errorf("Expected the wait to fall back to the HelmRelease spec.timeout (45m0s), got %s", budget)
		}
	})

	t.Run("PrefersUninstallTimeoutOverSpecTimeout", func(t *testing.T) {
		// Given a HelmRelease with a shorter uninstall timeout than its spec.timeout
		hr := helmRelease("45m", "25m")

		// When the delete waits
		budget := resolvedBudget(t, hr, kustomizationWithHelmRelease)

		// Then the uninstall timeout wins
		if budget != "25m0s" {
			t.Errorf("Expected spec.uninstall.timeout (25m0s) to win, got %s", budget)
		}
	})

	t.Run("TakesTheLongestAcrossHelmReleases", func(t *testing.T) {
		// Given a kustomization that wraps two HelmReleases
		twoReleases := &unstructured.Unstructured{Object: map[string]any{
			"status": map[string]any{"inventory": map[string]any{"entries": []any{
				map[string]any{"id": "system-gitops_cert-manager_helm.toolkit.fluxcd.io_HelmRelease", "v": "v2"},
				map[string]any{"id": "system-gitops_trust-manager_helm.toolkit.fluxcd.io_HelmRelease", "v": "v2"},
			}}},
		}}

		// When the delete waits with both releases declaring 40m
		budget := resolvedBudget(t, helmRelease("", "40m"), twoReleases)

		// Then the longest declared budget sets the wait
		if budget != "40m0s" {
			t.Errorf("Expected the longest declared budget (40m0s), got %s", budget)
		}
	})

	t.Run("IgnoresHelmReleaseDeclaringNoTimeout", func(t *testing.T) {
		// Given a HelmRelease that declares neither timeout
		hr := helmRelease("", "")

		// When the delete waits
		budget := resolvedBudget(t, hr, kustomizationWithHelmRelease)

		// Then the wait keeps the base budget
		if budget != "20ms" {
			t.Errorf("Expected the unextended budget (20ms), got %s", budget)
		}
	})

	t.Run("SurvivesUnreadableHelmRelease", func(t *testing.T) {
		// Given a HelmRelease entry that cannot be read
		var hr *unstructured.Unstructured

		// When the delete waits
		budget := resolvedBudget(t, hr, kustomizationWithHelmRelease)

		// Then the wait keeps the base budget
		if budget != "20ms" {
			t.Errorf("Expected the unextended budget (20ms), got %s", budget)
		}
	})

	t.Run("ExplicitDeleteTimeoutBoundsTheChartBudget", func(t *testing.T) {
		// Given a blueprint DeleteTimeout of 10m on a kustomization whose chart declares 45m
		manager := setup(t, neverDeletedClient(helmRelease("", "45m"), kustomizationWithHelmRelease))
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{{
				Name:          "pki-install",
				DeleteTimeout: &blueprintv1alpha1.DurationString{Duration: 10 * time.Minute},
			}},
		}

		// When DeleteBlueprint waits on the delete
		err := manager.DeleteBlueprint(context.Background(), blueprint, "system-gitops")

		// Then the DeleteTimeout bounds the wait
		if err == nil {
			t.Fatal("Expected the never-deleted kustomization to time out, got nil")
		}
		if !strings.Contains(err.Error(), "timed out after 10m0s") {
			t.Errorf("Expected DeleteTimeout (10m0s) to bound the wait, got: %v", err)
		}
	})
}

func TestBaseKubernetesManager_DeleteKustomizationReadFailures(t *testing.T) {
	setup := func(t *testing.T, kubeClient client.KubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := kubernetes.NewKubernetesManager(kubeClient, mocks.ConfigHandler)
		manager.Timing.KustomizationWaitPollInterval = 10 * time.Millisecond
		manager.Timing.KustomizationReconcileTimeout = 500 * time.Millisecond
		manager.Timing.KustomizationDeletionPerEntryTimeout = time.Millisecond
		return manager
	}

	t.Run("ToleratesTransientReadFailure", func(t *testing.T) {
		// Given two failed status reads, then two live reads, then a gone kustomization
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
		manager := setup(t, kubernetesClient)
		ctx, rec := recordProgress(t)

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(ctx, "demo-resources", "system-gitops")

		// Then the delete succeeds and reports the retry as progress
		if err != nil {
			t.Errorf("Expected transient read failures to be tolerated, got error: %v", err)
		}
		if reads < 4 {
			t.Errorf("Expected the wait to poll past the failures, got %d reads", reads)
		}
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
		// Given status reads that fail until the kustomization is gone
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
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "demo-resources", "system-gitops")

		// Then it fails with an unread-inventory error and a hint
		if err == nil {
			t.Fatal("Expected an unverified disappearance to fail, got nil")
		}
		if !strings.Contains(err.Error(), "before windsor could read its inventory") {
			t.Errorf("Expected an unread-inventory error, got: %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "grep Terminating") {
			t.Errorf("Expected a hint that finds leftovers, got: %v", we)
		}
	})

	t.Run("FailsAfterMaxConsecutiveReadFailures", func(t *testing.T) {
		// Given status reads that always fail and a tolerance of 2
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		reads := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			reads++
			return nil, fmt.Errorf("etcdserver: request timed out")
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationWaitMaxReadFailures = 2

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "demo-resources", "system-gitops")

		// Then it fails after exactly the configured tolerance
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
		// Given status reads that alternate failure and success, then a gone kustomization
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
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationWaitMaxReadFailures = 1

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "demo-resources", "system-gitops")

		// Then a successful read resets the failure count
		if err != nil {
			t.Errorf("Expected alternating failures to be tolerated, got error: %v", err)
		}
	})
}

func TestBaseKubernetesManager_DeleteKustomizationVerifiesChartResources(t *testing.T) {
	setup := func(t *testing.T, kubeClient client.KubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := kubernetes.NewKubernetesManager(kubeClient, mocks.ConfigHandler)
		manager.Timing.KustomizationWaitPollInterval = 5 * time.Millisecond
		manager.Timing.KustomizationReconcileTimeout = 40 * time.Millisecond
		manager.Timing.KustomizationDeletionPerEntryTimeout = 0
		manager.Timing.KustomizationDeletionMaxExtraTimeout = 0
		manager.Timing.KustomizationAbandonedGraceMaxExtra = 0
		clock := newFakeClock()
		manager.Shims.TimeNow = clock.Now
		manager.Shims.TimeSleep = clock.Sleep
		return manager
	}

	// kustomizationWrappingChart returns a kustomization whose whole inventory is one HelmRelease.
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
		// Given a kustomization and HelmRelease that disappear while a chart resource stays live
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
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "cert-manager", "system-pki")

		// Then it names the leftover chart resource
		if err == nil {
			t.Fatal("Expected a live chart resource to fail the delete, got nil")
		}
		if !strings.Contains(err.Error(), "cert-manager-webhook:dynamic-serving") {
			t.Errorf("Expected the leftover chart resource to be named, got: %v", err)
		}
	})

	t.Run("AcceptsADeleteWhoseChartResourcesAreAllGone", func(t *testing.T) {
		// Given the same teardown with every chart resource gone
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
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "cert-manager", "system-pki")

		// Then it returns no error
		if err != nil {
			t.Errorf("Expected a fully drained chart to pass, got: %v", err)
		}
	})

	t.Run("FailsWhenAHelmReleaseWasNeverReadable", func(t *testing.T) {
		// Given a HelmRelease whose reads fail with a transient API error
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
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "cert-manager", "system-pki")

		// Then it fails with an unread-chart error that names the release
		if err == nil {
			t.Fatal("Expected an unverifiable HelmRelease to fail the delete, got nil")
		}
		if !strings.Contains(err.Error(), "never read what helmrelease system-pki/cert-manager managed") {
			t.Errorf("Expected an unread-chart error naming the release, got: %v", err)
		}
	})

	t.Run("FailsWhenAHelmReleaseWasAlreadyGoneBeforeTheFirstPoll", func(t *testing.T) {
		// Given a HelmRelease that is already gone on the first poll
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
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "cert-manager", "system-pki")

		// Then it fails with an unread-chart error that names the release
		if err == nil {
			t.Fatal("Expected an unobserved HelmRelease to fail the delete, got nil")
		}
		if !strings.Contains(err.Error(), "never read what helmrelease system-pki/cert-manager managed") {
			t.Errorf("Expected an unread-chart error naming the release, got: %v", err)
		}
	})

	t.Run("AcceptsAHelmReleaseThatPublishesNoInventory", func(t *testing.T) {
		// Given a live HelmRelease that reports no inventory
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
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "cert-manager", "system-pki")

		// Then it returns no error
		if err != nil {
			t.Errorf("Expected a flux without chart inventory to degrade, got: %v", err)
		}
	})

	t.Run("FailsWhenAChartInventoryOnlyPartlyDecodes", func(t *testing.T) {
		// Given a HelmRelease inventory with an entry that cannot be decoded
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
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "cert-manager", "system-pki")

		// Then it fails with a partial-inventory error
		if err == nil {
			t.Fatal("Expected a partly-decoded chart inventory to fail the delete, got nil")
		}
		if !strings.Contains(err.Error(), "could not fully decode") {
			t.Errorf("Expected a partial-inventory error, got: %v", err)
		}
	})

	t.Run("ReportsALiveNestedHelmRelease", func(t *testing.T) {
		// Given a chart whose nested HelmRelease stays live after the outer release is gone
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
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "cert-manager", "system-pki")

		// Then it names the nested release
		if err == nil {
			t.Fatal("Expected a live nested HelmRelease to fail the delete, got nil")
		}
		if !strings.Contains(err.Error(), "HelmRelease/inner") {
			t.Errorf("Expected the nested release to be named, got: %v", err)
		}
	})

	t.Run("AcceptsAHelmReleaseThatDeletesBeforeTheFirstPoll", func(t *testing.T) {
		// Given a HelmRelease that is live before the delete and gone right after it
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
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "cert-manager", "system-pki")

		// Then the pre-delete snapshot verifies the chart resources are gone
		if err != nil {
			t.Errorf("Expected a fast-deleting chart captured before delete to pass, got: %v", err)
		}
	})

	t.Run("WillNotClaimDrainedWhenAnEntryCannotBeDecoded", func(t *testing.T) {
		// Given a stuck kustomization with an inventory entry that cannot be decoded
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
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization times out
		err := manager.DeleteKustomization(context.Background(), "cert-manager", "system-pki")

		// Then it does not claim the kustomization is drained
		if err == nil {
			t.Fatal("Expected the delete to time out, got nil")
		}
		if strings.Contains(err.Error(), "fully drained") {
			t.Errorf("Expected no drained claim over an undecodable inventory, got: %v", err)
		}
	})

	t.Run("RemembersChartResourcesAfterTheInventoryShrinks", func(t *testing.T) {
		// Given a HelmRelease inventory that empties while an earlier listed resource stays live
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
		manager := setup(t, kubernetesClient)

		// When DeleteKustomization runs
		err := manager.DeleteKustomization(context.Background(), "cert-manager", "system-pki")

		// Then the earlier inventory still names the live chart resource
		if err == nil {
			t.Fatal("Expected the remembered chart resource to fail the delete, got nil")
		}
		if !strings.Contains(err.Error(), "cert-manager-webhook:dynamic-serving") {
			t.Errorf("Expected the remembered chart resource to be named, got: %v", err)
		}
	})
}

func TestBaseKubernetesManager_DeleteBlueprintDeleteTimeoutOverride(t *testing.T) {
	setup := func(t *testing.T, kubeClient client.KubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := kubernetes.NewKubernetesManager(kubeClient, mocks.ConfigHandler)
		manager.Timing.KustomizationReconcileTimeout = 20 * time.Millisecond
		manager.Timing.KustomizationWaitPollInterval = 10 * time.Millisecond
		manager.Timing.KustomizationReconcileSleep = 0
		clock := newFakeClock()
		manager.Shims.TimeNow = clock.Now
		manager.Shims.TimeSleep = clock.Sleep
		return manager
	}

	// blueprintWithDeleteTimeout returns a one-kustomization blueprint. A nil override leaves DeleteTimeout unset.
	blueprintWithDeleteTimeout := func(override *time.Duration) *blueprintv1alpha1.Blueprint {
		kustomization := blueprintv1alpha1.Kustomization{Name: "test-kustomization"}
		if override != nil {
			kustomization.DeleteTimeout = &blueprintv1alpha1.DurationString{Duration: *override}
		}
		return &blueprintv1alpha1.Blueprint{
			Metadata:       blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{kustomization},
		}
	}

	t.Run("OverrideRaisesWaitPastSmallerSpecTimeout", func(t *testing.T) {
		// Given a 150ms DeleteTimeout and a 30ms live spec.timeout
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{
				"spec": map[string]any{"timeout": "30ms"},
			}}, nil
		}
		manager := setup(t, kubernetesClient)
		override := 150 * time.Millisecond

		// When DeleteBlueprint times out on the kustomization
		start := manager.Shims.TimeNow()
		err := manager.DeleteBlueprint(context.Background(), blueprintWithDeleteTimeout(&override), "test-namespace")
		elapsed := manager.Shims.TimeNow().Sub(start)

		// Then it waits out the override
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if elapsed < 150*time.Millisecond {
			t.Errorf("Expected wait window raised to the override (>=150ms), got %s", elapsed)
		}
	})

	t.Run("OverrideNotClampedByInstallCeiling", func(t *testing.T) {
		// Given a 150ms DeleteTimeout and a 50ms spec-timeout ceiling
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationSpecTimeoutCeiling = 50 * time.Millisecond
		override := 150 * time.Millisecond

		// When DeleteBlueprint times out on the kustomization
		start := manager.Shims.TimeNow()
		err := manager.DeleteBlueprint(context.Background(), blueprintWithDeleteTimeout(&override), "test-namespace")
		elapsed := manager.Shims.TimeNow().Sub(start)

		// Then the wait reaches the override
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if elapsed < 150*time.Millisecond {
			t.Errorf("Expected wait window to reach the override (>=150ms), got %s", elapsed)
		}
	})

	t.Run("OverrideCapsAtCeiling", func(t *testing.T) {
		// Given a DeleteTimeout far larger than the 100ms delete-timeout ceiling
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationDeleteTimeoutCeiling = 100 * time.Millisecond
		override := 876000 * time.Hour

		// When DeleteBlueprint times out on the kustomization
		start := manager.Shims.TimeNow()
		err := manager.DeleteBlueprint(context.Background(), blueprintWithDeleteTimeout(&override), "test-namespace")
		elapsed := manager.Shims.TimeNow().Sub(start)

		// Then the ceiling bounds the wait
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if elapsed < 100*time.Millisecond || elapsed > 250*time.Millisecond {
			t.Errorf("Expected wait window capped near the ceiling (~100ms), got %s", elapsed)
		}
	})

	t.Run("NoOverrideFallsBackToSpecTimeout", func(t *testing.T) {
		// Given no DeleteTimeout and a 150ms live spec.timeout
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{
				"spec": map[string]any{"timeout": "150ms"},
			}}, nil
		}
		manager := setup(t, kubernetesClient)

		// When DeleteBlueprint times out on the kustomization
		start := manager.Shims.TimeNow()
		err := manager.DeleteBlueprint(context.Background(), blueprintWithDeleteTimeout(nil), "test-namespace")
		elapsed := manager.Shims.TimeNow().Sub(start)

		// Then it waits out spec.timeout
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if elapsed < 150*time.Millisecond {
			t.Errorf("Expected wait window raised to spec.timeout (>=150ms), got %s", elapsed)
		}
	})
}
