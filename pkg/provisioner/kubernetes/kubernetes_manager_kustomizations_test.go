package kubernetes_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	meta "github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseKubernetesManager_ApplyKustomization(t *testing.T) {
	setup := func(t *testing.T, kubeClient client.KubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		manager := kubernetes.NewKubernetesManager(kubeClient, setupKubernetesMocks(t).ConfigHandler)
		manager.Timing.KustomizationWaitPollInterval = 50 * time.Millisecond
		manager.Timing.KustomizationReconcileTimeout = 100 * time.Millisecond
		manager.Timing.KustomizationReconcileSleep = 50 * time.Millisecond
		return manager
	}
	newKustomization := func() kustomizev1.Kustomization {
		return kustomizev1.Kustomization{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-kustomization",
				Namespace: "test-namespace",
			},
			Spec: kustomizev1.KustomizationSpec{
				Path: "./test-path",
			},
		}
	}

	t.Run("AppliesKustomization", func(t *testing.T) {
		// Given a manager with a client that accepts applies
		manager := setup(t, setupKubernetesMocks(t).KubernetesClient)

		// When a kustomization is applied
		err := manager.ApplyKustomization(newKustomization())

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("UnstructuredConversionError", func(t *testing.T) {
		// Given a manager whose conversion to unstructured fails
		manager := setup(t, setupKubernetesMocks(t).KubernetesClient)
		manager.Shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return nil, fmt.Errorf("forced conversion error")
		}

		// When a kustomization is applied
		err := manager.ApplyKustomization(newKustomization())

		// Then a conversion error is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to convert kustomization to unstructured") {
			t.Errorf("Expected conversion error, got: %v", err)
		}
	})

	t.Run("ApplyWithRetryError", func(t *testing.T) {
		// Given a client that cannot find the object and rejects the apply
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("apply error")
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		// When a kustomization is applied
		err := manager.ApplyKustomization(newKustomization())

		// Then an error is returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ApplyWithRetryExistingResourceConversionError", func(t *testing.T) {
		// Given an existing object whose conversion to unstructured fails
		kubernetesClient := client.NewMockKubernetesClient()
		existingObj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
				"kind":       "Kustomization",
				"metadata": map[string]any{
					"name":            "test-kustomization",
					"namespace":       "test-namespace",
					"resourceVersion": "123",
				},
			},
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return existingObj, nil
		}
		manager := setup(t, kubernetesClient)
		callCount := 0
		originalToUnstructured := manager.Shims.ToUnstructured
		manager.Shims.ToUnstructured = func(obj any) (map[string]any, error) {
			callCount++
			if callCount == 1 {
				return originalToUnstructured(obj)
			}
			return nil, fmt.Errorf("conversion error")
		}

		// When a kustomization is applied
		err := manager.ApplyKustomization(newKustomization())

		// Then a conversion error for the existing object is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to convert existing object to unstructured") {
			t.Errorf("Expected conversion error, got: %v", err)
		}
	})
}

