package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	meta "github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseKubernetesManager_ApplyKustomization(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		// Use shorter timeouts for tests
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationReconcileSleep = 50 * time.Millisecond
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kustomization := kustomizev1.Kustomization{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-kustomization",
				Namespace: "test-namespace",
			},
			Spec: kustomizev1.KustomizationSpec{
				Path: "./test-path",
			},
		}

		err := manager.ApplyKustomization(kustomization)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("UnstructuredConversionError", func(t *testing.T) {
		manager := setup(t)
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return nil, fmt.Errorf("forced conversion error")
		}

		kustomization := kustomizev1.Kustomization{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-kustomization",
				Namespace: "test-namespace",
			},
			Spec: kustomizev1.KustomizationSpec{
				Path: "./test-path",
			},
		}

		err := manager.ApplyKustomization(kustomization)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to convert kustomization to unstructured") {
			t.Errorf("Expected conversion error, got: %v", err)
		}
	})

	t.Run("ApplyWithRetryError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("apply error")
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		kustomization := kustomizev1.Kustomization{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-kustomization",
				Namespace: "test-namespace",
			},
			Spec: kustomizev1.KustomizationSpec{
				Path: "./test-path",
			},
		}

		err := manager.ApplyKustomization(kustomization)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ApplyWithRetryExistingResourceConversionError", func(t *testing.T) {
		manager := setup(t)
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
		callCount := 0
		originalToUnstructured := manager.shims.ToUnstructured
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			callCount++
			if callCount == 1 {
				return originalToUnstructured(obj)
			}
			return nil, fmt.Errorf("conversion error")
		}
		manager.client = kubernetesClient

		kustomization := kustomizev1.Kustomization{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-kustomization",
				Namespace: "test-namespace",
			},
			Spec: kustomizev1.KustomizationSpec{
				Path: "./test-path",
			},
		}

		err := manager.ApplyKustomization(kustomization)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to convert existing object to unstructured") {
			t.Errorf("Expected conversion error, got: %v", err)
		}
	})
}

