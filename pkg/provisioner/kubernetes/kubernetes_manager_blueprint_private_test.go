package kubernetes

import (
	"fmt"
	"strings"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestBaseKubernetesManager_waitForResumeReconcile(t *testing.T) {
	setup := func(t *testing.T, kubeClient client.KubernetesClient) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(kubeClient, mocks.ConfigHandler)
		manager.Timing.KustomizationWaitPollInterval = 10 * time.Millisecond
		manager.Timing.KustomizationReconcileSleep = 30 * time.Millisecond
		clock := newFakeClock()
		manager.Shims.TimeNow = clock.Now
		manager.Shims.TimeSleep = clock.Sleep
		return manager
	}

	staleObj := func() *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{
			"metadata": map[string]any{"generation": int64(2)},
			"status":   map[string]any{"observedGeneration": int64(1)},
		}}
	}

	t.Run("ReturnsAsSoonAsGenerationSettles", func(t *testing.T) {
		// Given a Kustomization whose observedGeneration catches up on the second read
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			calls++
			if calls >= 2 {
				return &unstructured.Unstructured{Object: map[string]any{
					"metadata": map[string]any{"generation": int64(2)},
					"status":   map[string]any{"observedGeneration": int64(2)},
				}}, nil
			}
			return staleObj(), nil
		}
		manager := setup(t, kubernetesClient)

		// When waiting for the resume to settle
		manager.waitForResumeReconcile("test-kustomization", "test-namespace")

		// Then it stops polling right after the generations match
		if calls != 2 {
			t.Errorf("Expected exactly 2 GetResource calls before settling, got %d", calls)
		}
	})

	t.Run("GivesUpAfterTheBudgetWithoutSettling", func(t *testing.T) {
		// Given a Kustomization whose observedGeneration never catches up
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			calls++
			return staleObj(), nil
		}
		manager := setup(t, kubernetesClient)

		// When waiting for the resume to settle
		manager.waitForResumeReconcile("test-kustomization", "test-namespace")

		// Then it returns once the fake clock passes the reconcile budget
		if calls == 0 {
			t.Error("Expected at least one GetResource call before giving up")
		}
	})

	t.Run("ReturnsImmediatelyOnAReadError", func(t *testing.T) {
		// Given a Kustomization that cannot be read
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			calls++
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager := setup(t, kubernetesClient)

		// When waiting for the resume to settle
		manager.waitForResumeReconcile("test-kustomization", "test-namespace")

		// Then it gives up on the first error
		if calls != 1 {
			t.Errorf("Expected exactly 1 GetResource call before giving up on error, got %d", calls)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestReconcileGenerationSettled(t *testing.T) {
	t.Run("FalseForNilObject", func(t *testing.T) {
		// Given a nil object
		var obj *unstructured.Unstructured

		// When the generation is checked
		settled := reconcileGenerationSettled(obj)

		// Then it is not settled
		if settled {
			t.Error("Expected false for a nil object")
		}
	})

	t.Run("FalseWhenObservedGenerationMissing", func(t *testing.T) {
		// Given an object with no status.observedGeneration
		obj := &unstructured.Unstructured{Object: map[string]any{
			"metadata": map[string]any{"generation": int64(2)},
		}}

		// When the generation is checked
		settled := reconcileGenerationSettled(obj)

		// Then it is not settled
		if settled {
			t.Error("Expected false when status.observedGeneration is missing")
		}
	})

	t.Run("FalseWhenObservedGenerationLagsBehind", func(t *testing.T) {
		// Given an object whose observedGeneration is behind its generation
		obj := &unstructured.Unstructured{Object: map[string]any{
			"metadata": map[string]any{"generation": int64(3)},
			"status":   map[string]any{"observedGeneration": int64(2)},
		}}

		// When the generation is checked
		settled := reconcileGenerationSettled(obj)

		// Then it is not settled
		if settled {
			t.Error("Expected false when observedGeneration lags generation")
		}
	})

	t.Run("TrueWhenObservedGenerationCaughtUp", func(t *testing.T) {
		// Given an object whose observedGeneration matches its generation
		obj := &unstructured.Unstructured{Object: map[string]any{
			"metadata": map[string]any{"generation": int64(3)},
			"status":   map[string]any{"observedGeneration": int64(3)},
		}}

		// When the generation is checked
		settled := reconcileGenerationSettled(obj)

		// Then it is settled
		if !settled {
			t.Error("Expected true when observedGeneration matches generation")
		}
	})
}

func TestReverseTopologicalKustomizations(t *testing.T) {
	t.Run("ReturnsEmptyForEmptyInput", func(t *testing.T) {
		// Given an empty input slice
		input := []blueprintv1alpha1.Kustomization{}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then no error is returned and the output is also empty
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 0 {
			t.Errorf("expected empty output, got %d entries", len(out))
		}
	})

	t.Run("ReturnsSingleNodeUnchanged", func(t *testing.T) {
		// Given a single kustomization with no dependencies
		input := []blueprintv1alpha1.Kustomization{{Name: "only"}}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then the single node is returned unchanged
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 1 || out[0].Name != "only" {
			t.Errorf("expected [only], got %v", kustomizationNames(out))
		}
	})

	t.Run("ReversesIndependentNodesByInputOrder", func(t *testing.T) {
		// Given three independent kustomizations with no dependency relation
		input := []blueprintv1alpha1.Kustomization{{Name: "a"}, {Name: "b"}, {Name: "c"}}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then the output is the input reversed
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := kustomizationNames(out)
		want := []string{"c", "b", "a"}
		if !equalStringSlice(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("PutsDependentBeforeDependencyWhenInputAlreadyTopological", func(t *testing.T) {
		// Given input in apply order: dependency b first, dependent a second
		input := []blueprintv1alpha1.Kustomization{
			{Name: "b"},
			{Name: "a", DependsOn: []string{"b"}},
		}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then a (the dependent) destroys first, then b
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := kustomizationNames(out)
		want := []string{"a", "b"}
		if !equalStringSlice(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("PutsDependentBeforeDependencyWhenInputOutOfOrder", func(t *testing.T) {
		// Given input in non-topological order: dependent a first, dependency b second
		input := []blueprintv1alpha1.Kustomization{
			{Name: "a", DependsOn: []string{"b"}},
			{Name: "b"},
		}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then a still destroys before b
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := kustomizationNames(out)
		want := []string{"a", "b"}
		if !equalStringSlice(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("OrdersDiamondDestroyDependentsFirst", func(t *testing.T) {
		// Given a diamond where a depends on b and c, and b and c depend on d
		input := []blueprintv1alpha1.Kustomization{
			{Name: "d"},
			{Name: "b", DependsOn: []string{"d"}},
			{Name: "c", DependsOn: []string{"d"}},
			{Name: "a", DependsOn: []string{"b", "c"}},
		}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then a destroys first, then the siblings in reverse input order, then d
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := kustomizationNames(out)
		want := []string{"a", "c", "b", "d"}
		if !equalStringSlice(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("HandlesDisconnectedComponents", func(t *testing.T) {
		// Given two chains: x depends on y, and p depends on q
		input := []blueprintv1alpha1.Kustomization{
			{Name: "y"},
			{Name: "x", DependsOn: []string{"y"}},
			{Name: "q"},
			{Name: "p", DependsOn: []string{"q"}},
		}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then each dependent precedes its dependency and the chains appear in reverse input order
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := kustomizationNames(out)
		want := []string{"p", "q", "x", "y"}
		if !equalStringSlice(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("IgnoresMissingDependencyName", func(t *testing.T) {
		// Given a kustomization that depends on a name not present in the slice
		input := []blueprintv1alpha1.Kustomization{
			{Name: "a", DependsOn: []string{"missing"}},
			{Name: "b"},
		}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then the missing dependency is ignored and the slice is reversed
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := kustomizationNames(out)
		want := []string{"b", "a"}
		if !equalStringSlice(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("DetectsTwoNodeCycle", func(t *testing.T) {
		// Given a two-node cycle: a depends on b, b depends on a
		input := []blueprintv1alpha1.Kustomization{
			{Name: "a", DependsOn: []string{"b"}},
			{Name: "b", DependsOn: []string{"a"}},
		}

		// When computing reverse-topological order
		_, err := reverseTopologicalKustomizations(input)

		// Then an error is returned mentioning the cycle
		if err == nil {
			t.Fatalf("expected cycle error, got nil")
		}
		if !strings.Contains(err.Error(), "cycle") {
			t.Errorf("expected error mentioning cycle, got %q", err.Error())
		}
	})

	t.Run("ReturnsSingleNodeSelfLoopWithoutError", func(t *testing.T) {
		// Given a single kustomization that depends on itself
		input := []blueprintv1alpha1.Kustomization{
			{Name: "a", DependsOn: []string{"a"}},
		}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then the node is returned without a cycle error
		if err != nil {
			t.Fatalf("unexpected error on single self-loop: %v", err)
		}
		if len(out) != 1 || out[0].Name != "a" {
			t.Errorf("expected [a], got %v", kustomizationNames(out))
		}
	})

	t.Run("DetectsLargerCycle", func(t *testing.T) {
		// Given a three-node cycle: a → b → c → a
		input := []blueprintv1alpha1.Kustomization{
			{Name: "a", DependsOn: []string{"b"}},
			{Name: "b", DependsOn: []string{"c"}},
			{Name: "c", DependsOn: []string{"a"}},
		}

		// When computing reverse-topological order
		_, err := reverseTopologicalKustomizations(input)

		// Then an error is returned mentioning the cycle
		if err == nil {
			t.Fatalf("expected cycle error, got nil")
		}
		if !strings.Contains(err.Error(), "cycle") {
			t.Errorf("expected error mentioning cycle, got %q", err.Error())
		}
	})

	t.Run("DoesNotMutateInput", func(t *testing.T) {
		// Given an input slice
		input := []blueprintv1alpha1.Kustomization{
			{Name: "b"},
			{Name: "a", DependsOn: []string{"b"}},
		}
		inputNames := kustomizationNames(input)

		// When computing reverse-topological order
		_, err := reverseTopologicalKustomizations(input)

		// Then the input slice is unchanged
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		gotNames := kustomizationNames(input)
		if !equalStringSlice(gotNames, inputNames) {
			t.Errorf("input mutated: was %v, now %v", inputNames, gotNames)
		}
	})
}

func TestDependencyClosure(t *testing.T) {
	t.Run("IncludesStartWithNoDependencies", func(t *testing.T) {
		// Given a kustomization with no DependsOn
		ks := []blueprintv1alpha1.Kustomization{{Name: "solo"}}

		// When dependencyClosure runs
		closure := dependencyClosure(ks, "solo")

		// Then the closure is just itself
		if len(closure) != 1 || !closure["solo"] {
			t.Errorf("expected closure {solo}, got %v", closure)
		}
	})

	t.Run("IncludesADirectDependency", func(t *testing.T) {
		// Given app depends on controller
		ks := []blueprintv1alpha1.Kustomization{
			{Name: "app", DependsOn: []string{"controller"}},
			{Name: "controller"},
		}

		// When dependencyClosure runs from app
		closure := dependencyClosure(ks, "app")

		// Then both are in the closure
		if !closure["app"] || !closure["controller"] {
			t.Errorf("expected closure {app, controller}, got %v", closure)
		}
	})

	t.Run("FollowsATransitiveChain", func(t *testing.T) {
		// Given app depends on controller, which depends on crds
		ks := []blueprintv1alpha1.Kustomization{
			{Name: "app", DependsOn: []string{"controller"}},
			{Name: "controller", DependsOn: []string{"crds"}},
			{Name: "crds"},
		}

		// When dependencyClosure runs from app
		closure := dependencyClosure(ks, "app")

		// Then the closure reaches all three
		if !closure["app"] || !closure["controller"] || !closure["crds"] {
			t.Errorf("expected closure {app, controller, crds}, got %v", closure)
		}
	})

	t.Run("TreatsAMissingDependsOnNameAsNoEdge", func(t *testing.T) {
		// Given app depends on a name absent from the blueprint
		ks := []blueprintv1alpha1.Kustomization{
			{Name: "app", DependsOn: []string{"not-in-blueprint"}},
		}

		// When dependencyClosure runs from app
		closure := dependencyClosure(ks, "app")

		// Then the closure is just app
		if len(closure) != 1 || !closure["app"] {
			t.Errorf("expected closure {app}, got %v", closure)
		}
	})

	t.Run("DoesNotIncludeAnUnrelatedKustomization", func(t *testing.T) {
		// Given a third kustomization with no path from app
		ks := []blueprintv1alpha1.Kustomization{
			{Name: "app", DependsOn: []string{"controller"}},
			{Name: "controller"},
			{Name: "unrelated"},
		}

		// When dependencyClosure runs from app
		closure := dependencyClosure(ks, "app")

		// Then it is not in the closure
		if closure["unrelated"] {
			t.Errorf("expected unrelated excluded from closure, got %v", closure)
		}
	})
}