func TestBaseKubernetesManager_WaitForKustomizations(t *testing.T) {
	setup := func(t *testing.T, kubeClient client.KubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		manager := kubernetes.NewKubernetesManager(kubeClient, setupKubernetesMocks(t).ConfigHandler)
		manager.Timing.KustomizationWaitPollInterval = 50 * time.Millisecond
		manager.Timing.KustomizationWaitMinErrorDuration = 150 * time.Millisecond
		manager.Timing.KustomizationReconcileTimeout = 100 * time.Millisecond
		manager.Timing.KustomizationReconcileSleep = 50 * time.Millisecond
		return manager
	}
	listOf := func(items ...unstructured.Unstructured) *unstructured.UnstructuredList {
		return &unstructured.UnstructuredList{Items: items}
	}
	withTimeout := func(name string, d time.Duration) blueprintv1alpha1.Kustomization {
		return blueprintv1alpha1.Kustomization{Name: name, Timeout: &blueprintv1alpha1.DurationString{Duration: d}}
	}
	wait := func(manager *kubernetes.BaseKubernetesManager, blueprint *blueprintv1alpha1.Blueprint) error {
		return manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))
	}

	t.Run("SucceedsWhenKustomizationIsReady", func(t *testing.T) {
		// Given a kustomization that is Ready
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(kustomizationListItem("test-kustomization", "True", "", "")), nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", 200*time.Millisecond),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReportsEachKustomizationToTheCallersGroup", func(t *testing.T) {
		// Given a ready kustomization and a recording presenter
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(kustomizationListItem("test-kustomization", "True", "", "")), nil
		}
		manager := setup(t, kubernetesClient)
		ctx, rec := recordProgress(t)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "test-kustomization"}}}

		// When the wait runs
		err := manager.WaitForKustomizations(ctx, blueprint, presenter.NewGroup(ctx, "kustomization:"))

		// Then the kustomization is one child of the caller's operation and ends as applied
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		started := rec.started()
		if len(started) != 2 || started[1] != "test-kustomization" {
			t.Fatalf("Unexpected operations %v", started)
		}
		var ended []string
		for _, e := range rec.events {
			if e.Kind == presenter.KindApplied {
				ended = append(ended, e.Message)
			}
		}
		if strings.Join(ended, ",") != "test-kustomization" {
			t.Errorf("Expected the child to end as applied, got %v", ended)
		}
	})

	t.Run("ShowsADependentLayerOnlyAfterItsDependencyIsReady", func(t *testing.T) {
		// Given "a" Ready from the second list and "b", which depends on it, Ready from the third
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		lists := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			lists++
			aStatus, bStatus := "False", "False"
			if lists >= 2 {
				aStatus = "True"
			}
			if lists >= 3 {
				bStatus = "True"
			}
			return listOf(
				kustomizationListItem("a", aStatus, "", ""),
				kustomizationListItem("b", bStatus, "", ""),
			), nil
		}
		manager := setup(t, kubernetesClient)
		ctx, rec := recordProgress(t)
		b := withTimeout("b", time.Second)
		b.DependsOn = []string{"a"}
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{withTimeout("a", time.Second), b}}

		// When the wait runs
		err := manager.WaitForKustomizations(ctx, blueprint, presenter.NewGroup(ctx, "kustomization:"))

		// Then "b" starts only after "a" ends
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		var order []string
		for _, e := range rec.events {
			if e.Message == "a" || e.Message == "b" {
				order = append(order, string(e.Kind)+":"+e.Message)
			}
		}
		want := []string{"applying:a", "applied:a", "applying:b", "applied:b"}
		if strings.Join(order, ",") != strings.Join(want, ",") {
			t.Errorf("Expected %v, got %v", want, order)
		}
	})

	t.Run("LeavesUnreadyLayersOpenOnTimeout", func(t *testing.T) {
		// Given a kustomization that never becomes ready and a recording presenter
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(kustomizationListItem("test-kustomization", "False", "Progressing", "still reconciling")), nil
		}
		manager := setup(t, kubernetesClient)
		ctx, rec := recordProgress(t)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", 200*time.Millisecond),
		}}
		layers := presenter.NewGroup(ctx, "kustomization:")

		// When the wait times out and the caller closes its group with the error
		err := manager.WaitForKustomizations(ctx, blueprint, layers)
		if err == nil {
			t.Fatal("Expected a timeout error")
		}
		layers.Close(err)

		// Then the unready kustomization ends as failed only when the caller closes the group
		var failed []string
		for _, e := range rec.events {
			if e.Kind == presenter.KindFailed && e.Err != nil {
				failed = append(failed, e.Message)
			}
		}
		if strings.Join(failed, ",") != "test-kustomization" {
			t.Errorf("Expected the child to fail, got %v", failed)
		}
	})

	t.Run("SurvivesPeriodicReconcileFlipOnceObservedReady", func(t *testing.T) {
		// Given "a" Ready on the first list and not Ready later, and "b" Ready from the second list
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		listCalls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			listCalls++
			aStatus, bStatus := "True", "False"
			if listCalls > 1 {
				aStatus, bStatus = "False", "True"
			}
			return listOf(
				kustomizationListItem("a", aStatus, "", ""),
				kustomizationListItem("b", bStatus, "", ""),
			), nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("a", 500*time.Millisecond),
			withTimeout("b", 500*time.Millisecond),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the flip of "a" is ignored and the wait succeeds on a later tick
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if listCalls < 2 {
			t.Errorf("Expected at least two ticks for \"b\" to become ready, got %d list calls", listCalls)
		}
	})

	t.Run("NotFoundKeepsPollingUntilReady", func(t *testing.T) {
		// Given a kustomization that is absent from the first list and Ready later
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			if calls < 2 {
				return &unstructured.UnstructuredList{}, nil
			}
			return listOf(kustomizationListItem("test-kustomization", "True", "", "")), nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", 500*time.Millisecond),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the wait succeeds once the kustomization appears
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if calls < 2 {
			t.Errorf("Expected the list to be polled at least twice, got %d calls", calls)
		}
	})

	t.Run("CRDNotYetRegisteredIsToleratedForFullTimeout", func(t *testing.T) {
		// Given a list that fails not-found past the error floor and then shows a Ready kustomization
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			if calls <= 5 {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return listOf(kustomizationListItem("test-kustomization", "True", "", "")), nil
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationWaitMinErrorDuration = 150 * time.Millisecond
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", 10*time.Minute),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the not-found errors do not use the error budget and the wait succeeds
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("NilListWithNoErrorIsTreatedAsEmpty", func(t *testing.T) {
		// Given a client that returns a nil list with no error before the kustomization is Ready
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			if calls < 2 {
				return nil, nil
			}
			return listOf(kustomizationListItem("test-kustomization", "True", "", "")), nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", 500*time.Millisecond),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the nil list is treated as empty and the wait succeeds
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("AuthErrorGetsTheSameBudgetAsAnyOtherError", func(t *testing.T) {
		// Given a list that returns a typed auth error on every poll
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			return nil, apierrors.NewUnauthorized("cluster credentials rejected")
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationWaitMinErrorDuration = 150 * time.Millisecond
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", 5*time.Second),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the wait retries and fails on the error budget with the underlying error
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "cluster credentials rejected") {
			t.Errorf("Expected error to surface the underlying failure, got: %v", err)
		}
		if strings.Contains(err.Error(), "timeout waiting for kustomizations") {
			t.Errorf("Expected the error-budget path, not the generic timeout error, got: %v", err)
		}
		if calls < 2 {
			t.Errorf("Expected multiple retries before giving up, got %d calls", calls)
		}
	})

	t.Run("BuildFailedConditionFailsImmediately", func(t *testing.T) {
		// Given a kustomization with a Ready=False/BuildFailed condition on every poll
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			return listOf(
				kustomizationListItem("crds-core", "False", meta.BuildFailedReason, "kustomize build failed: accumulating components: no such file or directory"),
			), nil
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationWaitMinErrorDuration = 150 * time.Millisecond
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("crds-core", 10*time.Minute),
		}}

		// When the wait runs
		start := time.Now()
		err := wait(manager, blueprint)
		elapsed := time.Since(start)

		// Then the wait fails on the first tick and names the kustomization and message
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "crds-core") || !strings.Contains(err.Error(), "no such file or directory") {
			t.Errorf("Expected the failure to name the kustomization and its message, got: %v", err)
		}
		if !errors.Is(err, kubernetes.ErrKustomizationFailed) {
			t.Errorf("Expected the failure to match ErrKustomizationFailed, got: %v", err)
		}
		if elapsed >= manager.Timing.KustomizationWaitMinErrorDuration {
			t.Errorf("Expected an immediate failure bypassing the error-streak budget, took %s", elapsed)
		}
		if calls > 2 {
			t.Errorf("Expected the wait to stop after the first tick observes the failure, got %d calls", calls)
		}
	})

	t.Run("FailsTheStepOfTheFailedKustomizationOnly", func(t *testing.T) {
		// Given "a" with a BuildFailed condition, "b" still progressing, and a recording presenter
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(
				kustomizationListItem("a", "False", meta.BuildFailedReason, "kustomize build failed"),
				kustomizationListItem("b", "False", "Progressing", "still reconciling"),
			), nil
		}
		manager := setup(t, kubernetesClient)
		ctx, rec := recordProgress(t)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "a"}, {Name: "b"}}}

		// When the wait observes the failure
		err := manager.WaitForKustomizations(ctx, blueprint, presenter.NewGroup(ctx, "kustomization:"))

		// Then only "a" ends as failed, with its reason
		if err == nil {
			t.Fatal("Expected an error")
		}
		var failed []string
		for _, e := range rec.events {
			if e.Kind == presenter.KindFailed && e.Err != nil {
				failed = append(failed, e.Message+": "+e.Err.Error())
			}
		}
		if len(failed) != 1 || !strings.HasPrefix(failed[0], "a: ") || !strings.Contains(failed[0], "BuildFailed") {
			t.Errorf("Expected only a to fail with its reason, got %v", failed)
		}
	})

	t.Run("ArtifactFailedFailsImmediately", func(t *testing.T) {
		// Given a kustomization with a Ready=False/ArtifactFailed condition
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(kustomizationListItem("test-kustomization", "False", meta.ArtifactFailedReason, "boom")), nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", 10*time.Minute),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the wait fails with the underlying message
		if err == nil {
			t.Fatal("Expected an immediate failure for ArtifactFailed, got nil")
		}
		if !strings.Contains(err.Error(), "boom") {
			t.Errorf("Expected the underlying message surfaced, got: %v", err)
		}
		if !errors.Is(err, kubernetes.ErrKustomizationFailed) {
			t.Errorf("Expected the failure to match ErrKustomizationFailed, got: %v", err)
		}
	})

	t.Run("ReconciliationFailedRecoversWithinBudgetSucceeds", func(t *testing.T) {
		// Given a kustomization that reports ReconciliationFailed for two ticks and then Ready
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			if calls < 3 {
				return listOf(kustomizationListItem("test-kustomization", "False", meta.ReconciliationFailedReason, "webhook not ready")), nil
			}
			return listOf(kustomizationListItem("test-kustomization", "True", "", "")), nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", time.Second),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the transient failure is tolerated and the wait succeeds
		if err != nil {
			t.Errorf("Expected ReconciliationFailed to be tolerated within the error budget, got %v", err)
		}
		if calls < 3 {
			t.Errorf("Expected multiple polls before recovery, got %d calls", calls)
		}
	})

	t.Run("ReconciliationFailedExceedingBudgetFails", func(t *testing.T) {
		// Given a kustomization that reports ReconciliationFailed on every poll
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(kustomizationListItem("test-kustomization", "False", meta.ReconciliationFailedReason, "boom")), nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", time.Second),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the wait fails on the error budget with the underlying message
		if err == nil {
			t.Fatal("Expected an error once the error budget is exceeded, got nil")
		}
		if !strings.Contains(err.Error(), "boom") {
			t.Errorf("Expected the underlying message surfaced, got: %v", err)
		}
		if strings.Contains(err.Error(), "timeout waiting for kustomizations") {
			t.Errorf("Expected the error-budget path, not the generic timeout error, got: %v", err)
		}
		if errors.Is(err, kubernetes.ErrKustomizationFailed) {
			t.Errorf("Expected a retryable failure not to match ErrKustomizationFailed, got: %v", err)
		}
	})

	t.Run("ReconciliationFailedStreakSurvivesReadyFlippingToUnknown", func(t *testing.T) {
		// Given a Ready condition that alternates between ReconciliationFailed and Unknown/Progressing
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			if calls%2 == 0 {
				return listOf(kustomizationListItem("test-kustomization", "Unknown", meta.ProgressingReason, "Reconciliation in progress")), nil
			}
			return listOf(kustomizationListItem("test-kustomization", "False", meta.ReconciliationFailedReason, "dry-run failed")), nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", 2*time.Second),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the failure budget still applies and the wait fails before the overall timeout
		if err == nil {
			t.Fatal("Expected an error once the error budget is exceeded, got nil")
		}
		if !strings.Contains(err.Error(), "dry-run failed") {
			t.Errorf("Expected the underlying message surfaced, got: %v", err)
		}
		if strings.Contains(err.Error(), "timeout waiting for kustomizations") {
			t.Errorf("Expected the error-budget path, not the generic timeout error, got: %v", err)
		}
	})

	t.Run("FailureBudgetUsesTheFailingKustomizationsOwnTimeout", func(t *testing.T) {
		// Given a large total timeout and a failing dependent kustomization with a short timeout
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(
				kustomizationListItem("base", "True", "", ""),
				kustomizationListItem("slow", "False", meta.ReconciliationFailedReason, "dry-run failed"),
			), nil
		}
		manager := setup(t, kubernetesClient)
		slow := withTimeout("slow", 200*time.Millisecond)
		slow.DependsOn = []string{"base"}
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("base", 4*time.Second),
			slow,
		}}

		// When the wait runs
		started := time.Now()
		err := wait(manager, blueprint)
		elapsed := time.Since(started)

		// Then the wait fails near the own timeout of the failing kustomization
		if err == nil || !strings.Contains(err.Error(), "dry-run failed") {
			t.Fatalf("Expected the error-budget failure, got: %v", err)
		}
		if elapsed > 700*time.Millisecond {
			t.Errorf("Expected failure within the Kustomization's own budget, took %s", elapsed)
		}
	})

	t.Run("FailureBudgetNeverDropsBelowTheMinimum", func(t *testing.T) {
		// Given a failing kustomization whose own timeout is shorter than the minimum error duration
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(
				kustomizationListItem("base", "True", "", ""),
				kustomizationListItem("quick", "False", meta.ReconciliationFailedReason, "dry-run failed"),
			), nil
		}
		manager := setup(t, kubernetesClient)
		quick := withTimeout("quick", 20*time.Millisecond)
		quick.DependsOn = []string{"base"}
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("base", 4*time.Second),
			quick,
		}}

		// When the wait runs
		started := time.Now()
		err := wait(manager, blueprint)
		elapsed := time.Since(started)

		// Then the wait fails only after the minimum error duration from the first observed failure
		if err == nil || !strings.Contains(err.Error(), "dry-run failed") {
			t.Fatalf("Expected the error-budget failure, got: %v", err)
		}
		floor := manager.Timing.KustomizationWaitMinErrorDuration
		if elapsed < floor {
			t.Errorf("Expected the wait to last at least %s, failed after %s", floor, elapsed)
		}
	})

	t.Run("FailureStreakRestartsAfterAFullBudgetWithoutFailures", func(t *testing.T) {
		// Given a kustomization that fails, progresses past the budget, fails again, and becomes Ready
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			switch {
			case calls == 1, calls == 17:
				return listOf(kustomizationListItem("test-kustomization", "False", meta.ReconciliationFailedReason, "transient")), nil
			case calls > 17:
				return listOf(kustomizationListItem("test-kustomization", "True", "", "")), nil
			}
			return listOf(kustomizationListItem("test-kustomization", "False", meta.ProgressingReason, "")), nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", 2*time.Second),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the old failure does not count against the new one and the wait succeeds
		if err != nil {
			t.Errorf("Expected the stale failure streak to expire, got %v", err)
		}
	})

	t.Run("ProgressingConditionKeepsPollingUntilReady", func(t *testing.T) {
		// Given a kustomization that reports Ready=False/Progressing before it becomes Ready
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			status := "False"
			reason := meta.ProgressingReason
			if calls >= 3 {
				status, reason = "True", ""
			}
			return listOf(kustomizationListItem("test-kustomization", status, reason, "")), nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", 500*time.Millisecond),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the wait keeps polling and succeeds
		if err != nil {
			t.Errorf("Expected Progressing to keep polling rather than fail, got %v", err)
		}
		if calls < 3 {
			t.Errorf("Expected multiple polls before Ready, got %d calls", calls)
		}
	})

	t.Run("ReportsMultiLineFailureMessageOnOneLine", func(t *testing.T) {
		// Given a kustomization with a multi-line ReconciliationFailed message before it becomes Ready
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			if calls < 3 {
				return listOf(kustomizationListItem("b", "False", meta.ReconciliationFailedReason, "dry-run failed\n  spec.selector: Required value")), nil
			}
			return listOf(kustomizationListItem("b", "True", "", "")), nil
		}
		manager := setup(t, kubernetesClient)
		ctx, rec := recordProgress(t)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{withTimeout("b", time.Second)}}

		// When the wait runs
		err := manager.WaitForKustomizations(ctx, blueprint, presenter.NewGroup(ctx, "kustomization:"))

		// Then the progress line joins the reason and the flattened message on one line
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		want := "ReconciliationFailed: dry-run failed spec.selector: Required value"
		if !slices.Contains(rec.progress(), want) {
			t.Errorf("Expected progress %q, got %v", want, rec.progress())
		}
	})

	t.Run("TruncatesLongFailureMessageInProgress", func(t *testing.T) {
		// Given a kustomization with a failure message longer than the limit before it becomes Ready
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			if calls < 3 {
				return listOf(kustomizationListItem("b", "False", meta.ReconciliationFailedReason, strings.Repeat("x", 250))), nil
			}
			return listOf(kustomizationListItem("b", "True", "", "")), nil
		}
		manager := setup(t, kubernetesClient)
		ctx, rec := recordProgress(t)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{withTimeout("b", time.Second)}}

		// When the wait runs
		err := manager.WaitForKustomizations(ctx, blueprint, presenter.NewGroup(ctx, "kustomization:"))

		// Then the progress line cuts the message at 200 characters and marks the cut
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		want := "ReconciliationFailed: " + strings.Repeat("x", 200) + "..."
		if !slices.Contains(rec.progress(), want) {
			t.Errorf("Expected progress %q, got %v", want, rec.progress())
		}
	})

	t.Run("TransientErrorIsToleratedThenSucceeds", func(t *testing.T) {
		// Given a list that fails five times and then recovers within the error budget
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			if calls <= 5 {
				return nil, fmt.Errorf("connection refused")
			}
			return listOf(kustomizationListItem("test-kustomization", "True", "", "")), nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", 10*time.Minute),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the transient errors are tolerated and the wait succeeds
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("TransientErrorStreakExceedingBudgetEventuallyFails", func(t *testing.T) {
		// Given a list that fails on every poll with an error budget below the total timeout
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			return nil, fmt.Errorf("connection refused")
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationWaitMinErrorDuration = 150 * time.Millisecond
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", 5*time.Second),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the wait fails with the error-budget message and the underlying error
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "connection refused") {
			t.Errorf("Expected error to surface the underlying failure, got: %v", err)
		}
		if !strings.Contains(err.Error(), "checks failing") {
			t.Errorf("Expected the error-budget message, got: %v", err)
		}
		if strings.Contains(err.Error(), "timeout waiting for kustomizations") {
			t.Errorf("Expected the error-budget path, not the generic timeout error, got: %v", err)
		}
		if calls < 2 {
			t.Errorf("Expected multiple retries before giving up, got %d calls", calls)
		}
	})

	t.Run("ErrorStreakResetsAfterAnIntermediateSuccess", func(t *testing.T) {
		// Given short error streaks split by clean ticks, and Ready from the tenth call
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			if calls >= 10 || calls%3 == 0 {
				readyStatus := "False"
				if calls >= 10 {
					readyStatus = "True"
				}
				return listOf(kustomizationListItem("test-kustomization", readyStatus, "", "")), nil
			}
			return nil, fmt.Errorf("connection refused")
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", 10*time.Second),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then no streak exceeds the budget and the wait succeeds
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if calls < 10 {
			t.Errorf("Expected at least 10 calls, got %d", calls)
		}
	})

	t.Run("ContextCancelledInterruptsWait", func(t *testing.T) {
		// Given a kustomization that never becomes Ready and a cancelled context
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(kustomizationListItem("test-kustomization", "False", "", "")), nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", 10*time.Minute),
		}}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// When the wait runs
		err := manager.WaitForKustomizations(ctx, blueprint, presenter.NewGroup(ctx, "kustomization:"))

		// Then the wait returns the context error
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	})

	t.Run("BecomesReadyFromAWatchEventWithoutAPollTick", func(t *testing.T) {
		// Given a listed kustomization that is not Ready and a watch that reports it Ready
		kubernetesClient := client.NewMockKubernetesClient()
		listCalls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			listCalls++
			list := listOf(kustomizationListItem("app", "False", "", ""))
			list.SetResourceVersion("41")
			return list, nil
		}
		events := watch.NewFakeWithChanSize(1, false)
		ready := kustomizationListItem("app", "True", "", "")
		events.Modify(&ready)
		var watchedVersion string
		kubernetesClient.WatchResourcesFunc = func(_ context.Context, _ schema.GroupVersionResource, _, resourceVersion string) (watch.Interface, error) {
			watchedVersion = resourceVersion
			return events, nil
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationWaitPollInterval = time.Hour
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{withTimeout("app", time.Minute)}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the event completes the wait after one list, and the watch starts at the list version
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if listCalls != 1 {
			t.Errorf("Expected 1 list call, got %d", listCalls)
		}
		if watchedVersion != "41" {
			t.Errorf("Expected the watch to start at resource version 41, got %q", watchedVersion)
		}
		if !events.IsStopped() {
			t.Error("Expected the wait to stop the watch")
		}
	})

	t.Run("BuildFailedWatchEventFailsImmediately", func(t *testing.T) {
		// Given a watch that reports a BuildFailed condition
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(kustomizationListItem("app", "False", "", "")), nil
		}
		events := watch.NewFakeWithChanSize(1, false)
		failed := kustomizationListItem("app", "False", meta.BuildFailedReason, "kustomize build failed")
		events.Modify(&failed)
		kubernetesClient.WatchResourcesFunc = func(context.Context, schema.GroupVersionResource, string, string) (watch.Interface, error) {
			return events, nil
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationWaitPollInterval = time.Hour
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{withTimeout("app", time.Minute)}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the wait fails with the build error
		if err == nil || !strings.Contains(err.Error(), "kustomize build failed") {
			t.Fatalf("Expected the build failure, got %v", err)
		}
	})

	t.Run("ReconnectsWithANewListAfterTheWatchCloses", func(t *testing.T) {
		// Given a first watch that closes and a second list that reports Ready
		kubernetesClient := client.NewMockKubernetesClient()
		listCalls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			listCalls++
			if listCalls == 1 {
				return listOf(kustomizationListItem("app", "False", "", "")), nil
			}
			return listOf(kustomizationListItem("app", "True", "", "")), nil
		}
		watchCalls := 0
		kubernetesClient.WatchResourcesFunc = func(context.Context, schema.GroupVersionResource, string, string) (watch.Interface, error) {
			watchCalls++
			closed := watch.NewFake()
			closed.Stop()
			return closed, nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{withTimeout("app", time.Minute)}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the wait lists again and succeeds
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if listCalls != 2 || watchCalls != 1 {
			t.Errorf("Expected 2 list calls and 1 watch call, got %d and %d", listCalls, watchCalls)
		}
	})

	t.Run("ReconnectsWithANewListAfterAnExpiredWatch", func(t *testing.T) {
		// Given a first watch that reports an expired resource version
		kubernetesClient := client.NewMockKubernetesClient()
		listCalls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			listCalls++
			if listCalls == 1 {
				return listOf(kustomizationListItem("app", "False", "", "")), nil
			}
			return listOf(kustomizationListItem("app", "True", "", "")), nil
		}
		expired := watch.NewFakeWithChanSize(1, false)
		expired.Error(&metav1.Status{Status: metav1.StatusFailure, Code: 410, Reason: metav1.StatusReasonExpired})
		kubernetesClient.WatchResourcesFunc = func(context.Context, schema.GroupVersionResource, string, string) (watch.Interface, error) {
			return expired, nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{withTimeout("app", time.Minute)}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the wait stops the expired watch, lists again, and succeeds
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if listCalls != 2 {
			t.Errorf("Expected 2 list calls, got %d", listCalls)
		}
		if !expired.IsStopped() {
			t.Error("Expected the expired watch to be stopped")
		}
	})

	t.Run("FallsBackToAListWhenTheWatchCannotOpen", func(t *testing.T) {
		// Given a watch that fails to open and a second list that reports Ready
		kubernetesClient := client.NewMockKubernetesClient()
		listCalls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			listCalls++
			if listCalls == 1 {
				return listOf(kustomizationListItem("app", "False", "", "")), nil
			}
			return listOf(kustomizationListItem("app", "True", "", "")), nil
		}
		kubernetesClient.WatchResourcesFunc = func(context.Context, schema.GroupVersionResource, string, string) (watch.Interface, error) {
			return nil, fmt.Errorf("watch refused")
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{withTimeout("app", time.Minute)}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the next poll tick lists again and the wait succeeds
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if listCalls != 2 {
			t.Errorf("Expected 2 list calls, got %d", listCalls)
		}
	})

	t.Run("ReconciliationFailedBudgetExpiresWhileTheWatchIsQuiet", func(t *testing.T) {
		// Given a ReconciliationFailed kustomization and an open watch that sends no events
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(kustomizationListItem("app", "False", meta.ReconciliationFailedReason, "dry-run failed")), nil
		}
		quiet := watch.NewFake()
		kubernetesClient.WatchResourcesFunc = func(context.Context, schema.GroupVersionResource, string, string) (watch.Interface, error) {
			return quiet, nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{withTimeout("app", time.Minute)}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then a poll tick checks the cached state and the failure budget ends the wait
		if err == nil || !strings.Contains(err.Error(), "dry-run failed") {
			t.Fatalf("Expected the error-budget failure, got %v", err)
		}
	})

	t.Run("ContextCancelledDuringAWatchStopsTheWatch", func(t *testing.T) {
		// Given an open watch that sends no events and a context that ends during the wait
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(kustomizationListItem("app", "False", "", "")), nil
		}
		quiet := watch.NewFake()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		kubernetesClient.WatchResourcesFunc = func(context.Context, schema.GroupVersionResource, string, string) (watch.Interface, error) {
			time.AfterFunc(20*time.Millisecond, cancel)
			return quiet, nil
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationWaitPollInterval = time.Hour
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{withTimeout("app", time.Minute)}}

		// When the wait runs
		err := manager.WaitForKustomizations(ctx, blueprint, presenter.NewGroup(ctx, "kustomization:"))

		// Then the wait returns the context error and stops the watch
		if err != context.Canceled {
			t.Fatalf("Expected context.Canceled, got %v", err)
		}
		if !quiet.IsStopped() {
			t.Error("Expected the wait to stop the watch")
		}
	})

	t.Run("TimeoutNamesTheStuckKustomization", func(t *testing.T) {
		// Given a kustomization that never becomes Ready
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(kustomizationListItem("test-kustomization", "False", "", "")), nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", 50*time.Millisecond),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then a timeout error names the stuck kustomization and does not report a failure
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "test-kustomization") {
			t.Errorf("Expected timeout error to name the stuck kustomization, got: %v", err)
		}
		if errors.Is(err, kubernetes.ErrKustomizationFailed) {
			t.Errorf("Expected a kustomization in progress not to match ErrKustomizationFailed, got: %v", err)
		}
	})

	t.Run("TimeoutSurfacesNotReadyCondition", func(t *testing.T) {
		// Given a kustomization stuck not-Ready with a diagnostic Flux condition
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(kustomizationListItem("dns", "False", "ReconciliationFailed", "dependency 'flux-system/pki-base' is not ready")), nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			item := kustomizationListItem(name, "False", "ReconciliationFailed", "dependency 'flux-system/pki-base' is not ready")
			return &item, nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("dns", 50*time.Millisecond),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the error names the kustomization and surfaces the condition reason and message
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "dns") {
			t.Errorf("Expected error to name the stuck kustomization, got: %v", err)
		}
		if !strings.Contains(err.Error(), "ReconciliationFailed") || !strings.Contains(err.Error(), "is not ready") {
			t.Errorf("Expected error to surface the condition reason and message, got: %v", err)
		}
	})

	t.Run("TimeoutNamesKustomizationsWithoutProbingOnceDescribeBudgetIsSpent", func(t *testing.T) {
		// Given two kustomizations that never become Ready and a describe budget of zero
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(
				kustomizationListItem("dns", "False", "", ""),
				kustomizationListItem("pki-base", "False", "", ""),
			), nil
		}
		called := false
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			called = true
			return &unstructured.Unstructured{}, nil
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.NotReadyDescribeBudget = 0
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("dns", 50*time.Millisecond),
			withTimeout("pki-base", 50*time.Millisecond),
		}}

		// When the wait times out
		err := wait(manager, blueprint)

		// Then the API is never probed and the error names every kustomization
		if err == nil {
			t.Fatal("Expected a timeout error, got nil")
		}
		if called {
			t.Error("Expected no GetResource calls once the budget is exhausted")
		}
		if !strings.Contains(err.Error(), "dns") || !strings.Contains(err.Error(), "pki-base") {
			t.Errorf("Expected both kustomizations named, got: %v", err)
		}
	})

	// withHelmRelease returns a GetResource func that serves each kustomization with one HelmRelease
	// in its inventory, and that HelmRelease with the given conditions.
	withHelmRelease := func(conditions ...map[string]any) func(schema.GroupVersionResource, string, string) (*unstructured.Unstructured, error) {
		return func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "helmreleases" {
				items := make([]any, len(conditions))
				for i, c := range conditions {
					items[i] = c
				}
				return &unstructured.Unstructured{Object: map[string]any{
					"apiVersion": "helm.toolkit.fluxcd.io/v2",
					"kind":       "HelmRelease",
					"metadata":   map[string]any{"name": name, "namespace": ns},
					"status":     map[string]any{"conditions": items},
				}}, nil
			}
			item := kustomizationListItem(name, "False", "HealthCheckFailed", "health check failed")
			item.Object["status"].(map[string]any)["inventory"] = map[string]any{
				"entries": []any{map[string]any{"id": "observability_grafana_helm.toolkit.fluxcd.io_HelmRelease"}},
			}
			return &item, nil
		}
	}
	condition := func(condType, status, reason, message string) map[string]any {
		return map[string]any{"type": condType, "status": status, "reason": reason, "message": message, "lastTransitionTime": "2026-11-06T00:00:00Z"}
	}

	t.Run("TimeoutWithAFailedHelmReleaseFailsTheKustomization", func(t *testing.T) {
		// Given a kustomization that never becomes Ready, with a HelmRelease that failed to install
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(kustomizationListItem("observability", "False", "HealthCheckFailed", "health check failed")), nil
		}
		kubernetesClient.GetResourceFunc = withHelmRelease(condition("Ready", "False", "InstallFailed", "image pull\nfailed"))
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("observability", 50*time.Millisecond),
		}}

		// When the wait times out
		err := wait(manager, blueprint)

		// Then the error matches ErrKustomizationFailed and names the kustomization and the HelmRelease
		if !errors.Is(err, kubernetes.ErrKustomizationFailed) {
			t.Fatalf("Expected an error that matches ErrKustomizationFailed, got: %v", err)
		}
		for _, want := range []string{"observability", "InstallFailed", "HelmRelease observability/grafana", "image pull failed"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Expected the error to contain %q, got: %v", want, err)
			}
		}
	})

	t.Run("ExhaustedBudgetWithAFailedHelmReleaseFailsTheKustomization", func(t *testing.T) {
		// Given a kustomization with a HelmRelease that failed to install, and a second kustomization
		// that reports ReconciliationFailed until its error budget runs out
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(
				kustomizationListItem("observability", "False", "HealthCheckFailed", "health check failed"),
				kustomizationListItem("dashboards", "False", meta.ReconciliationFailedReason, "no matches for kind"),
			), nil
		}
		kubernetesClient.GetResourceFunc = withHelmRelease(condition("Ready", "False", "InstallFailed", "install failed"))
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("observability", 5*time.Second),
			withTimeout("dashboards", time.Second),
		}}

		// When the wait fails on the error budget
		err := wait(manager, blueprint)

		// Then the error matches ErrKustomizationFailed and names the failed HelmRelease
		if !errors.Is(err, kubernetes.ErrKustomizationFailed) {
			t.Fatalf("Expected an error that matches ErrKustomizationFailed, got: %v", err)
		}
		if !strings.Contains(err.Error(), "HelmRelease observability/grafana") || !strings.Contains(err.Error(), "InstallFailed") {
			t.Errorf("Expected the failed HelmRelease named, got: %v", err)
		}
		if strings.Contains(err.Error(), "timeout waiting for kustomizations") {
			t.Errorf("Expected the error-budget path, not the generic timeout error, got: %v", err)
		}
	})

	t.Run("TimeoutWithAStalledHelmReleaseFailsTheKustomization", func(t *testing.T) {
		// Given a kustomization that never becomes Ready, with a Stalled HelmRelease
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(kustomizationListItem("observability", "False", "HealthCheckFailed", "health check failed")), nil
		}
		kubernetesClient.GetResourceFunc = withHelmRelease(
			condition("Ready", "False", "UpgradeFailed", "upgrade failed"),
			condition("Stalled", "True", "RetriesExceeded", "retries exhausted"),
		)
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("observability", 50*time.Millisecond),
		}}

		// When the wait times out
		err := wait(manager, blueprint)

		// Then the error matches ErrKustomizationFailed and reports the Stalled reason
		if !errors.Is(err, kubernetes.ErrKustomizationFailed) {
			t.Fatalf("Expected an error that matches ErrKustomizationFailed, got: %v", err)
		}
		if !strings.Contains(err.Error(), "RetriesExceeded") || !strings.Contains(err.Error(), "retries exhausted") {
			t.Errorf("Expected the Stalled reason and message, got: %v", err)
		}
	})

	t.Run("TimeoutWithAProgressingHelmReleaseIsNotAFailure", func(t *testing.T) {
		// Given a kustomization that never becomes Ready, with a HelmRelease that is still in progress
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(kustomizationListItem("observability", "False", "HealthCheckFailed", "health check failed")), nil
		}
		kubernetesClient.GetResourceFunc = withHelmRelease(condition("Ready", "Unknown", "Progressing", "installing"))
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("observability", 50*time.Millisecond),
		}}

		// When the wait times out
		err := wait(manager, blueprint)

		// Then the error is the timeout error, which does not match ErrKustomizationFailed
		if err == nil {
			t.Fatal("Expected a timeout error, got nil")
		}
		if errors.Is(err, kubernetes.ErrKustomizationFailed) {
			t.Errorf("Expected a HelmRelease in progress not to match ErrKustomizationFailed, got: %v", err)
		}
		if !strings.Contains(err.Error(), "timeout waiting for kustomizations") {
			t.Errorf("Expected the timeout error, got: %v", err)
		}
	})

	t.Run("TimeoutSkipsTheHelmReleasesOfAReadyKustomization", func(t *testing.T) {
		// Given a Ready kustomization with a failed HelmRelease, and a kustomization that never becomes Ready
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(
				kustomizationListItem("ready", "True", "", ""),
				kustomizationListItem("stuck", "False", "", ""),
			), nil
		}
		failed := withHelmRelease(condition("Ready", "False", "InstallFailed", "install failed"))
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" && name == "stuck" {
				item := kustomizationListItem(name, "False", "", "")
				return &item, nil
			}
			return failed(gvr, ns, name)
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("ready", 50*time.Millisecond),
			withTimeout("stuck", 50*time.Millisecond),
		}}

		// When the wait times out
		err := wait(manager, blueprint)

		// Then only the kustomization that is not Ready counts, and the error is the timeout error
		if err == nil {
			t.Fatal("Expected a timeout error, got nil")
		}
		if errors.Is(err, kubernetes.ErrKustomizationFailed) {
			t.Errorf("Expected the HelmRelease of a Ready kustomization to be skipped, got: %v", err)
		}
	})

	t.Run("MissingStatus", func(t *testing.T) {
		// Given a kustomization with no status
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(unstructured.Unstructured{Object: map[string]any{"metadata": map[string]any{"name": "test-kustomization"}}}), nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", 50*time.Millisecond),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then a timeout error is returned
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	})

	t.Run("MissingConditions", func(t *testing.T) {
		// Given a kustomization with an empty status
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(unstructured.Unstructured{Object: map[string]any{
				"metadata": map[string]any{"name": "test-kustomization"},
				"status":   map[string]any{},
			}}), nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", 50*time.Millisecond),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then a timeout error is returned
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	})

	t.Run("ConditionTypeNotReady", func(t *testing.T) {
		// Given a kustomization with only a NotReady condition
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(unstructured.Unstructured{Object: map[string]any{
				"metadata": map[string]any{"name": "test-kustomization"},
				"status": map[string]any{
					"conditions": []any{
						map[string]any{"type": "NotReady", "status": "True"},
					},
				},
			}}), nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", 50*time.Millisecond),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then a timeout error is returned
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	})

	t.Run("ConditionReadyFalse", func(t *testing.T) {
		// Given a kustomization with Ready=False and no reason
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(kustomizationListItem("test-kustomization", "False", "", "")), nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("test-kustomization", 50*time.Millisecond),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then a timeout error is returned
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	})

	t.Run("SucceedsForADependencyChain", func(t *testing.T) {
		// Given two Ready kustomizations where k2 depends on k1
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(
				kustomizationListItem("k1", "True", "", ""),
				kustomizationListItem("k2", "True", "", ""),
			), nil
		}
		manager := setup(t, kubernetesClient)
		k2 := withTimeout("k2", 75*time.Millisecond)
		k2.DependsOn = []string{"k1"}
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("k1", 50*time.Millisecond),
			k2,
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("WithBlueprintNilReturnsError", func(t *testing.T) {
		// Given a manager and no blueprint
		manager := setup(t, setupKubernetesMocks(t).KubernetesClient)

		// When the wait runs
		err := wait(manager, nil)

		// Then a blueprint-not-provided error is returned
		if err == nil {
			t.Fatal("Expected error for nil blueprint, got nil")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected 'blueprint not provided' error, got: %v", err)
		}
	})

	t.Run("SkipsDestroyOnlyKustomizations", func(t *testing.T) {
		// Given a Ready regular kustomization and a destroy-only kustomization with a terminal failure
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(
				kustomizationListItem("regular-kustomization", "True", "", ""),
				kustomizationListItem("destroy-only-kustomization", "False", meta.BuildFailedReason, "boom"),
			), nil
		}
		manager := setup(t, kubernetesClient)
		destroyOnlyTrue := true
		destroyOnly := withTimeout("destroy-only-kustomization", 200*time.Millisecond)
		destroyOnly.DestroyOnly = &destroyOnlyTrue
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("regular-kustomization", 200*time.Millisecond),
			destroyOnly,
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the destroy-only kustomization is excluded and the wait succeeds
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("DuplicateNameCompletesOnceBothTicksObserveReady", func(t *testing.T) {
		// Given a blueprint that declares "crds-core" twice and a Ready cluster object
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return listOf(kustomizationListItem("crds-core", "True", "", "")), nil
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			withTimeout("crds-core", 500*time.Millisecond),
			withTimeout("crds-core", 500*time.Millisecond),
		}}

		// When the wait runs
		err := wait(manager, blueprint)

		// Then the duplicate name is collapsed and the wait succeeds
		if err != nil {
			t.Errorf("Expected the duplicate name to be collapsed and the wait to succeed, got %v", err)
		}
	})
}

