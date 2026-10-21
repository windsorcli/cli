package flux

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestFluxStack_PlanSummary(t *testing.T) {
	t.Run("ReturnsNilForNilBlueprint", func(t *testing.T) {
		// Given a stack with a valid runtime
		m := setupFluxMocks(t)
		s := newTestFluxStack(m)

		// When PlanSummary is called with nil blueprint
		result, hints := s.PlanSummary(nil)

		// Then nil is returned for both
		if result != nil {
			t.Errorf("expected nil results, got %v", result)
		}
		if hints != nil {
			t.Errorf("expected nil hints, got %v", hints)
		}
	})

	t.Run("SkipsDestroyOnlyKustomizations", func(t *testing.T) {
		// Given a blueprint with one normal and one destroyOnly kustomization
		m := setupFluxMocks(t)
		s := newTestFluxStack(m)

		// When PlanSummary is called
		results, _ := s.PlanSummary(testBlueprint())

		// Then only the non-destroyOnly kustomizations appear in results (cleanup-only is excluded)
		for _, r := range results {
			if r.Name == "cleanup-only" {
				t.Errorf("expected cleanup-only to be excluded, but it appeared in results")
			}
		}
	})

	t.Run("ParsesDiffLinesForExistingKustomization", func(t *testing.T) {
		// Given an existing kustomization that returns a unified diff with additions and removals
		m := setupFluxMocks(t)
		m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "flux" {
				// Exit code 1 means changes detected — real *exec.ExitError required for errors.As
				diff := "--- a/deploy.yaml\n+++ b/deploy.yaml\n@@ -1,3 +1,4 @@\n+added line\n-removed line\n unchanged\n"
				return diff, exitError(t, 1)
			}
			return "", nil
		}
		s := newTestFluxStack(m)

		// When PlanSummary is called for the blueprint
		results, _ := s.PlanSummary(testBlueprint())

		// Then the first result has parsed add/remove counts
		if len(results) == 0 {
			t.Fatal("expected at least one result")
		}
		r := results[0]
		if r.Err != nil {
			t.Fatalf("expected no error, got %v", r.Err)
		}
		if r.IsNew {
			t.Errorf("expected IsNew=false for existing kustomization")
		}
		if r.Added != 1 || r.Removed != 1 {
			t.Errorf("expected +1 -1, got +%d -%d", r.Added, r.Removed)
		}
	})

	t.Run("ReportsMissingKustomizationFileForNewKustomization", func(t *testing.T) {
		// Given a kustomization that does not exist in the cluster and whose local path
		// has no kustomization.yaml or .yml of its own
		m := setupFluxMocks(t)
		m.kubernetesManager.KustomizationExistsFunc = func(name, namespace string) (bool, error) {
			return false, nil
		}
		s := newTestFluxStack(m)

		// When PlanSummary is called
		results, err := s.PlanSummary(testBlueprint())

		// Then the same actionable error from captureKustomizeBuild's precondition is
		// recorded against the result, not kustomize's own low-level failure
		if err != nil {
			t.Fatalf("expected no top-level error, got %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected results")
		}
		r := results[0]
		if r.Err == nil {
			t.Fatal("expected an error naming the missing kustomization file")
		}
		if !strings.Contains(r.Err.Error(), "kustomization.yaml") {
			t.Errorf("expected the missing-file error, got %v", r.Err)
		}
	})

	t.Run("TreatsKustomizationAsNewWhenClusterUnreachable", func(t *testing.T) {
		// Given a kubernetes manager that returns an error on KustomizationExists
		m := setupFluxMocks(t)
		m.kubernetesManager.KustomizationExistsFunc = func(name, namespace string) (bool, error) {
			return false, fmt.Errorf("connection refused")
		}
		m.shims.ReadFile = func(name string) ([]byte, error) { return []byte(validKustomizationYAML), nil }
		m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "kustomize" {
				return "apiVersion: v1\nkind: Namespace\n", nil
			}
			return "", nil
		}
		s := newTestFluxStack(m)

		// When PlanSummary is called
		results, _ := s.PlanSummary(testBlueprint())

		// Then the component is treated as new and resources are counted
		if len(results) == 0 {
			t.Fatal("expected results")
		}
		r := results[0]
		if r.Err != nil {
			t.Fatalf("expected no error, got %v", r.Err)
		}
		if !r.IsNew {
			t.Errorf("expected IsNew=true when cluster is unreachable")
		}
	})

	t.Run("CountsResourcesFromKustomizeBuildForNewKustomization", func(t *testing.T) {
		// Given a kustomization that does not exist in the cluster and kustomize build returns two resources
		m := setupFluxMocks(t)
		m.kubernetesManager.KustomizationExistsFunc = func(name, namespace string) (bool, error) {
			return false, nil
		}
		m.shims.ReadFile = func(name string) ([]byte, error) { return []byte(validKustomizationYAML), nil }
		m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "kustomize" {
				return "apiVersion: v1\nkind: Namespace\n---\napiVersion: apps/v1\nkind: Deployment\n", nil
			}
			return "", nil
		}
		s := newTestFluxStack(m)

		// When PlanSummary is called
		results, _ := s.PlanSummary(testBlueprint())

		// Then the result is marked new and Added reflects the resource count
		if len(results) == 0 {
			t.Fatal("expected results")
		}
		r := results[0]
		if r.Err != nil {
			t.Fatalf("expected no error, got %v", r.Err)
		}
		if !r.IsNew {
			t.Errorf("expected IsNew=true")
		}
		if r.Added != 2 {
			t.Errorf("expected Added=2, got %d", r.Added)
		}
	})

	t.Run("ReturnsDegradedRowsAndHintWhenFluxNotInstalled", func(t *testing.T) {
		// Given flux CLI is not on PATH
		m := setupFluxMocks(t)
		m.shims.LookPath = func(file string) (string, error) {
			if file == "flux" {
				return "", fmt.Errorf("not found")
			}
			return "/usr/bin/" + file, nil
		}
		m.kubernetesManager.KustomizationExistsFunc = func(name, namespace string) (bool, error) {
			return true, nil // existing — would need flux
		}
		s := newTestFluxStack(m)

		// When PlanSummary is called
		results, hints := s.PlanSummary(testBlueprint())

		// Then each result is marked Degraded and a hint is returned instead of an error entry
		if len(results) == 0 {
			t.Fatal("expected results")
		}
		for _, r := range results {
			if r.Err != nil {
				t.Errorf("expected no error for degraded row, got %v", r.Err)
			}
			if !r.Degraded {
				t.Errorf("expected Degraded=true for %q", r.Name)
			}
		}
		if len(hints) == 0 {
			t.Error("expected at least one hint when flux is missing")
		}
		found := false
		for _, h := range hints {
			if strings.Contains(h, "flux") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected flux hint, got %v", hints)
		}
	})

	t.Run("RecordsErrorWhenFluxDiffFails", func(t *testing.T) {
		// Given an existing kustomization where flux diff fails with a non-1 exit code
		m := setupFluxMocks(t)
		m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "flux" {
				return "", exitError(t, 2)
			}
			return "", nil
		}
		s := newTestFluxStack(m)

		// When PlanSummary is called
		results, _ := s.PlanSummary(testBlueprint())

		// Then an error is recorded for the affected kustomization
		if len(results) == 0 {
			t.Fatal("expected results")
		}
		if results[0].Err == nil {
			t.Errorf("expected error for failing diff, got nil")
		}
	})
}

