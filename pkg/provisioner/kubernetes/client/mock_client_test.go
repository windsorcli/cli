package client

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// =============================================================================
// Public Methods
// =============================================================================

func TestMockKubernetesClient_GetResource(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesClient {
		t.Helper()
		return NewMockKubernetesClient()
	}
	gvr := schema.GroupVersionResource{Group: "g", Version: "v", Resource: "r"}
	ns, name := "ns", "name"
	obj := &unstructured.Unstructured{}

	t.Run("FuncSet", func(t *testing.T) {
		client := setup(t)
		errVal := fmt.Errorf("err")
		client.GetResourceFunc = func(g schema.GroupVersionResource, n, nm string) (*unstructured.Unstructured, error) {
			return obj, errVal
		}
		res, err := client.GetResource(gvr, ns, name)
		if res != obj {
			t.Errorf("Expected obj, got %v", res)
		}
		if err != errVal {
			t.Errorf("Expected err, got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		client := setup(t)
		res, err := client.GetResource(gvr, ns, name)
		if res != nil {
			t.Errorf("Expected nil, got %v", res)
		}
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesClient_ListResources(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesClient {
		t.Helper()
		return NewMockKubernetesClient()
	}
	gvr := schema.GroupVersionResource{Group: "g", Version: "v", Resource: "r"}
	ns := "ns"
	list := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{{}}}

	t.Run("FuncSet", func(t *testing.T) {
		client := setup(t)
		errVal := fmt.Errorf("err")
		client.ListResourcesFunc = func(g schema.GroupVersionResource, n string) (*unstructured.UnstructuredList, error) {
			return list, errVal
		}
		res, err := client.ListResources(gvr, ns)
		if !reflect.DeepEqual(res, list) {
			t.Errorf("Expected list, got %v", res)
		}
		if err != errVal {
			t.Errorf("Expected err, got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		client := setup(t)
		res, err := client.ListResources(gvr, ns)
		if !reflect.DeepEqual(res, &unstructured.UnstructuredList{}) {
			t.Errorf("Expected empty list, got %v", res)
		}
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesClient_ApplyResource(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesClient {
		t.Helper()
		return NewMockKubernetesClient()
	}
	gvr := schema.GroupVersionResource{Group: "g", Version: "v", Resource: "r"}
	obj := &unstructured.Unstructured{}
	opts := metav1.ApplyOptions{}

	t.Run("FuncSet", func(t *testing.T) {
		client := setup(t)
		errVal := fmt.Errorf("err")
		client.ApplyResourceFunc = func(g schema.GroupVersionResource, o *unstructured.Unstructured, op metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, errVal
		}
		res, err := client.ApplyResource(gvr, obj, opts)
		if res != obj {
			t.Errorf("Expected obj, got %v", res)
		}
		if err != errVal {
			t.Errorf("Expected err, got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		client := setup(t)
		res, err := client.ApplyResource(gvr, obj, opts)
		if res != obj {
			t.Errorf("Expected obj, got %v", res)
		}
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesClient_DeleteResource(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesClient {
		t.Helper()
		return NewMockKubernetesClient()
	}
	gvr := schema.GroupVersionResource{Group: "g", Version: "v", Resource: "r"}
	ns, name := "ns", "name"
	opts := metav1.DeleteOptions{}

	t.Run("FuncSet", func(t *testing.T) {
		client := setup(t)
		errVal := fmt.Errorf("err")
		client.DeleteResourceFunc = func(g schema.GroupVersionResource, n, nm string, o metav1.DeleteOptions) error {
			return errVal
		}
		err := client.DeleteResource(gvr, ns, name, opts)
		if err != errVal {
			t.Errorf("Expected err, got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		client := setup(t)
		err := client.DeleteResource(gvr, ns, name, opts)
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesClient_PatchResource(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesClient {
		t.Helper()
		return NewMockKubernetesClient()
	}
	gvr := schema.GroupVersionResource{Group: "g", Version: "v", Resource: "r"}
	ns, name := "ns", "name"
	pt := types.MergePatchType
	data := []byte(`{"foo":"bar"}`)
	opts := metav1.PatchOptions{}
	obj := &unstructured.Unstructured{}

	t.Run("FuncSet", func(t *testing.T) {
		client := setup(t)
		errVal := fmt.Errorf("err")
		client.PatchResourceFunc = func(g schema.GroupVersionResource, n, nm string, p types.PatchType, d []byte, o metav1.PatchOptions) (*unstructured.Unstructured, error) {
			return obj, errVal
		}
		res, err := client.PatchResource(gvr, ns, name, pt, data, opts)
		if res != obj {
			t.Errorf("Expected obj, got %v", res)
		}
		if err != errVal {
			t.Errorf("Expected err, got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		client := setup(t)
		res, err := client.PatchResource(gvr, ns, name, pt, data, opts)
		if res != nil {
			t.Errorf("Expected nil, got %v", res)
		}
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesClient_CheckHealth(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesClient {
		t.Helper()
		return NewMockKubernetesClient()
	}
	ctx := context.Background()
	endpoint := "https://kubernetes.example.com:6443"

	t.Run("FuncSet", func(t *testing.T) {
		client := setup(t)
		errVal := fmt.Errorf("health check failed")
		client.CheckHealthFunc = func(c context.Context, e string) error {
			if c != ctx {
				t.Errorf("Expected context %v, got %v", ctx, c)
			}
			if e != endpoint {
				t.Errorf("Expected endpoint %s, got %s", endpoint, e)
			}
			return errVal
		}
		err := client.CheckHealth(ctx, endpoint)
		if err != errVal {
			t.Errorf("Expected err, got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		client := setup(t)
		err := client.CheckHealth(ctx, endpoint)
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}
