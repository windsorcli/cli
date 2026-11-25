package kubernetes_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// =============================================================================
// Test Setup
// =============================================================================

// nodeObject builds a Node object with a Ready condition and an InternalIP.
func nodeObject(name, ip string, ready bool) unstructured.Unstructured {
	status := "Unknown"
	if ready {
		status = "True"
	}
	return unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": name},
		"status": map[string]any{
			"conditions": []any{map[string]any{"type": "Ready", "status": status}},
			"addresses":  []any{map[string]any{"type": "InternalIP", "address": ip}},
		},
	}}
}

// podObject builds a pod that is bound to nodeName, with a deletion timestamp when deleting is true.
func podObject(name, nodeName string, deleting bool) unstructured.Unstructured {
	meta := map[string]any{"name": name}
	if deleting {
		meta["deletionTimestamp"] = "2026-11-22T17:09:38Z"
	}
	return unstructured.Unstructured{Object: map[string]any{
		"metadata": meta,
		"spec":     map[string]any{"nodeName": nodeName},
	}}
}

// nodeTestManager returns a manager with a fake clock and a mock client that serves nodes and
// records deleted node names.
func nodeTestManager(t *testing.T, nodes []unstructured.Unstructured, deleted *[]string) (*kubernetes.BaseKubernetesManager, *client.MockKubernetesClient) {
	t.Helper()
	mocks := setupKubernetesMocks(t)
	kubeClient := client.NewMockKubernetesClient()
	kubeClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
		if gvr.Resource == "nodes" {
			return &unstructured.UnstructuredList{Items: nodes}, nil
		}
		return &unstructured.UnstructuredList{}, nil
	}
	kubeClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
		if gvr.Resource == "nodes" && deleted != nil {
			*deleted = append(*deleted, name)
		}
		return nil
	}
	manager := kubernetes.NewKubernetesManager(kubeClient, mocks.ConfigHandler)
	manager.Timing.NodeReadyPollInterval = time.Second
	manager.Timing.NodeRegistrationTimeout = 5 * time.Second
	clock := newFakeClock()
	manager.Shims.TimeNow = clock.Now
	manager.Shims.TimeSleep = clock.Sleep
	return manager, kubeClient
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseKubernetesManager_GetNodeNamesByAddress(t *testing.T) {
	t.Run("ReturnsNamesOfNodesAtAddress", func(t *testing.T) {
		// Given an old and a new node at one address, and another node at a second address
		manager, _ := nodeTestManager(t, []unstructured.Unstructured{
			nodeObject("old", "10.0.0.1", false),
			nodeObject("new", "10.0.0.1", true),
			nodeObject("other", "10.0.0.2", true),
		}, nil)

		// When the names are read for the first address
		names, err := manager.GetNodeNamesByAddress(context.Background(), "10.0.0.1")

		// Then both nodes at the address are returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !reflect.DeepEqual(names, []string{"old", "new"}) {
			t.Errorf("Expected [old new], got %v", names)
		}
	})

	t.Run("ReturnsErrorWhenListFails", func(t *testing.T) {
		// Given a client that cannot list nodes
		manager, kubeClient := nodeTestManager(t, nil, nil)
		kubeClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return nil, fmt.Errorf("connection refused")
		}

		// When the names are read
		_, err := manager.GetNodeNamesByAddress(context.Background(), "10.0.0.1")

		// Then the error is returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})
}