func TestBaseKubernetesManager_WaitForKustomizations(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		// Use shorter timeouts for tests
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond
		manager.kustomizationWaitMinErrorDuration = 150 * time.Millisecond
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationReconcileSleep = 50 * time.Millisecond
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("test-kustomization", "True", "", ""),
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: 200 * time.Millisecond,
					},
				},
			},
		}

		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReportsEachKustomizationToTheCallersGroup", func(t *testing.T) {
		// Given a ready kustomization and a recording presenter
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("test-kustomization", "True", "", ""),
			}}, nil
		}
		manager.client = kubernetesClient
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
		manager := setup(t)
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
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("a", aStatus, "", ""),
				kustomizationListItem("b", bStatus, "", ""),
			}}, nil
		}
		manager.client = kubernetesClient
		ctx, rec := recordProgress(t)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			{Name: "a", Timeout: &blueprintv1alpha1.DurationString{Duration: time.Second}},
			{Name: "b", DependsOn: []string{"a"}, Timeout: &blueprintv1alpha1.DurationString{Duration: time.Second}},
		}}

		// When the wait runs
		if err := manager.WaitForKustomizations(ctx, blueprint, presenter.NewGroup(ctx, "kustomization:")); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then "b" starts only after "a" ends
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
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("test-kustomization", "False", "Progressing", "still reconciling"),
			}}, nil
		}
		manager.client = kubernetesClient
		ctx, rec := recordProgress(t)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{{
			Name:    "test-kustomization",
			Timeout: &blueprintv1alpha1.DurationString{Duration: 200 * time.Millisecond},
		}}}
		layers := presenter.NewGroup(ctx, "kustomization:")

		// When the wait runs out of time and the caller closes its group with the error
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
		// "a" goes Ready on the first list, then flips back on every later list, while "b"
		// only becomes Ready on the second list — forcing a second tick to prove "a"'s flip
		// is ignored rather than un-readying it and stalling the wait.
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		listCalls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			listCalls++
			aStatus, bStatus := "True", "False"
			if listCalls > 1 {
				aStatus, bStatus = "False", "True"
			}
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("a", aStatus, "", ""),
				kustomizationListItem("b", bStatus, "", ""),
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "a", Timeout: &blueprintv1alpha1.DurationString{Duration: 500 * time.Millisecond}},
				{Name: "b", Timeout: &blueprintv1alpha1.DurationString{Duration: 500 * time.Millisecond}},
			},
		}

		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if listCalls < 2 {
			t.Errorf("Expected at least two ticks for \"b\" to become ready, got %d list calls", listCalls)
		}
	})

	t.Run("NotFoundKeepsPollingUntilReady", func(t *testing.T) {
		// Given a kustomization that isn't created yet, then becomes Ready
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			if calls < 2 {
				return &unstructured.UnstructuredList{}, nil
			}
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("test-kustomization", "True", "", ""),
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{Duration: 500 * time.Millisecond},
				},
			},
		}

		// When waiting for kustomizations
		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))

		// Then the wait succeeds once the resource appears, rather than treating a missing
		// list entry as fatal
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if calls < 2 {
			t.Errorf("Expected the list to be polled at least twice, got %d calls", calls)
		}
	})

	t.Run("CRDNotYetRegisteredIsToleratedForFullTimeout", func(t *testing.T) {
		// Given the Kustomization CRD isn't registered yet (a bootstrap race between Flux's own
		// rollout and this wait starting), so every list fails not-found, then the CRD lands and
		// the kustomization is Ready — well past the short error-tolerance floor
		manager := setup(t)
		manager.kustomizationWaitMinErrorDuration = 150 * time.Millisecond
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			if calls <= 5 {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("test-kustomization", "True", "", ""),
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{Duration: 10 * time.Minute},
				},
			},
		}

		// When waiting for kustomizations
		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))

		// Then a not-found list error is free — it never counts against the error budget, so the
		// wait outlasts the short error-tolerance floor and succeeds once the CRD is registered
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("NilListWithNoErrorIsTreatedAsEmpty", func(t *testing.T) {
		// Given a client whose ListResources returns a nil list with no error — an edge case a
		// real client shouldn't produce, but one a test double or future implementation could
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			if calls < 2 {
				return nil, nil
			}
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("test-kustomization", "True", "", ""),
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{Duration: 500 * time.Millisecond},
				},
			},
		}

		// When waiting for kustomizations
		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))

		// Then the nil list is treated as empty (not-ready-yet) rather than panicking
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("AuthErrorGetsTheSameBudgetAsAnyOtherError", func(t *testing.T) {
		// Given ListResources returns a typed auth error (the way the real dynamic client
		// surfaces it) on every poll, with no recovery in sight — mirroring
		// kustomize-controller and helm-controller, which don't special-case auth/RBAC/TLS
		// as terminal either: their own dependency-readiness checks retry-with-backoff on
		// any apiserver error, reserving no-retry for static errors in the object's own spec
		manager := setup(t)
		manager.kustomizationWaitMinErrorDuration = 150 * time.Millisecond
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			return nil, apierrors.NewUnauthorized("cluster credentials rejected")
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{Duration: 5 * time.Second},
				},
			},
		}

		// When waiting for kustomizations
		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))

		// Then the wait retries across the same error budget as any other error — it doesn't
		// fail on the very first occurrence — but still ends once the budget is exceeded,
		// well before the much longer total timeout, with the underlying error surfaced
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
		// Given a Kustomization stuck reporting a Ready=False/BuildFailed condition on every
		// poll (e.g. a spec.components path composed against a source that no longer has it) —
		// a state re-polling can never resolve, unlike a raw list error
		manager := setup(t)
		manager.kustomizationWaitMinErrorDuration = 150 * time.Millisecond
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("crds-core", "False", meta.BuildFailedReason, "kustomize build failed: accumulating components: no such file or directory"),
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "crds-core",
					Timeout: &blueprintv1alpha1.DurationString{Duration: 10 * time.Minute},
				},
			},
		}

		start := time.Now()
		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))
		elapsed := time.Since(start)

		// Then it fails immediately — well inside the error-streak budget, let alone the
		// 10-minute timeout — naming the kustomization, reason, and underlying message
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "crds-core") || !strings.Contains(err.Error(), "no such file or directory") {
			t.Errorf("Expected the failure to name the kustomization and its message, got: %v", err)
		}
		if elapsed >= manager.kustomizationWaitMinErrorDuration {
			t.Errorf("Expected an immediate failure bypassing the error-streak budget, took %s", elapsed)
		}
		if calls > 2 {
			t.Errorf("Expected the wait to stop after the first tick observes the failure, got %d calls", calls)
		}
	})

	t.Run("FailsTheStepOfTheFailedKustomizationOnly", func(t *testing.T) {
		// Given "a" with a BuildFailed condition and "b" still progressing, and a recording presenter
		manager := setup(t)
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("a", "False", meta.BuildFailedReason, "kustomize build failed"),
				kustomizationListItem("b", "False", "Progressing", "still reconciling"),
			}}, nil
		}
		manager.client = kubernetesClient
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
		// Given the other hard-failure reason kustomize-controller reports on Ready=False. An
		// artifact fetch failure never resolves on retry either.
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("test-kustomization", "False", meta.ArtifactFailedReason, "boom"),
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "test-kustomization", Timeout: &blueprintv1alpha1.DurationString{Duration: 10 * time.Minute}},
			},
		}

		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))
		if err == nil {
			t.Fatal("Expected an immediate failure for ArtifactFailed, got nil")
		}
		if !strings.Contains(err.Error(), "boom") {
			t.Errorf("Expected the underlying message surfaced, got: %v", err)
		}
	})

	t.Run("ReconciliationFailedRecoversWithinBudgetSucceeds", func(t *testing.T) {
		// Given a Kustomization that reports ReconciliationFailed for a couple of ticks. This is
		// the same transient admission-webhook race kustomize-controller resolves on its own next
		// retry. The Kustomization then recovers to Ready well inside the error-budget window.
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			if calls < 3 {
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
					kustomizationListItem("test-kustomization", "False", meta.ReconciliationFailedReason, "webhook not ready"),
				}}, nil
			}
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("test-kustomization", "True", "", ""),
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "test-kustomization", Timeout: &blueprintv1alpha1.DurationString{Duration: time.Second}},
			},
		}

		// When waiting for kustomizations
		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))

		// Then the transient failure does not abort the wait. It recovers like a normal retry.
		if err != nil {
			t.Errorf("Expected ReconciliationFailed to be tolerated within the error budget, got %v", err)
		}
		if calls < 3 {
			t.Errorf("Expected multiple polls before recovery, got %d calls", calls)
		}
	})

	t.Run("ReconciliationFailedExceedingBudgetFails", func(t *testing.T) {
		// Given a Kustomization that reports ReconciliationFailed on every poll, past the
		// error-budget window. This is a genuinely stuck reconcile, not a transient race.
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("test-kustomization", "False", meta.ReconciliationFailedReason, "boom"),
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "test-kustomization", Timeout: &blueprintv1alpha1.DurationString{Duration: time.Second}},
			},
		}

		// When waiting for kustomizations
		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))

		// Then it eventually fails once the failure streak exceeds the error budget. The error
		// names the underlying message instead of tolerating the failure forever.
		if err == nil {
			t.Fatal("Expected an error once the error budget is exceeded, got nil")
		}
		if !strings.Contains(err.Error(), "boom") {
			t.Errorf("Expected the underlying message surfaced, got: %v", err)
		}
		if strings.Contains(err.Error(), "timeout waiting for kustomizations") {
			t.Errorf("Expected the error-budget path, not the generic timeout error, got: %v", err)
		}
	})

	t.Run("ReconciliationFailedStreakSurvivesReadyFlippingToUnknown", func(t *testing.T) {
		// Given a Kustomization whose Ready condition alternates between ReconciliationFailed and
		// Unknown/Progressing on every poll, as kustomize-controller reports it while it keeps
		// retrying a reconcile that can never succeed
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			if calls%2 == 0 {
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
					kustomizationListItem("test-kustomization", "Unknown", meta.ProgressingReason, "Reconciliation in progress"),
				}}, nil
			}
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("test-kustomization", "False", meta.ReconciliationFailedReason, "dry-run failed"),
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "test-kustomization", Timeout: &blueprintv1alpha1.DurationString{Duration: 2 * time.Second}},
			},
		}

		// When waiting for kustomizations
		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))

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
		// Given a blueprint whose longest dependency chain makes the total timeout large, and a
		// dependent Kustomization with a short timeout of its own that fails on every poll
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("base", "True", "", ""),
				kustomizationListItem("slow", "False", meta.ReconciliationFailedReason, "dry-run failed"),
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "base", Timeout: &blueprintv1alpha1.DurationString{Duration: 4 * time.Second}},
				{Name: "slow", DependsOn: []string{"base"}, Timeout: &blueprintv1alpha1.DurationString{Duration: 200 * time.Millisecond}},
			},
		}

		// When waiting for kustomizations
		started := time.Now()
		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))
		elapsed := time.Since(started)

		// Then the wait fails near the failing Kustomization's own timeout, not a quarter of the total
		if err == nil || !strings.Contains(err.Error(), "dry-run failed") {
			t.Fatalf("Expected the error-budget failure, got: %v", err)
		}
		if elapsed > 700*time.Millisecond {
			t.Errorf("Expected failure within the Kustomization's own budget, took %s", elapsed)
		}
	})

	t.Run("FailureBudgetNeverDropsBelowTheMinimum", func(t *testing.T) {
		// Given a failing Kustomization whose own timeout is shorter than the minimum error
		// duration, in a blueprint whose total timeout is large
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("base", "True", "", ""),
				kustomizationListItem("quick", "False", meta.ReconciliationFailedReason, "dry-run failed"),
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "base", Timeout: &blueprintv1alpha1.DurationString{Duration: 4 * time.Second}},
				{Name: "quick", DependsOn: []string{"base"}, Timeout: &blueprintv1alpha1.DurationString{Duration: 20 * time.Millisecond}},
			},
		}

		// When waiting for kustomizations
		started := time.Now()
		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))
		elapsed := time.Since(started)

		// Then the wait fails only after the minimum error duration, measured from the first observed
		// failure one poll in, not at the shorter own timeout
		if err == nil || !strings.Contains(err.Error(), "dry-run failed") {
			t.Fatalf("Expected the error-budget failure, got: %v", err)
		}
		floor := manager.kustomizationWaitMinErrorDuration + manager.kustomizationWaitPollInterval/2
		if elapsed < floor {
			t.Errorf("Expected the wait to last at least %s, failed after %s", floor, elapsed)
		}
	})

	t.Run("FailureStreakRestartsAfterAFullBudgetWithoutFailures", func(t *testing.T) {
		// Given a Kustomization that fails once, reports Progressing for longer than the budget,
		// fails once more, then becomes Ready
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			switch {
			case calls == 1, calls == 17:
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
					kustomizationListItem("test-kustomization", "False", meta.ReconciliationFailedReason, "transient"),
				}}, nil
			case calls > 17:
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
					kustomizationListItem("test-kustomization", "True", "", ""),
				}}, nil
			}
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("test-kustomization", "False", meta.ProgressingReason, ""),
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "test-kustomization", Timeout: &blueprintv1alpha1.DurationString{Duration: 2 * time.Second}},
			},
		}

		// When waiting for kustomizations
		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))

		// Then the old failure no longer counts against the new one, and the wait succeeds
		if err != nil {
			t.Errorf("Expected the stale failure streak to expire, got %v", err)
		}
	})

	t.Run("ProgressingConditionKeepsPollingUntilReady", func(t *testing.T) {
		// Given a Kustomization that reports Ready=False/Progressing — a normal in-flight
		// state, not a terminal failure — before becoming Ready
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			status := "False"
			reason := meta.ProgressingReason
			if calls >= 3 {
				status, reason = "True", ""
			}
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("test-kustomization", status, reason, ""),
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "test-kustomization", Timeout: &blueprintv1alpha1.DurationString{Duration: 500 * time.Millisecond}},
			},
		}

		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))
		if err != nil {
			t.Errorf("Expected Progressing to keep polling rather than fail, got %v", err)
		}
		if calls < 3 {
			t.Errorf("Expected multiple polls before Ready, got %d calls", calls)
		}
	})

	t.Run("TransientErrorIsToleratedThenSucceeds", func(t *testing.T) {
		// Given ListResources fails with a non-fatal error a few times (e.g. a brief
		// apiserver restart) but recovers well within the error-tolerance budget
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			if calls <= 5 {
				return nil, fmt.Errorf("connection refused")
			}
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("test-kustomization", "True", "", ""),
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{Duration: 10 * time.Minute},
				},
			},
		}

		// When waiting for kustomizations
		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))

		// Then the transient errors are tolerated — well past the old fixed 3-strike limit —
		// and the wait succeeds once the list recovers
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("TransientErrorStreakExceedingBudgetEventuallyFails", func(t *testing.T) {
		// Given ListResources fails transiently on every poll, with no recovery in sight, and
		// an error-tolerance budget dominated by the small floor (well below the total timeout)
		manager := setup(t)
		manager.kustomizationWaitMinErrorDuration = 150 * time.Millisecond
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			return nil, fmt.Errorf("connection refused")
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{Duration: 5 * time.Second},
				},
			},
		}

		// When waiting for kustomizations
		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))

		// Then the wait ends once the error streak exceeds its time budget — well before the
		// much longer total timeout — with a message distinct from the generic timeout error
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
		// Given ListResources alternates between short transient-error streaks and single
		// successful ticks, with no streak individually exceeding the error budget
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			calls++
			// Two failures, then one clean (not-yet-ready) tick, repeated, then Ready for good
			// from call 10 on. The clean tick has no error, so it resets the streak.
			if calls >= 10 || calls%3 == 0 {
				readyStatus := "False"
				if calls >= 10 {
					readyStatus = "True"
				}
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
					kustomizationListItem("test-kustomization", readyStatus, "", ""),
				}}, nil
			}
			return nil, fmt.Errorf("connection refused")
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{Duration: 10 * time.Second},
				},
			},
		}

		// When waiting for kustomizations
		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))

		// Then no individual streak ever exceeds the budget, so the wait succeeds
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if calls < 10 {
			t.Errorf("Expected at least 10 calls, got %d", calls)
		}
	})

	t.Run("ContextCancelledInterruptsWait", func(t *testing.T) {
		// Given a kustomization that never becomes Ready and a long internal timeout
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("test-kustomization", "False", "", ""),
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{Duration: 10 * time.Minute},
				},
			},
		}

		// When the parent context is cancelled
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Then the wait returns promptly with the context error instead of blocking
		// until the (long) internal timeout, so a caller's SIGTERM/deadline can end it.
		err := manager.WaitForKustomizations(ctx, blueprint, presenter.NewGroup(ctx, "kustomization:"))
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("test-kustomization", "False", "", ""),
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: 50 * time.Millisecond,
					},
				},
			},
		}

		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
		// The timeout error names which kustomization is still not Ready so the
		// operator isn't left with a bare "timeout waiting for kustomizations".
		if err != nil && !strings.Contains(err.Error(), "test-kustomization") {
			t.Errorf("Expected timeout error to name the stuck kustomization, got: %v", err)
		}
	})

	t.Run("TimeoutSurfacesNotReadyCondition", func(t *testing.T) {
		// Given a kustomization stuck not-Ready with a diagnostic Flux condition
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("dns", "False", "ReconciliationFailed", "dependency 'flux-system/pki-base' is not ready"),
			}}, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			item := kustomizationListItem(name, "False", "ReconciliationFailed", "dependency 'flux-system/pki-base' is not ready")
			return &item, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "dns",
					Timeout: &blueprintv1alpha1.DurationString{Duration: 50 * time.Millisecond},
				},
			},
		}

		// When the wait ends
		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))

		// Then the error names the kustomization and surfaces the condition reason+message
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

	t.Run("MissingStatus", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				{Object: map[string]any{"metadata": map[string]any{"name": "test-kustomization"}}},
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: 50 * time.Millisecond,
					},
				},
			},
		}

		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	})

	t.Run("MissingConditions", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				{Object: map[string]any{
					"metadata": map[string]any{"name": "test-kustomization"},
					"status":   map[string]any{},
				}},
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: 50 * time.Millisecond,
					},
				},
			},
		}

		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	})

	t.Run("ConditionTypeNotReady", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				{Object: map[string]any{
					"metadata": map[string]any{"name": "test-kustomization"},
					"status": map[string]any{
						"conditions": []any{
							map[string]any{"type": "NotReady", "status": "True"},
						},
					},
				}},
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: 50 * time.Millisecond,
					},
				},
			},
		}

		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	})

	t.Run("ConditionReadyFalse", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("test-kustomization", "False", "", ""),
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: 50 * time.Millisecond,
					},
				},
			},
		}

		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	})

	t.Run("WithBlueprintCalculatesTimeout", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("k1", "True", "", ""),
				kustomizationListItem("k2", "True", "", ""),
			}}, nil
		}
		manager.client = kubernetesClient

		timeout1 := 50 * time.Millisecond
		timeout2 := 75 * time.Millisecond
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "k1",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout1,
					},
				},
				{
					Name: "k2",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout2,
					},
					DependsOn: []string{"k1"},
				},
			},
		}

		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		calculatedTimeout := manager.calculateTotalWaitTime(blueprint)
		expectedTimeout := timeout1 + timeout2
		if calculatedTimeout != expectedTimeout {
			t.Errorf("Expected calculated timeout %v, got %v", expectedTimeout, calculatedTimeout)
		}
	})

	t.Run("WithBlueprintNilReturnsError", func(t *testing.T) {
		manager := setup(t)
		err := manager.WaitForKustomizations(context.Background(), nil, presenter.NewGroup(context.Background(), "kustomization:"))
		if err == nil {
			t.Error("Expected error for nil blueprint, got nil")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected 'blueprint not provided' error, got: %v", err)
		}
	})

	t.Run("SkipsDestroyOnlyKustomizations", func(t *testing.T) {
		// Given the namespace already holds a destroy-only kustomization reporting a terminal
		// failure, alongside the regular kustomization this wait actually targets. If the
		// destroy-only entry were mistakenly evaluated, its terminal condition would fail the
		// wait immediately — succeeding here proves it is genuinely excluded, not just unlucky
		// enough to avoid a plain not-Ready timeout.
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("regular-kustomization", "True", "", ""),
				kustomizationListItem("destroy-only-kustomization", "False", meta.BuildFailedReason, "boom"),
			}}, nil
		}
		manager.client = kubernetesClient

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "regular-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: 200 * time.Millisecond,
					},
				},
				{
					Name:        "destroy-only-kustomization",
					DestroyOnly: &destroyOnlyTrue,
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: 200 * time.Millisecond,
					},
				},
			},
		}

		// When waiting for kustomizations
		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))

		// Then the wait succeeds without waiting on the not-Ready destroy-only kustomization
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("DuplicateNameCompletesOnceBothTicksObserveReady", func(t *testing.T) {
		// Given a blueprint whose "crds-core" name is declared twice — e.g. withCrdLayer's
		// synthesized CRD kustomization colliding with a name declared elsewhere — with the
		// cluster object immediately reporting Ready. A non-deduplicated name list would compare
		// a readyKustomizations map (naturally deduplicated, so it can only ever reach 1 entry
		// for this name) against a target count of 2, which is never satisfiable and would hang
		// until the timeout even though nothing is actually unready.
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				kustomizationListItem("crds-core", "True", "", ""),
			}}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "crds-core", Timeout: &blueprintv1alpha1.DurationString{Duration: 500 * time.Millisecond}},
				{Name: "crds-core", Timeout: &blueprintv1alpha1.DurationString{Duration: 500 * time.Millisecond}},
			},
		}

		err := manager.WaitForKustomizations(context.Background(), blueprint, presenter.NewGroup(context.Background(), "kustomization:"))
		if err != nil {
			t.Errorf("Expected the duplicate name to be collapsed and the wait to succeed, got %v", err)
		}
	})
}