func TestBaseKubernetesManager_GetHelmReleasesForKustomization(t *testing.T) {
	setup := func(t *testing.T, kubeClient client.KubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		return kubernetes.NewKubernetesManager(kubeClient, setupKubernetesMocks(t).ConfigHandler)
	}
	kustomizationWithRelease := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
			"kind":       "Kustomization",
			"status": map[string]any{
				"inventory": map[string]any{
					"entries": []any{
						map[string]any{
							"id": "test-namespace_test-release_helm.toolkit.fluxcd.io_HelmRelease",
						},
					},
				},
			},
		},
	}

	t.Run("ReturnsHelmReleasesFromInventory", func(t *testing.T) {
		// Given a kustomization whose inventory holds one HelmRelease
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			if gvr.Group == "kustomize.toolkit.fluxcd.io" && gvr.Resource == "kustomizations" {
				return kustomizationWithRelease, nil
			}
			if gvr.Group == "helm.toolkit.fluxcd.io" && gvr.Resource == "helmreleases" {
				return &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "helm.toolkit.fluxcd.io/v2",
						"kind":       "HelmRelease",
						"metadata": map[string]any{
							"name":      "test-release",
							"namespace": "test-namespace",
						},
						"spec": map[string]any{
							"chart": map[string]any{
								"spec": map[string]any{
									"chart": "test-chart",
								},
							},
						},
					},
				}, nil
			}
			return nil, fmt.Errorf("unexpected resource request")
		}
		manager := setup(t, kubernetesClient)

		// When the HelmReleases are requested
		releases, err := manager.GetHelmReleasesForKustomization("test-kustomization", "test-namespace")

		// Then the HelmRelease is returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(releases) != 1 {
			t.Fatalf("Expected 1 release, got %d", len(releases))
		}
		if releases[0].Name != "test-release" {
			t.Errorf("Expected name 'test-release', got %s", releases[0].Name)
		}
	})

	t.Run("GetResourceError", func(t *testing.T) {
		// Given a client that fails to get the kustomization
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("get resource error")
		}
		manager := setup(t, kubernetesClient)

		// When the HelmReleases are requested
		releases, err := manager.GetHelmReleasesForKustomization("test-kustomization", "test-namespace")

		// Then an error is returned with no releases
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if len(releases) != 0 {
			t.Errorf("Expected 0 releases, got %d", len(releases))
		}
	})

	t.Run("ReturnsErrorWhenHelmReleaseGetFails", func(t *testing.T) {
		// Given a kustomization with a HelmRelease that the client fails to get
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				return kustomizationWithRelease, nil
			}
			return nil, fmt.Errorf("get resource error")
		}
		manager := setup(t, kubernetesClient)

		// When the HelmReleases are requested
		_, err := manager.GetHelmReleasesForKustomization("test-kustomization", "test-namespace")

		// Then a get-helm-release error is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get helm release") {
			t.Errorf("Expected get resource error, got: %v", err)
		}
	})

	t.Run("NilKustomizationReturnsNoReleases", func(t *testing.T) {
		// Given a client that returns no kustomization object and no error
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, nil
		}
		manager := setup(t, kubernetesClient)

		// When the HelmReleases are requested
		releases, err := manager.GetHelmReleasesForKustomization("test-kustomization", "test-namespace")

		// Then no error and no releases are returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(releases) != 0 {
			t.Errorf("Expected 0 releases, got %d", len(releases))
		}
	})

	t.Run("KustomizationNotFound", func(t *testing.T) {
		// Given a client that reports the kustomization as not found
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager := setup(t, kubernetesClient)

		// When the HelmReleases are requested
		releases, err := manager.GetHelmReleasesForKustomization("test-kustomization", "test-namespace")

		// Then no error and no releases are returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(releases) != 0 {
			t.Errorf("Expected 0 releases, got %d", len(releases))
		}
	})

	t.Run("FromUnstructuredError", func(t *testing.T) {
		// Given a manager whose conversion from unstructured fails
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{}}, nil
		}
		manager := setup(t, kubernetesClient)
		manager.Shims.FromUnstructured = func(obj map[string]any, target any) error {
			return fmt.Errorf("forced conversion error")
		}

		// When the HelmReleases are requested
		_, err := manager.GetHelmReleasesForKustomization("test-kustomization", "test-namespace")

		// Then an error is returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})
}