func TestFluxStack_PlanComponentSummary(t *testing.T) {
	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		// Given a stack with a valid runtime
		m := setupFluxMocks(t)
		s := newTestFluxStack(m)

		// When PlanComponentSummary is called with nil blueprint
		result := s.PlanComponentSummary(nil, "my-app")

		// Then an error is set on the result
		if result.Err == nil {
			t.Error("expected error for nil blueprint, got nil")
		}
	})

	t.Run("ReturnsErrorForMissingKustomization", func(t *testing.T) {
		// Given a blueprint that does not contain the requested kustomization
		m := setupFluxMocks(t)
		s := newTestFluxStack(m)

		// When PlanComponentSummary is called for a non-existent name
		result := s.PlanComponentSummary(testBlueprint(), "nonexistent")

		// Then an error is set indicating not found
		if result.Err == nil {
			t.Error("expected not-found error, got nil")
		}
		if !strings.Contains(result.Err.Error(), "not found") {
			t.Errorf("expected 'not found' in error, got: %v", result.Err)
		}
	})

	t.Run("MarksNewKustomizationAsNew", func(t *testing.T) {
		// Given a kustomization that does not exist in the cluster (cluster unreachable)
		m := setupFluxMocks(t)
		m.kubernetesManager.KustomizationExistsFunc = func(name, namespace string) (bool, error) {
			return false, fmt.Errorf("cluster not reachable")
		}
		s := newTestFluxStack(m)

		// When PlanComponentSummary is called
		result := s.PlanComponentSummary(testBlueprint(), "my-app")

		// Then IsNew is set
		if !result.IsNew {
			t.Error("expected IsNew=true for unreachable cluster, got false")
		}
		if result.Name != "my-app" {
			t.Errorf("expected Name=my-app, got %q", result.Name)
		}
	})
}