func TestBaseKubernetesManager_GetHelmReleasesForKustomization(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		// Return a Kustomization with a valid HelmRelease inventory entry
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			if gvr.Group == "kustomize.toolkit.fluxcd.io" && gvr.Resource == "kustomizations" {
				return &unstructured.Unstructured{
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
				}, nil
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
					},
				}, nil
			}
			return nil, fmt.Errorf("unexpected resource request")
		}
		manager.client = kubernetesClient

		releases, err := manager.GetHelmReleasesForKustomization("test-kustomization", "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(releases) != 1 {
			t.Errorf("Expected 1 release, got %d", len(releases))
		}
	})

	t.Run("GetResourceError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("get resource error")
		}
		manager.client = kubernetesClient

		releases, err := manager.GetHelmReleasesForKustomization("test-kustomization", "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if len(releases) != 0 {
			t.Errorf("Expected 0 releases, got %d", len(releases))
		}
	})

	t.Run("KustomizationNotFound", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		releases, err := manager.GetHelmReleasesForKustomization("test-kustomization", "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(releases) != 0 {
			t.Errorf("Expected 0 releases, got %d", len(releases))
		}
	})

	t.Run("FromUnstructuredError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{
				Object: map[string]any{},
			}, nil
		}
		manager.client = kubernetesClient
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
			return fmt.Errorf("forced conversion error")
		}
		_, err := manager.GetHelmReleasesForKustomization("test-kustomization", "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})
}