func TestBaseKubernetesManager_ApplyGitRepository(t *testing.T) {
	setup := func(t *testing.T, kubeClient client.KubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		return kubernetes.NewKubernetesManager(kubeClient, setupKubernetesMocks(t).ConfigHandler)
	}
	newRepo := func() *sourcev1.GitRepository {
		return &sourcev1.GitRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.GitRepositorySpec{
				URL:      "https://github.com/test/repo",
				Interval: metav1.Duration{Duration: time.Minute},
			},
		}
	}
	notFoundClient := func(applied **unstructured.Unstructured) *client.MockKubernetesClient {
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if applied != nil {
				*applied = obj
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		return kubernetesClient
	}

	t.Run("AppliesGitRepository", func(t *testing.T) {
		// Given a client that accepts the apply
		manager := setup(t, notFoundClient(nil))

		// When a GitRepository is applied
		err := manager.ApplyGitRepository(newRepo())

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ToUnstructuredError", func(t *testing.T) {
		// Given a manager whose conversion to unstructured fails
		manager := setup(t, setupKubernetesMocks(t).KubernetesClient)
		manager.Shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return nil, fmt.Errorf("forced conversion error")
		}

		// When a GitRepository is applied
		err := manager.ApplyGitRepository(newRepo())

		// Then an error is returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ValidateFieldsError", func(t *testing.T) {
		// Given a conversion that returns empty metadata and spec
		manager := setup(t, setupKubernetesMocks(t).KubernetesClient)
		manager.Shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return map[string]any{
				"metadata": map[string]any{},
				"spec":     map[string]any{},
			}, nil
		}

		// When a GitRepository is applied
		err := manager.ApplyGitRepository(newRepo())

		// Then an error is returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ApplyWithRetryError", func(t *testing.T) {
		// Given a client that cannot find the object and rejects the apply
		kubernetesClient := notFoundClient(nil)
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("apply error")
		}
		manager := setup(t, kubernetesClient)

		// When a GitRepository is applied
		err := manager.ApplyGitRepository(newRepo())

		// Then an error is returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("NilRepository", func(t *testing.T) {
		// Given a manager with the production conversion
		manager := setup(t, setupKubernetesMocks(t).KubernetesClient)

		// When a nil GitRepository is applied
		err := manager.ApplyGitRepository(nil)

		// Then the conversion error for a nil pointer is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "ToUnstructured requires a non-nil pointer to an object") {
			t.Errorf("Expected error containing 'ToUnstructured requires a non-nil pointer to an object', got %v", err)
		}
	})

	t.Run("SuccessWithSecretRef", func(t *testing.T) {
		// Given a GitRepository with a secretRef
		var appliedObj *unstructured.Unstructured
		manager := setup(t, notFoundClient(&appliedObj))
		manager.Shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}
		secretName := "test-secret"
		repo := newRepo()
		repo.Spec.SecretRef = &meta.LocalObjectReference{Name: secretName}

		// When the GitRepository is applied
		err := manager.ApplyGitRepository(repo)

		// Then the applied spec holds the secretRef
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if appliedObj == nil {
			t.Fatal("Expected object to be applied")
		}
		spec, ok := appliedObj.Object["spec"].(map[string]any)
		if !ok {
			t.Fatal("Expected spec to be present")
		}
		secretRef, exists := spec["secretRef"]
		if !exists {
			t.Fatal("Expected secretRef to be present in spec")
		}
		secretRefMap, ok := secretRef.(map[string]any)
		if !ok {
			t.Fatalf("Expected secretRef to be a map, got %T", secretRef)
		}
		if secretRefMap["name"] != secretName {
			t.Errorf("Expected secretRef name to be '%s', got '%v'", secretName, secretRefMap["name"])
		}
	})

	t.Run("SuccessWithoutSecretRef", func(t *testing.T) {
		// Given a GitRepository with no secretRef
		var appliedObj *unstructured.Unstructured
		manager := setup(t, notFoundClient(&appliedObj))
		manager.Shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}
		repo := newRepo()
		repo.Spec.SecretRef = nil

		// When the GitRepository is applied
		err := manager.ApplyGitRepository(repo)

		// Then the applied spec has no secretRef
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if appliedObj == nil {
			t.Fatal("Expected object to be applied")
		}
		spec, ok := appliedObj.Object["spec"].(map[string]any)
		if !ok {
			t.Fatal("Expected spec to be present")
		}
		if _, exists := spec["secretRef"]; exists {
			t.Error("Expected secretRef to not be present in spec when nil")
		}
	})
}