func TestFluxStack_PlanDestroySummary(t *testing.T) {
	t.Run("ReturnsNilForNilBlueprint", func(t *testing.T) {
		m := setupFluxMocks(t)
		s := newTestFluxStack(m)
		got, err := s.PlanDestroySummary(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil, got %#v", got)
		}
	})

	t.Run("BuildsResourceListFromClusterInventory", func(t *testing.T) {
		// Given a kubernetes manager that returns inventory entries for the
		// kustomization
		m := setupFluxMocks(t)
		m.kubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			if name == "my-app" {
				return []kubernetes.InventoryEntry{
					{Namespace: "monitoring", Name: "grafana", Group: "apps", Kind: "Deployment"},
					{Namespace: "monitoring", Name: "grafana-config", Group: "", Kind: "ConfigMap"},
					{Namespace: "", Name: "admin", Group: "rbac.authorization.k8s.io", Kind: "ClusterRole"},
				}, nil
			}
			return nil, nil
		}
		s := newTestFluxStack(m)

		// When PlanDestroySummary runs against the test blueprint
		results, err := s.PlanDestroySummary(testBlueprint())

		// Then the eligible kustomizations are returned and resources are
		// addressed in flux's banner format with action Delete
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// testBlueprint has my-app, infra-base, cleanup-only(destroyOnly).
		// destroyOnly is filtered, so two results.
		if len(results) != 2 {
			t.Fatalf("expected 2 results, got %d: %#v", len(results), results)
		}
		var myApp KustomizePlan
		for _, r := range results {
			if r.Name == "my-app" {
				myApp = r
			}
		}
		if myApp.Name == "" {
			t.Fatalf("expected my-app result, got %#v", results)
		}
		want := []ResourceChange{
			{Address: "Deployment/monitoring/grafana", Action: ActionDelete},
			{Address: "ConfigMap/monitoring/grafana-config", Action: ActionDelete},
			{Address: "ClusterRole/admin", Action: ActionDelete},
		}
		if len(myApp.Resources) != len(want) {
			t.Fatalf("expected %d resources, got %d: %#v", len(want), len(myApp.Resources), myApp.Resources)
		}
		for i, r := range myApp.Resources {
			if r != want[i] {
				t.Errorf("[%d] expected %#v, got %#v", i, want[i], r)
			}
		}
		if myApp.Removed != len(want) {
			t.Errorf("expected Removed=%d, got %d", len(want), myApp.Removed)
		}
	})

	t.Run("MarksKustomizationNewWhenAbsentFromCluster", func(t *testing.T) {
		// Given a kubernetes manager that returns nil entries (kustomization not
		// present in the cluster)
		m := setupFluxMocks(t)
		m.kubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			return nil, nil
		}
		s := newTestFluxStack(m)

		results, err := s.PlanDestroySummary(testBlueprint())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, r := range results {
			if !r.IsNew {
				t.Errorf("expected IsNew=true for absent kustomization %q, got false", r.Name)
			}
			if len(r.Resources) != 0 {
				t.Errorf("expected no resources for absent kustomization %q, got %#v", r.Name, r.Resources)
			}
		}
	})

	t.Run("PropagatesNonConnectivityErrorToCaller", func(t *testing.T) {
		// A structured API error is real, not "cluster gone". It must fail the plan.
		m := setupFluxMocks(t)
		m.kubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			return nil, fmt.Errorf("forbidden")
		}
		s := newTestFluxStack(m)

		_, err := s.PlanDestroySummary(testBlueprint())

		if err == nil {
			t.Fatal("expected error from cluster failure, got nil")
		}
		if !strings.Contains(err.Error(), "forbidden") {
			t.Errorf("expected cluster error to propagate, got %v", err)
		}
	})

	t.Run("StopsQueryingOnceClusterUnreachable", func(t *testing.T) {
		// A refused dial marks the cluster unreachable on the first kustomization.
		// Every remaining kustomization then skips its query and reports not-deployed.
		m := setupFluxMocks(t)
		var visited []string
		m.kubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			visited = append(visited, name)
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: fmt.Errorf("connection refused")}
		}
		s := newTestFluxStack(m)

		results, err := s.PlanDestroySummary(testBlueprint())

		if err != nil {
			t.Fatalf("expected nil error once the cluster is confirmed unreachable, got %v", err)
		}
		// testBlueprint has my-app, infra-base, cleanup-only (destroyOnly, filtered).
		if len(visited) != 1 || visited[0] != "my-app" {
			t.Errorf("expected only the first eligible kustomization to be queried, got %v", visited)
		}
		if len(results) != 2 {
			t.Fatalf("expected 2 results, got %d: %#v", len(results), results)
		}
		for _, r := range results {
			if !r.IsNew {
				t.Errorf("expected IsNew=true for kustomization %q once cluster is unreachable, got false", r.Name)
			}
		}
	})

	t.Run("StopsQueryingOnClientSideTimeout", func(t *testing.T) {
		// A context deadline is a net.Error with Timeout()=true, not a *net.OpError.
		// It must also mark the cluster unreachable.
		m := setupFluxMocks(t)
		var visited []string
		m.kubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			visited = append(visited, name)
			return nil, fmt.Errorf("Get %q: %w", "https://203.0.113.1/", context.DeadlineExceeded)
		}
		s := newTestFluxStack(m)

		results, err := s.PlanDestroySummary(testBlueprint())

		if err != nil {
			t.Fatalf("expected nil error once the cluster is confirmed unreachable, got %v", err)
		}
		if len(visited) != 1 {
			t.Errorf("expected only the first eligible kustomization to be queried, got %v", visited)
		}
		for _, r := range results {
			if !r.IsNew {
				t.Errorf("expected IsNew=true for kustomization %q, got false", r.Name)
			}
		}
	})

	t.Run("PropagatesTLSErrorToCaller", func(t *testing.T) {
		// A TLS/certificate failure implements net.Error but reports Timeout()=false.
		// The server did answer, so this must hard-fail the plan, not mark not-deployed.
		m := setupFluxMocks(t)
		m.kubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			return nil, fakeTimeoutError{msg: "x509: certificate signed by unknown authority", timeout: false}
		}
		s := newTestFluxStack(m)

		_, err := s.PlanDestroySummary(testBlueprint())

		if err == nil {
			t.Fatal("expected TLS failure to propagate as an error, got nil")
		}
		if !strings.Contains(err.Error(), "certificate signed by unknown authority") {
			t.Errorf("expected TLS error to propagate, got %v", err)
		}
	})

	t.Run("PropagatesMidQueryConnectionResetToCaller", func(t *testing.T) {
		// A reset on an already-established connection is a *net.OpError, but not
		// one from dial: the server was reachable moments ago. It must hard-fail the
		// plan rather than latching the cluster as gone for every remaining name.
		m := setupFluxMocks(t)
		m.kubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			return nil, &net.OpError{Op: "read", Net: "tcp", Err: fmt.Errorf("connection reset by peer")}
		}
		s := newTestFluxStack(m)

		_, err := s.PlanDestroySummary(testBlueprint())

		if err == nil {
			t.Fatal("expected connection-reset failure to propagate as an error, got nil")
		}
		if !strings.Contains(err.Error(), "connection reset by peer") {
			t.Errorf("expected reset error to propagate, got %v", err)
		}
	})

	t.Run("TreatsMissingKubeconfigAsNotDeployed", func(t *testing.T) {
		// After `windsor destroy` tears down the cluster's terraform component,
		// the kubeconfig is gone. Subsequent destroy cycles must still produce a
		// plan rather than fail to query a cluster that no longer exists. The
		// kustomizations are by definition not deployed (they live on the cluster
		// that was just destroyed), so each result should be IsNew=true.
		m := setupFluxMocks(t)
		m.kubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			return nil, fmt.Errorf("stat /home/user/.kube/config: %w", os.ErrNotExist)
		}
		s := newTestFluxStack(m)

		results, err := s.PlanDestroySummary(testBlueprint())

		if err != nil {
			t.Fatalf("expected nil error when kubeconfig is missing, got %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected results for blueprint kustomizations, got none")
		}
		for _, r := range results {
			if !r.IsNew {
				t.Errorf("expected IsNew=true for kustomization %q when kubeconfig is missing, got false", r.Name)
			}
			if len(r.Resources) != 0 {
				t.Errorf("expected no resources for kustomization %q, got %#v", r.Name, r.Resources)
			}
		}
	})

	t.Run("FiltersDestroyOnlyAndDestroyFalse", func(t *testing.T) {
		// destroyOnly hooks and pinned destroy=false kustomizations are filtered
		// to match DeleteBlueprint's eligibility gate.
		m := setupFluxMocks(t)
		var visited []string
		m.kubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			visited = append(visited, name)
			return []kubernetes.InventoryEntry{}, nil
		}
		falseVal := false
		trueVal := true
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "t"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "regular"},
				{Name: "pinned-off", Destroy: &blueprintv1alpha1.BoolExpression{Value: &falseVal}},
				{Name: "hook-only", DestroyOnly: &trueVal},
			},
		}
		s := newTestFluxStack(m)

		_, err := s.PlanDestroySummary(bp)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(visited) != 1 || visited[0] != "regular" {
			t.Errorf("expected only 'regular' kustomization to be queried, got %v", visited)
		}
	})
}