func TestBaseKubernetesManager_ApplyGitRepository(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		repo := &sourcev1.GitRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.GitRepositorySpec{
				URL: "https://github.com/test/repo",
				Interval: metav1.Duration{
					Duration: time.Minute,
				},
			},
		}

		err := manager.ApplyGitRepository(repo)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ToUnstructuredError", func(t *testing.T) {
		manager := setup(t)
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return nil, fmt.Errorf("forced conversion error")
		}

		repo := &sourcev1.GitRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.GitRepositorySpec{
				URL: "https://github.com/test/repo",
				Interval: metav1.Duration{
					Duration: time.Minute,
				},
			},
		}

		err := manager.ApplyGitRepository(repo)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ValidateFieldsError", func(t *testing.T) {
		manager := setup(t)
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return map[string]any{
				"metadata": map[string]any{},
				"spec":     map[string]any{},
			}, nil
		}

		repo := &sourcev1.GitRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.GitRepositorySpec{
				URL: "https://github.com/test/repo",
				Interval: metav1.Duration{
					Duration: time.Minute,
				},
			},
		}

		err := manager.ApplyGitRepository(repo)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ApplyWithRetryError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("apply error")
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		repo := &sourcev1.GitRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.GitRepositorySpec{
				URL: "https://github.com/test/repo",
				Interval: metav1.Duration{
					Duration: time.Minute,
				},
			},
		}

		err := manager.ApplyGitRepository(repo)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("NilRepository", func(t *testing.T) {
		manager := setup(t)
		err := manager.ApplyGitRepository(nil)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "ToUnstructured requires a non-nil pointer to an object") {
			t.Errorf("Expected error containing 'ToUnstructured requires a non-nil pointer to an object', got %v", err)
		}
	})

	t.Run("SuccessWithSecretRef", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		var appliedObj *unstructured.Unstructured
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			appliedObj = obj
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}

		secretName := "test-secret"
		repo := &sourcev1.GitRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.GitRepositorySpec{
				URL: "https://github.com/test/repo",
				Interval: metav1.Duration{
					Duration: time.Minute,
				},
				SecretRef: &meta.LocalObjectReference{
					Name: secretName,
				},
			},
		}

		err := manager.ApplyGitRepository(repo)
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
			t.Error("Expected secretRef to be present in spec")
		} else {
			secretRefMap, ok := secretRef.(map[string]any)
			if !ok {
				t.Errorf("Expected secretRef to be a map, got %T", secretRef)
			} else if secretRefMap["name"] != secretName {
				t.Errorf("Expected secretRef name to be '%s', got '%v'", secretName, secretRefMap["name"])
			}
		}
	})

	t.Run("SuccessWithoutSecretRef", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		var appliedObj *unstructured.Unstructured
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			appliedObj = obj
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}

		repo := &sourcev1.GitRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.GitRepositorySpec{
				URL: "https://github.com/test/repo",
				Interval: metav1.Duration{
					Duration: time.Minute,
				},
				SecretRef: nil,
			},
		}

		err := manager.ApplyGitRepository(repo)
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
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		manager.client = kubernetesClient
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
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

		repo := &sourcev1.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.OCIRepositorySpec{
				URL: "oci://test-registry.com/test-image",
			},
		}

		err := manager.ApplyOCIRepository(repo)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ToUnstructuredError", func(t *testing.T) {
		manager := setup(t)
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return nil, fmt.Errorf("conversion error")
		}

		repo := &sourcev1.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.OCIRepositorySpec{
				URL: "oci://test-registry.com/test-image",
			},
		}

		err := manager.ApplyOCIRepository(repo)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to convert ocirepository to unstructured") {
			t.Errorf("Expected conversion error, got: %v", err)
		}
	})

	t.Run("ValidationError", func(t *testing.T) {
		manager := setup(t)
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return map[string]any{
				"metadata": map[string]any{
					"name": "",
				},
			}, nil
		}

		repo := &sourcev1.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.OCIRepositorySpec{
				URL: "oci://test-registry.com/test-image",
			},
		}

		err := manager.ApplyOCIRepository(repo)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid ocirepository fields") {
			t.Errorf("Expected validation error, got: %v", err)
		}
	})

	t.Run("ValidateFieldsError_NilObject", func(t *testing.T) {
		manager := setup(t)
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return nil, nil
		}

		repo := &sourcev1.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.OCIRepositorySpec{
				URL: "oci://test-registry.com/test-image",
			},
		}

		err := manager.ApplyOCIRepository(repo)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid ocirepository fields") {
			t.Errorf("Expected validation error, got: %v", err)
		}
	})

	t.Run("ValidateFieldsError_MissingMetadata", func(t *testing.T) {
		manager := setup(t)
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return map[string]any{
				"spec": map[string]any{
					"url": "oci://test-registry.com/test-image",
				},
			}, nil
		}

		repo := &sourcev1.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.OCIRepositorySpec{
				URL: "oci://test-registry.com/test-image",
			},
		}

		err := manager.ApplyOCIRepository(repo)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid ocirepository fields") {
			t.Errorf("Expected validation error, got: %v", err)
		}
	})
}