func TestBaseKubernetesManager_RemoveStaleNodes(t *testing.T) {
	t.Run("DeletesNotReadyNodeReplacedByReadyNodeAtSameAddress", func(t *testing.T) {
		// Given a NotReady node under its old name and a Ready node at the same address under a new name
		var deleted []string
		manager, _ := nodeTestManager(t, []unstructured.Unstructured{
			nodeObject("worker-1-abc", "10.0.0.1", false),
			nodeObject("talos-xyz-123", "10.0.0.1", true),
		}, &deleted)

		// When stale nodes are removed for the old name
		got, err := manager.RemoveStaleNodes(context.Background(), "10.0.0.1", []string{"worker-1-abc"})

		// Then only the old node is deleted
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !reflect.DeepEqual(deleted, []string{"worker-1-abc"}) || !reflect.DeepEqual(got, deleted) {
			t.Errorf("Expected to delete [worker-1-abc], deleted %v, returned %v", deleted, got)
		}
	})

	t.Run("DoesNothingWhenNodeKeptItsName", func(t *testing.T) {
		// Given one Ready node that has the previous name
		var deleted []string
		manager, _ := nodeTestManager(t, []unstructured.Unstructured{nodeObject("worker-1", "10.0.0.1", true)}, &deleted)

		// When stale nodes are removed
		got, err := manager.RemoveStaleNodes(context.Background(), "10.0.0.1", []string{"worker-1"})

		// Then nothing is deleted
		if err != nil || len(got) != 0 || len(deleted) != 0 {
			t.Errorf("Expected no deletion, got deleted=%v returned=%v err=%v", deleted, got, err)
		}
	})

	t.Run("NeverDeletesReadyNode", func(t *testing.T) {
		// Given an old node that is still Ready next to a new Ready node
		var deleted []string
		manager, _ := nodeTestManager(t, []unstructured.Unstructured{
			nodeObject("old", "10.0.0.1", true),
			nodeObject("new", "10.0.0.1", true),
		}, &deleted)

		// When stale nodes are removed
		_, err := manager.RemoveStaleNodes(context.Background(), "10.0.0.1", []string{"old"})

		// Then neither node is deleted
		if err != nil || len(deleted) != 0 {
			t.Errorf("Expected no deletion, got deleted=%v err=%v", deleted, err)
		}
	})

	t.Run("KeepsSoleNotReadyNodeWhenNoReplacementRegisters", func(t *testing.T) {
		// Given a NotReady node and no replacement
		var deleted []string
		manager, _ := nodeTestManager(t, []unstructured.Unstructured{nodeObject("old", "10.0.0.1", false)}, &deleted)

		// When stale nodes are removed and the registration timeout ends
		_, err := manager.RemoveStaleNodes(context.Background(), "10.0.0.1", []string{"old"})

		// Then the node is kept
		if err != nil || len(deleted) != 0 {
			t.Errorf("Expected no deletion, got deleted=%v err=%v", deleted, err)
		}
	})

	t.Run("WaitsForReplacementToRegister", func(t *testing.T) {
		// Given a replacement that appears on the third list
		var deleted []string
		manager, kubeClient := nodeTestManager(t, nil, &deleted)
		lists := 0
		kubeClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			lists++
			items := []unstructured.Unstructured{nodeObject("old", "10.0.0.1", false)}
			if lists >= 3 {
				items = append(items, nodeObject("new", "10.0.0.1", true))
			}
			return &unstructured.UnstructuredList{Items: items}, nil
		}

		// When stale nodes are removed
		_, err := manager.RemoveStaleNodes(context.Background(), "10.0.0.1", []string{"old"})

		// Then the old node is deleted once the replacement is Ready
		if err != nil || !reflect.DeepEqual(deleted, []string{"old"}) {
			t.Errorf("Expected to delete [old], got deleted=%v err=%v", deleted, err)
		}
	})

	t.Run("DoesNotTouchNodesAtOtherAddresses", func(t *testing.T) {
		// Given a NotReady node at another address that has a previous name
		var deleted []string
		manager, _ := nodeTestManager(t, []unstructured.Unstructured{
			nodeObject("elsewhere", "10.0.0.9", false),
			nodeObject("new", "10.0.0.1", true),
		}, &deleted)

		// When stale nodes are removed for the first address
		_, err := manager.RemoveStaleNodes(context.Background(), "10.0.0.1", []string{"elsewhere"})

		// Then nothing is deleted
		if err != nil || len(deleted) != 0 {
			t.Errorf("Expected no deletion, got deleted=%v err=%v", deleted, err)
		}
	})

	t.Run("ReturnsErrorWhenDeleteFails", func(t *testing.T) {
		// Given a stale node whose delete fails
		manager, kubeClient := nodeTestManager(t, []unstructured.Unstructured{
			nodeObject("old", "10.0.0.1", false),
			nodeObject("new", "10.0.0.1", true),
		}, nil)
		kubeClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return fmt.Errorf("forbidden")
		}

		// When stale nodes are removed
		_, err := manager.RemoveStaleNodes(context.Background(), "10.0.0.1", []string{"old"})

		// Then the error is returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ReturnsNothingForEmptyPreviousNames", func(t *testing.T) {
		// Given no previous names
		var deleted []string
		manager, _ := nodeTestManager(t, []unstructured.Unstructured{nodeObject("old", "10.0.0.1", false)}, &deleted)

		// When stale nodes are removed
		got, err := manager.RemoveStaleNodes(context.Background(), "10.0.0.1", nil)

		// Then nothing happens
		if err != nil || got != nil || len(deleted) != 0 {
			t.Errorf("Expected no-op, got deleted=%v returned=%v err=%v", deleted, got, err)
		}
	})
}