func TestFluxStack_PlanDestroyComponentSummary(t *testing.T) {
	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		m := setupFluxMocks(t)
		s := newTestFluxStack(m)
		got := s.PlanDestroyComponentSummary(nil, "x")
		if got.Err == nil {
			t.Error("expected error for nil blueprint, got nil")
		}
	})

	t.Run("ReturnsErrorForMissingKustomization", func(t *testing.T) {
		m := setupFluxMocks(t)
		s := newTestFluxStack(m)
		got := s.PlanDestroyComponentSummary(testBlueprint(), "nonexistent")
		if got.Err == nil || !strings.Contains(got.Err.Error(), "not found") {
			t.Errorf("expected not-found error, got %v", got.Err)
		}
	})

	t.Run("RejectsDestroyOnly", func(t *testing.T) {
		// destroyOnly kustomizations are skipped by DeleteBlueprint, so
		// previewing one would mislead.
		m := setupFluxMocks(t)
		s := newTestFluxStack(m)
		got := s.PlanDestroyComponentSummary(testBlueprint(), "cleanup-only")
		if got.Err == nil || !strings.Contains(got.Err.Error(), "destroyOnly") {
			t.Errorf("expected destroyOnly error, got %v", got.Err)
		}
	})

	t.Run("RejectsDestroyFalsePinned", func(t *testing.T) {
		falseVal := false
		bp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "pinned", Destroy: &blueprintv1alpha1.BoolExpression{Value: &falseVal}},
			},
		}
		m := setupFluxMocks(t)
		s := newTestFluxStack(m)
		got := s.PlanDestroyComponentSummary(bp, "pinned")
		if got.Err == nil || !strings.Contains(got.Err.Error(), "destroy=false") {
			t.Errorf("expected destroy=false error, got %v", got.Err)
		}
	})

	t.Run("ReturnsResourcesForNamedKustomization", func(t *testing.T) {
		m := setupFluxMocks(t)
		m.kubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			return []kubernetes.InventoryEntry{
				{Namespace: "monitoring", Name: "grafana", Group: "apps", Kind: "Deployment"},
			}, nil
		}
		s := newTestFluxStack(m)

		got := s.PlanDestroyComponentSummary(testBlueprint(), "my-app")

		if got.Err != nil {
			t.Fatalf("unexpected error: %v", got.Err)
		}
		if len(got.Resources) != 1 || got.Resources[0].Action != ActionDelete {
			t.Errorf("expected one Delete resource, got %#v", got.Resources)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestCountDiffLines(t *testing.T) {
	t.Run("CountsAddedAndRemovedLines", func(t *testing.T) {
		// Given a unified diff with additions and removals
		diff := "--- a/file.yaml\n+++ b/file.yaml\n+added line\n-removed line\n unchanged\n"

		// When counted
		added, removed := countDiffLines(diff)

		// Then headers are excluded and only content lines counted
		if added != 1 || removed != 1 {
			t.Errorf("expected +1 -1, got +%d -%d", added, removed)
		}
	})

	t.Run("ReturnsZeroForEmptyDiff", func(t *testing.T) {
		// Given an empty string
		added, removed := countDiffLines("")

		// Then both counts are zero
		if added != 0 || removed != 0 {
			t.Errorf("expected 0 0, got %d %d", added, removed)
		}
	})
}

func TestCountKustomizeResources(t *testing.T) {
	t.Run("CountsKindOccurrences", func(t *testing.T) {
		// Given YAML output with two resources
		yaml := "apiVersion: v1\nkind: Namespace\nmetadata:\n  name: foo\n---\napiVersion: apps/v1\nkind: Deployment\n"

		// When counted
		n := countKustomizeResources(yaml)

		// Then two resources are detected
		if n != 2 {
			t.Errorf("expected 2, got %d", n)
		}
	})

	t.Run("ReturnsZeroForEmptyOutput", func(t *testing.T) {
		// Given empty output
		n := countKustomizeResources("")

		// Then zero is returned
		if n != 0 {
			t.Errorf("expected 0, got %d", n)
		}
	})
}

func TestParseFluxDiffResources(t *testing.T) {
	t.Run("ExtractsResourceBannersAndMapsVerbs", func(t *testing.T) {
		// Given flux diff stdout containing the v2.3+ resource banners interleaved
		// with unified-diff content (which must not leak into the resource list)
		diff := strings.Join([]string{
			"► Deployment/monitoring/grafana created",
			"► ConfigMap/monitoring/grafana-config drifted",
			"@@ -1,3 +1,4 @@",
			"+    new: value",
			"-    old: value",
			"► Service/monitoring/legacy-exporter deleted",
			"► Namespace/cluster-system unchanged",
			"► CustomResourceDefinition/foos.example.com configured",
			"",
		}, "\n")

		// When parsed
		got := parseFluxDiffResources(diff)

		// Then unchanged is dropped, configured maps to update, and order is preserved
		want := []ResourceChange{
			{Address: "Deployment/monitoring/grafana", Action: ActionCreate},
			{Address: "ConfigMap/monitoring/grafana-config", Action: ActionUpdate},
			{Address: "Service/monitoring/legacy-exporter", Action: ActionDelete},
			{Address: "CustomResourceDefinition/foos.example.com", Action: ActionUpdate},
		}
		if len(got) != len(want) {
			t.Fatalf("expected %d resources, got %d: %#v", len(want), len(got), got)
		}
		for i, r := range got {
			if r != want[i] {
				t.Errorf("[%d] expected %#v, got %#v", i, want[i], r)
			}
		}
	})

	t.Run("HandlesClusterScopedResourcesWithoutNamespace", func(t *testing.T) {
		// Given a banner for a cluster-scoped resource (Kind/Name without namespace)
		diff := "► ClusterRole/admin created\n"

		// When parsed
		got := parseFluxDiffResources(diff)

		// Then the kind/name token is captured verbatim
		if len(got) != 1 {
			t.Fatalf("expected 1 resource, got %d", len(got))
		}
		if got[0].Address != "ClusterRole/admin" || got[0].Action != ActionCreate {
			t.Errorf("unexpected resource: %#v", got[0])
		}
	})

	t.Run("ReturnsNilForNoBanners", func(t *testing.T) {
		// Given output that contains diff lines but no banners
		diff := "+ added\n- removed\n unchanged\n"

		// When parsed
		got := parseFluxDiffResources(diff)

		// Then nothing is returned
		if got != nil {
			t.Errorf("expected nil, got %#v", got)
		}
	})
}

func TestParseKustomizeBuildResources(t *testing.T) {
	t.Run("EmitsCreateForEachResource", func(t *testing.T) {
		// Given a multi-document YAML stream rendered by kustomize build
		yaml := strings.Join([]string{
			"apiVersion: v1",
			"kind: Namespace",
			"metadata:",
			"  name: monitoring",
			"---",
			"apiVersion: apps/v1",
			"kind: Deployment",
			"metadata:",
			"  name: grafana",
			"  namespace: monitoring",
			"---",
			"apiVersion: rbac.authorization.k8s.io/v1",
			"kind: ClusterRole",
			"metadata:",
			"  name: grafana-reader",
			"",
		}, "\n")

		// When parsed
		got := parseKustomizeBuildResources(yaml)

		// Then namespaced and cluster-scoped resources are addressed correctly
		want := []ResourceChange{
			{Address: "Namespace/monitoring", Action: ActionCreate},
			{Address: "Deployment/monitoring/grafana", Action: ActionCreate},
			{Address: "ClusterRole/grafana-reader", Action: ActionCreate},
		}
		if len(got) != len(want) {
			t.Fatalf("expected %d resources, got %d: %#v", len(want), len(got), got)
		}
		for i, r := range got {
			if r != want[i] {
				t.Errorf("[%d] expected %#v, got %#v", i, want[i], r)
			}
		}
	})

	t.Run("SkipsDocumentsMissingKindOrName", func(t *testing.T) {
		// Given a stream with one well-formed doc and one that lacks metadata.name
		yaml := strings.Join([]string{
			"apiVersion: v1",
			"kind: Namespace",
			"metadata:",
			"  name: foo",
			"---",
			"apiVersion: v1",
			"kind: ConfigMap",
			"metadata: {}",
			"",
		}, "\n")

		// When parsed
		got := parseKustomizeBuildResources(yaml)

		// Then only the well-formed doc is included
		if len(got) != 1 || got[0].Address != "Namespace/foo" {
			t.Errorf("expected only Namespace/foo, got %#v", got)
		}
	})
}