func TestBaseKubernetesManager_CheckGitRepositoryStatus(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "source.toolkit.fluxcd.io/v1",
							"kind":       "GitRepository",
							"metadata": map[string]any{
								"name": "repo1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":    "Ready",
										"status":  "True",
										"message": "Ready",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		err := manager.CheckGitRepositoryStatus()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ListResourcesError", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return nil, fmt.Errorf("list resources error")
		}
		manager.client = kubernetesClient

		err := manager.CheckGitRepositoryStatus()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "list resources error") {
			t.Errorf("Expected error containing 'list resources error', got %v", err)
		}
	})

	t.Run("FromUnstructuredError", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "source.toolkit.fluxcd.io/v1",
							"kind":       "GitRepository",
							"metadata": map[string]any{
								"name": "repo1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":    "Ready",
										"status":  "True",
										"message": "Ready",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
			return fmt.Errorf("forced conversion error")
		}

		err := manager.CheckGitRepositoryStatus()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "forced conversion error") {
			t.Errorf("Expected error containing 'forced conversion error', got %v", err)
		}
	})

	t.Run("RepositoryNotReady", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "source.toolkit.fluxcd.io/v1",
							"kind":       "GitRepository",
							"metadata": map[string]any{
								"name": "repo1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":    "Ready",
										"status":  "False",
										"message": "repo not ready",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		err := manager.CheckGitRepositoryStatus()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "repo1") || !strings.Contains(err.Error(), "repo not ready") {
			t.Errorf("Expected error containing repo name and message, got %v", err)
		}
	})

	t.Run("OCIRepositoryNotReady", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		callCount := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			callCount++
			if gvr.Resource == "gitrepositories" {
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{}}, nil
			}
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "source.toolkit.fluxcd.io/v1",
							"kind":       "OCIRepository",
							"metadata": map[string]any{
								"name": "oci-repo1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":    "Ready",
										"status":  "False",
										"message": "oci repo not ready",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		err := manager.CheckGitRepositoryStatus()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "oci-repo1") || !strings.Contains(err.Error(), "oci repo not ready") {
			t.Errorf("Expected error containing oci repo name and message, got %v", err)
		}
	})

	t.Run("OCIRepositoryListError", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		callCount := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			callCount++
			if gvr.Resource == "gitrepositories" {
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{}}, nil
			}
			return nil, fmt.Errorf("oci list error")
		}
		manager.client = kubernetesClient

		err := manager.CheckGitRepositoryStatus()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to list oci repositories") {
			t.Errorf("Expected oci list error, got %v", err)
		}
	})

	t.Run("OCIRepositoryFromUnstructuredError", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		callCount := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			callCount++
			if gvr.Resource == "gitrepositories" {
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{}}, nil
			}
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "source.toolkit.fluxcd.io/v1",
							"kind":       "OCIRepository",
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
			return fmt.Errorf("forced conversion error")
		}

		err := manager.CheckGitRepositoryStatus()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to convert oci repository") {
			t.Errorf("Expected oci conversion error, got %v", err)
		}
	})
}