func TestBaseKubernetesManager_ApplyOCIRepository(t *testing.T) {
	setup := func(t *testing.T, kubeClient client.KubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		return kubernetes.NewKubernetesManager(kubeClient, setupKubernetesMocks(t).ConfigHandler)
	}
	newRepo := func(name string) *sourcev1.OCIRepository {
		return &sourcev1.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "test-namespace",
			},
			Spec: sourcev1.OCIRepositorySpec{
				URL: "oci://test-registry.com/test-image",
			},
		}
	}

	t.Run("AppliesOCIRepository", func(t *testing.T) {
		// Given a client that cannot find the object and accepts the apply
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		manager := setup(t, kubernetesClient)
		manager.Shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return map[string]any{
				"apiVersion": "source.toolkit.fluxcd.io/v1",
				"kind":       "OCIRepository",
				"metadata": map[string]any{
					"name":      "test-repo",
					"namespace": "test-namespace",
				},
				"spec": map[string]any{
					"url": "oci://test-registry.com/test-image",
				},
			}, nil
		}

		// When an OCIRepository is applied
		err := manager.ApplyOCIRepository(newRepo("test-repo"))

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ToUnstructuredError", func(t *testing.T) {
		// Given a manager whose conversion to unstructured fails
		manager := setup(t, setupKubernetesMocks(t).KubernetesClient)
		manager.Shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return nil, fmt.Errorf("conversion error")
		}

		// When an OCIRepository is applied
		err := manager.ApplyOCIRepository(newRepo("test-repo"))

		// Then a conversion error is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to convert ocirepository to unstructured") {
			t.Errorf("Expected conversion error, got: %v", err)
		}
	})

	t.Run("ValidationError", func(t *testing.T) {
		// Given a conversion that returns an empty name
		manager := setup(t, setupKubernetesMocks(t).KubernetesClient)
		manager.Shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return map[string]any{
				"metadata": map[string]any{
					"name": "",
				},
			}, nil
		}

		// When an OCIRepository is applied
		err := manager.ApplyOCIRepository(newRepo(""))

		// Then a validation error is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid ocirepository fields") {
			t.Errorf("Expected validation error, got: %v", err)
		}
	})

	t.Run("ValidationErrorForNilObject", func(t *testing.T) {
		// Given a conversion that returns a nil object
		manager := setup(t, setupKubernetesMocks(t).KubernetesClient)
		manager.Shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return nil, nil
		}

		// When an OCIRepository is applied
		err := manager.ApplyOCIRepository(newRepo("test-repo"))

		// Then a validation error is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid ocirepository fields") {
			t.Errorf("Expected validation error, got: %v", err)
		}
	})

	t.Run("ValidationErrorForMissingMetadata", func(t *testing.T) {
		// Given a conversion that returns no metadata
		manager := setup(t, setupKubernetesMocks(t).KubernetesClient)
		manager.Shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return map[string]any{
				"spec": map[string]any{
					"url": "oci://test-registry.com/test-image",
				},
			}, nil
		}

		// When an OCIRepository is applied
		err := manager.ApplyOCIRepository(newRepo("test-repo"))

		// Then a validation error is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid ocirepository fields") {
			t.Errorf("Expected validation error, got: %v", err)
		}
	})
}