func TestBaseKubernetesManager_DeleteKustomization_UnreachableNodePods(t *testing.T) {
	// stuckSetup builds a manager whose kustomization never disappears and whose inventory holds one Namespace.
	stuckSetup := func(t *testing.T, nodes []unstructured.Unstructured, pods []unstructured.Unstructured, nsTerminating bool) *kubernetes.BaseKubernetesManager {
		t.Helper()
		manager, kubeClient := nodeTestManager(t, nodes, nil)
		manager.Timing.KustomizationWaitPollInterval = 50 * time.Millisecond
		manager.Timing.KustomizationReconcileTimeout = 100 * time.Millisecond
		withGVRs(kubeClient)
		prev := kubeClient.ListResourcesFunc
		kubeClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			if gvr.Resource == "pods" {
				return &unstructured.UnstructuredList{Items: pods}, nil
			}
			return prev(gvr, namespace)
		}
		kubeClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			switch gvr.Resource {
			case "namespaces":
				meta := map[string]any{"name": name}
				if nsTerminating {
					meta["deletionTimestamp"] = "2026-11-22T17:09:38Z"
				}
				return &unstructured.Unstructured{Object: map[string]any{"metadata": meta}}, nil
			default:
				return inventoryKustomization("_system-telemetry__Namespace"), nil
			}
		}
		return manager
	}

	t.Run("FailsEarlyNamingTheUnreachableNodes", func(t *testing.T) {
		// Given a terminating namespace whose only pods are deleting on NotReady nodes
		manager := stuckSetup(t,
			[]unstructured.Unstructured{nodeObject("ghost-1", "10.0.0.1", false), nodeObject("ghost-2", "10.0.0.2", false), nodeObject("live", "10.0.0.3", true)},
			[]unstructured.Unstructured{podObject("fluent-bit-a", "ghost-1", true), podObject("node-exporter-b", "ghost-2", true)},
			true)

		// When the kustomization delete waits
		err := manager.DeleteKustomization(context.Background(), "telemetry-install", "system-flux")

		// Then it fails before the full wait and names both nodes and the remedy
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "ghost-1, ghost-2") {
			t.Errorf("Expected error to name the nodes, got %v", err)
		}
		for _, want := range []string{"/fluent-bit-a on ghost-1", "/node-exporter-b on ghost-2"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Expected error to name the pod %q with its node, got %v", want, err)
			}
		}
		we, _ := err.(*werror.WindsorError)
		if we == nil || !strings.Contains(we.Remediation, "kubectl delete node ghost-1 ghost-2") {
			t.Errorf("Expected a kubectl delete node remediation, got %v", err)
		}
	})

	t.Run("CapsTheNamedPods", func(t *testing.T) {
		// Given twelve pods that are deleting on one NotReady node
		var pods []unstructured.Unstructured
		for i := 0; i < 12; i++ {
			pods = append(pods, podObject(fmt.Sprintf("pod-%02d", i), "ghost-1", true))
		}
		manager := stuckSetup(t, []unstructured.Unstructured{nodeObject("ghost-1", "10.0.0.1", false)}, pods, true)

		// When the kustomization delete waits
		err := manager.DeleteKustomization(context.Background(), "telemetry-install", "system-flux")

		// Then the error counts all pods, names ten, and counts the rest
		if err == nil || !strings.Contains(err.Error(), "12 pod(s)") || !strings.Contains(err.Error(), "pod-09 on ghost-1, and 2 more") || strings.Contains(err.Error(), "pod-10") {
			t.Errorf("Expected the error to name ten pods and count two more, got %v", err)
		}
	})

	t.Run("IgnoresPodsOnNodesThatNoLongerExist", func(t *testing.T) {
		// Given a pod on a deleted node next to a pod on a NotReady node
		manager := stuckSetup(t,
			[]unstructured.Unstructured{nodeObject("ghost-1", "10.0.0.1", false)},
			[]unstructured.Unstructured{podObject("gone", "deleted-node", true), podObject("stuck", "ghost-1", true)},
			true)

		// When the kustomization delete waits
		err := manager.DeleteKustomization(context.Background(), "telemetry-install", "system-flux")

		// Then it still fails early and names only the NotReady node
		if err == nil || !strings.Contains(err.Error(), "node(s) ghost-1,") || strings.Contains(err.Error(), "deleted-node") {
			t.Errorf("Expected an early failure naming only ghost-1, got %v", err)
		}
	})

	t.Run("KeepsWaitingWhenAPodRunsOnAReadyNode", func(t *testing.T) {
		// Given a pod that is deleting on a Ready node
		manager := stuckSetup(t,
			[]unstructured.Unstructured{nodeObject("ghost-1", "10.0.0.1", false), nodeObject("live", "10.0.0.3", true)},
			[]unstructured.Unstructured{podObject("a", "ghost-1", true), podObject("b", "live", true)},
			true)

		// When the kustomization delete waits
		err := manager.DeleteKustomization(context.Background(), "telemetry-install", "system-flux")

		// Then it ends with the ordinary timeout error, not the unreachable node error
		if err == nil || strings.Contains(err.Error(), "cannot finish deleting") {
			t.Errorf("Expected the ordinary timeout error, got %v", err)
		}
	})

	t.Run("KeepsWaitingWhenAPodIsNotBeingDeleted", func(t *testing.T) {
		// Given a pod on a NotReady node that has no deletion timestamp
		manager := stuckSetup(t,
			[]unstructured.Unstructured{nodeObject("ghost-1", "10.0.0.1", false)},
			[]unstructured.Unstructured{podObject("a", "ghost-1", false)},
			true)

		// When the kustomization delete waits
		err := manager.DeleteKustomization(context.Background(), "telemetry-install", "system-flux")

		// Then it ends with the ordinary timeout error
		if err == nil || strings.Contains(err.Error(), "cannot finish deleting") {
			t.Errorf("Expected the ordinary timeout error, got %v", err)
		}
	})

	t.Run("KeepsWaitingWhenNamespaceIsNotTerminating", func(t *testing.T) {
		// Given a namespace with no deletion timestamp
		manager := stuckSetup(t,
			[]unstructured.Unstructured{nodeObject("ghost-1", "10.0.0.1", false)},
			[]unstructured.Unstructured{podObject("a", "ghost-1", true)},
			false)

		// When the kustomization delete waits
		err := manager.DeleteKustomization(context.Background(), "telemetry-install", "system-flux")

		// Then it ends with the ordinary timeout error
		if err == nil || strings.Contains(err.Error(), "cannot finish deleting") {
			t.Errorf("Expected the ordinary timeout error, got %v", err)
		}
	})
}