func TestBaseKubernetesManager_GetKustomizationStatus(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":   "Ready",
										"status": "True",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		status, err := manager.GetKustomizationStatus([]string{"k1"})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !status["k1"] {
			t.Errorf("Expected k1 to be ready, got %v", status["k1"])
		}
	})

	t.Run("ListResourcesError", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return nil, fmt.Errorf("list resources error")
		}
		manager.client = kubernetesClient

		status, err := manager.GetKustomizationStatus([]string{"k1"})
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "list resources error") {
			t.Errorf("Expected error containing 'list resources error', got %v", err)
		}
		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})

	t.Run("FromUnstructuredError", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":   "Ready",
										"status": "True",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
			return fmt.Errorf("forced conversion error")
		}

		status, err := manager.GetKustomizationStatus([]string{"k1"})
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "forced conversion error") {
			t.Errorf("Expected error containing 'forced conversion error', got %v", err)
		}
		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})

	t.Run("KustomizationNotReady", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":   "Ready",
										"status": "False",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		status, err := manager.GetKustomizationStatus([]string{"k1"})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if status["k1"] {
			t.Errorf("Expected k1 to be not ready, got %v", status["k1"])
		}
	})

	t.Run("KustomizationFailed", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":    "Ready",
										"status":  "False",
										"reason":  "ReconciliationFailed",
										"message": "kustomization failed",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		status, err := manager.GetKustomizationStatus([]string{"k1"})
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "kustomization k1 failed: kustomization failed") {
			t.Errorf("Expected error containing 'kustomization k1 failed: kustomization failed', got %v", err)
		}
		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})

	t.Run("KustomizationArtifactFailed", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":    "Ready",
										"status":  "False",
										"reason":  "ArtifactFailed",
										"message": "kustomization path not found: stat /tmp/kustomization-1671333540/kustomize\\ingress\\cleanup: no such file or directory",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		status, err := manager.GetKustomizationStatus([]string{"k1"})
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "kustomization k1 failed: kustomization path not found") {
			t.Errorf("Expected error containing 'kustomization k1 failed: kustomization path not found', got %v", err)
		}
		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})

	t.Run("KustomizationBuildFailed", func(t *testing.T) {
		// A BuildFailed Ready=False condition (a bad spec.components/spec.path in the composed
		// Kustomization itself, distinct from ArtifactFailed's missing-fetched-content case) must
		// also fail fast rather than reading back as merely not-ready.
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":    "Ready",
										"status":  "False",
										"reason":  "BuildFailed",
										"message": "accumulating components: no such file or directory",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		status, err := manager.GetKustomizationStatus([]string{"k1"})
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "kustomization k1 failed: accumulating components") {
			t.Errorf("Expected error containing 'kustomization k1 failed: accumulating components', got %v", err)
		}
		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})

	t.Run("KustomizationMissing", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":   "Ready",
										"status": "True",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		status, err := manager.GetKustomizationStatus([]string{"k1", "k2"})
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
		manager := NewKubernetesManager(m.KubernetesClient, m.ConfigHandler)

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
		manager := NewKubernetesManager(m.KubernetesClient, m.ConfigHandler)

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
		manager := NewKubernetesManager(m.KubernetesClient, m.ConfigHandler)

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

// =============================================================================
// Test Private Methods
// =============================================================================

func TestBaseKubernetesManager_describeNotReadyKustomizations(t *testing.T) {
	t.Run("ExhaustedBudgetNamesWithoutProbing", func(t *testing.T) {
		// Given a diagnostic budget that is already spent before the first probe
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		manager.notReadyDescribeBudget = 0
		kubernetesClient := withGVRs(client.NewMockKubernetesClient())
		called := false
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			called = true
			return &unstructured.Unstructured{}, nil
		}
		manager.client = kubernetesClient

		// When describing not-ready kustomizations
		got := manager.describeNotReadyKustomizations([]string{"dns", "pki-base"}, "flux-system")

		// Then the API is never probed and every kustomization is named plainly,
		// bounding the diagnostic when the API is the thing that's unresponsive.
		if called {
			t.Error("Expected no GetResource calls once the budget is exhausted")
		}
		if !strings.Contains(got, "dns") || !strings.Contains(got, "pki-base") {
			t.Errorf("Expected both kustomizations named, got: %q", got)
		}
	})
}