func TestBaseKubernetesManager_GetKustomizationStatus(t *testing.T) {
	setup := func(t *testing.T, kubeClient client.KubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		return kubernetes.NewKubernetesManager(kubeClient, setupKubernetesMocks(t).ConfigHandler)
	}
	kustomization := func(name string, condition map[string]any) unstructured.Unstructured {
		return unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
			"kind":       "Kustomization",
			"metadata": map[string]any{
				"name": name,
			},
			"status": map[string]any{
				"conditions": []any{condition},
			},
		}}
	}
	clientWith := func(items ...unstructured.Unstructured) *client.MockKubernetesClient {
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: items}, nil
		}
		return kubernetesClient
	}

	t.Run("ReportsReadyKustomization", func(t *testing.T) {
		// Given a Ready kustomization
		manager := setup(t, clientWith(kustomization("k1", map[string]any{"type": "Ready", "status": "True"})))

		// When the status is requested
		status, err := manager.GetKustomizationStatus([]string{"k1"})

		// Then the kustomization is reported as ready
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !status["k1"] {
			t.Errorf("Expected k1 to be ready, got %v", status["k1"])
		}
	})

	t.Run("ListResourcesError", func(t *testing.T) {
		// Given a client that fails to list kustomizations
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return nil, fmt.Errorf("list resources error")
		}
		manager := setup(t, kubernetesClient)

		// When the status is requested
		status, err := manager.GetKustomizationStatus([]string{"k1"})

		// Then the list error is returned with a nil status
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "list resources error") {
			t.Errorf("Expected error containing 'list resources error', got %v", err)
		}
		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})

	t.Run("FromUnstructuredError", func(t *testing.T) {
		// Given a kustomization whose conversion from unstructured fails
		manager := setup(t, clientWith(kustomization("k1", map[string]any{"type": "Ready", "status": "True"})))
		manager.Shims.FromUnstructured = func(obj map[string]any, target any) error {
			return fmt.Errorf("forced conversion error")
		}

		// When the status is requested
		status, err := manager.GetKustomizationStatus([]string{"k1"})

		// Then the conversion error is returned with a nil status
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "forced conversion error") {
			t.Errorf("Expected error containing 'forced conversion error', got %v", err)
		}
		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})

	t.Run("KustomizationNotReady", func(t *testing.T) {
		// Given a kustomization with Ready=False and no reason
		manager := setup(t, clientWith(kustomization("k1", map[string]any{"type": "Ready", "status": "False"})))

		// When the status is requested
		status, err := manager.GetKustomizationStatus([]string{"k1"})

		// Then the kustomization is reported as not ready
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if status["k1"] {
			t.Errorf("Expected k1 to be not ready, got %v", status["k1"])
		}
	})

	t.Run("KustomizationFailed", func(t *testing.T) {
		// Given a kustomization with a ReconciliationFailed condition
		manager := setup(t, clientWith(kustomization("k1", map[string]any{
			"type":    "Ready",
			"status":  "False",
			"reason":  "ReconciliationFailed",
			"message": "kustomization failed",
		})))

		// When the status is requested
		status, err := manager.GetKustomizationStatus([]string{"k1"})

		// Then a failure error is returned with a nil status
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "kustomization k1 failed: kustomization failed") {
			t.Errorf("Expected error containing 'kustomization k1 failed: kustomization failed', got %v", err)
		}
		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})

	t.Run("KustomizationArtifactFailed", func(t *testing.T) {
		// Given a kustomization with an ArtifactFailed condition
		manager := setup(t, clientWith(kustomization("k1", map[string]any{
			"type":    "Ready",
			"status":  "False",
			"reason":  "ArtifactFailed",
			"message": "kustomization path not found: stat /tmp/kustomization-1671333540/kustomize\\ingress\\cleanup: no such file or directory",
		})))

		// When the status is requested
		status, err := manager.GetKustomizationStatus([]string{"k1"})

		// Then a failure error is returned with a nil status
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "kustomization k1 failed: kustomization path not found") {
			t.Errorf("Expected error containing 'kustomization k1 failed: kustomization path not found', got %v", err)
		}
		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})

	t.Run("KustomizationBuildFailed", func(t *testing.T) {
		// Given a kustomization with a BuildFailed condition
		manager := setup(t, clientWith(kustomization("k1", map[string]any{
			"type":    "Ready",
			"status":  "False",
			"reason":  "BuildFailed",
			"message": "accumulating components: no such file or directory",
		})))

		// When the status is requested
		status, err := manager.GetKustomizationStatus([]string{"k1"})

		// Then a failure error is returned with a nil status
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "kustomization k1 failed: accumulating components") {
			t.Errorf("Expected error containing 'kustomization k1 failed: accumulating components', got %v", err)
		}
		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})

	t.Run("KustomizationMissing", func(t *testing.T) {
		// Given a cluster that holds k1 Ready and no k2
		manager := setup(t, clientWith(kustomization("k1", map[string]any{"type": "Ready", "status": "True"})))

		// When the status of k1 and k2 is requested
		status, err := manager.GetKustomizationStatus([]string{"k1", "k2"})

		// Then k1 is ready and k2 is not ready
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !status["k1"] {
			t.Errorf("Expected k1 to be ready, got %v", status["k1"])
		}
		if status["k2"] {
			t.Errorf("Expected k2 to be not ready, got %v", status["k2"])
		}
	})
}