func TestBaseKubernetesManager_getHelmRelease(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		expectedObj := &unstructured.Unstructured{
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
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return expectedObj, nil
		}
		manager.client = kubernetesClient

		release, err := manager.getHelmRelease("test-release", "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if release == nil {
			t.Error("Expected helm release, got nil")
		}
		if release.Name != "test-release" {
			t.Errorf("Expected name 'test-release', got %s", release.Name)
		}
	})

	t.Run("GetResourceError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("get resource error")
		}
		manager.client = kubernetesClient

		_, err := manager.getHelmRelease("test-release", "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get helm release") {
			t.Errorf("Expected get resource error, got: %v", err)
		}
	})
}

func TestBaseKubernetesManager_calculateTotalWaitTime(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		return manager
	}

	t.Run("EmptyBlueprint", func(t *testing.T) {
		manager := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{},
		}

		timeout := manager.calculateTotalWaitTime(blueprint)
		expected := constants.DefaultKustomizationWaitTotalTimeout
		if timeout != expected {
			t.Errorf("Expected default timeout %v, got %v", expected, timeout)
		}
	})

	t.Run("SingleKustomizationNoDependencies", func(t *testing.T) {
		manager := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "k1",
				},
			},
		}

		timeout := manager.calculateTotalWaitTime(blueprint)
		expected := constants.DefaultFluxKustomizationTimeout
		if timeout != expected {
			t.Errorf("Expected default timeout %v, got %v", expected, timeout)
		}
	})

	t.Run("SingleKustomizationWithCustomTimeout", func(t *testing.T) {
		manager := setup(t)
		customTimeout := 10 * time.Minute
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "k1",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: customTimeout,
					},
				},
			},
		}

		timeout := manager.calculateTotalWaitTime(blueprint)
		if timeout != customTimeout {
			t.Errorf("Expected custom timeout %v, got %v", customTimeout, timeout)
		}
	})

	t.Run("SingleKustomizationWithZeroTimeout", func(t *testing.T) {
		manager := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "k1",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: 0,
					},
				},
			},
		}

		timeout := manager.calculateTotalWaitTime(blueprint)
		expected := constants.DefaultFluxKustomizationTimeout
		if timeout != expected {
			t.Errorf("Expected default timeout %v, got %v", expected, timeout)
		}
	})

	t.Run("TwoKustomizationsWithDependency", func(t *testing.T) {
		manager := setup(t)
		timeout1 := 5 * time.Minute
		timeout2 := 3 * time.Minute
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "k1",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout1,
					},
				},
				{
					Name: "k2",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout2,
					},
					DependsOn: []string{"k1"},
				},
			},
		}

		timeout := manager.calculateTotalWaitTime(blueprint)
		expected := timeout1 + timeout2
		if timeout != expected {
			t.Errorf("Expected sum %v, got %v", expected, timeout)
		}
	})

	t.Run("LongDependencyChain", func(t *testing.T) {
		manager := setup(t)
		timeout1 := 2 * time.Minute
		timeout2 := 3 * time.Minute
		timeout3 := 4 * time.Minute
		timeout4 := 5 * time.Minute
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "k1",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout1,
					},
				},
				{
					Name: "k2",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout2,
					},
					DependsOn: []string{"k1"},
				},
				{
					Name: "k3",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout3,
					},
					DependsOn: []string{"k2"},
				},
				{
					Name: "k4",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout4,
					},
					DependsOn: []string{"k3"},
				},
			},
		}

		timeout := manager.calculateTotalWaitTime(blueprint)
		expected := timeout1 + timeout2 + timeout3 + timeout4
		if timeout != expected {
			t.Errorf("Expected sum %v, got %v", expected, timeout)
		}
	})

	t.Run("MultipleIndependentChains", func(t *testing.T) {
		manager := setup(t)
		timeout1 := 2 * time.Minute
		timeout2 := 3 * time.Minute
		timeout3 := 10 * time.Minute
		timeout4 := 1 * time.Minute
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "k1",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout1,
					},
				},
				{
					Name: "k2",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout2,
					},
					DependsOn: []string{"k1"},
				},
				{
					Name: "k3",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout3,
					},
				},
				{
					Name: "k4",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout4,
					},
					DependsOn: []string{"k3"},
				},
			},
		}

		timeout := manager.calculateTotalWaitTime(blueprint)
		chain1Total := timeout1 + timeout2
		chain2Total := timeout3 + timeout4
		expected := chain2Total
		if chain1Total > chain2Total {
			expected = chain1Total
		}
		if timeout != expected {
			t.Errorf("Expected max chain total %v, got %v", expected, timeout)
		}
	})

	t.Run("ForkedDependencyChain", func(t *testing.T) {
		manager := setup(t)
		timeout1 := 2 * time.Minute
		timeout2 := 3 * time.Minute
		timeout3 := 4 * time.Minute
		timeout4 := 5 * time.Minute
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "k1",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout1,
					},
				},
				{
					Name: "k2",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout2,
					},
					DependsOn: []string{"k1"},
				},
				{
					Name: "k3",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout3,
					},
					DependsOn: []string{"k1"},
				},
				{
					Name: "k4",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout4,
					},
					DependsOn: []string{"k2", "k3"},
				},
			},
		}

		timeout := manager.calculateTotalWaitTime(blueprint)
		chain1Total := timeout1 + timeout2 + timeout4
		chain2Total := timeout1 + timeout3 + timeout4
		expected := chain2Total
		if chain1Total > chain2Total {
			expected = chain1Total
		}
		if timeout != expected {
			t.Errorf("Expected max chain total %v, got %v", expected, timeout)
		}
	})

	t.Run("MixedDefaultAndCustomTimeouts", func(t *testing.T) {
		manager := setup(t)
		customTimeout := 7 * time.Minute
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "k1",
				},
				{
					Name: "k2",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: customTimeout,
					},
					DependsOn: []string{"k1"},
				},
			},
		}

		timeout := manager.calculateTotalWaitTime(blueprint)
		expected := constants.DefaultFluxKustomizationTimeout + customTimeout
		if timeout != expected {
			t.Errorf("Expected sum %v, got %v", expected, timeout)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestKustomizationFailureText(t *testing.T) {
	t.Run("JoinsAMultiLineMessageOntoOneLine", func(t *testing.T) {
		// Given a failure with a multi-line message
		failure := &kustomizationFailedError{name: "b", reason: "ReconciliationFailed", message: "dry-run failed\n  spec.selector: Required value"}

		// When the failure text is built
		text := kustomizationFailureText(failure)

		// Then the reason and the flattened message share one line
		if text != "ReconciliationFailed: dry-run failed spec.selector: Required value" {
			t.Errorf("Unexpected text %q", text)
		}
	})

	t.Run("TruncatesLongMessages", func(t *testing.T) {
		// Given a failure message longer than the limit
		long := strings.Repeat("x", kustomizationWaitMessageLimit+50)
		failure := &kustomizationFailedError{name: "b", reason: "ReconciliationFailed", message: long}

		// When the failure text is built
		text := kustomizationFailureText(failure)

		// Then the message is cut and marked
		if text != "ReconciliationFailed: "+strings.Repeat("x", kustomizationWaitMessageLimit)+"..." {
			t.Errorf("Unexpected text %q", text)
		}
	})
}