func TestBaseKubernetesManager_KustomizationExists(t *testing.T) {
	t.Run("ReturnsTrueWhenExists", func(t *testing.T) {
		// Given a kubernetes manager whose client returns a resource successfully
		m := setupKubernetesMocks(t)
		m.KubernetesClient.(*client.MockKubernetesClient).GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		}
		manager := kubernetes.NewKubernetesManager(m.KubernetesClient, m.ConfigHandler)

		// When KustomizationExists is called
		exists, err := manager.KustomizationExists("my-app", "flux-system")

		// Then it returns true with no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !exists {
			t.Error("expected exists=true, got false")
		}
	})

	t.Run("ReturnsFalseWhenNotFound", func(t *testing.T) {
		// Given a kubernetes manager whose client returns a not-found error
		m := setupKubernetesMocks(t)
		m.KubernetesClient.(*client.MockKubernetesClient).GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("%q not found", name)
		}
		manager := kubernetes.NewKubernetesManager(m.KubernetesClient, m.ConfigHandler)

		// When KustomizationExists is called
		exists, err := manager.KustomizationExists("my-app", "flux-system")

		// Then it returns false with no error
		if err != nil {
			t.Fatalf("expected no error for not-found, got %v", err)
		}
		if exists {
			t.Error("expected exists=false, got true")
		}
	})

	t.Run("ReturnsErrorOnOtherAPIError", func(t *testing.T) {
		// Given a kubernetes manager whose client returns an unexpected API error
		m := setupKubernetesMocks(t)
		m.KubernetesClient.(*client.MockKubernetesClient).GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("connection refused")
		}
		manager := kubernetes.NewKubernetesManager(m.KubernetesClient, m.ConfigHandler)

		// When KustomizationExists is called
		exists, err := manager.KustomizationExists("my-app", "flux-system")

		// Then it returns the error and false
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if exists {
			t.Error("expected exists=false on error, got true")
		}
	})
}
